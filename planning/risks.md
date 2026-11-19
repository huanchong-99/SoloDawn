# Risks, limits and open questions

Three lists. **Q**: questions where the plan had to interpret or narrow a
fixed decision; each needs the coordinator. **K**: limits the design accepts
knowingly. **T**: technical risks, each tied to the spike or test that
settles it.

## Q. Needs coordinator confirmation

| Id | Question | What the plan currently does | Why it matters |
| --- | --- | --- | --- |
| Q-1 | R-131 names TCP as one of the paths tried. Between two nodes that are both behind NAT, a direct TCP path needs simultaneous open, which pion does not implement `[unverified]`. | TCP is ICE-TCP to a listening or router-mapped port of a node, plus TURN over TLS. No TCP hole punching between two NATed nodes. | Building simultaneous open outside pion is a separate transport and a lot of code for networks where UDP is blocked on both sides and neither router maps ports. |
| Q-2 | The mock-up shows an 8-character pairing code. | 16 characters (80 bits), because the code authenticates the pairing against the rendezvous server. | 8 characters can be searched offline by whoever runs the server. The alternative that keeps 8 characters is a password-authenticated key exchange, in Go and in the browser. |
| Q-3 | The design's browser baseline is Chrome 123, Safari 17.5, Firefox 120. Ed25519 in WebCrypto arrived later in Chrome and Firefox `[unverified]`. | Assumes it is available; spike S-7 checks. | If not: raise the baseline, or sign in the browser with a small script library and an extractable key. |
| Q-4 | Level 2 relies on the CLI's sandbox. Claude Code has none on native Windows `[documented]`. | Unsettled; spike S-5. | Without a sandbox, level 2 there is either "ask for every command" or "trust the CLI's own allow rules". The node will not classify commands itself. |
| Q-5 | Reminders when the page is closed use Web Push, which passes through the browser vendor's push service (ciphertext only). The browser hands each home a key that lets the home send it reminders. | Planned in M8. | The vendor's service is a third party in the path. If that is not acceptable, reminders work only while a page is open. |
| Q-6 | R-42 says the secretary posts the approval card. | The card is written by the AI's host and shown under the secretary's name. No AI turn is spent. | A card that depends on a model turn can be late or missing. |
| Q-7 | R-24 says the secretary merges. | The secretary calls `merge_task`; the home refuses unless checks and review passed; the assignee's node runs the merge. | Alternative: the home merges by itself as soon as review passes and only tells the secretary. Fewer turns, but the secretary loses control of ordering. |
| Q-8 | R-12: one session per AI per group; R-13: one worktree per task. | The session's working folder is the project folder that contains all worktrees. | Alternative: one session per task, working folder = the task's worktree. Tighter sandbox scope and clean context, but no memory across tasks and it departs from R-12 as written. |
| Q-9 | R-92: other forges can be configured. | Step 1 supports GitHub through `gh`, and no remote. Templates for other forges in step 2. | Keeps step 1 small. |
| Q-10 | R-41: pushing code needs approval at level 2. | Every push asks, including pushes of the task's own branch. | A task produces several cards. A rule "pushing the task's own branch is inside the workspace" would remove most of them; that is the owner's call. |
| Q-11 | R-41 lists bulk deletion and use of secrets. | Neither is detected inside the workspace: the sandbox allows deletion there, and reading a secrets file is a read. | Detecting them means inspecting commands, which the plan refuses to do. Git protects tracked files; the instructions forbid posting secrets. |
| Q-12 | R-134: the home holds the only copy of a group. | No copy and no move in step 1. | A dead disk loses the group's history. Code is in Git and is not affected. |
| Q-13 | Notes are the long-term memory (R-102) but retention is three months (R-135). | Notes are deleted with everything else. | Either notes are exempt, or "long-term" means three months. |
| Q-14 | R-34 notifications and R-163. | In-page notifications from M4; closed-page reminders depend on Q-5. | — |

## K. Known limits

| Id | Limit | Consequence |
| --- | --- | --- |
| K-1 | A group works only while its home is on and reachable. | Stated to the user at creation and on the computers page (R-134). |
| K-2 | A browser cannot open a connection without some signalling path: a rendezvous service, or a node on the same machine. Two devices behind NAT on different networks cannot find each other while no rendezvous is reachable. | "Works without the server" means: open connections continue, nodes on one network or with a reachable remembered address reconnect, and a local browser works through its local node. |
| K-3 | An inbox item is written to a session at most once. A node that stops between recording and writing loses that turn. | The secretary's follow-up covers it. No redelivery machinery. |
| K-4 | Device log: a stolen device and its owner can race to revoke each other, and receivers may disagree. | Recovery is manual. Not designed in step 1. |
| K-5 | Signalling envelopes older than two minutes are dropped. | A device with a badly wrong clock cannot connect; the error says so. |
| K-6 | The node cannot stop the owner from resuming a session id in their own terminal outside SoloDawn. Two writers lose messages `[verified]`. | Stated in the documentation and on the terminal page. |
| K-7 | Usage figures are whatever each CLI reports. | The card shows what exists. |
| K-8 | Check commands run on the assignee's machine with the owner's rights, outside any sandbox. | Acceptable while the group admin is the machine owner (step 1). Step 2 adds per-owner acceptance. |
| K-9 | Home and clients do not verify each other's software version beyond the protocol number. | A protocol change bumps `v`; old clients are told to update. |

## T. Technical risks

| Id | Risk | Settled by |
| --- | --- | --- |
| T-1 | Claude Code's control messages on stdin (interrupt) and the event shapes are not a published contract; Codex app-server is marked experimental. Either can change in any release. | The node uses a handful of messages; fake CLIs pin the expected shapes; a CI job diffs the Codex schema; `live` tests before each release; the CLI version is shown next to each AI. |
| T-2 | Claude Code may make its bare mode the default for `-p`, which does not use subscription sign-in `[documented]`. | The node reports `login_invalid` and does nothing else (R-56). The owner chooses API key or waits. |
| T-3 | How long a permission prompt call or a long tool call may stay open is unknown for both CLIs. | Spikes S-2, S-3. The flows already work with a short wait. |
| T-4 | Windows is unverified everywhere: ConPTY, sockets, interrupt, sandbox. | Spike S-5; Windows runs in CI from M1. |
| T-5 | Browser and pion interoperability: ICE-TCP from Safari, `.local` candidates, message size. | Spike S-1. |
| T-6 | About 240 MB of memory per open CLI session `[verified for Claude Code]`. | `max_open` and dormancy. Measured in A8.6. |
| T-7 | Anyone can obtain relay credentials by presenting any key; the quota is per device id. | A second quota per source address on the rendezvous. Small, and at a boundary. |
| T-8 | Subscriptions are sold for ordinary individual use; heavy automated use may be rate-limited by the vendor. | Limits R-60 to R-62; `limited` state; nothing hidden from the owner. |
| T-9 | Unsigned binaries trigger warnings when downloaded by hand. | The AI installs from the command line; documentation covers the manual case (R-151). |
| T-10 | Plan size against "least code". A desk estimate before this plan was 7,000 to 12,000 lines for node, rendezvous and web client together. | Every pull request is reviewed for code that can be deleted. The backend list names what not to build. |
| T-11 | Port 443 shared by HTTPS and TURN. | Spike S-6; fallback is a second address or port for TURN. |
| T-12 | A secretary woken only by events may not notice a silent assignee. | Observed in A5.1 `live`; the fix is one timer in the home (`flows/task.md` H). |

## Assumptions that would change the design if false

1. A group has at most a few dozen members and a few hundred open tasks, so
   a home keeps all state rows in memory.
2. One node hosts at most a handful of open sessions at a time.
3. Messages are text of a few kilobytes; nothing binary travels in groups.
4. The owner's Git and `gh` are already signed in on every machine that
   runs an AI.
