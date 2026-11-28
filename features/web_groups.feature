@web
Feature: Group list and group cache of the web client
  The web client holds a cache of every group it may read: it applies the rows a home pushes,
  keeps the place it has read to, and draws the group list from what it holds. A cache is never
  an authoritative copy: a group is created, and a message posted, by a command, and shows when
  its rows arrive (planning/flows/sync.md; acceptance tests A2.1, A2.2 and A2.6).

  Background:
    Given a home that holds the group "Website", and a browser of its owner that reads it

  Scenario: The list shows each group with its last message and what waits for the reader
    Given the home also holds the group "Docs", where "Mina" wrote "Section 3 is in." a minute ago
    And a brief in "Website" waits for the reader to confirm it
    And a confirmation in "Website" waits for the reader, the owner of "Codex"
    When the browser has read both groups
    Then the list reads "Docs" with "Mina: Section 3 is in.", then "Website" with "System: Let Codex act on the request from Mina."
    And "Website" shows that 2 cards wait for the reader, and "Docs" shows none

  Scenario: A group created in the dialog shows in another browser without a reload
    Given a second browser of the same person is connected
    When the person creates the group "Handbook" in the dialog of the first browser
    Then the home has made the group that the command's nonce names
    And both browsers list "Handbook" without a reload
    And the first browser has opened its chat

  Scenario: The group list offers to join a group by code
    When the reader opens the group list
    Then "Join a group" beside "New group" opens the join page, which asks for an invitation code and a name

  Scenario: The list shows a task and a notice as the chat words them
    Given the last message of "Website" is the task T-12 "Show connection path", and the home also holds "Docs", whose last message says that "Mina" joined
    When the browser has read both groups
    Then the list reads "System: T-12 Show connection path" under "Website" and "System: Mina joined, invited by Alex." under "Docs"

  Scenario: A group created with nobody else is ready for its people
    Given the person owns a second computer, "desk", which is connected
    When the person creates "Studio" in the dialog with nobody ticked
    Then the home has made "Studio" from its name and the person's name alone
    And its chat opens ready for the first message and for inviting people, without AIs and without a secretary warning
    And "desk" has been told nothing, since "Studio" is on a computer of the person

  Scenario: A computer that connects later is told the home of a group on another person's computer
    Given the browser also reads "Docs", homed on a computer of "Mina"
    When the person's computer "desk" connects, then "laptop"
    Then "desk" and "laptop" have each been told once which computer is the home of "Docs"
    And neither has been told the home of "Website", which is on a computer of the person

  Scenario: A group known from a computer of the person shows while its home is away
    Given "desk", a computer of the person, knows that "Docs" is homed on a computer out of reach
    When "desk" connects
    Then the list shows "Docs" with "cannot reach the group home" and no word of its messages
    And its chat says neither its home nor how many members it has
    When that computer connects, with "Mina: Section 3 is in." in "Docs"
    Then the list shows "Docs" with "Mina: Section 3 is in."

  Scenario: A group made with an invitation code shows the code before the dialog closes
    When the person ticks "Also make an invitation code", chooses 7 days and creates the group "Notes"
    Then the home has made "Notes" and then a code for 168 hours
    And the dialog shows the code with "Copy code", and its one button reads "Open the group"
    When the person presses "Open the group"
    Then the chat of "Notes" opens

  Scenario: A code the home refuses after making the group is said in the dialog
    When the person ticks "Also make an invitation code", creates the group "Notes", and the home refuses the code
    Then the dialog says why the code was not made, and not that someone could not be added
    And its one button reads "Open the group"

  Scenario: The dialog sends to the computer that was chosen, whatever happens to the others
    Given the person owns a second computer, "laptop", which is connected
    When the person chooses "laptop" in the dialog, the first computer goes away, and they create the group "Notes"
    Then "laptop" has made the group "Notes", and the first computer was sent no command

  Scenario: A group is not created on a computer that went away
    Given the person owns a second computer, "laptop", which is connected
    When the person chooses "laptop" in the dialog, "laptop" goes away, and they press create
    Then no command is sent, and the dialog says "Not sent: the group home cannot be reached."
    When the person presses create again
    Then the computer that is left has made the group "Notes"

  Scenario: A computer that connects while the dialog is open is the one chosen
    Given the dialog is open while no computer is connected
    When the computer connects and the person creates the group "Notes"
    Then the computer has made the group "Notes", and the dialog says nothing of a home out of reach

  Scenario: A browser that was away receives exactly the messages it missed, in order
    Given the browser has read the group up to its last row
    When the connection is lost, 20 messages are written, and the browser connects again
    Then the browser asks for the rows after the last one it applied
    And it is not told to reset
    And its main line ends with exactly those 20 messages, in the order they were written

  Scenario: After 1500 missed rows the browser starts again from the newest 50
    Given the browser has read the group up to its last row
    When the connection is lost, 1500 messages are written, and the browser connects again
    Then the browser is told to reset and holds the newest 50 messages of the main line
    When the reader scrolls back twice
    Then the browser has asked for the page before the oldest message it held, and then for the one before that
    And its main line holds the newest 150 messages in the order they were written

  Scenario: A card that is still pending is there after a cold start, however old
    Given a decision in "Website" waits for the reader, and 80 messages were written after it
    When a browser that holds nothing reads the group
    Then the decision waits for the reader in that browser
    And its main line holds the decision and the newest 50 messages
    When the reader of that browser scrolls back
    Then the browser asks for the page before the oldest of those 50, not for the one before the card
    And its main line holds the decision and all 80 messages

  Scenario: A reset that breaks off after its first batch starts again from nothing
    Given the browser has read the group up to its last row
    When 1500 messages are written while it is away, and its next read breaks off after the first batch
    Then the browser holds rows of that batch and its place is 0
    When the browser connects again
    Then it asks for the group from 0

  Scenario: With the home stopped the group is read-only, and reading goes on when the home returns
    Given the browser has read the group up to its last row
    And the reader has opened the chat of "Website"
    When the home stops
    Then the chat says "Cannot reach the group home. What is already here can still be read."
    And the composer takes no text, and the list marks the group as one whose home cannot be reached
    And the messages read before are still shown
    When the home returns with a message written meanwhile
    Then the browser asks for the rows after the last one it applied, and shows that message
    And the composer takes text again

  Scenario: A row deleted on purpose does not come back with an older page
    Given the browser has read the group up to its last row
    When a message is deleted on purpose, and a page that still holds it arrives afterwards
    Then the browser keeps the marker and does not show the message

  Scenario: The create-group dialog follows by default
    Given the owner's computer "desk" has Claude Code and Codex with defaults
    When the owner ticks Claude Code and Codex in the create-group dialog
    Then each has the switch on
    When the owner turns Codex's switch off and creates the group
    Then Claude Code's host.join carries follow, and Codex's carries its four fields and no follow

  Scenario: Choosing a folder for one AI fills the others on that computer
    Given the owner's computer "desk" has Claude Code and Codex, and "laptop" has Claude Code
    When the owner ticks all three in the create-group dialog
    Then "Create group" is disabled and explains that Claude Code needs a folder
    When the owner chooses a folder for Claude Code on "desk"
    Then Codex on "desk" uses that folder too, "laptop" still has no folder, and the reason names the AI on "laptop"
    When the owner chooses a folder for the AI on "laptop"
    Then "Create group" is enabled without a hint
