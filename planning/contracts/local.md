# Contract: local interfaces

Everything a node does on its own machine that another part depends on: the
command line, the local socket, where projects live, which Git commands run,
the outbound secret check, self-update. How the CLIs are started and what a
turn looks like is in `contracts/drivers.md`.

Markers: `[verified]` observed in the driver experiments or in a spike;
`[documented]` in the vendor's documentation but not run by us;
`[unverified]` an assumption that a spike must settle (`steps/step-1.md`).

## 1. Command line

| Command | Does |
| --- | --- |
| `solodawn node` | runs the node in the foreground |
| `solodawn rendezvous …` | runs the rendezvous service (`contracts/rendezvous.md`) |
| `solodawn pair <secret> [--name <label>] [--rendezvous <url>]` | starts the node if needed, pairs it, prints the result (`flows/pairing.md` A). Without `--name` the label is the computer's host name; without `--rendezvous` the node uses its configured rendezvous, or the official one. A node that already has an owner refuses, and its one line points to `solodawn pair-browser` |
| `solodawn pair-browser` | prints a pairing link for a new browser |
| `solodawn open` | pairs and opens the local browser on the node's own web client |
| `solodawn status` | prints version, device fingerprint, connections and their paths, open sessions |
| `solodawn service install`, `service remove` | registers or removes start-at-login for the current user (launchd agent, systemd user unit, Windows scheduled task) |
| `solodawn mcp --token <t>` | the MCP shim; started by a CLI, not by people |
| `solodawn version` | prints the version |

`version` prints the version of its own binary, and `service install` and
`service remove` write or delete the start-at-login entry themselves; these
need no running node (`install.md` runs `version` before pairing). Every
other command except `node`, `rendezvous` and `mcp` talks to the running
node through the local socket and exits non-zero with one line of text on
failure.

## 2. Data directory

The per-user application data folder of the system, subfolder `solodawn`:

```
node.db            SQLite database (contracts/storage.md)
device.key         the device's private key: the 32-byte Ed25519 seed and
                   nothing else, readable by the user only (mode 0600)
sock               local socket
projects/<group>/repo/            the clone; folder of group sessions
projects/<group>/wt/<num>-<slug>/ worktree of a task; folder of its task session
projects/<group>/review/<num>/    read-only checkout; folder of a review session
logs/node.log      rotated at 10 MB, one generation kept
```

A node has **one clone per group**, shared by every AI on that node that is
in the group. `host.join` clones only if the folder does not exist yet.

## 3. Local socket

A Unix domain socket at `sock`, mode 0600. On Windows, where a mode does not
limit who connects, the socket has instead a protected DACL with one entry:
full access for the owner. The test there is that another account cannot
connect.

`[verified]` in spike S-5, on Windows Server 2022 with Go 1.26: a Go
listener and a Go client exchange lines over a Unix socket at `sock`; closing
the listener removes the file and the name can be bound again; `chmod 0600`
changes neither the DACL nor the reported mode; with the protected DACL set
on the socket right after binding, the owner connects and another local
account, not an administrator, is refused with "access denied".
`[unverified]` the other supported Windows versions.

**A `sock` left behind.** When the node starts and `sock` exists, it dials
it. If the dial is refused, the file is stale: the node removes it and
binds. If the dial succeeds, a node is already running: the new one says so
in one line and exits without binding.

One JSON line opens a connection:

- `{"mcp":"<session token>"}`: the rest of the connection is the MCP
  JSON-RPC stream of that session. The shim copies its stdin to the socket
  and the socket to its stdout and does nothing else.
- `{"cmd":"pair","secret":"…","name":"…"}`, with `"rendezvous":"<url>"`
  when the option was given, and the other commands of section 1: the
  node answers with JSON lines. Each line that the command prints
  is one line `{"text":"…"}`; the last line is `{"done":true,"ok":true}` or
  `{"done":true,"error":"…"}`.

A first line that is neither gets one line `{"done":true,"error":"…"}`, and
the node closes the connection.

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
`task/<num>-<slug>`, where `<slug>` is the task row's `slug`;
`<base>` is the **name** of the base branch (for
example `main`); `<base-ref>` is what to branch from and diff against:
`origin/<base>` with a remote, `<base>` without one; `<start>` is the commit
noted when a takeover started (`flows/takeover.md` step 5). A group has a
remote when its `forge` is `github`, and none when it is `none`
(`contracts/rows.md` section 1). Nothing fetches when the group has no
remote.

| When | Command |
| --- | --- |
| first AI of this node joins the group | `git clone <repo> projects/<group>/repo`; when the HEAD of `<repo>` is not `<base>`, `git clone --branch <base> <repo> projects/<group>/repo`, so that the clone's checked-out branch is `<base>` |
| a group session wakes (with a remote) | `git -C repo fetch origin`, `git -C repo merge --ff-only origin/<base>` |
| task session created | with a remote `git -C repo fetch origin`; then `git -C repo worktree add ../<wt> -b <br> <base-ref>` |
| task session created for a task taken over from another AI | with a remote: fetch, then `git -C repo worktree add ../<wt> -b <br> origin/<br>` if that branch exists there, else as for a new task. Without a remote: `git -C repo worktree add ../<wt> <br>` |
| task session created for a task that returns to a node which still has the local branch `<br>` | with a remote: fetch, `git -C repo worktree add ../<wt> <br>` (the branch is reused, not created), then `git -C <wt> merge --ff-only origin/<br>`. If the branch has diverged, the worktree is removed again and the session fails with the output of Git. Without a remote: as the row above |
| `push_branch` | `git -C <wt> push origin refs/heads/<br>:refs/heads/<br>`: the full ref on both sides, never a short name; never with a force option |
| `run_checks` | refused unless `git -C <wt> status --porcelain --untracked-files=all` is empty: untracked files always show, whatever the user's Git configuration. Before the status, the assume-unchanged bit, a speed hint that `core.ignoreStat` sets on every path, is cleared in the worktree's index for each path that has it (`git -C <wt> ls-files -v` tags it in lower case; `git -C <wt> update-index --no-assume-unchanged -z --stdin`), so a change shows. Refused when a tracked path has the skip-worktree bit (tag `S` or `s`), as in a sparse checkout, whose content the status never shows; the error names up to 10 such paths. Then each entry of `group.checks`, in order, through the system shell, in the worktree. The time left in the current turn (`limits.turn_minutes`) limits the whole run of these entries together, not each command. The commit recorded is `git -C <wt> rev-parse HEAD` |
| hard check: secrets | the patterns of section 8 over the added lines of `git diff --text <base-ref>...HEAD`, files Git takes as binary included. More than 64 MiB of added lines fail the check with "too large to check" and their size |
| hard check: tests | fails when the diff changes source files and no test file; what counts as each is below the table |
| `submit_task` | refused unless the tree is clean, by the status command of `run_checks`, and both HEAD and `<br>` are the commit of the last `run_checks`, which passed. With a remote: the push above, then `gh pr list --head <br> --base <base> --state open --json number,headRefOid,url,state,baseRefName`. A pull request that is closed (merged included) or whose base is not `<base>` is never reused. If the list is empty: `gh pr create --head <br> --base <base> --title <task title> --body <summary>` |
| hosted checks | every 30 s, one request: `gh pr view <pr> --json headRefOid,statusCheckRollup`; what each read counts as is below the table. A result is reported only from an answer whose `headRefOid` is the submitted commit `[unverified]` that both fields come back in one answer; else `gh api` for the check runs of that commit |
| review session created or reused | with a remote `git -C repo fetch origin`; first round `git -C repo worktree add --detach ../review/<num> <commit>`, later `git -C review/<num> checkout --detach <commit>`; then `git diff <base-ref>...<commit>` written to `review/<num>/.solodawn-review.diff` |
| merge, with a remote | `gh pr merge <pr> --repo <owner>/<repo> --squash --delete-branch --match-head-commit <commit>` `[documented, round 1]`, run from `projects/<group>`, outside the clone; `<owner>/<repo>` is the group's GitHub repository. With `--repo`, gh merges and deletes the remote branch only, whatever its version, and never touches the worktree or the local branch. After the merge the node removes the worktree (row "task merged") and then deletes the local branch with `git -C repo branch -D <br>`, under the clone's lock |
| merge, no remote | only if `git -C repo rev-parse <br>` is `<commit>`: `git -C repo merge --squash <commit>` and a commit on `<base>` whose message names the task and the commit; on failure `git -C repo reset --merge` |
| node restarts with a merge it took and did not report | no second attempt. It knows it took one from the audit row `merge` (task, commit) written in the same transaction as the item's `delivered` row. It looks: `gh pr view <pr> --json state,mergeCommit,headRefOid`, or without a remote the last commit message on `<base>`. Merged, and with a remote `headRefOid` is the reviewed commit: `task merged`. Merged with another head, or not merged: `task conflict` |
| merge failed, or the head is not the reviewed commit | `task conflict` with the command's output |
| task given to another AI | with a remote, first the push of `push_branch`; then `git -C repo worktree remove --force ../<wt>`. Only commits carry over: uncommitted changes in the worktree are discarded. The reviewer's checkout stays |
| reviewer changed | the old reviewer's node removes `review/<num>` |
| task merged | `git -C repo worktree remove ../<wt>`; the reviewer's node removes `review/<num>` |
| task cancelled | `git -C repo worktree remove --force ../<wt>`: uncommitted changes in the worktree are discarded; the branch `<br>` keeps its commits. The reviewer's node removes `review/<num>` |
| hand-back facts | in the session's folder: `git rev-parse HEAD`, `git status --porcelain --untracked-files=all`, `git log <start>..HEAD --format='%h %s'`, `git diff --shortstat <start>`, `git diff --name-only <start>`. The `files` of the hand-back are that list plus the untracked files of the status |
| statistics | `git diff --shortstat <base-ref>...<commit>` |

- **Slug.** The home writes `slug` into the task row once, when it creates
  the task, and never changes it, so a later title edit renames no branch.
  The home is its only producer; hosts read it from the row. From the
  title: lower-case ASCII letters and digits are kept (upper-case ASCII
  letters are lowered); every other run of characters becomes one `-`;
  leading and trailing `-` are trimmed; the result is cut to 40 characters
  and trimmed of `-` again; if nothing is left, it is `task`. Example:
  "Node: report connection path changes" gives
  `node-report-connection-path-changes`.
- **An interrupted squash merge** (no remote), which a restart leaves
  staged in `repo` while `<base>` does not yet record the merge, counts as
  a failed merge. On restart, before anything else touches the clone, the
  host runs `git -C repo reset --merge` under the clone's lock; the row on
  the restart above then reports `task conflict`. HS-7b implements it.
- **One lock per clone.** Every Git command that changes the clone or one
  of its worktrees runs under one lock per clone: `clone`, `fetch`, `merge`
  (fast-forwards, and the local squash merge with its commit or reset),
  `worktree add` and `worktree remove`.
- **A worktree left by a crash.** When the node stopped after
  `worktree add` and before it recorded the session, the item comes again;
  a `wt/<num>-<slug>` that is already a worktree on `<br>` is reused as it
  is, not added again.
- **Hosted checks.** A read is finished when its `headRefOid` is the
  submitted commit and every check in it has completed; a read that fails,
  or that shows another head or a check still running, counts as
  unfinished, and the next read follows 30 s later. An empty rollup (no
  checks) counts as passed only after two consecutive empty reads at least
  30 s apart. 60 minutes after `checks.local.at` of the submitted commit
  the checks count as failed: `task cloud` not passed, whose items are
  those of the last read as they were, followed by
  `{"name":"hosted checks","ok":false,"detail":"hosted checks timed out"}`.
  After a restart the host reads again for every task at step
  `cloud_check` whose assignee runs on it; the 60 minutes still count from
  `checks.local.at`, so the limit left may be slightly short.
- **Text the host writes.** Before the host sends them, the output tails of
  check items, the Git output in `task conflict`, and the `detail` of
  `task fail`, `login_invalid` and `turn_interrupted` go through the
  patterns of section 8; each match is replaced by `[redacted]`. A
  private key goes through its footer, or to the end of the text without
  one; a private-key footer without its header, as in a tail cut inside a
  key, from the start of the text through that footer. A check's last
  64 KiB of output are redacted before the tail is cut from them.
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
- **Hard check "tests"** uses two fixed lists; there is no setting. A
  *test file* has a name that contains `_test.`, `.test.` or `.spec.` or
  starts with `test_`, or lies in a folder named `tests` or `__tests__` at
  any depth; these are matched with exact case, whatever the extension. A
  *source file* is any other file whose last extension, in lower case, is
  one of the list below: the languages whose test files these names cover
  (Go, JavaScript and TypeScript, Python, PHP, Dart, Elixir).

  ```
  .cjs .cts .dart .ex .go .js .jsx .mjs .mts .php .py .ts .tsx
  ```

  A change to a file of any other type does not trigger the check: the
  check neither fails nor passes because of it. A deleted file counts as
  neither a test file nor a source file. Support for projects with
  other conventions is a later option. This is the list of the code's
  `checkCodeExtensions` (`internal/host/checks.go`): the HS-5 list cut to
  those languages.
- "Fake tests" has no command; it is a veto item in review.
- Both hard checks always run (R-103). There is no setting to turn one off.
  They outlive the turn's deadline and have their own limit of 2 minutes; a
  check that reaches it fails with `timed out`. An explicit stop of the run
  (a pause, an interrupt, a shutdown) stops them too, and then nothing is
  reported.

## 8. Outbound secret check (R-137)

One fixed list of patterns, in one source file, used for the hard check on
diffs (section 7), the text arguments of tools and the texts of approvals
(below), and the text the host writes (section 7).

Patterns:

- a PEM or OpenPGP private-key header;
- well-known credential formats, recognised by prefix and length (the list
  is short and is part of the code review). The vendors document the
  prefixes, not a fixed length (GitHub warns that a token's length can
  change), so each is a prefix, not preceded by a letter or digit,
  followed by at least the number of key characters given:

  | Credential | Prefix | Then | Source |
  | --- | --- | --- | --- |
  | Anthropic API key | `sk-ant-` | 32 of `A-Za-z0-9_-` | [Claude docs][ak] |
  | OpenAI API keys, project keys included | `sk-` (`sk-proj-` for a project key) | 32 of `A-Za-z0-9_-` | [OpenAPI spec][oa], [Cookbook][op] |
  | GitHub personal access token (classic) | `ghp_` | 36 of `A-Za-z0-9_` | [GitHub docs][gh], [changelog][ghc]: `A-Za-z0-9_`, and lengths will grow |
  | GitHub fine-grained personal access token | `github_pat_` | 30 of `A-Za-z0-9_` | [GitHub docs][gh] |
  | GitHub OAuth, user, installation and refresh tokens | `gho_`, `ghu_`, `ghs_`, `ghr_` | 30 of `A-Za-z0-9_.-` | [GitHub docs][gh] |
  | AWS access key id | `AKIA`, `ASIA` | 16 of `A-Z0-9` | as HS-6 has it |

  The minimum runs are this contract's choice; the code follow-up is
  HS-6b. `ghs_` includes the stateless `ghs_<app id>_<JWT>` format that
  GitHub began on 2026-04-27, hence `.` and `-`;
- environment-file content: three or more lines of `NAME=value` in a row
  where a name contains `KEY`, `SECRET`, `TOKEN` or `PASSWORD`, matched
  without regard to case (`api_key` matches).

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
- **Versions**: a release tag is `vMAJOR.MINOR.PATCH`, three decimal
  numbers. `<version>` in a file name is the exact tag, `v` included:
  `solodawn_v1.4.0_linux_amd64`. A release is newer when its three numbers,
  compared as numbers from left to right, are higher than those of the
  running binary. Pre-release tags, such as `v1.5.0-rc.1`, are ignored. The
  release build stamps its tag into the binary as the version. A
  development build, whose version is no release tag, never updates
  itself.
- Once a day the node reads the latest release. If it is newer, the node
  downloads the file for its system, compares its SHA-256 with the line in
  `SHA256SUMS`, and replaces its own binary.
- It restarts only when no session is `working` or in `takeover`.
- A mismatch is logged and shown in `host.info`; nothing is replaced. The
  shape of `update` there is in `contracts/datachannel.md` section 7.

## 10. Local web client

`http://localhost:47800/` serves the embedded web client. For the static
files, requests whose `Host` header is not `localhost` or `127.0.0.1` are
refused. `/rv` on the same port is served on the node's network addresses
too and is authenticated by its challenge (`contracts/rendezvous.md`
section 1). The page gets no special trust: it pairs like any browser
(`solodawn open`).

[ak]: https://platform.claude.com/docs/en/get-api-key
[oa]: https://github.com/openai/openai-openapi/blob/manual_spec/openapi.yaml
[op]: https://developers.openai.com/cookbook/examples/agents_sdk/context_personalization
[gh]: https://docs.github.com/en/authentication/keeping-your-account-and-data-secure/about-authentication-to-github
[ghc]: https://github.blog/changelog/2021-03-04-authentication-token-format-updates/
