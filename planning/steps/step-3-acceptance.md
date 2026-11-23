# Step 3: acceptance tests

The acceptance tests of step 3 (A10.x), one group per feature of
`steps/step-2-3.md`. They were split from `steps/step-2-3-acceptance.md`
for its length: how acceptance is tested, the cast and the test hook are
there. They rest on the design answers D-16 to D-20 of
`steps/parallel-plan-step-2-3.md` section 1; each heading names the answers
and the contract of its group. Where a contract differs from this file, the
contract holds.

TCP simultaneous open (3.4, D-19) has no test: it is not in the first version.
If spike S-9 shows a clear benefit, its tests are written with its plan.

## More CLIs (D-16; `contracts/step-3/drivers.md`)

- **A10.1** Every driver declares the capability table; Claude Code and Codex
  declare the values of section 8 of that contract, and A9.40 still holds.
  With a declaration made up in the test that varies each field in turn,
  each switch of its section 5 holds, A3.1 passes, and in the browser what
  is switched off is not offered.
- **A10.2** For a CLI whose terms allow API keys only, `agent.set` takes
  `api_key`; any other type is refused, and the browser offers none (R-54).
- **A10.3** For each CLI added, the M3, M4 and M7 tests of step 1 that are not
  `live` pass with its fake CLI, except those of a feature its table switches
  off. The fake's message shapes are copied from captures of the real program;
  until A10.4 has run, its level tables carry `[unverified]`.
- **A10.4** For each CLI added: A3.8, the second half of A4.1, A4.10, A7.1 and
  A7.8 with the real program, except those its table switches off. (`live`)

## Style presets (D-17; `contracts/step-3/knowledge.md`)

- **A10.5** A preset is a note of type `preset`. The secretary's brief names
  it; after the confirm, the `assign` turn of every task of that brief holds
  the preset's text unchanged, and a brief without one holds none. A brief
  naming a preset the group lacks is refused, with no brief row. A5.14 holds
  for presets. In the browser: the brief card names the preset; an editor
  lists, creates and changes them.

## Knowledge base (D-18; `contracts/step-3/knowledge.md`)

- **A10.6** `kb_put` by an AI stores a document. `kb_search` for a word that
  is in it returns it; for a word that is in none, an empty list; from an AI
  of another group homed on the same node, not this document. After the home
  restarts the search answers the same. In the browser: the group information
  lists the documents, and its search shows what `kb_search` returns.
- **A10.7** A5.14 holds for a document; after `forget` no search finds it. A
  `kb_put` whose text holds a test secret pattern fails with `secret`, and the
  home has no row for it.

## External AIs (D-20, R-166; `contracts/step-3/external.md`)

Numbered after the tests of the next group, which were written first.

Cast: X, an external AI that the owner adds on node P1 in the test, and M,
a managed working AI with a fake CLI. X's agent is a test program: a stdio
MCP client started through `solodawn mcp --member`, and a test runner
around `solodawn wait` and `solodawn done`. The test shortens the 180
seconds of presence and the turn limit.

- **A10.17** Adding. `host.join` with `agent` `external`, a name, an
  account type and a tool label gives a member row with `cli` `external`,
  that `tool`, `secretary` `false`, the default profile and, once the
  node has reported, `state` `offline`. No `agent.set` came first. Without
  `account`, with `secretary` `true`, or with a `tool` of 33 code points
  it is `bad_request`; so is `agent.set` for `external`. `member set` with
  `tool` by the owner changes the row, and by anyone else is `forbidden`.
  A second `host.join` adds a second external AI. `host.info` has `exe`
  and one agent `external` with `takeover` `none`, `models` `[]` and both
  members. In the browser: the dialog that adds an AI offers the external
  agent under each of the owner's nodes; the setup panel shows the prompt
  with that member's id and the node's program path, and the server entry
  alone; the member card shows `external` and the label, and no terminal
  button.
- **A10.18** Looking once. With nothing waiting, `solodawn wait --member X
  --timeout 0` exits 3 at once and prints nothing. After the owner
  mentions X, it exits 0 with one line of JSON: `run`, `kind` `group`,
  `folder` the clone, a `context` that holds the group instructions and
  the lines of section 9, and a `text` that holds the message; the home
  holds no item for X any more, and X is `working`. While that run is
  open the command exits 3, also with a second mention queued. `solodawn
  done` with that `run` exits 0 and X is `waiting`; the next look hands
  out the second mention, once. A `done` for the first run then changes
  nothing and exits 0. An id that names no external AI of this node exits
  1 with one line.
- **A10.19** Waiting. `solodawn wait --member X` blocks, and X is
  `waiting`; a mention makes it print the hand-out and exit 0. With
  `--timeout 2` and nothing waiting it exits 3 after two seconds. A wait
  that is open when X is removed exits 1.
- **A10.20** The setup check, and one start per item. After `host.join`
  the client's check message is in the main line and its item waits.
  With the test runner as a loop: the agent is started once, in `folder`,
  with `context` and `text`; it answers with `post`; the runner ends the
  run; X went `offline`, `waiting`, `working`, `waiting`. The same with
  the runner as a task that looks every second with `--timeout 0`: while
  the agent runs, every look exits 3 and no second agent is started. Two
  mentions then start the agent twice, one after the other. In the
  browser: the four marks of the setup panel turn on in that order, and
  the button sends the check again.
- **A10.21** Tools. A client behind `solodawn mcp --member X` is listed
  exactly the tools of `external` in `tools.json`, and `approve`. With no
  run open, `post`, `ask` to M with its answer, `read_thread`,
  `list_members`, `list_tasks`, `note_put`, `kb_put` and `kb_search` act
  as X; `run_checks` and `submit_review` fail with `conflict`; a `post`
  whose text holds a test secret fails with `secret`. `next_item` answers
  `{"none":true}` at once when nothing waits, and the hand-out when
  something does; `done` with its `run` ends that run. A connection whose
  id names no external AI of this node is closed at once.
- **A10.22** A task. A brief names X assignee and M reviewer. X's hand-out
  has `kind` `task`, `task`, a `folder` that is the task's worktree on its
  branch, and the brief with its criteria in `context`. X commits there;
  `run_checks` and `submit_task` succeed with no `task` argument; M
  reviews; the node merges; the worktree is gone. With the roles
  swapped, X's hand-out has `kind` `review`, the review checkout as
  `folder` with `.solodawn-review.diff`, and `submit_review` succeeds.
  A5.1 holds with X as one of its two AIs, with and without a remote.
  `start_task` in a group run gives the task number, and the `assign`
  item is handed out after that run ended.
- **A10.23** A run that died. `solodawn done` with `--failed` in a task
  run: a `system` row `turn_interrupted` with `crash` and that text, the
  task's `failures` is 1, and the secretary has its `notice`. A run that
  nobody ends: after `limits.turn_minutes` the same with `timeout`, X is
  no longer `working`, and the next look hands out the next item. A node
  restarted during a run reports `crash`. In all three the items of the
  dead run are not handed out again, and a later `done` for it changes
  nothing.
- **A10.24** Held. While the group is paused, while the item's task is
  paused, and after `host.offline`, a look exits 3 although an item
  waits, and a blocking wait keeps blocking; X is `paused`, `waiting` and
  `offline` in turn. After the resume the item is handed out. An item the
  owner jumped is handed out before an older one.
- **A10.25** Usage and the member list. `report_usage` with `window_pct`
  40 and a reset time puts both into X's row, and `list_members` shows
  them; a second report replaces them; 101 is `bad_request`; M is not
  listed the tool. The secretary's next turn lists X as `external` with
  its label, computer, tier, state and skills. In the browser: the member
  card shows the reported usage.
- **A10.26** Approval. In a task run, a call of `approve` with Claude
  Code's input gives an approval card for the owner in that task; after
  the owner approves the call returns `{"behavior":"allow"}`, and after a
  denial the denial message. An agent that never calls it produces no
  card.
- **A10.27** Offline, and removal. The runner stops: after the presence
  time X is `offline`, and the secretary's member list says so. A mention
  made meanwhile stays in the inbox and is handed out when the runner
  returns. `member remove` while X is assignee of an unfinished task is
  `conflict`. After the task is reassigned it succeeds: the row is gone,
  X's open MCP connection is closed, and `solodawn wait` and `solodawn
  mcp` with that id fail. In the browser: the owner removes X from the
  member panel.
- **A10.28** A real agent. The owner pastes the setup prompt into one
  real agent on a computer with a node. Without further help it adds the
  server, writes and starts its runner, answers the setup check once, and
  is `waiting` afterwards with no AI process running. (`live`)

## AI models, profiles and escalation (R-165; `contracts/step-3/profiles.md`, `contracts/step-3/drivers.md` section 10)

Cast: working AIs S (`strong`), M (`standard`) and L (`light`) of one
owner, and the secretary C (`standard` unless a test says otherwise); the
fake CLIs fail checks and reviews on script.

- **A10.8** A new AI's row has `tier` `standard` and `skills` `""`. `member
  set` by its owner with `tier` and `skills` changes the row; by its host,
  by another person or by an admin who is not its owner it is `forbidden`;
  a `tier` outside the three values, and `skills` of 201 code points or
  with a line break, are `bad_request`. In the browser: the AI member panel
  shows both, the owner edits them there, and nobody else is offered the
  editor.
- **A10.9** The next turn of the secretary after A10.8 lists every AI member
  with its tier, state and skills as in section 3 of the contract; an AI
  that is `limited`, and one whose host is offline, are listed so. The
  secretary's instructions hold the four rules. `list_members` is as in
  step 1.
- **A10.10** M is the assignee. After one failed review nothing differs
  from step 1. After the second in a row: the task row has `stuck` with
  `kind` `review`, `n` 2, `ask` `true`; M's `reviewed` item ends with the
  line that names S and neither L nor C; `submit_task` fails with
  `conflict`, and there is no push, no pull request and no `report`. None
  of these clears it: L's reply to M's call; S's reply to L's call in
  this thread; S's reply to M's call in another task's thread. S's host
  goes offline: still refused. S returns and replies to M's call in this
  thread: `ask` is `false` and the next `submit_task` succeeds. A third
  failed review then gives `n` 3, `ask` `true`, the line again and the
  refusal again. The same with two failed hosted checks and `kind`
  `checks`. In the browser: the badge "must ask a stronger AI" while `ask`
  is `true`.
- **A10.11** S is the assignee, or every AI keeps the default tier: after
  two failed reviews in a row `stuck.ask` is `false`, the item has no added
  line and `submit_task` succeeds. With C `strong` and S the assignee: the
  line names C, and C's reply clears `ask`. A1 to A8 pass with default
  profiles.
- **A10.12** M fails hosted checks once, then a review once: `n` is 1 and
  nothing is asked. M fails a review, passes hosted checks on the next
  commit and fails the review again: `n` is 2 and `ask` is `true`, since a
  passed hosted check clears only `kind` `checks`. M fails a review, then
  passes one: `stuck` is gone, and a later single failure asks nothing.
  `task assign` to another AI removes `stuck`.
- **A10.13** Discovery. With each fake CLI scripted to list two models,
  `host.info` carries them under that agent as `models`, with id, name,
  levels and, where the script says so, `fast`; an id with a word of the
  table has that tier, any other `standard`. A fake CLI whose listing
  fails, and one without a listing (Qwen Code), give `models` `[]`, and
  `host.info` is otherwise as before. `gemini-3-pro` is suggested
  `standard`, `gpt-6-mini` `light`; Claude Code's `default` entry has the
  tier of its `resolvedModel`. In the browser: the dialog for adding an AI
  offers the listed models and their levels; with the empty list it offers
  two text fields. For each real CLI, the list is what the CLI's own picker shows.
  (`live` for the last sentence)
- **A10.14** Refresh. The fake CLI's list gains a model. A `host.info`
  request within 60 s answers the old list; one with `refresh` the new
  one, and a push carries it. In the browser: the refresh button on the
  AI member panel shows the new model, with no restart of the node.
- **A10.15** Choosing. `host.join` twice with one agent, two names and two
  models gives two member rows with `model`, `effort` and the tier the
  dialog was sent with; `host.info` lists both under that agent;
  `host.leave` without `member` is `bad_request`, and with it removes that
  one. Each member's sessions start the fake CLI with its own model and
  level in the arguments or parameters of section 10.4; a member with
  `""` passes neither. After `member set` with another model the next
  start of a session of that member, a resume, passes the new one. A
  model the CLI refuses, the fake Claude Code exiting and the fake Codex
  answering `thread/start` with an error, is handled as a crash in both:
  `turn_interrupted` with `crash`, in a task session `task fail`, no
  `session_replaced`, and the session keeps its id. (`live`:
  once per real CLI, the model the CLI reports is the one chosen)
- **A10.16** The owner's edit stays. The owner sets `tier` `light` and
  `skills` on a member whose model is suggested `strong`. After a refresh
  whose list suggests otherwise, lacks that model, or is empty, and after
  a restart of the node, the member row is unchanged byte for byte. In the
  browser: the panel shows the owner's tier, the suggestion beside it, and
  "not listed" for a model that is gone.

