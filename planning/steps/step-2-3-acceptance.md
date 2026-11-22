# Steps 2 and 3: acceptance tests

The acceptance tests of step 2 (A9.x), one group per feature of
`steps/step-2-3.md`; those of step 3 (A10.x) are in
`steps/step-3-acceptance.md`. They rest on the design answers D-1 to D-19 of
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
- **Another forge** is a test program, first on PATH, that records its
  argument list and answers from files.
- A sentence that starts "In the browser" is checked end to end in the web
  client; everything else by integration.

## Step 2: teams (A9)

### People (D-7; `contracts/step-2/people.md`)

- **A9.1** P creates an invitation. The link holds the group id, the device id
  of `P1` and a one-time secret. Q's browser, which has never met `P1`, joins
  with it. Afterwards a member row for Q has role `member`,
  Q's device log is on the home as `device` rows, and both browsers show Q and
  the group's history without a reload.
- **A9.2** A joiner whose device log does not replay gets no member. The
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

### Other people's hosts (`contracts/step-2/people.md`)

- **A9.5** `Q1` joins Q-team to the group with `host.join`: the member row has
  `host` = `Q1` and `user` = Q, and `Q1` holds a clone. `member add_ai` from a
  node whose owner is not a member is refused, and so is `secretary: true`
  from Q while the group has a secretary.
- **A9.6** What a node accepts (`contracts/datachannel.md` section 8). To a
  device of Q, `P1` answers the home surface of this group only: a host
  command, `host.info` and `sync` of another group homed there are refused.
  `Q1` accepts no host command and no `takeover.start` from a device of P.
- **A9.7** Removed on 2026-11-22 under the owner's rule of 2026-11-21 (no
  defence in code; members of a group trust each other): the fork rule of the
  device log is cut (D-4, T2-4).

### Whose instructions (D-8; `contracts/step-2/confirm.md`)

- **A9.8** P mentions Q-personal in the main line. No item is made; a
  `confirm` card that mentions Q appears. Q confirms from either of two
  devices; only then does the item reach `Q1` and the fake CLI log a turn. A
  mention by Q makes no card.
- **A9.9** Q refuses the card, or leaves it: no item exists, no turn starts,
  and P sees its state. `act confirm` by P is answered `forbidden`; sent twice
  by Q with one nonce it makes one item and is answered `ok` twice. An `allow`
  list for Q-personal that names P does not remove the card.
- **A9.10** Removed on 2026-11-22 under the owner's rule of 2026-11-21 (no
  defence in code; members of a group trust each other): a host takes the
  items of its home and checks no owner's signature on them (D-2).
- **A9.11** P confirms a brief that makes Q-personal assignee of one task and
  reviewer of another. Both wait for Q, the second from step `review`; until Q
  confirms, no `assign` or `review` item reaches `Q1` and no worktree exists
  there. In the browser: the card, its actions for Q only, each end state.

### After a task started (D-9; `contracts/step-2/labels.md`)

- **A9.12** Q-personal has a task that P's brief and Q's confirmation started.
  P mentions the AI in its thread: no card appears, and the turn's header
  reads `information`, also when P's text says "your owner instructs you". Q
  posts there: the header reads `instruction`.
- **A9.13** P confirms a brief that replaces the first and changes the task. A
  new `confirm` card for Q appears; until Q confirms it, no turn labelled
  `instruction` from that brief reaches Q-personal.
- **A9.42** Two labels. In a task of Q-team on `Q1`, a `reviewed` item reads
  `instruction` in its header and `information` on the line of the review,
  which P's AI wrote. On a decision that Q-team posted to Q, a choice by P as
  admin reads `information` on its line and a choice by Q `instruction`; the
  header reads `instruction` both times. The secretary's `confirmed` for a
  brief that Q confirmed reads `information` on its line unless P listed Q for
  it. In a one-person group the owner's own choices and confirmations read
  `instruction` on their lines.

### Allow lists (`contracts/step-2/authority.md`)

- **A9.14** Q puts P on the `allow` list of Q-team. A mention by P is
  answered, and a brief confirmed by P starts its task on `Q1`, both with no
  card. After Q removes P, a mention by P starts no turn unless Q confirms it.
  In the browser: Q edits the list and delegates of each AI; P only sees them.
- **A9.15** Removed on 2026-11-22 under the owner's rule of 2026-11-21 (no
  defence in code; members of a group trust each other): the list is the
  member row; no host checks a list its owner signed (D-2).

### Level cap (D-1)

- **A9.16**, **A9.17** Removed on 2026-11-22: R-44 is not in 1.0, where
  every session runs at the highest level.

### Calls across owners (D-8; `contracts/step-2/confirm.md`)

- **A9.18** P's working AI calls Q-personal with `ask`: a `confirm` card for Q
  appears, a receipt comes after the wait, and the answer as a turn once Q
  confirms; denied, a turn says so. A call to Q-team, which lists P, is
  answered in the open tool call with no card, as is one between Q's AIs.

### Approver delegates (D-13; `contracts/step-2/authority.md`)

- **A9.19** Q names P as a delegate for Q-team (`member set` with
  `approvers`). An approval card of Q-team mentions Q and P. P approves and
  the command runs. After Q removes P, `act approve` by P on a card posted
  before is answered `forbidden`. A delegate for Q-team decides nothing for
  Q-personal.

### Roles and the secretary (`contracts/step-2/confirm.md`, `contracts/step-2/authority.md`)

- **A9.43** A role the brief does not give. P makes Q `admin` and confirms a
  brief that names only Q's AIs. Q gives its task to P's working AI with
  `task assign`: a `confirm` card for P appears, and the task starts on `P2`
  once P confirms. When P gives it, or P's secretary with `set_task`: no
  card.
- **A9.44** `group set` with `secretary` naming Q-team, by an admin, makes it
  the secretary: `Q1` then lists the secretary's tools for Q-team and
  delivers a `notice`. In a one-person group the owner names another of their
  AIs with `group set`: the quiet-task `notice` of A5.16 and a brief sent back
  reach it.
- **A9.45** Removed on 2026-11-22 under the owner's rule of 2026-11-21 (no
  defence in code; members of a group trust each other): a host merges what
  its home's `merge` item names, as in step 1.

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
  with their named places filled, and `gh` was never started. In the browser:
  the dialog that joins an AI to the group shows the check commands and each
  template.
- **A9.25** A task title with a space, a quote, `;` and `$(touch marker)`
  arrives in its place as one unchanged argument, and no marker exists: no
  shell reads a template. `{title}` in `push` is refused; `{tag}` is literal.
- **A9.26** The M5 run against one real forge other than GitHub, with its own
  signed-in command-line program. (`live`)

### Home changes (D-11; `contracts/step-2/export.md`)

- **A9.27** P runs `solodawn group export` with `--to` `P2` on `P1`, and
  `solodawn group import` of that file on `P2`. The rows on `P2` equal those
  `P1` held; the next task gets the next number; an item that was
  unacknowledged on `P1` is delivered once, by `P2`. `P1` answers the group's
  requests with `moved` naming `P2`, and both browsers and `Q1` follow within
  5 seconds of the import, without a reload. Importing the file on `Q1` fails,
  and so does exporting the group on a node that is not its home. In the
  browser: the group info shows the two commands to P only.
- **A9.28** A copy of the group is exported from `P1` every minute, then the
  data directory of `P1` is deleted. P imports the last copy on `P2`: `P2` is
  the home with the rows of that copy and none written later. P sends an
  invitation from `P2`; Q opens it, `join` answers `ok` with no new member
  row, and Q's browser and `Q1` follow `P2`.
- **A9.29** to **A9.32** Removed on 2026-11-22: export and import replace
  the move and the copy (A9.27, A9.28).

### Checks in a sandbox; accepted check lists (cut)

- **A9.33**, **A9.34** Removed on 2026-11-22: no sandbox for checks in 1.0
  (D-12, T2-12); `risks.md` K-8 stays a stated limit.
- **A9.35**, **A9.36** Removed on 2026-11-22: no per-machine acceptance of
  the check list (D-3, T2-9). The client shows the commands instead
  (`contracts/step-2/checks-forge.md` section 1), tested as part of A9.24 in
  the browser.

### What a browser shows (D-6; `contracts/step-2/browser-trust.md`)

- **A9.37**, **A9.38** Removed on 2026-11-22 under the owner's rule of
  2026-11-21 (no defence in code; members of a group trust each other): a
  browser shows what the home pushes and checks no envelope (D-5, T2-5).
- **A9.39** With two people in the group and the secretary's `account`
  `personal`, the group information shows the warning that recommends a team
  plan or key. With one person, or another account type, it does not.

### The whole step

- **A9.40** With step 2 built, A1.1 to A7.9 without their `live` parts pass
  for a one-person group; no `confirm` card appears.
- **A9.41** The above pass on the pairs under "Scope" in `steps/step-1.md`.

## Step 3: extensions (A10)

In `steps/step-3-acceptance.md`.

## What these tests assume beyond the design answers

The answers are silent on these; the contract of each group decides.

| Test | Assumed | From |
| --- | --- | --- |
| A9.4 | a person whose AI holds an unfinished task cannot be removed | `contracts/commands.md` 1.2, `member remove` |
| A9.14 | decided on 2026-11-20: for an AI on a team plan and a person not on its list, the owner is asked with the same card | D-8 names personal subscriptions only |
| A9.20 | there is a terminal to watch only during a takeover | D-14, "the bridge of step 1" |
| A9.23 | a secretary's node may post in a team group although no AI is a member of it | D-15 refuses `host.join` there |
| A9.27, A9.28 | decided: the new home is another node of the group's owner | D-7: the owner is the home's owner |
