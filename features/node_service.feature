Feature: Start the node at login
  "solodawn service install" and "solodawn service remove" write or delete
  the start-at-login entry themselves; no node needs to run, and none is
  started now.

  Background:
    Given a clean login registration for the current user

  Scenario: Register the node at login
    When the owner runs "solodawn service install" with no node running
    Then the current user has a SoloDawn login registration

  Scenario: Remove an installed login service
    Given the owner runs "solodawn service install" with no node running
    When the owner runs "solodawn service remove" with no node running
    Then the current user has no SoloDawn login registration
