Feature: Host ask and reply MCP tools
  Scenario: An AI asks another and receives the reply in its open tool call
    Given a host group with AIs "Claude Code" and "Codex" on task "T-12"
    When "Claude Code" calls ask to "Codex" with "Which event reports a path change?" and "Codex" replies "Use PathChanged."
    Then the open ask call returns answer "Use PathChanged."
    And the home received a call post at depth 1 and a reply post in task "T-12"

  Scenario: A busy callee leaves the caller with a receipt and a later turn reply
    Given a host group with AIs "Claude Code" and "Codex" on task "T-12" and ask wait 1 second
    When "Claude Code" calls ask to "Codex" with "Are the fixtures ready?" and "Codex" does not reply in time
    Then the ask call returns a pending receipt for the call
    And when "Codex" replies "Ready now." later the reply item is not taken by an open call

  Scenario: A call at depth 5 is refused while depth 4 is accepted
    Given a host group with AIs "Claude Code" and "Codex" on task "T-12"
    When "Claude Code" calls ask to "Codex" while answering a call of depth 3 without waiting
    Then the home received a call post at depth 4
    When "Claude Code" calls ask to "Codex" while answering a call of depth 4
    Then the ask call fails with "limit: call depth 5 exceeds ask_depth limit 4"

  Scenario: Outbound secrets in ask and reply are refused before signing
    Given a host group with AIs "Claude Code" and "Codex" on task "T-12"
    When "Claude Code" calls ask to "Codex" containing a GitHub personal access token
    Then the ask call fails with "secret: this looks like a secret; remove it and send again"
    And no command was sent to the home
