# Step 1: one person, several machines

Task lists: `steps/step-1-backend.md`, `steps/step-1-frontend.md`.
Acceptance tests: `steps/step-1-acceptance.md`.

## Scope

In: every rule tagged `[S1]` in `product-rules.md`. One person; any number of
their computers and browsers; Claude Code and Codex; project groups with a
secretary; tasks from brief to merge with GitHub or with no remote;
approvals; AI calling AI; terminal takeover; direct, relayed and
server-less connectivity; the seven pages of the design; Chinese and
English; dark and light.

Out (later steps): more than one person in a group, invitations, confirming
other people's requests, watching a terminal, the team-wide group, forges
other than GitHub, CLIs other than Claude Code and Codex, phone app.

Step 1 is done when every acceptance test of `steps/step-1-acceptance.md`
passes on these pairs, at the browser baseline that spike S-7 fixed
(`risks.md` Q-3): the node on macOS, Windows and Linux; the web client in
Safari, Chrome and Firefox on macOS, Chrome and Firefox on Windows and
Linux, Safari on iOS, Chrome on Android.

## M0. Spikes

Throwaway code. Each spike is a pull request whose description answers its
questions with observed facts. Spike code is never merged into `rebuild`;
the planning lead records the answers in the plan. A "no" answer changes
the plan before code is built on it.

| Spike | Questions |
| --- | --- |
| S-1 transport | Does a pion node connect with each supported browser through signalling only? Are 60 KiB messages accepted both ways? Does a browser reach the node's ICE-TCP port? Are `.local` candidates a problem? Do both sides read the selected pair's type? Does a relay-free second connection, opened while the first is relayed, work? Does pion's opt-in renomination move a live relayed connection to a direct pair by itself, with each browser? How large is a signed description with all candidates, and how long does gathering take: can complete descriptions replace trickling? Do two nodes that are each a client of the other keep two working connections? **Answered** (pull request 56, and the A.1 design review made with its harness): `contracts/rendezvous.md` section 5, `contracts/rendezvous-evidence.md`, `flows/connect.md` |
| S-2 Claude Code driver | Do the flags of `contracts/drivers.md` 1.1 behave as marked: `--setting-sources user` keeps project hooks out; `--append-system-prompt` works with stream-json; what are the effective file and network limits at levels 1 and 2? How long can an `approve` call stay open, with and without progress notifications? What happens to a tool call that runs 10 minutes? Is the input of a retried request byte-identical? Which usage and rate-limit figures appear in events? Can a session be resumed from another working folder? Does the MCP session token still identify the caller after a resume? **Answered** (pull request 57): `contracts/drivers.md` 1.1 |
| S-3 Codex driver | Does `-c mcp_servers.…` work on `app-server`? Exact thread parameters for folder, sandbox, approval policy, instructions. Which approval request types exist? How long can one stay open? Is a retried request's input identical? Tool timeout setting. Usage and rate-limit notifications. Memory of one process per session. **Answered** (pull request 57): `contracts/drivers.md` 1.2 |
| S-4 takeover | For each CLI, with its version recorded: structured, then interactive in a pseudo-terminal in a trusted folder, then structured again, on one session id, history intact. Outcomes, not only flags: at level 1 (the secretary, a reviewer, a level-1 task) reading and the permitted MCP tools work, no project file changes, no execute tool exists, and check, push and submit are neither offered nor accepted; at level 2 an edit in the worktree succeeds and going beyond it asks the owner in the terminal; level 3 survives the resume; project configuration stays off by default; the MCP token still names the right session. Can a brand-new session be started directly in interactive mode with an id the node chose? **Answered** (pull request 57): `contracts/drivers.md` 1.3, `flows/takeover.md` F. One expected outcome did not hold: at level 1 Codex keeps a shell that reads and cannot write |
| S-5 Windows | ConPTY with both CLIs. Unix socket. Interrupt without signals. What level 2 can mean for Claude Code without its sandbox. |
| S-6 one port, relay | HTTPS and TURN over TLS on 443 chosen by server name, with real `turns:` clients; automatic certificates for two names. Credentials with the device id in the username; renewal. Cutting off an allocation that is already open when the byte quota is used. |
| S-7 browser keys | The lowest version of each supported browser in which Ed25519 in WebCrypto works with a non-extractable key kept in IndexedDB. Those versions, or the design's baseline where it is higher, become the supported baseline. **Answered**: `risks.md` Q-3 |
| S-8 port mapping | UPnP and NAT-PMP mapping on a real router; whether the mapped port is reached from outside; publishing the external port the router actually granted, also when it is another port than the one asked for, as a signalling address and as an ICE candidate in pion. Between nodes behind real NATs, the offering side on ephemeral sockets of its own: hole punching across different NATs; two peers behind one NAT, by LAN host candidates and, where those are prevented, by hairpinning. Whether a mapped port for ordinary offers, with private probe sockets, would gain material reachability (`contracts/rendezvous.md` 5.1, points 6 to 8). |
| S-9 TCP simultaneous open | Between two nodes behind different NATs with UDP blocked and no mapped port: does it succeed, on which router types, and how much code outside pion does it take? This decides whether step 3 builds it. Nothing in step 1 depends on the answer. |

## M1 to M8. Milestones and acceptance tests

The eight milestones after M0, the acceptance tests of each (A1.1 to
A8.6) and how acceptance is tested are in `steps/step-1-acceptance.md`.

## Order and parallel work

```
M0 ─ M1 ─ M2 ─ M3 ─ (first slice of M5) ─ M4 ─ M5 ─ M6
                                           └──── M7
M8 after M5; Windows and Linux checks run from M1 on, not only at the end.
```

Backend and frontend work in parallel from the first day: the frontend
builds against the golden examples of `internal/proto/testdata` and a mock
transport until the matching backend milestone lands. The golden examples
are frozen slice by slice, not all at once (`steps/step-1-backend.md`).
