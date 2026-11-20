Feature: Claude Code driver
  Scenario: Two turns run in one Claude Code process and reach the configured MCP shim
    Given a scripted Claude Code session with two turns that call "post"
    When the Claude driver starts a fresh session and sends both turns
    Then the Claude driver reports Ready once per turn and two completed TurnEnded events with usage
    And the MCP shim received both "post" calls with the start's token
    And closing the Claude process records one start-exit pair in the lifecycle log

  Scenario: A Claude Code process that exits mid-turn closes Events without TurnEnded and resumes cleanly
    Given a scripted Claude Code session that exits mid-turn before a result
    When the Claude driver starts a fresh session and sends a turn
    Then the Claude driver reports Ready and closes Events without TurnEnded
    When the Claude driver resumes that session id with a new token and sends the next turn
    Then the resumed Claude session completes the turn and calls the MCP shim with the new token
