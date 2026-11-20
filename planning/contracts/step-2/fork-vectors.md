# Contract (step 2): vectors of the fork rule

The shared test vectors that hold the Go node and the web client to the
fork rule of the device log (`contracts/step-2/authority.md` section 7,
design point D-4). It adds to `contracts/vectors.md` and is folded into it
when step 1 is built. The file split from the authority contract only for
its length; the rule itself is there.

## 1. What changes, and where

Unit T2-4 changes these files in one pull request:

| File | Change |
| --- | --- |
| `internal/proto/testdata/crypto.json` | the part `devlogs` (section 2) and a new part `forks` (section 3) |
| `internal/identity/vectors_test.go` | the generator of that file and its Go reader |
| `web/src/lib/vectors.test.ts` | its web reader |
| `internal/identity/devlog.go`, `web/src/lib/devlog.ts` | the rule itself |

## 2. `devlogs`

`accept` true gains a third meaning beside "appended" and "the entry
already held": the entry replaced the one held at its version.

The case "for a version already held, a different entry and the same entry
again" of `contracts/vectors.md` section 4 keeps its entries, its flags
(true, true, false, false, true, true) and its `valid`. Only its name
changes, to "an entry that loses to the one held", and the reason of two
refusals:

| Entry | Step 1 | Step 2 |
| --- | --- | --- |
| the third: version 2 again, signed by the device that signed the entry held | refused: first accepted wins | refused by the tie-break: its `s` (`CQDZ…`) is a higher text than that of the entry held (`6z6X…`) |
| the fourth: version 2 again, signed by the device that entry 2 adds | refused: first accepted wins | refused by rule 5: its signer is not in the log before version 2 |

No other case of the part names a held version with another entry
`[verified]` (read from the file). The outcome of the third entry rests on
those signature bytes: the generator asserts the order of the two texts, so
that a change of keys cannot turn the case silently.

## 3. `forks`, a new part

```json
{"name":"two devices revoke each other","user":"<user id>",
 "entries":[{"p":"…","k":"…","s":"…"}],
 "orders":[[0,1,2,3],[0,1,3,2]],"log":[0,1,2],"valid":["<device id>"]}
```

| Field | Meaning |
| --- | --- |
| `entries` | signed entries of the log of `user`, in no order |
| `orders` | lists of indexes into `entries`. Each list is applied, entry by entry, to an empty log |
| `log` | the indexes of the entries every order must end with, in the order of their versions |
| `valid` | as in `devlogs`: the devices of `keys` in the valid set afterwards, sorted as byte strings |

Every field is always present. Cases the part must hold:

- two devices revoke each other at one version (A9.7): the entry of the
  earlier device stays, in both orders;
- two entries of one device for one version: the lower `s` stays;
- two first entries: the lower `s` stays, and what the other log held is
  judged again;
- the loser first, followed by an entry that still fits and by one that
  does not: the first is kept, the second and all after it are dropped;
  the same entries with the winner first end with the same log;
- an entry by an earlier device that fails rule 5 or rule 6 against the
  entries before its version: nothing changes;
- the first device, revoked at a version, signs for that version and wins;
- a `revoke` written where section 7 of the authority contract says, then
  entries of the revoked device for that version and for every lower one:
  all refused;
- a device whose `add` was dropped is added again by another device.

## 4. Not verified

Nothing here was run. The cases were worked by hand and in the throwaway
model that `contracts/step-2/authority-record.md` section 3 describes.
