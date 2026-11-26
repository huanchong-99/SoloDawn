@web
Feature: Watching and the team main line (U2-3)

  Scenario: A team group has a main line and people without project controls
    Given a team group with a bulletin and two people
    When the team group is opened
    Then the bulletin names its AI project and signer and no project controls appear

  Scenario: A watch request names its AI and requester
    Given a watch confirmation addressed to the owner
    Then the owner sees who wants to watch which AI

  Scenario: The terminal of another person's AI is titled as watched
    Given the AI "Codex" of another person in the group
    When its terminal is opened
    Then the page is titled "Watching a terminal"
