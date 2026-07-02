use std::{collections::HashMap, path::Path, sync::Arc, time::Duration};

use futures::StreamExt;
use once_cell::sync::Lazy;
use regex::Regex;
use serde::{Deserialize, Serialize};
use serde_json::Value;
use workspace_utils::{
    approvals::ApprovalStatus, diff::normalize_unified_diff, msg_store::MsgStore,
    path::make_path_relative,
};

use crate::{
    approvals::ToolCallMetadata,
    executors::codex::protocol::{
        CommandExecutionRequestApprovalParams, CommandExecutionStatus, FileUpdateChange,
        JSONRPCResponse, McpToolCallError, McpToolCallResult, McpToolCallStatus, PatchApplyStatus,
        PatchChangeKind, ServerNotification, ServerRequest, ThreadItem, ThreadStartResponse,
        TurnPlanStepStatus, TurnPlanUpdatedNotification,
    },
    logs::{
        ActionType, CommandExitStatus, CommandRunResult, FileChange, NormalizedEntry,
        NormalizedEntryError, NormalizedEntryType, TodoItem, ToolResult, ToolResultValueType,
        ToolStatus,
        plain_text_processor::PlainTextLogProcessor,
        utils::{
            ConversationPatch, EntryIndexProvider,
            patch::{add_normalized_entry, replace_normalized_entry, upsert_normalized_entry},
        },
    },
};

trait ToNormalizedEntry {
    fn to_normalized_entry(&self) -> NormalizedEntry;
}

trait ToNormalizedEntryOpt {
    fn to_normalized_entry_opt(&self) -> Option<NormalizedEntry>;
}

#[derive(Default)]
struct StreamingText {
    index: usize,
    content: String,
}

#[derive(Default)]
struct CommandState {
    index: Option<usize>,
    command: String,
    /// Output aggregated from stdout and stderr, streamed via
    /// `item/commandExecution/outputDelta`.
    output: String,
    formatted_output: Option<String>,
    status: ToolStatus,
    exit_code: Option<i32>,
    item_id: String,
}

impl ToNormalizedEntry for CommandState {
    fn to_normalized_entry(&self) -> NormalizedEntry {
        let content = self.command.clone();

        NormalizedEntry {
            timestamp: None,
            entry_type: NormalizedEntryType::ToolUse {
                tool_name: "bash".to_string(),
                action_type: ActionType::CommandRun {
                    command: self.command.clone(),
                    result: Some(CommandRunResult {
                        exit_status: self
                            .exit_code
                            .map(|code| CommandExitStatus::ExitCode { code }),
                        output: if self.formatted_output.is_some() {
                            self.formatted_output.clone()
                        } else {
                            build_command_output(&self.output)
                        },
                    }),
                },
                status: self.status.clone(),
            },
            content,
            metadata: serde_json::to_value(ToolCallMetadata {
                tool_call_id: self.item_id.clone(),
            })
            .ok(),
        }
    }
}

struct McpToolState {
    index: Option<usize>,
    server: String,
    tool: String,
    arguments: Option<Value>,
    result: Option<ToolResult>,
    status: ToolStatus,
}

impl ToNormalizedEntry for McpToolState {
    fn to_normalized_entry(&self) -> NormalizedEntry {
        let tool_name = format!("mcp:{}:{}", self.server, self.tool);
        NormalizedEntry {
            timestamp: None,
            entry_type: NormalizedEntryType::ToolUse {
                tool_name: tool_name.clone(),
                action_type: ActionType::Tool {
                    tool_name,
                    arguments: self.arguments.clone(),
                    result: self.result.clone(),
                },
                status: self.status.clone(),
            },
            content: self.tool.clone(),
            metadata: None,
        }
    }
}

#[derive(Default)]
struct WebSearchState {
    index: Option<usize>,
    query: Option<String>,
    status: ToolStatus,
}

impl ToNormalizedEntry for WebSearchState {
    fn to_normalized_entry(&self) -> NormalizedEntry {
        NormalizedEntry {
            timestamp: None,
            entry_type: NormalizedEntryType::ToolUse {
                tool_name: "web_search".to_string(),
                action_type: ActionType::WebFetch {
                    url: self.query.clone().unwrap_or_else(|| "...".to_string()),
                },
                status: self.status.clone(),
            },
            content: self
                .query
                .clone()
                .unwrap_or_else(|| "Web search".to_string()),
            metadata: None,
        }
    }
}

#[derive(Default)]
struct PatchState {
    entries: Vec<PatchEntry>,
}

struct PatchEntry {
    index: Option<usize>,
    path: String,
    changes: Vec<FileChange>,
    status: ToolStatus,
    item_id: String,
}

impl ToNormalizedEntry for PatchEntry {
    fn to_normalized_entry(&self) -> NormalizedEntry {
        let content = self.path.clone();

        NormalizedEntry {
            timestamp: None,
            entry_type: NormalizedEntryType::ToolUse {
                tool_name: "edit".to_string(),
                action_type: ActionType::FileEdit {
                    path: self.path.clone(),
                    changes: self.changes.clone(),
                },
                status: self.status.clone(),
            },
            content,
            metadata: serde_json::to_value(ToolCallMetadata {
                tool_call_id: self.item_id.clone(),
            })
            .ok(),
        }
    }
}

struct LogState {
    entry_index: EntryIndexProvider,
    assistant: Option<StreamingText>,
    thinking: Option<StreamingText>,
    commands: HashMap<String, CommandState>,
    mcp_tools: HashMap<String, McpToolState>,
    patches: HashMap<String, PatchState>,
    web_searches: HashMap<String, WebSearchState>,
}

#[derive(Clone, Copy)]
enum StreamingTextKind {
    Assistant,
    Thinking,
}

impl LogState {
    fn new(entry_index: EntryIndexProvider) -> Self {
        Self {
            entry_index,
            assistant: None,
            thinking: None,
            commands: HashMap::new(),
            mcp_tools: HashMap::new(),
            patches: HashMap::new(),
            web_searches: HashMap::new(),
        }
    }

    fn streaming_text_update(
        &mut self,
        content: String,
        type_: StreamingTextKind,
        mode: UpdateMode,
    ) -> (NormalizedEntry, usize, bool) {
        let index_provider = &self.entry_index;
        let entry = match type_ {
            StreamingTextKind::Assistant => &mut self.assistant,
            StreamingTextKind::Thinking => &mut self.thinking,
        };
        let is_new = entry.is_none();
        let (content, index) = if entry.is_none() {
            let index = index_provider.next();
            *entry = Some(StreamingText { index, content });
            (&entry.as_ref().unwrap().content, index)
        } else {
            let streaming_state = entry.as_mut().unwrap();
            match mode {
                UpdateMode::Append => streaming_state.content.push_str(&content),
                UpdateMode::Set => streaming_state.content = content,
            }
            (&streaming_state.content, streaming_state.index)
        };
        let normalized_entry = NormalizedEntry {
            timestamp: None,
            entry_type: match type_ {
                StreamingTextKind::Assistant => NormalizedEntryType::AssistantMessage,
                StreamingTextKind::Thinking => NormalizedEntryType::Thinking,
            },
            content: content.clone(),
            metadata: None,
        };
        (normalized_entry, index, is_new)
    }

    fn streaming_text_append(
        &mut self,
        content: String,
        type_: StreamingTextKind,
    ) -> (NormalizedEntry, usize, bool) {
        self.streaming_text_update(content, type_, UpdateMode::Append)
    }

    fn streaming_text_set(
        &mut self,
        content: String,
        type_: StreamingTextKind,
    ) -> (NormalizedEntry, usize, bool) {
        self.streaming_text_update(content, type_, UpdateMode::Set)
    }

    fn assistant_message_append(&mut self, content: String) -> (NormalizedEntry, usize, bool) {
        self.streaming_text_append(content, StreamingTextKind::Assistant)
    }

    fn thinking_append(&mut self, content: String) -> (NormalizedEntry, usize, bool) {
        self.streaming_text_append(content, StreamingTextKind::Thinking)
    }

    fn assistant_message(&mut self, content: String) -> (NormalizedEntry, usize, bool) {
        self.streaming_text_set(content, StreamingTextKind::Assistant)
    }

    fn thinking(&mut self, content: String) -> (NormalizedEntry, usize, bool) {
        self.streaming_text_set(content, StreamingTextKind::Thinking)
    }
}

#[derive(Clone, Copy)]
enum UpdateMode {
    Append,
    Set,
}

/// Converts v2 `FileUpdateChange` payloads into normalized file changes.
///
/// The v2 `diff` field carries the full file content for `add`/`delete` kinds
/// and a unified diff for `update` (with a `\n\nMoved to: <path>` suffix
/// appended when the file was moved).
fn normalize_file_changes(
    worktree_path: &str,
    changes: &[FileUpdateChange],
) -> Vec<(String, Vec<FileChange>)> {
    changes
        .iter()
        .map(|change| {
            let relative = make_path_relative(&change.path, worktree_path);
            let file_changes = match &change.kind {
                PatchChangeKind::Add => vec![FileChange::Write {
                    content: change.diff.clone(),
                }],
                PatchChangeKind::Delete => vec![FileChange::Delete],
                PatchChangeKind::Update { move_path } => {
                    let mut edits = Vec::new();
                    let mut unified_diff = change.diff.clone();
                    if let Some(dest) = move_path {
                        let dest_rel =
                            make_path_relative(dest.to_string_lossy().as_ref(), worktree_path);
                        edits.push(FileChange::Rename { new_path: dest_rel });
                        if let Some(idx) = unified_diff.rfind("\n\nMoved to: ") {
                            unified_diff.truncate(idx);
                        }
                    }
                    let diff = normalize_unified_diff(&relative, &unified_diff);
                    edits.push(FileChange::Edit {
                        unified_diff: diff,
                        has_line_numbers: true,
                    });
                    edits
                }
            };
            (relative, file_changes)
        })
        .collect()
}

fn format_todo_status(status: TurnPlanStepStatus) -> String {
    match status {
        TurnPlanStepStatus::Pending | TurnPlanStepStatus::Unknown => "pending",
        TurnPlanStepStatus::InProgress => "in_progress",
        TurnPlanStepStatus::Completed => "completed",
    }
    .to_string()
}

/// Regex matching npm warning/notice lines that should be filtered from Codex stderr output.
static NPM_NOISE_RE: Lazy<Regex> =
    Lazy::new(|| Regex::new(r"(?i)^npm (warn|WARN|notice|ERR!)").expect("valid regex"));

pub fn normalize_logs(msg_store: Arc<MsgStore>, worktree_path: &Path) {
    let entry_index = EntryIndexProvider::start_from(&msg_store);

    // Codex-specific stderr normalizer: filters npm warnings before display
    {
        let msg_store = msg_store.clone();
        let entry_index = entry_index.clone();
        tokio::spawn(async move {
            let mut stderr = msg_store.stderr_chunked_stream();
            let mut processor = PlainTextLogProcessor::builder()
                .normalized_entry_producer(|content: String| NormalizedEntry {
                    timestamp: None,
                    entry_type: NormalizedEntryType::ErrorMessage {
                        error_type: NormalizedEntryError::Other,
                    },
                    content: strip_ansi_escapes::strip_str(&content),
                    metadata: None,
                })
                .time_gap(Duration::from_secs(2))
                .transform_lines(Box::new(|lines: &mut Vec<String>| {
                    lines.retain(|line| !NPM_NOISE_RE.is_match(line.trim()));
                }))
                .index_provider(entry_index)
                .build();
            while let Some(Ok(chunk)) = stderr.next().await {
                for patch in processor.process(chunk) {
                    msg_store.push_patch(patch);
                }
            }
        });
    }

    let worktree_path_str = worktree_path.to_string_lossy().to_string();
    tokio::spawn(async move {
        let mut state = LogState::new(entry_index.clone());
        let mut stdout_lines = msg_store.stdout_lines_stream();

        while let Some(Ok(line)) = stdout_lines.next().await {
            if let Ok(error) = serde_json::from_str::<Error>(&line) {
                add_normalized_entry(&msg_store, &entry_index, error.to_normalized_entry());
                continue;
            }

            if let Ok(approval) = serde_json::from_str::<Approval>(&line) {
                if let Some(entry) = approval.to_normalized_entry_opt() {
                    add_normalized_entry(&msg_store, &entry_index, entry);
                }
                continue;
            }

            if let Ok(response) = serde_json::from_str::<JSONRPCResponse>(&line) {
                handle_jsonrpc_response(&response, &msg_store, &entry_index);
                continue;
            }

            // Approval prompts arrive as server-initiated requests; surface the
            // pending command like the old exec_approval_request event did.
            if let Ok(server_request) = serde_json::from_str::<ServerRequest>(&line) {
                if let ServerRequest::CommandExecutionRequestApproval { params, .. } =
                    server_request
                {
                    handle_command_approval_request(&mut state, params, &msg_store, &entry_index);
                }
                continue;
            }

            let notification: ServerNotification = match serde_json::from_str(&line) {
                Ok(value) => value,
                Err(_) => continue,
            };

            match notification {
                ServerNotification::ThreadStarted(payload) => {
                    msg_store.push_session_id(payload.thread.id);
                }
                ServerNotification::AgentMessageDelta(payload) => {
                    state.thinking = None;
                    let (entry, index, is_new) = state.assistant_message_append(payload.delta);
                    upsert_normalized_entry(&msg_store, index, entry, is_new);
                }
                ServerNotification::ReasoningSummaryTextDelta(payload)
                | ServerNotification::ReasoningTextDelta(payload) => {
                    state.assistant = None;
                    let (entry, index, is_new) = state.thinking_append(payload.delta);
                    upsert_normalized_entry(&msg_store, index, entry, is_new);
                }
                ServerNotification::ReasoningSummaryPartAdded(_) => {
                    state.assistant = None;
                    state.thinking = None;
                }
                ServerNotification::ItemStarted(payload) => {
                    handle_item_started(
                        &mut state,
                        payload.item,
                        &msg_store,
                        &entry_index,
                        &worktree_path_str,
                    );
                }
                ServerNotification::ItemCompleted(payload) => {
                    handle_item_completed(
                        &mut state,
                        payload.item,
                        &msg_store,
                        &entry_index,
                        &worktree_path_str,
                    );
                }
                ServerNotification::CommandExecutionOutputDelta(payload) => {
                    if let Some(command_state) = state.commands.get_mut(&payload.item_id) {
                        if payload.delta.is_empty() {
                            continue;
                        }
                        command_state.output.push_str(&payload.delta);
                        let Some(index) = command_state.index else {
                            tracing::error!("missing entry index for existing command state");
                            continue;
                        };
                        replace_normalized_entry(
                            &msg_store,
                            index,
                            command_state.to_normalized_entry(),
                        );
                    }
                }
                ServerNotification::FileChangePatchUpdated(payload) => {
                    let normalized = normalize_file_changes(&worktree_path_str, &payload.changes);
                    apply_patch_changes(
                        &mut state,
                        &payload.item_id,
                        normalized,
                        &msg_store,
                        &entry_index,
                    );
                }
                ServerNotification::TurnPlanUpdated(payload) => {
                    handle_plan_update(payload, &msg_store, &entry_index);
                }
                ServerNotification::TurnCompleted(_) => {
                    // Exit signaling is handled by the app-server client.
                }
                ServerNotification::Error(payload) => {
                    let message = payload.error.message;
                    let codex_error_info = payload.error.codex_error_info;
                    let content = if payload.will_retry {
                        // Transient: the app-server retries on its own.
                        format!("Stream error: {message} {codex_error_info:?}")
                    } else {
                        format!("Error: {message} {codex_error_info:?}")
                    };
                    add_normalized_entry(
                        &msg_store,
                        &entry_index,
                        NormalizedEntry {
                            timestamp: None,
                            entry_type: NormalizedEntryType::ErrorMessage {
                                error_type: NormalizedEntryError::Other,
                            },
                            content,
                            metadata: None,
                        },
                    );
                }
                ServerNotification::Warning(payload) => {
                    add_normalized_entry(
                        &msg_store,
                        &entry_index,
                        NormalizedEntry {
                            timestamp: None,
                            entry_type: NormalizedEntryType::ErrorMessage {
                                error_type: NormalizedEntryError::Other,
                            },
                            content: payload.message,
                            metadata: None,
                        },
                    );
                }
            }
        }
    });
}

fn handle_command_approval_request(
    state: &mut LogState,
    params: CommandExecutionRequestApprovalParams,
    msg_store: &Arc<MsgStore>,
    entry_index: &EntryIndexProvider,
) {
    state.assistant = None;
    state.thinking = None;

    let command_text = params
        .command
        .filter(|command| !command.is_empty())
        .or_else(|| params.reason.filter(|reason| !reason.is_empty()))
        .unwrap_or_else(|| "command execution".to_string());

    let command_state = state.commands.entry(params.item_id.clone()).or_default();
    command_state.item_id = params.item_id;
    if command_state.command.is_empty() {
        command_state.command = command_text;
    }
    if let Some(index) = command_state.index {
        replace_normalized_entry(msg_store, index, command_state.to_normalized_entry());
    } else {
        let index =
            add_normalized_entry(msg_store, entry_index, command_state.to_normalized_entry());
        command_state.index = Some(index);
    }
}

fn handle_item_started(
    state: &mut LogState,
    item: ThreadItem,
    msg_store: &Arc<MsgStore>,
    entry_index: &EntryIndexProvider,
    worktree_path: &str,
) {
    match item {
        ThreadItem::CommandExecution { id, command, .. } => {
            state.assistant = None;
            state.thinking = None;
            if command.is_empty() {
                return;
            }
            let command_state = state.commands.entry(id.clone()).or_default();
            command_state.item_id = id;
            command_state.command = command;
            command_state.status = ToolStatus::Created;
            let index =
                add_normalized_entry(msg_store, entry_index, command_state.to_normalized_entry());
            command_state.index = Some(index);
        }
        ThreadItem::FileChange { id, changes, .. } => {
            state.assistant = None;
            state.thinking = None;
            let normalized = normalize_file_changes(worktree_path, &changes);
            apply_patch_changes(state, &id, normalized, msg_store, entry_index);
        }
        ThreadItem::McpToolCall {
            id,
            server,
            tool,
            arguments,
            ..
        } => {
            state.assistant = None;
            state.thinking = None;
            state.mcp_tools.insert(
                id.clone(),
                McpToolState {
                    index: None,
                    server,
                    tool,
                    arguments: (!arguments.is_null()).then_some(arguments),
                    result: None,
                    status: ToolStatus::Created,
                },
            );
            let mcp_tool_state = state.mcp_tools.get_mut(&id).unwrap();
            let index =
                add_normalized_entry(msg_store, entry_index, mcp_tool_state.to_normalized_entry());
            mcp_tool_state.index = Some(index);
        }
        ThreadItem::WebSearch { id, query } => {
            state.assistant = None;
            state.thinking = None;
            state.web_searches.insert(
                id.clone(),
                WebSearchState {
                    index: None,
                    query: (!query.is_empty()).then_some(query),
                    status: ToolStatus::Created,
                },
            );
            let web_search_state = state.web_searches.get_mut(&id).unwrap();
            let index = add_normalized_entry(
                msg_store,
                entry_index,
                web_search_state.to_normalized_entry(),
            );
            web_search_state.index = Some(index);
        }
        // Streaming text items are driven by their delta notifications and
        // finalized in `item/completed`.
        ThreadItem::AgentMessage { .. }
        | ThreadItem::Reasoning { .. }
        | ThreadItem::ImageView { .. }
        | ThreadItem::ContextCompaction { .. }
        | ThreadItem::Other => {}
    }
}

fn handle_item_completed(
    state: &mut LogState,
    item: ThreadItem,
    msg_store: &Arc<MsgStore>,
    entry_index: &EntryIndexProvider,
    worktree_path: &str,
) {
    match item {
        ThreadItem::AgentMessage { text, .. } => {
            state.thinking = None;
            let (entry, index, is_new) = state.assistant_message(text);
            upsert_normalized_entry(msg_store, index, entry, is_new);
            state.assistant = None;
        }
        ThreadItem::Reasoning { summary, .. } => {
            state.assistant = None;
            let text = summary.join("\n\n");
            if !text.is_empty() {
                let (entry, index, is_new) = state.thinking(text);
                upsert_normalized_entry(msg_store, index, entry, is_new);
            }
            state.thinking = None;
        }
        ThreadItem::CommandExecution {
            id,
            aggregated_output,
            exit_code,
            status,
            ..
        } => {
            if let Some(mut command_state) = state.commands.remove(&id) {
                command_state.formatted_output = aggregated_output;
                command_state.exit_code = exit_code;
                command_state.status = match status {
                    CommandExecutionStatus::Completed => ToolStatus::Success,
                    CommandExecutionStatus::Failed | CommandExecutionStatus::Declined => {
                        ToolStatus::Failed
                    }
                    CommandExecutionStatus::InProgress | CommandExecutionStatus::Unknown => {
                        if exit_code.unwrap_or(0) == 0 {
                            ToolStatus::Success
                        } else {
                            ToolStatus::Failed
                        }
                    }
                };
                let Some(index) = command_state.index else {
                    tracing::error!("missing entry index for existing command state");
                    return;
                };
                replace_normalized_entry(msg_store, index, command_state.to_normalized_entry());
            }
        }
        ThreadItem::FileChange { id, status, .. } => {
            if let Some(patch_state) = state.patches.remove(&id) {
                let status = match status {
                    PatchApplyStatus::Completed => ToolStatus::Success,
                    _ => ToolStatus::Failed,
                };
                for mut entry in patch_state.entries {
                    entry.status = status.clone();
                    let Some(index) = entry.index else {
                        tracing::error!("missing entry index for existing patch entry");
                        continue;
                    };
                    replace_normalized_entry(msg_store, index, entry.to_normalized_entry());
                }
            }
        }
        ThreadItem::McpToolCall {
            id,
            status,
            result,
            error,
            ..
        } => {
            if let Some(mut mcp_tool_state) = state.mcp_tools.remove(&id) {
                mcp_tool_state.status = match status {
                    McpToolCallStatus::Failed => ToolStatus::Failed,
                    McpToolCallStatus::Completed
                    | McpToolCallStatus::InProgress
                    | McpToolCallStatus::Unknown => ToolStatus::Success,
                };
                match (result, error) {
                    (_, Some(McpToolCallError { message })) => {
                        mcp_tool_state.status = ToolStatus::Failed;
                        mcp_tool_state.result = Some(ToolResult {
                            r#type: ToolResultValueType::Markdown,
                            value: Value::String(message),
                        });
                    }
                    (Some(result), None) => {
                        mcp_tool_state.result = Some(mcp_tool_result(result));
                    }
                    (None, None) => {}
                }
                let Some(index) = mcp_tool_state.index else {
                    tracing::error!("missing entry index for existing mcp tool state");
                    return;
                };
                replace_normalized_entry(msg_store, index, mcp_tool_state.to_normalized_entry());
            }
        }
        ThreadItem::WebSearch { id, query } => {
            state.assistant = None;
            state.thinking = None;
            if let Some(mut entry) = state.web_searches.remove(&id) {
                entry.status = ToolStatus::Success;
                if !query.is_empty() {
                    entry.query = Some(query);
                }
                let normalized_entry = entry.to_normalized_entry();
                let Some(index) = entry.index else {
                    tracing::error!("missing entry index for existing websearch entry");
                    return;
                };
                replace_normalized_entry(msg_store, index, normalized_entry);
            }
        }
        ThreadItem::ImageView { id: _, path } => {
            state.assistant = None;
            state.thinking = None;
            let path_str = path.to_string_lossy().to_string();
            let relative_path = make_path_relative(&path_str, worktree_path);
            add_normalized_entry(
                msg_store,
                entry_index,
                NormalizedEntry {
                    timestamp: None,
                    entry_type: NormalizedEntryType::ToolUse {
                        tool_name: "view_image".to_string(),
                        action_type: ActionType::FileRead {
                            path: relative_path.clone(),
                        },
                        status: ToolStatus::Success,
                    },
                    content: relative_path.clone(),
                    metadata: None,
                },
            );
        }
        ThreadItem::ContextCompaction { .. } => {
            add_normalized_entry(
                msg_store,
                entry_index,
                NormalizedEntry {
                    timestamp: None,
                    entry_type: NormalizedEntryType::SystemMessage,
                    content: "Context compacted".to_string(),
                    metadata: None,
                },
            );
        }
        ThreadItem::Other => {}
    }
}

/// Creates or updates the rendered patch entries for a `FileChange` item,
/// pairing new changes with existing entries in order. Entries beyond the new
/// change count are dropped and their rendered rows removed.
fn apply_patch_changes(
    state: &mut LogState,
    item_id: &str,
    normalized: Vec<(String, Vec<FileChange>)>,
    msg_store: &Arc<MsgStore>,
    entry_index: &EntryIndexProvider,
) {
    let patch_state = state.patches.entry(item_id.to_string()).or_default();
    let normalized_len = normalized.len();
    let mut iter = normalized.into_iter();
    for entry in &mut patch_state.entries {
        let Some((path, file_changes)) = iter.next() else {
            break;
        };
        entry.path = path;
        entry.changes = file_changes;
        entry.status = ToolStatus::Created;
        if let Some(index) = entry.index {
            replace_normalized_entry(msg_store, index, entry.to_normalized_entry());
        } else {
            let index = add_normalized_entry(msg_store, entry_index, entry.to_normalized_entry());
            entry.index = Some(index);
        }
    }
    for (path, file_changes) in iter {
        let mut entry = PatchEntry {
            index: None,
            path,
            changes: file_changes,
            status: ToolStatus::Created,
            item_id: item_id.to_string(),
        };
        let index = add_normalized_entry(msg_store, entry_index, entry.to_normalized_entry());
        entry.index = Some(index);
        patch_state.entries.push(entry);
    }
    for entry in patch_state.entries.drain(normalized_len..) {
        if let Some(index) = entry.index {
            msg_store.push_patch(ConversationPatch::remove(index));
        }
    }
}

fn mcp_tool_result(result: McpToolCallResult) -> ToolResult {
    let texts: Option<Vec<String>> = result
        .content
        .iter()
        .map(|block| {
            if block.get("type").and_then(Value::as_str) == Some("text") {
                block
                    .get("text")
                    .and_then(Value::as_str)
                    .map(ToString::to_string)
            } else {
                None
            }
        })
        .collect();

    if let Some(texts) = texts {
        ToolResult {
            r#type: ToolResultValueType::Markdown,
            value: Value::String(texts.join("\n")),
        }
    } else {
        ToolResult {
            r#type: ToolResultValueType::Json,
            value: result
                .structured_content
                .unwrap_or_else(|| Value::Array(result.content)),
        }
    }
}

fn handle_plan_update(
    payload: TurnPlanUpdatedNotification,
    msg_store: &Arc<MsgStore>,
    entry_index: &EntryIndexProvider,
) {
    let explanation = payload
        .explanation
        .as_deref()
        .map(str::trim)
        .filter(|text| !text.is_empty())
        .map(ToString::to_string);
    let todos: Vec<TodoItem> = payload
        .plan
        .into_iter()
        .map(|item| TodoItem {
            content: item.step,
            status: format_todo_status(item.status),
            priority: None,
        })
        .collect();
    let content = explanation.unwrap_or_else(|| {
        if todos.is_empty() {
            "Plan updated".to_string()
        } else {
            format!("Plan updated ({} steps)", todos.len())
        }
    });

    add_normalized_entry(
        msg_store,
        entry_index,
        NormalizedEntry {
            timestamp: None,
            entry_type: NormalizedEntryType::ToolUse {
                tool_name: "plan".to_string(),
                action_type: ActionType::TodoManagement {
                    todos,
                    operation: "update".to_string(),
                },
                status: ToolStatus::Success,
            },
            content,
            metadata: None,
        },
    );
}

fn handle_jsonrpc_response(
    response: &JSONRPCResponse,
    msg_store: &Arc<MsgStore>,
    entry_index: &EntryIndexProvider,
) {
    // thread/start, thread/resume, and thread/fork responses all share this
    // shape; the returned thread id is the session id used for follow-ups.
    let Ok(response) = serde_json::from_value::<ThreadStartResponse>(response.result.clone())
    else {
        return;
    };

    msg_store.push_session_id(response.thread.id.clone());

    handle_model_params(
        response.model.as_deref(),
        response.reasoning_effort.as_deref(),
        msg_store,
        entry_index,
    );
}

fn handle_model_params(
    model: Option<&str>,
    reasoning_effort: Option<&str>,
    msg_store: &Arc<MsgStore>,
    entry_index: &EntryIndexProvider,
) {
    let mut params = vec![];
    if let Some(model) = model {
        params.push(format!("model: {model}"));
    }
    if let Some(reasoning_effort) = reasoning_effort {
        params.push(format!("reasoning effort: {reasoning_effort}"));
    }
    if params.is_empty() {
        return;
    }

    add_normalized_entry(
        msg_store,
        entry_index,
        NormalizedEntry {
            timestamp: None,
            entry_type: NormalizedEntryType::SystemMessage,
            content: params.join("  "),
            metadata: None,
        },
    );
}

fn build_command_output(output: &str) -> Option<String> {
    let cleaned = output.trim();
    if cleaned.is_empty() {
        None
    } else {
        Some(cleaned.to_string())
    }
}

#[derive(Serialize, Deserialize, Debug)]
pub enum Error {
    LaunchError { error: String },
    AuthRequired { error: String },
}

impl Error {
    pub fn launch_error(error: String) -> Self {
        Self::LaunchError { error }
    }
    pub fn auth_required(error: String) -> Self {
        Self::AuthRequired { error }
    }

    pub fn raw(&self) -> String {
        serde_json::to_string(self).unwrap_or_default()
    }
}

impl ToNormalizedEntry for Error {
    fn to_normalized_entry(&self) -> NormalizedEntry {
        match self {
            Error::LaunchError { error } => NormalizedEntry {
                timestamp: None,
                entry_type: NormalizedEntryType::ErrorMessage {
                    error_type: NormalizedEntryError::Other,
                },
                content: error.clone(),
                metadata: None,
            },
            Error::AuthRequired { error } => NormalizedEntry {
                timestamp: None,
                entry_type: NormalizedEntryType::ErrorMessage {
                    error_type: NormalizedEntryError::SetupRequired,
                },
                content: error.clone(),
                metadata: None,
            },
        }
    }
}

#[derive(Serialize, Deserialize, Debug)]
pub enum Approval {
    ApprovalResponse {
        call_id: String,
        tool_name: String,
        approval_status: ApprovalStatus,
    },
}

impl Approval {
    pub fn approval_response(
        call_id: String,
        tool_name: String,
        approval_status: ApprovalStatus,
    ) -> Self {
        Self::ApprovalResponse {
            call_id,
            tool_name,
            approval_status,
        }
    }

    pub fn raw(&self) -> String {
        serde_json::to_string(self).unwrap_or_default()
    }

    pub fn display_tool_name(&self) -> String {
        let Self::ApprovalResponse { tool_name, .. } = self;
        match tool_name.as_str() {
            "codex.exec_command" => "Exec Command".to_string(),
            "codex.apply_patch" => "Edit".to_string(),
            other => other.to_string(),
        }
    }
}

impl ToNormalizedEntryOpt for Approval {
    fn to_normalized_entry_opt(&self) -> Option<NormalizedEntry> {
        let Self::ApprovalResponse {
            call_id: _,
            tool_name: _,
            approval_status,
        } = self;
        let tool_name = self.display_tool_name();

        match approval_status {
            ApprovalStatus::Pending | ApprovalStatus::Approved => None,
            ApprovalStatus::Denied { reason } => Some(NormalizedEntry {
                timestamp: None,
                entry_type: NormalizedEntryType::UserFeedback {
                    denied_tool: tool_name.clone(),
                },
                content: reason
                    .clone()
                    .unwrap_or_else(|| "User denied this tool use request".to_string())
                    .trim()
                    .to_string(),
                metadata: None,
            }),
            ApprovalStatus::TimedOut => Some(NormalizedEntry {
                timestamp: None,
                entry_type: NormalizedEntryType::ErrorMessage {
                    error_type: NormalizedEntryError::Other,
                },
                content: format!("Approval timed out for tool {tool_name}"),
                metadata: None,
            }),
        }
    }
}
