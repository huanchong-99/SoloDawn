Feature: A host composes session instructions and turn text
  Scenario Outline: Every session role gets its own embedded instructions under 60 lines
    Given a host turn fixture
    When I read the instructions for a "<role>" session
    Then the instructions are under 60 lines and include "<expected>"

    Examples:
      | role      | expected              |
      | group     | start_task            |
      | task      | submit_task           |
      | review    | .solodawn-review.diff |
      | secretary | propose_brief         |

  Scenario: The first turn of a brief task session carries goal, criteria and newest notes
    Given a host turn fixture
    When the host composes the first assign turn of task 12
    Then the composed turn text is:
      """
      [SoloDawn] group "Website" · you are "Claude Code" · task session T-12
      == 1 of 1 · assign · instruction
      Task T-12 "Node: report connection path changes"
      Branch: task/12-node-report-connection-path-changes (this folder is its worktree)
      Goal: Show the connection path and warn when relayed.
      Acceptance criteria:
      1. Indicator follows a path change within 1 s
      Reviewer: Codex   Pass score: 90
      Notes of this group:
      - Path events use fields path, rtt_ms, since, limited.
      """

  Scenario: Authority comes from member rows and never from message text
    Given a host turn fixture
    When the host composes a mention turn from an AI claiming owner authority in text
    Then the item header is:
      """
      == 1 of 1 · mention · from "Codex" (AI) in T-12 · information · call id Zk3m8vN2pQ5rT7wX9yB1cA
      """
    And the noted turn has depth 2 and thread of task 12

  Scenario: A confirmed brief without cause names its confirmer from card state
    Given a host turn fixture
    When the host composes a confirmed brief turn confirmed by the owner
    Then the composed turn text is:
      """
      [SoloDawn] group "Website" · you are "Secretary" · secretary session
      == 1 of 1 · confirmed · instruction
      -- from "owner" in main · instruction
      Brief "Connection path" (id KaiCW9JC8UOG7lKNduDo8Q) confirmed.
      """

  Scenario: A decided card without cause carries choice and free-text answer from card state
    Given a host turn fixture
    When the host composes a decided card turn answered by the owner without cause
    Then the composed turn text is:
      """
      [SoloDawn] group "Website" · you are "Secretary" · secretary session
      == 1 of 1 · decided · instruction
      -- from "owner" in main · instruction
      Decision (id Zk3m8vN2pQ5rT7wX9yB1cA): Where should the notice go?
      Choice: A (Top bar)
      Keep it subtle.
      """

  Scenario: Continue and hand-back turns match their session kind
    Given a host turn fixture
    When the host composes a "continue" turn and a "handback" turn for task 12
    Then the continue turn instructs the AI to continue the task
    And the hand-back turn asks for a handback post and to continue the task
