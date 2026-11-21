Feature: AI member status and host events in the home
  A host reports the state, task, path and usage of each of its AI members in member status, and
  attaches an event when a turn is interrupted, sign-in is not valid, or a session is replaced
  from notes. A closed host connection sets state = offline for its AIs once no host connection
  of that device remains open.

  Scenario: A crashed turn writes turn_interrupted in the task thread and the AI works again
    Given a group home for host events with a secretary, "codex" on the laptop, "claude" on the home node, and task 1
    When the laptop reports "codex" as "working" on task 1 over "direct" with 12 turns today
    Then the home answers ok to the status
    And "codex" is "working" on task 1 over "direct" with 12 turns today
    When time advances and the laptop reports "codex" as "dormant" with event "turn_interrupted" for reason "crash" on task 1
    Then the home answers ok to the status
    And "codex" is "dormant" on no task
    And task 1 has 1 thread message, a "system" message "turn_interrupted" for "codex"
    And no inbox item was queued by the status
    When time advances and the laptop reports "codex" as "working" on task 1 over "direct" with 13 turns today
    And time advances and the laptop reports "codex" as "working" on task 1 over "relay" with 13 turns today
    Then "codex" is "working" on task 1 over "relay" with 13 turns today and kept its state time on the path change

  Scenario Outline: Host event codes write system rows and notify the secretary only for login_invalid
    Given a group home for host events with <secretary>, "codex" on the laptop, "claude" on the home node, and task 1
    When the laptop reports "codex" as "<state>" with host event "<code>" on "<where>"
    Then the home answers ok to the status
    And the "<code>" system message for "codex" is in "<where>"
    And the status queued "<notice>"

    Examples:
      | secretary    | state   | code             | where  | notice               |
      | a secretary  | dormant | turn_interrupted | task 1 | none                 |
      | a secretary  | dormant | turn_interrupted | main   | none                 |
      | a secretary  | limited | login_invalid    | main   | notice login_invalid |
      | no secretary | limited | login_invalid    | main   | none                 |
      | a secretary  | working | session_replaced | task 1 | none                 |
      | a secretary  | working | session_replaced | main   | none                 |

  Scenario: Closing a host's last connection sets its AIs offline and leaves other hosts untouched
    Given a group home for host events with a secretary, "codex" on the laptop, "claude" on the home node, and task 1
    And the laptop has reported "codex" as "working" on task 1 over "direct" with 3 turns today
    And the laptop has 2 open connections to the home
    When one laptop connection closes
    Then "codex" is still "working" on task 1
    When the last laptop connection closes
    Then "codex" is "offline" on no task
    And "claude" is still "working" on task 1

  Scenario Outline: Invalid or unauthorised member status commands are refused
    Given a group home for host events with a secretary, "codex" on the laptop, "claude" on the home node, and task 1
    When <attempt>
    Then the status is refused with "<error>"

    Examples:
      | attempt                                                     | error       |
      | the owner's browser reports "codex" as "working"            | forbidden   |
      | the laptop, signed as "codex", reports "codex" as "working" | forbidden   |
      | the laptop reports an unknown member as "working"           | not_found   |
      | the laptop reports "codex" with an unknown host event code  | bad_request |
