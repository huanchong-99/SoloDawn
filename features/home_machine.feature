Feature: The task machine moves one task row
  The home applies planning/contracts/task-machine.md to a task one event at a time.
  The task's assignee is "ai-assignee", its reviewer "ai-reviewer", its commit "c1".

  Scenario: Between a passed review and the host's report only an admin's cancel moves the task
    Given a task row that is "active" at step "review"
    When the reviewer sends "review" to the task machine
    Then the task row is "active" at step "merge" and waits for "nothing"
    And the task machine queues "merge ai-assignee c1"
    When the secretary sends "pause" to the task machine
    Then the task machine answers "conflict"
    When the secretary names "ai-other" as the task's assignee
    Then the task machine answers "conflict"
    When the host sends "submitted" for commit "c1" to the task machine
    Then the task machine answers "conflict"
    When the assignee sends "approval" to the task machine
    Then the task machine answers "ok"
    And the task row is "active" at step "merge" and waits for "nothing"
    When the secretary sends "cancel" to the task machine
    Then the task machine answers "forbidden"
    When the host sends "merged" for commit "c2" to the task machine
    Then the task machine answers "conflict"
    When the admin sends "cancel" to the task machine
    Then the task row is "cancelled" at step "merge" and waits for "nothing"
    And every queued item of the task is retired and its pending cards expire

  Scenario: The host's report of the reviewed commit ends the task
    Given a task row that is "active" at step "merge"
    When the host sends "merged" for commit "c1" to the task machine
    Then the task row is "merged" at step "merge" and waits for "nothing"
    And the task machine queues "notice task_merged"

  Scenario: A reviewer named while the task is paused gets the review when it resumes
    Given a task row that is "active" at step "review"
    And the task row has no reviewer and waits for one
    When the secretary sends "pause" to the task machine
    And the secretary names "ai-other" as the task's reviewer
    Then the task row is "paused" at step "review" and waits for "manual"
    And the task machine queues "nothing"
    When the secretary sends "resume" to the task machine
    Then the task row is "active" at step "review" and waits for "nothing"
    And the task machine queues "review ai-other c1 unless queued, resume ai-assignee"

  Scenario: Deciding the last card of a pre-empted task leaves the pause to the host
    Given a task row that is "active" at step "local_check"
    And the task row waits for the approval card "A1"
    When the host sends "preempted" to the task machine
    And a person decides the approval card "A1"
    Then the task row is "paused" at step "local_check" and waits for "preempted"
    And the task machine queues "decided ai-assignee"
    When the host sends "resume" to the task machine
    Then the task row is "active" at step "local_check" and waits for "nothing"
