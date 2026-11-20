Feature: Verified node self-update
  The node replaces its binary with a newer release whose SHA-256 matches
  SHA256SUMS, and restarts into it only when no session is working or in
  takeover. The binaries are real builds with the release tag stamped in.

  Background:
    Given the node runs release "v0.3.1"

  Scenario: A newer release whose binary matches the manifest is installed
    Given the release server offers "v0.3.2" with a matching checksum
    When the node checks for an update
    Then the installed binary prints version "v0.3.2"
    And the update status is "pending" for "v0.3.2"
    When the node applies its update
    Then the node has requested 1 restart

  Scenario: A mismatching download replaces nothing
    Given the release server offers "v0.3.2" with a mismatching checksum
    When the node checks for an update
    Then the installed binary prints version "v0.3.1"
    And the update status is "mismatch" for "v0.3.2"
    When the node applies its update
    Then the node has requested 0 restarts

  Scenario Outline: The node does not restart while a session is <state>
    Given the release server offers "v0.3.2" with a matching checksum
    And a session is <state>
    When the node checks for an update
    And the node applies its update
    Then the installed binary prints version "v0.3.2"
    And the node has requested 0 restarts
    When the session becomes idle
    And the node applies its update
    Then the node has requested 1 restart

    Examples:
      | state    |
      | working  |
      | takeover |

  Scenario: A pre-release tag is ignored
    Given the release server offers "v0.4.0-rc.1" with a matching checksum
    When the node checks for an update
    And the node applies its update
    Then the installed binary prints version "v0.3.1"
    And there is no update status
    And the node has requested 0 restarts
