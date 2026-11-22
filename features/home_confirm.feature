Feature: Confirmation cards in the home
  Requests outside an AI's task participation wait for its owner's word.
  Cards, call denials and current approval delegates are committed by the home.

  Background:
    Given a confirmation home with two owners

  Scenario: Another owner's mention waits, accepts either device, and delivers once
    When the first owner mentions the other AI
    Then a pending "direct" card exists with no delivery
    When the first owner tries to confirm it
    Then the confirmation command answers "forbidden"
    When the second owner's second device confirms it twice with one nonce
    Then the confirmation command answers ""
    And the confirmation card is "confirmed"
    And the latest delivery is "mention"
    When the second owner mentions their own AI
    Then the latest delivery is "mention"

  Scenario: Personal accounts ignore the allow list, and denial delivers nothing
    When the second owner configures the other AI as "personal" allowing the first owner
    And the first owner mentions the other AI
    Then a pending "direct" card exists with no delivery
    When the second owner denies it
    Then the confirmation card is "denied"
    And the latest delivery is ""

  Scenario Outline: Listed people direct shared-account AIs until removed
    When the second owner configures the other AI as "<account>" allowing the first owner
    And the first owner mentions the other AI
    Then the latest delivery is "mention"
    When the second owner removes the first owner from the allow list
    And the first owner mentions the other AI
    Then a pending "direct" card exists with no delivery

    Examples:
      | account |
      | team    |
      | api_key |

  Scenario: A cross-owner call is delivered on confirmation and answers its caller on denial
    When the first owner's AI calls the other AI
    Then a pending "ask" card exists with no delivery
    When the second owner confirms it
    Then the latest delivery is "mention"
    When the first owner's AI calls the other AI
    And the second owner denies it
    Then the confirmation card is "denied"
    And the call is denied with a reply and secretary notice

  Scenario: Approval delegates must still be listed on the current AI row
    When the second owner adds the first owner as an approval delegate
    And the other AI posts an approval card for both owners
    And the first owner approves that approval
    Then the confirmation command answers ""
    And the latest delivery is "decided"
    When the other AI posts an approval card for both owners
    And the second owner removes the first owner as an approval delegate
    And the first owner approves that approval
    Then the confirmation command answers "forbidden"

  Scenario: Pending cards expire when their request is retained no longer
    When the first owner mentions the other AI
    And retention removes the request
    Then the confirmation card is "expired"

  Scenario: Watching another owner's AI needs that owner's approval
    When the first owner asks to watch the other AI
    Then a pending "watch" card exists with no delivery
    When the second owner confirms it
    Then the confirmation card is "confirmed"
    And the latest delivery is "watch"

  Scenario: Watch admission does not restrict thread, reply or text fields
    When the first owner asks to watch with thread, reply and text fields
    Then a pending "watch" card exists with no delivery
    When the second owner confirms it
    Then the confirmation card is "confirmed"
    And the latest delivery is "watch"
