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
    When the browser has read both groups
    Then the list reads "Docs" with "Mina: Section 3 is in.", then "Website" with "Secretary: Show connection paths"
    And "Website" shows that 1 card waits for the reader, and "Docs" shows none

  Scenario: A group created in the dialog shows in another browser without a reload
    Given a second browser of the same person is connected
    When the person creates the group "Handbook" in the dialog of the first browser
    Then the home has made the group that the command's nonce names
    And both browsers list "Handbook" without a reload
    And the first browser has opened its chat

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
    Then the browser has asked for the newest page and then for the page before its lowest message
    And its main line holds the newest 100 messages in the order they were written

  Scenario: A card that is still pending is there after a cold start, however old
    Given a decision in "Website" waits for the reader, and 80 messages were written after it
    When a browser that holds nothing reads the group
    Then the decision waits for the reader in that browser
    And its main line holds the decision and the newest 50 messages

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
