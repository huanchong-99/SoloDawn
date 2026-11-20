Feature: Group home
  A node that is home to a group takes signed commands from the group's
  clients. It answers a command once, however often the command arrives, and
  only when it is signed by the device of its connection and that device is a
  member's.

  Scenario: A command sent twice produces one row and the same answer
    Given a group home with its owner as the only member
    When the owner's browser sends a command that writes a note
    And the owner's browser sends the same command again
    Then the home has written 1 row
    And both answers are the same success

  Scenario: A command signed by another device than the connection's is refused
    Given a group home with its owner as the only member
    When a command that the owner's browser signed arrives on the connection of the owner's laptop
    Then the answer is the error "forbidden"
    And the home has written 0 rows
    And the same command on the browser's own connection is still answered with success

  Scenario Outline: A command from a device that is not a member's is refused
    Given a group home with its owner as the only member
    When <who> sends a command that writes a note
    Then the answer is the error "<code>"
    And the home has written 0 rows

    Examples:
      | who                                   | code           |
      | a device that no person owns          | unknown_device |
      | a device of a person who is no member | forbidden      |
