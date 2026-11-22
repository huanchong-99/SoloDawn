@web
Feature: Moving a group's home (U2-5)

  Scenario: A browser follows moved without sending an unanswered command twice
    Given a browser reading a project on its old home
    When a command response says that the group moved to another computer
    Then the group stays on the same route with its cached rows and its new home
    And the browser reads from the new home without replaying the command

  Scenario: Only the owner sees local move commands
    Given the owner reads a project and has another computer
    When the owner opens the export and import guidance
    Then it shows the group's export command naming that computer and the import command
    And it explains the unavailable interval and duplicate-command risk
