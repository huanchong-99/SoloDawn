@web
Feature: Member panel and adding an AI to a group
  The side panel of a group's chat shows its AI members, its human members, and a dialog for
  adding a registered AI from one of the owner's connected computers, each AI in a folder of its
  computer; the dialog that creates a group adds people and AIs the same way
  (planning/contracts/rows.md section 2, planning/contracts/commands.md sections 1.1 and 2;
  acceptance tests A2.1, A2.10, A3.1 and A3.4 as seen in the client).

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

  Scenario: The people list shows names and roles, and nothing about being online
    Given the group also has "Mina" as an admin and "Sam" as a member
    Then the people section reads "People · 3"
    And the people list shows "Alex · You · Owner", "Mina · Admin" and "Sam" with no state

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
    Then "Claude Code · workstation" says "In this group · adds another member" and is selectable again and "Codex · workstation" is disabled as "Choose an account type on the Computers page first"
    And "Claude Code · workstation" is initially chosen with name "Claude Code 2" and the owner chooses "Codex · laptop"
    When the owner toggles "Make this AI the group's secretary" on and off, then checks it and names the AI "Lead Secretary"
    And the owner submits the add-AI dialog
    Then "laptop" receives a signed "host.join" for "codex" named "Lead Secretary" with secretary true
    And the dialog closes and the new AI card "Lead Secretary" appears in the panel

  Scenario: The folder list says which AI of the group has a folder, and a folder of another group cannot be chosen
    Given on "workstation" Claude Code of this group works in "/home/dev/website" and an AI of the group "Docs" in "/home/dev/notes"
    When the owner opens "Add AI" with "Claude Code · workstation" chosen and opens its folder list
    Then "/home/dev/website" is chosen and says "Claude Code works here too", and "/home/dev/notes" cannot be chosen and says "Taken by the group Docs"
    When the owner types "/home/dev/notes" as another folder and takes it
    Then the list says "This folder belongs to the group Docs. Choose another one." and the folder stays "/home/dev/website"
    When the owner closes the list
    Then the dialog says the folder is shared with Claude Code and takes one task at a time

  Scenario: A group is created with a person and two AIs, each in its folder, the first AI the secretary
    Given "Mina" is in the group "Website" on "workstation", and a second computer "laptop" is connected
    When the owner opens "New group" from the group list
    Then the dialog offers "Mina" and the four AIs, with "Codex · workstation" and "Claude Code · laptop" disabled and their computers before their reasons
    When the owner ticks "Mina", "Claude Code · workstation" and "Codex · laptop"
    Then "Claude Code" is the secretary, "Codex" is not, each secretary choice is named for its AI, and each AI has the newest folder that no other group has
    When the owner makes "Codex" the secretary, names the group "Studio" and themself "Alex", and creates it
    Then the home makes "Studio", adds Mina, and each computer receives a signed "host.join" with the AI's folder, "Codex" as the secretary
    And the chat of "Studio" opens with Mina and the two AIs

  Scenario: A folder that is another group's, refused by the AI's computer, is reported with that group
    Given a second computer "laptop" is connected
    When "laptop" refuses "host.join" with "conflict" naming the group "Docs" and the owner submits "Codex · laptop"
    Then the dialog stays open and says the folder is the workspace of another group, in the words of the computer that name "Docs"

  Scenario: A name that a member of the group has is refused by the dialog, and nothing is sent
    Given a second computer "laptop" is connected
    When the owner opens "Add AI", chooses "Codex · laptop", names the AI "Claude Code" and submits
    Then the dialog stays open and says "That name is already used in this group." and "laptop" receives no "host.join"
    When the owner names the AI "Helper" and submits
    Then "laptop" receives a signed "host.join" for "codex" named "Helper" and the dialog closes

  Scenario: Two members of a new group cannot have one name, and nothing is sent
    Given the owner is on the group list
    When the owner opens "New group", ticks "Claude Code · workstation", names the group "Studio" and themself "Claude Code", and creates it
    Then the dialog stays open and says "Two members cannot have one name: change one of the names." and the home receives no command
    When the owner names themself "Alex" and creates it
    Then the home makes "Studio" and the chat of "Studio" opens

  Scenario: A group whose second person is refused says where it stopped, and opens all the same
    Given "Mina" and "Sam" are in the group "Website" on "workstation", and the home no longer has a log of "Sam"
    When the owner opens "New group", ticks "Mina", "Sam" and "Claude Code · workstation", names the group "Studio" and themself "Alex", and creates it
    Then the dialog stays open and says "Mina" was "Added", "Sam" was refused with "Not in any of your groups on this computer any more: invite them by link." and "Claude Code" is "Not sent: an earlier step was refused"
    And the dialog says "The group exists, but not everyone could be added. Add them again from its member panel." and its button reads "Open the group" with no "Cancel"
    And the home made "Studio" and added Mina and tried Sam, and "workstation" received no "host.join"
    When the owner presses "Open the group"
    Then the chat of "Studio" opens with Mina and without Sam

  Scenario: A group whose second AI is refused for its folder says so beside it, and the earlier ones stay added
    Given "Mina" is in the group "Website" on "workstation", and "laptop" is connected and refuses "host.join" with "not_found"
    When the owner opens "New group", ticks "Mina", "Claude Code · workstation" and "Codex · laptop", names the group "Studio" and themself "Alex", and creates it
    Then Mina and "Claude Code" are "Added" and "Codex" says "There is no such folder on that computer."
    And both computers received "host.join" and the button reads "Open the group"

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
