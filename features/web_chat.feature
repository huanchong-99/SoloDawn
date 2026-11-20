@web
Feature: Chat main line of the web client
  The chat of a group shows its main line, newest at the bottom, under the days the messages
  were written on, and the page before is fetched as the reader scrolls back. A message is written
  in the composer, which completes mentions from the members and tells the writer before the row
  of a message would be too large (planning/contracts/rows.md section 5; acceptance test A2.1).

  Background:
    Given the chat of the group "Website" open in a browser of its owner, set to English

  Scenario: A message written in the composer appears in the main line of both browsers
    Given a second browser of the same person reads the group
    When the person writes "The path is direct." and sends it
    Then the home is sent a post of kind "text" to the main line, signed by this device
    And the message "The path is direct." appears under the name "Alex", marked "You"
    And the second browser holds the message too
    And the composer is empty again

  Scenario: A mention is completed from the members and named in the post
    When the person types "ask @c"
    Then the composer offers "Claude Code" and "Codex"
    When the person moves down once and presses Enter
    Then the composer reads "ask @Codex " and offers nothing
    When the person adds "for the fields" and sends the message
    Then the post names the member "Codex" in its mentions
    And the message shows "@Codex" as a mention

  Scenario: The composer tells the writer before a message is too long
    When the person writes a message of 5830 letters
    Then the composer says nothing about its length
    When the person writes "@Codex " and 5823 letters, which is as many characters
    Then the composer says "Long message: 76% of what one message can hold."
    And the message can be sent
    When the person writes a message of 9000 letters
    Then the composer says "Too long to send: 114% of what one message can hold. Shorten it, or send it in parts."
    And the message cannot be sent

  Scenario: Messages are shown under the day they were written on
    Given messages written yesterday at 23:50 and today at 00:10 and 00:12 by the same member
    Then the main line shows a day that begins with "yesterday" and a day that begins with "today"
    And the last message is joined to the one before it, and the first of today is not

  Scenario: A message shows its mentions and its code as written
    Given "Codex" wrote "@Alex run `go test ./...` first" and mentioned the reader
    Then the message shows the mention "@Alex" and the code "go test ./..."
    And the rest of its text is shown as written

  Scenario: Scrolling back fetches the pages before, one at a time
    Given the group holds 120 messages and the browser has read the newest 50
    When the top of the main line comes into view
    Then the browser fetches pages until the home has no more
    And the main line shows all 120 messages in the order they were written
    And the 50 messages shown before are the same parts of the page as before
    And it says "This is where the group starts."

  Scenario: No page is asked for while the group is being read again
    Given the group holds 120 messages and the browser has read the newest 50
    And the reader has scrolled back to the start, with the top of the main line still in view
    When the home stops, 1500 messages are written, and the home returns
    Then the first page the browser asks for is the one before the oldest message that the new read brought
    And the main line shows the newest messages without a gap

  Scenario: Without the home the top of the main line does not claim to be the start
    Given the group holds 120 messages and the browser has read the newest 50
    When the home stops and the top of the main line comes into view
    Then no page is fetched
    And the top of the main line says "Earlier messages cannot be loaded right now."

  Scenario: What is typed while a message is on its way stays in the composer
    When the person sends "first" and types " and more" before the home has answered
    Then the home is sent the post "first"
    And the composer holds " and more"

  Scenario: The main line is marked as focused by the keyboard only once a key was used
    Then the chat is not marked as used with the keyboard, although its main line has focus
    When the person presses a key
    Then the chat is marked as used with the keyboard
    When the person uses the pointer
    Then the chat is not marked as used with the keyboard any more

  Scenario: Enter sends a message, but not while an input method composes
    When the person types "你好" and presses Enter while the input method composes
    Then nothing is sent
    When the person presses Enter again
    Then the home is sent the post "你好"

  Scenario: A command that the home refuses is told to the writer
    Given the home refuses posts
    When the person writes "hello" and sends it
    Then the composer says "Not sent: the home refused it as too long or malformed."
    And the text stays in the composer
