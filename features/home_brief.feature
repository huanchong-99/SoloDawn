Feature: Briefs and their confirmation in the home
  The secretary posts a brief; its requester or an admin confirms it or sends it back. Confirming
  creates the brief's tasks through row 1 of the task machine. The group has an owner, who is an
  admin and the requester of every brief, a secretary, and three working AIs: "ai-one", "ai-two"
  and "ai-three". The brief has two tasks: task 1 for "ai-one", reviewed by "ai-two", and task 2
  for "ai-two", reviewed by "ai-one", which depends on task 1.

  Scenario: Nothing starts before the brief is confirmed
    Given a group home with a secretary and three working AIs
    When the secretary posts the brief
    Then the home answers ok
    And the brief is "pending"
    And the home holds no task and has queued no item

  Scenario: A confirm sent twice creates the tasks once
    Given a group home with a secretary and three working AIs
    And the secretary has posted the brief
    When the owner confirms the brief
    And the owner sends the same confirm again
    Then the home answers ok to both
    And the brief is "confirmed"
    And the home holds 2 tasks
    And task 1 is "active" and task 2 is "queued" until task 1 is merged
    And the home has queued "assign ai-one task 1, confirmed secretary"

  Scenario: A task that depends on another starts only after that one merged
    Given a group home with a secretary and three working AIs
    And the secretary has posted the brief
    And the owner has confirmed the brief
    When the secretary gives task 2 to "ai-three" while it is queued
    Then task 2 is "queued" and the home has queued nothing that makes a worktree
    When task 1 is merged
    Then task 2 is "active" and the home has queued "assign ai-three task 2"

  Scenario: Sending a brief back tells the secretary, and a new brief supersedes it
    Given a group home with a secretary and three working AIs
    And the secretary has posted the brief
    When the owner sends the brief back with the note "Include the relay path."
    Then the brief is "revise" with the note "Include the relay path."
    And the home has queued "confirmed secretary"
    And the home holds no task
    When the secretary posts a brief that replaces it
    Then the home answers ok
    And the brief is "superseded" with the note "Include the relay path."

  Scenario Outline: Only the requester or an admin decides a pending brief
    Given a group home with a secretary and three working AIs
    And the secretary has posted the brief
    When <who> sends "<action>" on the brief
    Then the home answers "<code>"
    And the brief is "pending"
    And the home holds no task and has queued no item

    Examples:
      | who                                 | action  | code      |
      | the secretary, for itself           | confirm | forbidden |
      | the owner's laptop, for "ai-one"    | confirm | forbidden |
      | the owner's laptop, for "ai-one"    | revise  | forbidden |
      | the owner, with an arg              | confirm | bad_request |
