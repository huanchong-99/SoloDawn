Feature: Milestone 2 groups, messages, and sync on the local link
  Connected browsers of a node's owner create a project group on its home,
  exchange messages live within a second, catch up across disconnects by delta
  or cold reset with older pending cards and paged scroll-back, keep command
  nonces idempotent, refuse commands and reads from non-member devices, and
  resume sync without reloading or losing rows when the home restarts.

  Scenario: Creating a group and posting messages reaches a second connected browser within one second
    Given an M2 node with connected browser A and browser B of its owner
    When browser A creates the project group "Website" on the M2 home
    Then browser B sees "Website" without reloading and syncs its state
    When the local host adds the worker AI "builder" to "Website"
    And browser A posts "Ship the status bar" in "Website"
    Then browser B receives "Ship the status bar" in "Website" within 1 second
    When browser B posts "On it now" in "Website"
    Then browser A receives "On it now" in "Website" within 1 second

  Scenario: Reconnecting after twenty missed messages receives those twenty in order without reset
    Given an M2 node where browser A and browser B have synced "Website"
    When browser B disconnects from the M2 home
    And browser A posts 20 numbered messages in "Website"
    And browser B reconnects and syncs "Website" from its cursor
    Then browser B was not reset on "Website" and received those 20 messages in order

  Scenario: Reconnecting after 1500 missed rows resets with the newest 50, keeps an older pending card, and scrolls back in creation order
    Given an M2 node where browser A and browser B have synced "Website" with worker AI "builder"
    And "builder" has posted a pending decision card in "Website" synced by browser B
    When browser B disconnects from the M2 home
    And browser A posts 1500 numbered messages in "Website"
    And browser B reconnects and syncs "Website" from its cursor
    Then browser B was reset on "Website" and holds the pending decision card and newest messages 1451 to 1500
    When browser B scrolls back two pages of 50 messages in "Website"
    Then browser B holds messages 1351 to 1500 in "Website" in creation order with its sync cursor unchanged
    When a cold browser C connects and syncs "Website" from scratch
    Then browser C holds the pending decision card and newest messages 1451 to 1500 in "Website"

  Scenario: Sending a command twice with the same nonce writes one row and returns the same answer both times
    Given an M2 node where browser A and browser B have synced "Website"
    When browser A sends a text post command twice with the same nonce in "Website"
    Then both M2 command answers are the same success and "Website" holds 1 message row

  Scenario: Commands and reads from a non-member device are refused
    Given an M2 node where browser A and browser B have synced "Website"
    When a non-member device sends a text post command to "Website"
    Then the M2 home refuses that command with "unknown_device" and "Website" still holds 0 message rows
    And the M2 home refuses sync and fetch from that non-member device on "Website" with "forbidden"

  Scenario: Stopping and restarting the home disconnects browsers and resumes sync without reload or lost rows
    Given an M2 node where browser A and browser B have synced "Website"
    And browser A posts "Before stop one" in "Website"
    And browser B receives "Before stop one" in "Website" within 1 second
    When the M2 home stops
    Then browser B's connection to the home is gone and it still holds "Before stop one" in "Website"
    When the M2 home restarts from its disk store
    And browser A reconnects, syncs "Website", and posts "Posted while B reconnects"
    And browser B reconnects and syncs "Website" without reloading
    Then browser B was not reset on "Website"
    When browser B posts "After home restart" in "Website"
    Then browser A receives "After home restart" in "Website" within 1 second
    And both browsers hold "Before stop one, Posted while B reconnects, After home restart" in "Website" in creation order
