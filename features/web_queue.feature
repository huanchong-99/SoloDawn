@web
Feature: A message that waits for a busy AI, and letting it go first
  A person's message that waits for an AI busy with a task says so to its author,
  who can let it go first (R-72); waiting is the default and needs nothing. What the message shows
  afterwards follows the rows: the task is pre-empted, and continues by itself.

  Background:
    Given Alex in "Website", where Codex works on T-12 since before anything Alex writes

  Scenario: The author lets a waiting message go first
    When Alex writes to Codex
    Then the message says that Codex is busy with T-12 and offers "Go first, pausing T-12"
    When Alex presses "Go first, pausing T-12"
    Then the home receives "jump" for that message, and it reads "Codex takes this next." without a button
    When Codex takes the message and T-12 is pre-empted
    Then the message says that T-12 is paused while Codex takes it
    When T-12 continues
    Then the message says nothing of it any more

  Scenario: A reviewer's review is interrupted, not a task of its own
    Given Codex reviews T-12
    When Alex writes to Codex
    Then the message offers "Go first, interrupting the review"

  Scenario: Every waiting message can go first
    When Alex writes to Codex twice
    Then both messages offer "Go first, pausing T-12"

  Scenario: A message that waits for two AIs goes first with both
    Given Claude works on T-13
    When Alex writes to Codex and Claude
    Then both lines say that the message waits, and one choice offers "Go first, interrupting T-13 and T-12"
    When Alex presses "Go first, interrupting T-13 and T-12"
    Then the home receives one "jump", and both AIs take the message next
    When Codex takes the message and T-12 is pre-empted
    Then the message says that T-12 is paused and that Claude takes it next

  Scenario: A message in the thread of the task that the AI works on
    Given T-12's card in the main line, its thread open
    When Alex writes to Codex in the thread
    Then the message offers "Go first, interrupting what Codex is doing"
    When Alex presses "Go first, interrupting what Codex is doing"
    Then the home receives "jump" for the message in the thread

  Scenario: The message was taken in the meantime
    Given the home answers "conflict" to a jump
    When Alex writes to Codex
    And Alex presses "Go first, pausing T-12"
    Then the message says nothing of it any more

  Scenario: The home refuses for another reason
    Given the home answers "stale" to a jump
    When Alex writes to Codex
    And Alex presses "Go first, pausing T-12"
    Then the message still offers it, and says why it was refused
    When Codex takes the message
    Then neither is left

  Scenario: Somebody else's message says nothing
    When Mina writes to Codex
    Then no message says that it waits

  Scenario: The home is away
    When Alex writes to Codex
    And the home stops
    Then the choice is disabled
