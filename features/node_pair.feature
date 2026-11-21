Feature: Room pairing, pair.browser and the host surface
  A fresh browser and an unowned node pair through a rendezvous room, and an
  owner browser adds another browser with pair.browser under once-only receipts.

  Scenario: Onboarding room pairing, fingerprint, bootstrap rule and adoption (A1.1)
    Given an in-process rendezvous and an unowned pairing node
    When a fresh browser creates a pairing secret and joins its room
    And the pairing node runs pair with that secret and label "workstation"
    Then the node fingerprint seen in the room equals the node key fingerprint
    When the pairing browser opens ctl and sends hello before any device log
    Then the pairing welcome reports owner false
    And host.info and settings.set before enrolment are refused with "forbidden"
    When the pairing browser sends a log whose add entry for the node was signed by another device
    Then the pairing node still has no owner and refuses host.info with "forbidden"
    When the pairing browser sends a valid device log adding the node signed by the pairing browser
    Then pair completes with the node fingerprint and active path
    When the pairing browser sends hello again
    Then the pairing welcome reports owner true
    And host.info and settings.set succeed for the owner browser

  Scenario: pair.browser returns the same link on replay until used or expired (A1.9)
    Given an owned node with a connected owner browser over the rendezvous
    When the owner browser sends pair.browser with nonce "first"
    And the owner browser repeats pair.browser with nonce "first" while the link is valid
    Then both answers return ok with the same #pair link and expiry
    When a second browser pairs using that #pair link
    Then the second browser receives the device log, adopts the owner and reads owner true
    When the owner browser repeats pair.browser with nonce "first" after the link was used
    Then the pair.browser repeat is refused with "conflict"
    When the owner browser sends pair.browser with nonce "second"
    And 10 minutes pass so the second link expires
    And the owner browser repeats pair.browser with nonce "second"
    Then the pair.browser repeat is refused with "conflict"
