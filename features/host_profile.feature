Feature: Host profile turns and escalation
  Scenario: The secretary sees the assignment roster and rules (A10.9 host)
    Given the host has the secretary's synced AI profiles
    When the secretary takes its next turn
    Then the turn begins with:
      """
      [SoloDawn] group "Website" · you are "Secretary" · secretary session
      AI members (CLI on computer · folder · tier · state · skills):
      - "Claude Code" · claude on node · /home/dev/website · strong · offline · window 40% used, resets in 3 h · Go backend
      - "Codex" · codex on node · /home/dev/website-review · light · limited · window 100% used · docs
      - "Secretary" · claude on node · /home/dev/website · standard · working on T-12
      == 1 of 1 · confirmed · instruction
      """
    And the secretary instructions contain the assignment rules
    And list_members still returns:
      """
      {"members":[
        {"name":"Claude Code","kind":"ai","state":"offline","host":"node","workspace":"/home/dev/website","account":"personal","usage":{"window_pct":40,"window_resets":1763609000000}},
        {"name":"Codex","kind":"ai","state":"limited","host":"node","workspace":"/home/dev/website-review","account":"personal","usage":{"window_pct":100}},
        {"name":"Secretary","kind":"ai","state":"working","host":"node","workspace":"/home/dev/website","account":"personal","task":"T-12","usage":{}},
        {"name":"owner","kind":"human"}
      ]}
      """

  Scenario: Repeated failed reviews tell the assignee whom to ask (A10.10 host)
    Given the host has a standard assignee and an offline strong secretary
    When the host composes a turn after the 1st failed review
    Then the turn has no escalation line
    When the host composes a turn after the 2nd failed review
    Then the turn ends with the escalation line:
      """
      This is failure 2 in a row of the same kind (review). Before you submit again, call ask to one of: "Secretary" (strong). Then continue with the answer.
      """
    When the host composes a turn after the 3rd failed review
    Then the turn ends with the escalation line:
      """
      This is failure 3 in a row of the same kind (review). Before you submit again, call ask to one of: "Secretary" (strong). Then continue with the answer.
      """
    And the task instructions say to ask before submitting

  Scenario: An unanswered escalation blocks submission before the report reaches the home (A10.10 host)
    Given a task session whose task asks its assignee to ask a stronger AI first
    When the assignee submits the task before the answer
    Then the submission returns conflict naming the offline strong secretary and no report was posted
    When the host receives the task row with the escalation answered and the assignee retries
    Then the retried submission posts its summary as the report in the task's thread
