Feature: Verified node self-update

  Background:
    Given the node has release "0.3.1" installed

  Scenario: Install a newer release whose binary matches the manifest
    Given the release server offers "0.3.2" with a matching checksum
    When the node checks for an update
    And the node applies its verified update
    Then the installed binary contains release "0.3.2"
    And the update status is "pending" for "0.3.2"
    And the node has requested 1 restart

  Scenario: A corrupt download leaves the installation intact
    Given the release server offers "0.3.2" with a mismatching checksum
    When the node checks for an update
    And the node applies its verified update
    Then the installed binary contains release "0.3.1"
    And the update status is "mismatch" for "0.3.2"
    And the node has requested 0 restarts

  Scenario Outline: An active session defers installation and restart
    Given the release server offers "0.3.2" with a matching checksum
    When the node checks for an update
    And a session is in <state> when the node applies its verified update
    Then the installed binary contains release "0.3.1"
    And the node has requested 0 restarts
    When the session becomes idle and the node applies its verified update
    Then the installed binary contains release "0.3.2"
    And the node has requested 1 restart

    Examples:
      | state    |
      | working  |
      | takeover |
