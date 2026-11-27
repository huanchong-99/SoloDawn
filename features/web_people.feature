@web
Feature: People in a group
  The browser renders live human roles and sends the group's signed people commands.

  Scenario: An owner invites someone with an expiring link
    Given I am the group owner
    When I create an invitation
    Then I can copy its link before it expires
    And after expiry the link is not offered for copying

  Scenario: An owner adds a person of another group on this computer without a link
    Given I am the group owner, and "Mina" is a member of another group on its home, and "Sam" was in one that is gone
    When I open "Invite"
    Then the dialog lists "Mina" and "Sam" under "In your other groups here", each with "Add", and offers "Cancel"
    When I add Mina, and then Sam, whom the home no longer knows
    Then the home has added Mina with "add_person", her row says "Added", and Sam's row says to send the code or the link
    And Mina is listed among the people of the group, and the dialog offers "Done" in place of "Cancel"

  Scenario: An owner makes, replaces and turns off the group's invitation code
    Given I am the group owner
    When I open "Invite" while the group has no invitation code open
    Then the dialog has asked the home for the code, and offers to make one for 1 hour, 24 hours or 7 days, with 24 hours chosen
    When I make a code for 7 days, which the home first refuses
    Then the dialog shows the code and until when it is valid
    And I can copy the code, and the invitation, whose last line is the code
    When I choose "New code", am told that the current code stops working at once, and confirm
    Then the dialog shows the new code
    When I turn the code off
    Then the dialog shows no code and offers to make one

  Scenario: The dialog shows the open code, and reads it again when the home is reached again
    Given I am the group owner
    When I open "Invite" while the group's code is open
    Then the dialog shows the code, and offers "Done"
    When I copy the code, it is replaced elsewhere, and the home is reached again
    Then the dialog shows the new code, with neither copy ticked
    When the code is turned off elsewhere and the home is reached again
    Then the dialog shows no code

  Scenario: Removing a person says that an open code ends with it, and leaving does not
    Given I am a group admin
    When I press "Remove" for a member
    Then the confirmation says that an open invitation code of the group is ended as well
    When I press "Leave" for myself
    Then the confirmation says nothing of a code

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

  Scenario: A person joins by an invitation code pasted in lower case with spaces
    Given I open "Join a group" with no link
    When I paste the code in lower case with spaces and no hyphens, give my name, and the home admits me
    Then the client is asked to join with that code alone, and opens the group the home named

  Scenario: A text that is no code is refused on the page
    Given I open "Join a group" with no link
    When I type a code with the letter O and my name, and join
    Then the page says that a code has 16 characters and never the letters I, L, O or U, and nothing is sent

  Scenario: While nothing answers a code, the page says so
    Given I open "Join a group" with no link
    When I type a code and my name, and join
    Then the page says that it looks for the group's home, and what to check while nothing answers
    When nothing has answered after 10 minutes
    Then the page says that the group's home never answered
    When I join again and the home refuses the code
    Then the page says that the code has ended

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
