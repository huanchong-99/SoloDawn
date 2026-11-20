Feature: Rendezvous signalling
  Devices that cannot reach each other directly meet at a rendezvous. A device first proves that
  it holds its key. Then it sends signed envelopes to another device by its id, is told which of
  the devices it watches are connected, and meets a device it has never seen in a pairing room.
  The rendezvous forwards what it is given without reading it, so a receiver checks every envelope
  itself.

  Background:
    Given a local rendezvous

  Scenario: A1.7 A device spends one daily quota across two open TURN allocations
    Given a local TLS rendezvous with a 1 MiB relay quota
    When the browser signs the challenge and allocates a UDP and a TLS relay
    And each allocation forwards 256 KiB to a peer and 256 KiB back
    Then neither open allocation forwards another byte in either direction
    When the browser sends {"t":"quota"}
    Then the browser receives {"t":"quota","used":1048576,"limit":1048576}

  Scenario: A device that signs the challenge is told the ICE servers
    When the browser answers the challenge with its signature of that challenge
    Then the browser receives {"t":"ready","ice":[],"ttl":3600}
    When the browser sends {"t":"ice"}
    Then the browser receives {"t":"ice","ice":[],"ttl":3600}

  Scenario Outline: A device that does not prove its key is disconnected
    When the browser answers the challenge with <answer>
    Then the browser is disconnected

    Examples:
      | answer                                          |
      | its signature of another connection's challenge |
      | its signature of an envelope with that text     |
      | the node's signature of that challenge          |
      | a request for the ICE servers                   |

  Scenario: An envelope reaches a connected device under its sender's id
    Given the browser and the node are signed in
    When the browser sends the node an envelope
    Then the node receives that envelope from the browser

  Scenario: The sender is told that a device is not connected
    Given the browser is signed in
    When the browser sends the node an envelope
    Then the browser is told that the node is not connected

  Scenario: A watcher follows the devices on its list
    Given the browser and the node are signed in
    When the browser watches the node and the tablet
    Then the browser sees the node online
    And the browser sees the tablet offline
    When the tablet signs in
    Then the browser sees the tablet online
    When the node leaves
    Then the browser sees the node offline

  Scenario: A new connection of a device replaces its earlier one
    Given the browser and the node are signed in
    And the browser watches the node
    And the browser sees the node online
    When the node signs in again
    Then the node's earlier connection is disconnected
    When the browser sends the node an envelope
    Then the node receives that envelope from the browser
    And the browser has received nothing else

  Scenario: Two devices meet in a pairing room and a third stays outside
    Given the browser, the node and the tablet are signed in
    When the browser joins the room of a pairing secret
    And the node joins the room
    Then the browser is told that the room is joined
    And the node is told that the room is joined
    When the tablet joins the room
    Then the tablet is told that the room is full
    When the browser sends its pair message through the room
    Then the node receives that room message
    When the tablet sends its pair message through the room
    Then the browser has received nothing else
    And the node has received nothing else
    And the tablet has received nothing else

  Scenario Outline: A frame holds 16 KiB, 10 seconds hold 120 frames and a watch list 64 devices
    Given the browser is signed in
    When the browser <action>
    Then the browser receives <outcome>

    Examples:
      | action                                 | outcome                         |
      | sends a frame of 16384 bytes           | 1 answer                        |
      | sends a frame of 16385 bytes           | 0 answers and is disconnected   |
      | watches 64 devices                     | 64 answers                      |
      | watches 65 devices                     | 0 answers and is disconnected   |
      | asks for the ICE servers 119 times     | 119 answers                     |
      | asks for the ICE servers 120 times     | 119 answers and is disconnected |

  Scenario Outline: A1.5 A description that the rendezvous changed in transit is refused
    Given the browser and the node are signed in
    When the browser offers the node a connection and the rendezvous <action> the SDP fingerprint in transit
    Then the node's check of the offer returns "<result>"

    Examples:
      | action    | result   |
      | preserves | accepted |
      | changes   | bad_sig  |

  Scenario: The web client is served beside signalling
    Then the address "/" returns the file "index.html" of the web client
    And the address "/install.md" returns the file "install.md" of the web client

  Scenario: The rendezvous command serves signalling on the address it is given
    Given the command "solodawn rendezvous" listening on a local port
    When the browser answers the challenge with its signature of that challenge
    Then the browser receives {"t":"ready","ice":[],"ttl":3600}
