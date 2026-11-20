Feature: Authenticated peer connections
  Scenario: Both nodes can offer over the same two signalling paths
    Given two authorized nodes signed in on two local signalling paths
    When each node offers the other a connection
    Then both directions are direct and answer host.info
    And an ordered ctl push follows the reply in each direction
