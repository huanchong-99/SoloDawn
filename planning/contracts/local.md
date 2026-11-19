# Contract: local interfaces

Everything a node does on its own machine that another part depends on: the
command line, the local socket, where projects live, which Git commands run,
the outbound secret check, self-update. How the CLIs are started and what a
turn looks like is in `contracts/drivers.md`.

Markers: `[verified]` observed in the driver experiments; `[documented]` in
the vendor's documentation but not run by us; `[unverified]` an assumption
that a spike must settle (`steps/step-1.md`).

## 1. Command line

| Command | Does |
| --- | --- |
| `solodawn node` | runs the node in the foreground |
| `solodawn rendezvous …` | runs the rendezvous service (`contracts/rendezvous.md`) |
| `solodawn pair <secret> [--name <label>]` | starts the node if needed, pairs it, prints the result (`flows/pairing.md` A) |
| `solodawn pair-browser` | prints a pairing link for a new browser |
| `solodawn open` | pairs and opens the local browser on the node's own web client |
| `solodawn status` | prints version, device fingerprint, connections and their paths, open sessions |
| `solodawn service install`, `service remove` | registers or removes start-at-login for the current user (launchd agent, systemd user unit, Windows scheduled task) |
| `solodawn mcp --token <t>` | the MCP shim; started by a CLI, not by people |
| `solodawn version` | prints the version |

Every command except `node`, `rendezvous` and `mcp` talks to the running node
through the local socket and exits non-zero with one line of text on failure.

## 2. Data directory

The per-user application data folder of the system, subfolder `solodawn`:

```
node.db            SQLite database (contracts/storage.md)
device.key         the device's private key, readable by the user only
sock               local socket
projects/<group>/repo/            the clone; folder of group sessions
projects/<group>/wt/<num>-<slug>/ worktree of a task; folder of its task session
projects/<group>/review/<num>/    read-only checkout; folder of a review session
logs/node.log      rotated at 10 MB, one generation kept
```

A node has **one clone per group**, shared by every AI on that node that is
in the group. `host.join` clones only if the folder does not exist yet.

## 3. Local socket

A Unix domain socket at `sock`, mode 0600. `[unverified]` Unix sockets
through Go on the supported Windows versions; spike S-5. One JSON line opens
a connection:

- `{"mcp":"<session token>"}`: the rest of the connection is the MCP
  JSON-RPC stream of that session. The shim copies its stdin to the socket
  and the socket to its stdout and does nothing else.
- `{"cmd":"pair","secret":"…","name":"…"}` and the other commands of section
  1: the node answers with JSON lines, the last one
  `{"done":true,"ok":true}` or `{"done":true,"error":"…"}`.

A session token is 16 random bytes created when the node starts a CLI
process and forgotten when that process exits.

## 4 to 6. Moved

Starting the CLIs, the turn format and the instructions are now in
`contracts/drivers.md` (its sections 1, 3 and 4). The numbers of the
sections below are unchanged.

## 7. Git and check commands

**Credentials.** The node runs only the `git` and `gh` command-line
programs that the owner has installed and signed in on this machine, as the
owner's own user. It never reads, stores, copies or forwards a token or a
credential: it opens no credential file, sets no token in an environment
variable or an argument, and calls no forge API directly. If `git` or `gh`
is not signed in, the command fails and its error text is what the AI and
the owner see (R-137, C-11).

Names used below: `<wt>` is `wt/<num>-<slug>`; `<br>` is
`task/<num>-<slug>`; `<base>` is the **name** of the base branch (for
example `main`); `<base-ref>` is what to branch from and diff against:
`origin/<base>` with a remote, `<base>` without one. Nothing fetches when the
group has no remote.

| When | Command |
| --- | --- |
| first AI of this node joins the group | `git clone <repo> projects/<group>/repo` |
| a group session wakes (with a remote) | `git -C repo fetch origin`, `git -C repo merge --ff-only origin/<base>` |
| task session created | with a remote `git -C repo fetch origin`; then `git -C repo worktree add ../<wt> -b <br> <base-ref>` |
| task session created for a task taken over from another AI | with a remote: fetch, then `git -C repo worktree add ../<wt> -b <br> origin/<br>` if that branch exists there, else as for a new task. Without a remote: `git -C repo worktree add ../<wt> <br>` |
| `push_branch` | `git -C <wt> push origin <br>`; never with a force option |
| `run_checks` | refused unless `git -C <wt> status --porcelain` is empty. Then each entry of `group.checks`, through the system shell, in the worktree, stopped at `limits.turn_minutes`. The commit recorded is `git -C <wt> rev-parse HEAD` |
| hard check: secrets | the patterns of section 8 over the added lines of `git diff <base-ref>...HEAD` |
| hard check: tests | fails when the diff changes source files and no test file |
| `submit_task` | refused unless the tree is clean and HEAD is the checked commit. With a remote: the push above, then `gh pr view <br> --json number,headRefOid`, or if there is none `gh pr create --head <br> --base <base> --title <task title> --body <summary>` |
| hosted checks | every 30 s, one request: `gh pr view <pr> --json headRefOid,statusCheckRollup`. A result is reported only from an answer whose `headRefOid` is the submitted commit `[unverified]` that both fields come back in one answer; else `gh api` for the check runs of that commit |
| review session created or reused | with a remote `git -C repo fetch origin`; first round `git -C repo worktree add --detach ../review/<num> <commit>`, later `git -C review/<num> checkout --detach <commit>`; then `git diff <base-ref>...<commit>` written to `review/<num>/.solodawn-review.diff` |
| merge, with a remote | `gh pr merge <pr> --squash --delete-branch --match-head-commit <commit>` `[documented, round 1]` |
| merge, no remote | only if `git -C repo rev-parse <br>` is `<commit>`: `git -C repo merge --squash <commit>` and a commit on `<base>` whose message names the task and the commit; on failure `git -C repo reset --merge` |
| node restarts with a merge it took and did not report | no second attempt. It looks: `gh pr view <pr> --json state,mergeCommit`, or without a remote the last commit message on `<base>`. Merged: `task merged`. Not merged: `task conflict` |
| merge failed, or the head is not the reviewed commit | `task conflict` with the command's output |
| task given to another AI | with a remote, first `git -C <wt> push origin <br>`; then as the next row |
| task merged, cancelled, or given away | `git -C repo worktree remove ../<wt>`; the reviewer's node removes `review/<num>` |
| hand-back facts | in the session's folder: `git rev-parse HEAD`, `git status --porcelain`, `git log <start>..HEAD --format='%h %s'`, `git diff --shortstat <start>` |
| statistics | `git diff --shortstat <base-ref>...<commit>` |

- Checks are about a commit, so they need a clean tree: the commit that is
  checked is the commit that is submitted, reviewed and merged
  (`contracts/task-machine.md` section 4).
- The node runs exactly the strings in `group.checks`, as the owner, outside
  the CLI's sandbox (decided, `risks.md` Q-19). Level 2 limits the AI's own
  actions, not what the project's checks execute.
- **Environment of a check command**: only an allow list of variables is
  passed on, nothing else from the node's environment. The list: `PATH`,
  `HOME`, `USER`, `LOGNAME`, `SHELL`, `LANG`, every `LC_*`, `TZ`, `TMPDIR`,
  `TEMP`, `TMP`; on Windows also `SystemRoot`, `ComSpec`, `PATHEXT`,
  `USERPROFILE`, `HOMEDRIVE`, `HOMEPATH`, `APPDATA`, `LOCALAPPDATA`. No key
  or token variable of any AI tool or forge reaches a check. A project that
  needs another variable sets it inside its own check command; a toolchain
  that needs a prepared environment (a Windows C or C++ build, for
  instance) is initialised by that command too.
- "Fake tests" has no command; it is a veto item in review.
- Both hard checks always run (R-103). There is no setting to turn one off.

## 8. Outbound secret check (R-137)

One fixed list of patterns, in one source file, used in two places: the hard
check on diffs (section 7) and everything an AI sends to the group.

Patterns:

- a PEM private-key header;
- well-known credential formats, recognised by prefix and length (the list
  is short and is part of the code review);
- environment-file content: three or more lines of `NAME=value` in a row
  where a name contains `KEY`, `SECRET`, `TOKEN` or `PASSWORD`.

Applied on the AI's own node, before signing, to:

- every text argument of the tools that send to the group: `post`, `ask`,
  `reply`, `start_task`, `submit_task`, `submit_review`, `note_put`,
  `request_decision`, `propose_brief`;
- the request text and the justification of an approval
  (`flows/approval.md` step 4).

On a match the tool fails with `secret: this looks like a secret; remove it
and send again`, and nothing is sent. Messages have no attachments in step
1; when they do, the same check applies to them and to their file names.

This is a net for accidents, not a guarantee. It has no configuration and
is not applied to what people type.

## 9. Self-update (R-151)

- **Source**: the releases of this repository. Each release holds one file
  per target named `solodawn_<version>_<os>_<arch>` (with `.exe` on
  Windows) and one `SHA256SUMS`.
- Once a day the node reads the latest release. If it is newer, the node
  downloads the file for its system, compares its SHA-256 with the line in
  `SHA256SUMS`, and replaces its own binary.
- It restarts only when no session is `working` or in `takeover`.
- A mismatch is logged and shown in `host.info` (`update`); nothing is
  replaced.

## 10. Local web client

`http://localhost:47800/` serves the embedded web client. Requests whose
`Host` header is not `localhost` or `127.0.0.1` are refused. The page gets
no special trust: it pairs like any browser (`solodawn open`).
