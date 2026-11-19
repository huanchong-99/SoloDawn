# Steps 2 and 3: outline

Step 1 is specified in detail. These two are outlines: what is added, what
it rests on in step 1, and which contracts grow. They are refined when step
1 is close to done. Rules referenced are in `product-rules.md`.

## Step 2: teams

Several people in one group, each with their own computers and AIs.

### What is added

| Area | Addition | Rules |
| --- | --- | --- |
| People | Invitation link carrying the group id, the home's device id and fingerprint, and a one-time secret. The joiner's browser checks the fingerprint, presents its device log, and becomes a member. Roles `owner`, `admin`, `member`; removing a member. | R-140 |
| Other people's hosts | A member's node joins its AI to a group homed on someone else's node. Nothing new on the wire: it is the `host.join` and `member add_ai` of step 1. | R-134 |
| Whose instructions | A new card kind `confirm`: when someone mentions another person's personal-subscription AI, the home asks that owner. The owner's signed confirmation travels in the inbox item as its `cause`, and the host requires it. No auto-accept switch. | R-51 |
| After a task started | Messages from others in the thread are delivered labelled `information`. A change of direction needs a new confirmation. | R-52 |
| Allow lists | For team-plan and API-key AIs the owner lists who may direct them (`member.allow`), in a list the owner signs, which the host checks. | R-53 |
| Level cap | A task caused by someone other than the owner gets a task session at level 2 at most. | R-44 |
| Calls across owners | `ask` to someone else's AI follows the same confirmation or allow list. | R-115 |
| Approver delegates | The owner names people who may decide approvals for their AIs. | R-42 |
| Watching | A member asks to watch a terminal; the owner confirms; a receive-only terminal channel. | R-81 |
| Team-wide group | Group kind `team`: no repository, no tasks. Secretaries post cross-project notices there. | R-14 |
| Other forges | The forge commands of `contracts/local.md` section 7 become per-group templates (`risks.md` Q-9). | R-92 |
| Home changes | Moving a group to another home; a copy of the group for safekeeping (`risks.md` Q-12). | R-134, R-135 |
| Checks in a sandbox | Project checks, which in a team may be triggered by someone else's request, run inside an existing operating-system sandbox, not one written here: on macOS and Linux preferably Anthropic's open-source sandbox-runtime, otherwise bubblewrap or sandbox-exec. Windows is stated as a known limit. `[unverified]` To be shown feasible by an experiment before it is planned (`risks.md` Q-19). The experiment measures compatibility first, overhead second: can one real offline build and test, with its subprocesses, use the checkout, the toolchain and scratch space while other user data stays unreadable and writes and network outside stay denied? Windows is reported separately. | R-40, R-44 |

### What step 1 already provides for it

- Every command is a signed envelope, and every inbox item is signed by the
  home and carries the signed decision that caused it. In step 1 the home's
  signature is enough, because the home is the owner's device. In step 2 a
  host must also find its owner's signature for the task: the confirmation
  of the brief or of the request, or the owner's allow list. That rule is
  designed at the start of step 2; the fields it needs are already on the
  wire.
- Member rows already carry `account`, `allow`, `approvers`.
- Device logs are already distributed through homes.

### Known design points to settle at the start of step 2

1. **Level of a task session.** Each task has its own session, so the
   level cap of R-44 is set when that session is created. What remains to
   settle is who counts as having caused a task: the person who confirmed
   the brief, or the owner who confirmed the request for their AI.
2. **Check commands on other people's machines.** `group.checks` is set by
   a group admin and executed by every assignee's node. Each machine owner
   must accept the list, and any change to it, before their node runs it.
3. **What a browser trusts.** In step 1 the browser shows what the home
   pushes. With homes run by others, the browser should verify message
   envelopes before showing an author. The envelopes are already in the
   rows.
4. **Stolen device race** in the device log (`risks.md` K-4) becomes more
   serious when other people's homes hold a copy of the log.
5. **Secretary on a personal subscription** in a busy team makes its owner
   a bottleneck for confirmations; the documentation recommends a team plan
   or API key for the secretary.

### Contracts that grow

- `contracts/commands.md`: `invite`, `join`, `member set role`,
  `member remove` for people; card kind `confirm` with `act confirm`.
- `contracts/rows.md`: `msg` kind `confirm`; group kind `team`.
- `contracts/datachannel.md`: section 8 row "a device of another member"
  becomes active; receive-only terminal channel.
- Nothing in the step 1 shapes changes meaning.

## Step 3: extensions

Three items decided by the owner, and one conditional item added by the
coordinator (3.4).

### 3.1 More CLIs, one at a time

For each candidate, in this order:

1. **Terms.** Is starting the vendor's official program under the user's
   own sign-in, from another program, allowed for that account type? If the
   answer is "API key only", the CLI is offered for API keys only (R-54
   is this case for Google models).
2. **Capabilities.** A documented structured or headless mode; resuming a
   session by id; an MCP client; an approval channel that is not a terminal
   prompt; an interactive command that resumes the same session.
3. **Driver.** The same five operations as the two step 1 drivers: start or
   resume, send a turn, events (turn end with usage, approval request,
   exit), interrupt, close. Plus the level mapping and a fake CLI script.
4. **Acceptance.** The M3, M4 and M7 tests of step 1 run for the new CLI.

A CLI that lacks one capability is offered with that feature switched off
(for example no takeover) only if the product still makes sense without it;
otherwise it waits.

`[unverified]` From earlier desk research, candidates that appear to have
headless mode, resume and MCP: Qwen Code, OpenCode, Amp. Others had gaps.
Every claim is rechecked when the work starts; CLIs change monthly.

### 3.2 Design style presets

Named style guides that the secretary attaches to briefs for interface
work: a preset is a document (principles, tokens, do and do not). Stored
per group as notes of a new type; chosen in the brief.

### 3.3 Architecture knowledge base

Long-lived, searchable documents per group that outlast message retention:
decisions, module contracts, conventions. Two tools, `kb_put` and
`kb_search`; stored on the home.

### 3.4 TCP simultaneous open between two NATed nodes (conditional)

Built only if spike S-9 of step 1 shows a clear benefit: a measurable
share of real networks where UDP is blocked on both sides, neither router
maps a port, and simultaneous open succeeds. It would be a transport beside
pion's ICE, so the bar is high (`risks.md` Q-1).

## Not in the first version

Phone app. Bridges to chat applications. Cloud-hosted AIs joining through a
public interface of the home. A built-in rule engine for quality gates.
