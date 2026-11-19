# Risks, limits and open questions

Four lists. **Q, settled**: points where the plan had to interpret or narrow
a fixed decision, with the coordinator's decision. **Q, open**: new points
that came up while applying those decisions. **K**: limits the design
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
| Q-4 | Level 2 on Claude Code for native Windows, where the CLI has no sandbox. | Wait for spike S-5, then decide. | `contracts/local.md` 4.1; `flows/approval.md` H; `steps/step-1.md` S-5 |
| Q-5 | Reminders while the page is closed pass through the browser vendor's push service. | Accepted: ciphertext only, and as little content as possible (the group id and "something is new"). | `product-rules.md` R-163; `contracts/commands.md` 1.7; `steps/step-1.md` A8.3 |
| Q-6 | R-42 says the secretary posts the approval card. | The platform writes the card and shows it under the secretary's name. No AI turn. | `product-rules.md` R-42; `flows/approval.md` step 6; `contracts/rows.md` 5 |
| Q-7 | R-24 says the secretary merges. | When checks and review pass, the platform merges deterministically and shows it as the secretary's merge. On a conflict the secretary assigns an AI to resolve it. | `product-rules.md` R-24; `contracts/commands.md` 1.5, 1.8; `contracts/mcp-tools.md` 2; `flows/task.md` E; `steps/step-1.md` A5.10, A5.12 |
| Q-8 | Session granularity. | The secretary has one long session per group. A working AI has one session per task, in the task's worktree, ending with the task. Project memory is the ledger and the notes. | `product-rules.md` R-12; `architecture.md` 6; `flows/agent-turn.md`; `flows/task.md` B, D, I; `flows/takeover.md` A; `contracts/local.md` 2, 4; `contracts/storage.md` 3 |
| Q-9 | R-92: other forges. | Step 1: GitHub and "no remote". Others in step 2. | `product-rules.md` R-92; `steps/step-2-3.md` |
| Q-10 | R-41: every push asks at level 2. | Pushing the task's own branch is inside the workspace and needs no approval. Another branch, or a forced push, needs approval. | `product-rules.md` R-41; `contracts/mcp-tools.md` `push_branch`, `submit_task`; `flows/task.md` steps 8, 10; `steps/step-1.md` A5.13 |
| Q-11 | R-41: bulk deletion and use of secrets cannot be detected without inspecting commands. | Commands are never inspected. Bulk deletion relies on Git and on the CLI's sandbox. One small outbound check refuses what an AI sends to the group when it matches common secret formats or looks like an environment file. | `product-rules.md` R-41, R-137; `architecture.md` C-10; `contracts/local.md` 8; `flows/approval.md` step 4; `steps/step-1.md` A3.9 |
| Q-12 | The home holds the only copy of a group. | No move and no copy in step 1; both in step 2. | `product-rules.md` R-135; `steps/step-2-3.md` |
| Q-13 | Notes are long-term memory but retention is three months. | The ledger and the notes are exempt from retention and kept until a person deletes them. | `product-rules.md` R-102, R-135; `contracts/storage.md` 1, 4; `contracts/commands.md` 1.6; `flows/sync.md` G; `steps/step-1.md` A5.14 |
| Q-14 | Notifications. | Follows Q-5. | `product-rules.md` R-163 |

## Q. Open: raised while applying the decisions

| Id | Point | What the plan currently does | Why it needs a look |
| --- | --- | --- | --- |
| Q-15 | Q-8 gives a working AI one session per task. It does not say where a turn goes that belongs to no task: a direct question from the owner (R-70), a main-line mention, a call from a thread where the AI has no role. | Each working AI also has one long **group session** per group, read only, in the project clone. It cannot change code; `start_task` moves the work into a new task session. | Without it, either every question becomes a task (against R-70) or every message starts cold (against R-120). The cost is one more session per AI per group, dormant most of the time. |
| Q-16 | Q-10 makes pushing the task's branch free of approval without inspecting commands. | The node does the push itself (`push_branch`, and inside `submit_task`). `submit_task` also opens the pull request. AIs are told not to push or open pull requests by themselves; if one runs `git push` itself at level 2, the CLI asks and a card appears, even for its own branch. | It moves "open the pull request" from the AI to the platform. It is the only way found to honour Q-10 and Q-11 together. |
| Q-17 | The design has no screen for the project memory, and Q-13 says a person can delete entries. Q-5 makes a reminder unable to say what is waiting. | A plain list of notes and ledger entries per group with a delete action, placed in settings (F-13). A reminder names the group only; the "waiting for you" bar shows the item after opening. | Both need a design decision before the frontend builds them. |

## K. Known limits

| Id | Limit | Consequence |
| --- | --- | --- |
| K-1 | A group works only while its home is on and reachable, and the home holds the only copy (Q-12). | Stated to the user at creation and on the computers page (R-134). |
| K-2 | A browser cannot open a connection without some signalling path: a rendezvous service, or a node on the same machine. Two devices behind NAT on different networks cannot find each other while no rendezvous is reachable. | "Works without the server" means: open connections continue, nodes on one network or with a reachable remembered address reconnect, and a local browser works through its local node. |
| K-3 | An inbox item is written to a session at most once. A node that stops between acknowledging and writing loses that turn. | The secretary's follow-up covers it. No redelivery machinery. |
| K-4 | Device log: a stolen device and its owner can race to revoke each other, and receivers may disagree. | Recovery is manual. Not designed in step 1. |
| K-5 | Signalling envelopes older than two minutes are dropped. | A device with a badly wrong clock cannot connect; the error says so. |
| K-6 | The node cannot stop the owner from resuming a session id in their own terminal outside SoloDawn. Two writers lose messages `[verified]`. | Stated in the documentation and on the terminal page. |
| K-7 | Usage figures are whatever each CLI reports. | The card shows what exists. |
| K-8 | Check commands run on the assignee's machine with the owner's rights, outside any sandbox. | Acceptable while the group admin is the machine owner (step 1). Step 2 adds per-owner acceptance. |
| K-9 | Home and clients do not verify each other's software version beyond the protocol number. | A protocol change bumps `v`; old clients are told to update. |
| K-10 | The secretary's session lives in the project clone. If the owner makes it change files during a takeover, those changes are not on a task branch. | The hand-back summary lists them; the terminal page says the secretary's terminal is not for code changes. |
| K-11 | When a task is given to another AI, uncommitted changes of the previous one are discarded with its worktree. | Committed (and pushed) work carries over. |
| K-12 | The outbound secret check is a short pattern list. It catches accidents, not intent. | Stated as such; no configuration. |
| K-13 | A new task session knows nothing of earlier tasks except the notes it is given and what it reads in the group. | The secretary is instructed to write lessons as notes (R-102). |

## T. Technical risks

| Id | Risk | Settled by |
| --- | --- | --- |
| T-1 | Claude Code's control messages on stdin (interrupt) and the event shapes are not a published contract; Codex app-server is marked experimental. Either can change in any release. | The node uses a handful of messages; fake CLIs pin the expected shapes; a CI job diffs the Codex schema; `live` tests before each release; the CLI version is shown next to each AI. |
| T-2 | Claude Code may make its bare mode the default for `-p`, which does not use subscription sign-in `[documented]`. | The node reports `login_invalid` and does nothing else (R-56). The owner chooses API key or waits. |
| T-3 | How long a permission prompt call or a long tool call may stay open is unknown for both CLIs. | Spikes S-2, S-3. The flows already work with a short wait. |
| T-4 | Windows is unverified everywhere: ConPTY, sockets, interrupt, sandbox. | Spike S-5; Windows runs in CI from M1. |
| T-5 | Browser and pion interoperability: ICE-TCP from Safari, `.local` candidates, message size. | Spike S-1. |
| T-6 | About 240 MB of memory per open CLI session `[verified for Claude Code]`. With a session per task there are more sessions, though most are dormant. | `max_open` and dormancy; one turn at a time per AI per group. Measured in A8.6. |
| T-7 | Anyone can obtain relay credentials by presenting any key; the quota is per device id. | A second quota per source address on the rendezvous. Small, and at a boundary. |
| T-8 | Subscriptions are sold for ordinary individual use; heavy automated use may be rate-limited by the vendor. | Limits R-60 to R-62; `limited` state; nothing hidden from the owner. |
| T-9 | Unsigned binaries trigger warnings when downloaded by hand. | The AI installs from the command line; documentation covers the manual case (R-151). |
| T-10 | Plan size against "least code". A desk estimate before this plan was 7,000 to 12,000 lines for node, rendezvous and web client together. | Every pull request is reviewed for code that can be deleted. The backend list names what not to build. |
| T-11 | Port 443 shared by HTTPS and TURN. | Spike S-6; fallback is a second address or port for TURN. |
| T-12 | A secretary woken only by events may not notice a silent assignee. | Observed in A5.1 `live`; the fix is one timer in the home (`flows/task.md` H). |
| T-13 | Starting a brand-new session directly in interactive mode with a chosen id (takeover of an AI without a task) is untested on both CLIs. | Spike S-4. |

## Assumptions that would change the design if false

1. A group has at most a few dozen members and a few hundred open tasks, so
   a home keeps all state rows in memory.
2. One node hosts at most a handful of open sessions at a time.
3. Messages are text of a few kilobytes; nothing binary travels in groups.
4. The owner's Git and `gh` are already signed in on every machine that
   runs an AI.
