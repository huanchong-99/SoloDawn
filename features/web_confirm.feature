@web
Feature: Confirmation cards and access to an AI (U2-2)

  Scenario: Only the person asked can confirm or deny (A9.8, A9.9)
    Given a direct request on another person's AI
    Then the requester sees whom it waits for and no actions
    When the person asked confirms on either browser
    Then each browser sends the signed confirm action
    When the person asked denies on one browser
    Then that browser sends the signed deny action

  Scenario: Requests show their kind and outcome (A9.9, A9.11)
    Given the examples of all four confirmation states
    Then the cards show the three kinds, four states and no actions for the observer
    When the person asked is no longer in the group
    Then the decisions still show their outcomes and times
    When the referenced call has not been loaded
    Then the card identifies the requester as unknown

  Scenario: Only each AI's owner can change its lists (A9.14)
    Given a team AI and a personal AI with the same owner
    Then a different member sees the access lists without edit controls
    When the owner changes both lists on the team AI
    Then the browser sends both lists to the home and shows the update
    When the owner clears the allow list on the personal AI
    Then the browser sends only the empty allow list

  Scenario: Offline and refused edits do not appear saved
    Given a pending confirmation and one owned AI
    Then offline actions and access edits cannot be submitted
    When the home refuses the confirmation and access changes
    Then both errors are shown and the access changes can be cancelled

  Scenario: An AI in a one-person group has no other person to list
    Given the owner opens the editor with no other members
    Then both lists have no choices and Save is disabled
