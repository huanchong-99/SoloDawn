Feature: The host drives Codex over app-server and resume
  Scenario: Two turns with declined then accepted command approval run in one Codex process
    Given the scripted Codex approval session is configured at level 2
    When the host starts Codex and sends the first turn
    Then Codex requests command approval and ends the declined turn with 35980 tokens
    When the host sends the follow-up turn on the same Codex process and accepts the retry
    Then Codex ends the second turn with 41562 tokens and logs one process start and exit

  Scenario: A Codex session that exits mid-turn resumes in a new process with a fresh MCP token
    Given the scripted Codex mid-turn exit session is configured at level 2
    When the host starts Codex and reads until the process exits mid-turn
    And the host resumes that Codex thread id with a new MCP token
    Then the resumed Codex turn observes the new MCP token and ends with 67311 tokens

  Scenario: Interactive Codex takeover requires a thread id and passes the level settings
    When the host asks the Codex driver for an interactive command without a thread id
    Then the Codex driver refuses the interactive start
    When the host asks the Codex driver for a level 2 interactive command on thread "thr-42"
    Then the interactive Codex command resumes "thr-42" with workspace-write and project docs disabled
