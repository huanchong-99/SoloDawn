# Contract: identities and the signed envelope

One primitive authenticates everything: the signed envelope. This file
defines it, the device log that says which devices belong to a person, and
the pairing exchange that introduces two devices that have never met.

The Go node and the web client both implement this file and must give the
same answer for the same bytes. The test vectors of `contracts/vectors.md`
hold them to it.

## 1. Encodings

- Binary values in JSON are base64url without padding, and only the
  canonical spelling is a value: the text that encoding the bytes gives. A
  reader decodes, encodes again and compares. Padding, `+` or `/`, a line
  break, or other values in the unused last bits make a text no value at
  all. (A tolerant decoder gives one key several spellings, and a revoked
  key could then be added again under another one `[verified]`.)
- Times are integer milliseconds since the Unix epoch, UTC.
- Random ids are 16 random bytes in canonical base64url: 22 characters.
  Where a receiver reads a field that holds one, anything else is a wrong
  type (`contracts/conventions.md` section 2).

## 2. Device

- A device is one node installation or one browser profile on one origin.
- It owns an Ed25519 key pair, generated on first run.
  - Node: the file `device.key` of the data directory
    (`contracts/local.md` section 2).
  - Browser: a non-extractable WebCrypto key stored in IndexedDB. Only
    browsers with native Ed25519 in WebCrypto are supported; no signature
    library is bundled (R-164). The lowest versions are in
    `docs/decisions/implemented/architecture/2026-11-20-browser-baseline.md`.
- **Device id** = the 32-byte public key, base64url: 43 characters, in the
  canonical spelling of section 1.
- **Weak keys.** Fourteen 32-byte values are never a device id. They are
  the encodings of the points of small order, under which a signature can
  be made without any private key: the values that, with the highest bit of
  the last byte cleared, are one of these (hex):

  ```
  0000000000000000000000000000000000000000000000000000000000000000
  0100000000000000000000000000000000000000000000000000000000000000
  26e8958fc2b227b045c3f489f2ef98f0d5dfac05d3c63339b13802886d53fc05
  c7176a703d4dd84fba3c0b760d10670f2a2053fa2c39ccc64ec7fd7792ac037a
  ecffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff7f
  edffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff7f
  eeffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff7f
  ```

  Both implementations compare these bytes themselves, in a key and in the
  first half of a signature (section 3), and do not leave it to their
  Ed25519 library, because the libraries differ `[verified]`. Under the
  identity point as key, Go and the three browser engines accept a forged
  signature, and OpenSSL, which runs the web client's unit tests, refuses
  it. A signature by a real key that begins with the identity point is
  accepted by Go and refused by OpenSSL.
- A text that is not a device id by these rules is never used as a key: as
  `k` it fails `bad_sig`, as `dev` or `user` it makes a log entry refused,
  and a `pair` message that carries it is ignored.
- Being a point of the curve is not part of being a device id. Nothing
  verifies under a `k` that is not one (section 3, check 1). A `dev` is
  not tested for it by either implementation, and a browser does not hand
  a `dev` to `importKey` to find out: an entry may add 32 bytes that are no
  point, and that device can never sign.
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
- `s` = the 64-byte Ed25519 signature over the UTF-8 bytes of
  `"solodawn/1\n"` followed by `p`, base64url.
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

1. `bad_sig`: `k` is a device id (section 2). `s` is 64 bytes in their
   canonical spelling: the first 32 are not one of the fourteen values of
   section 2, and the last 32, read as a little-endian number, are below
   the order of the group, 2^252 + 27742317777372353535851937790883648493.
   And `s` verifies for `k` over the prefix and `p`. A `k` whose bytes are
   no point of the curve fails here like any signature that does not
   verify, also where the library refuses such a key before it verifies
   anything (WebCrypto may throw from `importKey`).
2. `unknown_device`: `k` is a currently valid device of someone the receiver
   has a reason to listen to (the receiver's owner; a member of a group it
   homes; the home of a group it participates in; or a pairing peer, section
   6).
3. `bad_request`: `p` is a JSON object in which `t` is a string, `n` an id
   and `ts` an integer; `g`, when there, is an id, and `to` a string. The
   payload is parsed at this point because the two checks that follow read
   `to` and `ts`. How fields, `null` and integers are read is in
   `contracts/conventions.md` section 2.
4. `not_for_me`: `to`, when there, is the receiver's device id; an empty
   `to` is a value like any other, and is not that id. A signalling payload
   must carry `to`; a beacon (`contracts/rendezvous.md` section 4) carries
   none and is not checked for one. A host command without one is refused
   with the same code by the node that receives it (`contracts/commands.md`
   section 2).
5. `stale`: `ts` is at most two minutes from the receiver's clock for a
   signalling payload or a beacon, and at most 24 hours for a command,
   earlier or later. Exactly two minutes, or exactly 24 hours, still passes.

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
| `user` | the person whose log this is |
| `ver` | 1, 2, 3 … with no gaps |
| `op` | `add` or `revoke` |
| `dev` | the device added or revoked |
| `kind` | `node` or `browser`; on an `add` |
| `name` | label chosen by the person; on an `add` |

Every implementation must judge an entry the same way, or logs fork. An
entry is **accepted**, and appended, when all of the following hold. Any
other entry is **refused**: the log stays exactly as it was, and the entries
that come after are judged against that unchanged log.

1. `k` is a device id and `s` verifies (section 3, check 1). No other check
   of section 3 applies: an entry is addressed to nobody and is replayed
   long after it was signed.
2. `p` is at most 2 KiB: 2,048 bytes when encoded as `TextEncoder` encodes
   text. That keeps the `device` row of an entry within the size of a row
   (`contracts/conventions.md` section 4).
   `p` is a JSON object in which `t` is `"device"`, `user` is this log's
   user id, `op` is `"add"` or `"revoke"`, `dev` is a device id, and `ver`
   is an integer. On an `add`, `kind` is `"node"` or `"browser"` and `name`
   is a string. Nothing else is read: not `n` or `ts`, not `kind` or `name`
   on a `revoke`, not a field under another name. How fields, `null` and
   integers are read is in `contracts/conventions.md` section 2.
3. `ver` is the number of entries in the log plus one.
4. Entry 1 is the genesis: `op` is `add`, and `dev`, `k` and `user` are the
   same key.
5. In a later entry, `k` is in the valid set before it.
6. An `add` does not name a device that was ever revoked, and a `revoke`
   does not remove the last valid device: a log with no valid device could
   never change again, and nothing its person owns could be operated.

`add` puts `dev` in the valid set; `revoke` removes it for good.

An entry whose `ver` the log already holds is never appended. If it is the
entry held at that version (the same `k` and `s`, and the same `p` as
`TextEncoder` encodes it, so that a lone surrogate and U+FFFD agree) it is
not refused either, and nothing changes: logs are passed on, so one entry
arrives many times. Any other entry for that version is refused. First
accepted wins.

These rules accept, on purpose: a `revoke` of a device that was never added
(it can then never be added); an `add` of a device that is already valid
(the latest `add` gives its `kind` and `name`); a device revoking itself;
and a `revoke` of the first device, after which the user id is still the
person's id.

Anyone can verify a whole log by replaying it from entry 1.

Where logs live and how they spread:

- Every node, and every browser, stores the log of its owner.
- Every home stores the log of each person in its groups and shows entries
  to clients as `device` rows.
- Entries travel in one frame, `devlog` (`contracts/datachannel.md` 3.1),
  always as the original signed entries. A device that creates or accepts
  an entry sends it to every connected peer that keeps that log, browsers
  included: an entry a node writes reaches the owner's other nodes through
  a browser connected to them (`risks.md` K-24). After every `welcome`
  each side sends every log the peer has a reason to know (section 3,
  check 2), in full from version 1, in several frames, in order, if one
  frame cannot hold it.

Two entries for one version make a fork, and receivers may then hold
different logs. Step 1 does not resolve forks (`risks.md` K-4).

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
  characters in upper case, without the hyphens, as ASCII. A secret that a
  person typed is read by removing the hyphens and turning `a` to `z` into
  `A` to `Z`, and nothing more: what remains must be 16 characters of
  `0123456789ABCDEFGHJKMNPQRSTVWXYZ`. No other character is changed, so a
  space, one of `I`, `L`, `O`, `U`, or a letter outside ASCII makes it no
  secret, also a letter that a general upper-casing would turn into valid
  characters (`\u017f` into `S`, `\u00df` into `SS`).
- **Room id** = the first 16 bytes of SHA-256(`"solodawn/room/1\n"` +
  secret bytes), as 32 lower-case hex characters.
- Each side sends through the room one message:

```json
{"t":"pair","k":"<my device id>","kind":"node","name":"workstation",
 "role":"a","mac":"<HMAC>"}
```

- `role` is `a` for the side that created the secret, `b` for the other.
- `mac` = HMAC-SHA256(key = secret bytes, message = `"solodawn/pair/1\n"`
  + role + `"\n"` + device id + `"\n"` + kind + `"\n"` + name),
  base64url. Each part is the field as it stands in the message after
  JSON decoding, encoded the way `TextEncoder` encodes text: UTF-8, with a
  lone surrogate becoming U+FFFD. Nothing is trimmed, normalised or
  escaped. `name` comes last and may hold line breaks: the three parts
  before it cannot, so the message is read back in one way only.
- A side accepts a `pair` message when its `role` is the other role (the
  room can hand a side its own message back), its `k` is a device id, its
  `kind` is `node` or `browser`, its `name` is a string, and its `mac` is
  the one that follows from those four values. It ignores any other. After
  that it knows that the other's device id, kind and name are authentic
  for the next 10 minutes, and accepts signed signalling from it. The
  rendezvous service sees the room id and both messages, and can change
  nothing in them without the secret: `kind` and `name` are what the side
  that already belongs to a person writes into the `add` entry.
- Until the newcomer is in the log, the connection carries `hello` and
  `devlog` and nothing else.
- Once the data channel is open, the side that already belongs to a person
  appends an `add` entry for the newcomer and sends the whole log in
  `devlog` (section 4). If neither side belongs to a person (first run), the
  browser writes the genesis entry for itself and then the `add` for the
  node.
- **Adoption.** A device that belongs to no person adopts the person of a
  log that arrives on the connection it paired on, if all of this holds:
  the log is valid; it contains an `add` for this device that is signed by
  the device at the other end of that connection; and at the end of the
  log both that device and this one are valid. It then stores the owner
  and the log in one step. A log split over several `devlog` frames is
  judged once complete: when the frame that holds the `add` for this device
  has arrived, since the peer writes that entry last, just before it sends
  the log. A device that already belongs to a person never adopts another,
  and a pairing between devices of two different people changes neither
  log. Pairing trusts whoever holds the secret; it does not
  verify a human identity.
- When the newcomer has been enrolled, the client side sends `hello` again
  and reads `owner: true` from the new `welcome`.
- The shared test vectors pin the secret bytes, the room id and both `mac`
  values.

The code has 16 characters by decision (R-152, `risks.md` Q-2). Eight would
need a password-authenticated key exchange, which is not added.

## 7. What is deliberately absent

- No application-layer encryption: every channel is DTLS end to end, and
  signalling carries nothing secret.
- No certificate authority, no accounts, no server-side identity.
- No canonical JSON, no key rotation, no multi-signature.

## 8. Shared test vectors

The cases that both implementations must pass, and the shape of the file
that holds them, are in `contracts/vectors.md`.
