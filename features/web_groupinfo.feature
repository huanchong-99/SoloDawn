@web
Feature: Group info panel and project memory

  Background:
    Given a project group "Website" homed on "workstation", open in a browser of its owner in English

  Scenario: Project memory lists notes and the review ledger, and forgetting removes an entry within 1 second
    Given the home holds two notes and a scored review whose task was removed by retention
    Then no memory fetch has been sent and the group info panel folds "Project memory" with "Kept past retention"
    When the owner opens "Project memory"
    Then the browser fetches kind "note" and kind "review" from 0 with limit 50
    And the notes list shows "Keep `rtt_ms` empty on relay." on "T-12" by "Alex" and "Path events include the connection id." by "Secretary"
    And the review ledger shows "T-12", "86 / 90", "Show connection paths", "Claude Code · Round 1" and remark "Missing relay path" at "path.go:88"
    When the owner presses "Delete note: Keep rtt_ms empty on relay." and cancels the confirmation
    Then no forget command was sent and the confirmation is gone
    When the owner presses "Delete note: Keep rtt_ms empty on relay." and confirms "Delete"
    Then the home receives a signed "forget" for "note" and that note disappears within 1 second with focus on the notes heading
    When the owner presses "Delete review: T-12 Show connection paths" and confirms "Delete"
    Then the home receives a signed "forget" for "msg" and the review ledger reads "No scored reviews yet."
    When another device forgets the remaining note
    Then the notes list drops it within 1 second and reads "No notes yet."

  Scenario: Older pages of project memory load on demand
    Given the home holds 65 notes
    When the owner opens "Project memory"
    Then the notes list shows the newest 50 notes in creation order and offers "Load earlier"
    When the owner presses "Load earlier"
    Then all 65 notes are shown in creation order and "Load earlier" is gone

  Scenario: Offline read-only, non-admin member, refusal and narrow-screen Escape states
    Given the home holds one note and one scored review and the owner has opened "Project memory"
    When the home disconnects
    Then the delete buttons are disabled with reason "Cannot reach the group home." while the note and review stay readable
    When the home reconnects and the reader becomes a non-admin member
    Then the delete buttons are disabled with reason "Only an admin can delete project memory."
    When the reader becomes an admin again and the home refuses "forget" with "forbidden"
    And the owner confirms deleting the note
    Then the dialog stays open and says "Not sent: this device may not do that."
    When the chat is reopened at 820 px wide, the owner opens "People and details" and "Project memory", opens delete on the note and presses Escape
    Then the confirmation dialog closes and the side panel stays open
