Feature: Approval and decision cards in the home
  Working AIs post approval cards for their tasks, and AIs post decision cards in a task thread or
  in the main line. Listed approvers approve or deny an approval card from any of their devices;
  listed addressees or admins choose on a decision card. Reassigning a task marks its pending
  approval cards reassigned, and merging or cancelling a task marks its pending cards expired.

  Scenario: Approving from a second device lets the task continue and queues decided
    Given a card home with an active task assigned to "ai-one"
    When "ai-one" posts an approval card for command "pnpm add -D @playwright/test"
    Then the card home answers ok
    And the approval card is "pending" with command "pnpm add -D @playwright/test"
    And the card task waits for "approval" on the approval card
    When the owner's second browser approves the approval card
    Then the card home answers ok
    And the approval card is "approved" by "owner"
    And the card task waits for "nothing"
    And the card home has queued "decided ai-one task 1" with no cause

  Scenario: Denying an approval card tells the asking AI and clears the wait
    Given a card home with an active task assigned to "ai-one"
    And "ai-one" has posted an approval card for command "git push origin scratch"
    When the owner's first browser denies the approval card
    Then the card home answers ok
    And the approval card is "denied" by "owner"
    And the card task waits for "nothing"
    And the card home has queued "decided ai-one task 1" with no cause

  Scenario: An approval takes priority over a pending decision until decided, and choose tells both AIs
    Given a card home with an active task assigned to "ai-one"
    When the secretary posts a decision card on the task with options "A, B"
    Then the card home answers ok
    And the card task waits for "decision" on the decision card
    When "ai-one" posts an approval card for command "npm install"
    Then the card home answers ok
    And the card task waits for "approval" on the approval card
    When the owner's first browser approves the approval card
    Then the card home answers ok
    And the card task waits for "decision" on the decision card
    And the card home has queued "decided ai-one task 1" with no cause
    When the owner's second browser chooses option "B" with text "Quiet indicator." on the decision card
    Then the card home answers ok
    And the decision card is "decided" with choice "B" and text "Quiet indicator."
    And the card task waits for "nothing"
    And the card home has queued "decided secretary task 1, decided ai-one task 1" with no cause

  Scenario: A multiple-choice decision card accepts a custom free-text answer without an option
    Given a card home with an active task assigned to "ai-one"
    And the secretary has posted a decision card on the task with options "A, B"
    When the owner's first browser answers the decision card with free text "Use a footer badge instead."
    Then the card home answers ok
    And the decision card is "decided" with no choice and text "Use a footer badge instead."
    And the card task waits for "nothing"
    And the card home has queued "decided secretary task 1, decided ai-one task 1" with no cause

  Scenario: Reassignment marks pending approvals reassigned, and cancellation expires remaining cards
    Given a card home with an active task assigned to "ai-one"
    And the secretary has posted a decision card on the task with options "A, B"
    And "ai-one" has posted an approval card for command "npm install"
    When the card task is reassigned to "ai-two"
    Then the approval card is "reassigned" and the decision card is "pending"
    And the card task waits for "decision" on the decision card
    When the owner's first browser tries to approve the approval card
    Then the card home answers "conflict"
    When the card task is cancelled
    Then the decision card is "expired"
    When the owner's first browser tries to choose option "A" on the decision card
    Then the card home answers "conflict"

  Scenario Outline: Unauthorized signers and malformed card actions are refused
    Given a card home with an active task assigned to "ai-one"
    And "ai-one" has posted an approval card for command "npm install"
    And the secretary has posted a decision card on the task with options "A, B"
    When <attempt>
    Then the card home answers "<code>"
    And the approval card is "pending" and the decision card is "pending"
    And the card home has queued nothing

    Examples:
      | attempt                                                                   | code        |
      | a non-approver member tries to approve the approval card                  | forbidden   |
      | "ai-one" tries to approve its own approval card                           | forbidden   |
      | the owner's first browser tries to approve the approval card with an arg  | bad_request |
      | the owner's first browser tries to choose option "C" on the decision card | bad_request |
      | the owner's first browser tries to choose nothing on the decision card    | bad_request |
      | "ai-two" tries to post an approval card on "ai-one"'s task                | forbidden   |
