@web
Feature: End-to-end harness
  The end-to-end tests run the web client in Chromium, Firefox and WebKit against real processes:
  a rendezvous and nodes with the fake Claude Code and Codex first on their PATH. A run builds the
  web client into the binary once. Each test starts its own processes on free 127.0.0.1 ports,
  every node with a HOME and a data directory of its own, and what a test started is stopped and
  removed when the test ends, also when it fails or times out. Nothing of it is left in the test
  process.

  Scenario: A test starts a rendezvous and two nodes and stops them
    When a test starts a rendezvous and two nodes
    Then the browser loads the web client from the rendezvous at its 127.0.0.1 address
    And each node has made its data directory in a HOME of its own
    And each node finds the built solodawn and the fake CLIs first on its PATH
    When the test stops them
    Then none of their processes runs and their directory is gone
    And the test process has the HOME, PATH and working directory it had before

  Scenario: A start that fails stops what it had started
    When a test starts a rendezvous and a node that cannot make its data directory
    Then the start fails with the node's own message
    And none of the processes it started runs and its directory is gone

  Scenario: What failed, timed-out and killed tests started is stopped and removed
    When a run has a test that fails, one that times out and one whose worker is killed, each after starting a rendezvous and two nodes
    Then the run fails
    And nothing that the failed and the timed-out test started runs, and their directories are gone
    And what the killed test started runs until the teardown of the run removes it
