//! Local wire types for the Codex `app-server` JSON-RPC protocol (v2 thread/turn API).
//!
//! Ported from `openai/codex` @ `rust-v0.142.5` (`codex-rs/app-server-protocol`) so the
//! workspace does not depend on upstream's unpublished, fast-moving protocol crates.
//! Only the subset the executor actually uses is defined here. Types we deserialize
//! (responses, notifications, items) are deliberately lenient: unknown fields are
//! ignored, most fields default, and tagged enums carry catch-all variants so additive
//! upstream changes never break parsing.

use std::{collections::HashMap, path::PathBuf};

use serde::{Deserialize, Serialize};
use serde_json::Value;

// === JSON-RPC envelope ===
// Codex app-server speaks JSON-RPC without the `"jsonrpc": "2.0"` field
// (upstream `jsonrpc_lite.rs`).

#[derive(Debug, Clone, PartialEq, Eq, PartialOrd, Ord, Hash, Serialize, Deserialize)]
#[serde(untagged)]
pub enum RequestId {
    String(String),
    Integer(i64),
}

/// Any valid JSON-RPC object that can be decoded off the wire.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(untagged)]
pub enum JSONRPCMessage {
    Request(JSONRPCRequest),
    Notification(JSONRPCNotification),
    Response(JSONRPCResponse),
    Error(JSONRPCError),
}

/// A request that expects a response.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct JSONRPCRequest {
    pub id: RequestId,
    pub method: String,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub params: Option<Value>,
}

/// A notification which does not expect a response.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct JSONRPCNotification {
    pub method: String,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub params: Option<Value>,
}

/// A successful (non-error) response to a request.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct JSONRPCResponse {
    pub id: RequestId,
    pub result: Value,
}

/// A response to a request that indicates an error occurred.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct JSONRPCError {
    pub error: JSONRPCErrorError,
    pub id: RequestId,
}

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct JSONRPCErrorError {
    pub code: i64,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub data: Option<Value>,
    pub message: String,
}

// === Client requests ===

/// Requests sent from this client to the Codex app-server.
///
/// Serialized as `{"method": "...", "id": ..., "params": {...}}`, matching the
/// upstream `client_request_definitions!` macro output.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(tag = "method", rename_all = "camelCase")]
pub enum ClientRequest {
    Initialize {
        #[serde(rename = "id")]
        request_id: RequestId,
        params: InitializeParams,
    },
    #[serde(rename = "thread/start")]
    ThreadStart {
        #[serde(rename = "id")]
        request_id: RequestId,
        params: ThreadStartParams,
    },
    #[serde(rename = "thread/resume")]
    ThreadResume {
        #[serde(rename = "id")]
        request_id: RequestId,
        params: ThreadResumeParams,
    },
    #[serde(rename = "thread/fork")]
    ThreadFork {
        #[serde(rename = "id")]
        request_id: RequestId,
        params: ThreadForkParams,
    },
    #[serde(rename = "turn/start")]
    TurnStart {
        #[serde(rename = "id")]
        request_id: RequestId,
        params: TurnStartParams,
    },
    GetAuthStatus {
        #[serde(rename = "id")]
        request_id: RequestId,
        params: GetAuthStatusParams,
    },
    #[serde(rename = "review/start")]
    ReviewStart {
        #[serde(rename = "id")]
        request_id: RequestId,
        params: ReviewStartParams,
    },
}

/// Notifications sent from this client to the Codex app-server.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(tag = "method", content = "params", rename_all = "camelCase")]
pub enum ClientNotification {
    Initialized,
}

// === initialize ===

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct InitializeParams {
    pub client_info: ClientInfo,
}

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct ClientInfo {
    pub name: String,
    pub title: Option<String>,
    pub version: String,
}

/// Lenient: the v2 response gained several additive fields (codexHome,
/// platformFamily, ...) which we do not consume.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct InitializeResponse {
    #[serde(default)]
    pub user_agent: Option<String>,
}

// === getAuthStatus ===

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct GetAuthStatusParams {
    pub include_token: Option<bool>,
    pub refresh_token: Option<bool>,
}

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct GetAuthStatusResponse {
    #[serde(default)]
    pub auth_method: Option<AuthMode>,
    #[serde(default)]
    pub auth_token: Option<String>,
    #[serde(default)]
    pub requires_openai_auth: Option<bool>,
}

/// Authentication mode for OpenAI-backed providers. Tolerates unknown modes so
/// new upstream variants never fail the auth check.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "lowercase")]
pub enum AuthMode {
    ApiKey,
    Chatgpt,
    #[serde(rename = "chatgptAuthTokens")]
    ChatgptAuthTokens,
    #[serde(rename = "agentIdentity")]
    AgentIdentity,
    #[serde(rename = "personalAccessToken")]
    PersonalAccessToken,
    #[serde(other)]
    Unknown,
}

// === Shared config enums ===

/// Determines when the user is consulted to approve Codex actions.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "kebab-case")]
pub enum AskForApproval {
    #[serde(rename = "untrusted")]
    UnlessTrusted,
    OnFailure,
    OnRequest,
    Never,
}

/// Sandbox mode for Codex command execution.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "kebab-case")]
pub enum SandboxMode {
    ReadOnly,
    WorkspaceWrite,
    DangerFullAccess,
}

// === Thread lifecycle ===

#[derive(Debug, Clone, Default, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct ThreadStartParams {
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub model: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub model_provider: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub cwd: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub approval_policy: Option<AskForApproval>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub sandbox: Option<SandboxMode>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub config: Option<HashMap<String, Value>>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub base_instructions: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub developer_instructions: Option<String>,
}

/// Overrides are flattened inline at the top level, not nested under `overrides`.
#[derive(Debug, Clone, Default, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct ThreadResumeParams {
    pub thread_id: String,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub model: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub model_provider: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub cwd: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub approval_policy: Option<AskForApproval>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub sandbox: Option<SandboxMode>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub config: Option<HashMap<String, Value>>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub base_instructions: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub developer_instructions: Option<String>,
}

/// Fork loads the source thread from disk and starts a live copy under a new
/// thread id, replacing the old manual rollout-file copy + resume dance.
#[derive(Debug, Clone, Default, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct ThreadForkParams {
    pub thread_id: String,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub model: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub model_provider: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub cwd: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub approval_policy: Option<AskForApproval>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub sandbox: Option<SandboxMode>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub config: Option<HashMap<String, Value>>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub base_instructions: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub developer_instructions: Option<String>,
}

impl ThreadForkParams {
    pub fn from_thread_start(thread_id: String, overrides: ThreadStartParams) -> Self {
        Self {
            thread_id,
            model: overrides.model,
            model_provider: overrides.model_provider,
            cwd: overrides.cwd,
            approval_policy: overrides.approval_policy,
            sandbox: overrides.sandbox,
            config: overrides.config,
            base_instructions: overrides.base_instructions,
            developer_instructions: overrides.developer_instructions,
        }
    }
}

/// Lenient projection of the upstream `ThreadStartResponse` /
/// `ThreadResumeResponse` / `ThreadForkResponse`, which all share this shape.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct ThreadStartResponse {
    pub thread: Thread,
    #[serde(default)]
    pub model: Option<String>,
    #[serde(default)]
    pub reasoning_effort: Option<String>,
}

pub type ThreadResumeResponse = ThreadStartResponse;
pub type ThreadForkResponse = ThreadStartResponse;

/// Lenient: upstream `Thread` carries many more fields; only the id is used.
/// `thread.id` is the session id persisted for follow-ups.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct Thread {
    pub id: String,
}

/// Lenient projection of the upstream `Turn`.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct Turn {
    pub id: String,
    #[serde(default)]
    pub status: TurnStatus,
}

#[derive(Debug, Clone, Copy, Default, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub enum TurnStatus {
    Completed,
    Interrupted,
    Failed,
    #[default]
    InProgress,
    #[serde(other)]
    Unknown,
}

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct TurnError {
    pub message: String,
    #[serde(default)]
    pub codex_error_info: Option<Value>,
    #[serde(default)]
    pub additional_details: Option<String>,
}

// === Turn input ===

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(tag = "type", rename_all = "camelCase")]
pub enum UserInput {
    #[serde(rename_all = "camelCase")]
    Text {
        text: String,
        /// UI-defined spans within `text`; always sent (empty) per the v2 wire shape.
        #[serde(default)]
        text_elements: Vec<Value>,
    },
}

impl UserInput {
    pub fn text(text: String) -> Self {
        Self::Text {
            text,
            text_elements: Vec::new(),
        }
    }
}

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct TurnStartParams {
    pub thread_id: String,
    pub input: Vec<UserInput>,
}

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct TurnStartResponse {
    pub turn: Turn,
}

// === review/start ===

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct ReviewStartParams {
    pub thread_id: String,
    pub target: ReviewTarget,
    /// Where to run the review: inline (default) on the current thread or
    /// detached on a new thread.
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub delivery: Option<ReviewDelivery>,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub enum ReviewDelivery {
    Inline,
    Detached,
}

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(tag = "type", rename_all = "camelCase")]
pub enum ReviewTarget {
    /// Review the working tree: staged, unstaged, and untracked files.
    UncommittedChanges,
    /// Review changes between the current branch and the given base branch.
    #[serde(rename_all = "camelCase")]
    BaseBranch { branch: String },
    /// Review the changes introduced by a specific commit.
    #[serde(rename_all = "camelCase")]
    Commit { sha: String, title: Option<String> },
    /// Arbitrary instructions, equivalent to the old free-form prompt.
    #[serde(rename_all = "camelCase")]
    Custom { instructions: String },
}

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct ReviewStartResponse {
    /// Thread where the review runs (the original thread for inline reviews).
    #[serde(default)]
    pub review_thread_id: Option<String>,
}

// === Thread items ===

/// Internally tagged (`"type"`) item payload carried by `item/started` and
/// `item/completed`. Unknown item kinds decode as [`ThreadItem::Other`] and
/// must never fail deserialization.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(tag = "type", rename_all = "camelCase")]
pub enum ThreadItem {
    #[serde(rename_all = "camelCase")]
    AgentMessage { id: String, text: String },
    #[serde(rename_all = "camelCase")]
    Reasoning {
        id: String,
        #[serde(default)]
        summary: Vec<String>,
        #[serde(default)]
        content: Vec<String>,
    },
    #[serde(rename_all = "camelCase")]
    CommandExecution {
        id: String,
        /// The command as a single shell string (no longer an argv array).
        command: String,
        #[serde(default)]
        status: CommandExecutionStatus,
        /// The command's output, aggregated from stdout and stderr.
        #[serde(default)]
        aggregated_output: Option<String>,
        #[serde(default)]
        exit_code: Option<i32>,
        #[serde(default)]
        duration_ms: Option<i64>,
    },
    #[serde(rename_all = "camelCase")]
    FileChange {
        id: String,
        #[serde(default)]
        changes: Vec<FileUpdateChange>,
        #[serde(default)]
        status: PatchApplyStatus,
    },
    #[serde(rename_all = "camelCase")]
    McpToolCall {
        id: String,
        server: String,
        tool: String,
        #[serde(default)]
        status: McpToolCallStatus,
        #[serde(default)]
        arguments: Value,
        #[serde(default)]
        result: Option<McpToolCallResult>,
        #[serde(default)]
        error: Option<McpToolCallError>,
    },
    #[serde(rename_all = "camelCase")]
    WebSearch {
        id: String,
        #[serde(default)]
        query: String,
    },
    #[serde(rename_all = "camelCase")]
    ImageView { id: String, path: PathBuf },
    #[serde(rename_all = "camelCase")]
    ContextCompaction { id: String },
    /// Catch-all for item kinds this executor does not render.
    #[serde(other)]
    Other,
}

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct FileUpdateChange {
    pub path: String,
    pub kind: PatchChangeKind,
    /// For `add`/`delete` this is the full file content; for `update` it is a
    /// unified diff (with `\n\nMoved to: <path>` appended when the file moved).
    #[serde(default)]
    pub diff: String,
}

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(tag = "type", rename_all = "camelCase")]
pub enum PatchChangeKind {
    Add,
    Delete,
    // Upstream does not camelCase this variant's fields; keep `move_path` as-is.
    Update { move_path: Option<PathBuf> },
}

#[derive(Debug, Clone, Copy, Default, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub enum CommandExecutionStatus {
    #[default]
    InProgress,
    Completed,
    Failed,
    Declined,
    #[serde(other)]
    Unknown,
}

#[derive(Debug, Clone, Copy, Default, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub enum PatchApplyStatus {
    #[default]
    InProgress,
    Completed,
    Failed,
    Declined,
    #[serde(other)]
    Unknown,
}

#[derive(Debug, Clone, Copy, Default, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub enum McpToolCallStatus {
    #[default]
    InProgress,
    Completed,
    Failed,
    #[serde(other)]
    Unknown,
}

/// MCP content blocks stay wire-shaped (`serde_json::Value`), mirroring upstream.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct McpToolCallResult {
    #[serde(default)]
    pub content: Vec<Value>,
    #[serde(default)]
    pub structured_content: Option<Value>,
}

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct McpToolCallError {
    pub message: String,
}

// === Server notifications ===

/// Method name of the notification that terminates a turn.
pub const TURN_COMPLETED_METHOD: &str = "turn/completed";

/// Notifications emitted by the app-server that this executor consumes.
/// Methods not listed here simply fail to parse and are skipped by callers.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(tag = "method", content = "params", rename_all = "camelCase")]
pub enum ServerNotification {
    #[serde(rename = "thread/started")]
    ThreadStarted(ThreadStartedNotification),
    #[serde(rename = "item/started")]
    ItemStarted(ItemStartedNotification),
    #[serde(rename = "item/completed")]
    ItemCompleted(ItemCompletedNotification),
    #[serde(rename = "item/agentMessage/delta")]
    AgentMessageDelta(TextDeltaNotification),
    #[serde(rename = "item/reasoning/summaryTextDelta")]
    ReasoningSummaryTextDelta(TextDeltaNotification),
    #[serde(rename = "item/reasoning/textDelta")]
    ReasoningTextDelta(TextDeltaNotification),
    #[serde(rename = "item/reasoning/summaryPartAdded")]
    ReasoningSummaryPartAdded(ReasoningSummaryPartAddedNotification),
    #[serde(rename = "item/commandExecution/outputDelta")]
    CommandExecutionOutputDelta(CommandExecutionOutputDeltaNotification),
    #[serde(rename = "item/fileChange/patchUpdated")]
    FileChangePatchUpdated(FileChangePatchUpdatedNotification),
    #[serde(rename = "turn/completed")]
    TurnCompleted(TurnCompletedNotification),
    #[serde(rename = "turn/plan/updated")]
    TurnPlanUpdated(TurnPlanUpdatedNotification),
    Error(ErrorNotification),
    Warning(WarningNotification),
}

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct ThreadStartedNotification {
    pub thread: Thread,
}

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct ItemStartedNotification {
    pub item: ThreadItem,
}

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct ItemCompletedNotification {
    pub item: ThreadItem,
}

/// Shared shape for `item/agentMessage/delta` and the `item/reasoning/*Delta`
/// notifications; the item/turn ids are not needed for log normalization.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct TextDeltaNotification {
    pub delta: String,
}

/// Section boundary within a reasoning summary; payload fields are unused.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct ReasoningSummaryPartAddedNotification {}

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct CommandExecutionOutputDeltaNotification {
    pub item_id: String,
    pub delta: String,
}

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct FileChangePatchUpdatedNotification {
    pub item_id: String,
    #[serde(default)]
    pub changes: Vec<FileUpdateChange>,
}

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct TurnCompletedNotification {
    pub turn: Turn,
}

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct TurnPlanUpdatedNotification {
    #[serde(default)]
    pub explanation: Option<String>,
    #[serde(default)]
    pub plan: Vec<TurnPlanStep>,
}

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct TurnPlanStep {
    pub step: String,
    pub status: TurnPlanStepStatus,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub enum TurnPlanStepStatus {
    Pending,
    InProgress,
    Completed,
    #[serde(other)]
    Unknown,
}

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct ErrorNotification {
    pub error: TurnError,
    /// True when the error is transient and the app-server will retry; such
    /// errors do not interrupt the turn.
    #[serde(default)]
    pub will_retry: bool,
}

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct WarningNotification {
    pub message: String,
}

// === Server requests (approvals) ===

/// Requests initiated by the app-server that expect a response from us.
///
/// The legacy v1 `applyPatchApproval` / `execCommandApproval` variants are kept
/// because they remain wire-valid; Codex 0.142.x only sends the v2
/// `item/*/requestApproval` forms for turns started via `turn/start`.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(tag = "method", rename_all = "camelCase")]
pub enum ServerRequest {
    ApplyPatchApproval {
        #[serde(rename = "id")]
        request_id: RequestId,
        params: ApplyPatchApprovalParams,
    },
    ExecCommandApproval {
        #[serde(rename = "id")]
        request_id: RequestId,
        params: ExecCommandApprovalParams,
    },
    #[serde(rename = "item/commandExecution/requestApproval")]
    CommandExecutionRequestApproval {
        #[serde(rename = "id")]
        request_id: RequestId,
        params: CommandExecutionRequestApprovalParams,
    },
    #[serde(rename = "item/fileChange/requestApproval")]
    FileChangeRequestApproval {
        #[serde(rename = "id")]
        request_id: RequestId,
        params: FileChangeRequestApprovalParams,
    },
}

impl TryFrom<JSONRPCRequest> for ServerRequest {
    type Error = serde_json::Error;

    fn try_from(value: JSONRPCRequest) -> Result<Self, Self::Error> {
        serde_json::from_value(serde_json::to_value(value)?)
    }
}

/// Legacy v1 params; only `call_id` is consumed, the rest is preserved so the
/// full request can be forwarded verbatim to the approval service.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct ApplyPatchApprovalParams {
    pub call_id: String,
    #[serde(flatten)]
    pub rest: serde_json::Map<String, Value>,
}

/// Legacy v1 params; see [`ApplyPatchApprovalParams`].
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct ExecCommandApprovalParams {
    pub call_id: String,
    #[serde(flatten)]
    pub rest: serde_json::Map<String, Value>,
}

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct CommandExecutionRequestApprovalParams {
    pub item_id: String,
    /// The command to be executed (absent for network-access prompts).
    #[serde(default)]
    pub command: Option<String>,
    /// Optional explanatory reason (e.g. request for network access).
    #[serde(default)]
    pub reason: Option<String>,
    #[serde(flatten)]
    pub rest: serde_json::Map<String, Value>,
}

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct FileChangeRequestApprovalParams {
    pub item_id: String,
    #[serde(default)]
    pub reason: Option<String>,
    #[serde(flatten)]
    pub rest: serde_json::Map<String, Value>,
}

/// Legacy v1 decision (snake_case on the wire).
#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub enum ReviewDecision {
    Approved,
    ApprovedForSession,
    Denied,
    Abort,
}

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct ApplyPatchApprovalResponse {
    pub decision: ReviewDecision,
}

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct ExecCommandApprovalResponse {
    pub decision: ReviewDecision,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub enum CommandExecutionApprovalDecision {
    /// User approved the command.
    Accept,
    /// User approved the command for the rest of the session.
    AcceptForSession,
    /// User denied the command; the agent continues the turn.
    Decline,
    /// User denied the command; the turn is interrupted.
    Cancel,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub enum FileChangeApprovalDecision {
    /// User approved the file changes.
    Accept,
    /// User approved the file changes for the rest of the session.
    AcceptForSession,
    /// User denied the file changes; the agent continues the turn.
    Decline,
    /// User denied the file changes; the turn is interrupted.
    Cancel,
}

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct CommandExecutionRequestApprovalResponse {
    pub decision: CommandExecutionApprovalDecision,
}

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct FileChangeRequestApprovalResponse {
    pub decision: FileChangeApprovalDecision,
}

#[cfg(test)]
mod tests {
    use serde_json::json;

    use super::*;

    #[test]
    fn turn_start_params_encode_user_input_text_without_data_wrapper() {
        let params = TurnStartParams {
            thread_id: "thread-1".to_string(),
            input: vec![UserInput::text("hello world".to_string())],
        };

        let encoded = serde_json::to_value(&params).expect("serialize TurnStartParams");
        assert_eq!(
            encoded,
            json!({
                "threadId": "thread-1",
                "input": [
                    {"type": "text", "text": "hello world", "textElements": []}
                ]
            })
        );

        let decoded: TurnStartParams =
            serde_json::from_value(encoded).expect("round-trip TurnStartParams");
        assert_eq!(decoded, params);
    }

    #[test]
    fn thread_item_command_execution_decodes_v2_shape() {
        let item: ThreadItem = serde_json::from_value(json!({
            "type": "commandExecution",
            "id": "item_3",
            "command": "cargo test -p executors",
            "cwd": "/repo",
            "processId": null,
            "source": "agent",
            "status": "completed",
            "commandActions": [{"type": "unknown", "command": "cargo test -p executors"}],
            "aggregatedOutput": "ok. 12 passed",
            "exitCode": 0,
            "durationMs": 5120
        }))
        .expect("decode CommandExecution item");

        match item {
            ThreadItem::CommandExecution {
                id,
                command,
                status,
                aggregated_output,
                exit_code,
                duration_ms,
            } => {
                assert_eq!(id, "item_3");
                assert_eq!(command, "cargo test -p executors");
                assert_eq!(status, CommandExecutionStatus::Completed);
                assert_eq!(aggregated_output.as_deref(), Some("ok. 12 passed"));
                assert_eq!(exit_code, Some(0));
                assert_eq!(duration_ms, Some(5120));
            }
            other => panic!("expected CommandExecution, got {other:?}"),
        }
    }

    #[test]
    fn thread_item_tolerates_unknown_kinds() {
        let item: ThreadItem = serde_json::from_value(json!({
            "type": "todoList",
            "id": "item_9",
            "items": [{"text": "step", "completed": false}]
        }))
        .expect("unknown item kinds must not error");
        assert_eq!(item, ThreadItem::Other);
    }

    #[test]
    fn client_request_serializes_method_and_id() {
        let request = ClientRequest::ThreadStart {
            request_id: RequestId::Integer(7),
            params: ThreadStartParams {
                model: Some("gpt-5.2".to_string()),
                cwd: Some("/repo".to_string()),
                approval_policy: Some(AskForApproval::UnlessTrusted),
                sandbox: Some(SandboxMode::DangerFullAccess),
                ..Default::default()
            },
        };

        let encoded = serde_json::to_value(&request).expect("serialize ClientRequest");
        assert_eq!(
            encoded,
            json!({
                "method": "thread/start",
                "id": 7,
                "params": {
                    "model": "gpt-5.2",
                    "cwd": "/repo",
                    "approvalPolicy": "untrusted",
                    "sandbox": "danger-full-access"
                }
            })
        );
    }

    #[test]
    fn approval_decisions_use_camel_case_wire_values() {
        assert_eq!(
            serde_json::to_value(CommandExecutionApprovalDecision::AcceptForSession).unwrap(),
            json!("acceptForSession")
        );
        assert_eq!(
            serde_json::to_value(FileChangeApprovalDecision::Decline).unwrap(),
            json!("decline")
        );
        assert_eq!(
            serde_json::to_value(ReviewDecision::ApprovedForSession).unwrap(),
            json!("approved_for_session")
        );
    }

    #[test]
    fn turn_completed_notification_decodes_statuses() {
        let notification: ServerNotification = serde_json::from_str(
            r#"{"method":"turn/completed","params":{"threadId":"t","turn":{"id":"turn_1","items":[],"status":"interrupted","error":null}}}"#,
        )
        .expect("decode turn/completed");
        match notification {
            ServerNotification::TurnCompleted(payload) => {
                assert_eq!(payload.turn.status, TurnStatus::Interrupted);
            }
            other => panic!("expected TurnCompleted, got {other:?}"),
        }

        // Unknown statuses must decode instead of erroring.
        let turn: Turn = serde_json::from_value(json!({"id": "turn_2", "status": "paused"}))
            .expect("unknown turn status must not error");
        assert_eq!(turn.status, TurnStatus::Unknown);
    }
}
