@web
Feature: Member panel and adding an AI to a group
  The side panel of a group's chat shows its AI members, its human members, and a dialog for
  adding a registered AI from one of the owner's connected computers (planning/contracts/rows.md
  section 2, planning/contracts/commands.md section 2; acceptance tests A3.1, A3.4 as seen in the
  client).

  Background:
    Given a group "Website" homed on "workstation", open in a browser of its owner in English

  Scenario: AI member cards show secretary, working, waiting, sleeping, limited and offline states
    Given the group has a secretary, a working AI on T-12, a waiting AI on T-13, Mina's AI on T-11, and a dormant AI
    Then the AI section header reads "AI · 5" and places the secretary first with the secretary mark
    And the cards show "Waiting · 2 items waiting on you", "Working · T-12 Fix review remarks", "Waiting · T-13 waiting for your approval", "Waiting · T-11 waiting for Mina's approval" and "Sleeping · wakes when @mentioned"
    And the computers read "workstation · group home", "laptop · direct" and "windows · relayed, limited"
    And the accounts read "Personal subscription · takes orders from owner", "Team plan" and "API key"
    And owned AI cards link to "Open terminal" when direct and disable "Open terminal" when relayed or offline, while Mina's AI shows none
    And the cards show a window usage bar where reported
    When the dormant AI wakes to work and two other AIs go offline and hit a limit
    Then those cards update to "Working", "Offline" with "offline" and "Limited"

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

  Scenario: Adding an AI reads connected computers, suggests a free name and sends host.join
    Given the group has no secretary yet and a second computer "laptop" is connected
    When the owner opens "Add AI"
    Then "Claude Code · workstation" is disabled as "Already in this group" and "Codex · workstation" is disabled as "Choose an account type on the Computers page first"
    And "Codex · laptop" is chosen with name "Codex" and "Claude Code · laptop" warns "Not signed in" and suggests "Claude Code 2"
    When the owner toggles "Make this AI the group's secretary" on and off, then checks it and names the AI "Lead Secretary"
    And the owner submits the add-AI dialog
    Then "laptop" receives a signed "host.join" for "codex" named "Lead Secretary" with secretary true
    And the dialog closes and the new AI card "Lead Secretary" appears in the panel

  Scenario: A name conflict when adding an AI is reported clearly
    Given a second computer "laptop" is connected
    When "laptop" refuses "host.join" with "conflict" and the owner submits "Codex · laptop"
    Then the dialog stays open and says "That name is already used in this group."

  Scenario: An unreachable group home from the AI's computer is reported clearly
    Given a second computer "laptop" is connected
    When "laptop" refuses "host.join" with "home_unreachable" and the owner submits "Codex · laptop"
    Then the dialog stays open and says "The AI's computer cannot reach this group's home right now. Try again when both are online."

  Scenario: A lost AI computer and disabled Add AI states explain why
    Given a second computer "laptop" is connected
    When the owner opens "Add AI" and "laptop" disconnects before the owner submits
    Then no command is sent to "workstation" and the dialog says "Not sent: the AI's computer cannot be reached."
    When the owner closes the dialog and becomes a non-admin member
    Then the member adds an AI from their connected computer
    When the group home disconnects
    Then the "Add AI" button is disabled with reason "Cannot reach the group home."
