Feature: The run loop of a group home
  A node that is home to groups runs each group by itself: it applies the
  commands in the order they arrive, keeps the result in its store, and tells
  the connected clients what changed, so that none of them has to read again.
  The connections of these scenarios are those of the owner's browser and of
  the owner's laptop.

  Scenario: A group created on one connection shows up on the other
    Given a home that the owner's browser and the owner's laptop are connected to
    When the owner's browser creates the group "Website"
    Then the owner's browser is answered with success
    And both connections are pushed a list of groups that holds "Website"

  Scenario: A message posted on one connection reaches the other
    Given a home that the owner's browser and the owner's laptop are connected to
    And the group "Website", which both connections read
    When the owner's laptop posts "Shall we start with the landing page?"
    Then the owner's laptop is answered with success
    And both connections are pushed the message "Shall we start with the landing page?" with the sequence number 3

  Scenario: A home that starts again goes on where it stopped
    Given a home that the owner's browser and the owner's laptop are connected to
    And the group "Website", which both connections read
    And the owner's laptop posts "Before the restart."
    When the home stops and starts again
    Then the home lists the group "Website" to both connections that connect again
    When the owner's browser posts "After the restart."
    Then the owner's browser is answered with success
    And both connections are pushed the message "After the restart." with the sequence number 4
