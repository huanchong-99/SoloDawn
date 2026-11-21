Feature: Pausing and resuming a group in the home
  An admin pauses a group to stop running turns and hold inbox items on hosts,
  and resumes it to let active work continue through row 24 of the task
  machine.

  Scenario: Pausing sets the group pause, mentions still queue while paused, and resuming wakes active wait-free work
    Given a group for pause and resume with an owner, an admin, a member and three working AIs
    And an active pause-test task "working" at step "local_check" assigned to "ai-one"
    And an active pause-test task "waiting-approval" at step "local_check" assigned to "ai-two" waiting for an approval
    And an active pause-test task "merging" at step "merge" assigned to "ai-three" for commit "c1"
    When "the owner" sends group "pause"
    Then the pause home answers "ok"
    And the group is paused by "owner" with system message "group_paused"
    When "the owner" posts a text mentioning "ai-one" while the group is paused
    Then the pause home answers "ok"
    And the pause home has queued "mention ai-one"
    When "the admin" sends group "resume"
    Then the pause home answers "ok"
    And the group is unpaused with system message "group_resumed" by "admin"
    And the pause home has queued "resume ai-one task working"

  Scenario: Resuming queues review for active review tasks unless already queued, and skips paused review tasks
    Given a group for pause and resume with an owner, an admin, a member and three working AIs
    And "the owner" has paused the group
    And an active pause-test task "needs-review" at step "review" assigned to "ai-one" and reviewed by "ai-two" for commit "c1"
    And an active pause-test task "queued-review" at step "review" assigned to "ai-two" and reviewed by "ai-three" for commit "c2" with its review already queued
    And an active pause-test task "blocked-review" at step "review" assigned to "ai-three" and reviewed by "ai-one" for commit "c3" waiting for an approval
    And a paused pause-test task "paused-review" at step "review" assigned to "ai-one" and reviewed by "ai-three" for commit "c4"
    When "the owner" sends group "resume"
    Then the pause home answers "ok"
    And the pause home has queued "review ai-one task blocked-review c3, resume ai-one task needs-review, review ai-two task needs-review c1, resume ai-two task queued-review"

  Scenario: Pausing a paused group and resuming an unpaused group change nothing
    Given a group for pause and resume with an owner, an admin, a member and three working AIs
    And an active pause-test task "working" at step "local_check" assigned to "ai-one"
    When "the owner" sends group "resume"
    Then the pause home answers "ok" with no rows and no items
    When "the owner" sends group "pause"
    Then the pause home answers "ok"
    When "the admin" sends group "pause"
    Then the pause home answers "ok" with no rows and no items

  Scenario Outline: Only an admin may pause or resume a group
    Given a group for pause and resume with an owner, an admin, a member and three working AIs
    When "<who>" sends group "<op>"
    Then the pause home answers "forbidden" with no rows and no items

    Examples:
      | who           | op     |
      | the member    | pause  |
      | the member    | resume |
      | the secretary | pause  |
      | the secretary | resume |
      | ai-one        | pause  |
      | ai-one        | resume |
