# Steps 2 and 3: outline

Step 1 is specified in detail. These two are outlines: what is added, what
it rests on in step 1, and which contracts grow. They are refined when step
1 is close to done. Rules referenced are in `product-rules.md`.

## Step 2: teams

Several people in one group, each with their own computers and AIs.

### What is added

| Area | Addition | Rules |
| --- | --- | --- |
| People | Invitation link carrying the group id, the home's device id and a one-time secret. The joiner's browser presents its device log and becomes a member. Roles `owner`, `admin`, `member`; removing a member. | R-140 |
| Other people's hosts | A member's node joins its AI to a group homed on someone else's node. Nothing new on the wire: it is the `host.join` and `member add_ai` of step 1. | R-134 |
| Whose instructions | A new card kind `confirm`: when someone mentions another person's personal-subscription AI, the home asks that owner, and queues the item once the owner confirms. No auto-accept switch. | R-51 |
| After a task started | Messages from others in the thread are delivered labelled `information`. A change of direction needs a new confirmation. | R-52 |
| Allow lists | For team-plan and API-key AIs the owner lists who may direct them (`member.allow`); the home reads the row. | R-53 |
| Level cap | A task caused by someone other than the owner gets a task session at level 2 at most. Not in 1.0 (2026-11-22): every session runs at the highest level. | R-44 |
| Calls across owners | `ask` to someone else's AI follows the same confirmation or allow list. | R-115 |
| Approver delegates | The owner names people who may decide approvals for their AIs. | R-42 |
| Watching | A member asks to watch a terminal; the owner confirms; a receive-only terminal channel. | R-81 |
| Team-wide group | Group kind `team`: no repository, no tasks. Secretaries post cross-project notices there. | R-14 |
| Other forges | The forge commands of `contracts/local.md` section 7 become per-group templates (`risks.md` Q-9). The client shows a group's check commands and templates before an AI joins it. | R-92 |
| Home changes | Moving a group to another home; a copy of the group for safekeeping (`risks.md` Q-12): one export on the old node and one import on the new (`contracts/step-2/export.md`). | R-134, R-135 |
| Checks in a sandbox | Cut on 2026-11-22: no sandbox for checks in 1.0; `risks.md` K-8 stays a stated limit. | R-40 |

### What step 1 already provides for it

- Every command is a signed envelope, so a home knows who sent it. Inbox
  items are plain objects that a host takes from its home; in step 2 too,
  because members of a group trust each other (owner's rules of 2026-11-21
  and 2026-11-22). The home decides who may direct an AI.
- Member rows already carry `account`, `allow`, `approvers`.
- Device logs are already distributed through homes.

### Known design points to settle at the start of step 2

1. **Level of a task session.** Not in 1.0 (2026-11-22): R-44 is not
   applied, and every session runs at the highest level.
2. **Check commands on other people's machines.** Cut on 2026-11-22: no
   acceptance per machine; the client shows the commands.
3. **What a browser trusts.** Cut on 2026-11-22: it shows what the home
   pushes, as in step 1.
4. **Stolen device race.** Cut on 2026-11-22: the step 1 rule stays
   (`risks.md` K-4). The cuts are listed in `steps/parallel-plan-step-2-3.md`,
   "Cuts of 2026-11-22".
5. **Secretary on a personal subscription** in a busy team makes its owner
   a bottleneck for confirmations; the documentation recommends a team plan
   or API key for the secretary.

### Contracts that grow

- `contracts/commands.md`: `invite`, `join`, `member set role`,
  `member remove` for people; card kind `confirm` with `act confirm`.
- `contracts/messages.md`: kind `confirm`. `contracts/rows.md`: group kind
  `team`.
- `contracts/datachannel.md`: section 8 row "a device of another member"
  becomes active; receive-only terminal channel.
- Nothing in the step 1 shapes changes meaning.
- The full lists are in the contracts of `contracts/step-2/` and
  `contracts/step-3/`, each of which names the step 1 files it adds to.

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
Every claim is rechecked when the work starts; CLIs change monthly. Desk
research unit T3-0 did so: OpenCode and Qwen Code passed, for API keys
only, and get drivers; Amp is deferred (`contracts/step-3/drivers.md`
section 8).

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
Viewing the full text of a long approval request (`risks.md` K-21).
