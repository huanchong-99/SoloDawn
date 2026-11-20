Feature: Relay-free probes and moving a connection as a reconnect
  Scenario: A relayed connection moves to direct after direct traffic is unblocked
    Given two authorized nodes with a local relay
    When the client connects to the node on one local network
    Then the client observes path "direct" within 1 second and host.info succeeds
    When the client reconnects with UDP and ICE-TCP between the nodes blocked
    Then both ends observe path "relay" within 1 second
    And host.info is sent every second on the client
    When UDP and ICE-TCP between the nodes are unblocked
    Then the path becomes "direct" within one probe interval plus 5 seconds
    And every periodic host.info request was answered once

  Scenario: Closing the old connection after hello completes the move when welcome arrives in time
    Given a client connected to a node over the local relay with direct traffic unblocked
    When a move is forced through a probe and the old connection closes after hello
    And welcome arrives on the probe before the 10-second deadline
    Then the client sent no new offer and completed the move to "direct"
    And host.info succeeds on the client

  Scenario: Withholding welcome through the deadline starts one ordinary reconnect when the old connection closed
    Given a client connected to a node over the local relay with direct traffic unblocked
    When a move is forced through a probe and welcome is withheld after the old connection closes
    Then the client starts one ordinary reconnect after the 10-second deadline and host.info succeeds

  Scenario: Withholding welcome through the deadline keeps the old connection when it is still serving
    Given a client connected to a node over the local relay whose probe withholds welcome without closing the old connection
    When a move is forced through a probe and the 10-second deadline passes
    Then the client sent no ordinary offer and discarded the probe
    And host.info continues on the old connection
