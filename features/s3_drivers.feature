Feature: Step 3 driver acceptance (A10.1 to A10.4)
  OpenCode and Qwen use the step-1 session helpers with their HTTP fake CLIs.
  Level 2 and takeover are absent from both declarations, so A4.1, A4.2,
  A4.3, A4.6, A4.10 and A7.1 to A7.9 are switched off.
  The driver level tables remain unverified until real CLI acceptance runs.

  Scenario: Driver declarations preserve the step-1 declarations (A10.1)
    Then Drivers and DriverCaps have the same keys
    And the S3 declarations match the shipped OpenCode and Qwen contract

  Scenario Outline: Added CLI capability switches and API-key-only accounts (A10.1, A10.2)
    Given an S3 project with an API-key-only "<cli>" member
    Then the S3 capability exclusions hold for that member

    Examples:
      | cli      |
      | opencode |
      | qwen     |

  Scenario Outline: Named step-1 session cases run with each added CLI (A10.3)
    Given an S3 project with an API-key-only "<cli>" member
    Then the named step-1 case "<case>" runs for that S3 member

    Examples:
      | cli      | case       |
      | opencode | M3 tools   |
      | qwen     | M3 tools   |
      | opencode | A3.1/A3.2  |
      | qwen     | A3.1/A3.2  |
      | opencode | A3.3       |
      | qwen     | A3.3       |
      | opencode | A3.4       |
      | qwen     | A3.4       |
      | opencode | A3.5       |
      | qwen     | A3.5       |
      | opencode | A3.6       |
      | qwen     | A3.6       |
      | opencode | A3.7       |
      | qwen     | A3.7       |
      | opencode | A3.9       |
      | qwen     | A3.9       |
      | opencode | A3.10      |
      | qwen     | A3.10      |
      | opencode | A4.4       |
      | qwen     | A4.4       |
      | opencode | A4.5       |
      | qwen     | A4.5       |
      | opencode | A4.7       |
      | qwen     | A4.7       |
      | opencode | A4.8       |
      | qwen     | A4.8       |
      | opencode | A4.9       |
      | qwen     | A4.9       |

    # issues.md: "At level 1 a request of Codex, OpenCode or Qwen Code produces a card"
    @pending
    Examples:
      | cli      | case       |
      | opencode | A4.4 asked |
      | qwen     | A4.4 asked |

  @live
  Scenario: OpenCode repository configuration is isolated by session kind (A10.4, A3.8)
    Then the named live step-1 case A3.8 runs for "opencode"

  # issues.md: "A Qwen Code group session may load the repository's configuration"
  @live @pending
  Scenario: Qwen repository configuration is isolated by session kind (A10.4, A3.8)
    Then the named live step-1 case A3.8 runs for "qwen"
