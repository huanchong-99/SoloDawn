@web
Feature: What a person does with a task: answer in its thread, stop it, carry it on, give it to another AI
  A person writes in a task's thread with the composer inside it (planning/contracts/commands.md
  1.3; planning/product-rules.md R-33, R-60). An admin pauses, resumes, reassigns and cancels a
  task from its card (planning/contracts/task-machine.md section 3, rows 11 to 13, 15 and 22). The
  card says why a task stands still and what moves it on. A refusal is shown and changes nothing.

  Background:
    Given the chat of "Website" open in a browser of Alex, its owner, with Codex on T-12 and Claude Code as its reviewer

  Scenario: A person answers an AI's question in the thread
    Given Codex asked "@Alex keep the field?" in the thread of T-12
    When Alex opens the thread of T-12
    Then its composer is named "Message in T-12's thread" and says "Write in T-12's thread, @ an AI to reach it"
    When Alex types "@" there
    Then it offers "Codex", "Claude Code", "Alex" and "Secretary" in this order
    When Alex types "Yes, keep it." there
    Then it says "@ an AI if one should answer. Otherwise this just stays in the thread."
    When Alex folds the thread and opens it again
    Then its composer still holds "Yes, keep it."
    When Alex types "@Codex yes, keep it." there and sends it
    Then the home receives that text in the thread of T-12 with a mention of Codex
    And the thread shows "@Codex yes, keep it.", its composer is empty and the main line has no such message

  Scenario: A reply carries on a task that the round limit paused
    Given T-12 is paused after 10 rounds
    Then its card says "The AIs went 10 rounds without a person. Reply in its thread to carry on." with "Reply" and "Resume"
    When Alex presses "Reply"
    Then the thread of T-12 is open and focus is in its composer
    When Alex types "Carry on." there
    Then it says "Sending this carries the task on."

  Scenario: Away from the home the thread takes no text, and only the main composer says why
    When Alex opens the thread of T-12
    And the home stops
    Then both composers take no text and one notice says why
    And the menu of T-12 offers nothing that can be pressed

  Scenario: An admin pauses a task and resumes it
    Then the menu of T-12 offers "Pause", "Reassign" and "Cancel task", and goes when the page scrolls
    When Alex picks "Pause"
    Then the home receives "pause" for T-12 and focus is on its menu button
    When T-12 is paused by hand
    Then its card says "Paused by hand. Its AIs stay stopped until it is resumed." with "Resume"
    And the menu of T-12 offers "Resume", "Reassign" and "Cancel task"
    When Alex presses "Resume"
    Then the home receives "resume" for T-12

  Scenario: A cancel takes two steps
    When Alex picks "Cancel task"
    Then the card asks "Cancel T-12 for good?", says what that does, and nothing is sent
    When Alex presses "Keep it"
    Then the question is gone, nothing is sent and focus is on the menu button
    When Alex picks "Cancel task" again
    And Alex presses "Cancel task"
    Then the home receives "cancel" for T-12 and focus is on its thread
    When T-12 is cancelled
    Then its card has no menu

  Scenario: An admin gives a task to another AI
    When Alex picks "Reassign"
    Then the form names Codex to do it and Claude Code to review it, and cannot be sent yet
    When Alex chooses "Claude Code · workstation" to do it
    Then the form says "One AI cannot both do the work and review it."
    When Alex chooses "Qwen Code · mina-mac · Offline" to do it
    Then the form says that Codex stops and that Mina is asked first
    When Alex presses "Reassign"
    Then the home receives "assign" for T-12 with Qwen Code as its assignee and no reviewer

  Scenario: What others change while the form is open stays theirs
    When Alex picks "Reassign"
    And the secretary gives T-12 to Qwen Code, and its review to Codex
    Then the form names Qwen Code to do it and Codex to review it, and cannot be sent yet
    When Alex chooses "Claude Code · workstation" to review it
    And the secretary gives T-12 back to Codex
    And Alex presses "Reassign"
    Then the home receives "assign" for T-12 with Claude Code as its reviewer and no assignee

  Scenario: A task without a reviewer is given one, and one with a merge conflict goes back
    Given T-12 waits for a reviewer
    Then its card says "Ready for review, and nobody is named to do it. The secretary or an admin names a reviewer." with "Name a reviewer"
    When Alex presses "Name a reviewer"
    And Alex chooses "Claude Code · workstation" to review it
    And Alex presses "Reassign"
    Then the home receives "assign" for T-12 with Claude Code as its reviewer and no assignee
    When T-12 has a merge conflict
    And Alex presses "Hand back to Codex"
    Then the home receives "assign" for T-12 with Codex as its assignee

  Scenario: A stopped task is resumed, and a refusal is shown
    Given T-12 stopped after 3 failures
    Then its card says "Its AIs stay stopped until you resume it or give it to another AI. The count then starts again." with "Resume"
    When the home answers "conflict" and Alex presses "Resume"
    Then the card says "Nothing changed: the task or its AIs moved on in the meantime." and still offers "Resume"
    When the home answers "forbidden" and Alex presses "Resume"
    Then the card says "Not sent: this device may not do that." and still offers "Resume"
    When the home answers "paused" and Alex confirms a cancel
    Then the question says "Nothing changed: the task or its AIs moved on in the meantime."
    When T-12 is carried on
    Then the refusal is gone

  Scenario: A question goes away when another person was faster
    When Alex picks "Reassign"
    And T-12 enters "merge"
    Then the form is gone and the menu of T-12 offers "Cancel task"
    When T-12 has a merge conflict
    Then the form stays away
    When T-12 enters "merge" again
    And Alex picks "Cancel task"
    Then the card also says "It has passed review and may be merging right now."

  Scenario: A pre-empted task carries on by itself
    Given T-12 is pre-empted
    Then its card says "A message jumped the queue. It carries on by itself afterwards." without a button

  Scenario: The owner of the AI said no
    Given Mina denied that Qwen Code takes T-12
    Then its card says "Mina said no to Qwen Code here. The secretary or an admin gives the role to another AI." with "Reassign"
    When Alex presses "Reassign"
    Then the form names Qwen Code to do it
    When Mina gives T-12 to Qwen Code all the same
    Then the card of T-12 says nothing of it

  Scenario: A member who is no admin sees why, and writes in the thread
    Given Alex is a member and T-12 stopped after 3 failures
    Then its card says "Its AIs stay stopped until an admin resumes it or gives it to another AI." without a button
    And the card of T-12 has no menu
    When Alex opens the thread of T-12
    Then its composer takes text
