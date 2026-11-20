Feature: Authenticated peer connections
  Scenario: Both nodes can offer over the same two signalling paths
    Given two authorized nodes signed in on two local signalling paths
    When each node offers the other a connection
    Then both directions are direct and answer host.info
    And an ordered ctl push follows the reply in each direction

  Scenario: With direct traffic blocked the path is relay
    Given two authorized nodes signed in on two local signalling paths
    And a relay on this machine, with UDP and ICE-TCP between the nodes blocked
    When the node offers the tablet a connection
    Then both ends report the path relay and the connection answers host.info

  Scenario: A replayed offer is not processed
    Given two authorized nodes signed in on two local signalling paths
    And the node offers the tablet a connection dated 119 seconds ahead
    And the node offers the tablet a newer connection
    When the tablet receives the first offer again
    Then the tablet answers nothing and the newer connection answers host.info
    When the tablet receives the newer offer again on two other paths
    Then each of those paths gets the same answer and the newer connection answers host.info

  Scenario: Early and repeated candidates
    Given two authorized nodes whose signalling the test delivers
    When the node offers the tablet a connection
    And two copies of a candidate reach each side before the description it belongs to
    Then the connection opens and answers host.info
    When another copy of each of those candidates arrives
    Then the connection answers host.info
