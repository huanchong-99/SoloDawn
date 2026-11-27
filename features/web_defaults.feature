@web
Feature: Defaults on the Computers page

  Background:
    Given the owner's computer "workstation" with Claude Code and Codex, both with an account type

  Scenario: The owner sets a CLI's defaults once
    Then Claude Code shows "Not set" and "Set defaults"
    When the owner opens the defaults of Claude Code
    And chooses model "Opus", level "high", work share More and skills "Go backend"
    Then the hint says saving also updates 2 group members that follow these
    When the owner saves the defaults
    Then exactly one agent.defaults is sent with "opus", "high", "strong" and "Go backend"
    And Claude Code shows "Opus · high · Work share More · Go backend"
    And focus is on Claude Code's "Edit" button

  Scenario: Each group line says whether it follows
    Then the "Website" line of Claude Code says "Follows defaults"
    And the "Docs" line of Claude Code shows the badge "Own setting in this group"

  Scenario: A refused save keeps the editor
    When the node refuses agent.defaults with "bad_request"
    Then the editor stays open with the failure text

  Scenario: Phone width
    When the page is 375 px wide with the defaults editor open
    Then no element is wider than the page, and the buttons are at least 44 px high
