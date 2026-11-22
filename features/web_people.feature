@web
Feature: People in a group
  The browser renders live human roles and sends the group's signed people commands.

  Scenario: An owner invites someone with an expiring link
    Given I am the group owner
    When I create an invitation
    Then I can copy its link before it expires
    And after expiry the link is not offered for copying

  Scenario: An owner changes roles and removes a person
    Given I am the group owner
    When I promote a person and the home updates their role
    Then their row shows that they are an admin
    When I remove the person
    Then a task conflict is shown without removing their row
    When the home accepts the removal
    Then the person is no longer listed

  Scenario: An admin can invite and remove members but cannot set roles
    Given I am a group admin
    Then I can invite, remove a member, or leave, but cannot remove the owner or set roles

  Scenario: A member cannot invite or remove other people
    Given I am a group member
    Then I can only leave the group

  Scenario: An unpaired browser opens an invitation
    Given I open a valid invitation link
    When I submit my name on an unpaired browser
    Then the page offers to connect a computer

  Scenario: Leaving a pending join cancels it
    Given I open a valid invitation link
    When I submit my name and navigate away while joining
    Then the invitation attempt is cancelled

  Scenario: Admission opens the joined group
    Given I open a valid invitation link
    When I submit my name and the home admits me
    Then the client opens the joined group

  Scenario: An invalid invitation never starts a join
    Given I open a malformed invitation
    Then the page rejects the link without showing the form

  Scenario: An admitted browser can open its group after a notification failure
    Given I open a valid invitation link
    When the home first refuses and then admits me but another computer cannot be notified
    Then the notification warning links to the group

  Scenario: The join page uses the shell language and theme controls
    Given I open a valid invitation link
    When I change theme and language from the join page header
    Then the join page uses dark mode and Chinese text

  Scenario: A refusal arriving after navigation does not reopen the join page
    Given I open a valid invitation link
    When I submit my name and the home refuses after I navigate away
    Then the groups page stays open

  Scenario: The panel still hands back an owned AI
    Given I am the group owner with an AI in takeover
    When I hand back from its member row
    Then its computer receives the hand-back command
