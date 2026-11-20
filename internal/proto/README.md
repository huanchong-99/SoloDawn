# Protocol wire types

These structs describe the JSON exchanged during signalling, pairing, device-log
transfer, session setup and group reads and commands. They do not authenticate
peers, apply rows or execute commands. The source of truth is
[the wire contracts](../../planning/contracts/).

An envelope's `p` is a **string**: its original UTF-8 bytes are signed. A batch
can mix row types, so its elements stay as JSON until the consumer reads `r`
(and `gone` for a deletion). Message bodies similarly depend on `kind`.

The examples in [testdata](testdata/) are shared with the web client.
`go test ./internal/proto` checks encoding and decoding against them.
Use `go test ./internal/proto -update` only for an agreed change to the wire.

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

A `json.RawMessage` is not looked into; decode it with `Decode` when its type is
known. So a wire type states the contract in its tags: `omitempty` for an optional
field, a pointer without it for "or `null`". `go test ./internal/proto` reads every
example in `testdata` with `Decode`. A missing field and a wrong type are told apart
only by the error text, which is for logs.

`testdata/crypto.json` is not a wire example: it holds the key, signature, pairing
and device-log test vectors of [`internal/identity`](../identity/README.md), which
writes and checks it.
