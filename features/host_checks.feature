Feature: A host checks the committed task worktree
  Background:
    Given a clean task branch for the host check runner

  Scenario: Uncommitted changes prevent any project check from starting
    Given the task worktree contains an uncommitted file
    When the host is asked to run a check that writes a marker
    Then the host refuses the dirty worktree and the marker is absent

  Scenario: A secret added by the task fails the hard check
    Given the task commit adds a line matching the secret checker
    When the host runs checks with an empty project command list
    Then the secret check fails for the task commit and the tests check passes

  Scenario: A project check gets the tool path and home without the node's test token
    Given the host environment contains a test token
    When the host runs a check that inspects PATH, HOME and the test token
    Then the command confirms PATH and HOME are set and the token is absent
