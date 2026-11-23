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
empty string, list or object standing in for "nothing".

No field is both: a field is either always present or optional.

- The complete list of optional fields of each row type is in
  `contracts/rows.md`; of each message body and card state, in
  `contracts/messages.md`; of each command, in `contracts/commands.md`; of
  each inbox item, in `contracts/datachannel.md` section 6.
- **Tests are strict in both directions.** A golden example with a field the
  types do not know fails; a type with an always-present field the example
  lacks fails; a `null` where the contract does not say "or `null`" fails.
- **At run time** a receiver reads the fields it knows, by their exact
  names, and ignores every other field: nodes of different versions are
  online together. A name written in another case is another name, and is
  ignored like any field the receiver does not know. When a name appears
  twice in one object, the last one counts. A missing always-present field
  and a field of the wrong type are both `bad_request`, and nothing tells
  the two apart; in a device-log entry either makes the entry refused
  (`contracts/envelope.md` section 4).
- **Only what is read is judged.** These rules are about the fields a
  receiver reads. A field it does not read is not looked at, whatever it
  holds, `null` included: `n` and `ts` of a device-log entry, `kind` and
  `name` of a `revoke`, a field it does not know.
- **`null`**, in a field that is read, is right only where the contract
  marks the field "or `null`". In any other field, an optional one
  included, it is a wrong type. It never stands for a missing field.
- **Integers.** A number is read as `JSON.parse` reads it, to the nearest
  double. It is an integer when that value is a whole number whose
  absolute value is at most 2^53 − 1, which is what JavaScript's
  `Number.isSafeInteger` tests. So `2.0` and `2.0000000000000001` are the
  integer 2, and nothing beyond 2^53 − 1 is an integer, however it is
  written. In a field that holds an integer, any other number is a wrong
  type.
- **Ids.** A random id is 22 characters, the canonical spelling of 16
  bytes (`contracts/envelope.md` section 1). In a field that holds one,
  `n` included, any other text is a wrong type. This also keeps out what
  two decoders read differently: for the escape of a lone surrogate
  (`"\ud800"`) Go reads U+FFFD and a browser keeps the surrogate, so two
  such nonces would be one nonce to a node and two to a browser. Free text
  may still hold one (`risks.md` K-17). A value that a contract names
  beside the id is as good as an id there: the `by` of a message row may
  be `"system"`.
- **For implementers, writing.** Where a language writes an unset list or
  an unset raw JSON value as `null` (Go does, for a nil slice and a nil
  `json.RawMessage`), the code that builds a row, a frame, a payload or a
  body sets them to an empty list or an empty object. A `null` list on the
  wire is a bug in the sender, not something receivers tolerate.
- **For implementers, reading.** In a browser, `JSON.parse` and a check of
  each field's type read this way. Go's `encoding/json` does not
  `[verified]`:
  - it also matches a name written in another case, folding Unicode: for
    `{"to":"A","TO":"B"}` it reads B where a browser reads A, and
    `"t\u017f"` (a long s) sets `ts`;
  - it reads `null` into a field that is not a pointer as the zero value,
    without an error, and into a pointer as nil, which is also what a
    missing field leaves;
  - after decoding, a field that was missing and a field that held its
    zero value look the same;
  - it refuses `2.0` for an integer, and takes an integer beyond 2^53 − 1
    that a browser reads as another number.

  A Go node therefore decodes what it reads from the wire through one
  function of `internal/proto` that applies the rules above. The envelope
  check and the handler of a payload both use it, so that no payload is
  read two ways. To keep exact names it drops every member whose name
  holds an upper-case letter, U+017F or U+212A before the standard decoder
  sees it. That works because of the rule on names at the top of this
  section, and it binds maps as well: the keys of a map on the wire are
  made of `a` to `z`, `0` to `9` and `_`, like the agent names that key
  `caps` in `settings.set`. An id is never a key.

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

- A `ctl` message is at most 60 KiB `[verified]`: in spike S-1 a pion node
  exchanged one 61,440-byte text message with each of Chromium, Firefox
  and WPE WebKit on Linux, over UDP, over ICE-TCP and through the relay.
- A row is at most 16 KiB: the UTF-8 bytes of its JSON as the home encodes
  it, without HTML escaping (in Go, `SetEscapeHTML(false)`), so that `<`,
  `>` and `&` count one byte each, as a browser counts them. Nodes encode
  payloads the same way. The home is the only writer of rows: it answers
  `bad_request` to a command that would write a larger row, and the
  message of that error names the limit and the size the row would have
  had.
- A card is written smaller, so that it can always be decided: the row of
  a message that has a `state` is at most 12 KiB when it is created. A
  decision adds less than 4 KiB, because the `text` of an `act` is at
  most 1 KiB (`contracts/messages.md` section 3).
- Nothing else makes a row outgrow its limit later. What a task row shows
  of its thread's last message is a short preview (`contracts/rows.md`
  section 4), and what an approval card shows of a long request is its
  beginning (`contracts/messages.md` 2.3).
- These numbers are enough for what a home sends. A `rows` or `fetched`
  frame always has room for a row. An inbox item
  (`contracts/datachannel.md` section 6) carries at most one message row
  and one task row beside its small plain `item` object, well under 50 KiB.
- Whoever builds a list fills a frame only as far as it fits, and sends
  the rest in another frame (`rows`, `devlog`) or as another page
  (`fetched`).
- A node that writes free text into a command of its own (the `detail` of
  a check, of a failure, of a host event) shortens it so that the row
  fits, and keeps the end of an output.
- A `device` row holds its log entry, which is why the `p` of an entry is
  at most 2 KiB (`contracts/envelope.md` section 4).

The limit of a signalling frame is in `contracts/rendezvous.md` section 2.

## 5. Where each shape is frozen

"Frozen" means: a struct in `internal/proto`, a golden file in
`internal/proto/testdata/`, and the same file parsed by the web client's
tests. All three change in one pull request: whoever adds or changes a
golden file also updates the schema in `web/src/lib/proto.ts` and the file
map of `proto.test.ts`, because the web client's golden test fails on a file
it has no mapping for and the web job of CI runs on every pull request
(decided by coordinator). In step 1, P-1 and P-2 freeze the shapes assigned
to them below before the code that uses them (`steps/parallel-plan-units.md`
section 1). Until a golden file exists, the example and field table in the
contract are the reference, and the web client types the shape from them.

**One answer for the same bytes.** Each side has a file map that names what
reads each golden file: `shapes` in `internal/proto/shapes_test.go`, and
`schemaOf` in `web/src/lib/mock.ts`. `internal/proto/testdata/wire.json`
holds what the two readers must agree on, and both test suites judge
every golden file by it: which of its members may be left out, which may be
`null`, which numbers may be a fraction, which texts are random ids; and
cases with one value replaced, such as a slug that the home's rule cannot
make. The Go tests write that file from the tags of the Go types, so a
changed tag is a changed line there, and the web client's tests then hold
its schema to the same line. Two things are outside it: a member that the
Go type of a frame keeps as JSON (a row in `rows`, the `msg` of an inbox
item) is judged through its own golden file; and `t`, `op`, a post's `kind`
and an event's `code` are read before a shape is chosen, so the Go types
hold them as text and only the schemas name them.

| Shapes | Frozen in |
| --- | --- |
| envelope; `offer`, `answer`, `ice`; `pair`; device-log entries; `hello`, `welcome`, `devlog` | B-01 slice (a), done |
| `sync`, `reset`, `rows`, `synced`, `fetch`, `fetched`, `cmd` and its answers, the error answer; the six row types and the deletion marker | B-01 slice (b), done |
| every message body and card state (`contracts/messages.md`): one golden message row per kind without a card, `row-msg-<kind>.json`, and one per status of each card kind, `row-msg-<kind>-<status>.json`. The existing `row-msg.json`, a decided decision card, is renamed `row-msg-decision-decided.json`; it is not kept twice. In the same pull request: `pos` on `note` rows (`contracts/rows.md` section 6), and the helper that lists the `null`s of an encoded value | B-01 slice (d), before B-10, whose tests use the helper |
| the shared test vectors of `contracts/vectors.md` | B-02, completed by B-02b and B-02c |
| the WebSocket frames of signalling: `challenge`, `auth`, `ready`, `ice` (request and answer), `to`, `from`, `offline`, `watch`, `presence`, `join`, `joined`, `room` | B-06 |
| `quota` (request and answer) | B-07 |
| the payloads of `group`, `member`, `post` (every kind), `act`, `jump`, `task`, `note`, `forget`, `push`; the review outcome and the host events (`contracts/commands.md` 1.8, 1.9); `inbox` with the signed `item` payload, `ack`; the payloads of `host.join`, `host.leave`; `fetch` of one row (`r`, `row`). Changes to shapes frozen earlier, in the same pull request: the task row's `slug`, `plan`, `review.failed` and `wait.kept`, with `row-task.json`'s branch made by the slug rule; the decision state `expired` (`row-msg-decision-expired.json`); `mentions` of an approval card holding its approvers; `act` payloads in the examples without empty `arg` or `text`; `welcome.addrs`. The approval body's `full` is frozen already (pull request 117) | P-1 |
| `groups`; `host.info`, with `homes` entries of `g`, `home` and `name`; `audit` and its answer; `pty.resize`; the local-network `beacon` payload; the payloads of `settings.set`, `pair.browser`, `agent.set`, `perm.set`, `host.offline`, `host.online`, `takeover.start`, `takeover.end`; the MCP tool list (`tools.json`, `contracts/mcp-tools.md` section 5) | P-2, after P-1 and B-07 |
| `inbox` and `item-*` re-frozen with a plain unsigned `item` object (`item`, `ai`, `why` and the optional fields of `contracts/datachannel.md` section 6), with no `env` wrapper and no `cause`, `g` or `to` (owner's decision of 2026-11-21: no attack-and-defence code) | P-1b |
| one confirmed task end to end, with and without a remote | B-01 slice (c) |
| the additions of `contracts/step-3/profiles.md` section 6: the four fields of an AI member, `member` in `host.leave`, `refresh` and `models` in `host.info`, `stuck` on the task row | P-6 |
| the additions of `contracts/step-3/external.md` section 12: `tool` on an AI member, `host.join` for `external`, `exe` and the `external` agent in `host.info`, the tool list `external`, the hand-out | P-7 |

**Feature files of the parallel plan** (`steps/parallel-plan.md` section 2).
The module's seam unit owns `features/<module>.feature`. A Go slice puts
its acceptance scenarios in `features/<module>_<slice>.feature`; its
`features/<module>_<slice>_test.go` registers the steps from `init` with
the X-1 registry in `features/acceptance_test.go`. A web slice puts its
scenarios in `features/web_<slice>.feature`, tagged `@web`, and runs them
with `web/features/<slice>.spec.ts`. Each unit edits only its own files.
