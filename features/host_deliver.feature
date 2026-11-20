Feature: Host inbox delivery and group state sync

  Scenario: An item not signed by the group home is refused and an item sent twice runs one turn across a restart
    Given a paired host on disk with AI "Claude Code" in group "Website" homed on "home-1"
    When an inbox item signed by "impostor" is pushed for "Website"
    Then the delivery audit records 1 "refused" row and 0 "delivered" rows
    When "home-1" pushes mention item 11 for "Claude Code" twice
    Then "Claude Code" has received 1 turn and "home-1" has received 2 acknowledgements for item 11
    When the host restarts on its store and "home-1" pushes mention item 11 again
    Then "Claude Code" has received 1 turn and "home-1" has received 3 acknowledgements for item 11

  Scenario: Cold-start items wait for sync and stale task work is refused while notices still deliver
    Given a paired host on disk configured for group "Website" on "home-1"
    When the host attaches to "home-1" while sync is in flight
    And "home-1" pushes assign item 21 for "Claude Code" on task "T-1" and notice item 22 for secretary "Sec"
    Then "Claude Code" has received 0 turns and "home-1" has received 0 acknowledgements
    When "home-1" completes sync at sequence 9 with task "T-1" already merged
    Then the delivery audit records 1 "refused" row and 1 "delivered" row
    And "Sec" has received 1 turn and "Claude Code" has received 0 turns

  Scenario: Sync cursor resets to zero on reset and keeps its cursor on an interrupted delta
    Given a paired host on disk configured for group "Website" on "home-1"
    When the host attaches to "home-1" and an initial reset breaks off after row sequence 4
    And the host reconnects to "home-1" and completes sync at sequence 10
    Then the sync requests asked for since "0,0"
    When "home-1" pushes live row sequence 12 and a reconnect delta breaks off before synced
    And the host reconnects to "home-1" and completes sync at sequence 15
    Then the sync requests asked for since "0,0,12,12"

  Scenario: An open tool call receives its reply immediately while a session mention waits for the running turn
    Given a paired host on disk with AI "Claude Code" working on a turn in group "Website" on "home-1"
    And "Claude Code" has an open ask waiting on call "call-1"
    When "home-1" pushes reply item 31 answering "call-1" and mention item 32 for "Claude Code"
    Then the open ask receives item 31 and "Claude Code" has received 0 new turns
    When the running turn of "Claude Code" finishes and calls Deliver
    Then "Claude Code" has received 1 new turn and "home-1" has received 1 acknowledgement for item 32
