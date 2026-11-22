Feature: Watch a takeover, post to a team group and use custom forges

  @pending
  Scenario: Terminal watching needs the owner's consent and ends at hand-back (A9.20)
    Given Q-team is in a takeover on Q1
    When P asks to watch the takeover
    Then Q sees a pending watch confirmation and P cannot start watching
    When Q confirms the watch request
    Then P receives the owner's terminal bytes in order
    And P's input and resize do not reach the terminal
    When Q hands back the takeover
    Then P's watch channel closes within 2 seconds

  @pending
  Scenario: An open direct watch closes after the same connection selects TURN (A9.21)
    Given Q-team is in a takeover on Q1
    And Q has granted P access to watch
    And P opens a direct watch on Q1
    When the same connection selects the cast TURN relay
    Then the watch closes, the original ctl syncs and watch.start is refused with relay

  Scenario: A team group has no repository, tasks, briefs, or AI members (A9.22)
    Given P creates a team group on P2 without a repository
    Then its group row has no repository
    And the team group refuses task start, brief posts, host.join, member add_ai and check settings
    And no node holds a clone for the team group

  Scenario: A project secretary can post a bulletin to a known team group (A9.23)
    Given P creates a team group on P2 without a repository
    And the secretary's node knows the team group
    When the secretary posts a notice to the team group using the post tool
    Then the team main line shows a bulletin naming the secretary and project
    And the working AI's call to the same tool is refused
    When P posts a team message naming the working AI
    Then the team message creates no inbox item

  @pending
  Scenario: Custom forge templates drive tasks from a brief to merge (A9.24)
    Given a project uses the fake forge templates
    When the secretary proposes two tasks for cross-review and P confirms the brief
    Then the stale reviewed head is refused and the secretary reassigns the task
    And both tasks merge after checks and cross-review and receive summaries
    And the forge records the filled template arguments and gh never runs

  @pending
  Scenario: Custom forge templates preserve argument boundaries (A9.25)
    Given a project uses the fake forge templates
    And the project refuses a push template containing the title place
    When P confirms a brief with a title containing spaces, quotes and shell punctuation
    Then the forge receives the title as one argument and the tag as a literal without creating a marker

  @pending @live
  Scenario: Tasks run from brief to merge on a real non-GitHub forge (A9.26)
    Given a signed-in non-GitHub forge and its scratch repository
    When the owner posts a feature request in the main line and the secretary proposes a two-task brief
    Then before confirm no task worktree exists and no assign item is queued on the home
    When the owner confirms the brief and both tasks push their branches, pass local and hosted checks, review each other, merge and summarize
    Then both assigned branches were pushed without approval cards
    And both tasks are merged on origin with two summary cards carrying pull-request numbers and no approval card
    And each passing review was followed by its merge with no AI turn in between and a task_merged row in the thread
    And the forge reports both distinct requests merged at their reviewed heads
    And each task ran in its own session and worktree, and after the merge those sessions and folders are gone
