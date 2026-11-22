@web
Feature: Style presets in the web client
  Scenario: A brief names a preset and opens its text
    Given a group with a brief naming Calm dashboard
    When the reader opens the brief preset
    Then the card names Calm dashboard and shows its text
    And typed notes do not appear in Project memory

  Scenario: A person lists creates and changes presets
    Given an online human member opens Style presets
    Then the editor fetches presets and lists their name and text
    When the member creates Quiet forms
    Then a preset note command creates Quiet forms
    When the member edits Quiet forms
    Then a preset note command replaces its text

  Scenario: Missing presets and read-only states
    Given a group with a brief naming Calm dashboard
    When the named preset is deleted and the reader opens it
    Then the brief says the preset was deleted
    When the home disconnects and the reader opens Style presets
    Then presets remain readable but writing is disabled
