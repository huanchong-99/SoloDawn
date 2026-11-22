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
    And an unowned pairing node on that rendezvous
    When a fresh browser creates a pairing secret and joins its room
    And the pairing node runs pair with that secret and label "workstation"
    Then the node fingerprint seen in the room equals the node key fingerprint
    When the pairing browser opens ctl and sends hello before any device log
    Then the pairing welcome reports owner false
    And host.info before enrolment is refused with "forbidden"
    And cmd before enrolment is refused with "forbidden"
    When the pairing browser sends a log whose add entry for the node was signed by another device
    Then the pairing node still has no owner and refuses host.info with "forbidden"
    When the pairing browser sends a valid device log adding the node signed by the pairing browser
    Then pair completes with the node fingerprint and active path
    When the pairing browser sends hello again
    Then the pairing welcome reports owner true
    And host.info succeeds for the owner browser
    When two foreground nodes running in separate home directories on that rendezvous
    And a fresh browser pairs the first node as "alpha" and adds the second node as "beta"
    Then the browser lists both nodes with their names, system, version and active path
    When two authorized nodes signed in on that rendezvous
    And each node offers the other a connection
    Then both directions are direct and answer host.info
    When a relay on this machine, with UDP and ICE-TCP between the nodes blocked
    And the node offers the tablet a connection
    Then both ends report the path relay and the connection answers host.info
    And host.info on that connection succeeds every second
