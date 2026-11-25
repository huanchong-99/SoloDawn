---
description: "Real-machine acceptance checklist (I-11) covering browser pairs, live CLI model lists, terminal takeover and watching, firewall and relay behavior, log bundles, and external-AI scheduled jobs."
kind: tutorial
---
# Real-Machine Acceptance Checklist (I-11)

## Summary

Use this checklist to execute **I-11** (real-machine acceptance across macOS, Linux, Windows, iOS, and Android) before signing off a release on real hardware, real CLI accounts, real networks, and OS job schedulers. Every check specifies concrete steps, the expected observable result, and the exact logs, dumps, or captures to attach if the check fails.

Do not record personal usernames, hostnames, internal company network names, or private machine paths when completing this checklist in public issue trackers or pull requests. Replace `<label>`, `<secret>`, `<member-id>`, `<exe>`, and `<rv-url>` with generic labels.

## Prerequisites

- Built release archives and `SHA256SUMS` (following [install.md](../../install.md)) or freshly built `solodawn` / `solodawn.exe` binaries for macOS (`darwin/amd64` or `arm64`), Linux (`linux/amd64` or `arm64`), and Windows (`windows/amd64` or `arm64`).
- Test devices meeting the supported browser baseline ([2026-11-20-browser-baseline.md](../decisions/implemented/architecture/2026-11-20-browser-baseline.md)):
  - **macOS:** Safari 17.5+, Chrome 137+, Firefox 136+
  - **Windows:** Chrome 137+, Edge 137+, Firefox 136+
  - **Linux:** Chrome 137+, Firefox 136+
  - **iOS:** Safari 17.5+ (browser tab and Add to Home Screen app)
  - **Android:** Chrome 137+
- Signed-in accounts for the real CLIs under test (`claude`, `codex`, plus `opencode` / `qwen` via API key for Step 3 tests, and at least one real external MCP agent for `A10.28`).
- Two network environments:
  1. A **home or open LAN** where direct UDP / ICE-TCP (`47801`) paths succeed.
  2. A **restricted or company network** (or host firewall blocking inbound UDP/TCP peer ports) that forces peer traffic through the rendezvous TURN relay on port `443`.
- A public HTTPS + TURN rendezvous endpoint (`<rv-url>`) for multi-network and `A8.4` checks, plus a scratch GitHub repository for `A5.13`.

## Steps

### 1. Installation, Pairing, Service Start, and Reboot (`A8.1`)

Run on a clean user account on **macOS**, **Linux**, and **Windows**.

1. **Steps:**
   1. Verify the downloaded binary checksum against `SHA256SUMS` and install it as described in [install.md](../../install.md). On macOS, verify quarantine removal runs only after the checksum matches.
   2. Ensure `git config --global user.name` and `user.email` are set.
   3. In the browser UI (**Add a computer**), copy the onboarding prompt (`16`-character `<secret>`) and run `solodawn pair <secret> --name "<label>" [--rendezvous <rv-url>]` (or paste the onboarding prompt into a CLI). Confirm `solodawn version` matches the release tag and compare the browser's **Fingerprint** against `solodawn status`.
   4. Check the registered start-at-login entry created on adoption (or run `solodawn service install` if testing registration directly):
      - macOS: `~/Library/LaunchAgents/org.solodawn.node.plist`
      - Linux: `~/.config/systemd/user/solodawn.service`
      - Windows: Scheduled Task `\SoloDawn-<SID>` running `conhost.exe --headless "<exe>" node`
   5. Log out of the desktop session and log back in, then perform a **full OS reboot** (`A8.1`, [issues.md](../../issues.md)). Wait 30 seconds after login and run `solodawn status` and inspect the **Computers** page.
2. **Expected result:**
   - Binary hash matches `SHA256SUMS`, `solodawn pair` prints `<label> (<fingerprint>) direct|relay`, and the fingerprint matches the browser.
   - Both after re-login and after full OS reboot, the node starts automatically without an interactive terminal window, `solodawn status` reports `version <tag>`, `fingerprint <fp>`, and active connections, and the **Computers** page shows the node **Online** with its **Account type** prompt or configured AIs.
3. **Failure reporting:**
   - Attach `solodawn version`, full `solodawn status` output, UTC timestamp of reboot/login, and the node log bundle (`logs/node.log` and `logs/node.log.1` — see Step 3).
   - Attach OS service inspection output with personal paths sanitized:
     - macOS: `launchctl print gui/$(id -u)/org.solodawn.node` and sanitized contents of `~/Library/LaunchAgents/org.solodawn.node.plist`.
     - Linux: `systemctl --user status solodawn.service` and `journalctl --user -u solodawn.service -n 100`.
     - Windows: `schtasks /Query /TN "\SoloDawn-<SID>" /V /FO LIST` and PowerShell output from `solodawn service install`.

### 2. macOS Application Firewall Inbound Prompt (`install.md`, `issues.md`)

Run on **macOS** with the unsigned `solodawn` binary and System Settings -> Network -> **Firewall** enabled.

1. **Steps:**
   1. Confirm `solodawn` is not yet pre-allowed in Firewall Options (or remove any prior entry under System Settings -> Network -> Firewall -> Options).
   2. Start `solodawn node` on a LAN where a remote browser (or second LAN node) connects from another machine on the same network, using `<rv-url>` for signalling.
   3. Observe the macOS incoming-network prompt ("Do you want the application `solodawn` to accept incoming network connections?"):
      - **Pass 1 (Block / Deny or ignore):** click **Deny** (or wait with it unapproved). Check connection state in **Computers** and `solodawn status`.
      - **Pass 2 (Allow):** switch `solodawn` to **Allow incoming connections** in Firewall Options (or click **Allow** on a fresh binary / re-added entry) and reconnect or wait one probe interval plus `5 s`.
2. **Expected result:**
   - During Pass 1, incoming UDP/ICE-TCP is blocked by the macOS firewall (`verdict: 2 (Block)`), signaling succeeds, and the node still connects over TURN showing **Relayed · limited** (`relay`) in **Computers** (`solodawn status` lists `relay`).
   - During Pass 2 after allowing `solodawn`, the direct connection succeeds (via `A1.3` direct path / probe promotion) and **Computers** transitions to **Direct** (`direct`).
3. **Failure reporting:**
   - Attach the UTC time of both passes, `solodawn status`, `logs/node.log`, Firewall Options state (`/usr/libexec/ApplicationFirewall/socketfilterfw --getappblocked <exe>`), and the **Computers** connection status label.

### 3. Log Folder Bundling and Windows `tar` Lock Test ([diagnostics.md](../../planning/contracts/diagnostics.md))

Run on **Windows** while `solodawn node` is actively writing logs (during an active turn or sync), and spot-check on **macOS** and **Linux**.

1. **Steps:**
   1. Locate the data directory log folder:
      - macOS: `~/Library/Application Support/solodawn/logs`
      - Linux: `${XDG_DATA_HOME:-$HOME/.local/share}/solodawn/logs`
      - Windows: `%LocalAppData%\solodawn\logs`
   2. While `solodawn node` is running and actively appending to `logs\node.log` (do not stop the node), execute the platform command:
      - Windows (PowerShell):
        ```powershell
        tar -a -cf "$([Environment]::GetFolderPath('Desktop'))\solodawn-logs.zip" -C "$env:LocalAppData\solodawn" logs
        ```
      - macOS (Bash/Zsh):
        ```sh
        tar czf ~/Desktop/solodawn-logs.tgz -C "$HOME/Library/Application Support/solodawn" logs
        ```
      - Linux (Bash/Zsh):
        ```sh
        tar czf ~/solodawn-logs.tgz -C "${XDG_DATA_HOME:-$HOME/.local/share}/solodawn" logs
        ```
   3. Inspect the archive contents (`tar -tf` / `Expand-Archive`) and open the extracted `node.log` (and `node.log.1` if rotated). Check whether `logs/level` (`debug` vs default `info`, if supported in the tested build) behaves as documented in [diagnostics.md](../../planning/contracts/diagnostics.md) section 3.
2. **Expected result:**
   - Every archive command exits `0` while the node stays running, including on Windows (verifying Windows `bsdtar` reads the open `node.log` handle without a sharing-violation error).
   - Extracted `logs/node.log` (and `.1` when rotated past `10 MiB`) is non-empty, valid UTF-8 text with UTC microsecond timestamps and `node=<8-char-id>` fields, and contains zero message text, turn bodies, credentials, or repo file contents.
3. **Failure reporting:**
   - Attach the exact shell command, exit code, stdout/stderr (especially any Windows file-lock or `tar` permission error), `Get-ChildItem "$env:LocalAppData\solodawn\logs"` (or `ls -la` on Unix), and a redacted copy of `node.log`.

### 4. Multi-OS Browser Pair Matrix, Ed25519 Keys, and Web Push (`A8.3`, `A8.5`, `A9.40`, `A9.41`)

Exercise each supported node OS × browser combination named in [step-1.md](../../planning/steps/step-1.md) ("Scope") and [2026-11-20-browser-baseline.md](../decisions/implemented/architecture/2026-11-20-browser-baseline.md):

| Node OS | Browsers to test against that node |
| --- | --- |
| macOS (`darwin`) | macOS Safari 17.5+, Chrome 137+, Firefox 136+; iOS Safari 17.5+ (tab + Add to Home Screen); Android Chrome 137+ |
| Windows (`windows`) | Windows Chrome 137+, Edge 137+, Firefox 136+; macOS Safari 17.5+ |
| Linux (`linux`) | Linux Chrome 137+, Firefox 136+; iOS Safari 17.5+ |

1. **Steps:**
   1. On each pair in the matrix:
      - Open the web UI, confirm the upgrade notice does **not** appear, pair via `#pair=<secret>` (`solodawn pair-browser`), verify `solodawn status` reflects the browser connection, reload the page, and confirm the IndexedDB non-extractable Ed25519 key still authenticates without re-pairing (specifically covering Firefox 136+ structured clone).
      - Verify **two tabs** of the same browser profile trigger the exclusive Web Lock notice `"SoloDawn is open in another tab."` and switching via **Use here** cleanly hands over connections (`A1.12`).
      - Verify **one-person mode (`A9.40`)**: create a group, add AIs, run a brief -> task -> review -> merge flow without a `confirm` card appearing.
      - Verify **two-person mode (`A9.41`)**: invite a second browser/person, send a group `text` message (`A9.1`–`A9.4`), exercise a `confirm` card (`A9.5`–`A9.19`), pause/resume, group export/import (`A9.27`–`A9.29`), and a team group bulletin (`A9.30`–`A9.32`).
   2. On **Desktop Chrome**, **Android Chrome**, and **iOS Safari (added to the home screen)**: grant notification permission (`A8.3`), close/background the tab/app while the node remains running, trigger an approval request or mention from another device/task, inspect the push notification, and click **Open** (or tap the notification).
2. **Expected result:**
   - Every matrix pair completes pairing, survives page reload without losing its device key, completes sync and group commands cleanly, and renders no layout/theme breakage (`light`/`dark`, `en`/`zh-CN`).
   - Closed-page push notifications appear on Desktop Chrome, Android Chrome, and iOS Home Screen web app (`A8.3`), display only the group name, card type (`Approval`, `Confirm start`, `Your call`, `Task done`, `Mentioned you`), and one-line title (no message text or command body), and open the target card when tapped.
   - Note on iOS: verify Apple Push Service accepts the node's VAPID `sub` claim (`issues.md` Web Push limitation).
3. **Failure reporting:**
   - Name the exact `(node OS, browser name + version, OS version)`, UTC time, reproduction step, browser DevTools Console errors, Application -> IndexedDB / Service Workers state, push subscription error (if any on iOS/Android), plus `solodawn status` and `logs/node.log`.

### 5. Real CLI Model Lists, Refresh, and Selected-Model Execution (`A10.13`–`A10.16`)

Run against real signed-in CLIs (`claude`, `codex`, `opencode`, `qwen`) on a live node.

1. **Steps:**
   1. Run each CLI's native interactive/list picker (`claude` model selector, `codex` model list, `opencode models --verbose`, `qwen` has no headless list) to record the live models and effort/variant levels available under the active subscription/key.
   2. In SoloDawn's **Add an AI** dialog for each computer:
      - Inspect the **Model**, **Level**, **Tier** suggestion (`light`/`standard`/`strong`), and **Fast mode** indicator for `claude`, `codex`, and `opencode`.
      - Inspect `qwen` (expect empty `models` list showing free-text **Model** and **Level** inputs).
   3. Join two members using the **same** CLI agent on one node with different `model` and `effort` picks (`A10.15`), mention each, and verify in the CLI's own session metadata/events that each process runs its chosen model and effort.
   4. Edit an AI's `Tier` (for example, override suggested `strong` to `light`) and `Skills`, then click **Refresh the models** on the AI panel (`host.info` with `refresh: true`) (`A10.14`, `A10.16`). Confirm newly enabled/changed upstream models appear without restarting `solodawn node` while saved `Tier` and `Skills` remain untouched.
   5. Change a member's **Model** to a different valid model (verify next turn/resume uses the new model, keeping session history on Claude/Codex/OpenCode, or next new session on Qwen per [profiles.md](../../planning/contracts/step-3/profiles.md) / [step-3/drivers.md](../../planning/contracts/step-3/drivers.md)).
   6. Temporarily set a member's **Model** to a nonexistent/invalid id (e.g., `invalid-model-check`) and mention the member (`A10.15`). Afterward restore a valid model.
2. **Expected result:**
   - Browser pickers match each CLI's real model & effort catalog (`models: []` yields text fields on Qwen Code).
   - Refresh updates `models` without a node restart and never overwrites owner-chosen `Tier`, `Skills`, `Model`, or `Level` (a removed stored model displays **Not listed**).
   - Each member runs with its chosen model/effort; the invalid-model test fails cleanly with a `turn_interrupted` (`crash`) system message and preserves the session id without emitting `session_replaced`.
3. **Failure reporting:**
   - Attach the CLI version (`claude --version`, `codex --version`, etc.), sanitized output of the CLI's native model listing, the `host.info` `agents[].models` payload (captured from browser DevTools Network/WebRTC message inspection or page state), UI screenshots if misaligned, and `logs/node.log`.

### 6. Terminal Takeover and Watching on Home and Company Networks (`A7.1`, `A7.2`, `A7.5`, `A7.8`, `A7.9`, `A9.20`, `A9.21`)

Run using real `claude` and `codex` members on a workstation node and at least two browsers (`O` owner browser, `W` watcher browser in a shared group).

1. **Steps — Home network (`Direct` path):**
   1. Confirm **Computers** and the AI member card show `direct` / **Direct**.
   2. On a working **Claude Code** member with an active task (`A7.1`), click **Open terminal**. Run a prompt inside the interactive terminal cleanly editing a file in the task worktree (do not modify outside paths yet), observe real-time PTY rendering/resizing, then click **Hand back**. Verify a summary card appears listing changed files and commits, and send a follow-up thread mention verifying the AI remembers the terminal conversation.
   3. Repeat **Open terminal** -> interactive prompt -> **Hand back** on **Codex** (`A7.2`) and on **Windows ConPTY** (exit the CLI process inside the terminal and confirm the final lines of output render before hand-back; see [issues.md](../../issues.md) ConPTY history).
   4. Click **Open terminal** on the **Secretary** (`A7.8`) and ask it in the terminal to edit a repository file (`Read-only group session`). Verify no file on disk changes.
   5. **Live watch (`A9.20`, [watch-team.md](../../planning/contracts/step-2/watch-team.md)):** While `O` holds a direct terminal takeover open, have watcher `W` click **Ask to watch**. Have `O` click **Confirm** on the confirm card (`Let <who> watch <ai>'s terminal.`), then have `W` open **Watch**. Verify `W` sees `"Watching · receive only"` (`watch.readonly`), cannot send keystrokes or resize `O`'s terminal, and receives live PTY bytes + resize updates cleanly. Test ending via `O`'s **Stop watching** (`watch.end`) and via `O`'s **Hand back**.
2. **Steps — Company network (`Relayed · limited` path) and mid-session path switch (`A7.5`, `A7.9`):**
   1. Move the browser (or connect the node behind a restrictive company firewall blocking direct UDP/inbound peer TCP so ICE falls back to the rendezvous `443` TURN relay).
   2. Wait for the **Computers** indicator to show **Relayed · limited**. Verify normal chat, task status, and approval cards still arrive and answer over the relay.
   3. Inspect **Open terminal** and **Watch** controls (`A7.5`): attempt takeover / watch over the relay.
   4. Switch back to a **Direct** home connection, open a terminal takeover (and optionally a watch stream), then simulate dropping direct paths while keeping TURN reachable (e.g., disconnect/switch browser to a hotspot/company VPN forcing a relay transition mid-session, `A7.9`, `A9.21`).
3. **Expected result:**
   - **Home network (`direct`):** Claude Code (`A7.1`), Codex (`A7.2`), Windows ConPTY tail output, and Secretary read-only enforcement (`A7.8`) all succeed; queued mentions sent during takeover arrive in order after **Hand back**; `W` receives live terminal output only after `O` confirms and loses access immediately on `watch.end` or hand-back.
   - **Company network (`relay`):** Chat and approvals remain functional; **Open terminal** is disabled (`"Direct link only"` / `"The terminal works over a direct connection only"`) and `takeover.start` / `watch.start` refuse with `relay`.
   - **Mid-takeover switch to relay (`A7.9`, `A9.21`):** Browser displays `"Handed back · switched to relay"` (and `"Watching ended"` for watchers), the PTY/watch channel closes cleanly, control returns to the AI, and `host.info` / chat continues working on the existing `ctl` connection without re-pairing or refreshing.
4. **Failure reporting:**
   - Attach network label (`home-direct` vs `company-relay` — never private hostnames/SSIDs/IPs), browser path indicator, `solodawn status`, `logs/node.log` + `logs/node.log.1` covering the failure window (check `ice.pair`, `ice.state`, `ctl.open`, `conn.close` lines), browser Console errors, and `chrome://webrtc-internals` / WebRTC candidate-pair dump (`selectedCandidatePairId` / local & remote candidate types).

### 7. Live CLI Approvals, Hooks, Questions, GitHub Remote, and Rendezvous Footprint (`A3.8`, `A4.1`, `A4.10`, `A4.11`, `A5.13`, `A8.4`, `A8.6`, `A9.26`, `A10.4`)

Run the remaining `live` checks referenced by **I-11** in [step-1-acceptance.md](../../planning/steps/step-1-acceptance.md), [step-3-acceptance.md](../../planning/steps/step-3-acceptance.md), and [parallel-plan-units.md](../../planning/steps/parallel-plan-units.md):

1. **Steps:**
   1. **Repo hooks (`A3.8`, `A10.4`):** In a task worktree with a test repo hook writing a marker file, run one group-session mention and one task-session turn on each real CLI. Confirm the hook fires in `task`/`review` sessions and **not** in group sessions (`--setting-sources user` / `project_doc_max_bytes = 0`; note documented Qwen limitation in [install.md](../../install.md)). For **Qwen Code** record four results: group and task session, each with the hook unapproved and approved (approve by starting `qwen` once in the folder; its documentation says unapproved project hooks run in neither), and whether `QWEN.md` was read; then decide the `@pending` scenario of `features/s3_drivers.feature` ([issues.md](../../issues.md), "A Qwen Code group session may load the repository's configuration").
   2. **Sandbox containment & approvals (`A4.1`, `A4.10`):** At level 2 on each CLI, ask the AI to install/fetch from network or write a marker file outside its worktree (`A4.10`). Verify no outside file exists before approval, a single approval card appears, and on **native Windows Claude Code** setting level 2 returns a clean refusal explaining level 2 lacks a Windows sandbox (`risks.md` `Q-4`).
   3. **Interactive question cards (`A4.1` second half, `A4.11`):**
      - On **Claude Code** at permission modes `1`, `2`, and `3`, trigger `AskUserQuestion` and confirm a `decision` card reaches the browser and the selected `updatedInput.answers` reply reaches Claude Code cleanly.
      - On **Codex** with `-c features.default_mode_request_user_input=true`, test whether Codex emits `item/tool/requestUserInput` in Default mode (`A4.11`); if unsupported on the installed CLI release, verify Codex asks through `request_decision` per [drivers.md](../../planning/contracts/drivers.md) section 1.2 and record the outcome.
   4. **GitHub remote M5 flow (`A5.13`):** Run a full brief -> task -> `run_checks` -> `push_branch` -> `submit_task` -> review -> merge pass against a **scratch GitHub repository**, verifying `push_branch` requires no approval card while a direct `git push` to another branch produces one.
   5. **Another forge (`A9.26`):** With a signed-in forge program other than `gh` (for example `glab`, or `tea` for a Gitea or Forgejo server) and a scratch repository there, run `TestS2LiveForgeFeature` as [s2_watch.md](../../features/s2_watch.md) ("A9.26 live run") says.
   6. **Public standalone rendezvous (`A8.4`) and resource footprint (`A8.6`):** Start `solodawn rendezvous --listen :443 --domain <rv-host> --turn-domain <turn-host> ...` on a clean server (`A8.4`), pass `A1.1`–`A1.3` across two real networks, and record for `A8.6`: CLI versions, node binary size (`<= 30 MB` target), node idle RSS (`<= 40 MB` target), peak RSS per open CLI session, and rendezvous RSS with 100 idle connections.
2. **Expected result:**
   - All `live` hooks, level-2 approvals, question replies (`updatedInput.answers` / `request_decision`), GitHub PR checks/merge, both merged requests on the other forge (`A9.26`), `A8.4` TLS + TURN `:443` pass, and `A8.6` footprint figures meet contract bounds.
3. **Failure reporting:**
   - Attach CLI version (`claude --version`, `codex --version`, etc.), permission level, group/task state at failure, `logs/node.log`, `solodawn status`, and for `AskUserQuestion` / `requestUserInput` failures, sanitized CLI stderr/stream events if captured.

### 8. External-AI Setup Prompt and Scheduled Jobs (`A10.28`, [step-3/external.md](../../planning/contracts/step-3/external.md))

Run with a real external agent (for example Cursor, ZCode, or another MCP-capable CLI agent) on live **Linux**, **macOS**, and **Windows** nodes.

#### 8a. Interactive Self-Setup Prompt (`A10.28`)

1. **Steps:**
   1. On **Add an AI**, choose **External AI**, pick an account type, add an optional tool label, and submit.
   2. Open the member's **Setup** panel, copy the **Self-setup prompt**, and paste it verbatim into the real external agent on that node's computer.
   3. Watch the four marks under **Setup check** (`Runner seen`, `Item taken`, `Member answered`, `Run ended`) and click **Send setup check again** once after setup settles to re-verify.
2. **Expected result:**
   - Without manual edits, the agent registers the `solodawn` stdio MCP server (`command: <exe>`, `args: ["mcp", "--member", "<member-id>"]`), installs a runner script + background/scheduled loop, answers `Setup check: answer here in one line, then finish.` **once** (not multiple times), flips all four Setup check marks to **Complete**, returns member state to `Waiting`, and leaves **no long-lived AI CLI process running** when idle.

#### 8b. Scheduled Job Execution Under `crontab`, `launchd`, and PowerShell / Task Scheduler

Verify all three scheduled-task runners described in `web/src/components/external/prompt.txt` (`solodawn wait --member <id> --timeout 0` invoked once per minute):

1. **Linux (`crontab` with `flock`):**
   - **Steps:** Install the cron entry with explicit absolute paths or `PATH` (cron provides a minimal `PATH`):
     ```cron
     * * * * * flock -n /tmp/solodawn-<member>.lock /bin/sh /path/to/runner.sh
     ```
     Test: (i) an idle tick while no mention waits (`--timeout 0` exits `3`); (ii) a normal mention answered via MCP `post` followed by `solodawn done --member <id> --run <run>`; (iii) a mention whose CLI turn lasts **more than 65 seconds** (spanning a second cron minute boundary); (iv) a simulated agent CLI exit error (`solodawn done ... --failed "exit <code>"`).
   - **Expected result:** `jq` and `<exe>` resolve under cron's default `PATH`; idle ticks return immediately with no agent spawned; while the >65s turn runs, the next minute's cron trigger hits `flock -n` and exits without starting a second agent for that member; member state stays active/waiting (never flips `Offline` across 60 s intervals while `180 s` presence holds); failed CLI exits record `turn_interrupted` (`crash`) cleanly.
2. **macOS (`launchd` LaunchAgent):**
   - **Steps:** Create `~/Library/LaunchAgents/solodawn.<member>.plist` setting `<key>Label</key><string>solodawn.<member></string>`, `ProgramArguments` (`["/bin/sh", "/path/to/runner.sh"]`), `<key>StartInterval</key><integer>60</integer>`, and PATH (explicit `PATH` in `runner.sh` or `EnvironmentVariables`), load it with `launchctl bootstrap gui/$(id -u) ~/Library/LaunchAgents/solodawn.<member>.plist`, and exercise the same four cases (idle, normal reply, >65 s turn across a `60 s` interval, failed exit). Afterward clean up with `launchctl bootout gui/$(id -u)/solodawn.<member>`.
   - **Expected result:** `launchd` invokes `runner.sh` every 60 s, never starts an overlapping instance while a turn is active, completes UTF-8 prompt delivery via `jq`, updates the setup/thread UI cleanly, and stops cleanly on `bootout`.
3. **Windows (`PowerShell` + Task Scheduler `schtasks`):**
   - **Steps:** Write `runner.ps1` with `[Console]::OutputEncoding = $OutputEncoding = [Text.UTF8Encoding]::new($false)`, `solodawn.exe wait --member <id> --timeout 0`, `$LASTEXITCODE -ne 0` early exit, `ConvertFrom-Json`, `Push-Location $i.folder`, non-interactive UTF-8 text (include multi-byte Unicode characters in a test mention), and `solodawn.exe done --member <id> --run $i.run [--failed 'run failed']`. Register with:
     ```powershell
     schtasks /Create /TN "SoloDawn <member>" /SC MINUTE /MO 1 /TR "powershell -NoProfile -ExecutionPolicy Bypass -File C:\path\to\runner.ps1"
     ```
     Exercise idle tick, normal mention, >65 s overlapping interval, and CLI failure, then remove with `schtasks /Delete /TN "SoloDawn <member>" /F`.
   - **Expected result:** Non-ASCII text survives `$j | ConvertFrom-Json` and pipe to the agent without mojibake or UTF-8 BOM issues, `$LASTEXITCODE` accurately chooses success vs `--failed`, idle ticks spawn zero agents, Task Scheduler does not overlap a running instance, and `done` transitions state back to `Waiting`.
4. **Failure reporting for Section 8 (Setup + Schedulers):**
   - Attach: target OS, agent name & version, sanitized Setup panel status (which of the four marks completed), `solodawn status`, `logs/node.log`, manual `solodawn wait --member <id> --timeout 0` stdout + exit code, sanitized `runner.sh` / `runner.ps1`, and OS scheduler status/logs:
     - Linux: `crontab -l` and recent `cron` service log entries (`journalctl -u cron` or `/var/log/syslog`).
     - macOS: `launchctl print gui/$(id -u)/solodawn.<member>` and any `StandardOutPath` / `StandardErrorPath` log configured on the plist.
     - Windows: `schtasks /Query /TN "SoloDawn <member>" /V /FO LIST` and Windows Event Viewer -> `Microsoft-Windows-TaskScheduler/Operational` events for that task.

## Verify

1. Record every matrix row, OS version, browser version, CLI version, release tag, and `A8.6` memory/size measurement in a release verification report (containing zero personal accounts, tokens, machine names, or private paths).
2. Confirm zero unexplained errors in `logs/node.log` across all nodes, and remove any scratch test cron/launchd/schtasks jobs, temporary scratch groups, and test repo hooks afterward.

## Failure and recovery

- If a real-machine test fails to acquire a `direct` path on macOS, verify System Settings -> Network -> **Firewall** -> Options for `solodawn` before classifying as a networking bug.
- If a scheduled external-AI job runs manually in an interactive shell but fails under `cron`, `launchd`, or `schtasks`, check the scheduler's restricted `PATH` (`jq`, `solodawn`, and the AI CLI binary path) and file permissions first, then capture the scheduler's stderr and environment into the failure report.
- If a `live` CLI behavior diverges from the fake-CLI contract (such as Codex `item/tool/requestUserInput` in `A4.11` or a CLI refusing a listed model id), capture the exact CLI version and sanitized protocol exchange and open or update the corresponding entry in [issues.md](../../issues.md).

## References

- [install.md](../../install.md) — release verification, macOS quarantine and firewall note
- [issues.md](../../issues.md) — known platform limitations and unverified live items
- [planning/contracts/diagnostics.md](../../planning/contracts/diagnostics.md) — node log paths, events, and archive shell lines
- [planning/contracts/drivers.md](../../planning/contracts/drivers.md) — Claude Code and Codex driver contracts and live caveats
- [planning/contracts/step-3/drivers.md](../../planning/contracts/step-3/drivers.md) — capability switches and model discovery/selection
- [planning/contracts/step-3/profiles.md](../../planning/contracts/step-3/profiles.md) — AI member profiles and refresh behavior
- [planning/contracts/step-3/external.md](../../planning/contracts/step-3/external.md) — external AI setup prompt, runners, and setup check
- [planning/contracts/step-2/watch-team.md](../../planning/contracts/step-2/watch-team.md) — terminal watching consent, direct path, and revocation
- [planning/steps/step-1.md](../../planning/steps/step-1.md) — Step 1 browser/OS scope matrix
- [planning/steps/step-1-acceptance.md](../../planning/steps/step-1-acceptance.md) — A1–A8 acceptance definitions
- [planning/steps/step-2-3-acceptance.md](../../planning/steps/step-2-3-acceptance.md) — A9 acceptance matrix (`A9.40`, `A9.41`)
- [planning/steps/step-3-acceptance.md](../../planning/steps/step-3-acceptance.md) — A10 acceptance definitions (`A10.4`, `A10.13`–`A10.28`)
- [planning/steps/parallel-plan-units.md](../../planning/steps/parallel-plan-units.md) — I-11 scope and live handover list
- [planning/risks.md](../../planning/risks.md) — `Q-4`, `Q-20`, `K-1`–`K-21`
