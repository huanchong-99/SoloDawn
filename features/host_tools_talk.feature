Feature: Host talking and group state MCP tools
  Scenario: Text an AI prints outside tool calls sends nothing to the home until post is called
    Given a connected host session for AI "Claude Code" in a group with task "T-12"
    When the AI prints "Private scratch reasoning" outside tool calls
    Then the home has received 0 requests
    When the AI calls "post" with '{"text":"Ready for review.","kind":"progress","mentions":["Codex"]}'
    Then the tool succeeds with an id
    And the home has received 1 signed "post" command as that AI in task "T-12"

  Scenario Outline: Outbound secret check blocks AI text before signing or sending
    Given a connected host session for AI "Claude Code" in a group with task "T-12"
    When the AI calls "<tool>" with '<args>'
    Then the tool fails with "secret: this looks like a secret; remove it and send again"
    And the home has received 0 requests

    Examples:
      | tool     | args                                                            |
      | post     | {"text":"Key: sk-proj-EXAMPLEexampleEXAMPLEexample_EXAMPLE-00"} |
      | note_put | {"text":"Header: -----BEGIN PRIVATE KEY-----"}                  |

  Scenario: Handback post is refused outside a hand-back turn and carries Git facts in one
    Given a connected host session for AI "Claude Code" in a group with task "T-12"
    When the AI calls "post" with '{"text":"Back to structured mode.","kind":"handback"}'
    Then the tool fails with code "bad_request"
    And the home has received 0 requests
    When the task session enters a hand-back turn after committing "fix.go" and leaving "notes.txt" untracked
    And the AI calls "post" with '{"text":"Fixed the path check.","kind":"handback"}'
    Then the tool succeeds with an id
    And the home received a "handback" post listing files "fix.go, notes.txt" and 1 commit

  Scenario: Group read and memory tools map member names and task numbers
    Given a connected host session for AI "Claude Code" in a group with task "T-12"
    When the AI calls "list_members" with '{}'
    Then the tool result JSON contains '"name":"Codex"' and '"task":"T-12"'
    When the AI calls "list_tasks" with '{"state":"active"}'
    Then the tool result JSON contains '"task":"T-12"' and '"assignee":"Claude Code"'
    When the AI calls "read_thread" with '{}'
    Then the tool result JSON contains '"by":"Codex"' and '"text":"Looks good."'
    When the AI calls "note_put" with '{"text":"Always check path state."}'
    Then the tool succeeds with an id
    When the AI calls "note_list" with '{"task":"T-12"}'
    Then the tool result JSON contains '"task":"T-12"' and '"by":"Claude Code"'
