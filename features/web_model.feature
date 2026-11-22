@web
Feature: Choosing AI models and levels
  A10.13–A10.16 as seen in the client: host lists supply model choices, an owner
  refreshes and edits explicitly, and several members may use one CLI.

  Background:
    Given the Website group and its model catalog open in English

  Scenario: Adding another member of the same CLI chooses a listed model
    When the owner opens Add AI
    Then the existing CLI is selectable again with a free member name
    When the owner chooses Opus and high
    Then fast mode is shown and the suggested tier is Strong
    When the owner submits Add AI
    Then host.join sends the exact listed model and level

  Scenario: An empty catalog offers model and level text fields
    Given the CLI lists no models
    When the owner opens Add AI
    And types a custom model and level
    And the owner submits Add AI
    Then host.join sends the exact typed model and level

  Scenario: Model choices reset dependent values and candidates reset the draft
    When the owner opens Add AI
    And the owner chooses Opus and high
    And chooses Haiku without levels
    Then no level is offered and the tier is Light
    When the owner returns to the CLI defaults
    Then no level is offered and the tier is Standard
    When the owner changes computer CLI candidates
    Then the model and level draft return to the CLI defaults

  Scenario: Everyone sees model names and levels but only the owner can edit
    Then the owner card shows Opus and high and the other card shows CLI defaults
    And the other card has no model editor or refresh
    When the browser switches to Chinese
    Then model labels and defaults are translated

  Scenario: Refresh changes the catalog without changing the member
    Given the stored model is no longer listed
    Then the card and editor preserve the unlisted model
    When the owner refreshes after the catalog gains the stored model
    Then refresh true is sent and the model name and badge update without a restart
    And the stored model level tier and skills are unchanged

  Scenario: The owner changes only model and level and can cancel
    When the owner opens the model editor
    Then Save is disabled and the editor has focus
    When the owner chooses Haiku in the editor
    And saves the model editor
    Then exactly one member set changes only model and level
    When the owner opens the model editor again and cancels
    Then focus returns to Change model

  Scenario: A failed edit stays open and offline controls are disabled
    Given the CLI lists no models
    When the owner opens the model editor
    And saves typed model and level while the home holds its answer
    Then the editor is disabled while saving
    When the home refuses the model change
    Then the exact model error is shown
    When the home disconnects
    Then Save and refresh are disabled

  Scenario: Computers lists two members of one CLI in one group
    When the browser opens Computers with two hosted members of one CLI
    Then both rows show Website and their distinct member names
