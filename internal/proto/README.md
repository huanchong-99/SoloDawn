# Protocol wire types

These structs describe the JSON exchanged during signalling, pairing, device-log
transfer, session setup, group reads, commands, the inbox of a host and the host
surface of a node ([`host.go`](host.go): `host.info`, `pty.resize`, the
`groups` push and the host commands of `settings.set`, `agent.set`, `perm.set` and
the takeover). They do
not authenticate peers, apply rows or execute commands. The source of truth is
[the wire contracts](../../planning/contracts/).

An envelope's `p` is a **string**: its original UTF-8 bytes are signed. A batch
can mix row types, so its elements stay as JSON until the consumer reads `r`
(and `gone` for a deletion). Message bodies depend on `kind`. For a known
shape, e.g. an answered call, use `MessageRow[CallBody, CallAnsweredState]`;
a card's state is never `null`. A message that is not a card has the state
`NoCard`, which is `null` and nothing else. The body is fixed at creation; only
the card state changes.

## Commands and items

Step 2 adds `watch.start` (plain success, as in `cmd-ok`), host command
`watch.end`, and item `watch` (always `msg`, never `task`). The required
`agents[].groups[].watchers` user-id list is `[]` without grants.
`watch:<g>:<member>` carries ordered, reliable, binary terminal bytes from
the node only; channel labels have no shared code definitions here.

There is one kind of group (P-8, decision of 2026-11-26): the group row,
`group create` and `group set` carry no kind, repository, branch, forge,
templates or check commands. An AI member carries `workspace`, the absolute
path of its folder on its host, in its row, `member add_ai`, `host.join` and
`agents[].groups[]` of `host.info`. `MemberAddPerson` (`member add_person`)
adds a person the home knows without an invitation; it has `user` and `name`
and no `id`, which is `n`. `HostWorkspacesRequest` and `HostWorkspaces` are
`host.workspaces` and its answer, which has no `t`. `hello` and `welcome`
carry `v` 3.

The task row has `report` (the report message under review) and no slug,
branch, commit, pull request, checks or line counts; `review.report` names
the report a review was about. A submission is a `post` of kind `report`,
and a host sends no task command but `fail` and `preempted`. The bodies of
`report` and `handback` are `EmptyBody`: their text is the account. A manual
pause's `TaskWait` and the `reassigned` state of an approval card
(`ReassignedState`) carry `by`, the member who did it.

Moved errors carry the destination device in `err.home`; codes remain text.

The payload of a command is read in two steps. The receiver reads `t`, and the
`op`, `kind` or `action` beside it, and then decodes the payload into the type
of that command ([`command.go`](command.go)): `GroupCreate`, `TaskStart`,
`Post[BriefBody]`, `Act`. The types hold `t` and those keys as text and do not
judge them; the home's command table does. Every home command starts with
`HomeCommand` (`g` is always there, `as` when a node signs for an AI), every
host command with `HostCommand` (`to` is always there).

- A `set` command carries only what it changes, so each of its fields is a
  pointer: nil is "not sent", and a pointer to `""` or to an empty list is a value.
- An optional text or id is a plain field with `omitempty`, left out when it is
  empty. An empty text that is sent all the same is read as left out. An empty
  id that is sent is refused, like any text that is no id: `"assignee":""`,
  `"as":""`.
- "At least one" is not judged here: a `set` that changes nothing and a
  `task assign` with neither role decode, and the home refuses them
  ([issues](../../issues.md)).
- `Post[B]` holds what the author signs of the body: the row's body type,
  `EmptyBody` for a summary, `ReviewSigned` for a review, `ConfirmSigned` for a
  watch confirmation card. The home adds the rest.
- `MemberStatus[E]` carries a host's fact as one of the event types, which are
  the system bodies without `ai`. Read it as `MemberStatus[SystemCode]` first to
  learn the code.
- An `InboxItem` holds a plain `Item` object (`Item`), with no envelope or
  `cause`. `Msg` beside it stays as JSON, like a row in a batch; `Task` is a
  `TaskRow`.
- `NewNonce` makes a random 16-byte `ID` in canonical base64url spelling
  ([envelope.md, section 1](../../planning/contracts/envelope.md)).

Sender tests pass what their code encoded to `UnexpectedNulls`, with the JSON
Pointers that may be null for that shape (e.g. `/thread`, `/re` and `/state` for
a text message). It returns every other null, in lists as well.

The examples in [testdata](testdata/) are shared with the web client.
`go test ./internal/proto` checks encoding and decoding against them.
Use `go test ./internal/proto -update` only for an agreed change to the wire.

## One answer in Go and in the browser

Every example has one type here and one schema in the web client: the file map
`shapes` in [`shapes_test.go`](shapes_test.go), and `schemaOf` in
`web/src/lib/mock.ts`. Each test suite fails on an example it has no entry for.

[`testdata/wire.json`](testdata/wire.json) holds what the two readers must agree
on, and both suites read it. `-update` writes it from the tags of the types, after
the examples, so a changed tag shows as a changed line of that file:

- `shapes`: for the members each example has, which may be left out, which may
  be `null`, which numbers may be a fraction, which texts are random ids, and
  which members the frame's type keeps as JSON. Both suites change every value
  of every example in turn (left out, `null`, a list for an object and the
  reverse, a text for a number and the reverse, a fraction, 2^53, a text that is
  no id) and expect what these lists say.
- `cases`: examples with one value replaced, and whether they are read: which
  booleans may be `false`.

Two things are outside this agreement. A member kept as JSON here (`opaque`) is
judged by whoever decodes it. And the web client's schemas name `t`, `op`, a
post's `kind` and an event's `code`, which the types here leave to their reader.

## Reading the wire

`Decode(data, &v)` is how a node reads everything that comes from the wire
([conventions, section 2](../../planning/contracts/conventions.md)); `encoding/json`
alone reads four things differently from a browser. An error from `Decode` is
`bad_request`.

- **Exact names.** A member whose name holds an upper-case letter, a long s or a
  Kelvin sign is dropped at every depth before decoding, so `TO` is never read as `to`.
- **Presence.** A field without `omitempty` must be there. `0`, `false`, `""` and `[]`
  are values.
- **`null`.** Only a pointer field without `omitempty`, a field its contract marks
  "or `null`", takes it. Anywhere else, an optional field included, it is refused.
- **Integers.** A number is read as its nearest double, and an integer field takes it
  only if that is a whole number of at most 2^53 - 1 either way: `2.0` is 2. A number
  beyond the largest double, which a browser reads as `Infinity`, is refused only by a
  field that reads it; elsewhere it is ignored like any member the receiver does not know.
- **Ids.** A field of type `ID` takes a random id, the canonical base64url spelling of 16 bytes
  (22 characters), and no other text, an empty one included. `n` and `g` of `PayloadHeader` have
  this type, and so has every id in a command payload, in an `Item`, and `g` of `inbox` and `ack`.
  Of the bodies a `post` carries, a review's and a confirm card's ids are checked:
  `task` and `report` of `ReviewSigned`, `ai` of `ConfirmSigned`.
  The host surface uses it too: `g` of `Home`, `AuditRow`, `PTYResize`, `WatchStart`, `WatchEnd`,
  `HostJoin`, `HostLeave` and `Reminder`, `msg` of `Reminder`, `ai` of `PTYResize`, `member` of
  `WatchStart`, `WatchEnd` and `HostLeave`, and `g`, `member` and `task` of `AgentGroup`.
  Rows, the other message bodies and the frames that read
  a group still hold ids as plain strings ([issues](../../issues.md)).

A `json.RawMessage` is not looked into; decode it with `Decode` when its type is
known. So a wire type states the contract in its tags: `omitempty` for an optional
field, a pointer without it for "or `null`". Because `Decode` drops members by the
characters of their names, a wire type has no map whose keys are not fixed names:
such a key could hold an upper-case letter and would be dropped. `go test ./internal/proto` reads every
example in `testdata` with `Decode`. A missing field and a wrong type are told apart
only by the error text, which is for logs.

`testdata/crypto.json` is not a wire example: it holds the key, signature, pairing
and device-log test vectors of [`internal/identity`](../identity/README.md), which
writes and checks it. Nor is `testdata/wire.json`, described above. Nor is
`testdata/tools.json`: it is the MCP `tools/list` result of each kind of session
([mcp-tools.md, section 5](../../planning/contracts/mcp-tools.md)), between a CLI
and the node, where names follow MCP (`inputSchema`). Its type is `ToolLists`
([`tools.go`](tools.go)); the tests read it with `encoding/json`, refusing a member
the type does not name, not with `Decode`.

- `host.info` is answered with `HostInfo` and pushed as `HostInfoPush`, the same
  `HostState` without `re` and `ok`. `update`, `relay` and an agent's `account` may be `null`;
  each agent entry carries `Driver` (`Accounts`, `Takeover`) (`step-3/drivers.md` section 6).
- External AIs add the optional `tool` to AI member rows, `member add_ai`,
  `member set` and `host.join`, which also takes `account` for an external AI.
  Both `host.info` frames carry `exe` and an `external` agent entry. The
  `external` MCP tool list adds `next_item`, `done` and `report_usage`.
  `Handout` is the result of `wait` or a successful `next_item` that found work.
- People and confirmations (`step-2/people.md`, `step-2/confirm.md`) add `Invite`
  (`HomeCommand`, answered with `link` and `expires` in `CmdResult`), `Join`, `Role` on
  `MemberSet`, `MemberJoinedBody` and `MemberRemovedBody`, `HomeAdd` (`home.add`,
  populating `host.info.homes` with `home`), `ConfirmBody` and `ConfirmSigned` with
  card states (`pending`, `confirmed`, `denied`, `expired`), `call` state `denied`
  (`TimedState`), `"confirm"` in `PushOn.On`, `Reminder.Kind` and `TaskWait.Kind`, and `Asked` on
  `TaskRow`.
- Invitation codes (`step-2/people.md` 2.6) add `Code` (`HomeCommand` with `op` and `hours`, a
  pointer so that 0 is told from left out; `new` and `show` are answered with `code` and
  `expires` in `CmdResult`), `join` on `Welcome`, `hosts` on `ICEConfig`, and the host's `join`,
  which is a `RoomData`; `leave` is a `RoomID`.
- Presets and knowledge-base documents (`step-3/knowledge.md`) add optional `type`
  (the field `NoteType`, JSON `type`) and `name` on `NoteRow` and `Note` (answered with `id` and `replaced`
  in `CmdResult`), `preset` on `BriefBody`, and `q` on `FetchKind`. The tool lists in
  `testdata/tools.json` add `kb_put` and `kb_search` to every kind of session. Since P-8 the
  file has one task list, `task`, and `handback` in the `post` of every list but `external`.
- Member profiles and escalation (`step-3/profiles.md`) add `Profile` (`model`, `effort`,
  `tier`, `skills`) to AI member rows and `ProfileSet` to `member add_ai`, `member set` and
  `host.join`, `member` on `HostLeave`, `refresh` on `HostInfoRequest`, `models` (`Model`) on each
  agent of `host.info`, and `stuck` (`Stuck`) on `TaskRow`.
- `True` is a boolean that decodes only `true`: `ok` of a success (`Success`), `off` of a push
  and `jump` of an item.
- In `SettingsSet`, `caps` is `AgentCaps`: a member per agent being changed, its
  caps whole. A field per agent, not a map, so that every name on the wire is fixed.
- `pair.browser`, `host.offline` and `host.online` are a `HostCommand` alone;
  `takeover.end` is a `Takeover`, `takeover.start` a `TakeoverStart`.
- `Beacon` is the payload of the signed local-network beacon; it has no `g` or `to`.
- `testdata/trace-local.json` holds the complete signed transcript of one
  confirmed task from request to summary
  ([trace-task.md](../../planning/flows/trace-task.md)). Its type is `TaskTrace`
  ([`trace.go`](trace.go)). The home writes it, not this package:
  `internal/home/trace_test.go` sends its signed commands through the home's
  command table and fails when the file differs from what the home answers, pushes
  and delivers; `go test ./internal/home -run TestTaskTraces -update` writes it
  again. `web/src/lib/trace.test.ts` verifies it on its own: the device log,
  every command signature, the rows, and the inbox items with their acknowledgements.
  Until the home follows P-8, the file is the old transcript with only its typed parts
  (the group, the AI members, the inbox items and the task rows beside them) brought to
  the new shapes, so that this package reads it; its signed commands and pushed rows are
  still the old flow.
