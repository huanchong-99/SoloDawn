# Contract: wire conventions

Rules that hold for everything on the wire: frames on `ctl`, rows, command
payloads, message bodies, signalling frames. The other contract files give
the shapes; this file says how to read them. The golden examples in
`internal/proto/testdata/` follow it, and both test suites check it
strictly.

## 1. Encodings

Binary values, times and ids are written as `contracts/envelope.md` section
1 says.

## 2. Presence: when a field appears

Every frame, row, payload and body has a fixed set of fields, whose names
are made of `a` to `z`, `0` to `9` and `_`. For each field the contract says
one of two things.

**Always present** (the default: every field shown in an example or a table,
unless marked optional). When there is nothing to say, the value is written
by its type:

| Type of the field | "Nothing" is written as | Never |
| --- | --- | --- |
| a list | `[]` | `null`, left out |
| an object whose keys are themselves optional (`usage`, `checks`, a `body` without content) | `{}` | `null`, left out |
| a single value that can be "none", which its contract marks "or `null`" | `null` | left out |
| a number, a boolean, a string | its value; `0`, `false` and `""` are values, not absence | `null` |

**Optional** (marked "optional" in the field table of its contract). The
field is left out when it does not apply or is not known yet. When it is
there, it has a real value: an optional field is never `null`, and never an
empty list or object standing in for "nothing".

No field is both: a field is either always present or optional.

- The complete list of optional fields of each row type is in
  `contracts/rows.md`; of each message body and card state, in
  `contracts/messages.md`; of each command, in `contracts/commands.md`.
- **Tests are strict in both directions.** A golden example with a field the
  types do not know fails; a type with an always-present field the example
  lacks fails; a `null` where the contract does not say "or `null`" fails.
- **At run time** a receiver reads the fields it knows, by their exact
  names, and ignores every other field: nodes of different versions are
  online together. A name written in another case is another name, and is
  ignored like any field the receiver does not know. When a name appears
  twice in one object, the last one counts. A missing always-present field,
  or a field of the wrong type, is `bad_request`.
- **For implementers, writing.** Where a language writes an unset list or
  an unset raw JSON value as `null` (Go does, for a nil slice and a nil
  `json.RawMessage`), the code that builds a row, a frame, a payload or a
  body sets them to an empty list or an empty object. A `null` list on the
  wire is a bug in the sender, not something receivers tolerate.
- **For implementers, reading.** `JSON.parse` already reads as the run-time
  rule says. Go's `encoding/json` does not `[verified]`: it also matches a
  name written in another case, folding Unicode, so that for
  `{"to":"A","TO":"B"}` it reads B where a browser reads A, and `"t\u017f"`
  (a long s) sets `ts`. A Go node therefore decodes what it reads from the wire through one
  function of `internal/proto` that keeps exact names only. The envelope
  check and the handler of a payload both use it, so that no payload is
  read two ways.

How both are tested, and by which task, is in `steps/step-1-backend.md`,
working rules.

## 3. Requests, answers, pushes

On `ctl` (`contracts/datachannel.md`):

| Kind | Shape |
| --- | --- |
| request | has `t` and `id` (an integer, unique per connection, chosen by the sender) |
| success | has `re` (that `id`) and `"ok": true`, then its result fields. It has `t` where the contract names one: `welcome`, `synced`, `fetched`, `host.info`. The answers to `cmd` and `audit` have no `t` |
| error | exactly `{"re": <id>, "err": {"code": "…", "msg": "…"}}`, for **every** request type. No `t`, no `ok`, nothing else. `msg` is an English sentence for logs; clients show their own text for `code` |
| push | has `t`; no `id`, no `re` |

A client tells a success from an error by the presence of `err`, never by
`t`. Error codes are in `contracts/datachannel.md` section 9.

## 4. Sizes

- A `ctl` message is at most 60 KiB. `[unverified]` This assumes both
  WebRTC stacks accept SCTP messages of that size; spike S-1 measures it.
- The payload `p` of an envelope is at most 24 KiB.
- A row, and an inbox item with the rows it carries, must fit one message.
  The two numbers above do not yet guarantee it: a message row repeats its
  text inside `env.p`, where it is escaped a second time, a decided card
  holds a second envelope in `state.act`, and an inbox item wraps all of
  that once more. The limits that do guarantee it are set together with the
  result of spike S-1 (`risks.md` T-14).

The limit of a signalling frame is in `contracts/rendezvous.md` section 2;
that of a message's `text` in `contracts/rows.md` section 5.

## 5. Where each shape is frozen

"Frozen" means: a struct in `internal/proto`, a golden file in
`internal/proto/testdata/`, and the same file parsed by the web client's
tests. A shape is frozen by the first backend task that needs it, at the
start of its pull request, so that the frontend can build against the file
while the rest is written. Until its golden file exists, the example and
the field table in the contract are the reference, and the web client types
the shape from them. This table is the only place that says who freezes
what.

| Shapes | Frozen in |
| --- | --- |
| envelope; `offer`, `answer`, `ice`; `pair`; device-log entries; `hello`, `welcome`, `devlog` | B-01 slice (a), done |
| `sync`, `reset`, `rows`, `synced`, `fetch`, `fetched`, `cmd` and its answers, the error answer; the six row types and the deletion marker | B-01 slice (b), done |
| every message body and card state (`contracts/messages.md`): one golden message row per kind without a card, `row-msg-<kind>.json`, and one per status of each card kind, `row-msg-<kind>-<status>.json`. The existing `row-msg.json`, a decided decision card, is renamed `row-msg-decision-decided.json`; it is not kept twice. In the same pull request: `pos` on `note` rows (`contracts/rows.md` section 6), and the helper that lists the `null`s of an encoded value | B-01 slice (d), before B-11 |
| key, signature, pairing, device-log and adoption test vectors (`contracts/envelope.md` section 8) | B-02 |
| the WebSocket frames of signalling: `challenge`, `auth`, `ready`, `ice` (request and answer), `to`, `from`, `offline`, `watch`, `presence`, `join`, `joined`, `room` | B-06 |
| `quota` (request and answer) | B-07 |
| the local-network `beacon` payload | B-09 |
| `groups`; `host.info`; the payloads of `settings.set` and `pair.browser` | B-10 |
| the payloads of `group`, `member`, `post`, `note`, `forget` | B-12 |
| the payload of `agent.set` | B-14 |
| `inbox` with the signed `item` payload, `ack`; the payloads of `host.join`, `host.leave` | B-15 |
| the payloads of `perm.set`; `audit` and its answer | B-17 |
| the payloads of `act`, `task`, `jump`, `push`, `takeover.start`, `takeover.end`, `host.offline`, `host.online`; `pty.resize` | the task that implements each (`steps/step-1-backend.md`) |
| one confirmed task end to end, with and without a remote | B-01 slice (c) |
