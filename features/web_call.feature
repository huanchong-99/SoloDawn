@web
Feature: AI-to-AI call block
  An AI-to-AI call block shows who calls whom, whether the caller waits on the spot or gets a
  receipt first, the call depth, the caller's question with mentions and code spans, the waiting
  indicator with time since the call was made while unanswered, and the elapsed duration with the
  callee's reply folded inside the block once answered, in both threads and the main line.

  Background:
    Given a group "Website" with "Claude Code", "Codex" and task 12 "Show connection path"

  Scenario: A thread call transitions from waiting to answered with elapsed time and inline reply
    Given "Claude Code" calls "Codex" in task 12 waiting on the spot at depth 1 made 12 seconds ago
    When the thread of task 12 is mounted
    Then the call row has the sky "Call" badge and shows "Claude Code → Codex"
    And the call block shows "Waits for answer", waiting status "Waiting for reply · 12 s", "Depth 1" and no reply
    And the call question appears once inside the call block with mention "@Codex" and code "path"
    When 5 seconds pass
    Then the call block counts up to "Waiting for reply · 17 s"
    When "Codex" replies to the call after 38 seconds
    Then the call block shows "Took 38 s" and folds the reply of "Codex" with code "limited" inside it
    And the thread has one item and no separate reply row
    And switching the language to Chinese words the call metadata as "当场等结果 · 用时 38 秒 · 调用第 1 层"

  Scenario: An unanswered receipt-first call stays waiting with live elapsed time, and main-line calls fold replies
    Given an unanswered receipt-first call at depth 2 in the main line made 6 minutes ago
    When the reader opens the main line of "Website"
    Then the main-line call shows "Gets a receipt", waiting status "Waiting for reply · 6 min", "Depth 2" and no dawn highlight
    When 3 minutes pass and a second main-line call is answered in 2 seconds
    Then the unanswered call shows "Waiting for reply · 9 min" and the answered main-line call folds its reply with "Took 2 s"

  Scenario: The states sheet mounts call under the call area and renders all call wire examples
    When the reader opens "#/states" in a development server
    Then the states sheet marks "call" as mounted by "call"
    And the sheet renders waiting and denied calls and the answered call with the reply "Use the path change event."

  Scenario: An answered call keeps the reply after its author leaves
    Given a main-line call whose reply is from a former member
    When the reader opens the main line of "Website"
    Then the reply text and the former member's avatar remain visible
