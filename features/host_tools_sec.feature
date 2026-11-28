Feature: Secretary coordination and decision MCP tools
  Scenario: Secretary proposes a brief and updates a task while working sessions cannot see secretary tools
    Given a host group with secretary "Secretary", AIs "Claude Code" and "Codex", owner "owner", and task "T-12"
    Then only "request_decision" of the HS-11 tools is listed to working AI "Claude Code"
    When secretary "Secretary" invokes "propose_brief" with '{"title":"Path notice","goal":"Show path changes.","criteria":["Updates within 1 s"],"scope_in":["host"],"scope_out":["relay"],"tasks":[{"title":"Emit path event","assignee":"Claude Code","reviewer":"Codex"}]}'
    Then the secretary tool call returns a random id
    And the home received a signed "brief" post from "Secretary" mentioning "owner" with pass 90
    When secretary "Secretary" invokes "set_task" with '{"task":"T-12","reviewer":"Claude Code","action":"pause"}'
    Then the secretary tool call returns ok
    And the home received "task" commands "assign, pause" for task "T-12"

  Scenario: Decision cards number options from A, default to the group's people in task sessions, and block secrets
    Given a host group with secretary "Secretary", AIs "Claude Code" and "Codex", owner "owner", and task "T-12"
    When task session "Claude Code" on "T-12" invokes "request_decision" with '{"question":"Where does the badge go?","options":[{"label":"Header","effect":"Always visible"},{"label":"Footer","effect":"Quiet"}],"recommended":1}'
    Then the secretary tool call returns a random id
    And the home received a "decision" post on "T-12" recommending "B" to "owner"
    When task session "Claude Code" on "T-12" invokes "request_decision" with '{"question":"Leak sk-proj-EXAMPLEexampleEXAMPLEexample_EXAMPLE-00?","options":[]}'
    Then the secretary tool call fails with "secret: this looks like a secret; remove it and send again"

  Scenario: Proposing a brief with fewer than two working AIs beside the secretary refuses without forwarding or accepting invented names
    Given a host group with secretary "Secretary", one working AI "Claude Code", owner "owner"
    When secretary "Secretary" invokes "propose_brief" with '{"title":"Path notice","goal":"Show path changes.","criteria":["Updates within 1 s"],"scope_in":["host"],"scope_out":["relay"],"tasks":[{"title":"Emit path event","assignee":"Claude Code","reviewer":"Ghost"}]}'
    Then the secretary tool call fails with "conflict: cannot propose brief: a group needs at least 2 working AIs besides the secretary so assignee and reviewer are different AIs; do not invent AI names — tell the person in the main line to add working AIs via Add AI"
    And the home received no commands
