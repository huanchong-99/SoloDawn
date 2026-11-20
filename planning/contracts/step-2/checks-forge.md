# Contract (step 2): accepting check lists, forge templates

What a machine owner accepts before their node runs a group's commands
(design point D-3), and how a group names the commands of a forge other
than GitHub (D-10). Rules: R-40, R-44, R-92, R-103, R-136, R-137. It adds
to `contracts/local.md` section 7, `contracts/commands.md`,
`contracts/rows.md` section 1 and `contracts/task-machine.md`, and changes
the meaning of no step 1 shape. It is folded into those files when step 1
is built; until then this file is the statement of record for its subject.
Markers `[verified]`, `[documented]`, `[unverified]` as in that file.

## 1. The rule

In a team, a group admin who is not the machine's owner sets the checks
and the forge commands, and every assignee's node runs them as its owner
(`risks.md` K-8). So:

- A host runs no check command and no forge command of a group unless its
  owner has accepted exactly the list the host holds now.
- "The list" is the group's `forge`, `checks` and `templates`, as one
  hash (section 2). A change of any of them is another hash and asks again.
- Acceptance is per machine and per group: one host command,
  `checks.accept` (section 3), signed by a device of the machine's owner.
- There is no exception: not for an empty list, not for `forge: github`
  or `none`, not for the person who set the list. A group row carries no
  signature, so a host cannot tell who wrote the list it holds, and a
  home of someone else could show a host another list than it shows a
  browser. The host relies on its owner's signature over the hash only.
- Acceptance gives no authority to anyone: whose instructions an AI
  follows is `contracts/step-2/authority.md`.

## 2. The hash

The node and the web client each compute it from the group row they hold.
`hash` = SHA-256 over the bytes of `"solodawn/accept/1\n"` followed by a
sequence of items, written as base64url (`contracts/envelope.md` section
1): 43 characters.

Each item is a text, written as the number of its UTF-8 bytes in decimal
digits, a colon, and those bytes. Text is encoded the way `TextEncoder`
encodes it (`contracts/envelope.md` section 6). A count is the text of
its decimal digits. The items, in this order:

1. `forge`;
2. the number of entries of `checks`, then each entry in the row's order;
3. only when `forge` is `custom`, for each of `push`, `open`, `checks`,
   `merge` in this order: the number of arguments of that template, then
   each argument in order.

Nothing is trimmed, sorted or normalised. Lengths and counts make the
sequence readable in one way only, whatever a command contains.

Example `[verified]` (computed for this file). For a row with

```json
{"forge":"custom","checks":["go vet ./...","go test ./..."],
 "templates":{"push":["git","push","origin","{branch}"],
  "open":["gh","pr","create","--head={branch}","--base={base}",
          "--title={title}","--body={summary}"],
  "checks":["gh","pr","checks","{request}"],
  "merge":["gh","pr","merge","{request}","--squash",
           "--match-head-commit={commit}"]}}
```

the hashed text begins
`solodawn/accept/1\n6:custom1:212:go vet ./...13:go test ./...1:43:git4:push`
and the hash is `j-xacoOxVZG8QPCjzKHL7LDYQTGGT8dK_1fSY5Vf4as`. For
`{"forge":"github","checks":[]}` the hashed text is
`solodawn/accept/1\n6:github1:0` and the hash is
`f7DsroUQUZ7FI-VmMKS7gZLsn7kZu1H3dU_JQwDK9s8`.

The shared vectors (`contracts/vectors.md`) get a part `accept`: cases of
`{"name","forge","checks","templates","hash"}`, with `templates` written
`{}` when `forge` is not `custom`.

## 3. `checks.accept`

A host command (`contracts/commands.md` section 2): accepted only from a
device in the node's owner's log, with `to` equal to the node's device
id. An approver delegate cannot send it.

```json
{"t":"checks.accept","n":"…","ts":1763600000000,"to":"<device id>",
 "g":"Qm1…","hash":"<43 characters>"}
```

- The node compares `hash` with the hash of the group row it holds. Equal:
  it stores the hash for the group and writes the audit row `setting`
  with the hash as `detail`. Not equal: `conflict`, and nothing is stored.
  A `g` in which this node has no AI is `not_found`.
- **What the client shows before it sends.** Everything the hash covers:
  `forge`, every entry of `checks` in full, and for `custom` each of the
  four templates argument by argument. `hash` is computed by the browser
  from the group row it showed, never copied from `host.info`.
- Storage: one more host table, `accept`, with `g TEXT PRIMARY KEY` and
  `hash TEXT NOT NULL`. The row is removed when the node's last AI leaves
  the group.
- `host.info` (`contracts/datachannel.md` section 7) gets one list,
  always present, with one entry per group in which this node has an AI:
  `"accept":[{"g":"Qm1…","want":"<hash>","have":null}]`. `want` is the
  hash of the row the node holds now; `have` is the stored hash, or
  `null`. The client asks its person when the two differ.

## 4. What is refused, and the wait `accept`

While `have` is not `want` for a group, on that node:

| Operation | What happens |
| --- | --- |
| `run_checks` | the tool fails with `accept`; nothing runs, the hard checks included |
| `push_branch`, `submit_task` | the tool fails with `accept` before anything is pushed or posted |
| reading hosted checks | not run; it starts again after acceptance |
| the `merge` item | not taken and not acknowledged; it is taken after acceptance by the usual rule (`contracts/datachannel.md` section 6, "Still current") |
| the push before a task changes hands (`contracts/local.md` section 7) | not run. The worktree is removed as that row says; commits that were never pushed stay on the task branch of this node's clone |

Cloning, fetching, worktrees, the review checkout and the facts of a
hand-back run no command of the group and are not gated.

- **Error code** `accept`, host only (one more row of
  `contracts/datachannel.md` section 9). The tool error text is
  `accept: the owner of this computer has not accepted this project's
  commands yet; end this turn, a new one follows when they have`.
- **The fact for the group.** The first time the node refuses something
  for a task under the current `want`, it reports that, and later that
  the owner has accepted. One new `task` op (`contracts/commands.md` 1.5):

| `op` | Fields | By | Result |
| --- | --- | --- | --- |
| `accept` | `id`, `ok` (bool) | host of the assignee | — |

- **Task machine** (`contracts/task-machine.md` section 3): two more
  rows, and one more value of `wait.kind` while `active`, `accept`.

| # | Command, by | Allowed when | Task becomes | Items | Counters |
| --- | --- | --- | --- | --- | --- |
| CF-1 | `task accept` with `ok: false`, by host | `active` | wait `accept`, `who` = the owner of the assignee | — | — |
| CF-2 | `task accept` with `ok: true`, by host | `active`, wait `accept` | wait cleared | — (the host continues itself) | — |

- There is no `wait.msg`: nothing is a card.
- At step `merge`, CF-1 and CF-2 change `wait` and nothing else. They are
  added to the rows that may change a task at that step
  (`contracts/task-machine.md` section 2, "point of no return").
- Row 11 with another assignee clears a wait `accept`: the new assignee's
  host reports its own.
- Another wait can replace `accept` meanwhile (a card, a pause). The host
  sends CF-2 only for a task whose row still shows wait `accept`.
- **After acceptance** the host sends CF-2 for each such task of its AIs,
  then: gives each session whose tool was refused one turn, with the
  header `resume · instruction` (`contracts/drivers.md` section 3) and the
  text `The owner of this computer accepted the project's commands.
  Continue.`; takes a held `merge` item; reads hosted checks again.
- The gate is the host's. The wait only tells the group; a home that
  never writes it changes nothing about what runs.

## 5. Forge templates

`group.forge` gets a third value, and the group row one optional field
(`contracts/rows.md` section 1):

| Field | Values |
| --- | --- |
| `forge` | `github`, `none` (as in step 1), or `custom`: the group's own templates |
| `templates` | optional: present exactly when `forge` is `custom`. An object with the four keys `push`, `open`, `checks`, `merge`, each a list of strings |

`forge: github` stays what `contracts/local.md` section 7 says, command
for command; it is not rewritten as templates. `group create`
(`contracts/commands.md` 1.1) carries `templates` exactly when its
`forge` is `custom`; `group set` gets `templates` among its "any of"
fields, refused with `bad_request` unless the group's `forge` is
`custom`. `forge` itself is set at creation only, as in step 1.

A template is an argument list, never a shell string, and never goes
through a shell: the first entry is the program, the others are its
arguments, each handed over as one argument whatever it holds.

- **Places.** Inside an argument, each of the six texts below is replaced
  by its value, in one pass; a value is never searched for places. Every
  other text is literal, braces included, and there is no escape.

| Place | Value | Allowed in |
| --- | --- | --- |
| `{branch}` | the task branch, `task/<num>-<slug>` | all four |
| `{base}` | the name of the base branch | all four |
| `{commit}` | the full id of the commit: the head for `push` and `open`, the submitted commit for `checks` and `merge` | all four |
| `{request}` | the number of the task's request, `task.pr` | `checks`, `merge` |
| `{title}` | the task's title | `open` |
| `{summary}` | the `summary` argument of `submit_task` | `open` |

- **Rules for a template.** Each is checked by the home on `group create`
  and `group set` (`bad_request`), and again by a host on the row it
  holds before it runs anything, because the home may be someone else's
  (the tool fails with `bad_request`; a `merge` is reported as `task
  conflict` with that detail):
  - all four keys are there, and each list has at least one entry;
  - the first entry holds no place, no `/` and no `\`: it is a program
    name looked up on the owner's `PATH`, like `git` and `gh`, never a
    file of the repository;
  - a place appears only in a template that allows it;
  - no argument begins with `{title}` or `{summary}`. Those two are text
    an AI wrote, and an argument that begins with one could be read as an
    option. They are written after literal text, as in `--title={title}`.
- **On Windows** the program must resolve to an `.exe` file. A `.cmd` or
  `.bat` file is started through the command interpreter, which reads
  the arguments again, so it is refused with `cli` `[unverified]`.
- **Credentials.** A template runs as `git` and `gh` do in step 1: as the
  owner's own user, signed in by the owner. The node sets no token and
  reads no credential (section 8, point 1).

## 6. Running a template

Names as in `contracts/local.md` section 7. With `forge: custom` these
replace the forge commands of that section; everything else there stays.

| Template | Runs when | In folder | Outcome |
| --- | --- | --- | --- |
| `push` | `push_branch`; first step of `submit_task`; before a task changes hands | `<wt>` | exit 0: pushed. Else the tool fails with `cli` and the output |
| `open` | in `submit_task` after `push`, only when the task row has no `pr` | `<wt>` | exit 0, and standard output names the request (below). Else `cli` |
| `checks` | every 30 s while the task is at `cloud_check` | `repo` | exit 0: passed. Exit 8: not finished, read again. Any other: failed. `task cloud` is sent on passed or failed, with one item named `checks` whose `detail`, on failure, is the end of the output |
| `merge` | on the `merge` item | `repo` | exit 0: merged. Else `task conflict` with the output |

- **`push`** must publish the task branch under its own name on `origin`:
  a task that changes hands is continued from `origin/<br>`. Never with a
  force option; the node cannot see that, the owner who accepts it can.
- **`open`.** The request is read from standard output: the last line
  that, without surrounding spaces, begins with `https://` or `http://`
  is `pr_url`, and `pr` is the decimal number that ends its path. No such
  line, or no number: `cli`. `[documented]` `gh pr create` prints that
  URL; `[unverified]` for every other forge program.
- **`checks`.** Exit status 8 for "not finished" is the convention of
  `gh pr checks` `[documented]`. A forge whose program answers otherwise
  needs a small program that follows it, installed on each machine like
  the forge program. With no hosted checks, a program that exits 0.
- **Which commit.** Step 1 reports hosted checks only from an answer that
  names the submitted commit. A template has `{commit}` for that; the
  node cannot check that the program used it. Limit: a template that
  asks by request or by branch can answer for the previous head just
  after a new submission.
- **`merge`** should refuse when the request's head is not `{commit}`,
  where the forge program can (`gh pr merge --match-head-commit`). After
  exit 0 the node runs `git -C repo fetch origin` and reports `task
  merged` with `merge_commit` = `git -C repo rev-parse origin/<base>`.
  Limit: another merge that lands in between is named instead.
- **A node that restarts with a merge it took and did not report** makes
  no second attempt, as in step 1, and looks with Git alone. After a
  fetch, the task is merged when merging `<commit>` into `origin/<base>`
  would change nothing: `git -C repo merge-tree --write-tree
  origin/<base> <commit>` exits 0 and prints the tree of `origin/<base>`.
  Merged: `task merged`. Not: `task conflict`. `[verified]` on Git 2.56
  for a squash merge, before it, after a later commit on the base, and
  for a conflict (exit 1). `[documented]` the option needs Git 2.38.
- **A request opened and not reported** (the node stopped in between):
  the next `submit_task` runs `open` again, and the forge program's
  refusal is what the AI and the owner see.

## 7. Checks in a sandbox (D-12, spike S-10)

S-10 `[verified]`: Linux bubblewrap ran a Go build and test and 988 web
tests and blocked private files, host paths, host ports and the network.
macOS `sandbox-exec` (deprecated by Apple) works in part; see below.

- **Where.** Linux: `bwrap --unshare-all`. macOS: `sandbox-exec`, whose
  profile grants write to `/dev/null`. Only the worktree is writable.
- **Loopback.** Linux: the private network namespace, so a check may
  listen on `localhost`. macOS has none, and allowing `localhost` would
  expose the host's services: blocked, and a check that needs a port fails.
- **The check strings run unchanged** (HS-5, `contracts/local.md` section
  7): the sandbox wraps the process that runs them and needs no rewrite.
- **Dependencies** are installed inside the sandbox, offline from the
  lockfile, where the toolchain allows. An install outside it runs
  postinstall scripts unconfined; the prompt of section 3 says so.
- **Fail closed.** If the sandbox cannot start (AppArmor can block user
  namespaces on Ubuntu 24.04), `run_checks` runs nothing and says why.
- **Windows** has no sandbox, a stated limit: checks run as in step 1,
  under the acceptance of section 1, and the prompt says so.
- Not decided: whether forge templates (network, sign-in) run inside it.

## 8. Open points

Unit P-4 freezes the shapes of sections 2 to 5; code is T2-9 (acceptance
and the gate), T2-8 (templates) and T2-12 (sandbox).

1. **Needs the coordinator.** C-11 (`architecture.md` section 1) and
   R-137 let the node run only `git` and `gh` for the forge; a template
   names another program. Read here as "only forge programs the owner
   installed and signed in, through a list the owner accepted; never a
   token or a forge API called by the node". C-11 and R-137 need that
   widening for step 2, or D-10 cannot be built.
2. Every group asks once per machine, also a group of one person on one
   machine. An exception would need a signed group row.

## 9. Not verified

- No forge program was run. `gh pr create` and `gh pr checks` are cited
  from their documentation; for other programs, whether `open` prints a
  URL and which exit statuses `checks` gives is unknown.
- How Go starts a program from an argument list on Windows, and what it
  does with `.cmd` and `.bat` files.
- That a held `merge` item and a wait `accept` at step `merge` fit every
  case of the "point of no return" rule; only the contract was read.
- The hash was computed with Python only, not the Go node or a browser.
