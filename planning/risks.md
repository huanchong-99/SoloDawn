# Risks, limits and open questions

Four lists. **Q, settled**: points where the plan had to interpret or narrow
a fixed decision, with the coordinator's decision. **Q, open**: points still
waiting for one. **K**: limits the design
accepts knowingly. **T**: technical risks, each tied to the spike or test
that settles it.

## Q. Settled by the coordinator (2026-11-20)

Every row below is **decided by coordinator**. "Applied in" names where the
decision now lives.

| Id | Question | Decision | Applied in |
| --- | --- | --- | --- |
| Q-1 | R-131 names TCP. Two nodes that are both behind NAT would need TCP simultaneous open, which pion does not do `[unverified]`. | Step 1 tries together: UDP hole punching, IPv6, TCP to a port the other node has open, forwarded or mapped (UPnP, NAT-PMP), with the relay on 443 as last resort. Simultaneous open is an experiment (S-9), built in step 3 only if it shows a clear benefit. | `product-rules.md` R-131; `contracts/rendezvous.md` 5; `flows/connect.md` step 5; `steps/step-1.md` S-9; `steps/step-2-3.md` 3.4 |
| Q-2 | The mock-up shows an 8-character pairing code. | 16 characters. No extra key-exchange algorithm for the sake of 8. The onboarding copy changes to 16. | `product-rules.md` R-152; `contracts/envelope.md` 6; `steps/step-1-frontend.md` F-04 |
| Q-3 | Ed25519 in WebCrypto is newer than the design's browser baseline `[unverified]`. | Raise the baseline to versions with native support; a spike finds the exact versions; no signature library is bundled. | `product-rules.md` R-164; `architecture.md` C-9; `contracts/envelope.md` 2; `steps/step-1.md` S-7; `steps/step-1-frontend.md` F-00, F-01 |
| Q-4 | Level 2 on Claude Code for native Windows, where the CLI has no sandbox. | Wait for spike S-5, then decide. | `contracts/drivers.md` 1.1; `flows/approval.md` H; `steps/step-1.md` S-5 |
| Q-5 | Reminders while the page is closed pass through the browser vendor's push service. | Accepted: ciphertext only, and little content. What the content may be was then fixed in Q-17. | `product-rules.md` R-163; `contracts/commands.md` 1.7; `steps/step-1.md` A8.3 |
| Q-6 | R-42 says the secretary posts the approval card. | The platform writes the card and shows it under the secretary's name. No AI turn. | `product-rules.md` R-42; `flows/approval.md` step 6; `contracts/messages.md` 1 |
| Q-7 | R-24 says the secretary merges. | When checks and review pass, the platform merges deterministically and shows it as the secretary's merge. On a conflict the secretary assigns an AI to resolve it. | `product-rules.md` R-24; `contracts/commands.md` 1.5, 1.8; `contracts/mcp-tools.md` 2; `flows/task.md` E; `steps/step-1.md` A5.10, A5.12 |
| Q-8 | Session granularity. | The secretary has one long session per group. A working AI has one session per task, in the task's worktree, ending with the task. Project memory is the ledger and the notes. | `product-rules.md` R-12; `sessions.md`; `flows/agent-turn.md`; `flows/task.md` B, D, I; `flows/takeover.md` A; `contracts/local.md` 2; `contracts/drivers.md` 1; `contracts/storage.md` 3 |
| Q-9 | R-92: other forges. | Step 1: GitHub and "no remote". Others in step 2. | `product-rules.md` R-92; `steps/step-2-3.md` |
| Q-10 | R-41: every push asks at level 2. | Pushing the task's own branch is inside the workspace and needs no approval. Another branch, or a forced push, needs approval. | `product-rules.md` R-41; `contracts/mcp-tools.md` `push_branch`, `submit_task`; `flows/task.md` steps 8, 10; `steps/step-1.md` A5.13 |
| Q-11 | R-41: bulk deletion and use of secrets cannot be detected without inspecting commands. | Commands are never inspected. Bulk deletion relies on Git and on the CLI's sandbox. One small outbound check refuses what an AI sends to the group when it matches common secret formats or looks like an environment file. | `product-rules.md` R-41, R-137; `architecture.md` C-10; `contracts/local.md` 8; `flows/approval.md` step 4; `steps/step-1.md` A3.9 |
| Q-12 | The home holds the only copy of a group. | No move and no copy in step 1; both in step 2. | `product-rules.md` R-135; `steps/step-2-3.md` |
| Q-13 | Notes are long-term memory but retention is three months. | The ledger and the notes are exempt from retention and kept until a person deletes them. | `product-rules.md` R-102, R-135; `contracts/storage.md` 1, 4; `contracts/commands.md` 1.6; `flows/sync.md` G; `steps/step-1.md` A5.14 |
| Q-14 | Notifications. | Follows Q-5. | `product-rules.md` R-163 |
| Q-15 | Q-8 gives a working AI one session per task. Where does a turn go that belongs to no task: a direct question (R-70), a main-line mention, a call from another task's thread? | Accepted as proposed: each working AI has one read-only **group session** per group, in the project clone, for exactly those turns. To change code it calls `start_task`, and the work moves to a new task session. | `product-rules.md` R-12; `sessions.md`; `flows/agent-turn.md` A; `flows/task.md` G; `flows/ask.md` step 4; `contracts/drivers.md` 1, 4 |
| Q-16 | Q-10 and Q-11 together: pushing the task's branch without approval, without inspecting commands. | Accepted as proposed: the node pushes (`push_branch`) and, in `submit_task`, pushes and opens the pull request. The node only runs the owner's installed and signed-in `git` and `gh`; it never reads a token or a credential, and the contract says so. | `product-rules.md` R-41, R-137; `architecture.md` C-11; `contracts/local.md` 7; `contracts/mcp-tools.md` 1; `flows/task.md` steps 8, 10 |
| Q-17 | No screen for project memory; what a closed-page reminder may say. | (1) Deleting memory by hand lives in the **group info panel, under "Project memory"**: memory belongs to the project, so it goes with the group, not with global settings. (2) A reminder **may carry a short card summary**: group name, card type, one-line title. The payload is encrypted end to end for the browser (RFC 8291), so the push vendor sees ciphertext only. No message text, no code. The frontend supplies the missing designs. | `product-rules.md` R-102, R-163; `contracts/commands.md` 1.7; `steps/step-1.md` A8.3; `steps/step-1-frontend.md` F-13, F-15; `steps/step-1-backend.md` B-25 |
| Q-18 | The mock-up labels an approval card with what the command does ("install software", "push code"). Telling those apart means reading the command, which C-10 forbids. | The card shows the kind of request the CLI made (`command`, `file`, `network`, `other`) and the one-line description the CLI attached. The reminder's title is that same description. No label that needs the command to be read. The reminder may also carry the opaque group and card ids and the member's display name. | `product-rules.md` R-41; `contracts/messages.md` 2.3; `contracts/commands.md` 1.7; `steps/step-1-frontend.md` F-09 |
| Q-19 | Round 1, F-11. The project's check commands are run by the node, as the owner, outside the CLI's sandbox, on code the AI wrote. | Step 1: keep it, with two additions. Check commands get a minimal environment (an allow list: `PATH`, `HOME`, locale and the like), so no key or token variable reaches them. The plan and the interface say plainly that level 2 limits the AI's own actions and that project checks run outside the sandbox. Step 2: checks run in an existing operating-system sandbox (sandbox-runtime, bubblewrap or sandbox-exec; Windows a stated limit), to be verified by an experiment first. | `product-rules.md` R-40; `contracts/local.md` 7; `contracts/drivers.md` 2; `flows/approval.md` G; `steps/step-1.md` A5.17; `steps/step-1-frontend.md` F-09; `steps/step-2-3.md` |
| Q-20 | Round 1, F-10. During a takeover the secretary could be made to edit files outside any task branch. | Accepted: the secretary's terminal keeps its read-only level. The secretary writes no code; to change code the owner takes over a working AI. | `product-rules.md` R-21; `flows/takeover.md` A, B; `contracts/drivers.md` 1.3; `steps/step-1.md` A7.8 |
| Q-21 | Round 2, F-1. What happens to the tasks of an AI that leaves a group. | Accepted: an AI cannot leave a group while it is assignee or reviewer of a task that is not final; the tasks are reassigned or cancelled first. | `contracts/commands.md` 1.2, 2 (`host.leave`); `contracts/task-machine.md` 4; `steps/step-1.md` A5.18 |
| Q-22 | Round 2, F-2. What may change a task between a passed review and the host's report of the merge. | Accepted: nothing but a person's cancel. No extra command for the host to claim the merge. K-14 stays as written. | `contracts/task-machine.md` 2, row 15, section 4; `flows/task.md` steps 17, 18; `risks.md` K-14; `steps/step-1.md` A5.18 |

Also confirmed by the coordinator, as defaults: a quiet task wakes the
secretary after 30 minutes; receipts are kept 24 hours; when a task changes
hands the previous node pushes the branch before removing its worktree.
And, as a reading of R-63: after a task has stopped (three failures, or a
sixth failed review) only a person may resume or reassign it; the secretary
may cancel it.

## Q. Open

None at present. Q-4 is settled as "wait for spike S-5" and comes back when
that spike has an answer.

## K. Known limits

| Id | Limit | Consequence |
| --- | --- | --- |
| K-1 | A group works only while its home is on and reachable, and the home holds the only copy (Q-12). | Stated to the user at creation and on the computers page (R-134). |
| K-2 | A browser cannot open a connection without some signalling path: a rendezvous service, or a node on the same machine. Two devices behind NAT on different networks cannot find each other while no rendezvous is reachable. | "Works without the server" means: open connections continue, nodes on one network or with a reachable remembered address reconnect, and a local browser works through its local node. |
| K-3 | An inbox item is written to a session at most once. A node that stops after recording the delivery and before writing the turn loses that turn. | The home's quiet-task notice brings the secretary to it (`flows/task.md` H). No redelivery machinery. |
| K-4 | Device log: two entries for one version make a fork, and receivers may disagree. It happens when a stolen device and its owner race to revoke each other, and when two devices of one person each add a device at the same moment. | Guidance: add one device at a time. Recovery is manual. Not designed in step 1. |
| K-5 | Signalling envelopes older than two minutes are dropped. | A device with a badly wrong clock cannot connect; the error says so. |
| K-6 | The node cannot stop the owner from resuming a session id in their own terminal outside SoloDawn. Two writers lose messages `[verified]`. | Stated in the documentation and on the terminal page. |
| K-7 | Usage figures are whatever each CLI reports. | The card shows what exists. |
| K-8 | In step 1, check commands run on the assignee's machine with the owner's rights, outside any sandbox, and what they execute is the task's code (Q-19). | They get a minimal environment; the interface says what level 2 does and does not limit. Step 2 moves checks into an operating-system sandbox and adds per-owner acceptance of the command list. |
| K-9 | Home and clients do not verify each other's software version beyond the protocol number. | A protocol change bumps `v`; old clients are told to update. |
| K-10 | (Removed in round 1: the secretary's terminal is read only, see Q-20.) | — |
| K-11 | When a task is given to another AI, uncommitted changes of the previous one are discarded with its worktree. | Committed (and pushed) work carries over. |
| K-12 | The outbound secret check is a short pattern list. It catches accidents, not intent. | Stated as such; no configuration. |
| K-13 | A new task session knows nothing of earlier tasks except the notes it is given and what it reads in the group. | The secretary is instructed to write lessons as notes (R-102). |
| K-14 | A merge is an action outside the system and cannot be taken back. A person's cancel that arrives while the host is merging leaves the change merged under a cancelled task. A node that stops in the middle of a merge reports, when it restarts, what it finds: merged, or a conflict. | Stated on the cancel confirmation for a task at step `merge`. No retry, no rollback (R-24). |

## T. Technical risks

| Id | Risk | Settled by |
| --- | --- | --- |
| T-1 | Claude Code's control messages on stdin (interrupt) and the event shapes are not a published contract; Codex app-server is marked experimental. Either can change in any release. | The node uses a handful of messages; fake CLIs pin the expected shapes; a CI job diffs the Codex schema; `live` tests before each release; the CLI version is shown next to each AI. |
| T-2 | Claude Code may make its bare mode the default for `-p`, which does not use subscription sign-in `[documented]`. | The node reports `login_invalid` and does nothing else (R-56). The owner chooses API key or waits. |
| T-3 | How long a permission prompt call or a long tool call may stay open is unknown for both CLIs; so is whether a retried request has byte-identical input, which the late-approval record relies on. | Spikes S-2, S-3. The flows already work with a short wait; a retry that is not recognised produces a new card. |
| T-4 | Windows is unverified everywhere: ConPTY, sockets, interrupt, sandbox. | Spike S-5; Windows runs in CI from M1. |
| T-5 | Browser and pion interoperability: ICE-TCP from Safari, `.local` candidates, message size. | Spike S-1. |
| T-6 | About 240 MB of memory per open CLI session `[verified for Claude Code]`. With a session per task there are more sessions, though most are dormant. | `max_open` and dormancy; one turn at a time per AI per group. Measured in A8.6. |
| T-7 | Anyone can obtain relay credentials by presenting any key; the quota is per device id. | A second quota per source address on the rendezvous. Small, and at a boundary. |
| T-8 | Subscriptions are sold for ordinary individual use; heavy automated use may be rate-limited by the vendor. | Limits R-60 to R-62; `limited` state; nothing hidden from the owner. |
| T-9 | Unsigned binaries trigger warnings when downloaded by hand. | The AI installs from the command line; documentation covers the manual case (R-151). |
| T-10 | Plan size against "least code". A desk estimate before this plan was 7,000 to 12,000 lines for node, rendezvous and web client together. | Every pull request is reviewed for code that can be deleted. The backend list names what not to build. |
| T-11 | Port 443 shared by HTTPS and TURN. | Spike S-6; fallback is a second address or port for TURN. |
| T-12 | (Settled in round 1: the home's quiet-task timer, `contracts/task-machine.md` row 23.) | — |
| T-13 | Starting a brand-new session directly in interactive mode with a chosen id (takeover of an AI without a task) is untested on both CLIs. | Spike S-4. |

## Assumptions that would change the design if false

1. A group has at most a few dozen members and a few hundred open tasks, so
   a home keeps all state rows in memory.
2. One node hosts at most a handful of open sessions at a time.
3. Messages are text of a few kilobytes; nothing binary travels in groups.
4. The owner's Git and `gh` are already signed in on every machine that
   runs an AI.
