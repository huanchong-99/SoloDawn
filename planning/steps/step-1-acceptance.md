# Step 1: milestones and acceptance tests

The milestones M1 to M8 of step 1 and the acceptance tests of each. Scope,
spikes (M0) and the order of the milestones are in `steps/step-1.md`; the
tasks that make each test pass are in `steps/step-1-backend.md` and
`steps/step-1-frontend.md`.

## How acceptance is tested

- **Unit tests** for pure logic: envelope, device log, command handling, the
  task machine, limits.
- **Integration tests** run real nodes and a real rendezvous in one test
  process or on one machine, with **fake CLIs**: two small test programs
  (`internal/host/fakecli/claude` and `.../codex`), built as `claude` and
  `codex` and put first on PATH, that speak the structured protocols and
  follow a script. Their message shapes are copied from what the real CLIs
  sent in spikes S-2 and S-3. They test the node, not the vendors' sandboxes.
  These tests cost no model usage and run in CI.
- **End-to-end tests** drive the web client in a browser against those
  nodes.
- **Live tests**, a short list marked `live`, use the real CLIs with the
  owner's sign-in. They run by hand before a milestone is called done.

"Within n seconds" is measured by the test.

## M1. Identity and connectivity

- **A1.1** A fresh browser and a fresh node pair through the onboarding
  prompt. The fingerprint on the page equals the one printed by
  `solodawn status`. Before the log arrives, the connection accepts nothing
  but `hello` and `devlog`. A log whose entry for the new device is signed
  by a device other than the pairing peer is not adopted.
- **A1.2** A second node is added. The browser lists both with system,
  version and path.
- **A1.3** On one local network the path is `direct`. With UDP and the ICE-TCP
  port blocked the path is `relay` and the indicator says so within 1
  second. A `host.info` request sent every second is answered throughout.
  (The sentence on becoming `direct` again after unblocking was removed by
  the decision of 2026-11-21: no relay-free probe and move. A relayed
  connection becomes direct only when ICE finds a direct pair itself or at
  the next reconnect.)
- **A1.4** With the rendezvous stopped, open **direct** connections keep
  working for 10 minutes. (The sentence on reconnecting without the
  rendezvous was removed by the decisions of 2026-11-21, no local-network
  beacon, and 2026-11-22, PR-2 cancelled.)
- **A1.5** Removed under the owner's rule of 2026-11-21: no defence in code
  against a dishonest rendezvous; handled outside the product in the
  [deployment guide](../../docs/cookbook/self-hosting.md).
- **A1.6** Removed by the owner's decision of 2026-11-21 (no stolen-device
  defence in code; revoking is postponed, and a lost device is handled outside
  the product in the [deployment guide](../../docs/cookbook/self-hosting.md)).
- **A1.7** When the relay quota is used up, an allocation that is already
  open stops forwarding, and `quota` reports it.
- **A1.8** Six targets build with cgo disabled. Binary size and idle memory
  of a node are recorded in the pull request with the machine named
  (targets: under 30 MB on disk, under 40 MB resident).
- **A1.9** `pair.browser` sent again with the same nonce returns the same
  link while the link is valid and `conflict` after it was used or expired.
- **A1.10** An old offer of a device, sent to the node again inside the
  freshness window after a newer connection from that device has opened, is
  not processed and closes nothing: no connection is created for it, and the
  live connection keeps answering `host.info`. A copy of the offer for the
  current `conn` gets the same answer again, on the path it came by, and
  nothing else happens (`contracts/rendezvous.md` 2.3; covered by
  `features/peer.feature:14-21`).
- **A1.11** Deliver two copies of an offerer candidate before the offer and
  two copies of an answerer candidate before the answer. Each receiver makes
  zero calls to add that candidate before setting the corresponding remote
  description and exactly one afterward. Deliver another copy after the
  connection opens. The count stays one and `host.info` succeeds.
- **A1.12** A second tab of one browser, which has the same device key, shows
  that SoloDawn is open in another tab and offers no connection. After "Use
  here" in it, it connects, and the first tab closes its connections and shows
  the notice (`flows/connect.md` F). Run with a valid device id beginning with
  `-`. After takeover, a reconnect trigger in the first tab opens no
  signalling or peer connection.
- **A1.13** Removed by the decision of 2026-11-21: no relay-free probe and
  move (units PR-1 and W-3 are not built; `internal/peer/README.md`,
  "Limitations").

## M2. Groups, messages, sync

- **A2.1** A group is created from a browser with a name, the person's
  name and two of the person's connected AIs, each with a folder, one of
  them the secretary. A second browser of the same person that was already
  connected shows the group and both AIs without a reload; a message
  posted in one appears in the other within 1 second on a direct path. A
  group created with no AI can be written in by its people.
- **A2.2** A browser disconnects; 20 messages are posted; on reconnect it
  receives exactly those 20 in order. With 1500 missed rows it is told to
  reset first, shows the newest 50, and scrolling back fetches older pages
  in creation order. A pending card older than those 50 is present after a
  cold start.
- **A2.3** A command sent twice with the same nonce produces one row and
  the same answer both times.
- **A2.4** A command from a device that is not a member's is refused.
- **A2.5** With retention set to 7 days and the clock advanced, older
  messages are gone after the daily run and newer ones remain; a card that
  is still pending remains however old. A client whose cursor is older
  than a removed deletion marker is reset.
- **A2.6** With the home stopped the browser shows "cannot reach the group
  home" and is read-only; when it returns, sync resumes without reload.
- **A2.7** A row written while a first read is being sent is neither
  missing at `synced` nor applied twice. A deleted row does not come back
  when an older `fetch` page arrives after the deletion. (The sentence on a
  move from a relayed to a direct path was removed with A1.3 and A1.13.)
- **A2.8** An AI is added with a folder that does not exist on its
  computer: the dialog says so, and the group has no such member.
- **A2.9** `host.join` with a folder that is the workspace of an AI of
  another group on that computer is answered `conflict` and names that
  group, whichever home that group has. Two such joins sent at once for
  one folder and two groups: one succeeds. After that AI has left its
  group, the join succeeds.
- **A2.10** In the folder list of the add dialog, a folder that an AI of
  this group already has says which AI. A folder that an AI of another
  group has cannot be chosen and says which group.

## M3. AI sessions

- **A3.1** Mentioning a Claude Code member gets an answer posted with
  `post`. A second mention shows it remembers the first. One CLI process
  existed throughout.
- **A3.2** The same with Codex.
- **A3.3** The same with the AI on a node other than the home; the AI's
  process runs in the folder it was added with.
- **A3.4** After the idle period (set to 1 minute) the process is gone and
  the card says dormant. A mention resumes it with its memory.
- **A3.5** With `max_open` 1 and two AIs, the first `waiting`: mentioning
  the second makes the first dormant first. With the first still `working`,
  the second's turn starts when the first's has ended.
- **A3.6** Killing the CLI mid-turn produces a `turn_interrupted` row; the
  next mention works.
- **A3.7** Text the model prints outside tools does not appear in the group.
- **A3.8** A repository hook that would create a marker file runs in task and review sessions, and does not run in group sessions, on each CLI. (`live`)
- **A3.9** A `post` from an AI that contains a test secret pattern fails
  with `secret`; no row for it exists on the home.
- **A3.10** An inbox item sent twice by the home produces one turn, also
  across a restart of the host.
- **A3.11** For a Claude Code and a Codex that were used in two folders
  each, the add dialog lists those folders, newest first, and neither a
  folder that has been deleted nor one under the node's data directory.
  (`live` for OpenCode and Qwen Code.)
- **A3.12** A folder typed by hand is accepted when it exists. Claude Code
  reads its session files and Codex queries `codex app-server` `thread/list`
  (`contracts/local.md` section 7), without reading those config or auth files
  directly: with `~/.claude.json`, `~/.codex/auth.json` and
  `~/.codex/config.toml` unreadable, the same folders are listed.
- **A3.13** With the folder of an AI removed after it joined, the next
  mention produces a `turn_interrupted` row that names the folder; with
  the folder back, the next mention works.

## M4. Permissions and approvals

These tests need a task session, so they follow the first slice of M5
(`start_task`, backend task B-16).

- **A4.1** At level 2 an AI that needs to install a package produces a card
  with the exact command. Approving from a second device lets it run. With
  the real CLIs (`live`): `--mcp-config` takes a JSON string on Claude Code,
  and a command that the sandbox blocks for want of the network becomes a
  card once the AI asks through its CLI (R-41 rests on this;
  `contracts/drivers.md` 1.1 says what changes if it fails), and on Claude
  Code `AskUserQuestion` reaches `approve` under each permission mode, and
  the `updatedInput.answers` reply is honoured.
- **A4.2** Denying tells the AI; the command does not run.
- **A4.3** With the wait set to 20 seconds and no answer, the turn ends and
  the task waits. The host is restarted. Approving afterwards makes the AI
  retry and the command runs without a second card; a different command
  produces a new card.
- **A4.4** At level 1 a request to edit a file changes nothing on disk and
  produces no card.
- **A4.5** Superseded by owner override 1 of 2026-11-21
  (`contracts/drivers.md` sections 1.1 and 2): even at level 3, CLI permission
  prompts and multiple-choice questions produce approval and decision cards
  (`features/host_approve.feature`).
- **A4.6** Removed under the owner's rule of 2026-11-21: no defence in code
  against an approval from a device outside the owner's log.
- **A4.7** Pausing the group interrupts a running turn within 2 seconds.
  Mentions made while paused are delivered after resume. A task that waits
  for an approval is not woken by the resume.
- **A4.8** "All my AIs offline" closes every session on that node; items
  stay on the home; "online" delivers them. It works with the home stopped.
- **A4.9** With `turn_minutes` 1 a long turn is interrupted with reason
  `timeout`.
- **A4.10** On each CLI at level 2, an AI asked to write a marker file
  outside its workspace produces a card, and no file exists before approval.
  (`live`)
- **A4.11** On the real Codex CLI with
  `-c features.default_mode_request_user_input=true`, an AI in Default mode
  asked to present a multiple-choice question with its ask-the-user tool
  emits `item/tool/requestUserInput`, produces a `decision` card, and
  receives the chosen option (run in B-17 / I-11; `contracts/drivers.md` 1.2
  says what changes if the flag does not work). (`live`)

## M5. Tasks

The tests of this milestone follow the owner's decision of 2026-11-26: a
task is work in the assignee's folder, a scored review, and done. The
sentences that tested checks, pushes and the merge are removed with that
behaviour; a number is not used again.

- **A5.1** Request, brief, confirm, two tasks reviewed by each other's AI,
  both done, two summary cards. The only human action is the confirm, and
  no approval card appears. Each task ran in its own session, in the
  folder of its assignee; after each task is done its sessions are gone,
  and the node's data directory holds no project folder.
- **A5.2** Before the confirm no `assign` item was created and no task
  session exists.
- **A5.3** Removed on 2026-11-26: the node runs no checks and merges
  nothing.
- **A5.4** A review of 86 against 90 returns the task to `work`. A veto
  gives 0. The sixth failed review makes the task `failed`. A review by
  the task's own assignee, or of a report other than the one under review,
  is refused.
- **A5.5** Three consecutive crashed turns with no submission in between
  make the task `failed` and mention the owner. The secretary cannot
  resume or reassign it; a person can.
- **A5.6** A task that depends on another starts only after that one is
  done; reassigning it while it is queued starts nothing.
- **A5.7** An owner's direct request that needs a change: the AI's group
  session lists `start_task` and no tool that submits; after `start_task`
  a task card exists, the secretary is notified, and the change is made in
  a task session in the same folder.
- **A5.8** Two AIs made to answer each other are paused at 10 rounds; a
  person's message in the thread resumes them, and so does `task resume`,
  and either way the held mentions go out.
- **A5.9**, **A5.10** Removed on 2026-11-26: no hard check reads a diff,
  and nothing is merged, so there is no conflict.
- **A5.11** "Jump the queue" on a direct request interrupts the running
  turn; the task shows as pre-empted meanwhile and continues afterwards by
  itself. A jump aimed at the session that is running interrupts it and
  goes first without pausing the task; the session then continues by itself
  with one turn of its own.
- **A5.12** When a review passes, the task is done with no AI turn in
  between, and the thread shows `task_done` under the secretary's name.
- **A5.13** Removed on 2026-11-26: the node pushes nothing.
- **A5.14** With retention set to 7 days and the clock advanced, notes and
  review messages remain. `forget` removes one, and a connected browser
  drops it within 1 second.
- **A5.15** `act confirm` sent twice with the same nonce creates its tasks
  once and is answered `ok` twice; `task fail` sent twice counts once.
- **A5.16** With `idle_notice_minutes` 1, an active task with no thread
  message produces one `notice` for the secretary.
- **A5.17** Removed on 2026-11-26: the node runs no check command.
- **A5.18** An AI that is assignee or reviewer of an unfinished task cannot
  be removed. (The sentences on the step `merge` were removed on
  2026-11-26.)
- **A5.19** A review item queued for one report is not delivered after the
  task was submitted again: the home has removed it, and a host that still
  holds it refuses it. A review session that is resumed after its node
  restarted still submits its review for the report it was given.
- **A5.20** Two tasks of one brief for the same AI: the second is `queued`
  with wait `busy` until the first is done, and then starts by itself.
  With three, the release starts one and leaves the other queued. The same
  with two AIs that were added with one folder on one computer. A second
  task for an AI in another folder starts at once. Giving a started task to
  an AI whose folder is busy with another task is refused with `conflict`.
  Giving it to an AI in another folder starts the task that was queued
  behind it. `start_task` while the AI's folder is busy answers `queued`,
  and the task's first turn, when it starts, holds its title and plan.
- **A5.21** The reviewer's turn names the assignee's computer and folder
  and says which of the three cases holds: the same folder, another folder
  on this computer, another computer.
- **A5.22** `submit_task` whose summary holds a test secret pattern fails
  with `secret`; the task is not submitted.
- **A5.23** A task that the secretary or an admin paused shows who paused
  it, in the reader's language. An approval card that ended because its
  task was given to another AI shows who gave it and to which AI. Both
  still show the name after that member has left the group.
- **A5.24** Two AIs share a folder. The first task's assignee is still
  running the turn in which it submitted when the review passes. The
  second task's first turn starts only after that assignee's process has
  exited.

## M6. AI calls AI

- **A6.1** On one machine, one AI asks another; the answer reaches the
  caller's open tool call while its turn is running; both messages are in
  the thread with the elapsed time.
- **A6.2** The same across two machines.
- **A6.3** A busy callee: the caller gets a receipt after the wait, and the
  answer later as a turn.
- **A6.4** A call at depth 5 is refused.

## M7. Terminal takeover

- **A7.1** "Open terminal" on a Claude Code member that has a task shows
  its native interface in its workspace; a typed prompt is answered;
  hand-back produces the AI's own account in the thread; a later mention in
  the task's thread shows the AI remembers the terminal exchange. (`live`)
- **A7.2** The same with Codex. (`live`)
- **A7.3** Mentions during takeover are held and delivered in order after
  hand-back.
- **A7.4** The fake CLIs log every start and exit with the session id.
  Across ten switches no start comes before the previous exit.
- **A7.5** On a relayed path the button is disabled and `takeover.start`
  returns `relay`.
- **A7.6** Closing the terminal in order (the button, or a tab close that
  the browser signals) hands back within 2 seconds. A connection that is
  simply lost hands back when the node's stack reports it. `takeover.start`
  sent again after the hand-back is answered `conflict`.
- **A7.7** Opening the terminal of a working AI that has no task opens its
  group session and creates no task card; hand-back posts the AI's account
  in the main line.
- **A7.8** In the secretary's terminal, a request to edit a file changes
  nothing on disk. (`live`)
- **A7.9** With `pty` open on a direct path, keep TURN reachable and make the
  same peer connection select a relayed pair. Verify the transition from its
  selected-pair statistics. The end that observes `relay` closes `pty`, the
  node hands back, and `host.info` still succeeds on the original `ctl`
  without a new offer.

## M8. Shipping

- **A8.1** On a clean machine of each system, pasting the onboarding prompt
  into an AI CLI installs the node, verifies the checksum, pairs, and the
  node is running again after a reboot.
- **A8.2** Postponed after 1.0 (no self-update). Self-update replaces version n with n + 1 when the sum matches,
  replaces nothing when it does not, and does not restart during a turn or
  a takeover.
- **A8.3** With the page closed and the owner's node still connected to the
  home, an approval request produces a browser notification that shows the
  group, the card type and a one-line title, and opens the card (desktop
  Chrome, Android Chrome, iOS after adding to the home screen). The pushed
  content, inspected in the test, holds no message text and no command.
- **A8.4** A rendezvous started with one command on a fresh server passes
  A1.1 to A1.3.
- **A8.5** Every test above passes on the pairs named under "Scope" in
  `steps/step-1.md`.
- **A8.6** Recorded, with the machine, the CLI versions and the workload
  named: node binary size, node idle memory, peak memory per open session
  of each CLI, rendezvous memory with 100 idle connections.
- **A8.7** A node that starts on the data directory of `v3.0.0-rc.1` keeps
  its pairing, its owner's devices, its account types and its peers,
  forgets the devices of everyone else who was in its groups, lists no
  group, and leaves the folder `projects/` as it was. `group import` of a
  file that rc.1 wrote is refused in one line that says why. A node of
  rc.1 that connects to it is answered `version`, and a node or browser of
  this version that reaches a home of rc.1 stops at `version` instead of
  retrying with an older one.
  The migration half (what stays and what is forgotten) is proved by
  `internal/store/forget_test.go`, which seeds a database shaped as rc.1 left
  it and runs the real migration; the migration opens no file but the
  database, which is why `projects/` is left as it was.
