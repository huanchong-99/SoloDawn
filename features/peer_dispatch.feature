Feature: Offers and data channels are dispatched by kind and label
  Scenario: A registered label is handed over, an unknown one is closed
    Given a node that hands channels of the family "test:" to a handler of its own
    And a browser connected to the node through a relay on this machine
    When the browser opens the channels "test:g:member" and "unknown" beside ctl
    Then the node's handler gets "test:g:member" as a stream of the browser's device
    And bytes cross that stream in both directions
    And the node closes "unknown"
    And the node still reads the browser's hello on ctl
