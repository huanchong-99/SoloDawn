Feature: Reading a group and subscribing with sync
  A client reads a group and subscribes to its changes through sync: the home
  takes a cut of the store on the group's goroutine, sends either a delta or a
  reset with the state rows, pending cards and newest 50 main-line messages,
  answers synced at the cut, and pushes every later change afterwards.

  Scenario: Reconnecting after twenty missed messages delivers a delta in order
    Given a group home with the group "Website" read by the owner's browser
    When the owner's browser disconnects and 20 messages are posted
    And the owner's browser reconnects and syncs from its cursor
    Then the owner's browser is not told to reset
    And the owner's browser receives those 20 messages in order before synced

  Scenario: Reconnecting after 1500 missed rows resets with the newest 50 and an older pending card
    Given a group home with the group "Website" read by the owner's browser
    And a pending decision card posted before 1500 main-line messages
    When the owner's browser reconnects and syncs from its cursor
    Then the owner's browser is told to reset first
    And the owner's browser receives the state rows, the pending card and the newest 50 messages in sequence order

  Scenario: Syncing below the group's purge mark resets the client
    Given a group home with the group "Website" read by the owner's browser
    When a deleted message ages past retention and raises the purge mark above the browser's cursor
    And the owner's browser reconnects and syncs from its cursor
    Then the owner's browser is told to reset first

  Scenario: A host syncing with state_only receives only state rows at the cut and afterwards
    Given a group home with the group "Website" read by the owner's browser
    And a pending decision card posted before 2 main-line messages
    When the owner's laptop syncs the group with state_only
    Then the owner's laptop is told to reset first
    And the owner's laptop receives only the group's state rows before synced
    When a message and a task update are written to the group
    Then the owner's laptop is pushed the task row and not the message row

  Scenario: A row written while a first read is being sent arrives once after synced
    Given a group home on disk with the group "Website" and 5 messages
    When the owner's browser starts a cold sync held on its first frame while another message is posted
    Then the owner's browser receives the rows up to synced, then synced, and then the new message once
