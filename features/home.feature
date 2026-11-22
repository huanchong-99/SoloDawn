Feature: Group home
  A node that is home to a group takes signed commands from the group's
  clients. It answers a command once, however often the command arrives, and
  only when it is signed by a member's device.

  Scenario: A command sent twice produces one row and the same answer
    Given a group home with its owner as the only member
    When the owner's browser sends a command that writes a note
    And the owner's browser sends the same command again
    Then the home has written 1 row
    And both answers are the same success

  Scenario Outline: A command from a device that is not a member's is refused
    Given a group home with its owner as the only member
    When <who> sends a command that writes a note
    Then the answer is the error "<code>"
    And the home has written 0 rows

    Examples:
      | who                                   | code           |
      | a device that no person owns          | unknown_device |
      | a device of a person who is no member | forbidden      |
