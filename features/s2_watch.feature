Feature: Watch a takeover

  Scenario: Terminal watching needs the owner's consent and ends at hand-back (A9.20)
    Given Q-team is in a takeover on Q1
    When P asks to watch the takeover
    Then Q sees a pending watch confirmation and Q1 takes no connection of P
    When Q confirms the watch request
    Then Q1 takes P's connection and refuses a watch of Q-personal
    And P receives the owner's terminal bytes in order
    And P's input and resize do not reach the terminal
    When Q hands back the takeover
    Then P's watch channel closes within 2 seconds and Q1 ends P's connection

  Scenario: An open direct watch closes after the same connection selects TURN (A9.21)
    Given Q-team is in a takeover on Q1
    And Q has granted P access to watch
    And P opens a direct watch on Q1
    When the direct path slows down and is cut, so the same connection selects the cast TURN relay
    Then the watch closes and the original ctl answers, refusing watch.start with relay
