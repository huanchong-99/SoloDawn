Feature: Step 2 exporting, importing, moving and restoring a group
  A group owner moves a group from one owned node to another by exporting with
  --to and importing on the new home so rows, task counters and unacknowledged
  inbox items carry over while browsers and member hosts follow moved within
  5 seconds, and restores a lost home on another owned node from a periodic
  copy export followed by an invitation that re-points existing members.

  Scenario: Exporting with --to P2 on P1 and importing on P2 moves the group cleanly (A9.27)
    Given the export cast where P's group "Atlas" on P1 has Q, AIs on P1, P2 and Q1, task 1, a gone note and an unacknowledged item for Q-team
    When P runs solodawn group export for "Atlas" with --to P2 on P1
    Then P1 omits "Atlas" from welcome and answers cmd, sync and fetch for "Atlas" with moved naming P2
    When P runs solodawn group import of the exported file on P2
    Then the rows of "Atlas" on P2 equal the rows P1 held including gone markers
    And P's browser, Q's browser and Q1 follow "Atlas" to P2 within 5 seconds of the import without a reload
    And the unacknowledged item from P1 is delivered once to Q-team by P2 and the next task on P2 gets number 2
    And importing the exported file on Q1 fails and exporting "Atlas" on a node that is not its home fails

  Scenario: Restoring the last periodic copy on P2 after P1's data directory is deleted (A9.28)
    Given the export cast where P's group "Beacon" on P1 has Q on Q's browser and Q-team on Q1
    When periodic copy exports of "Beacon" run on P1 and P writes rows after the last copy before P1's data directory is deleted
    And P runs solodawn group import of the last copy of "Beacon" on P2
    Then P2 is the home of "Beacon" with the rows of that last copy and none written later
    When P creates an invitation for "Beacon" on P2 and Q's browser opens it and sends join as "Q"
    Then join on P2 answers ok with no new member row for Q
    And Q's browser and Q1 follow P2 for "Beacon"
