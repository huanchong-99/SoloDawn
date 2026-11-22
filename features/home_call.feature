Feature: AI-to-AI calls and answers in the home
  One AI asks another through a call post; the home stores the call as waiting and queues a mention
  item for the callee. When the callee answers with a reply post, even after the caller's wait has
  elapsed, the home marks the call answered with the reply id and elapsed milliseconds, stores the
  reply, and queues a reply item for the caller. A call whose depth exceeds limits.ask_depth is
  refused with limit.

  Scenario: A busy callee replies after the wait and the home wakes the caller
    Given a group home for calls with an active task and three AIs
    When "ai-one" calls "ai-two" in the task thread at depth 1 asking "Which event reports a path change?"
    Then the call home answers ok
    And the call is "waiting" in the task thread and queued "mention ai-two"
    When "ai-two" replies to the call 48000 ms later with "Use the path change event."
    Then the call home answers ok
    And the call is "answered" after 48000 ms with the reply in the task thread
    And the call home has queued "reply ai-one" for the reply

  Scenario: Depth 4 is accepted and depth 5 is refused with limit
    Given a group home for calls with an active task and three AIs
    When "ai-one" calls "ai-two" in the task thread at depth 4 asking "Fourth hop?"
    Then the call home answers ok
    And the call is "waiting" in the task thread and queued "mention ai-two"
    When "ai-two" calls "ai-three" in the task thread at depth 5 asking "Fifth hop?"
    Then the call home answers "limit"
    And the call home has queued nothing since the last check
