Feature: Group and member commands in the home
  A node's owner creates a project group with g equal to n, writing the group
  row and the owner's member row and marking the group list changed. Host nodes
  add AI members with unique names; one AI is the secretary, and an AI with an
  unfinished task cannot be removed or made secretary.

  Scenario: Creating and renaming a group marks the group list changed
    Given an uncreated group home on the owner's node
    When the owner creates project group "Website" as member "alex"
    Then the group command answers ok and marks groups changed
    And the group row is "Website" with pass 90 and foundation pass 70 and owner "alex"
    When the owner renames the group to "Docs"
    Then the group command answers ok and marks groups changed
    And the group row is named "Docs"

  Scenario: AI members keep unique names and move the secretary flag
    Given a created group "Website" with owner "alex"
    When the owner's laptop adds AI "Claude" on "claude" as secretary
    And the owner's laptop adds AI "Codex" on "codex" as worker
    Then the group secretary is "Claude" and "Codex" is a working AI
    When the owner's laptop adds AI "alex" on "codex" as worker
    Then the group command answers "conflict"
    When the owner sets the group secretary to "Codex"
    Then the group secretary is "Codex" and "Claude" is a working AI

  Scenario: Removing the secretary clears group.secretary and writes no_secretary
    Given a created group "Website" with owner "alex"
    And the owner's laptop has added AI "Claude" on "claude" as secretary
    And inbox item 41 is queued for "Claude"
    When the owner removes AI "Claude"
    Then the group command answers ok
    And the group has no secretary and holds a "no_secretary" system message
    And AI "Claude" is gone and inbox item 41 is retired

  Scenario: An AI with an unfinished task cannot be removed or made secretary
    Given a created group "Website" with owner "alex"
    And the owner's laptop has added AI "Claude" on "claude" as secretary
    And the owner's laptop has added AI "Codex" on "codex" as worker
    And "Codex" is the assignee of an "active" task
    When the owner removes AI "Codex"
    Then the group command answers "conflict"
    When the owner sets the group secretary to "Codex"
    Then the group command answers "conflict"
    When that task becomes "done"
    And the owner removes AI "Codex"
    Then the group command answers ok
    And AI "Codex" is gone and inbox item 0 is retired

  Scenario Outline: Invalid group and member commands are refused
    Given a created group "Website" with owner "alex"
    When <attempt>
    Then the group command answers "<code>"

    Examples:
      | attempt                                      | code        |
      | the owner sends group create with g not n    | bad_request |
      | the owner sends group set with no fields     | bad_request |
      | the owner sends member set with no fields    | bad_request |
      | a non-admin member sends group set           | forbidden   |
