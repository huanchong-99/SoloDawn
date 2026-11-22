Feature: Confirmation cards, allow lists, call denials and approver delegates in the home
  When a request's word is not enough for an AI outside a task it takes part in, the home writes a
  confirm card instead of queuing a mention item. Only the AI's owner confirms or denies a confirm
  card, from any of the owner's devices. Team and API-key AIs accept listed members without a card,
  personal AIs always ask their owner, denied calls become denied and notify the caller and the
  secretary, and approval delegates are checked against the AI member row's current approvers.

  Scenario: Mentioning another owner's personal AI waits for confirmation from either of the owner's devices
    Given a two-owner confirm home with "P" and "Q"
    When "P" mentions "Q-personal" in the main line saying "Check the socket path"
    Then the confirm home answers ok
    And no item is queued and a "pending" "direct" confirm card waits for "Q"
    When "Q"'s second browser confirms the confirm card
    Then the confirm home answers ok
    And the confirm card is "confirmed" by "Q"
    And the confirm home has queued "mention Q-personal"
    When "Q"'s first browser mentions "Q-personal" in the main line saying "Run it now"
    Then the confirm home answers ok
    And the confirm home has queued "mention Q-personal" with no new confirm card

  Scenario: Denying a confirm card, forbidden actors, idempotent confirm, and personal allow list ignored
    Given a two-owner confirm home with "P" and "Q"
    And "Q" sets the allow list of "Q-personal" to include "P"
    When "P" mentions "Q-personal" in the main line saying "Still needs Q"
    Then the confirm home answers ok
    And no item is queued and a "pending" "direct" confirm card waits for "Q"
    When "P" tries to confirm the confirm card
    Then the confirm home answers "forbidden"
    When "Q"'s first browser denies the confirm card
    Then the confirm home answers ok
    And the confirm card is "denied" by "Q"
    And the confirm home has queued nothing
    When "P" mentions "Q-personal" in the main line saying "Second request"
    And "Q"'s first browser confirms the confirm card twice with the same nonce
    Then the confirm home answers ok to both
    And the confirm home has queued "mention Q-personal"

  Scenario: Allow list on a team AI lets listed members direct it until removed
    Given a two-owner confirm home with "P" and "Q"
    When "Q" sets the allow list of "Q-team" to include "P"
    And "P" mentions "Q-team" in the main line saying "Check team logs"
    Then the confirm home answers ok
    And the confirm home has queued "mention Q-team" with no new confirm card
    When "Q" clears the allow list of "Q-team"
    And "P" mentions "Q-team" in the main line saying "After removal"
    Then the confirm home answers ok
    And no item is queued and a "pending" "direct" confirm card waits for "Q"
    When "Q"'s first browser confirms the confirm card
    Then the confirm home answers ok
    And the confirm home has queued "mention Q-team"

  Scenario: Calls across owners wait on an ask card, deliver on confirm, deny on refusal, and skip the card when allowed
    Given a two-owner confirm home with "P" and "Q"
    When "P-worker" calls "Q-personal" in the task thread asking "Which port?"
    Then the confirm home answers ok
    And the call is "waiting" and a "pending" "ask" confirm card waits for "Q"
    When "Q"'s first browser confirms the confirm card
    Then the confirm home answers ok
    And the confirm home has queued "mention Q-personal"
    When "Q-personal" replies to the call with "Port 47800."
    Then the confirm home answers ok
    And the call is "answered" and the confirm home has queued "reply P-worker"
    When "P-worker" calls "Q-personal" in the task thread asking "Second question?"
    And "Q"'s second browser denies the confirm card
    Then the confirm home answers ok
    And the call is "denied" and the confirm home has queued "reply P-worker, notice secretary"
    When "Q" sets the allow list of "Q-team" to include "P"
    And "P-worker" calls "Q-team" in the task thread asking "Allowed call?"
    Then the confirm home answers ok
    And the confirm home has queued "mention Q-team" with no new confirm card
    When "Q-team" calls "Q-personal" in the main line asking "Same owner call?"
    Then the confirm home answers ok
    And the confirm home has queued "mention Q-personal" with no new confirm card

  Scenario: Approver delegates decide cards only while listed on the AI's member row
    Given a two-owner confirm home with "P" and "Q"
    When "Q" names "P" as an approver delegate for "Q-team"
    And "Q-team" posts two main-line approval cards naming "Q" and "P"
    And "P" approves the first approval card
    Then the confirm home answers ok
    And the confirm home has queued "decided Q-team"
    When "Q" removes "P" from the approver delegates of "Q-team"
    And "P" tries to approve the second approval card
    Then the confirm home answers "forbidden"
    When "Q-personal" posts a main-line approval card naming "Q" and "P"
    And "Q" names "P" as an approver delegate for "Q-team"
    And "P" tries to approve the third approval card
    Then the confirm home answers "forbidden"
