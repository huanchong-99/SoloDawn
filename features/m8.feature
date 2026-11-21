Feature: M8 shipping: clean installation, reboot persistence, and standalone rendezvous
  Installing a release binary after verifying SHA256SUMS, pairing with a fresh
  browser through solodawn pair, and restarting the node preserves its device
  identity, owner enrolment, and status. A standalone rendezvous started with
  one command serves onboarding pairing, multi-node listing, direct paths, and
  relay fallback.

  Scenario: Clean install verifies SHA256SUMS, pairs, registers login startup, and persists across restart (A8.1)
    Given a clean user home with release "v1.0.0" and a local rendezvous
    When the install script rejects a mismatching SHA256SUMS and installs "v1.0.0" once SHA256SUMS matches
    And a fresh browser pairs with the installed node via "solodawn pair" as "workstation"
    Then the current user has a login service registration and status matches the browser fingerprint
    When the installed node stops and starts again as after a reboot
    Then the restarted node keeps its fingerprint, owner enrolment, and connected status

  Scenario: Standalone rendezvous started with one command passes A1.1 to A1.3 (A8.4)
    Given a fresh server running "solodawn rendezvous" with TLS, STUN, and TURN in one command
    When a fresh browser and first node "workstation" pair over that rendezvous checking bootstrap refusals
    Then the room fingerprint matches "solodawn status" and the first node connects on path "direct"
    When a second node "laptop" pairs with the browser over that rendezvous
    Then the browser lists both nodes with system, version, and path "direct"
    When UDP and ICE-TCP between the browser and the first node are blocked
    Then the reconnected path is "relay" within 1 second and "host.info" succeeds every second
