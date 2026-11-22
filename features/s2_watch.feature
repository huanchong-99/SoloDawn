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
  Scenario: A relayed watch request is refused while ctl keeps answering (A9.21)
    Given Q-team is in a takeover on Q1
    And Q has granted P access to watch
    When P connects to Q1 through TURN
    Then watch.start is refused with relay and the same ctl still answers

  @pending
  Scenario: A team group has no repository, tasks, briefs, or AI members (A9.22)
    Given P creates a team group on P2 without a repository
    Then its group row has no repository
    And the team group refuses task start, brief posts, host.join, member add_ai and check settings
    And no node holds a clone for the team group

  @pending
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
    When the secretary proposes two tasks for cross-review and P confirms the brief
    Then both tasks merge after checks and cross-review and receive summaries
    And both tasks land on the scratch remote and the forge reports both requests
