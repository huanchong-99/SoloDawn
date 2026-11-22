Feature: A task role needs its owner's word
  Background:
    Given a role-confirmation home

  Scenario: One brief card covers active and queued roles, with no delivery before approval
    When the first owner confirms a brief assigning two tasks to the other AI
    Then the active role waits for confirmation and the dependent role waits for dependencies
    And the held role does not exempt a thread mention
    When the other owner confirms the role card
    Then the role delivery is "assign"
    And the active role has no confirmation wait

  Scenario: Denying the role notifies the secretary and later activation cannot bypass it
    When the first owner confirms a brief assigning two tasks to the other AI
    And the other owner denies the role card
    Then the role delivery is "notice"
    When the first owner pauses and resumes the role task
    Then the role delivery is ""

  Scenario: A reviewer is asked at role creation but waits only when review is reached
    When the first owner confirms a brief naming the other AI as reviewer
    Then the active role has no confirmation wait
    When the assignee submits passing local checks
    Then the active role waits for confirmation
    And the first owner cannot confirm the role card
    When the other owner confirms the role card
    Then the role delivery is "review"

  Scenario: An admin's reassignment to another owner's AI asks that owner
    When the first owner assigns the existing task to the other AI
    Then the active role waits for confirmation
    When the other owner confirms the role card
    Then the role delivery is "assign"

  Scenario: The owner's own reassignment is sufficient without a card
    When the other owner assigns the existing task to their AI
    Then the role delivery is "assign"
    And no role-confirmation card was created

  Scenario: A replacement brief requires a new confirmation
    When the first owner confirms a brief assigning two tasks to the other AI
    And the other owner confirms the role card
    And the first owner confirms another brief for the other AI
    Then the active role waits for confirmation
