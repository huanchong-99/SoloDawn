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

Step 1 is done when every acceptance test below passes on macOS, Windows and
Linux, with Chrome, Safari and Firefox at the design's baseline versions.

## How acceptance is tested

- **Unit tests** for pure logic: envelope, device log, command handling, task
  state machine, limits.
- **Integration tests** run real nodes and a real rendezvous in one test
  process or on one machine, with **fake CLIs**: two small test programs
  named `claude` and `codex`, first on PATH, that speak the structured
  protocols and follow a script (post this, ask for that permission, exit
  mid-turn). These tests cost no model usage and run in CI.
- **End-to-end tests** drive the web client in a browser against those
  nodes.
- **Live tests**, a short list marked `live`, use the real CLIs with the
  owner's sign-in. They run by hand before a milestone is called done.

An acceptance test names what is done and what must be observed. "Within n
seconds" is measured by the test.

## M0. Spikes

Throwaway code. Each spike answers its questions with observed facts, in the
pull request that adds it. A "no" answer changes the plan before code is
built on it.

| Spike | Questions |
| --- | --- |
| S-1 transport | Does a pion node connect with Chrome, Firefox and Safari through signalling only? Are 60 KiB messages accepted both ways? Does a browser reach the node's ICE-TCP port? Are `.local` candidates a problem? Do both sides read the selected pair's type? Does a second, relay-free connection opened while the first is relayed work, and can the session move without loss? |
| S-2 Claude Code driver | Do the flags of `contracts/local.md` 4.1 behave as marked: `--setting-sources user` keeps project hooks out; `--append-system-prompt` works with stream-json; which flags express levels 1 and 2? How long can an `approve` call stay open, with and without progress notifications? What happens to a tool call that runs 10 minutes? Which usage and rate-limit figures appear in events? Can a session be resumed from another working folder? |
| S-3 Codex driver | Does `-c mcp_servers.…` work on `app-server`? Exact parameter names for folder, sandbox, approval policy, instructions. Which approval request types exist? How long can one stay open? Tool timeout setting. Usage and rate-limit notifications. Memory of one process per session. |
| S-4 Claude Code takeover | Structured, then interactive in a pseudo-terminal in a trusted folder, then structured again, on one session id, history intact. |
| S-5 Windows | ConPTY with both CLIs. Unix socket. Interrupt without signals. What level 2 can mean for Claude Code without its sandbox. |
| S-6 one port | HTTPS and TURN over TLS on 443 chosen by server name; automatic certificates for two names. |
| S-7 browser keys | Ed25519 in WebCrypto with a non-extractable key kept in IndexedDB, on the three baseline browsers. |
| S-8 port mapping | UPnP and NAT-PMP mapping, and advertising the mapped address as an ICE candidate in pion. |

## M1. Identity and connectivity

Acceptance:

- **A1.1** A fresh browser and a fresh node pair through the onboarding
  prompt. The fingerprint on the page equals the one printed by
  `solodawn status`.
- **A1.2** A second node is added. The browser lists both with system,
  version and path.
- **A1.3** On one local network the path is `lan`. With UDP and the ICE-TCP
  port blocked the path is `relay` and the indicator says so within 1
  second. After unblocking, the path becomes `direct` within one probe
  interval plus 5 seconds. A request sent every second throughout is
  answered exactly once each.
- **A1.4** With the rendezvous stopped, open connections keep working for 10
  minutes. One of two nodes on the same network is restarted; they reconnect
  without the rendezvous.
- **A1.5** A test rendezvous that alters the description in transit causes
  the connection to be refused.
- **A1.6** Revoking a browser closes its connection within 1 second; its
  next attempt is refused.
- **A1.7** After the relay quota is used up the relay stops forwarding and
  `quota` reports it.
- **A1.8** Six targets build with cgo disabled. Binary size and idle memory
  of a node are recorded in the pull request (targets: under 30 MB on disk,
  under 40 MB resident).

## M2. Groups, messages, sync

- **A2.1** A group is created from a browser. A second browser of the same
  person sees it; a message posted in one appears in the other within 1
  second on a direct path.
- **A2.2** A browser disconnects; 20 messages are posted; on reconnect it
  receives exactly those 20 in order. With 1500 missed rows it resets, shows
  the newest 50, and scrolling back fetches older pages.
- **A2.3** A command sent twice with the same nonce produces one row.
- **A2.4** A command whose signer is not the connection's device is refused.
  A command from a device that is not a member's is refused.
- **A2.5** With retention set to 7 days and the clock advanced, older
  messages are gone after the daily run and newer ones remain.
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
- **A3.5** With `max_open` 1 and two AIs, mentioning the second makes the
  first dormant first.
- **A3.6** Killing the CLI mid-turn produces `turn_interrupted`; the next
  mention works.
- **A3.7** Text the model prints outside tools does not appear in the group.
- **A3.8** A repository hook that would create a marker file does not run by
  default, and runs after the owner enables repository configuration.
  (`live`)

## M4. Permissions and approvals

- **A4.1** At level 2 an AI that needs to push produces a card with the
  exact command. Approving from a second device lets it run. The audit shows
  the request and the decision with the deciding device.
- **A4.2** Denying tells the AI; the command does not run.
- **A4.3** With the wait set to 20 seconds and no answer, the turn ends and
  the task waits. Approving afterwards makes the AI retry and the command
  runs without a second card.
- **A4.4** At level 1 a request to edit a file changes nothing on disk and
  produces no card.
- **A4.5** At level 3 no card appears.
- **A4.6** An approval envelope from a device outside the owner's log,
  injected at the home by a test hook, is ignored by the host; the audit
  says `refused`.
- **A4.7** Pausing the group interrupts a running turn within 2 seconds.
  Mentions made while paused are delivered after resume.
- **A4.8** "All my AIs offline" closes every session on that node; mentions
  are held; "online" delivers them.
- **A4.9** With `turn_minutes` 1 a long turn is interrupted with reason
  `timeout`.

## M5. Tasks

Run once with a scratch GitHub repository and once with `forge: none`.

- **A5.1** Request, brief, confirm, two tasks reviewed by each other's AI,
  both merged, two summary cards. The only human action is the confirm
  (level 3).
- **A5.2** Before the confirm no worktree exists and no `assign` item was
  created.
- **A5.3** `submit_task` is refused after failing checks, accepted after
  passing ones, and refused again if a commit was added since.
- **A5.4** A review of 86 against 90 returns the task to `local_check` in
  round 2. A veto gives 0. A sixth failed round makes the task `failed`.
- **A5.5** Three consecutive `fail` reports make the task `failed` and
  mention the owner.
- **A5.6** A task that depends on another starts only after that one merged.
- **A5.7** An owner's direct request that changes code creates a task card
  and notifies the secretary.
- **A5.8** Two AIs made to answer each other are paused at 10 rounds; a
  person's message in the thread resumes them.
- **A5.9** A diff containing a test secret pattern fails the hard check.
- **A5.10** A merge conflict returns the task to `local_check` with the
  detail.
- **A5.11** "Jump the queue" on a direct request interrupts the running
  turn; the interrupted task continues afterwards by itself.

## M6. AI calls AI

- **A6.1** On one machine, one AI asks another; the answer is the tool
  result; both messages are in the thread with the elapsed time.
- **A6.2** The same across two machines.
- **A6.3** A busy callee: the caller gets a receipt after the wait, and the
  answer later as a turn.
- **A6.4** A call at depth 5 is refused.

## M7. Terminal takeover

- **A7.1** "Open terminal" on a Claude Code member shows its native
  interface; a typed prompt is answered; hand-back produces the summary
  with files and commits; a later mention shows the AI remembers the
  terminal exchange. (`live`)
- **A7.2** The same with Codex. (`live`)
- **A7.3** Mentions during takeover are held and delivered in order after
  hand-back.
- **A7.4** Sampling the process list every 100 ms across ten switches never
  shows two CLI processes for one session id.
- **A7.5** On a relayed path the button is disabled and `takeover.start`
  returns `relay`.
- **A7.6** Closing the browser tab hands back within 5 seconds.

## M8. Shipping

- **A8.1** On a clean machine of each system, pasting the onboarding prompt
  into an AI CLI installs the node, verifies the checksum, pairs, and the
  node is running again after a reboot.
- **A8.2** Self-update replaces version n with n + 1 when the sum matches,
  and replaces nothing when it does not.
- **A8.3** With the page closed, an approval request produces a browser
  notification that opens the card (desktop Chrome, Android Chrome, iOS
  after adding to the home screen).
- **A8.4** A rendezvous started with one command on a fresh server passes
  A1.1 to A1.3.
- **A8.5** Every test above passes on Windows and Linux.
- **A8.6** Recorded: node binary size, node idle memory, memory per open
  session of each CLI, rendezvous memory with 100 idle connections.

## Order and parallel work

```
M0 ─ M1 ─ M2 ─ M3 ─ M4 ─ M5 ─ M6
                └──── M7 (needs M3)
M8 after M5; Windows and Linux checks run from M1 on, not only at the end.
```

Backend and frontend work in parallel from the first day: the frontend
builds against the golden examples of `internal/proto/testdata` and a mock
transport until the matching backend milestone lands.
