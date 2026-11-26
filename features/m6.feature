Feature: Cross-node AI sessions and AI-to-AI calls
  An AI member hosted on a node other than the group home receives mentions
  over ctl, answers with post, keeps its session memory across mentions, and
  runs in one CLI process throughout, in the folder it was added with
  (step-1-acceptance.md A3.3). One AI can
  also ask another AI on the same machine or across two machines over ctl:
  an answer in time returns to the caller's open tool call with both messages
  and elapsed time recorded in the thread, a busy callee yields a receipt and
  a later reply turn, and a call at depth 5 is refused with limit (A6.1-A6.4).
  After each scenario its one-person group shows no confirm card (A9.40).

  Scenario Outline: A3.3 Mentioning a <cli> member on a node other than the home
    Given two owned nodes over ctl and a group on the first node
    And "<agent>" on the second node is scripted to answer "<first_mention>" with "<first_answer>" and "<second_mention>" with "<second_answer>" as "<member>"
    And the owner joins "<agent>" on the second node to the group as "<member>"
    When the owner mentions "<member>" in the group with "<first_mention>"
    Then "<member>" posts "<first_answer>" from the second node and enters waiting
    When the owner mentions "<member>" in the group with "<second_mention>"
    Then "<member>" posts "<second_answer>" from the second node and enters waiting
    And one "<agent>" CLI process existed throughout on the second node
    And the second node runs the sessions of "<member>" in the folder it was added with

    Examples:
      | cli         | agent  | member | first_mention               | first_answer              | second_mention                        | second_answer                 |
      | Claude Code | claude | Claude | Remember passphrase oak-42. | Stored passphrase oak-42. | What passphrase did you store earlier | You asked me to store oak-42. |
      | Codex       | codex  | Codex  | Remember passphrase ash-84. | Stored passphrase ash-84. | What passphrase did you store earlier | You asked me to store ash-84. |

  Scenario: A6.1 On one machine one AI asks another and receives the answer in its open tool call
    Given one M6 node over ctl with a group and joined AIs "Claude" and "Codex"
    When "Claude" on a task asks "Codex" "Which event reports path changes?" and "Codex" replies "Use PathChanged." while the call is open
    Then "Claude" receives "Use PathChanged." in its running turn, posts "Confirmed in turn: Use PathChanged.", and started only the group session and one task session process
    And the M6 task thread holds the answered call and reply with elapsed milliseconds

  Scenario: A6.2 Across two machines one AI asks another over ctl and receives the answer in its open tool call
    Given two owned nodes over ctl and a group on the first node
    And the owner joins "claude" on the first node as "Claude" and "codex" on the second node as "Codex"
    When "Claude" on a task asks "Codex" "Which event reports path changes?" and "Codex" replies "Use PathChanged." while the call is open
    Then "Claude" receives "Use PathChanged." in its running turn, posts "Confirmed in turn: Use PathChanged.", and started only the group session and one task session process
    And the M6 task thread holds the answered call and reply with elapsed milliseconds

  Scenario: A6.3 With a busy callee the caller receives a receipt after the wait and the answer later as a turn
    Given one M6 node over ctl with a group, ask_wait_s 1, and joined AIs "Claude" and "Codex"
    When "Codex" is busy on another task and "Claude" on a task asks "Codex" "Are the fixtures ready?"
    Then after the wait "Claude" receives a pending receipt, posts "Waiting on receipt.", and enters waiting while the call stays waiting
    When busy "Codex" finishes its turn and replies "Fixtures are ready." to the call
    Then "Claude" receives the reply as a later turn, posts "Late answer: Fixtures are ready.", and started only the group session and one task session process
    And the M6 task thread holds the answered call and reply with elapsed milliseconds at least 950

  Scenario: A6.4 A call at depth 5 is refused with limit
    Given one M6 node over ctl with a group and joined AIs "Claude" and "Codex"
    When "Claude" and "Codex" chain 4 calls in the M6 group and "Claude" attempts a 5th call
    Then the 5th ask call fails with "limit: call depth 5 exceeds the ask_depth limit of 4" and "Claude" posts "Depth 5 refused with limit."
    And the M6 group holds 4 call messages at depths 1 to 4 and none at depth 5
