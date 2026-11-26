Feature: Host task and review MCP tools
  Scenario: A direct request starts a task from the group session and moves work to a task session (A5.7 host half)
    Given a working AI group session with an owner request in the main line
    Then the group session lists "start_task" and none of "submit_task, submit_review"
    When the AI calls start_task with title "Fix path event" and plan "Add path status and tests."
    Then start_task returns task "T-1" with moved true and sends a direct task start
    When the host admits the task session of that task at levels 1 and 3
    Then at both levels the task session lists "submit_task" and none of "start_task, submit_review"

  Scenario: start_task while the AI's folder is busy answers queued (A5.20 host half)
    Given a working AI group session with an owner request in the main line
    And the home queues a new task of that AI behind another task in its folder
    When the AI calls start_task with title "Fix path event" and plan "Add path status and tests."
    Then start_task returns task "T-1" with queued true and sends a direct task start

  Scenario: submit_task posts its summary as the task's report and nothing else (A5.22 host half)
    Given a task session of "Claude Code" in its workspace for an active task at step work
    When the AI calls submit_task with summary "Key: sk-proj-EXAMPLEexampleEXAMPLEexample_EXAMPLE-00"
    Then the task tool fails with code "secret" and nothing was sent to the home
    When the AI calls submit_task with summary "Ran go test ./...: 42 passed. Commit 9f2c1ab on branch t12-path-events."
    Then submit_task returns ok and the home received only that summary, as a post of kind report in the task's thread
