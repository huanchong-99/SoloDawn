Feature: Host interactive pseudo-terminals
  Scenario: An interactive terminal bridges bytes both ways and cleans up when the channel closes
    Given an interactive PTY host with a session folder
    When the host starts an interactive PTY session for group "rZNvy-1jH6Z-BcPqA5U5BQ" and member "1PC8WineBrUQ-apCjx7tug"
    And the owner opens terminal channel "pty:rZNvy-1jH6Z-BcPqA5U5BQ:1PC8WineBrUQ-apCjx7tug"
    And the owner writes "roundtrip-check" to the terminal channel
    Then the terminal channel receives "WD:session-folder" and "ACK:roundtrip-check"
    When the owner closes the terminal channel
    Then the interactive PTY exits, removes its temporary MCP config and notifies completion once

  Scenario: Back-pressure on the terminal stream stops reading from the pseudo-terminal
    Given an interactive PTY host with a session folder
    When the host starts a flooding interactive PTY session for group "rZNvy-1jH6Z-BcPqA5U5BQ" and member "1PC8WineBrUQ-apCjx7tug"
    And the owner opens a back-pressured terminal channel "pty:rZNvy-1jH6Z-BcPqA5U5BQ:1PC8WineBrUQ-apCjx7tug"
    Then pseudo-terminal reads pause while the terminal write buffer is full
    When the terminal write buffer drains
    Then pseudo-terminal reads resume and closing the PTY ends the stream

  Scenario Outline: pty.resize validates terminal dimensions and resizes open sessions
    Given an interactive PTY host with a session folder
    When the owner sends pty.resize for group "<group>" and AI "<ai>" with cols <cols> and rows <rows> as type "<type>"
    Then the pty.resize outcome is "<outcome>"

    Examples:
      | type       | group                  | ai                     | cols | rows | outcome     |
      | pty.resize | rZNvy-1jH6Z-BcPqA5U5BQ | 1PC8WineBrUQ-apCjx7tug | 120  | 32   | ok          |
      | pty.resize | rZNvy-1jH6Z-BcPqA5U5BQ | 1PC8WineBrUQ-apCjx7tug | 1    | 1    | ok          |
      | pty.resize | rZNvy-1jH6Z-BcPqA5U5BQ | 1PC8WineBrUQ-apCjx7tug | 0    | 32   | bad_request |
      | pty.resize | rZNvy-1jH6Z-BcPqA5U5BQ | 1PC8WineBrUQ-apCjx7tug | 120  | 0    | bad_request |
      | other      | rZNvy-1jH6Z-BcPqA5U5BQ | 1PC8WineBrUQ-apCjx7tug | 120  | 32   | bad_request |

  Scenario: An interactive process exiting on its own closes the bridged terminal channel
    Given an interactive PTY host with a session folder
    When the host starts a self-exiting interactive PTY session for group "rZNvy-1jH6Z-BcPqA5U5BQ" and member "1PC8WineBrUQ-apCjx7tug"
    And the owner opens terminal channel "pty:rZNvy-1jH6Z-BcPqA5U5BQ:1PC8WineBrUQ-apCjx7tug"
    Then the terminal channel closes and completion is notified once

  Scenario: Unmatched or malformed terminal channels are closed and drained
    Given an interactive PTY host with a session folder
    When an unmatched terminal channel "pty:rZNvy-1jH6Z-BcPqA5U5BQ:1PC8WineBrUQ-apCjx7tug" and a malformed channel "pty:bad" arrive with unread bytes
    Then both terminal channels are closed and drained
