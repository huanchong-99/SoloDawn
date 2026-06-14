# Census: rs-server-routes-config

Unit scope: `crates/server/src/routes/config.rs` (1106 lines, single file).
Mount: merged at root via `config::router()` in `crates/server/src/routes/mod.rs:143`; effective prefix `/api` (frontend calls `/api/...`).
Tooling note: fast-context used for the editor-availability trace (succeeded); remaining cross-file queries fell back to Grep after fast-context returned `resource_exhausted` quota errors.

## Module map

| File | Purpose | Public surface | Relations | Notes |
|---|---|---|---|---|
| `crates/server/src/routes/config.rs` | Axum route module for server config: system info, full config replace, MCP server CRUD per executor, executor profiles get/put, editor & agent availability checks, batch AI CLI install, system prerequisite detection, native Claude Code credential detection. | `pub fn router()`; types `Environment`, `UserSystemInfo`, `McpServerQuery`, `McpConfigError`, `GetMcpServerResponse`, `UpdateMcpServersBody`, `ProfilesContent`, `CheckEditorAvailabilityQuery`, `CheckEditorAvailabilityResponse`, `CheckAgentAvailabilityQuery`, `InstallAiClisResponse`, `PrerequisiteStatus`, `SystemPrerequisites`, `NativeCredentialsStatus`. HTTP routes: `GET /info`, `PUT /config`, `GET /sounds/{sound}`, `GET+POST /mcp-config`, `GET+PUT /profiles`, `GET /editors/check-availability`, `GET /agents/check-availability`, `POST /agents/install-ai-clis`, `GET /system/prerequisites`, `GET /native-credentials-status`. | Consumes `services::config` (Config, EditorConfig, EditorType, SoundFile, save_config_to_file, WorkflowModelLibraryItem), `executors` (ExecutorConfigs, ExecutorProfileId, BaseCodingAgent, AvailabilityInfo, McpConfig, read/write_agent_config), `db::models::ModelConfig`, `utils` (assets, env_compat, git, oauth). TS types exported via `bin/generate_types.rs` into `shared/types.ts`. Frontend consumers: `lib/api.ts` (configApi), `ConfigProvider.tsx`, `useEditorAvailability.ts`, `useNativeCredentials.ts`, `AgentSettingsNew.tsx`, `RuntimeSettingsNew.tsx`, `Step0Project.tsx`. | Live, heavily-used module. Contains G1 IDE-connect deletion target (`check_editor_availability` + its query/response types). Two flagged-off / shim features: `REMOTE_FEATURES_ENABLED` const (always false) and `DISABLE_NATIVE_CREDENTIALS` env gate. |

## Key handlers / functions

| Symbol | Lines | Role | Usage status |
|---|---|---|---|
| `router()` | 41-59 | Builds the config sub-router | Merged at `routes/mod.rs:143`. Live. |
| `REMOTE_FEATURES_ENABLED` (const) | 61-65 | Compile-time false flag surfaced as `remote_features_enabled` in `/info` | Read by frontend `ConfigProvider.tsx:88` -> `remoteFeaturesEnabled`. Always false; gating shim for not-yet-shipped remote/beta-workspace stack. |
| `Environment` / `Environment::new()` | 67-102 | OS/arch/container detection + `workspace_root_hint` from `SOLODAWN_WORKSPACE_ROOT` (compat `GITCORTEX_WORKSPACE_ROOT`) | `workspace_root_hint` consumed by `Step0Project.tsx:77,201`. Live. |
| `UserSystemInfo` + `get_user_system_info` | 104-145 | `GET /info` aggregate payload | Live; primary bootstrap call. Header comment is a stale TODO (says it "replaces GET /config and /config/constants"). |
| `update_config` | 147-210 | `PUT /config` full-replace with admin gate (`SOLODAWN_ADMIN_TOKEN`/`X-Admin-Token`) | Live; full replacement (not merge), known concurrent-overwrite caveat documented inline. |
| `track_config_events` | 212-242 | Fires analytics on false->true transitions | Called only by `handle_config_events` (244). Live, internal. |
| `handle_config_events` | 244-258 | Post-save side effects: analytics, auto project setup, model-library->DB sync | Called only by `update_config`. Live, internal. |
| `sync_model_library_to_db` | 260-323 | Upsert `workflow_model_library` into `model_config` DB w/ encrypted creds | Called by `handle_config_events`. Live. |
| `get_sound` | 325-336 | `GET /sounds/{sound}` serves embedded wav | Live (notification sounds). |
| MCP block: `get_mcp_servers`/`update_mcp_servers`/`update_mcp_servers_in_config`/`get_mcp_servers_from_config_path`/`set_mcp_servers_in_config_path` | 338-573 | `GET+POST /mcp-config` per-executor MCP server read/write | Live; `lib/api.ts` + settings UI. Has unit tests (575-627). |
| `get_profiles`/`update_profiles` | 629-691 | `GET+PUT /profiles` executor-profile JSON | Live. |
| `CheckEditorAvailabilityQuery/Response` + `check_editor_availability` | 693-719 | `GET /editors/check-availability` — checks if an external IDE/editor exists | Live: frontend `useEditorAvailability.ts` -> `EditorAvailabilityIndicator` -> `GeneralSettingsNew.tsx` + `OnboardingDialog.tsx`. **BUT marked DELETE by `docs/audit/R1-ide-editor-connection-deletion-audit.md` (G1 IDE-connect feature removal).** |
| `CheckAgentAvailabilityQuery` + `check_agent_availability` | 721-739 | `GET /agents/check-availability` | Live; `useAgentAvailability`. KEEP (agent != editor). |
| `InstallAiClisResponse` + `truncate_output` + `resolve_install_single_cli_script` + `BATCH_INSTALL_CLIS` + `install_ai_clis` | 741-920 | `POST /agents/install-ai-clis` batch CLI installer (PS on Windows, bash on Unix) | Live: `AgentSettingsNew.tsx` via `lib/api.ts`. BACKLOG-003 plans to supersede with per-CLI + progress API (not yet implemented). |
| `PrerequisiteStatus`/`SystemPrerequisites` + `detect_tool_version` + `get_system_prerequisites` | 922-1029 | `GET /system/prerequisites` detects node/npm/git/gh | Live: `RuntimeSettingsNew.tsx`. |
| `NativeCredentialsStatus` + `get_native_credentials_status` | 1031-1106 | `GET /native-credentials-status` detects local Claude Code OAuth creds | Live: `useNativeCredentials.ts`. Gated off by `DISABLE_NATIVE_CREDENTIALS` env (V1 test mode shim). |

## Invisible / flagged features

- `REMOTE_FEATURES_ENABLED` (const false): reserved gate for the unshipped remote/beta-workspaces/shared-cloud stack. Surfaced to FE as `remoteFeaturesEnabled` and consumed by `ConfigProvider`; the value is just hardwired false until backing services ship.
- `DISABLE_NATIVE_CREDENTIALS` env: V1/test shim that forces native Claude Code credential detection off so only explicitly-configured models (e.g. GLM-5.1) appear. Matches MEMORY note.
- Admin gate in `update_config`: opt-in via `SOLODAWN_ADMIN_TOKEN` + `X-Admin-Token`; no-op when unset (single-user/dev). G24 TODO to replace with RBAC claims on `RequestContext`.
- `env_compat::var_opt_with_compat` legacy-name shim: reads `SOLODAWN_*` falling back to old `GITCORTEX_*` env names (rebrand migration compat) for workspace root and install dir.
