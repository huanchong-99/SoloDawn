# Contract: diagnostics

What a node writes to its log, and what a failing test leaves behind, so
that one occurrence of an intermittent failure names its cause. The owner's
rule of 2026-11-23 (`issues.md`): such a failure must not have to be
reproduced to be understood. The log file, its place in the data directory
and its rotation are in `contracts/local.md` section 2 and do not change.
The units are lane DIAGNOSTICS, `steps/parallel-plan-step-2-3.md` section
4.3.

Markers as in `contracts/local.md`: `[verified]` run by the planning lead,
`[unverified]` an assumption.

## 1. Lines

- A package logs an event through the default logger of the standard
  library's `log/slog`: the event name is the message, everything else is a
  field. There is no new package and no handler. The default handler writes
  through the `log` package, so a line goes where `internal/node` already
  points `log` (`logs/node.log`), and to standard error in a unit test.
  `[verified]` with Go 1.27.4: lines of a logger made with `slog.With`
  before `log.SetOutput` follow the new output, its flags and the level.
- The node sets the flags of `log` to date, time with microseconds, UTC, and
  puts them back when it closes. A line reads:

  ```
  2026/11/23 13:58:47.917306 INFO inbox.take node=ab12cd34 g=Jq3 ai=M7 task="" took=17,18 left=19 paused=false
  ```

- Every line has `node`, the first 8 characters of the node's device id.
  `host`, `home`, `peer` and `node` each keep one logger with that field,
  made where the package is given the node's key or id. It tells apart two
  nodes in one test process, and the logs of two machines.
- A field holds an id, a count, a sequence number, a state, a duration in
  milliseconds (`ms`) or a reason from the lists of section 2 (`why`). A
  device id is cut to 8 characters; other ids are whole. A session is the
  three fields `g`, `ai`, `task`. A line never holds the text of a message
  or a turn, file content, a path inside a project, a token or a key: the
  owner sends this file to others.
- A line is written where the decision is made, by the goroutine that makes
  it, with no condition around it; `slog` drops what is below the level. So
  the order of the lines of one goroutine is the order of what it did.
- The error lines the packages write today with `log.Printf` stay as they
  are.

## 2. Events

Level `info` unless marked. "Where" is the function as it is on `rebuild`
today; the builder may move a line to the nearest place that has the fields.

| Event | Where | Fields besides `node` |
| --- | --- | --- |
| `node.start`, `node.stop` | `node`: the runtime opens, closes | `version`, `os`, `level` |
| `home.dial` | `node`: `connectHome` starts the dial of a home | `home` |
| `home.attach` | `home`: `Loop.Attach` | `dev` |
| `home.detach` | `home`: `Loop.Detach` | `dev`, `why`: `gave_up` (the queue was full or a send failed), `ended` |
| `cmd.reply` | `home`: where a command's answer is queued, a refusal too | `g`, `dev`, `re`, `t` (the key of the command table, `task/checked`), `code` (`ok` or the error code) |
| `rows.send` | `home`: `runGroup.pushRows` | `g`, `seq` (first and last), `to` (connections sent to) |
| `inbox.send` | `home`: `runGroup.deliver`, per connection | `g`, `dev`, `items` |
| `ack.recv` | `home`: `runGroup.ack` | `g`, `dev`, `item` |
| `host.attach`, `host.detach` | `host`: `Host.Attach`; `watchHomeConn` when the connection is done | `home` |
| `sync.begin`, `sync.end` | `host`: `syncOneGroup` | `g`, `home`; at the end `seq`, `ok` |
| `rows.recv` | `host`: `pushRows` | `g`, `home`, `seq` |
| `inbox.recv` | `host`: `handleInboxElem` | `g`, `item`, `ai`, `kind` (the item's `why`), `seen` (delivered before, only acknowledged again) |
| `inbox.hand` | `host`: `tryOpenWait`, an item given to an open wait | `g`, `item`, `wait` |
| `inbox.take` | `host`: `takeEligibleBatch`, when the AI's queue is not empty | `g`, `ai`, `task`, `took`, `left` (item numbers), `paused` (the one pause reading of the batch) |
| `ack.send` | `host`: `sendAck` | `g`, `home`, `item` |
| `turn.begin`, `turn.end` | `host`: one entry in `Listeners` | `g`, `ai`, `task`; at the end `status` |
| `turn.interrupt` | `host`: `claimTurnInterrupt` | `g`, `ai`, `task`, `why` (the runtime's reason: `stop`, `timeout`, …) |
| `proc.start`, `proc.exit` | `host`: a CLI process starts; has exited | `g`, `ai`, `task`, `agent`; `resume` at the start, `why` (the runtime's reason, as for `turn.interrupt`) at the exit |
| `run.open`, `run.close` | `host`: `extOpenRun`, `extEnd` (external AIs) | `g`, `ai`, `run`; at the close `why`: `done`, `crash`, `timeout` |
| `tool` | `host`: `invokeTool`, when the call returns | `g`, `ai`, `task`, `name`, `code`, `ms` |
| `settle.begin`, `settle.end` | `host`: `Host.Settle` | — |
| `sig.send`, `sig.recv` | `peer`: an offer or an answer; a candidate at `debug` | `dev`, `conn`, `kind` |
| `ice.state` | `peer`: the connection state changes | `dev`, `conn`, `state` |
| `ice.pair` | `peer`: the selected pair changes | `dev`, `conn`, `local`, `remote` (candidate types), `proto`, `path` (`direct`, `relay`) |
| `ctl.open`, `conn.close` | `peer`: `ctl` is ready; the connection is done | `dev`, `conn`; at the close `why` (`ice_failed`, `ctl_closed`, `closed`), `ms` open |
| `bg.start`, `bg.stop` | `host`, `home`, `node`: each goroutine a wait group starts (a delivery, a group sync, a poll of hosted checks, a status flush, a connection's pump), and in `host` the goroutines of a watched terminal; not the read loops of a CLI process or a terminal, the first of which `proc.*` covers | `name`; `g` and `task` when the work has them |

A change of path is two `ice.pair` lines of one `conn` with different
`path`. A turn that ran out is `turn.interrupt` with `why=timeout`. A pause
or a resume is the `cmd.reply` of its command.

What each of the failures of 2026-11-23 would have shown (`issues.md`):

| Failure | Lines |
| --- | --- |
| A9.24, the retry commit not submitted | `cmd.reply t=task/checked` logged before the `rows.send` that carries its rows; then `tool name=submit_task` with its refusal |
| A9.24, hosted checks run again | a second `bg.start name=cloud_poll` for the task after the `bg.stop` of the first |
| A9.23, the bulletin before the attach | `tool name=post code=home_unreachable` between `home.dial` and `host.attach` of that home |
| A10.3, the pause read (FLAKY-27) | `inbox.take` with the newer item in `took` and the older in `left` |
| Host teardown | any line of that node, a `bg.stop` too, after `settle.end` |
| A7.9 on Windows | `ice.pair` and `ice.state` of the takeover connection on the next failing run |

## 3. Levels and the switch

- `info` is the default and writes every event not marked `debug`: a
  failure on a machine that was not prepared is still explained.
- `debug` adds the marked lines: ICE candidates. The acceptance harness
  runs at `debug`.
- The switch is the file `logs/level` in the data directory. The node reads
  it once when it starts: the word `debug` sets `debug`; a missing file or
  any other content is `info`. The owner writes or deletes the file and
  starts the node again. `node.start` says which level is on.

## 4. Tests that explain themselves

1. **Waits.** Every wait in `features/` and in the test files of
   `internal/` that can time out fails with what it waited for, how long,
   and what it saw: the frames it received (their `t`, `re`, group and
   `seq`), or the last value it read. The form is
   `timed out after 10s waiting for <what>; saw <n>: <list>`, the list cut
   to its last 20. `features/` has one helper for this; each package under
   `internal/` uses its own existing helpers. A root test beside
   `imports_test.go` fails when a string in a test file starts with
   `timed out` and has no `; saw `.
2. **A failed scenario.** Before any step set's own clean-up runs, the
   acceptance harness writes one folder per failed scenario, and the failure
   names it:
   - `nodes.log`: every line the test process logged during the scenario.
     The harness points `log` at a buffer it empties before each scenario;
     the in-process nodes are told apart by `node`.
   - `goroutines.txt`: the goroutines of the test process
     (`pprof.Lookup("goroutine")`, with stacks).
   - for each node run as a child process: its `logs` folder, and what it
     wrote to standard error. On Unix the harness sends it `SIGQUIT` first,
     so the Go runtime adds that node's goroutines there. On Windows a
     child node has no goroutine dump.
   - the fake CLI log, when the scenario has one.
   The folder is under `$SOLODAWN_TEST_ARTIFACTS`, or under the system's
   temporary folder when that is not set.
3. **Unit tests** need nothing more: the lines go to standard error, which
   `go test` prints for a failing package, and a test binary that runs out
   of time prints its goroutines.
4. **CI.** The `test`, `acceptance-race` and `other-os` jobs of
   `.github/workflows/backend.yml` set `SOLODAWN_TEST_ARTIFACTS` to a folder
   under the runner's temporary folder and upload it when the job fails, as
   `diagnostics-<os>` (`diagnostics-race-<os>` for `acceptance-race`), kept 14 days; no file is no error.

## 5. On the owner's machines

The log is `logs/node.log` and the generation before it `logs/node.log.1`,
in the data directory:

| System | Folder |
| --- | --- |
| macOS | `~/Library/Application Support/solodawn/logs` |
| Linux | `$XDG_DATA_HOME/solodawn/logs`, or `~/.local/share/solodawn/logs` |
| Windows | `%LocalAppData%\solodawn\logs` |

After a failure the owner sends that folder, with the time of the failure;
the times in the log are UTC. There is no command for it: one line of the
system's own shell does it, and `install.md` gives the line for each
system, in the order of the table. Windows has `tar` since Windows 10.

```
tar czf ~/Desktop/solodawn-logs.tgz -C "$HOME/Library/Application Support/solodawn" logs
tar czf ~/solodawn-logs.tgz -C "${XDG_DATA_HOME:-$HOME/.local/share}/solodawn" logs
tar -a -cf "$env:USERPROFILE\Desktop\solodawn-logs.zip" -C "$env:LocalAppData\solodawn" logs
```

## 6. Not built, not verified

- The web client and the rendezvous service log nothing new (coordinator,
  2026-11-23). A connection
  between a browser and a node is seen from the node's side only.
- An installed node has no goroutine dump; the events are meant to make it
  unnecessary (coordinator, 2026-11-23).
- `[unverified]` The events were chosen by reading the entries of
  `issues.md`; none was tried against a failing run. The volume was not
  measured: the estimate is about ten lines per turn and four per inbox
  item, plus the `bg.*` lines, which the two files of 10 MB hold for days of work.
- `[unverified]` The three shell lines of section 5 were not run; whether
  the Windows `tar` line works, and reads the file while the node writes it,
  is not known. The owner's real-machine test I-11 checks it.
