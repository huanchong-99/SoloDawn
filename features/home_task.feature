Feature: Task commands in the home
  Working AIs start direct tasks; the assignee's host reports crashed or timed-out turns and
  pre-emption; the secretary and admins assign, pause, resume and cancel tasks through the task
  machine. One folder takes one task at a time: a task whose assignee's folder is busy waits as
  queued with wait busy.

  Scenario: A direct task creates a task card and notifies the secretary
    Given a group home for task commands
    When "ai-one" starts a direct task titled "Fix: relay notice" for the owner's request
    Then the home answers ok with task number 1
    And task 1 is "active" at step "work" with no wait
    And task 1 has a task card in the main line
    And the home queued "assign ai-one with request, notice secretary task_started"

  Scenario: Three consecutive failures stop the task and mention the owner; only a person recovers it
    Given a group home for task commands
    And task 1 is active for "ai-one" from a confirmed brief
    When the host of "ai-one" reports a "crash" failure on task 1
    And the host of "ai-one" sends the same failure command again
    Then both failure commands are answered ok and task 1 has 1 failure
    When the host of "ai-one" reports a "timeout" failure on task 1
    And the host of "ai-one" reports a "crash" failure on task 1
    Then task 1 is "failed" at step "work" with no wait
    And the thread of task 1 holds a "task_failed" system message mentioning the owner
    When the secretary tries to resume task 1
    Then the home refuses the task command with "forbidden"
    When the secretary tries to assign task 1 to "ai-two"
    Then the home refuses the task command with "forbidden"
    When the owner assigns task 1 to "ai-two"
    Then the home answers ok to the task command
    And task 1 is "active" at step "work" with no wait
    And task 1 has 0 failures and the home queued "assign ai-two"

  Scenario: Cancelling a task expires its pending approval, retires its items and tells the secretary of a dependent task
    Given a group home for task commands
    And task 1 is active for "ai-one" from a confirmed brief
    And task 1 has a pending approval, a queued assign item and queued dependent task 2
    When the secretary cancels task 1
    Then the home answers ok to the task command
    And task 1 is "cancelled" at step "work" with no wait
    And the pending approval of task 1 is "expired" and its queued item is retired
    And the home queued "notice secretary dependency_cancelled"
    When the owner tries to cancel task 1
    Then the home refuses the task command with "conflict"

  Scenario: A pre-empted task pauses with a system message and continues when its host resumes it
    Given a group home for task commands
    And task 1 is active for "ai-one" from a confirmed brief
    When the host of "ai-one" reports task 1 pre-empted by the owner's request
    Then task 1 is "paused" at step "work" waiting for "preempted"
    And the thread of task 1 holds a "preempted" system message for the owner
    When the host of "ai-one" resumes task 1
    Then the home answers ok to the task command
    And task 1 is "active" at step "work" with no wait
    And the home queued "nothing"

  Scenario: A task paused by hand records who paused it (A5.23)
    Given a group home for task commands
    And task 1 is active for "ai-one" from a confirmed brief
    When the secretary pauses task 1
    Then the home answers ok to the task command
    And task 1 is "paused" at step "work" waiting for "manual" set by the secretary
    When the owner resumes task 1
    Then task 1 is "active" at step "work" with no wait
    When the owner pauses task 1
    Then task 1 is "paused" at step "work" waiting for "manual" set by the owner
    When the host of "ai-one" reports task 1 pre-empted by the owner's request
    Then the home refuses the task command with "paused"

  Scenario: A second task for a busy folder waits and the release starts one queued task at a time (A5.20)
    Given a group home for task commands
    And task 1 is active for "ai-one" from a confirmed brief
    When "ai-one" starts a direct task titled "Second change" for the owner's request
    Then the home answers ok with task number 2
    And task 2 is "queued" at step "work" waiting for "busy"
    And the home queued "notice secretary task_started"
    When "ai-one" starts a direct task titled "Third change" for the owner's request
    Then the home answers ok with task number 3
    And task 3 is "queued" at step "work" waiting for "busy"
    When task 1 is done
    Then task 1 is "done" at step "review" with no wait
    And task 2 is "active" at step "work" with no wait
    And task 3 is "queued" at step "work" waiting for "busy"
    And the home queued "notice secretary task_done, assign ai-one with its task message"
    When the owner cancels task 2
    Then task 3 is "active" at step "work" with no wait
    And the home queued "assign ai-one with its task message"

  Scenario: Two AIs added with one folder on one computer take one task at a time (A5.20)
    Given a group home for task commands
    And "ai-two" and "ai-three" were added with one folder on one computer
    And task 1 is active for "ai-two" with "ai-one" as reviewer
    When "ai-three" starts a direct task titled "Same folder" for the owner's request
    Then task 2 is "queued" at step "work" waiting for "busy"
    When "ai-one" starts a direct task titled "Another folder" for the owner's request
    Then task 3 is "active" at step "work" with no wait
    When task 1 is done
    Then task 2 is "active" at step "work" with no wait
    And the home queued "notice secretary task_done, assign ai-three with its task message"

  Scenario: A started task goes only to an AI whose folder is free, and its old folder starts the task queued behind it (A5.20)
    Given a group home for task commands
    And task 1 is active for "ai-one" with "ai-two" as reviewer
    When "ai-two" starts a direct task titled "Other folder" for the owner's request
    Then task 2 is "active" at step "work" with no wait
    When "ai-one" starts a direct task titled "Queued change" for the owner's request
    Then task 3 is "queued" at step "work" waiting for "busy"
    When the owner tries to assign task 2 to "ai-one"
    Then the home refuses the task command with "conflict"
    And task 2 is "active" at step "work" with no wait
    When the owner assigns task 1 to "ai-three"
    Then the home answers ok to the task command
    And task 1 is "active" at step "work" with no wait
    And task 3 is "active" at step "work" with no wait
    And the home queued "assign ai-three, assign ai-one with its task message"
