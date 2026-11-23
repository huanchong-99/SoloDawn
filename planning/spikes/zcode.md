# T3-0b · ZCode as an external AI

## One prompt per run (Q3)

[verified] `zcode --prompt 'PROMPT' --cwd DIR --mode MODE --output-format json` runs one non-interactive prompt and exits. `-p` aliases `--prompt`. `--json` also selects JSON. Use `--output-format stream-json` for NDJSON ending with a `result` line. Without `--mode`, prompt mode defaults to `yolo`. Errors may appear only on stderr. [CLI reference](https://github.com/zai-org/ZCode/blob/fec975905a2abd4c07e35c40d51fa097a2258ec9/apps/zcode-cli/docs/usage/02-cli-reference.md#L15-L59)

[verified] On Linux x64, the CLI bundled with [desktop 3.19.1](https://cdn-zcode.z.ai/zcode/electron/releases/3.19.1/linux-x64/latest.yml) passed `--help`, `--version` (0.18.0), `doctor --json` and `-p /help`. With no model configured, a normal prompt exited 1 with a diagnostic on stderr. [unverified] A completed model prompt.

## Stdio MCP (Q4)

[verified] Use `~/.zcode/cli/config.json`. Enable `features.mcp` and set `mcp.servers.solodawn` to `type: "stdio"`, `command`, `args` and `enabled: true`. The Linux app-server probe launched a fixture from user config. The same fixture in project `.zcode/config.json` showed `untrusted`. [unverified] A headless model calling SoloDawn's tools. [Config paths](https://github.com/zai-org/ZCode/blob/fec975905a2abd4c07e35c40d51fa097a2258ec9/apps/zcode-cli/docs/usage/03-config.md#L3-L16) · [Stdio config](https://github.com/zai-org/ZCode/blob/fec975905a2abd4c07e35c40d51fa097a2258ec9/apps/zcode-cli/docs/usage/07-mcp.md#L3-L34) · [Project trust](https://github.com/zai-org/ZCode/blob/fec975905a2abd4c07e35c40d51fa097a2258ec9/apps/zcode-cli/packages/bootstrap/src/app/runtime-config.ts#L94-L121)

## Usage and quota (Q8)

[verified] JSON and stream results can include per-prompt token `usage` and cumulative session `projection.totalTokenCount`. Plan quota comes from a separate service. ZCode's app reads `percentage` and `nextResetTime` from `/api/monitor/usage/quota/limit`. Z.ai documents a Personal-plan usage plugin for Claude Code. [unverified] A public quota API for external runners and live usage or quota values. [Prompt output](https://github.com/zai-org/ZCode/blob/fec975905a2abd4c07e35c40d51fa097a2258ec9/apps/zcode-cli/packages/cli/src/prompt-command.ts#L384-L446) · [Quota client](https://github.com/zai-org/ZCode/blob/fec975905a2abd4c07e35c40d51fa097a2258ec9/packages/services/src/usage-stats/providers/bigmodelUsageQuotaProvider.ts#L1704-L1708) · [Quota fields](https://github.com/zai-org/ZCode/blob/fec975905a2abd4c07e35c40d51fa097a2258ec9/packages/services/src/usage-stats/providers/bigmodelUsageQuotaMapper.ts#L54-L69) · [Usage plugin](https://docs.z.ai/devpack/extension/usage-query-plugin)

## User-owned scripts (Q1)

[verified] ZCode's CLI docs show an external launcher spawning a prompt. No reviewed clause expressly forbids the owner from launching the official CLI with their own script. Z.ai §III.4(b) prohibits automated access to “unauthorized data, content, or servers/accounts”. ZCode §IV also restricts access by “bots, spiders, or other automated methods” under its network-security rules. Coding Plan §4 limits quota to “officially supported tools” and permits restrictions for “bulk or automated usage on behalf of others”. Z.ai lists ZCode as a supported tool. [unverified] Z.ai's interpretation of those clauses for a user's Coding Plan runner. [Launcher example](https://github.com/zai-org/ZCode/blob/fec975905a2abd4c07e35c40d51fa097a2258ec9/apps/zcode-cli/docs/usage/13-automation.md#L1-L60) · [Z.ai terms](https://docs.z.ai/legal-agreement/terms-of-use) · [ZCode terms](https://zcode.z.ai/en/terms) · [Coding Plan terms](https://docs.z.ai/legal-agreement/subscription-terms#4-usage-rules) · [Supported tools](https://docs.z.ai/devpack/tool/others)
