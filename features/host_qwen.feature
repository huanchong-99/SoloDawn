Feature: The host drives Qwen Code over qwen serve and resume
  Scenario: Two turns with declined then accepted command approval run in one Qwen Code process
    Given the scripted Qwen Code approval session is configured
    When the host starts Qwen Code and sends the first turn
    Then Qwen Code requests command approval and completes the declined turn
    When the host sends the follow-up turn on the same Qwen Code process and accepts the retry
    Then Qwen Code completes the second turn with usage and logs one process start and exit

  Scenario: A Qwen Code session that exits mid-turn resumes in a new process with a fresh MCP token
    Given the scripted Qwen Code mid-turn exit session is configured
    When the host starts Qwen Code and reads until the process exits mid-turn
    And the host resumes that Qwen Code session id with a new MCP token
    Then the resumed Qwen Code turn observes the new MCP token and completes

  Scenario: Interactive Qwen Code takeover is refused naming takeover
    When the host asks the Qwen Code driver for an interactive command
    Then the Qwen Code driver refuses the interactive start naming takeover

  Scenario: Qwen Code user questions round-trip on one turn
    Given a scripted Qwen Code session with ask_user_question requests
    When the host starts Qwen Code and begins the user-question turn
    Then Qwen Code asks user questions that the host answers and leaves unanswered
