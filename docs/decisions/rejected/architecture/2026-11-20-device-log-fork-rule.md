# Agent Note: Device log forks: the earlier device wins, and what followed is applied again

Status: rejected — the owner's rule of 2026-11-21 keeps stolen-device defence out of the code; step 2 keeps the step 1 rule (cut 2026-11-22)

## Problem

Every node and browser replays a person's device log by the same rules
([envelope contract](../../../../planning/contracts/envelope.md) section 4). Two entries for one
version make a fork. Step 1 keeps the entry a receiver saw first, so two receivers can hold
different logs for one person (`planning/risks.md` K-4). From step 2 the homes of other people
hold a copy of the log, and a host read it to decide whose word it acts on (section 1 of the
step 2 authority contract as it stood on 2026-11-20), so a disagreement about a log became a
disagreement about authority.

The owner accepted design answer D-4 of `planning/steps/parallel-plan-step-2-3.md` as written:
"Two entries for one version: the one signed by the device that is earlier in the log wins, on
every receiver. The other and what follows it is dropped." Contract unit C2-1 found two gaps in
that text.

1. **Dropping all that follows does not make receivers agree.** A log holds 1 (genesis, device
   A), 2 (A adds B) and 3 (A adds C). B signs entry 4; C, which has seen it, signs entry 5; A
   signs another entry 4. A receiver that gets B's 4, C's 5 and then A's 4 drops both and holds
   three entries and A's 4. A receiver that gets A's 4 first refuses B's 4 and then appends C's
   5, which fits. An entry does not name the entry before it, so the second receiver cannot know
   that C built on the loser. The two stay apart until they next exchange whole logs.
2. **One device can sign two entries for one version**, and D-4 says nothing about it.

## Proposal

For an entry whose version the log holds and which is not the entry held there, as written in
section 7 of the authority contract:

1. The entry is judged against the entries before that version, by the rules of the log.
2. It wins when its signer's first `add` has the lower version than that of the held entry's
   signer. When one device signed both, the entry whose `s` is the lower text wins.
3. When it wins, the entries from that version on leave the log and the entry is appended.
   The entry it replaced is dropped. The others that left are applied again in order, up to
   the first that is refused. That one and all after it are dropped.

The change against D-4 is the third point: "what follows is dropped" becomes "what follows is
applied again". The second point adds the tie-break D-4 lacked.

The example of the problem under this rule: the first receiver holds B's 4 and C's 5 when A's 4
arrives. A is earlier than B, so A's 4 replaces B's 4, which is dropped; C's 5 is applied again
and fits, because C is still valid and 5 is the next version. It holds 1, 2, 3, A's 4 and C's 5.
The second receiver holds A's 4, refuses B's 4, which loses, and appends C's 5: the same five
entries. Under the first wording of the rule the replaced entry was applied again as well; it
is refused at once, and with "up to the first that is refused" nothing after it was ever kept.

A client that revokes a device with an earlier device writes the `revoke` for the lowest version
above that device's first `add` whose entry it, or a later device, signed; it then adds again the
devices this dropped that the person keeps. Nothing here is built: unit T2-4 builds the rule and
its vectors.

Origin: the owner accepted D-4 as written. The agent of contract unit C2-1 proposed this
refinement in pull request 168. The coordinator (the main session) accepted it on 2026-11-20,
acting under the owner's written authorization that delegates to the coordinator every decision
that needs the owner's assent. The owner did not decide it in person.

## Alternatives considered

**All that follows is dropped (D-4 as accepted).** It lost for the reason in point 1 of the
problem: the outcome depends on the order of arrival, which is what the rule exists to remove.

**The first entry accepted keeps its version (step 1).** Receivers that saw different entries
first never agree, and a stolen device that races its owner's `revoke` wins on some of them.

**For one signer, the entry held stays.** That is the step 1 rule again for a device that signs
twice, the one case a stolen device can produce at will.

**For one signer, compare the 64 signature bytes.** The same order in effect, with a decoding
step in the web client. The canonical text is compared with the language's own string comparison
in Go and in the browser.

**A `revoke` always goes to the next version.** The revoked device then replaces an older entry
that it, or a later device, signed, and the `revoke` after it no longer holds. In the model
below the revoked device changed the log in 1,099 of 2,896 cases.

**A message that sends the winning entry back to a peer that offered the loser.** Not needed: a
receiver already forwards every entry it accepts, and peers exchange whole logs after `welcome`.

## Acceptance criteria

- The part `forks` of `internal/proto/testdata/crypto.json` holds the cases of the fork
  vectors (`planning/contracts/step-2/fork-vectors.md`, deleted on 2026-11-22), and every listed
  order ends with the same log in the Go node and in the web client.
- Acceptance test A9.7 of `planning/steps/step-2-3-acceptance.md` passes end to end.
- `planning/contracts/envelope.md` section 4 carries the rule when step 2 is folded into it, and
  this record moves to `implemented/`.

## Risks

- Who holds the oldest device wins. A first device cannot be taken back, and a `revoke` signed
  by a later device holds only while the older one signs nothing against it. D-4 states this
  limit; the refinement does not change it.
- A device can lower the `s` of its own entry by signing again, and so replace it. The placement
  of a `revoke` is what keeps this from undoing a `revoke`.
- A `revoke` written below the end of the log drops later entries. The client adds the kept
  devices again; until those entries arrive, a receiver refuses those devices.
- Entries that arrive out of order are refused as a gap, as in step 1. Agreement rests on peers
  exchanging whole logs in order.

## Verification

Nothing was run in the product. The agent of C2-1 ran a model in Python and did not keep it. An
entry was a tuple of version, signer, `add` or `revoke`, device and a number standing for `s`;
no keys were used. The model applied again the entries after the replaced one, not that one,
as point 3 now says.

- **Agreement.** 10,000 runs (25 seeds of 400): 2 to 5 receivers, 3 to 25 entries signed on a
  receiver's own log, 3 in 10 of them for a version already held, each forwarded to every other
  receiver with probability one half. The receivers then exchanged whole logs in pairs until
  nothing changed. Every run ended with one log on all receivers, after at most 4 rounds.
- **A `revoke` that holds.** 3,000 random logs, 2,896 of which had a valid device earlier than
  the one revoked. After the `revoke`, the revoked device tried every version with every `add`
  and `revoke`, each with the lowest `s`. With the `revoke` placed as proposed, no entry changed
  a log. With the `revoke` appended, 1,099 logs changed.

The model does not cover signature checks, the 2 KiB limit or entries that arrive out of order.

## Why rejected

On 2026-11-22 the step 2 plan was cut to the owner's rules of 2026-11-21: least code, and no
defence in code against a stolen device or a dishonest party (`planning/steps/parallel-plan-step-2-3.md`,
"Cuts of 2026-11-22"). A host no longer reads other people's logs to decide whose word it acts on;
the home of a group decides. The step 1 rule stays: for a version already held, the first entry
accepted wins, and `planning/risks.md` K-4 stays a stated limit (add one device at a time; recover
by hand). Unit T2-4, which would have built this rule, is cancelled.
