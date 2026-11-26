@web
Feature: Group trust (U2-4)

  Scenario: Group info shows the warning of a personal secretary without a consent gate
    Given a group whose secretary is the personal AI of another person
    Then the secretary warning is visible without blocking group settings

  Scenario: Group settings name no command that runs on a computer
    Given a group of the owner
    When the owner opens group settings
    Then the settings offer its name, its secretary, the pass score and the foundation tasks, and no command
