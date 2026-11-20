# Contract (step 2): authority across owners, the record

Where `contracts/step-2/authority.md` goes when step 1 is built, what
changed in other files with it, what the coordinator settled, and what is
not verified. Split from that file for its length; the rules are there,
and a section named below without a file is one of its sections.

## 1. Where it goes, and what changed elsewhere

| File | Gains |
| --- | --- |
| `contracts/envelope.md`, `contracts/vectors.md` | section 4: the fork rule; `contracts/step-2/fork-vectors.md`; the part `accept` with `repo` and `base` (`contracts/step-2/checks-forge.md` section 2) |
| `contracts/commands.md`, `contracts/rows.md` | 1.1: who may set `secretary`; 1.2: `allow` from the host; 1.3: unique titles in a brief; 1.4: `arg` on a brief's `confirm`, who may `approve` and `deny`; section 2: `allow.set`, the cap under "Levels". Section 1: `secretary_by`; section 2: `secretary_joined` |
| `contracts/datachannel.md`, `contracts/conventions.md` | section 6: `req`, and section 3 in place of "What a host accepts (step 1)"; section 7: the lists in `host.info`. Section 4: an item with `req` stays under 54 KiB |
| `contracts/messages.md`, `contracts/storage.md` | 2.2: `start` in the `task` body. Host table `allow`; the open calls and decision cards and, on a task session, `capped` and the pull request it opened or reused; the exemption of section 6 |
| `flows/approval.md`, `flows/agent-turn.md`, `flows/task.md` | steps 9 and 10; step 2, the level of step 5 and the labels of step 6; E: the check before a merge; G: the `re` of section 4 |
| `contracts/mcp-tools.md`, `contracts/drivers.md` | section 2: the secretary's tools only by the consent of section 3. What `contracts/step-2/labels.md` section 5 lists |
| `flows/pairing.md`, `flows/takeover.md` | D: where a client writes a `revoke`, and adding again what it dropped. A: a task registered for a takeover is O's own; a capped session stays capped in the terminal |

Changed with this file: `confirm.md` (the request and the word behind a task
without a brief; a brief's `arg`; a role the brief does not give; labels),
`browser-trust.md` (a `task` request; 8.1), `checks-forge.md` (the hash) and
`watch-team.md` here, with `labels.md` and `fork-vectors.md` split off;
`contracts/step-3/knowledge.md` section 8 (`v` 1 from a host); the
acceptance tests, with those of step 3 split off; `risks.md` K-4.

P-4 freezes `allow.set`, `req`, `body.start`, `secretary_by`,
`secretary_joined`, a brief's `arg` and the lists in `host.info`. T2-3 builds sections 2 to 5 on the
host, T2-2 the home's side (sections 5 and 6), T2-4 section 7 and its
vectors, T2-13 the labels.

## 2. Settled by the coordinator (2026-11-20)

| | Written here | The other answer |
| --- | --- | --- |
| 1 | The signed list is a host command; the member row is a copy | a list signed into the row: a home could show an old one, so the host would need versions |
| 2 | A brief's `confirm` carries the digest of the brief, a change to a step 1 command under `v` 2 | `msg` alone: a home can put another brief under the same id, and with it its own work at the owner's level |
| 3 | The request of a task without a brief is its signed `task start`, shown in the `task` message | the message that led to it: a home could hang any plan on an old message of the owner |
| 4 | D-4 refined: after a replacement the entries that followed are applied again, not all dropped; one signer: the lower `s`. Record: `docs/decisions/proposed/architecture/2026-11-20-device-log-fork-rule.md` | all dropped: a receiver that saw the winner first keeps an entry that another has dropped |
| 5 | Where a `revoke` is written, and adding again what it dropped | always the next version: the revoked device replaces an older entry and is valid again |
| 6 | Only O's signature as a person carries O's level | "O's word", also through O's AIs: a capped task could raise itself by a call |
| 7 | The hash of `checks.accept` covers `repo` and `base`; N merges only the pull request it opened or reused, after reading its base and head (T2-3, `internal/host/merge.go`) | a limit: a node merges into whatever the group row and the item name |
| 8 | A role the brief does not give an AI needs its owner's card, except on that owner's own home when the owner, or one of the owner's AIs, signed the `task assign` | a card on every home: a one-person group would ask at each reassignment, against A9.40. No card on the own home at all: an admin could put the owner's AI to work at the owner's level, against R-44 |
| 9 | A secretary serves by its owner's consent: joined as one, or named by a `group set` its owner signed; the home refuses any other | the group row alone: a home could make any AI take every `notice` |

## 3. Not verified

- Nothing here was run in the product. A throwaway model of section 7 (no
  keys, random logs and orders) ended with one log on every receiver in
  10,000 runs; after a `revoke` written as that section says, no entry of
  the revoked device changed a log in 2,896.
- `[unverified]` Read against `contracts/`, not the host code of step 1.
  `gh pr view --json baseRefName,headRefName` was not run; step 1 reads
  `baseRefName` in the same way (`internal/host/forge.go`).
