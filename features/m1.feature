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
