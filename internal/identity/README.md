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
| receive an envelope | `OpenSignal(env, me, known, now)` for signalling, `OpenCommand` for a command |
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
  nonce, so neither signature verifies as the other. A device id or signature verifies only in its
  one canonical base64url spelling, so a key has exactly one device id.
- **Receiving.** `OpenSignal` and `OpenCommand` fail with the code of the first check that fails, in
  the contract's order: `bad_sig`, `unknown_device`, `not_for_me`, `stale`. They differ only in how
  far `ts` may be from the receiver's clock: two minutes and 24 hours; a `ts` exactly that far away
  is still accepted. Between the second and the third check they parse the common fields and
  answer `bad_request` when the payload is not JSON or one of them has the wrong type. The codes are
  `Code` values whose text is the wire code. `known` is the caller's: this package does not know
  owners, groups or pairing peers.
- **Device log.** `Apply` accepts an entry when its signature holds, it is a `device` payload with
  op `add` or `revoke` of a device id for this log's person, its version is the next one, and its
  signer is valid before it; entry 1 must be the `add` of its own signer, which is the person. An `add` of a key
  that was ever revoked is refused. For a version the log already holds, the same envelope again
  returns `added == false` without an error and any other is refused: the first accepted wins. A
  refused entry leaves the log as it was. `Entries` are the envelopes as signed, in order, for
  storing and passing on.
- **Adoption.** `Adopts` is the log's half of the rule in section 6 of the contract. That the
  device belongs to nobody, and that the log arrived on the connection it paired on, is known only
  to the caller.
- **Pairing.** A `Secret` is the canonical 16 characters; `ParseSecret` takes what a person copied,
  in either case, with or without hyphens. `Verify` takes the role the other side must have: a
  message in the receiver's own role is its own message sent back.

## Shared test vectors

[`../proto/testdata/crypto.json`](../proto/testdata/crypto.json) has the shape of section 8 of the
contract: `keys`, `envelopes`, `rendezvous`, `pairing`, `devlogs`. Its cases are written in
`vectors_test.go`; one test fails when the file is not exactly what they encode to, a second reads
the file and holds this package to every case. After an agreed change to a rule or a value, rewrite
the file with `go test ./internal/identity -update`. Unlike its neighbours in `testdata`, the file
is not a wire message.

## Known limitations

- How long a pairing secret is valid, that it is used once, and that a paired connection carries
  only `hello` and `devlog` until the newcomer is enrolled are rules about connections and belong
  to `internal/node`.
- `kind` and `name` of an `add` entry are not checked.
- Three limits are recorded in [issues.md](../../issues.md): an empty `to` is read as no `to`; the
  Go and browser JSON parsers can disagree on a payload that only its signer could write; and
  `crypto/ed25519` verifies signatures of small-order public keys that other implementations refuse.

## Verification and sources

`go test ./internal/identity` covers every rule of sections 3, 4 and 6 of the contract that needs
no connection; `features/identity.feature` runs the main paths through the exported functions. The
reference values in the tests (device id, fingerprint, signatures, room, mac) were computed with
Node's crypto module, and `crypto.json` was checked once with a separate WebCrypto script; that
script is not in the repository.
