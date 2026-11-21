Feature: Host approvals and CLI decision cards
  Scenario Outline: Permission request at default level 3 produces an approval card and returns the decision
    Given a paired host with a second browser device and a default level 3 "<cli>" "<session>" session
    When the "<cli>" session asks to run "pnpm add -D @playwright/test" with why "Run browser checks"
    And the second browser device "<verb>" the posted approval card while the session waits
    Then the "<cli>" session receives "<outcome>"
    And the posted approval card has "<session>" thread and command "pnpm add -D @playwright/test" and why "Run browser checks"
    And the approval table holds 0 rows

    Examples:
      | cli    | session | verb     | outcome |
      | claude | task    | approves | allow   |
      | codex  | task    | approves | allow   |
      | claude | task    | denies   | deny    |
      | codex  | task    | denies   | deny    |
      | claude | group   | approves | allow   |
      | claude | review  | approves | allow   |

  Scenario: Claude Code AskUserQuestion maps options and custom free text through decision cards
    Given a paired host with a second browser device and a default level 3 "claude" "task" session
    When Claude Code calls AskUserQuestion with header "Storage" and question "Which engine?" and options "SQLite:Embedded" and "Postgres:Server"
    And the second browser device chooses option "A" with note "" on the posted decision card
    Then the posted decision card has options "A:SQLite" and "B:Postgres" and context "Storage"
    And Claude Code receives allow with answer "SQLite" for "Which engine?" and note ""
    When Claude Code calls AskUserQuestion with header "Cache" and question "Which cache?" and options "Memory:In-process" and "Redis:Shared"
    And the second browser device chooses option "B" with note "Use TLS" on the posted decision card
    Then Claude Code receives allow with answer "Redis" for "Which cache?" and note "Use TLS"
    When Claude Code calls AskUserQuestion with header "Custom" and question "Any custom store?" and options "SQLite:Embedded" and "Postgres:Server"
    And the second browser device answers the posted decision card with custom text "Use DuckDB"
    Then Claude Code receives allow with answer "Use DuckDB" for "Any custom store?" and note ""

  Scenario: Codex requestUserInput maps option and custom free-text answers in task and secretary group sessions
    Given a paired host with a second browser device and a default level 3 "codex" "task" session
    When the Codex session asks requestUserInput id "q_task" header "Tests" question "Which test flag?" with options "race:Race detector" and "short:Fast run"
    And the second browser device chooses option "A" with note "" on the posted decision card
    Then Codex Reply receives "race" for question "q_task"
    Given a paired host with a second browser device and a default level 3 "codex" "group" session
    When the Codex session asks requestUserInput id "q_arch" header "Layout" question "Where should settings live?" with options "Top bar:Visible" and "Sidebar:Compact"
    And the second browser device chooses option "B" with note "" on the posted decision card
    Then the posted decision card has no task thread and options "A:Top bar" and "B:Sidebar"
    And Codex Reply receives "Sidebar" for question "q_arch"
    When the Codex session asks requestUserInput id "q_custom" header "" question "Custom branch prefix?" with options "feat:Feature" and "fix:Bugfix"
    And the second browser device answers the posted decision card with custom text "task/"
    Then Codex Reply receives "task/" for question "q_custom"

  Scenario: Timed-out wait, restart, late approval via FetchRow, and one-time retry
    Given a paired host with a second browser device and a default level 3 "claude" "task" session with a 1-second approval wait
    When the "claude" session asks to run "npm install chalk" with why "Install chalk" and no decision arrives
    Then the "claude" session receives deny "No decision yet. The request is still open. Stop this action and end your turn; you will be told when it is decided."
    And the approval table holds 1 row in state "pending"
    When the host restarts on the same store and the home marks the pending card approved by the second browser device
    And the "claude" session retries "npm install chalk" with why "Install chalk"
    Then the "claude" session receives allow without posting another card
    And the approval table holds 0 rows

  Scenario: Level 1 and outbound secrets produce no card
    Given a paired host with a second browser device and a default level 3 "claude" "task" session
    When the "claude" session asks at level 1 to run "rm -rf build"
    Then the "claude" session receives deny "Denied: this session is read only."
    When the "claude" session asks to run "export TOKEN=ghp_0123456789abcdefghijklmnopqrstuvwxyzABCD" with why "Deploy"
    Then the "claude" session receives deny "This request contains a secret and cannot be shown for approval"
    And no card was posted to the home
