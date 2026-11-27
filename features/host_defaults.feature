Feature: Defaults of a CLI on a node

  Scenario: The owner sets the defaults of a CLI (A10.32)
    Given a node with claude found and its account declared
    Then host.info lists claude without defaults
    When the owner sends agent.defaults for claude with "opus", "high", "strong" and "Go backend"
    Then host.info lists claude with defaults "opus", "high", "strong" and "Go backend"
    And agent.defaults for "external" is refused with "bad_request"
    And agent.defaults with tier "expert" is refused with "bad_request"
    And agent.defaults without skills is refused with "bad_request"

  Scenario: Joining follows the defaults unless told otherwise (A10.33)
    Given claude defaults "opus", "high", "strong" and "Go backend" on the node
    When the owner joins claude to "Website" following the defaults
    Then the home has a claude member in "Website" that follows, with "opus", "high", "strong" and "Go backend"
    When the owner joins claude to "Website" again as "Haiku" with model "haiku" and tier "light"
    Then the "Haiku" member has its own profile "haiku", "", "light" and ""

  Scenario: A changed default reaches every member that follows it (A10.34)
    Given claude on the node is a member of "Website" and "App" following the defaults, homed on two nodes
    And claude is a member of "Docs" with its own profile "haiku", "", "light" and ""
    And codex on the node is a member of "Website" following the defaults
    When the owner sends agent.defaults for claude with "sonnet", "", "standard" and ""
    Then the claude members of "Website" and "App" have "sonnet", "", "standard" and ""
    And the claude member of "Docs" keeps "haiku", "", "light" and ""
    And no member set was sent for the codex member

  Scenario: A home that was away catches up after its sync (A10.34)
    Given claude on the node is a member of "App" following the defaults
    And the home of "App" is disconnected
    When the owner sends agent.defaults for claude with "sonnet", "", "standard" and ""
    And the home of "App" connects again
    Then after the sync the claude member of "App" has "sonnet", "", "standard" and ""

  Scenario: A restore is filled by the host and settles (A10.30 host)
    Given claude defaults "opus", "high", "strong" and "Go backend" on the node
    And claude is a member of "Docs" with its own profile "haiku", "", "light" and ""
    When the owner sets follow on that member at the home
    Then the host sends exactly one member set for it, with "opus", "high", "strong" and "Go backend"

  Scenario: The roster shows the values in force, not the following (A10.29 host)
    Given the secretary's group has a claude member that follows defaults with tier "strong" and skills "Go backend"
    When the secretary takes its next turn
    Then its roster line for that member shows "strong" and "Go backend"
    And no turn text and no list_members answer contains "follow"
