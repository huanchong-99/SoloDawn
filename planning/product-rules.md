# Product rules (fixed)

These are decisions already made by the owner. They are restated here so that
implementers who cannot see the decision log have one place to read them.
They are not open for redesign; see `README.md` rule 1.

Where a rule cites `Q-n`, the coordinator settled an open point under the
owner's delegation; the question and the decision are in `risks.md`.

Tags: `[S1]` applies from step 1 (one person, several machines), `[S2]` from
step 2 (teams), `[S3]` from step 3 (extensions).

## Principles

- **R-1** `[S1]` The platform does not follow AI tool versions. It starts the
  vendors' official, unmodified CLI programs and talks to them through the
  interfaces those programs document. No vendor SDK is bundled, no ACP.
- **R-2** `[S1]` The platform contains no language model. It does only work
  that needs no judgement: moving messages, task state, timers, limits,
  permission checks. Anything that needs judgement is done by an AI member.
- **R-3** `[S1]` Least code. The same feature in fewer lines wins. No line
  budget is set.
- **R-4** `[S1]` Windows, macOS and Linux are all first-class. Resource use
  must be small.

## Groups and workspaces

By the owner's decision of 2026-11-26
([record](../docs/decisions/proposed/feature/2026-11-26-groups-workspaces.md)):

- **R-10** `[S1]` There is one kind of group: people and AIs. A group is
  created by choosing people and connected AIs, and people and AIs can be
  added later. A group needs no AI: one without is a group of people. No
  repository, branch, forge or check command is registered with a group.
- **R-11** `[S1]` Only a connected AI can be added: a CLI that a paired
  computer of its owner found and whose account type the owner declared
  (R-50), or an external agent (R-166). It joins with a **workspace**: a
  folder on its computer, which its owner chooses when adding it, from the
  folders that CLI was used in on that computer or by typing one. The AI
  works directly in that folder, and every member of the group sees its
  path. Nothing is cloned, and the folder need not be a Git repository. Two
  AIs of one group may have the same folder; a folder is the workspace of
  AIs of one group only.
- **R-12** `[S1]` An AI may be in several groups. Contexts never mix across
  groups. Session granularity (settled by the coordinator, Q-8): the
  secretary has one long session per group; a working AI has one session per
  task, whose working folder is the member's workspace and which ends when
  the task ends. What carries over between tasks is the ledger and the notes
  (R-102), not a session. A working AI also has one read-only group
  session per group, in the member's workspace, for what belongs to no task:
  questions, main-line mentions, calls from other tasks. To change code it
  starts a task, which gets its own session (Q-15). One folder takes one
  task at a time: a task whose assignee's folder is in use by another task
  waits its turn.
- **R-13** Removed on 2026-11-26: a task has no branch and no worktree of
  its own (R-11, R-90).
- **R-14** Removed on 2026-11-26: there is no team-wide kind of group and
  no relaying to one (R-10).

## The secretary

- **R-20** `[S1]` Each group has at most one secretary. It is an AI member
  kept alive by some node, with the secretary instructions loaded. Secretary
  contexts of different groups never mix.
- **R-21** `[S1]` The secretary coordinates and never writes code, in its
  terminal either: to change code the owner takes over a working AI (Q-20).
- **R-22** `[S1]` Workflow: take the request; ask only if unclear; post a
  task brief and wait for the requester to confirm; split and assign; AIs
  work and report; follow up on silence and reassign on failure; bring
  questions of direction, trade-off, money, permission or safety to a person;
  accept through scored review of work the AI tested itself; post a summary.
- **R-23** `[S1]` Nothing starts before the brief is confirmed.
- **R-24** `[S1]` When the review passes, the task is done, without a human
  click and without an AI turn; the platform shows it under the secretary's
  name. The platform merges nothing: the secretary arranges the closing step
  the project needs, such as a merge.

## Main line and threads

- **R-30** `[S1]` The main line carries only: requests, task briefs, task
  cards, decision requests, approval cards, summaries; and confirm cards
  `[S2]` (2026-11-20, for step 2: R-51).
- **R-31** `[S1]` Progress, AI-to-AI discussion and review remarks go in the
  thread of the task they belong to.
- **R-32** `[S1]` Message kinds in a thread: progress, question, call,
  completion report, review, approval request, decision request.
- **R-33** `[S1]` Mentioning a dormant AI wakes it; it reads context first.
- **R-34** `[S1]` A person is notified only when mentioned, when a decision or
  approval waits for them, or when a task completes. They can adjust this.

## Permissions and approvals

- **R-40** `[S1]` Three levels, set per project and per AI by the owner of the
  machine: (1) read only; (2) work inside the workspace, the default; (3)
  unrestricted. The owner may set level 3 for all projects at once. Level 2
  limits what the AI itself does.
- **R-41** `[S1]` At level 2 the AI changes files in the workspace without
  asking. What reaches further (a change outside the workspace, or the
  network: installing software, pushing code) is meant to happen only
  after the AI has asked through its CLI and the owner has approved the
  card. Level 2 does not promise to limit reading, and Codex reads
  anywhere at level 1 too. What each CLI was measured to confine, and what
  is not shown yet, is in `contracts/drivers.md` section 1 (Q-25). As settled:
  - the platform never inspects commands. Bulk deletion inside the
    workspace is left to the CLI's sandbox and to the project's own version
    control, not to a card (Q-11). A card shows the kind of request and the
    CLI's own one-line
    description, never a label derived from reading the command (Q-18);
  - secrets are guarded where they would leave the machine (R-137).
- **R-42** `[S1]` Approval flow: the AI stops and asks; an approval card
  appears in the main line under the secretary's name and mentions the
  machine owner; the platform writes the card itself, no AI turn is spent
  (Q-6); the owner approves or refuses from any of their devices; only the
  owner or someone the owner named may decide; while unanswered the task is
  suspended and the secretary may reassign it.
- **R-43** `[S1]` Approval requests come from the CLI's own permission
  interface. The platform never reads terminal output to detect them.
- **R-44** `[S2]` Work a colleague assigns to my AI runs at level 2 at most.
  Not in 1.0 (2026-11-22): every session runs at the highest level, with no
  level control; R-44 returns with level control.
- **R-45** `[S1]` By the owner's decision of 2026-11-22, AI configuration
  shipped inside a repository (plugins, hooks, project MCP servers, the
  project's own instruction file) is always loaded in task and review
  sessions and never loaded in group sessions (`contracts/drivers.md` 1.1,
  1.2; `risks.md` Q-24, K-25). Add only repositories you trust.

## Whose instructions an AI follows

- **R-50** `[S1]` Account type is declared by the owner when registering an
  AI: personal subscription, team plan, or API key. The platform does not
  verify it and never reads credentials.
- **R-51** `[S2]` Personal subscription: the AI obeys its owner only. When
  someone else mentions it, the platform mentions the owner; if the owner
  confirms, it is as if the owner had sent that message. There is no switch to
  auto-accept a given person.
- **R-52** `[S2]` After a task has started, remarks added by others are
  information, not orders. Changing direction or adding work needs the owner
  again.
- **R-53** `[S2]` Team plan or API key: the owner may list members who can
  direct the AI without per-request confirmation.
- **R-54** `[S3]` Google models are supported through API keys only.
- **R-55** `[S1]` The platform does no region blocking.
- **R-56** `[S1]` If a CLI's sign-in stops working in background mode, the
  node tells the owner. It never silently switches to pay-as-you-go.

## Runaway limits (defaults, adjustable by group admin and owner)

- **R-60** `[S1]` AI-to-AI exchanges in one thread pause after 10 rounds with
  no human message; the secretary alerts the person responsible. Each AI has
  an hourly wake-up cap.
- **R-61** `[S1]` Usage is shown per AI. The owner may cap usage per day and
  per task; reaching a cap stops the AI until the owner decides. When a
  subscription is rate-limited the AI pauses, the secretary says when it
  resumes and may reassign.
- **R-62** `[S1]` One run lasts at most 60 minutes, then stops and reports.
- **R-63** `[S1]` Three consecutive failures on a task stop it and alert the
  owner. Review may send work back at most five times, then a person decides.
- **R-64** `[S1]` Emergency stop: a privileged member can pause every AI in a
  group; a machine owner can take all of their AIs offline with one action,
  and only they can.

## Bypassing the secretary

- **R-70** `[S1]` An owner may mention their own AI directly for questions
  that change no code: no secretary, no task card.
- **R-71** `[S1]` Direct work that changes code is allowed; a task card is
  created automatically and the secretary is told.
- **R-72** `[S1]` If the AI is busy with a secretary-assigned task, the owner
  is asked: queue (default) or jump the queue (the current task pauses).

## Terminal takeover

- **R-80** `[S1]` Every AI card has "open terminal": the AI's native CLI,
  with typing, slash commands and plugins.
- **R-81** `[S1]` Only the owner may take over. Others may watch with the
  owner's consent `[S2]`.
- **R-82** `[S1]` During takeover the card says so, and incoming messages
  queue until hand-back.
- **R-83** `[S1]` A takeover runs in the folder of the session that is taken
  over, the member's workspace. What is changed for a task there goes through
  the same review. A takeover of a working AI that has no unfinished task
  opens its group session and creates no task.
- **R-84** `[S1]` On hand-back the AI posts its own account of what was
  asked and what was done: in the thread of the task, or in the main line
  for a session without one. The node reads no facts from the folder.
- **R-85** `[S1]` The platform never reads terminal content and never bases a
  decision on it.

## Code flow

- **R-90** `[S1]` The AI brings its folder up to date, tests and publishes
  its work the way the project does; then a different AI reviews and scores
  it. A project without rules of its own for publishing gets these: the AI
  commits its work, pushes nothing to a branch that others use before the
  review has passed, and pushes to a branch of its own when its reviewer
  works on another computer. The platform creates no branch, runs no check,
  opens no pull request and merges nothing; its instructions tell each AI
  these duties, and no code enforces them (`contracts/drivers.md` section 4).
- **R-91**, **R-92** Removed on 2026-11-26 with the flow they varied: no
  forge is configured and none is needed.

## Quality

- **R-100** `[S1]` Requirements are clarified before work.
- **R-101** `[S1]` Acceptance criteria are generated per project and scored:
  pass at 90, foundation tasks at 70, a veto item scores zero.
- **R-102** `[S1]` A ledger of scored points and a set of context notes form
  the group's long-term memory. They are exempt from the retention period of
  R-135 and stay until a person deletes them (Q-13), which is done in the
  group's own panel under "Project memory" (Q-17).
- **R-103** `[S1]` A group has no check commands, and the node runs no hard
  check on a diff: the AI tests the way the project does (R-90). A fake test
  or a missing test is a veto in review. No built-in rule engine.

## AI calling AI

- **R-110** `[S1]` A tool lets an AI ask another AI mid-task and continue with
  the answer. Same interface on one machine or across machines.
- **R-111** `[S1]` Only text travels in messages. Code travels the way the
  project shares it.
- **R-112** `[S1]` Small requests wait for the answer; large ones return a
  receipt and notify on completion.
- **R-113** `[S1]` Every call is a visible thread message: the caller
  mentions the callee, the callee replies in the thread.
- **R-114** `[S1]` Calls count toward the 10-round limit. Nesting depth is at
  most 4.
- **R-115** `[S2]` Authority carries over: my own AIs freely; a colleague's
  personal-subscription AI needs that colleague; team and API-key AIs follow
  their lists.
- **R-165** `[S3]` The node lists the models and thinking levels each
  installed CLI offers, by the CLI's own listing, again whenever the
  owner asks. The owner chooses model and level for each AI member, may
  add several members from one CLI, and gives each a profile: how strong
  it is and what it is good at. The node suggests the strength from the
  model's name; what the owner set is never overwritten. The secretary is shown every AI's
  profile and state and chooses assignee and reviewer by them: hard work
  to strong AIs, simple work to light ones, independent tasks spread over
  different AIs, none to an AI that is limited or offline. An assignee
  whose task fails twice in a row in the same way (review) must ask a
  stronger AI and get an answer before it submits
  again; with no stronger AI in the group nothing changes. Contracts:
  `contracts/step-3/profiles.md`, `contracts/step-3/drivers.md` section
  10.
- **R-166** `[S3]` Any agent that speaks MCP joins a group as an external
  AI, without a driver (owner, 2026-11-19 and 2026-11-23). The owner adds
  it in the browser and pastes one setup prompt into the agent. The agent
  adds the node's MCP server to its own configuration and writes its own
  runner, which waits for items with a command of the node and starts the
  agent once per item. Waiting never keeps an AI session running. The node
  names the folder to work in; it neither starts, stops nor sandboxes the
  agent, and the agent reports its own quota, which the secretary sees
  with every AI's when it assigns work; there are no caps. Terminal
  takeover, permission levels, approval routing by the CLI's own channel,
  usage read from events and the owner's choice of model belong to the
  four CLIs that have a driver; no driver is built for a further CLI.
  Contract: `contracts/step-3/external.md`.

## Sessions

- **R-120** `[S1]` A session is started once and kept. It is not cold-started
  per message.
- **R-121** `[S1]` States: working, waiting, dormant. After an idle period
  (default 15 minutes) the node records the session id and closes the
  process; a new message resumes it by id.
- **R-122** `[S1]` Each machine has a cap on concurrently open sessions; over
  the cap, the least recently used goes dormant.
- **R-123** `[S1]` If a session cannot be resumed, a new one takes over from
  the task card and the notes.
- **R-124** `[S1]` Driving is hybrid. Automatic work uses the CLI's structured
  mode. Takeover uses the interactive CLI in a pseudo-terminal, bytes only.
  One session has exactly one writer at any time.

## Data, network, security

- **R-130** `[S1]` The official server stores nothing. It serves the website
  and the rendezvous service.
- **R-131** `[S1]` Several paths are tried at once (UDP hole punching, TCP,
  IPv6); the first to work is used; ICE may move to a better one by itself (R-132).
  In step 1, TCP means a direct connection to a port the other node has
  open or forwarded on its router (no UPnP or NAT-PMP mapping: cut
  2026-11-21), plus the relay on port 443. TCP simultaneous open between two nodes that are both behind NAT
  is an experiment (spike S-9) and is built in step 3 only if it shows a
  clear benefit (Q-1).
- **R-132** `[S1]` Relay is a bridge and a last resort. A relayed connection
  moves to a direct pair only when ICE finds one or at its next reconnect;
  no separate attempt is made (cut 2026-11-21). Relay carries small data
  only (chat, tasks); terminal streams are direct only. Relay content is
  end-to-end encrypted. The user is told when relayed.
- **R-133** `[S1]` The system must work without the server: existing direct
  connections continue, and self-hosters run the same programs. Local-network
  discovery is not built (cut 2026-11-21), nor a node's remembered addresses
  for its own signalling (PR-2 cancelled 2026-11-22); a browser keeps the
  `addrs` of each node's `welcome` and signals on them as well.
- **R-134** `[S1]` Each group has a home: one machine holding its history,
  member list and task state. Others sync from it and connect to it. Default
  is the secretary's machine. The user is told plainly that this machine must
  stay on. A user's own machine forwarding for its own group is not "relay".
- **R-135** `[S1]` All data stays on users' machines. Default retention is
  three months, rolling daily; adjustable. The ledger and the notes are
  exempt (R-102). Moving a group to another home, and copies for
  safekeeping, come in step 2 (Q-12).
- **R-136** `[S1]` A node acts only on instructions signed by its owner, and
  on the items of the homes of its groups. From step 2 a home may belong to
  another member, whom the owner trusts by joining the group (2026-11-22: no
  defence in code against a dishonest home).
- **R-137** `[S1]` Secrets never leave the machine. The node reads no AI
  credential, and project secret files are never posted to a group. An
  outbound check refuses what an AI sends to the group when it matches
  common secret formats or looks like an environment file (Q-11). The node
  runs no `git`, no `gh` and no command of a project or a forge.
- **R-138** `[S1]` Every computer and every browser has its own identity key.
  A lost device can be revoked from another device.
- **R-139** `[S1]` Each node keeps a local audit record of who made its AIs
  do what and what was approved; it follows retention. No screen or request
  reads it in version 1.
- **R-140** `[S2]` An invitation is a one-time link, valid 10 minutes, that
  admits one person to one group. (2026-11-22: no fingerprint and no check
  against a rendezvous that stands in for a home; the [deployment guide](../docs/cookbook/self-hosting.md) says
  to send the link privately and to use a trusted rendezvous.) A person who
  is already in another group on the same home is added by an admin without
  a link and without a step of their own (2026-11-26).

## Distribution

- **R-150** `[S1]` Apache-2.0. The name stays SoloDawn.
- **R-151** `[S1]` No installers, no code signing. Release binaries for
  Windows, macOS and Linux with SHA-256 sums, verified on install.
  Self-update comes after 1.0; until then a user updates by hand
  (`install.md`).
- **R-152** `[S1]` Onboarding: the website shows a prompt; the user gives it
  to their AI; the AI installs the node and pairs it. The pairing code in
  the prompt has 16 characters (Q-2).
- **R-153** `[S1]` Self-hosting is documented: the rendezvous service is one
  program file ([self-hosting](../docs/cookbook/self-hosting.md)).

## Interface

- **R-160** `[S1]` Website first; no phone app in the first version. One
  responsive web client; it can be added to a phone's home screen.
- **R-161** `[S1]` Pages: onboarding, group list, group chat (main line and
  threads), task and approval cards, AI member cards (state, usage, open
  terminal), my computers and AIs, settings.
- **R-162** `[S1]` Chinese and English. Dark and light themes.
- **R-163** `[S1]` Notifications use the browser's permission prompt; the
  user chooses. Reminders while the page is closed go through the browser
  vendor's push channel, encrypted end to end for the browser, carrying a
  short summary only: group name, card type, a one-line title. Never
  message text, never code (Q-5, Q-14, Q-17).
- **R-164** `[S1]` Supported browsers are those with native Ed25519 in
  WebCrypto; older ones see an upgrade notice. No signature library is
  bundled (Q-3).

## Not rebuilt

Per-CLI adapters for eight tools, provider switching, chat-app connectors,
container and split deployment, manual workflow graphs, slash-command
presets, system tray, telemetry.
