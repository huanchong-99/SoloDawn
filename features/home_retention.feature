Feature: Daily retention in the group home
  Retaining a node's store expires messages and finished tasks older than the
  retention_days setting while keeping pending cards, notes and review
  messages, and raises a group's purge mark only once deletion markers age out
  themselves.

  Scenario: Older messages expire after daily retention while newer messages and pending cards remain (A2.5)
    Given a group home with retention set to 7 days
    And 8 days ago the group received the text message "Eight days ago", the pending decision card "Still waiting" and the decided decision card "Already answered"
    And 6 days ago the group received the text message "Six days ago"
    When daily retention runs today
    Then the store holds the messages "Still waiting, Six days ago"
    And a cold sync of the group delivers "Still waiting, Six days ago"

  Scenario: Notes and review messages survive retention, and aged deletion markers raise the purge mark (A2.5, A5.14)
    Given a group home with retention set to 7 days
    And 8 days ago the group received the note "Keep paths canonical" and the review message "Score 92"
    And a note "Obsolete note" from 10 days ago was forgotten 8 days ago
    And a note "Recent forget" from 10 days ago was forgotten 6 days ago
    When daily retention runs today
    Then the store still holds the note "Keep paths canonical" and the review message "Score 92"
    And the deletion marker for "Obsolete note" is gone while the deletion marker for "Recent forget" remains
    And syncing from before the removed marker for "Obsolete note" resets the client while syncing from that marker sends a delta without reset
