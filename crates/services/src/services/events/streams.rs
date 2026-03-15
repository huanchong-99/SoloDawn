use std::{
    collections::HashMap,
    sync::{Arc, Mutex, PoisonError},
    time::{Duration, Instant},
};

use db::models::{
    execution_process::ExecutionProcess,
    project::Project,
    scratch::Scratch,
    task::{Task, TaskWithAttemptStatus},
    workspace::Workspace,
};
use futures::StreamExt;
use serde_json::json;
use tokio_stream::wrappers::{BroadcastStream, errors::BroadcastStreamRecvError};
use utils::log_msg::LogMsg;
use uuid::Uuid;

use super::{
    EventService,
    patches::execution_process_patch,
    types::{EventError, EventPatch, RecordTypes},
};

// ─── G33-008: In-memory task→project_id cache ────────────────────────────────

/// TTL for cached task→project_id entries (5 minutes).
const TASK_CACHE_TTL: Duration = Duration::from_secs(300);

#[derive(Clone)]
struct CacheEntry {
    project_id: Uuid,
    inserted_at: Instant,
}

/// Shared, lazily-populated cache mapping task_id → project_id.
#[derive(Default, Clone)]
struct TaskProjectCache {
    inner: Arc<Mutex<HashMap<Uuid, CacheEntry>>>,
}

impl TaskProjectCache {
    fn get(&self, task_id: Uuid) -> Option<Uuid> {
        let mut map = self.inner.lock().unwrap_or_else(PoisonError::into_inner);
        if let Some(entry) = map.get(&task_id) {
            if entry.inserted_at.elapsed() < TASK_CACHE_TTL {
                return Some(entry.project_id);
            }
            // Expired – remove and fall through to DB lookup
            map.remove(&task_id);
        }
        None
    }

    fn insert(&self, task_id: Uuid, project_id: Uuid) {
        let mut map = self.inner.lock().unwrap_or_else(PoisonError::into_inner);
        map.insert(task_id, CacheEntry { project_id, inserted_at: Instant::now() });
    }
}

impl EventService {
    /// Build a full tasks snapshot patch for the given project (used for resync on Lagged).
    async fn build_tasks_snapshot(
        pool: &sqlx::SqlitePool,
        project_id: Uuid,
    ) -> Result<LogMsg, sqlx::Error> {
        let tasks =
            Task::find_by_project_id_with_attempt_status(pool, project_id).await?;
        let tasks_map: serde_json::Map<String, serde_json::Value> = tasks
            .into_iter()
            .map(|task| {
                (
                    task.id.to_string(),
                    serde_json::to_value(&task).unwrap_or(serde_json::Value::Null),
                )
            })
            .collect();
        let patch = json!([{
            "op": "replace",
            "path": "/tasks",
            "value": tasks_map
        }]);
        let patch: json_patch::Patch = serde_json::from_value(patch)
            .map_err(|e| sqlx::Error::Protocol(format!("tasks snapshot serialization: {e}")))?;
        Ok(LogMsg::JsonPatch(patch))
    }

    /// Stream raw task messages for a specific project with initial snapshot
    pub async fn stream_tasks_raw(
        &self,
        project_id: Uuid,
    ) -> Result<futures::stream::BoxStream<'static, Result<LogMsg, std::io::Error>>, EventError>
    {
        // Get initial snapshot of tasks
        let tasks = Task::find_by_project_id_with_attempt_status(&self.db.pool, project_id).await?;

        // Convert task array to object keyed by task ID
        let tasks_map: serde_json::Map<String, serde_json::Value> = tasks
            .into_iter()
            .map(|task| {
                let id = task.id.to_string();
                // G33-010: avoid unwrap
                (id, serde_json::to_value(&task).unwrap_or(serde_json::Value::Null))
            })
            .collect();

        let initial_patch = json!([
            {
                "op": "replace",
                "path": "/tasks",
                "value": tasks_map
            }
        ]);
        let initial_msg = LogMsg::JsonPatch(
            serde_json::from_value(initial_patch)
                .map_err(|e| EventError::Other(anyhow::anyhow!("tasks snapshot: {e}")))?,
        );

        // Clone necessary data for the async filter
        let db_pool = self.db.pool.clone();

        // G33-008: shared in-memory task→project_id cache to avoid per-event DB queries
        let task_project_cache = TaskProjectCache::default();

        // Get filtered event stream
        let filtered_stream =
            BroadcastStream::new(self.msg_store.get_receiver()).filter_map(move |msg_result| {
                let db_pool = db_pool.clone();
                let cache = task_project_cache.clone();
                async move {
                    match msg_result {
                        Ok(LogMsg::JsonPatch(patch)) => {
                            // Filter events based on project_id
                            if let Some(patch_op) = patch.0.first() {
                                // Check if this is a direct task patch (new format)
                                if patch_op.path().starts_with("/tasks/") {
                                    match patch_op {
                                        json_patch::PatchOperation::Add(op) => {
                                            // Parse task data directly from value
                                            if let Ok(task) =
                                                serde_json::from_value::<TaskWithAttemptStatus>(
                                                    op.value.clone(),
                                                )
                                                && task.project_id == project_id
                                            {
                                                // Populate cache for future Remove verification
                                                cache.insert(task.id, task.project_id);
                                                return Some(Ok(LogMsg::JsonPatch(patch)));
                                            }
                                        }
                                        json_patch::PatchOperation::Replace(op) => {
                                            // Parse task data directly from value
                                            if let Ok(task) =
                                                serde_json::from_value::<TaskWithAttemptStatus>(
                                                    op.value.clone(),
                                                )
                                                && task.project_id == project_id
                                            {
                                                cache.insert(task.id, task.project_id);
                                                return Some(Ok(LogMsg::JsonPatch(patch)));
                                            }
                                        }
                                        // G33-002: verify Remove belongs to this project via cache or DB
                                        json_patch::PatchOperation::Remove(op) => {
                                            // Extract task_id from path segment e.g. "/tasks/<uuid>"
                                            let path_str = op.path.to_string();
                                            let task_id_str =
                                                path_str.strip_prefix("/tasks/").unwrap_or("");
                                            if let Ok(task_id) = task_id_str.parse::<Uuid>() {
                                                // Fast path: check the in-memory cache first
                                                if let Some(cached_project_id) =
                                                    cache.get(task_id)
                                                {
                                                    if cached_project_id == project_id {
                                                        return Some(Ok(LogMsg::JsonPatch(
                                                            patch,
                                                        )));
                                                    }
                                                    // Belongs to a different project
                                                    return None;
                                                }
                                                // Slow path: DB lookup (task might already be
                                                // deleted, so not found is treated as "skip")
                                                if let Ok(Some(task)) =
                                                    Task::find_by_id(&db_pool, task_id).await
                                                {
                                                    if task.project_id == project_id {
                                                        cache.insert(task_id, task.project_id);
                                                        return Some(Ok(LogMsg::JsonPatch(
                                                            patch,
                                                        )));
                                                    }
                                                }
                                            }
                                            // Could not verify ownership; skip to avoid leaking
                                            return None;
                                        }
                                        _ => {}
                                    }
                                } else if let Ok(event_patch_value) = serde_json::to_value(patch_op)
                                    && let Ok(event_patch) =
                                        serde_json::from_value::<EventPatch>(event_patch_value)
                                {
                                    // Handle old EventPatch format for non-task records
                                    match &event_patch.value.record {
                                        RecordTypes::Task(task) => {
                                            if task.project_id == project_id {
                                                return Some(Ok(LogMsg::JsonPatch(patch)));
                                            }
                                        }
                                        RecordTypes::DeletedTask {
                                            project_id: Some(deleted_project_id),
                                            ..
                                        } => {
                                            if *deleted_project_id == project_id {
                                                return Some(Ok(LogMsg::JsonPatch(patch)));
                                            }
                                        }
                                        RecordTypes::Workspace(workspace) => {
                                            // G33-008: check cache before hitting DB
                                            let task_pid =
                                                if let Some(pid) =
                                                    cache.get(workspace.task_id)
                                                {
                                                    Some(pid)
                                                } else if let Ok(Some(task)) =
                                                    Task::find_by_id(&db_pool, workspace.task_id)
                                                        .await
                                                {
                                                    cache.insert(workspace.task_id, task.project_id);
                                                    Some(task.project_id)
                                                } else {
                                                    None
                                                };
                                            if task_pid == Some(project_id) {
                                                return Some(Ok(LogMsg::JsonPatch(patch)));
                                            }
                                        }
                                        RecordTypes::DeletedWorkspace {
                                            task_id: Some(deleted_task_id),
                                            ..
                                        } => {
                                            // Check if deleted workspace belonged to a task in our project
                                            let task_pid =
                                                if let Some(pid) = cache.get(*deleted_task_id) {
                                                    Some(pid)
                                                } else if let Ok(Some(task)) =
                                                    Task::find_by_id(&db_pool, *deleted_task_id)
                                                        .await
                                                {
                                                    cache.insert(*deleted_task_id, task.project_id);
                                                    Some(task.project_id)
                                                } else {
                                                    None
                                                };
                                            if task_pid == Some(project_id) {
                                                return Some(Ok(LogMsg::JsonPatch(patch)));
                                            }
                                        }
                                        _ => {}
                                    }
                                }
                            }
                            None
                        }
                        Ok(other) => Some(Ok(other)), // Pass through non-patch messages
                        // G33-001: resync tasks snapshot on Lagged instead of silently dropping
                        Err(BroadcastStreamRecvError::Lagged(skipped)) => {
                            tracing::warn!(
                                skipped = skipped,
                                project_id = %project_id,
                                "tasks stream lagged; resyncing snapshot"
                            );
                            match Self::build_tasks_snapshot(&db_pool, project_id).await {
                                Ok(snapshot) => Some(Ok(snapshot)),
                                Err(err) => {
                                    tracing::error!(
                                        error = %err,
                                        project_id = %project_id,
                                        "failed to resync tasks after lag"
                                    );
                                    Some(Err(std::io::Error::other(format!(
                                        "failed to resync tasks after lag: {err}"
                                    ))))
                                }
                            }
                        }
                    }
                }
            });

        // Start with initial snapshot, Ready signal, then live updates
        let initial_stream = futures::stream::iter(vec![Ok(initial_msg), Ok(LogMsg::Ready)]);
        let combined_stream = initial_stream.chain(filtered_stream).boxed();

        Ok(combined_stream)
    }

    /// Stream raw project messages with initial snapshot
    pub async fn stream_projects_raw(
        &self,
    ) -> Result<futures::stream::BoxStream<'static, Result<LogMsg, std::io::Error>>, EventError>
    {
        fn build_projects_snapshot(projects: Vec<Project>) -> LogMsg {
            // Convert projects array to object keyed by project ID
            let projects_map: serde_json::Map<String, serde_json::Value> = projects
                .into_iter()
                .map(|project| {
                    (
                        project.id.to_string(),
                        serde_json::to_value(project).unwrap(),
                    )
                })
                .collect();

            let patch = json!([
                {
                    "op": "replace",
                    "path": "/projects",
                    "value": projects_map
                }
            ]);

            LogMsg::JsonPatch(serde_json::from_value(patch).unwrap())
        }

        // Get initial snapshot of projects
        let projects = Project::find_all(&self.db.pool).await?;
        let initial_msg = build_projects_snapshot(projects);

        let db_pool = self.db.pool.clone();

        // Get filtered event stream (projects only)
        let filtered_stream =
            BroadcastStream::new(self.msg_store.get_receiver()).filter_map(move |msg_result| {
                let db_pool = db_pool.clone();
                async move {
                    match msg_result {
                        Ok(LogMsg::JsonPatch(patch)) => {
                            if let Some(patch_op) = patch.0.first()
                                && (patch_op.path().starts_with("/projects/") || patch_op.path() == "/projects")
                            {
                                return Some(Ok(LogMsg::JsonPatch(patch)));
                            }
                            None
                        }
                        Ok(other) => Some(Ok(other)), // Pass through non-patch messages
                        Err(BroadcastStreamRecvError::Lagged(skipped)) => {
                            tracing::warn!(
                                skipped = skipped,
                                "projects stream lagged; resyncing snapshot"
                            );

                            match Project::find_all(&db_pool).await {
                                Ok(projects) => Some(Ok(build_projects_snapshot(projects))),
                                Err(err) => {
                                    tracing::error!(
                                        error = %err,
                                        "failed to resync projects after lag"
                                    );
                                    Some(Err(std::io::Error::other(format!(
                                        "failed to resync projects after lag: {err}"
                                    ))))
                                }
                            }
                        }
                    }
                }
            });

        // Start with initial snapshot, Ready signal, then live updates
        let initial_stream = futures::stream::iter(vec![Ok(initial_msg), Ok(LogMsg::Ready)]);
        let combined_stream = initial_stream.chain(filtered_stream).boxed();

        Ok(combined_stream)
    }

    /// Build a full execution-processes snapshot for the given session (used for Lagged resync).
    async fn build_execution_processes_snapshot(
        pool: &sqlx::SqlitePool,
        session_id: Uuid,
        show_soft_deleted: bool,
    ) -> Result<LogMsg, sqlx::Error> {
        let processes =
            ExecutionProcess::find_by_session_id(pool, session_id, show_soft_deleted).await?;
        let processes_map: serde_json::Map<String, serde_json::Value> = processes
            .into_iter()
            .map(|p| {
                (
                    p.id.to_string(),
                    serde_json::to_value(&p).unwrap_or(serde_json::Value::Null),
                )
            })
            .collect();
        let patch = json!([{
            "op": "replace",
            "path": "/execution_processes",
            "value": processes_map
        }]);
        let patch: json_patch::Patch = serde_json::from_value(patch).map_err(|e| {
            sqlx::Error::Protocol(format!("execution_processes snapshot serialization: {e}"))
        })?;
        Ok(LogMsg::JsonPatch(patch))
    }

    /// Stream execution processes for a specific session with initial snapshot (raw LogMsg format for WebSocket)
    pub async fn stream_execution_processes_for_session_raw(
        &self,
        session_id: Uuid,
        show_soft_deleted: bool,
    ) -> Result<futures::stream::BoxStream<'static, Result<LogMsg, std::io::Error>>, EventError>
    {
        // Get execution processes for this session
        let processes =
            ExecutionProcess::find_by_session_id(&self.db.pool, session_id, show_soft_deleted)
                .await?;

        // Convert processes array to object keyed by process ID
        let processes_map: serde_json::Map<String, serde_json::Value> = processes
            .into_iter()
            .map(|process| {
                (
                    process.id.to_string(),
                    // G33-010: avoid unwrap
                    serde_json::to_value(&process).unwrap_or(serde_json::Value::Null),
                )
            })
            .collect();

        let initial_patch = json!([{
            "op": "replace",
            "path": "/execution_processes",
            "value": processes_map
        }]);
        let initial_msg = LogMsg::JsonPatch(
            serde_json::from_value(initial_patch)
                .map_err(|e| EventError::Other(anyhow::anyhow!("execution_processes snapshot: {e}")))?,
        );

        // G33-003: track process_id → session_id for Remove verification
        let session_ownership_cache: Arc<Mutex<HashMap<Uuid, Uuid>>> =
            Arc::new(Mutex::new(HashMap::new()));

        let db_pool_exec = self.db.pool.clone();

        // Get filtered event stream
        let filtered_stream =
            BroadcastStream::new(self.msg_store.get_receiver()).filter_map(move |msg_result| {
                let db_pool_exec = db_pool_exec.clone();
                let ownership = session_ownership_cache.clone();
                async move {
                    match msg_result {
                        Ok(LogMsg::JsonPatch(patch)) => {
                            // Filter events based on session_id
                            if let Some(patch_op) = patch.0.first() {
                                // Check if this is a modern execution process patch
                                if patch_op.path().starts_with("/execution_processes/") {
                                    match patch_op {
                                        json_patch::PatchOperation::Add(op) => {
                                            // Parse execution process data directly from value
                                            if let Ok(process) =
                                                serde_json::from_value::<ExecutionProcess>(
                                                    op.value.clone(),
                                                )
                                                && process.session_id == session_id
                                            {
                                                // Populate ownership cache
                                                {
                                                    let mut map = ownership.lock()
                                                        .unwrap_or_else(PoisonError::into_inner);
                                                    map.insert(process.id, process.session_id);
                                                }
                                                if !show_soft_deleted && process.dropped {
                                                    let remove_patch =
                                                        execution_process_patch::remove(process.id);
                                                    return Some(Ok(LogMsg::JsonPatch(
                                                        remove_patch,
                                                    )));
                                                }
                                                return Some(Ok(LogMsg::JsonPatch(patch)));
                                            }
                                        }
                                        json_patch::PatchOperation::Replace(op) => {
                                            // Parse execution process data directly from value
                                            if let Ok(process) =
                                                serde_json::from_value::<ExecutionProcess>(
                                                    op.value.clone(),
                                                )
                                                && process.session_id == session_id
                                            {
                                                {
                                                    let mut map = ownership.lock()
                                                        .unwrap_or_else(PoisonError::into_inner);
                                                    map.insert(process.id, process.session_id);
                                                }
                                                if !show_soft_deleted && process.dropped {
                                                    let remove_patch =
                                                        execution_process_patch::remove(process.id);
                                                    return Some(Ok(LogMsg::JsonPatch(
                                                        remove_patch,
                                                    )));
                                                }
                                                return Some(Ok(LogMsg::JsonPatch(patch)));
                                            }
                                        }
                                        // G33-003: verify Remove belongs to this session
                                        json_patch::PatchOperation::Remove(op) => {
                                            let path_str = op.path.to_string();
                                            let proc_id_str = path_str
                                                .strip_prefix("/execution_processes/")
                                                .unwrap_or("");
                                            if let Ok(proc_id) = proc_id_str.parse::<Uuid>() {
                                                // Check in-memory ownership cache first
                                                let cached_sid = {
                                                    let map = ownership.lock()
                                                        .unwrap_or_else(PoisonError::into_inner);
                                                    map.get(&proc_id).copied()
                                                };
                                                match cached_sid {
                                                    Some(sid) if sid == session_id => {
                                                        return Some(Ok(LogMsg::JsonPatch(patch)));
                                                    }
                                                    Some(_) => {
                                                        // Belongs to a different session
                                                        return None;
                                                    }
                                                    None => {
                                                        // DB lookup – process may already be deleted
                                                        if let Ok(Some(proc)) =
                                                            ExecutionProcess::find_by_id(
                                                                &db_pool_exec,
                                                                proc_id,
                                                            )
                                                            .await
                                                        {
                                                            if proc.session_id == session_id {
                                                                let mut map = ownership.lock()
                                                                    .unwrap_or_else(PoisonError::into_inner);
                                                                map.insert(proc_id, proc.session_id);
                                                                return Some(Ok(LogMsg::JsonPatch(
                                                                    patch,
                                                                )));
                                                            }
                                                            return None;
                                                        }
                                                        // Not found → skip to avoid leaking
                                                        return None;
                                                    }
                                                }
                                            }
                                            return None;
                                        }
                                        _ => {}
                                    }
                                }
                                // Fallback to legacy EventPatch format for backward compatibility
                                else if let Ok(event_patch_value) = serde_json::to_value(patch_op)
                                    && let Ok(event_patch) =
                                        serde_json::from_value::<EventPatch>(event_patch_value)
                                {
                                    match &event_patch.value.record {
                                        RecordTypes::ExecutionProcess(process) => {
                                            if process.session_id == session_id {
                                                if !show_soft_deleted && process.dropped {
                                                    let remove_patch =
                                                        execution_process_patch::remove(process.id);
                                                    return Some(Ok(LogMsg::JsonPatch(
                                                        remove_patch,
                                                    )));
                                                }
                                                return Some(Ok(LogMsg::JsonPatch(patch)));
                                            }
                                        }
                                        RecordTypes::DeletedExecutionProcess {
                                            session_id: Some(deleted_session_id),
                                            ..
                                        } => {
                                            if *deleted_session_id == session_id {
                                                return Some(Ok(LogMsg::JsonPatch(patch)));
                                            }
                                        }
                                        _ => {}
                                    }
                                }
                            }
                            None
                        }
                        Ok(other) => Some(Ok(other)), // Pass through non-patch messages
                        // G33-001: resync execution_processes snapshot on Lagged
                        Err(BroadcastStreamRecvError::Lagged(skipped)) => {
                            tracing::warn!(
                                skipped = skipped,
                                session_id = %session_id,
                                "execution_processes stream lagged; resyncing snapshot"
                            );
                            match Self::build_execution_processes_snapshot(
                                &db_pool_exec,
                                session_id,
                                show_soft_deleted,
                            )
                            .await
                            {
                                Ok(snapshot) => Some(Ok(snapshot)),
                                Err(err) => {
                                    tracing::error!(
                                        error = %err,
                                        session_id = %session_id,
                                        "failed to resync execution_processes after lag"
                                    );
                                    Some(Err(std::io::Error::other(format!(
                                        "failed to resync execution_processes after lag: {err}"
                                    ))))
                                }
                            }
                        }
                    }
                }
            });

        // Start with initial snapshot, Ready signal, then live updates
        let initial_stream = futures::stream::iter(vec![Ok(initial_msg), Ok(LogMsg::Ready)]);
        let combined_stream = initial_stream.chain(filtered_stream).boxed();

        Ok(combined_stream)
    }

    /// Stream a single scratch item with initial snapshot (raw LogMsg format for WebSocket)
    pub async fn stream_scratch_raw(
        &self,
        scratch_id: Uuid,
        scratch_type: &db::models::scratch::ScratchType,
    ) -> Result<futures::stream::BoxStream<'static, Result<LogMsg, std::io::Error>>, EventError>
    {
        // Treat errors (e.g., corrupted/malformed data) the same as "scratch not found"
        // This prevents the websocket from closing and retrying indefinitely
        let scratch = match Scratch::find_by_id(&self.db.pool, scratch_id, scratch_type).await {
            Ok(scratch) => scratch,
            Err(e) => {
                tracing::warn!(
                    scratch_id = %scratch_id,
                    scratch_type = %scratch_type,
                    error = %e,
                    "Failed to load scratch, treating as empty"
                );
                None
            }
        };

        let initial_patch = json!([{
            "op": "replace",
            "path": "/scratch",
            "value": scratch
        }]);
        let initial_msg = LogMsg::JsonPatch(
            serde_json::from_value(initial_patch)
                .map_err(|e| EventError::Other(anyhow::anyhow!("scratch snapshot: {e}")))?,
        );

        let type_str = scratch_type.to_string();
        let scratch_type_resync = *scratch_type;
        let db_pool_scratch = self.db.pool.clone();

        // Filter to only this scratch's events by matching id and payload.type in the patch value
        let filtered_stream =
            BroadcastStream::new(self.msg_store.get_receiver()).filter_map(move |msg_result| {
                let id_str = scratch_id.to_string();
                let type_str = type_str.clone();
                let db_pool_scratch = db_pool_scratch.clone();
                let scratch_type_resync = scratch_type_resync;
                async move {
                    match msg_result {
                        Ok(LogMsg::JsonPatch(patch)) => {
                            if let Some(op) = patch.0.first()
                                && op.path() == "/scratch"
                            {
                                // Extract id and payload.type from the patch value
                                let value = match op {
                                    json_patch::PatchOperation::Add(a) => Some(&a.value),
                                    json_patch::PatchOperation::Replace(r) => Some(&r.value),
                                    _ => None,
                                };

                                let matches = value.is_some_and(|v| {
                                    let id_matches =
                                        v.get("id").and_then(|v| v.as_str()) == Some(&id_str);
                                    let type_matches = v
                                        .get("payload")
                                        .and_then(|p| p.get("type"))
                                        .and_then(|t| t.as_str())
                                        == Some(&type_str);
                                    id_matches && type_matches
                                });

                                if matches {
                                    return Some(Ok(LogMsg::JsonPatch(patch)));
                                }
                            }
                            None
                        }
                        Ok(other) => Some(Ok(other)),
                        // G33-001: resync scratch snapshot on Lagged
                        Err(BroadcastStreamRecvError::Lagged(skipped)) => {
                            tracing::warn!(
                                skipped = skipped,
                                scratch_id = %scratch_id,
                                "scratch stream lagged; resyncing snapshot"
                            );
                            // Re-fetch from DB and emit a fresh replace patch
                            let resync_scratch = Scratch::find_by_id(
                                &db_pool_scratch,
                                scratch_id,
                                &scratch_type_resync,
                            )
                            .await
                            .ok()
                            .flatten();
                            let resync_patch = json!([{
                                "op": "replace",
                                "path": "/scratch",
                                "value": resync_scratch
                            }]);
                            match serde_json::from_value::<json_patch::Patch>(resync_patch) {
                                Ok(p) => Some(Ok(LogMsg::JsonPatch(p))),
                                Err(err) => {
                                    tracing::error!(
                                        error = %err,
                                        scratch_id = %scratch_id,
                                        "failed to resync scratch after lag"
                                    );
                                    Some(Err(std::io::Error::other(format!(
                                        "failed to resync scratch after lag: {err}"
                                    ))))
                                }
                            }
                        }
                    }
                }
            });

        let initial_stream = futures::stream::iter(vec![Ok(initial_msg), Ok(LogMsg::Ready)]);
        let combined_stream = initial_stream.chain(filtered_stream).boxed();
        Ok(combined_stream)
    }

    /// Build a full workspaces snapshot (used for Lagged resync).
    async fn build_workspaces_snapshot(
        pool: &sqlx::SqlitePool,
        archived: Option<bool>,
        limit: Option<i64>,
    ) -> Result<LogMsg, sqlx::Error> {
        let workspaces = Workspace::find_all_with_status(pool, archived, limit).await?;
        let workspaces_map: serde_json::Map<String, serde_json::Value> = workspaces
            .into_iter()
            .map(|ws| {
                (
                    ws.id.to_string(),
                    serde_json::to_value(&ws).unwrap_or(serde_json::Value::Null),
                )
            })
            .collect();
        let patch = json!([{
            "op": "replace",
            "path": "/workspaces",
            "value": workspaces_map
        }]);
        let patch: json_patch::Patch = serde_json::from_value(patch).map_err(|e| {
            sqlx::Error::Protocol(format!("workspaces snapshot serialization: {e}"))
        })?;
        Ok(LogMsg::JsonPatch(patch))
    }

    pub async fn stream_workspaces_raw(
        &self,
        archived: Option<bool>,
        limit: Option<i64>,
    ) -> Result<futures::stream::BoxStream<'static, Result<LogMsg, std::io::Error>>, EventError>
    {
        let workspaces = Workspace::find_all_with_status(&self.db.pool, archived, limit).await?;
        let workspaces_map: serde_json::Map<String, serde_json::Value> = workspaces
            .into_iter()
            .map(|ws| {
                (
                    ws.id.to_string(),
                    // G33-010: avoid unwrap
                    serde_json::to_value(&ws).unwrap_or(serde_json::Value::Null),
                )
            })
            .collect();

        let initial_patch = json!([{
            "op": "replace",
            "path": "/workspaces",
            "value": workspaces_map
        }]);
        let initial_msg = LogMsg::JsonPatch(
            serde_json::from_value(initial_patch)
                .map_err(|e| EventError::Other(anyhow::anyhow!("workspaces snapshot: {e}")))?,
        );

        let db_pool_ws = self.db.pool.clone();

        let filtered_stream = BroadcastStream::new(self.msg_store.get_receiver()).filter_map(
            move |msg_result| {
                let db_pool_ws = db_pool_ws.clone();
                async move {
                match msg_result {
                    Ok(LogMsg::JsonPatch(patch)) => {
                        if let Some(op) = patch.0.first()
                            && (op.path().starts_with("/workspaces/") || op.path() == "/workspaces")
                        {
                            // If archived filter is set, handle state transitions
                            if let Some(archived_filter) = archived {
                                // Extract workspace data from Add/Replace operations
                                let value = match op {
                                    json_patch::PatchOperation::Add(a) => Some(&a.value),
                                    json_patch::PatchOperation::Replace(r) => Some(&r.value),
                                    json_patch::PatchOperation::Remove(_) => {
                                        // Allow remove operations through - client will handle
                                        return Some(Ok(LogMsg::JsonPatch(patch)));
                                    }
                                    _ => None,
                                };

                                if let Some(v) = value
                                    && let Some(ws_archived) =
                                        v.get("archived").and_then(serde_json::Value::as_bool)
                                {
                                    if ws_archived == archived_filter {
                                        // Workspace matches this filter
                                        // Convert Replace to Add since workspace may be new to this filtered stream
                                        if let json_patch::PatchOperation::Replace(r) = op {
                                            let add_patch = json_patch::Patch(vec![
                                                json_patch::PatchOperation::Add(
                                                    json_patch::AddOperation {
                                                        path: r.path.clone(),
                                                        value: r.value.clone(),
                                                    },
                                                ),
                                            ]);
                                            return Some(Ok(LogMsg::JsonPatch(add_patch)));
                                        }
                                        return Some(Ok(LogMsg::JsonPatch(patch)));
                                    }
                                    // Workspace no longer matches this filter - send remove
                                    let remove_patch = json_patch::Patch(vec![
                                        json_patch::PatchOperation::Remove(
                                            json_patch::RemoveOperation {
                                                path: op
                                                    .path()
                                                    .to_string()
                                                    .try_into()
                                                    .expect("Workspace path should be valid"),
                                            },
                                        ),
                                    ]);
                                    return Some(Ok(LogMsg::JsonPatch(remove_patch)));
                                }
                            }
                            return Some(Ok(LogMsg::JsonPatch(patch)));
                        }
                        None
                    }
                    Ok(other) => Some(Ok(other)),
                    // G33-001: resync workspaces snapshot on Lagged
                    Err(BroadcastStreamRecvError::Lagged(skipped)) => {
                        tracing::warn!(
                            skipped = skipped,
                            "workspaces stream lagged; resyncing snapshot"
                        );
                        match Self::build_workspaces_snapshot(&db_pool_ws, archived, limit).await {
                            Ok(snapshot) => Some(Ok(snapshot)),
                            Err(err) => {
                                tracing::error!(
                                    error = %err,
                                    "failed to resync workspaces after lag"
                                );
                                Some(Err(std::io::Error::other(format!(
                                    "failed to resync workspaces after lag: {err}"
                                ))))
                            }
                        }
                    }
                }
            }},
        );

        let initial_stream = futures::stream::iter(vec![Ok(initial_msg), Ok(LogMsg::Ready)]);
        Ok(initial_stream.chain(filtered_stream).boxed())
    }
}
