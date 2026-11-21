Feature: Task merge, conflict and completion summary in the home
  When a review passes, the home sets step merge and queues merge for the assignee's host without
  starting an AI turn. The host reports task merged or task conflict. On task merged the home marks
  the task merged, writes system task_merged in the thread, notifies the secretary, and releases
  any queued task whose dependencies are now all merged with no cause and stats.started set to the
  release time. The secretary then posts a summary card in the thread and the main line.

  Scenario: Passed review queues merge with no AI turn, then merge and summary record completion
    Given a merge home with a GitHub remote and an active task at step "review"
    When the reviewer passes commit "c1" with score 94 on pull request 28
    Then the merge home queues "merge assignee c1 pr=28" and no session turn item
    When the assignee's host reports commit "c1" merged as "m1" with 318 added and 42 removed
    Then the task is "merged" and its thread ends with system "task_merged"
    And the merge home queues "notice secretary task_merged"
    When the secretary posts summary "Path changes are reported and shown." for the task
    Then the summary appears in the thread and the main line with score 94, 1 round, 72 minutes, 318 added, 42 removed and pull request 28

  Scenario: Merge conflict returns the task to local_check, notifies the secretary, and resolves to merge
    Given a merge home without a remote and an active task at step "merge" for commit "c1"
    When the assignee's host reports a merge conflict on commit "c1" with detail "CONFLICT (content): Merge conflict in path.go"
    Then the task is "active" at step "local_check" waiting on "conflict"
    And the thread ends with a failed "merge" checks message carrying that detail
    And the merge home queues "notice secretary conflict"
    When the secretary assigns the conflicted task to "ai-three"
    And "ai-three"'s host checks and submits commit "c2" and the reviewer passes it with score 92
    And "ai-three"'s host reports commit "c2" merged as "m2" with 40 added and 6 removed
    Then the task is "merged" and its thread ends with system "task_merged"

  Scenario: Merging a dependency releases a queued task without cause and with a fresh start time
    Given a merge home where confirming a two-task brief started task 1 and queued task 2
    When 15 minutes pass and task 1 reaches step "merge" and merges commit "c1" as "m1"
    Then task 2 becomes "active" at step "branch" with stats.started at the release time
    And the merge home queues "notice secretary task_merged, assign ai-two task 2" with no cause on task 2
    When 20 minutes pass and task 2 reaches step "merge" and merges commit "c2" as "m2"
    And the secretary posts summary "Second task complete." for task 2
    Then task 2's summary records 20 minutes of active work
