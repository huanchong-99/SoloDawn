Feature: M4 permissions, approvals, decisions, stops and limits
  Task and group CLI sessions raise approval and decision cards through the
  node's structured drivers and MCP tools, owners decide from any enrolled
  browser device with multiple-choice options or custom free text, timed-out
  waits survive host restarts so an approved command runs once on retry without
  a second card while a different command raises a new card, reassigned or
  cancelled tasks retire pending cards, group pause and node offline controls
  stop running turns and hold inbox items, and long turns time out.

  Scenario: Approving task permission requests from a second browser lets Claude Code and Codex run (A4.1)
    Given an M4 project group with a second browser and configured AIs "Claude" and "Codex"
    When an M4 task for "Claude" asks permission to run "pnpm add -D @playwright/test" with why "Run browser checks" before posting "Claude installed @playwright/test."
    Then the M4 second browser sees a pending approval card on "Claude"'s task for "Bash" command "pnpm add -D @playwright/test" with why "Run browser checks"
    And the M4 second browser approves that pending card and "Claude" posts "Claude installed @playwright/test."
    And the M4 approval card is approved by the owner from the second browser with 0 pending host records
    When an M4 task for "Codex" asks command approval for "pnpm exec playwright install chromium" with why "Install browser binary" before posting "Codex installed chromium."
    Then the M4 second browser sees a pending approval card on "Codex"'s task for "commandExecution" command "pnpm exec playwright install chromium" with why "Install browser binary"
    And the M4 second browser approves that pending card and "Codex" posts "Codex installed chromium."
    And the M4 approval card is approved by the owner from the second browser with 0 pending host records

  Scenario: Denying task permission requests tells Claude Code and Codex, and blocked actions do not run (A4.2)
    Given an M4 project group with a second browser and configured AIs "Claude" and "Codex"
    When an M4 task for "Claude" asks permission to run "npm publish --tag latest" with why "Publish package" and expects owner denial
    Then the M4 first browser denies that pending card and "Claude" posts "Stopped without publishing."
    And the M4 approval card is denied by the owner with no task wait and 0 pending host records
    When an M4 task for "Codex" asks file approval on "config/prod.toml" diff "+port = 80" with why "Edit production config" and expects owner denial
    Then the M4 second browser sees a pending approval card on "Codex"'s task for "fileChange" command "update config/prod.toml\n+port = 80" with why "Edit production config"
    And the M4 second browser denies that pending card and "Codex" posts "Stopped without editing config."
    And the M4 approval card is denied by the owner with no task wait and 0 pending host records

  Scenario: Multiple-choice and free-text decision cards return answers to Claude Code, Codex and request_decision (A4.1)
    Given an M4 project group with a second browser and configured AIs "Claude" and "Codex"
    When an M4 task for "Claude" asks two four-option AskUserQuestion prompts and posts "Claude recorded SQLite and embedded SQL."
    Then the M4 browsers answer "Claude"'s first decision card with option "C" and note "Enable WAL mode" and second with free text "Embed numbered SQL files"
    And "Claude" posts "Claude recorded SQLite and embedded SQL." in the M4 group
    When the M4 owner mentions "Codex" in the main line and it asks two four-option requestUserInput prompts
    Then the M4 browsers answer "Codex"'s main-line decision cards with option "D" and with free text "zstd dictionary"
    And "Codex" posts "Codex recorded MsgPack and zstd." in the M4 group
    When a second M4 task for "Claude" raises a four-option request_decision card and ends its turn
    Then the M4 second browser chooses option "B" with note "Keep contrast high" and wakes "Claude" to post "Received Top bar decision."

  Scenario: With the wait set to 20 seconds and no answer, the turn ends, the host restarts, late approval allows one retry without a second card, and a different command produces a new card (A4.3)
    Given an M4 project group with a 20-second approval wait, a second browser and configured AI "Claude"
    When an M4 task for "Claude" asks permission to run "npm install chalk" with why "Install chalk" and nobody answers for 20 seconds
    Then "Claude"'s turn ends after the 20-second wait while the card stays pending, the task waits on approval, and the host keeps 1 pending record
    When the M4 host restarts on the node store
    And "Claude" is scripted on resume to retry "npm install chalk" and then ask to run "npm install typescript" before posting "Chalk retried once and TypeScript approved."
    And the M4 second browser approves the first card after the restart
    Then the M4 browsers see no second card for "npm install chalk" and see a new pending approval card from "Claude" for "npm install typescript"
    When the M4 second browser approves that new pending card
    Then "Claude" posts "Chalk retried once and TypeScript approved." in the M4 group after 2 resumed starts of the same CLI session and 2 total approval cards

  Scenario: Reassignment marks pending approval cards reassigned, cancellation expires pending cards, and late actions fail with conflict (HM-10 card retirement)
    Given an M4 project group with a 1-second approval wait, a second browser and configured AIs "Claude" and "Codex"
    When an M4 task for "Claude" times out waiting for approval on "npm install left-pad"
    And the M4 owner reassigns that task to "Codex" scripted to post "Codex took over T-1."
    Then the M4 approval card has status "reassigned", "Codex" posts "Codex took over T-1.", and approving the reassigned card fails with "conflict"
    When a second M4 task for "Claude" leaves a pending decision card and a timed-out pending approval card on "npm install rimraf"
    And the M4 owner cancels that second task
    Then both pending M4 cards on the second task have status "expired" and late actions on them fail with "conflict"

  Scenario: Pausing the group interrupts a running turn within 2 seconds, holds mentions until resume, and leaves an approval-waiting task asleep (A4.7)
    Given an M4 project group with a 1-second approval wait, a second browser and configured AIs "Claude" and "Codex"
    When an M4 task for "Claude" times out waiting for approval on "npm install dotenv"
    And "Codex" starts a main-line turn posting "Codex analyzing before pause." and waiting while the group is active
    And the M4 owner pauses the group
    Then "Codex"'s running turn is interrupted within 2 seconds with reason "stop" and system message "group_paused"
    When the M4 owner posts mentions "@Codex first queued note" and "@Codex second queued note" while the group is paused
    Then the 2 M4 mention items stay held on the home and "Codex" does not post while paused
    When the M4 owner resumes the group
    Then "Codex" posts "Codex answered held mentions." in the M4 group with system message "group_resumed" and 0 queued home items
    And "Claude"'s approval-waiting task was not woken by the group resume

  Scenario: Taking all AIs offline closes every session on the node, keeps items on the home until online, and works with the home stopped (A4.8)
    Given an M4 project group with a second browser and configured AIs "Claude" and "Codex"
    When "Claude" holds an open waiting session after posting "Claude ready." and "Codex" is running a turn after posting "Codex busy."
    And the M4 owner takes all AIs on the node offline
    Then both M4 sessions on the node close, "Codex" records turn_interrupted "stop", and both AIs show state "offline"
    When the M4 owner mentions "Claude" with "@Claude while node offline"
    Then the M4 mention item stays unacknowledged on the home and "Claude" starts no new process
    When "Claude" is scripted to keep its resumed session open after posting "Claude back online." and the M4 owner brings the node online
    Then "Claude" posts "Claude back online." and the M4 home inbox is empty
    When the M4 home stops and the M4 owner takes all AIs on the node offline while the home is stopped
    Then "Claude"'s open session on the node closes with the home stopped
    When the M4 home restarts and the M4 owner mentions "Claude" with "@Claude after home restart"
    Then "Claude" shows state "offline" on the restarted M4 home and the mention item stays on the home
    When "Claude" is scripted to post "Claude online after home restart." and the M4 owner brings the node online
    Then "Claude" posts "Claude online after home restart." in the M4 group and the M4 home inbox is empty

  Scenario: With turn_minutes set to 1 a long turn is interrupted with reason timeout (A4.9)
    Given an M4 project group with turn_minutes 1, a second browser and configured AI "Claude"
    When an M4 task for "Claude" posts "Claude started long turn." and runs past the 1-minute turn limit
    Then "Claude"'s task turn is interrupted with reason "timeout" and the task records 1 failure
