Feature: The task machine moves one task row
  The home applies planning/contracts/task-machine.md to a task one event at a time.
  The task's assignee is "ai-assignee", its reviewer "ai-reviewer", the report under review "c1".

  Scenario: A passed review makes the task done at once, and a done task moves no more (A5.12)
    Given a task row that is "active" at step "review"
    When the reviewer sends "review" to the task machine
    Then the task row is "done" at step "review" and waits for "nothing"
    And the task machine queues "notice task_done"
    And the task machine writes a "task_done" system message and its pending cards expire
    When the secretary sends "pause" to the task machine
    Then the task machine answers "conflict"
    When the secretary names "ai-other" as the task's assignee
    Then the task machine answers "conflict"
    When the assignee sends "report" to the task machine
    Then the task machine answers "conflict"
    When the admin sends "cancel" to the task machine
    Then the task machine answers "conflict"

  Scenario: A review of another report is refused and a new report sends the task through review again (A5.4)
    Given a task row that is "active" at step "review"
    When the reviewer sends "review" for report "c0" to the task machine
    Then the task machine answers "conflict"
    When the assignee sends "report" for report "c2" to the task machine
    Then the task row is "active" at step "review" and waits for "nothing"
    And the task machine retires the queued review items and queues "review ai-reviewer c2"

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
    Given a task row that is "active" at step "work"
    And the task row waits for the approval card "A1"
    When the host sends "preempted" to the task machine
    And a person decides the approval card "A1"
    Then the task row is "paused" at step "work" and waits for "preempted"
    And the task machine queues "decided ai-assignee"
    When the host sends "resume" to the task machine
    Then the task row is "active" at step "work" and waits for "nothing"
