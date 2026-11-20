Feature: Start the node at login
  Registering for the next login does not require starting a second node now.

  Background:
    Given a clean login registration for the current user

  Scenario: Register the node at login
    When the node installs its login service
    Then the current user has a SoloDawn login registration

  Scenario: Remove an installed login service
    Given the node installs its login service
    When the node removes its login service
    Then the current user has no SoloDawn login registration
