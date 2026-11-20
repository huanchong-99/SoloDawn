@web
Feature: Browser signalling
  The web client signs in on every signalling address it has, a rendezvous and a node on the same
  machine alike, and says everything on each of them. What another device sends back is handed to
  the connection only when its signed envelope passes the receiver's checks, so a rendezvous that
  changes a description on the way gets nothing through.

  Background:
    Given a browser that knows a node, with a rendezvous and a local node as signalling addresses

  Scenario: The browser proves its key on each address with that address's challenge
    When the rendezvous and the local node each send a challenge of their own
    Then each receives the browser's signature of its own challenge
    When each answers that the browser is signed in
    Then the browser is told the ICE servers of each address

  Scenario: An offer goes out on every address, and a slow one delays no other
    Given the rendezvous has signed the browser in and the local node has not yet
    When the browser sends the node an offer
    Then the rendezvous has the offer and the local node has nothing
    When the local node signs the browser in
    Then the local node has the offer too

  Scenario: An answer whose description a rendezvous changed is refused
    Given both addresses have signed the browser in
    When the node's signed answer arrives through the rendezvous with its description changed
    Then the connection is given nothing
    When the same answer arrives through the local node as the node signed it
    Then the connection is given that answer from the local node
