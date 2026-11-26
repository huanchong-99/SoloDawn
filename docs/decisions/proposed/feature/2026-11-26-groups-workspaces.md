# Agent Note: One kind of group, connected AIs and workspaces

Status: proposed

## Problem

Until 2026-11-26 a group was a project, and a project was a Git repository
([rules](../../../../planning/product-rules.md), R-10 to R-14 and R-90 to R-92 as they stood). Creating
a group asked for a repository address, a base branch, a forge and a list of check commands. When
an AI joined, its node cloned the repository; every task got a branch and a worktree; the node ran
the group's check commands and two hard checks of its own, pushed the branch, opened the pull
request, polled the hosted checks and, after a scored review, merged. A second kind of group,
`team`, existed for people without a repository, with a `bulletin` that a secretary could post
there. That machinery was built so that what was reviewed was exactly the commit that was merged,
and so that an AI never touched its owner's own working tree.

The owner tried the product in the browser simulation on 2026-11-26 and found that this is not
what making a group is. In the owner's words, translated from the decision log (which is kept
outside the repository): there should be only one kind of group, and "when I need to bring in a
colleague I add a person right here"; check commands "and so on" should not be asked for, and "a
Git repository is all wrong: what is making a group? I choose my friends or myself, and which AIs,
the ones that are already connected"; "only an AI that was connected can be added", and for that
AI one chooses "which workspace it is in: even for one repository there may be five or six on one
computer, and on different computers the repository is in different places", so that "as I add
the AI, the AI tells me which workspaces it can start in".

The same simulation showed a second, smaller gap: when a task was paused by hand or given to
another AI, the client could not say who had done it, because neither the wait nor the card state
recorded the actor.

## Proposal

**The owner's decision (2026-11-26).** The owner originated the change and, when the coordinator
asked follow-up questions, chose these points. The decision log records the answers and not the
options that were not chosen.

1. There is one kind of group. A group is created by choosing people and connected AIs, each AI
   with one workspace; people and AIs can be added at any time afterwards.
2. Only a connected AI can be added: a CLI that a paired computer of its owner found and whose
   account type the owner declared, or an external agent.
3. A workspace is a folder on the AI's computer. The AI reports the folders its CLI was used in
   there; the person can also name a folder by hand.
4. Git is not required. The AI changes files directly in the folder; SoloDawn creates no branch
   and merges nothing.
5. AIs in different folders or on different computers each work in their own workspace, and the
   system prompt tells them to keep in sync themselves.
6. No check commands are registered: the AI tests the way the project does.

**The coordinator's decisions under it.** The owner has delegated every project decision to the
coordinator (the main session), who decides and reports. These were decided on 2026-11-26, the
first five on the recommendation of the plan's author, the next two on questions raised by its
independent review, row 8 in the demo walk-through, rows 9 and 11 in the review's second pass and
row 10 in the first review; the owner was told and may change any
of them.

| # | Question | Decided | Reason given |
| --- | --- | --- | --- |
| 1 | Choosing friends when a group is created | a person who is already in one of the creator's groups on the same home is added at once (`member add_person`); anyone else gets a one-time link, as before | the owner's words: "I choose my friends or myself" |
| 2 | After a review passes | the task is done at once; the secretary arranges the closing step the project needs, such as a merge | the platform runs no Git, so it cannot perform or observe that step |
| 3 | The terminal of an AI that has no task | it opens in the AI's folder, on its group session, and creates no task card | the node reads no facts from the folder, so the old rule "cancel the task if nothing changed" cannot be kept |
| 4 | Two AIs of one group with the same folder on one computer | allowed; the dialog says which AI has the folder, and the folder takes one task at a time | none is recorded |
| 5 | Who sees the folder path of an AI | every member of the group | the reviewer and the secretary need it to find an AI's work |
| 6 | One folder in two groups | refused: `host.join` answers `conflict` and names the group | "one task at a time per folder" is the home's rule and holds only inside a group; across groups two tasks would change one folder at once |
| 7 | Pushing before review | the project's own rules for publishing come first; without them nothing is pushed to a branch others use before the review passes, and the AI pushes to a branch of its own when its reviewer works on another computer | unreviewed work must not land on a shared branch; a reviewer on another computer can read only what was pushed |
| 8 | Who paused, who reassigned | the `manual` wait of a task gains `by`, and so does the approval state `reassigned`: the member id of the sender, written by the home | found in the simulation: the client could only say "the secretary or an admin" |
| 9 | Protocol version | `hello` and `welcome` carry `v` 3; a node answers `version` to 1 and 2; neither the node nor the browser retries with an older version | the rows changed shape and a node of `v3.0.0-rc.1` would misread them; with the retry, a new client would still talk to an old home |
| 10 | Groups made before the change | deleted by a migration at the first start, with the device records of everyone but the node's owner; an export file of the old version is refused | only the pre-release `v3.0.0-rc.1` was published; keeping old groups would mean mapping branches, pull requests and worktrees onto a model that has none |
| 11 | The list of folders | cut where the answer would pass the size of one `ctl` message; Codex's folders are read from its session files | the review asked for no cap and for Codex's documented request; a bound has to exist, and the finders are a package that starts no CLI |

**What changes**, by the document that owns it:

- Rules: [product rules](../../../../planning/product-rules.md) R-10 to R-14, R-20, R-22, R-24, R-83, R-84,
  R-90 to R-92, R-103, R-111, R-137, R-140.
- Rows and commands: [rows](../../../../planning/contracts/rows.md) (no `kind`, `repo`, `base`, `forge`,
  `checks` on a group; `workspace` on an AI; a task with `report` and the states and steps of the
  shorter machine), [commands](../../../../planning/contracts/commands.md) (`group create` with two
  fields, `member add_ai` with `workspace`, `member add_person`, `host.join` with `workspace` and
  its `conflict`), [people](../../../../planning/contracts/step-2/people.md) section 2.5.
- The task: [task machine](../../../../planning/contracts/task-machine.md). Work, a submission that is
  the assignee's `report` message, a scored review, done. A folder takes one task at a time: a
  task whose assignee's folder is in use waits as `queued` with wait `busy` and starts by itself.
- The host: [local interfaces](../../../../planning/contracts/local.md) section 7 (the folder of a
  session, the one-group rule, where the list of folders comes from),
  [data channel](../../../../planning/contracts/datachannel.md) (`host.workspaces`, `workspace` in
  `host.info`, `v` 3), [storage](../../../../planning/contracts/storage.md) section 7 (migration 4).
- What an AI is told: [drivers](../../../../planning/contracts/drivers.md) sections 3 and 4. The
  instructions now carry what the platform did before: bring the folder up to date, test the
  project's way, publish by the project's rules or else to a branch of its own, never stash,
  reset, clean, `git add -A`, switch branches or force a push in a folder that may hold someone
  else's work, and end the turn after `submit_task`.
- Flows: [task](../../../../planning/flows/task.md), [trace](../../../../planning/flows/trace-task.md),
  [delivery](../../../../planning/flows/agent-turn.md), [takeover](../../../../planning/flows/takeover.md).

**What is removed.** The clone and the worktrees; the check runner and both hard checks on a
diff; the forge commands, the custom forge templates and their contract; the merge, its item and
its "point of no return"; the tools `push_branch` and `run_checks`; the task commands `branch`,
`checked`, `submitted`, `cloud`, `merged` and `conflict`; the `checks` message; the group kind
`team` and the `bulletin`; the task origin `takeover`; the slug. They were justified by the
guarantee named under Problem, and that guarantee belongs to a model in which the platform owns
the repository. The owner's model has no repository to own, so the machinery has nothing to act
on. Reintroducing any of it would need a group to name a repository again, which is a reversal of
this decision and a new record.

**Choices made while writing the contracts** (unit C-1, under the coordinator's instruction to
decide by the plan's principles where the plan is silent, and to record the choice here):

1. This record is `proposed`, in class `feature`: nothing is built yet, and the class set of the
   project standard is closed. It moves to `implemented/` when the change is on the integration
   branch, with its evidence.
2. A submission (task-machine row 5) is accepted while the task is `paused`, as the host's
   `task submitted` was: a pause can arrive while `submit_task` is on its way.
3. `task start` keeps its field `origin`, whose one value is now `direct`.
4. The review turn names the computer and folder of the member who wrote the report, which is
   the assignee at the time of the submission, not whoever is assignee when the review is read.
5. `host.workspaces` resolves each folder as `host.join` does and merges two spellings of one
   folder, so that the path it lists equals the `workspace` of a member that has it. The time of
   a folder is the modification time of its newest session file.
6. A direct task that was created `queued` gets, when it starts, an `assign` item that carries
   its `task` message; the owner's original message travels only with a task that starts at once.
7. A takeover whose session folder is gone is refused with `cli` and the path.
8. `post` takes the kind `handback` in every tool list but the external one, since a takeover can
   now reach a working AI's group session; that hand-back goes to the main line and queues no
   item.
9. Numbers are kept where other documents and the code cite them: task-machine rows 3, 4, 6, 8
   and 9 and message section 2.8 are left unused, and removed rules, constraints and acceptance
   tests keep a one-line entry.

## Alternatives considered

**Keeping a group as a Git repository** (clone, branch, worktree and merge), the model described
under Problem. It lost: the owner tried it in the demo walk-through and rejected it, and the
platform then needs a Git operation for every step of a task.

**For the owner's six points** the decision log records what the owner said and chose, and not
the options the coordinator offered in its follow-up questions.

**One folder shared by AIs of two groups, stated as a limit** (question 6). Raised by the review
as the alternative to refusing. It lost because two groups, possibly with different homes, would
then change one folder at the same time, which is exactly what the busy rule promises cannot
happen.

**Push to a shared branch before review, or never push unless the project's notes say so**
(question 7). Both were put by the review. The first lets unreviewed commits reach a branch
others use; the second leaves a reviewer on another computer with nothing to read. The decided
rule sits between them and yields to the project's own rules.

**A count cap of 200 folders; no cap at all** (question 11). The plan first had the count, the
review asked for none. The answer is one `ctl` message, which has a size limit, so the bound is
that size.

**Codex's documented `thread/list` request for its folders** (question 11). Verified to work, and
kept as what the finder changes to if the file layout changes. It lost for now because it needs a
running CLI process and the driver's connection, and the finders are a package that imports
neither.

**Keep `v` 2, or keep the retry with version 1** (question 9). The plan first changed no version;
the review showed that both clients fall back to 1 and that an old home accepts it, which is the
mixed connection the change is meant to stop.

**Convert the groups of the published pre-release** (question 10). Considered in the review and
declined: one pre-release exists, the owner is in effect its only user, and its rows have no
meaning in the new model.

For questions 1 to 5 and 8 the available history names the question and the answer; which other
answers were weighed is not recorded. For question 3 the plan names what it replaces: the earlier
rule, under which such a takeover created a task and cancelled it when nothing had changed.

## Acceptance criteria

- The acceptance sentences written for the change pass: A2.1, A2.8 to A2.10, A3.3, A3.11 to
  A3.13, A5.1, A5.2, A5.4 to A5.7, A5.12, A5.19 to A5.24, A7.1, A7.7 and A8.7
  ([step 1](../../../../planning/steps/step-1-acceptance.md)); A9.22 and A9.46
  ([step 2](../../../../planning/steps/step-2-3-acceptance.md)); A10.10, A10.12, A10.17, A10.18,
  A10.21 and A10.22 ([step 3](../../../../planning/steps/step-3-acceptance.md)).
- The node starts no `git` and no `gh` process and runs no command that a group names.
- A group is created in the browser from a name, people and connected AIs with a folder each, and
  an AI's process runs in the folder it was added with.
- A node that starts on the data directory of `v3.0.0-rc.1` keeps pairing, its owner's devices,
  account types and peers, lists no group and leaves `projects/` as it was.
- This record is then moved to `implemented/` and rewritten in the present tense.

## Risks

The limits this decision accepts are K-26 to K-38 of [risks](../../../../planning/risks.md). In short:

- An AI works at the highest level in a folder its owner uses too. The instructions name what it
  must not do there; that is a sentence, not a barrier.
- A failed, cancelled or reassigned task leaves its changes in the folder.
- One folder takes one task at a time and serves one group, so less runs in parallel.
- What was reviewed is no longer, by construction, what is delivered: the review names a report,
  the report is the assignee's statement, and the merge afterwards is a step someone arranges.
- The scan of a diff for secrets and the check for a change without tests are gone; a missing
  test is caught by the reviewer or not at all.
- A reviewer on another computer sees the work only if the project has a shared remote.
- The list of folders reads session files that no vendor documents as an interface; the field
  for OpenCode is not verified, and OpenCode and Qwen Code were not run.
- Groups of the published pre-release are deleted, and its nodes and browsers cannot talk to the
  new version.

## Related

- The units that build it are listed in
  [the plan of steps 2 and 3](../../../../planning/steps/parallel-plan-step-2-3.md), last section.
- The plan and its independent review, on which this record rests, are working documents kept
  outside the repository. Nothing in this record was run: it describes a decision and the
  contracts written for it.
