---
description: "Real-machine acceptance checklist (I-11): what the owner runs on his own computers, phones, accounts and networks, step by step, with what to see and where to write the result."
kind: tutorial
---
# Real-Machine Acceptance Checklist (I-11)

## Summary

The owner runs these steps on his own computers, phones, CLI accounts and networks before version 1 is called done. Each step says what to do, what to see and what to write. CI runs everything else.

Do not record personal user names, host names, company network names or private paths in public places. Write `<label>`, `<secret>`, `<exe>` and `<rv-url>` instead.

## Prerequisites

- **A release with binaries.** Install `v3.0.0-rc.1`, a GitHub pre-release of this repository (the owner decided the name on 2026-11-26). `install.md` installs from the releases page, and the six `solodawn_<version>_<os>_<arch>` files with `SHA256SUMS` must be attached to it; `release.yml` builds them for a pushed `vMAJOR.MINOR.PATCH` or `vMAJOR.MINOR.PATCH-rc.N` tag but keeps them only as a workflow artifact, so the owner publishes the pre-release with those files. The update steps that need a second, newer release (step 11) are done when the next one exists (`v3.0.0-rc.2` or `v3.0.0`), not before.
- **The rendezvous.** Before step 1, set up the server you will keep with [self-hosting](self-hosting.md) steps 1 to 7 (a Linux server you own, with root access, and two DNS names; your AI can run the commands). This checklist needs the relay, so use setup C, or setup B with every address of the run allowed (phones on mobile data, the company network, `Q`'s computer); setup A (`--no-relay`, VPN only) cannot exercise the relay or the phones. You choose. **It opens ports to the internet:** step 3 opens TCP 443, UDP 3478 and UDP 20000 to 29999, to everyone in setup C and to your allowed addresses in setup B; step 7 and, for setup C, "Additional measures for setup C" check and limit it. Use a domain name, not an address: a node keeps the address it was paired with, and `pair` is refused once a node has an owner ([issues.md](../../issues.md), 2026-11-25). Every computer of this checklist pairs to this one server; there is no second server. This setup is the `A8.4` run, and its address is `<rv-url>`.
- **Computers.** One each of macOS, Linux and Windows, each with a clean user account. For [step 9](#9-two-people) and [step 10](#10-moving-a-group) three computers `P1`, `P2` and `Q1`, and a second person `Q` with their own browser.
- **Browsers** ([baseline](../decisions/implemented/architecture/2026-11-20-browser-baseline.md)): macOS Safari 17.5+ and Chrome 137+; Windows Chrome 137+ (Edge 137+ for one look, step 4); Linux Chrome 137+; iOS Safari 17.5+ (a tab, and added to the Home Screen); Android Chrome 137+. Firefox is covered by CI (decision of 2026-11-22).
- **CLI accounts**, signed in: `claude`, `codex`; `opencode` and `qwen` only if you have a key; one external MCP agent for step 8; a scratch GitHub repository for step 7.3.
- **Two networks.** A home or open LAN where direct UDP and ICE-TCP (`47801`) work, and a restricted or company network (or a host firewall that blocks them) that forces the relay on port `443`.
- **The permission level.** Version 1 has no control for it; the level is 3. The checks that need level 1 or 2 are in the [last table](#sentences-with-no-real-machine-step).
- **Where to write.** Each step ends with **Write**. Collect the lines in one release verification report: a line per id, pass or fail, with the OS, browser, CLI and release versions. The report holds no personal accounts, tokens, machine names or paths.

## Steps

### 1. Install, pair, start at login, reboot (`A8.1`)

Run on a clean user account on macOS, Linux and Windows.

- **Do:**
  1. In the browser, open **Add a computer**, copy **Prompt for your AI**, and paste it into an AI CLI (Claude Code or Codex) on the machine. Install nothing by hand.
  2. When the AI is done, run `solodawn version` and `solodawn status`; compare the fingerprint with the one on the browser page.
  3. Find the start-at-login entry: macOS `~/Library/LaunchAgents/org.solodawn.node.plist`; Linux `~/.config/systemd/user/solodawn.service`; Windows the scheduled task `\SoloDawn-<SID>` that runs `conhost.exe --headless "<exe>" node`.
  4. Log out and in again, then reboot the machine. Wait 30 seconds after login.
- **See:** the AI downloads the binary and `SHA256SUMS` from the same release and checks the digest (on macOS the quarantine mark is cleared only after it matches), asks you for a name and email if the Git identity was empty, and runs `solodawn pair`, which prints `<label> (<fingerprint>) direct` or `relay`. `solodawn version` is the release tag and the fingerprints agree. After the re-login and after the reboot the node runs with no terminal window, `solodawn status` answers, and **Computers** shows it **Online**.
- **Write:** `A8.1` per system: release tag, OS version, anything the AI had to ask you.

### 2. macOS Application Firewall (`install.md`)

Run on macOS right after step 1, with Firewall on (System Settings, Network, Firewall). The node already runs as the login service.

- **Do:** In System Settings, Network, Firewall, Options, remove `solodawn` and any Go test binary (`*.test`) if listed: a development or pre-identity build of `solodawn` shares their identity, so macOS would apply their entry without asking. Restart the node: `launchctl bootout "gui/$(id -u)/org.solodawn.node"`, then `launchctl bootstrap "gui/$(id -u)" "$HOME/Library/LaunchAgents/org.solodawn.node.plist"`. When the system asks whether to accept incoming connections: **Pass 1**, deny. From a browser on another computer of the LAN, open the page and read **Computers**; also run `solodawn status`. **Pass 2**, allow `solodawn` in Firewall Options and reload the browser page (a relayed connection does not move to direct by itself; a browser's own ICE may do it sooner, so note which browser).
- **See:** macOS asks about `solodawn` by name (`codesign -dv "$(command -v solodawn)"` shows `Identifier=org.solodawn.node`). Pass 1: signalling works and the connection is relayed (**Relayed · limited**, `relay`). Pass 2: after the reload the connection is **Direct** (`direct`).
- **Write:** `A1.3` (direct on a LAN) and the firewall note: the times of both passes, the browser.

### 3. Logs and the Windows archive ([diagnostics.md](../../planning/contracts/diagnostics.md))

- **Do:**
  1. Open the log folder: macOS `~/Library/Application Support/solodawn/logs`; Linux `${XDG_DATA_HOME:-$HOME/.local/share}/solodawn/logs`; Windows `%LocalAppData%\solodawn\logs`.
  2. macOS and Linux, node running: run the archive line of `install.md` ("After a failure").
  3. Windows: first run the archive line with the node **running** and note whether it works (`diagnostics.md` section 6 does not know); then stop the node as `install.md` says (the `Stop-ScheduledTask` block, or Ctrl+C) and run it again.
  4. Write the word `debug` into `logs/level`, restart the node with the stop and start commands of `install.md` ("Update"), then delete the file.
- **See:** every archive command that `install.md` gives exits `0` and holds `node.log` (and `node.log.1` once it passed 10 MiB): UTF-8, UTC microsecond timestamps, `node=<8 characters>` on each line, no message text, file content, token or key. The first line after the restart (item 4) is `node.start` with `level=debug`.
- **Write:** the archive result for each system, and for Windows whether the running-node archive passed or failed (with the error).

### 4. Browser pairs and Web Push (`A8.5`, `A8.3`, `A9.40`)

Pair each node OS with the browsers of the table.

| Node OS | Browsers |
| --- | --- |
| macOS | macOS Safari and Chrome; iOS Safari (tab and Home Screen); Android Chrome |
| Windows | Windows Chrome; macOS Safari |
| Linux | Linux Chrome; iOS Safari |

- **Do:**
  1. On each pair: open the page, pair with **Add a browser** (the link has `#pair=<secret>`; `solodawn pair-browser` prints it), check `solodawn status`, reload, and confirm the browser still signs in with its stored key (no new pairing).
  2. On Windows also open the page once in Edge and see that it loads and signs in.
  3. Open a second tab of the same browser; use **Use here** to take the connection back (`A1.12`).
  4. One-person group (`A9.40`): create a group, add AIs, run brief, task, review and merge. In dark and light, English and Chinese. Then a message that appears in both browsers of the pair with no reload (`A9.41`); the invitation link is step 9.
  5. On desktop Chrome, Android Chrome and iOS Safari added to the Home Screen: allow notifications (`A8.3`), close the page, leave the node running, make something need you: ask an AI of the group to reply with a mention of you, read the notification and tap it.
- **See:** no upgrade notice on a supported browser; the second tab shows **SoloDawn is open in another tab.** and **Use here** moves the connection; no `confirm` card in the one-person group; no layout break. The closed-page notification for the mention reads `<group> · Mentioned you` with the name of who mentioned you as its text, never message text or a command, and tapping it opens that message. (The other kinds, `Approval`, `Confirm start`, `Confirm`, `Your call` and `Task done`, add `who: <one-line title>`.) On iOS the push service accepts the node's VAPID `sub` claim ([issues.md](../../issues.md), Web Push).
- **Write:** `A8.5` one line per pair (node OS, browser and version, OS version); `A8.3` per device; `A9.40`; `A9.41` per pair.

### 5. CLI model lists and selected models (`A10.13` to `A10.16`)

- **Do:**
  1. In each real CLI (`claude`, `codex`, `opencode models --verbose`; `qwen` has no list) note the models and effort levels of your account.
  2. In **Add an AI** (for `claude`, `codex`, `opencode`) read **Model**, **Level**, **Tier** and **Fast mode**; for `qwen` expect two free-text fields.
  3. Add two members of one CLI on one node with different **Model** and **Level**, mention each, and read the model and effort in the CLI's own session record (`A10.15`).
  4. Set a member's **Tier** and **Skills**, press **Refresh the models**, then restart the node (`A10.14`, `A10.16`). Change a **Model**; set one that does not exist (for example `invalid-model-check`), mention the member, then restore it.
  5. With a key for `opencode` and `qwen`: run `A3.1` (two mentions, the second shows it remembers, one process), `A3.4` (idle for the idle time, 15 minutes by default, then resume), and `A3.6` (kill the CLI mid-turn, the next mention works). No takeover for them.
  6. Sign-in failure: sign out of `claude` (then `codex`), mention that member, write what the browser shows, and attach the sanitized CLI output; sign in again. Do it once per CLI.
- **See:** the pickers match each CLI's own list; refresh adds models with no restart and never changes **Tier**, **Skills**, **Model** or **Level** (a removed model shows **Not listed**); each member runs its chosen model; the bad model ends as `turn_interrupted` (`crash`), keeps the session id and sends no `session_replaced`; the sign-in failure shows **`<AI>`'s sign-in is no longer valid.** in the thread and mentions you; a crash (`turn_interrupted`) or a lost session instead is a fail: write it and attach the output.
- **Write:** `A10.13` to `A10.16` per CLI and version; for `opencode` and `qwen` the version, what differed from the documented API in [issues.md](../../issues.md), the three sessions of step 5.5, and the sign-in failure texts of step 5.6 (they close the two `issues.md` entries on `LoginInvalid` matching and the missing invalid-login captures).

### 6. Terminal takeover and watching (`A7.1`, `A7.2`, `A7.5`, `A7.8`, `A7.9`, `A9.20`, `A9.21`)

Use real `claude` and `codex` members. `O` is the owner's browser; `W` is a teammate's browser in the same group.

- **Do, home network (direct):**
  1. Take a Claude Code member that has a task: **Open terminal**, ask it to edit a file of the task worktree, resize the window, then **Hand back** (`A7.1`). Mention it in the task thread.
  2. The same with Codex (`A7.2`), and on Windows exit the CLI inside the terminal and watch the last output lines.
  3. **Open terminal** on the secretary and ask it to edit a repository file (`A7.8`).
  4. While `O` has a terminal open, `W` presses **Ask to watch**. On `O`'s terminal page the consent card appears: `O` presses **Confirm**. The **Watch** page opens by itself once `O` has confirmed. Then `O` presses **Stop watching**, and once more end it with **Hand back** (`A9.20`).
- **Do, company network (relay):** put `O` or the node behind the restricted network. Read **Computers**, send chat, answer a card, look at **Open terminal** and **Watch**. Then open a terminal and a watch on the home network and switch to the restricted one mid-session, also on Windows (`A7.5`, `A7.9`, `A9.21`).
- **See:** the terminal works and resizes, hand-back posts a summary of files and commits, the AI remembers the exchange. The secretary's terminal: record whether a file changed; its session runs at level 3 and is held only by its instructions ([issues.md](../../issues.md), "Group and review sessions default to permission level 3"). `W` sees **Watching · receive only**, cannot type or resize, loses the stream on **Stop watching** and on hand-back, and gets no output before consent. Relay: chat and cards work, **Open terminal** says **Direct link only**, **Watch** is disabled and the node answers `relay`. After the switch the terminal shows **Handed back · switched to relay** only if the same connection moved to a relayed pair; if the network change dropped the connection and it reconnected, it shows plain **Handed back** (write which one). `W` sees **Watching ended**, and `ctl` keeps answering with no re-pairing.
- **Write:** `A7.1`, `A7.2`, `A7.8` (changed or not), `A7.5`, `A7.9`, `A9.20`, `A9.21`; attach, for a failure, the `ice.pair`, `ice.state`, `ctl.open` and `conn.close` lines of both logs and the browser's `chrome://webrtc-internals` pair dump.

### 7. Live CLI checks, remote, rendezvous (`A3.8`, `A4.1`, `A4.11`, `A5.13`, `A10.4`, `A1.4`, `A8.6`)

1. **Repository hooks (`A3.8`, `A10.4`).** In a test repository add a hook that writes a marker file. Run one group mention, one task turn and one review turn on each real CLI. The hook must fire in task and review sessions and not in group sessions (Claude `--setting-sources`, Codex `project_doc_max_bytes = 0`). For Qwen Code record four results: group and task session, each with the hook unapproved and approved (approve it by starting `qwen` once in the folder), and whether `QWEN.md` was read; then decide the `@pending` scenario of `features/s3_drivers.feature` ([issues.md](../../issues.md), "A Qwen Code group session may load the repository's configuration"). **Write:** `A3.8`, `A10.4` per CLI.
2. **Questions (`A4.1`, `A4.11`).** Claude Code: ask it for a multiple-choice question (`AskUserQuestion`); a `decision` card must reach the browser and the answer must reach the CLI. Codex with `-c features.default_mode_request_user_input=true`: see whether it sends `item/tool/requestUserInput`; if the installed release has no such tool, see that it asks through `request_decision` ([drivers.md](../../planning/contracts/drivers.md) 1.2). **Write:** `A4.1`, `A4.11` with CLI versions.
3. **GitHub remote (`A5.13`).** With a scratch GitHub repository, run brief, task, `run_checks`, `push_branch`, `submit_task`, review and merge. **See:** the task's branch is on the remote and the request merges at its reviewed head. **Write:** `A5.13`, `A5.1`.
4. **Rendezvous and footprint (`A1.4`, `A8.6`).** With a direct connection open and answering `host.info`, stop the rendezvous on the server (`sudo systemctl stop solodawn-rendezvous`), wait 10 minutes, read `host.info` again, and start it again (`A1.4`). Record for `A8.6`: CLI versions, node binary size (target 30 MB or less), node idle memory (40 MB or less) and peak memory per open session of each CLI, with the machine named. **Write:** `A1.4`, `A8.6`.

### 8. External AI: setup prompt and scheduled jobs (`A10.28`, [external.md](../../planning/contracts/step-3/external.md))

Run with a real external MCP agent (for example Cursor or ZCode) on Linux, macOS and Windows nodes.

#### 8a. Self-setup

- **Do:** **Add an AI**, choose **External AI**, pick an account type, submit. Open the member's **Setup** panel, copy the **Self-setup prompt**, paste it unedited into the agent on that computer, watch the four marks (`Runner seen`, `Item taken`, `Member answered`, `Run ended`), and press **Send setup check again** once at the end.
- **See:** with no help the agent adds the `solodawn` stdio MCP server (`command: <exe>`, `args: ["mcp", "--member", "<member-id>"]`), writes and starts a runner, answers `Setup check: answer here in one line, then finish.` once, all four marks turn **Complete**, the member is `Waiting`, and no AI process stays running while idle.
- **Write:** `A10.28` per system and agent version.

#### 8b. Scheduled runners (`solodawn wait --member <id> --timeout 0` once a minute)

Run each case: idle tick (exit `3`), a normal mention answered with the MCP `post` and `solodawn done --member <id> --run <run>`, a turn longer than 65 seconds, a failed exit (`solodawn done ... --failed "exit <code>"`).

- **Linux (`cron` with `flock`):** `* * * * * flock -n /tmp/solodawn-<member>.lock /bin/sh /path/to/runner.sh`, with absolute paths (cron's `PATH` is minimal). **See:** `jq` and `<exe>` are found; an idle tick spawns nothing; the next minute starts no second agent during the long turn; the member never goes `Offline` (presence holds 180 seconds); a failed exit records `turn_interrupted` (`crash`).
- **macOS (`launchd`):** `~/Library/LaunchAgents/solodawn.<member>.plist` with `Label`, `ProgramArguments` `["/bin/sh", "/path/to/runner.sh"]`, `StartInterval` 60 and a `PATH`; `launchctl bootstrap gui/$(id -u) <plist>`; at the end `launchctl bootout gui/$(id -u)/solodawn.<member>`. **See:** the same.
- **Windows (`schtasks`):** `runner.ps1` sets UTF-8 output, calls `solodawn.exe wait`, returns when `$LASTEXITCODE -ne 0`, reads the JSON, `Push-Location $i.folder`, and ends with `solodawn.exe done`; register it with `schtasks /Create /TN "SoloDawn <member>" /SC MINUTE /MO 1 /TR "powershell -NoProfile -ExecutionPolicy Bypass -File C:\path\to\runner.ps1"`, remove it with `schtasks /Delete /TN "SoloDawn <member>" /F`. Put non-ASCII text in a test mention. **See:** the text arrives without mojibake or a BOM, `$LASTEXITCODE` picks success or `--failed`, no overlap, `done` returns the member to `Waiting`.
- **Write:** `A10.28` per scheduler.

### 9. Two people

Cast: `P` owns `P1` (the group's home) and `P2`; `Q` owns `Q1` and has two AIs on it, `Q-personal` (account type personal) and `Q-team` (account type team, on a different CLI). `P` is the owner of the group, `Q` joins as a member. Use real CLIs.

- **Do:**
  1. `P` creates a project group, then **Invite**, **Create link**; `Q` (with a computer already connected) opens the link, **Join group**. Open the link a second time and, after it expires, once more (`A9.1`, `A9.2`).
  2. `P` presses **Make admin** on `Q`, `Q` edits the group settings; **Make member**, `Q` tries again (`A9.3`).
  3. `Q` presses **Add AI** on `Q1` for `Q-personal` and `Q-team` (`A9.5`).
  4. `P` mentions `Q-personal` (`A9.8`); `Q` mentions it too. `Q` denies one request and ignores another (`A9.9`). `P` mentions `Q-team`.
  5. `Q` presses **Edit access** on `Q-team`, adds `P` under **Allow list**, saves. `P` mentions `Q-team` and confirms a brief for it. Then `Q` removes `P` and `P` mentions again (`A9.14`). The same list on `Q-personal` must change nothing.
  6. `P` confirms a brief that gives `Q-personal` one task and the review of another (`A9.11`). `P` cancels the task and confirms a new brief (`A9.13`).
  7. `P`'s AI on `P2` asks `Q-personal` through its `ask` tool; `Q` denies that card once and confirms it the next time; then it asks `Q-team` (`A9.18`).
  8. `Q` adds `P` under **Approval delegates** of `Q-team`; make `Q-team` need an approval; `P` approves; check that the command ran. `Q` removes `P`; answer an older card as `P` (`A9.19`).
  9. `P` makes `Q` admin and confirms a brief that names only `Q`'s AIs. `Q` presses **Reassign** on a task to an AI of `P` on `P2` (`A9.43`).
  10. In **Group info** choose `Q-team` as the Secretary; ask it to write a brief (`A9.44`). Read the warning under **Secretary** with two people and `Q-personal` as secretary, then with one person and with an account type other than personal (`A9.39`).
  11. `P` creates a **Team group** and asks the project's secretary to post a notice there (`A9.22`, `A9.23`).
  12. On `P1`, `P` presses **Remove** on `Q` while an AI of `Q` holds an unfinished task, then after it is done (`A9.4`).
- **See:** `Q` joins as `member` and both browsers show the group and its history with no reload; a used or expired link is refused. Admin `Q` can edit settings and invite; as member `Q` is refused. Only a mention by `P` of `Q-personal` makes a `Confirm` card for `Q`; nothing runs before `Q` confirms from either of two devices, a refusal or silence runs nothing, and `P` sees the card's state. A listed `P` runs `Q-team` with no card; after removal a card again. The `Q-personal` list shows that it has no effect on a personal account. A brief gives `Q` cards for both roles; after the cancel and new brief a new card. The `ask` to `Q-personal` waits for `Q`, a denial reaches the AI as a turn, and `Q-team` answers with no card. The delegate approves and the command runs; after removal the old card is refused. `Q` as admin: the reassign makes a card for `P`; the task starts on `P2` after `P` confirms. The secretary is `Q-team`. The warning appears only with two people and a personal secretary. The team group has no task list, no brief and no AI cards. The first removal is refused (`conflict`), the second one works and `Q`'s browser no longer lists the group.
- **Write:** `A9.1` to `A9.5`, `A9.8`, `A9.9`, `A9.11`, `A9.13`, `A9.14`, `A9.18`, `A9.19`, `A9.22`, `A9.23`, `A9.39`, `A9.43`, `A9.44`, `A9.41` (the pairs of step 4 for the two-person run).

### 10. Moving a group

`A9.27`, `A9.28`, [export.md](../../planning/contracts/step-2/export.md). Group `G1` is the group of step 9: `P1` is its home, `P2` is another computer of `P`, and `Q` has joined, with `Q1`. `G2` is a second group whose home is `P1`.

- **Do:**
  1. Create `G2` on `P1`, as in step 9.1 let `Q` join it, and add a message.
  2. Back up `G2` on `P1` as `install.md` ("Back up a group") says: on Linux or macOS a `crontab` line with `* * * * *` instead of `0 3 * * *`, on Windows the `Register-ScheduledTask` block and then `Start-ScheduledTask -TaskName SoloDawnBackup`. Wait for the file, then remove the schedule at once, so that this file is the last copy.
  3. Move `G1` (`A9.27`): in **Group info** of `G1` open **Move group**, **Export and import**; it shows the commands with the group and the device ids filled in. On `P1` run `solodawn group export <group id> <file> --to <P2 device id>` from it. This stops `P1` being the home at once, and `G1` is unavailable until the import finishes. Copy the file to `P2` and run `solodawn group import <file>` there. Try the import on `Q1`, and the export of `G1` on `P1` again (now not its home).
  4. Restore `G2` (`A9.28`): write one more message to `G2`, then stop `P1`'s node (the commands of `install.md`, "Update") and do not log in again on `P1` or reboot it until the end of this step. Copy the backup file to `P2` and run `solodawn group import <backup file>` there. `A9.28` deletes `P1`'s data directory; a stopped `P1` cannot answer either, so nothing is moved or deleted here.
  5. `P` sends an invitation to `G2` from `P2`; `Q` opens it.
  6. Start `P1`'s node again. From now on `G2` may be shown from `P1`'s old copy or from `P2`, whichever home answers first ([issues.md](../../issues.md), "Two homes that both hold a group"). No command deletes a group in version 1, so treat `G2` as a used scratch group and do not use it again.
- **See:** after step 3 both browsers and `Q1` follow `P2` by their normal retry with no reload; the next task of `G1` has the next number; the import on `Q1` and the export on a non-home node fail; the group info shows the two commands to `P` only. After step 4 `P2` is the home of `G2` with the rows of the backup, without the last message. After step 5 `Q` opens the link, is still listed once under People, and follows `P2`.
- **Reassignment between two people's computers:** with a remote, reassign a task with unpushed commits from an AI on one computer to an AI on the other, with the old computer **on**. **See:** the new AI's worktree has the old AI's last commits; if the push is refused it is told once in the task's thread ([issues.md](../../issues.md), "A task handed to an AI of another node can start without the last commits").
- **Write:** `A9.27`, `A9.28`, and the reassignment result.

### 11. Update by hand (`install.md`)

Needs two releases (see Prerequisites). Run on macOS, Linux and Windows, with the node as the login service.

- **Do:** with the older release installed and a group with history, follow "Update" in `install.md`: download the new `SHA256SUMS` and binary, stop the node (macOS `launchctl bootout`, Linux `systemctl --user stop`, Windows the `Stop-ScheduledTask` block), replace the binary with the commands of "Verify and install", start it again, run `solodawn version`. Repeat once with the node running in a terminal: stop the login service first, then start `solodawn node` in the terminal, and press Ctrl+C there before you replace the binary.
- **See:** the new tag; the node is online with the same pairing and the same groups; on Windows the binary could be replaced after the stop block.
- **Write:** the update of `install.md` per system, with both tags.

## Verify

1. Every id above has a line in the report.
2. `logs/node.log` of every node has no unexplained error line.
3. Remove what you made: cron, launchd and scheduler jobs, scratch repositories, test hooks (scratch groups stay: version 1 cannot delete a group).

## Failure reports

Attach: the id, the OS, browser, CLI and release versions, the UTC time, the steps, `solodawn status`, the log archive of step 3 (`node.log` and `node.log.1`), the browser console, and for CLI failures the CLI's version and sanitized output. Add or update the entry in [issues.md](../../issues.md), with no usernames, host names or private paths.

If a direct path fails on macOS, look at System Settings, Network, Firewall, Options first. If a scheduled job works by hand but not under `cron`, `launchd` or `schtasks`, look at the scheduler's `PATH` (`jq`, `solodawn`, the AI's binary) and file permissions, and attach its stderr. If a CLI's behaviour differs from its fake CLI, attach the exact version and the sanitized protocol exchange.

## Sentences with no real-machine step

| Sentence | Why |
| --- | --- |
| `A1.3` "after unblocking, direct within one probe interval plus 5 s"; `A1.13`; first sentence of `A2.7` | Removed: no relay-free probe and move. |
| `A1.4` second sentence | Removed: no beacon, `PR-2` cancelled. |
| `A9.7`, `A9.10`, `A9.15`, `A9.37`, `A9.38`, `A9.45`; `A9.16`, `A9.17`; `A9.29` to `A9.36` | Removed in the acceptance text. |
| `A4.6` | Removed by the owner's rule of 2026-11-21 (no defence in code). |
| `A4.5` | Contradicted by [drivers.md](../../planning/contracts/drivers.md) 1.1: at level 3 the CLIs still ask and it becomes a card; step 7.2 shows the question cards. |
| `A8.2` self-update | Not in version 1; step 11 is the update by hand. |
| `A8.5` on Firefox | CI, Firefox 136, every pull request. |
| `A4.1` at levels 1 and 2, `A4.4`, `A4.10`, the level 1 checks of OpenCode and Qwen Code, `A5.13`'s level 2 card check | Need a permission level other than 3. Version 1 has no control for it (`perm.set` is postponed, [commands.md](../../planning/contracts/commands.md) section 2); only a row in the node's database sets it. A coordinator's agent does it on a test node (needs `sqlite3`) and runs these with the real CLIs. |
| `A9.26` | The live run needs a source checkout, the Go toolchain and a forge template configuration ([s2_watch.md](../../features/s2_watch.md), "A9.26 live run"). A coordinator's agent runs it with the owner's signed-in forge program. |
| `A8.6`, rendezvous memory with 100 idle connections | The repository has no tool that makes the connections. A coordinator's agent runs it with a load tool. |
| Exhausted usage limit of a CLI | Cannot be made on demand; attach the sanitized output when one happens ([issues.md](../../issues.md), "Fake CLIs lack invalid-login and exhausted-limit captures"). |
| `A9.12`, `A9.42`, the header of `A9.13` | The header is text the node writes; the fake CLI logs it and a real CLI adds nothing. |
| `A9.24`, `A9.25` | A fake forge program records the arguments; `A9.26` covers a real one. |
| `A10.4` for OpenCode and Qwen Code | Their tables have no level 2 and no takeover: only `A3.8` and level 1 apply (step 7.1 and the level row above). |
| Qwen Code and OpenCode MCP connection | Not asked, left as it is ([issues.md](../../issues.md), 2026-11-26). |
| Windows `AF_UNIX` shim EOF | Accepted limitation ([issues.md](../../issues.md), 2026-11-21). |

## References

- [install.md](../../install.md) — install, update, backup, failure archive
- [self-hosting.md](self-hosting.md) — the rendezvous and its checks
- [issues.md](../../issues.md) — known limits and unverified items
- [diagnostics.md](../../planning/contracts/diagnostics.md) — log paths, events, archive lines
- [drivers.md](../../planning/contracts/drivers.md) and [step-3/drivers.md](../../planning/contracts/step-3/drivers.md) — CLI drivers, levels, models
- [step-3/profiles.md](../../planning/contracts/step-3/profiles.md) and [step-3/external.md](../../planning/contracts/step-3/external.md) — profiles, external AI
- [step-2/watch-team.md](../../planning/contracts/step-2/watch-team.md) and [step-2/export.md](../../planning/contracts/step-2/export.md) — watching, export and import
- [step-1-acceptance.md](../../planning/steps/step-1-acceptance.md), [step-2-3-acceptance.md](../../planning/steps/step-2-3-acceptance.md), [step-3-acceptance.md](../../planning/steps/step-3-acceptance.md) — the acceptance tests
- [parallel-plan-units.md](../../planning/steps/parallel-plan-units.md) — I-11
