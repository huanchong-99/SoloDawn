# Protocol wire types

These structs describe the JSON exchanged during signalling, pairing, device-log
transfer, session setup and group reads and commands. They do not authenticate
peers, apply rows or execute commands. The source of truth is
[the wire contracts](../../planning/contracts/).

An envelope's `p` is a **string**: its original UTF-8 bytes are signed. A batch
can mix row types, so its elements stay as JSON until the consumer reads `r`
(and `gone` for a deletion). Message bodies depend on `kind`. For a known
shape, e.g. an answered call, use `MessageRow[CallBody, CallAnsweredState]`.
The body is fixed at creation; only the card state changes.

Sender tests can pass their encoded JSON to `UnexpectedNulls`, with the JSON
Pointers allowed to be null for that shape (e.g. `/thread`, `/re`, and
`/state` for text messages). It reports every other null, including in lists.

The examples in [testdata](testdata/) are shared with the web client.
`go test ./internal/proto` checks encoding and decoding against them.
Use `go test ./internal/proto -update` only for an agreed change to the wire.

`testdata/crypto.json` is not a wire example: it holds the key, signature, pairing
and device-log test vectors of [`internal/identity`](../identity/README.md), which
writes and checks it.
