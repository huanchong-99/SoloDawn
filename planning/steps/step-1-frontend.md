# Step 1: frontend task list

> Historical task breakdown. Steps 1 to 3 are built; `planning/contracts/` and `planning/flows/` are the current specification.

For the frontend implementers (local agents). Read first: `README.md`,
`architecture.md` sections 2 to 4, `contracts/conventions.md`,
`contracts/envelope.md`, `contracts/rendezvous.md`,
`contracts/datachannel.md`, `contracts/rows.md`, `contracts/messages.md`,
`contracts/commands.md`, the flows, and the design mock-ups with their
design notes. Milestones and acceptance tests are in
`steps/step-1-acceptance.md`.

## Working rules

- The web client contains **no group logic**. It renders rows and sends
  commands. If a screen seems to need a rule (who may approve, when a task
  is blocked), the answer is a field in a row; if the field is missing, the
  contract is incomplete: report it, do not compute it in the client.
- Types come from the contracts. `web/src/lib/proto.ts` mirrors
  `internal/proto`; a test parses every golden file in
  `internal/proto/testdata/` and fails on an unknown or missing field.
  Which fields are always there and which may be left out, and what the
  client does at run time with a field it does not know, is
  `contracts/conventions.md` section 2, with the optional fields listed in
  `contracts/rows.md` and `contracts/messages.md`. A shape that has no
  golden file yet (`contracts/conventions.md` section 5 says which task
  adds it) is typed from the example and field table in its contract.
- A golden file changes in both languages in one pull request, whoever
  opens it (`contracts/conventions.md` section 5).
- Validate only at boundaries: messages from a data channel or WebSocket,
  and what a person types. Components trust their props.
- Least code. Svelte 5 runes for state; no state library; no component
  library unless a component is hard to make accessible by hand (dialog and
  details are native elements in the design).
- Dependencies allowed without asking: `svelte`, `vite`, `@xterm/xterm` with
  its fit add-on, an i18n library that compiles messages, a PWA plugin, a
  test runner, Playwright. Anything else: ask the coordinator.
- Design tokens, layout breakpoints, component states and accessibility
  rules are the design's. This list does not repeat them.

## Structure (proposed)

```
web/src/lib/proto.ts        wire types
web/src/lib/identity.ts     device key, envelope, device log, pairing
web/src/lib/rendezvous.ts   signalling client
web/src/lib/peer.ts         connections, path, reconnect, probe-and-move, ctl requests
web/src/lib/groups.svelte.ts   per-group row cache and derived views
web/src/lib/nodes.svelte.ts    host.info per node
web/src/lib/mock.ts         mock transport fed by golden files
web/src/routes/…            onboarding, groups, chat, terminal, nodes, settings
web/src/components/…        cards, thread, member card, indicator, base parts
```

## Tasks

`→` means "depends on". Backend ids refer to `steps/step-1-backend.md`.

| Id | Task | → | Done when |
| --- | --- | --- | --- |
| F-00 | Scaffold: Vite, Svelte 5, TypeScript, with the build written to `internal/node/webdist/` (ignored by Git) and no Go file under `web/`; design tokens and base styles from the mock-ups; theme switch; Chinese and English message files; application shell (navigation, top bar, bottom bar on phones); hash routing for static hosting; web manifest; an upgrade notice shown instead of the application when the browser lacks Ed25519 in WebCrypto (C-9) | — | the shell renders at 375, 820, 1100, 1440 px in both themes and both languages |
| F-01a | Spike S-7: the lowest version of each supported browser (done; the result is `docs/decisions/implemented/architecture/2026-11-20-browser-baseline.md`) | — | S-7 answered for the three baseline browsers |
| F-01b | `identity.ts`, with WebCrypto only and no signature library: device key kept in IndexedDB; envelope sign and verify, and the receiver's checks; device-log replay; adoption; fingerprint; pairing secret, `mac`, room id and the check of a `pair` message. Merged (pull request 66) | F-01a, B-02b | `identity.ts` passes every case of `testdata/crypto.json` (`contracts/vectors.md`) under the unit-test runner |
| F-01c | The same vectors in the three baseline browsers. The unit-test runner's Ed25519 library answers differently from the browsers' on weak keys (`contracts/envelope.md` section 2), so a missing check shows only in a browser. No signature bytes are compared, because Safari's signatures are randomised (`contracts/vectors.md` section 2). WebKit runs on macOS: its Linux ports (GTK, WPE) have another Ed25519 implementation, so CI needs a macOS runner for this task. One more case may be run to observe, without asserting anything, what each browser answers for a key of mixed order (`risks.md` K-15) | F-01d | every case of `testdata/crypto.json` passes in Chromium, in Firefox, and in WebKit on macOS |
| F-01d | `identity.ts`, what was settled after F-01b: the pairing `mac` over `kind` and `name`, an id as a type, the size limit of a log entry, the vector parts added since. In the pull request of backend task B-02c | F-01b, B-02c | every case of `testdata/crypto.json` passes under the unit-test runner |
| F-02 | `proto.ts` and the strict golden-file test (presence by `contracts/conventions.md` section 2; an error answer is told by `err` and has no `t`); `mock.ts`, a transport that answers `hello`, `sync`, `fetch`, `host.info` from golden files, pushes `groups`, and accepts commands | B-01a, B-01b | every golden file parses; components can be developed with no node |
| F-03 | `rendezvous.ts` and `peer.ts`: handshake, signed offer and candidates on every signalling path, holding early candidates, `ctl` requests and pushes, path classification from the browser's own connection statistics, reconnect with back-off, the relay-free probe and the move as a reconnect (`hello` on the probe, close the old connection on `welcome`, `sync` from the last row applied), sending unanswered commands again; one active tab per browser: the lock, the notice with "Use here", and closing the connections when the lock is lost (`flows/connect.md` F) | F-01b, B-06, B-08, B-09 | A1.3, A1.11 and A1.13 from the browser side; A1.12 |
| F-04 | Onboarding page: prompt with the 16-character pairing secret in four groups (the mock-up's 8-character sample is out of date) and countdown, live pairing steps (the second `hello` once enrolled), fingerprint, account type per found CLI; after pairing, the request to keep storage (`risks.md` K-20) | F-03, B-10 | A1.1 |
| F-05 | Connection indicator in the top bar with its four states; "my computers and AIs" page, part 1: computers, path, version, add a computer, add a browser, revoke a device | F-03, B-10 | A1.2, A1.6 |
| F-06 | `groups.svelte.ts`: apply `rows` (a row only if its `seq` is higher than the cached one; a `gone` row kept as a hidden marker), `reset` before rows, `synced`, `groups`; the cursor moves only at `synced` and with live rows, and returns to 0 with the cache on every `reset` (round 3, R3-5: test a connection lost after the first batch of a reset); `fetch` pages by `pos`; messages ordered by `pos`; derived lists (main line, thread, members, "waiting for you"); group list page; create-group dialog | F-02, B-11, B-12 | A2.1, A2.2, A2.6 |
| F-07 | Chat main line: row layout, text messages, composer with mention completion and a length hint (the client estimates the row by `contracts/rows.md` section 5 and warns before the limit), date separators, scroll-back | F-06 | A2.1 |
| F-08 | AI member panel and cards: state shape and sentence, computer and path, account type, usage bar; add an AI to a group; registered AIs on the computers page | F-06, B-15 | A3.1, A3.4 as seen in the client |
| F-09 | Approval card in all states, labelled with the kind of request the CLI made (command, file, network, other) and the CLI's own one-line description; on a card that shows a part of a long request, what `risks.md` K-21 says and recommends; the mock-up's labels such as "install software" would need the command to be read and are replaced (**the frontend adjusts the design**, `risks.md` Q-18); "waiting for you" bar; permission three-way control per project and AI, whose explanation of level 2 says what `product-rules.md` R-41 promises and no more, and that the project's check commands run outside the sandbox (Q-19); emergency stop dialog and paused banner; audit list; in-page notifications | F-08, B-17, B-18 | A4.1, A4.2, A4.7, A4.8 |
| F-10 | Brief card; task card with the six-step bar, badges, checks and review lines; thread with every message kind; decision card; summary card; review block; `task_merged` and approval cards shown under the secretary's name; the conflict and no-reviewer waits; "jump the queue" prompt | F-09, B-01d, B-19 to B-21 | A5.1 as seen in the client; every state in the design's component sheet is reachable from golden files |
| F-11 | Call block in threads | F-10, B-22 | A6.1 as seen in the client |
| F-12 | Terminal page: xterm.js on the `pty` channel, resize, takeover bar that says messages to the AI wait until hand-back (no count, `flows/takeover.md` step 9), hand-back, key row on phones, disabled state unless the path is `direct`, and closing the channel when the path becomes relayed (`flows/connect.md` step 9); `takeover.start` carries the task when opened from a task card; a line telling the owner that opening the terminal of an AI without a task creates one | F-08, B-23 | A7.1, A7.5, A7.6, A7.7, A7.9 |
| F-13 | Settings page: permissions defaults, retention with confirmation, notifications, appearance, this browser; service worker and Web Push subscription; a reminder shows `group · card type · who: title` from the pushed summary, worded in the user's language, and opens the card (**no mock-up yet; the frontend supplies the design**); add-to-home-screen hint for iOS, with its reason (`risks.md` K-20) | F-09, B-25 | A8.3 |
| F-15 | Group info panel with a "Project memory" section: the group's notes and its ledger entries (`fetch` with `kind: review`), each with a delete action that sends `forget` after a confirmation. Memory belongs to the project, so it lives with the group and not in the global settings (**no mock-up yet; the frontend supplies the design**) | F-10, B-21 | A5.14 as seen in the client |
| F-14 | End-to-end suite in Playwright against real nodes with fake CLIs, reusing the browser configuration of F-01c; accessibility pass (keyboard, focus, contrast, reduced motion, screen-reader names) at the design's widths | all | the browser-side checks of every acceptance test run in CI on Chrome, Firefox and WebKit |

## What each screen reads

| Screen | Source |
| --- | --- |
| group list | `welcome.groups` of every connected home, replaced by each `groups` push; last main-line `msg` row and the count of pending cards per group |
| main line | `msg` rows with `thread = null` or `main = true`, ordered by `pos`; `task` rows for task cards |
| thread | `fetch` with the task id, plus live `msg` rows of that thread |
| "waiting for you" | pending `brief` where the viewer is the requester; pending `approval` with the viewer in `body.approvers`; pending `decision` with the viewer in `body.to` |
| AI member card | `member` row; "open terminal" enabled when the viewer owns the AI and the browser's own path to that node is `direct` |
| computers and AIs | the owner's device log; `host.info` of each connected node; the browser's own path to each node |
| settings | `host.info.settings` of each node; browser preferences |
| project memory, in the group info panel | `note` rows and `msg` rows of kind `review` of a group |
| connection indicator | the browser's path to the home of the open group |

## Texts

- Interface strings are in message files, both languages complete.
- `system` rows carry a `code` and fields; the client owns their wording.
- Error `code`s from responses map to client wording; `msg` is for the
  console only.
- Chat content written by people and AIs is shown as written.

## Not in step 1

Invitations and member management for other people, the confirmation card
for other people's requests, watching a terminal, team-wide group screens.
The components should not anticipate them.
