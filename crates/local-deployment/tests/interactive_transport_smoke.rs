//! PHASE SMOKE — end-to-end proof of the no-`-p` interactive transport against
//! the REAL `claude` binary.
//!
//! This is an `#[ignore]`d integration test: it needs a live `claude` install,
//! real native-OAuth (subscription) credentials at `~/.claude/.credentials.json`,
//! and network access. CI skips it. Run it explicitly with:
//!
//! ```text
//! RUST_MIN_STACK=268435456 \
//!   cargo test -p local-deployment --test interactive_transport_smoke -- --ignored --nocapture
//! ```
//!
//! What it exercises (the genuine production seams, no mocks):
//! 1. `cc_switch::create_interactive_isolated_home` provisions the isolated home
//!    and transcript path; we copy the real `.credentials.json` (and `settings.json`)
//!    into it so the genuine `claude` consumes the subscription credential.
//! 2. `ClaudeCode{interactive,...}.build_interactive_command_parts()` builds the
//!    real argv (no `-p`, with `--session-id`).
//! 3. `LocalContainerService::spawn_interactive_claude` spawns the genuine binary
//!    (piped/null stdin, one-turn-then-exit), tails the on-disk transcript JSONL
//!    into a `MsgStore`, and drives `Finished` off the child's real exit.
//! 4. We register the child in the container's `child_store` (as the production
//!    caller does) so the completion-on-exit watcher fires promptly, then drain
//!    the store and normalize via the SAME `ClaudeCode::normalize_logs` pass the
//!    `-p` path uses, asserting the assistant reply is present.

use std::{
    collections::HashMap,
    path::{Path, PathBuf},
    sync::Arc,
    time::{Duration, Instant},
};

use executors::{
    command::CommandParts,
    executors::{StandardCodingAgentExecutor, claude::ClaudeCode},
};
use local_deployment::container::LocalContainerService;
use services::services::{
    approvals::Approvals, cc_switch, config::Config, git::GitService, image::ImageService,
    queued_message::QueuedMessageService,
};
use tokio::sync::RwLock;
use utils::{log_msg::LogMsg, msg_store::MsgStore};
use uuid::Uuid;

/// Build a minimal real `LocalContainerService`. The only collaborator the
/// interactive transport touches is `child_store` (completion watcher) and
/// `msg_stores` (tailer registration); the rest are wired with their genuine
/// public constructors but never exercised by this path.
async fn build_container() -> LocalContainerService {
    // In-memory pool: spawn_interactive_claude never queries the DB. The
    // background workspace-cleanup task may log on the unmigrated pool; harmless.
    let pool = sqlx::SqlitePool::connect("sqlite::memory:")
        .await
        .expect("in-memory sqlite pool");
    let db = db::DBService { pool: pool.clone() };

    let msg_stores: Arc<RwLock<HashMap<Uuid, Arc<MsgStore>>>> =
        Arc::new(RwLock::new(HashMap::new()));
    let config = Arc::new(RwLock::new(Config::default()));
    let git = GitService::new();
    let image_service = ImageService::new(pool).expect("image service");
    let approvals = Approvals::new(msg_stores.clone());
    let queued = QueuedMessageService::new();

    LocalContainerService::new(
        db,
        msg_stores,
        config,
        git,
        image_service,
        None,
        approvals,
        queued,
    )
}

/// Copy `~/.claude/.credentials.json` (required) and `settings.json` (optional)
/// into the freshly-provisioned interactive home so the genuine `claude` reads
/// the subscription credential via `CLAUDE_CONFIG_DIR`.
fn copy_real_credentials(home_dir: &Path) {
    let real = dirs::home_dir()
        .expect("home dir")
        .join(".claude");
    let creds = real.join(".credentials.json");
    assert!(
        creds.is_file(),
        "live test requires real credentials at {}",
        creds.display()
    );
    std::fs::copy(&creds, home_dir.join(".credentials.json"))
        .expect("copy .credentials.json into interactive home");

    let settings = real.join("settings.json");
    if settings.is_file() {
        let _ = std::fs::copy(&settings, home_dir.join("settings.json"));
    }
}

/// Construct the interactive `ClaudeCode` via serde (fields like the approvals
/// service are `#[serde(skip)]`), forcing the native `claude` binary on PATH via
/// `base_command_override` (PROBE: claude 2.1.177 is installed natively).
fn interactive_claude(session_uuid: &str) -> ClaudeCode {
    serde_json::from_value(serde_json::json!({
        "interactive": true,
        "interactive_session_id": session_uuid,
        "model": "claude-sonnet-4-6",
        "base_command_override": "claude",
    }))
    .expect("construct interactive ClaudeCode")
}

/// Extract assistant-message texts from the normalized `JsonPatch` entries the
/// `ClaudeLogProcessor` pushed into the store. Each patch op carries a
/// `{"type":"NORMALIZED_ENTRY","content":{entry_type:{type:"assistant_message"},content:"..."}}`.
fn assistant_texts(store: &MsgStore) -> Vec<String> {
    let mut out = Vec::new();
    for msg in store.get_history() {
        let LogMsg::JsonPatch(patch) = msg else {
            continue;
        };
        let value = serde_json::to_value(&patch).unwrap_or(serde_json::Value::Null);
        let ops = value.as_array().cloned().unwrap_or_default();
        for op in ops {
            let v = op.get("value");
            let is_entry = v
                .and_then(|x| x.get("type"))
                .and_then(|t| t.as_str())
                == Some("NORMALIZED_ENTRY");
            if !is_entry {
                continue;
            }
            let content = v.and_then(|x| x.get("content"));
            let is_assistant = content
                .and_then(|c| c.get("entry_type"))
                .and_then(|et| et.get("type"))
                .and_then(|t| t.as_str())
                == Some("assistant_message");
            if !is_assistant {
                continue;
            }
            if let Some(text) = content.and_then(|c| c.get("content")).and_then(|t| t.as_str()) {
                out.push(text.to_string());
            }
        }
    }
    out
}

/// Wait until `LogMsg::Finished` is present in the store, or `deadline` elapses.
/// Returns the elapsed time to Finished, or `None` on timeout.
async fn await_finished(store: &MsgStore, deadline: Duration) -> Option<Duration> {
    let start = Instant::now();
    loop {
        let finished = store
            .get_history()
            .iter()
            .any(|m| matches!(m, LogMsg::Finished));
        if finished {
            return Some(start.elapsed());
        }
        if start.elapsed() >= deadline {
            return None;
        }
        tokio::time::sleep(Duration::from_millis(100)).await;
    }
}

#[tokio::test(flavor = "multi_thread", worker_threads = 4)]
#[ignore = "needs live claude binary + real native-OAuth creds + network"]
async fn interactive_transport_smoke_pong() {
    // A real temp working dir (drives the transcript slug).
    let work = tempfile::tempdir().expect("temp working dir");
    let working_dir: PathBuf = work.path().to_path_buf();

    // 1. Provision the isolated interactive home + copy real creds in.
    let home = cc_switch::create_interactive_isolated_home(None, &working_dir)
        .expect("provision interactive home");
    copy_real_credentials(&home.home_dir);
    eprintln!("[smoke] home_dir       = {}", home.home_dir.display());
    eprintln!("[smoke] session_uuid   = {}", home.session_uuid);
    eprintln!("[smoke] transcript     = {}", home.transcript_path.display());

    // 2. Build the interactive argv via the real executor seam.
    let claude = interactive_claude(&home.session_uuid);
    let parts: CommandParts = claude
        .build_interactive_command_parts()
        .expect("build interactive command parts");
    let (program, args) = parts
        .into_resolved()
        .await
        .expect("resolve claude binary on PATH");
    eprintln!("[smoke] program        = {}", program.display());
    eprintln!("[smoke] args           = {args:?}");

    // (d) argv must carry --session-id and NEVER -p / --print.
    assert!(
        args.iter().any(|a| a == "--session-id"),
        "interactive argv must contain --session-id: {args:?}"
    );
    assert!(
        args.iter().any(|a| a == &home.session_uuid),
        "interactive argv must carry the session uuid: {args:?}"
    );
    assert!(
        !args.iter().any(|a| a == "-p" || a == "--print"),
        "interactive argv must NOT contain -p/--print: {args:?}"
    );

    // 3. Spawn the genuine binary via the production seam.
    let container = build_container().await;
    let exec_id = Uuid::new_v4();

    let mut env: HashMap<String, String> = HashMap::new();
    let home_str = home.home_dir.to_string_lossy().to_string();
    // CLAUDE_CONFIG_DIR is the real redirect (transcript + credential discovery);
    // CLAUDE_HOME mirrors it so RB-37 cleanup still locates the dir.
    env.insert("CLAUDE_CONFIG_DIR".to_string(), home_str.clone());
    env.insert("CLAUDE_HOME".to_string(), home_str);

    let (store, child) = container
        .spawn_interactive_claude(
            exec_id,
            (program, args.clone()),
            "Reply with exactly: PONG".to_string(),
            working_dir.clone(),
            env,
            vec!["ANTHROPIC_API_KEY".to_string()],
            home.transcript_path.clone(),
            home.session_uuid.clone(),
        )
        .await
        .expect("spawn interactive claude");

    // Register the child so the completion-on-exit watcher fires promptly
    // (this is exactly what the production caller does after spawn).
    container.add_child_to_store(exec_id, child).await;

    // 4a/4c. Await Finished. Generous outer bound, but well under the 120s idle
    // safety net so a pass proves completion came from child-exit, not idle.
    let spawn_started = Instant::now();
    let finished_at = await_finished(&store, Duration::from_secs(90)).await;
    let time_to_finished = finished_at.unwrap_or_else(|| spawn_started.elapsed());
    eprintln!("[smoke] time-to-Finished = {:?}", time_to_finished);
    assert!(
        finished_at.is_some(),
        "LogMsg::Finished was not emitted within 90s (transcript: {})",
        home.transcript_path.display()
    );
    // The completion-on-exit watcher must beat the ~120s idle net; require it to
    // arrive comfortably inside that window.
    assert!(
        time_to_finished < Duration::from_secs(110),
        "Finished arrived at {:?} — too close to the 120s idle net; \
         completion-on-exit watcher likely did not fire",
        time_to_finished
    );

    // (a) transcript file must exist on disk.
    assert!(
        home.transcript_path.is_file(),
        "transcript file missing at {}",
        home.transcript_path.display()
    );
    let transcript = std::fs::read_to_string(&home.transcript_path).unwrap_or_default();
    eprintln!(
        "[smoke] transcript bytes = {} ({} lines)",
        transcript.len(),
        transcript.lines().count()
    );

    // (b) Run the SAME single normalization pass the -p path uses, then assert an
    // assistant entry contains the model's reply (PONG).
    claude.normalize_logs(store.clone(), &working_dir);
    // Give the async normalization task a moment to drain the store history.
    tokio::time::sleep(Duration::from_millis(500)).await;

    let texts = assistant_texts(&store);
    eprintln!("[smoke] assistant entries = {}", texts.len());
    for (i, t) in texts.iter().enumerate() {
        eprintln!("[smoke]   assistant[{i}] = {t:?}");
    }
    assert!(
        !texts.is_empty(),
        "pipeline produced no assistant NormalizedEntry"
    );
    let joined = texts.join("\n");
    assert!(
        joined.to_uppercase().contains("PONG"),
        "no assistant reply contained PONG; got: {texts:?}"
    );
    eprintln!("[smoke] PONG observed — interactive transport OK");

    // ---- Follow-up: a second spawn with --resume <uuid> must APPEND to the
    // SAME <uuid>.jsonl (no --fork-session). ----
    let len_before_follow_up = std::fs::metadata(&home.transcript_path)
        .map(|m| m.len())
        .unwrap_or(0);

    let follow_parts: CommandParts = claude
        .build_interactive_follow_up_command_parts(&home.session_uuid)
        .expect("build follow-up command parts");
    let (fprogram, fargs) = follow_parts
        .into_resolved()
        .await
        .expect("resolve follow-up claude binary");
    eprintln!("[smoke] follow-up args = {fargs:?}");
    assert!(
        fargs.iter().any(|a| a == "--resume"),
        "follow-up argv must use --resume: {fargs:?}"
    );
    assert!(
        !fargs.iter().any(|a| a == "--fork-session"),
        "follow-up argv must NOT use --fork-session: {fargs:?}"
    );
    assert!(
        !fargs.iter().any(|a| a == "-p" || a == "--print"),
        "follow-up argv must NOT contain -p/--print: {fargs:?}"
    );

    // FIX VERIFIED (live, claude 2.1.177): `build_interactive_follow_up_command_parts`
    // now emits ONLY `--resume <uuid>` — it no longer also emits the conflicting
    // `--session-id <uuid>`. The genuine binary previously REJECTED the combination:
    //   "Error: --session-id can only be used with --continue or --resume if
    //    --fork-session is also specified."
    // which made the follow-up turn exit immediately WITHOUT appending. With the
    // S2-builder fix in place, the follow-up must RESUME and APPEND to the same
    // <uuid>.jsonl — asserted as a HARD requirement below. Guard the regression:
    // the follow-up argv must NOT carry `--session-id` at all.
    assert!(
        !fargs.iter().any(|a| a == "--session-id"),
        "follow-up argv must NOT emit --session-id (the S2-builder fix removed it; \
         claude 2.1.177 rejects --session-id alongside --resume without \
         --fork-session): {fargs:?}"
    );

    let follow_exec_id = Uuid::new_v4();
    let mut fenv: HashMap<String, String> = HashMap::new();
    let home_str2 = home.home_dir.to_string_lossy().to_string();
    fenv.insert("CLAUDE_CONFIG_DIR".to_string(), home_str2.clone());
    fenv.insert("CLAUDE_HOME".to_string(), home_str2);

    let (fstore, fchild) = container
        .spawn_interactive_claude(
            follow_exec_id,
            (fprogram, fargs),
            "Reply with exactly: PING".to_string(),
            working_dir.clone(),
            fenv,
            vec!["ANTHROPIC_API_KEY".to_string()],
            home.transcript_path.clone(),
            home.session_uuid.clone(),
        )
        .await
        .expect("spawn interactive follow-up");
    container.add_child_to_store(follow_exec_id, fchild).await;

    let follow_finished = await_finished(&fstore, Duration::from_secs(90)).await;
    eprintln!("[smoke] follow-up time-to-Finished = {follow_finished:?}");
    assert!(
        follow_finished.is_some(),
        "follow-up Finished not emitted within 90s"
    );

    let len_after_follow_up = std::fs::metadata(&home.transcript_path)
        .map(|m| m.len())
        .unwrap_or(0);
    eprintln!(
        "[smoke] transcript len {} -> {} (follow-up append)",
        len_before_follow_up, len_after_follow_up
    );

    // HARD ASSERTION (post S2-builder fix): a second turn via
    // `build_interactive_follow_up_command_parts(uuid)` must RESUME the same
    // logical session and APPEND to the SAME <uuid>.jsonl (no --fork-session,
    // no new transcript). The file must therefore have grown.
    assert!(
        len_after_follow_up > len_before_follow_up,
        "follow-up --resume did NOT append to the same transcript file \
         (before={len_before_follow_up}, after={len_after_follow_up}); the resume \
         turn must grow {}",
        home.transcript_path.display()
    );
    eprintln!("[smoke] follow-up appended to same transcript — resume OK");

    // The appended tail must contain the follow-up's reply (PING), proving the
    // resume actually ran a turn against the same session (not just a no-op
    // header rewrite). Re-read + normalize the full transcript and look for PING.
    claude.normalize_logs(fstore.clone(), &working_dir);
    tokio::time::sleep(Duration::from_millis(500)).await;
    let follow_texts = assistant_texts(&fstore);
    eprintln!("[smoke] follow-up assistant entries = {}", follow_texts.len());
    for (i, t) in follow_texts.iter().enumerate() {
        eprintln!("[smoke]   follow-assistant[{i}] = {t:?}");
    }
    let follow_joined = follow_texts.join("\n");
    assert!(
        follow_joined.to_uppercase().contains("PING"),
        "resumed follow-up turn produced no assistant reply containing PING; \
         got: {follow_texts:?}"
    );
    eprintln!("[smoke] PING observed on resumed session — follow-up resume OK");
}

// ---------------------------------------------------------------------------
// NON-LIVE coverage (always runs in CI; no claude binary / network required).
//
// These assert, at the argv/env level, the property the production router
// (`LocalContainerService::try_spawn_interactive_native_oauth`) guarantees:
// EVERY auth mode (native / official-key / relay) is composed onto the no-`-p`
// interactive transport — never the metered `-p`/stream-json path. The router
// itself is private and DB-coupled, so we reconstruct its load-bearing
// composition from the SAME public building blocks it uses:
//   `cc_switch::setup_interactive_auth` (per-mode auth env)
//   `ClaudeCode::build_interactive_command_parts` / `_follow_up_` (argv)
// and assert the invariant the acceptance criterion mandates: no active `-p`.
// ---------------------------------------------------------------------------
#[cfg(test)]
mod router_argv_env {
    use executors::executors::claude::ClaudeCode;
    use services::services::cc_switch::{self, InteractiveAuthMode};

    /// Build the interactive `ClaudeCode` the router builds (interactive=true +
    /// session uuid threaded), resolve initial argv, and assert it carries
    /// `--session-id` and NEVER `-p`/`--print`.
    async fn resolve_initial_argv(session_uuid: &str) -> Vec<String> {
        let claude: ClaudeCode = serde_json::from_value(serde_json::json!({
            "interactive": true,
            "interactive_session_id": session_uuid,
            "model": "claude-sonnet-4-6",
            "base_command_override": "claude",
        }))
        .expect("construct interactive ClaudeCode");
        let parts = claude
            .build_interactive_command_parts()
            .expect("build interactive command parts");
        let (_program, args) = parts.into_resolved().await.expect("resolve argv");
        args
    }

    fn assert_no_dash_p(args: &[String], ctx: &str) {
        assert!(
            !args.iter().any(|a| a == "-p" || a == "--print"),
            "{ctx}: interactive argv must NOT contain -p/--print: {args:?}"
        );
        assert!(
            !args.iter().any(|a| a == "--output-format"),
            "{ctx}: interactive argv must NOT request stream-json (--output-format): {args:?}"
        );
    }

    /// Mode resolution must match the router's `(api_key, base_url)` mapping.
    #[test]
    fn mode_resolution_matches_router_inputs() {
        assert_eq!(
            InteractiveAuthMode::resolve(None, None),
            InteractiveAuthMode::NativeOauth,
            "native subscription: no key, no base_url"
        );
        assert_eq!(
            InteractiveAuthMode::resolve(Some("sk-ant-x"), None),
            InteractiveAuthMode::OfficialKey,
            "official key: key, no base_url"
        );
        assert_eq!(
            InteractiveAuthMode::resolve(Some("relay-tok"), Some("https://relay.example/v1")),
            InteractiveAuthMode::Relay,
            "relay: key + base_url"
        );
    }

    /// NATIVE: argv has no `-p`; auth env scrubs every key var (subscription
    /// billing) and copies creds (no key env set).
    #[tokio::test]
    async fn native_mode_interactive_argv_and_env() {
        let args = resolve_initial_argv("native-uuid-argv").await;
        assert!(
            args.iter().any(|a| a == "--session-id"),
            "native initial argv must carry --session-id: {args:?}"
        );
        assert_no_dash_p(&args, "native");

        let wd = std::path::Path::new(r"E:\SoloDawn");
        let home = cc_switch::create_interactive_isolated_home(Some("native-uuid-argv"), wd)
            .expect("provision home");
        let env = cc_switch::setup_interactive_auth(
            &home,
            None,
            None,
            "claude-sonnet-4-6",
            std::path::Path::new("/nonexistent/.claude"),
        )
        .expect("native auth");
        assert_eq!(env.mode, InteractiveAuthMode::NativeOauth);
        // No key env at all; everything that could redirect billing is scrubbed.
        assert!(!env.set.contains_key("ANTHROPIC_API_KEY"));
        assert!(!env.set.contains_key("ANTHROPIC_AUTH_TOKEN"));
        for key in [
            "ANTHROPIC_API_KEY",
            "ANTHROPIC_AUTH_TOKEN",
            "ANTHROPIC_BASE_URL",
            "CLAUDE_CODE_OAUTH_TOKEN",
        ] {
            assert!(
                env.unset.iter().any(|k| k == key),
                "native must scrub {key}"
            );
        }
        // Native omits --settings (mirrors the -p path).
        assert!(env.settings_arg.is_none());
        let _ = std::fs::remove_dir_all(&home.home_dir);
    }

    /// OFFICIAL KEY: argv has no `-p`; env SETS ANTHROPIC_API_KEY and scrubs relay
    /// vars; a `--settings` path is produced (router appends it to argv).
    #[tokio::test]
    async fn official_key_mode_interactive_argv_and_env() {
        let args = resolve_initial_argv("key-uuid-argv").await;
        assert_no_dash_p(&args, "official-key");

        let wd = std::path::Path::new(r"E:\SoloDawn");
        let home = cc_switch::create_interactive_isolated_home(Some("key-uuid-argv"), wd)
            .expect("provision home");
        let env = cc_switch::setup_interactive_auth(
            &home,
            Some("sk-ant-official"),
            None,
            "claude-sonnet-4-6",
            std::path::Path::new("/nonexistent/.claude"),
        )
        .expect("official-key auth");
        assert_eq!(env.mode, InteractiveAuthMode::OfficialKey);
        assert_eq!(
            env.set.get("ANTHROPIC_API_KEY").map(String::as_str),
            Some("sk-ant-official")
        );
        assert!(env.unset.iter().any(|k| k == "ANTHROPIC_AUTH_TOKEN"));
        assert!(env.unset.iter().any(|k| k == "ANTHROPIC_BASE_URL"));
        // Router would append `--settings <path>` — assert the path exists so the
        // composed argv stays free of any `-p` after the append.
        let settings = env.settings_arg.expect("official key returns --settings path");
        let mut composed = args.clone();
        composed.push("--settings".to_string());
        composed.push(settings.to_string_lossy().to_string());
        assert_no_dash_p(&composed, "official-key (with --settings appended)");
        let _ = std::fs::remove_dir_all(&home.home_dir);
    }

    /// RELAY: argv has no `-p`; env routes via ANTHROPIC_AUTH_TOKEN + ANTHROPIC_BASE_URL
    /// and UNSETS ANTHROPIC_API_KEY (relay endpoint billing).
    #[tokio::test]
    async fn relay_mode_interactive_argv_and_env() {
        let args = resolve_initial_argv("relay-uuid-argv").await;
        assert_no_dash_p(&args, "relay");

        let wd = std::path::Path::new(r"E:\SoloDawn");
        let home = cc_switch::create_interactive_isolated_home(Some("relay-uuid-argv"), wd)
            .expect("provision home");
        let env = cc_switch::setup_interactive_auth(
            &home,
            Some("relay-token"),
            Some("https://relay.example.com/v1"),
            "claude-sonnet-4-6",
            std::path::Path::new("/nonexistent/.claude"),
        )
        .expect("relay auth");
        assert_eq!(env.mode, InteractiveAuthMode::Relay);
        assert_eq!(
            env.set.get("ANTHROPIC_AUTH_TOKEN").map(String::as_str),
            Some("relay-token")
        );
        assert_eq!(
            env.set.get("ANTHROPIC_BASE_URL").map(String::as_str),
            Some("https://relay.example.com")
        );
        assert!(!env.set.contains_key("ANTHROPIC_API_KEY"));
        assert!(env.unset.iter().any(|k| k == "ANTHROPIC_API_KEY"));
        let _ = std::fs::remove_dir_all(&home.home_dir);
    }

    /// Follow-up argv (any mode) uses ONLY `--resume` — never `-p`, never
    /// `--session-id` (the S2-builder fix), never `--fork-session`.
    #[tokio::test]
    async fn follow_up_argv_resume_only_no_dash_p() {
        let claude: ClaudeCode = serde_json::from_value(serde_json::json!({
            "interactive": true,
            "interactive_session_id": "resume-uuid",
            "model": "claude-sonnet-4-6",
            "base_command_override": "claude",
        }))
        .expect("construct interactive ClaudeCode");
        let parts = claude
            .build_interactive_follow_up_command_parts("resume-uuid")
            .expect("build follow-up parts");
        let (_program, args) = parts.into_resolved().await.expect("resolve follow-up argv");
        assert!(
            args.iter().any(|a| a == "--resume"),
            "follow-up must use --resume: {args:?}"
        );
        assert!(
            !args.iter().any(|a| a == "--session-id"),
            "follow-up must NOT use --session-id (S2 fix): {args:?}"
        );
        assert!(
            !args.iter().any(|a| a == "--fork-session"),
            "follow-up must NOT use --fork-session: {args:?}"
        );
        assert_no_dash_p(&args, "follow-up");
    }
}
