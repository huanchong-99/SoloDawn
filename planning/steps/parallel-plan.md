# Parallel plan for the rest of version 1

How everything left in steps 1, 2 and 3 is cut into small units that many
sessions build at the same time. This file holds the rules, lanes, waves,
critical path and what needs the owner's hardware. The units are in
`steps/parallel-plan-units.md` (step 1) and
`steps/parallel-plan-step-2-3.md` (design decisions, contract units and code
units of steps 2 and 3). `steps/step-1-backend.md` and
`steps/step-1-frontend.md` stay the statement of what each task must do; a
unit here names the part of a task it takes.

State when this was written: merged are B-00, B-01 (a), (b), (d), B-02,
B-02b, B-02c, B-05, B-06, F-00, F-01a, F-01b, F-01d, F-02 and the motion
foundation. In flight: B-13, B-07, B-08, F-06/F-07, F-01c. Spikes S-1 to S-4
and S-7 are answered.

## 1. How serial dependencies are cut

The old order (B-10 → B-11 → B-12 → B-14 → …) is serial mostly because a
later task needed code of an earlier one to exist. Five seams remove that.

1. **Wire shapes first.** Lane PROTO freezes every remaining step 1 shape
   before the code that uses it (units P-1, P-2), instead of "the first task
   that needs it". This changes the table of `contracts/conventions.md`
   section 5; unit C-0 edits that table. One lane owns `internal/proto/`,
   `web/src/lib/proto.ts`, `proto.test.ts` and the file map in `mock.ts`.
2. **Registries instead of shared switch statements.** `home` commands, MCP
   tools, `ctl` request handlers of the node and godog step sets are each a
   map declared once by a seam unit; a unit adds its entry from an `init`
   in its own file. No unit edits another unit's file to be reachable.
3. **Interfaces of `architecture.md` 8.3 first.** HM-0 and HS-0 declare the
   interfaces through which `home` and `host` see connections, with the
   method sets of the `peer` types of B-08. Everything in `home` and `host`
   is built and tested against them with in-memory fakes.
4. **The local link is the first transport.** N-1 builds the in-process
   connection between a host and a home on the same node, which the product
   needs anyway (`architecture.md` 2.1). Integration of M2 to M6 runs on it
   and does not wait for `peer`, the relay or pairing.
5. **Pure cores, late wiring.** The task machine (HM-M), every command
   function, the session table, workspaces, checks, the forge steps and the
   secret check are pure or touch only Git and the file system. They start
   at once. Wiring units (N-4) and integration units (I-*) are explicit and
   own no logic.

## 2. Rules for a unit

- One unit, one pull request, one session, the files it owns and no others.
  A unit that finds it must touch a file it does not own stops and says so.
- A unit's tests use fakes for what is not merged yet. The fake lives in the
  unit's own `_test.go` file, never in shared code.
- Acceptance scenarios: a unit adds `features/<module>_<slice>.feature` and
  `features/<module>_<slice>_test.go`; web units add
  `features/web_<slice>.feature` (tagged `@web`) and
  `web/features/<slice>.spec.ts`. `features/<module>.feature` stays with the
  module's seam unit. **Needs the coordinator's word**: the rule says
  `features/<module>.feature`; this plan reads it as "at least that file".
- Shared files with one owner for the whole plan: `go.mod`, `go.sum` (X-1,
  then only the unit named there), `features/acceptance_test.go` (X-1),
  `internal/proto/**` and the three web proto files (lane PROTO),
  `web/src/messages/*`, `web/src/App.svelte`, `web/src/lib/route.svelte.ts`,
  `web/package.json`, `web/pnpm-lock.yaml` (U-0, then lane UI-SHELL),
  `.github/workflows/*` (N-7, then lane CI), `issues.md` (the coordinator
  collects entries from pull request descriptions).
- Model: Fable 5.2 (F) for interface work, contracts and the hardest core
  logic; GPT-6.2 Astra (A) for routine backend and test infrastructure. The
  reviewer is always the other model. Fable ends on Nov 23: every F unit of
  step 1 is scheduled in waves 1 to 4.
- Size: S under 200 changed lines, M 200 to 400, L above 400 (allowed only
  where splitting would put two owners on one file).

## 3. Lanes

Two units of one lane never run at the same time unless their owned files
differ, which the unit tables state. Lanes never share files.

| Lane | Owns | Units |
| --- | --- | --- |
| PROTO | `internal/proto/**`, `web/src/lib/proto.ts`, `proto.test.ts`, `mock.ts` | P-1 → P-2 → P-4 → P-5, strictly serial; P-3 beside P-4 on files of its own |
| HOME | `internal/home/<file>.go`, one file per unit; `features/home*` | HM-* |
| HOST | `internal/host/<file>.go`, one file per unit; `internal/host/instructions/`; `features/host*` | HS-* |
| NODE | `internal/node/<file>.go`, `cmd/solodawn/`, `features/node*`, `features/cmd*` | N-* |
| PEER | `internal/peer/`, `internal/rendezvous/`, `features/peer*`, `features/rendezvous*` | B-07, B-08 (in flight), PR-*, RV-1 |
| WEB-LIB | `web/src/lib/<file>.ts`, one file per unit | W-* |
| UI-SHELL | `web/src/App.svelte`, routes table, `web/src/messages/`, `web/src/styles/` | U-0, then U-15, U-16 |
| UI | `web/src/routes/<Page>.svelte`, `web/src/components/<area>/`, `web/src/messages/<area>.*.json` | U-1 to U-14 |
| TEST | `features/m<n>*`, `features/netsim/`, `web/e2e/` | X-1, NET-1, I-*, E-* |
| CI | `.github/workflows/` | N-7, E-0, I-11 |
| PLAN | `planning/` | C-*, C2-*, C3-* |
| SPIKE | no merged files | S-5, S-6, S-8, S-10, T3-0 |

## 4. Waves

A wave is 45 minutes: build, review, merge. A unit starts when everything it
depends on is merged. In flight at the start: B-13, B-07, B-08, F-06/F-07,
F-01c; they are expected to merge during wave 1.

| Wave | Clock | Units | Count |
| --- | --- | --- | --- |
| 1 | 0:00 | X-1 (merges first, about 15 minutes), P-1, HM-0, HM-M, HS-0, HS-4, HS-5, HS-6, HS-7a, HS-7b, N-0, N-5, N-6, N-7, NET-1, W-1, W-2, W-4, S-5, S-6, S-10, T3-0, C-0, C2-0 to C2-7, C3-1, C3-2; U-0 as soon as F-06/F-07 merge | 34 |
| 2 | 0:45 | P-2, HM-1 to HM-17, HS-1, HS-2, HS-8 to HS-15, HS-16a, HS-19, HS-21, N-1, N-2, PR-1, PR-3, RV-1, W-3, E-0, U-1, U-3, U-4, U-7 to U-10, U-14, U-15, S-8 | 48 |
| 3 | 1:30 | P-3, P-4, HS-3, HS-16b, HS-17, HS-18, HS-20, N-3, N-4, N-8, PR-2, I-1, I-3, U-2, U-5, U-6, U-11, U-12, U-13 | 19 |
| 4 | 2:15 | I-2, I-4, I-5, I-6, I-7, I-8, U-16; step 2: T2-1 to T2-13, U2-1 to U2-4; step 3: P-5, T3-1 | 26 |
| 5 | 3:00 | I-9 (E-M1 to E-M8, eight units), I-10; step 2: I2-1; step 3: T3-2 to T3-6, U3-1, U3-2 | 17 |
| 6 | 3:45 | I-11 on three systems, I2-2, I3-1, fixes found by waves 4 and 5 | 5 + fixes |
| 7 | 4:30 | second pass of I-11 after fixes; the owner's live list (section 6) | 3 + owner |

P-3 and P-4 run together in wave 3 although both are in lane PROTO: P-3
owns only the transcript files and their two test files, which P-2 leaves
out of the generic file map.

Maximum useful parallelism is wave 2: about 48 sessions. Above that,
sessions would share files. Wave 1 needs 34, waves 3 and 4 about 20 to 26.

## 5. Critical path

Each arrow is one wave of 45 minutes.

- **Group logic**: HM-0 → HM-8 (brief and confirm) → I-1/I-3 (home and host
  on the local link) → I-5 (M5 from brief to merge) → I-9 (browser checks)
  → I-11 (three systems) → second pass. Seven waves: **5 h 15 min**.
- **Transport**: B-08 (in flight) → PR-1 (probe and move) → N-4 (wiring) →
  I-2 (M1 on real transport) and I-8 (browser against a real node) → I-9 →
  I-11 → second pass. Seven waves, the same length; a slip of B-08 moves the
  whole chain.
- **Steps 2 and 3**: C2-* (wave 1) → P-4 (wave 3; lane PROTO is serial, so
  it waits for P-2) → T2-* (wave 4) → I2-1 (wave 5) → I2-2 (wave 6). It
  ends one wave before the step 1 path.

With one fix wave per integration wave that fails, the estimate is **7 to 8
hours**, which leaves two hours of the ten. The path cannot be shortened by
more sessions: waves 1 to 3 are already limited by dependencies, not by
people. It is shortened only by merging faster than 45 minutes per unit.

What makes it longer, in order of likelihood: mutation testing on large
packages (run gremlins on the changed files only); P-1 running long (it is
L); interfaces of HM-0 and HS-0 changing after dependants started (their
pull requests merge first in wave 1, reviewed before anything else).

## 6. What needs the owner's hardware or accounts

Nothing below blocks a unit: each has a stand-in, and the real run is a
checklist item for the owner at the end.

| Need | Used by | Stand-in so work continues | The owner's run |
| --- | --- | --- | --- |
| Real Claude Code and Codex sign-ins | the `live` tests A3.8, A4.1 (second half), A4.10, A5.13 (the `git push` card), A7.1, A7.2, A7.8; Q-4 in S-5 | fake CLIs of B-13 with the captures of pull request 57; S-5 uses any console program for ConPTY and reads the vendor documentation for level 2 on Windows | one session per CLI, about 30 minutes, with the list printed by I-11 |
| Real routers and NATs | S-8 (UPnP, NAT-PMP, hairpin, hole punching), S-9, A1.3 and A1.4 outside one LAN | NET-1: Linux network namespaces with nftables NAT of each type, a blocked-UDP rule, and miniupnpd as the gateway | S-8 on the home router; S-9 only if step 3.4 is wanted |
| A public server, a domain, DNS | S-6 and A8.4 (certificates for two names, `turns:` on 443), A1.7 over the internet | RV-1 with a local ACME test server (pebble) and names in the hosts file; the quota test runs on loopback | start the rendezvous on the real server once; A8.4 |
| A GitHub account with a scratch repository | M5 "with a remote", B-21 | a bare repository on disk as the remote and a fake `gh` first on PATH that records calls and answers from files | the M5 run against GitHub, once |
| iOS device, Android device | Safari on iOS, Chrome on Android in A8.5; A8.3 on iOS after adding to the home screen | Playwright WebKit on a macOS machine and Chrome's device emulation; an Android emulator on a Linux machine for Chrome on Android | iOS: pairing, chat, an approval, a push reminder |
| A clean machine of each system, with a reboot | A8.1 | Devin Linux, macOS and Windows machines: install, pair, restart the service; a reboot is replaced by stopping and starting the login session where the machine allows | one real reboot per system |
| Web Push through the vendors' services | A8.3 | a local push service endpoint in the test; the pushed content is inspected there | desktop Chrome and Android Chrome once |
| Windows | S-5, N-5, HS-19 (ConPTY), I-11 | a Devin Windows machine covers all four | none |
| Accounts for step 3 CLIs | T3-2 to T3-4 | terms and capabilities are desk research; the driver is built against captures made with an API key if the owner provides one, otherwise against the documented protocol and marked unverified | one key per CLI |

## 7. Open points for the coordinator

1. The reading of the feature-file rule in section 2.
2. C-0 changes who freezes which wire shape (PROTO lane instead of the first
   task that needs it).
3. Step 2 contracts are written as new files under `contracts/step-2/`, so
   that no step 1 contract file has two writers while step 1 is being
   built; they are folded into the main files when step 1 is done.
4. The design answers of `steps/parallel-plan-step-2-3.md` section 1 are
   proposals. D-1, D-4 and D-11 are architecture decisions and need a yes
   before their contract units merge.

## 8. Not verified

- No unit size was measured; sizes are estimates from the contracts.
- The method sets of the `peer` types are those of the B-08 branch at the
  time HM-0 and HS-0 start; B-08 was not read for this plan.
- That miniupnpd, pebble and an Android emulator run on Devin machines.
- That `modernc.org/sqlite` is built with FTS5 (used by T3-6).
- Whether F-06/F-07 already split the message files; U-0 adapts to what
  they merged.
