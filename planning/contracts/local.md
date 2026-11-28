# Contract: local interfaces

Everything a node does on its own machine that another part depends on: the
command line, the local socket, the folders AIs work in, the outbound
secret check, self-update. How the CLIs are started and what a
turn looks like is in `contracts/drivers.md`.

Markers: `[verified]` observed in the driver experiments or in a spike;
`[documented]` in the vendor's documentation but not run by us;
`[unverified]` an assumption that a spike must settle (`steps/step-1.md`).

## 1. Command line

| Command | Does |
| --- | --- |
| `solodawn node` | runs the node in the foreground, serving the web client and built-in `/rv` signalling on `http://localhost:47800` (falling back to a free port when `47800` is in use); `node --help` and `node -h` print node startup help to stdout and exit 0 |
| `solodawn rendezvous …` | runs the rendezvous service (`contracts/rendezvous.md`) |
| `solodawn pair <secret> [--name <label>] [--rendezvous <url>]` | starts the node if needed, pairs it, prints the result (`flows/pairing.md` A). Without `--name` the label is the computer's host name; without `--rendezvous` the node uses its configured rendezvous, or the official one. A node that already has an owner refuses, and its one line points to `solodawn pair-browser` |
| `solodawn pair-browser` | prints a pairing link for a new browser |
| `solodawn open` | pairs and opens the local browser on the node's own web client |
| `solodawn group export <group id> <file> [--to <device id>]`, `solodawn group import <file>` | step 2: write a group to a file, as a move (`--to`) or a copy, or install one (`contracts/step-2/export.md` section 1) |
| `solodawn status` | prints version, device fingerprint, connections and their paths, open sessions |
| `solodawn service install`, `service remove` | registers or removes start-at-login for the current user (launchd agent, systemd user unit, Windows scheduled task) |
| `solodawn mcp --token <t>` | the MCP shim; started by a CLI, not by people |
| `solodawn mcp --member <id>`, `solodawn wait --member <id> [--timeout <s>]`, `solodawn done --member <id> --run <n> [--failed <text>]` | step 3, for an external AI and its runner: the shim under a member id, taking an item, ending a run (`contracts/step-3/external.md` sections 3 and 5) |
| `solodawn version` | prints the version |
| `solodawn --help`, `solodawn -h`, `solodawn help` | prints the command summary, including how to start `solodawn node` and `http://localhost:47800`, to stdout and exits 0 |

`version` prints the version of its own binary, `help` (`--help`, `-h`, `node
--help`, `node -h`) prints CLI usage to stdout and exits 0, and `service
install` and `service remove` write or delete the start-at-login entry
themselves; these need no running node (`install.md` runs `version` before
pairing). Every other command except `node`, `rendezvous` and `mcp` talks to
the running node through the local socket and exits non-zero with one line of
text on failure. Unknown commands, unknown flags, or extra unexpected
arguments print `usage: solodawn <command> [arguments]` to stderr and exit 2
(`rendezvous` prints its flag usage to stderr and exits 2).

## 2. Data directory

The per-user application data folder of the system, subfolder `solodawn`:

```
node.db            SQLite database (contracts/storage.md)
device.key         the device's private key: the 32-byte Ed25519 seed and
                   nothing else, readable by the user only (mode 0600)
sock               local socket
logs/node.log      rotated at 10 MB, one generation kept
```

The data directory holds no project: an AI works in a folder of its owner
(section 7). A folder `projects/` that `v3.0.0-rc.1` left there is not
read, changed or removed (`contracts/storage.md` section 7).

## 3. Local socket

A Unix domain socket at `sock`, mode 0600, in the data directory. On Unix
the node creates that directory with mode 0700; an existing one must be a
directory owned by the current user, and its mode is set to 0700, or the
node refuses to start. The socket is bound under umask 0077, so it is never
open to others, not even before its mode is set to 0600. On Windows, where a mode
does not limit who connects, the directory (when the node creates it) and
the socket have instead a protected DACL with one entry: full access for
the owner. The test there is that another account cannot connect.

`[verified]` in spike S-5, on Windows Server 2022 with Go 1.26: a Go
listener and a Go client exchange lines over a Unix socket at `sock`; closing
the listener removes the file and the name can be bound again; `chmod 0600`
changes neither the DACL nor the reported mode; with the protected DACL set
on the socket right after binding, the owner connects and another local
account, not an administrator, is refused with "access denied".
`[unverified]` the other supported Windows versions.

**One node per data directory.** Before it touches `sock`, a starting node
takes an exclusive lock on the file `lock` in the data directory (`flock`
on Unix, `LockFileEx` on Windows) and holds it until it closes its socket,
when it releases it before closing the file; the system releases it when
the process ends. When another process holds the lock, a node is already
running: the new one says `a node is already running` in one line and exits
without binding. Any other failure to lock, such as a file system without
locks, is reported as that error, with the path of `lock`. With the lock, a `sock` that exists was left by
a node that stopped: the node removes it and binds.

One JSON line opens a connection:

- `{"mcp":"<session token>"}`: the rest of the connection is the MCP
  JSON-RPC stream of that session. The shim copies its stdin to the socket
  and the socket to its stdout and does nothing else.
- `{"cmd":"pair","secret":"…","name":"…"}`, with `"rendezvous":"<url>"`
  when the option was given, and the other commands of section 1: the
  node answers with JSON lines. Each line that the command prints
  is one line `{"text":"…"}`; the last line is `{"done":true,"ok":true}` or
  `{"done":true,"error":"…"}`.

The first line is at most 64 KiB and must arrive within 10 s; after it the
connection has no deadline. A first line that is neither, is longer, or
does not arrive in time gets one line `{"done":true,"error":"…"}`, and the
node closes the connection.

The command reads lines until one has `"done":true`. That line must carry
`"ok":true` or an `error`; a last line with neither is a failure of the
command, like an error.

A session token is 16 random bytes created when the node starts a CLI
process and forgotten when that process exits. A connection whose session
token is unknown or forgotten is closed at once.

From step 3 the node also takes, in `mcp`, the member id of an external AI
it hosts, and the commands `wait` and `done`
(`contracts/step-3/external.md` sections 3 and 5).

## 4 to 6. Moved

Starting the CLIs, the turn format and the instructions are now in
`contracts/drivers.md` (its sections 1, 3 and 4). The numbers of the
sections below are unchanged. Section 7 held the Git and check commands
until the decision of 2026-11-26.

## 7. Workspaces

The node runs no `git`, no `gh` and no command of a project. It creates no
branch, no clone and no worktree, and it changes no file of a project
itself: everything in a folder is done by the AI, through its own CLI
(owner's decision of 2026-11-26).

**The folder of a session.** Every session of an AI in a group (its group
session, each task session and each review session, `sessions.md`) runs in
that member's **workspace**: the folder its owner gave it when the AI was
added (R-11).

- `host.join` carries the folder (`contracts/commands.md` section 2). The
  node takes an absolute path at which a folder exists now, resolves its
  symbolic links, and keeps the resolved path in the row of the AI's group
  session (`contracts/storage.md` section 3). Every later session of that
  AI in that group copies it from there. The host does not read it from the
  member row, which the home keeps for people and other members to read.
- The node creates no folder, prepares nothing in one, and removes nothing
  from one when a session ends, a task is done or cancelled, or an AI
  leaves.
- **Before a CLI starts** for a session, in structured mode or in a
  terminal, the host checks that the folder exists. A folder that is gone
  (deleted, renamed, on a drive that is not mounted) fails the start: the
  host reports `turn_interrupted` with `crash` and the path in `detail`,
  and in a task session also `task fail`; a takeover is refused with `cli`
  and the path (`flows/takeover.md` E). Nothing is retried; when the folder
  is back, the next item works.

**One folder, one group.** A folder is the workspace of AIs of one group
only. `host.join` answers `conflict`, naming the group, when the resolved
folder is that of the group session of an AI of another group on this node.
The host keeps this rule, because only the host knows every group on its
computer, whichever home each has. The check and the writing of the new
group session's row are under one lock, and joins on one node run one at a
time. Inside a group the folder may be shared by several AIs; the home then
gives it one task at a time (`contracts/task-machine.md` section 2). Paths
are compared as resolved text: two spellings of one folder that differ only
in case count as two folders (`risks.md` K-35).

**The folders a CLI was used in.** `host.workspaces`
(`contracts/datachannel.md` section 7) lists, for one CLI, the folders it
was used in on this computer, so that the person can choose one. The four
finders are the package `internal/host/workspaces`, which imports nothing
from `host`; each takes the user's home folder (for Codex and OpenCode, a
function that asks the CLI's server) and returns folders with the time of
the newest session in each.

| CLI | Source | Status |
| --- | --- | --- |
| Claude Code | the session files `~/.claude/projects/<name>/*.jsonl` (`CLAUDE_CONFIG_DIR` in place of `~/.claude` when set). For each folder there, the newest file is read line by line until a record has `cwd`, for at most 200 lines; that value is the folder. The folder's own name cannot be used: it is the path with every character outside ASCII letters and digits replaced by `-` | `[verified]` on one macOS computer with Claude Code 2.1.311: all 23 folders that held sessions gave a `cwd`, on line 3 in most files and at the latest on line 48. The file layout is not a documented interface |
| Codex | the app-server request `thread/list` with `sortKey` `updated_at` and an empty `modelProviders` (every provider; omitted, it lists only the configured one), through the host's app-server connection (as `model/list` is probed), paged by `nextCursor` until it is null: each thread's `cwd` is the folder and `updatedAt` (seconds) its time. `sourceKinds` is left out, so the threads of interactive sources (`cli`, `vscode`) are listed, not those of `codex exec`. Its own interface: it lists the compressed rollouts (`.jsonl.zst`) too, and does not depend on the file layout | `[verified]` in the generated schema of Codex CLI 0.164.0 and by a run on one macOS computer: 40 threads, 27 folders, newest first, the same with and without `modelProviders` there (one provider); `[source]` the provider filter, `app-server/src/request_processors/thread_processor.rs:5919-5929` of `rust-v0.164.0` |
| OpenCode | `GET /api/session?order=desc&limit=1000` (newest created first) of `opencode serve`, then `&cursor=` with `cursor.next` until a page is empty, through the server the driver already starts to list models; the finder gets the fetch as a function. Each session's `location.directory` is the folder and `time.updated` (milliseconds) its time. Version 1.19.6 has no `/api/project` | `[source]` OpenCode v1.19.6: the route in `packages/protocol/src/groups/session.ts`, the fields in `packages/schema/src/session.ts`, the listing of every session without `directory` or `project` in `packages/core/src/session.ts` (`list`) |
| Qwen Code | the session files `projects/<sanitized-cwd>/chats/*.jsonl` under `QWEN_RUNTIME_DIR`, else `QWEN_HOME`, else `~/.qwen`, read as for Claude Code: every record has `cwd` | `[documented]` the path ([headless documentation](https://qwenlm.github.io/qwen-code-docs/en/users/features/headless/)); `[source]` Qwen Code v0.28.6, not run: `cwd: string` in `ChatRecord`, `packages/core/src/services/chatRecordingService.ts:549`; the directory in `Storage.getRuntimeBaseDir` and `getGlobalQwenDir`, `packages/core/src/config/storage.ts:172-203`. A runtime directory set in Qwen Code's settings is not followed, since the node does not open that file |

- Claude Code and Qwen Code: the finder walks the projects newest first and
  opens a session file only when the request asks for the next folder, so
  the walk stops when the answer is full. Codex and OpenCode: every page is
  fetched, then the sessions are sorted by update time, before the first
  folder; the walk stops early only on the request's side. OpenCode's server
  orders by creation time, so the newest update can be on any page. The time
  of a folder is that of its newest session: the modification time of the
  newest session file, or for Codex and OpenCode the newest update time
  that the server gives for a session there.
- What the request does with what a finder returns: it resolves each folder
  as `host.join` does, so that a path of the list equals the `workspace` of
  a member that has it, and keeps the newer time when two spellings name
  one folder; it leaves out a folder that does not exist now and a folder
  under the node's own data directory (section 2); it orders the rest by
  newest use and cuts the list where the answer would pass the size of a
  `ctl` message (`contracts/conventions.md` section 4).
- A source that cannot be read gives an empty list, not an error: the
  person types the folder. For `external` the list is always empty.
- **What the node does not open**: `~/.claude.json` (it lists projects by
  path, and holds account data and MCP tokens), `~/.codex/auth.json`,
  `~/.codex/config.toml`, Qwen Code's settings file. From a session file
  the node keeps one value, the folder, and nothing else (R-50, R-137).
- Nothing is stored for a folder typed by hand beyond the member: once the
  CLI has run there, the folder is in its own history.

## 8. Outbound secret check (R-137)

One fixed list of patterns, in one source file, used for the text
arguments of tools, the texts of approvals and the text the host writes
(all below). It reads what an AI sends to the group and what the host
writes into a row; it reads no folder and no diff.

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
  `reply`, `start_task`, `submit_task`, `submit_review`, `note_put`, `kb_put`
  (both `name` and `text`), `request_decision`, `propose_brief`;
- the request text and the justification of an approval
  (`flows/approval.md` step 4).

On a match the tool fails with `secret: this looks like a secret; remove it
and send again`, and nothing is sent. Messages have no attachments in step
1; when they do, the same check applies to them and to their file names.

**Text the host writes.** Before the host sends them, the `detail` of `task
fail`, `login_invalid` and `turn_interrupted` go through the same patterns;
each match is replaced by `[redacted]`. A private key goes through its
footer, or to the end of the text without one; a private-key footer without
its header, as in a tail cut inside a key, from the start of the text
through that footer.

This is a net for accidents, not a guarantee. It has no configuration and
is not applied to what people type.

## 9. Self-update (R-151)

Postponed after 1.0: the node does not check for releases or replace its
program, and `host.info.update` stays `null`. A user updates by hand
(`install.md`). What follows is the design for later.

- **Source**: the releases of this repository. Each release holds one file
  per target named `solodawn_<version>_<os>_<arch>` (with `.exe` on
  Windows) and one `SHA256SUMS`.
- **Versions**: a release tag is `vMAJOR.MINOR.PATCH`, three decimal
  numbers. `<version>` in a file name is the exact tag, `v` included:
  `solodawn_v1.4.0_linux_amd64`. A release is newer when its three numbers,
  compared as numbers from left to right, are higher than those of the
  running binary. Pre-release tags, such as `v1.5.0-rc.1`, are ignored. A
  release candidate's tag is `vMAJOR.MINOR.PATCH-rc.N` (N a decimal number
  without leading zeros) and is built like a release. The release build
  stamps its tag into the binary as the version. A
  development build, whose version is no release tag, never updates
  itself.
- Once a day the node reads the latest release. If it is newer, the node
  downloads the file for its system, compares its SHA-256 with the line in
  `SHA256SUMS`, and replaces its own binary.
- It restarts only when no session is `working` or in `takeover`.
- A mismatch is logged and shown in `host.info`; nothing is replaced. The
  shape of `update` there is in `contracts/datachannel.md` section 7.

## 10. Local web client

`http://localhost:47800/` serves the embedded web client; when that port is
taken, the node serves it on a free one, and `solodawn open` uses the actual
address. `/rv` on the same
port is served on the node's network addresses too and is authenticated by
its challenge (`contracts/rendezvous.md` section 1). The page gets no
special trust: it pairs like any browser (`solodawn open`).

[ak]: https://platform.claude.com/docs/en/get-api-key
[oa]: https://github.com/openai/openai-openapi/blob/manual_spec/openapi.yaml
[op]: https://developers.openai.com/cookbook/examples/agents_sdk/context_personalization
[gh]: https://docs.github.com/en/authentication/keeping-your-account-and-data-secure/about-authentication-to-github
[ghc]: https://github.blog/changelog/2021-03-04-authentication-token-format-updates/
