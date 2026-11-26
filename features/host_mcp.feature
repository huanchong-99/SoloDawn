Feature: Host MCP server and stdio shim
  A CLI session reaches its node through one stdio MCP server named solodawn,
  forwarded over the node's local socket with the session token.

  Scenario: A registered tool is listed and called through the shim over the local socket
    Given a host with an active task session at level 2 and a local MCP socket
    And MCP tools "mcp_echo" for all sessions and "mcp_checks" for task sessions above level 1
    When an MCP client connects through the shim with the session's token
    Then the server lists "mcp_checks" and "mcp_echo" with object schemas
    And calling "mcp_echo" with "<ok> & ready" returns '{"caller":"Claude Code","echo":"<ok> & ready","open":1}'
    And no MCP tool call remains open on the session

  Scenario: A tool not for the caller's level is neither listed nor callable
    Given a host with an active task session at level 1 and a local MCP socket
    And MCP tools "mcp_echo" for all sessions and "mcp_checks" for task sessions above level 1
    When an MCP client connects through the shim with the session's token
    Then the server lists only "mcp_echo"
    And calling "mcp_checks" is refused by the MCP server

  Scenario: Tool errors carry their wire code and unexpected errors answer internal
    Given a host with an active task session at level 2 and a local MCP socket
    And MCP failure tools "mcp_conflict" and "mcp_crash"
    When an MCP client connects through the shim with the session's token
    Then calling "mcp_conflict" returns tool error "conflict: the task is not active"
    And calling "mcp_crash" returns tool error "internal: the node failed to run the tool; the cause is in its log"

  Scenario Outline: Tokens that do not name an active session are refused
    Given a host with an active task session at level 2 and a local MCP socket
    When the token state is "<state>"
    Then an MCP client connecting through the shim fails to initialise

    Examples:
      | state     |
      | unknown   |
      | forgotten |
      | ended     |

  Scenario Outline: Invalid shim arguments print usage and exit with code 2
    When the MCP shim is invoked with "<args>"
    Then the shim exits with code 2 and prints "usage: solodawn <command> [arguments]" on stderr

    Examples:
      | args                  |
      |                       |
      | --token               |
      | --token tok extra-arg |
      | --unknown value       |
