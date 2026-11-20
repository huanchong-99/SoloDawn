Feature: SQLite group history
  Scenario: Editing an earlier card advances sync without moving its page position
    Given the main line has a pending card followed by a message
    When the card is decided
    Then syncing after sequence 2 returns only the decided card at sequence 3
    And paging before position 2 returns that card at position 1
