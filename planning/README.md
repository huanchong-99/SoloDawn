# SoloDawn rebuild: planning

Status: **plan converged after three review rounds; implementation
started.**

Three review rounds are answered in `reviews/`; the last ended with "ready
to start". The coordinator has settled Q-1 to Q-28 (`risks.md`). Next:

1. Work proceeds by `steps/step-1.md`: the backend by
   `steps/step-1-backend.md`, starting with B-00; the frontend by
   `steps/step-1-frontend.md`, starting with F-00.
2. Five points from the last review are fixed inside the tasks that
   implement them (`steps/step-1-backend.md`, "Fix in the task (round 3)").
3. The files in `contracts/` are the statement of record. A pull request
   that needs a contract changed says so in its description; the planning
   lead changes the contract. Code does not silently depart from it.
4. The answers of the spikes (M0) are recorded here by the planning lead.
   S-1, S-2, S-3, S-4 and S-7 are answered and recorded
   (`steps/step-1.md`, M0). Q-4 waits for spike S-5.
5. Questions that implementation raises about a contract are settled in
   the contract itself. The first batches, from the reviews of backend
   tasks B-01 and B-02, are in `contracts/conventions.md`,
   `contracts/messages.md` and `contracts/envelope.md`.

SoloDawn is being rebuilt from an empty tree. This directory holds the plan
that the implementation will follow. It is written for two readers: the
backend implementer, who starts from `contracts/` and
`steps/step-1-backend.md`, and the frontend implementer, who starts from
`contracts/datachannel.md`, `steps/step-1-frontend.md` and the
design mock-ups (kept outside the repository; every field the mock-ups need is
listed in the contracts).

## One-paragraph summary

A small program, the **node**, runs on each of the owner's computers. It keeps
official AI coding CLIs alive as long-lived sessions, lets them talk to each
other and to people in **groups** (one group per project), and enforces what
each AI may do. People use a **web client**. Nodes and browsers connect
peer-to-peer over WebRTC data channels. A tiny **rendezvous** service helps
peers find each other and relays ciphertext only when no direct path exists.
All data lives on the users' own machines.

## Who writes what

| Role | Model | Writes | Never writes |
| --- | --- | --- | --- |
| Planning lead | Fable 5.2 (local agent) | every file in `planning/` except `reviews/gpt-6.2-*`; replies in `reviews/fable-round-N.md` | reviewer files |
| Reviewer | GPT-6.2 Astra (cloud agent) | `reviews/gpt-6.2-round-N.md`, PR comments | any other planning file |
| Coordinator | main session | nothing here; merges or closes PRs | planning content |
| Owner | human | final acceptance | — |

One writer per file. The reviewer proposes changes in its review file; the
lead applies them (or explains why not) and records the outcome.

## How a round works

1. The lead pushes plan files to the `rebuild` branch and opens the round with
   `questions-roundN.md`.
2. The reviewer creates a branch named `devin/planning-review-N` from
   `rebuild`, adds `planning/reviews/gpt-6.2-round-N.md`, and opens a pull
   request against `rebuild`. Short remarks may go in PR comments.
3. The lead reads the PR, answers every finding in
   `planning/reviews/fable-round-N.md`, updates the plan files, and pushes to
   `rebuild`.
4. The coordinator decides whether to merge the PR. Nobody else merges.
5. Two or three rounds are expected. The plan is final when a round produces
   no blocker and no major finding.

Never force-push. Never touch `main`. Never push tags.

## Review file format

Each finding in `reviews/gpt-6.2-round-N.md`:

```
### F-<n> <short title>
Severity: blocker | major | minor
Where: <file>#<section>
Problem: <what breaks, and under which conditions>
Proposal: <the smallest change that fixes it>
Confidence: verified (<how>) | reasoned | guess
```

End the file with a section `## Unverified`, listing every statement in the
review that was not checked against documentation, source, or a test.

Each answer in `reviews/fable-round-N.md`:

```
### F-<n>
Decision: accepted | accepted with changes | rejected | needs coordinator
Change: <files and sections edited, or the reason for rejecting>
```

## Rules for everyone writing here

1. **Fixed decisions stay fixed.** `product-rules.md` and the "Fixed
   constraints" section of `architecture.md` restate what the owner has
   decided. If one of them looks wrong, do not design around it: file a
   finding marked `needs coordinator` and say why.
2. **Least code that does the job.** Prefer one mechanism used three times
   over three mechanisms. Every proposal is judged on how much code it adds.
3. **No defensive programming beyond system boundaries.** Validate at the
   boundaries only: bytes from the network, input from people, output from
   external processes. Inside the program, trust invariants; when one is
   broken, fail immediately with a clear error. Do not propose fallbacks,
   retries, nil checks, or layered error wrapping that the plan does not ask
   for. Reviews must not suggest them either.
4. **Say what is not verified.** Mark claims `[verified]`, `[documented]`, or
   `[unverified]`. An unverified claim that matters becomes a spike in
   `steps/step-1.md`.
5. **English, impersonal.** No names, places, accounts, or machine paths.
   People are "the owner", "a member", "the coordinator".
6. **Self-contained files.** Each file can be read alone. Cross-references
   use relative paths.

## File index

| File | Contents |
| --- | --- |
| `README.md` | this file |
| `product-rules.md` | product behaviour already decided, numbered `R-n` |
| `architecture.md` | fixed constraints, components, responsibilities, diagram, trust model, repository layout and import rules |
| `sessions.md` | kinds of CLI session, their folders, levels and states |
| `flows/pairing.md` | first run: browser and node pair, device list |
| `flows/connect.md` | finding a peer, racing paths, relay fallback, switching back |
| `flows/sync.md` | how a client catches up with a group home |
| `flows/agent-turn.md` | delivering a message to an AI and getting its reply |
| `flows/approval.md` | an AI asks for permission, the owner decides |
| `flows/task.md` | brief, confirm, work, checks, review, merge |
| `flows/trace-task.md` | one confirmed task on the wire: who signs what, which commit |
| `flows/ask.md` | one AI calls another, same machine or not |
| `flows/takeover.md` | the owner takes the terminal and hands it back |
| `contracts/envelope.md` | identities, signed envelope, device log |
| `contracts/vectors.md` | the shared test vectors that hold the Go node and the web client to `contracts/envelope.md` |
| `contracts/rendezvous.md` | WebSocket signalling protocol, STUN and relay |
| `contracts/rendezvous-evidence.md` | what spike S-1 and the A.1 design review measured for the ICE rules, the sources read, and what is not verified |
| `contracts/datachannel.md` | data-channel protocol: channels, framing, session, reading, inbox, terminal |
| `contracts/conventions.md` | rules for everything on the wire: which fields are always present, requests and answers, sizes, and which task freezes which shape |
| `contracts/rows.md` | the rows a home pushes: group, member, device, task, message, note |
| `contracts/messages.md` | message kinds, the body of each, and the states of cards |
| `contracts/commands.md` | every signed command, who may send it, what it does |
| `contracts/mcp-tools.md` | tools the node offers to AI sessions |
| `contracts/storage.md` | SQLite schema draft |
| `contracts/task-machine.md` | the one table of task transitions |
| `contracts/drivers.md` | how each CLI is started, levels, turn format, instructions |
| `contracts/local.md` | local interfaces: command line, MCP shim socket, Git commands, outbound secret check, self-update |
| `steps/step-1.md` | step 1 scope, spikes, order of the milestones |
| `steps/step-1-acceptance.md` | step 1 milestones M1 to M8 and their acceptance tests |
| `steps/step-1-backend.md` | step 1 backend task list |
| `steps/step-1-frontend.md` | step 1 frontend task list |
| `steps/step-2-3.md` | outline of steps 2 and 3 |
| `risks.md` | risks, open questions, items needing coordinator confirmation |
| `questions-round1.md` | review questions for round 1 |
| `questions-round2.md` | review questions for round 2 |
| `questions-round3.md` | review questions for round 3, the final check |
| `reviews/fable-round-3.md` | the outcome of the three rounds, and where the last five points are fixed |
| `reviews/` | review rounds |

## Vocabulary

| Term | Meaning |
| --- | --- |
| node | the program installed on a computer |
| host role | the part of a node that runs AI sessions for its owner |
| home role | the part of a node that stores a group and applies its rules |
| group home | the one node that plays the home role for a given group |
| client | anything that connects to a home: a browser or a host-role node |
| member | a person or an AI in a group |
| owner (of an AI) | the person whose computer and account the AI runs on |
| secretary | the coordinating AI member of a group; one per group |
| main line | the top-level timeline of a group |
| thread | the messages attached to one task |
| rendezvous | the small public service for signalling, STUN and relay |
| device | one node or one browser profile, identified by its own key |
