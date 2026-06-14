# Census: rs-services-orch-llm-prompt

**Unit scope:**
- `crates/services/src/services/orchestrator/llm.rs` (1512 lines)
- `crates/services/src/services/orchestrator/prompt_handler.rs` (1474 lines)

**Branch:** refactor/streamline-and-quality-gate-rules
**Tooling note:** fast-context MCP available and used for cross-file caller tracing (two queries succeeded). Follow-up exact-symbol confirmation via Grep.

These two files are the LLM-client abstraction layer (`llm.rs`) and the interactive-terminal prompt decision engine (`prompt_handler.rs`) of the orchestrator. Both are re-exported through `orchestrator/mod.rs`.

## Module map

| File / Symbol (lines) | Purpose | Public surface | Relations | Notes |
|---|---|---|---|---|
| **llm.rs** module | LLM client abstractions + concrete clients (OpenAI / Anthropic / Claude-Code-native) + factory. | see below | Consumed by `agent.rs`, `concierge/agent.rs`, `resilient_llm.rs`. Re-exported in `mod.rs`. | Multi-provider, multi-format. Provider chosen by `resolve_endpoint(api_type,base_url)`. |
| `trait LLMClient` (24-43) | Core async trait: `chat()` + default `provider_status`/`reset_provider`/`take_provider_events`. | `pub trait` | Implemented by OpenAI/Anthropic/Claude-native/RateLimited/Resilient/Mock clients. `Box<dyn LLMClient>` is the universal handle. | Trait dispatch — alive. |
| `RateLimitedClient<T>` (45-93) | Wraps any `LLMClient` with governor per-second limiter; `until_ready()` waits (not reject). | `pub struct`, `pub fn new` | Wraps every single client built by `build_single_client`. Forwards provider_* calls (G24-001). | Alive. |
| `OpenAICompatibleClient` (95-364) | OpenAI `/chat/completions` client; multi-format response parsing (OpenAI / Responses API / direct content). | `pub struct`, `pub fn new`; re-exported in mod.rs | Built in `build_single_client` for `OpenAIChat`+`Google`. | `pub`+re-exported but **only** internal caller is `build_single_client`; no external importer. |
| `MockLLMClient` (103-158) | Test double (`#[cfg(test)]`). | `#[cfg(test)] pub`; re-exported `#[cfg(test)]` in mod.rs | Used by `rate_limit_tests` here + orchestrator `tests.rs`/`runtime_test.rs`. | Test-only by design. |
| `ChatRequest/ChatMessage/ChatResponse/ChatChoice/UsageInfo` (160-191) | Private serde DTOs for OpenAI format. | private | used by OpenAICompatibleClient. | Alive. |
| `AnthropicCompatibleClient` (366-589) | Anthropic `/v1/messages` client (api_type `anthropic`/`anthropic-compatible`); sends both `x-api-key` + `Bearer`; SSE stream + non-stream JSON fallback. | `pub struct`, `pub fn new` | Built in `build_single_client` for `AnthropicMessages`. | Alive. Streaming hardcoded `stream:true`. |
| `AnthropicRequest/AnthropicMessage` (381-396) | Private serde DTOs (string `system`). | private | — | Alive. |
| `ClaudeCodeNativeClient` (591-911) | OAuth client calling `api.anthropic.com/v1/messages` with local Claude Code CLI credentials (Max/Pro). Computes `cch` integrity hash, billing header, CC identity system block. | `pub struct`, `pub fn try_new`; assoc helpers private | Constructed via `create_claude_code_native_client`. | INVISIBLE FEATURE: subscription-billed LLM via local CLI OAuth. Reads `~/.claude/.credentials.json` per request; token never logged. |
| `ClaudeCredentials/ClaudeOAuth/NativeAnthropicRequest/SystemTextBlock` (595-625) | Private serde DTOs for native client. | private | — | Alive. |
| `detect_cc_version()` (913-938) | Run `claude --version` (strips dev PORT envs) to fill billing header; defaults `2.1.92`. | private fn | called by `try_new`. | Alive. Spawns subprocess. |
| `create_claude_code_native_client(model)` (940-944) | Factory → `Option<Box<dyn LLMClient>>`. | `pub fn`; re-exported in mod.rs | Called by `agent.rs:201` (native fallback) and `server/routes/planning_drafts.rs` (System B). | Alive. |
| `build_terminal_completion_prompt(...)` (946-962) | Format terminal-completion review prompt. | `pub fn`; re-exported in mod.rs | Called by `agent.rs:4994`. | Alive. |
| `build_single_client(config)` (964-991) | Build one rate-limited client by `ApiFormat`. | private fn | Called by `create_llm_client`. Tested. | Alive. |
| `create_llm_client(config)` (993-1036) | Public factory: validates config, single-provider or `ResilientLLMClient` (multi fallback). | `pub fn`; re-exported in mod.rs | Called by `agent.rs:183,210` and `concierge/agent.rs:324` and `server/routes/planning_drafts.rs`. | Primary production entry. Alive. |
| `use normalize_base_url` (15) | Top-level import of deprecated fn. | — | NOT used in production body (only `resolve_endpoint`/`ApiFormat` are). Test mod has its own local import (1155). | CANDIDATE: unused deprecated import suppressed by `#[allow(deprecated, unused_imports)]`. |
| test mods (1038-1512) | rate_limit / anthropic_protocol / url_normalization / full_chain / claude_code_native tests. | `#[cfg(test)]` | — | `url_normalization_tests` exercises deprecated `normalize_base_url`. `test_probe_subscription_model_acceptance` is `#[ignore]` manual network probe. |
| **prompt_handler.rs** module | Decide how to auto-respond to interactive terminal prompts (Enter/YesNo/Choice/ArrowSelect/Input/Password) with safety escalation + optional LLM input generation. | see below | Owned by `OrchestratorAgent` (`agent.rs`). Re-exported `PromptHandler` in mod.rs. | Rule-based engine; LLM only for free-form Input. |
| `LLMPromptCallback` type (26-28) | Boxed async callback `String -> Option<String>` for free-form input. | `pub type` | Built in `agent.rs:213`, passed to `new_with_llm`. | Alive. |
| consts + danger heuristics (34-168) | Confidence threshold, spinner/checklist/destructive marker tables; `should_require_user_confirmation`, advisory-checklist detection. | private fns | Used by `make_decision`. Heavily tested. | Alive. Safety-critical. |
| `LLMPromptDecisionRequest` (174-189) | Serde DTO for structured LLM prompt-decision request. | `pub struct` | Only consumed by `build_llm_decision_prompt` + its test. No production caller. | CANDIDATE: orphaned (production uses inline `format!` instead). |
| `LLMPromptDecisionResponse` (191-204) | Serde DTO for structured LLM prompt-decision response. | `pub struct` | **No reference anywhere** outside its own definition. | CANDIDATE: fully dead. |
| `PromptHandler` (210-682) | Struct + impl: state machines per terminal, decision logic, publish to message bus. | `pub struct`; `new`, `new_with_llm`, `set_task_context`, `clear_task_context`, `handle_prompt_event`, `reset_terminal_state`, `handle_user_prompt_response`, `handle_user_approval` | `handle_prompt_event` ← `agent.rs:1559`; `handle_user_prompt_response` ← `agent.rs:9166`; `new`/`new_with_llm` ← `agent.rs:229,231`. | Core alive. Several pub methods unused (see candidates). |
| `set_task_context` / `clear_task_context` (255-265) | Populate/clear per-terminal task context used in LLM input prompt. | `pub async fn` | **No caller** (production or test). `task_contexts` is read by Input path but never written. | CANDIDATE: dead API; LLM input context always empty. |
| `reset_terminal_state` (579-585) | Reset one terminal's state machine. | `pub async fn` | **No external caller.** Internal resets call `sm.reset()` directly. | CANDIDATE: dead public API. |
| `handle_user_approval` (671-681) | Backward-compat alias → `handle_user_prompt_response`. | `pub async fn` | Only referenced by its own tests; agent.rs calls the real method (doc-comment mention only). | CANDIDATE: dead alias. |
| `build_llm_decision_prompt(req)` (688-740) | Build a structured JSON-instruction LLM prompt for prompt decisions. | `pub fn` | Only caller = `test_build_llm_decision_prompt` (same file). Production Input path uses inline `format!` (507-513), not this. | CANDIDATE: orphaned helper. |
| test mod (746-1474) | 17 async/sync tests for decision rules + bus publishing. | `#[cfg(test)]` | Imports `terminal::prompt_detector`, `message_bus`. | Alive (high coverage of decision rules). |

## Cross-cutting / in-flight relevance
- **(d) planning-draft confirm→materialize + AuditPlan System B:** `create_llm_client` and `create_claude_code_native_client` (both in llm.rs) are consumed by `server/routes/planning_drafts.rs` for audit-plan generation. Removing them would break System B.
- **Claude Code native client (INVISIBLE FEATURE):** subscription-billed LLM path via `~/.claude/.credentials.json`; hardcoded fallback model `claude-sonnet-4-6` lives in `agent.rs:199`, the probe test for swapping it is `llm.rs:1477`.
- No relevance found to (a) open-in-external-IDE, (b) VS Code webview, (c) Quality Gate System A in these two files.
