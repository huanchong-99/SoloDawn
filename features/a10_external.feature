Feature: External agents take and finish one item without a CLI driver

  @pending
  Scenario: Join and label external members without agent.set (A10.17)
    Given a running external acceptance project
    When the owner joins external member "X" with account "api_key" and tool "ZCode"
    Then external member "X" has state "offline"
    And the node rejects invalid external joins and agent settings
    When the owner relabels X to "Cursor"
    Then the external home rejects a non-owner tool edit
    When the owner joins external member "Y" with account "team" and tool "Other"
    Then host info advertises both external members and the node executable

  @pending
  Scenario: A nonblocking look hands out each mention once (A10.18)
    Given an external acceptance project with X
    Then an external look exits 3
    When the external owner mentions X with "First mention"
    Then an external look exits 0
    And the external handout is a "group" run containing "First mention"
    And the external handout folder is the project clone
    And the external home has 0 inbox items
    And external member "X" has state "working"
    And an external look exits 3
    When the external owner mentions X with "Second mention"
    Then an external look exits 3
    When the external runner ends its current run
    Then external member "X" has state "waiting"
    And an external look exits 0
    And the external handout is a "group" run containing "Second mention"
    When the external runner repeats done for the previous run
    Then external member "X" has state "working"
    And the external home has 0 inbox items
    When the external runner ends its current run
    Then an external look exits 3
    And an external look rejects an unknown member

  @pending
  Scenario: Blocking and timed waits end for the right reason (A10.19)
    Given an external acceptance project with X
    When an external blocking wait starts
    Then external member "X" has state "waiting"
    And the external blocking wait has not exited
    When the external owner mentions X with "Wake the runner"
    Then the external blocking wait exits 0
    And the external handout is a "group" run containing "Wake the runner"
    When the external runner ends its current run
    Then an empty external wait times out after two seconds
    When an external blocking wait starts
    And the external owner removes X
    Then the external blocking wait exits 1

  @pending @not-windows
  Scenario Outline: A runner starts exactly one agent for each item (A10.20)
    Given an external acceptance project with X
    And the external owner mentions X with "Setup check: answer here in one line, then finish."
    Then the external home has 1 inbox items
    And external member "X" has state "offline"
    When the "<runner>" runs 1 queued external items
    Then X posts "Setup check answered" in the external project
    And external member "X" has state "waiting"

    Examples:
      | runner    |
      | loop      |
      | scheduler |

  @pending @not-windows
  Scenario Outline: A runner starts queued items one after the other (A10.20)
    Given an external acceptance project with X
    And the external owner mentions X with "First external mention"
    And the external owner mentions X with "Second external mention"
    Then the external home has 2 inbox items
    And external member "X" has state "offline"
    When the "<runner>" runs 2 queued external items
    Then X posts "First external answer" in the external project
    And X posts "Second external answer" in the external project
    And external member "X" has state "waiting"

    Examples:
      | runner    |
      | loop      |
      | scheduler |

  @pending
  Scenario: External MCP acts as X and is scoped to its open run (A10.21)
    Given an external acceptance project with X
    And a scripted stdio MCP client connects as X
    Then external MCP lists exactly the frozen external tools and approve
    When external MCP calls "post" with {"text":"Hello from X"} and gets "ok"
    Then X posts "Hello from X" in the external project
    And external MCP calls "read_thread" with {} and gets "ok"
    And external MCP calls "list_members" with {} and gets "ok"
    And external MCP calls "list_tasks" with {} and gets "ok"
    And external MCP calls "note_put" with {"text":"X's note"} and gets "ok"
    And external MCP calls "kb_put" with {"name":"external-note","text":"X's memory"} and gets "ok"
    And external MCP calls "kb_search" with {"query":"memory"} and gets "ok"
    And X asks the managed member and gets its scripted answer
    And external MCP calls "run_checks" with {} and gets "conflict"
    And external MCP calls "submit_review" with {"score":100,"items":[]} and gets "conflict"
    And an external post with a test secret is refused
    And external MCP next_item returns none
    When the external owner mentions X with "Pick this up manually"
    Then external MCP next_item returns a handout
    And the external handout is a "group" run containing "Pick this up manually"
    When external MCP ends the run
    Then external member "X" has state "waiting"
    And external MCP rejects an unknown member

  @pending
  Scenario Outline: External task and review runs use their own checkout (A10.22)
    Given a project with external X and forge "<forge>"
    And a scripted stdio MCP client connects as X
    When the external brief runs with X as "<role>"
    Then the external task merges and its worktree is removed

    Examples:
      | forge  | role     |
      | none   | assignee |
      | none   | reviewer |
      | github | assignee |
      | github | reviewer |

  @pending
  Scenario: A task started in a group run waits for that run to end (A10.22)
    Given an external acceptance project with X
    And a scripted stdio MCP client connects as X
    When the external owner mentions X with "Start the work"
    Then an external look exits 0
    When X starts an external task from the group run
    Then an external look exits 3
    When the external runner ends its current run
    Then an external look exits 0
    And the external handout is a "task" run containing "External criteria"

  @pending
  Scenario Outline: Interrupted runs are retired and notify the secretary (A10.23)
    Given an external acceptance project with X
    And an external task run and a managed secretary
    When the external task run ends by "<end>"
    Then the external task has one failure and the secretary receives the "<reason>" notice
    And an external look exits 3
    And late done for the interrupted external run is harmless
    When the external owner mentions X with "After interruption"
    Then an external look exits 0
    And the external handout is a "group" run containing "After interruption"

    Examples:
      | end       | reason  |
      | failed    | crash   |
      | timeout   | timeout |
      | restart   | crash   |

  @pending
  Scenario: A paused group and an offline host hold waiting mentions (A10.24)
    Given an external acceptance project with X
    When the external owner sets group to "pause"
    And the external owner mentions X with "Held in group"
    Then an external look exits 3
    And external member "X" has state "paused"
    When an external blocking wait starts
    Then the external blocking wait has not exited
    When the external owner sets group to "resume"
    Then the external blocking wait exits 0
    And the external handout is a "group" run containing "Held in group"
    When the external runner ends its current run
    And the external owner sets host to "offline"
    And the external owner mentions X with "Held offline"
    Then an external look exits 3
    And external member "X" has state "offline"
    When an external blocking wait starts
    Then the external blocking wait has not exited
    When the external owner sets host to "online"
    Then the external blocking wait exits 0
    And the external handout is a "group" run containing "Held offline"

  @pending
  Scenario: A paused task holds its item and a jump goes first (A10.24)
    Given an external acceptance project with X
    And a queued, paused external task
    Then an external look exits 3
    And external member "X" has state "waiting"
    When an external blocking wait starts
    Then the external blocking wait has not exited
    When the external owner resumes the task
    Then the external blocking wait exits 0
    And the external handout is a "task" run containing "External criteria"
    When the external runner ends its current run
    And the external owner queues and jumps a newer item
    Then the jump heads the next external handout

  @pending
  Scenario: Usage reaches the row, list_members and the secretary's turn (A10.25)
    Given an external acceptance project with X
    And a scripted stdio MCP client connects as X
    When X reports 40 percent used with a reset in 90 minutes
    Then X's member row and list_members contain that usage
    And a managed member is not offered report_usage
    And the external secretary's next turn lists X with "window 40% used, resets in 2 h"
    When X reports 55 percent used with a reset in 150 minutes
    Then X's member row and list_members contain that usage
    And external MCP calls "report_usage" with {"window_pct":101} and gets "bad_request"

  @pending
  Scenario: External approvals are opt-in and return the owner's decision (A10.26)
    Given an external acceptance project with X
    And a scripted stdio MCP client connects as X
    And an external task run
    Then the external task has no approval card
    When X requests a Bash approval in the external task
    Then the owner can approve it and X receives allow
    When X requests a second Bash approval in the external task
    Then the owner can deny it and X receives the denial message

  @pending
  Scenario: Presence expires and removal revokes access (A10.27)
    Given an external acceptance project with X
    And a scripted stdio MCP client connects as X
    When the external runner stops
    Then X becomes offline after the external presence interval
    And the external secretary's next turn lists X as offline
    When the external owner mentions X with "Keep this offline mention"
    Then the external home has 1 inbox items
    When an external look exits 0
    Then the external handout is a "group" run containing "Keep this offline mention"
    When the external runner ends its current run
    And the external owner tries to remove X from an unfinished task
    Then removal is refused until the external task is reassigned
    When a scripted stdio MCP client connects as X
    And the external owner removes X
    Then the external member is gone and its MCP connection closes
    And an external look rejects the removed member
    And external MCP rejects the removed member

  @pending @live
  Scenario: A real agent follows the setup prompt without help (A10.28)
    Given the owner pastes the external setup prompt into a real MCP-capable agent on the node's computer
    When the agent adds the server and writes and starts its own runner without further help
    Then exactly one setup answer appears and the member is waiting with no agent process left running
