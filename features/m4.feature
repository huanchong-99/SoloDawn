Feature: M4 permissions, approvals, decisions and the group pause
  Task and group CLI sessions raise approval and decision cards through the
  node's structured drivers and MCP tools, owners decide from any enrolled
  browser device with multiple-choice options or custom free text, timed-out
  waits survive host restarts so an approved command runs once on retry without
  a second card while a different command raises a new card, reassigned or
  cancelled tasks retire pending cards, and pausing the group stops a running
  turn and holds mentions until the resume. The offline switch (A4.8) and the
  turn time limit (A4.9) are in host_limits.feature.
  After each scenario its one-person group shows no confirm card (A9.40).

  Scenario: Approving task permission requests from a second browser lets Claude Code and Codex run (A4.1)
    Given an M4 group with a second browser and configured AIs "Claude" and "Codex"
    When an M4 task for "Claude" asks permission to run "pnpm add -D @playwright/test" with why "Run browser checks" before posting "Claude installed @playwright/test."
    Then the M4 second browser sees a pending approval card on "Claude"'s task for "Bash" command "pnpm add -D @playwright/test" with why "Run browser checks"
    And the M4 second browser approves that pending card and "Claude" posts "Claude installed @playwright/test."
    And the M4 approval card is approved by the owner from the second browser with 0 pending host records
    When an M4 task for "Codex" asks command approval for "pnpm exec playwright install chromium" with why "Install browser binary" before posting "Codex installed chromium."
    Then the M4 second browser sees a pending approval card on "Codex"'s task for "commandExecution" command "pnpm exec playwright install chromium" with why "Install browser binary"
    And the M4 second browser approves that pending card and "Codex" posts "Codex installed chromium."
    And the M4 approval card is approved by the owner from the second browser with 0 pending host records

  Scenario: Denying task permission requests tells Claude Code and Codex, and blocked actions do not run (A4.2)
    Given an M4 group with a second browser and configured AIs "Claude" and "Codex"
    When an M4 task for "Claude" asks permission to run "npm publish --tag latest" with why "Publish package" and expects owner denial
    Then the M4 first browser denies that pending card and "Claude" posts "Stopped without publishing."
    And the M4 approval card is denied by the owner with no task wait and 0 pending host records
    When an M4 task for "Codex" asks file approval on "config/prod.toml" diff "+port = 80" with why "Edit production config" and expects owner denial
    Then the M4 second browser sees a pending approval card on "Codex"'s task for "fileChange" command "update config/prod.toml\n+port = 80" with why "Edit production config"
    And the M4 second browser denies that pending card and "Codex" posts "Stopped without editing config."
    And the M4 approval card is denied by the owner with no task wait and 0 pending host records

  Scenario: Multiple-choice and free-text decision cards return answers to Claude Code, Codex and request_decision (A4.1)
    Given an M4 group with a second browser and configured AIs "Claude" and "Codex"
    When an M4 task for "Claude" asks two four-option AskUserQuestion prompts and posts "Claude recorded SQLite and embedded SQL."
    Then the M4 browsers answer "Claude"'s first decision card with option "C" and note "Enable WAL mode" and second with free text "Embed numbered SQL files"
    And "Claude" posts "Claude recorded SQLite and embedded SQL." in the M4 group
    When the M4 owner mentions "Codex" in the main line and it asks two four-option requestUserInput prompts
    Then the M4 browsers answer "Codex"'s main-line decision cards with option "D" and with free text "zstd dictionary"
    And "Codex" posts "Codex recorded MsgPack and zstd." in the M4 group
    When the M4 owner cancels that task and a second M4 task for "Claude" raises a four-option request_decision card and ends its turn
    Then the M4 second browser chooses option "B" with note "Keep contrast high" and wakes "Claude" to post "Received Top bar decision."

  # SoloDawn's half of level 1 is the settings it starts the CLI with, which
  # make a CLI that obeys them leave the file alone, and the host's refusal of
  # a request to edit. That the real CLI obeys its settings is the vendor's
  # half, on the real-machine list.
  Scenario Outline: At level 1 a request to edit a file changes nothing on disk and produces no card (A4.4)
    Given an M4 group with a "<cli>" AI whose CLI edits a file on every turn
    When a task of that AI runs at level 1
    Then the CLI refused its own edit, the file and the workspace are unchanged and there is no card
    When that task is cancelled and a task of that AI runs at level 3
    Then the same CLI edited the file and there is no card

    Examples:
      | cli    |
      | claude |
      | codex  |

  Scenario Outline: At level 1 the host refuses a CLI's request to edit a file, without a card (A4.4)
    Given an M4 group with a "<cli>" AI whose CLI edits a file on every turn
    When a task of that AI runs at level 1 and its CLI asks the host to edit the file
    Then the CLI refused its own edit, the file and the workspace are unchanged and there is no card

    Examples:
      | cli    |
      | claude |

    Examples:
      | cli    |
      | codex  |

  Scenario: With the wait set to 20 seconds and no answer, the turn ends, the host restarts, late approval allows one retry without a second card, and a different command produces a new card (A4.3)
    Given an M4 group with a 20-second approval wait, a second browser and configured AI "Claude"
    When an M4 task for "Claude" asks permission to run "npm install chalk" with why "Install chalk" and nobody answers for 20 seconds
    Then "Claude"'s turn ends after the 20-second wait while the card stays pending, the task waits on approval, and the host keeps 1 pending record
    When the M4 host restarts on the node store
    And "Claude" is scripted on resume to retry "npm install chalk" and then ask to run "npm install typescript" before posting "Chalk retried once and TypeScript approved."
    And the M4 second browser approves the first card after the restart
    Then the M4 browsers see no second card for "npm install chalk" and see a new pending approval card from "Claude" for "npm install typescript"
    When the M4 second browser approves that new pending card
    Then "Claude" posts "Chalk retried once and TypeScript approved." in the M4 group after 2 resumed starts of the same CLI session and 2 total approval cards

  Scenario: Reassignment marks pending approval cards reassigned, cancellation expires pending cards, and late actions fail with conflict (HM-10 card retirement)
    Given an M4 group with a 1-second approval wait, a second browser and configured AIs "Claude" and "Codex"
    When an M4 task for "Claude" times out waiting for approval on "npm install left-pad"
    And the M4 owner reassigns that task to "Codex" scripted to post "Codex took over T-1."
    Then the M4 approval card has status "reassigned", "Codex" posts "Codex took over T-1.", and approving the reassigned card fails with "conflict"
    When a second M4 task for "Claude" leaves a pending decision card and a timed-out pending approval card on "npm install rimraf"
    And the M4 owner cancels that second task
    Then both pending M4 cards on the second task have status "expired" and late actions on them fail with "conflict"

  Scenario: Pausing the group interrupts a running turn within 2 seconds, holds mentions until resume, and leaves an approval-waiting task asleep (A4.7)
    Given an M4 group with a 1-second approval wait, a second browser and configured AIs "Claude" and "Codex"
    When an M4 task for "Claude" times out waiting for approval on "npm install dotenv"
    And "Codex" starts a main-line turn posting "Codex analyzing before pause." and is scripted to post "Codex answered held mentions." on its next turn
    And the M4 owner pauses the group
    Then "Codex"'s running turn is interrupted within 2 seconds with reason "stop" and system message "group_paused"
    When the M4 owner mentions "Codex" with "@Codex first queued note"
    And the M4 owner mentions "Codex" with "@Codex second queued note"
    Then the M4 home inbox contains 2 items while the group is paused
    When the M4 owner resumes the group
    Then "Codex" posts "Codex answered held mentions." after system message "group_resumed" and the M4 home inbox is empty
    And "Claude"'s approval-waiting task was not woken by the group resume
