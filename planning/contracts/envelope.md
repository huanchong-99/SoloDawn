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
  - Browser: a non-extractable WebCrypto key stored in IndexedDB. Only
    browsers with native Ed25519 in WebCrypto are supported; no signature
    library is bundled (R-164). `[unverified]` The exact minimum versions;
    spike S-7 fixes them.
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
4. `stale`: `ts` is close enough to the receiver's clock: within two
   minutes for signalling payloads, within 24 hours for commands.

A command that passes these checks and was already accepted is not an
error: the receiver answers what it answered the first time, from its
receipt, and does nothing else (`contracts/commands.md`, "Once only"). A
host acting on an envelope that was forwarded to it (an inbox item, the
decision of an approval) checks that it names the exact thing the host is
holding, and records what it has acted on.

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
- Every home stores the log of each person in its groups and shows entries
  to clients as `device` rows.
- Entries travel in one frame, `devlog` (`contracts/datachannel.md` 3.1),
  always as the original signed entries. A device that creates an entry
  sends it to every one of the owner's nodes and every home it is connected
  to; nodes and homes pass on what they accept; after every `welcome` each
  side sends the logs the other may be missing.

Known limit: two entries for the same version make a fork, and receivers
may accept different ones. It happens when a stolen device and its owner
race to revoke each other, and also, innocently, when two devices of the
same person each add a device at the same moment. Step 1 does not resolve
forks: the guidance is to add one device at a time, and recovery is manual
(`risks.md` K-4).

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
- **Secret bytes**, wherever the secret is hashed or used as a key: the 16
  characters in upper case, without the hyphens, as ASCII.
- **Room id** = first 16 bytes of SHA-256(`"solodawn/room/1\n"` + secret
  bytes), hex.
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
- Until the newcomer is in the log, the connection carries `hello` and
  `devlog` and nothing else.
- Once the data channel is open, the side that already belongs to a person
  appends an `add` entry for the newcomer and sends the whole log in a
  `devlog` frame. If neither side belongs to a person (first run), the
  browser writes the genesis entry for itself and then the `add` for the
  node.
- **Adoption.** A device that belongs to no person adopts the person of a
  log that arrives on the connection it paired on, if all of this holds:
  the log is valid; it contains an `add` for this device that is signed by
  the device at the other end of that connection; and at the end of the
  log both that device and this one are valid. It then stores the owner
  and the log in one step. A device that already belongs to a person never
  adopts another, and a pairing between devices of two different people
  changes neither log. Pairing trusts whoever holds the secret; it does not
  verify a human identity.
- When the newcomer has been enrolled, the client side sends `hello` again
  and reads `owner: true` from the new `welcome`.
- The shared test vectors of section 8 pin the secret bytes, the room id
  and both `mac` values.

The code has 16 characters by decision (R-152, `risks.md` Q-2). Eight would
need a password-authenticated key exchange, which is not added. Any text
that still shows an 8-character sample is out of date.

## 7. What is deliberately absent

- No application-layer encryption: every channel is DTLS end to end, and
  signalling carries nothing secret.
- No certificate authority, no accounts, no server-side identity.
- No canonical JSON, no key rotation, no multi-signature.

## 8. Shared test vectors

`internal/proto/testdata/crypto.json`, produced by backend task B-02 and
read by the Go tests and by the web client's tests. Both must pass every
case. Its shape:

```json
{"keys":[{"name":"browser","seed":"<32 bytes>","device":"<device id>",
          "fingerprint":"3F9A 0C21 77BE"}],
 "envelopes":[{"name":"valid","p":"{\"t\":\"post\",…}","k":"<device id>",
               "s":"<signature>","ok":true}],
 "rendezvous":[{"nonce":"q0J…","k":"<device id>","s":"<signature>"}],
 "pairing":[{"shown":"7K2M-9QXD-4F8T-WB3N","secret":"7K2M9QXD4F8TWB3N",
             "room":"<32 hex digits>",
             "a":{"device":"<device id>","mac":"<mac>"},
             "b":{"device":"<device id>","mac":"<mac>"}}],
 "devlogs":[{"name":"add, then revoke","user":"<user id>",
             "entries":[{"env":{"p":"…","k":"…","s":"…"},"accept":true}],
             "valid":["<device id>"]}]}
```

| Part | Meaning | Cases it must hold |
| --- | --- | --- |
| `keys` | `seed` is the 32-byte Ed25519 seed; `device` and `fingerprint` follow from it (section 2) | at least two keys |
| `envelopes` | verifying `s` for `k` over `"solodawn/1\n"` + `p` gives `ok` | a valid one; the same with one byte of `p` changed; one signed by another key; one whose `p` holds non-ASCII text and escaped characters, to pin "the exact bytes" |
| `rendezvous` | `s` is the signature over `"solodawn/rv/1\n"` + `nonce` (`contracts/rendezvous.md` 2.1) | one |
| `pairing` | `shown` is the code as displayed, `secret` its canonical bytes as text; `room` and both `mac` values follow (section 6) | one |
| `devlogs` | the entries are applied in order to an empty log of `user`; `accept` is what section 4 requires for each; `valid` is the set of valid devices afterwards, sorted | genesis and an add; an add by a device that is not in the log; a revoke; an add signed by a revoked device; a second add of a revoked key; a gap in `ver`; a second, different entry for a version already accepted; a first entry whose `dev` is not its signer |

Every field is always present (`contracts/conventions.md` section 2).
