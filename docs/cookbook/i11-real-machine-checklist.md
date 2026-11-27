---
description: "Real-machine acceptance checklist (I-11) for the release candidate v3.0.0-rc.2: what the owner runs on his own computers, phones, accounts and networks, step by step, with what to see and where to write the result."
kind: tutorial
---
# Real-Machine Acceptance Checklist (I-11)

## Summary

The owner runs these steps on his own computers, phones, CLI accounts and networks before version 1 is called done. Each step says what to do, what to see and what to write. CI runs everything else.

Part 1 (steps 1 to 11) walks through the product as it is now: one kind of group, made of people and AIs that are already connected; each AI works in one folder on its own computer; a task goes work, report, review, done. Part 2 (steps 12 to 20) holds the checks that need real networks, phones and CLI accounts. Do Part 1 first, in order: each step uses what the one before it made.

Words used below:

- **AI**: a command-line AI tool (`claude`, `codex`, `opencode`, `qwen`) on one of your computers, signed in, with an account type chosen on the **Computers** page. Connecting one needs nothing else.
- **Folder**: the folder on that computer that the AI works in. It is your own folder, not a copy. SoloDawn needs no Git, makes no branch and merges nothing; if the project uses Git, the AI uses it by the project's own rules.
- **Group**: the room where people and AIs talk and tasks are given. There is one kind. A group with people and no AI is an ordinary group.
- **Task**: goes through **Work**, **Review** and **Done**. The AI that does the work sends a **report**; another AI reviews it. A task that passes the review is done at once. One folder holds one task at a time.

Do not record personal user names, host names, company network names or private paths in public places. Write `<label>`, `<secret>`, `<exe>` and `<rv-url>` instead.

## Prerequisites

- **A release with binaries.** Install `v3.0.0-rc.2`, a GitHub pre-release of this repository. It is not published yet: the owner publishes it (with the six `solodawn_<version>_<os>_<arch>` files and `SHA256SUMS` attached) before these steps are run, and writes every step against that name. `install.md` installs from the releases page; `release.yml` builds the files for a pushed `vMAJOR.MINOR.PATCH` or `vMAJOR.MINOR.PATCH-rc.N` tag but keeps them only as a workflow artifact. Step 1 also needs `v3.0.0-rc.1` on one computer, as the older release.
- **The rendezvous.** Before step 2, set up the server you will keep with [self-hosting](self-hosting.md) steps 1 to 7 (a Linux server you own, with root access, and two DNS names; your AI can run the commands). This checklist needs the relay, so use setup C, or setup B with every address of the run allowed (phones on mobile data, the company network, `Q`'s computer); setup A (`--no-relay`, VPN only) cannot exercise the relay or the phones. You choose. **It opens ports to the internet:** step 3 of that guide opens TCP 443, UDP 3478 and UDP 20000 to 29999, to everyone in setup C and to your allowed addresses in setup B; its step 7 and, for setup C, "Additional measures for setup C" check and limit it. Use a domain name, not an address: a node keeps the address it was paired with, and `pair` is refused once a node has an owner ([issues.md](../../issues.md), 2026-11-25). Every computer of this checklist pairs to this one server; there is no second server. This setup is the `A8.4` run, and its address is `<rv-url>`.
- **Computers.** One each of macOS, Linux and Windows, each with a clean user account, except the one that runs `v3.0.0-rc.1` in step 1. For Part 1 you need two computers, `P1` (the home of the groups) and `P2`, and a second person `Q` with their own computer `Q1` and browser. Steps 19 and 20 use the same three.
- **Folders** (made before step 4, one per AI, each on the computer the AI runs on): `F1`, a plain folder on `P1` with a few files and **no Git**; `F2`, on `P2`, a Git project cloned from a scratch repository you own on a code host, with a remote (step 17.3 uses it); `F3`, another plain folder on `P1`. Commit or set aside anything of your own in a folder you use for real: the AIs work in it at full permission.
- **Browsers** ([baseline](../decisions/implemented/architecture/2026-11-20-browser-baseline.md)): macOS Safari 17.5+ and Chrome 137+; Windows Chrome 137+ (Edge 137+ for one look, step 14); Linux Chrome 137+; iOS Safari 17.5+ (a tab, and added to the Home Screen); Android Chrome 137+. Firefox is covered by CI (decision of 2026-11-22).
- **CLI accounts**, signed in: `claude`, `codex`; `opencode` and `qwen` only if you have a key; one external MCP agent for step 18.
- **Two networks.** A home or open LAN where direct UDP and ICE-TCP (`47801`) work, and a restricted or company network (or a host firewall that blocks them) that forces the relay on port `443`.
- **The permission level.** Version 1 has no control for it; the level is 3. The checks that need level 1 or 2 are in the [last table](#sentences-with-no-real-machine-step).
- **Where to write.** Each step ends with **Write**. Collect the lines in one release verification report: a line per id, pass or fail, with the OS, browser, CLI and release versions. The report holds no personal accounts, tokens, machine names or paths.

## Part 1: the product as it is now

### 1. Upgrade from `v3.0.0-rc.1` (`A8.7`, `install.md`)

Run on the computer that runs `v3.0.0-rc.1` with the node as the login service (use `P1`), and repeat on any other system that has rc.1. Before you start the rc.1 computer needs at least one group with history and with a person from another of your groups, an AI with an account type, and a paired browser. If it has none, install rc.1 by its own `install.md` and make them first.

- **Do:**
  1. On rc.1, write down what **Computers** lists (your computers, each AI's account type) and the names of the people in your groups. Export one group with `solodawn group export <group id> <file>` and keep the file. Look at the old `projects/` folder in the node's data folder (macOS `~/Library/Application Support/solodawn`, Linux `${XDG_DATA_HOME:-$HOME/.local/share}/solodawn`, Windows `%LocalAppData%\solodawn`).
  2. Before updating the others, update only `P1`: follow "Update" in `install.md`: download the new `SHA256SUMS` and binary, stop the node (macOS `launchctl bootout`, Linux `systemctl --user stop`, Windows the `Stop-ScheduledTask` block), replace the binary with the commands of "Verify and install", start it again, run `solodawn version`. A second computer still on rc.1 stays as it is for the moment.
  3. Reload the browser. Look at **Computers** and at the group list.
  4. Run `solodawn group import <file>` with the file of item 1.
  5. Update the second computer, then repeat the update once with the node running in a terminal: stop the login service first, start `solodawn node` in the terminal, and press Ctrl+C there before you replace the binary.
- **See:** after item 2 the tag is `v3.0.0-rc.2`. In item 3 your computers are still listed and **Online**, with the same pairing and the same account types, and the group list is **empty**: every old group is gone, with its messages, tasks, notes and sessions. The people who were in your groups are not in any group of yours any more: they must be invited again by link (step 9). The old `projects/` folder is still there and untouched; it may hold task branches that were never pushed, so copy what you need before you delete it. Item 4 is refused in one line that says the file was written by rc.1. While one computer is still on rc.1, it cannot work with the updated one: the updated node answers it `version`; write what **Computers** and `solodawn status` show for it. After item 5 both work together. On Windows the binary could be replaced after the stop block.
- **Write:** `A8.7` per system: both tags, what stayed, what went, the text of the refusal in item 4, the update of `install.md` per system.

### 2. Install, pair, start at login, reboot (`A8.1`)

Run on a clean user account on macOS, Linux and Windows. `P1` is already done by step 1 and only needs a reboot check; do the rest fresh.

- **Do:**
  1. In the browser, open **Add a computer**, copy **Prompt for your AI**, and paste it into an AI CLI (Claude Code or Codex) on the machine. Install nothing by hand.
  2. When the AI is done, run `solodawn version` and `solodawn status`; compare the fingerprint with the one on the browser page.
  3. Find the start-at-login entry: macOS `~/Library/LaunchAgents/org.solodawn.node.plist`; Linux `~/.config/systemd/user/solodawn.service`; Windows the scheduled task `\SoloDawn-<SID>` that runs `conhost.exe --headless "<exe>" node`.
  4. Log out and in again, then reboot the machine. Wait 30 seconds after login.
- **See:** the AI downloads the binary and `SHA256SUMS` from the same release and checks the digest (on macOS the quarantine mark is cleared only after it matches), and runs `solodawn pair`, which prints `<label> (<fingerprint>) direct` or `relay`. `install.md` has no Git step and no `git` or `gh` is needed for SoloDawn itself; the prompt still tells the AI to check the Git identity, which is a known leftover ([issues.md](../../issues.md), 2026-11-27): write whether the AI asked. `solodawn version` is `v3.0.0-rc.2` and the fingerprints agree. After the re-login and after the reboot the node runs with no terminal window, `solodawn status` answers, and **Computers** shows it **Online**.
- **Write:** `A8.1` per system: release tag, OS version, anything the AI had to ask you.

### 3. Connect your AIs and choose their account types

- **Do:** Open **Computers**. Under each computer, find the AIs it found (`claude`, `codex`, and the others you have). For each one press **Account type** and choose the type of the account it is signed in with (personal, team or API key). Leave one AI without a type.
- **See:** each AI shows **Signed in** and its account type; the one without a type says **Account type not set**. An AI with no type, or whose sign-in is no longer valid, cannot be chosen when you make a group (step 4); it appears there greyed out with the reason.
- **Write:** the list of AIs per computer, and whether the unset one was refused with the reason in step 4.

### 4. Create the first group: an AI in a folder each (`A2.8`, `A3.11`)

- **Do:**
  1. Open the group list and press **New group**. Fill **Group name** (`Alpha`), **Your name in the group**, and **Home computer** (`P1`).
  2. In **AIs**, tick one AI on `P1` and one on `P2`. Give each a name. Mark the first as the secretary.
  3. For each, press **Choose**: the dialog **Choose a folder** lists the folders that AI was used in on its computer, newest first. Pick `F1` for the AI on `P1` and, for the AI on `P2`, type the path of `F2` in **Another folder**.
  4. Press **Create group**.
  5. Try a folder that does not exist, then a relative path such as `project`, as the folder of a new AI (**Add AI** in the member panel, or this dialog).
  6. Look at the member panel.
- **See:** under **People** the dialog says **Nobody else is in your groups on this computer yet.** The list of folders is the folders the CLI itself has used; a folder that no longer exists, and the node's own data folder, are not listed. The group is created and the member panel shows each AI with its **Folder**. A folder that does not exist is refused (**There is no such folder on that computer.**); a relative path is refused (**Give the whole path of the folder…**). Nothing is cloned, no branch is made, and the node's data folder gets no project folder for the group.
- **Write:** `A2.8`, `A3.11` per CLI and version (the folders listed; note when the list is empty and you had to type).

### 5. The folder list and the one-group rule (`A2.9`, `A2.10`, `A3.12`, `A3.13`)

- **Do:**
  1. Press **New group** again, tick the AI of `P1` that is in `Alpha`, press **Choose** and look at the list. Do not create this group yet.
  2. Type the path of `F1` in **Another folder** and try to use it. Then choose `F3` instead and press **Cancel**.
  3. In `Alpha`, press **Add AI** for a second AI on `P1`, press **Choose**, and look at `F1` in the list. Choose it and add the AI.
  4. Rename or move `F1` on `P1` so that it is gone, mention the AI in `Alpha`, then put it back and mention it again.
- **See:** in item 1 `F1` says **Taken by the group Alpha** and cannot be chosen. In item 2 the typed path is refused (**This folder belongs to the group Alpha. Choose another one.**, or the computer's own refusal naming the group). In item 3 `F1` says it is the folder of the first AI (**…works here too**) and, once chosen, **Shared with <AI>: the folder takes one task at a time.** In item 4 the first mention ends as a failed turn (`turn_interrupted`) that names the folder; after you put it back the next mention works.
- **Write:** `A2.9`, `A2.10`, `A3.12`, `A3.13`.

### 6. A task from brief to done, with a review (`A5.1`, `A5.4`, `A5.21`)

Use two AIs in `Alpha` that are in different folders (`F1` on `P1`, `F2` on `P2`), and each reviews the other's task.

- **Do:**
  1. In the main line of `Alpha`, ask the secretary for two small tasks: one creates `hello-a.txt` with one line in it, the other creates `hello-b.txt`, one for each AI, each reviewed by the other.
  2. When the brief card appears, read it and press **Confirm & start**.
  3. Open each task's card and its thread. Watch the bar **Work**, **Review**, **Done**.
  4. Read the report message in each thread, and the review block under it.
  5. Look in `F1` and `F2` for the files, and on each computer run `git status` and `git branch` if the folder is a Git project.
- **See:** the brief is the only thing you confirm; no approval card appears. Each AI works in its own folder and posts a **report** in the task thread that says what it changed, what it ran and how it ended, and where the work is (a branch and commit, or "in this folder only"). The other AI reviews it and posts a score; a score at or above the pass score ends with **Passed review** and `T-n passed review and is done`, and the card is **Done** at once. The review says where it read the work from; if the assignee was on another computer and the project shares nothing, it says it cannot reach it, which is the known limit and a pass of this step only if it then asks the assignee (write which). SoloDawn made no branch (none named `solodawn/…`), no worktree and no merge; any branch you see is one the AI made by the project's own rules. The secretary posts a summary.
- **Write:** `A5.1` with the two summaries, `A5.4` (if a review sent a task back, that it went to **Work** and the AI got the review; otherwise write that none was seen, CI covers it), `A5.21` (what the review says about where the work was).

### 7. A second task waits for its folder (`A5.20`, `A5.24`)

- **Do:**
  1. Ask the secretary for two tasks for the same AI, the one in `F1`, so that the second cannot start until the first is done.
  2. Look at the second task's card while the first works. Do the same for the AI you added to `F1` in step 5, which shares the folder.
  3. Let the first task finish and be reviewed.
- **See:** the second task shows **Queued** and says it **waits for its folder, which T-n is using**; no AI starts it. When the first task is done the second starts by itself, in the same folder; with three tasks (ask for three to see it) only one is released at a time. For the AI that shares the folder, a task of its own waits in the same way. A task for an AI in another folder starts at once.
- **Write:** `A5.20`; for `A5.24` write whether the second task's first turn started before the first AI had ended its turn (it must not).

### 8. Pause and reassign: who did it (`A5.23`)

- **Do:**
  1. Give a task that takes a minute or two. While it is **Working**, open its menu (the `⋯` of the card) and press **Pause**.
  2. Read the card. Press **Resume**.
  3. Ask the secretary in the main line to pause the task, and read the card again; resume it.
  4. Open the menu, press **Reassign**, choose the AI in the other folder as who does it, read what the dialog says, and confirm.
  5. Look in the old AI's folder.
- **See:** after item 1 the card says `<your name> paused the task. Its AIs stay stopped until it is resumed.` After item 3 it names the secretary instead. In item 4 the dialog says `<AI> stops here. What it changed stays in its folder.`; the task goes to the new AI and starts in its folder. If a request card of that task was waiting, it closes and reads `<name> gave the task to <AI>; this request is closed.` (cards are rare at level 3: skip when none). In item 5 whatever the old AI changed is still there: SoloDawn removes nothing (a known limit, [issues.md](../../issues.md)).
- **Write:** `A5.23`, and what was left in the old folder.

### 9. A person by link (`A9.1`, `A9.2`)

`Q` has a computer `Q1` that is paired and online, and their own browser.

- **Do:** In `Alpha`, press **Invite**, then **Create link**. `Q` opens the link and presses **Join group** with a name. Open the same link a second time, then once more after it expires.
- **See:** `Q` joins as a member and both browsers show the group and its history with no reload. A used or expired link is refused.
- **Write:** `A9.1`, `A9.2`.

### 10. People you already know, added with no link (`A9.46`)

- **Do:**
  1. Press **New group** and name it `Beta`, with home `P1`. In **People**, `Q` is listed, ticked. Tick one AI too, with folder `F3`, and press **Create group**.
  2. Make a third group `Gamma` with no people ticked and no AI. Open its **Invite** dialog and look under **In your other groups here**. Press **Add** next to `Q`.
  3. Ask who is not listed: someone who has never been in a group of yours on this computer.
  4. In `Beta`, have `Q` press **Add AI** for an AI of theirs on `Q1`.
- **See:** `Q`'s connected browser shows `Beta` and then `Gamma` at once, with no link and no reload; a browser of `Q` that connects later lists them too. A person who is in no group of yours on that computer is not listed: only the link works. Adding someone already in the group, or with a name in use, says so (**Already in the group, or the name is taken.**). `Q`'s AI joins `Beta` with its own folder on `Q1`, from `Q`'s browser.
- **Write:** `A9.46`.

### 11. Take over an AI (`A7.1`, `A7.2`, `A7.3`, `A7.7`, `A7.8`)

Use real `claude` and `codex` members, on the home network (direct). The relay and watching are step 16.

- **Do:**
  1. Take a Claude Code AI that has a task: press **Open terminal** on it. Ask it to edit a file in its folder, resize the window, then press **Hand back**. Mention the AI in the task's thread.
  2. The same with Codex. On Windows exit the CLI inside the terminal and watch the last output lines.
  3. Take an AI that has **no** task.
  4. While a terminal is open, mention the AI from another browser tab.
  5. Open the secretary's terminal and ask it to edit a project file.
- **See:** the terminal opens in the AI's own folder and says that what you change stays there and that, when you hand back, the AI writes in the thread what was done. The terminal works and resizes; hand-back posts the AI's own account of what it did (no list of files or commits is added), and in the thread the AI remembers the exchange. For an AI with no task the terminal opens on its group session, says so (**no task is created**), no task card appears, and the account is posted in the main line. A mention during the takeover is held and delivered, in order, after hand-back. Secretary: record whether a file changed; its session runs at level 3 and is held only by its instructions ([issues.md](../../issues.md), "Group and review sessions default to permission level 3").
- **Write:** `A7.1`, `A7.2`, `A7.3`, `A7.7`, `A7.8` (changed or not).

## Part 2: what needs real networks, phones and accounts

### 12. macOS Application Firewall (`install.md`)

Run on macOS right after step 2, with Firewall on (System Settings, Network, Firewall). The node already runs as the login service.

- **Do:** In System Settings, Network, Firewall, Options, remove `solodawn` and any Go test binary (`*.test`) if listed: a development or pre-identity build of `solodawn` shares their identity, so macOS would apply their entry without asking. Restart the node: `launchctl bootout "gui/$(id -u)/org.solodawn.node"`, then `launchctl bootstrap "gui/$(id -u)" "$HOME/Library/LaunchAgents/org.solodawn.node.plist"`. When the system asks whether to accept incoming connections: **Pass 1**, deny. From a browser on another computer of the LAN, open the page and read **Computers**; also run `solodawn status`. **Pass 2**, allow `solodawn` in Firewall Options and reload the browser page (a relayed connection does not move to direct by itself; a browser's own ICE may do it sooner, so note which browser).
- **See:** macOS asks about `solodawn` by name (`codesign -dv "$(command -v solodawn)"` shows `Identifier=org.solodawn.node`). Pass 1: signalling works and the connection is relayed (**Relayed · limited**, `relay`). Pass 2: after the reload the connection is **Direct** (`direct`).
- **Write:** `A1.3` (direct on a LAN) and the firewall note: the times of both passes, the browser.

### 13. Logs and the Windows archive ([diagnostics.md](../../planning/contracts/diagnostics.md))

- **Do:**
  1. Open the log folder: macOS `~/Library/Application Support/solodawn/logs`; Linux `${XDG_DATA_HOME:-$HOME/.local/share}/solodawn/logs`; Windows `%LocalAppData%\solodawn\logs`.
  2. macOS and Linux, node running: run the archive line of `install.md` ("After a failure").
  3. Windows: first run the archive line with the node **running** and note whether it works (`diagnostics.md` section 6 does not know); then stop the node as `install.md` says (the `Stop-ScheduledTask` block, or Ctrl+C) and run it again.
  4. Write the word `debug` into `logs/level`, restart the node with the stop and start commands of `install.md` ("Update"), then delete the file.
- **See:** every archive command that `install.md` gives exits `0` and holds `node.log` (and `node.log.1` once it passed 10 MiB): UTF-8, UTC microsecond timestamps, `node=<8 characters>` on each line, no message text, file content, token or key. The first line after the restart (item 4) is `node.start` with `level=debug`.
- **Write:** the archive result for each system, and for Windows whether the running-node archive passed or failed (with the error).

### 14. Browser pairs and Web Push (`A8.5`, `A8.3`, `A9.40`)

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
  4. One-person group (`A9.40`): in a group with only you, run a brief, a task and a review as in step 6. In dark and light, English and Chinese. Then a message that appears in both browsers of the pair with no reload (`A9.41`); the invitation link is step 9.
  5. On desktop Chrome, Android Chrome and iOS Safari added to the Home Screen: allow notifications (`A8.3`), close the page, leave the node running, make something need you: ask an AI of the group to reply with a mention of you, read the notification and tap it.
- **See:** no upgrade notice on a supported browser; the second tab shows **SoloDawn is open in another tab.** and **Use here** moves the connection; no `confirm` card in the one-person group; no layout break. The closed-page notification for the mention reads `<group> · Mentioned you` with the name of who mentioned you as its text, never message text or a command, and tapping it opens that message. (The other kinds, `Approval`, `Confirm start`, `Confirm`, `Your call` and `Task done`, add `who: <one-line title>`.) On iOS the push service accepts the node's VAPID `sub` claim ([issues.md](../../issues.md), Web Push).
- **Write:** `A8.5` one line per pair (node OS, browser and version, OS version); `A8.3` per device; `A9.40`; `A9.41` per pair.

### 15. CLI model lists and selected models (`A10.13` to `A10.16`)

- **Do:**
  1. In each real CLI (`claude`, `codex`, `opencode models --verbose`; `qwen` has no list) note the models and effort levels of your account.
  2. In **Add AI** (for `claude`, `codex`, `opencode`) read **Model**, **Thinking level**, **Work share** and **Fast mode**; for `qwen` expect two free-text fields.
  3. Add two members of one CLI on one node with different **Model** and **Thinking level** (each in a folder of its own), mention each, and read the model and effort in the CLI's own session record (`A10.15`).
  4. In **Edit profile** set a member's **Work share** and **Skills**, press **Refresh the models**, then restart the node (`A10.14`, `A10.16`). Change its **Model** in **Edit profile**; set one that does not exist (for example `invalid-model-check`), mention the member, then restore it.
  5. With a key for `opencode` and `qwen`: run `A3.1` (two mentions, the second shows it remembers, one process), `A3.4` (idle for the idle time, 15 minutes by default, then resume), and `A3.6` (kill the CLI mid-turn, the next mention works). No takeover for them. Also read the folder list for each (step 4, `A3.11`): a CLI that was never used lists nothing, and you type the folder.
  6. Sign-in failure: sign out of `claude` (then `codex`), mention that member, write what the browser shows, and attach the sanitized CLI output; sign in again. Do it once per CLI.
- **See:** the pickers match each CLI's own list; refresh adds models with no restart and never changes **Work share**, **Skills**, **Model** or **Thinking level** (a removed model shows **Not listed**); each member runs its chosen model; the bad model ends as `turn_interrupted` (`crash`), keeps the session id and sends no `session_replaced`; the sign-in failure shows **`<AI>`'s sign-in is no longer valid.** in the thread and mentions you; a crash (`turn_interrupted`) or a lost session instead is a fail: write it and attach the output.
- **Write:** `A10.13` to `A10.16` per CLI and version; for `opencode` and `qwen` the version, what differed from the documented API in [issues.md](../../issues.md), the three sessions of item 5, and the sign-in failure texts of item 6 (they close the two `issues.md` entries on `LoginInvalid` matching and the missing invalid-login captures).

### 16. Watching a terminal and the relay (`A7.5`, `A7.9`, `A9.20`, `A9.21`)

Use real `claude` and `codex` members. `O` is the owner's browser; `W` is a teammate's browser (`Q`) in the same group.

- **Do, home network (direct):** While `O` has a terminal open, `W` presses **Ask to watch**. On `O`'s terminal page the consent card appears: `O` presses **Confirm**. The **Watch** page opens by itself once `O` has confirmed. Then `O` presses **Stop watching**, and once more end it with **Hand back** (`A9.20`).
- **Do, company network (relay):** put `O` or the node behind the restricted network. Read **Computers**, send chat, answer a card, look at **Open terminal** and **Watch**. Then open a terminal and a watch on the home network and switch to the restricted one mid-session, also on Windows (`A7.5`, `A7.9`, `A9.21`).
- **See:** `W` sees **Watching · receive only**, cannot type or resize, loses the stream on **Stop watching** and on hand-back, and gets no output before consent. Relay: chat and cards work, **Open terminal** says **Direct link only**, **Watch** is disabled and the node answers `relay`. After the switch the terminal shows **Handed back · switched to relay** only if the same connection moved to a relayed pair; if the network change dropped the connection and it reconnected, it shows plain **Handed back** (write which one). `W` sees **Watching ended**, and `ctl` keeps answering with no re-pairing.
- **Write:** `A7.5`, `A7.9`, `A9.20`, `A9.21`; attach, for a failure, the `ice.pair`, `ice.state`, `ctl.open` and `conn.close` lines of both logs and the browser's `chrome://webrtc-internals` pair dump.

### 17. Live CLI checks, review across computers, rendezvous (`A3.8`, `A4.1`, `A4.11`, `A5.21`, `A10.4`, `A1.4`, `A8.6`)

1. **Repository hooks (`A3.8`, `A10.4`).** In a test Git project (`F2` will do) add a hook that writes a marker file. Run one group mention, one task turn and one review turn on each real CLI. The hook must fire in task and review sessions and not in group sessions (Claude `--setting-sources`, Codex `project_doc_max_bytes = 0`). For Qwen Code record four results: group and task session, each with the hook unapproved and approved (approve it by starting `qwen` once in the folder), and whether `QWEN.md` was read; then decide the `@pending` scenario of `features/s3_drivers.feature` ([issues.md](../../issues.md), "A Qwen Code group session may load the repository's configuration"). **Write:** `A3.8`, `A10.4` per CLI.
2. **Questions (`A4.1`, `A4.11`).** Claude Code: ask it for a multiple-choice question (`AskUserQuestion`); a `decision` card must reach the browser and the answer must reach the CLI. Codex with `-c features.default_mode_request_user_input=true`: see whether it sends `item/tool/requestUserInput`; if the installed release has no such tool, see that it asks through `request_decision` ([drivers.md](../../planning/contracts/drivers.md) 1.2). **Write:** `A4.1`, `A4.11` with CLI versions.
3. **Review across computers (`A5.21`).** Make the folder of the reviewer on `P1` a clone of the same scratch repository as `F2` on `P2`. Give a task to the AI in `F2`, reviewed by the AI on `P1`. **See:** the project has no rules for publishing, so the assignee commits and pushes to a branch of its own, not to `main`, before the review; its report names the branch and commit; the reviewer on the other computer fetches that branch and reads it without changing what its own folder holds, and the review passes. SoloDawn merges nothing: after the pass the secretary asks the assignee, or a person, for the closing step (the merge). Then repeat with two plain folders and no shared remote: the reviewer says it cannot see the work and asks the assignee, and does not score what it has not read (the known limit, [issues.md](../../issues.md)). **Write:** `A5.21` for both cases.
4. **Rendezvous and footprint (`A1.4`, `A8.6`).** With a direct connection open and answering `host.info`, stop the rendezvous on the server (`sudo systemctl stop solodawn-rendezvous`), wait 10 minutes, read `host.info` again, and start it again (`A1.4`). Record for `A8.6`: CLI versions, node binary size (target 30 MB or less), node idle memory (40 MB or less) and peak memory per open session of each CLI, with the machine named. **Write:** `A1.4`, `A8.6`.

### 18. External AI: setup prompt and scheduled jobs (`A10.28`, [external.md](../../planning/contracts/step-3/external.md))

Run with a real external MCP agent (for example Cursor or ZCode) on Linux, macOS and Windows nodes.

#### 18a. Self-setup

- **Do:** **Add AI**, choose **External AI**, pick an account type, choose its folder, submit. Open the member's **Setup** panel, copy the **Self-setup prompt**, paste it unedited into the agent on that computer, watch the four marks (`Runner seen`, `Item taken`, `Member answered`, `Run ended`), and press **Send setup check again** once at the end.
- **See:** with no help the agent adds the `solodawn` stdio MCP server (`command: <exe>`, `args: ["mcp", "--member", "<member-id>"]`), writes and starts a runner, answers `Setup check: answer here in one line, then finish.` once, all four marks turn **Complete**, the member is `Waiting`, and no AI process stays running while idle.
- **Write:** `A10.28` per system and agent version.

#### 18b. Scheduled runners (`solodawn wait --member <id> --timeout 0` once a minute)

Run each case: idle tick (exit `3`), a normal mention answered with the MCP `post` and `solodawn done --member <id> --run <run>`, a turn longer than 65 seconds, a failed exit (`solodawn done ... --failed "exit <code>"`).

- **Linux (`cron` with `flock`):** `* * * * * flock -n /tmp/solodawn-<member>.lock /bin/sh /path/to/runner.sh`, with absolute paths (cron's `PATH` is minimal). **See:** `jq` and `<exe>` are found; an idle tick spawns nothing; the next minute starts no second agent during the long turn; the member never goes `Offline` (presence holds 180 seconds); a failed exit records `turn_interrupted` (`crash`).
- **macOS (`launchd`):** `~/Library/LaunchAgents/solodawn.<member>.plist` with `Label`, `ProgramArguments` `["/bin/sh", "/path/to/runner.sh"]`, `StartInterval` 60 and a `PATH`; `launchctl bootstrap gui/$(id -u) <plist>`; at the end `launchctl bootout gui/$(id -u)/solodawn.<member>`. **See:** the same.
- **Windows (`schtasks`):** `runner.ps1` sets UTF-8 output, calls `solodawn.exe wait`, returns when `$LASTEXITCODE -ne 0`, reads the JSON, `Push-Location $i.folder`, and ends with `solodawn.exe done`; register it with `schtasks /Create /TN "SoloDawn <member>" /SC MINUTE /MO 1 /TR "powershell -NoProfile -ExecutionPolicy Bypass -File C:\path\to\runner.ps1"`, remove it with `schtasks /Delete /TN "SoloDawn <member>" /F`. Put non-ASCII text in a test mention. **See:** the text arrives without mojibake or a BOM, `$LASTEXITCODE` picks success or `--failed`, no overlap, `done` returns the member to `Waiting`.
- **Write:** `A10.28` per scheduler.

### 19. Two people, the rest (`A9.3` to `A9.5`, `A9.8` to `A9.19`, `A9.22`, `A9.39`, `A9.43`, `A9.44`, `A9.4`)

Cast: `P` owns `P1` (the group's home) and `P2`; `Q` owns `Q1` and has two AIs on it, `Q-personal` (account type personal) and `Q-team` (account type team, on a different CLI). `P` is the owner of the group, `Q` joined as a member (step 9). Use real CLIs and the group `Alpha`.

- **Do:**
  1. `P` presses **Make admin** on `Q`, `Q` edits the group settings; **Make member**, `Q` tries again (`A9.3`).
  2. `Q` presses **Add AI** on `Q1` for `Q-personal` and `Q-team`, each with a folder on `Q1` (`A9.5`).
  3. `P` mentions `Q-personal` (`A9.8`); `Q` mentions it too. `Q` denies one request and ignores another (`A9.9`). `P` mentions `Q-team`.
  4. `Q` presses **Edit access** on `Q-team`, adds `P` under **Allow list**, saves. `P` mentions `Q-team` and confirms a brief for it. Then `Q` removes `P` and `P` mentions again (`A9.14`). The same list on `Q-personal` must change nothing.
  5. `P` confirms a brief that gives `Q-personal` one task and the review of another (`A9.11`). `P` cancels the task and confirms a new brief (`A9.13`).
  6. `P`'s AI on `P2` asks `Q-personal` through its `ask` tool; `Q` denies that card once and confirms it the next time; then it asks `Q-team` (`A9.18`).
  7. `Q` adds `P` under **Approval delegates** of `Q-team`; make `Q-team` need an approval; `P` approves; check that the command ran. `Q` removes `P`; answer an older card as `P` (`A9.19`).
  8. `P` makes `Q` admin and confirms a brief that names only `Q`'s AIs. `Q` presses **Reassign** on a task to an AI of `P` on `P2` (`A9.43`).
  9. In **Group info** choose `Q-team` as the Secretary; ask it to write a brief (`A9.44`). Read the warning under **Secretary** with two people and `Q-personal` as secretary, then with one person and with an account type other than personal (`A9.39`).
  10. `P` makes a group with `Q` and no AI and posts in its main line; `Q` reads it, and a person who joins it later reads its history (`A9.22`).
  11. On `P1`, `P` presses **Remove** on `Q` while an AI of `Q` holds an unfinished task, then after it is done (`A9.4`).
- **See:** both browsers show the group and its history with no reload. Admin `Q` can edit settings and invite; as member `Q` is refused. Only a mention by `P` of `Q-personal` makes a `Confirm` card for `Q`; nothing runs before `Q` confirms from either of two devices, a refusal or silence runs nothing, and `P` sees the card's state. A listed `P` runs `Q-team` with no card; after removal a card again. The `Q-personal` list shows that it has no effect on a personal account. A brief gives `Q` cards for both roles; after the cancel and new brief a new card. The `ask` to `Q-personal` waits for `Q`, a denial reaches the AI as a turn, and `Q-team` answers with no card. The delegate approves and the command runs; after removal the old card is refused. `Q` as admin: the reassign makes a card for `P`; the task starts on `P2` after `P` confirms. The secretary is `Q-team`. The warning appears only with two people and a personal secretary. The group with no AI has a main line and a member list. The first removal is refused (`conflict`), the second one works and `Q`'s browser no longer lists the group.
- **Write:** `A9.3` to `A9.5`, `A9.8`, `A9.9`, `A9.11`, `A9.13`, `A9.14`, `A9.18`, `A9.19`, `A9.22`, `A9.39`, `A9.43`, `A9.44`, `A9.4`, `A9.41` (the pairs of step 14 for the two-person run).

### 20. Moving a group

`A9.27`, `A9.28`, [export.md](../../planning/contracts/step-2/export.md). Group `G1` is the group `Alpha` of step 19: `P1` is its home, `P2` is another computer of `P`, and `Q` has joined, with `Q1`. `G2` is a second group whose home is `P1`.

- **Do:**
  1. Create `G2` on `P1` as in step 4, let `Q` join it as in step 9, and add a message.
  2. Back up `G2` on `P1` as `install.md` ("Back up a group") says: on Linux or macOS a `crontab` line with `* * * * *` instead of `0 3 * * *`, on Windows the `Register-ScheduledTask` block and then `Start-ScheduledTask -TaskName SoloDawnBackup`. Wait for the file, then remove the schedule at once, so that this file is the last copy.
  3. Move `G1` (`A9.27`): in **Group info** of `G1` open **Move group**, **Export and import**; it shows the commands with the group and the device ids filled in. On `P1` run `solodawn group export <group id> <file> --to <P2 device id>` from it. This stops `P1` being the home at once, and `G1` is unavailable until the import finishes. Copy the file to `P2` and run `solodawn group import <file>` there. Try the import on `Q1`, and the export of `G1` on `P1` again (now not its home).
  4. Restore `G2` (`A9.28`): write one more message to `G2`, then stop `P1`'s node (the commands of `install.md`, "Update") and do not log in again on `P1` or reboot it until the end of this step. Copy the backup file to `P2` and run `solodawn group import <backup file>` there. `A9.28` deletes `P1`'s data directory; a stopped `P1` cannot answer either, so nothing is moved or deleted here.
  5. `P` sends an invitation to `G2` from `P2`; `Q` opens it.
  6. Start `P1`'s node again. From now on `G2` may be shown from `P1`'s old copy or from `P2`, whichever home answers first ([issues.md](../../issues.md), "Two homes that both hold a group"). No command deletes a group in version 1, so treat `G2` as a used scratch group and do not use it again.
- **See:** after step 3 both browsers and `Q1` follow `P2` by their normal retry with no reload; the next task of `G1` has the next number; the import on `Q1` and the export on a non-home node fail; the group info shows the two commands to `P` only. The AIs keep their folders on their own computers; the export holds rows only, and nothing of a folder moves. After step 4 `P2` is the home of `G2` with the rows of the backup, without the last message. After step 5 `Q` opens the link, is still listed once under People, and follows `P2`.
- **Write:** `A9.27`, `A9.28`.

## Verify

1. Every id above has a line in the report.
2. `logs/node.log` of every node has no unexplained error line.
3. Remove what you made: cron, launchd and scheduler jobs, scratch repositories, test hooks and the folders `F1` to `F3` (scratch groups stay: version 1 cannot delete a group).

## Failure reports

Attach: the id, the OS, browser, CLI and release versions, the UTC time, the steps, `solodawn status`, the log archive of step 13 (`node.log` and `node.log.1`), the browser console, and for CLI failures the CLI's version and sanitized output. Add or update the entry in [issues.md](../../issues.md), with no usernames, host names or private paths.

If a direct path fails on macOS, look at System Settings, Network, Firewall, Options first. If a scheduled job works by hand but not under `cron`, `launchd` or `schtasks`, look at the scheduler's `PATH` (`jq`, `solodawn`, the AI's binary) and file permissions, and attach its stderr. If a CLI's behaviour differs from its fake CLI, attach the exact version and the sanitized protocol exchange.

## Sentences with no real-machine step

| Sentence | Why |
| --- | --- |
| `A1.3` "after unblocking, direct within one probe interval plus 5 s"; `A1.13`; first sentence of `A2.7` | Removed: no relay-free probe and move. |
| `A1.4` second sentence | Removed: no beacon, `PR-2` cancelled. |
| `A5.3`, `A5.9`, `A5.10`, `A5.13`, `A5.17`, `A9.23` to `A9.26` | Removed on 2026-11-26 with the Git, check, forge and team-group parts: the node pushes, checks and merges nothing. |
| `A9.7`, `A9.10`, `A9.15`, `A9.37`, `A9.38`, `A9.45`; `A9.16`, `A9.17`; `A9.29` to `A9.36` | Removed in the acceptance text. |
| `A4.6` | Removed by the owner's rule of 2026-11-21 (no defence in code). |
| `A4.5` | Contradicted by [drivers.md](../../planning/contracts/drivers.md) 1.1: at level 3 the CLIs still ask and it becomes a card; step 17.2 shows the question cards. |
| `A8.2` self-update | Not in version 1; step 1 is the update by hand. |
| `A8.5` on Firefox | CI, Firefox 136, every pull request. |
| `A4.1` at levels 1 and 2, `A4.4`, `A4.10`, the level 1 checks of OpenCode and Qwen Code | Need a permission level other than 3. Version 1 has no control for it (`perm.set` is postponed, [commands.md](../../planning/contracts/commands.md) section 2); only a row in the node's database sets it. A coordinator's agent does it on a test node (needs `sqlite3`) and runs these with the real CLIs. |
| `A5.22`, `A5.24`, the second half of `A2.9` (two joins at once) | Need a secret pattern in a summary or the exact timing of two turns and two joins; CI runs them with fake CLIs. |
| `A8.6`, rendezvous memory with 100 idle connections | The repository has no tool that makes the connections. A coordinator's agent runs it with a load tool. |
| Exhausted usage limit of a CLI | Cannot be made on demand; attach the sanitized output when one happens ([issues.md](../../issues.md), "Fake CLIs lack invalid-login and exhausted-limit captures"). |
| `A9.12`, `A9.42`, the header of `A9.13` | The header is text the node writes; the fake CLI logs it and a real CLI adds nothing. |
| `A10.4` for OpenCode and Qwen Code | Their tables have no level 2 and no takeover: only `A3.8` and level 1 apply (step 17.1 and the level row above). |
| Qwen Code and OpenCode MCP connection | Not asked, left as it is ([issues.md](../../issues.md), 2026-11-26). |
| Windows `AF_UNIX` shim EOF | Accepted limitation ([issues.md](../../issues.md), 2026-11-21). |

## References

- [install.md](../../install.md) — install, update, backup, failure archive
- [self-hosting.md](self-hosting.md) — the rendezvous and its checks
- [issues.md](../../issues.md) — known limits and unverified items
- [2026-11-26-groups-workspaces.md](../decisions/proposed/feature/2026-11-26-groups-workspaces.md) — the model: one kind of group, connected AIs and their folders
- [diagnostics.md](../../planning/contracts/diagnostics.md) — log paths, events, archive lines
- [drivers.md](../../planning/contracts/drivers.md) and [step-3/drivers.md](../../planning/contracts/step-3/drivers.md) — CLI drivers, levels, models
- [step-3/profiles.md](../../planning/contracts/step-3/profiles.md) and [step-3/external.md](../../planning/contracts/step-3/external.md) — profiles, external AI
- [step-2/watch-team.md](../../planning/contracts/step-2/watch-team.md) and [step-2/export.md](../../planning/contracts/step-2/export.md) — watching, export and import
- [step-1-acceptance.md](../../planning/steps/step-1-acceptance.md), [step-2-3-acceptance.md](../../planning/steps/step-2-3-acceptance.md), [step-3-acceptance.md](../../planning/steps/step-3-acceptance.md) — the acceptance tests
- [parallel-plan-units.md](../../planning/steps/parallel-plan-units.md) — I-11
