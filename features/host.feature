Feature: Host sessions
  Scenario Outline: A test can observe every process switch without a vendor account
    Given "<cli>" is found first on PATH with version "<version>"
    When I switch between structured and terminal starts 10 times on session "797fa93e-af8a-4768-b847-42582d5583a0"
    Then the lifecycle log contains 11 start-exit pairs for that session in order

    Examples:
      | cli    | version               |
      | claude | 2.1.311 (Claude Code) |
      | codex  | codex-cli 0.164.0     |

  Scenario Outline: A session changes state only as the session contract lists
    Given a host whose AI has a "<from>" task session
    When the host moves the task session from "<from>" to "<to>"
    Then the move ends with "<outcome>"
    And the host holds the task session as "<state>"

    Examples:
      | from     | to       | outcome                                                              | state    |
      | dormant  | working  | ok                                                                   | working  |
      | waiting  | working  | ok                                                                   | working  |
      | working  | waiting  | ok                                                                   | waiting  |
      | waiting  | dormant  | ok                                                                   | dormant  |
      | working  | dormant  | ok                                                                   | dormant  |
      | dormant  | takeover | ok                                                                   | takeover |
      | waiting  | takeover | ok                                                                   | takeover |
      | working  | takeover | ok                                                                   | takeover |
      | takeover | working  | ok                                                                   | working  |
      | takeover | dormant  | ok                                                                   | dormant  |
      | dormant  | waiting  | conflict: the session is dormant: no change from dormant to waiting   | dormant  |
      | takeover | waiting  | conflict: the session is takeover: no change from takeover to waiting | takeover |

  Scenario Outline: A node that starts finds no session with a process
    Given a host whose AI has a "<left>" task session
    When the host starts again on its store
    Then the start reports the task session as it was left, "<left>"
    And the host holds the task session as "<state>"

    Examples:
      | left     | state    |
      | working  | dormant  |
      | waiting  | dormant  |
      | dormant  | dormant  |
      | takeover | takeover |

  Scenario: An AI runs one turn at a time in a group
    Given a host whose AI has a "dormant" group session
    And a host whose AI has a "working" task session
    When the host moves the group session from "dormant" to "working"
    Then the move ends with "busy: another session of this AI in the group is working or taken over"
    And the host holds the group session as "dormant"

  Scenario Outline: A tool is listed only to the sessions it is for
    Given a host whose AI has a "dormant" task session
    And a tool "seam_checks" that is for task sessions above level 1
    When a CLI process is admitted for the task session at level <level>
    Then the tool "seam_checks" is <listed> to that process

    Examples:
      | level | listed     |
      | 1     | not listed |
      | 2     | listed     |

  Scenario: An inbox item is in the audit record once
    Given a host without sessions
    When the host records inbox item 4411 of home "home-device" as "delivered"
    And the host records inbox item 4411 of home "home-device" as "delivered"
    Then the first record is written and the second is not

  Scenario: A command for a home that is not connected
    Given a host without sessions
    When the host sends a command to a home that is not connected
    Then the host's command fails with "home_unreachable: the home of that group is not connected"
