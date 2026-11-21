Feature: Host permission levels and perm.set
  Scenario: Every AI defaults to permission level 3 without repository configuration
    Given a fresh host for permission levels
    Then effective permission for "claude" in the project is level 3 with repo_config "false"
    And effective permission for "codex" in the project is level 3 with repo_config "false"
    And a "task" session admitted at its effective level is offered the task submission tools

  Scenario Outline: Task submission tools depend on session kind and permission level
    Given a fresh host for permission levels
    When a "<kind>" session is admitted at level <level>
    Then the task submission tools are <offered> to that session

    Examples:
      | kind   | level | offered     |
      | task   | 1     | not offered |
      | task   | 2     | offered     |
      | task   | 3     | offered     |
      | group  | 3     | not offered |
      | review | 3     | not offered |

  Scenario Outline: perm.set stores the permission level, repo_config and an audit setting row
    Given a fresh host for permission levels
    When owner device "owner-browser" sends perm.set for "<agent>" with level <level> and repo_config "<repo>"
    Then perm.set succeeds
    And effective permission for "<agent>" in the project is level <level> with repo_config "<repo>"
    And the last audit row records a setting for "<agent>" by "owner-browser" with detail "level <level> repo_config <repo>"

    Examples:
      | agent  | level | repo  |
      | claude | 1     | false |
      | claude | 3     | true  |
      | codex  | 1     | false |
      | codex  | 2     | true  |
      | codex  | 3     | false |

  # Claude Code has no sandbox on Windows, where perm.set refuses level 2 with conflict.
  @not-windows
  Scenario: perm.set accepts level 2 for Claude Code on systems with a sandbox
    Given a fresh host for permission levels
    When owner device "owner-browser" sends perm.set for "claude" with level 2 and repo_config "true"
    Then perm.set succeeds
    And effective permission for "claude" in the project is level 2 with repo_config "true"

  Scenario: all_level3 overrides perm and default_level while preserving repo_config
    Given a fresh host for permission levels
    And host setting "default_level" is "2"
    When owner device "owner-browser" sends perm.set for "claude" with level 1 and repo_config "true"
    And host setting "all_level3" is "true"
    Then effective permission for "claude" in the project is level 3 with repo_config "true"
    And effective permission for "codex" in the project is level 3 with repo_config "false"
    When host setting "all_level3" is "false"
    Then effective permission for "claude" in the project is level 1 with repo_config "true"
    And effective permission for "codex" in the project is level 2 with repo_config "false"

  Scenario Outline: perm.set refuses invalid agent names and out-of-range levels
    Given a fresh host for permission levels
    When owner device "owner-browser" sends perm.set for "<agent>" with level <level> and repo_config "false"
    Then perm.set is refused with "bad_request"
    And effective permission for "claude" in the project is level 3 with repo_config "false"
    And the audit record holds 0 setting rows

    Examples:
      | agent  | level |
      | gemini | 3     |
      | claude | 0     |
      | claude | 4     |
