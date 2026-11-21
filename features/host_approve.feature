Feature: Host approvals
  Scenario Outline: Level 2 approval card is approved from a second owner device
    Given a paired host with a second browser device and a level 2 "<cli>" task session
    When the "<cli>" session asks to run "pnpm add -D @playwright/test" with why "Run browser checks"
    And the second browser device approves the posted approval card while the session waits
    Then the "<cli>" session receives allow
    And the posted card has command "pnpm add -D @playwright/test" and why "Run browser checks"
    And the approval table holds 0 rows
    And the host audit records "requested" and "approval" with result "approved" by the second browser device

    Examples:
      | cli    |
      | claude |
      | codex  |

  Scenario Outline: Denying a waiting approval card tells the AI and removes the record
    Given a paired host with a second browser device and a level 2 "<cli>" task session
    When the "<cli>" session asks to run "git push origin extra-branch" with why "Publish extra branch"
    And the second browser device denies the posted approval card while the session waits
    Then the "<cli>" session receives deny "Denied by the owner."
    And the approval table holds 0 rows
    And the host audit records "requested" and "approval" with result "denied" by the second browser device

    Examples:
      | cli    |
      | claude |
      | codex  |

  Scenario: Timed-out wait, host restart, late approval, one-time retry, and new command card
    Given a paired host with a second browser device and a level 2 "claude" task session with a 1-second approval wait
    When the "claude" session asks to run "npm install left-pad" with why "Install package" and no decision arrives
    Then the "claude" session receives deny "No decision yet. The request is still open. Stop this action and end your turn; you will be told when it is decided."
    And the approval table holds 1 row in state "pending"
    When the host restarts on the same store and the second browser device approves the pending card
    Then the approval table holds 1 row in state "granted"
    When the "claude" session retries "npm install left-pad" with why "Install package"
    Then the "claude" session receives allow without posting another card
    And the approval table holds 0 rows
    When the "claude" session asks to run "npm install chalk" with why "Install different package" and the second browser device approves while waiting
    Then a second approval card was posted and the "claude" session receives allow

  Scenario: An approval decision from an unknown device is refused by the host
    Given a paired host with a second browser device and a level 2 "claude" task session
    When the "claude" session asks to run "curl https://example.com" with why "Fetch fixture"
    And an unknown device injects an approval decision while the session waits
    And the second browser device approves the posted approval card while the session waits
    Then the "claude" session receives allow
    And the host audit contains a "refused" row naming the unknown device
    When the "claude" session asks to run "curl https://example.org" with why "Second fetch" under a 1-second wait and no decision arrives
    And an unknown device injects a late approval decision after the wait ended
    Then the late forged decision is refused and the approval table holds 1 row in state "pending"

  Scenario: Level 1, level 3, and outbound secrets produce no card
    Given a paired host with a second browser device and a level 2 "claude" task session
    When the "claude" session asks at level 1 to run "rm -rf build"
    Then the "claude" session receives deny "Denied: this session is read only."
    When the "claude" session asks at level 3 to run "rm -rf build"
    Then the "claude" session receives allow
    When the "claude" session asks to run "export TOKEN=ghp_0123456789abcdefghijklmnopqrstuvwxyzABCD" with why "Deploy"
    Then the "claude" session receives deny "This request contains a secret and cannot be shown for approval"
    And no approval card was posted to the home
