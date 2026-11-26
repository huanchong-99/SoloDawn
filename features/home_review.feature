Feature: Submissions and reviews in the home
  The assignee submits its task with a post of kind report in the task's thread; the reviewer AI
  posts a scored review of that report. A task sends work back on up to five failed reviews, stops
  on the sixth failed review, and is done when a review passes. The secretary then posts the
  summary of the done task.

  Scenario: A submission is the assignee's report, and a newer report replaces it under review
    Given a review home
    When the reviewer submits report "r0"
    Then the review home answers "forbidden"
    When the assignee submits report "r1"
    Then the review home answers ok
    And the review task is "active" at step "review" for report "r1"
    And the review home has queued "review reviewer r1"
    When the assignee submits report "r2"
    Then the review home answers ok
    And the review task is "active" at step "review" for report "r2"
    And the review home has queued "review reviewer r2"
    When "reviewer" posts a review of report "r1" with score 95
    Then the review home answers "conflict"

  Scenario: A passed review makes the task done, and the secretary's summary records it (A5.12)
    Given a review home
    And report "r1" has reached step "review"
    And 72 minutes pass
    When "reviewer" posts a review of report "r1" with score 92
    Then the review home answers review passed true in round 1
    And the review task is "done" at step "review" for report "r1"
    And the thread of the review task ends with system "task_done"
    And the review home has queued "notice secretary task_done"
    When the assignee submits report "r2"
    Then the review home answers "conflict"
    When the secretary posts summary "Path changes are reported and shown." for the review task
    Then the review home answers ok
    And the summary appears in the thread and the main line with score 92, 1 round and 72 minutes

  Scenario: Low scores, vetoes, wrong reviewer or report, and the sixth failed review (A5.4)
    Given a review home
    And report "r1" has reached step "review"
    When "assignee" posts a review of report "r1" with score 95
    Then the review home answers "forbidden"
    When "reviewer" posts a review of report "r9" with score 95
    Then the review home answers "conflict"
    When "reviewer" posts a review of report "r1" with score 86
    Then the review home answers review passed false in round 1
    And the review task is "active" at step "work" for report "r1"
    And the latest review row has score 86 and ok false
    And the review home has queued "reviewed assignee"
    When report "r2" is submitted for review
    And "reviewer" posts a veto review of report "r2" with score 95
    Then the review home answers review passed false in round 2
    And the latest review row has score 0 and ok false
    And the review task is "active" at step "work" for report "r2"
    And the review home has queued "reviewed assignee"
    When 3 more reviews fail on new reports
    Then the review task is "active" at step "work" for report "r5"
    When report "r6" is submitted for review
    And "reviewer" posts a review of report "r6" with score 86
    Then the review home answers review passed false in round 6
    And the review task is "failed" at step "review" for report "r6"
    And the review home has queued "notice secretary task_failed"
