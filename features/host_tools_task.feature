Feature: Host task and review MCP tools
  Scenario: A direct request starts a task from the group session and moves work to a task session (A5.7 host half)
    Given a working AI group session with an owner request in the main line
    Then the group session lists "start_task" and none of "push_branch, run_checks, submit_task"
    When the AI calls start_task with title "Fix path event" and plan "Add path status and tests."
    Then start_task returns task "T-1" with moved true and sends a direct task start
    When the host creates the task worktree and admits the task session at levels 1 and 2
    Then level 1 lists none of "push_branch, run_checks, submit_task" and level 2 lists all three without "start_task"

  Scenario: run_checks and submit_task enforce a clean, passing, unchanged commit without a remote (A5.3)
    Given a level-2 task session on a local repository with a project check
    When the task worktree has an uncommitted file and the AI calls run_checks
    Then the task tool fails with code "conflict"
    And no task checked command was sent to the home
    When the AI commits a failing change with a test file and runs run_checks then submit_task
    Then run_checks reports ok false and submit_task fails with code "conflict"
    When the AI commits a passing fix and runs run_checks then submit_task with summary "Fixed locally."
    Then run_checks reports ok true and submit_task sends report and task submitted without a pull request
    When the AI commits one more change without re-running checks and calls submit_task
    Then the task tool fails with code "conflict"

  Scenario: Remote task pushes its branch, opens a pull request, reports hosted checks, and is reviewed (A5.3 remote)
    Given a level-2 task session with a bare Git origin and a fake gh
    When the AI commits a passing change with a test file and calls push_branch
    Then push_branch pushes the assigned branch to origin and returns its commit
    When the AI runs run_checks and submit_task with summary "Open path PR."
    Then submit_task opens pull request 31, sends report and task submitted, and reports task cloud
    When the reviewer opens the review checkout for that commit and submits a review with score 94
    Then submit_review sends a signed review post for that commit and returns passed true in round 1
