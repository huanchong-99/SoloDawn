# Parallel plan: steps 2 and 3

The design points to settle before code, a proposed answer for each, the
contract changes as units, and the code units that follow. Rules, lanes and
waves are in `steps/parallel-plan.md`; the outline being refined is
`steps/step-2-3.md`. Columns of the unit tables are those of
`steps/parallel-plan-units.md`.

The owner accepted D-1, D-4 and D-11 as written and the other answers as
proposals; a contract unit or its reviewer may still object. The contract
units also settle what the review of this plan found open: the proof per
account and item kind (D-2), a tie-break for one signer (D-4), which shown
fields a signature covers (D-5), authentication of an invitation (D-7),
receipts and a signed redirect in the snapshot (D-11).

## 1. Design points and proposed answers

| # | Point | Proposed answer | Reason |
| --- | --- | --- | --- |
| D-1 | Who caused a task, for the level cap of R-44 (known point 1) | The person who confirmed the brief, or who made the direct request, caused the task. The owner's confirmation of a request for their AI (the `confirm` card) permits the AI to take part and does not make the owner the cause. The task session gets the owner's level when the cause is the owner, otherwise the lower of that level and 2 | R-44 is about whose instruction the AI follows; one signer of one envelope decides, with no new field |
| D-2 | What a host requires before it acts for a home that is not its owner's (step-2-3, "What step 1 already provides") | An item is acted on when its `cause` holds an envelope signed by a device of the host's owner (brief confirmation or `act confirm`), or when the person who signed the cause is in the allow list the owner signed for that AI | the fields are already on the wire; one rule covers R-51, R-53 and R-115 |
| D-3 | Check commands on other people's machines (known point 2) | The host keeps, per group, the hash of the check list and forge templates its owner accepted. `run_checks`, push and merge refuse with wait `accept` until the owner sends `checks.accept` with the current hash. Any change of the list asks again | one mechanism for checks and for forge templates (D-10); nothing runs that the machine owner has not seen |
| D-4 | Stolen-device race in the device log (known point 4, `risks.md` K-4) | Two entries for one version: the one signed by the device that is earlier in the log wins, on every receiver. The other and what follows it is dropped. Stated limit: who holds the oldest device wins | deterministic, so receivers agree without a new protocol; the first device is the person's id already |
| D-5 | What a browser trusts (known point 3) | For a home that is not one of the viewer's own devices, the browser verifies the envelope of a row against the author's device log before showing the author, and shows an "unverified" mark otherwise. Rows of the viewer's own home are shown as now | the envelopes are in the rows; no extra traffic |
| D-6 | A secretary on a personal subscription (known point 5) | No mechanism. The interface warns when the secretary of a group with several people is on a personal subscription; the documentation recommends a team plan or key | the rule R-51 must not get an auto-accept switch |
| D-7 | Invitation | The link carries group id, home device id, fingerprint and a one-time secret, and joining reuses the pairing room and `mac` of `contracts/envelope.md` section 6 with role `join`. Roles: `owner` (one, the home's owner), `admin`, `member` | pairing already solves "two devices that have never met"; no second handshake |
| D-8 | The `confirm` card | One card kind for three requests: directing someone's personal-subscription AI, calling it with `ask`, and watching its terminal. `body.what` says which. The signed `act confirm` travels as the `cause` | one kind, one command, three uses |
| D-9 | Messages from others after a task started (R-52) | Delivered with the label `information` in the turn text. A change of direction is a new brief or a new `confirm` card | no judgement by the node about what a message means |
| D-10 | Other forges | Per group: four templates (push, open request, read checks, merge) as argument lists with named places, never a shell string. Accepted per machine by D-3 | no shell parsing; the owner sees what will run |
| D-11 | Moving a group; a copy for safekeeping (`risks.md` Q-12) | Move: the home stops writing, sends a signed snapshot (rows, counters, inbox) to the new home, and the group's owner signs the new home id; clients follow the `groups` push. Copy: the same snapshot sent on a schedule to a second node, read-only, restored by hand. No live replication | the home stays the single writer (`architecture.md` 7) |
| D-12 | Checks in a sandbox | Decide after spike S-10. If one real offline build and test passes inside bubblewrap on Linux and `sandbox-exec` or the vendor's sandbox runtime on macOS, checks run inside it there; Windows is a stated limit | the outline marks it unverified |
| D-13 | Approver delegates (R-42) | A list of persons per AI in the list the owner signs (the same list as `allow`). The host accepts a decision signed by a device of the owner or of a listed person | reuses the signed list of D-2 |
| D-14 | Watching a terminal (R-81) | A receive-only channel `watch:*` served by the AI's host, fed from the bridge of step 1, on a `direct` path only, opened after a `confirm` card with `what: watch` | the same limits as takeover |
| D-15 | Team-wide group | Group kind `team`: `task`, `brief` and `host.join` are refused, no clone exists; a secretary posts there with the `post` tool and a group id | least change: refusals, not a second model |
| D-16 | Which CLIs first (step 3.1) | Desk research on terms and capabilities for OpenCode, Qwen Code and Amp in one unit; drivers are built only for those that pass both, API key first. A CLI without takeover is offered with takeover off | the outline's order; nothing is built on an unchecked claim |
| D-17 | Style presets (step 3.2) | A note of type `preset` per group; a brief names one; its text is put into the `assign` turn | notes already outlive retention |
| D-18 | Knowledge base (step 3.3) | Notes of type `kb` with a full-text index on the home; tools `kb_put` and `kb_search`; exempt from retention | one table, two tools |
| D-19 | TCP simultaneous open (step 3.4) | Not in version 1. It is built only if S-9 shows a clear benefit, and S-9 needs real NATs | the outline's own condition |

## 2. Contract units

Planning tasks, built by Fable and reviewed by Astra, all in wave 1. Each
writes new files, so that no step 1 contract has two writers. They are
folded into the main contract files after step 1 is built.

| Id | Writes | Settles | Owns |
| --- | --- | --- | --- |
| C2-0 | Acceptance tests of steps 2 and 3, numbered A9.x (step 2) and A10.x (step 3) | what "done" means | `planning/steps/step-2-3-acceptance.md` |
| C2-1 | Authority across owners: the host's rule, the signed list with `allow` and delegates, the level cap, the fork rule of the device log with its vectors | D-1, D-2, D-4, D-13 | `planning/contracts/step-2/authority.md` |
| C2-2 | People: `invite`, `join`, roles, `member remove`; the joining flow | D-7 | `planning/contracts/step-2/people.md` |
| C2-3 | The `confirm` card: body, states, `act confirm`; the `information` label | D-8, D-9 | `planning/contracts/step-2/confirm.md` |
| C2-4 | The `watch:*` channel; group kind `team` | D-14, D-15 | `planning/contracts/step-2/watch-team.md` |
| C2-5 | Acceptance of check lists by machine owners; forge templates | D-3, D-10 | `planning/contracts/step-2/checks-forge.md` |
| C2-6 | Moving and copying a group: snapshot shape, commands, what clients do | D-11 | `planning/contracts/step-2/move.md` |
| C2-7 | What the browser verifies and shows | D-5, D-6 | `planning/contracts/step-2/browser-trust.md` |
| C3-1 | The capability table a driver declares, and what is switched off without each | D-16 | `planning/contracts/step-3/drivers.md` |
| C3-2 | Note types `preset` and `kb`; `kb_put`, `kb_search`; the brief's preset field | D-17, D-18 | `planning/contracts/step-3/knowledge.md` |

Spike **S-10** (wave 1, Astra, a Devin Linux machine and a Devin macOS
machine): the experiment of D-12, compatibility first, overhead second. Its
answer goes into C2-5 before T2-12 starts.

## 3. Code units of step 2

All in new files. They start when their contract unit and P-4 are merged
and the step 1 file they call exists. Each installs itself through the
tables of step 1 (commands, tools, subcommands, channel labels, mounting
points); where a step 1 file must still change, the unit names that file
in its pull request and is its only writer in that wave. C2-5 and C3-1
are complete only when S-10 and T3-0 have reported.

| Id | W | Does | Tests | Owns | Needs | M | Sz |
| --- | --- | --- | --- | --- | --- | --- | --- |
| P-4 | 4 | Wire shapes of step 2: `invite`, `join`, role and removal, `confirm` body and states, group kind `team`, `checks.accept`, templates, the snapshot, `watch` | golden round trip | lane PROTO | P-3, C2-1 to C2-6 | F | L |
| T2-1 | 5 | Home: invitations, joining, roles, removing a member; admission of a joiner by the node and the browser | A9 people | `home/cmd_people.go`, `node/admit.go`, `lib/join.ts` | P-4, HM-5 | A | M |
| T2-2 | 5 | Home: the `confirm` card and `act confirm`; who must be asked | A9 confirm | `home/cmd_confirm.go` | P-4, HM-10 | F | M |
| T2-3 | 5 | Host: the owner-signature rule, the signed list, the level cap, delegates | A9 authority | `host/authority.go` | P-4, HS-13, HS-16a | F | M |
| T2-4 | 5 | Device log fork rule in Go and in the web client, with the vector generator and the fork vectors in the same pull request | the new vectors | `internal/identity/devlog.go`, `lib/devlog.ts`, the fork vectors of `crypto.json` and their generator | P-4 | F | S |
| T2-5 | 5 | Browser: verify row envelopes for homes of others | A9 trust | `lib/verify.ts` | P-4, C2-7 | A | S |
| T2-6 | 5 | Host: the receive-only terminal channel | A9 watch | `host/watch.go` | P-4, HS-19, HS-20 | A | S |
| T2-7 | 5 | Home: group kind `team` | A9 team | `home/team.go` | P-4, HM-5 | A | S |
| T2-8 | 5 | Host: forge templates | A9 forge | `host/forge_tmpl.go` | P-4, HS-7a, HS-7b | A | M |
| T2-9 | 5 | Host: acceptance of the check list and templates | A9 checks | `host/accept.go` | P-4, HS-5 | A | S |
| T2-10 | 5 | Moving a group to another home | A9 move | `home/move.go`, `node/move.go` | P-4, HM-1, N-4 | F | M |
| T2-11 | 5 | A copy of a group on a second node | A9 copy | `home/copy.go` | P-4, HM-1 | A | M |
| T2-12 | 5 | Checks inside the system's sandbox on Linux and macOS, only where S-10 showed both compatibility and isolation | A9 sandbox | `host/sandbox_linux.go`, `host/sandbox_darwin.go` | S-10, HS-5 | A | M |
| T2-13 | 5 | The `information` label in turn text | A9 label | `host/turn_info.go` | P-4, HS-14 | A | S |
| U2-1 | 5 | Members, roles and the invitation dialog | A9 as seen | `cmp/people/` | U-0, P-4 | F | M |
| U2-2 | 5 | The `confirm` card; the allow list and delegates editor | A9 as seen | `cmp/confirm/`, `cmp/allow/` | U-0, P-4 | F | M |
| U2-3 | 5 | Watching a terminal; team-wide group screens | A9 as seen | `cmp/watch/`, `cmp/team/` | U-11, P-4 | F | M |
| U2-4 | 5 | The unverified mark; the prompt to accept checks; move and copy in settings; the secretary warning | A9 as seen | `cmp/trust/`, `cmp/move/` | U-12, P-4 | F | M |
| I2-1 | 6 | Step 2 acceptance with two persons, three nodes and fake CLIs | A9.x | `features/s2*` | T2-1 to T2-13 | A | L |
| I2-2 | 7 | Step 2 in the browsers and on three systems | A9.x as seen | `web/e2e/s2.spec.ts` | I2-1, U2-1 to U2-4 | A | M |

## 4. Code units of step 3

| Id | W | Does | Tests | Owns | Needs | M | Sz |
| --- | --- | --- | --- | --- | --- | --- | --- |
| T3-0 | 1 | Desk research: terms and capabilities of each candidate CLI, each claim with its source (no code; the answer goes into C3-1) | — | pull request description | — | A | S |
| P-5 | 5 | Wire shapes: note types `preset` and `kb`, the brief's preset field, `kb_put`, `kb_search` | golden round trip | lane PROTO | P-4, C3-2 | F | S |
| T3-1 | 4 | The capability table and the switches it drives | A10 capabilities | `host/caps.go` | C3-1, HS-1, HS-2 | A | S |
| T3-2 to T3-4 | 5+ | One driver and one fake CLI per CLI that passed T3-0, one at a time | M3, M4, M7 tests for that CLI | `host/<cli>.go`, `host/fakecli/<cli>/` | T3-1, captures | A | 3 × M |
| T3-5 | 6 | Presets: the note type, the brief field, the turn text | A10 presets | `home/preset.go`, `host/turn_preset.go` | P-5 | A | S |
| T3-6 | 6 | Knowledge base: index, `kb_put`, `kb_search`, exempt from retention | A10 knowledge | `internal/store/kb.go`, `home/kb.go`, `host/tools_kb.go` | P-5 | A | M |
| U3-1 | 6 | Preset choice in the brief card; the preset editor | A10 as seen | `cmp/preset/` | U-7, P-5 | F | S |
| U3-2 | 6 | Knowledge base panel in the group info | A10 as seen | `cmp/kb/` | U-14, P-5 | F | S |
| I3-1 | 7 | Step 3 acceptance, for the CLIs that passed T3-0 only | A10.x | `features/s3*` | T3-1 to T3-6, U3-1, U3-2 | A | M |

Fable ends on Nov 23. If step 2 or 3 interface units slip past that date,
the coordinator decides who makes their visual decisions; no GPT session
does.

## 5. Not verified

- None of the proposed answers was checked against the vendors' terms or
  against code; they follow from the contracts as written.
- D-4 was not checked against every case of `risks.md` K-4.
- Which candidate CLIs still have headless mode, resume and MCP.
