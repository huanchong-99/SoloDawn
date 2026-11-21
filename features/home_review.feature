Feature: Checks, submissions and reviews in the home
  The assignee's host reports local checks, submissions and hosted checks; the reviewer AI posts
  scored reviews. A task accepts a submission only for the commit that passed local checks, moves
  through hosted checks when the group has a remote, sends work back on up to five failed reviews,
  stops on the sixth failed review, and moves to merge when a review passes.

  Scenario: Submissions require passing local checks on the submitted commit
    Given a review home without a remote
    When the assignee's host submits commit "c1"
    Then the review home answers "conflict"
    When the assignee's host reports "failing" local checks on commit "c1"
    And the assignee's host submits commit "c1"
    Then the review home answers "conflict"
    When the assignee's host reports "passing" local checks on commit "c1"
    And the assignee's host submits commit "c2"
    Then the review home answers "conflict"
    When the assignee's host submits commit "c1"
    Then the review home answers ok
    And the review task is "active" at step "review" for commit "c1"
    And the review home has queued "review reviewer c1"

  Scenario: Hosted checks gate review with a remote and pass leads to merge
    Given a review home with a GitHub remote
    And the assignee's host has reported passing local checks on commit "c1"
    When the assignee's host submits commit "c1" on pull request 31
    Then the review home answers ok
    And the review task is "active" at step "cloud_check" for commit "c1"
    When the assignee's host reports "passing" hosted checks on commit "c1"
    Then the review home answers ok
    And the review task is "active" at step "review" for commit "c1"
    And the review home has queued "review reviewer c1"
    When "reviewer" posts a review of commit "c1" with score 92
    Then the review home answers review passed true in round 1
    And the review task is "active" at step "merge" for commit "c1"
    And the review home has queued "merge assignee c1 pr=31"
    When the assignee's host reports "passing" local checks on commit "c2"
    Then the review home answers "conflict"

  Scenario: Low scores, vetoes, wrong reviewer or commit, and the sixth failed review
    Given a review home without a remote
    And commit "c1" has reached step "review"
    When "assignee" posts a review of commit "c1" with score 95
    Then the review home answers "forbidden"
    When "reviewer" posts a review of commit "c9" with score 95
    Then the review home answers "conflict"
    When "reviewer" posts a review of commit "c1" with score 86
    Then the review home answers review passed false in round 1
    And the review task is "active" at step "local_check" for commit "c1"
    And the latest review row has score 86 and ok false
    And the review home has queued "reviewed assignee"
    When commit "c2" is checked and submitted for review
    And "reviewer" posts a veto review of commit "c2" with score 95
    Then the review home answers review passed false in round 2
    And the latest review row has score 0 and ok false
    And the review task is "active" at step "local_check" for commit "c2"
    And the review home has queued "reviewed assignee"
    When 3 more reviews fail on new commits
    Then the review task is "active" at step "local_check" for commit "c5"
    When commit "c6" is checked and submitted for review
    And "reviewer" posts a review of commit "c6" with score 86
    Then the review home answers review passed false in round 6
    And the review task is "failed" at step "review" for commit "c6"
    And the review home has queued "notice secretary task_failed"
