Feature: Cross-node AI sessions
  An AI member hosted on a node other than the group home receives mentions
  over ctl, answers with post, keeps its session memory across mentions, and
  runs in one CLI process throughout (step-1-acceptance.md A3.3).

  Scenario Outline: A3.3 Mentioning a <cli> member on a node other than the home
    Given two owned nodes over ctl and a project group on the first node
    And "<agent>" on the second node is scripted to answer "<first_mention>" with "<first_answer>" and "<second_mention>" with "<second_answer>" as "<member>"
    And the owner joins "<agent>" on the second node to the group as "<member>"
    When the owner mentions "<member>" in the group with "<first_mention>"
    Then "<member>" posts "<first_answer>" from the second node and enters waiting
    When the owner mentions "<member>" in the group with "<second_mention>"
    Then "<member>" posts "<second_answer>" from the second node and enters waiting
    And one "<agent>" CLI process existed throughout on the second node

    Examples:
      | cli         | agent  | member | first_mention               | first_answer              | second_mention                        | second_answer                 |
      | Claude Code | claude | Claude | Remember passphrase oak-42. | Stored passphrase oak-42. | What passphrase did you store earlier | You asked me to store oak-42. |
      | Codex       | codex  | Codex  | Remember passphrase ash-84. | Stored passphrase ash-84. | What passphrase did you store earlier | You asked me to store ash-84. |
