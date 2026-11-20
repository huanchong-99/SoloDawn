Feature: The host merges the reviewed task commit
  Background:
    Given a reviewed task commit in a local task worktree

  Scenario: A commit added after review is refused
    Given the assignee has advanced the task branch after review
    When the host takes the merge item for the reviewed commit
    Then the host reports a conflict naming the moved head
    And the base commit and index are unchanged

  Scenario: An overlapping change on the base fails the squash
    Given the base has independently changed the same line
    When the host takes the merge item for the reviewed commit
    Then the host reports the Git merge conflict
    And the base commit and index are unchanged

  Scenario: Restart after the squash commit and before the report
    When the host takes the merge item for the reviewed commit
    Then the base contains the reviewed change in a single squash commit
    When the restarted host inspects the taken merge item
    Then the host reports the existing squash commit

  Scenario: Restart after taking the item and before merging
    When the restarted host inspects the taken merge item
    Then the host reports that the base does not record this merge
    And the base commit and index are unchanged
