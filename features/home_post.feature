Feature: Messages, notes and deletion markers in the home
  People and AIs post messages in the main line and in task threads, write context notes, people
  edit them in place, and admins delete notes or review messages with gone markers. Connected browsers receive live pushes of
  messages, thread summaries and deletion markers, and the host receives the inbox items that wake
  each AI.

  Scenario: A message posted on one browser reaches a second connected browser and wakes the right AI
    Given a group home with two connected browsers of its owner, a secretary and two working AIs on its host
    When the first browser posts the text "Please check the relay indicator." in the main line
    Then the second browser receives that "text" message in a live push
    And the host receives "mention secretary" for that message
    When "ai-one" posts the progress "Inspecting the path state." in the thread of task 1 mentioning "ai-two"
    Then the second browser receives that "progress" message and task 1 with 1 thread message
    And the host receives "mention ai-two task 1" for that message

  Scenario: Completion reports update the task thread
    Given a group home with two connected browsers of its owner, a secretary and two working AIs on its host
    When "ai-one" posts a completion report for commit "9f2c1ab" in the thread of task 1
    Then the second browser receives that "report" message and task 1 with 1 thread message

  Scenario: Notes and review messages survive retention, and forget removes them with gone markers
    Given a group home with two connected browsers of its owner, a secretary and two working AIs on its host
    And the group holds an old text message, an old review message and an old note
    When 8 days pass and daily retention runs for a 7-day window
    Then the old text message is gone from the store while the old review message and the old note remain
    When the first browser forgets the old note
    Then the second browser receives a gone marker for that "note"
    When the first browser forgets the old review message
    Then the second browser receives a gone marker for that "msg"

  Scenario: A member edits a note in place, and no AI and no edit of a forgotten note can
    Given a group home with two connected browsers of its owner, a secretary and two working AIs on its host
    And the group holds an old text message, an old review message and an old note
    When the non-admin member edits the old note to "Path events carry the connection id and the path."
    Then the second browser receives the old note with that text, in its place, by the non-admin member
    When "ai-one" tries to edit the old note
    Then the post home answers "forbidden"
    When the first browser forgets the old note
    Then the second browser receives a gone marker for that "note"
    When the non-admin member tries to edit the old note
    Then the post home answers "not_found"

  Scenario Outline: Refusals on posts and forget leave the group unchanged
    Given a group home with two connected browsers of its owner, a secretary and two working AIs on its host
    And the group holds an old text message, an old review message and an old note
    When <attempt>
    Then the post home answers "<code>"

    Examples:
      | attempt                                                                | code        |
      | a non-admin member tries to forget the old note                        | forbidden   |
      | the first browser tries to forget the old text message                 | conflict    |
      | "ai-two" tries to post a completion report in the thread of task 1     | forbidden   |
      | "ai-one" tries to post progress in the main line                       | bad_request |
