@web
Feature: Watching, and a group of people without AIs (U2-3)

  Scenario: A group with people and no AI has a main line, its people and a way to add an AI
    Given a group with two people and no AI
    When the group is opened
    Then the main line asks for a message to the people, the member list shows both people and offers to add an AI, and no secretary warning appears

  Scenario: A watch request names its AI and requester
    Given a watch confirmation addressed to the owner
    Then the owner sees who wants to watch which AI

  Scenario: The terminal of another person's AI is titled as watched
    Given the AI "Codex" of another person in the group
    When its terminal is opened
    Then the page is titled "Watching a terminal"
