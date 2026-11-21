Feature: Detecting AI CLIs, sign-in status and declaring accounts
  Scenario: Found CLIs on PATH are reported with cleaned versions and cached for 60 seconds
    Given a host with fake CLIs "claude,codex" on PATH
    When the host reads its agents
    Then the host reports agents:
      | agent  | cli_version | account | login_ok |
      | claude | 2.1.311     | -       | true     |
      | codex  | 0.164.0     | -       | true     |
    When fake CLIs on PATH change to "" and 59 seconds pass
    And the host reads its agents
    Then the host reports 2 agents
    When 1 more second passes
    And the host reads its agents
    Then the host reports 0 agents

  Scenario Outline: Declaring an agent account checks the agent, account and PATH
    Given a host with fake CLIs "<installed>" on PATH
    When owner device "owner-1" sends agent.set for "<agent>" with account "<account>"
    Then agent.set ends with "<outcome>"
    And the setting audit rows are "<audit>"

    Examples:
      | installed    | agent  | account    | outcome                                   | audit                            |
      | claude,codex | claude | personal   | ok                                        | claude\|account personal\|owner-1 |
      | claude,codex | codex  | team       | ok                                        | codex\|account team\|owner-1      |
      | claude       | claude | api_key    | ok                                        | claude\|account api_key\|owner-1  |
      | claude       | codex  | personal   | conflict: codex is not found on this machine |                                  |
      | claude,codex | gemini | personal   | bad_request: unknown agent: gemini        |                                  |
      | claude,codex | claude | enterprise | bad_request: unknown account: enterprise  |                                  |

  Scenario: Sign-in status follows CLI events and agent.set without reading credentials
    Given a host with fake CLIs "claude,codex" on PATH
    And a group session on that host for "claude"
    When that session reports "login_invalid"
    And the host reads its agents
    Then the host reports agents:
      | agent  | cli_version | account | login_ok |
      | claude | 2.1.311     | -       | false    |
      | codex  | 0.164.0     | -       | true     |
    When that session ends a turn with status "failed"
    And the host reads its agents
    Then agent "claude" has login_ok "false"
    When that session ends a turn with status "completed"
    And the host reads its agents
    Then agent "claude" has login_ok "true"
    When that session reports "login_invalid"
    And owner device "owner-1" sends agent.set for "claude" with account "personal"
    And the host reads its agents
    Then the host reports agents:
      | agent  | cli_version | account  | login_ok |
      | claude | 2.1.311     | personal | true     |
      | codex  | 0.164.0     | -        | true     |
