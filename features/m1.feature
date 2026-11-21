Feature: M1 real-transport identity, connectivity and group-list push
  Browsers and nodes pair over real rendezvous rooms and WebRTC data channels,
  enforce bootstrap and envelope rules, fall back to TURN when UDP and ICE-TCP
  are blocked, deduplicate replayed offers and trickled candidates, and push
  group list changes to connected owner devices.

  Scenario: Onboarding pairing, status fingerprint, bootstrap rules and adding a second node (A1.1, A1.2)
    Given an M1 rendezvous with TURN relay
    And a fresh M1 node process "workstation" and a fresh M1 browser
    When the M1 browser starts onboarding pairing with "workstation"
    Then the M1 room pair fingerprint equals "solodawn status" on "workstation"
    When the M1 browser opens ctl to "workstation" and sends hello before any device log
    Then the M1 pairing welcome reports owner false and refuses host.info and commands with "forbidden"
    When the M1 browser sends a device log whose add entry for "workstation" was signed by another device
    Then "workstation" still has no owner and refuses host.info with "forbidden"
    When the M1 browser sends the valid device log adding "workstation" and sends hello again
    Then "solodawn pair" on "workstation" exits 0 with its fingerprint and welcome reports owner true
    When a second fresh M1 node process "laptop" pairs with the M1 browser
    Then the M1 browser lists both nodes with system, version and active path

  Scenario: Relay fallback within 1 second and host.info answered every second when UDP and ICE-TCP are blocked (A1.3)
    Given an M1 owned node and browser on a relay-only path with UDP and ICE-TCP blocked
    When the M1 browser connects over ctl and sends hello
    Then both M1 ends report path "relay" within 1 second
    And an M1 host.info request sent every second for 3 seconds is answered throughout

  Scenario: A rendezvous that alters the SDP description in transit causes the connection to be refused (A1.5)
    Given an M1 owned node and browser signed in on an M1 rendezvous
    When the M1 rendezvous alters SDP descriptions in transit and the browser offers a connection
    Then the M1 offer is refused before ctl opens and the node accepts no session
    When the M1 rendezvous forwards descriptions unchanged and the browser offers a connection
    Then the M1 connection opens ctl and answers host.info

  Scenario: pair.browser with the same nonce returns the same link while valid and conflict when used or expired (A1.9)
    Given an M1 owned node with a connected owner browser for browser pairing
    When the M1 browser sends pair.browser with nonce "link-one" twice while valid
    Then both M1 pair.browser answers return the same link and expiry
    When a second M1 browser pairs through that link
    Then the second M1 browser adopts the owner, reads owner true and answers host.info
    When the M1 browser repeats pair.browser with nonce "link-one" after use
    Then the M1 pair.browser answer is refused with "conflict"
    When the M1 browser sends pair.browser with nonce "link-two", 10 minutes pass and it repeats "link-two"
    Then the M1 pair.browser answer is refused with "conflict"

  Scenario: Replayed retired offers close nothing while current-offer copies answer per path through ts + 120000 (A1.10)
    Given an M1 owned node and browser signed in on two M1 signalling paths
    When the M1 browser opens a first connection and replaces it with a second connection
    And the M1 node receives the retired first offer again
    Then the M1 node sends no answer, opens no extra connection and answers host.info on the live connection
    When the M1 node receives copies of the live offer on two other paths
    Then both M1 replay paths receive the same answer and the live connection answers host.info
    When the M1 browser opens a connection dated 119 seconds ahead, replaces it and replays the future-dated offer
    Then the M1 node sends no answer, remembers it through ts + 120000 and answers host.info on the live connection
    When the M1 browser opens a connection near expiry, replaces it and replays the retired offer across ts + 120000
    Then the M1 node opens no extra connection and the live connection keeps answering host.info

  Scenario: Early and duplicate candidates are held before descriptions and applied once afterward (A1.11)
    Given an M1 owned node and browser whose signalling the test delivers over a relay-only path
    When the M1 browser offers a connection with two copies of each side's relay candidate delivered before its description
    Then the M1 connection opens ctl and answers host.info
    When another copy of each M1 early relay candidate is delivered
    Then the M1 connection still answers host.info

  Scenario: Creating a group pushes groups to an already-connected second browser within 1 second (A2.1)
    Given an M1 owned node with two already-connected browsers of its owner
    When the first M1 browser creates group "Website"
    Then the second M1 browser receives a groups push listing "Website" within 1 second without reconnecting
    When the second M1 browser syncs "Website" and the first M1 browser posts "hello from browser one"
    Then the second M1 browser receives that message row within 1 second
