Feature: Node ctl session and device-log adoption
  A pairing connection to an unowned node accepts only hello and devlog until
  a valid device log signed by the pairing peer enrolls the node.

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

  Scenario: A hello of protocol version 1 or 2 is answered version, and version 3 is welcomed (A8.7 node half)
    Given an unowned node and a pairing browser connected over ctl
    When the browser sends hello with protocol version 1, then 2
    Then both were answered "version"
    When the browser sends hello before any device log arrives
    Then welcome reports owner false

  Scenario Outline: A node of this version that reaches a home of v3.0.0-rc.1 stops at version and does not retry with an older one (A8.7 node half)
    Given a node of this version that remembers a home which answers its hello with <answer>
    When the node has connected to that home twice
    Then each connection carried one hello, of protocol version 3, and nothing after it
    And the node's log says that the home speaks another protocol version

    Examples:
      | answer              |
      | the error version   |
      | a version-2 welcome |
