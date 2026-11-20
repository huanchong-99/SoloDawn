# Contract: identities and the signed envelope

One primitive authenticates everything: the signed envelope. This file
defines it, the device log that says which devices belong to a person, and
the pairing exchange that introduces two devices that have never met.

The Go node and the web client both implement this file and must give the
same answer for the same bytes. The test vectors of section 8 hold them to
it.

## 1. Encodings

- Binary values in JSON are base64url without padding, and only the
  canonical spelling is a value: the text that encoding the bytes gives. A
  reader decodes, encodes again and compares. Padding, `+` or `/`, a line
  break, or other values in the unused last bits make a text no value at
  all. (A tolerant decoder gives one key several spellings, and a revoked
  key could then be added again under another one `[verified]`.)
- Times are integer milliseconds since the Unix epoch, UTC.
- Random ids are 16 random bytes, base64url (22 characters). An id is
  compared as text and never decoded.

## 2. Device

- A device is one node installation or one browser profile on one origin.
- It owns an Ed25519 key pair, generated on first run.
  - Node: the file `device.key` of the data directory
    (`contracts/local.md` section 2).
  - Browser: a non-extractable WebCrypto key stored in IndexedDB. Only
    browsers with native Ed25519 in WebCrypto are supported; no signature
    library is bundled (R-164). `[unverified]` The exact minimum versions;
    spike S-7 fixes them.
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

  Both implementations compare these bytes themselves and do not leave it
  to their Ed25519 library, because the libraries differ `[verified]`: Go
  and the three browser engines accept a forged signature under the
  identity point; OpenSSL, which runs the web client's unit tests, refuses
  it.
- A text that is not a device id by these rules is never used as a key: as
  `k` it fails `bad_sig`, as `dev` or `user` it makes a log entry refused,
  and a `pair` message that carries it is ignored.
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

1. `bad_sig`: `k` is a device id (section 2); `s` is 64 bytes in their
   canonical spelling, of which the first 32 are not one of the fourteen
   values of section 2; and `s` verifies for `k` over the prefix and `p`.
   (Go accepts a signature that begins with the identity point, OpenSSL
   refuses it `[verified]`; comparing the bytes makes the answer the same
   everywhere.)
2. `unknown_device`: `k` is a currently valid device of someone the receiver
   has a reason to listen to (the receiver's owner; a member of a group it
   homes; the home of a group it participates in; or a pairing peer, section
   6).
3. `bad_request`: `p` is a JSON object in which `t` and `n` are strings and
   `ts` is an integer, and `g` and `to`, when there, are strings. The
   payload is parsed at this point because the two checks that follow read
   `to` and `ts`. Fields are read as `contracts/conventions.md` section 2
   says, and an integer is a number whose value is one, however it is
   written: `2.0` is 2, because a browser cannot tell the two apart.
4. `not_for_me`: `to` is the receiver's device id. A signalling payload
   must carry it. Another payload may have none, and an empty `to` is read
   as none; a host command without one is refused with the same code by
   the node that receives it (`contracts/commands.md` section 2).
5. `stale`: `ts` is at most two minutes from the receiver's clock for a
   signalling payload, and at most 24 hours for a command, earlier or
   later. Exactly two minutes, or exactly 24 hours, still passes.

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
2. `p` is a JSON object in which `t` is `"device"`, `user` is this log's
   user id, `op` is `"add"` or `"revoke"`, `dev` is a device id, and `ver`
   is an integer as section 3 reads one. On an `add`, `kind` is `"node"`
   or `"browser"` and `name` is a string. Nothing else is read: not `n` or
   `ts`, not `kind` or `name` on a `revoke`, not a field under another
   name (`contracts/conventions.md` section 2).
3. `ver` is the number of entries in the log plus one.
4. Entry 1 is the genesis: `op` is `add`, and `dev`, `k` and `user` are the
   same key.
5. In a later entry, `k` is in the valid set before it.
6. An `add` does not name a device that was ever revoked, and a `revoke`
   does not remove the last valid device: a log with no valid device could
   never change again, and nothing its person owns could be operated.

`add` puts `dev` in the valid set; `revoke` removes it for good.

An entry whose `ver` the log already holds is never appended. If it is the
entry held at that version (the same `p`, `k` and `s`) it is not refused
either, and nothing changes: logs are passed on, so one entry arrives many
times. Any other entry for that version is refused. First accepted wins.

These rules accept, on purpose: a `revoke` of a device that was never added
(it can then never be added); an `add` of a device that is already valid
(the latest `add` gives its `kind` and `name`); a device revoking itself;
and a `revoke` of the first device, after which the user id is still the
person's id.

Anyone can verify a whole log by replaying it from entry 1.

Where logs live and how they spread:

- Every node stores the log of its owner.
- Every home stores the log of each person in its groups and shows entries
  to clients as `device` rows.
- Entries travel in one frame, `devlog` (`contracts/datachannel.md` 3.1),
  always as the original signed entries. A device that creates an entry
  sends it to every one of the owner's nodes and every home it is connected
  to; nodes and homes pass on what they accept; after every `welcome` each
  side sends the logs the other may be missing.

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
  person typed is read by removing the hyphens and making the letters
  upper case, and nothing more: what remains must be 16 characters of
  `0123456789ABCDEFGHJKMNPQRSTVWXYZ`. A space, or one of `I`, `L`, `O`,
  `U`, makes it no secret; no look-alike is mapped to another character.
- **Room id** = the first 16 bytes of SHA-256(`"solodawn/room/1\n"` +
  secret bytes), as 32 lower-case hex characters.
- Each side sends through the room one message:

```json
{"t":"pair","k":"<my device id>","kind":"node","name":"workstation",
 "role":"a","mac":"<HMAC>"}
```

- `role` is `a` for the side that created the secret, `b` for the other.
- `mac` = HMAC-SHA256(key = secret bytes, message =
  `"solodawn/pair/1\n"` + role + `"\n"` + device id), base64url.
- A side accepts a `pair` message when its `role` is the other role (the
  room can hand a side its own message back), its `k` is a device id, and
  its `mac` is the one that follows from that `role` and `k`. It ignores
  any other. After that it knows the other's device id is authentic for
  the next 10 minutes and accepts signed signalling from it. The
  rendezvous service sees the room id and both device ids, and cannot
  forge a `mac` without the secret.
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
need a password-authenticated key exchange, which is not added.

## 7. What is deliberately absent

- No application-layer encryption: every channel is DTLS end to end, and
  signalling carries nothing secret.
- No certificate authority, no accounts, no server-side identity.
- No canonical JSON, no key rotation, no multi-signature.

## 8. Shared test vectors

`internal/proto/testdata/crypto.json`, produced by backend task B-02 and
read by the Go tests and by the web client's tests. Both must pass every
case. Every field is always present. Its shape:

```json
{"keys":[{"name":"browser","seed":"<32 bytes>","device":"<device id>",
          "fingerprint":"3F9A 0C21 77BE"}],
 "envelopes":[{"name":"valid","p":"{\"t\":\"post\",…}","k":"<device id>",
               "s":"<signature>","ok":true}],
 "rendezvous":[{"nonce":"q0J…","k":"<device id>","s":"<signature>"}],
 "pairing":[{"shown":"7K2M-9QXD-4F8T-WB3N","secret":"7K2M9QXD4F8TWB3N",
             "room":"<room id>",
             "a":{"device":"<device id>","mac":"<mac>"},
             "b":{"device":"<device id>","mac":"<mac>"}}],
 "devlogs":[{"name":"add, then revoke","user":"<user id>",
             "entries":[{"env":{"p":"…","k":"…","s":"…"},"accept":true}],
             "valid":["<device id>"]}],
 "adopt":[{"name":"first run","user":"<user id>","me":"<device id>",
           "peer":"<device id>","entries":[{"p":"…","k":"…","s":"…"}],
           "ok":true}]}
```

| Part | Meaning | Cases it must hold |
| --- | --- | --- |
| `keys` | `seed` is the 32-byte Ed25519 seed; `device` and `fingerprint` follow from it (section 2) | at least two keys |
| `envelopes` | `ok` is whether the envelope passes check 1 of section 3. Where `ok` is true, `k` is one of `keys`, and signing `p` with its seed gives exactly `s` | a valid one; the same with one byte of `p` changed; one signed by another key; one whose `p` holds non-ASCII text and escaped characters, to pin "the exact bytes"; `k` in a spelling that is not canonical, and `s` likewise; a forged signature under each of the fourteen weak keys; a signature by a valid key that begins with the identity point |
| `rendezvous` | `s` answers `nonce` for `k` (`contracts/rendezvous.md` 2.1) | one |
| `pairing` | `shown` is the code as a person typed it, `secret` its secret bytes as text; `room` and both `mac` values follow (section 6) | as displayed; in lower case; without hyphens |
| `devlogs` | the entries are applied in order to an empty log of `user`. `accept` true: the entry is not refused, that is, it is appended, or it is the entry already held at its version. False: it is refused. `valid` is the valid set afterwards, the device ids sorted as byte strings, ascending | genesis and an add; an add by a device that is not in the log; a revoke; an add signed by a revoked device; a second add of a revoked key, in its own spelling and in another one; a gap in `ver`; for a version already held, a different entry and the same entry again; a first entry whose `dev`, `k` and `user` are not one key; a payload that is not JSON, and one with another `t` or `op`; `dev` a weak key; `ver` written `2.0`; a field `VER` beside the right `ver`; an `add` with another `kind`, and one without `name`; each entry that section 4 accepts on purpose; a `revoke` of the last valid device |
| `adopt` | `entries` is a valid log of `user`. `ok` says whether device `me`, which belongs to no person and has paired with `peer`, adopts it (section 6) | first run: the browser's genesis and its add of the node; a node added to an existing person; an add of `me` signed by a device other than `peer`; `me` revoked later in the log; `peer` revoked later; a log that never adds `me` |
