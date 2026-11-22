# Step 3: acceptance tests

The acceptance tests of step 3 (A10.x), one group per feature of
`steps/step-2-3.md`. They were split from `steps/step-2-3-acceptance.md`
for its length: how acceptance is tested, the cast and the test hook are
there. They rest on the design answers D-16 to D-19 of
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

## AI profiles and escalation (R-165; `contracts/step-3/profiles.md`)

Cast: AIs S (`strong`), M (`standard`) and L (`light`) of one owner; the
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
  line that names S and not L; `submit_task` fails with `conflict`, and
  nothing is pushed. M calls `ask` to L and L replies: still refused. M
  calls `ask` to S and S replies: `ask` is `false` and the next
  `submit_task` succeeds. The same with two failed hosted checks and
  `kind` `checks`. In the browser: the badge "must ask a stronger AI"
  while `ask` is `true`.
- **A10.11** S is the assignee, or every AI keeps the default tier: after
  two failed reviews in a row `stuck.ask` is `false`, the item has no added
  line and `submit_task` succeeds. A1 to A8 pass unchanged with default
  profiles.
- **A10.12** M fails hosted checks once, then a review once: `n` is 1 and
  nothing is asked. M fails a review, then passes one: `stuck` is gone, and
  a later single failure asks nothing. `task assign` to another AI removes
  `stuck`.

