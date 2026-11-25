Feature: Step 2 exporting and importing a group

  Scenario: Move a group to another node of its owner (A9.27)
    Given P has a project group "Atlas" on P1 with Q and Q-team on Q1
    And the group has a secretary on P1, P-work on P2, task 1 and a gone note
    And P1 holds an unacknowledged mention for Q-team
    When P exports the group on P1 with --to P2
    Then P1 no longer lists the group and answers cmd, sync and fetch with moved to P2
    When P imports the file on P2
    Then P2 is home and has the exported rows
    And the two browser peers follow P2 within 5 seconds of import without a reload
    And Q1 follows P2
    And P2 delivers the unacknowledged mention once and the next task gets number 2
    And the export cannot be imported on Q1 or exported again from P1

  # Q1's offer to P2 before the import is dropped and waits 10 seconds for an answer, then backs off (issues.md, 2026-11-26).
  @pending
  Scenario: A member's node follows a moved group within 5 seconds (A9.27)
    Given P has a project group "Atlas" on P1 with Q and Q-team on Q1
    And the group has a secretary on P1, P-work on P2, task 1 and a gone note
    And P1 holds an unacknowledged mention for Q-team
    When P exports the group on P1 with --to P2
    And P imports the file on P2
    Then Q1 follows P2 within 5 seconds of import

  Scenario: Restore the last copy of a lost home and rejoin by invitation (A9.28)
    Given P has a project group "Beacon" on P1 with Q and Q-team on Q1
    When P takes two successive copies on P1 and writes a note after each copy
    And P1 is stopped and its data directory is deleted
    And P imports the file on P2
    Then P2 is home and has the exported rows
    When Q follows an invitation from P2 and join succeeds without adding a member row
    Then Q's browser peer and Q1 use P2 as the restored home
