Feature: Offer kinds and channel labels are dispatched through one table
  Scenario: A registered label is handed over, an unknown one is closed
    Given a node with a test handler registered for the channel family "test:"
    And a browser connected to the node through a relay on this machine
    When the browser opens the channels "test:g:member" and "unknown" beside ctl
    Then the node hands "test:g:member" to the handler with the browser's connection, and it opens
    And the node closes "unknown"
    And the node still reads the browser's hello on ctl
