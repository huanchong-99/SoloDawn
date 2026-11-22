Feature: AI-to-AI calls and answers in the home
  One AI asks another through a call post; the home stores the call as waiting and queues a mention
  item for the callee. When the callee answers with a reply post, even after the caller's wait has
  elapsed, the home marks the call answered with the reply id and elapsed milliseconds, stores the
  reply, and queues a reply item for the caller. A call whose depth exceeds limits.ask_depth is
  refused with limit, a call is answered at most once, and repeated commands receive their receipts.

  Scenario: A busy callee replies after the wait and the home wakes the caller
    Given a group home for calls with an active task and three AIs
    When "ai-one" calls "ai-two" in the task thread at depth 1 asking "Which event reports a path change?"
    Then the call home answers ok
    And the call is "waiting" in the task thread and queued "mention ai-two"
    When "ai-two" replies to the call 48000 ms later with "Use the path change event."
    Then the call home answers ok
    And the call is "answered" after 48000 ms with the reply in the task thread
    And the call home has queued "reply ai-one" for the reply

  Scenario: Repeated call and reply return receipts while a second answer is refused
    Given a group home for calls with an active task and three AIs
    When "ai-one" calls "ai-two" in the task thread at depth 1 asking "Is the branch ready?"
    And "ai-one" sends the same call command again
    Then the call home answers ok to both
    And the call is "waiting" in the task thread and queued "mention ai-two"
    When "ai-two" replies to the call 1200 ms later with "Yes, checked."
    And "ai-two" sends the same reply command again
    Then the call home answers ok to both
    And the call home has queued "reply ai-one" for the reply
    When "ai-two" replies to the call 2000 ms later with "Second answer."
    Then the call home answers "conflict"

  Scenario: Depth 4 is accepted and depth 5 is refused with limit
    Given a group home for calls with an active task and three AIs
    When "ai-one" calls "ai-two" in the task thread at depth 4 asking "Fourth hop?"
    Then the call home answers ok
    And the call is "waiting" in the task thread and queued "mention ai-two"
    When "ai-two" calls "ai-three" in the task thread at depth 5 asking "Fifth hop?"
    Then the call home answers "limit"
    And the call home has queued nothing since the last check

  Scenario Outline: Calls and replies refuse invalid principals and targets
    Given a group home for calls with an active task and three AIs
    And "ai-one" has called "ai-two" in the main line
    When <attempt>
    Then the call home answers "<code>"
    And the call home has queued nothing since the last check

    Examples:
      | attempt                                      | code        |
      | the owner posts a call to "ai-two"           | forbidden   |
      | "ai-one" posts a call to "ai-one"            | bad_request |
      | "ai-one" posts a call to "owner"             | conflict    |
      | "ai-three" replies to the main-line call     | conflict    |
      | "ai-two" replies to the call in the thread   | bad_request |
