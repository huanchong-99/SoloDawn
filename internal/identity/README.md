---
description: 'Device keys, signed envelopes, the device log and the pairing exchange: what a caller gets from internal/identity, which checks it makes and which it leaves to the caller, and how the shared crypto vectors are kept.'
kind: reference
---

# Identity

## Summary

The Go implementation of [contracts/envelope.md](../../planning/contracts/envelope.md): a device's
Ed25519 key, the signed envelope that authenticates everything on the wire, the device log that says
which devices are a person, and the secret two devices use to meet for the first time. The web
client implements the same rules; both are held to one file of test vectors. The package holds no
connection, clock or database: every function works on the values it is given.

## Use this capability

| Need | Entry |
| --- | --- |
| the node's key, created on first start | `LoadKey(dataDir)`; then `Key.ID`, `Key.Fingerprint` |
| send a payload | `Key.Sign(p)`, with `p` the payload's JSON text |
| receive an envelope | `OpenSignal(env, me, known, now)` for signalling, `OpenCommand` for a command; then `proto.Decode` for the payload |
| rendezvous handshake | `Key.SignChallenge(nonce)`, `VerifyChallenge(device, nonce, s)` |
| a person's devices | `Log{User: id}`, `Log.Apply(entry)`, `Replay(user, entries)`, `Log.Valid(device)` |
| first-run enrolment | `Log.Adopts(me, peer)` |
| pairing | `NewSecret`, `ParseSecret(text)`, `Secret.String`, `Secret.Room`, `Secret.MAC`, `Secret.Verify` |

## Contract

- **Key file.** `device.key` in the data directory holds the 32-byte seed and is created with mode
  0600. `LoadKey` does not create the directory, and refuses a file of any other length. On Windows
  the mode has no effect; the per-user data directory is what keeps the file private.
- **Signature.** `Sign` covers `"solodawn/1\n"` followed by `p` exactly as given; `p` is never
  parsed and encoded again, on either side. A rendezvous answer covers `"solodawn/rv/1\n"` and the
  nonce as text, so neither signature verifies as the other.
- **Device id.** A text is a device id when it is the canonical base64url spelling of 32 bytes
  that are not one of the fourteen weak values (the encodings of the points of small order).
  Anything else is never used as a key: as `k` it is `bad_sig`, as `dev` it makes a log entry
  refused, and `Secret.Verify` refuses a pair message that carries it. Being a point of the curve
  is not tested: nothing verifies under a key that is none, and a log may add a `dev` that is
  none, which can then never sign.
- **Verifying.** A signature is 64 bytes in their canonical spelling whose first half is not a weak
  value. Both byte comparisons are made here, before `crypto/ed25519` is asked, because that
  library verifies forged signatures under a weak key and signatures that begin with the identity
  point, and other libraries do not. That the second half is below the order of the group is left
  to the library, which refuses it.
- **Receiving.** `OpenSignal` and `OpenCommand` fail with the code of the first check that fails, in
  the contract's order: `bad_sig`, `unknown_device`, `bad_request`, `not_for_me`, `stale`. The
  payload is read with `proto.Decode`: `t` is a string, `n` a random id (`proto.ID`: 16 bytes in
  their canonical spelling, 22 characters) and `ts` an integer; `g` is an id and `to` a string
  when they are there. An empty `n` or `g` is no id. Signalling must carry `to`; a command may
  have none. An empty `to` is a value and is nobody's device id. `ts` may be two minutes from the
  receiver's clock for signalling and 24 hours for a command, and exactly that far still passes.
  The codes are `Code` values whose text is the wire code. `known` is the caller's: this package
  does not know owners, groups or pairing peers.
- **Device log.** `Apply` accepts an entry when its signature holds; its `p` is at most 2,048
  bytes; it is a `device` payload for this log's person with an integer `ver` and an op `add` or
  `revoke` of a device id; an `add` has a `kind` that is `node` or `browser` and a `name` that is
  a string; its version is the next one; and its signer is valid before it. Entry 1 must be the
  `add` of its own signer, which is the person. An `add` of a key that was ever revoked is
  refused, and so is a `revoke` of the last valid device. Nothing else of the payload is read: not
  `n` or `ts`, and not `kind` or `name` on a `revoke`. For a version the log already holds, the
  same envelope again returns `added == false` without an error and any other is refused: the
  first accepted wins. A refused entry leaves the log as it was. `Entries` are the envelopes as
  signed, in order, for storing and passing on.
- **Adoption.** `Adopts` is the log's half of the rule in section 6 of the contract. That the
  device belongs to nobody, and that the log arrived on the connection it paired on, is known only
  to the caller.
- **Pairing.** A `Secret` is the canonical 16 characters. `ParseSecret` takes what a person copied:
  it removes the hyphens and turns `a` to `z` into upper case, and refuses everything else, also a
  letter that a general upper-casing would turn into a valid character. `MAC` takes a pair
  message and covers its `role`, `k`, `kind` and `name`, as the text that goes on the wire: a
  byte that is not UTF-8 counts as U+FFFD, which is what `encoding/json` writes for it and what
  a browser's `TextEncoder` gives for a lone surrogate. `Verify` takes the role the other side
  must have: a message in the receiver's own role is its own message sent back. It also refuses a
  `t` that is not `pair` and a `kind` that is neither `node` nor `browser`. The caller reads the
  message with `proto.Decode` first, which is what makes `name` a string.

## Shared test vectors

[`../proto/testdata/crypto.json`](../proto/testdata/crypto.json) holds the cases that keep this
package and the web client to one answer. Its shape, and the cases each part must hold, are in
[contracts/vectors.md](../../planning/contracts/vectors.md). The cases are written in
`vectors_test.go`; one test fails when the file is not exactly what they encode to, a second reads
the file and holds this package to every case, signing again where the contract asks for the same
signature bytes. After an agreed change to a rule or a value, rewrite the file with
`go test ./internal/identity -update`. Unlike its neighbours in `testdata`, the file is not a wire
message.

## Known limitations

- How long a pairing secret is valid, that it is used once, and that a paired connection carries
  only `hello` and `devlog` until the newcomer is enrolled are rules about connections and belong
  to `internal/node`. So does refusing a host command that has no `to`.
- A key that is a real key with a point of small order added to it passes the list of weak values
  (`planning/risks.md` K-15).

## Verification and sources

`go test ./internal/identity` covers every rule of sections 2, 3, 4 and 6 of the contract that
needs no connection; `features/identity.feature` runs the main paths through the exported
functions. `forge_test.go` makes, with its own arithmetic, the signatures no honest signer writes:
forgeries under each weak key, a signature that begins with the identity point, one with the group
order added to its second half, and a key that is no point of the curve. The reference values in
the tests (device id, fingerprint, signatures, room, mac) were computed with Node's crypto module.
The web client's `vectors.test.ts` reads the same `crypto.json` with its own implementation, in
the same pull request as every change of the file.
