Feature: Driver capability table and switches
  Every CLI driver declares its capability table, and the host switches
  agent.set accounts, the MCP approve tool, and terminal takeover from that
  declaration without naming individual CLIs outside the drivers.

  Scenario: Every driver declares the capability table and Claude Code and Codex match section 8 (A10.1)
    Then every registered host driver declares a valid capability table
    And the Claude Code and Codex capability declarations match section 8 of the step 3 driver contract

  Scenario Outline: Synthetic capability declarations switch agent.set and the approve tool (A10.1)
    Given a synthetic CLI "synth_cli" with accounts "<accounts>", approvals "<approvals>", and takeover "<takeover>"
    When on the capability host, owner device "owner-1" sends agent.set for "synth_cli" with account "<try_account>"
    Then the capability host agent.set ends with "<set_outcome>"
    And MCP tool "approve" for "synth_cli" is "<struct_approve>" in a structured session and "<inter_approve>" in a terminal session

    Examples:
      | accounts      | approvals | takeover | try_account | set_outcome                                               | struct_approve | inter_approve |
      | personal,team | tool      | new      | personal    | ok                                                        | listed         | omitted       |
      | personal,team | tool      | new      | api_key     | bad_request: account api_key is not in synth_cli accounts | listed         | omitted       |
      | team          | conn      | resume   | team        | ok                                                        | omitted        | omitted       |
      | api_key       | conn      | none     | personal    | bad_request: account personal is not in synth_cli accounts | omitted       | omitted       |

  Scenario: With a restricted synthetic capability declaration A3.1 still runs two mentions in one CLI process (A10.1)
    Given a capability test node where "codex" has a synthetic API-key-only declaration without takeover
    When on the capability test node, configuring AI "Restricted AI" on "codex" refuses personal naming accounts and accepts api_key
    And on the capability test node, "Restricted AI" is scripted on thread "01a1f63f-701c-78c1-bbf3-f1ac61c56b91" to reply "Remembered key-42." and "First mention said key-42."
    Then on the capability test node, mentioning "Restricted AI" twice in "Website" gets "Remembered key-42." then "First mention said key-42." in 1 "codex" process on session "01a1f63f-701c-78c1-bbf3-f1ac61c56b91"
    And structured sessions on the capability test node for "codex" omit MCP tool "approve" and takeover.start refuses naming takeover

  Scenario Outline: An API-key-only CLI accepts api_key on agent.set and refuses other account types (A10.2)
    Given a synthetic CLI "api_only_cli" with accounts "api_key", approvals "conn", and takeover "none"
    When on the capability host, owner device "owner-1" sends agent.set for "api_only_cli" with account "<account>"
    Then the capability host agent.set ends with "<outcome>"

    Examples:
      | account  | outcome                                                       |
      | api_key  | ok                                                            |
      | personal | bad_request: account personal is not in api_only_cli accounts |
      | team     | bad_request: account team is not in api_only_cli accounts     |
