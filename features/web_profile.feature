@web
Feature: AI profiles and stronger-AI escalation
  A10.8, A10.10 and A10.16 as seen in the client: profiles belong to their owners,
  model suggestions are choices, and a stuck task asks for a stronger AI.

  Background:
    Given the Website group open in its owner's browser in English

  Scenario: Everyone sees profiles but only the owner can edit
    Then the cards show exact tier and skills values with an empty-skills fallback
    And the other person's card has no profile editor
    When the browser switches to Chinese
    Then the profile labels are 派活 and 擅长, the strong tier is 多, and the share sits with the quota

  Scenario: The secretary's card and editor offer no work share, and it comes back with its stored value
    Given the owner's AI is the group's secretary
    Then its card shows only the quota in its block, with no work share and no suggestion
    When the owner opens the profile editor
    Then the editor offers model, thinking level and skills and no work share
    When the owner cancels the profile editor and the AI is no longer the secretary
    Then its card shows its stored work share Normal and the suggestion More again

  Scenario: The owner edits a profile and cancels without losing focus
    When the owner opens the profile editor
    Then Save is disabled until something changes
    When the owner chooses Light and writes Documentation
    And saves the profile
    Then exactly one member set sends light and Documentation
    When the owner reopens the profile editor
    And cancels the profile editor
    Then focus returns to Edit profile

  Scenario: A model suggestion is an explicit owner choice
    Then the owner's card offers More and Use labelled Suggested: More
    When the owner uses the suggestion
    Then exactly one member set sends only tier strong
    When the suggestion matches the stored tier
    Then no suggestion is offered
    When the model is no longer listed
    Then no suggestion is offered for the unlisted model
    And the other person's card has no suggestion

  Scenario: Profile skills count Unicode code points at the limit
    When the owner opens the profile editor
    Then 200 astral characters allow Save and 201 mark skills invalid and disable Save

  Scenario: A stuck task asks for a stronger AI
    When a task card arrives with stuck ask true
    Then its badge is Must ask a stronger AI
    When the task's stuck ask becomes false
    Then its badge is In progress

  Scenario: Adding an AI sends the chosen profile with CLI defaults
    When the owner opens Add AI and chooses Light with skills Documentation
    And submits the add-AI dialog
    Then host.join contains the exact profile and CLI defaults

  Scenario: Profile errors, busy state and offline controls are visible
    When the owner opens the profile editor
    And saves changed skills while the home holds its answer
    Then the editor and suggestion are disabled while sending
    When the home refuses the change
    Then the profile error is visible and the editor stays open
    When the owner cancels and reopens the editor
    Then the error is cleared and the stored skills are restored
    When the group home disconnects
    Then the editor and suggestion are disabled offline
