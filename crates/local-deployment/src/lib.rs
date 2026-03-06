#![warn(clippy::pedantic)]
#![allow(
    clippy::doc_markdown,
    clippy::module_name_repetitions,
    clippy::must_use_candidate,
    clippy::missing_errors_doc,
    clippy::missing_panics_doc,
    clippy::similar_names,
    clippy::too_many_lines
)]

use std::{collections::HashMap, sync::Arc};

use async_trait::async_trait;
use db::{DBService, models::terminal::Terminal};
use deployment::{Deployment, DeploymentError};
use executors::profile::ExecutorConfigs;
use services::services::{
    analytics::{AnalyticsConfig, AnalyticsContext, AnalyticsService, generate_user_id},
    approvals::Approvals,
    auth::AuthContext,
    config::{Config, load_config_from_file, save_config_to_file},
    container::ContainerService,
    events::EventService,
    file_search::FileSearchCache,
    filesystem::FilesystemService,
    git::GitService,
    image::ImageService,
    oauth_credentials::OAuthCredentials,
    orchestrator::{MessageBus, OrchestratorRuntime, RuntimeActionService, SharedMessageBus},
    project::ProjectService,
    queued_message::QueuedMessageService,
    repo::RepoService,
    terminal::{PromptWatcher, process::ProcessManager},
};
use tokio::sync::RwLock;
use utils::{
    api::oauth::LoginStatus,
    assets::{config_path, credentials_path},
    msg_store::MsgStore,
};
use uuid::Uuid;

use crate::container::LocalContainerService;
mod command;
pub mod container;
mod copy;

#[derive(Clone)]
pub struct LocalDeployment {
    config: Arc<RwLock<Config>>,
    user_id: String,
    db: DBService,
    analytics: Option<AnalyticsService>,
    container: LocalContainerService,
    git: GitService,
    project: ProjectService,
    repo: RepoService,
    image: ImageService,
    filesystem: FilesystemService,
    events: EventService,
    file_search_cache: Arc<FileSearchCache>,
    approvals: Approvals,
    queued_message_service: QueuedMessageService,
    auth_context: AuthContext,
    oauth_handoffs: Arc<RwLock<HashMap<Uuid, PendingHandoff>>>,
    orchestrator_runtime: OrchestratorRuntime,
    process_manager: Arc<ProcessManager>,
    message_bus: SharedMessageBus,
    prompt_watcher: PromptWatcher,
}

#[derive(Debug, Clone)]
struct PendingHandoff {
    provider: String,
    app_verifier: String,
}

#[async_trait]
impl Deployment for LocalDeployment {
    async fn new() -> Result<Self, DeploymentError> {
        let config_path = config_path()?;
        let mut raw_config = load_config_from_file(&config_path);

        let profiles = ExecutorConfigs::get_cached();
        if !raw_config.onboarding_acknowledged
            && let Ok(recommended_executor) = profiles.get_recommended_executor_profile()
        {
            raw_config.executor_profile = recommended_executor;
        }

        // Check if app version has changed and set release notes flag
        {
            let current_version = utils::version::APP_VERSION;
            let stored_version = raw_config.last_app_version.as_deref();

            if stored_version != Some(current_version) {
                // Show release notes only if this is an upgrade (not first install)
                raw_config.show_release_notes = stored_version.is_some();
                raw_config.last_app_version = Some(current_version.to_string());
            }
        }

        // Always save config (may have been migrated or version updated)
        save_config_to_file(&raw_config, &config_path)?;

        let config = Arc::new(RwLock::new(raw_config));
        let user_id = generate_user_id();
        let analytics = AnalyticsConfig::new().map(AnalyticsService::new);
        let git = GitService::new();
        let project = ProjectService::new();
        let repo = RepoService::new();
        let msg_stores = Arc::new(RwLock::new(HashMap::new()));
        let filesystem = FilesystemService::new();

        // Create shared components for EventService
        let events_msg_store = Arc::new(MsgStore::new());
        let events_entry_count = Arc::new(RwLock::new(0));

        // Create DB with event hooks
        let db = {
            let hook = EventService::create_hook(
                events_msg_store.clone(),
                events_entry_count.clone(),
                DBService::new().await?, // Temporary DB service for the hook
            );
            DBService::new_with_after_connect(hook).await?
        };

        let image = ImageService::new(db.clone().pool)?;
        {
            let image_service = image.clone();
            tokio::spawn(async move {
                tracing::info!("Starting orphaned image cleanup...");
                if let Err(e) = image_service.delete_orphaned_images().await {
                    tracing::error!("Failed to clean up orphaned images: {}", e);
                }
            });
        }

        let approvals = Approvals::new(msg_stores.clone());
        let queued_message_service = QueuedMessageService::new();

        let oauth_credentials = Arc::new(OAuthCredentials::new(credentials_path()?));
        if let Err(e) = oauth_credentials.load().await {
            tracing::warn!(?e, "failed to load OAuth credentials");
        }

        let profile_cache = Arc::new(RwLock::new(None));
        let auth_context = AuthContext::new(oauth_credentials.clone(), profile_cache.clone());

        let oauth_handoffs = Arc::new(RwLock::new(HashMap::new()));

        // We need to make analytics accessible to the ContainerService
        // TODO: Handle this more gracefully
        let analytics_ctx = analytics.as_ref().map(|s| AnalyticsContext {
            user_id: user_id.clone(),
            analytics_service: s.clone(),
        });
        let container = LocalContainerService::new(
            db.clone(),
            msg_stores.clone(),
            config.clone(),
            git.clone(),
            image.clone(),
            analytics_ctx,
            approvals.clone(),
            queued_message_service.clone(),
        );

        let events = EventService::new(db.clone(), events_msg_store, events_entry_count);

        let file_search_cache = Arc::new(FileSearchCache::new());

        // Create message bus and orchestrator runtime
        let message_bus = Arc::new(MessageBus::new(1000));
        let orchestrator_runtime =
            OrchestratorRuntime::new(Arc::new(db.clone()), message_bus.clone());
        let process_manager = Arc::new(ProcessManager::new());
        let prompt_watcher = PromptWatcher::new(message_bus.clone(), process_manager.clone());
        orchestrator_runtime
            .set_runtime_actions(Arc::new(RuntimeActionService::new(
                Arc::new(db.clone()),
                message_bus.clone(),
                process_manager.clone(),
                prompt_watcher.clone(),
            )))
            .await;

        // Reconcile terminal statuses on startup
        // Reset any terminals that are marked as running but have no actual process
        if let Err(e) = Self::reconcile_terminal_statuses(&db, &process_manager).await {
            tracing::warn!("Failed to reconcile terminal statuses on startup: {}", e);
        }

        let deployment = Self {
            config,
            user_id,
            db,
            analytics,
            container,
            git,
            project,
            repo,
            image,
            filesystem,
            events,
            file_search_cache,
            approvals,
            queued_message_service,
            auth_context,
            oauth_handoffs,
            orchestrator_runtime,
            process_manager,
            message_bus,
            prompt_watcher,
        };

        Ok(deployment)
    }

    fn user_id(&self) -> &str {
        &self.user_id
    }

    fn config(&self) -> &Arc<RwLock<Config>> {
        &self.config
    }

    fn db(&self) -> &DBService {
        &self.db
    }

    fn analytics(&self) -> &Option<AnalyticsService> {
        &self.analytics
    }

    fn container(&self) -> &impl ContainerService {
        &self.container
    }

    fn git(&self) -> &GitService {
        &self.git
    }

    fn project(&self) -> &ProjectService {
        &self.project
    }

    fn repo(&self) -> &RepoService {
        &self.repo
    }

    fn image(&self) -> &ImageService {
        &self.image
    }

    fn filesystem(&self) -> &FilesystemService {
        &self.filesystem
    }

    fn events(&self) -> &EventService {
        &self.events
    }

    fn file_search_cache(&self) -> &Arc<FileSearchCache> {
        &self.file_search_cache
    }

    fn approvals(&self) -> &Approvals {
        &self.approvals
    }

    fn queued_message_service(&self) -> &QueuedMessageService {
        &self.queued_message_service
    }

    fn auth_context(&self) -> &AuthContext {
        &self.auth_context
    }

    fn orchestrator_runtime(&self) -> &OrchestratorRuntime {
        &self.orchestrator_runtime
    }

    fn process_manager(&self) -> &Arc<ProcessManager> {
        &self.process_manager
    }
}

impl LocalDeployment {
    /// Get the shared message bus for WebSocket event broadcasting.
    pub fn message_bus(&self) -> &SharedMessageBus {
        &self.message_bus
    }

    /// Get the prompt watcher for terminal prompt detection.
    pub fn prompt_watcher(&self) -> &PromptWatcher {
        &self.prompt_watcher
    }

    /// Reconcile terminal statuses on startup
    ///
    /// Resets any terminals that are marked as running/waiting in the database
    /// but have no actual process in the process manager (e.g., after a restart).
    async fn reconcile_terminal_statuses(
        db: &DBService,
        process_manager: &ProcessManager,
    ) -> anyhow::Result<()> {
        // Find all terminals with active statuses (including 'starting' for interrupted spawns)
        let terminal_ids: Vec<String> = sqlx::query_scalar(
            "SELECT id FROM terminal WHERE status IN ('starting', 'started', 'waiting', 'working', 'running', 'active')"
        )
        .fetch_all(&db.pool)
        .await?;

        let mut reset_count = 0;
        for terminal_id in terminal_ids {
            // Check if the process is actually running
            if !process_manager.is_running(&terminal_id).await {
                // Reset the terminal status to not_started
                Terminal::update_status(&db.pool, &terminal_id, "not_started").await?;
                Terminal::update_process(&db.pool, &terminal_id, None, None).await?;
                reset_count += 1;
                tracing::info!(
                    terminal_id = %terminal_id,
                    "Reset stale terminal status to not_started"
                );
            }
        }

        if reset_count > 0 {
            tracing::info!(
                count = reset_count,
                "Reconciled stale terminal statuses on startup"
            );
        }

        Ok(())
    }

    pub fn remote_client(&self) -> Result<(), DeploymentError> {
        Err(DeploymentError::Other(anyhow::anyhow!(
            "Remote client not configured"
        )))
    }

    pub async fn get_login_status(&self) -> LoginStatus {
        if self.auth_context.get_credentials().await.is_none() {
            self.auth_context.clear_profile().await;
            return LoginStatus::LoggedOut;
        }

        if let Some(cached_profile) = self.auth_context.cached_profile().await {
            return LoginStatus::LoggedIn {
                profile: cached_profile,
            };
        }

        // No remote client available - return logged out
        LoginStatus::LoggedOut
    }

    pub async fn store_oauth_handoff(
        &self,
        handoff_id: Uuid,
        provider: String,
        app_verifier: String,
    ) {
        self.oauth_handoffs.write().await.insert(
            handoff_id,
            PendingHandoff {
                provider,
                app_verifier,
            },
        );
    }

    pub async fn take_oauth_handoff(&self, handoff_id: &Uuid) -> Option<(String, String)> {
        self.oauth_handoffs
            .write()
            .await
            .remove(handoff_id)
            .map(|state| (state.provider, state.app_verifier))
    }
}
