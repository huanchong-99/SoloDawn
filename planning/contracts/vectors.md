# Contract: shared test vectors

`internal/proto/testdata/crypto.json` holds the cases that keep the two
implementations of `contracts/envelope.md` to one answer: the Go node and
the web client. Every case must pass in both; the web client runs them
under its unit-test runner and in each baseline browser, because their
Ed25519 libraries differ (section 2 of that file). Which task produces the
file is in `contracts/conventions.md` section 5.

Sections named below are those of `contracts/envelope.md`.

## 1. Shape

```json
{"keys":[{"name":"browser","seed":"<32 bytes>","device":"<device id>",
          "fingerprint":"3F9A 0C21 77BE"}],
 "envelopes":[{"name":"valid","p":"{\"t\":\"post\",…}","k":"<device id>",
               "s":"<signature>","ok":true}],
 "rendezvous":[{"nonce":"q0J…","k":"<device id>","s":"<signature>"}],
 "received":[{"name":"to is null","kind":"command","me":"<device id>",
              "now":1763600000000,"known":true,
              "env":{"p":"…","k":"…","s":"…"},"code":"bad_request"}],
 "pairing":[{"shown":"7K2M-9QXD-4F8T-WB3N","secret":"7K2M9QXD4F8TWB3N",
             "room":"<room id>",
             "a":{"device":"<device id>","kind":"browser","name":"phone",
                  "mac":"<mac>"},
             "b":{"device":"<device id>","kind":"node","name":"workstation",
                  "mac":"<mac>"}}],
 "pair_messages":[{"name":"name changed on the way","me":"b",
                   "secret":"7K2M9QXD4F8TWB3N",
                   "msg":{"t":"pair","k":"<device id>","kind":"browser",
                          "name":"laptop","role":"a","mac":"<mac>"},
                   "ok":false}],
 "bad_secrets":[{"name":"a space","shown":"7K2M 9QXD-4F8T-WB3N"}],
 "devlogs":[{"name":"add, then revoke","user":"<user id>",
             "entries":[{"env":{"p":"…","k":"…","s":"…"},"accept":true}],
             "valid":["<device id>"]}],
 "adopt":[{"name":"first run","user":"<user id>","me":"<device id>",
           "peer":"<device id>","entries":[{"p":"…","k":"…","s":"…"}],
           "ok":true}]}
```

Every field is always present.

## 2. Signature bytes

Go signs as RFC 8032 says, so the same key and text always give the same
signature. For every case of `envelopes` whose `ok` is true, and for every
case of `rendezvous`, the Go tests sign again with the seed of `k` and must
get exactly `s`.

The web client's tests never compare signature bytes: Safari's signatures
are randomised (WebKit bug 262499, recorded in
`docs/decisions/implemented/architecture/2026-11-20-browser-baseline.md`).
They verify the `s` of the vectors, and they verify what the client signed
itself.

## 3. Parts

| Part | Meaning |
| --- | --- |
| `keys` | `seed` is the 32-byte Ed25519 seed; `device` and `fingerprint` follow from it (section 2) |
| `envelopes` | `ok` is whether the envelope passes check 1 of section 3. Where `ok` is true, `k` is one of `keys` |
| `rendezvous` | `s` answers `nonce` for `k` (`contracts/rendezvous.md` 2.1); `k` is one of `keys` |
| `received` | the envelope arrives at device `me`, whose clock reads `now`, as a command or as a signalling payload (`kind` is `command` or `signal`); `known` says whether the receiver listens to its signer. `code` is `ok`, or the code of the first check of section 3 that fails |
| `pairing` | `shown` is a secret as a person typed it, `secret` its secret bytes as text; `room` follows, and so does the `mac` of each side from its role, `device`, `kind` and `name` (section 6) |
| `pair_messages` | a side with role `me` that holds `secret` receives `msg`; `ok` says whether it accepts it (section 6) |
| `bad_secrets` | `shown` is a text that is no secret (section 6) |
| `devlogs` | the entries are applied in order to an empty log of `user`. `accept` true: the entry is not refused, that is, it is appended, or it is the entry already held at its version. False: it is refused. `valid` lists the devices of `keys` that are in the valid set afterwards, sorted as byte strings, ascending. A device that is not one of `keys` is never listed, valid or not |
| `adopt` | `entries` is a valid log of `user`. `ok` says whether device `me`, which belongs to no person and has paired with `peer`, adopts it (section 6) |

## 4. Cases each part must hold

**`keys`**: at least two.

**`envelopes`**

- a valid one; the same with one byte of `p` changed; one signed by
  another key;
- one whose `p` holds non-ASCII text and escaped characters, to pin "the
  exact bytes";
- `k` in a spelling that is not canonical, and `s` likewise;
- a forged signature under each of the fourteen weak keys;
- a signature by a valid key that begins with the identity point;
- a valid signature with the order of the group added to its second half;
- a `k` of 32 bytes that is no point of the curve.

**`rendezvous`**: one.

**`received`**

- `ok`: a command without `to`; a command and a signalling payload whose
  `to` is `me`; `ts` written with a fraction of zero; `ts` exactly two
  minutes from `now` in a signalling payload, and exactly 24 hours in a
  command; a field `TO` that names another device beside a `to` that is
  `me`.
- `not_for_me`: `to` is another device; `to` is `""`; a signalling payload
  without `to`.
- `bad_request`: `to` is `null`; `ts` beyond 2^53 − 1; no `ts`; an `n` that
  is no id, written with the escape of a lone surrogate; a `p` that is not
  a JSON object.
- `stale`: one millisecond beyond each of the two windows.
- the order of the checks: a signer the receiver does not listen to, once
  with a signature that does not verify (`bad_sig`) and once with a `p`
  that is not JSON (`unknown_device`); a `to` that is another device with a
  `ts` outside the window (`not_for_me`).

**`pairing`**: a secret as displayed; in lower case; without hyphens; a
`name` with non-ASCII text and a line break.

**`pair_messages`**: a message that is accepted; the same with `name`
changed, and with `kind` changed, each keeping its `mac`; a message in the
receiver's own role; a `k` that is a weak key, and a `kind` that is neither
`node` nor `browser`, each with the `mac` that follows from it.

**`bad_secrets`**: with a space; with one of `I`, `L`, `O`, `U`; one
character short, and one too long; fifteen valid characters and
`\u017f`, which a general upper-casing turns into `S`; fourteen valid
characters and `\u00df`, which JavaScript's `toUpperCase` turns into `SS`.

**`devlogs`**

- genesis and an add; an add by a device that is not in the log; a revoke;
- an add signed by a revoked device; a second add of a revoked key, in its
  own spelling and in another one;
- a gap in `ver`; for a version already held, a different entry and the
  same entry again;
- a first entry whose `dev`, `k` and `user` are not one key;
- a payload that is not JSON, and one with another `t` or `op`;
- `dev` a weak key (refused); `dev` 32 bytes in their canonical spelling
  that are no point of the curve (accepted);
- `ver` written `2.0000000000000001`, which is 2; `ver` that is `null`;
- a field `VER` beside the right `ver`;
- an `add` with another `kind`, one without `name`, and one whose `name`
  is `null`;
- each entry that section 4 accepts on purpose; a `revoke` of the last
  valid device.

**`adopt`**: first run, with the browser's genesis and its add of the node;
a node added to an existing person; an add of `me` signed by a device other
than `peer`; `me` revoked later in the log; `peer` revoked later; a log
that never adds `me`.
