# T3-0b · ZCode driver spike

[verified] Reviewed the [ZCode v3.18.2 source][source] and ran its CLI (0.17.1) and the CLI bundled with [desktop 3.19.1][release] (0.18.0) on Linux x64. The Apache-2.0 first-party TypeScript tree includes Electron desktop, a local browser workbench and a terminal CLI/TUI. Z.ai documents account sign-in, API key and GLM Coding Plan setup in ZCode. [Source license][license] · [App surfaces][surfaces] · [Z.ai setup][setup]

## 1. Terms

[verified] The Z.ai API terms grant “the right to use Z.ai’s API to integrate the Services into your applications”. Z.ai §III.4(b) restricts automated access “for the purpose of accessing, obtaining, or monitoring any unauthorized data, content, or servers/accounts”. ZCode §III says “Without the consent of ZCode, any direct or indirect authorization of a third party to use your account or access the information under your account shall be invalid.” §IV restricts access using “bots, spiders, or other automated methods” and prohibits “Reverse-engineering any algorithms, source code, or mechanisms of ZCode”. [Z.ai terms][zai-terms] · [ZCode terms][zcode-terms]

[verified] GLM Coding Plan §4 says quota “is only used within officially supported tools” and names “SDK-based access or other third-party integrations” as examples of unauthorized or unsupported use. It bars “general-purpose API access or any scenarios outside such tools” without a separate written agreement and permits restrictions for “account sharing, bulk or automated usage on behalf of others”. ZCode is listed at 1.5× usage. [Subscription terms §4][plan] · [Supported tools][tools]

[unverified] Express permission for SoloDawn to drive `zcode` under the owner's account, ordinary API key or Coding Plan. The API grant covers API integration. The supported-tools listing names ZCode itself. The CLI source documents local prompt automation. [CLI automation][automation]

## 2. Install and platforms

[verified] The source-built `zcode` distribution needs Node.js. Its generated `install.sh` fetches a runtime archive and installs the command in `~/.local/bin` by default. The agent also builds as a Node bundle or a standalone SEA binary with embedded Node. SEA targets macOS, Linux and Windows (x64 and ARM64). Z.ai publishes separate desktop installers for all three. [Packaging][packaging] · [SEA targets][sea] · [Downloads][downloads]

[verified] Linux x64 with the bundled `zcode.cjs` and its provider defaults: `--help`, `--version` (0.18.0), `doctor --json` and `-p /help --mode plan` exited 0. [unverified] An official npm install, a hosted standalone CLI installer and runtime on macOS or Windows.

## 3. Structured turns

[verified] `zcode -p <text> --cwd <dir> --mode <mode> [--resume <id>] --json` returns a summary. `--output-format stream-json` streams NDJSON and ends with `type: "result"` on success. Each caller-supplied prompt needs its own process. Normal turns map to `turn.started`, `turn.completed` (tokens and optional `usage`), `turn.failed`, `tool.updated` (including tool errors), `permission.requested` and `permission.resolved`. A credential-free prompt exited 1 with “Select a model before sending a message” on stderr and empty stdout. `/help` emitted plain text even with `stream-json`. [CLI reference][cli] · [Stream writer][stream] · [Result fields][result] · [Event mapping][events] · [Turn payloads][payload]

[verified] `zcode app-server --stdio` keeps one process open for `session/create`, `session/send`, `session/subscribe` / `session/events` and `session/resume`. RPC failures use an `error` reply. The bundled server answered `runtime/capabilities`, accepted a send and answered `session/read` after the client replied to `session/requestRuntimePreferences`. ZCode's design calls this a private agent server. [unverified] A supported third-party protocol and live model turn completion. [Protocol design][design] · [Error envelope][rpc-error] · [Host callback][host] · [Feedback #286][feedback]

## 4. MCP

[verified] User or project config can enable `features.mcp` and set `mcp.servers.solodawn` to `type: "stdio"` with `command` and `args`. User config wins for the same name. The CLI flag reference lists no MCP injection flag. The private server accepts `mcpServers` on create/resume. Linux 0.18.0 launched an injected stdio fixture with its argument, and `mcp/list` reported one tool. [unverified] Model invocation of SoloDawn's server without per-call approval. [MCP config][mcp] · [CLI reference][cli] · [Server schema][mcp-schema] · [Create/resume schema][create]

## 5. Approvals and levels

[verified] The prompt command has no external reply channel. Its broker denies requests except two workflow tools it auto-approves. The private server sends `interaction/requestPermission` with session, turn and tool-call IDs and awaits the client's decision (SoloDawn `conn` candidate). [unverified] Live approval round trip. [Prompt broker][headless] · [Server broker][broker] · [Step-3 §4](../contracts/step-3/drivers.md#4-the-capability-table)

| SoloDawn level | ZCode evidence |
| --- | --- |
| 1 · No file changes | [verified] `plan` restricts ordinary writes and commands but allows some MCP and session-control operations. [unverified] A configuration that guarantees no file changes. |
| 2 · Folder-limited writes | [verified] `build` asks for ordinary writes, `edit` generally allows workspace edits and `--cwd` selects the workspace. The modes provide tool policy only. [unverified] A filesystem sandbox that blocks writes outside that folder. |
| 3 · No prompts | [verified] `yolo` bypasses ordinary approvals. Explicit always-ask tools can still prompt. `guarded` asks on dangerous commands. [unverified] Full bypass. |

[Permission modes][permissions] · [Always-ask policy][always] · [SoloDawn levels](../contracts/drivers.md#2-what-the-node-does-itself-by-level)

[unverified] The required task/review versus group configuration split. ZCode loads project config and gates project hooks on workspace trust. [ZCode config][config] · [Step-3 §3](../contracts/step-3/drivers.md#3-what-every-driver-has)

## 6. Resume

[verified] `--resume <id>` uses the same data and working directories. `--continue` picks the latest session for the directory. In 0.18.0, `session/resume` on a fresh stdio process recovered the same session and persisted user message ID after the first process exited. [Session docs][resume] · [Resume schema][create]

## 7. Models and thinking

[verified] The prompt CLI has no `--model` flag. Startup config accepts `defaultModelSelection` with provider/model IDs and supported options. The private server's `workspace-config` subscription carries model choices, per-model effort values and defaults as JSON. `session/create` accepts `model` and `thoughtLevel`, with `session/setModel` and `session/setThoughtLevel` for updates. [unverified] A populated, account-specific catalog and exact effort selection accepted before the first turn as required by step-3 §10. [CLI flags][cli] · [Provider config][model-config] · [Catalog schema][catalog] · [Subscription test][catalog-test] · [Start schema][create] · [Setters][setters] · [Step-3 §10](../contracts/step-3/drivers.md#10-models-discovery-suggestion-and-start)

## 8. Usage and quota

[verified] Prompt summaries can include per-turn token `usage` and cumulative session `totalTokenCount`. The protocol's `turn.completed` has `tokenCount` and optional `usage`. Plan quota comes from a separate service. ZCode app source maps `percentage` and `nextResetTime` from `/api/monitor/usage/quota/limit`. Z.ai documents a Personal-plan usage plugin for Claude Code. [unverified] A documented quota API for SoloDawn, live token counts and stable percent/reset data for scheduling. [Prompt results][result] · [Turn payload][payload] · [Quota client][quota] · [Quota fields][quota-fields] · [Usage plugin][plugin]

## 9. Terminal takeover

[verified] The CLI routes `zcode tui --resume <id>` to a new TUI process with a resume request. [unverified] Attaching a TUI to a live stdio process, or handing off the same session to a fresh PTY with equivalent level, MCP and repository settings and returning it to structured mode. SoloDawn should report `takeover: none` pending that proof. [TUI entry][tui] · [Resume flag][cli] · [Step-3 §4](../contracts/step-3/drivers.md#4-the-capability-table) · [Takeover data channel](../contracts/datachannel.md#terminal)

## Verdict

**No-go.** The documented prompt interface lacks persistent turns and an approval reply channel. The persistent interface is private. Required level-1 enforcement, repository configuration isolation and permission to drive the CLI with the owner's credentials remain unverified. [Step-3 §3](../contracts/step-3/drivers.md#3-what-every-driver-has)

[source]: https://github.com/zai-org/ZCode/tree/fec975905a2abd4c07e35c40d51fa097a2258ec9
[release]: https://cdn-zcode.z.ai/zcode/electron/releases/3.19.1/linux-x64/latest.yml
[license]: https://github.com/zai-org/ZCode/blob/fec975905a2abd4c07e35c40d51fa097a2258ec9/NOTICE.md#L71-L73
[surfaces]: https://github.com/zai-org/ZCode/blob/fec975905a2abd4c07e35c40d51fa097a2258ec9/README.en.md#L54-L111
[setup]: https://docs.z.ai/devpack/tool/zcode
[zai-terms]: https://docs.z.ai/legal-agreement/terms-of-use
[zcode-terms]: https://zcode.z.ai/en/terms
[plan]: https://docs.z.ai/legal-agreement/subscription-terms#4-usage-rules
[tools]: https://docs.z.ai/devpack/tool/others
[automation]: https://github.com/zai-org/ZCode/blob/fec975905a2abd4c07e35c40d51fa097a2258ec9/apps/zcode-cli/docs/usage/13-automation.md
[packaging]: https://github.com/zai-org/ZCode/blob/fec975905a2abd4c07e35c40d51fa097a2258ec9/README.en.md#L166-L191
[sea]: https://github.com/zai-org/ZCode/blob/fec975905a2abd4c07e35c40d51fa097a2258ec9/apps/zcode-cli/docs/usage/01-quickstart.md#L16-L24
[downloads]: https://zcode.z.ai/en
[cli]: https://github.com/zai-org/ZCode/blob/fec975905a2abd4c07e35c40d51fa097a2258ec9/apps/zcode-cli/docs/usage/02-cli-reference.md#L15-L54
[stream]: https://github.com/zai-org/ZCode/blob/fec975905a2abd4c07e35c40d51fa097a2258ec9/apps/zcode-cli/packages/cli/src/headless-workflow.ts#L246-L264
[result]: https://github.com/zai-org/ZCode/blob/fec975905a2abd4c07e35c40d51fa097a2258ec9/apps/zcode-cli/packages/cli/src/prompt-command.ts#L384-L435
[events]: https://github.com/zai-org/ZCode/blob/fec975905a2abd4c07e35c40d51fa097a2258ec9/apps/zcode-cli/packages/bootstrap/src/zcode-protocol/session-mapper.ts#L1500-L1537
[payload]: https://github.com/zai-org/ZCode/blob/fec975905a2abd4c07e35c40d51fa097a2258ec9/packages/shared/src/zcode-protocol/index.ts#L1692-L1728
[design]: https://github.com/zai-org/ZCode/blob/fec975905a2abd4c07e35c40d51fa097a2258ec9/apps/zcode-cli/docs/design/v2/zcode-protocol-agent-server.md
[rpc-error]: https://github.com/zai-org/ZCode/blob/fec975905a2abd4c07e35c40d51fa097a2258ec9/packages/shared/src/zcode-protocol/index.ts#L484-L502
[host]: https://github.com/zai-org/ZCode/blob/fec975905a2abd4c07e35c40d51fa097a2258ec9/apps/zcode-cli/packages/bootstrap/src/zcode-protocol/server-operations.ts#L3616-L3649
[feedback]: https://github.com/zai-org/feedback/issues/286
[mcp]: https://github.com/zai-org/ZCode/blob/fec975905a2abd4c07e35c40d51fa097a2258ec9/apps/zcode-cli/docs/usage/07-mcp.md#L3-L28
[mcp-schema]: https://github.com/zai-org/ZCode/blob/fec975905a2abd4c07e35c40d51fa097a2258ec9/packages/shared/src/zcode-protocol/index.ts#L805-L818
[create]: https://github.com/zai-org/ZCode/blob/fec975905a2abd4c07e35c40d51fa097a2258ec9/packages/shared/src/zcode-protocol/index.ts#L2023-L2065
[headless]: https://github.com/zai-org/ZCode/blob/fec975905a2abd4c07e35c40d51fa097a2258ec9/apps/zcode-cli/packages/cli/src/headless-workflow.ts#L31-L50
[broker]: https://github.com/zai-org/ZCode/blob/fec975905a2abd4c07e35c40d51fa097a2258ec9/apps/zcode-cli/packages/bootstrap/src/zcode-protocol/interaction-broker.ts#L106-L143
[permissions]: https://github.com/zai-org/ZCode/blob/fec975905a2abd4c07e35c40d51fa097a2258ec9/apps/zcode-cli/docs/usage/05-permissions.md
[always]: https://github.com/zai-org/ZCode/blob/fec975905a2abd4c07e35c40d51fa097a2258ec9/apps/zcode-cli/packages/core/src/permission/service.ts#L396-L405
[config]: https://github.com/zai-org/ZCode/blob/fec975905a2abd4c07e35c40d51fa097a2258ec9/apps/zcode-cli/docs/usage/03-config.md#L5-L16
[resume]: https://github.com/zai-org/ZCode/blob/fec975905a2abd4c07e35c40d51fa097a2258ec9/apps/zcode-cli/docs/usage/06-sessions.md#L3-L20
[model-config]: https://github.com/zai-org/ZCode/blob/fec975905a2abd4c07e35c40d51fa097a2258ec9/apps/zcode-cli/docs/usage/03-config.md#L18-L59
[catalog]: https://github.com/zai-org/ZCode/blob/fec975905a2abd4c07e35c40d51fa097a2258ec9/packages/shared/src/zcode-protocol-v4/workspace-config.ts#L13-L61
[catalog-test]: https://github.com/zai-org/ZCode/blob/fec975905a2abd4c07e35c40d51fa097a2258ec9/apps/zcode-cli/packages/bootstrap/tests/zcode-protocol.test.ts#L1335-L1351
[setters]: https://github.com/zai-org/ZCode/blob/fec975905a2abd4c07e35c40d51fa097a2258ec9/packages/shared/src/zcode-protocol/index.ts#L2446-L2466
[quota]: https://github.com/zai-org/ZCode/blob/fec975905a2abd4c07e35c40d51fa097a2258ec9/packages/services/src/usage-stats/providers/bigmodelUsageQuotaProvider.ts#L1704-L1708
[quota-fields]: https://github.com/zai-org/ZCode/blob/fec975905a2abd4c07e35c40d51fa097a2258ec9/packages/services/src/usage-stats/providers/bigmodelUsageQuotaMapper.ts#L37-L69
[plugin]: https://docs.z.ai/devpack/extension/usage-query-plugin
[tui]: https://github.com/zai-org/ZCode/blob/fec975905a2abd4c07e35c40d51fa097a2258ec9/apps/zcode-cli/packages/cli/src/run.ts#L642-L654
