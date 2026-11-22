@web
Feature: Forge commands and group trust (U2-4)

  Scenario: Group info discloses custom commands without a consent gate
    Given a project with custom forge templates and a personal secretary owned by another person
    When the reader opens the group's commands
    Then the repository base forge checks and argument lists are disclosed with the execution warning
    And the secretary warning is visible without blocking group settings

  Scenario: A built-in forge remains fixed when editing group settings
    Given a project using the built-in GitHub forge
    When the owner opens group settings
    Then the forge is described as fixed and no forge selector or template editor is offered
