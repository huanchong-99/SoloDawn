Feature: Task commands in the home
  Working AIs start direct and takeover tasks; the assignee's host reports branch creation,
  crashed or timed-out turns and pre-emption; the secretary and admins assign, pause, resume
  and cancel tasks through the task machine.

  Scenario: A direct task creates a task card, notifies the secretary and accepts only its own branch
    Given a group home for task commands
    When "ai-one" starts a direct task titled "Fix: relay notice" for the owner's request
    Then the home answers ok with task number 1
    And task 1 is "active" at step "branch" with no wait
    And task 1 has a task card in the main line
    And the home queued "assign ai-one with request, notice secretary task_started"
    When the host of "ai-one" reports branch "task/9-wrong" for task 1
    Then the home refuses the task command with "bad_request"
    When the host of "ai-one" reports branch "task/1-fix-relay-notice" for task 1
    Then the home answers ok to the task command
    And task 1 is "active" at step "local_check" with no wait

  Scenario: Three consecutive failures stop the task and mention the owner; only a person recovers it
    Given a group home for task commands
    And task 1 is active for "ai-one" from a confirmed brief
    When the host of "ai-one" reports a "crash" failure on task 1
    And the host of "ai-one" sends the same failure command again
    Then both failure commands are answered ok and task 1 has 1 failure
    When the host of "ai-one" reports a "timeout" failure on task 1
    And the host of "ai-one" reports a "crash" failure on task 1
    Then task 1 is "failed" at step "local_check" with no wait
    And the thread of task 1 holds a "task_failed" system message mentioning the owner
    When the secretary tries to resume task 1
    Then the home refuses the task command with "forbidden"
    When the secretary tries to assign task 1 to "ai-two"
    Then the home refuses the task command with "forbidden"
    When the owner assigns task 1 to "ai-two"
    Then the home answers ok to the task command
    And task 1 is "active" at step "local_check" with no wait
    And task 1 has 0 failures and the home queued "assign ai-two"

  Scenario: At step merge pause and reassignment are refused and only an admin cancels
    Given a group home for task commands
    And task 1 is active at step "merge" with a pending approval, a queued merge item and queued dependent task 2
    When the secretary tries to pause task 1
    Then the home refuses the task command with "conflict"
    When the owner tries to assign task 1 to "ai-two"
    Then the home refuses the task command with "conflict"
    When the secretary tries to cancel task 1
    Then the home refuses the task command with "forbidden"
    When the owner cancels task 1
    Then the home answers ok to the task command
    And task 1 is "cancelled" at step "merge" with no wait
    And the pending approval of task 1 is "expired" and its merge item is retired
    And the home queued "notice secretary dependency_cancelled"

  Scenario: A pre-empted task pauses with a system message and continues when its host resumes it
    Given a group home for task commands
    And task 1 is active for "ai-one" from a confirmed brief
    When the host of "ai-one" reports task 1 pre-empted by the owner's request
    Then task 1 is "paused" at step "local_check" waiting for "preempted"
    And the thread of task 1 holds a "preempted" system message for the owner
    When the host of "ai-one" resumes task 1
    Then the home answers ok to the task command
    And task 1 is "active" at step "local_check" with no wait
    And the home queued "nothing"
