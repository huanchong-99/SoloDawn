Feature: M1 real-transport group-list push
  Connected browsers of a node's owner over direct WebRTC data channels receive
  live group-list pushes and synced message rows within 1 second without
  reconnecting.

  Scenario: Creating a group pushes groups to an already-connected second browser within 1 second (A2.1)
    Given an M1 owned node with two already-connected browsers of its owner
    When the first M1 browser creates group "Website"
    Then the second M1 browser receives a groups push listing "Website" within 1 second without reconnecting
    When the second M1 browser syncs "Website" and the first M1 browser posts "hello from browser one"
    Then the second M1 browser receives that message row within 1 second

  # The 10 minutes are not run. Besides the node closing it, what ends an
  # open connection is its ctl closing, failed ICE, a replacing offer or a
  # missing answer to the offer (internal/peer/signal.go, unanswered.go); none
  # of it is tied to a signalling path or left on a timer once the connection
  # is open. As in A2.1, the node is the test's peerNode, not the node runtime,
  # and the browsers are not web/src/lib/peer.ts.
  Scenario: With the rendezvous stopped, open direct connections keep working (A1.4)
    Given an M1 owned node with two directly connected browsers and a rendezvous process
    When the M1 rendezvous process stops and every signalling path to it has ended
    Then both M1 connections are still the direct ones and answer host.info
    When the first M1 browser creates group "Offline"
    Then the second M1 browser receives a groups push listing "Offline" within 1 second without reconnecting
    When 10 minutes pass on the M1 node's clock
    Then both M1 connections are still the direct ones and answer host.info
    When the second M1 browser syncs "Offline" and the first M1 browser posts "sent without a rendezvous"
    Then the second M1 browser receives that message row within 1 second
