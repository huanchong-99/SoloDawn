Feature: Pausing and resuming a group in the home
  The home half of A4.7: an admin pauses a group, mentions made while it is
  paused are still queued for the hosts to hold, and the resume wakes active
  work through row 24 of the task machine, but not a task that waits for an
  approval nor one at step merge.

  Scenario: Pausing queues mentions, and resuming wakes only work with no wait
    Given a group for pause and resume with an owner, an admin and three working AIs
    And an active pause-test task "working" at step "local_check" assigned to "ai-one"
    And an active pause-test task "waiting-approval" at step "local_check" assigned to "ai-two" waiting for an approval
    And an active pause-test task "merging" at step "merge" assigned to "ai-three"
    When "the owner" sends group "pause"
    Then the group is paused by "owner" with system message "group_paused"
    When "the owner" posts a text mentioning "ai-one" while the group is paused
    Then the pause home has queued "mention ai-one"
    When "the admin" sends group "resume"
    Then the group is unpaused with system message "group_resumed" by "admin"
    And the pause home has queued "resume ai-one task working"
