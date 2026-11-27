# Issues

The issue record of this project: bugs, deviations from a requirement or from the owner's expectation, and known limitations, open and resolved. Read it before implementing or fixing anything. Add an entry for every such finding, not for every step; update the entry when it is resolved, and keep resolved entries.

Entry format: `### YYYY-MM-DD · type · title`, where type is `bug`, `deviation` (a feature does not do what was required or expected) or `limitation` (known, accepted for now). Then the symptom, the cause with its location (`file:line` where it is code), and the status (open, or resolved with the version or commit). Link a lesson, decision or postmortem instead of repeating it.

## Open

### 2026-11-27 · bug · In the demo, a person removed right after joining still said hello

- Symptom: in `?demo`, removing Nora within 2.5 s of her joining by code left 「我进来了，有需要我的地方 @ 我。」 by her under 「Alex 把 Nora 移出了群。」, a post by someone no longer in the group (`shots/m3/demo-d4-panel-expire.png` of the first M3-8 run).
- Cause: the scripted hellos are posted by a timer that never looked whether the person was still a member (`web/features/sim/people.ts`, `#noraJoins`, `added`, `#leoJoins` and `#direct`'s thanks, each `this.world.after(…, () => this.post(…))`). The client shows rows in the order the home wrote them, which was right.
- Status: resolved on `m3/web`: `#sayLater` (`web/features/sim/people.ts:80-84`) posts only for a member; test "says nothing for a person removed before their hello" in `web/features/sim/sim.test.ts`, which fails on the old code.

### 2026-11-27 · bug · After joining in the demo, the top bar said 「重连中」 until a reload

- Symptom: in `?demo`, after joining a group by code the connection indicator stayed 「重连中」.
- Cause: the demo's `FakePeer` told its path once, as it came online (`web/features/sim/session.ts`, `#open`). A join starts following the path only when it is admitted, after that one report, and closing the replaced peer had cleared the path. `peer.ts` reads the browser's statistics again, so a real connection recovers.
- Status: resolved on `m3/web`: the demo peer tells its path again a second later (`web/features/sim/session.ts:94`); the indicator reads 「直连」 again in the click-through.

### 2026-11-27 · bug · In the demo, joining a group of one's own computer said that not every computer was told

- Symptom: in `?demo`, typing the open invitation code of 官网改版 on 加入群 ended in 「你已加入群聊，但并非所有已连接的电脑都收到了通知。」 instead of opening the group.
- Cause: the join dials the group's home again, which replaces the browser's connection to that computer (`hub.peer`). In peer.ts a closed peer reports its connection lost (`Peer.close` → `#end` → `#offline`, `web/src/lib/peer.ts:604-614`), so the client stops asking that home anything on it; the demo's `FakePeer.close` (`web/features/sim/session.ts`, before M3-8) only closed its line. The home kept its `ctl`, `ownNodes` listed it, and the `home.add` sent on the closed line failed as `lost`.
- Status: resolved in unit M3-8: `FakePeer.close` reports the loss as peer.ts does (`web/features/sim/session.ts:137-143`); the click-through opens the group.

### 2026-11-27 · deviation · The demo answered `join` of a member with `conflict`

- Symptom: in `?demo`, a person who opened a link of a group they are in was told they are already in it; `contracts/step-2/people.md` 2.2 point 3 answers such a `join` `ok` and writes nothing (the restore case), which a typed code of one's own group also needs (A9.52).
- Cause: `People.join` in `web/features/sim/people.ts` returned `conflict` for a member.
- Status: resolved in unit M3-8: a member is answered `ok`.

### 2026-11-27 · bug · The demo world wrote a second document when a preset or knowledge-base document was saved again

- Symptom: in `?demo`, saving a style preset or a knowledge-base document under a name that already existed added a second entry of that name instead of replacing the first, which `planning/contracts/step-3/knowledge.md` section 2 requires.
- Cause: the demo's `note` handler (`web/features/sim/director.ts`, `#note`, before M2) wrote every `note` under the command's new `id` and never looked up the note of that type and name.
- Status: resolved by unit M2: `notePut` in `web/features/sim/memory.ts` replaces the note of that type and name, keeping its id and place (`features/sim/sim.test.ts`, "writes, edits, searches and pages the project memory and the knowledge base").

### 2026-11-27 · limitation · With the nightfall theme colour, links read as secondary text

- Symptom: under 夜幕 Nightfall, links and the "更改" actions of the side panel are a neutral grey close to `--text-2` (`web/src/styles/accents.css`, `nightfall` block).
- Cause: a neutral accent has no hue to set it apart, which is what the person chooses it for; the contrast test still holds the accent 3:1 off the hairline and the inset fill, so the focus ring and chosen borders stay seen (`web/features/a11y.spec.ts`).
- Status: accepted (M4, `web/design/README.md` "Theme colours"); links keep their underline on hover.

### 2026-11-27 · limitation · Some theme colours sit near the colour of a result or of the waiting signal

- Symptom: the status dots of "AI working" (filled accent circle) and "passed" (filled green circle) differ by colour, the working pulse and the text label beside them (`web/src/styles/base.css`, `.st`). Measured hue distances: 浅湾 Shallows 17° (light) and 15° (dark) from the green of a passed result, 深潭 Deep pool 39° and 45°, 林荫 Tree shade 48°; 朝霞 Morning glow 26° and 24° from the red of a failure; the magenta waiting colour of 晨曦 First light 40° and 41° from that red (the default amber is 31° and 30°).
- Cause: nine accents across the hue circle cannot all stay far from green, red and the waiting colour; the a11y test holds only the waiting colour 45° from the accent (`web/features/a11y.spec.ts`).
- Status: accepted (M4 review, `web/design/README.md` "Theme colours"): the text label and the state glyph carry the state, as the design requires ("state is never carried by colour alone").

### 2026-11-27 · deviation · The tier read as a thinking setting, the quota was hard to find, and new groups took no model

- Symptom: the owner, clicking the demo, read "档位: 轻量 / 标准 / 强" in "编辑档案" as a thinking setting, could not find the AI's quota, found no thinking level in that editor, and could choose an AI's model and level only after the group was made.
- Cause: the tier was worded as a strength although the secretary uses it to share out work (`internal/host/instructions/secretary.md:18`); the usage meter sat at the bottom of the card, apart from the tier; model and level had their own "更改" editor, and `web/src/components/model/ModelFields.svelte` hid the level whenever the chosen model listed none, which includes "the CLI's default", every new AI's model; `web/src/components/groups/CreateGroup.svelte` sent `newDraft()` for every AI.
- Status: resolved on branch `m/ai-settings`: the tier is worded "派活: 少 / 适中 / 多" ("Work share: Less / Normal / More") in the client, the secretary's instructions and the contracts (stored values unchanged); it sits first on the card in one block with the usage line; "编辑档案" edits model, thinking level, work share and skills; the level is always shown, with its reason when there is none to choose; the create-group dialog gives each ticked AI model, level and work share, sent in its `host.join`.

### 2026-11-27 · bug · The demo panel refused a typed or pasted pairing code on the first try

- Symptom: in `?demo=fresh`, a reviewer typed the onboarding page's code into the demo panel's pairing field and pressed **交给 MacBook Pro**; the panel said 「这不是配对码」. The same code paired on the second try. Reproduced on 47ae57c1e in 3 of 3 runs that click into the field and then type or paste the code (one of them after clearing the field and clicking on the page for 3 s); 0 of 4 runs that hand over the prefilled field as it is or select it before typing.
- Cause: every render of the panel wrote the page's code into the field (`web/features/sim/panel.ts:113` on 47ae57c1e, `input.value = codeOnPage()`), and an open panel renders every 2 s while it is neither hovered nor focused (`panel.ts:55-57`). A click into the field puts the caret after that code, so typing or pasting the code made 38 characters, which `parseSecret` refuses (`panel.ts:131-133`). The refusal renders the panel again, which puts the bare code back, so the second try hands it over. `codeOnPage` also read the panel's own text, so once the page showed no code it took the example code of the panel's message. Demo only: the real client has no field for a code (the browser creates the secret, or reads it from a join link in `web/src/lib/join.ts:48`), and a real node reads the code once from `solodawn pair <code>` (`internal/node/node.go:758`, `internal/node/pair.go:212`).
- Status: resolved on branch `fix/pairing-first-try`: the field holds only what the person typed, kept across renders and emptied once the computer heard the code; the page's code is the placeholder and is handed over from an empty field; `codeOnPage` reads the page outside the panel (`web/features/sim/panel.ts:31-37,50-53,118-147`). Test: `web/features/sim/panel.test.ts`.

### 2026-11-27 · bug · Four OS-specific functions failed the CRAP limit on any other system

- Symptom: `gate.py --stage merge` on macOS failed the crap gate on `internal/node/service_linux.go:25 InstallService` (CRAP 20, CC 4), `:47 RemoveService` (12, CC 3), `internal/node/socket_windows.go:30 privateDir` (20, CC 4) and `internal/rendezvous/truncated_windows.go:15 readDatagram` (12, CC 3), each "no coverage data for this file".
- Cause: a file that does not build on the host has no coverage data, and `.project-standard/scripts/crap.py` (`measure`) counts it as 0%; at 0% only CC 2 or less passes the limit of 6. CI runs these files' tests on Linux and Windows, but the limit is enforced locally. Found on the way: a copy of `.project-standard/` into a worktree also copies `state/` of the main checkout, whose session file and `base_commit` make `crap.py` measure against another session's base; remove the copied `state/` before measuring a worktree.
- Status: resolved by the commit "Node, rendezvous: OS-specific functions are shims; their decisions are portable and tested everywhere": the branching is in `installServiceFile` and `removeServiceFile` (`internal/node/service.go`), `createDir` (`internal/node/socket.go`) and `readSkipping` (`internal/rendezvous/truncated.go`), at 100% on every system; the OS functions are CC 1. Code in a file built for one system keeps CC 2 or less.

### 2026-11-27 · bug · `TestNodeWatchGrantAdmission` reported the relay refusal as a missing field

- Symptom: on a Mac whose firewall drops direct UDP to the test binary, both subtests failed at `internal/node/node_test.go:1283` with `proto: a field that is always present is missing`, on `rebuild` 47ae57c1e and equally before the protocol 3 change (b051eafef).
- Cause: the connection was relayed, so the node answered `watch.start` with `{"re":2,"err":{"code":"relay","msg":"watch requires a direct path"}}` (`internal/host/watch.go:95`), as `planning/contracts/step-2/watch-team.md` section 4 requires. `checkWatchStart` decoded that error answer as a `proto.Success`, whose `ok` is always present. The product was right; the test hid the answer.
- Status: resolved. `checkWatchStart` now fails with the error answer's code and text (`watch.start answered relay: watch requires a direct path`). The test also no longer starts a loopback TURN server (`startTestRVWithRelay`), which it never needed: it checks admission by grant, the `relay` refusal is covered by `internal/host/watch_test.go`, and with a TURN server ICE could nominate a relay pair first on a loaded runner. It now uses the plain rendezvous of the other node tests, so ICE has no relay candidate, and a host without a direct UDP path fails at the WebRTC connection's time-out. Should the node still answer `relay` (a path that is relayed, or not yet selected), the test now names that refusal instead of a missing field.

### 2026-11-27 · limitation · `takeover.go:210` (`== ""` to `!= ""`) decided a value nobody reads

- Symptom: CONDITIONALS_NEGATION at `internal/host/takeover.go:210:58` lived in mutation run 45905182817 (shard host-s-takeover).
- Cause: `target.kind` is read only by `createSession` (`internal/host/deliver.go:1312,1315`), which runs only when `Sessions.Get(target.key)` misses (`prepareTargetSession`, `deliver.go:1276-1278`). With no task the key is the group session's, and its absence returns `not_found` at `deliver.go:1306-1309` before `kind` is read; with a task, `lastTask` has just read that task session from `Sessions.All()`, so it exists. Only a session ended between those two calls on another goroutine could tell the mutant apart.
- Status: resolved by the commits "Mutation: tests kill the batch's survivors; guards and a takeover branch that could never matter go" and "CI: every product file is mutated; node shards stay within the runner's memory": `takeoverTarget` sets only the task from `lastTask`. The one difference left is under a race, a group session that appears between `followMember` and `createSession`: `createSession` then also runs `cleanupTaskSessions`, which ends only sessions of done, cancelled or reassigned tasks, before `Create` refuses the existing key.

### 2026-11-27 · bug · The node-admit-local mutation shard ran the runner out of memory

- Symptom: `go node-admit-local` of mutation run 45905182817 failed after 9 minutes with the annotation "Out of memory."; its log was never uploaded. A rerun of the same commit passed (126 killed, 13 timed out, the rest on the filter list).
- Cause: gremlins ran four mutants at once (`--workers 8`), each test binary capped at 4 GiB by `ulimit -v`. The shard's timed-out mutants come one after another (seven `local.go` mutants in a row), so four runaways at 4 GiB fill the runner's 16 GiB.
- Status: resolved by the same commit: the shard is split into node-admit-dir and node-export-local, and those and the other node shards whose timed-out mutants come in runs (node-node, node-pair, node-socket-update) take `--workers 4`, two mutants at once and at most 8 GiB (`.github/workflows/mutation.yml`, matrix key `workers`).

### 2026-11-27 · limitation · Equivalent web mutants of the model batch, and a gremlins blind spot

- Symptom: Stryker survivors that no test can kill: `groups.svelte.ts:143` (`id === undefined` guard; `member(undefined)` and `#former.get(undefined)` find nothing), `members.ts:254,381` (the `[]` of a missing group is only read through `m.host` and `p.user`), `nodes.svelte.ts:110` (`group.home !== node`: `infos` holds owner homes only, `#read` via `activeCtl`, and `unheard` keeps groups of non-owner homes), `taskops.ts:66` (`other !== task`: a `busy` wait is only on a queued task, which `hasSession` excludes, `planning/contracts/rows.md` section 4) and `taskops.ts:88` (`task.wait?.by`: the manual hold exists only when `task.wait.kind` is `manual`). gremlins reported `internal/home/cmd_task.go:418:99` NOT COVERED: Go's cover tool ends a block at a function literal, so the condition after it in an `if` init had no block.
- Status: resolved by the commit "Mutation: tests kill the batch's survivors; guards and a takeover branch that could never matter go": the guards are gone with no change of behaviour, and the `IndexFunc` call is its own statement (as `internal/node/schedule.go` in b4e9fda48).

### 2026-11-27 · bug · Four files of the model batch belonged to no mutation shard

- Symptom: mutation run 45905182817 failed "Files in exactly one shard": `internal/home/cmd_summary.go`, `internal/host/tools_workspaces.go`, `internal/host/tree_unix.go` and `tree_windows.go` were never mutated.
- Cause: the `own` regexes of `.github/workflows/mutation.yml` did not match their names.
- Status: resolved by the commit "CI: every product file is mutated; node shards stay within the runner's memory" (home-cmd-task-ev `^(cmd_[st]|e)`, host-mcp-merge `^(mc|me|tr|tools_w)`). Their mutants are first seen in the next run.

### 2026-11-27 · deviation · The onboarding prompt still tells the AI to check the Git identity

- Symptom: the prompt that **Add a computer** gives for an AI CLI has a step "Check `git config --global user.email`; if it is empty, ask me for a name and an email and set them". `install.md` no longer has that step, and SoloDawn commits nothing and needs no Git.
- Cause: the texts `pairing.prompt.text_rv` of `web/src/messages/pairing.en.json:15` and `pairing.zh-CN.json:15` were not changed with the model, and `web/src/lib/onboard.test.ts:242-253` and `web/e2e/connect.spec.ts:577` assert the line.
- Status: resolved by this commit: the prompt now has three steps and no `git config` check; the three tests assert the new steps. Found while rewriting the real-machine checklist (unit DOC-1).

### 2026-11-27 · limitation · An AI works in its owner's own folder, with no isolation

- Symptom: an AI runs at permission level 3 in the folder it was given, which is often the folder its owner uses. Uncommitted work of the owner is within its reach, and every session of the AI in the group (group, task and review) sees the same files. Before the model change a task had a worktree of its own.
- Cause: the folder is the AI's workspace and nothing is copied (`planning/contracts/local.md` section 7; plan `MODEL-1126/plan.md` risk 1). The prompt forbids stash, reset, clean, `git add -A`, `git commit -a`, checking out over files, rebase, switching branches and a force push in the folder (`internal/host/instructions/task.md:19`); that is an instruction, not a barrier. The node reads and changes no Git state, so it cannot tell whether the owner's changes are in the way.
- Status: open, accepted by the coordinator's decision of 2026-11-26 ([decision](docs/decisions/proposed/feature/2026-11-26-groups-workspaces.md)). `install.md` and the release notes tell the owner to commit or set aside their own changes before giving a folder to an AI.

### 2026-11-27 · limitation · Half-finished work stays in the folder after a failure, a cancel or a reassignment

- Symptom: a task that fails, is cancelled or is given to another AI leaves whatever its AI changed in the folder, and the next task in that folder starts on top of it. A task whose AI pushed to a branch of its own leaves the folder on that branch.
- Cause: the node removes nothing when a task ends: there is no worktree to throw away (plan `MODEL-1126/plan.md` risk 2). The home frees the folder when the task row changes (`planning/contracts/task-machine.md`), not when the old AI's turn ends. The prompts ask the AI to call `submit_task` last and to end its turn (`internal/host/instructions/task.md`), and the secretary to ask an AI what state it leaves in its folder before it cancels or reassigns a task (`internal/host/instructions/secretary.md`).
- Status: open, accepted. The owner or the secretary has to decide what to do with what is left.

### 2026-11-27 · limitation · Review across computers depends on the project's own way of sharing work

- Symptom: a reviewer on another computer than the assignee's can read the work only if the project shares it: with Git, a remote and a pushed branch that the report names. With no shared remote the reviewer cannot see the work and says so in the thread.
- Cause: SoloDawn carries text only (R-111) and moves no file; the review turn says where the work is and which of three remarks applies (plan `MODEL-1126/plan.md` 1.6 and risk 4). The assignee is told to push to a branch of its own when its reviewer is on another computer, and the secretary to choose a reviewer on the assignee's computer when the project has no shared remote (`internal/host/instructions/task.md:21`, `secretary.md:22`).
- Status: open, accepted.

### 2026-11-27 · limitation · What was reviewed and what is delivered are no longer guaranteed the same

- Symptom: a review names a report, and a passed review makes the task `done` at once. Nothing ties the report to a commit, nothing stops a change after the review, and a closing step such as a merge is arranged by the secretary or a person. The two checks on a task's diff that stopped a submission, the scan for secrets and the check for a change without tests, are gone: only what an AI sends to the group is scanned, and a missing test is caught by the reviewer or not at all.
- Cause: the node runs no Git, so it has no commit to review, check or merge (plan `MODEL-1126/plan.md` 1.6, 1.8 and risks 5 and 6). `review.report` is the id of the assignee's own statement (`planning/contracts/rows.md`).
- Status: open, accepted by the coordinator's decision of 2026-11-26. The review instructions still make a fake or missing test a veto (`internal/host/instructions/review.md`).

### 2026-11-27 · limitation · A9.12 failed once with `home_unreachable`: P1's ICE agent stood in a UDP write

- Symptom: a full acceptance run on this Mac at a 1-minute load of about 2 failed A9.12 in `Given the confirmation cast with Q` with `join Q-personal: ... home_unreachable`. Repeats on `batch/model-1126` 591d32812: A9.12 alone 469 times (6 to 10 processes at a time, load 12 to 22, 49 of them under `-race`), no `home_unreachable`; the scenarios of `s2_confirm.feature` up to A9.19, 160 processes of 14 casts, 8 at a time beside 8 busy loops (load 14 to 20): one, `host.join` of Q-personal on Q1 in A9.13 answered after 15,003 ms. The other 4,462 joins of Q-personal and P-work to P1 took 6 to 664 ms, median 615.
- Cause: the case of the entry "On a Mac with the Application Firewall, a UDP write can block every connection on one local address". Q1 offered 4 ms after `home.add`; P1 answered, both exchanged candidates within 10 ms and went to ICE `connecting`, and neither logged anything more in the 15 s. P1's goroutine dump holds that connection's agent loop in a write to `127.0.0.1` that waits for the socket (`sendBindingRequest` → pion ice v4.4.8 `udp_mux.go:337` → `WriteToInet4`). An answered attempt is left to ICE (`flows/connect.md` step 15), which fails it after 30 s, so Q1 offers again after `host.join` has stopped waiting for the home (`joinAttachTimeout`, `internal/host/join.go:19`; `joinWaitHome`, `join.go:340-350`). The step and the product do what their contracts say: the cast sends `host.join` after `home.add` answers, and `host.join` waits for the attachment. Two `timed out after 3m0s waiting for ctl` failures of A9.19 in the same runs had the same write blocked in P1 (under `sendBindingSuccess` and `sendBindingRequest`).
- Status: open, accepted for the local machine with the parent entry: CI decides; nothing changed.

### 2026-11-27 · bug · A node of the step-2 cast sometimes exits at start: its own rendezvous answers 400 or 404

- Symptom: in the same repeats, 9 of about 2,700 casts failed in `Given the confirmation cast with Q` because P1 or Q1 never opened its socket; the node's stderr held only `failed to WebSocket dial: expected handshake response status code 101 but got 400` (or `404`). Also seen in the stress of FLAKY-33 above.
- Cause: when port 47800 is taken, `listenNodeHTTP` fell back to `:0`, and macOS could hand out a port that another program held on `127.0.0.1`; the node then dialled it (`internal/node/node.go:430` and `:308` before the fix) and ended on that error.
- Fix: the fallback picks a port that is free on `127.0.0.1` and listens on all addresses with it, picking again (up to 3 times) when that port is in use.
- Status: resolved.

### 2026-11-27 · limitation · Acceptance scenarios of the old model fail until units AC-1 to AC-3 rewrite them

- Symptom: after unit AC-0 of MODEL-1126 `features/` compiles and its fixtures follow the new model (every AI joins with a workspace folder of its own, no bare repository, no fake `gh` or forge program, hello and welcome at version 3), but scenarios of the old model fail. The steps whose bodies the model took away fail on purpose with `rewritten by AC-<n>: <reason>` (`rewritten`, `features/helpers_test.go`); others fail because their expectations name the old model: step names `branch`, `local_check` and `merge` (`home_task`, `home_machine`, `home_review`), the roster line without the folder (`host_turn`, `host_profile`, `a10_profiles`, `a10_external`), the review checkout in the turn text (`host_turn.feature:2`), and a second task of one AI that now waits as `busy` (`m4.feature:33`, `m4.feature:48`, `m6.feature:26-39`, `s3_drivers.feature:20`).
- Cause: plan `MODEL-1126/plan.md` section 4 gives the rewrite of these scenarios to AC-1 (M3 to M8), AC-2 (host and node) and AC-3 (home, step 2, step 3); the list by unit is `MODEL-1126/ac0-failing.md`.
- Status: resolved on `batch/model-1126`: AC-1 to AC-3 rewrote the scenarios (commits `c038a82e9`, `b0fb4fbe6`, `591d32812`), and no step calls `rewritten` any more.

### 2026-11-26 · bug · A task's menu stays open after one of its items is picked

- Symptom: in headless Chromium (Playwright 1.64, a trusted click) on the demo world at `?demo`, opening the `⋯` menu of a running task and pressing 暂停, 换人 or 取消任务 carries the action out but leaves the popover open over the card (`.menu:popover-open` still matches, and no `toggle` event follows the click). The next press on `⋯` then closes it instead of opening it. Seen while taking the screenshots of the redesign; it is the same with the menu's styles as they were before the redesign (`web/src/components/task/TaskMenu.svelte` at fe89a231b and at 51aee6673).
- Cause: the items closed the menu through `popovertarget` + `popovertargetaction="hide"`, which the browser runs as the button's activation behaviour, after the click listeners and the microtask checkpoint that follows them, and skips when the pressed button is no longer connected or is disabled. The demo answers a request through promises alone, so for 暂停 and 恢复 the task's new row arrived inside that checkpoint, the keyed `{#each ops as op (op)}` (`web/src/components/task/TaskMenu.svelte:90`) replaced the pressed item, and the hide never ran (reproduced 2026-11-26 in headless Chromium at 1440: no `toggle` event, a MutationObserver saw the item replaced and `isConnected` false before the activation). 换人 and 取消任务 closed normally in that run, since they only open a question. A real home answers in a later task, which is why the e2e of M5 did not see it; the menu depended on that timing.
- Fix: each item hides the menu itself before it hands on the pick (`hide()` at `web/src/components/task/TaskMenu.svelte:99`); the two attributes are gone. The `pick` helper of `web/features/taskops.spec.ts:46` now expects the menu to hide on every pick; with the old menu it fails in 9 steps. After the fix all four items close the menu in the same Chromium probe at 1440 and 390.
- Status: resolved on `web/redesign` (commit `web: redesign review fixes`).

### 2026-11-26 · deviation · Rough edges of the web client seen while walking every flow of the demo

- Symptom: walking every flow of `?demo` (`web/features/sim`, branch `web/demo-sim`) in the built-in browser showed, in the client itself:
  1. OpenCode and Qwen Code show as `opencode` and `qwen` in Add AI, on the Computers page and in onboarding, and Add AI suggests the name `qwen`: `cliLabels` knows Claude Code and Codex only (`web/src/lib/onboard.svelte.ts:37`).
  2. A refused join says only `无法加入（join：conflict）` (`web/src/components/people/JoinForm.svelte:56`, `people.join.failure`).
  3. After a person is removed, their earlier messages and the `member_joined` line name "已离开的成员"; removing a person takes no confirmation (`web/src/components/people/PersonActions.svelte:50`).
  4. An approval made void by the owner's own reassignment says the secretary reassigned it (`approval.hint.reassigned`); a task the owner paused says the secretary or an admin paused it (`task.hold.manual`).
  5. A takeover of an idle AI shows the host's task title `Owner takeover` in English (flows/takeover.md A names it).
  6. A watcher's terminal page is titled "终端接管".
  7. The group list previews a task message as "系统：任务" and a system row as "系统：通知".
  8. Onboarding's "创建第一个项目群" opens the group list without the create dialog.
  9. The composer of a group without a secretary still suggests "发需求给 @秘书".
  10. The connection indicator stays "直连" while one of two computers is down (`indicator` summarises online homes only, `web/src/lib/nodes.svelte.ts`).
  11. A team bulletin reads "来自Alex的电脑" without a space.
- Cause (lines as of `ea96a5ce7`): `cliLabels` knew two CLIs (`web/src/lib/onboard.svelte.ts:37`); `JoinForm.svelte:56` showed `people.join.failure` for every code; display names came from `group.member(id)` (`web/src/lib/groups.svelte.ts:131`), which forgets a removed row, and `PersonActions.svelte:50` removed at the first press; the texts `approval.hint.reassigned` (`approval.zh-CN.json:18`) and `task.hold.manual` (`task.en.json:63`, `task.zh-CN.json:63`); the title is the host's (`internal/host/takeover.go:261`, `planning/flows/takeover.md` A); `App.svelte:44` titled every terminal page `page.terminal`; `GroupList.svelte:29` used `subject`, the text of the row, empty for a task or a notice; `StatusPanel.svelte:179` only linked to `#/groups`; the composer hint ignored `group.secretary` (`Composer.svelte:50`); `indicator` summarised the connected homes only (`web/src/lib/nodes.svelte.ts:50`); `team.computer` had no spaces (`team.zh-CN.json:7`).
- Status: resolved on `fix/demo-findings`; the actor of a pause or reassignment is the separate open entry "No row says who paused or reassigned a task, and a removed member's name lasts only while the page is open".

### 2026-11-26 · limitation · No row says who paused or reassigned a task, and a removed member's name lasts only while the page is open

- Symptom: a task paused by hand says only that it was paused by hand, and an approval made void by a reassignment only that the task was reassigned, even when the reader did it; before 2026-11-26 the Chinese texts named the secretary. A member removed before this page was opened is shown as "Former member" on what they wrote.
- Cause: the wait `manual` of a task row and the approval state `reassigned` carry no actor (`planning/contracts/rows.md` section 4, `planning/contracts/messages.md` 3.2; `waitFor`, `internal/home/machine.go:362`, and `tskCardRows`, `internal/home/cmd_task.go:349`), and a member's `gone` marker carries no name (`rows.md`, `gone`). The web client keeps the last row it held of each member that left (`Group.nameOf`, `web/src/lib/groups.svelte.ts:142`), and a browser cache lives in memory (`planning/flows/sync.md` G).
- Status: the first half is resolved by the product-model change MODEL-1126 ([decision](docs/decisions/proposed/feature/2026-11-26-groups-workspaces.md)): the wait `manual` carries `wait.by` (`internal/home/machine.go:589`) and the approval state `reassigned` carries `by` (`tskCardRows`, `internal/home/cmd_task.go:386-396`), and the client says who paused or reassigned. The second half stays open, accepted: a member removed before the page was opened is shown as "Former member".

### 2026-11-26 · bug · A9.27 fails in its set-up and connections when six copies run at once on a Mac

- Symptom: 60 runs of the unchanged A9.27 on this Mac, six processes in parallel (1-minute load up to about 17): 40 passed, 20 failed before or after the export, never with `group rows differ`: 10 × `timed out after 16s waiting for ctl connection`, 7 × `home_unreachable` from `JoinAI` in the step "the group has a secretary on P1, P-work on P2 ...", 2 × the browser peers following P2 in 5.9 and 6.2 s (want 5), 1 × `EOF`, 1 × `timed out after 3m0s waiting for ctl`.
- Cause: not looked into. The connection time-outs fit the entry "On a Mac with the Application Firewall, a UDP write can block every connection on one local address"; the `home_unreachable` ones may be the class of `features/s2_watch_test.go` `prepareBulletin` (a command sent before P2's host attached to P1).
- Status: open; found while reproducing the entry "A9.27 failed once on Linux: P2 held a later member row than the export".

### 2026-11-26 · limitation · A hand-back or continue turn whose CLI must start in a folder that is gone is not reported

- Symptom: the host checks a session's folder where delivery prepares a turn, where an external hand-out is made and at `takeover.start` (`prepareTargetSession`, `internal/host/deliver.go`). A structured start that no item causes is not checked: the hand-back turn after a takeover, a continue turn of a session that went dormant, the replacement of a session the CLI could not resume. There the CLI fails to start, the session goes `dormant` and the error is logged, with no `turn_interrupted` and no `task fail`. The check also runs before every turn, not only before a start: a session whose CLI is still open gets no turn in a folder that is gone, and stays `waiting`.
- Cause: `planning/contracts/local.md` section 7 puts the check "before a CLI starts". Unit HS-A put it where delivery already records the outcome of a batch (delivered, acknowledged, failure reported), which is one place for managed AIs, external hand-outs and takeovers. The runtime's own starts (`startFromDormant`, `internal/host/run.go`) report no failed start of any kind.
- Status: resolved by the review of HS-A: the check is in `startFromDormant` (`internal/host/run.go`), the one place every start of the runtime passes, and reports the crash there; delivery no longer checks before a turn, so a session whose CLI is still open gets its turn. `TestMissingFolderFailsTheStart` (`internal/host/run_test.go`) covers a turn for items, a continue turn and a hand-back.

### 2026-11-26 · deviation · Between HS-A and HS-B: a Codex group session that has never run cannot be taken over, and the instructions name removed tools

- Symptom: `takeover.start` for a working AI without an unfinished task opens its group session and creates no task. For a CLI whose takeover is `resume` (Codex) and a group session that has never run, the start answers `cli` (`codex: interactive takeover requires an existing thread id`). `internal/host/instructions/*.md` and the hand-back sentence of `ownTurnBody` (`internal/host/turn.go`) still name `push_branch`, `run_checks`, the review diff and "run the checks".
- Cause: unit HS-A removed the takeover task, its structured `assign` turn and the Git facts (`internal/host/takeover.go`) only as far as package `host` needed to compile. The structured first turn of a group session, the turn texts and the instructions are unit HS-B's.
- Status: resolved by unit HS-B: a session that has never run of a `resume` CLI gets one `TakeoverTurn` in structured mode before the terminal opens (`readyForTerminal`, `internal/host/takeover.go`), and the instructions and the hand-back sentence name no removed tool or check.

### 2026-11-26 · bug (test) · The owed-turn line of `delivery.forget` was covered by a race

- Symptom: with the merge steps gone from `afterGroupState`, `go test -cover ./internal/host/` reported `forget` (`internal/host/deliver.go`) at 85.7%: the line that drops the owed turns of a forgotten group did not run.
- Cause: `TestGroupItsOwnerWasRemovedFromIsLeft` opens a pre-emption and then lets two background goroutines race: `afterGroupState`, whose sweep drops a pre-emption that has not ended (`dropUninterrupted`), and `leaveGroup`, which calls `forget`. Whichever came first removed the owed turn, and the test accepts both.
- Status: resolved. `TestGroupWhoseHomeRefusesItIsLeft` calls `forget` with a turn owed in the group and one owed in another group, and checks that the first alone is dropped.

### 2026-11-26 · bug · FLAKY-33: A9.8 failed once: a browser's ICE agent stood still for 5 × 7.8 s

- Symptom: CI run 45727940554 (`acceptance-race`, Linux, PR #417), A9.8 `Given the confirmation cast with Q`: Q's test browser and P1 exchanged the offer, the answer and all candidates in 35 ms; no pair was selected; P1 gave up at 30 s (`ice_failed`), then the browser. The browser's pion ICE agent loop was blocked five times for 7.8 s, the time a pion TURN request takes to give up (retransmissions at 0.2 ... 7.8 s, `pion/turn/v5@v5.1.3/internal/client/transaction.go`). Not reproduced in 434 runs of the outline (Mac `-race`, Linux in colima, Linux under CPU stress).
- Cause: not proven. pion leaves a TURN client's CreatePermission unanswered in two ways only, and one unanswered permission blocks the whole ICE agent for 7.8 s per peer address, because the relay candidate's `WriteTo` makes the permission on the agent loop (`pion/ice/v4@v4.4.8/agent.go:668-695`, `candidatepair.go:130`, `pion/turn/v5 internal/client/udp_conn.go:166-221,364-388`):
  1. The server has no allocation for the client (`pion/turn/v5 internal/server/turn.go:368-375`). Nothing in the scenario closed one; no cause known.
  2. The client's read loop ended: `Client.Listen` (`pion/turn/v5 client.go:286-312`) exits for good at any error of `HandleInbound`, and one STUN request from anyone at the client's socket is such an error (`client.go:679-760`). Every later request of that client then times out. `TestStrayRequestAtTURNClient` (`internal/peer/relay_test.go`) showed it with our own sockets: one Binding request at each end's TURN client socket, and the relayed connection never came up (`turnc ... unexpected STUN request message ... Exiting loop`). A product node behind a relay would have lost its relay path for good from one stray datagram. No sender of such a request was found in the CI run.
- Status: open until a failure with the new diagnostics names the trigger. Cause 2 is removed on `fix/turn-client`: the socket of a TURN client over UDP, the only socket pion ICE opens with `ListenPacket`, passes on datagrams from the address it last sent to, its TURN server, and drops the rest as `turn.stray` (`turnNet`, `serverOnly`, `internal/peer/sockets.go`). Nothing else legitimately reaches that socket: a peer's relayed data comes through the server, as pion's own client already requires for data and channel data (`isTURNServer`), and host and server-reflexive candidates have their own sockets (the UDP mux of `Listen`, or pion's `ListenUDP`). The acceptance harness now writes pion's debug lines of the test TURN server (`turn`), the TURN clients (`turnc`) and the ICE agents (`ice`) of the test process into `nodes.log` (`peer.PionLog`, `features/diag_test.go`): cause 1 shows as `Failed to handle datagram: ... no allocation found`, a read loop that ends as `Exiting loop`. The child nodes' pion lines are not captured. The block of the whole agent per unanswered permission stays: it is pion's.
- Stress on this Mac, `-race`, 10 processes in parallel with busy loops, 1-minute load 13 to 27: the outline 165 times with the change, 115 times without it. No `turn.stray`, no TURN request that timed out, no TURN read loop that ended before its close. 4 runs with the change (all at load 21 to 27) and 1 without it (load about 20) failed in `Q opens a second confirmation browser` with `not ready after 8s` twice: the browser's agent was silent for the 8 seconds and a UDP write of it to a peer address ended only at the close (`use of closed network connection`), the form of the entry "On a Mac with the Application Firewall, a UDP write can block every connection on one local address". 1 run with the change failed in the cast's set-up: Q1's node got 404 from its rendezvous and never came online (`failed to WebSocket dial: expected handshake response status code 101 but got 404`); not looked into, nothing in it touches TURN or ICE.

### 2026-11-26 · deviation · The Computers page says a relayed node switches back to direct by itself

- Symptom: a relayed computer shows "The node keeps trying a direct connection and switches back automatically." (`nodes.relay.body`, `web/src/messages/nodes.en.json:23`, `nodes.zh-CN.json:23`, shown by `web/src/routes/Nodes.svelte:190`). No code tries: the relay-free probe and move were cut on 2026-11-21 ([connect](planning/flows/connect.md) B).
- Cause: the text predates the cut. A relayed connection becomes direct only when ICE finds a direct pair itself; a reconnect runs ICE again and stays relayed if direct is still blocked.
- Status: resolved on 2026-11-26: `nodes.relay.body` (en and zh) now says the connection may turn direct after a reconnect or if the browser finds a direct path itself, and otherwise stays relayed. No test or e2e spec carries the text.

### 2026-11-26 · limitation · A task handed to an AI of another node can start without the last commits

- Symptom: the new assignee's node creates its worktree from the published branch while the old assignee's node still pushes its last commits. The new AI's own push is then refused, and `push_branch` or `submit_task` fetches the branch under the clone's lock and tells it to merge `origin/<br>`, run the checks and push again (`planning/contracts/mcp-tools.md`). If the old node is off when the task moves, its unpushed commits stay on it: it pushes them when it comes back, and if the new AI has pushed by then, that push is refused, told once in the task's thread, and the commits stay on the old node's local branch.
- Cause: the home sends the task row to one node and the `assign` item to the other at the same time, and no message says that a branch has been published (`planning/flows/task.md`, "Reassignment": two nodes do not wait for each other).
- Status: ended by the product-model change MODEL-1126 ([decision](docs/decisions/proposed/feature/2026-11-26-groups-workspaces.md)): a task has no branch and no worktree, so nothing is published or created from a branch when it is reassigned. The new assignee's instructions tell it to find where the earlier work is before it starts (`internal/host/instructions/task.md`); what the old AI left in its own folder is the new entry "Half-finished work stays in the folder after a failure, a cancel or a reassignment".

### 2026-11-26 · limitation · A push holds the clone's lock, its hook included

- Symptom: while an AI's `push_branch` or `submit_task` pushes, every other Git command of the group's clone waits: the fetch before a dormant group session's turn, a new task or review worktree, a local merge, a branch deletion. A project `pre-push` hook that runs the tests makes that minutes, up to the push's 60-minute limit.
- Cause: a push moves `refs/remotes/origin/<br>` of the clone, which a concurrent fetch also locks ("cannot lock ref"), so it runs under the clone's lock (`PushBranch`, `internal/host/forge.go`; `planning/contracts/local.md` section 7). A push to `origin`'s push URL instead of its name was tried: it left the ref alone with Git 2.54.0 but updated it with Git 2.56.1 on CI.
- Status: ended by the product-model change MODEL-1126 ([decision](docs/decisions/proposed/feature/2026-11-26-groups-workspaces.md)): the node makes no clone and runs no Git push; an AI that pushes does it in its own folder by the project's rules.

### 2026-11-26 · limitation · A node that missed its owner's removal and shares no other group with that home keeps the group

- Symptom: a node that is off when its owner is removed keeps the group's remembered home and its AIs' dormant sessions, and offers that home a connection once a minute.
- Cause: the home drops the offers of an unknown device without an answer (`planning/contracts/datachannel.md` section 8), as for a home that is gone for good.
- Status: open, accepted: nothing wakes those sessions. `host.leave` needs the home, too.

### 2026-11-26 · bug · After the person scrolls away from a card fetched alone, a page below it can move the view

- Symptom: after a jump to a card fetched alone (`Group.seek`), the person scrolls towards the newer messages, which ends the hold (`web/src/lib/jump.ts`); the next page of the gap moved what they read by about 1400 px in Chromium (`web/e2e/jump.spec.ts`, "lets the card go", checks only that the card is not put back). The same happens on `origin/rebuild` without the hold. Whether it happens depends on whether a row above the gap, the card or one before it, is still in view, not on reduced motion.
- Cause: the page of the gap lands inside the view, below the card, while Chromium's scroll anchoring keeps the card, the row above it, in place; what the person reads below it moves down by the page's height.
- Status: open, accepted for 1.0. A fix needs `Group` to expose the gap, so that the line can tell where a page lands. Not measured in Firefox or WebKit.

### 2026-11-26 · limitation · An entry stored before a stop can be missing from the groups' rows

- Symptom: a node stores an accepted device-log entry and stops before its groups have taken the `device` event. Those groups have no row for the entry; for a member they refuse that device, and every later entry of that person (`errGap`), until the group is moved. Only a move repairs it. The owner's devices are still accepted, since the group takes the owner's log from the node.
- Cause: `dvlReceive` (`internal/node/devlog.go`) writes `device_log` in one transaction and the groups write their rows in their own, later (`devEvent`, `internal/home/ev_device.go`); nothing compares the two at start. Found by reading.
- Status: open. A pass at `StartLoop` that writes what `LackingEntries` (`internal/store/home_reads.go`) returns for every group would close it, also for groups founded before 2026-11-26; it changes the sequence numbers that many home tests expect.

### 2026-11-25 · limitation · `group set` has no range for its numbers

- Symptom: a home stores any integer as `pass`, `pass_foundation` or a limit, also 0 and negative ones. Some have an effect nobody chose: `turn_minutes` 0 ends every check command at once.
- Cause: `planning/contracts/commands.md` 1.1 names no range and `grpSetValid` (`internal/home/cmd_group.go`) checks only that a field is there.
- Status: open, accepted: only an admin sends it. The web client's settings take 1 to 100 for the scores and at least 1 for a limit (0 for `review_rounds` and `ask_depth`, where it means something: a failed review stops the task for a person, AIs do not ask each other), at most 4 for `ask_depth` (R-114), 55 seconds for `ask_wait_s` (`flows/ask.md` D) and 10 minutes for `approval_wait_s` (`flows/approval.md` H) (`limitFields`, `web/src/lib/groupset.ts`).

### 2026-11-26 · limitation · A home that never answers stays in the group list as away

- Symptom: the group of a home that is gone for good, or that ignores this browser, stays in the list as away, with no way to remove it; the home is offered a connection once a minute for as long as the browser runs.
- Cause: the homes this browser remembers (`joined`, `web/src/lib/net.svelte.ts`) and a node's `host.info.homes` (`planning/contracts/step-2/people.md` section 7) are never pruned, and a home drops the offers of an unknown device without an answer (`planning/contracts/datachannel.md` section 8).
- Status: open, accepted for 1.0.

### 2026-11-26 · limitation · Two homes that both hold a group: the first to be reached shows it

- Symptom: a lost home that returns after its group was restored elsewhere lists the group too. The browser shows one copy, that of the home it reached first, so a reload can show the other; when the copy shown is deleted, the other appears at its home's next list.
- Cause: nothing in `welcome.groups` says which copy is the newer (`planning/contracts/step-2/export.md` section 4 speaks of a lost home); `Home.#list` (`web/src/lib/groups.svelte.ts`) leaves a group to the home within reach that listed it first.
- Status: open, accepted: version 1 has no command or button that deletes a group, so a returned old home's copy stays; the owner treats it as a used scratch group (`docs/cookbook/i11-real-machine-checklist.md` step 20).

### 2026-11-26 · bug · The fixtures of the confirmation scenarios did not match the contracts

- Symptom: the first run of A9.8 to A9.19 and A9.42 to A9.44 failed in A9.9, A9.11, A9.42 and A9.44 with nothing wrong in the product.
- Cause, in `features/s2_confirm*`: `state.by` of a card was expected to be a user id, it is a member id (`contracts/messages.md` section 3); a scripted tool without arguments sent `"arguments": null`, which the host refuses; a `choose` had an empty `arg`; the group had `forge: none`, which is one machine only, so a reviewer on another node could not fetch the commit; the scripted results of a Claude Code tool call were not compared; the one-person quiet-task scenario needed the secretary's tools ("A changed secretary gets no secretary tools on its host").
- Status: resolved in the change that enables the scenarios. The cast's group now has the fake forge of A9.24.

### 2026-11-26 · bug · FLAKY-32: A5.15 hung once: the fake Claude's MCP shim never connected

- Symptom: one run of A5.15 with forge `none` (`features/m5.feature`) waited for the secretary's brief until it timed out; the MCP shim of the fake Claude never reached the bridge.
- Cause: not proven. A listener that ended silently fits ("FLAKY-32, found on the way: an accept error ended the node's listeners" below; `acceptMCPLoop` had the same form), and nothing would have shown it: the fixture hid the accept error and the fake CLI dropped the shim's stderr.
- Status: open, not reproduced: about 1000 runs under `-race` with those diagnostics, 6 to 14 in parallel (665 of the scenario as it is here, 340 of the `none` example of `acc/m5-remote`; 1-minute load 17 to 44), without a failure; the slowest of the first 300 took 2.8 s. `acceptMCPLoop` (`features/m5_test.go`) now fails the scenario with the accept error and goes on, and the shim's stderr is in `SOLODAWN_FAKECLI_LOG` (`internal/host/fakecli/mcp.go`), which the folder of a failed scenario holds; the next occurrence names its place.

### 2026-11-26 · limitation · OpenCode and Qwen Code are not asked whether the `solodawn` MCP server connected

- Symptom: an AI of these two whose MCP server did not connect would work on without its tools; not observed, neither program is installed here.
- Cause: `internal/host/opencode.go` and `internal/host/qwen.go` read no state of the MCP server, and no contract names one.
- Status: open, left as it is (coordinator, 2026-11-26).

### 2026-11-26 · limitation · macOS may ask about `solodawn` again after each update

- Symptom: after an update the firewall may ask again whether `solodawn` may accept incoming connections, although it was allowed before; not observed yet.
- Cause: the release binaries are signed ad hoc (`.github/workflows/release.yml`), so their designated requirement is their cdhash, which changes with every release; the firewall stores it with the app's entry.
- Status: open, accepted for 1.0 (coordinator, 2026-11-26): `install.md` says to allow it again. Signing with a Developer ID is a later decision of the owner.

### 2026-11-26 · limitation · On a Mac with the Application Firewall, a UDP write can block every connection on one local address

- Symptom: on the development Mac, some runs of A9.2 (five or six WebRTC connections between in-process peers each) failed with `timed out … waiting for ctl … saw 1: connection ended before it was ready`. Two causes were found; they are different failures:
  1. A write that blocks (the goroutine dump below): one connection exchanged offer, answer and candidates, then stayed silent until the other side reported ICE `failed` after 30 seconds. The dump covers the failures of this kind only, 4 of about 120 runs before the fix of cause 2.
  2. The fixture sent every ICE candidate to a joining browser twice (see the entry "The A9.2 fixture sent each signalling frame twice" below): the signalling pacer of `internal/peer/path.go` waited for a turn (A9.2 opens one owner and three joining connections within 1.5 seconds, just over 90 frames in 11 seconds), so the answer came too late and the offer was retired at 10 seconds. No blocked write is in that dump.
- Cause of 1: the goroutine dump shows the agent's task loop inside a UDP write that waits for the socket to become writable (`candidatepair.go:131` → `udp_mux.go:337` of pion ice v4.4.8, the binding response of a host candidate to a `127.0.0.1` address). pion writes on that loop with no deadline, and Go holds the socket's write lock while the write waits (`internal/poll/fd_unix.go`), so every answered connection on that local address is mute, not one agent. The write blocks on this machine only, as far as known; the Application Firewall, which holds packets of unsigned binaries (2026-11-22 below), is the likely reason and was not proved (a two-socket program of 80,000 writes did not block).
- Bounded in the product: nothing closes the stuck side by itself, but the peer's next offer does (`internal/peer/signal.go:185`, `old.Close()`), and pion then aborts the write after a 5 second grace (`udp_mux.go` `scheduleWriteAbort`/`checkWriteAborts`, through `Agent.Close`). `signal.go:185` runs under `p.mu`, so the node's signalling pauses for up to about 5 seconds in that case; no other peer code waits for the agent loop under the lock (`AddRemoteCandidate` runs in its own goroutine). No product change is needed.
- Status: open, accepted for the local machine: CI decides. In the product the attempt is retired and offered again (`flows/connect.md` step 15).

### 2026-11-26 · bug · The A9.2 fixture sent each signalling frame twice

- Symptom: A9.2 waited for a signalling turn in 2 of 2 runs (8.9 s) and failed on this Mac in some runs (cause 2 of the entry above).
- Cause: `peerNode.routesFor` in `features/helpers_test.go` added the node's rendezvous path and then every route of `Pairer.Routes`, which holds the same path, so each frame to a joining browser went twice over one WebSocket. The product skips such a route (`internal/node/node.go` `routesFor`, #373).
- Status: resolved on `s2/lift-people`: the fixture skips a route whose ID is already there. A9.2 then passed 21 of 22 runs under `-race -tags acceptance`; the one failure is cause 1 (its goroutine dump holds the blocked `WriteToInet4` under `sendBindingSuccess`).

### 2026-11-25 · limitation · A paired node's rendezvous addresses cannot be changed

- Symptom: `solodawn pair <secret> --rendezvous <new>` on a node that already has an owner prints "this node already has an owner; to add a browser, run solodawn pair-browser", exits 1 and leaves the saved addresses as they were. Version 1 has no other command that writes them, so a node cannot be moved to another rendezvous.
- Cause: `cmdPair` reads the line with `ReadPair(line, owner, nil)`, which returns `ErrOwned` before any address is read or saved (`internal/node/node.go:710-730`, `internal/node/socket.go:160-162`); `saveRendezvous` is reached only through `preparePairRV` for an unowned node.
- Status: open, found 2026-11-25 in the review of the self-hosting guide. The guide tells the owner to choose a name that they will keep (a domain, not an address) before pairing ([self-hosting](docs/cookbook/self-hosting.md), step 5).

### 2026-11-25 · limitation · The web client works out which messages wait for an AI

- Symptom: a message of one's own can stop saying that it waits although the AI has not taken it: when the AI's row changed in the middle of a turn (its model edited, a status sent), or when the AI's next turn took older items of another session first.
- Cause: no row tells what a home has queued; `waiting` (`web/src/lib/queue.ts`) compares the sequence of the message with that of the AI's row, which changes when a turn starts or ends.
- Status: open, accepted: the message is delivered as before, only the choice to let it go first is gone.

### 2026-11-23 · deviation · An agent without a driver cannot join a group

- Symptom: only Claude Code, Codex, OpenCode and Qwen Code can be AI members. An agent its owner runs outside SoloDawn (a self-built agent, ZCode, Cursor, Cline) cannot join. The owner required on 2026-11-19 that any AI that supports MCP can join a group and work, and that a per-CLI adapter is needed only to wake it and keep it running.
- Cause: only the second tier was planned and built. The MCP shim takes only the session token that the node creates when it starts a CLI itself ([local.md](planning/contracts/local.md), section 3), `host.join` takes only an agent with a driver (`agtValidAgent`, `internal/host/agents.go:258`), and nothing lets an agent take an inbox item.
- Status: open, found 2026-11-23 and planned as the external AI (R-166, [external.md](planning/contracts/step-3/external.md); [parallel plan](planning/steps/parallel-plan-step-2-3.md), section 4.2). The implementation supports joining, setup and member-scoped MCP without a driver. The [browser test](web/e2e/external.spec.ts) and [I3-3 scenarios](features/a10_external.feature) cover waiting, queued work, the owner-only tool label, managed replies, task and review checkouts, usage, and presence expiration. A10.28 needs a live agent. The agent writes its own runner around `solodawn wait`.

### 2026-11-23 · deviation · An intermittent failure cannot be explained from one occurrence

- Symptom: several tests called flaky were product races (A9.24 twice, the A9.23 bulletin attach, the A10.3 pause read, a host teardown race). Each took hours of stress runs to find, and the first Windows A7.9 failure contained no candidate-pair trace. The owner's rule of 2026-11-23: an intermittent failure must not depend on being reproduced; the logs of its one occurrence, in CI or on the owner's machines, must point at the cause.
- Cause: the node logs only errors, in prose (`log.Printf` in `internal/host/deliver.go`, `internal/home/run.go`, `internal/node/session.go`). A wait in a test that times out says only that it did (`features/m4_test.go:70`, "timed out waiting on M4 home frames"). CI keeps nothing from a failed scenario (`.github/workflows/backend.yml`).
- Status: resolved by DG-1 to DG-7 of `planning/steps/parallel-plan-step-2-3.md` section 4.3, of which DG-4 (the node's events, the level file, "After a failure" in `install.md`) was the last. The contract is [diagnostics](planning/contracts/diagnostics.md); its section 6 lists what is not verified, the Windows `tar` line of `install.md` among it (I-11).

### 2026-11-23 · limitation · A required escalation can outlast its last stronger candidate

- Symptom: if the owner lowers the last stronger AI's tier while `stuck.ask` is true, the turn and the host submission refusal say no candidate is currently listed.
- Cause: the task row is authoritative. The profile contract does not clear an existing escalation on profile edits, and the host cannot clear it.
- Status: the host still refuses without pushing. The owner can restore a stronger tier or arrange reassignment.

### 2026-11-26 · limitation · A pre-empted task that is reassigned waits for row 13

- Symptom: a task reassigned while it is `paused` with wait `preempted` stays paused: its old host ends the session, and its new host has none.
- Cause: the host brings back only a task it has a task session for (`adoptPreempted`, `internal/host/deliver.go`).
- Status: open, accepted. The secretary or an admin resumes the task.

### 2026-11-23 · limitation · Qwen Code model and level changes wait for a new session

- Symptom: a changed model or level of a Qwen Code member applies only to its next new session; the serve protocol documents startupConfig only at creation.
- Cause: the [serve protocol](https://qwenlm.github.io/qwen-code-docs/en/developers/qwen-serve-protocol/) documents `startupConfig.modelServiceId` and `startupConfig.reasoningEffort` for `POST /session`, not resume. `qwenProcess.open` in `internal/host/qwen.go:218` sends `{}` on resume. The driver reads no capabilities and adds no capability probe.
- Status: accepted for T3-10. New-session refusals use the crash path; resumes preserve history and their original selection. Real daemon behavior remains unverified.

### 2026-11-23 · deviation · OpenCode model selection uses the documented structured reference

- Symptom: model selection does not send the contract's unverified `provider/model#variant` notation.
- Cause: the [OpenAPI](https://opencode.ai/docs/openapi.json) documents `POST /api/session/{sessionID}/model` with `{"model":{"id":"…","providerID":"…","variant":"…"}}`. `openCodeProcess.selectModel` in `internal/host/opencode.go:245` splits the public `provider/model` id on the first `/`, omits an empty variant, and skips an empty model. Discovery reads `{data: Model.Info[]}`; unknown shapes yield `[]`.
- Status: accepted for T3-10; the coordinator will flag the contract in the PR. Fixtures are synthetic, with no real CLI capture.

### 2026-11-23 · limitation · Remembered project homes without local AIs stay connected

- Symptom: remembering a project home now retains a connection even when no AI is hosted locally in that group.
- Cause: `internal/host/join.go:120-180,420-439` reconciles all remembered homes. The remembered-home wire shape carries no group kind. State-only sync supplies it after connection, as specified in [watch-team.md section 10 ("Which home")](planning/contracts/step-2/watch-team.md#10-a-secretarys-notice-r-14).
- Status: the coordinator accepted keeping the connection on 2026-11-23.

### 2026-11-22 · limitation · Stored AI rows written before profiles are not migrated

- Symptom: syncing an untouched AI row written before profiles may omit the four fields, or carry an empty tier if written after the wire freeze but before the home assigned defaults.
- Cause: home sync sends stored state and deltas without rewriting them (`internal/home/sync.go`). The home treats an empty stored tier as standard for escalation and writes the default on the next member edit.
- Status: open for existing rows. New AI rows always have all four fields, and saved owner edits survive a home restart.

### 2026-11-22 · limitation · Custom forge results depend on the template program

- Symptom: a checks template can report an older request head, and a successful merge can name a later merge on the base branch.
- Cause: `internal/host/forge_tmpl.go:124-147` supplies the submitted `{commit}` but cannot verify its use by the forge program. After merging it fetches `origin` and reads the base tip, as specified in `checks-forge.md` section 3.
- Status: ended by the product-model change MODEL-1126 ([decision](docs/decisions/proposed/feature/2026-11-26-groups-workspaces.md)): forge templates, hosted checks and the merge are removed.

### 2026-11-22 · limitation · Group moves follow replies, not unsolicited pushes

- Symptom: a browser follows `moved` replies to `cmd`, `sync` and `fetch`; it cannot follow an unsolicited move notification. Since 2026-11-26 it reads a group that left its home's list, so that an idle browser gets the reply (below, "An idle browser lost a moved group").
- Cause: the frozen error has a response id and a destination home, but no group id; there is no moved push in `proto.ts`. The export contract specifies request replies, and U2-5 preserves those shapes unchanged.
- Status: response-based following is implemented. Before import, connection failures use the normal peer retry; a connected destination answering `not_found` is read again when import pushes `groups`. Real CLI export/import and multi-machine WebRTC migration remain unverified.

### 2026-11-22 · limitation · Foreign home attribution depends on a device row

- Symptom: a foreign home with no matching device row shows its computer name but says its owner was not provided.
- Cause: `welcome` carries the device name and an owner boolean, not its owner's user id. Group details use a matching live `device` row rather than guessing from a person's group role.
- Status: the home now writes the owner's `device` rows too (2026-11-26), so the row is there for every home whose owner's log names it. Real-host custom forge execution and Windows program resolution are not verified by the browser UI.

### 2026-11-22 · limitation · The Qwen Code driver is declared from documentation, not from a capture

- Symptom: `DriverCaps["qwen"]` declares levels `1` and `3` and approvals `conn`, and `qwenDriver` speaks the `qwen serve` daemon API (`internal/host/qwen.go`), but no run of the real Qwen Code exists: the fake Qwen Code's scripts are written from the documented API, not cut from captures.
- Cause: T3-0 read Qwen Code 0.28.0's documentation only, and no owner QwenCloud key was available to T3-3. The `serve` flags (`--token`, `--workspace`), the routes (`POST /session`, `/session/<id>/{resume,prompt,cancel,permission/<request>,events}`), the event names (`session_update`, `permission_request`, `turn_complete`, `turn_error`), the option ids `proceed_once` and `cancel`, approval modes `plan` and `yolo`, the address line `qwen serve listening on <url> (mode=..., workspace=...)`, `QWEN_CODE_SYSTEM_SETTINGS_PATH`, instructions taken as the first block of the first prompt, and a `401` in a `turn_error` meaning a bad key are all `[documented]`.
- Status: open until the live part of M3, M4 and M7 runs once with the real Qwen Code and an owner key (`drivers.md` section 7); then the fake's scripts are replaced by captures and the declaration corrected.

### 2026-11-22 · limitation · A Qwen Code group session may load the repository's configuration

- Symptom: `contracts/step-3/drivers.md` section 3 requires the repository's configuration to be kept out of group sessions. `qwenDriver` cannot do that: a Qwen Code group session may load the project's `.qwen/settings.json`, `QWEN.md` and extensions (`internal/host/qwen.go`).
- Cause: Qwen Code 0.28.0 documents no switch for all of it. Folder trust is not one: in an untrusted folder Qwen Code asks before every tool, even at `yolo`, which would turn level 3 into approval cards.
- Read again 2026-11-26 (`docs/users/features/hooks.md` and `headless.md` at tag v0.28.0, not run): hooks do have a switch, `disableAllHooks` in the system settings file that the driver already writes (it also ends the person's own hooks; `QWEN.md` and MCP servers stay). `--safe-mode` drops the rest too, but with it the solodawn MCP server. And a project's hooks run only after a person approved them in an interactive session; non-interactive runs leave unapproved ones out, and an approval mode from settings does not change that. So the first half of A3.8 may hold already, and its second half ("runs in task and review sessions") likely fails in a fresh worktree.
- Status: accepted by the coordinator (2026-11-22): Qwen Code ships with this limitation, the risk R-45 already accepts (only trusted repositories go into a group; `install.md`). Open until a capture shows a switch that keeps the configuration out and leaves approvals alone. The scenario of A10.4 for Qwen Code stays `@pending`: it needs the real program, an owner key, and a decision on the second half of A3.8 (`docs/cookbook/i11-real-machine-checklist.md` step 17).

### 2026-11-22 · limitation · The multi-person label scenarios cannot admit Q yet

- Symptom: A9.12 and five A9.42 cases (failing review, decision by P, decision by Q, secretary confirmation without Q on the allow list, and secretary confirmation with Q on it) each failed under `-race` in `Given the confirmation cast with Q`. The reply was `bad_request: the home has no such command`; decoding that refusal as a successful command reply reported `proto: a field that is always present is missing`.
- Cause: the cast sends `invite` before it can admit Q (`features/s2_confirm_test.go:119`), and the home had no registered handler for it (`internal/home/check.go:109`).
- Status: resolved. #382 registered `invite`, the cast admits Q with `JoinGroup` (`features/s2_cast_test.go`), and the scenarios of A9.12 and A9.42 are enabled.

### 2026-11-22 · limitation · The A9.44 quiet-task fixture does not reach the timer

- Symptom: A9.44 remains pending after HM-13. With the scenario enabled, the brief proposed by the replacement secretary never appeared in either acceptance run (`features/s2_confirm.feature:560`).
- Cause: the replacement secretary had no secretary tools on its host (entry of 2026-11-26 in Resolved), so its `propose_brief` was refused.
- Status: resolved. The first secretary proposes the brief before the change, and the scenario is enabled; the timer itself is also covered by `features/home_rounds.feature:40`.

### 2026-11-22 · limitation · Confirmation browser controls are not yet exercised

- Symptom: the I2-1b scenarios exercise the card and member commands over the browser connection but do not check the rendered buttons, expired card or member editor.
- Cause: the cast supplies protocol clients, not a rendered browser (`features/s2_cast_test.go:762`); the confirmation controls and member editor are planned in U2-2.
- Status: open; verify the UI cases of A9.8, A9.9, A9.11 and A9.14 with U2-2.

### 2026-11-22 · limitation · The cast's approval fixture does not execute shell commands

- Symptom: A9.19 verifies the approval tool returns allow and the fake CLI resumes; it does not verify that the command runs in a real CLI.
- Cause: the replay fixture supports protocol I/O only (`internal/host/fakecli/replay.go:98`).
- Status: open; actual command execution is unverified.

### 2026-11-22 · deviation · A confirmed brief cannot yet be replaced

- Symptom: a new brief cannot replace a confirmed one, although A9.13 and `labels.md` section 4 said it did.
- Cause: `internal/home/cmd_brief.go:153` accepts only pending or revised briefs as predecessors, as `planning/contracts/commands.md` 1.3 says.
- Status: resolved by decision (2026-11-25): the code stays. A started task's direction changes by cancelling the task and confirming a new brief, and `labels.md` section 4 and A9.13 in `planning/steps/step-2-3-acceptance.md` now say so. The A9.13 scenario follows that sequence and is enabled.

### 2026-11-22 · limitation · The OpenCode driver is declared from documentation, not from a capture

- Symptom: `DriverCaps["opencode"]` declares levels `1` and `3` and approvals `conn`, and `openCodeDriver` speaks the `opencode serve` API (`internal/host/opencode.go`), but no run of the real OpenCode exists: the fake OpenCode's scripts are written from the documented API, not cut from captures.
- Cause: T3-0 read OpenCode 1.19.6's documentation only, and no owner API key for a provider was available to T3-2. `drivers.md` section 4 asks that a value be declared from a run; levels `1` and `3`, the permission names, the event names (`session.idle`, `session.error`, `permission.asked`, `message.part.updated`), `POST /permission/<id>/reply`, and `OPENCODE_DISABLE_PROJECT_CONFIG` keeping the repository's configuration out are all `[documented]`.
- Status: open until the live part of M3, M4 and M7 runs once with the real OpenCode and an owner key (`drivers.md` section 7); then the fake's scripts are replaced by captures and the declaration corrected.

### 2026-11-22 · deviation · The Go export scenarios do not exercise browser UI or an OS timer

- Symptom: A9.27 and A9.28 exercise browser peers at the protocol boundary, not the browser's own follow logic or the owner-only commands in group info. A9.28 takes successive copies without waiting for an OS timer at one-minute intervals.
- Cause: `features/s2_export.feature:12,19,24` use the I2-0 Go cast and two invocations of the copy command. The product does not schedule copies (`planning/contracts/step-2/export.md:60-62`).
- Status: open for the OS timer. Both scenarios run since 2026-11-26 and pass at the protocol boundary: Q's peer asks P1, gets `moved` and dials P2 itself. The owner-only commands in the group info are covered by `web/e2e/s2.spec.ts` (A9.27), the browser's following by `features/web_move.feature` and `web/src/lib/{groups,net}.test.ts`.

### 2026-11-26 · bug · The web client does not follow a moved group and does not replace the old home on a rejoin

- Symptom: against A9.27, a browser whose group left the list is not sent to the new home; against A9.28, a rejoin of the group on another home does not replace the old home.
- Cause: after the export P1 pushes `groups` without the group, `Home.#list` (`web/src/lib/groups.svelte.ts:391`) drops it and nothing asks P1 again, so `moved` (`web/src/lib/groups.svelte.ts:357`) is never seen. The rejoin gap was not traced.
- Status: fixed by [#403](https://github.com/huanchong-99/SoloDawn/pull/403) (merged), both gaps; see "An idle browser lost a moved group, and an open tab stayed on a lost home".

### 2026-11-22 · limitation · Task participation history is inferred from current fields

- Symptom: a final task cancelled before its first role item was queued can qualify for the thread exception after its expired card is pruned, as can an assignee newly named by row 11 before its first activation. A reviewer's earlier participation can be lost after a later reassignment/review overwrites its evidence.
- Cause: `internal/home/cmd_confirm.go:48-80` derives participation from current roles, state, step and latest review, excluding unconfirmed role cards. Acknowledged inbox items are deleted; task rows contain no durable participation history. A foreign reassignment to the same AI after expiry needs a new request, since the request's one card is closed.
- Status: open, accepted for T2-2b pending the owner's review: no new table, export format or wire shape. Held role cards no longer qualify; known participation survives finalization while its current fields still show it.

### 2026-11-22 · limitation · A late watch item can outlive hand-back

- Symptom: a watch item arriving between PTY shutdown and the hand-back turn can leave a grant through the next takeover.
- Cause: `internal/host/pty.go` `runStop` clears grants before `internal/host/takeover.go` starts the hand-back turn; `watchItem` in `internal/host/watch.go` still sees the session in `takeover` during that interval.
- Status: open.

### 2026-11-22 · limitation · A failed move can leave a usable export file

- Symptom: if committing the source deletion fails after publishing the SQLite file, the command fails and the source remains home, but the export file is already usable as a copy.
- Cause: `internal/store/export.go` publishes the completed SQLite snapshot before deleting the group's rows in the source transaction. The filesystem rename cannot be rolled back with SQLite.
- Status: open. Do not import a failed move as a move; retry export first.

### 2026-11-22 · limitation · An inbox item handled just before a move can be handled again

- Symptom: if a host acted on an item before a move but its ack did not reach the old home in time, the item is still unacknowledged in the export. The new home sends it with a new local item ID.
- Cause: delivery deduplication is keyed by home device and item ID, and the inbox format does not carry the old home's delivery identity.
- Status: open. Acknowledging the imported item deletes it at its new home.

### 2026-11-22 · limitation · Remembered addresses are not dialled yet

- Symptom: `home.add` records signalling addresses, but a node still needs a shared configured rendezvous to reach an external home.
- Cause: the node's route lookup in `internal/node/node.go` uses active pairing and configured rendezvous paths, not the remembered addresses.
- Status: open.

### 2026-11-22 · limitation · Rendezvous has no explicit room leave

- Symptom: cancelling an invitation stops browser handling, but the rendezvous socket remains in the room until expiry or disconnect. Retrying the same room on the same socket does not trigger a new `joined` from a home already in the room.
- Cause: `internal/rendezvous/room.go` has no leave frame. It ignores duplicate joins by the same socket and caps membership at 16 rooms per connection.
- Status: open; the browser must keep the socket alive for existing homes.

### 2026-11-22 · deviation · Host turn labels approximate task participation by role

- Symptom: a role still held for confirmation counts as participation for host labels, although `planning/contracts/step-2/confirm.md:37-38,156-172` also requires a queued assign or review item.
- Cause: `internal/host/turn_info.go:13` reads the source task's assignee and reviewer. Ended sessions are deleted (`internal/host/session.go:284-300`), so the session table is not a durable record of queued role items.
- Status: open, accepted for T2-13; labels enforce nothing.

### 2026-11-22 · limitation · Direct-confirmation label history is bounded

- Symptom: confirmation cards older than one page are not consulted for carried-message and recovery labels unless already present in the turn's fetched history.
- Cause: `internal/host/turn_info.go:74` fetches at most one page of 100 `confirm` messages per Compose. Host state-only sync excludes message rows (`internal/host/deliver.go:259`).
- Status: open; T2-2a supplies the home card producer in `internal/home/cmd_confirm.go`. Live mention labels continue to trust the home's queueing decision.

### 2026-11-22 · limitation · A hand-back that starts while the node is stopping is lost

- Symptom: when the node is stopped with a terminal open and the interactive CLI exits at once, the hand-back can reach `Runner.Turn` after `settleRuntime` has begun. The start is refused, the session goes `dormant`, and nothing is posted because the home connection is already closed, so the next start does not hand the session back as `planning/flows/takeover.md` section E says.
- Cause: `Close` closes the sockets first (`internal/node/node.go`), which closes the owner's terminal and starts the hand-back before `Settle`; a refused start moves the row to `dormant` (`startFromDormant` in `internal/host/run.go`), and `handBackSaved` (`internal/host/takeover.go`) only finds rows left in `takeover`.
- Status: open, accepted (coordinator, 2026-11-22, HS-20 review): rare, no code.

### 2026-11-22 · limitation · `max_open` is not applied when a takeover starts

- Symptom: a takeover of an AI whose open sessions are already at `max_open` opens its session anyway, so one session more than `max_open` can be open (`planning/flows/agent-turn.md` step 5.1).
- Cause: `takeoverStart` and `openSession` (`internal/host/takeover.go`) take no `max_open` slot; the hand-back turn takes none either, because a taken-over session already counts as open (`startHandBack` in `internal/host/run.go`).
- Status: open, accepted (coordinator, 2026-11-22, HS-20 review): least code.

### 2026-11-22 · deviation · `host.offline` leaves an open terminal open

- Symptom: `host.offline` closes the structured sessions only; a terminal opened by takeover stays open, its member stays `takeover`, and `takeover.start` is accepted while offline. `planning/contracts/commands.md` (`host.offline`) says every session is closed.
- Cause: `handleHostOffline` (`internal/host/limits.go`) closes structured CLIs only. The terminal is the owner's own work; going offline only stops AI turns.
- Status: by design (coordinator, 2026-11-22, HS-20 review, decision 2). When such a terminal closes, the hand-back starts no AI turn while the AI is held (offline or rate limited): the session goes `dormant` and the host posts the facts (`handBackTurn` in `internal/host/takeover.go`).

### 2026-11-22 · limitation · A screen reader does not read what the terminal prints

- Symptom: on `#/terminal/<g>/<member>` a screen reader reaches the terminal's input field and its description, but hears nothing of what the AI's CLI prints.
- Cause: xterm.js exposes the screen to assistive technology only with its `screenReaderMode` option, which `web/src/routes/Terminal.svelte` (the `new XTerm({ … })` options) leaves off.
- Status: open, found in U-16b and left out of its fixed scope. `screenReaderMode: true` is the one-line change; it was not measured with a CLI that redraws its screen.

### 2026-11-22 · limitation · A waiting notification covers "Hand back" on a phone's terminal

- Symptom: on the terminal page below 768 px, an in-page notification that waits on the reader lies over the takeover bar, "Hand back" included, until it is opened or closed (measured in Chrome at 390 px: notification 12 to 378 by 64 to 130, "Hand back" 272 to 374 by 62 to 106). From 768 px it overlaps the button by at most 14 px.
- Cause: the stack hangs under the page's own head (`web/src/components/notify/Notifications.svelte`, the rule for `.app[data-full='phone']`), and the takeover bar is the first thing under that head.
- Status: open, found in U-16b, whose scope names the chat only. The notification's own close button stays reachable.

### 2026-11-22 · limitation · Faint text in an ANSI colour stays below 4.5 to 1 in the terminal

- Symptom: text a CLI prints faint (SGR 2) in one of the sixteen colours is drawn at half that colour's opacity: 2.8 to 1 (red) to 3.5 to 1 (green) on the terminal's background, for the four colours the page's theme sets. Faint text in the default colour is 5.6 to 1.
- Cause: xterm.js halves the opacity of faint text (`DomRenderer.ts`, the `.xterm-dim` rules); `web/src/routes/Terminal.svelte` replaces only the default colour's rule with `--term-dim`, since one grey for every colour would drop the colour.
- Status: open, accepted in U-16b: the colours are the CLI's own.

### 2026-11-22 · limitation · A denied call draws as waiting

- Symptom: a `call` message whose state is `denied` (`row-msg-call-denied`, frozen by P-4a) shows the waiting line and the waiting state in the call area.
- Cause: `callView` treats every status other than `answered` as waiting (`web/src/lib/call.ts:78`). The states sheet therefore lists three call rows, and `web/features/call.spec.ts` expects them.
- Status: resolved in U2-2. The call block shows “Denied” without a waiting timer or reply.

### 2026-11-22 · deviation · A9.26 live M5 forge coverage awaits a configured live run

- Symptom: the original live fixture checked two remote filenames without reusing M5's forge steps or inspecting request heads.
- Cause: A9.26 used the subprocess fixture instead of M5's two-task forge flow.
- Status: ended by the product-model change MODEL-1126 ([decision](docs/decisions/proposed/feature/2026-11-26-groups-workspaces.md)): the forge and A9.26 are removed, and the real-machine checklist has no forge step.

### 2026-11-21 · deviation · The `rounds_limit` system message has an id derived from the triggering post's nonce

- Symptom: when a `post` by an AI in a task's thread reaches `limits.rounds` (task-machine row 21), the command writes both the `post` row under `n` and the main-line `rounds_limit` `system` row under the first 16 bytes of the SHA-256 of `SoloDawn rounds limit <n>` in base64url, rather than a random id.
- Cause: `n` already names the `post` row, and a command function is pure and has no randomness (`revDerivedID` in `internal/home/cmd_review.go:149`, called from `internal/home/rounds.go:176`).
- Status: open, decided by unit HM-13 on 2026-11-21, for the same reason as `briefTaskID` below.

### 2026-11-21 · limitation · Quiet-period tracking lives in memory

- Symptom: a home restart forgets which quiet periods already notified the secretary and can send one more `quiet` notice if an active task stays silent across the restart.
- Cause: row 23 writes no row (`planning/contracts/task-machine.md` row 23), so `After["quiet"]` records each notified task's last-activity timestamp in the group's `State.quiet` in memory (`rndRememberQuiet` in `internal/home/rounds.go:121`, `internal/home/home.go:69`).
- Status: open, accepted in step 1.

### 2026-11-22 · limitation · Existing stores do not get the `row_named` index

- Symptom, cause and status: T3-5 added `row_named` to migration 1 in place (`internal/store/schema.sql`), so a store created before it keeps working without the index and a preset lookup scans the group's notes instead; accepted before 1.0, since migrations are cut.

### 2026-11-22 · limitation · Web Push VAPID `sub` URL is not yet checked against Apple's push service

- Symptom: none observed; `wpushSubscriber` (`https://github.com/huanchong-99/SoloDawn`) has not yet been exercised against Apple's Web Push service on iOS.
- Cause: `internal/home/webpush.go` (`wpushSubscriber`) sets the VAPID JWT `sub` claim to the repository URL, and no iOS device run is available in automated CI.
- Status: open for the live iOS check of `A8.3` (`I-10` / `I-11`).

### 2026-11-22 · limitation · An unanswered AI-to-AI `call` stays `waiting`

- Symptom: when the called AI never answers with `reply`, the `call` message keeps `state.status = "waiting"` for good; the web call block (`U-10`) shows it as waiting, with the time since it was made.
- Cause: `planning/contracts/messages.md` section 3.4 gives a call only `waiting` and `answered`, with no `expired` as decisions have. The caller is not blocked: `ask` returns its receipt after `limits.ask_wait_s` (45 s, `internal/host/tools_ask.go:117-119`) and goes on with its turn.
- Status: open, accepted for 1.0 (coordinator, 2026-11-22, `HS-12` review): no expiry, less code.

### 2026-11-21 · bug · go-pty v0.2.3 blocks Unix Close during Read, keeps the slave TTY open, and misresolves PATH with Dir on Windows

- Symptom: three defects in `github.com/aymanbagabas/go-pty` v0.2.3 (with `github.com/creack/pty` v1.1.24). (1) On macOS, calling `Pty.Close` while another goroutine is blocked in `Pty.Read` returns without closing the kernel master descriptor, so `Pty.Read` stays blocked and the attached child receives neither `EOF` nor `SIGHUP`. (2) On Unix, when the attached child exits by itself, `Pty.Read` never reaches `EOF`. (3) On Windows, starting a command by bare name (such as `claude` or `codex`) with `Cmd.Dir` set fails to find the executable on `PATH`.
- Cause: (1) `creack/pty` opens `/dev/ptmx` on Darwin without `O_NONBLOCK` and passes the descriptor to `os.NewFile` (`pty_darwin.go:14-18`), which leaves `poll.FD` in blocking mode (`isBlocking == 1`) and defers `syscall.Close` until `Read` returns. (2) `Cmd.Start` on Unix leaves the parent's copy of `pty.slave` open (`cmd_unix.go:42-52`). (3) `Cmd.Start` on Windows joins `c.Dir` and `c.Path` in `lookExtensions` and `joinExeDirAndFName` (`cmd_windows.go:36-50`, `:240-243`, `:345`) whenever `c.Path` has no volume or separator.
- Workaround: `ptyWrapStarted` (`internal/host/pty_unix.go`) duplicates the started master descriptor with `F_DUPFD_CLOEXEC`, sets `O_NONBLOCK`, wraps it with `os.NewFile` so Go's netpoller owns it, and closes `go-pty`'s master and slave handles; `ptyOpenHandle` (`internal/host/pty.go`) resolves `argv[0]` through `exec.LookPath` before `ptmx.Command`.
- Status: open, external; avoided in HS-19.

### 2026-11-21 · limitation · Finished tasks stay in a group's state in memory after retention until a restart

- Symptom: after the daily retention run deletes a `done` or `cancelled` task row older than `retention_days` from SQLite, `State.Tasks` in the running home still holds that task row until the loop restarts. A rule or a state read that inspects `State.Tasks` before a restart still sees the finished task; a cold read from the store snapshot and a restarted loop do not.
- Cause: retention runs `home.Retain` (`internal/home/retention.go`, `Retain`; `internal/store/retention.go`, `Retain`) directly against SQLite, deleting aged rows without removing finished tasks from `State.Tasks` (`internal/home/home.go`).
- Status: open, accepted for step 1 (`internal/home/README.md`, "Retention"). Finished tasks take no further commands and occupy little memory until the next restart; `grp.num` keeps the highest task number so task numbers are never reused after a restart.

### 2026-11-22 · limitation · macOS Application Firewall blocks incoming direct connections to an unsigned `solodawn` binary

- Symptom: on macOS with the Application Firewall enabled, incoming UDP and ICE-TCP flows to an unsigned `solodawn` (or test) binary are blocked (`verdict: 2 (Block)`) unless the user clicks Allow when macOS asks or adds `solodawn` under System Settings → Network → Firewall → Options, so direct connections fail and fall back to the relay.
- Cause: release and development binaries are not code-signed (`install.md`, "Verify and install"), and the macOS Application Firewall blocks unsolicited incoming connections to unsigned executables until allowed.
- Status: open, documented in `install.md`. Why macOS sometimes never asks: the resolved entry "Every Go binary shares one macOS firewall identity" (2026-11-26).

### 2026-11-21 · limitation · Windows `AF_UNIX` half-close does not always deliver EOF; a shim may outlive its CLI until the session closes

- Symptom: on Windows, `solodawn mcp --token <t>` can stay blocked in `bridgeShim`'s socket-to-stdout copy (`internal/host/shim.go:60`) after its CLI has closed `stdin`, and `TestRunShimArgumentsErrorsAndHalfClose` hung on Windows CI (`internal/host/mcp_test.go:827,908,914`) when `verifyShimUnixHalfClose` waited for end-of-input after the shim half-closed the socket.
- Cause: when `stdin` reaches EOF, `copyShimInput` calls `half.CloseWrite()` on `*net.UnixConn` (`internal/host/shim.go:74-76`, `shutdown` with `SD_SEND`) without closing the connection, and Windows `afunix.sys` occasionally fails to deliver `io.EOF` when `CloseWrite` races with a concurrent `WSARecv` on the peer (`golang/go#73140`).
- Status: open, accepted on 2026-11-21 with no product code changed per the owner's rule on rare cases. When the node closes the session (`internal/host/run.go`), closing the session kills the CLI's process tree (`detachTerminal`), which includes the shim, and `ServeMCP` closes its socket end when the session ends.

### 2026-11-21 · limitation · `AskUserQuestion` reaching `approve` under each permission mode is not yet verified by a live CLI capture

- Symptom: no spike capture or scripted fake-CLI test shows Claude Code invoking `--permission-prompt-tool mcp__solodawn__approve` when the model calls `AskUserQuestion` under `--permission-mode bypassPermissions` (level 3, the default), or honouring the `updatedInput.answers` reply (`planning/contracts/mcp-tools.md` section 3).
- Cause: the macOS spike captures (`spikes/b-04-macos/captures/claude/*.jsonl`) mention `AskUserQuestion` only in the `system/init` tool list, and `internal/host/approve_test.go` calls `Tools["approve"]` directly.
- Status: open; marked `[unverified]` in `planning/contracts/mcp-tools.md` section 3 and added to the `live` checks of `A4.1` (`B-17`) and `I-11`.

### 2026-11-21 · limitation · Claude Code `AskUserQuestion` with `multiSelect` uses a single-choice `decision` card

- Symptom: when Claude Code calls `approve` for an `AskUserQuestion` entry with `multiSelect: true`, the host posts a single-choice `decision` card (`internal/host/approve.go`, `postAndWaitDecision`) whose `act choose` carries at most one option id, so `updatedInput.answers[question]` holds one option label unless the owner enters multiple picks in the free-text field.
- Cause: `decision` cards (`planning/contracts/messages.md` section 2.4, `planning/contracts/commands.md` section 1.4) take a single `arg` option id plus optional `text`, and owner override 1 of 2026-11-21 does not add multi-select cards.
- Status: open, accepted for 1.0 (`planning/contracts/mcp-tools.md` section 3); multiple picks are given in the free-text field.

### 2026-11-21 · limitation · An immediate `submit_task` after `run_checks` can read `checks.local` before the home's row push

- Symptom: a scripted CLI that calls `submit_task` immediately after `run_checks` returns can read a stale `task` row in `Host.Rows` and fail with `conflict` (`local checks have not passed`), whereas with a real model the gap between the two tool calls is seconds.
- Cause: the home answers `task checked` before pushing the `rows` frame that carries the updated `checks.local` (`internal/home/README.md`), and `tskPrepareSubmit` (`internal/host/tools_task.go:318`) reads `checks.local` from `Host.Rows`.
- Status: ended by the product-model change MODEL-1126 ([decision](docs/decisions/proposed/feature/2026-11-26-groups-workspaces.md)): `run_checks` and `checks.local` are removed; `submit_task` is the submission and reads no check result.

### 2026-11-21 · deviation · Host derives the data folder from SQLite's `pragma_database_list`

- Symptom: for an in-memory store (`:memory:`), `projectDir` returns `""` and delivery skips disk clone and worktree creation; for a disk store it derives `projects/<group>` from the directory of the `main` database file (`filepath.Join(filepath.Dir(dbFile), "projects", group)`), tying the host to the store's file layout.
- Cause: `host.Host` (`internal/host/host.go:75`) has no `Dir` field yet, so `projectDir` (`internal/host/run.go`, shared by `sessionDir`, `deliver.go` and `join.go`) reads `file` from `pragma_database_list WHERE name = 'main'`.
- Status: open for the one place left, `dataDir` in `internal/host/tools_workspaces.go`, which leaves the node's own data folder out of the folder list. The delivery, clone and worktree uses are ended by the product-model change MODEL-1126 ([decision](docs/decisions/proposed/feature/2026-11-26-groups-workspaces.md)). `node` setting a `Dir` field on `Host` can replace the `pragma_database_list` lookup.

### 2026-11-22 · limitation · Step 17 cleanup leaves narrow turn-start windows and retries dirty merged worktrees

- Symptom: a `notice` or `mention` arriving during step 17 cleanup can start one more turn in an obsolete session (whose CLI then stays open until `idle_minutes` expires), and a merged task worktree left with untracked files fails the non-forced `git worktree remove` and retries one Git call on each later `rows` push.
- Cause: `finishSessionClose` (`internal/host/run.go`) starts `go Deliver` before the caller's `Sessions.End` (`internal/host/deliver.go`), `Runner.Close` finds no `liveProc` while `startFromDormant` is between `Move(Dormant, Working)` and `rt.procs[key] = lp`, and `RemoveTaskWorkspace(..., TaskMerged)` omits `--force` so unsaved files in a merged worktree are preserved (`planning/contracts/local.md` section 7).
- Status: open, accepted on 2026-11-22 (`HS-23` review), for the turn-start windows only. The retry of a dirty merged worktree is ended by the product-model change MODEL-1126 ([decision](docs/decisions/proposed/feature/2026-11-26-groups-workspaces.md)): a task has no worktree to remove.

### 2026-11-21 · limitation · `reportWorkspaceFailure` duplicates `member status` assembly from the session runtime

- Symptom: on a task or review workspace Git failure, `reportWorkspaceFailure` (`internal/host/deliver.go:934`) builds `member status` (`turn_interrupted`) by hand and infers `state` (`waiting` when any session of the AI is `waiting`, else `dormant`) instead of sharing the session runtime's status builder.
- Cause: HS-13 (#186) and HS-15 (#187) were built on separate branches without a shared status helper on `host.Runtime`.
- Status: ended by the product-model change MODEL-1126 ([decision](docs/decisions/proposed/feature/2026-11-26-groups-workspaces.md)): `reportWorkspaceFailure` is removed with the task and review workspaces.

### 2026-11-21 · limitation · `max_open` is enforced in the session runtime rather than in delivery

- Symptom: `deliver.go` composes and hands an eligible turn to `Runner.Turn` without counting open sessions against `max_open` itself.
- Cause: `planning/flows/agent-turn.md` step 5.1 places `max_open` (closing the longest-`waiting` session, or waiting when every open session is `working` or `takeover`) in the session runtime (`HS-15`, #187, `tryReserveSlot`), so delivery does not duplicate it; `Deliver` (`deliverNow` in `internal/host/deliver.go`) walks all held AI keys on each wake so a freed slot reaches waiting AIs.
- Status: open, coordinated between HS-13 (#186) and HS-15 (#187).

### 2026-11-21 · limitation · `member/status` refuses `cap_reached` while `settings.set` is postponed

- Symptom: a `member status` carrying host event code `cap_reached` (`planning/contracts/commands.md` section 1.9; `planning/contracts/messages.md` section 2.11) is answered `bad_request` ("unknown host event code").
- Cause: nothing in 1.0 can set `agent.caps`: `settings.set`, its only writer, is postponed after 1.0 (`planning/contracts/commands.md`), so `HS-18` sends no `cap_reached` and `internal/home/cmd_hostev.go` (`hevEventReaders`) reads `rate_limited` but not `cap_reached` (coordinator's decision of 2026-11-22).
- Status: open until `settings.set` comes back with caps.

### 2026-11-21 · limitation · Reassigning a task's assignee also marks a reviewer's pending approval card `reassigned`

- Symptom: if a task's assignee changes while the task's reviewer has a pending approval card on that task, the reviewer's card becomes `reassigned` along with the old assignee's cards, so it can no longer be approved or denied and the reviewer's waiting CLI receives no decision.
- Cause: row 17 allows both the assignee and the reviewer to post approval cards on the task (`internal/home/machine.go:224`), while row 11 sets `Effects.Cards = "reassigned"` for all pending approval cards of the task whenever `Assignee` changes (`internal/home/machine.go:617`; `planning/contracts/task-machine.md` row 11) without distinguishing who asked. Hitting this requires a review session asking for approval and an assignee reassignment while that request is open.
- Status: open, accepted as a rare limitation by the coordinator's decision of 2026-11-21 (#184).

### 2026-11-21 · limitation · A member's `limited` state from `login_invalid` ends at its next completed turn, not at `agent.set`

- Symptom: after `login_invalid` puts an AI member into `limited`, fixing sign-in and sending `agent.set` sets `login_ok` back to `true` in `host.info`, while the group still shows the member as `limited` until the next item arrives and its turn completes.
- Cause: `agent.set` clears `agtPart.invalid` (`internal/host/agents.go`) and is not a session `Event`, while the member status overlay `runtimePart.limited` (`internal/host/run.go:69`, set at `:664-680`) is cleared in `finishActiveTurn` (`internal/host/run.go:473-482`) when a turn finishes. `runtimePart.isLimited` (`:682-686`) feeds member status only and holds no delivery.
- Status: open, accepted by the coordinator's decision of 2026-11-21 (`HS-3`, `HS-15`); `planning/contracts/commands.md` section 2 and `planning/flows/agent-turn.md` section E match this behaviour.

### 2026-11-21 · limitation · Claude driver matches English error text for `LoginInvalid` and `NoSession`

- Symptom: `claudeDiagEvent` (`internal/host/claude.go`) recognises an unresumable session or invalid sign-in from HTTP status `401` and English error substrings (`no conversation found`, `session not found`, `/login`, `not logged in`, `invalid api key`, `authentication`) in `result` or non-zero-exit `stderr`, so a wording change in a newer Claude Code release can report a failed turn or crash instead.
- Cause: Claude Code has no dedicated stream-json event type for sign-in or missing-session errors (see also the open entry on missing invalid-login captures).
- Status: open. Revisit error patterns when live failure captures are added.

### 2026-11-21 · limitation · Codex `default_mode_request_user_input` and `item/tool/requestUserInput` are unverified against a live session

- Symptom: Codex CLI 0.164.0 hides its `request_user_input` tool in Default mode (`codex features list` reports `default_mode_request_user_input` as `under development`, default `false`), and no spike capture covers `item/tool/requestUserInput` (`planning/contracts/drivers.md` section 1.2, step 4).
- Cause: `codexBaseFlags` (`internal/host/codex.go`) passes `-c features.default_mode_request_user_input=true` on `codex app-server` and `codex resume`, and `onUserInputRequest` maps `item/tool/requestUserInput` from the generated schema (`[documented]`), without a live run on the owner's sign-in yet.
- Status: open for the live check in B-17 / I-11 (`planning/steps/step-1-acceptance.md` A4.11). If the flag does not offer a working `request_user_input` tool in Default mode, record in `planning/contracts/drivers.md` section 1.2 that a Codex AI asks through `request_decision` (`HS-11`) instead.

### 2026-11-21 · limitation · The host has no shutdown hook for open CLI processes

- Symptom: when `solodawn node` stops on `SIGINT` or `SIGTERM`, open AI CLI processes are not closed explicitly before `node.db` closes; they end on stdin EOF when the node process exits.
- Cause: `host.Host` exposes no shutdown method, and CLI processes are started with `context.Background()` (`internal/host/run.go:380`).
- Status: open, accepted for 1.0 (`N-4` review).

### 2026-11-22 · limitation · `net_status` omits `rtt_ms` and reports `mapped_ports` as `false` in step 1

- Symptom: `net_status` (`internal/node/netstatus.go`) omits `rtt_ms` from `paths` entries and sets `nat.mapped_ports` to `false`.
- Cause: `*peer.Connection` (`internal/peer/sockets.go`) exposes `Path()` without ICE candidate-pair round-trip statistics (the field and its reporter were removed until it does), and router port mapping (`PR-3`) was cut from step 1 on 2026-11-21 (`planning/contracts/mcp-tools.md` section 1).
- Status: open, accepted for step 1 (`N-8`).

### 2026-11-21 · limitation · `host.info` is pushed for `watchers` only and reports `queued` as `0`

- Symptom: `Surface` (`internal/node/surface.go`) answers `host.info` requests with `HostState` and pushes `host.info` only after a request with `refresh` and when `watchers` change, and every `AgentGroup.Queued` entry in `host.info` is `0` (`internal/node/surface.go:154`). A browser that stays connected therefore hears of a group that the person joined in another browser only at its next `host.info`, which is its next connection.
- Cause: `settings.set` is postponed with `U-12`, the 1.0 web client requests `host.info` on connect and after `agent.set`, and held items are held on the home (`not_before`, `HM-7`) rather than counted on the host.
- Status: open for the push of the other fields, accepted for 1.0 (`N-3`). `watchers` is pushed when a watch grant is added or ends (`Host.WatchersChanged`, `internal/host/watch.go`; `Surface.pushHostInfo`, `internal/node/surface.go`). `queued` counts the items the host holds since HS-20 (`Host.QueuedCount`, `internal/host/deliver.go`). The web client shows no held count: the terminal page and the card of an AI in takeover say that messages to it wait until hand-back (`U-11`, coordinator 2026-11-22; `planning/flows/takeover.md` step 9).

### 2026-11-20 · limitation · Self-update leaves three things to the node runtime (N-4)

- Symptom, from the independent review of `internal/node`:
  1. `Apply` checks `Busy` and then calls `Restart` (`internal/node/update.go:98`). A turn or a takeover admitted between the two is restarted.
  2. `Status` (`internal/node/update.go:40`) is written by `Check` and read for `host.info`; read from another goroutine, it races.
  3. `Restart` (`:39`) must replace the process in place (exec on Unix). launchd and systemd stop the whole process group when the node's process ends, so a child started before the exit would be stopped with it.
- Cause: the `Updater` holds no lock and does not restart by itself; the owner of the runtime is to call `Check` and `Apply` under the lock that admits turns and takeovers, copy `Status` under it, and supply a `Restart` that execs.
- Status: open, postponed off the step-1 main line (`N-6`). Version 1 ships without self-update; `install.md` says how to update by hand.

### 2026-11-21 · limitation · `Sessions` handlers and runtime pairing/surface wiring across `HM-2`, `HM-3`, `N-3` and `N-4`

- Symptom: before runtime assembly, inbound `ctl` requests for `sync`, `fetch` and `host.info` were answered `bad_request` on a bare `Sessions` hub (`internal/node/session.go`), and `Pairer.Serve`, `Pairer.Known`/`Pairer.Routes`, `Pairer.HandleEvent`/`PairConfig.Paths`, `Pairer.Peer`, local socket `pair`/`pair-browser`/`open`, and `SurfaceConfig.Agents` (`(*Host).Agents`) awaited assembly in `internal/node/node.go`.
- Cause: `Sessions`, `HM-2`, `HM-3`, `HM-7`, `N-3` and `N-4` were built as separate units (`settings.set` is postponed with `U-12`).
- Status: resolved in `HM-7` (#221), `HM-2` (#205), `HM-3` (#207), `N-3` (#223) and `N-4` (#226).

### 2026-11-21 · limitation · Postponed step-2 contracts still assume an item-level `cause`

- Symptom: the step-2 planning documents (`planning/contracts/step-2/authority.md:36, 85, 92-93, 104, 108`, `planning/contracts/step-2/confirm.md:145-147, 196-223, 244`, `planning/contracts/step-2/watch-team.md:46-67` and `planning/steps/parallel-plan-step-2-3.md:20-21, 27`) require an inbox item to carry a signed `cause` when a group home belongs to another person, whereas `P-1b` removed `cause` from step-1 items.
- Cause: step 2 is postponed while step 1 ships with plain unsigned items under the owner's 2026-11-21 decision.
- Status: resolved in T2-13; the step-2 contracts now use plain items and host-written context labels, without an item-level `cause`.

### 2026-11-21 · limitation · Group and review sessions default to permission level 3 and rely on instructions not to edit files

- Symptom: at the default permission level of 3 (owner's decision of 2026-11-21), a group session or a review session starts its CLI with full tool access rather than a CLI-enforced read-only sandbox, so a model that ignores its prompt could edit files in `repo/` or `review/<num>/`.
- Cause: `readLaunchConfig` (`internal/host/run.go`) applies `perm.level` or `default_level` (default 3) to every session kind so that group and review sessions keep the CLI's ask-the-user tool and their multiple-choice questions reach the owner as decision cards (`planning/sessions.md` section 1). At level 1 Claude Code runs with `--tools Read,Glob,Grep` (`planning/contracts/drivers.md` section 1.1), which strips that tool.
- Status: open, accepted by the coordinator's decision of 2026-11-21 (#187). Each session's instructions (`planning/contracts/drivers.md` section 4) tell group and review sessions not to change code.

### 2026-11-21 · limitation · A late duplicate `TurnEnded` from a CLI can end the next turn

- Symptom: if a CLI process emits a duplicate `TurnEnded` for a finished turn and `runProcessLoop` handles that event after `Runner.Turn` has started the next turn on the same `waiting` process, the runtime ends the new turn prematurely, moves the session back to `waiting`, reports `waiting` status, and calls `Deliver` early.
- Cause: `liveProc` tracks an open turn with a boolean `turnActive` rather than a per-turn identifier (`internal/host/run.go:107`). Once `claimWaitingProc` sets `lp.turnActive = true` for the next turn (`internal/host/run.go:285`), `onProcessTurnEnded` (`internal/host/run.go:458`) passes `finishActiveTurn` (`internal/host/run.go:473`) for the stale event, clears `lp.turnActive` (`internal/host/run.go:479`), and moves the session from `working` to `waiting` (`internal/host/run.go:466`).
- Status: open, accepted on 2026-11-21 with no product code added per the owner's rule on rare cases. In production the next turn starts only after `Deliver` (`internal/host/run.go:470`) fetches held items from the home, leaving a small window for a duplicate event from the previous turn to arrive that late.

### 2026-11-21 · limitation · The jobs of a group in the home's run loop have no bound

- Symptom: not observed. A client that sends `cmd` frames faster than its group applies them makes the list of that group's jobs grow, and with it the memory of the node, until the client stops. A group applies a command in one transaction of the store, so this needs a client that sends on purpose or a store that has become slow. Ticks and events are jobs as well: while a group does not keep up, each tick of `LoopConfig.Every` and each `Event` adds one job to its list.
- Cause: `Loop.Cmd` hands a command to the goroutine of its group and returns (`internal/home/run.go`, `route`), and `Tick` and `Event` hand theirs to every group (`all`); the list (`runQueue` in `internal/home/run_conn.go`) takes every job. That is on purpose: no caller, hook or timer can then wait for a group, which is what rules out a deadlock ([decision](docs/decisions/implemented/architecture/2026-11-21-home-run-loop.md)).
- Status: open, without effect in step 1, where every client of a home is a device of its owner. The request table of the `ctl` session (N-2) is the place to limit how many requests of one connection are unanswered; a tick that finds the tick before it still waiting could be left out.

### 2026-11-21 · limitation · The limit of waiting frames does not meet a slow browser

- Symptom: not observed. The run loop gives a connection up when 1,024 frames wait for it. A `peer.Connection` never lets a frame wait: its `Send` hands the text to the data channel, which keeps it in its own send buffer without a limit. A browser that stops reading while its connection stays open so makes that buffer grow, and the loop does not see it.
- Cause: `(*Connection).Send` calls `SendText` and returns (`internal/peer/ctl.go:174`); `home.Client` has `Send` and `Done` only (`internal/home/home.go:27`), so the loop cannot ask how much is buffered. The limit (`runBacklog`, `internal/home/run_conn.go`) applies to a client whose `Send` waits, such as a local link.
- Status: open. Either `peer.Connection.Send` waits, or fails, above a buffered amount, or the node closes a connection whose buffer stays above a limit; owner N-2 or the owner of `internal/peer`.

### 2026-11-21 · limitation · A command without `g` knows only the devices of the node's owner

- Symptom: a `cmd` whose payload has no `g` is answered `bad_request` when a device of the node's owner signed it, and `unknown_device` when a device of another member of a group homed here did, although that device is known to its group.
- Cause: such a command belongs to no group, so the run loop answers it with a state that holds the owner's log only (`internal/home/run.go`, `stray`). It is the case of "A member of another group on the same home is answered `unknown_device`" below, for a payload that names no group.
- Status: open, without effect in step 1. Settled with that entry in step 2.

### 2026-11-21 · deviation · `act confirm` does not make the `replaces` check of the post again

- Symptom: `planning/contracts/commands.md` 1.4 says that a `confirm` makes "the checks of a `brief` post (1.3)" again. One of them is that `body.replaces`, when not `null`, names a brief that is `pending` or `revise`. The post itself made that brief `superseded`, so the check, made again, would refuse every confirm of a brief that replaces another.
- Cause: the contract lists the post's checks without saying which of them are about the group when the brief is confirmed. `(*State).briefStill` (`internal/home/cmd_brief.go:243`) makes the others again: the shape of the body, the requester a member, every assignee and reviewer a working AI. That an entry's assignee is not its reviewer is made again by the task machine's role rule, which `Step` answers `conflict` (`internal/home/machine.go:253`).
- Status: open, decided by unit HM-8 on 2026-11-21. The contract should say that the `replaces` check is the post's only.

### 2026-11-21 · deviation · Derived member and task ids where the home creates rows without a client id

- Symptom: `planning/contracts/rows.md` says that the ids of members, tasks and messages are random ids. The home gives the owner's `member` row in `group create` the first 16 bytes of the SHA-256 of `SoloDawn group owner <n>`, and the task of entry `i` of a confirmed brief (with its `task` message) the first 16 bytes of the SHA-256 of `SoloDawn brief task <n of the act> <i>`.
- Cause: the function of a command is pure and has no randomness (`internal/home/README.md`, "Functions are pure"); `grpOwnerID` (`internal/home/cmd_group.go:89`) and `briefTaskID` (`internal/home/cmd_brief.go:286`). Each id has the form of a random id, and nobody knows it before the command arrives, because `n` is random. An id that a task or a message already has is answered `conflict` (`internal/home/cmd_brief.go:272`).
- Status: open, decided by units HM-8 and HM-5 on 2026-11-21. Should the seam ever give functions a source of random ids, the derivations can go.

### 2026-11-21 · deviation · A `task_failed` row written beside a `checks` or `review` row has an id derived from the command's nonce

- Symptom: when `task cloud` reaches `limits.failures` (row 6 into row 14) or `post` of kind `review` exceeds `limits.review_rounds` (row 7 into row 14), the command writes two message rows in the task's thread: the `checks` or `review` row under `n`, and the `task_failed` `system` row under the first 16 bytes of the SHA-256 of `SoloDawn task failed <n>` in base64url.
- Cause: `n` already names the `checks` or `review` message row, and a command function is pure and has no randomness (`revDerivedID` in `internal/home/cmd_review.go:149`).
- Status: open, decided in unit HM-11 on 2026-11-21, for the same reason as `briefTaskID` above.

### 2026-11-21 · deviation · Passed hosted checks write a `checks` message in the task's thread

- Symptom: `task cloud` with `ok: true` writes a `checks` message (`stage: "cloud"`, `ok: true`) in the task's thread, where `planning/contracts/task-machine.md` row 6 and `planning/flows/trace-task.md` step 8 mention a `checks` message only when hosted checks do not pass.
- Cause: `planning/contracts/commands.md` 1.5 and `planning/contracts/messages.md` 2.8 state that the home writes a `checks` message from `task checked` (`local`) and `task cloud` (`cloud`), and `internal/proto/testdata/task-cloud.json` carries `items` when `ok` is `true`, which `TaskRow.Checks.Cloud` does not store (`revCloud` in `internal/home/cmd_review.go:39`).
- Status: ended by the product-model change MODEL-1126 ([decision](docs/decisions/proposed/feature/2026-11-26-groups-workspaces.md)): hosted checks, `task cloud` and the `checks` message are removed.

### 2026-11-20 · limitation · `tools.json` freezes JSON Schema keywords that the MCP SDK may write otherwise

- Symptom: `internal/proto/testdata/tools.json` (#165, P-2) freezes each tool's `inputSchema` with a fixed set of keywords: `properties` always there, `{}` for a tool without arguments, `required` only when an argument must be given, and `"additionalProperties": false` (`planning/contracts/mcp-tools.md` section 5). The node's MCP server builds its tools with `github.com/modelcontextprotocol/go-sdk` v1.8.0 and `github.com/google/jsonschema-go`, and it is not shown that they write exactly these keywords: for example `properties: {}` for a struct without fields, or a `$schema` or `title` of their own.
- Cause: the file was written from the contract, before any server code (`internal/proto/tools_test.go`, `toolLists`).
- Status: open until I-1 checks the live registered tools together; the MCP server half is settled in HS-8 (`internal/host/mcp.go`). Every tool registers a hand-written `json.RawMessage` rather than calling `jsonschema.For`, and the server takes the tools' schemas as they are (`emptyToolSchema` when empty), projecting only the per-caller differences of `mcp-tools.md` section 5 (`projectCallerSchema`); each tool's own golden test pins its schema, and `TestToolListsMatchGoldenSchemas` (`internal/host/mcp_test.go`) checks all five session lists of `tools.json` by tool name.

### 2026-11-20 · limitation · The `post` tool lists `handback` for a whole session, not for a hand-back turn

- Symptom: `tools.json` lists `handback` among the kinds of `post` in every session a takeover can reach (the secretary's, review and task sessions), because a tool list is fixed per session, not per turn. `planning/contracts/mcp-tools.md` section 1 allows it only in a hand-back turn, so a model can call it in any turn of those sessions.
- Cause: `tools/list` is read once per session (`internal/proto/tools_test.go`, `toolLists`; `planning/contracts/mcp-tools.md` section 5).
- Status: open, by design: `post` refuses `handback` outside a hand-back turn with `bad_request` (`internal/host/tools_talk.go`, `checkPostKind`, HS-20).

### 2026-11-20 · limitation · A task stopped by its sixth failed review is reviewed again when it is resumed

- Symptom: after the sixth failed review a task is `failed` at step `review`. When an admin resumes or reassigns it, the reviewer is asked to review the same commit again, although that commit has just failed.
- Cause: row 7 of `contracts/task-machine.md` above the limit is row 14 alone, which leaves the step (`internal/home/machine.go:560`); section 2 then gives a task that becomes active at step `review` its `review` item (`internal/home/machine.go:428`). Row 6 is different: failed hosted checks set step `local_check` before they stop the task.
- Status: open. No change, by the coordinator's decision of 2026-11-20: the contract says so explicitly, and it lets a person resume the task with another reviewer for the same commit.

### 2026-11-20 · limitation · A child of a Windows check can start before the job holds the shell

- Symptom: on Windows a check's `cmd.exe` joins its job object only after it has started, so a process it starts in that moment is outside the job and survives the check.
- Cause: Go starts the process before `killCheckTree` assigns it (`internal/host/checks_tree_windows.go`, `newCheckJob`).
- Status: ended by the product-model change MODEL-1126 ([decision](docs/decisions/proposed/feature/2026-11-26-groups-workspaces.md)): the node runs no check commands, and `checks_tree_windows.go` is removed.

### 2026-11-20 · limitation · A key in a UTF-16 file passes the secrets check

- Symptom: a UTF-16LE file holding a private key or a token passes the secrets hard check: with `--text`, Git shows its bytes, and every second byte is NUL, so no pattern matches.
- Cause: the patterns of `local.md` §8 match ASCII text (`internal/host/secret.go`); `checkAddedLines` reads the diff as it comes.
- Status: ended by the product-model change MODEL-1126 ([decision](docs/decisions/proposed/feature/2026-11-26-groups-workspaces.md)): the hard check on a task's diff is removed. The scan of what an AI sends to the group stays (`internal/host/secret.go`); the loss of the diff scan is in the entry "What was reviewed and what is delivered are no longer guaranteed the same".

### 2026-11-20 · limitation · Files hidden by ignore rules can change what a check runs

- Symptom: a file ignored through `.git/info/exclude` or `core.excludesFile`, not through the project's `.gitignore`, is shown by neither `git status` nor `git ls-files`; it can change what a project check does, so a passing check may not describe the commit alone (L5).
- Cause: `run_checks` runs in the task's worktree (`internal/host/checks.go`, `cleanCheckHead`), where such files can exist.
- Status: ended by the product-model change MODEL-1126 ([decision](docs/decisions/proposed/feature/2026-11-26-groups-workspaces.md)): the node runs no check commands; the AI tests the way the project does.

### 2026-11-20 · bug · Two nodes starting together could each remove the other's socket

- Symptom: two nodes that started together on one data directory both ran over a stale `sock`: one removed the other's live socket, and the first one's close then removed the second's (89 of 3000 rounds in the independent review of `internal/node`).
- Cause: the dial of `sock` and its removal were not one step (`internal/node/socket.go`, before this fix).
- Status: resolved. `local.md` section 3, "One node per data directory", now holds the rule: an exclusive lock on `lock` in the data directory, taken by `ListenLocal` (`internal/node/socket.go:38`, `lockData` at `:61`) before the socket is touched.

### 2026-11-20 · limitation · Windows scheduled task expands `%VAR%` in the executable path

- Symptom: when `solodawn.exe` is installed under a directory whose path contains a matching `%VAR%` pair, `Start-ScheduledTask` expands `%VAR%` in `IExecAction.Arguments` (`internal/node/service_windows.go:17`, `taskAction`) and fails to start the binary.
- Cause: Windows Task Scheduler applies `ExpandEnvironmentStringsW` (which has no `%%` escape) to `IExecAction.Arguments`.
- Status: open, off the main line.

### 2026-11-20 · limitation · The start-at-login entries are not run by a real service manager

- Symptom: the unit and acceptance tests check the files and the task definition that `solodawn service install` writes, not a service manager starting the node from them:
  1. a systemd user unit started by a real user manager, with the escaping of `ExecStart` (`$$`, `%%`) and the `Environment="PATH=…"` line (`internal/node/service_linux.go`);
  2. launchd loading the agent with its `EnvironmentVariables` (`internal/node/service_darwin.go`);
  3. the scheduled task running the node headless through `conhost.exe --headless` (`internal/node/service_windows.go`); Windows unit tests only build the action.
- Cause: nothing is installed on the development machine, by the coordinator's decision of 2026-11-20.
- Status: open, covered in CI by `.github/workflows/service-smoke.yml` on `ubuntu-latest`, `macos-latest` and `windows-latest` against the assembled `solodawn` binary (N-4). Still unverified: a real interactive desktop login and machine reboot on each system (I-11).

### 2026-11-20 · limitation · `SHA256SUMS` proves the download intact, not where it came from

- Symptom: the node checks the binary against `SHA256SUMS` from the same release (`local.md` section 9). Whoever can replace the binary there can replace `SHA256SUMS` too, so the check catches a damaged or cut download, not a forged release.
- Cause: the contract has no signature over releases; binaries are not signed (decided). The download itself is https only, follows at most five redirects, each to https, is limited in size, and is given up after 60 s without data, whatever its total time (`internal/node/update.go`, `get`).
- Status: accepted trade-off of the contract.

### 2026-11-20 · bug · go-nat v0.2.0 gets four things wrong in port mapping

- Symptom: against NET-1's gateway (miniupnpd 2.3.9) in spike S-8 ([#134](https://github.com/huanchong-99/SoloDawn/pull/134)), go-nat failed four ways. (1) NAT-PMP returns the external port it asked for, not the one the router granted: it reported 55459 where the router mapped 30000, so a node would publish an unreachable port. (2) NAT-PMP `DeletePortMapping` leaves the mapping in place. (3) NAT-PMP calls ignore the context: with the router silent, a call with a 2 s context had not returned after 10 s. (4) UPnP asks only for random external ports and fails with 606 where the router restricts ports. Discovery also always takes 5 s.
- Cause: in go-nat v0.2.0, `natpmp.go:106` and `:115` discard the mapping result; `natpmp.go:125-127` only forgets the port; `natpmp.go:43` uses `natpmp.NewClient`, which retries for about 128 s (go-nat-pmp v1.0.2 `natpmp.go:34-35`); `nat.go:123-125` draws random ports and `upnp.go:250-275` never tries `AddAnyPortMapping`; `upnp.go:152` waits 5 s. Details in [`spikes/S-8/results.md`](https://github.com/huanchong-99/SoloDawn/blob/spike/S-8-portmap/spikes/S-8/results.md), section 1.
- Status: open, external. Avoided by the coordinator's decision of 2026-11-20: PR-3 uses go-nat-pmp (`NewClientWithTimeout`) and goupnp's `internetgateway2` (IGDv2 first, then IGDv1) directly, not go-nat, and replaces the go-nat pin in `deps_test.go` with these two.

### 2026-11-20 · limitation · pion drops `tcptype` from server-reflexive candidates

- Symptom: in spike S-8 ([#134](https://github.com/huanchong-99/SoloDawn/pull/134)), a node's mapped ICE-TCP listener published as `typ srflx ... tcptype passive` was never dialled: two symmetric NATs with UDP between them blocked and no TURN did not connect. The same endpoint published as `typ host ... tcptype passive` connected in 206 ms.
- Cause: pion ice v4.4.8 `UnmarshalCandidate` gives `tcptype` to host candidates only (`candidate_base.go:962-993`), so a srflx candidate has no TCP type and the agent creates no active candidate for it (`agent.go:1313-1323`).
- Workaround: publish a mapped TCP endpoint as `typ host ... tcptype passive` with the external address and the granted port (PR-3).
- Status: open, external; not reported upstream for now.

### 2026-11-20 · limitation · pion can select the relay while a direct pair is valid

- Symptom: in spike S-8 ([#134](https://github.com/huanchong-99/SoloDawn/pull/134)), 5 of 63 runs with TURN, a port-mapped answerer and a restricted-cone offerer selected the relay after 2.0 s, although direct pairs were valid from about 630 ms. None of 160 runs without a mapping, none of 160 with a mapped offerer, and none with another offerer NAT type did so.
- Cause, from the ICE trace of [run 44704969215](https://github.com/huanchong-99/SoloDawn/actions/runs/44704969215): a check relayed through the answerer's TURN allocation reaches the offerer before that relay candidate is trickled, so the offerer files it as peer-reflexive and validates the pair. When the relay candidate arrives, pion ice v4.4.8 replaces the peer-reflexive candidate but keeps the pair's old priority (`agent.go:1247-1262`, `setPriorityOverride`). That pair stays the best valid one, and the controlling agent nominates only the best valid pair once its type's wait has passed, 2 s for relay (`selection.go:74-84`). Details in [`spikes/S-8/results.md`](https://github.com/huanchong-99/SoloDawn/blob/spike/S-8-portmap/spikes/S-8/results.md), section 2.
- Workaround: the connection works over the relay until its next reconnect; the probe that was to move it to a direct path was cut on 2026-11-21 (`planning/flows/connect.md` B).
- Status: open, external; not reported upstream for now.

### 2026-11-26 · limitation · A7.9 mostly fails on a Mac with its firewall on

- Symptom: on macOS with the application firewall enabled, A7.9 fails most runs at "direct UDP is cut" with no selected-pair change (9 of 12 runs of the fixture of #373 after the merge of #374, 3 of 4 before it). It passes in CI on all three systems.
- Cause: the cut needs the pair from the TURN relay to the node's host candidate, both on the machine's LAN address, and the firewall drops UDP to that address for the unsigned test binary. In a traced failure pion sent 9 checks on that pair and none reached the node; a two-socket UDP program loses every packet the same way. Why some runs pass was not traced.
- Status: open, accepted; CI decides.

### 2026-11-20 · limitation · Three peer scenarios time out on a Mac with its firewall on

- Symptom: "Both nodes can offer over the same two signalling paths", "A replayed offer is not processed" and "Early and repeated candidates" fail with `context deadline exceeded` after 20 s on macOS with the application firewall enabled, on `rebuild` as merged (815c52aa6) and with the audit fixes; the other 77 scenarios pass. They pass on Linux in CI, and on GitHub's `macos-latest` runner (#145).
- Cause, not verified: `features/peer_test.go` makes each node's sockets with `peer.Listen(0, 0)`, which listens on every interface (`internal/peer/sockets.go:22-23`), and the firewall holds incoming traffic to the unsigned test binary on addresses other than loopback. The package's own tests keep the sockets on loopback (`internal/peer/sockets.go:26-27`).
- Status: open. Bind the acceptance nodes to loopback as the unit tests do, or exclude the scenarios on macOS by tag.

### 2026-11-20 · limitation · Rendezvous steps reserve a port by closing it

- Symptom: not observed. The steps listen on 127.0.0.1:0, close the listener and give its port to `solodawn rendezvous`; another process can bind the port in between, and `rvListening` would take that process for the rendezvous.
- Cause: `features/rendezvous_test.go:162-169` and `features/relay_test.go:89-94`; the command cannot take a listener or report the port it chose.
- Status: open, until the command can listen on port 0 and print its address.

### 2026-11-20 · limitation · Three web unit tests time out on a fully busy machine

- Symptom: with 14 busy loops on 12 cores, `web/src/main.test.ts:4`, `web/src/lib/motion.test.ts:29` and `web/src/lib/prefs.test.ts:26` exceeded Vitest's 5 s test limit; one idle run right after `pnpm install` also failed two tests in two files (output not kept). CI passed.
- Cause: each is the first test of its file to import the client's modules afresh inside the test, so their transformation counts against the test's limit.
- Status: open; the gate can fail spuriously while other work loads the machine.

### 2026-11-20 · limitation · A node's socket does not fit under a long HOME on macOS

- Symptom: with a HOME of more than about 60 bytes, `solodawn node` exits with `listen unix <HOME>/Library/Application Support/solodawn/sock: bind: invalid argument`. Seen on macOS with HOME under a session's temporary directory of over 100 bytes.
- Cause: a Unix socket path holds at most 103 bytes on macOS (107 on Linux), and on macOS the socket is 41 bytes below HOME (`internal/node/dir.go:23-24`, `internal/node/socket.go:46`). The system's temporary directory there (`/var/folders/…/T/`, 49 bytes) leaves room for a short name only, so `t.TempDir()` as HOME fails. `internal/node/socket_test.go:289-298`, `features/cmd_local_test.go:29-34` and `web/e2e/nodes.ts` create their directories under `/tmp` on macOS for this reason.
- Effect: none for an ordinary home directory such as `/Users/<name>`.
- Status: open, accepted for tests; a test that sets HOME for a node on macOS keeps it under `/tmp`.

### 2026-11-20 · deviation · End-to-end steps are not in a `.spec.ts` file

- Symptom: the feature-file convention (`planning/steps/parallel-plan.md` section 2, `planning/contracts/conventions.md` section 5) puts a web slice's steps in `web/features/<slice>.spec.ts`. The steps of `features/web_e2e.feature` are in `web/features/e2e.ts`, and the acceptance gate's `vitest run features` does not run them; the `Browser end-to-end` workflow does.
- Cause: `pnpm test` and the gates use `web/vite.config.ts`, whose Vitest default pattern would run a `web/features/*.spec.ts` without the global setup, Go and the browsers that end-to-end tests need. F-01c keeps `web/features/browser-crypto.ts` out of that run the same way.
- Status: open, accepted by the coordinator on 2026-11-20. `web/vite.config.ts` excludes `e2e/**` ([#132](https://github.com/huanchong-99/SoloDawn/pull/132)), so `web/e2e/*.spec.ts`, I-8's `connect.spec.ts` included, run only under `web/e2e/config.ts`.

### 2026-11-20 · limitation · Two workflows download the same pinned browsers

- Symptom: `.github/workflows/e2e.yml` repeats the download steps of `.github/workflows/browser-vectors.yml`: Chrome for Testing 137, Edge 137 and Firefox 136 with their URLs and SHA-256 sums. A change of version has to be made in both files.
- Cause: E-0 added the end-to-end workflow beside F-01c's without changing it.
- Status: open. A composite action that both workflows use can replace the steps later (coordinator, 2026-11-20).

### 2026-11-20 · limitation · The end-to-end harness runs on macOS and Linux only

- Symptom: the global setup of `web/e2e/config.ts` throws on Windows.
- Cause: `web/e2e/nodes.ts` starts every process as the leader of its own process group, stops it with `process.kill(-pgid)`, and finds what a dead test process left with `ps`; Windows has neither.
- Status: accepted for version 1: Windows end-to-end is not part of version 1 (coordinator, 2026-11-26); the end-to-end tests do not run on Windows. Windows is covered by the Go suites on CI and the browsers by the Linux and macOS jobs.

### 2026-11-20 · limitation · Fake CLIs lack invalid-login and exhausted-limit captures

- Symptom: neither scripted CLI covers an invalid-login startup error or an exhausted rate limit.
- Cause: the [macOS driver spike](https://github.com/huanchong-99/SoloDawn/pull/57) captured signed-in sessions and rate-limit updates, not these failures.
- Status: open. Add scripts from real captures before B-14 handles either failure.

### 2026-11-20 · limitation · Web client can paint one frame in the wrong theme

- Symptom: when the theme chosen in the browser (`sd-theme` = `light` or `dark`) is the opposite of the system setting, the page can paint one frame in the system theme while it loads, then switches.
- Cause: `data-theme` is set on `<html>` by the module script after the bundle has loaded (`web/src/lib/prefs.svelte.ts:40-47`); until then `tokens.css` follows the system (`web/src/styles/tokens.css:6`). `web/index.html` has no inline script that sets it earlier.
- Status: open. No inline script for now, by the coordinator's decision of 2026-11-20 (one inline line is the usual fix; no gate measures it and a later Content-Security-Policy may forbid it).

### 2026-11-20 · limitation · Older browsers get an unstyled notice or a blank page

- Symptom: a browser below the supported versions (Chrome and Edge 137, Firefox 136, Safari 17.5; [decision](docs/decisions/implemented/architecture/2026-11-20-browser-baseline.md)) sees the upgrade notice. Chrome and Edge 123 to 136 and Firefox 120 to 135 show it styled. A browser without `light-dark()` (Chrome below 123, Firefox below 120, Safari below 17.5) shows it with default colours, and one without nested rules that start with a type selector (Chrome below 120, Firefox below 117, Safari below 17.2) also loses the layout those rules give; a browser that cannot run the bundle shows a blank page.
- Cause: the notice is mounted by the same bundle (`web/src/lib/boot.ts:32-34`) and styled by the same tokens (`web/src/styles/tokens.css`, every colour a `light-dark()` pair) and nested rules (`web/src/styles/base.css`) as the application, and the build targets the supported versions (`web/vite.config.ts:21-25`), so the CSS ships as written.
- History: until the baseline was set, `web/vite.config.ts` had no `build.target`, so Vite 8.3.8's default (`chrome111`, `edge111`, `firefox114`, `safari16.4`, `ios16.4`) applied, and Lightning CSS, which minifies the CSS for the build target, rewrote every `light-dark()` pair into two custom properties and flattened the nesting. The built notice was then styled down to those versions; only the development server served the CSS as written, which is what the first version of this entry described.
- Status: open, accepted by the coordinator on 2026-11-20 as a known limitation; restated the same day when the coordinator chose to ship the CSS as written.

### 2026-11-20 · limitation · Safari deletes the device key after seven days without a visit

- Symptom: in Safari on macOS and iOS, when the person does not interact with the SoloDawn page during seven days of Safari use, Safari deletes the origin's IndexedDB, and with it the device key once the identity module keeps it there (F-01b). The browser then starts as a new device and has to be paired again. A web app added to the home screen keeps its own count and is not affected; Lockdown Mode disables IndexedDB altogether.
- Cause: WebKit's tracking prevention deletes all script-writable storage of a site after seven days of browser use without interaction with it ([WebKit blog](https://webkit.org/blog/10218/full-third-party-cookie-blocking-and-more/)). Not a SoloDawn defect; [decision](docs/decisions/implemented/architecture/2026-11-20-browser-baseline.md), Consequences.
- Mitigation: F-13 shows the iOS add-to-home-screen hint. F-04 asks for persistent storage (`navigator.storage.persist()`) once the device is enrolled, after pairing; Safari grants it by heuristics such as being opened as a home screen web app ([WebKit blog](https://webkit.org/blog/14403/updates-to-storage-policy/)). `loadDevice` does not ask on first run, because Firefox answers the call with a permission prompt (`dom/quota/StorageManager.cpp`, `PersistentStoragePermissionRequest::Start`, asks through `nsContentPermissionUtils::AskPermission`; MDN, [Storage quotas and eviction criteria](https://developer.mozilla.org/en-US/docs/Web/API/Storage_API/Storage_quotas_and_eviction_criteria)).
- Status: open, accepted as a limitation of the platform.

### 2026-11-20 · limitation · Safari's verify() fails every signature over an empty message

- Symptom: WebKit 27.2 (Playwright build, macOS) returns false from `crypto.subtle.verify` for a valid Ed25519 signature over an empty message, RFC 8032 test 1 included; its own signature of the empty message is valid (OpenSSL verifies it). Chromium 159 and Firefox 159 verify both.
- Cause: in WebKit's `Source/WebCore/PAL/pal/crypto/CryptoKitShim.swift`, `isValidSignature` returns false when the data is empty. No bug report found; the first affected Safari version is unknown.
- Effect: none on the protocol; envelopes and challenges have non-empty context prefixes. F-01c asserts WebKit's false result on RFC 8032 test 1 explicitly, separately from the protocol vectors.
- Status: open, external.

### 2026-11-20 · limitation · Random ids are not checked in rows, most message bodies and the frames that read a group

- Symptom: `contracts/conventions.md` section 2 makes any text that is no random id a wrong type in every field that holds one. `proto.Decode` and `decode` check it for `n` and `g` of a signed payload, which is what the envelope check reads (`contracts/envelope.md` section 3, check 3); for `conn` of a signalling payload, which a node drops otherwise (`internal/peer/signal.go:104-109`) and so does the browser (`web/src/lib/rendezvous.ts:175-180`); and, since unit P-1, for every id in the payload of a home or host command, in the payload of an inbox item, and for `g` of `inbox` and `ack`. Of the bodies a `post` carries, a review's is checked too: its `task` is refused when it is no id, because `ReviewSigned` is a type of P-1. The other bodies are not yet: `brief`, `approval`, `decision` and `call` take any text for the ids they hold (`requester`, `assignee`, `reviewer`, `replaces`, `task`, `ai`, `to`), although the `thread`, `re` and `mentions` around them are checked. Nor is a row (`g` of a row, `thread`, `re`, `task`, `mentions`, `assignee`, a row's own `id`), nor `g` of `sync`, `reset`, `rows` and `fetch`.
- Cause: only those fields have the type (`ID` in `internal/proto/envelope.go`, `command.go` and `inbox.go`; `Id` in `web/src/lib/proto.ts`). The row types and the body types a row shares with its `post` were frozen before the type existed, and code outside `internal/proto` builds them from plain strings (`internal/store`, `internal/host`), so a change of their field types is a change in those packages too. For rows, `contracts/rows.md` says "random ids unless stated" without saying per field which values stand beside the id (the `by` of a message row may be `"system"`, a device row's `id` is `<user id>:<ver>`).
- Status: open, by the coordinator's decision of 2026-11-20: they stay as they are in P-1. A home looks up every id that a body names (`contracts/commands.md` 1.3), so a text that is no id names nothing there. The rows and bodies get the type in one change with `internal/store` and `internal/host`.

### 2026-11-20 · limitation · The shared wire test does not show where one reader is stricter about a text

- Symptom: `internal/proto/testdata/wire.json` lists, per example, the texts that are random ids, and both test suites check that such a text refuses a value that is no id. The reverse is not checked: that a text which is not listed takes any text on both sides. So a value rule that only one reader has passes unnoticed. Two are known, both in the web client alone: `conn` of a signalling payload is an `Id` (`web/src/lib/proto.ts:41`) where Go has a plain string (`internal/proto/envelope.go:27`), and a pairing room is 32 lower-case hex digits (`web/src/lib/proto.ts:130`) where Go has a plain string (`internal/proto/rendezvous.go:67`). The independent review of P-1 probed for more and found none.
- Cause: the shared changes are those the Go tags can state (left out, `null`, the type of a value, a fraction, an id, a slug); a rule a schema adds to a text (a pattern, a literal) is in no tag. The names a schema gives `t`, `op`, a post's `kind` and an event's `code` are outside the agreement on purpose (`contracts/conventions.md` section 5).
- Status: open, accepted by the coordinator on 2026-11-20 with no code change. Both known cases behave alike in the end, because the Go code that uses the field checks it itself: `internal/peer` refuses a `conn` that is no id (`internal/peer/signal.go:104-109`), and the rendezvous a room that is not 32 lower-case hex digits (`internal/rendezvous/room.go:10-19`).

### 2026-11-20 · limitation · A message cut short by a closing transport is forwarded as if it were complete

- Symptom: when a client's transport closes in the middle of a WebSocket message, the rendezvous can read the bytes received so far as one complete message. This happens between two fragments of a fragmented message, and also inside one unfragmented frame when the transport closes at the boundary of a read. If those bytes happen to form a valid frame, it is handled like any other: a `to` is forwarded as `from`. `planning/contracts/rendezvous.md` section 2 says that a frame the server cannot read closes its sender.
- Cause: `github.com/coder/websocket` v1.8.15 takes a plain `io.EOF` for the end of the message (`read.go:447-449`, `msgReader.Read`). Between fragments, the header of the next fragment fails with `io.EOF` when the transport has closed (`readFrameHeader`, `frame.go:52-58`). Inside one frame, `Conn.Read` collects the payload with `io.ReadAll`, and when no byte arrives for its next read, `io.ReadFull` returns `io.EOF`, not `io.ErrUnexpectedEOF` (`readFramePayload`, `read.go:282`). `Conn.Read` then returns the bytes so far without an error, to `internal/rendezvous/server.go:179`.
- Why it is harmless: what is forwarded is a complete, valid frame that the same authenticated sender could have sent on purpose; the sender's connection is already gone; and the receiver checks the signed envelope itself (`planning/contracts/envelope.md` section 3).
- Status: open, accepted by the coordinator on 2026-11-20 with no code change; the in-frame case was added after the rendezvous audit of the same day. Revisit if the library changes, or if the server ever acts on a frame beyond forwarding it.

### 2026-11-20 · limitation · A frame dropped for a slow receiver can leave it with stale state

- Symptom: when a receiver's queue of 128 frames is full, the rendezvous drops the next frame for it and closes neither connection (`planning/contracts/rendezvous.md` section 2). A dropped `from` or `room` is covered by its sender's offer timeout and back-off. A frame the server itself generates has no sender to send it again: after a dropped `presence` a watcher shows the wrong online state until that device's next change or the next `watch`; after a dropped `joined` a pairing member waits until it joins again; a dropped `offline`, `ice` or `quota` leaves that request unanswered.
- Cause: `client.sendFrom` drops a frame when the queue is full, holds 16 frames of its sender, or holds 32 from senders off the receiver's watch list (`internal/rendezvous/server.go:129-167`, limits at `:81-82`, queue size at `:92`).
- Status: open. Coalescing the server's own frames is not built, by the coordinator's decision of 2026-11-20.

### 2026-11-20 · limitation · The rendezvous challenge is not bound to a server

- Symptom: a malicious rendezvous, or any node that serves `/rv`, can hand a device that connects to it the nonce of another server's `challenge`, and answer that server with the device's signature. It is then signed in there as the device: the device's own connection there is replaced and closed, and the attacker receives the frames sent to the device, which shows who signals it and when, and can watch other devices under its id. It also gets the device's TURN credentials in `ready` and can spend the device's daily relay quota, cutting off the device's own relayed connections.
- Cause: the answer signs `"solodawn/rv/1\n"` and the nonce only, nothing that names the server (`planning/contracts/rendezvous.md` section 2.1; `internal/identity/key.go:75-82`, checked at `internal/rendezvous/server.go:200-210`).
- Status: open design issue, owner the coordinator, to be designed after the quota reset. The challenge format is unchanged until then.

### 2026-11-20 · limitation · A late `offline` can retire a newer unanswered offer

- Symptom: a node that offers again soon after an offer to an absent device can retire the new offer at once, as if every path had reported the target offline, and wait out a back-off before the next one.
- Cause: the rendezvous answers each `to` it cannot deliver with `{"t":"offline","k":…}`, which names the target but not the `conn` (`planning/contracts/rendezvous.md` section 2.2). `internal/peer/path.go:140-145` passes every such report to `Peer.Offline`, which takes it for each unanswered offer to that target and removes the reporting path (`internal/peer/unanswered.go:38-43`). A report for an earlier frame (the earlier offer or one of its candidates) still on its way when the next offer leaves therefore counts against the new one. The back-off of at least one second makes this rare; a presence report can end the back-off sooner (`planning/flows/connect.md` step 16).
- Status: open, accepted by the coordinator on 2026-11-20 with no contract change.

### 2026-11-20 · deviation · No receiver check or shared vector for a beacon yet

- Symptom: `planning/contracts/envelope.md` section 3, checks 4 and 5, now gives a beacon (`planning/contracts/rendezvous.md` section 4) a third set of receiver checks: no `to`, and a two-minute window. The code and the vectors know two kinds only. `OpenSignal` requires `to`, and `OpenCommand` allows 24 hours. So neither reads a beacon as the contract says.
- Cause: the rule was settled after B-02c and F-01d (#136). The identity code has the two kinds only: `internal/identity/envelope.go:30` and `:37`, and `web/src/lib/identity.ts:185-195`. `planning/contracts/vectors.md` section 3 lets `received` be `command` or `signal` only, and `internal/proto/testdata/crypto.json` has no beacon case.
- Status: open. PR-2 adds a beacon case to `received` in `vectors.md` and `crypto.json`, and a third envelope kind in both identity implementations, before it reads beacons.

### 2026-11-20 · deviation · The web client does not handle `full` yet

- Symptom: a browser that joins a pairing room that already has two members is answered `{"t":"full","room":"<room id>"}` (`planning/contracts/rendezvous.md` section 2.2), but the web client drops the frame without telling the page, so a third browser keeps waiting for `joined`.
- Cause: `full` is not among the frames the web rendezvous client reads (`web/src/lib/rendezvous.ts:21`); a frame that does not parse is ignored (`web/src/lib/rendezvous.ts:165-168`). The server sends it from `internal/rendezvous/room.go` (`join`).
- Status: open, owner W-2.

### 2026-11-20 · limitation · A8.4 does not run on Windows

- Symptom: on Windows the A8.4 scenario (`features/rendezvous_acme.feature`) cannot start its rendezvous: the binary exits with `lookup rv.localhost: no such host` (CI run 44716573698, `test (windows-latest)`). The scenario is tagged `@not-windows`, which the acceptance runner leaves out on Windows (`features/acceptance_test.go`, `tags`); Linux and macOS run it.
- Cause: the Windows resolver answers no name under `.localhost` (RFC 6761), and the rendezvous looks up its `--domain` for the relay address at startup (`internal/rendezvous/relay.go`, `relay.generator`, `net.LookupIP`). The certificate library refuses `localhost` itself, a name without a dot, so the test cannot use it instead.
- Status: open. A hosts file entry for `rv.localhost` and `turn.rv.localhost` on the Windows runner would let it run; that changes the machine, not the test.

### 2026-11-20 · limitation · A TURN alias links one device's addresses for the life of the process

- Symptom: the alias in a TURN username stays the same for a device as long as the rendezvous process runs, so whoever sees its TURN traffic (the network path to the server, or a relay peer) can tell that allocations from different IP addresses belong to one pseudonym. The alias does not reveal the device id.
- Cause: the alias is an HMAC-SHA256 of the device id under a secret drawn once per process start (`internal/rendezvous/relay.go:79-86`, secret at `:94`), and the daily quota is kept per alias, so it cannot change within a run.
- Status: open, accepted by the coordinator on 2026-11-20 after the rendezvous security review (F8).

### 2026-11-20 · limitation · Limits by address let one NAT crowd its users and a spoofer fill a victim's places

- Symptom: the relay's 128 allocations and the shared TLS port's 256 connections are counted per IPv4 address or IPv6 /64 (`planning/contracts/rendezvous.md` sections 3 and 6). Devices behind one NAT or carrier-grade NAT share one count, so busy neighbours can leave a device without relay. A sender that can forge UDP source addresses can, with TURN credentials of its own (any device key it makes gets them), create allocations from a victim's address without seeing the answers; each holds one of the victim's places for its lifetime, up to an hour, and blind refreshes can keep it.
- Cause: the server can tell sources apart only by their address (`internal/rendezvous/limits.go:41-50`, `source`; `internal/rendezvous/tls.go:192-205`, `admit`), and pion's nonces are not tied to the client's address (pion/turn v5.1.3 `internal/server/util.go:59`, `NonceHash.Generate`), so a nonce fetched from the sender's own address works for forged ones.
- Status: open, accepted by the coordinator on 2026-11-20 after the rendezvous security review (F9).

### 2026-11-20 · limitation · The host gives no notice when something `host.info` shows has changed

- Symptom: `planning/contracts/datachannel.md` section 7 pushes `host.info` whenever one of its fields changes. `open`, and `state`, `task` and `queued` of each AI in a group, come from the host's sessions, and the host tells nobody when one of them changes: whoever builds `host.info` can only read the session table again (`Sessions.All`, `internal/host/session.go:341`).
- Cause: the seam of the host (HS-0, #159) has no hook on a change of the session table (`internal/host/session.go:115`); which changes matter depends on what `host.info` is built from, which is N-3's.
- Status: open. N-3 decides whether it needs a notice or reads the table again, and asks the owner of `internal/host/session.go` for the hook if it does (coordinator, 2026-11-20).

### 2026-11-20 · limitation · A commit that fails leaves the session table ahead of the store

- Symptom: not observed. If the store fails to commit a transaction whose function succeeded (an I/O error at `COMMIT`), the session table in memory keeps the changes the store lost, until the node restarts and reads the table again.
- Cause: `Sessions.Write` puts a change into memory inside the transaction and takes it out again only when the transaction's function returns an error (`internal/host/session.go:180`, `Write`). The store reports a failed commit only after the next transaction may have started (`internal/store/store.go:76-86`, `:88-92`), and has no hook that runs before it.
- Status: open, accepted for step 1. A store hook that runs on the writer after a failed commit would close it.

### 2026-11-20 · limitation · Cards and home-written rows are one line each in the chat main line

- Symptom: in the main line of the web client a brief, an approval, a decision, a summary and a call are one outlined line that names the kind and the card's subject, with the dawn edge and "Waiting on you" when the card waits for the reader. It has no buttons: the reader cannot confirm, approve or choose there. A row the home wrote (`task`, `system`) is a quiet centred line; a `system` row shows its `code` as it is on the wire, not a sentence.
- Cause: F-07 is the row layout and text messages (`planning/steps/step-1-frontend.md`). The line is drawn in `web/src/components/chat/Message.svelte`; `subject` in `web/src/lib/groups.svelte.ts` gives the subject.
- Status: open. The card units replace the line (U-4 approvals, U-7 brief, decision and summary, U-8 task, U-9 the wording of `system` rows, U-10 calls).

### 2026-11-20 · limitation · In Safari the main line moves when a message arrives while the reader is scrolled back

- Symptom: not observed, expected from the engine. The main line is a reversed scroller, whose offset is counted from its bottom. A message that arrives at the bottom while the reader is scrolled back pushes what they read up by its height, unless the browser anchors the scroll position. Chrome and Firefox do; Safari has no scroll anchoring.
- Cause: `web/src/components/chat/Chat.svelte`, `.scroller` (`flex-direction: column-reverse`). The reversed scroller is what keeps a page added above from moving anything, in every browser.
- Status: open. No correction in script was added, because it could not be tried in Safari here.

### 2026-11-20 · limitation · A device in the logs of two people is read as the device of the lower user id

- Symptom: nothing stops a person from adding to their own log a device that another person's log already holds. A home then reads that device's commands as coming from whichever of the two people has the lower user id (`planning/contracts/commands.md`, "Who"). A member could so turn the commands of another member's device into their own, which that device's person did not ask for.
- Cause: `(*State).user` walks the logs in the order of their user ids and returns the first that holds the device as valid (`internal/home/check.go:92`). `planning/contracts/envelope.md` section 4 says nothing about one device in two logs.
- Status: open, without effect in step 1, where a group has one person. Step 2, which adds people, has to settle it.
- 2026-11-26: `watchDeviceUser` (`internal/host/watch.go:107`) treats a revoke of the device in anyone's log as blocking it for a watch, unlike the home's check, which takes the first log that holds it as valid (`internal/home/check.go:92`). Accepted by the coordinator.

### 2026-11-20 · limitation · A member of another group on the same home is answered `unknown_device`, not `forbidden`

- Symptom: check 2 of `planning/contracts/envelope.md` section 3 knows a device of "a member of a group it homes". A command for group A from a device whose person is a member only of group B, homed on the same node, is answered `unknown_device`, where `forbidden` is the code for a known signer that may not do this.
- Cause: `(*State).known` asks the logs of one group's `State` (`internal/home/check.go:88`), which hold the node owner's log and those of that group's people.
- Status: open, without effect in step 1, where every group on a node has the same one person. For step 2, by the coordinator's decision of 2026-11-20 after the review of #160.

### 2026-11-21 · deviation · From 768 px the top bar of an open chat names the page, not the group

- Symptom: at 768 px and wider, the shell's top bar over an open chat says "Chats", and the chat's own head with the group's name, its members and its home is below it. The design has one bar with the group's name (`web/design/chat.html`).
- Cause: the top bar's title is the page's (`page.<page>` in `web/src/App.svelte`), and no mounting point lets a page put its own head into the shell's bar (`web/src/lib/mounts.svelte.ts`). Below 768 px the chat's route has the whole screen, so its head is the only bar there.
- Status: open, for the lane UI-SHELL (U-15, U-16) or a follow-up of U-0.

### 2026-11-21 · limitation · The texts of the development-only states sheet are in the build

- Symptom: the build carries the texts of the area `states` (about 0.5 KiB), although only a development server opens `#/states`.
- Cause: `web/src/messages/files.ts` takes every message file eagerly, and the loader merges them all (`web/src/messages/texts.ts`).
- Status: open, accepted with U-0 (#170).

### 2026-11-23 · bug · FLAKY-29: Qwen A4.9 timed out waiting for its first post

- Symptom: step 4 (`features/s3_driver_limits_test.go:77`) failed once in CI with `timed out waiting on home pushes`.
- Cause: not established. The log did not record the environment or whether the post reached MCP. The fake CLI defect fixed in #377 needs a variable that nothing sets, so it is not the cause. Whether the teardown race of #363 was is not known.
- Status: open; the next occurrence needs the host and MCP lines of [diagnostics](planning/contracts/diagnostics.md). The lost event stream of 2026-11-26 (Resolved) needs the proxy of `features/editcli`, which A4.9 does not use.

### 2026-11-26 · intermittent · A9.4 setup once failed with a 401 from the node's own rendezvous dial

- Symptom: seen once in a full acceptance run on 267c4adf3 on macOS under load: node Q1 never opened its socket; stderr `failed to WebSocket dial: ... got 401`.
- Cause: not established; likely the same as the 404 at startup fixed by "Node: a port taken on 127.0.0.1 counts as taken" (the node's dial to 127.0.0.1 reached another process's rendezvous).
- Status: open until it is seen again with the new diagnostics or not seen in the next full runs.

## Resolved

### 2026-11-27 · bug · The acceptance suite's clean-up list raced when two people joined at once

- Symptom: `go test -tags acceptance -race` on "People who type one invitation code meet the home in its room and join, at once (A9.47)" failed one run in three with `WARNING: DATA RACE` (`testing.go: race detected during execution of test`); the two writes were the two browsers' clean-ups.
- Cause: the function `afterScenario` returns appended to a plain slice (`features/acceptance_test.go:136`, before the fix), and `twoJoin` (`features/node_pair_code_test.go:458`, `twoJoin`) opens one browser (`newPeerBrowser`, which registers its clean-up) in each of two goroutines.
- Status: resolved in unit M3-7: the list is guarded by a mutex in `afterScenario`; `-race -count=3` of the invitation-code scenarios passes.

### 2026-11-26 · bug · Scroll-back stopped after a jump to the top: the edge's observer read its oldest entry

- Symptom: CI run 45845149651 (Chrome 137, PR #422), `web/e2e/jump.spec.ts` "keeps a card 160 messages back in place at 390 px with reduce motion": no page was asked for after the jump (`waiting()` stayed 0). On `origin/rebuild` with Playwright Chromium 1248, 390x844, reduced motion: 2 of 240 runs failed, 22 of 240 with a `requestAnimationFrame` loop in the page.
- Cause: `web/src/components/chat/Chat.svelte:69` set `atEdge` from the first entry of the `IntersectionObserver` callback. The observer computes once per frame and tells later, all entries since it last told, oldest first. Sequence in the failing runs: the chat mounts and creates the observer; the read draws the newest 50 messages; frame N computes `false` (the edge is 1286 px above the view); before that is told, `Group.seek` answers and the jump scrolls to the card, the oldest held message, so the line is at its top; frame N+1 computes `true`; one callback gets `[false, true]` and set `atEdge` to `false`. The observer, already at `true`, never told again, so `Group.older` was never called. Reduced motion jumps at once, in one frame; a smooth jump keeps scrolling over later frames. Batch J (`batch/1126-j`) plays no part: it happens on `origin/rebuild`. The "stale observer in Chromium" of the entry "A jump to a card older than the messages read drifts out of view" was this: with the fix and the hold scrolling on every change of the line's size, 720 runs of the 390 px case (480 with a `requestAnimationFrame` loop) and 60 rounds of `jump.spec.ts` with the loop passed; the hold's check stays because without it a smooth jump ended in one step (1 scroll event instead of 35).
- Status: resolved on branch `fix/jump-flake`: `atEdge` follows the last entry. Test: `web/src/components/team/chat.test.ts` "fetches the page before by the last of the changes the observer tells at once".

### 2026-11-26 · bug · Pairing that cannot connect gave no reason, and `pair` outlived the code

- Symptom (local smoke test of v3.0.0-rc.1, findings F2 to F5, a host firewall blocking the direct connection): the page showed "establishing a direct connection" until "expired"; `solodawn pair` printed only `pairing secret expired`, up to 4 minutes after the page had expired; stopping the node printed `context canceled`. The onboarding note said that the node keeps trying direct (`pairing.status.relay_note`). `install.md` did not say that pairing installs the login service.
- Cause: the browser's peer retried a connection that ICE failed without telling the page (`Peer.#ended`, `web/src/lib/peer.ts`); `waitPair` returned `errPairExpired` whatever it had seen, and `ctx.Err()` on a stop (`internal/node/pair.go`); role `b` started its own 10 minutes when it joined, since nothing carried the code's expiry; the note predates the cut of the relay-free probe (as `nodes.relay.body` below).
- Status: resolved on `fix/pairing-reasons`: the page shows `pairing.error.unreached` (en, zh) when ICE fails an attempt and keeps it at expiry; `pair` says `no connection to the browser could be made; check a firewall on this computer or its network` when the browser was found but never connected, and `the node stopped`; `pair` carries `exp` (`contracts/envelope.md` section 6) and role `b` ends with the creator; the note and `install.md` are corrected.

### 2026-11-26 · bug · Every Go binary shares one macOS firewall identity, so a block for a test binary silently blocks `solodawn`

- Symptom: in the smoke test of `v3.0.0-rc.1` on the Mac mini, macOS never asked about `solodawn` and dropped every incoming flow to it: the firewall log shows `Handle flow with processPID: … and bundleid: a.out`, then `Found known filter app by signing identifier: … processName: peer.test, … designatedRequirement: (null), bundle id: a.out, filterAction: 2` and `return known verdict: 2`. The entry was an old "Block incoming connections" for `…/go-build540054719/b308/peer.test`. A copy re-signed as `org.solodawn.smoke1126` still showed `bundleid: a.out`.
- Cause: the firewall keys an app by its code-signing identifier (the flow's `bundleid`), and keeps no designated requirement for a linker-signed binary, so one entry matches every binary with that identifier. Evidence in the same log: Playwright's `chrome-headless-shell` (ad-hoc, `Identifier=headless_shell`, no Info.plist) shows `bundleid: headless_shell`, and Firefox's `plugin-container` (`Identifier=plugin-container`, Info.plist `org.mozilla.plugincontainer`) shows `bundleid: plugin-container`. Go's linker signs every darwin binary ad hoc as `a.out` (it links to a file of that name), so every `go build` and `go test` binary, and the released `solodawn` up to `v3.0.0-rc.1`, is the same app to the firewall: one Allow or Block applies to all of them. Why the re-signed copy stayed `a.out` was not proved; re-signing keeps the Mach-O `LC_UUID`, and the firewall maps flows to executables by that UUID (`processPathFromUUID`, `Updating app rule to contain the effective UUID` in `socketfilterfw`). It also explains the local firewall failures of test binaries in the entries of 2026-11-22, 2026-11-20 (three peer scenarios) and A7.9: in the same log the test binaries (`features.test`, the scenarios' `solodawn.exe`) are matched to the `peer.test` entry with verdict 2.
- Status: resolved for release builds on branch `ci/macos-identity`: `.github/workflows/release.yml` builds on macOS and links the darwin pair with Apple's linker, embeds `cmd/solodawn/Info.plist` (`CFBundleIdentifier` `org.solodawn.node`) as `__TEXT,__info_plist`, and signs them ad hoc, which takes the identifier from that Info.plist; the job fails unless `codesign -dv` shows `Identifier=org.solodawn.node`. The Linux and Windows binaries are unchanged (the four of `v3.0.0-rc.1` were rebuilt byte for byte on macOS). Development builds from `go build` and test binaries keep `a.out`. Not proved on the firewall itself: that needs a listening binary, which makes macOS ask; the owner's next run of checklist step 2 is the proof. macOS may ask again after each update: see the limitation "macOS may ask about `solodawn` again after each update".

### 2026-11-26 · bug · Five of the six release binaries recorded a modified work tree

- Symptom: `go version -m` of the `v3.0.0-rc.1` binaries shows `vcs.modified=true` and a module version ending in `+dirty` for every binary but `linux_amd64`.
- Cause: the build loop of `.github/workflows/release.yml` writes each binary into `dist/`, which Git did not ignore, so every build after the first saw an untracked file.
- Status: resolved on branch `ci/macos-identity`: `/dist/` is in `.gitignore`, and the release job fails unless `go version -m` of every binary shows `vcs.modified=false`.

### 2026-11-26 · bug · A9.27 failed once on Linux: P2 held a later member row than the export

- Symptom: CI run 45794186246 (`test`, Linux, PR #421, `batch/1126-i` 5ebd10ee7), A9.27 `Then P2 is home and has the exported rows`: `group rows differ`. One row differs: the secretary's member row, exported as seq 23 with `"path":"local"`, was seq 32 with `"path":"direct"` on P2. macOS and Windows of the same run passed.
- Cause: the fixture, not the product. `group export --to P2` makes P1's host dial P2 at once (P1's `home.dial` to P2 in the run's log is in the same millisecond as P2's host leaving P1); once the import installs the group, P1 attaches and its host reports member status with the path it now has, `direct` (`hevUpdatedAI`, `internal/home/cmd_hostev.go:170`), as it should. P2 serves the group from the moment the import ends, so `compareRows` (`features/s2_export_test.go:414`) asked for the file's rows byte for byte at a moment when any connected host may already have written a later version. On the Linux runner P1's status (P2 `cmd.reply ... t=member/status`) landed before the step read P2's database. Batch I (pairing, release build) is not involved: it touches neither the home nor the hosts; the same race is on `rebuild`.
- Status: resolved on branch `fix/a927-rows`. `compareRows` replaces each row whose seq is above every seq of the export by the export's row of the same type and id (`s2RowsAtCut`), so a later version of an exported row passes, while a changed row with an exported seq, a missing row or a new row still fail. It is shared by A9.27, A9.28 and the copy check on P1, where hosts write the same way. Proof: a probe that waits in `checkRows` until P2 holds the secretary's `direct` status failed 3 of 3 with the old comparison, with the CI's diff, and passed 3 of 3 with the new one.

### 2026-11-26 · bug · FLAKY-34: A7.4 failed once: the test browser dropped the interactive CLI's first output

- Symptom: CI run 45758291863 (`test (macos-latest)`, `rebuild` ed742b4e8), A7.4 on the fifth Codex takeover: `timed out after 15s waiting for "\"ready\":" in the pty output; saw 1:` with nothing received. The fake CLI log ends with `start codex` (the CLI ran); the goroutine dump shows the host's `pumpOut` attached to that PTY and waiting to read (`internal/host/pty.go:286`), with nothing left to send. The same code passed on the PR runs and in 8 runs under CPU load on the Mac.
- Cause: the fixture, not the node. The test browser opened `pty:<g>:<member>` with `CreateDataChannel` and set `OnMessage` after it returned (`features/m7_test.go` `openPTY` and `newM7PTY`, before this fix lines 741-745 and 155-167). On a connected PeerConnection pion opens the channel and starts its read loop inside `CreateDataChannel`, and drops a message that arrives while no `OnMessage` handler is set (`pion/webrtc/v4@v4.2.26/datachannel.go:319-332`, `peerconnection.go:2728-2730`). The CLI printed `{"ready":…}` at start, so the node sends it the moment the channel opens (flows/takeover.md step 7); when the test goroutine was descheduled for the node's round trip, the line was lost. A 300 ms sleep between `CreateDataChannel` and `newM7PTY` failed A7.4 every time with the same message and the same fake CLI log. `dispatchWorld.channel` (`features/peer_dispatch_test.go`), which A9.20's owner terminal uses and which also waits for the CLI's first output, had the same window. A browser cannot lose these bytes: it sets `onmessage` in the same task, before any event runs.
- Status: resolved on branch `fix/a74-flake`. Both fixtures open their channels with `openBrowserChannel` (`features/peer_dispatch_test.go:233`): the PeerConnection's API detaches data channels (`browserAPI`, `:266`), so a message stays in the SCTP stream until the channel's reader takes it, and opened and closed follow the detached reader. With the same 300 ms sleep A7.4 passes.

### 2026-11-26 · bug · A jump to a card older than the messages read drifts out of view

- Symptom: a reminder opens a task whose card is far back and was not read (160 messages): its thread opens and has the focus, but after the gap to the newer messages is paged in, the card is not where the jump put it. Measured with Playwright Firefox 1555 and Chromium 1248, 1280x720, real pages through a fake `fetch`, card 160 back fetched with `seek`; the thread open or not makes no difference. A card that is held, or 25 messages back, stays in place in both.
- Cause: the card is fetched alone (`Group.seek`), so the pages of scroll-back (`Group.older`, `web/src/components/chat/Chat.svelte`) are inserted below it, in a reversed scroller whose offset counts from the bottom, and the browser's scroll anchoring was all that kept it in place. Shared condition: the card is near the top of what is held, so `edge` (`Chat.svelte:68-73`, 600 px margin) stays in reach and the pages follow at once, until the rows above the card fill 600 px. Two causes, neither is Firefox's own:
  1. Pages that arrive while a row still rises: `Message.svelte:97-99` runs `rise`, a transform (`base.css:1263`), on every newly drawn message, the fetched card too. A running transform between the anchor and the scroller turns scroll anchoring off for that layout, in both engines, and the page inserted below is not compensated. With 10 older held rows and pages 90 ms apart: Firefox -1301, Chromium -1428 (374 is right).
  2. A page that goes past the card (deterministic, both engines): the first row in view is the anchor, a held row above the card, the day or the edge. Messages older than the card are inserted between it and the card, so the card goes down: with 40 messages before it 1473 to 1495, with 120 3713 to 3735. Safari has no scroll anchoring at all, so there every page below the card moved it up.
- Status: resolved on branch `web/jump-anchor`. `jumpToMessage` (`web/src/lib/jump.ts`) no longer leaves the card to the browser's anchoring: in the main line it scrolls the card to the middle, as far as the line reaches, turns `overflow-anchor` off on the scroller, and on every change of the line's size (a `ResizeObserver`) scrolls it back to the same offset from the scroller's top, measured from layout offsets, which a running `rise` does not move; a smooth jump still on its way is sent to the new place instead. It ends on any scroll that is not the hold's own once the card has landed (keys, scrollbar, focus, assistive technology, find; wheel and touch also during the smooth scroll), when the person sends a message, jumps elsewhere or goes to the latest message; a card the jump puts at the end of the line is not held, so the line keeps following new messages. The hold never scrolls while the line has not moved the card, so the first call of its `ResizeObserver` does not end a smooth jump at once. This entry said before that a scroll in that frame left `edge`'s observer stale in Chromium; it was not stale: `Chat.svelte:69` read the oldest of its entries (entry "Scroll-back stopped after a jump to the top"). `rise` stays: it no longer matters. `web/e2e/jump.spec.ts` jumps to a card 160 back with 39 before it, through a reminder's address, and lets the three pages go after the jump: before the fix the card went down by 1134 px (1280x720) and 1144 px (390x844) with the page that goes past it, in Chromium, with and without reduced motion; after it, by 1 px at most.
### 2026-11-26 · bug · A Qwen Code task session of A10.3 lost its event stream before the first turn

- Symptom: `acceptance-race` on Linux, run 45739795462 on `rebuild` 267c4adf: the qwen row of A4.4 in `features/s3_drivers.feature` (example #19) failed with `step 1: step 2: timed out after 10s waiting for home pushes`. In `nodes.log` the CLI process ended 16 ms after `proc.start` and before `turn.begin` (`proc.exit why=""`), the turn failed with `Post ".../session/s3-session/prompt": connection refused`, and `fakecli.log` holds `start qwen s3-session` without its `exit`: the host killed a CLI whose script was still running. Reproduced once on Linux under load (the level 3 half of the same case), with the host's read of the stream ending in `unexpected EOF`, then `signal: killed`.
- Cause: both server drivers sent the JSON `null` as the body of every request without a value, the `GET` of the event stream included (`qwenProcess.request`, `internal/host/qwen.go:164`; `openCodeProcess.request`, `internal/host/opencode.go:195`). In this case the CLI is `features/editcli`, which puts a Go `httputil.ReverseProxy` before the fake server. The proxy's transport copies the 4 bytes, then reads the request body once more for its end; when the fake's answer comes first, the proxy writes its headers and the Go server closes the not yet finished request body, so that read fails with `http: invalid Read on closed Body`, the transport closes its connection to the fake, the proxy aborts the stream, and `qwenProcess.run` (`internal/host/qwen.go:209`) ends and kills the CLI. Isolated with a client, a `ReverseProxy` and an SSE server under `-race`: 2 and 3 streams of 3000 cut with the `null` body, none of 3000 without it. FLAKY-29 (A4.9) runs the plain fake, which has no proxy, so this does not explain it.
- Status: resolved. A request without a value has no body (`wireBody`, `internal/host/serve.go`); `TestServedCLIRequestWithoutValueHasNoBody` fails without it (`GET /events 4 null`).

### 2026-11-26 · bug · At level 1 a request of Codex, OpenCode or Qwen Code produces a card

- Symptom: a level 1 session of a CLI that asks on its connection (`ApprovalConn`) gets an `approval` card for the owner, who can allow it. [approval](planning/flows/approval.md) B.2 and [drivers](planning/contracts/drivers.md) section 2 say the host denies it at once, without a card.
- Cause: the level is checked in `callApprove` (`internal/host/approve.go:161`), the MCP tool that only Claude Code calls. `runAskedEvent` (`internal/host/approve.go:137`), the way of the other three, calls `handleApproval`, which does not look at the level.
- Status: resolved. `runAskedEvent` denies a level 1 request at once, without a card, as `callApprove` does (`internal/host/approve.go`, host unit H1b-2); the three `@pending` examples (`features/m4.feature`, `features/s3_drivers.feature`) are enabled.

### 2026-11-26 · bug · The push of a handed-over task branch runs from a folder that the new assignee's create deletes

- Symptom: with a remote, a task reassigned to another AI of the same node while its branch has unpushed commits: the commits of the old AI never reach the remote, and the pull request of the task still shows the old head (`pull request of task 1 ... got [OPEN <old head>]`). The last example of A5.5 in `features/m5.feature` without `@pending`: red in 9 of 10 runs without `-race`, green in 2 of 2 with it. Sometimes the new session's `git fetch origin` also ends with `cannot lock ref 'refs/remotes/origin/task/…': is at … but expected …`, the `assign` item is spent without a turn, and the task counts one failure. On one node the local branch survives, so the commits are not lost; they are published at the next submit.
- Cause: the delivery of the new `assign` runs `CreateTaskWorkspace` first: `slot.claim` (`internal/host/worktree.go:109`) deletes the old AI's worktree with `os.RemoveAll`, while `releaseUnmergedWorktree` (`internal/host/deliver.go:516`) runs `git push` with that folder as its working directory, in two or three cleanups at once. They end with `fatal: Unable to read current working directory` or `remote unpack failed: eof before pack header`. The error is dropped, and the next cleanup sees holder == assignee, so nothing pushes again. This is an ordering bug against `planning/flows/task.md` ("Reassignment": first the push, then the end of the task session and the removal of the worktree) and `planning/contracts/local.md` section 7 ("first the push"). Second effect: the push takes no lock of the clone, while `CreateTaskWorkspace` fetches under it (`workspaceBase`, `internal/host/workspace.go:89`), and a push moves `refs/remotes/origin/<br>` too, so the two write the same ref (the fetch error above); `local.md` section 7 ("One lock per clone") does not list `push`, and `TaskForge.PushBranch` (`internal/host/forge.go:36`) has no lock in `push_branch` and `submit_task` either. A lock on the push alone does not fix it: whichever wins, if the create is first, holder == assignee and nothing pushes.
- Status: resolved by host unit H1b-2: the old assignee's session ends, its branch is pushed under the clone's lock and its worktree removed before the new one is made (see the entry "A task handed to another AI of the same node was not published, or failed its fetch" in Resolved). The A5.5 example is enabled.

### 2026-11-26 · bug · A task handed to another AI of the same node was not published, or failed its fetch

- Symptom: with a remote, a task reassigned to another AI of the same node while its branch had unpushed commits: most often the commits were never pushed (`fatal: Unable to read current working directory`, 9 of 10 runs of the last example of A5.5 without `-race`); sometimes the new task session's `git fetch origin` failed with `cannot lock ref 'refs/remotes/origin/task/…'`, and the task counted a failure.
- Cause: delivery of the new `assign` deleted the old worktree (`slot.claim`, `internal/host/worktree.go`) while the clean-up ran `git push` in it (`releaseUnmergedWorktree`, `internal/host/deliver.go`), without the clone's lock and with its error dropped. The pushes of `push_branch` and `submit_task` and the fetch after a custom forge's merge did not take the lock either.
- Status: resolved. The session of the old assignee ends, its branch is pushed and its worktree removed before the new one is made (`createTaskFolder`, `handOver`), all under the clone's lock, as is every push and fetch. A push that fails is told in the thread. That a task whose workspace cannot be prepared loses its `assign` item is by design (`planning/flows/agent-turn.md` E: one outcome, no retry): the secretary gets the `crash` notice with Git's output, and a `resume` brings the first turn with the brief. Tests: `internal/host/handover_test.go`.

### 2026-11-26 · bug · A removed person's node kept their AIs' sessions, and a left team group stayed in the secretary's line

- Symptom: after `member remove` of a person, that person's node kept the group's home, rows and AI sessions, dormant or with an open CLI. A team group that its owner had left stayed in "Team groups" of a secretary's turn until the node restarted.
- Cause: the home pushed a removed person's connections `groups` and no rows (`pushTo`, `internal/home/run_push.go`) and closed them, and the host only asked for a sync when a group left the list (`internal/host/home_moved.go`), which the closed connection did not answer. A removal and a move could not be told apart. A late reader of a closed connection also lost frames that had arrived (`dequeue`, `internal/peer/ctl.go`).
- Status: resolved. The home pushes the rows of a removal to the person's nodes first (`told`), the host leaves a group that is no longer listed and whose rows no longer have its owner (`ownerLeft`, `leaveGroup`), or whose home answers `forbidden`; a group that was syncing when its home stopped listing it is decided when the sync ends (`syncOneGroup`), and the watch grants of the AIs that leave go with their sessions (`closeMemberSessions`). `Read` returns what arrived before the end. Tests: `internal/host/home_left_test.go`, `TestRunStopsPushingToADeviceTheGroupNoLongerServes`, `TestFinishEndsAfterWhatWasSent`, the removal scenario of A9.4 with two real nodes.

### 2026-11-25 · limitation · `host.offline` does not mark an AI that never had a session

- Symptom: an AI that was added to a group and never woken keeps `dormant` ("Sleeping") in the group for everybody but its owner after its owner took all AIs of the computer offline.
- Cause: `handleHostOffline` reports a status for each AI that has a session row (`distinctMemberSlots`, `internal/host/limits.go:209`); a member without one is in none. The owner's browser shows it offline from `host.info` (`statusIn`, `web/src/lib/stop.ts`).
- Status: resolved with the entry "The host sends no member status when a group is paused or resumed": `hostedSlots` (`internal/host/limits.go`) takes the member rows too.

### 2026-11-25 · deviation · The host sends no member status when a group is paused or resumed, or its AIs go offline or online

- Symptom: after "Resume", the card of an AI that was interrupted outside a task stays "Paused" until its next turn or until it goes dormant. During a pause every other AI reads "Waiting" or "Sleeping · wakes when @mentioned", beside a notice that says the group is paused. An AI that never had a session reads "Sleeping" to everybody but its owner after `host.offline` and after `host.online` (see the entry above).
- Cause: `afterGroupState` (`internal/host/deliver.go:383`) sends no status on pause or resume; seen by continuing `TestPausingInterruptsTheTurnsOfThePausedGroupOrTask` with a resume: `MemberState` is `waiting` and no command was sent. `handleHostOffline` and the online side report only members that have a session row (`distinctMemberSlots`, `internal/host/limits.go:209`). `planning/sessions.md` section 3 wants `paused` and `offline` reported.
- Status: resolved: a group row that changes the pause reports the state of every AI this node hosts in the group (`reportPause`, `internal/host/deliver.go`), and `host.offline` and `host.online` report the AIs that a member row names too (`hostedSlots`, `internal/host/limits.go`). Test: `TestPauseAndOfflineAreReportedOfEveryHostedAI`.

### 2026-11-26 · bug · A changed secretary gets no secretary tools on its host

- Symptom: after `group set` with `secretary`, the AI it names cannot propose a brief: `tools/call` of `propose_brief` is refused and its CLI ends. A9.44 wants its host to list the secretary's tools.
- Cause: the host goes by `Session.Secretary` (`internal/host/session.go:59`), which is written at `host.join` only (`internal/host/join.go`); `planning/contracts/step-2/authority.md` section 4 says the group row decides.
- Status: resolved: before an AI gets its next turn its session takes the name and the role of the rows (`followMember`, `internal/host/deliver.go`), and a group session whose role changed starts a new CLI session; `joinDemoteIfSecretary` is gone. Tests: `TestGroupSessionFollowsTheSecretaryOfTheRows`, `TestSavedSessionFollowsTheSecretaryOfTheRows`, and the scenario of A9.44.

### 2026-11-25 · limitation · A `jump` that crosses the delivery of its message delivers the message twice

- Symptom: the author sends `jump` at the moment the AI takes the waiting message. The turn that just started with the message is interrupted, and the message comes again as the next turn.
- Cause: the home retires the waiting item and queues the jump under a new item number (`rndRequeueJump`, `internal/home/rounds.go`), where [commands.md](planning/contracts/commands.md) 1.4 says the item "gets `jump: true` and is sent again". The host knows a delivered item by `(home, item)` (`hasAuditItem`, `internal/host/deliver.go`), so it cannot tell that the new item is a message it has delivered. An item it still holds, the waiting one or an earlier jump of the message, it does drop (`retiredByJump`).
- Status: resolved. [#410](https://github.com/huanchong-99/SoloDawn/pull/410) keeps the number of the item (`rndJumpItems`, `internal/home/rounds.go`), so the host acknowledges a message it has delivered without a turn (`handleInboxElem`, `firstJump`); `retiredByJump` is removed. Test: `TestJumpThatCrossesItsDeliveryDeliversTheMessageOnce`.

### 2026-11-26 · bug · A joined person's group lacked entries that the home already held

- Symptom: a person joins a second group of a home, or rejoins a restored group, with a browser that presents a shorter log than the home holds. The join answers `ok`, but the group gets no `device` rows for the entries in between, refuses those devices, and refuses every later entry of that person (`errGap`). A device that the home knows as revoked could join with the older log.
- Cause: `joinSnapshot` (`internal/node/admit.go:111`) judged the join on the presented log alone, against `planning/contracts/envelope.md` section 4 (first accepted wins); and an import wrote only the owner's missing rows, from the log read before the file's logs were taken.
- Status: resolved. The join is judged on the log the node holds plus what the presented one adds (`dvlWith`, `internal/node/devlog.go`); the import writes, in its own transaction, the `device` row of every stored entry of the group's people that the group lacks (`importDevices`, `internal/home/export.go`; `LackingEntries`, `internal/store/home_reads.go`), so a stop no longer leaves a moved group without them. Tests: `TestInvitationJoinAndMemberSurface` (short log, revoked device), `TestImportAddsTheMissingDeviceRows`.

### 2026-11-26 · bug · Entries that a rejoin or an import brought were not stored

- Symptom: after a member rejoined a restored group with a longer log than the copy had, a restart of the node lost the new entries: their devices got `unknown_device` until an older device of that person connected. An import whose file had a longer log raised no `device` event, so the person's other groups on that node never got the rows.
- Cause: `dvlKeepRows` and `keepImported` (`internal/node/devlog.go`, `export.go`) replaced the log in memory only.
- Status: resolved: `dvlKeep` takes a log it already holds as received entries (`dvlReceive`: stored, held, event raised), for a join and an import alike; `dvlPutEntries` ignores entries the join already stored. Tests: `TestLogsKeepFirstAtTheSameVersion`, `TestLocalGroupTransfer`.

### 2026-11-26 · bug · The owner of a group could not watch an AI on another person's computer

- Symptom: after the consent, the other person's node dropped the offers of the owner's devices that were enrolled before the group was made.
- Cause: `grpFounded` (`internal/home/cmd_group.go:90`) wrote no `device` rows, against `planning/contracts/rows.md` section 3, and `devEvent` adds only later entries; so `watchDeviceUser` (`internal/host/watch.go:105`) found no person for the device. A9.20 and A9.21 passed because P's watching browser was enrolled after the group was made.
- Status: resolved. The founding writes the owner's log as rows (`devOwner`, `internal/home/ev_device.go`), for both kinds of group, and an import writes the rows that the group lacks of its people's stored logs. The browser of A9.20 and A9.21 is now enrolled before the group. A group founded earlier gets the rows when it is moved; until then it has them only for entries accepted after its founding.

### 2026-11-26 · bug · A paired computer's entry in the device log had an empty name

- Symptom: `host_name` of every AI on a computer that was paired by the real client was empty; the terminal's header began with ` · T-1`.
- Cause: the node saves the label of `solodawn pair` only when it is adopted (`Pairer.finish`, `internal/node/pair.go:475`), so the `welcome` before that had `name` empty (`sesWelcomeBase`, `internal/node/session.go:341`). The browser takes that name (`#onOnline`, `web/src/lib/onboard.svelte.ts:392`) into the entry it signs (`:418`), from which the home reads `host_name` (`memDevice`, `internal/home/cmd_member.go:47`). `TestPairRoleBEndToEndAndBootstrap` asserted the empty name.
- Status: resolved: the `welcome` of a pairing connection carries the label of its pairing. An entry signed with an empty name keeps it.

### 2026-11-26 · bug · FLAKY-32, found on the way: an accept error ended the node's listeners

- Symptom: after one accept error, such as too many open files, the node stayed up and answered nothing on its local socket: every MCP shim and local command connected to the backlog and waited for ever. Nothing was logged. The loopback web server did the same on an error `net/http` does not take as temporary, and the ICE-TCP port on any error.
- Cause: `serve` (`internal/node/node.go:653` before the fix) left its loop on any error of `Accept`; `startLocalHTTP` (`node.go:307`) dropped the error that ended `Serve`; `TCPMuxDefault.start` of pion ice (`tcp_mux.go:101`) returns on any error of the listener of `internal/peer/sockets.go:34`.
- Status: resolved on `fix/mcp-accept`: the three accept through `peer.Steady` (`internal/peer/accept.go`), which ends only with its listener or its node, logs any other error as `accept.fail` and tries again after 5 ms, doubling to 1 s.

### 2026-11-26 · bug · FLAKY-32, found on the way: an AI whose `solodawn` MCP server did not connect worked on without its tools

- Symptom: Claude Code 2.1.311 and Codex 0.164.0 ran the turn as if all were well. Measured with the real programs and no model call, for a server that exits at once, one whose command does not exist and one that never answers: Claude's `system/init` lists `{"name":"solodawn","status":"failed"}`, after 30 s in the last case (a server that answers after 3 or 10 s is waited for, `connected`); Codex answers `thread/start` and `thread/resume` at once and reports `mcpServer/startupStatus/updated` with `failed`.
- Cause: `internal/host/claude.go` (`claudeOtherEvent`) read only the session id of `system/init`; `internal/host/codex.go` does not read that notification, and did not set `required`.
- Status: resolved on `fix/mcp-accept`: Claude's process is killed and the turn is a crash; Codex gets `mcp_servers.solodawn.required=true`, with which both requests failed in all three cases, and a failed request was a failed start already (`handleStartupError`). `planning/contracts/drivers.md` 1.1 and 1.2. The captured line is `internal/host/fakecli/testdata/claude-mcp-failed.json`.

### 2026-11-26 · bug · A member's node can follow a moved group more than 5 seconds after the import

- Symptom: A9.27 asks that Q1 follows P2 within 5 seconds of the import. Q1 took 11.6 seconds with the import right after the export; with minutes between them it can take up to 70 seconds. Once connected, everything else of A9.27 holds.
- Cause: Q1 gets `moved` when P1 announces its groups (`internal/host/home_moved.go:14`) and offers to P2 at once. P2 does not know Q1 before the import and drops the offer (`identity.OpenSignal` in `open`, `internal/peer/signal.go:104-110`, returns `unknown_device`, `internal/identity/envelope.go:59`; discarded in `internal/peer/path.go:227`), so Q1 waits 10 seconds for an answer (`internal/peer/unanswered.go:5-13`, `flows/connect.md` step 15) and then backs off 1, 2, 4 … 60 seconds. Nothing wakes it: a node dials a home with no presence channel (`internal/node/node.go:474`), and the import changes no presence, since P2 is online all the time. `export.md` section 3 step 4 ("clients retry as for any home that is down") and the 5 seconds of A9.27 do not agree. A browser follows by the same rules.
- Status: resolved by decision (2026-11-26): the 5 seconds contradicted `export.md` section 3 step 4 and `flows/connect.md` step 15, so A9.27 now says "by their normal retry" (`flows/connect.md` step 15, at most 10 seconds for an unanswered offer and 60 of back-off between two offers; connecting comes on top). The scenario, which imports right after the export, gives Q1 those 70 seconds and keeps 5 seconds for the two browser peers, which dial after the import. No product change.

### 2026-11-22 · limitation · A9.20 cannot set up a watcher before home invitation support lands

- Symptom: A9.20 fails in its first Given with `bad_request: the home has no such command`, before opening the owner's terminal or exercising the watch host.
- Cause: `joinWatcher` in `features/s2_watch_terminal_test.go` sends `invite`; `internal/home/people_seam.go` declares the invite/join command keys but registers no handlers, so `internal/home/check.go` refuses the command. The home invitation/join units T2-1a and T2-1b are outside T2-6.
- Status: resolved 2026-11-26, A9.20 is enabled. One product bug: nothing pushed `host.info` when `watchers` changed, against `datachannel.md` section 7. `watchItem` and `watchEnded` (`internal/host/watch.go:58`, `:416`) told the node nothing of it, and `pushHostInfo` (`internal/node/surface.go:139` before the fix) had `refresh` as its only caller. The step that waited for the push timed out in CI on Linux and Windows, since the grant reaches Q1 after P1 has answered Q; a browser asks once per connection (`web/src/components/watch/Watchers.svelte:15-25`), so an owner whose terminal page was open never saw or could stop a watcher. `Host.WatchersChanged` now pushes it, and the step waits for the push, which times out without it. #382 added the missing `invite` handler. The rest were faults of the steps: the Given timed out because the terminal peer (`features/s2_watch_peer_test.go`) offered only the candidates of this machine's own addresses, which a firewall drops. It now uses 127.0.0.1 alone, through the sockets of the M7 browser. The group is homed on P1, as in the cast, so the grant goes to a node that is not the home; P, no member of any group there, is admitted by the grant alone and cut off at hand-back. The peer waits for the size that Q1 pushes when it has attached the watch: its own side of the channel opens earlier, and bytes before that are not for the watcher (found as 1 failure in 9 runs; with a one-second delay before `attach` the old step fails every time and the new one passes). For a Windows console, Enter is `\r`, the size is read from standard output, and the watch must be the end of what the owner got, since the console writes more than the program does; the echo of the first line shows that it starts where the watch opened. That nothing is granted before the consent is read from the owner's `host.info`: an offer that Q1 drops does not show it.

### 2026-11-22 · limitation · A9.21 lacks a same-connection network-switch driver

- Symptom: the original fixture opened a new relay-only connection instead of moving an established watch to TURN.
- Cause: a transport-only probe with controlling ICE, renomination, warm relay pairs and bidirectional filtering on every watcher UDP socket kept the direct pair selected. Pion's automatic selector retained that pair; the binding-request selector also failed to move both ends. This is fixture evidence, not an end-to-end watch result.
- Status: resolved 2026-11-26, A9.21 is enabled, and passes on the development Mac with its firewall on. `moveToRelay` in `features/s2_watch_peer_test.go` uses the network control of A7.9 (#373) on 127.0.0.1: direct packets take 3 seconds until the measured round trip says so, then direct UDP is cut. Pion (v4.4.8, `agent.go`, `evaluateCandidatePairQuality`) rates the last round trip of a pair that answers no more, and 3 seconds put the dead direct pair below relay to relay (10 plus 20 for recent answers), so ICE renominates by itself in about 5 seconds, when the direct pair loses its own 20.

### 2026-11-26 · deviation · A person's second browser did not see the groups they joined

- Symptom: a colleague joined on one browser and saw no group, card or approval on another, nor on the first after its storage was cleared (R-42; gap 10 of the rules audit of 2026-11-25).
- Cause: `openSession` (`web/src/lib/net.svelte.ts`) dialled the owner's nodes and the homes stored by this browser's own join; `host.info.homes` (`planning/contracts/step-2/people.md` section 7, point 2) was read for names only.
- Status: resolved on `web/homes`: each foreign home of an own node's `host.info` is dialled and remembered (`Home.onhomes`). Tests: `web/src/lib/net.test.ts` ("the homes that an own computer knows"), the group feature, `web/e2e/m2.spec.ts` with real nodes.

### 2026-11-26 · deviation · An idle browser lost a moved group, and an open tab stayed on a lost home

- Symptom: after `group export --to` a member's idle browser dropped the group instead of following it (A9.27); after a rejoin of a restored group the open tab showed the lost home's stale copy (A9.28).
- Cause: the old home pushes `groups` without the group, `Home.#list` (`web/src/lib/groups.svelte.ts`) dropped it and asked nothing more, so no `moved` came (`planning/contracts/step-2/export.md` section 3). `setJoin` added a second home, and `find` returned the first one's copy.
- Status: resolved on `web/homes`: a group that left the list is read once more, and one listed by a node while its held home is out of reach, or no longer lists it, is taken over and read from the start (`Home.#enter`); one that a host names while its old home is connected is asked for there (`Home.expect`). Tests: `web/src/lib/{groups,net}.test.ts`, `features/web_move.feature`.

### 2026-11-26 · deviation · A stopped task or a failed sign-in alerted nobody who had the page open

- Symptom: neither `task_failed` nor `login_invalid` showed in the page outside the chat (R-56, R-63; gap 7 of the same audit, page part), and their push read as a mention.
- Cause: `isQuiet` (`web/src/lib/notify.svelte.ts`) dropped every row by `system` but a confirmation.
- Status: resolved on `web/homes`: a notice that mentions the reader notifies in its row's words, and the push of a stopped task says so (`web/src/sw.ts`). The push of a failed sign-in carries no title and no code (`planning/contracts/commands.md` 1.7), so it still reads "Mention" with the AI's name.

### 2026-11-25 · deviation · No person could write in a task's thread

- Symptom: an AI's question in a thread could not be answered, and a task paused by the round limit had no way back although the page said "Reply in its thread to carry on" (R-33, R-60, A5.8). A main-line mention goes to the AI's group session, never to its task session.
- Cause: the only text post of the web client sent `thread: null` (`post`, `web/src/lib/groups.svelte.ts`), and `Thread.svelte` had no field. The home's half works.
- Status: resolved on `web/task-actions`: the composer inside an open thread. Tests: `features/web_taskops.feature`, `web/e2e/m5.spec.ts`.

### 2026-11-25 · deviation · No person could pause, resume, reassign or cancel a task

- Symptom: a task stopped by three failures or six failed reviews, or paused, stayed so for ever: those resumes and reassignments are an admin's alone (`planning/contracts/task-machine.md` rows 11, 13 and 15), and the card did not say why a paused or queued task stood still.
- Cause: nothing in `web/src` sent a `task` command.
- Status: resolved on `web/task-actions`: `web/src/components/task/`, `web/src/lib/taskops.ts`, `features/web_taskops.feature`, `web/e2e/m5.spec.ts`.

### 2026-11-26 · deviation · A task whose AI's owner said no read "In progress" and nothing else

- Symptom: after the owner denied the role, the AI gets no work, the task has no wait, and its card did not say why it stood still.
- Cause: the wait of a `confirm` card ends with the denial (`planning/contracts/step-2/confirm.md` section 6) and no row takes its place.
- Status: resolved on `web/task-actions`: the card's line, with "Reassign" for an admin (`deniedHold`, `web/src/lib/taskops.ts`). Tests: `taskops.test.ts`, `features/web_taskops.feature`.

### 2026-11-26 · bug · Enter broke the line in the composer of a computer without a mouse

- Symptom: on Firefox 136 in CI, Enter in the composer of a task's thread left `@Claude Code yes, keep it.` and a line break in the field and sent nothing (`web/e2e/m5.spec.ts`). The main line's composer is the same component and did the same; the end-to-end tests send there with the button, so none saw it.
- Cause: `web/src/components/chat/Composer.svelte:31` before the fix let Enter send under `(pointer: fine)` only. A browser without a pointing device answers `(pointer: none)`: neither fine nor a touch screen. Headless Firefox 136 on Linux always does (`widget/headless/HeadlessLookAndFeelGTK.cpp`, `PrimaryPointerCapabilities` is 0).
- Status: resolved in PR #400: Enter sends unless the pointer is coarse. Test in `web/src/components/team/chat.test.ts` (fine, none, coarse).

### 2026-11-25 · deviation · A terminal opened on no chosen task

- Symptom: with two tasks the host took the one used last, and a review could not be taken over (F-12, `planning/flows/takeover.md` A).
- Cause: the route and `takeover.start` carried `task`, and no link named one (`MemberPanel.svelte`).
- Status: resolved on `web/task-actions`: the menu of a task card, and the AI card while its AI is on a task.

### 2026-11-25 · bug · A computer that missed the one `home.add` was shut out of the group

- Symptom: a person's computer that was asleep when they joined a group homed on another person's computer, or was paired later, could never add an AI to it: `host.join` ended in `home_unreachable`. For a team group its secretary could not post: `post` with `group` answered `not_found`. Joining again was not possible.
- Cause: the browser sent `home.add` once, to the computers connected at that moment (`web/src/lib/join.ts`). `host.join` does not make up for it, for project groups either: the node checks the sender of every signal with `knownPeer` (`internal/peer/signal.go:107`, `internal/node/node.go:426`), only the setting `homes` makes another person's computer known (`internal/node/admit.go:133`), and `host.join` writes that setting after it has connected.
- Status: resolved: the browser compares the groups' homes with `host.info.homes` when a computer connects and when a group appears, and sends what is missing (`Nodes.tell` in `web/src/lib/nodes.svelte.ts`). Joining keeps its own `home.add`: it also replaces the home of a group restored from a copy (`planning/contracts/step-2/export.md` section 4). Tests: `web/src/lib/nodes.test.ts` (`tell`), the group feature, `e2e/s2.spec.ts` (A9.23).

### 2026-11-25 · bug · Nobody could create a team group in the web client

- Symptom: the create dialog always made a project group and asked for a repository. In a team group the empty chat promised a brief from the secretary, the side panel offered project memory, style presets and a knowledge base, and the group list and the in-page notification of a mention named the person whose computer sent a bulletin instead of the AI it names (`planning/contracts/step-2/watch-team.md` sections 9 and 10).
- Cause: `create` sent a fixed `kind: "project"` (`web/src/lib/groups.svelte.ts`) and `CreateGroup.svelte` had no choice; #316 built only the reading half. `Chat.svelte`, the mounts of `chat.side`, `preview` in `GroupList.svelte` and `whoOf` in `web/src/lib/notify.svelte.ts` had no case for a team group or a bulletin.
- Status: resolved: the dialog asks for the kind. Project memory, style presets and the knowledge base are mounted for project groups only (`project` of an `Entry` in `web/src/lib/mounts.svelte.ts`). Tests: the group feature, `features/watch_team.spec.ts`, `e2e/s2.spec.ts` (A9.23), `web/src/lib/{groups,join,notify}.test.ts`, the team component tests.

### 2026-11-25 · bug · Group settings offered an external AI as secretary

- Symptom: chosen, such a secretary was never started and has no `propose_brief`, `set_task` or `summary`, so requests went nowhere. `planning/contracts/step-3/external.md` section 2: "The client does not offer it".
- Cause: `EditGroup.svelte:18` listed every AI member. The add-AI dialog already left the choice out.
- Status: resolved: `secretaryCandidates` in `web/src/components/groupinfo/trust.ts`; with nothing else to edit, group details no longer offer settings that could only send an empty `group set`. Tests in `details.test.ts`.

### 2026-11-25 · deviation · The web client had no emergency stop

- Symptom: no control sent `group pause`, `group resume`, `host.offline` or `host.online`, and a paused group looked like any other (R-64, A4.7 and A4.8 "as seen"). On the computers page an AI in `takeover` or `paused` showed the raw word.
- Cause: the unit that held them (U-5) was cancelled for its permission levels; `stateLabel` in `web/src/routes/Nodes.svelte` did not know the two states.
- Status: resolved on `web/stop-and-jump`: `web/src/components/stop/`, `web/src/lib/stop.ts`, `features/web_stop.feature`, `web/e2e/m3.spec.ts`.

### 2026-11-25 · deviation · A confirm card for a task without a brief names "Former member" as the requester

- Symptom: when a role in a task that has no brief is given to another person's AI (`task assign`), the owner is asked "Let Codex act on the request from Former member." Contract `planning/contracts/step-2/confirm.md` section 3: who asked is then the owner of the AI that started the task.
- Cause: the request is the `task` message, which the home writes with `by: "system"` (`internal/home/cmd_brief.go:339`); `requester` in `web/src/lib/confirm.ts` takes the author of the request. No row says who started a task once its assignee has changed.
- Status: resolved on `web/confirm-waits`. The sentence names the task and no person, "Let Codex take part in T-12 “Show connection path”." (`confirmSentence` in `web/src/lib/confirm.ts`): the request has the task's id, and task rows are always held. Section 3's person cannot be shown without a new field, which is not worth one.

### 2026-11-25 · bug · After a page load, a confirm card for a call said "from an unknown requester"

- Symptom: an owner who opened the app from the push read "Let Codex answer the call from an unknown requester." in the waiting bar, the group list and the card, until they opened the call's thread. The same for a mention in a thread, and for a main-line request older than the newest 50.
- Cause: a cold read brings every pending card and the newest 50 main-line messages, and no message of a thread (`internal/home/sync.go:100-103`); `requester` in `web/src/lib/confirm.ts` takes the author from the request's row.
- Status: resolved on `web/confirm-waits`: after `synced`, `Group` fetches the missing request of each confirm card that waits for the reader, one row each (`#requests` in `web/src/lib/groups.svelte.ts`).

### 2026-11-25 · bug · Confirm cards did not count as waiting, and task badges and system rows missed their step 2 wording

- Symptom: a card that asks an owner "may Pat direct your AI?" showed only in the chat: not in "Waiting on you", the group's count, the in-page notifications or the browser push, so it could expire unseen. A task held by such a card, and a task whose owner had taken over the terminal, showed "In progress". `member_joined`, `member_removed`, `rate_limited`, `rounds_limit`, `preempted`, `group_paused` and `group_resumed` showed as their codes. A card for a role named the secretary, who writes the brief, instead of the person who confirmed it.
- Cause: the lists of kinds in `waitsFor` and `subject` (`web/src/lib/groups.svelte.ts`), `reminderKinds` (`web/src/lib/push.ts`), `waitingKinds` and the `by: "system"` early return of `classify` (`web/src/lib/notify.svelte.ts`) and `kindLabels` (`web/src/sw.ts`) had no `confirm`; `waitBadge` and `taskBadge` (`web/src/lib/task.ts`) and `SystemRow.svelte` had no case for them, left out while those features were postponed.
- Status: resolved. Tests: `web/src/lib/{confirm,groups,notify,push,task,when}.test.ts`, `web/src/sw.test.ts`, the group, approval, notification, reminder, task and thread features.

### 2026-11-26 · bug · A turn that ends while the held items are read gives the AI its newer messages first

- Symptom: with 146 items held, `inbox.take` took items 352 to 369 and left 224 to 351, which went to a later turn. [agent-turn](planning/flows/agent-turn.md) B.6 gives a session all its held items, in order.
- Cause: `isItemHeld` (`internal/host/deliver.go:909`) reads whether the AI is working anew for each item of the queue that `selectEligibleBatch` walks (`internal/host/deliver.go:879`). Items read before the turn ends stay held, and those after it are taken. FLAKY-27 fixed the same race for the group pause.
- Status: resolved on `fix/host-delivery`: a walk reads each state once (`look`), the takeover, the holds and the tasks too. Found in the node log of `web/e2e/m2.spec.ts`. `TestWalkOfTheQueueReadsEachStateOnce`, `TestLookKeepsWhatItRead`.

### 2026-11-26 · bug · An item sent again while its turn was prepared came twice in one turn

- Symptom: the home sent an item again while delivery prepared its turn, and the preparation failed: the next turn had the item twice ("items 5,5"). A jump that retired the item meanwhile was delivered beside it.
- Cause: `prependHeld` (`internal/host/deliver.go:1158` at 10ddc7c72) put the taken items before the queue without regard to what had arrived.
- Status: resolved on `fix/host-delivery`: the queue is put behind the returned items by the rules of an arriving item (`enqueueHeld`), so a jump of another message goes first too (R-72). `TestItemTakenBackMeetsItsCopyInTheQueue`.

### 2026-11-26 · bug · A denied `ask` did not reach the caller

- Symptom: the owner of the called AI denied the call: an open `ask` was not ended, and after the wait the caller got a `reply` turn without text. Contract: [confirm.md](planning/contracts/step-2/confirm.md) section 5, acceptance A9.18.
- Cause: the home queues the denial as a `reply` item that carries the confirm card (`cfmDeniedResult`, `internal/home/cmd_confirm.go`). The host took only a `reply` message as the answer of an open call (`replyCallID`, `internal/host/deliver.go:571`), read only an answer from it (`decodeReplyItem`, `internal/host/tools_ask.go`), and its turn showed the card's `text`, which is empty (`buildReply`, `internal/host/turn.go:419`).
- Status: resolved on `fix/host-delivery`. `TestAskReturnsADenial`, `TestDeniedCallIsATurn`. The deny example of A9.18 stays `@pending` until the fixtures of `s2/lift-confirm` are merged; with them it passes.

### 2026-11-26 · bug · A host Git command left a maintenance process behind (flaky `TestInterruptedLocalSquash`)

- Symptom: `TestInterruptedLocalSquash` failed in about one of 600 runs under load: `TempDir RemoveAll cleanup: unlinkat …/.git: directory not empty`. What was left was `.git/objects/pack` and `.git/objects/info/packs`.
- Cause: after a commit, a merge or a fetch Git runs `git maintenance run --auto`, which by default goes on in a process of its own (`--detach`); when two loose objects happen to be in `objects/17`, that process repacks (Git 2.54). `runInput` (`internal/host/command.go`) returned while it wrote: in the product it changed the clone outside the clone's lock and outlived the command's limit. The Git commands of the test fixtures left such processes too.
- Status: resolved on `fix/host-delivery`: every command of the host has `gc.autoDetach=false` (`foregroundGC`), and so have the fixtures of the host tests and of the acceptance suite. `TestCommandKeepsGitMaintenanceInTheForeground`.

### 2026-11-25 · bug · `read_thread` gave an AI an empty text for most kinds

- Symptom: a brief, decision, approval, check result or review read back with `"text":""`, and an AI that reads context first (R-33), or the secretary told to write notes after every review, saw nothing of it. A pending decision or a brief in the history of a replaced session was only `(decision)` or `(brief)`.
- Cause: `fetchThreadMessages` (`internal/host/tools_talk.go`) returned `row.Text`; these kinds keep their content in `body`. `msgFormatters` (`internal/host/turn.go`) had no brief, and the decision formatter only an outcome.
- Status: resolved on `fix/host-context`; `TestReadThreadShowsWhatABodyHolds` pins the texts.

### 2026-11-25 · deviation · The confirmed scope reached no AI

- Symptom: the person confirms what is in and out of scope on the brief, but neither the assignee's nor the reviewer's turn said it.
- Cause: `briefSpecLines` (`internal/host/turn.go`) gave goal and criteria only, and `drivers.md` section 3 named no scope line.
- Status: resolved on `fix/host-context`: `In scope:` and `Out of scope:` follow the criteria.

### 2026-11-25 · deviation · The secretary was not told which team groups exist

- Symptom: R-14's relay happened only when the owner gave the group's id, because no turn, instruction or tool description named a team group.
- Cause: neither `sessionHeader` (`internal/host/turn.go`) nor `internal/host/instructions/secretary.md` said it.
- Status: resolved on `fix/host-context`: the secretary's header lists the team groups its node knows, and its instructions say when to post there.

### 2026-11-25 · deviation · `list_members` called every person online

- Symptom: a person without any browser connected was `"state":"online"`.
- Cause: `online` is written `true` when a person creates or joins a group and nothing writes `false` (`internal/home/cmd_group.go`, `cmd_people.go`); the tool passed it on.
- Status: the tool half is resolved on `fix/host-context`: a person has no state there. The row field and the badge in the web client stay as they are.

### 2026-11-25 · bug · The wake-up cap held people's messages and the answers to waiting calls

- Symptom: from the 21st item in an hour on, an AI got a person's message, a decided approval, a confirmed brief or the reply to its call up to an hour late, and no row or screen said so. The secretary, which gets every unaddressed message and every notice, fell silent in a busy hour (gap 6 of the rules audit of 2026-11-25).
- Cause: `inboxWake` (`internal/home/inbox.go:111` before the fix) exempted only `merge` and `watch` items, whoever caused the item. R-60 limits AIs that wake AIs.
- Status: resolved in branch `fix/home-items`: the items of a command of a device that hosts no AI of the group, and `reply` items, are neither held nor counted ([commands](planning/contracts/commands.md) 1.3). Test: `TestInboxWakeUpCapHoldsAndReleasesItems`, "people and replies are not held", and `features/home_inbox.feature`.

### 2026-11-26 · bug · E-M2 counted on the wake-up cap to keep its secretary silent

- Symptom: with the people's items let through, `web/e2e/m2.spec.ts` failed on every engine: "History line 1451" was not on the first page, or the first line of the page before was not where it was counted to be.
- Cause: the test sends 21 messages to a secretary and then 1500, and expects pages of 50 rows that hold only those lines. The old cap of 20 wake-ups held the 1500. Now they wake the secretary, whose fake CLI asks its question on every start and exits at a second turn, so decision cards and `turn_interrupted` rows take places on the pages. No row of the history is missing or moved: the loaded lines are contiguous up to 1500.
- Status: resolved in branch `fix/home-items`: the fake secretary's one turn stays open (`decisionSecretaryScript`), so the only card is the one from before the 1500 messages.

### 2026-11-25 · bug · A failed sign-in mentioned nobody, and the push of a system row named nobody

- Symptom: a `login_invalid` row reached no person though R-56 says the owner is told, and the push of a `task_failed` row had an empty `who` and no title, so the browser showed an empty body (gap 7 of the same audit, home part).
- Cause: `hevEventRows` (`internal/home/cmd_hostev.go:150` before the fix) wrote `mentions` `[]`, and `wpushMention` (`internal/home/webpush.go:236` before the fix) looked up the name of the member `system`.
- Status: resolved in branch `fix/home-items`: `login_invalid` mentions the AI's owner, and `wpushSystem` names the AI of the row, or the assignee and the title of its task. Tests: `TestHostEvEventsAndNotices`, `TestWebPushReminderKindsTitlesAndNoLeak`, `features/home_hostev.feature`, `features/home_webpush.feature`. The notification of an open page is another unit's.

### 2026-11-25 · deviation · `jump` moved another message than the one it named

- Symptom: `jump` on an older message moved the author's newest waiting message of that thread, and one AI only when the message mentioned several.
- Cause: the rule of [commands](planning/contracts/commands.md) 1.4, implemented by `rndRequeueJump` (`internal/home/rounds.go:60` before the fix), which read of `msg` only its author and thread.
- Status: resolved in branch `fix/home-items` (coordinator's decision of 2026-11-25): every unacknowledged `mention` item that `msg` produced is sent again with `jump: true`. Test: `TestRoundsJumpRequeuesTheItemsOfItsMessage`.

### 2026-11-26 · bug · A role that its owner gave in spite of a card stayed held

- Symptom: the owner of an AI denied a role and later gave it that task with their own `task assign`: the first `assign` reached the AI, the later `checks`, `reviewed`, `review` and `resume` items were dropped. In the same way a card written for one task of a brief held the items of another task of it that the AI already did by its owner's word, and gave it the wait `confirm`.
- Cause: `cftHoldItem` (`internal/home/cmd_confirm_task.go:138` before the fix) saw the exception of [confirm](planning/contracts/step-2/confirm.md) section 6 only in the command that changed the role; for every other command it looked at the request's one card, and nothing on disk said who had given the role.
- Status: resolved in branch `fix/home-items-3`: the task row's optional `asked` names the AIs whose role needs the card ([rows](planning/contracts/rows.md) section 4). A role given before the fix has none: nothing holds it, and a card still pending for it starts nothing, so the role is given again. Tests: `TestRoleDeniedThenGivenByTheOwner`, `TestRoleThatAsksNobodyIgnoresTheCardOfItsRequest`. The task card reads `asked` too since branch `web/leftovers` (`deniedHold`, `web/src/lib/taskops.ts`): its line that the owner said no went on after the owner gave the role.

### 2026-11-26 · bug · A jump that crossed the delivery of its message was delivered twice

- Symptom: a host that was delivering a message when its `jump` came got the message again as a jump item and gave it to the AI a second time.
- Cause: `rndRequeueJump` (`internal/home/rounds.go:58` before the fix) retired the item and queued a new one; the host knows what it has delivered by the item's number.
- Status: resolved in branch `fix/home-items-3`: the item jumps under its number (`Result.Jump`, `store.Tx.Jump`). Tests: `TestRoundsLoopJumpAndQuiet`, `TestHomeReadsJumpsAndRetiresTheQueue`, `features/home_rounds.feature`. That a resent jump does not interrupt its own turn is the host's part (`feat/host-jump`). The limitation that branch records for this is closed with it.

### 2026-11-26 · deviation · A reminder did not say what happened and its click opened nothing in a thread

- Symptom: the push of a stopped task and of a failed sign-in both read as a mention, and a click on the reminder of a row in a task's thread looked for a message that the main line never shows.
- Cause: `proto.Reminder` had no code, and `wpushNotify` (`internal/home/webpush.go:180` before the fix) always sent the row's id.
- Status: home part resolved in branch `fix/home-items-3`: `code` for a `system` row, and the task's id as `msg` for a row in a thread alone ([commands](planning/contracts/commands.md) 1.7). Tests: `TestWebPushReminderKindsTitlesAndNoLeak`, `features/home_webpush.feature`. The browser's part resolved in branch `web/leftovers`: words for `code`, `renotify`, and the click opens the task's thread, fetching its message when the read did not bring it (`web/src/sw.ts`, `web/src/lib/jump.ts`, `Group.seek`). Tests: `web/src/sw.test.ts`, `web/src/lib/jump.test.ts`, `features/web_reminder.feature`.

### 2026-11-25 · bug · Removing a person left their connections open

- Symptom: after `member remove` of a person, or after they left, their devices stayed connected to the home and could reconnect until the node restarted; they got no rows and no commands. A removed person who then added a device also kept the node from starting again: the entry was stored without the log it follows, and `OpenLogs` failed with "the version does not follow the log".
- Cause: the removal pushed `groups` only (`internal/home/cmd_people.go`). `Logs` kept the person's log in memory (`internal/node/devlog.go`), so `knownPeer` still admitted their devices and `dvlCommit` still stored their entries, and nothing closed a connection except the end of a watch grant.
- Status: resolved on `fix/removal-closes`. The home tells the node to drop the log (`LoopConfig.Forget`), and a connection that `knownPeer` no longer admits ends after its next `groups` push (`sesConn.sendGroups`, `peer.Connection.Finish`). An entry that comes between the removal's transaction and `Forget` is not stored either: `dvlPutEntries` stores an entry only behind the one it follows. `TestRemovedPersonIsDisconnected` fails without any of the three; a development node that already has such rows needs them deleted by hand. A9.4 passes once its fixtures send `home.add` (another unit).

### 2026-11-26 · bug · A revoked device stayed connected to a node that homes no group

- Symptom: the owner revoked a browser; on their computer that only hosts AIs, its connection and its open `pty:` channels stayed. `planning/flows/pairing.md` D.3 has each receiver of the entry close it.
- Cause: `sesLogChanged` (`internal/node/devlog.go`) closed only the revoked devices of other people, never the owner's.
- Status: resolved on `fix/removal-closes`: when a log changes, `sesLogChanged` closes every connection that `knownPeer` no longer admits. `TestRevokedDeviceIsDisconnectedWithoutAGroup` fails without it.

### 2026-11-25 · bug · A tool call that waits kept the node from stopping

- Symptom: a node stopped while a CLI waits in `approve`, `AskUserQuestion` or `ask` (up to `approval_wait_s`, 600 s) did not stop until that wait ended or the CLI died; a service manager killed it instead.
- Cause: the SDK hides the server's context from its handlers (`notDone`, go-sdk v1.8.0 `internal/jsonrpc2/conn.go:232-234`), so the end of the node's context cancelled no call, `Server.Run` (`internal/host/mcp.go`) waited for it, and `serve` (`internal/node/node.go`) waited for `Run` before `n.Close`, whose `Settle` ends the CLIs.
- Status: resolved on `fix/removal-closes`: `ServeMCP` closes the stream when its context ends, which is also how a removed member's stream ends; the read fails, and the SDK cancels the calls. `TestNodeStopEndsAWaitingToolCall` fails without the line.

### 2026-11-23 · bug · Removing an external member does not close its open MCP stream

- Symptom: in A10.27, the owner successfully removes X after reassigning its task, and the member row is gone, but the already-open MCP stream does not close.
- Cause: the member-removal path ended the host's member sessions (`internal/host/join.go:620`), but the local MCP server ran with the node's lifetime context (`internal/node/node.go`, `internal/host/mcp.go`). A tool call rechecked the token, but removal did not end an otherwise idle stream.
- Status: resolved on `fix/removal-closes`: `ServeMCP` watches the wake of the member (`untilRemoved`) and ends its context when the member is gone. `TestRemovalClosesExternalMCP` fails without it, and the second A10.27 scenario is enabled.

### 2026-11-23 · bug · The fake Qwen Code started OpenCode's MCP server

- Symptom: Qwen A4.9 failed every time at step 4 with `OPENCODE_CONFIG_CONTENT='{"mcp":{}}'` in the environment (#373). Nothing in the tests or CI sets it; found while looking for FLAKY-29.
- Cause: `startMCP` (`internal/host/fakecli/mcp.go:40-43`) replaced the server of any fake CLI with the one in `OPENCODE_CONFIG_CONTENT` whenever that variable was set. The Qwen Code driver passes only `QWEN_CODE_SYSTEM_SETTINGS_PATH` and inherits the rest of the environment, so the fake Qwen Code launched an empty command and never posted.
- Status: resolved in #377: only the fake OpenCode reads `OPENCODE_CONFIG_CONTENT`. `TestQwenServesItsScriptBehindTheBearerToken` sets it and checks that the fake Qwen Code starts its own server.

### 2026-11-24 · bug · A scenario could restore the host drivers while another node discovered agents

- Symptom: the Linux `-race` acceptance job reported a data race in A1.1 (`features/node_pair.feature:5`): a write of `host.Drivers` at `features/host_limits_test.go:237` against a read in `agtModels` (`internal/host/models.go:17`), from a `Pairer.Serve` goroutine started at `features/helpers_test.go:648`.
- Cause: `hostLimitsSteps` registered the restore of `host.Drivers`, `host.Compose` and `host.Instructions` in `afterScenario` for every scenario of the suite (`features/host_limits_test.go:232-240`), not only for the host limits scenarios that replace them. So A1.1's own cleanup rewrote the global while its paired node, still serving, read it.
- Status: resolved on `devin/race-store-drivers` in the test harness: `saveHLHooks` saves the three hooks and the three host limits bubbles and the host external run restore them with `defer` around the work that replaced them, so no other scenario writes them. `Drivers` stays a package-level registry: the product writes it only at package init (`Register` in each driver file), and only tests replace it.

### 2026-11-24 · bug · A restarted host was settled while its home loop could still push to it

- Symptom: the Linux `-race` acceptance job reported a data race in A4.3: `Store.Close` closing the write queue (`internal/store/store.go:73`, from `nodetest.Node.Close`) against `onWriter` sending on it (`:95`), from `projectDir` (`internal/host/run.go:184`) in `cleanupGroupState`, started by `pushRows` (`internal/host/deliver.go:347`).
- Cause: the scenario's second host. `restartHost` (`features/m4_test.go`) registered `link2.Close(); h2.Settle()` as its cleanup, which ran while the node's home loop was still open. The pump of `link2` could be inside `pushRows`: `h2.Settle` returned, then `pushRows` started `afterGroupState` on `h2` (`internal/host/deliver.go:347-348`). The node's own cleanup then closed the store, and it settles only the first host, so nothing waited for `h2` again. The product cannot reach this: `nodeRuntime.closeCore` and `nodetest.Node.StopHome` close the home loop between the link and the settle, and `Loop.Close` waits for every pump, so no push is in progress when `Host.Settle` starts. Only an acceptance scenario that restarts a host settles it while its home stays open.
- Status: resolved on `devin/race-store-drivers` in the test harness: the cleanup of the restarted host in `features/m4_test.go` and `features/m3_test.go` is `link.Close(); n.StopHome(); h2.Settle()`, and the M3 restart host, which was never settled, is now settled. Proved with a build overlay that sleeps 150 ms before the `d.wg.Go` in `pushRows`: A4.3 fails under `-race` on `rebuild` and passes with the fix. Limit: the mid-scenario `activeLink.Close(); activeHost.Settle()` in `restartHost` can still leave a late push on the first host; it cannot reach a closed store, because `Node.Close` settles that host again.

### 2026-11-23 · bug · An external AI's task hand-out named the task by its id

- Symptom: a task or review hand-out carried the task's internal id in `task`; `external.md` section 3.2 requires `T-<num>`. This blocked A10.22, A10.23, the paused-task half of A10.24 and A10.26.
- Cause: `extHandOut` sent `Task: s.Task` (`internal/host/external.go:307`).
- Status: resolved on `devin/host-handout-task-flaky29`: `task` is formatted by `turnCtx.taskRef` and stays empty for a group run. `TestExternalTaskHandOutNamesTheTaskByItsNumber` pins `T-7`.

### 2026-11-23 · bug · An external AI's timer callback could outlive Settle

- Symptom: `Settle` could return while an external AI's run timeout (`extEnd`, which calls `Deliver`) or presence report (`extReport`) was still running, and a `leave` or a hand-out during shutdown armed a new timer, such as a 180 s presence timer.
- Cause: `settleExternal` (`internal/host/external.go:56`) stopped the run and presence timers but did not wait for a callback already running, and `extLeave` (`:209`) and `extOpenRun` (`:298`) armed `time.AfterFunc` timers after it.
- Status: resolved on `devin/host-teardown-race`, with the pattern of `limits.go`: `extPart` has `settled` and `wg`, every timer callback starts on `wg` under `extPart.mu` unless settled (`extPart.after`), no timer is armed once settled, and `settleExternal` waits on `wg`. `TestSettleExternalWaitsForItsTimersAndArmsNoMore` holds a run timeout inside `Deliver` and pins that `Settle` waits for it and that a `leave` and a hand-out after it arm nothing.

### 2026-11-26 · bug · A takeover that started while the AI was being given a turn lost the turn's items

- Symptom: the owner opened the terminal while delivery prepared a turn (a `git fetch`, `task branch`, `Compose`, the wait for a `max_open` slot): the items were acknowledged and their turn was refused.
- Cause: `takeoverStart` (`internal/host/takeover.go:132`) claimed the hold without regard to a delivery step that had read the holds before (`isItemHeld`, `internal/host/deliver.go`), and `Runner.Turn` then answered `busy`, which is logged.
- Status: resolved on `feat/host-jump`: `takeover.start` answers `busy` while delivery may have read the AI's holds for a turn that has not begun (`givingTurn`), under the lock of the claim. `TestTakeoverStartIsBusyWhileDeliveryGivesATurn`.

### 2026-11-26 · bug · Five holes in the host half of the queue jump (review of #401)

- Symptom and cause, all in `internal/host/deliver.go` at b4d7c8bef: (1) an item the home sent again while its jump turn was prepared interrupted that turn, and was only acknowledged then (`firstJump`, line 873, did not look for its audit row); (2) an accepted `task preempted` whose answer was lost left the task pre-empted (line 949 read "unreachable" as "refused"), and so did a host restart; (3) a jump whose session could not be prepared, as after a failed `git fetch`, had interrupted the running turn already and shut the AI's queue (`jumpRunningTurn`, line 859); (4) a takeover, a group pause or a second jump during the `task resume` round trip lost the turn of its own (`continueInterrupted`, lines 957 to 980, read the holds before it); (5) a takeover during the `task preempted` round trip lost the jump item (`stepSessions`, line 839, had taken the batch before it).
- Status: resolved on `feat/host-jump`: `firstJump` skips a delivered item; `task resume` is owed by the task's row, and `adoptPreempted` finds a pre-empted task again; `canPrepare` goes before the interrupt; the holds are read after both round trips. Tests in `internal/host/jump_test.go`: `TestResentJumpItemDoesNotInterruptItsOwnTurn`, `TestTaskPreemptedWithALostAnswerIsResumed`, `TestTaskFoundPreemptedAfterALostAnswerIsResumed`, `TestPreemptedTaskIsBroughtBackAfterAHostRestart`, `TestJumpWhoseSessionCannotBePreparedInterruptsNothing`, `TestHoldDuringTaskResumeKeepsTheTurnOfItsOwn`, `TestSecondJumpInTheThreadOfThePreemptedTask`, `TestTakeoverDuringTaskPreemptedHoldsTheJump`.

### 2026-11-25 · bug · A `jump` at a busy AI lost the message: the host half of the queue jump was never built

- Symptom: while an AI ran a turn, the author of a waiting message chose "jump the queue". The turn was not interrupted, nothing was reported, and the message never reached the AI. Contract: [agent-turn.md](planning/flows/agent-turn.md) section C, acceptance A5.11.
- Cause: planned as HS-15 and B-27 and not built. Delivery took a jump item although the AI was working (`isItemHeld`, `internal/host/deliver.go:909` before the fix), wrote the audit row and acknowledged it (`runSessionTurn`, `deliver.go:1190-1195`), and only then called `Runner.Turn` (`deliver.go:1196`), which answered `busy` (`internal/host/session.go:260`); the error was logged. Nothing read `Turn.Jump`, sent `task preempted` or the host's `task resume`, or composed a `ContinueTurn`. The home queues the jump under a new item number, and the host kept the retired item of the same message beside it (`enqueueHeld`).
- Status: resolved on `feat/host-jump`: delivery interrupts the running turn, keeps the item held until that turn has ended, reports the pre-emption of a task session and gives the interrupted session its own turn (`internal/host/README.md`, "Queue jump"). Tests: `internal/host/jump_test.go` (`TestJumpAtABusyAIDeliversTheMessageAfterTheInterruptedTurn` is the regression test), and the two A5.11 scenarios in `features/m5.feature`. The entry the web client's branch (`web/queue-jump`) added for this bug under Open is this one.

### 2026-11-23 · bug · FLAKY-30: the kill of a PTY that outlives its stop timeout was covered only by timing on Windows

- Symptom: the Windows CI step "Uncovered Windows product statements" sometimes reported `internal/host/pty.go:194.23,194.41`, the `killTree` closure of `ptyActivate`.
- Cause: the closure runs only when the process is still running once `ptyStopTimeout` has passed after the handle closes (`internal/host/pty.go:336-337`). The only test that reached it, `TestPTYCloseTimeoutAndReentrancy` (`internal/host/pty_test.go:819`), starts a helper that ignores `SIGHUP` and sets the timeout to 30 ms (`:821`). On Unix the helper outlives the close every time. On Windows, closing the pseudo console (`internal/host/pty_windows.go:48`) ends its client processes, so when the helper ended within 30 ms the kill was skipped.
- Status: resolved on `devin/host-teardown-race`: `TestPTYActivateKillsAProcessThatOutlivesTheStopTimeout` (`internal/host/pty_test.go:796`) activates a running helper with a handle whose close leaves it running, so the stop timeout passes and the tree is killed on every OS.

### 2026-11-23 · bug · FLAKY-28: a takeover test read the last process before the fake driver stored it

- Symptom: `TestTakeoverHandBackAfterACommitPostsTheFacts` panicked on Windows CI with index -1 in `fakeDriver.lastProc` (`internal/host/run_test.go:85`).
- Cause: `fakeDriver.Start` records the launch (`internal/host/run_test.go:43`) and stores the process only after it is made, under a second lock (`:61`). `count` returned the number of launches (`:73`), so the wait for the hand-back turn (`internal/host/takeover_test.go:425`) could end while `procs` was still empty, and `lastProc` (`:426`) indexed it at -1.
- Status: resolved on `devin/host-teardown-race`: `count` returns the number of stored processes, and `TestFakeDriverCountsAStartOnlyWithItsProcess` holds a start inside its process maker and pins that it is not counted until its process is stored.

### 2026-11-23 · bug · Host acceptance teardown restored hooks while host delivery still ran

- Symptom: under `-race`, the host acceptance scenarios reported a data race between restoring `host.Deliver`, the seam provided at `internal/host/deliver.go:28`, in `afterScenario` (`features/host_run_test.go:322`) and a delivery wakeup reading it (`internal/host/run.go:592` in `onProcessTurnEnded`, and `:851` in `finishTurnProcessExit`, reached from `:813`). The wakeups at `internal/host/run.go:763`, `internal/host/takeover.go:90` and `internal/host/limits.go:196` read it the same way.
- Cause: `hostRunSteps` (`features/host_run_test.go:313-323`) wrote the four hooks back in its After hook in every scenario, not only in its own. Each step set has its own After hook, and they run in the order of the sorted step set names, so in a later step set's scenario the hooks were written back before that step set settled its host (for example `cleanup(w.h.Settle)` in `features/host_start_model_test.go:151`). In the step set's own scenarios, the teardown (`features/host_run_test.go:374-377`) polled the session rows until they were `dormant` (`waitAllSessionsIdle`, `:383-398`) instead of settling the host, and a row changes before the loop starts its delivery wakeup. The product closes a host with `Settle` (`internal/node/node.go:390`), which already waits for all of those wakeups (`settleRuntime`, `internal/host/run.go:79`; `settleTakeovers`, `internal/host/takeover.go:106`; `settleLimits`, `internal/host/limits.go:93`).
- Status: resolved on `devin/host-teardown-race`: `installHRDefaultHooks` saves and restores the hooks in the scenarios that install them, the teardown settles the host instead of polling, and `TestSettleWaitsForDeliveryAfterAnExitMidTurn` pins that `Settle` waits for the wakeup after a mid-turn exit, as `TestSettleWaitsForRuntimeDelivery` does after a turn's end.

### 2026-11-23 · bug · FLAKY-31: the two-task fixture assumed one notice per turn and immediate browser delivery

- Turn-count cause: in 12 of 400 traced Linux race samples, the Claude secretary began turn 8 on item 10, the second task's `task_merged` notice. The home queues that notice in the committed merge result (`internal/home/machine.go:581–585`, `internal/home/cmd_merge.go:45–58`), but browser rows and host inbox frames have independent connection pumps (`internal/home/run_conn.go:46–75`). The former browser callback could record eight turns after the home had already merged at seven. All 12 failures had `summaries browser=2 home=2`; this turn did not make a third card.
- Summary timeout: holding the secretary after confirmation until both merge notices were queued deterministically reproduced `timed out waiting for two summary cards; summaries browser=1 home=1` with both tasks merged. The host routes both notices to the secretary's group session and delivers them together (`internal/host/deliver.go:734–739`, `:857–871`). The fake CLI consumes one script entry per turn, and the fixture scripts one summary per notice (`features/m5_test.go:1702–1714`). Windows job 137765616100 passed all 100 traced samples; the log from the original Windows timeout (job 137696879496) did not capture the count or the notices, so that occurrence cannot be attributed to batching with certainty.
- Fix: count review pass and merge in the home's existing post-commit hook (`features/m5_test.go:722–743`); still verify the merge message in the browser. In the scripted fixture, release task 1 after the confirmation turn starts and task 2 after the first summary arrives (`features/m5_test.go:1613–1651`), so the notices cannot share a turn. The forced-delay run then produced both summaries. No product change or timeout increase.

### 2026-11-23 · bug · FLAKY-19b: the offline test used an older sync sequence

- Symptom: under Linux race stress, the test expected sequence 8 but received sequence 9 in the matching `Synced` reply for request 4, in nine runs.
- Cause: the test read `Loop.Groups` after observing the working row, but the run loop pushes changed rows before publishing the group summary (`internal/home/run_group.go:170`–`:172`, `internal/home/run_push.go:32`–`:33`). The test could sample the old summary; the queued sync then read the committed sequence (`internal/home/sync.go:41`).
- Status: fixed in `internal/home/cmd_hostev_test.go:523` and `:589`. The negative sync barrier uses the observed row sequence and checks the next frame. No product change.

### 2026-11-23 · bug · FLAKY-21: a pairing node duplicated ICE candidates on the same socket

- Symptom: `TestNetStatusLiveNodeRuntime` timed out before the browser's `ctl` opened.
- Cause: pairing saved the shared paths (`internal/node/pair.go:345–349`) and `routesFor` appended them again (`internal/node/node.go:471`). Each candidate went twice over one socket. With the writer held during gathering, all 16 sender slots filled before the SDP answer and the server dropped it (`internal/rendezvous/server.go:159`). The original run has no packet trace, so this is the established defect and the likely cause of that run.
- Status: fixed by using each path ID once and retaining the current ICE configuration. In the same schedule the answer entered the queue and `ctl` opened.

### 2026-11-23 · bug · Windows A7.9 could keep the direct pair selected

- Symptom: Windows kept a working 500 ms direct pair even with a succeeded sub-millisecond relay pair. A7.9 needs a direct-to-relay transition on the original connection to exercise terminal handback.
- Cause: the fixture delayed direct packets but still delivered them (`features/m7_test.go`, before this fix). Pion v4.4.8 ranks succeeded pairs before it tests the RTT improvement (`selection.go:276–287`, `agent.go:2394–2459`).
- Status: the fixture cuts direct UDP, pairs the browser's TURN allocation with the node interface that owns the advertised relay IP, and waits for ICE selection. The Windows exclusion is removed. The separate relay-to-direct probe was dropped from version 1 in [#172](https://github.com/huanchong-99/SoloDawn/pull/172#issuecomment-7057401511).

### 2026-11-23 · bug · External AI removal could be triggered by one click

- Symptom: a click on Remove AI next to Setup revoked the member ID used by the runner and MCP configuration.
- Cause: the member card sent `member remove` directly from the button's click handler (`web/src/components/external/MemberExternal.svelte`).
- Status: resolved in U3-6. Removal requires inline confirmation, which Escape cancels.

### 2026-11-23 · bug · FLAKY-27: paused mentions could be delivered out of order

- Symptom: the OpenCode and Qwen A10.3/A4.7 examples intermittently failed at step 10 with `timed out waiting on M4 home frames`, even after the resumed reply had been posted.
- Cause: `selectEligibleBatch` in `internal/host/deliver.go:853` read the live group pause state for each queued item. A resume between checks left the older mention held and sent the newer one first. The older mention then started another turn, which could prevent settlement. The M4 waiter already matched replies by `re` and kept rows that arrived before them.
- Status: fixed in #348. Each batch uses one group pause snapshot; the acceptance timeout is unchanged.

### 2026-11-23 · bug · FLAKY-26 bulletin scenario posts before its node attaches to the team home

- Symptom: A9.23 intermittently times out waiting for the team bulletin after the secretary's fake CLI crashes.
- Cause: `prepareBulletin` (`features/s2_watch_test.go:170-180`) waits only on P's own connection to P2, not P1's attachment. `home.add` replies before P1 attaches: `internal/host/join.go:417-433` reconciles connections, but `internal/node/teams.go:17` runs the dial with `go dial`. An early bulletin post gets `home_unreachable` (`internal/host/tools_bulletin.go:90-95`); the fake CLI expects `isError:false` and crashes. Suppressing P1's dial in a throwaway overlay proved this error and crash path.
- Status: resolved in this change. For an uncached team, `bulletinTeam` uses `joinWaitHome` to await the team home's attachment with the same bound as `host.join`, preserving `home_unreachable` when the wait times out or is cancelled. Cached groups are still checked first. Deterministic tests cover attachment during the wait and cancellation; the attachment test fails without the fix.

### 2026-11-23 · bug · FLAKY-24 invitation test assumes the join reply precedes group pushes

- Symptom: `TestInvitationJoinAndMemberSurface` intermittently fails with "proto: a field that is always present is missing" while decoding the second join reply.
- Cause: `internal/node/admit_test.go:233` decoded the next frame as a command result. Admission adopts the staged device revocation (`internal/node/admit.go:119`), which fans out device events (`internal/node/devlog.go:355`) to group goroutines. The first group's revocation can announce `groups` (`internal/home/run_push.go:155`) before the second group's join reply is queued. Unsolicited pushes are not ordered against replies from another group.
- Status: resolved in this change. The test accepts intervening `groups` frames and checks the join reply's request id and success.

### 2026-11-23 · deviation · Two profile acceptance sequences need clarification

- Symptom: A10.11 expected the strong secretary to be stronger than the assignee S, which the cast also called strong. A10.12 asked the same task to fail a review after passing one.
- Cause: [the profiles contract](planning/contracts/step-3/profiles.md) orders tiers strictly. A passing review moves the task to `merge`, but a merge conflict can return it to `local_check` (row 9, `conflicted` in `internal/home/machine.go`), so a later failure on the same task is reachable.
- Status: decided in the I3-2b review on 2026-11-23: A10.11 makes S standard and C strong in the secretary subcase; A10.12 uses a fresh task flow to test the same failure counter starting from nil after clearing. No product change is needed.

### 2026-11-23 · bug · Runtime work could outlive host shutdown

- Symptom: the macOS race detector reported a shared hold hook being restored while a host still read it.
- Cause: runtime callbacks in `internal/host/run.go` could deliver queued items or flush member status from an untracked goroutine. Member status calls `Host.Held` in `internal/host/host.go`.
- Status: runtime shutdown now joins those callbacks before returning. The reported macOS failure was not reproduced on Linux.

### 2026-11-23 · bug · Host tests observed status before work completed

- Symptom: a replacement CLI sometimes appeared to have its old turn metadata, the workspace test could time out before fake GitHub finished merging, and the hand-back test could look up a process before one existed.
- Cause: `startReplacementProcess` in `internal/host/run.go` sends the replacement status before setting the turn and writing it to the CLI. In `internal/host/forge_test.go`, the fake GitHub CLI inherited the race detector's exit delay for each invocation. In `internal/host/run_test.go`, the fake driver records a launch before it constructs and publishes the corresponding process.
- Status: the tests wait for the replacement turn, merged command and hand-back process; the fake forge CLI disables its race-detector exit delay.

### 2026-11-23 · bug · The terminal page stayed at "Direct connection required" after the path recovered

- Symptom: a terminal page opened on a relayed path kept saying "Direct connection required" when the path became direct again, until the page was reopened.
- Cause: `follow` in `web/src/lib/terminal.svelte.ts` (`:305`) starts a takeover only from `idle`, and two routes left the phase at `relay`: `Takeover.#relayed` (`:368`) set an idle takeover to `relay` when the browser's path was relayed, and `#refused` (`:364`) sets `relay` when the computer refuses `takeover.start` with `relay` while the browser's path still reads direct.
- Status: resolved in U11. `#relayed` no longer moves an idle takeover to `relay`, and it returns a `relay` phase to `idle`, so the takeover starts once the path is direct again; the takeover bar already says "Direct connection required" for an idle takeover on a relayed path. The `web_terminal.feature` scenario "A relayed path that recovers to direct opens the terminal without reopening the page" and two `terminal.test.ts` cases cover both routes.

### 2026-11-23 · bug · A9.24 can stall before submitting the retry commit

- Symptom: A9.24 times out at “both tasks merge after checks and cross-review and receive summaries.” In a stalled run, the retry commit gets its standalone `watch-push` but no submission push or `watch-checks` call, and the task stays at `active/local_check`.
- Reproduce: use a Go overlay to set `godog.Options.Paths` in `features/acceptance_test.go` to `[]string{"s2_watch.feature:37"}`, then run `go test -overlay <overlay.json> -tags acceptance -race -run '^TestFeatures$' -count=50 -timeout=35m ./features/`. The same step timed out on `origin/rebuild` at `aee115ba9` with `-count=8`.
- Cause: The home replied to `task checked` before publishing the updated task row. The next `submit_task` could compare the retry commit to the previous commit's cached check and reject it before pushing.
- Status: resolved in PR #338. The home sends changed rows before command replies; `TestRunPushesChangedRowsBeforeCommandReply` verifies the order.

### 2026-11-23 · bug · A9.24 can run hosted checks again for the superseded commit

- Symptom: A9.24 can report an unexpected `watch-checks` call for the first commit of the task that is reassigned.
- Cause: `internal/host/deliver.go:1378-1379` can select a stale `cloud_check` row after `tskFinishCloud` releases the polling slot, even after the task has reached review. The home acknowledges the result before sending the updated row (`internal/home/run_group.go:170-171`).
- Status: resolved in PR #335. The host keeps claims for accepted and superseded commits, while allowing retries after failed result delivery or an accepted resubmission. `TestStaleCloudCheckRowDoesNotRestartCompletedPoll` verifies that resuming from a stale row does not make a second forge call.

### 2026-11-25 · bug · An external AI could not write in its task's thread

- Symptom: in an open run of an external AI, `post` with kind `progress`, which its task instructions order, was refused by the home ("the post belongs in a task thread"); a plain `post` in a task or review run, or in a group run from a thread, went to the main line; `where` `thread` was `bad_request`; an `ask` in a group run from a thread went to the main line; and every call had depth 1, so the limit of 4 did not count external AIs.
- Cause: `extHandOut` dropped the `Turn` that `Compose` returns (`internal/host/external.go:293`), and `externalCaller` built the caller without one (`internal/host/tools_external.go:41`).
- Status: resolved on `fix/external-thread`: the open run keeps its turn and the caller has it until the run ends. `TestExternalCallsGoToThePlaceOfTheRun` and an A10.22 scenario fail with the old code. A node restart ends the run, so its place needs no recovery ([external.md](planning/contracts/step-3/external.md), 3.3).

### 2026-11-25 · bug · The answer to a CLI's question could outlive the host's Settle and crash the node

- Symptom: `Settle` returned while `runAskedEvent` still ran; `bg.stop name=ask` followed `settle.end`. With a card open it only waited, but an ask that had not posted its card yet, or whose post failed with the connection, wrote to the store after the node closed it: `panic: send on closed channel` (`internal/store/store.go:95`).
- Cause: `startAskedGoroutine` (`internal/host/approve.go:110`) started it with a plain `go` statement; only `TurnBegan` and `TurnEnded` of its session cancelled it, and no `Settled` entry did. The goroutines of a watched terminal (`internal/host/watch.go:99` and `:173`) are not waited for either; they end with their connection, which `node` closes first, and touch no store.
- Status: resolved on `fix/host-cloud-poll-settle`: the asks run on the runtime's wait group, and `Settle` (entry `"runtime"`) cancels them after it has ended the CLI processes and waits. `TestSettleWaitsForAnAskWithTheHomeClosed` and `TestSettleEndsAnAskWhoseCardIsBeingPosted` fail with the old `go` statement.

### 2026-11-25 · bug · The hosted checks poll could send a command after the host settled

- Symptom: after `Settle` a poll of hosted checks still ran, and the result it got sent `task cloud`, so `bg.stop name=cloud_poll` and the command could follow `settle.end`.
- Cause: `startHostedCheck` (`internal/host/tools_task.go:463`) started the poll with a plain `go` statement, and no `Settled` entry ended it or waited for it.
- Status: resolved on `fix/host-cloud-poll-settle`, with the pattern of `limits.go`: the polls run on `tskPart.wg`, and `Settle` (entry `"task"`) cancels them and waits. `TestSettleEndsTheHostedChecksPoll` checks that the poll has stopped when `Settle` returns and that none starts afterwards; it fails with the old `go` statement.

### 2026-11-22 · limitation · Browser invitation joining needs the node and home implementations

- Symptom: a current node does not admit a browser through an invitation; it knows only protocol version 1.
- Cause: W2-1 implements the browser exchange in `web/src/lib/join.ts`. The invitation and joining connection on the node are T2-1b, and the home commands are T2-1a. U2-1 supplies the join page and member controls.
- Status: resolved. T2-1a (#309), T2-1b (#304) and W2-1 (#307) are merged, but they never registered a home handler for `invite` (`internal/home/cmd_people.go` registered only `join`; found 2026-11-25), so no group could admit a second person. #382 added the handler. The two fixture faults that then remained are fixed with A9.1 to A9.6, A9.27 and A9.28 enabled: `peerNode.initCore` in `features/helpers_test.go` started the in-process home without `LoopConfig.Invite`, so the link of A9.2 was empty; and a person who joined in the cast of `features/s2_cast_test.go` sent no `home.add`, as the web client does (`web/src/lib/join.ts:86`), so the host answered `host.join` with `home_unreachable` (now `JoinGroup`).

### 2026-11-22 · bug · Codex treated every failed thread resume as a lost session

- Symptom: a refused model or effort on `thread/resume` could replace the session instead of reporting a crash and retaining its history.
- Cause: `internal/host/codex.go`, `codexProcess.handleStartupError`, emitted `NoSession` for every error on request 2 when a saved thread id was present, without distinguishing a missing thread from another rejected setting.
- Status: resolved in T3-11. Only known missing-thread diagnostics emit `NoSession`; other startup errors close the CLI and take the existing crash path. Fake-CLI tests cover refused-value crashes and retained session ids. Live model/effort selection and native terminal behavior remain unverified.

### 2026-11-22 · limitation · Model discovery has synthetic fixtures, not live captures

- Symptom: Claude Code and Codex model discovery was tested only against scripted listings, so the fields of a real catalog and whether a nonempty Codex `serviceTiers` means a fast mode were unverified.
- Cause: neither real CLI was available for T3-9. `internal/host/fakecli/testdata/claude-models.json` and `codex-models.json` are synthetic fixtures of `step-3/drivers.md` section 10.1.
- Status: resolved in T3-9 by a live check on 2026-11-23 with Claude Code 2.1.311 and Codex 0.164.0, no model called. Claude: the first stdout line is the `control_response` with 14 models, exit 0 after stdin closes, and some entries (Haiku 4.5) have no `supportsEffort`. Codex: two notifications (`remoteControl/status/changed`, `account/updated`) before the answer, then 8 models in one page with `nextCursor` null, each with `serviceTiers` `[{"id":"priority","name":"Fast",…}]`. The readers in `internal/host/models.go` handle all of it. No listed id has a word beyond the two rows of `internal/host/tiers.go`. The fakecli fixtures stay synthetic.

### 2026-11-22 · limitation · `host.CheckTakeover` and `driver` in `host.info`

- Status: resolved. T3-1 (#275) wired `checkTakeover` into `takeover.start` and uses its `assignFirst` result. T3-2 (#289) added `driver` for each agent in `host.info`.

### 2026-11-22 · bug · On Windows the last output of a CLI that exits by itself during a takeover is lost

- Symptom: when the interactive CLI exits on its own while the owner's terminal is open, the last lines it printed (for A7.4's fake CLI, `{"bye":…}` and the echo of the input line) never reach the pty channel. M7's A7.4 therefore does not wait for `"bye":` (`runOneSwitchCycle` in `features/m7_test.go`); it waits for the hand-back post, and the exit entry in the CLI log proves the exit. Seen on `test (windows-latest)` of PR #287; Linux and macOS deliver the line.
- Cause: on Windows `ptyBeforeExitStop` does nothing (`internal/host/pty_windows.go:16`), so the host closes the terminal as soon as the CLI exits and drops the frame that ConPTY has not rendered yet. Diagnosis: `WIN-CONPTY.review.md`.
- Status: resolved in win-pty. On process exit with a terminal attached, `ptyBeforeExitStop` closes the pseudo console but keeps its pipes, and the terminal closes when the output reaches its end (`internal/host/pty_windows.go`); `TestPTYWindowsSelfExitDeliversLastOutput` pins it on Windows CI.

### 2026-11-22 · bug · Role finishing read pending cards for commands without task work

- Symptom: the home command-replay acceptance scenario panicked when applying a note command with no disk reader.
- Cause: `cftFinish` in `internal/home/cmd_confirm_task.go` expired role cards on every successful command through `Disk.Pending`, including commands with no task rows or task items.
- Status: resolved in #312. Results without task work return unchanged; card expiry belongs to the timer, and role decisions write only changed task waits.

### 2026-11-22 · bug · Capability fixture cleanup races with A1.1 onboarding

- Symptom: race acceptance reported `WARNING: DATA RACE` in the A1.1 onboarding scenario even though its steps passed.
- Cause: the capability fixture registered a `DriverCaps` restore for every scenario (`features/host_caps_test.go:41`); A1.1 did not change the table, but its in-process pairing node read it while handling `host.info` (`internal/host/caps.go:46`). The I2-1b scenarios run after A1.1 and start their nodes as subprocesses.
- Status: resolved in I2-1b by registering the restore only when a capability step changes the table (`features/host_caps_test.go`, `setDriverCaps`).

### 2026-11-22 · deviation · `driver` in `host.info` was optional until T3-2 filled it

- Symptom: `planning/contracts/step-3/drivers.md` section 6 says each entry of `agents` carries `driver` always, but the frozen shape lets it be absent: Go `Driver *Driver` with `omitempty` (`internal/proto/host.go`, `HostAgent`), TS `v.optional(Driver)` (`web/src/lib/proto.ts`, `HostState.agents`), and `agents.driver` under `optional` in `internal/proto/testdata/wire.json`.
- Cause: nothing on rebuild fills `driver` yet; a required field now would make the web client reject every real `host.info` until T3-2 lands. P-5 froze `driver` as `accounts` and `takeover` only; the other declaration fields stay in the host's table (coordinator, 2026-11-22).
- Status: resolved by T3-2: `driver` is required in Go (`internal/proto/host.go`, `HostAgent`), TS (`web/src/lib/proto.ts`) and `wire.json`, and `Host.Agents` fills it for every agent. Before that it was open, accepted for P-5. T3-2, the pull request that fills `driver` for every agent, makes it required: Go ``Driver Driver `json:"driver"` `` (value type, no `omitempty`), TS `driver: Driver`, regenerate `wire.json` with `go test ./internal/proto -update`, and drop "once populated" from `internal/proto/README.md` (the `host.info` bullet). Also accepted: P-5 does not freeze the tool lists in `internal/proto/testdata/tools.json` (`planning/contracts/step-3/knowledge.md` section 7), because `internal/host` tests require it to equal the registered schemas; T3-5 adds `note_list` `type` and `propose_brief` `preset`, T3-6 adds `kb_put` and `kb_search` to all five lists, and the two merge with `go test ./internal/proto -update`.

### 2026-11-22 · bug · Watch confirmation admission rejected fields outside its checks

- Symptom: a person's watch request was refused when it carried a thread, reply or text, although the watch admission contract does not restrict them.
- Cause: `cfmWatchShape` (`internal/home/cmd_confirm.go:137`) checked those fields in addition to the checks in `planning/contracts/step-2/confirm.md` section 3.
- Status: resolved in #310 by retaining only the contracted watch checks.

### 2026-11-22 · deviation · Home confirmation reminders were not sent

- Symptom: cross-owner requests in the unfinished confirmation implementation created cards, but no subscribed browser received a confirmation reminder.
- Cause: the standalone reminder builder in `internal/home/cmd_confirm.go` was not connected to the Web Push extractor registry.
- Status: resolved in T2-2a: confirmation cards use the committed-row push path, name the AI, omit the title, and target only `body.to`. Tests decrypt the outgoing push payload.

### 2026-11-22 · bug · Removing a person left group devices live when another local membership remained

- Symptom: a removed person's device rows and in-memory device log remained in the group when they also belonged to another group homed on the node.
- Cause: `internal/home/cmd_people.go:124` put group device retirement under the last-local-membership condition intended only for node-wide `device_log` and `push_sub` deletion.
- Status: resolved in #309. Removal always retires the group's devices and drops its in-memory log; only node-wide deletion waits for the last local membership.

### 2026-11-22 · deviation · The home handled only AI member changes

- Symptom: an invited person could not join, change role or leave through home commands.
- Cause: `internal/home/cmd_member.go` dispatched only AI members; the joining seam had no production `join` handler.
- Status: resolved in T2-1a by `internal/home/cmd_people.go`: joining, restore reconnection, owner role changes, and cascading removal with last-membership device-log and push-subscription cleanup. Invitation transport and node connection closure belong to T2-1b.

### 2026-11-22 · bug · HM-13 and HM-15 home tests disagreed on rounds after both landed

- Symptom: after `origin/rebuild` (HM-15's `post/call` and `post/reply`) was merged into HM-13, `go test ./internal/home/` panicked with `home: two functions under post/call` in `TestRoundsTenExchangesPauseAndResume`, and two `TestCall*` cases got `thread.rounds` one higher than they expected.
- Cause: `rounds_test.go` cloned `home.Commands` and added a stand-in `post/call`, written before HM-15 existed; `Table.Add` refuses a second function under a key (`internal/home/home.go:307`). The `TestCall*` cases were written without HM-13's row 21, so they expected a thread call, and a thread reply that mentions an AI, to leave `thread.rounds` unchanged; `task-machine.md` row 21 and `flows/ask.md` A.3 and D count both.
- Status: resolved on `gemini/HM-13` (#190): the stand-in is gone and the rounds test uses the real `post/call`; the call tests expect the round.

### 2026-11-22 · bug · `Host.Hand` could lose the item that answers a wait when the wait ended at the same moment

- Symptom: an `ask` reply or an approval decision that arrived just as its wait ended was acknowledged and never seen. `Hand` returned `waited = true`, so `tryOpenWait` (`internal/host/deliver.go:528-540`) acked the item and did not queue it, while the waiting tool had already called `done()` and found its channel empty (`awaitApprovalItem`, `internal/host/approve.go:648-665`). The HS-12 reviewer's probe lost 16 of 200,000; `TestHandAndEndOfWaitNeverLoseAnItem` lost 1 to 18 of 200,000 in each of 20 runs without the fix (84 to 162 under `-race`).
- Cause: `(*Host).Hand` (`internal/host/host.go:293-297` before the fix) removed the wait through `release`, unlocked `h.waiting`, and only then sent into the channel. `done()` and the tool's last look at the channel could run in that gap.
- Status: resolved in unit `HAND-RACE`: `Hand` removes the wait and sends the item while it holds `h.waiting` (`internal/host/host.go:290-297`); the channel has room for the one item, so the send never blocks. The same test keeps every item in 20 runs, and in 5 under `-race`.

### 2026-11-22 · bug · `home.Loop.Close` and `node.Sessions` left connection pumps and host commands running into `Store.Close`

- Symptom: `A5.19` raced under `-race` between `Store.Close` and `Conn.pump` delivering an inbox frame to the host, and an in-flight host command on `Sessions` could write its receipt after `nodeRuntime.Close` closed `node.db`.
- Cause: `Loop.Attach` (`internal/home/run.go`) started `Conn.pump` and `Conn.watch` outside `Loop.wg`, and `Sessions.sesCmd` (`internal/node/session.go`) started `sesHostCmd` outside `nodeRuntime.bg`.
- Status: resolved in #250: `Loop.Attach` tracks `Conn.pump` and `Conn.watch` in `Loop.wg`, `SessionConfig.Spawn` runs `sesHostCmd` inside `nodeRuntime.bg`, and the 30 ms settle in `features/m5_test.go` is removed (`TestRunCloseWaitsForConnectionSends`, `TestCloseWaitsForInflightHostCmd`).

### 2026-11-21 · deviation · Step 17 session and workspace cleanup awaits a close hook from the session runtime

- Symptom: when a `rows` push marked a task `merged`, `cancelled` or reassigned, changed a task's reviewer, or marked a member `gone`, `deliver.go` did not end the session row or remove the task or review worktree.
- Cause: `planning/flows/agent-turn.md` D17 and G require closing any open CLI process and waiting for its exit (C-6) before ending the session row or deleting its working folder, and `host.Runtime` exposed `Turn` and `Answer` only.
- Status: resolved in unit `HS-23`: `Runtime.Close` (`internal/host/host.go`, `internal/host/run.go`) closes any open CLI process for a session and waits for its exit, and `cleanupGroupState` (`internal/host/deliver.go`) closes and ends obsolete task, review and gone-member sessions and removes obsolete `wt/<num>-<slug>` (plus `DeleteBranch` on `merged` and `PushBranch` on `github` handoff) and `review/<num>` worktrees idempotently across pushes and restarts.

### 2026-11-21 · limitation · Ending a session through `Sessions.End` does not close an open CLI process

- Symptom: when a session ended (`planning/flows/agent-turn.md` section D, step 17) and its caller removed the row with `Sessions.End`, an open CLI process for that session stayed in `runtimePart.procs` until its `idle_minutes` timer expired or `max_open` evicted a slot.
- Cause: `Runtime` exposed `Turn` and `Answer` only, and `Sessions.End` (`internal/host/session.go`) runs inside a `Store.Write` transaction without calling into the session runtime.
- Status: resolved in unit `HS-23`: `Runtime.Close` closes and waits for any open process outside `Store.Write`, and both step 17 cleanup (`internal/host/deliver.go`) and `host.leave` (`internal/host/join.go`) call `Runner.Close` before `Sessions.End`.

### 2026-11-22 · bug · Uncoordinated group sync in `host.join` raced with `pushRows` and wiped `Host.Rows`

- Symptom: on macOS CI (`features/m3_test.go:128-131`), M3 setup steps calling `host.join` over `LocalLink` occasionally failed within 0.1 s with `not_found: the group does not exist` (`internal/host/join.go:333`).
- Cause: `joinFetchGroup` (`internal/host/join.go:327` before the fix) called `joinSyncState`, which sent `proto.Sync{Group: group, Since: 0, StateOnly: true}` without marking `groupSync.syncing = true` in `Part[delivery](h)`. When the home pushed `reset` and `rows` before `synced`, `pushRows` -> `ensureGroupSynced` (`internal/host/deliver.go:279`) saw `syncing == false && live == false` and launched a concurrent `syncOneGroup` (`Since: 0`), whose `reset` push wiped `Host.Rows` (`internal/host/deliver.go:266`) after `LocalLink` queued `synced` for `joinSyncState` and before `joinFetchGroup` checked `joinCachedGroup` (`internal/host/join.go:330`).
- Status: resolved in branch `gemini/flaky-tests-6`: `joinFetchGroup` coordinates state synchronization through `syncGroupState` (`internal/host/deliver.go`), which claims `groupSync.syncing` (or waits on an in-flight sync) and shares `syncOneGroup` and `decodeSyncAnswer` so `pushRows` starts no duplicate cold sync. Test: `TestHostJoinSyncCoordinatesWithDeliveryPushesAndInflightSync`.

### 2026-11-21 · limitation · Background hosted-check polling sends `task cloud` once

- Symptom: if the home connection was unreachable at the moment `TaskForge.WaitHostedChecks` settled, the `task cloud` command failed and the task stayed in `cloud_check`.
- Cause: `tskPollHostedChecks` (`internal/host/tools_task.go`) calls `Host.Command` once without a separate retry loop on `home_unreachable`.
- Status: resolved in unit `HS-22`: once `tskFinishCloud` releases the claim after the failed command, the group sync on reconnect (`Attach` -> `syncOneGroup`) and any `rows` push run `afterGroupState` -> `resumeGroupHostedChecks`, which polls the `cloud_check` task again and sends `task cloud`. Test: `TestResumeHostedChecksAfterRestart`.

### 2026-11-21 · limitation · `Sessions` implements only the server side of a `ctl` connection

- Symptom: a host on one node had no outgoing `ctl` client session yet to a home on another node (sending `hello` with `kind: "host"`, reading `welcome`, exchanging `devlog` both ways, and passing pushes to `Host.Push`), so cross-node AI sessions (`A3.3`, unit `I-6`) had no transport path.
- Cause: `internal/node/session.go` (`N-2`) served inbound `ctl` connections (`Serve`); the client side of a `ctl` session was split out by the coordinator's decision of 2026-11-21.
- Status: resolved in unit `N-2b` (#222, `internal/node/client.go`: `(*Sessions).serveClient` and `(*Sessions).connectHome`, plus `attachRuntime` in `internal/host/run.go`) and wired in `N-4` (`internal/node/node.go`, `Host.Connect` -> `connectHome` -> `peer.Peer.Connect`).

### 2026-11-21 · deviation · The local link routes `cmd`, `sync` and `ack` until `fetch` lands

- Symptom: on `LocalLink`, a `fetch` request from the host was answered `bad_request`.
- Cause: `LocalLink.toHome` (`internal/node/local.go`) dispatched `cmd` to `Loop.Cmd`, `sync` to `Loop.Sync` and `ack` to `Loop.Ack`, while `Fetch` (`HM-3`, #207) landed in a separate unit.
- Status: resolved in branch `gemini/N-4` after `HM-3` (#207): `LocalLink.toHome` (`internal/node/local.go`) and `NewSessions` (`internal/node/session.go`) both route `fetch` requests to `Loop.Fetch` (`TestLocalLinkCommandAndRowPush`, `TestHomeLoopIntegrationAndDevlogForwarding`).

### 2026-11-21 · limitation · `home.Loop` keeps the `Owner` it was started with across adoption

- Symptom: when an unowned node starts `home.Loop` with `Owner: ""` and is later adopted over a pairing `ctl` session, `Logs.Owner()` becomes the adopter's user id (`internal/node/devlog.go`, `dvlAdopt`), while the running `home.Loop` still has `LoopConfig.Owner == ""` (`internal/home/run.go:65`) and refuses `group create` until the loop is restarted.
- Cause: `LoopConfig.Owner` was read as a static string when `StartLoop` constructed group states (`internal/home/run.go:175`, `:198`).
- Status: resolved in branch `gemini/N-4`: `LoopConfig.OwnerFn` (`internal/home/run.go`) reads the node's owner dynamically (`logs.Owner`), so an unowned node's `home.Loop` sees the adopted owner immediately without restarting (`TestRunDynamicOwnerFn`, `TestAdoptionRendezvousAndForegroundStop`).

### 2026-11-20 · deviation · Nothing resumes hosted checks after a node restart yet

- Symptom: after a restart, a task at step `cloud_check` is not polled again, so it stays there until something else moves it (decision D3 of the host-contract audit).
- Cause: `HostedChecksToResume` (`internal/host/forge.go`) selects the tasks to poll again and gives each its PR, commit, branch and `checks.local.at`, where the 60-minute limit counts from, but no code ran on group-state sync or `rows` pushes to call it and start `WaitHostedChecks` for each.
- Status: resolved in unit `HS-22`: `afterGroupState` (`internal/host/deliver.go`) calls `resumeGroupHostedChecks`, passing the group's task rows and local AI member ids to `HostedChecksToResume`, and `startHostedCheck` (`internal/host/tools_task.go`) claims `(group, task)` idempotently by commit in `Part[tskPart](h)` so a second `rows` push or a duplicate `submit_task` leaves a running check alone. Test: `TestResumeHostedChecksAfterRestart`.

### 2026-11-20 · deviation · `solodawn pair` does not start a node that is not running

- Symptom: `local.md` section 1 says `solodawn pair` "starts the node if needed". With no node running, `solodawn pair <secret>` fails on the dial of the local socket and pairs nothing.
- Cause: `pair` (`cmd/solodawn/main.go:61` before the fix) only sent the command through `Command.send` (`internal/node/cmd.go:40` before the fix).
- Status: resolved in branch `gemini/N-4`: `(Command).dialOrStart` (`internal/node/cmd.go`) spawns a detached `solodawn node` (`startDetached` in `proc_unix.go` / `proc_windows.go`) when `c.Cmd == "pair"` and `sock` is absent, then polls until `sock` accepts (`TestPairAutoStartsNode`).

### 2026-11-20 · limitation · End-to-end tests cannot run a node until the node runtime is wired

- Symptom: `startNet` of `web/e2e/nodes.ts` waits until each node answers on its local socket. Every node exits with status 1 and `node runtime is unavailable` right after it makes its data directory and binds the socket, so a start that awaits nodes fails with that message. The harness's own scenarios start nodes with `awaitNodes: false`. A node is started with no flags, because `solodawn node` takes none yet: it has no signalling or web port and no rendezvous address, and the harness hands over only its HOME, data directory, socket and environment.
- Cause: `startNode` was a stub that returned that error until N-4 assembled the runtime (`internal/node/socket.go:181-183` before the fix).
- Status: resolved in branch `gemini/N-4`: `startNode` points to `runForegroundNode` (`internal/node/node.go`), starting the full node runtime (`node.db`, `device.key`, `logs/node.log`, `home.Loop`, `host.Host` + `ServeMCP`, `AttachLocal`, `Logs` + `Sessions`, `peer.Peer`, embedded web UI + `/rv` with `:47800` to `:0` multi-node fallback, and local socket commands).

### 2026-11-21 · deviation · `peer.Connection` delivers `re` responses without waiting for earlier queued pushes

- Symptom: on a WebRTC `*peer.Connection`, when a remote home sends `rows` pushes immediately followed by a `synced` response (`planning/contracts/datachannel.md` section 4), `(*peer.Connection).Request` could return `synced` before the `serveClient` reader goroutine had read `rows` from `(*peer.Connection).Read` and applied them to `Host.Rows`. A caller such as `host.join` (`HS-21`) that reads `Host.Rows` immediately after `sync` returns could see `not_found` if scheduled ahead of the reader goroutine.
- Cause: `(*peer.Connection).deliver` (`internal/peer/ctl.go`) previously wrote non-`re` frames to the buffered `incoming` channel (`Read`) and wrote `re` frames directly to `waiting[re]` (`Request`) on Pion's callback goroutine, with no ordering barrier between the two channels.
- Status: resolved in #222: `(*peer.Connection).deliver` queues valid `re` responses in `incoming` alongside requests and pushes, and `(*peer.Connection).Read` hands any queued `re` frame to `response` before reading on; `(*Sessions).serveClient` (`internal/node/client.go`) starts its read loop before sending `hello`, applies `devlog` pushes at once, and holds other pushes until `Host.Attach` has returned. Test: `TestPushesDeliveredBeforeAnswer`.

### 2026-11-20 · limitation · The gate measures CRAP 6.01 for `deliver`, which Go covers fully

- Symptom: `crap.py` gave `deliver` in `internal/peer/ctl.go` a CRAP score of 6.01, above the limit of 6, while `go test -cover` reported `internal/peer` at 100% of statements.
- Cause: the empty `case <-c.Done():` of the `select` previously in `deliver` was a cover block with no statements and count 0, counted as an uncovered line at complexity 6.
- Status: resolved in #222: `queue` holds the `select`, `validFrame` checks incoming `ctl` framing, and `deliver` has complexity 2 (`lizard -C 6 -w` clean, 100.0% statement coverage).

### 2026-11-21 · deviation · Dead `cause` path in `internal/home` and `internal/store`

- Symptom: the home and store still carried, wrote and read an item `cause`, although `P-1b` re-froze `InboxItem` and `Item` without `cause` under the owner's 2026-11-21 decision (no attack-and-defence code). Nothing on the wire or on the host read it.
- Cause: `internal/home/home.go` (`Item.Cause`), `internal/home/run_group.go` (`runCause`), `internal/home/cmd_brief.go`, `internal/home/machine.go` and `internal/store/inbox.go` together with the `inbox.cause` column were built before `P-1b` dropped `cause`.
- Status: resolved in branch `gemini/HM-7`: removed `Item.Cause`, `runCause`, the dead `cause` assignments and assertions across `home`, `store` and `features/home_brief*`, and the `inbox.cause` column in `internal/store/schema.sql`.

### 2026-11-21 · deviation · Inbox items are not yet pushed in the wire `inbox` frame

- Symptom: the run loop stored an item with the seam's `Item` as JSON in the `env` column and pushed it as `home.ItemsPush`, one `home.QueuedItem` per frame, where `planning/contracts/datachannel.md` section 6 (re-frozen in P-1b as a plain unsigned `item` object) has an `inbox` frame of `InboxItem`s with optional `msg` and `task` row copies.
- Cause: the wire `item` and `inbox` frame belonged to unit HM-7; the loop had `runSeal` and `runItemFrame` (`internal/home/run_push.go`) in their place.
- Status: resolved in branch `gemini/HM-7`: `internal/home/inbox.go` encodes plain `proto.Item` JSON in `inbox.env`, copies live `msg` and `task` rows into `proto.InboxItem` when sent, batches due items per connection into `proto.Inbox` frames up to `runRoom` (61,376 bytes), implements `(*Loop).Ack` (routed from `LocalLink.toHome` and `Sessions.sesPush`), and paces excess wakes under `limits.wakes_per_hour` (`TestInboxGoldensAndRowCopies`, `TestInboxBatchesItemsToTheCtlLimit`, `TestInboxAckDeletesOnlyTheHostsOwnItem`, `TestInboxWakeUpCapHoldsAndReleasesItems`, `TestLocalLinkAckDeletesQueuedItem`).

### 2026-11-21 · limitation · A queued task's `stats.started` is the time its brief was confirmed

- Symptom: a task created queued by a confirm has `stats.started` set when it is created, and it keeps that time when the merge of its dependency makes it active. The minutes a summary counts for it include the time it waited.
- Cause: `briefCreate` sets `created` and `stats.started` to the home's time (`internal/home/cmd_brief.go:307`); row 8 of `planning/contracts/task-machine.md` does not name `stats.started`, and `Step` does not set it on a release (`internal/home/machine.go:577`). The contracts did not say when a queued task starts.
- Status: resolved in unit HM-12: when `task merged` releases a `queued` task into `active`, `mrgReleaseOne` (`internal/home/cmd_merge.go`) sets `stats.started` to the home's time (`c.Now`), and `planning/contracts/rows.md` section 4 states the rule.

### 2026-11-21 · limitation · A secretary's own hand-back summary queues a `notice` for the secretary

- Symptom: when the owner hands the secretary's terminal back and a `handback` message is posted in the main line signed for the secretary, the home queued a `notice` (`code: handback`) for `group.secretary`, waking the secretary again for the summary it or its host just posted.
- Cause: `planning/contracts/commands.md` 1.3 specified `item notice for the secretary` on `handback` without an exception when `as` is the secretary.
- Status: resolved. `gemini/HM-6` (#181) removed `post` of kind `handback` with takeover and made `planning/contracts/commands.md` 1.3 state that a post never queues an item for its own author (`postWakes` skips `msg.By`; tested in `internal/home/cmd_post_test.go`). HS-20 (#273) brings `handback` back: a hand-back queues the `notice` only when it is posted in a thread (`postHandback` in `internal/home/cmd_post.go`; `planning/contracts/commands.md` 1.3), so the secretary's own main-line hand-back queues none.

### 2026-11-21 · deviation · `web/src/lib/peer.ts` had no method to send a `ctl` push

- Symptom: onboarding (`flows/pairing.md` step 10) and device management must push `devlog` frames (`{"t":"devlog","user":"…","entries":[…]}`) without a request `id` (`planning/contracts/conventions.md` section 3). `Connection` and `Peer` in `web/src/lib/peer.ts` exposed `request(req)` only, which always attaches an `id` and awaits a response, while `#ctl` and `#send` were private.
- Cause: unit W-2 implemented `request` and incoming `onpush`, omitting outgoing pushes (`internal/peer/ctl.go` has `Send` beside `Request`).
- Status: resolved in #212: `Connection.send` and `Peer.send` send a `Devlog` frame without `id` and enforce the 60 KiB `ctl` frame limit (`web/src/lib/peer.ts`). Tests in `web/src/lib/peer.test.ts`.

### 2026-11-20 · limitation · A task's `branch` is text that no reader checks

- Symptom: `proto.Decode` and the web client refuse a task row whose `slug` the home's rule cannot make (unit P-1), but take any text as its `branch`, and as the `branch` of `task branch`. `contracts/local.md` section 7 derives the branch from `num` and `slug` (`task/<num>-<slug>`); `HostedChecksToResume` previously copied `branch` from the row (`internal/host/forge.go:220` before `HOST-BRANCH`), from where it would reach `git` and `gh` arguments.
- Cause: `TaskRow.Branch` and `TaskBranch.Branch` are plain strings (`internal/proto/task.go`, `internal/proto/command.go`). Giving the field a type of its own changes `internal/host`, which unit P-1 does not own; and whether the home must refuse a `task branch` whose `branch` is not `task/<num>-<slug>` was not in `contracts/commands.md` 1.5.
- Status: resolved on the host in #203 (`HostedChecksToResume` in `internal/host/forge.go` derives `task/<num>-<slug>` from `Num` and `Slug` through `taskName`, skips rows that fail `taskName`, and ignores `TaskRow.Branch`) and on the home in #183 (`tskBranch` in `internal/home/cmd_task.go:76` answers `bad_request` to any `branch` other than `task/<num>-<slug>`, and `planning/contracts/commands.md` 1.5 states the rule).

### 2026-11-20 · deviation · A `set` that changes nothing and a `task assign` without a role were not refused

- Symptom: `contracts/commands.md` gives `group set` and `member set` "any of" their fields, and `task assign` at least one of `assignee` and `reviewer`. A `group set` with no field, a `member set` with nothing but `id`, and a `task assign` with neither role were read by `proto.Decode` and by the web client's schemas without refusal.
- Cause: each of those fields is optional by itself (`GroupSet`, `MemberSet`, `TaskAssign` in `internal/proto/command.go`; the schemas of the same names in `web/src/lib/proto.ts`), and "at least one" is a rule across fields, which the wire types do not state.
- Status: resolved in #179 (`grpSetValid` in `internal/home/cmd_group.go:110` and `memSetValid` in `internal/home/cmd_member.go:144`) and #183 (`tskAssign` in `internal/home/cmd_task.go:94`), which answer `bad_request` as `contracts/commands.md` specifies.

### 2026-11-21 · bug · `Loop.Inbox` marked only the groups that existed when it was called

- Symptom: after `Loop.Inbox(conn)` had made a host connection the inbox taker (`internal/home/run.go:366`), a group created later on that home spawned its `runGroup` with `seat.inbox` false (`internal/home/run_group.go:52`) and did not deliver queued items to `conn` until `Loop.Inbox(conn)` was called again (which `LocalLink` never does, because it sends no `hello`).
- Cause: `Conn` recorded `welcomed` across groups (`internal/home/run_conn.go:34`), whereas `inbox` was kept per group seat (`internal/home/run_group.go:31`) and `spawn` called `seat` with `inbox` false (`internal/home/run.go:162`).
- Status: resolved in #209: `Conn.taker` (`internal/home/run_conn.go:36`) is set under `Loop.mu` in `Loop.Inbox` (`internal/home/run.go:366`), and `runGroup.seat` initialises `runSeat{inbox: c.taker}` (`internal/home/run_group.go:53`), covering both local and remote hosts. Test: `TestLocalLinkInboxForGroupCreatedAfterAttach`.

### 2026-11-20 · limitation · `ReviewedMerge.GitHubRepo` is not filled from the group's repository

- Symptom: the merge with a remote runs `gh pr merge <pr> --repo <owner>/<repo>` (`planning/contracts/local.md` section 7, row "merge, with a remote"), but no code sets `<owner>/<repo>` from the group's repository; an empty `GitHubRepo` would make the merge fail.
- Cause: `ReviewedMerge.GitHubRepo` (`internal/host/merge.go`) is set by its caller, and no unit called the merge before HS-13. The claim that `--repo` makes gh skip every local step (worktree removal, local branch deletion) rests on the gh 2.102.0 source (`pkg/cmd/pr/merge/merge.go`: `CanDeleteLocalBranch = !cmd.Flags().Changed("repo")`), not on a run against GitHub or on older gh versions.
- Status: resolved in HS-13: `githubRepo` (`internal/host/deliver.go`, `buildReviewedMerge`) extracts `<owner>/<repo>` from the group row's `repo` field when `forge` is `github`.

### 2026-11-20 · deviation · The audit record has no `what` for a CLI killed on an idle close

- Symptom: `planning/flows/agent-turn.md` section G has the node write an audit row, and nothing else, when it kills a CLI that did not answer a close within 10 s outside a turn. `planning/contracts/storage.md` section 3 lists eight values of `what` (`delivered`, `refused`, `requested`, `approval`, `takeover`, `handback`, `merge`, `setting`), and none of them says that.
- Cause: the list of `what` was written before that rule. The audit write takes any text as `what` and has a constant for each of the eight only (`internal/host/audit.go:6-15`); nothing kills a CLI yet.
- Status: resolved in #187: `planning/contracts/storage.md` section 3 adds `killed` to `audit.what`, `internal/host/audit.go` defines `AuditKilled`, and `internal/host/run.go` writes the row naming the member and task when an idle close does not finish within 10 s (`TestCloseTimeoutOnAllPathsAuditAndC6SwitchOrdering`).

### 2026-11-21 · bug · A missing clone folder was reported as a failed start of git

- Symptom: on Windows CI, `TestWorkspaceGitFailures` failed at `internal/host/workspace_test.go:552`: the error of a command in a missing clone folder was not `fs.ErrNotExist`. On macOS the same error read `fork/exec /usr/bin/git: no such file or directory`, naming git instead of the folder.
- Cause: the new runner sets `SysProcAttr` (`internal/host/command_unix.go:13`, `internal/host/command_windows.go:16`), and `os.StartProcess` checks the working folder only when `SysProcAttr` is nil (Go `src/os/exec_posix.go:30`). The start then failed in the OS: on Unix with ENOENT from the child's chdir, on Windows with `ERROR_DIRECTORY` ("The directory name is invalid"), which `syscall.Errno.Is` does not map to `fs.ErrNotExist`.
- Status: resolved in branch `fix/host-workspace-review`: `runInput` checks the folder itself before it starts the command (`internal/host/command.go:45`), so the error is `chdir <folder>: …`, as os/exec gives it without `SysProcAttr`, with `fs.ErrNotExist` on every system. Test: `TestCommandInAMissingFolder`.

### 2026-11-20 · bug · Crash recovery of the host's worktrees decided on the folder alone

- Symptom: the independent review of `internal/host` reproduced four states. (a) A SIGKILL of `worktree add -b` during checkout, held there by a slow smudge filter, left a folder with only `.git` and `.gitattributes`, locked `initializing`; `CreateTaskWorkspace` reused it, `git status` there showed every file staged as deleted, and `RemoveTaskWorkspace(TaskCancelled)` failed with "cannot remove a locked working tree". (b) After `add -b` failed past the branch's creation, every retry failed with "a branch named 'task/1-x' already exists". (c) A kill inside `worktree remove` left "missing but already registered worktree" for good, and a half-deleted folder without `.git` was returned as the task's folder. (d) After a killed clone, `EnsureGroupWorkspace` accepted an empty `repo/`, and every later command failed with "not a git repository".
- Cause: `internal/host/workspace.go:41`, `:89`, `:122` and `:157-161` before the fix took an existing folder as a good worktree or clone, and `add -b` assumed the branch did not exist.
- Status: resolved in branch `fix/host-workspace-review`. Under the clone's lock the host prunes, reuses a folder only when `git worktree list` shows it on its branch (or detached, for a review), not `locked initializing`, and recorded for the same AI, and otherwise deletes it and adds it again, with `--force --force` while it is still registered and without `-b` when the branch exists (`internal/host/worktree.go:40-112`, `:129-180`). The clone is made in `.repo-clone` and renamed to `repo`; a `repo` that is not a Git repository is an error that names it (`internal/host/workspace.go:51-72`). Tests: `TestWorkspaceAfterAKilledAdd` (a real SIGKILL through the runner during a slow smudge filter), `TestWorkspaceAfterAnAddThatFailedInCheckout`, `TestWorkspaceAfterAStoppedRemove`, `TestWorkspaceCloneAfterAStoppedClone`, `TestWorkspaceRefusesARepoFolderThatIsNotAClone`. `planning/contracts/local.md` section 7, "A worktree left by a crash".

### 2026-11-20 · bug · Two AIs on one node shared a task's worktree and a review checkout

- Symptom: when the new holder's create ran before the old holder's removal, the new AI got the old AI's uncommitted file, and the removal then force-deleted the new session's folder.
- Cause: `CreateTaskWorkspace` and the removals did not know which AI held the folder (`internal/host/workspace.go:84-97` and `:176-192` before the fix); the host relied on N-4 ordering them.
- Status: resolved in branch `fix/host-workspace-review`. Create and Remove, for tasks and reviews, take the member id of the AI that holds the folder; the host records it after the add, in the worktree's own admin folder (`internal/host/worktree.go:68-85`). A create adds a folder recorded for another AI, or with no record, again; a removal leaves a folder recorded for another AI untouched (`internal/host/worktree.go:114-126`). Tests: `TestWorkspaceTaskHandedToAnotherAIOnTheSameNode`, `TestWorkspaceReviewerChangedOnTheSameNode`, `TestWorkspaceWithoutAHolderRecord`, and the scenario "A task given to another AI of the same node carries over only its commits".

### 2026-11-20 · bug · Host Git commands could wait for ever

- Symptom: workspace commands had no context and no limit, so a fetch that hung held the clone lock until a restart, and ssh could prompt on `/dev/tty`. The host had four runners for its Git and gh commands.
- Cause: `workspaceGit` (`internal/host/workspace.go:47`), `checkGit` (`internal/host/checks.go:59`), `runIn` (`internal/host/forge.go:26`) and `commandText` (`internal/host/merge.go:176`) before the fix; only the last two took a context.
- Status: resolved in branch `fix/host-workspace-review`. One runner, `runIn` (`internal/host/command.go:27`), runs every git and gh command of the host within the caller's context and its own limit of 60 minutes for `git clone` and `git push` and 10 minutes for any other, with `GIT_TERMINAL_PROMPT=0`, `GH_PROMPT_DISABLED=1`, `SSH_ASKPASS_REQUIRE=never` and `GCM_INTERACTIVE=never` added to the node's environment, in a new session on Unix and without a console window on Windows, killing the whole session or process tree when it stops (`command_unix.go`, `command_windows.go`). The other three call it; every exported workspace function takes a context. Tests: `TestCommandLimits`, `TestCommandsNeverWaitForAPerson`, `TestCommandStopsAtItsLimit`. `planning/contracts/local.md` section 7, "Commands never wait for a person".

### 2026-11-20 · bug · A task slug went into paths unchecked

- Symptom: the slug `x/../../../../outside` gave a folder outside the project, with a nil error.
- Cause: `internal/host/workspace.go:87-90` before the fix joined `<num>-<slug>` into paths and the branch name as given.
- Status: resolved in branch `fix/host-workspace-review`: `taskName` refuses a slug that does not match `^[a-z0-9]+(-[a-z0-9]+)*$` or is longer than 40 characters, and a `num` below 1, before any path or branch is built (`internal/host/workspace.go:113`); `reviewSlot` refuses a `num` below 1 (`internal/host/workspace.go:215`). Test: `TestWorkspaceRefusesSlugsTheHomeCannotProduce`. The open entry for P-1 above covers the decoder.

### 2026-11-20 · bug · The review diff followed the owner's diff settings

- Symptom: with `diff.external` set, the review diff was the external program's output; `textconv` and `color.ui=always` changed it too.
- Cause: `internal/host/workspace.go:154` before the fix ran `git diff --no-color` only.
- Status: resolved in branch `fix/host-workspace-review` with `--no-ext-diff --no-textconv --no-color` (`internal/host/workspace.go:155`). Test: `TestWorkspaceReviewDiffIgnoresDiffSettings`.

### 2026-11-20 · bug · A tracked `.solodawn-review.diff` stopped the next review round

- Symptom: when the reviewed commit tracks `.solodawn-review.diff`, the untracked diff of the previous round made `checkout --detach` fail.
- Cause: `internal/host/workspace.go:161` before the fix.
- Status: resolved in branch `fix/host-workspace-review` with `git -C review/<num> checkout --force --detach <commit>` (`internal/host/worktree.go:167`); `planning/contracts/local.md` section 7 marks the name as reserved. Test: `TestWorkspaceReviewOfACommitThatTracksTheDiffName`.

### 2026-11-20 · bug · Two spellings of one clone path took two locks

- Symptom: a clone reached through a symbolic link and through its real path had two locks.
- Cause: `cloneLock` keyed by `filepath.Clean` only; `internal/host/merge.go:36`, `:62` and `:114` lock `m.Repo` and `internal/host/workspace.go:29` locked `filepath.Join(projectDir, "repo")`.
- Status: resolved in branch `fix/host-workspace-review`: `cloneLock`, the one helper both use, keys by the real path of the longest existing prefix (`internal/host/workspace.go:21-33`), so a clone has one key before and after it exists. A relative and an absolute spelling still differ; the node passes absolute paths. Test: `TestCloneLockFollowsSymbolicLinks`.

### 2026-11-20 · deviation · P-1's task row decoder must refuse a slug the home cannot produce

- Symptom: the host refuses a `slug` that does not match `^[a-z0-9]+(-[a-z0-9]+)*$` or is longer than 40 characters, and a `num` below 1 (`internal/host/workspace.go:113`, `taskName`), because both go into paths and branch names; a task row with such a slug was accepted on the wire.
- Cause: the decoder of the task row (P-1, `internal/proto`) checked no form of `slug`; `planning/contracts/local.md` section 7, "Slug", gives the only form the home produces.
- Status: resolved by P-1 on `rebuild`: `proto.Slug` (`internal/proto/slug.go`) and `proto.Decode` refuse such a row (`internal/proto/decode.go:163`). The host keeps its own refusal before it builds a path.

### 2026-11-20 · deviation · An open chat hid the shell's top bar on a phone from its own styles

- Symptom: below 768 px an open chat showed no top bar and no bottom navigation, because `web/src/routes/Groups.svelte` hid them with `.app:has(.groups.open)` in its own styles, and it read the group id from the address itself, because `route` carried only the page.
- Cause: the shell had no place where a page says that it takes the whole screen, and the route table had no parameters.
- Status: resolved in U-0 (#170): the route `chat` (`#/groups/<g>`) asks the shell for the whole screen on a phone (`full: 'phone'` in `web/src/lib/route.svelte.ts`, drawn by `.app[data-full='phone']` in `web/src/styles/base.css`), and `Groups.svelte` reads `params.g`. The title of the top bar from 768 px is the open entry of 2026-11-21.

### 2026-11-20 · deviation · Web mock does not answer `host.info` or push `groups`

- Symptom: F-02 in `planning/steps/step-1-frontend.md` asks for a mock that answers `hello`, `sync`, `fetch` and `host.info`; the settled plan (PR #58) adds that it pushes `groups`. `mockCtl` answers `hello`, `sync`, `fetch` and `cmd`, and pushes only `reset` and `rows`. `proto.ts` has no `host.info` type; it has a `Groups` type, written from the contract.
- Cause: neither frame has a Go type or a wire example yet, and the mock answers from wire examples only (`web/src/lib/mock.ts:63-75`, `web/src/lib/proto.ts:94-96`, `web/src/lib/proto.ts:372-377`). Both are frozen by unit P-2 (`planning/contracts/conventions.md` section 5; `planning/steps/parallel-plan-units.md` section 1), which also adds the mock answers for `host.info` and `groups`; B-10 named them before the parallel plan.
- Status: resolved in #165 (P-2): `host-info-answer.json` and `groups.json` exist with their Go types and schemas; `mockCtl` answers `host.info` and pushes `groups` before it answers a `group create` (`web/src/lib/mock.ts`, `mockCtl`).

### 2026-11-20 · bug · The local merge ran the project's Git hooks (review M5)

- Symptom: once a check's `npm install` had let Husky set `core.hooksPath` in the clone's config, a `prepare-commit-msg` hook that adds a trailer made `Apply` return a conflict ("last commit on base does not record …") although the base held the change, and the next round added a second squash commit; a commitlint-style `commit-msg` hook made every local merge a conflict. A configured signing key could make the commit wait for a person.
- Cause: `git merge --squash`, `git commit` and `git reset --merge` in `squashLocal` (`internal/host/merge.go`) ran with the clone's own hooks and signing settings, and the inspection compared the exact message.
- Status: resolved on branch `fix/host-merge-review`: the three commands, and the reset of an interrupted squash after a restart, run with `-c core.hooksPath=<repo>/.git/solodawn-no-hooks`, an empty folder the host makes; the commit with `--no-verify --no-gpg-sign` (`noHooks`, `squashLocal`, `resetLocal`). Pushes and fetches keep the project's hooks (Git LFS needs `pre-push`). `local.md` section 7, row "merge, no remote". Tests `TestLocalMergeRunsNoProjectHooks`, `TestLocalMergeWithoutHooksFolder`.

### 2026-11-20 · bug · `Submit` could reuse a pull request from a fork (review M6)

- Symptom: an open pull request from a fork whose branch has the task branch's name, into the same base, was taken as the task's pull request.
- Cause: in gh 2.102.0 `gh pr list --head` matches the branch name only; `openPR` (`internal/host/forge.go`) took the first entry.
- Status: resolved on branch `fix/host-merge-review`: `--json` asks for `isCrossRepository` and such entries are skipped; `local.md` section 7, row `submit_task`. Test `TestSubmitForgeSkipsForkPullRequests`, with a fake `gh`; not verified against GitHub.

### 2026-11-20 · bug · A merge queue or auto-merge was reported as a conflict (review M7)

- Symptom: when `gh pr merge` exits 0 and the pull request stays `OPEN` (a merge queue or auto-merge merges it later), `Apply` reported `task conflict` "pull request N is OPEN", and so did `Inspect` after a restart.
- Cause: `Apply` and `Inspect` (`internal/host/merge.go`) read the pull request once.
- Status: resolved on branch `fix/host-merge-review`: the pull request is read with one `gh api graphql` call (`state`, `headRefOid`, `mergeCommit`, `autoMergeRequest`, `mergeQueueEntry`). The merge is pending only while it is `OPEN` with a merge queue entry or auto-merge; `awaitPR` then reads every 30 s until an outcome, or 60 minutes after the merge command (after a restart, after `ReviewedMerge.Taken`, the audit row's time, or the restart), then `task conflict` "merge still pending after 60 minutes (merge queue or auto-merge)". An `OPEN` pull request with neither (the merge never ran, failed, or left the queue) is `task conflict` at once (re-check N2; `TestOpenPRWithNothingPending`). Right after a merge command that succeeded, that holds only from the second read, as GitHub's read can lag its write; at the limit, a last read that failed returns its error, so a merged PR out of reach is not reported as a conflict (focused review D1, D2; `TestReadFailureAtTheLimitAfterARestart`, `TestReadFailureAtTheLimitAfterAnOutage`, `TestFirstReadAfterTheMergeMayLag`). `local.md` section 7, rows "merge, with a remote" and "node restarts with a merge". Tests `TestMergeWaitsWhilePending`, `TestPendingMergeEnds`, `TestInspectResumesPendingMerge`, `TestPendingMergeStops`, with a fake `gh`; not verified against GitHub. A read that fails during the wait counts as unfinished and is repeated 30 s later, as for hosted checks (`TestPendingMergeOutlastsFailedReads`).

### 2026-11-20 · bug · `HostedChecksToResume` panicked on a row without `checks.local` (review M8)

- Symptom: a task row at `cloud_check` with no `checks.local` made the selection dereference a nil pointer.
- Cause: `time.UnixMilli(t.Checks.Local.At)` in `HostedChecksToResume` (`internal/host/forge.go`).
- Status: resolved on branch `fix/host-merge-review`: such a task resumes with its 60 minutes spent (`Since` is the zero time), so it ends with the timed-out result unless its first read has finished. Tests `TestHostedChecksToResumePausedAndWithoutLocalCheck`, `TestHostedChecksWithoutLocalCheckTimeOut`.

### 2026-11-20 · bug · Restart inspection without a remote looked at the last commit only (review L6)

- Symptom: after merging task t1 and then t2, `Inspect` of t1 reported a conflict.
- Cause: `inspectLocal` (`internal/host/merge.go`) compared the message of `git log -1` on the base only.
- Status: resolved on branch `fix/host-merge-review`: merged means a commit on the first-parent history of the base whose message is exactly the merge message; `local.md` section 7, row "node restarts with a merge". Test `TestInspectFindsAnEarlierLocalMerge`.

### 2026-11-20 · deviation · Paused tasks were not resumed at `cloud_check` (review L8)

- Symptom: after a restart, a paused task at `cloud_check` was not polled again, though task-machine row 6 takes `task cloud` while paused and `local.md` section 7 says every task at that step.
- Cause: `HostedChecksToResume` (`internal/host/forge.go`) kept `active` tasks only, and its test asserted the exclusion.
- Status: resolved on branch `fix/host-merge-review`: tasks at `cloud_check` that are active or paused resume, the two states row 6 accepts; the test now expects the paused task.

### 2026-11-20 · bug · `DeleteBranch` could drop unreviewed commits (review L9)

- Symptom: a local task branch that had moved past the merged commit was deleted with `git branch -D`, and its later commits with it.
- Cause: `DeleteBranch` (`internal/host/merge.go`) deleted the branch whatever it pointed at.
- Status: resolved on branch `fix/host-merge-review`: the branch is deleted with `git branch -D` only while `rev-parse` shows it is the merged commit; a moved branch is kept and `DeleteBranch` returns `deleted` false. `branch -D` refuses a branch that a worktree still has checked out, the only place it could move from; an earlier version used `update-ref -d`, which deleted it under a worktree whose removal had failed (re-check N1). `local.md` section 7, row "task merged". Tests `TestDeleteBranchOnlyAtTheMergedCommit`, `TestDeleteBranchRefusedWhileCheckedOut`, `TestDeleteBranchFailures`.

### 2026-11-20 · limitation · The task machine held `review.failed` and `wait.kept` beside the task row

- Symptom: on the branch of HM-M, `home.Step` took and returned `home.Task`, a `proto.TaskRow` with two more fields, `ReviewFailed` and `Kept`. A caller that kept only the `proto.TaskRow` would have lost the count of failed reviews and the wait a pause replaced.
- Cause: `contracts/rows.md` section 4 has `review.failed` and `wait.kept`; `proto.TaskRow` did not have them until P-1 froze them, and HM-M was built in the same wave.
- Status: resolved in PR #161, before it was merged: after P-1 (PR #162) `home.Step` takes and returns `proto.TaskRow`, and the two fields are `review.failed` and `wait.kept` of the row.

### 2026-11-20 · limitation · `group create` was sent as the contract describes it, before its wire example

- Symptom: the create-group dialog of the web client was written before P-1 froze the payload of `group`, with `g` equal to `n` as `planning/contracts/commands.md` 1.1 reads.
- Cause: the order of the units; `Home.create` in `web/src/lib/groups.svelte.ts`.
- Status: resolved in #164. P-1 (#162) froze `group-create.json` with `g` equal to `n`; `create` sends the fields of that example in its order, and its test reads the payload with the `GroupCreate` schema. The dialog still asks for the repository address with `forge: none`, since `repo` is absent only for a group without a repository (`planning/contracts/rows.md` section 1).

### 2026-11-20 · deviation · No unit measured a row against its size limit

- Symptom: `planning/contracts/conventions.md` section 4 has the home answer `bad_request`, naming the limit and the size, to a command that would write a row above 16 KiB, or a card above 12 KiB when it is created. No row of `planning/steps/parallel-plan-units.md` names the rule, and the first version of the seam handed the rows of a command's function to its caller unmeasured.
- Status: resolved in #160 by the coordinator's decision of 2026-11-20: `Table.Apply` measures every row of a result after numbering it and refuses the command for one above its limit (`State.Seal`, `internal/home/home.go:344`; `rowFits` at `:143`).

### 2026-11-20 · bug · The tests check counted deleted files

- Symptom: changing `a.go` and deleting `a_test.go` passed; only deleting `old.go` failed; renaming `x_test.go` to `y.go` passed. `local.md` §7 says a deleted file counts as neither a test file nor a source file. Found in the independent review of `internal/host` (M1).
- Cause: the names diff of the hard checks listed deleted paths (`internal/host/checks.go:163` before the fix); with `--no-renames` a rename is a deletion and an addition.
- Status: resolved on branch `fix/host-checks-review`: `--diff-filter=d` on the names diff. Test `TestTestsCheckIgnoresDeletedFiles`.

### 2026-11-20 · bug · The secrets check did not read files Git takes as binary

- Symptom: a `server.pem` holding a PKCS #8 key passed the secrets hard check when `.gitattributes` marked `*.pem binary` or the file held a NUL byte; Git printed "Binary files differ" instead of the added lines (M2).
- Cause: the patch diff ran without `--text` (`internal/host/checks.go:159` before the fix).
- Status: resolved on branch `fix/host-checks-review`: `--text` on the patch diff. A large binary then put tens of MiB through the scan: the check now matches in place with a literal prefilter and no copies, fails above 64 MiB of added lines with "too large to check", and the hard checks stop at their own 2-minute limit with "timed out". Tests `TestSecretsCheckReadsFilesGitTakesAsBinary`, `TestAddedLinesOverTheLimit`, `TestHardChecksTimeOutOnTheirOwnLimit`.

### 2026-11-20 · bug · A check's output tail could hold the end of a private key

- Symptom: a check that printed a 4.4 KiB key left a tail that began inside the key and ended with its END line, which redaction did not match (L1).
- Cause: the output was cut to 4 KiB before redaction, and redaction needed the header (`internal/host/checks.go:118-125` before the fix).
- Status: resolved on branch `fix/host-checks-review`: the last 64 KiB of output are redacted, then cut to the 4 KiB tail (`checkDetail`); redaction, and not the check, also replaces a private-key footer without its header from the start of the text. Tests `TestCheckOutputRedactsKeyLongerThanTail`, `TestCheckDetailIsErrorThenRedactedTail`, `TestRedactSecrets`.

### 2026-11-20 · bug · A turn that ran out took the hard checks with it

- Symptom: with a context of 300 ms and the check `sleep 2`, `Run` returned "context deadline exceeded" and no items, so neither hard check ran (L3; R-103 says both always run).
- Cause: the hard checks used the caller's context after the group's checks had used up its deadline (`internal/host/checks.go:38` before the fix).
- Status: resolved on branch `fix/host-checks-review`: the turn's deadline does not stop the hard checks, which have their own limit; an explicit cancel (pause, interrupt, shutdown) stops the run, which then reports nothing and returns the error. The group's checks share the turn's deadline, with `turn_minutes` bounding a context without one. Tests `TestTurnDeadlineKeepsHardChecks`, `TestCancelStopsRun`.

### 2026-11-20 · bug · A background child of a check outlived it and hid the error

- Symptom: `sleep 3 & echo done` gave `ok=false` with the detail "done\n": the `WaitDelay` error was dropped, and `sleep` kept running (L4).
- Cause: the detail was replaced by the output tail, and nothing killed the process group after `Wait` (`internal/host/checks.go:115-125` before the fix).
- Status: resolved on branch `fix/host-checks-review`: after `Wait` the process group is killed (Unix, `checks_tree_unix.go`); on Windows the shell runs in a job object closed after `Wait` (`checks_tree_windows.go`); without a job nothing is killed after `Wait`, as Go has released the process. The detail is the error, then the redacted tail. Test `TestBackgroundChildDiesWithCheck`.

### 2026-11-20 · bug · A clean status did not prove the checked commit

- Symptom: a tracked file with the skip-worktree or assume-unchanged bit could differ from the commit while `git status --porcelain` stayed empty, so `run_checks` checked content that was not the commit (L5).
- Cause: the clean-tree test read the status only (`internal/host/checks.go:70-80` before the fix).
- Status: resolved on branch `fix/host-checks-review`: a skip-worktree path (`S` or `s` in `git ls-files -v`) is refused with its own error naming up to 10 paths; the assume-unchanged bit, a speed hint that `core.ignoreStat` sets on every path, is cleared before the status, so a change shows and a clean tree runs. `local.md` §7 `run_checks` says so. Tests `TestSkipWorktreeIsRefusedByName`, `TestAssumeUnchangedDoesNotHideChanges`.

### 2026-11-20 · deviation · Redaction was not available to the node

- Symptom: the node must apply the one pattern list to the details of `task fail`, `login_invalid` and `turn_interrupted` (`local.md` §7), but the helper was unexported (L10).
- Cause: `redactSecrets` in `internal/host/secret.go:53` before the fix.
- Status: resolved on branch `fix/host-checks-review`: exported as `RedactSecrets`. Test `TestRedactSecretsFromOutsideHost`.

### 2026-11-20 · bug · OpenPGP private keys passed the secret check

- Symptom: `-----BEGIN PGP PRIVATE KEY BLOCK-----` matched no pattern (L11).
- Cause: the PEM pattern required `PRIVATE KEY-----` (`internal/host/secret.go:19` before the fix).
- Status: resolved on branch `fix/host-checks-review`: the pattern takes an OpenPGP block through its END line; `local.md` §8 reads "a PEM or OpenPGP private-key header". Tests in `TestRedactSecrets` and `TestCheckOutboundSecretRejects`.

### 2026-11-20 · bug · The signalling client could set off the rendezvous's own limits

- Symptom: a node's rendezvous connection could be closed by the server for two things the node did itself. A backlog of signalling left in bursts of 90 frames exactly 10 seconds apart, so ordinary latency between two bursts could bring 121 frames within the server's 10 seconds. A `to` frame of 16,383 or 16,384 bytes passed the node's own check, but the `from` the server forwards is 2 bytes longer than the 16 KiB limit, and the server closes the sender.
- Cause: the pacer allowed 90 frames in any 10 seconds (`internal/peer/path.go`, `pacer.turn`), with no margin in time against the server's count (`internal/rendezvous/server.go`, `client.admit`). `Path.Write` held every frame to 16,384 bytes, the `to` frames of `Path.send` included, while the server refuses a `to` whose `from` would exceed that (`internal/rendezvous/server.go`, `client.sendFrom`).
- Status: resolved on branch `fix/peer-pacer-margin`. The pacer allows 90 frames in any 11 seconds, and a `to` frame is refused locally over 16,382 bytes; `planning/contracts/rendezvous.md` section 2 states both. The browser's signalling client gets the same rules in #155.

### 2026-11-20 · bug · The independent review of `web/src/lib` found ten faults in signalling, the tab lock, the device log and the wire types, and its re-check five more

- Symptom: none showed in use, as no module has a caller outside tests yet (W-2, U-15). F1: a path sent every frame at once and any size; 150 `send` calls in one tick reached the socket, and the server closes a connection at its 121st frame in 10 seconds or at a frame over 16 KiB. F2: one URL the browser refuses (`ws://` from an HTTPS page, a malformed URL) made `rendezvous()` throw and left the sockets already opened. F3: `DeviceLog.apply` called for entries 1, 2, 3 at once refused a later one as `gap` when its signature was checked first. F4: a path closed before `ready` kept every frame sent to it, and `receive` was still called after `close()`. F5: a frame of a path could reach `receive` before a `from` that arrived ahead of it. F6: an `AbortError` of the session itself was read as a stolen lock. F7: `usage` and `checks` took `[]`, which Go refuses. F8: a `from` whose `conn` is no random id was delivered; Go drops it. F9: a `ready` with `ttl` below 1 was taken, and ICE entries pion and the browser refuse were passed on. F10: `{"re":1,"ok":true,"err":{…}}` was read as a success. The re-check of the first fix found: N1, one exception in `receive` or in `known` silenced a path for good; N2, a backlog left in bursts exactly at the server's 10-second boundary, so jitter closes the path; N3, a `to` of 16,383 or 16,384 bytes passed, whose `from` the server refuses; N4, the pace followed the wall clock, and setting it back an hour held a frame for 61 minutes; N5, TURN entries with an empty username or credential and ports outside 1 to 65535 were kept, on which Chromium throws.
- Cause: `web/src/lib/rendezvous.ts:33`, `:35`, `:45-52`, `:63-72`, `:88-93`, `web/src/lib/devlog.ts:50-51`, `web/src/lib/tablock.ts:13-17`, `web/src/lib/proto.ts:27`, `:77-78`, `:235-242`, `:275`, `:569-582` before the fix.
- Status: resolved in branch `fix/web-lib-review`. A path keeps 90 frames in any 11 seconds, a second inside the server's 10, by `performance.now()`, and queues the rest in order behind one timer; a frame over 16,384 UTF-8 bytes, or a `to` over 16,382 (its `from` is 2 bytes longer), throws `RangeError` before any path sends it (`web/src/lib/rendezvous.ts:26-34`, `:107-145`, `:219-224`); `internal/peer/path.go:53-78`, `:135-148` pace at 10 seconds and is fixed apart. A refused URL is a path that sends nothing and reports `closed` after the call (`:93-105`). Each path delivers through one promise chain, its `closed` last, and nothing after `close()` (`:77-91`, `:142-145`, `:182-198`); a check or a `receive` that throws is passed to `reportError`, its frame counts as dropped, and the frames after it still come (`:63`, `:83-91`). `DeviceLog.apply` checks entries one after another in the order of the calls (`web/src/lib/devlog.ts:46-57`). Only a rejection of `request()` is a steal; a session's error is thrown after `request()` resolves (`web/src/lib/tablock.ts:3-31`). `usage`, `checks` and empty bodies refuse an array through `record` (`web/src/lib/proto.ts:17-25`); `conn` is an `Id` (`:36`) and the browser drops a `from` whose `conn` is not (`web/src/lib/rendezvous.ts:175-180`); a success frame has no `err` (`web/src/lib/proto.ts:139-140`). A `ready` with `ttl` below 1 closes its path, such an `ice` answer is dropped, and only usable ICE entries go on (`web/src/lib/rendezvous.ts:37-61`): the entries Go drops (`internal/peer/path.go:245-270`), and also an empty URL list, TURN without a username or credential or with an empty one, and a port outside 1 to 65535, on which a browser's `RTCPeerConnection` throws (WebRTC, "validate an ICE server"; Chromium for the empty values and the port). Upper-case schemes and ports with leading zeros are dropped, which pion takes. Tests: `web/src/lib/rendezvous.test.ts`, `devlog.test.ts`, `tablock.test.ts`, `proto.test.ts`, `web/features/rendezvous.spec.ts`.

### 2026-11-20 · limitation · The own-address rule knows the interface addresses of the start only

- Symptom: the relay sends a datagram to one of the server's own addresses only when it is for one of its relay allocations (`planning/contracts/rendezvous.md` section 3). The server reads its interface addresses once, when it starts; an address that an interface gets later (a new DHCP lease, an added IPv6 address) is not one of its own addresses for this rule until the service restarts, so a client can relay to a service on the server through that address.
- Cause: `ownAddrs` read `net.InterfaceAddrs` once, from `relay.generator` at startup (`internal/rendezvous/relay.go:111-129` before the fix), with the addresses its name resolves to.
- Status: resolved in PR #151: `generator.ownAddr` reads them again once they are a minute old and keeps the previous set if the read fails (`internal/rendezvous/relay.go:133-160`); a new address counts as the server's own within a minute.

### 2026-11-20 · deviation · The plan had host files register their subcommands and channels

- Symptom: `planning/steps/parallel-plan-units.md` had HS-19 register the terminal channel in the table of PR-0 from `host/pty.go`, and every host unit register its subcommands and channels in the tables of N-0 and PR-0 from its own file. `host` may import neither `peer` nor `node` (`planning/architecture.md` 8.2, `imports_test.go`).
- Status: resolved in #135 by the coordinator's decision of 2026-11-20: `node` registers the subcommands of host features in the table of N-0 and gives each `Peer` the handlers of their channels when it creates it (`peer.New`, `peer.Channels`); a handler gets a `peer.Stream`. The entries of HS-19, N-4, PR-0 and PR-1 and the note of lane HOST say so.

### 2026-11-20 · deviation · The probe needs `peer/signal.go`, which PR-1 did not own

- Symptom: PR-1 owned `peer/probe.go` only. PR-0 hands a probe offer to its handler, but a probe also needs an answer without TURN (`create` takes its ICE servers from `paths`), an ordinary offer that closes a pending probe (`ordinaryOffer`), and a probe that does not take the device's slot before its first `hello` (`answer`), all in `internal/peer/signal.go`.
- Status: resolved in #135 by the coordinator's decision of 2026-11-20: PR-1 also owns those three changes in `peer/signal.go`.

### 2026-11-20 · bug · Go acceptance steps acted beyond their own scenario

Found in the audit of the harness; each was reproduced before the fix (lines are those before it).

- A failing step could end the whole suite: later scenarios and their After hooks did not run, godog printed no summary, go test said "subtest may have called FailNow on a parent test". Steps called `Fatal` on the suite's own test from a scenario's subtest (`features/node_update_test.go:98`, `:104`, `features/host_forge_test.go:26`, and `features/host_checks_test.go:36`, fixed on branch `ci/go127-windows-macos`).
- `go test -tags acceptance -count=2 ./features/` failed with `open …/TestFeatures…/017/v0.3.1: no such file or directory`: the cache of release builds was a package variable that outlived the TempDir of the first run (`features/node_update_test.go:89`).
- The fake node's socket stayed listening through every later scenario, and each node-update scenario left an HTTP server listening (five by the end): cleanups were registered with `f.t.Cleanup` (`features/cmd_local_test.go:38`, `:44`, `features/node_update_test.go:54`).
- Under `go test -run` naming the two login-service scenarios, "Register" failed and "Remove" passed without doing anything: HOME was set in a subtest of the suite's test that `-run` skipped (`features/node_service_test.go:30`).
- A scenario that changed a variable other than HOME, or the working directory, changed every later scenario: the guard of PR #121 checked HOME only and did not restore it (`features/acceptance_test.go:67-80`).
- With `commit.gpgsign = true` in the global Git configuration, a host-check scenario failed with "gpg failed to sign the data": git ran with the user's and the system's configuration.
- A step error X was printed as "after scenario hook failed: X, step error: X, X, step error: X": After hooks returned the step's error as their own (`features/identity_test.go:72`, `features/rendezvous_test.go:101`, `features/peer_test.go:61`).
- Status: resolved in PR #142: steps return errors (the comment on `fixtures.t` states the rule); `afterScenario` undoes per scenario; the build cache is `fixtures.stamped`; the login steps set and restore the variables around each operation; after each scenario the suite compares every variable and the working directory with their state at its start, names what changed (never values) and restores it; `TestFeatures` sets `GIT_CONFIG_GLOBAL` to the null device and `GIT_CONFIG_NOSYSTEM`; the hooks return nil.

### 2026-11-20 · bug · The web acceptance file failed on a busy machine although its tests passed

- Symptom: with every core busy, `features/web.spec.ts` failed 3 of 3 runs with "HookTimeoutError: Hook timed out in 10000ms" at `web/features/motion.ts:22`; all 213 tests passed.
- Cause: `browser.close()` took 10.0 s under that load (20 to 60 ms idle), Vitest's default hook limit.
- Status: resolved in PR #142: both browser hooks have 60 s; 3 of 3 loaded runs passed.

### 2026-11-20 · bug · The node could not start on Windows

- Symptom: on the first Windows runs of CI (Windows Server 2025, and 2022 in a trial), every `ListenLocal` failed, so `solodawn node` could not start: 13 tests of `internal/node` and `cmd/solodawn` failed, first with "remove …\sock: The system cannot find the file specified", then with "Access is denied".
- Cause: Windows refuses a dial to a path where no file is, as it refuses one to a stale socket, so `clearStale` removed a file that was not there (`internal/node/socket.go:45-46` before the fix). Then `ownerOnly` opened the socket with `WRITE_DAC` alone, and `SetSecurityInfo` answers "Access is denied" to such a handle on both versions (`internal/node/socket_windows.go:51` before the fix; found with a diagnostic test in CI).
- Also: five tests encoded Unix answers (the error of a dial to a missing socket, the text of exec on a non-executable, address in use, a binary's mode, the `.old` image the Windows install keeps), and the two DACL tests expected `D:P(…)` with the full SID where Windows writes `D:PAI(…)` and the alias `LA` of the runner's built-in Administrator.
- Status: resolved on branch `ci/go127-windows-macos`: a refused dial removes the file only if one is there, the handle also has `READ_CONTROL`, and the tests take the system's own answers.

### 2026-11-20 · deviation · CI gaps found in the audit of the workflows

- Actions were referenced by movable tags (`pnpm/action-setup` is third party); `release.yml` restored pnpm and Go caches into the build of released files; `sudo ./netsim.test` had no time limit; CI never vetted code behind the `acceptance` tag nor compiled the macOS and Windows tests. Resolved in PR #142: pins to the full commit, no cache in the release job, `-test.timeout=10m`, `go vet` with the tag and for every cross-compile target.
- Every job built with Go 1.26.0, eleven patch releases behind 1.26.11 (Backend run 44697108086), because setup-go v5.6.0 reads `go 1.26.0` in `go.mod:3` as an exact version. Resolved on branch `ci/go127-windows-macos` by the coordinator's decision of 2026-11-20: every setup-go takes `go-version: '1.27.x'` with `check-latest: true`, the newest patch of the line development uses (1.27.4); `go.mod` keeps `go 1.26.0` as the minimum.
- No job had a time limit, so a hung step could hold a runner for six hours (a `pnpm install` held a macOS runner for 30 minutes on 2026-11-20); and no Go code ran on Windows or macOS. Resolved on branch `ci/go127-windows-macos`: `timeout-minutes` on every job, and a Backend job on `windows-latest` and `macos-latest` (vet, `go test -race`, the acceptance suite, coverage of the files built only there).

### 2026-11-20 · deviation · Two host check tests skipped themselves on Windows

- Symptom: `TestChecksRequireBothGitDiffReads` and `TestCheckCancellationStopsForegroundPipeline` in `internal/host/checks_test.go` called `t.Skip` on Windows. `t.Skip(` is a listed directive under `[tamper]` in `.project-standard/gates.toml` with no exception, yet both merged with HS-5 (#94).
- Cause: both tests need a POSIX shell (a fake `git` written as a shell script; a pipeline with a FIFO), and the author guarded them with a runtime skip. Neither CI nor the reviews of #94 ran the directive check.
- Status: resolved. Both tests and their FIFO helper moved to `internal/host/checks_unix_test.go` under `//go:build !windows`, so they are not compiled where they cannot run; no listed directive remains in `cmd/`, `internal/` or `features/`.

### 2026-11-20 · bug · Wire types read seven things differently from the contracts

- Symptom: an audit of the merged wire layer found (1) `proto.ts` typed some 50 integer fields (`seq`, `re`, `id`, every time and count outside a message row) as any number, so the browser read `"seq":805.5` where `proto.Decode` refuses it; (2) `decode(Frame, …)` read a `welcome`, `synced` or `fetched` answer that lacks a field as a `cmd` answer `{re, ok}` instead of refusing it; (3) neither language had the optional `full` of an `approval` body (`contracts/messages.md` 2.3, added in PR #71 after the B-01d freeze), so it was dropped; (4) Go's `MessageRow[ApprovalBody, …]` took `"state":null` for a card, and `MessageRow[EmptyBody, StatusOnly]` took a state for a text message, both of which `proto.ts` refuses (`contracts/rows.md` section 5); (5) `testdata/welcome.json` gave the node version as `0.3.1`, where `contracts/local.md` section 9 and the `welcome` example (PR #112) make it the release tag, `v0.3.1`; (6) Go's `Success` took `"ok":false`, which `proto.ts` refuses (`contracts/conventions.md` section 3); (7) the web read a number beyond the largest double in a fraction field (`"window_pct":1e400`) as `Infinity`, which `proto.Decode` refuses.
- Cause: (1) `num` in place of `int` (`web/src/lib/proto.ts`); (2) the `Frame` union tried `CmdResult`, which has no `t` and drops unknown fields, after the typed answers; (3) the structs were frozen before the field existed; (4) `MessageRow` held its state as `*S` for every kind; (5) the example predates PR #112; (6) `Success.OK` was a plain `bool`; (7) `v.number()` accepts `Infinity`.
- Status: resolved in branch `fix/audit-proto`: integers are `int` except the four `usage` fractions (`web/src/lib/proto.ts:10-12`); the `cmd` answer in `Frame` has no `t` (`web/src/lib/proto.ts:562`); `full` is in both types and the approval examples (`internal/proto/messages.go:41`, `web/src/lib/proto.ts:317`); a card's state is a struct and a non-card's is `NoCard`, which is `null` only (`internal/proto/rows.go:98-115`, `internal/proto/messages.go:198-207`); the `welcome` example has `v0.3.1`; `Success.OK` has the type `True`, which takes `true` only (`internal/proto/session.go`); every number in `proto.ts` is finite.

### 2026-11-20 · bug · The language button was not named by its visible text

- Symptom: the top-bar language button shows "EN" (or "中") but was named "Switch language / 切换语言" by its `aria-label`, so a speech-input user saying "click EN" reached nothing (WCAG 2.2 SC 2.5.3), and the half of the name in the other language had no language of its own (SC 3.1.2). Found in the audit of the web shell.
- Cause: `aria-label={t('bar.lang')}` replaced the button's content as its name (`web/src/App.svelte:49` before the fix).
- Status: resolved on branch `fix/audit-web-shell-2`: no `aria-label`; the name is the visible text, marked with its language, then a visually hidden "Switch language / 切换语言" whose halves carry `en` and `zh-CN` (`web/src/App.svelte`, `textIn` and `otherLang` in `web/src/lib/prefs.svelte.ts`). The look is unchanged. Scenario "The language button is named by its visible text, each part in its own language".

### 2026-11-20 · deviation · The browser bar kept the system colour after a theme was chosen

- Symptom: with a light system and the dark theme chosen, the browser's bar (Android Chrome, Safari) stayed white: `<meta name="theme-color">` in `web/index.html` follows the system scheme only.
- Status: resolved on branch `fix/audit-web-shell-2`: an effect in `web/src/lib/prefs.svelte.ts` gives both tags the colour `index.html` gives the theme shown. The manifest colours stay fixed. Unit tests "the colour of the browser bar".

### 2026-11-20 · bug · Under forced colours a choice was told by the weight of its text alone

- Symptom: in a Windows contrast theme (forced colours), the sliding mark of the navigation and the thumb of each segmented control disappear, because the system drops backgrounds and shadows; the current page and the chosen theme and language were shown only in bold.
- Status: resolved on branch `fix/audit-web-shell-2`: `web/src/styles/base.css` section 8 draws them in `Highlight` and `HighlightText` with `forced-color-adjust: none` (otherwise the system's text backplate hides the text) and hides the hover layer over the current entry. Scenario "Under forced colours the chosen options and the current page are marked in system colours".

### 2026-11-20 · deviation · Defaults were stored on the first visit

- Symptom: the first load wrote the default theme and the language guessed from the browser to `localStorage`, so a person who never chose a language kept it after changing the browser's language.
- Cause: one effect wrote `<html>` and storage on every run, the first included (`web/src/lib/prefs.svelte.ts:40-47` before the fix).
- Status: resolved on branch `fix/audit-web-shell-2`: storage is written only when the person changes a setting (`set` in `web/src/lib/prefs.svelte.ts`). Unit test "writes nothing to this browser until the person changes a setting".

### 2026-11-20 · bug · Web device log refused the entry it held when `p` carried a lone surrogate

- Symptom: a browser holding a device-log entry whose `p` has U+FFFD refused with `fork` the same entry arriving with the escape `\ud800` in that place, and the reverse; a Go node, for the same bytes, does not refuse it (`contracts/envelope.md` section 4: the entry held at a version, the same `p`, `k` and `s`, is not refused). Both spellings are one signed text: the signature covers `p` as `TextEncoder` encodes it, a lone surrogate as U+FFFD, and `encoding/json` reads the escape as U+FFFD. A node passes entries on with U+FFFD, so a browser that wrote the escape and later sends its log again after a `welcome` has its own entry refused by a browser that got it through a node. Found in the audit of the identity code.
- Cause: `same` compared `p` as JavaScript strings (`web/src/lib/devlog.ts:111` before the fix).
- Status: resolved in PR #115: `same` compares `p.toWellFormed()` (`web/src/lib/devlog.ts:110-113`). `crypto.json` holds the case in `devlogs` (its entries are JSON text, so that `p` can carry the escape), and both vector runners pass it; `web/src/lib/devlog.test.ts` and `internal/identity/devlog_test.go` (`TestLogTakesTheSameEntryWithALoneSurrogateAgain`) test it too.

### 2026-11-20 · deviation · A device-log entry of more than 2 KiB is refused before the contract says so

- Symptom: `Log.Apply` and `DeviceLog.apply` refuse an entry whose `p` is more than 2,048 bytes as UTF-8. `planning/contracts/envelope.md` on `rebuild` has no such rule yet.
- Cause: the rule was settled by the coordinator (`planning/risks.md` Q-23) and is written in planning pull request #71, which is not merged: `contracts/envelope.md` section 4, rule 2, and the two cases of `contracts/vectors.md`. The code follows that text (`internal/identity/devlog.go:27-29`, `:89`; `web/src/lib/devlog.ts:118-128`), and `crypto.json` holds the cases.
- Status: resolved: PR #71 (4ac3e10f5) merged the rule into `planning/contracts/envelope.md` section 4, rule 2, so the code now follows the contract.

### 2026-11-20 · bug · Language options were spoken in the language of the page

- Symptom: on the settings page of the English interface, the option "简体中文" had no language of its own, so a screen reader spoke it with the English voice (eSpeak in NVDA reads no Chinese); in the Chinese interface "English" was given the Chinese voice. WCAG 2.2 SC 3.1.2 (Language of Parts, AA) asks that each passage's language be programmatically determined. Found in the audit of the web shell.
- Cause: the radio labels inherited `lang` from `<html>`, which follows the interface language (`web/src/routes/Settings.svelte:39-40` before the fix).
- Status: resolved on branch `fix/audit-web-shell`: each option's `<label>` carries the `lang` of the language it names (`web/src/routes/Settings.svelte:41`); the scenario "Each language option is marked with the language it is written in" of `features/web.feature` covers both interfaces.

### 2026-11-20 · deviation · Identity code was behind the contract settled in PR #69

- Symptom: after PR #69 the contract and both implementations differed: the pairing `mac` covered the role and the device id only, so the rendezvous service could change `kind` and `name` of a `pair` message; a `pair` message of any `kind` was accepted; `n` and `g` of a signed payload took any text, an empty one and the escape of a lone surrogate included, which a node and a browser read as different nonces; `crypto.json` had no `pair_messages` and no `kind` and `name` in `pairing`.
- Cause: the rules were settled after B-02b and F-01b merged (`internal/identity/pairing.go:60-64`, `internal/identity/envelope.go:45-51`, `web/src/lib/pairing.ts:46-56`, `web/src/lib/payload.ts:32-38`, all before the change).
- Status: resolved in PR #72 (B-02c with F-01d): the `mac` covers `role`, `k`, `kind` and `name` as `TextEncoder` encodes them (`internal/identity/pairing.go:57-75`, `web/src/lib/pairing.ts:46-72`); a random id is a type that `proto.Decode` and `proto.ts` check (`internal/proto/decode.go:152-157`, `web/src/lib/proto.ts:17-22`); `crypto.json` holds every case `contracts/vectors.md` names and both test suites pass on it.

### 2026-11-20 · deviation · Go identity and the web client answer differently until B-02b

- Symptom: for the same bytes, `internal/identity` and `web/src/lib/identity.ts` can give different answers. The web client follows the contract as settled in PR #58 and the coordinator's rules of 2026-11-20; the Go package follows the contract as it was. Known differences: weak keys as `k`, `dev`, `user`, pair `k` or signature R (web refuses, Go accepts); an empty `to` (web: a value, so `not_for_me`; Go: no `to`); signalling without `to` (web: `not_for_me`; Go accepts); a payload without `t`, `n` or `ts`, or with `null` in a field read (web: `bad_request`; Go: zero values); a key in another case, such as `TO` (web ignores it; Go reads it); an integer written `2.0` (web: 2; Go refuses) and one above 2^53 (web refuses; Go reads it); an `add` without `kind` `node` or `browser` or without a string `name` (web refuses; Go accepts); a `revoke` of the last valid device (web refuses; Go accepts); a long s in a pairing secret (web refuses; Go reads it as S); a pair message whose `k` is not a device id (web ignores it; Go checks only the mac); a signature whose S is not below L (both refuse, the web client before WebCrypto).
- Cause: the rules changed after B-02 merged; the Go package keeps the old behaviour (`internal/identity/key.go:87-91` signature check, `internal/identity/envelope.go:51-62` payload fields, `internal/identity/devlog.go:63-78` and `:93-104` log rules, `internal/identity/pairing.go:36-37` and `:66-68` secret and pair message). The web side: `web/src/lib/payload.ts`, `web/src/lib/identity.ts:126-136`, `:213-219`, `web/src/lib/devlog.ts:98-101`, `web/src/lib/pairing.ts:22-29`.
- Status: resolved in PR #67: `internal/identity` follows the settled contract, `internal/proto/testdata/crypto.json` holds the matching vectors, and `web/src/lib/vectors.test.ts` passes on them. The line references above are those before the change.

### 2026-11-20 · limitation · An empty `to` in a signed payload is read as no `to`

- Symptom: an envelope whose payload carries `"to":""` passes the `not_for_me` check on every receiver, although `contracts/envelope.md` section 3 says a `to` that is present must equal the receiver's device id.
- Cause: `proto.PayloadHeader.To` is a plain string (`internal/proto/envelope.go:17`), so `addressed` cannot tell an empty `to` from a missing one (`internal/identity/envelope.go:56`). Only a device the receiver already listens to can sign such a payload.
- Status: resolved in PR #67, by the rule the contract settled on (`contracts/envelope.md` section 3, check 4): an empty `to` is a value like any other and is nobody's device id, so it is `not_for_me` for signalling and commands alike, and `null` is `bad_request`. The payload is read into a pointer field, which tells an empty `to` from a missing one (`internal/identity/envelope.go:49`, `addressed` at `internal/identity/envelope.go:82`).

### 2026-11-20 · limitation · Go and browser JSON parsers can disagree on a signed payload

- Symptom: not observed. A log entry written by hand by one of the person's own valid devices could be accepted by a node and refused by a browser, or the reverse, which leaves their logs at different versions. Examples: `"ver":2.0` (refused by Go, read as 2 by `JSON.parse`), a key spelled `"VER"` (matched by Go, ignored by a browser). The common fields that `OpenSignal` and `OpenCommand` read disagree the same way: for `{"to":"A","TO":"B"}` Go reads `to` as B and `JSON.parse` as A, and Go also matches Unicode case folds, so `"tſ"` (long s) sets `ts`. Go and `JSON.parse` both keep the last of two equal keys.
- Cause: `entry` reads the payload with `encoding/json` into `proto.DeviceEntry` (`internal/identity/devlog.go:68`), and `addressed` into `proto.PayloadHeader` (`internal/identity/envelope.go:53`); `encoding/json` matches keys without regard to case and refuses a fraction for an integer, and the contract has no canonical JSON. The shared vectors (`internal/proto/testdata/crypto.json`) hold no such payload.
- Status: resolved in PR #67. `internal/identity` reads every payload with `proto.Decode` (`internal/proto/decode.go:40`), as every later task that reads the wire does; it reads as `contracts/conventions.md` section 2 says: names are exact, a field that is always present must be there, `null` is taken only by a field marked "or `null`", and an integer is a number whose nearest double is a whole number of at most 2^53 − 1, so `2.0` is 2. `internal/proto/testdata/crypto.json` holds such payloads in `received` and `devlogs`. What remains is the depth of nesting each parser accepts (`planning/risks.md` K-16).

### 2026-11-20 · limitation · Go verifies signatures of small-order public keys that OpenSSL refuses

- Symptom: for the device id `AQAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA` (the identity point), the signature made of the identity point and a zero scalar verifies over any text in `crypto/ed25519`, so `VerifyChallenge` and `OpenCommand` (with a `known` that admits the id) accept it without any private key. The other small-order points verify such forgeries for part of all texts. Node 24 (OpenSSL) refuses the same signature, so a Go receiver and another Ed25519 implementation can disagree on whether an envelope or log entry verifies.
- Cause: `ed25519.Verify` (`internal/identity/key.go:90`) follows RFC 8032 without refusing small-order or non-canonical public keys, and `contracts/envelope.md` section 2 does not say whether such a key is a device id.
- Status: resolved in PR #67. The fourteen encodings of small-order points are no device id and no first half of a signature (`contracts/envelope.md` sections 2 and 3); `verify` compares the bytes itself before it asks the library (`internal/identity/key.go:91-137`). As `k` such a value is `bad_sig`, as `dev` it makes a log entry refused, and `Secret.Verify` refuses a `pair` message that carries it. `crypto.json` holds a forged signature under each of the fourteen and one by a real key that begins with the identity point. A real key with a small-order point added to it still passes (`planning/risks.md` K-15).

### 2026-11-20 · deviation · Web note rows refuse `pos` until the Go type has it

- Symptom: the settled row contract (PR #58) gives note rows a `pos`; `decode` refuses a note row that carries one.
- Cause: `NoteRow` mirrors the merged Go `NoteRow` and `internal/proto/testdata/row-note.json`, which have no `pos` (`web/src/lib/proto.ts:261-270`).
- Status: resolved in B-01 slice (d). Both wire types require the creation position and the shared note example includes it.

### 2026-11-20 · bug · Web `verify` threw for a `k` or `s` of some hundred thousand characters

- Symptom: `verify`, `isDevice`, `verifyPair` and `DeviceLog.apply` threw `RangeError: Maximum call stack size exceeded` for a `k`, `s`, `dev` or `mac` of 400 000 base64url characters, where the contract wants `bad_sig`, a refused entry or an ignored `pair` message. Under Node 24 a text of 133 000 characters still passed and one of 267 000 threw; the limit differs by engine and was not measured in the browsers. Nothing was accepted that should have been refused. Found in the review of PR #66, before the code was merged.
- Cause: `decode` encodes the bytes it decoded to compare the spelling, and `encode` spread them all into the arguments of one `String.fromCharCode` call (`web/src/lib/identity.ts:18` before the fix).
- Status: resolved in PR #66: `encode` makes the text byte by byte (`web/src/lib/identity.ts:18-19`). Checked under vitest with 400 000 characters.

### 2026-11-20 · limitation · Browsers accept small-order Ed25519 public keys

- Symptom: `crypto.subtle.verify` returns true for the signature R = identity, S = 0 under the identity point as public key, over any message, in Chromium 159, Firefox 159 and WebKit 27.2 (spike S-7, 2026-11-20). WebCrypto requires false for small-order keys; web-platform-tests `eddsa_small_order_points` passes 8, 9 and 9 of 14 in Chrome 158, Firefox 160 and Safari 27.2 (wpt.fyi, 2026-11-19).
- Cause: the engines' Ed25519 libraries do not make the small-order checks ([Firefox bug 1929022](https://bugzilla.mozilla.org/show_bug.cgi?id=1929022), [WebKit bug 280673](https://bugs.webkit.org/show_bug.cgi?id=280673), both open).
- Effect: relying on WebCrypto alone would admit invalid signers. The envelope contract requires rejection before WebCrypto.
- Status: resolved for the web client in F-01b: the settled contract (PR #58, `contracts/envelope.md` section 2) makes the fourteen weak values no device id, and the client compares the bytes itself before WebCrypto is asked, for `k`, `dev`, `user`, a pair message's `k` and the first half of a signature (`web/src/lib/identity.ts:104-136`). The Go side follows in B-02b.

F-01c measurements. The first two results are native `subtle.verify`; the last is SoloDawn's receiver result for **each** of the fourteen weak-key vectors. CI runs four builds on every pull request; the others ran by hand on macOS 26.6.2.

| Browser                               | Run by                                                                               | Identity-point vector | Valid empty message (RFC 8032 §7.1 test 1) | Receiver  |
| ------------------------------------- | ------------------------------------------------------------------------------------ | --------------------- | ------------------------------------------ | --------- |
| Chrome for Testing 137.0.7151.119     | CI, Ubuntu 24.04; by hand, macOS                                                     | true                  | true                                       | `bad_sig` |
| Edge 137.0.3296.93                    | CI, Ubuntu 24.04                                                                     | true                  | true                                       | `bad_sig` |
| Firefox 136.0 (Playwright build 1478) | CI, Ubuntu 24.04; by hand, macOS                                                     | true                  | true                                       | `bad_sig` |
| WebKit 27.2 (Playwright build 2370)   | CI, macOS 26; by hand, macOS                                                         | true                  | false                                      | `bad_sig` |
| Chromium 156.0.8078.4 (Playwright)    | by hand, macOS                                                                       | true                  | true                                       | `bad_sig` |
| Firefox 157.0 (Playwright)            | by hand, macOS                                                                       | true                  | true                                       | `bad_sig` |
| Safari 26.6.2                         | by hand, macOS, once, with Vitest's preview provider, which is not in the repository | true                  | false                                      | `bad_sig` |

No Safari release runs in CI, and Safari 17.5, the baseline, was not run at all. Playwright's WebKit is a current build of Apple's port, so its row shows that the vectors pass on Apple's Ed25519 implementation of today, not on that of Safari 17.5. For Safari 17.5 the baseline still rests on the sources cited in the browser-baseline decision, not on a run.

### 2026-11-20 · limitation · Firefox 136 cannot export the JWK of a seed imported as PKCS #8

- Symptom: on Firefox 136.0, importing an Ed25519 seed succeeds and it signs, but exporting the imported private key as JWK rejects with `OperationError`. The other builds above export the expected public coordinate.
- Effect: the original shared test used that export to construct a key pair. SoloDawn does not export its private device keys.
- Status: handled in F-01c. The suite does not export: it imports the public key of the vector and proves that **each** seed signs for it. No signature bytes are compared.

### 2026-11-20 · limitation · Web client does not start when the browser blocks site storage

- Symptom: expected blank page when the person has blocked site data for the page's origin. Checked under vitest with a `localStorage` that throws (the import of `prefs` rejects); not reproduced in a real browser.
- Cause: `prefs` reads `localStorage` while its module loads (`web/src/lib/prefs.svelte.ts:12-16` before the fix), and both the application and the upgrade notice import it. A browser that refuses storage access throws on `localStorage` (Chrome raises a `SecurityError`), so the import fails before `boot` mounts anything.
- Status: resolved in 815f0547d (PR #61). Storage is read and written behind a guard (`web/src/lib/prefs.svelte.ts:17-35`): a failed read, or a value the client does not know, gives the default; a failed write is ignored. Checked under vitest with a `localStorage` getter that throws a `SecurityError`; still not tried in a real browser.

### 2026-11-20 · bug · Go acceptance suite ran the web feature

- Symptom: `go test -tags acceptance -count=1 ./features/...` failed with `step is undefined` for seven scenarios of `features/web.feature` (for example `the current page is "群聊"`), so the merge gate failed.
- Cause: `features/acceptance_test.go:44` sets `Paths: []string{"."}`, so godog runs every `.feature` in `features/`, including `web.feature`, whose steps are implemented in `web/features/web.spec.ts` (vitest-cucumber) and not in Go; `Strict: true` fails the undefined steps. Each branch passed on its own (the F-00 branch had no `go.mod`, so Go acceptance was skipped; the B-00 and B-01 branches had no `web.feature`); the conflict appeared only after both were merged.
- Status: resolved in PR #59: `web.feature` is tagged `@web` and the godog run uses `Tags: "~@web"`.

### 2026-11-20 · bug · The store wrote rows with HTML escapes

- Symptom: a note with 16,000 `<` characters was stored as 96,124 bytes, though `planning/contracts/conventions.md` section 4 counts a row's size on its JSON without HTML escaping and the home accepts rows up to 16 KiB on that count. A row the home accepts could be stored above the 60 KiB limit of a `ctl` message, so a `rows` or `fetched` frame built from it could not be sent. No node wrote rows yet, so nothing was broken.
- Cause: `Put` encoded the row with `json.Marshal` (`internal/store/put.go:17` before the fix), which escapes `<`, `>` and `&` as six-byte `<`, `>`, `&`. `internal/peer` encodes frames without escaping and leaves stored escapes as they are.
- Status: resolved in PR #163. `Put` and `PutReceipt` encode through `encode` (`internal/store/put.go:29`, a `json.Encoder` with `SetEscapeHTML(false)`, newline trimmed); `internal/store/put.go:18` and `internal/store/receipt.go:16` call it. Raw JSON the store returns (read paths, signed envelopes in the inbox) is never re-encoded. Test: `TestPutStoresWithoutHTMLEscapes` in `internal/store/read_test.go`. `internal/proto/decode.go:64` has the same `json.Marshal` pattern but is an intermediate that is decoded again at once; not changed (owned by P-1).

### 2026-11-20 · bug · A refused read left the top of the main line loading forever

- Symptom: when the home refused the `sync` of a group with `forbidden` while the group was still listed, the chat top showed the idle loading dots for good instead of "Earlier messages cannot be loaded right now."
- Cause: `Group.sync` returned on `answer.err` without touching the paging (`web/src/lib/groups.svelte.ts:160-170` before the fix), so `more=true` and `failed=false` stayed while the connection was up, and `stuck` in `web/src/components/chat/Chat.svelte:19` stayed false. No page is asked for after a refused read (the group is not live), which is intended.
- Status: resolved in PR #169: a refused read sets `paging().failed` (`web/src/lib/groups.svelte.ts:163`), a read that comes too late does not. The next successful read starts a fresh paging. Tests in `web/src/lib/groups.test.ts` (refused read, late refusal).

### 2026-11-20 · bug · `plain` copied its list at every mention

- Symptom: 2300 mentions in one text took about 69 ms to cut into parts, and it runs on every key press; 6000 overflowed the stack.
- Cause: `plain` (`web/src/lib/compose.ts` before the fix) was recursive and spread the result of the rest of the text into a new list at each mention, so the cost grew with the square of the mentions and the depth with their number.
- Status: resolved in PR #169: `plain` builds the list with a loop (`web/src/lib/compose.ts:53`), same output. Test in `web/src/lib/compose.test.ts` (2300 mentions give the same parts; 6000 take under ten times what 1500 do).

### 2026-11-21 · deviation · Seven error paths in `internal/store` had no unit tests and two `Scan` checks were unreachable

- Symptom: `go test -cover ./internal/store/` reported 93.7% of statements, leaving nine error-return blocks uncovered across `inbox.go`, `read.go` and `store.go`.
- Cause: unit tests did not exercise locked or closed database paths in `Enqueue` (`internal/store/inbox.go:20-22`), `Inbox` (`internal/store/inbox.go:35-37`), `Snapshot` (`internal/store/read.go:20-22`, `:26-28`, `:31-33`), `Snapshot.rows` (`internal/store/read.go:72-74`) and `transaction` (`internal/store/store.go:78-80`); and `Inbox` (`internal/store/inbox.go:44-46` before the fix) and `Snapshot.rows` (`internal/store/read.go:79-81` before the fix) checked `rows.Scan` errors that cannot occur. Both `inbox` and `row` are SQLite `STRICT` tables (`internal/store/schema.sql:1-17`, `:31-44`; `planning/contracts/storage.md` section 1) whose selected columns match the non-nil Go destination types (`INTEGER NOT NULL` and `INTEGER PRIMARY KEY` to `*int64`, `TEXT NOT NULL` to `*string` or `*[]byte`, nullable `TEXT` to `**string` or `*[]byte`), while `modernc.org/sqlite` steps and reads column values inside `rows.Next()`, surfacing any SQLite or I/O failure through `rows.Err()`.
- Status: resolved in #174: removed the two unreachable `rows.Scan` error branches in `internal/store/inbox.go` and `internal/store/read.go`, and added unit tests covering all seven reachable error paths (`TestInboxFailures` in `internal/store/delivery_test.go`, `TestSnapshotAndQueryFailures` in `internal/store/read_test.go`, `TestWriteBeginFailure` in `internal/store/store_test.go`), bringing `internal/store` to 100.0% statement coverage.

### 2026-11-26 · bug · A remote close was logged as `why=closed` when the connection state came before the `ctl` close

- Symptom: `TestPeerDiagnostics` failed in CI (run 45669069695, `internal/peer/diag_test.go:113`): the side that did not close logged `conn.close why=closed` instead of `ctl_closed`.
- Cause: pion reports either the `ctl` close or the state `closed` first; `connectionStateChanged` (`internal/peer/signal.go:310-311` before the fix) answered the state with `c.Close()`. A local `Close` takes the `once` before its own state change, so a `closed` state that gets through is always the remote's.
- Status: resolved: the state closes with `ctl_closed` (`internal/peer/signal.go:311`), as the `ctl` close does (`internal/peer/ctl.go:78`). Test `TestPeerDiagnosticsRemoteStateClosed`; `planning/contracts/diagnostics.md` names the side of each `why`.

### 2026-11-26 · bug · `TestUpdateSlowButSteady` failed on Windows CI, and the stall timer ran while the reader worked

- Symptom: `TestUpdateSlowButSteady` failed with `self-update: no data for 60 s` after 1.09 s in `test (windows-latest)` of PR #407.
- Cause: the stall timer was restarted at the end of each `Read` (`internal/node/update.go` `progressBody.Read` before the fix) and kept running while the reader (`stageRelease`: file write, `Sync`) and the code between `get` and the first read worked, so a reader that was slow, as on a loaded Windows runner, was cut as a stall although every byte had arrived. The test also gave 150 ms of margin to real timers, a 50 ms handler sleep against a 200 ms limit (`update_test.go` before the fix). Injecting a 250 ms delay after the reset in the old code reproduces the same message.
- Status: resolved. The timer now runs only while a `Read` waits for data (`internal/node/update.go:131-139`, `:171-175`). `TestUpdateSlowButSteady`, `TestUpdateSlowReaderIsNotAStall` and `TestUpdateStallsBetweenBytes` run in a synctest bubble on an in-memory transport, so they do not depend on real time. `TestUpdateStallsWithoutAnswer` and `TestUpdateStallsInTheBody` keep real timers and a real server: they pass whenever the stall fires, however slow the machine.

### 2026-11-26 · bug · A person with one device could not consent to a watcher, and a watcher's page did not recover

- Symptom: (1) an owner could answer a `watch` card in the chat only, and leaving the terminal page ends the takeover that the grant is for; the in-page and the system notification of the card led there without saying so. (2) The page of a watcher whose `watch.start` was refused stayed on the refusal after the consent. (3) The page of a watcher with no connection to the AI's computer dialled in a loop until the browser refused ("Cannot create so many PeerConnections"), so that watcher could never watch. (4) A refused or denied request could not be made again on a card whose button had sent one.
- Cause (before the fix): (1) `ConfirmCard` was mounted in the chat alone (`web/src/components/confirm/mounts.ts`), and the effect cleanup of `web/src/routes/Terminal.svelte:89` hands back; `Notifier.open` and `ReminderSync.openTarget` set the chat's address. (2) `Watch.follow` starts only from `idle` (`web/src/components/watch/watch.svelte.ts:49`). (3) the effect at `web/src/routes/Terminal.svelte:92-94` called `net.watchHost` tracked, which pushes to `homes` (`homeOf`, `web/src/lib/net.svelte.ts:106`): the effect invalidated itself, and its cleanup released the host it had just dialled. (4) `sent` in `AskToWatch.svelte` was never reset.
- Status: resolved. The terminal page shows the owner's pending `watch` cards of that AI over the terminal (`onOpenTerminal` in `web/src/lib/notify.svelte.ts`); both notifications leave the owner on it. `watchBar` and `watchRequest` (`web/src/components/watch/watch.svelte.ts`) say how the reader's request of this takeover stands; with the consent the page dials the computer again (`watchHost` in `web/src/lib/net.svelte.ts`) and tries a refused start again by the `Retry` of `peer.ts`. Tests: `web/src/components/watch/*.test.ts`, `notify.test.ts`, `net.test.ts`, and A9.20/A9.21 with three real nodes in `web/e2e/s2.spec.ts`.

### 2026-11-26 · bug · Review of the watcher's page: an untrue bar, retries without an end, a dial without an end

- Symptom: (1) with a consent and a computer that keeps refusing (the owner stopped the watcher), the bar promised the terminal "in a moment" for ever, and each retry removed and added "Ask to watch", which lost the keyboard focus. (2) Each change from relayed to direct started another chain of retries, an `ended` watch started again after relayed and direct, and a connected computer was asked once a minute after the takeover. (3) The page dialled the computer after the takeover had ended, until it was left. (4) A system notification of a request that the browser did not hold (the home was being reconnected) opened the chat, which hands the terminal back.
- Cause (before the fix): (1) `asks` in `web/src/components/watch/watch.svelte.ts:150,164` read `phase`, which each retry changes, and the bar with it. (2) `handBack` (`:80`) left the timer, `follow` (`:63`) did not read `ended`. (3) `watchHost` in `web/src/routes/Terminal.svelte:63` was kept until `ended`. (4) `here` in `ReminderSync.svelte` read the rows held.
- Status: resolved. `errorCode` stays until the answer and is cleared in `#close`, where the retry stops; `reached` in `Terminal.svelte` limits the dial and the starts; `maybeHere` in `ReminderSync.svelte` waits for `Group.seek`. The phase is written once in `#close`: `follow` reads it, and two writes would run its effect without an end. Tests: `web/src/components/watch/*.test.ts`, `net.test.ts`.

### 2026-11-26 · limitation · Two rough edges of the watcher's page

- The AI's computer loses its home for a moment: the home sets `offline` and a new `since` (`internal/home/cmd_hostev.go:167,184`), though the host keeps the takeover and the grant. A watcher who is not watching yet reads "The takeover has ended" and then "The owner's consent is needed", and the owner's card is hidden meanwhile. An open watch is not touched.
- A watch that was active and ended (connection lost, full buffer, owner's stop) stays on "Watching ended" without "Ask to watch", and a new consent does nothing on that page. `planning/contracts/step-2/watch-team.md` section 6 lets the browser start again after a lost connection. Leaving the page and coming back works.
- Status: open.

### 2026-11-26 · bug · `compose.test.ts` and `onboard.test.ts` failed three tests under load

- Symptom: in one full vitest run at a 1-minute load of 17, one test of `web/src/lib/compose.test.ts` and two of `web/src/lib/onboard.test.ts` failed; the rerun passed. Reproduced at a load of 20 to 45: the onboard pair failed in 1 of about 95 runs (`expected 'waiting' to be 'accounts'` at the first mock test, then `expected 'accounts' to be 'ready'` at the next), and the time ratio of the compose test went over its limit in 15 of 4000 measurements.
- Cause: (1) the compose test measured wall-clock time, `time(6000) < 10 * max(time(1500), 5 ms)` (`web/src/lib/compose.test.ts:295` before the fix): the parts of 6000 mentions take under 1 ms, the limit is 50 ms, and a stall of 50 ms or more in that one call, which a busy machine gives, fails it. (2) The first mock test waited for the stage with `vi.waitFor`, whose deadline is 1000 ms of real time (`web/src/lib/onboard.test.ts:285` before the fix), while the chain it waits for starts with the cold import of `mock.ts` and a key generation, and a stall of that chain past the deadline fails it. The failed test never destroyed its session, which went on and added its `onpush` to `mockPushes` after the next test's `beforeEach` had emptied it, so `mockPushes[0]` in the next test was the dead session's (`:353`). (3) Under fake timers `vi.waitFor` moves the fake clock by 50 ms at every real poll, so the countdown test's clock depended on how many polls the machine needed, and a faked `setTimeout` or `setImmediate` stopped the fake IndexedDB.
- Status: resolved. The compose test counts the characters `slice` copied (at most the length of the text, for 2300 and 6000 mentions), which fails for the recursive `plain` of before #169 (26 million for 2300), whatever the load. The onboard tests wait with `until` (`web/src/lib/onboard.test.ts:80`), which has no clock of its own, and the countdown tests fake only `setInterval`, `clearInterval` and `Date`. A 1.3 s stall in the mock import fails the old file with the same two messages and passes the new one; the file runs in 0.7 s instead of 4 s. `vi.waitFor` is still used elsewhere, mostly in files that do not wait on a cold import or a faked clock; they have not failed.

### 2026-11-26 · bug · `TestDiagNodeEvents` failed with `context canceled` about once in 200 runs

- Symptom: `diag_test.go:73: context canceled` from `n.peer.Offer`; 2 of 200 serial `-race` runs on a Mac, once of three in a CI baseline run (mutation run 45724118458). Not seen in 240 runs under load, nor with `GOMAXPROCS=1`.
- Cause: the test offered to the owner's key, which was not connected to the node's local rendezvous. `SetLocalDescription` starts gathering before `Offer` sends the offer (`internal/peer/signal.go:263-279`), so a candidate is written first; the server answers `offline` (`internal/rendezvous/server.go:292`), `Peer.Offline` retires the offer (`internal/peer/unanswered.go:35-45`, flows/connect.md step 15) and cancels its context, and the offer's `pacer.turn` then returns `context.Canceled` (`internal/peer/path.go:80`) when the offer goroutine runs after that reply. A 100 ms delay before the offer send in `Offer` makes the old test fail 5 of 5 and the fixed one pass 5 of 5. The product is as designed (an offer to an offline target is retired); the empty `node=` lines in the CI log are from a test harness that starts a home loop with no `Home` (`internal/node/devlog_test.go:280`); the node always sets `Home` (`internal/node/node.go:240`), and they are unrelated to the failure.
- Status: resolved. The test connects the owner's key to the local rendezvous first (`internal/node/diag_test.go`, `peer.Dial` on `n.httpPort`), so the target is online and no `offline` reply exists.
### 2026-11-26 · bug · A node exited at startup when another process held its web port on 127.0.0.1

- Symptom: in a stress run a node failed at startup with `failed to WebSocket dial: expected handshake response status code 101 but got 404` and exited.
- Cause: `listenNodeHTTP` (`internal/node/node.go` before the fix) bound the web port `47800` on the wildcard address and fell back to a free port only when that bind failed; `startLocalHTTP` then dials its own rendezvous at `ws://127.0.0.1:<port>/rv`. macOS grants the wildcard bind while another process holds `127.0.0.1:47800` (Windows, whose sockets Go opens without `SO_EXCLUSIVEADDRUSE`, is documented to allow it too; not tried here), and a connection to 127.0.0.1 then reaches that process, so the node's dial got the stranger's answer. Linux refuses the wildcard bind, and the fallback worked there.
- Status: resolved. `listenNodeHTTP` (`internal/node/node.go:420-431`) first binds `127.0.0.1:<port>` and treats the port as taken when that fails, so the node serves on a free port as `planning/contracts/local.md` section 10 says, and `solodawn open` shows that address. `TestNodeWebPortShadowedOnLoopback` holds the port on 127.0.0.1 with a server that answers 404; it fails with the message above on the old code on macOS. A process holding the port on `::1` only is not detected: the node's own dial does not use `::1`, but a browser opening `localhost:47800` may reach that process.

### 2026-11-26 · limitation · Three surviving host mutants had no behaviour to test

- Symptom: mutation run 45739942536 left `internal/host/limits.go:258` (`<` to `<=`), `takeover.go:251` and `codex.go:600` (`>` to `>=`) alive.
- Cause: `holdLimits` compared the clock with `rateUntil`, but the timer of `onRateLimited` deletes the entry at the reset, so only the entry's presence decides the hold. `lastTask` and `selectRateWindow` differ under the mutants only on a tie (equal `LastActive`, equal `usedPercent`), which no contract defines.
- Status: resolved. `holdLimits` checks that the entry exists; `lastTask` and `selectRateWindow` use `slices.MaxFunc`, which keeps the first on a tie as before and has no comparison to mutate.
### 2026-11-26 · bug · The web mutation job's step summary passed GitHub's 1 MiB limit

- Symptom: `$GITHUB_STEP_SUMMARY upload aborted ... got 1056k` in the shard "web rest" of `mutation.yml`.
- Cause: the vitest github-actions reporter (on whenever `GITHUB_ACTIONS` is set) appends a report to `$GITHUB_STEP_SUMMARY` after every test run (`jobSummary`), and Stryker runs the tests once per mutant.
- Status: resolved. The Mutate step runs Stryker with `GITHUB_STEP_SUMMARY` empty and then writes the score table and the mutants that lived or have no coverage to the summary.
- Mutants of the final gate that could not change behaviour, so the code that held them is gone: `confirm.ts` `if (state.status === 'expired') return label` (the label of an expired card has no `{who}`, so the replacement callback is never called) and `notify.svelte.ts` `route.page === 'terminal'` (only the terminal page has a `member` in its address, and `route.params` is not typed by page).

### 2026-11-26 · bug · A9.21 timed out on Windows: the relay's round trip read 0

- Symptom: on PR #420, `test (windows-latest)` (run 45758838792), A9.21 waited 2m54s for the TURN relay and saw the direct `srflx` pair until ICE failed. It passed on Windows in earlier runs and on the Mac.
- Cause: the fixture, not the product (only the watcher's pion peer in `features/m7_test.go` renominates). Pion v4.4.8 (`agent.go`, `evaluateCandidatePairQuality`) rates a pair whose last round trip is 0 as one without a measure, 30 below; `CandidatePair.UpdateRoundTripTime` stores a measured 0 as it is. The Windows clock of the test process moves in steps of about 0.5 ms (the timestamps of `nodes.log`), and from the first keep-alive on, each relay check of the watcher (`Received ChannelData` from its TURN client) and its answer (`Relay socket … received 64 bytes`) have one timestamp: the relay pair read 0 ms and scored 10 − 30 + 20 = 0, below the dead direct pair's 50 − 10·log10(3000) ≈ 15.2, so `findBestCandidatePair` kept the direct pair at every check, and no nomination request crossed the TURN server. Reproduced on the Mac with pion's round trip truncated to 2 ms steps: the old fixture fails with the CI message (relay RTT 0.00 ms at each check).
- Status: resolved on `fix/a921-flake`: `moveToRelay` (`features/s2_watch_peer_test.go`) also delays the watcher's packets to the TURN server by 5 ms (`m7NetControl.relayNs`, `features/m7_test.go`), so the relay pair reads at least a few ms and scores about 23 (5 ms) instead of 0. With the 2 ms steps the fixed fixture passes 3 of 3.

### 2026-11-26 · bug · A held item could wait for the next push after a sweep missed its AI

- Symptom: `TestWorkspaceFailureReportingConfigUnmarshalAndStoreErrors` timed out in a CI mutation baseline (`internal/host/deliver_test.go:1755`, waiting for 914 review failure and 912 group recovery; turns=3, acks 6 then 7): item 914 was handled, item 912 for the secretary never.
- Cause: a product race in delivery. The sweep of `deliverNow` and `afterGroupState` went over `heldKeys`, the AIs with held items. A step that takes a batch removes it from the held items and, when it cannot go (here a group refresh that failed with the group row it had read before the test changed `forge` to `none`), holds it again at its end (`deliverTargetBatch`, `internal/host/deliver.go:1331`). A sweep in between did not see the AI and did not set its recheck, so nothing looked at the item again until another push. Reproduced with the CI message by delaying that re-hold by one second.
- Status: resolved. The sweeps use `sweepKeys` (`internal/host/deliver.go:853`), which adds the AIs being stepped, so `deliverPair` sets their recheck. `TestSweepRechecksAnAIWhoseBatchIsInFlight` fails on the old code.

### 2026-11-26 · bug (test) · The PTY tree-kill test could not see a leaf that lived

- Symptom: `pty_unix.go:78` (`-pid` to `pid` or `+pid`) lived or timed out in mutation runs although `TestPTYKillTreeStopsLeaderAndChildSubprocess` checks that the leaf stops.
- Cause: the leaf helper listened but never accepted. `waitLeafStopped` dials every millisecond; once the backlog was full a dial failed, which it read as the leaf gone.
- Status: resolved. The leaf accepts and closes connections (`runPTYHangLeafHelper`, `internal/host/pty_test.go`), so the test fails under both mutants.

### 2026-11-26 · limitation · `watch.go:112` (`>` to `>=`) was an equivalent mutant

- Cause: device rows of one log have distinct versions from 1 (`planning/contracts/envelope.md` section 4), so `>=` chose the same row. A revoked device is never added again, so the latest entry is a revoke exactly when there is one.
- Status: resolved. `watchDeviceUser` returns no user when the device has a revoke and otherwise the user of its add, with no version comparison.

### 2026-11-27 · bug · An external AI's first review run cannot be handed out: "report not found"

- Symptom: with X external and the reviewer of a task, `solodawn wait --member X` exits 1 with `not_found: report not found` once the assignee has submitted (`features/a10_external.feature`, the reviewer example of A10.22 and "Two tasks run from brief to done with X as one of the two AIs").
- Cause: `extHandOut` (`internal/host/external.go:310`) composes the hand-out's context with `Compose(h, s, ReplacedTurn, nil)` before `commitTurnItems` records the item's report on the session (`noteReport`, `internal/host/deliver.go:1377-1389`). For a new review session `s.Report` is empty, so `replacedSessionLines` (`internal/host/turn.go:822`) fetches the message with id "" and fails.
- Status: resolved by the commit "Host: an external AI's review hand-out reads the report recorded for it". Cause `internal/host/external.go:310`: the context was composed before `noteReport` recorded the report. `extHandOut` now composes it from the recorded row after the acknowledgement.

### 2026-11-27 · bug (test) · A9.46's later browser of Q could not connect to P1: "connection ended before it was ready"

- Symptom: on Linux CI (PR #424, run 45906913476) the step "a browser of Q that connects to P1 later lists \"Beta\"" failed after 10 s with `timed out after 3m0s waiting for ctl to <P1>; saw 1: connection ended before it was ready`; locally 4 of 64 runs failed the same way with eight runs in parallel.
- Cause: `laterBrowserLists` (`features/s2_people_groups_test.go:93`) enrolled the browser with `AddBrowser`, which sends Q's new device entry as a `devlog` push and does not wait, then offered a ctl connection to P1 at once. The offer travels through the rendezvous and can reach P1 before P1 has applied the entry from Q's connected browser; `identity.OpenSignal` (`internal/peer/signal.go:110`) then drops it as `unknown_device` (`planning/contracts/envelope.md` section 3), as the contract requires, and the browser's offer ends unanswered after 10 s. P1's log shows the browser's ICE candidates received but no offer, and the entry's device rows sent just after the offer arrived. Not a product race: a device is admitted only once the home holds its entry. The "3m0s" in the message is `openS2PeopleCtl`'s budget, not the time waited.
- Status: resolved by the commit "Acceptance: a later browser connects only once its device is known (A9.46)". The step waits for the entry's device row on Q's connected browser (`awaitQJoinedOnView`), as `s2_confirm_test.go` `awaitSecondBrowser` does, before it connects.

### 2026-11-27 · expectation mismatch · An AI announced work in the group and nothing ran it

- Symptom: an AI posted that it would do something (the secretary: "I'll arrange the merge") and ended its turn; nothing followed. The owner's rule (2026-11-27): whoever posts that they will do something must do it; a report of what is done is fine, a promise in it must be kept.
- Cause: a session runs only when a new item reaches it, and a turn ends after the post. The instructions stated no duty to act before ending the turn, and the secretary's closing-step line (`internal/host/instructions/secretary.md:23`) said "arrange" without saying when or how.
- Status: resolved in branch `m/follow-through`. Every role's instructions say "Never end a turn with a promise": do it in the same turn, or hand it to a tool that makes it run (`start_task`, `set_task`, `propose_brief`, `ask`, a mention, `request_decision`) and say which (`planning/contracts/drivers.md` section 4). The secretary requests the closing step in the same turn. No code looks for promises in the text; the duty is not enforced (`planning/risks.md` K-26).

### 2026-11-27 · deviation · The demo world kept the old model's wording

- Symptom: AI reports in the demo said the change was "committed on branch t-1 and pushed to origin", and a person's message spoke of merging.
- Cause: `report` in `web/features/sim/content.ts` and `demoDay` in `web/features/sim/seed.ts`. SoloDawn makes no branches or merges; an AI works in its folder and says where its result is.
- Status: resolved. The report names the folder and the file; no branch, merge, push, origin, PR or check wording is left in `web/features/sim`.

### 2026-11-27 · bug (demo) · Moving a group emptied the computers' cards for a moment, and left them stale

- Symptom: after the group panel's "move" button, a computers page opened (or `host.info` read) within the next 2.5 s showed the moved group's AIs as "Not in any group yet" and "0 open" on every computer.
- Cause: `Events.move` (`web/features/sim/events.ts`) took the group out of the old computer at once and put it into the new one 2.5 s later. In between no computer held it, so `Host.#members` (`web/features/sim/host.ts:35`), which goes over `world.groups()`, found none of its AIs and `host.info` listed none.
- Status: resolved. The exported group stays on its old computer until the import (`World.arrive`, `web/features/sim/world.ts`); `SimNode.listed` does not list it meanwhile, and the old computer already answers `moved`. A world saved in between is imported when it is read back (`World.restore`). `keeps the AIs on their computers while a group moves` fails on the old code.

### 2026-11-27 · bug · The Computers page listed a group that moved as the home of the computer it left

- Symptom: after a group moved (the client follows `moved`), the old computer's card still read it under "Hosts" until the next connection, and the new computer's card listed it too.
- Cause: `hostedGroups` (`web/src/routes/Nodes.svelte:29`) took every `host.info.homes` entry of the computer as its own home. `host.info` is read on connect and pushed for `watchers` only (the limitation of 2026-11-21), and `nodes.infos` takes no push, so what it holds is as old as the last read; the browser knew by then that the group lives elsewhere. Now an entry is skipped when this browser holds the group on another computer. `web/src/routes/nodes.test.ts` fails on the old code.
- Status: resolved. What `host.info` shows of the AIs (`agents[].groups`) does not change with a move: an AI stays on its computer.

### 2026-11-27 · bug (demo) · A task given to another AI had it ask an answered question again

- Symptom: after a task was paused with its decision card answered and given to another AI, the new AI put the same decision again.
- Cause: `Ops.#handOver` (`web/features/sim/ops.ts`) dropped every mark of the task's script but its name, among them `ask`, so the next stage of the work asked again. The real client only draws the cards the node writes. Now `Tasks.restart` (`web/features/sim/tasks.ts`) keeps `ask` when a card of the task holds the question, answered or pending; a reassigned approval is asked again. `does not ask a question again that was answered when a task goes to another AI` fails on the old code.
- Status: resolved.

### 2026-11-27 · bug · A member that left showed "?" as its picture beside its name

- Symptom: the old messages of a removed member showed a "?" picture next to the name that was kept.
- Cause: `Message.svelte`, `Thread.svelte`, `TaskCard.svelte`, `BriefCard.svelte` and `ApprovalCard.svelte` gave `Avatar` the name of the member row, which is gone, and `?` when there is none, while the name beside it came from `Group.nameOf` (`web/src/lib/groups.svelte.ts:142`), which keeps the name a member left with.
- Status: resolved. The picture takes `nameOf` too. `web/src/components/chat/message.test.ts` fails on the old code.

### 2026-11-27 · note · The "waiting for you" chip jumped every time it was tried

- Symptom: one click on the chip at the top of a group was reported not to reach its card.
- Cause: not found. About 14 clicks (scrolling the main line away first, with the wheel and by script, and twice quickly) all ended with the card in view and its first control focused. A smooth scroll from the top of a long line starts about a second after the click and takes two more, so a look within 1.5 s sees the card still on its way (`jumpToMessage`, `web/src/lib/jump.ts`).
- Status: no change.

### 2026-11-27 · note · Adding a person to an existing group without a link is in the invitation dialog only

- Symptom: the walk through the demo found only the invitation link.
- Cause: `InvitationDialog.svelte` lists "people in your other groups on this computer" (`knownPeople`, `web/src/lib/members.ts:374`) with an Add button (`member add_person`), but only people of other groups homed on the group's own computer who are not in the group yet. The demo has none for "官网改版" (the only other group, "产品讨论", is on the office computer, and has Mina, who is in the site group already), so the section does not show. The members panel itself has no picker; the plan's picker is the create dialog's people list and this section.
- Status: as designed; nothing built.

### 2026-11-27 · deviation · The invitation dialog of the demo's website group offered nobody to add

- Symptom: the section "你在这台电脑上其他群里的人" did not show in "官网改版", so adding a person without a link could not be tried in `?demo`.
- Cause: `knownPeople` (`web/src/lib/members.ts:374`) lists the people of the other groups homed on the same computer, and the demo had none: its only other group, "产品讨论", is homed on the office computer and holds Mina, who is in the website group already.
- Status: resolved in the demo world only. `blog` (`web/features/sim/seed.ts`) is a third group, "博客", homed on the MacBook with Alex and Nora; the dialog lists Nora, and adding her makes her a member who says hello (`People.added`). `adds Nora, who is in the blog group on the same computer, without an invitation` in `web/features/sim/sim.test.ts` covers it.

### 2026-11-27 · bug (demo) · The closing step asked only the AI of the last task, in the main line

- Symptom: after a batch the secretary asked "the responsible AIs" in the main line and only the assignee of the last finished task answered; the AIs of the other folders were never asked, and a task with no brief got no closing step.
- Cause: `Tasks.#summary` (`web/features/sim/tasks.ts:329-332` before the fix) posted `allDone` once per brief, mentioning `t.assignee` of the task that ended last. `planning/flows/task.md` step 17 has the secretary ask the assignee in the task's thread after each task's summary.
- Status: resolved. `Tasks.#close` asks each task's assignee in that task's thread, whether or not it has a brief, and the assignee answers there from "my folder"; `allDone` is a plain notice. `sim.test.ts` checks the request and the answer per task.

### 2026-11-27 · bug · The "waiting for you" label was not a link

- Symptom: people click the bold label with the dot; nothing happened.
- Cause: `WaitingBar.svelte:15-18` drew it as a `<b>` with no link; only the chips after it jumped.
- Status: resolved. The label is a link to the first waiting card (`a.lead`, same look); `web/src/components/approval/waiting.test.ts` fails on the old code.

### 2026-11-27 · bug (demo) · The takeover terminal of an AI failed after its group moved

- Symptom: after a move, opening the terminal of an AI that stayed on the old computer answered "moved".
- Cause: `SimNode.#route` (`web/features/sim/node.ts:122-123` before the fix) answered `moved` to every request with the group's id, host commands such as `takeover.start` too. The real node gives host commands to the host (`internal/node/session.go:581-590`), and only the home's `Cmd` answers `moved`.
- Status: resolved. A command with `to` is a host command and skips the check (`SimNode.#movedTo`); `still starts a takeover on an AI of the computer after its group moved` fails on the old code.

### 2026-11-27 · bug (demo) · The brief's title was empty when the request ran on straight after "@秘书"

- Symptom: "@秘书首页加一个…" gave the chip "确认开工 · " with no title; "@秘书怎么…" was not taken as a question.
- Cause: the demo's secretary, not the client. `titleOf` (`web/features/sim/content.ts`), the goal and `isQuestion` (`web/features/sim/talk.ts`) cut `@\S+`, which takes the whole run of characters up to the next space, a sentence in Chinese. The client reads `@秘书` as a mention when a non-word character follows (`named`, `web/src/lib/compose.ts:39-46`), and the brief's title is written by the secretary AI, so the real product has no code that makes it.
- Status: resolved. `Talk.#request` cuts the names of the group's members. `sim.test.ts` fails on the old code.

### 2026-11-27 · limitation · A device that typed an invitation code and whose person is then added with `add_person` stays a joining peer for 10 minutes

- Symptom: a browser types a group's invitation code and sends no `join`; within 10 minutes an admin adds its person to a group on the same home with `member add_person`. Until the 10 minutes end, each new connection of that device to the home is a joining connection: its `welcome` has `groups: []` and `join`, and `sync` is answered `forbidden`. A `join` for the code's group on it is answered `ok` and ends it; so does the end of the 10 minutes, which closes the connection, and the next one is a member's.
- Cause: `Pairer.attach` (`internal/node/pair.go:175`) serves a device that has a `codePeer` as a joining peer whether or not its person is a member, because a member who types the code must get a joining connection too: only its `welcome` tells the browser the group (`planning/contracts/step-2/people.md` 2.2, point 3, and 4.1). The record ends when the home admits a joining connection of the device, by a code or by a one-time link (`admitted`, `internal/node/admit.go:150`), at the end of the 10 minutes or with the code (`dropPeer`); `add_person` admits no connection, so nothing tells the node.
- Status: open, accepted.
