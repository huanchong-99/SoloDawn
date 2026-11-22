Feature: Daily retention in the group home
  Once a day a group home expires messages and finished tasks older than the
  node's retention_days setting while keeping pending cards, notes, review
  messages and any confirmed brief that still has a queued task. Deletion
  markers raise a group's purge mark only once they age out themselves, and
  finished tasks removed from the store stay in the home's memory until a
  restart.

  Scenario: Older messages expire after daily retention while newer messages and pending cards remain (A2.5)
    Given a group home with retention set to 7 days
    And 8 days ago the group received the text message "Eight days ago", the pending decision card "Still waiting" and the decided decision card "Already answered"
    And 6 days ago the group received the text message "Six days ago"
    When the daily retention job runs today
    Then the store holds the messages "Still waiting, Six days ago"
    And a cold sync of the group delivers "Still waiting, Six days ago"
    And a second retention tick on the same day changes nothing

  Scenario: Notes and review messages survive retention, and aged deletion markers raise the purge mark (A2.5, A5.14)
    Given a group home with retention set to 7 days
    And 8 days ago the group received the note "Keep paths canonical" and the review message "Score 92"
    And a note "Obsolete note" from 10 days ago was forgotten 8 days ago
    And a note "Recent forget" from 10 days ago was forgotten 6 days ago
    When the daily retention job runs today
    Then the store still holds the note "Keep paths canonical" and the review message "Score 92"
    And the deletion marker for "Obsolete note" is gone while the deletion marker for "Recent forget" remains
    And syncing from before the removed marker for "Obsolete note" resets the client while syncing from that marker sends a delta without reset

  Scenario: A confirmed brief survives retention while a task made from it is queued and remains fetchable when released (R3-4)
    Given a group home with retention set to 7 days
    And 8 days ago the owner confirmed a brief with a queued dependent task and a second brief without a queued task
    When the daily retention job runs today
    Then the store still holds the brief with the queued task and has deleted the brief without a queued task
    When the dependency merges and releases the queued task
    Then the host receives the released task's assign item and can fetch its confirmed brief
    When another day passes and the daily retention job runs again
    Then the store has deleted the brief that had the queued task

  Scenario: Finished tasks older than retention leave the store at once and stay in memory until a restart
    Given a group home with retention set to 7 days
    And 8 days ago the group had an active task, a merged task and a cancelled task
    When the daily retention job runs today
    Then the store holds only the active task
    And the running home still holds all 3 tasks in memory
    When the home restarts on the same store
    Then the running home holds only the active task in memory and keeps the task counter at 3
