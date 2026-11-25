Feature: Secretary bulletins over existing home connections
  Scenario: A secretary posts to a known team as its owner
    Given a secretary with an existing connection to a known team home
    When the team secretary calls post with text "Ready" and mentions its owner
    Then the team home stores a signed person bulletin naming the project and secretary
    And the team home queues no inbox item

  Scenario: A secretary's turn and instructions say where it can post
    Given a secretary with an existing connection to a known team home
    When the host composes a turn of the team secretary
    Then the turn names the team group with its id and the instructions name the argument

  Scenario Outline: A bulletin tool refuses unavailable destinations and working AIs
    Given a secretary with an existing connection to a known team home
    When the bulletin caller is "<caller>" and its destination is "<destination>"
    Then the bulletin tool refuses with "<code>" without posting

    Examples:
      | caller    | destination  | code             |
      | worker    | team         | forbidden        |
      | secretary | unknown      | not_found        |
      | secretary | project      | conflict         |
      | secretary | disconnected | home_unreachable |
      | secretary | secret       | secret           |
