# Steps 2 and 3: acceptance tests

The acceptance tests of step 2 (A9.x) and step 3 (A10.x), one group per
feature of `steps/step-2-3.md`. They rest on the design answers D-1 to D-19 of
`steps/parallel-plan-step-2-3.md` section 1; each heading names the answers
and the contract of its group. Exact shapes, names and error codes are in
those contracts; where they differ from this file, the contract holds.

## How acceptance is tested

The four kinds of test of `steps/step-1-acceptance.md` apply unchanged, and
"within n seconds" is measured by the test. Added for these steps:

- **The cast.** Two persons, P and Q, and three nodes. P owns `P1`, the home
  of the group, and `P2`; Q owns `Q1`. Each has one browser, or two where a
  test says so. P is the group's `owner`, Q a `member`. The secretary runs on
  `P1`, a working AI of P on `P2`, and two working AIs of Q on `Q1`:
  **Q-personal** (`account: personal`) and **Q-team** (`account: team`). All
  are fake CLIs that log each start with its level and each turn's text.
- **The test hook on a home** does what a home run by someone else could do:
  queue an item, change a row, withhold a signature.
- **Another forge** is a test program, first on PATH, that records its
  argument list and answers from files.
- A sentence that starts "In the browser" is checked end to end in the web
  client; everything else by integration.

## Step 2: teams (A9)

### People (D-7; `contracts/step-2/people.md`)

- **A9.1** P creates an invitation. The link holds the group id, the device id
  and fingerprint of `P1`, and a one-time secret. Q's browser, which has never
  met `P1`, shows that fingerprint, equal to the one `solodawn status` prints
  on `P1`, before it joins. Afterwards a member row for Q has role `member`,
  Q's device log is on the home as `device` rows, and both browsers show Q and
  the group's history without a reload.
- **A9.2** A test rendezvous that puts its own device into the room in place
  of `P1` gets no member: Q's browser refuses a device whose id is not the one
  in the link. A room message whose `mac` was made for a pairing role, not for
  `join`, is ignored. So is a joiner whose device log does not replay. The
  link is refused when used a second time and when its validity has ended.
  `invite` by a `member` is answered `forbidden`.
- **A9.3** P makes Q `admin`: `group set` and `invite` by Q succeed. Back to
  `member`, both are answered `forbidden`. A second `owner`, a change of the
  owner's role, and `member set` with `role` by anyone but P are refused.
- **A9.4** P removes Q. Q's connections to `P1` close within 1 second and the
  next attempt is refused; Q's browser no longer lists the group; the rows of
  Q and of Q's AIs are `gone` markers. While an AI of Q is assignee or
  reviewer of an unfinished task the removal is answered `conflict`. The owner
  cannot be removed. In the browser: roles, invitation, removal.

### Other people's hosts (D-2, D-4; `contracts/step-2/authority.md`)

- **A9.5** `Q1` joins Q-team to the group with `host.join`: the member row has
  `host` = `Q1` and `user` = Q, and `Q1` holds a clone. `member add_ai` from a
  node whose owner is not a member is refused, and so is `secretary: true`
  from Q while the group has a secretary.
- **A9.6** What a node accepts (`contracts/datachannel.md` section 8). To a
  device of Q, `P1` answers the home surface of this group only: a host
  command, `host.info`, `audit`, and `sync` of another group homed there are
  refused. `Q1` accepts no host command and no `takeover.start` from a device
  of P. When Q revokes a browser from another device, `P1` closes that
  browser's connection within 1 second of holding the entry.
- **A9.7** The fork vectors of `crypto.json` give the same log in Go and in
  the web client, in both orders of arrival: of two entries for one version,
  the one signed by the device earlier in the log stays; the other is dropped,
  with what no longer fits after it. End to end: Q's first device and a
  second, stolen one revoke each other at one version. Whichever entry `P1`,
  `Q1` and each browser saw first, all end with the second device revoked and
  the first valid, and `P1` closes the second's connections within 1 second.

### Whose instructions (D-2, D-8; `contracts/step-2/confirm.md`)

- **A9.8** P mentions Q-personal in the main line. No item is made; a
  `confirm` card that mentions Q appears. Q confirms from either of two
  devices; only then does the item reach `Q1`, with Q's signed `act confirm`
  as its `cause`, and the fake CLI log a turn. A mention by Q makes no card.
- **A9.9** Q refuses the card, or leaves it: no item exists, no turn starts,
  and P sees its state. `act confirm` by P is answered `forbidden`; sent twice
  by Q with one nonce it makes one item and is answered `ok` twice. An `allow`
  list for Q-personal that names P does not remove the card.
- **A9.10** The test hook queues on `P1`, validly signed by the home, items
  for Q-personal: one with no `cause` signed by Q, one whose `cause` confirms
  another message, a `reply` to no open call, a `notice`, and an `assign`
  whose brief names another AI. `Q1` refuses each, with audit `refused`. When
  `P1` answers `version` to `Q1`'s `hello`, `Q1` sends none with `v` 1.
- **A9.11** P confirms a brief that makes Q-personal assignee of one task and
  reviewer of another. Both wait for Q, the second from step `review`; until Q
  confirms, no `assign` or `review` item reaches `Q1` and no worktree exists
  there. In the browser: the card, its actions for Q only, each end state.

### After a task started (D-9; `contracts/step-2/confirm.md`)

- **A9.12** Q-personal has a task that P's brief and Q's confirmation started.
  P mentions the AI in its thread: no card appears, and the turn's header
  reads `information`, also when P's text says "your owner instructs you". Q
  posts there: the header reads `instruction`.
- **A9.13** P confirms a brief that replaces the first and changes the task. A
  new `confirm` card for Q appears; until Q confirms it, no turn labelled
  `instruction` from that brief reaches Q-personal.

### Allow lists (D-2; `contracts/step-2/authority.md`)

- **A9.14** Q puts P on the `allow` list of Q-team. A mention by P is
  answered, and a brief confirmed by P starts its task on `Q1`, both with no
  card. After Q removes P, a mention by P starts no turn unless Q confirms it.
- **A9.15** The host checks the list its owner signed, not the row. With P put
  into the row's `allow` by the test hook, an item caused by P is refused by
  `Q1` with audit `refused`; with the signed list naming P it is acted on. In
  the browser: Q edits the list and delegates of each AI; P only sees them.

### Level cap (D-1; `contracts/step-2/authority.md`)

- **A9.16** Q sets level 3 for Q-team in the group. A task from a brief that P
  confirmed starts at level 2: the fake CLI's log says so, and a request to
  write outside the worktree produces a card. A task from a brief that Q
  confirmed starts at level 3. With Q's level at 1, P's task starts there too.
  `all_level3` on `Q1` does not lift the cap.
- **A9.17** The owner's confirmation does not make the owner the cause: with
  Q's level at 3, a task of Q-personal from a brief that P confirmed starts at
  level 2 after Q confirmed the card. A direct request: `start_task` by Q-team
  after a mention by P gives level 2, after one by Q level 3.

### Calls across owners (D-2, D-8; `contracts/step-2/confirm.md`)

- **A9.18** P's working AI calls Q-personal with `ask`: a `confirm` card for Q
  appears, a receipt comes after the wait, and the answer as a turn once Q
  confirms; denied, a turn says so. A call to Q-team, which lists P, is
  answered in the open tool call with no card, as is one between Q's AIs.

### Approver delegates (D-13; `contracts/step-2/authority.md`)

- **A9.19** Q names P as a delegate for Q-team in the list Q signs. An
  approval card of Q-team mentions Q and P. P approves, the command runs, and
  the audit on `Q1` names P's device as the one that decided. After Q removes
  P, `act approve` by P is answered `forbidden`, and the same envelope
  injected at the home is ignored by `Q1` with audit `refused`. A delegate for
  Q-team decides nothing for Q-personal.

### Watching (D-14; `contracts/step-2/watch-team.md`)

- **A9.20** P asks to watch Q-team's terminal. A `confirm` card with `what:
  watch` for Q appears; until Q confirms, `Q1` opens no `watch` channel for P.
  Then, while Q has the terminal open, P's browser gets the bytes of Q's `pty`
  channel, in order. Bytes and a `pty.resize` from P reach no pseudo-terminal:
  the fake CLI logs none. Hand-back closes `watch` within 2 seconds.
- **A9.21** On a relayed path between P's browser and `Q1` the button is
  disabled and the channel is refused with `relay`. When an open watch moves
  to a relayed pair, as in A7.9, the channel closes and `ctl` keeps answering.
  In the browser: P's terminal takes no input, and Q sees that P is watching.

### Team-wide group (D-15; `contracts/step-2/watch-team.md`)

- **A9.22** `group create` with kind `team` makes a group without `repo`. In
  it `task start`, `post` of kind `brief`, `host.join`, `member add_ai` and
  `group set` with `checks` are refused, and no node holds a clone for it.
- **A9.23** The secretary of a project group posts a notice with the `post`
  tool and the team group's id; it appears in that group's main line under the
  secretary's name. The same call by a working AI is refused. A person's
  message there that names an AI produces no item. In the browser: the team
  group has no task list, no brief and no AI cards.

### Other forges (D-10; `contracts/step-2/checks-forge.md`)

- **A9.24** A group whose four templates (push, open request, read checks,
  merge) name the test forge program passes A5.1, the last sentence of A5.3,
  A5.10 and A5.13. The argument lists the program recorded are the templates
  with their named places filled, and `gh` was never started.
- **A9.25** A task title with a space, a quote, `;` and `$(touch marker)`
  arrives in its place as one unchanged argument, and no marker exists: no
  shell reads a template. `{title}` in `push` is refused; `{tag}` is literal.
- **A9.26** The M5 run against one real forge other than GitHub, with its own
  signed-in command-line program. (`live`)

### Home changes (D-11; `contracts/step-2/move.md`)

- **A9.27** P moves the group from `P1` to `P2`. While it moves, `P1` refuses
  commands for the group and writes no row. Afterwards the group row names
  `P2`; the rows on `P2` equal those `P1` held; the next task gets the next
  number; an item that was unacknowledged on `P1` is delivered once, by `P2`.
- **A9.28** Both browsers and `Q1` follow the `groups` push: within 5 seconds,
  without a reload, they sync from `P2`. An envelope `P1` accepted before the
  move, sent to `P2` again, gets the same answer with no second effect.
- **A9.29** Only the owner moves a group: a move by another admin is refused,
  and a `groups` push naming another home without the owner's signature is
  ignored by browsers and `Q1`. `P2` refuses a snapshot whose signature fails
  or with one row altered; `P1` then stays the home and writes again. After a
  move, `P1` refuses the group's commands and `Q1` an item `P1` signs for it.
- **A9.30** `P2` becomes unreachable in the middle of a move: `P1` stays the
  home and writes again, and `P2` holds no part of the group. `P1` is
  restarted in the middle of a move: exactly one of the two is the home.
- **A9.31** With a copy to `P2` scheduled every minute, `P2` holds the group's
  rows as of the last run. The copy is read-only: `P2` answers no `sync` and
  no command for the group, and no client is told of it as a home.
- **A9.32** The data directory of `P1` is deleted. P restores the copy on `P2`
  by hand: `P2` is the home with the rows of the last copy and none written
  later, and clients follow as in A9.28. A copy with one byte changed on disk
  is refused. In the browser: move, copy and the last copy's time.

### Checks in a sandbox (D-12; `contracts/step-2/checks-forge.md`)

- **A9.33** On each of Linux and macOS where spike S-10 passed, a real offline
  build and test of a sample project, with its subprocesses, passes as a
  check, using the checkout, the toolchain and scratch space. A check command
  there cannot read a test file in the owner's home folder outside the
  checkout; a file it writes outside the worktree and scratch space does not
  exist afterwards; its connection to a listener on the same machine fails,
  and the listener saw none. A5.17 holds inside the sandbox.
- **A9.34** On Windows, and where S-10 did not pass, checks run as in step 1,
  and the browser says so beside levels and check lists.

### Accepted check lists (D-3; `contracts/step-2/checks-forge.md`)

- **A9.35** Q-team gets a task in P's group. Before Q accepted the group's
  check list and templates, `run_checks` is refused, the task waits with
  `accept` and names Q, and a marker file that the first check command would
  create does not exist. Q's browser shows the repository, the base branch and
  each command and template, and sends `checks.accept` with their hash; then
  `run_checks` runs them. With another hash, or signed by P, `Q1` refuses it.
- **A9.36** An admin changes one check command, then one template. After each
  change `run_checks`, the push and the merge on `Q1` are refused with wait
  `accept` until Q accepts the new hash. The acceptance survives a restart of
  `Q1`, and it is for one group: a second group with the same list asks Q
  again. In the browser: the prompt shows what changed.

### What a browser trusts (D-5, D-6; `contracts/step-2/browser-trust.md`)

- **A9.37** Q's browser, in the group homed on `P1`, shows the author of a row
  whose envelope verifies against that author's device log. It shows the
  "unverified" mark on a row the test hook changed: text that differs from the
  signed payload, an envelope by a device outside the author's log, an AI's
  message not signed by its `host`, and a signed kind with no envelope.
- **A9.38** The home's own rows (`task`, `checks`, `system`) have no mark and,
  but for `task_merged` (R-24), no member's name. P's browser, for which `P1`
  is its own device, shows the rows of A9.37 unmarked, as in step 1.
- **A9.39** With two people in the group and the secretary's `account`
  `personal`, the group information shows the warning that recommends a team
  plan or key. With one person, or another account type, it does not.

### The whole step

- **A9.40** With step 2 built, A1.1 to A7.9 without their `live` parts pass
  for a one-person group after its `checks.accept`; no `confirm` card appears.
- **A9.41** The above pass on the pairs under "Scope" in `steps/step-1.md`.

## Step 3: extensions (A10)

TCP simultaneous open (3.4, D-19) has no test: it is not in the first version.
If spike S-9 shows a clear benefit, its tests are written with its plan.

### More CLIs (D-16; `contracts/step-3/drivers.md`)

- **A10.1** Every driver declares the capability table; Claude Code and Codex
  declare the values of section 8 of that contract, and A9.40 still holds.
  With a declaration made up in the test that varies each field in turn,
  each switch of its section 5 holds, A3.1 passes, and in the browser what
  is switched off is not offered. On native Windows, `perm.set` level 2 for
  Claude Code is refused and the browser offers no level 2 (`risks.md` Q-4).
- **A10.2** For a CLI whose terms allow API keys only, `agent.set` takes
  `api_key`; any other type is refused, and the browser offers none (R-54).
- **A10.3** For each CLI added, the M3, M4 and M7 tests of step 1 that are not
  `live` pass with its fake CLI, except those of a feature its table switches
  off. The fake's message shapes are copied from captures of the real program;
  until A10.4 has run, its level tables carry `[unverified]`.
- **A10.4** For each CLI added: A3.8, the second half of A4.1, A4.10, A7.1 and
  A7.8 with the real program, except those its table switches off. (`live`)

### Style presets (D-17; `contracts/step-3/knowledge.md`)

- **A10.5** A preset is a note of type `preset`. The secretary's brief names
  it; after the confirm, the `assign` turn of every task of that brief holds
  the preset's text unchanged, and a brief without one holds none. A brief
  naming a preset the group lacks is refused, with no brief row. A5.14 holds
  for presets. In the browser: the brief card names the preset; an editor
  lists, creates and changes them.

### Knowledge base (D-18; `contracts/step-3/knowledge.md`)

- **A10.6** `kb_put` by an AI stores a document. `kb_search` for a word that
  is in it returns it; for a word that is in none, an empty list; from an AI
  of another group homed on the same node, not this document. After the home
  restarts the search answers the same. In the browser: the group information
  lists the documents, and its search shows what `kb_search` returns.
- **A10.7** A5.14 holds for a document; after `forget` no search finds it. A
  `kb_put` whose text holds a test secret pattern fails with `secret`, and the
  home has no row for it.

## What these tests assume beyond the design answers

The answers are silent on these; the contract of each group decides.

| Test | Assumed | From |
| --- | --- | --- |
| A9.4 | a person whose AI holds an unfinished task cannot be removed | `contracts/commands.md` 1.2, `member remove` |
| A9.14 | decided on 2026-11-20: for an AI on a team plan and a person not on its list, the owner is asked with the same card | D-2; D-8 names personal subscriptions only. For C2-1 |
| A9.20 | there is a terminal to watch only during a takeover | D-14, "the bridge of step 1" |
| A9.23 | a secretary's node may post in a team group although no AI is a member of it | D-15 refuses `host.join` there |
| A9.27 to A9.32 | decided: the new home is another node of the group's owner | D-7: the owner is the home's owner. For C2-6 |
| A9.28 | receipts travel in the snapshot | `contracts/commands.md`, "Once only" |
| A9.24, A9.40 | decided: every group asks once per machine, a one-person group too, so the A5 tests gain one step, the owner's `checks.accept` | `contracts/step-2/checks-forge.md` sections 1 and 8, point 2, which replaced the first answer (no acceptance of a list one set oneself) |
