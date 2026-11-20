Feature: Group and task workspaces on a host
  Background:
    Given a member joins a workspace group with a local repository

  Scenario: Members share the clone before the brief is confirmed (A5.2)
    When another member joins the workspace group
    Then the group clone is the only registered worktree
    And task 1 has no workspace or branch

  Scenario: Completing one task releases only its worktree and review checkout (A5.1 folders)
    When workspace task 1 starts
    And workspace task 2 starts
    And task 1 commits a change for review
    And the reviewer opens the submitted commit of task 1
    Then review 1 is detached at that commit and contains its diff
    When the host releases the task and review folders for task 1
    Then task 1 has no task or review folder
    And workspace task 2 is on its own branch

  Scenario: A queued dependent task starts from the merged prerequisite (A5.6 folders)
    When workspace task 1 starts
    And task 1 commits a change for review
    And another member joins the workspace group
    Then task 2 has no workspace or branch
    When the prerequisite task 1 is merged into the group base
    And workspace task 2 starts
    Then workspace task 2 is on its own branch
    And workspace task 2 starts at the merged prerequisite

  Scenario: Cancelling a task discards its uncommitted changes and keeps its branch
    When workspace task 1 starts
    And task 1 commits a change for review
    And task 1 leaves an uncommitted change
    And the host removes the worktree of cancelled task 1
    Then task 1 has no task or review folder
    And the branch of task 1 is at that commit
