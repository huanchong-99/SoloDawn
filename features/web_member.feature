@web
Feature: Member panel and adding an AI to a group
  The side panel of a group's chat shows its AI members, its human members, and a dialog for
  adding a registered AI from one of the owner's connected computers (planning/contracts/rows.md
  section 2, planning/contracts/commands.md section 2; acceptance tests A3.1, A3.4 as seen in the
  client).

  Background:
    Given a group "Website" homed on "workstation", open in a browser of its owner in English

  Scenario: AI member cards show secretary, working, waiting, sleeping and offline states
    Given the group has a secretary, a working AI on T-12, a waiting AI on T-13, Mina's AI on T-11, and a dormant AI
    Then the AI section header reads "AI · 5" and places the secretary first with the secretary mark
    And the cards show "Waiting · 2 items waiting on you", "Working · T-12 Fix review remarks", "Waiting · T-13 waiting for your approval", "Waiting · T-11 waiting for Mina's approval" and "Sleeping · wakes when @mentioned"
    And the computers read "workstation · group home", "laptop · direct" and "windows · relayed, limited"
    And the accounts read "Personal subscription · takes orders from owner", "Team plan" and "API key"
    And no usage bar or terminal button is shown
    When the dormant AI wakes to work and another AI goes offline, pauses, hits a limit and enters takeover
    Then those cards update to working, "Offline" with "offline", "Paused", "Limit reached" and "Owner is operating directly"

  Scenario: The people list shows names, roles and online status
    Given the group also has "Mina" as an online admin and "Sam" as an offline member
    Then the people section reads "People · 3"
    And the people list shows "Alex · You · Owner" as "Online", "Mina · Admin" as "Online", and "Sam" as "Offline"

  Scenario: Below 1024 px the member panel opens and closes from the chat header
    Given the chat is reopened on a screen 820 px wide
    When the owner opens "People and details"
    Then the side panel is open with the member list and the chat behind it is inert
    When the owner opens "Add AI" and presses Escape in the dialog
    Then the dialog closes and the side panel stays open
    When the owner presses "Close the panel"
    Then the side panel closes and focus returns to "People and details"

  Scenario: Adding an AI reads connected computers and sends host.join
    Given the group has no secretary yet and a second computer "laptop" is connected
    When the owner opens "Add AI"
    Then "Claude Code · workstation" is disabled as "Already in this group" and "Codex · workstation" is disabled as "Choose an account type on Computers first"
    And "Codex · laptop" is chosen with name "Codex" and "Claude Code · laptop" warns "Not signed in"
    When the owner toggles "Make this AI the group's secretary" on and off, then checks it and names the AI "Lead Secretary"
    And the owner submits the add-AI dialog
    Then "laptop" receives a signed "host.join" for "codex" named "Lead Secretary" with secretary true
    And the dialog closes and the new AI card "Lead Secretary" appears in the panel

  Scenario: Refusals and lost computers are reported without sending elsewhere
    Given a second computer "laptop" is connected
    When "laptop" refuses "host.join" with "conflict" and the owner submits "Codex · laptop"
    Then the dialog stays open and says "Not sent. The home answered: conflict"
    When "laptop" disconnects before the owner submits again
    Then no command is sent to "workstation" and the dialog says "Not sent: the group home cannot be reached."
    When the group home disconnects
    Then the "Add AI" button is disabled
