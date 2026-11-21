Feature: Node ctl session, device-log adoption and revocation
  A pairing connection to an unowned node accepts only hello and devlog until
  a valid device log signed by the pairing peer enrolls the node, and revoking
  a browser closes its open connection within 1 second and refuses reconnects.

  Scenario: Pairing bootstrap rules and signer check on adoption (A1.1)
    Given an unowned node and a pairing browser connected over ctl
    When the browser sends hello before any device log arrives
    Then welcome reports owner false
    And host.info and host commands on the pairing connection are refused with "forbidden"
    When the browser sends a device log whose add entry for the node was signed by another device
    Then the node still has no owner and refuses host.info with "forbidden"
    When the browser sends a device log whose add entry for the node is signed by the pairing browser
    And the browser sends hello again
    Then welcome reports owner true
    And host.info and a host command succeed with a stored receipt

  Scenario: Revoking a browser closes its connection within 1 second and refuses its next attempt (A1.6)
    Given an owned node with two browsers of its owner connected over ctl
    When the first browser revokes the second browser in a devlog push
    Then the second browser's connection closes within 1 second
    And the second browser's next connection attempt is refused with "unknown_device"
