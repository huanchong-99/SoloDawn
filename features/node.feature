Feature: Node
  The node assembles the SoloDawn runtime: data directory, single-instance
  local socket, embedded local web client and /rv signalling handler, MCP
  shim, room pairing, host.info, and multi-node fallback when default ports
  are already bound.

  Scenario: A development build reports its version
    When I run "solodawn version"
    Then it prints "dev" on stdout and exits with code 0

  Scenario: The standalone service refuses a shared signalling and relay name
    When I run "solodawn rendezvous --domain localhost --turn-domain localhost"
    Then it prints "relay needs a distinct --turn-domain" on stderr and exits with code 1

  Scenario: Two nodes start on one machine and answer status on their local sockets
    Given two foreground nodes running in separate home directories on this machine
    When the owner runs "solodawn status" in each node's environment
    Then each status prints its node's version and distinct device fingerprint

  Scenario: A second node is added and the browser lists both with system, version and path
    Given two foreground nodes running in separate home directories and a shared rendezvous
    When a fresh browser pairs the first node as "alpha" and adds the second node as "beta"
    Then the browser lists both nodes with their names, system, version and active path
