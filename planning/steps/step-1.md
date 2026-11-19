# Step 1: one person, several machines

Task lists: `steps/step-1-backend.md`, `steps/step-1-frontend.md`.

## Scope

In: every rule tagged `[S1]` in `product-rules.md`. One person; any number of
their computers and browsers; Claude Code and Codex; project groups with a
secretary; tasks from brief to merge with GitHub or with no remote;
approvals; AI calling AI; terminal takeover; direct, relayed and
server-less connectivity; the seven pages of the design; Chinese and
English; dark and light.

Out (later steps): more than one person in a group, invitations, confirming
other people's requests, watching a terminal, the team-wide group, forges
other than GitHub, CLIs other than Claude Code and Codex, phone app.

Step 1 is done when every acceptance test below passes on these pairs, at
the browser versions that spike S-7 fixes (C-9): the node on macOS, Windows
and Linux; the web client in Safari, Chrome and Firefox on macOS, Chrome and
Firefox on Windows and Linux, Safari on iOS, Chrome on Android.

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

## M0. Spikes

Throwaway code. Each spike is a pull request whose description answers its
questions with observed facts. Spike code is never merged into `rebuild`;
the planning lead records the answers in the plan. A "no" answer changes
the plan before code is built on it.

| Spike | Questions |
| --- | --- |
| S-1 transport | Does a pion node connect with each supported browser through signalling only? Are 60 KiB messages accepted both ways? Does a browser reach the node's ICE-TCP port? Are `.local` candidates a problem? Do both sides read the selected pair's type? Does a relay-free second connection, opened while the first is relayed, work? Does pion's opt-in renomination move a live relayed connection to a direct pair by itself, with each browser? How large is a signed description with all candidates, and how long does gathering take: can complete descriptions replace trickling? Do two nodes that are each a client of the other keep two working connections? |
| S-2 Claude Code driver | Do the flags of `contracts/drivers.md` 1.1 behave as marked: `--setting-sources user` keeps project hooks out; `--append-system-prompt` works with stream-json; what are the effective file and network limits at levels 1 and 2? How long can an `approve` call stay open, with and without progress notifications? What happens to a tool call that runs 10 minutes? Is the input of a retried request byte-identical? Which usage and rate-limit figures appear in events? Can a session be resumed from another working folder? Does the MCP session token still identify the caller after a resume? |
| S-3 Codex driver | Does `-c mcp_servers.…` work on `app-server`? Exact thread parameters for folder, sandbox, approval policy, instructions. Which approval request types exist? How long can one stay open? Is a retried request's input identical? Tool timeout setting. Usage and rate-limit notifications. Memory of one process per session. |
| S-4 takeover | Claude Code: structured, then interactive in a pseudo-terminal in a trusted folder, then structured again, on one session id, history intact. Both CLIs: the flags that give the interactive CLI the same level, setting sources and MCP configuration; can a brand-new session be started directly in interactive mode with an id the node chose? |
| S-5 Windows | ConPTY with both CLIs. Unix socket. Interrupt without signals. What level 2 can mean for Claude Code without its sandbox. |
| S-6 one port, relay | HTTPS and TURN over TLS on 443 chosen by server name, with real `turns:` clients; automatic certificates for two names. Credentials with the device id in the username; renewal. Cutting off an allocation that is already open when the byte quota is used. |
| S-7 browser keys | The lowest version of each supported browser in which Ed25519 in WebCrypto works with a non-extractable key kept in IndexedDB. Those versions, or the design's baseline where it is higher, become the supported baseline. |
| S-8 port mapping | UPnP and NAT-PMP mapping on a real router, and advertising the mapped address and port as an ICE candidate in pion. |
| S-9 TCP simultaneous open | Between two nodes behind different NATs with UDP blocked and no mapped port: does it succeed, on which router types, and how much code outside pion does it take? This decides whether step 3 builds it. Nothing in step 1 depends on the answer. |

## M1. Identity and connectivity

- **A1.1** A fresh browser and a fresh node pair through the onboarding
  prompt. The fingerprint on the page equals the one printed by
  `solodawn status`. Before the log arrives, the connection accepts nothing
  but `hello` and `devlog`.
- **A1.2** A second node is added. The browser lists both with system,
  version and path.
- **A1.3** On one local network the path is `lan`. With UDP and the ICE-TCP
  port blocked the path is `relay` and the indicator says so within 1
  second. After unblocking, the path becomes `direct` within one probe
  interval plus 5 seconds. Throughout, a command is sent every second and a
  row is written every second: every command is answered once, and every
  row is applied once.
- **A1.4** With the rendezvous stopped, open **direct** connections keep
  working for 10 minutes. One of two nodes on the same network is
  restarted; they reconnect without the rendezvous.
- **A1.5** A test rendezvous that alters the description in transit causes
  the connection to be refused.
- **A1.6** Revoking a browser closes its connection within 1 second; its
  next attempt is refused.
- **A1.7** When the relay quota is used up, an allocation that is already
  open stops forwarding, and `quota` reports it.
- **A1.8** Six targets build with cgo disabled. Binary size and idle memory
  of a node are recorded in the pull request with the machine named
  (targets: under 30 MB on disk, under 40 MB resident).

## M2. Groups, messages, sync

- **A2.1** A group is created from a browser. A second browser of the same
  person that was already connected shows it without a reload; a message
  posted in one appears in the other within 1 second on a direct path.
- **A2.2** A browser disconnects; 20 messages are posted; on reconnect it
  receives exactly those 20 in order. With 1500 missed rows it is told to
  reset first, shows the newest 50, and scrolling back fetches older pages
  in creation order. A pending card older than those 50 is present after a
  cold start.
- **A2.3** A command sent twice with the same nonce produces one row and
  the same answer both times.
- **A2.4** A command whose signer is not the connection's device is refused.
  A command from a device that is not a member's is refused.
- **A2.5** With retention set to 7 days and the clock advanced, older
  messages are gone after the daily run and newer ones remain. A client
  whose cursor is older than a removed deletion marker is reset.
- **A2.6** With the home stopped the browser shows "cannot reach the group
  home" and is read-only; when it returns, sync resumes without reload.

## M3. AI sessions

- **A3.1** Mentioning a Claude Code member gets an answer posted with
  `post`. A second mention shows it remembers the first. One CLI process
  existed throughout.
- **A3.2** The same with Codex.
- **A3.3** The same with the AI on a node other than the home.
- **A3.4** After the idle period (set to 1 minute) the process is gone and
  the card says dormant. A mention resumes it with its memory.
- **A3.5** With `max_open` 1 and two AIs, the first `waiting`: mentioning
  the second makes the first dormant first. With the first still `working`,
  the second's turn starts when the first's has ended.
- **A3.6** Killing the CLI mid-turn produces a `turn_interrupted` row; the
  next mention works.
- **A3.7** Text the model prints outside tools does not appear in the group.
- **A3.8** A repository hook that would create a marker file does not run by
  default, and runs after the owner enables repository configuration, on
  each CLI. (`live`)
- **A3.9** A `post` from an AI that contains a test secret pattern fails
  with `secret`; no row for it exists on the home.
- **A3.10** An inbox item that is not signed by the group's home, injected
  by a test hook, is refused by the host; the audit says `refused`. An item
  sent twice by the home produces one turn, also across a restart of the
  host.

## M4. Permissions and approvals

These tests need a task session, so they follow the first slice of M5
(`start_task`, backend task B-16).

- **A4.1** At level 2 an AI that needs to install a package produces a card
  with the exact command. Approving from a second device lets it run. The
  audit shows the request and the decision with the deciding device.
- **A4.2** Denying tells the AI; the command does not run.
- **A4.3** With the wait set to 20 seconds and no answer, the turn ends and
  the task waits. The host is restarted. Approving afterwards makes the AI
  retry and the command runs without a second card; a different command
  produces a new card.
- **A4.4** At level 1 a request to edit a file changes nothing on disk and
  produces no card.
- **A4.5** At level 3 no card appears.
- **A4.6** An approval envelope from a device outside the owner's log,
  injected at the home by a test hook, is ignored by the host; the audit
  says `refused`.
- **A4.7** Pausing the group interrupts a running turn within 2 seconds.
  Mentions made while paused are delivered after resume. A task that waits
  for an approval is not woken by the resume.
- **A4.8** "All my AIs offline" closes every session on that node; items
  stay on the home; "online" delivers them. It works with the home stopped.
- **A4.9** With `turn_minutes` 1 a long turn is interrupted with reason
  `timeout`.
- **A4.10** On each CLI at level 2, an AI asked to write a marker file
  outside its worktree produces a card, and no file exists before approval.
  (`live`)

## M5. Tasks

Run once with a scratch GitHub repository and once with `forge: none`.

- **A5.1** Request, brief, confirm, two tasks reviewed by each other's AI,
  both merged, two summary cards. The only human action is the confirm, at
  level 2, and no approval card appears. Each task ran in its own session
  in its own worktree; after the merge those sessions and folders are gone.
- **A5.2** Before the confirm no worktree exists and no `assign` item was
  created.
- **A5.3** `run_checks` refuses a worktree with uncommitted changes.
  `submit_task` is refused after failing checks, accepted after passing
  ones, and refused again if a commit was added since. A commit pushed to
  the branch after the review passed is not merged: the task reports a
  conflict.
- **A5.4** A review of 86 against 90 returns the task to `local_check`. A
  veto gives 0. The sixth failed review makes the task `failed`. A review
  by the task's own assignee, or of a commit other than the submitted one,
  is refused.
- **A5.5** Three consecutive failures (crashed turns, failed hosted checks)
  with no passing hosted check in between make the task `failed` and
  mention the owner.
- **A5.6** A task that depends on another starts only after that one merged.
- **A5.7** An owner's direct request that needs a code change: the AI's
  group session cannot modify a file; after `start_task` a task card exists,
  the secretary is notified, and the change is made in a new task session.
- **A5.8** Two AIs made to answer each other are paused at 10 rounds; a
  person's message in the thread resumes them.
- **A5.9** A diff containing a test secret pattern fails the hard check.
- **A5.10** A merge conflict returns the task to `local_check` with wait
  `conflict` and notifies the secretary. After the secretary names an AI,
  the task goes through checks and review again and merges.
- **A5.11** "Jump the queue" on a direct request interrupts the running
  turn; the task shows as pre-empted meanwhile and continues afterwards by
  itself.
- **A5.12** When a review passes, the merge follows with no AI turn in
  between, and the thread shows it under the secretary's name.
- **A5.13** At level 2, `push_branch` pushes the task's branch and no card
  appears. A `git push` of another branch that the AI runs itself produces
  a card. (With a remote.)
- **A5.14** With retention set to 7 days and the clock advanced, notes and
  review messages remain. `forget` removes one, and a connected browser
  drops it within 1 second.
- **A5.15** `act confirm` sent twice with the same nonce creates its tasks
  once and is answered `ok` twice; `task fail` sent twice counts once.
- **A5.16** With `idle_notice_minutes` 1, an active task with no thread
  message produces one `notice` for the secretary.

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
  its native interface in that task's worktree; a typed prompt is answered;
  hand-back produces the summary with files and commits; a later mention in
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
  simply lost hands back when the node's stack reports it.
- **A7.7** Opening the terminal of a working AI that has no task creates a
  task card. Handing back with nothing changed cancels it. Handing back
  after a commit keeps it, and the summary lists the commit.
- **A7.8** In the secretary's terminal, a request to edit a file changes
  nothing on disk. (`live`)

## M8. Shipping

- **A8.1** On a clean machine of each system, pasting the onboarding prompt
  into an AI CLI installs the node, verifies the checksum, pairs, and the
  node is running again after a reboot.
- **A8.2** Self-update replaces version n with n + 1 when the sum matches,
  replaces nothing when it does not, and does not restart during a turn or
  a takeover.
- **A8.3** With the page closed and the owner's node still connected to the
  home, an approval request produces a browser notification that shows the
  group, the card type and a one-line title, and opens the card (desktop
  Chrome, Android Chrome, iOS after adding to the home screen). The pushed
  content, inspected in the test, holds no message text and no command.
- **A8.4** A rendezvous started with one command on a fresh server passes
  A1.1 to A1.3.
- **A8.5** Every test above passes on the pairs named under "Scope".
- **A8.6** Recorded, with the machine, the CLI versions and the workload
  named: node binary size, node idle memory, peak memory per open session
  of each CLI, rendezvous memory with 100 idle connections.

## Order and parallel work

```
M0 ─ M1 ─ M2 ─ M3 ─ (first slice of M5) ─ M4 ─ M5 ─ M6
                                           └──── M7
M8 after M5; Windows and Linux checks run from M1 on, not only at the end.
```

Backend and frontend work in parallel from the first day: the frontend
builds against the golden examples of `internal/proto/testdata` and a mock
transport until the matching backend milestone lands. The golden examples
are frozen slice by slice, not all at once (`steps/step-1-backend.md`).
