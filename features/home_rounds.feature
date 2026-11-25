Feature: Rounds in threads, queue jumps and the quiet-task timer
  AI-to-AI exchanges in a task thread count rounds up to the group's limit, where the task pauses
  with wait "rounds", a "rounds_limit" system message is written in the main line, the secretary is
  notified, and the host holds thread mentions while the task is paused until a person's message in
  the thread or an admin's "task resume" ends the wait. "jump" sends every unacknowledged
  mention item that the named message of that person produced again at once, as a jump under its
  number. The "quiet" timer notifies the secretary once per quiet period for an active
  task with no wait before step "merge" that has had no thread message for "idle_notice_minutes".

  Scenario: Ten AI rounds pause the task until a person posts in the thread
    Given a rounds home with a secretary, two working AIs and an active task for "ai-one"
    When "ai-one" and "ai-two" exchange 10 thread mentions
    Then the rounds task is "paused" waiting for "rounds" at 10 rounds
    And the main line of the rounds home holds a "rounds_limit" system message for the task
    And the rounds host has received the 10th mention with the task "paused" and "notice secretary rounds_limit"
    When the owner posts "Keep the direct path simple." in the rounds task thread
    Then the rounds task is "active" waiting for "nothing" at 0 rounds

  Scenario: Task resume by an admin activates a rounds-paused task while the secretary is refused
    Given a rounds home with a secretary, two working AIs and an active task for "ai-one"
    And "ai-one" and "ai-two" have exchanged 10 thread mentions
    When the secretary tries to resume the rounds task
    Then the rounds home refuses with "forbidden"
    And the rounds task is "paused" waiting for "rounds" at 10 rounds
    When the owner resumes the rounds task
    Then the rounds task is "active" waiting for "nothing" at 0 rounds
    And the rounds host receives "resume ai-one"

  Scenario: Jump the queue on a direct request and on a running task thread
    Given a rounds home with a secretary, two working AIs and an active task for "ai-one"
    When the owner posts a direct request mentioning "ai-one" in the main line
    And the owner posts a newer message mentioning "ai-one" in the main line
    And the owner jumps the queue on that direct request
    Then the rounds host receives the first mention item again as a jump mention for "ai-one"
    When the host of "ai-one" reports the rounds task pre-empted by that direct request and then resumes it
    Then the rounds task was "paused" waiting for "preempted" meanwhile and is "active" waiting for "nothing" afterwards
    When the owner posts in the rounds task thread mentioning "ai-one" and jumps the queue on that thread message
    Then the rounds task is "active" waiting for "nothing" at 0 rounds
    And the rounds host receives a jump mention for "ai-one" on the task

  Scenario: An active task silent for idle_notice_minutes notifies the secretary once per quiet period
    Given a rounds home with idle_notice_minutes 1 and an active task with no thread message
    When 59 seconds pass on the rounds home and it ticks
    Then the rounds host has nothing new
    When 1 more second passes on the rounds home and it ticks
    Then the rounds host receives "notice secretary quiet"
    When the rounds host acknowledges that quiet notice and 5 minutes pass with ticks
    Then the rounds host has nothing new
    When "ai-one" posts progress in the rounds task thread and 1 minute passes with a tick
    Then the rounds host receives "notice secretary quiet"

  Scenario Outline: Jump refuses invalid principals, missing messages and empty queues
    Given a rounds home with a secretary, two working AIs and an active task for "ai-one"
    And the owner has posted a direct request mentioning "ai-one" whose item was <queue_state>
    When <actor> sends jump on <target>
    Then the rounds home refuses with "<code>"

    Examples:
      | queue_state    | actor          | target             | code      |
      | unacknowledged | "ai-one"       | the direct request | forbidden |
      | unacknowledged | a guest member | the direct request | forbidden |
      | unacknowledged | the owner      | a guest's message  | forbidden |
      | unacknowledged | the owner      | a missing message  | not_found |
      | acknowledged   | the owner      | the direct request | conflict  |
