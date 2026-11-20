@web
Feature: Browser connections
  The web client offers a node a connection over its signalling addresses and signs what it sends
  about it. What the node sends back is applied to the connection it names and to no other: a
  candidate that comes before the node's answer waits for it, and each candidate is applied once.
  After a connection is lost the client offers again, and what the node had not answered is asked
  again on the new connection.

  Background:
    Given a browser signed in on a rendezvous, and a node it knows

  Scenario: Early and repeated candidates of the node are applied once
    When the browser offers the node a connection
    Then the node receives an offer the browser signed
    When two copies of a candidate of the node arrive before its answer
    Then the browser has applied no candidate
    When the node's answer arrives
    Then the browser has applied the candidate exactly once
    When the connection opens and another copy of the candidate arrives
    Then the candidate is still applied once
    And a request on the connection is answered

  Scenario: An unanswered command is sent again on the next connection
    Given the browser is connected to the node
    When the browser sends a command and the connection is lost before the node answers
    Then the browser offers the node a new connection
    When the node welcomes the browser on it
    Then the command is sent again and its answer reaches the sender
