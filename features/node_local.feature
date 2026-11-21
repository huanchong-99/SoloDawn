Feature: Local link between a node's host and home
  When a host and a home run in the same node process, commands and row pushes
  cross their in-process link through the seam interfaces without WebRTC, and
  in-process test clusters start with scripted fake CLIs.

  Scenario: A host sends a command and receives a row push over the local link
    Given an in-process cluster of 1 node
    When the node's host creates group "Studio" on its local home
    And the node's host syncs group "Studio" over its local link
    And the node's host writes note "ship it" to group "Studio"
    Then the node's host receives a rows push for group "Studio" containing "ship it"
    And the node's local link reports path "local"
