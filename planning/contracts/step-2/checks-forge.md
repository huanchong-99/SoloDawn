# Contract (step 2): forge templates; showing a group's commands

How a group names the commands of a forge other than GitHub (D-10), and
where a machine owner sees the commands a group runs on their machine. Rules:
R-92, R-103, R-137. It adds to these files of `contracts/`: `local.md`
sections 2 and 7 (`forge: custom` has a remote), `commands.md`, `rows.md`
section 1, and changes the meaning of no step 1 shape. It is folded into them
when step 1 is built, and is the record until then. Markers as in `local.md`.

## 1. Showing the commands

There is no acceptance step and no gate (cut on 2026-11-22: design points
D-3 and D-12, units T2-9 and T2-12). In a team an admin who is not the
machine's owner sets the checks and forge commands, which each assignee's
node runs as its owner, outside any sandbox (`risks.md` K-8). The web client
shows them; nothing else changes:

- **Where.** In the dialog where an owner joins an AI to a group
  (`host.join`), and in the group info: the group's `repo` and `base`,
  `forge`, each check command in full and, for `forge: custom`, each
  template argument by argument.
- The sentence beside them: these commands run on this computer as you,
  outside any sandbox, whenever an AI of yours works on a task of this
  group; join only groups whose commands you would run yourself.

## 2. Forge templates

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
  and `group set` (`bad_request`):
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
- **Credentials.** A template runs as `git` and `gh` do in step 1, signed
  in by the owner; the node sets no token and reads no credential (4.1).

## 3. Running a template

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
  force option; the node cannot see that, the owner who reads it can.
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
  no second attempt, as in step 1, and reports `task conflict`; the owner
  looks at the request and cancels or reassigns the task.
- **A request opened and not reported** (the node stopped in between):
  the next `submit_task` runs `open` again, and the forge program's
  refusal is what the AI and the owner see.

## 4. Settled points

Unit P-4b freezes the shapes of section 2; code is T2-8.

1. **Other forge programs.** R-137 (`product-rules.md`) and C-11
   (`architecture.md` section 1) were widened, as D-10 cannot be built
   otherwise: the node runs a forge program other than `git` and `gh` when
   the group's templates name it, and the client shows the templates
   before the owner joins an AI to the group (section 1). No token, no
   forge API called by the node.

## 5. Not verified

- No forge program was run. `gh pr create` and `gh pr checks` are cited
  from their documentation; for other programs, whether `open` prints a
  URL and which exit statuses `checks` gives is unknown.
- How Go starts an argument list on Windows, with `.cmd` and `.bat` files.
