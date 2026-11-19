# Contract: identities and the signed envelope

One primitive authenticates everything: the signed envelope. This file
defines it, the device log that says which devices belong to a person, and
the pairing exchange that introduces two devices that have never met.

## 1. Encodings

- Binary values in JSON are base64url without padding.
- Times are integer milliseconds since the Unix epoch, UTC.
- Random ids are 16 random bytes, base64url (22 characters).

## 2. Device

- A device is one node installation or one browser profile on one origin.
- It owns an Ed25519 key pair, generated on first run.
  - Node: stored in the node's data directory, readable by the user only.
  - Browser: a non-extractable WebCrypto key stored in IndexedDB.
    `[unverified]` Ed25519 in WebCrypto needs recent browsers; see
    `risks.md` Q-3.
- **Device id** = the 32-byte public key, base64url (43 characters).
- **Fingerprint** (shown to people) = first 6 bytes of SHA-256 of the public
  key, as 12 upper-case hex digits in groups of four: `3F9A 0C21 77BE`.

## 3. Envelope

```json
{
  "p": "{\"t\":\"post\",\"n\":\"Zk3…\",\"ts\":1763600000000,\"g\":\"Qm1…\",\"text\":\"hello\"}",
  "k": "<device id of the signer>",
  "s": "<signature>"
}
```

- `p` is the payload: a JSON document carried as a string, so that the signed
  bytes are exactly the bytes received. No canonical form is needed.
- `s` = Ed25519 signature over the UTF-8 bytes of `"solodawn/1\n"` followed
  by `p`.
- Payload fields common to all envelopes:

| Field | Type | Meaning |
| --- | --- | --- |
| `t` | string | payload type |
| `n` | id | nonce, unique per signer; also the id of the thing created, if any |
| `ts` | time | signer's clock |
| `g` | id, optional | group the payload is about |
| `to` | device id, optional | intended receiver (signalling and host commands) |

A receiver checks, in this order, and drops the envelope on the first
failure with one error code:

1. `bad_sig`: `s` verifies for `k` over the prefix and `p`.
2. `unknown_device`: `k` is a currently valid device of someone the receiver
   has a reason to listen to (the receiver's owner; a member of a group it
   homes; the home of a group it participates in; or a pairing peer, section
   6).
3. `not_for_me`: if `to` is present, it equals the receiver's device id.
4. `stale`: for signalling payloads only, `ts` is within two minutes of the
   receiver's clock.
5. `replay`: a repeated (`k`, `n`) has no second effect. A home takes a
   command only from the connection of its signer and uses `n` as the id of
   the row it creates, so a repeat is answered `ok` and changes nothing. A
   host acting on a forwarded envelope checks that it names the exact
   pending thing (an approval id, an inbox item above its cursor).

After that the payload is parsed and handled according to its type.

## 4. Person and device log

A person is the set of devices in their device log. The **user id** is the
device id of the first device.

Each log entry is an envelope with payload:

```json
{"t":"device","n":"…","ts":1763600000000,
 "user":"<user id>","ver":3,"op":"add",
 "dev":"<device id>","kind":"node","name":"workstation"}
```

| Field | Values |
| --- | --- |
| `ver` | 1, 2, 3 … with no gaps |
| `op` | `add` or `revoke` |
| `kind` | `node` or `browser` (for `add`) |
| `name` | label chosen by the person (for `add`) |

Rules:

- Entry 1 is the genesis: `op` is `add`, and `dev`, `k` and `user` are the
  same key.
- Entry `ver` = v is accepted only if the receiver's current version for
  that user is v − 1 and the signer `k` is in the valid set at v − 1.
- `add` puts `dev` in the valid set. `revoke` removes it for good; a revoked
  key is never added again.
- A second, different entry for a version already accepted is rejected.
  First accepted wins.
- Anyone can verify a whole log by replaying it from entry 1.

Where logs live and how they spread:

- Every node stores the log of its owner.
- Every home stores the log of each person in its groups and pushes entries
  to clients as `device` rows (`contracts/datachannel.md`).
- A device that creates an entry sends it to every one of the owner's nodes
  it is connected to and to every home it is connected to. Nodes and homes
  pass on entries they accept to their own connected peers of that person.
- A device that was offline receives missing entries when it next connects.

Known limit: if a stolen device and its owner create conflicting entries for
the same version, receivers may accept different ones. Recovery from that
race is manual and is not designed in step 1. Listed in `risks.md` K-4.

## 5. AI members have no key

An AI's messages are signed by the node that runs it. The payload carries
`as` (the member id of the AI). A home accepts `as` = M from signer `k` only
if member M is an AI whose `host` is `k`.

## 6. Pairing two devices that have never met

Used when a browser meets its first node, when a node is added to an existing
person, and when a new browser is added. Sequence in `flows/pairing.md`.

- **Pairing secret**: 80 random bits, shown as 16 Crockford base32
  characters in four groups (`7K2M-9QXD-4F8T-WB3N`). Created by either side,
  valid for 10 minutes, used once. It travels inside the onboarding prompt
  that the person copies, or as a link that the node prints.
- **Room id** = first 16 bytes of SHA-256(`"solodawn/room/1\n"` + secret),
  hex.
- Each side sends through the room one message:

```json
{"t":"pair","k":"<my device id>","kind":"node","name":"workstation",
 "role":"a","mac":"<HMAC>"}
```

- `role` is `a` for the side that created the secret, `b` for the other.
- `mac` = HMAC-SHA256(key = secret bytes, message =
  `"solodawn/pair/1\n"` + role + `"\n"` + device id).
- Each side verifies the other's `mac`. After that it knows the other's
  device id is authentic for the next 10 minutes and accepts signed
  signalling from it. The rendezvous service sees the room id and both
  device ids, and cannot forge a `mac` without the secret.
- Once the data channel is open, the side that already belongs to a person
  appends an `add` entry for the newcomer and sends the log. If neither side
  belongs to a person (first run), the browser writes the genesis entry for
  itself and then the `add` for the node.

The mock-up shows an 8-character code. Eight characters are not enough
without a password-authenticated key exchange; see `risks.md` Q-2.

## 7. What is deliberately absent

- No application-layer encryption: every channel is DTLS end to end, and
  signalling carries nothing secret.
- No certificate authority, no accounts, no server-side identity.
- No canonical JSON, no key rotation, no multi-signature.
