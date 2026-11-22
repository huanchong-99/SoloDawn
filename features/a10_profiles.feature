@pending
Feature: AI profiles, assignment by skills and escalation to a stronger AI
  Working AIs S (strong), M (standard) and L (light) of one owner, and the
  secretary C (standard unless a scenario says otherwise).

  Scenario: The profile of a member (A10.8)
    Given the profiles cast
    When a new AI member "N" joins
    Then the member row of "N" has tier "standard" and skills ""
    When the owner sends member set for "N" with tier "strong" and skills "Go backend"
    Then the member row of "N" has tier "strong" and skills "Go backend"
    And member set for "N" with tier "light" is "forbidden" from its host, another person and an admin who is not its owner
    And member set for "N" is "bad_request" with tier "expert", with skills of 201 code points and with skills holding a line break
    And in the owner's browser the AI member panel of "N" shows tier "strong" and skills "Go backend"
    When the owner sets tier "light" and skills "docs" for "N" in the browser
    Then the member row of "N" has tier "light" and skills "docs"
    And no other person's browser offers the editor of "N"

  Scenario: The secretary's member list and rules (A10.9)
    Given the profiles cast
    And L is limited and the host of S is offline
    When C takes its next turn
    Then the turn lists every AI member with its tier, state and skills as in section 3 of the profiles contract
    And the turn lists L as "limited" and S as "offline"
    And C's instructions hold the four rules of assignment
    And list_members answers as in step 1

  Scenario: Escalation of a stuck assignee (A10.10)
    Given the profiles cast with M the assignee of task T
    When M fails a review
    Then task T has no stuck ask and submit_task by M succeeds as in step 1
    When M fails a review
    Then task T has stuck kind "review", n 2 and ask true
    And M's reviewed item ends with the line naming S and not L or C
    And submit_task by M fails with "conflict" with no push, no pull request and no report
    When L replies to M's call in the thread of task T
    And S replies to L's call in the thread of task T
    And S replies to M's call in the thread of another task
    And the host of S goes offline
    Then submit_task by M fails with "conflict" with no push, no pull request and no report
    When the host of S returns
    And S replies to M's call in the thread of task T
    Then task T has stuck kind "review", n 2 and ask false
    And submit_task by M succeeds
    When M fails a review
    Then task T has stuck kind "review", n 3 and ask true
    And M's reviewed item ends with the line naming S and not L or C
    And submit_task by M fails with "conflict" with no push, no pull request and no report
    And the same holds with hosted checks failing twice, stuck kind "checks" and M's checks item
    And the browser shows the badge "must ask a stronger AI" on task T exactly while ask is true

  Scenario: No stronger AI, and the secretary as candidate (A10.11)
    Given the profiles cast with S the assignee of task T
    When S fails a review
    And S fails a review
    Then task T has stuck kind "review", n 2 and ask false
    And S's reviewed item has no added line
    And submit_task by S succeeds
    Given the profiles cast with every AI at the default tier and M the assignee of task T
    When M fails a review
    And M fails a review
    Then task T has stuck kind "review", n 2 and ask false
    And M's reviewed item has no added line
    And submit_task by M succeeds
    Given the profiles cast with C strong and S the assignee of task T
    When S fails a review
    And S fails a review
    Then S's reviewed item ends with the line naming C and not M or L
    When C replies to S's call in the thread of task T
    Then task T has stuck kind "review", n 2 and ask false
    And the acceptance scenarios A1 to A8 pass with default profiles

  Scenario: What counts as failures in a row (A10.12)
    Given the profiles cast with M the assignee of task T
    When M fails hosted checks
    And M fails a review
    Then task T has stuck kind "review", n 1 and ask false
    When M fails a review
    And M passes hosted checks on the next commit
    And M fails a review
    Then task T has stuck kind "review", n 2 and ask true
    Given the profiles cast with M the assignee of task T
    When M fails a review
    And M passes a review
    Then task T has no stuck
    When M fails a review
    Then task T has stuck kind "review", n 1 and ask false
    When task assign gives task T to L
    Then task T has no stuck

  Scenario: Discovering models (A10.13)
    Given a host with fake CLIs "claude,codex,opencode,qwen" each scripted to list two models, one with fast
    When the host reads its agents
    Then host.info lists under each agent but qwen its two models with id, name, levels and fast as scripted
    And each listed model has the tier of the table for its id's words, else "standard"
    And the models of qwen are []
    When the listing of the fake codex fails
    And the host reads its agents with refresh
    Then the models of codex are [] and host.info is otherwise as before
    And the suggested tier of "gemini-3-pro" is "standard" and of "gpt-6-mini" is "light"
    And the claude entry "default" has the tier of its resolvedModel
    And the browser's dialog for adding an AI offers the listed models and their levels
    And for an agent with no models the dialog offers two text fields
    And for each real CLI the list is what the CLI's own picker shows

  Scenario: Refreshing models (A10.14)
    Given a host with fake CLIs "claude" each scripted to list two models, one with fast
    And the host reads its agents
    When the fake claude's list gains a model
    And the host reads its agents within 60 seconds
    Then host.info answers the old list
    When the host reads its agents with refresh
    Then host.info answers the new list and a push carries it
    And the refresh button on the browser's AI member panel shows the new model without a restart of the node

  Scenario: Choosing a model (A10.15)
    Given the profiles cast
    When host.join adds the agent "claude" as "Opus" with model "opus", effort "high" and tier "strong"
    And host.join adds the agent "claude" as "Haiku" with model "haiku", effort "" and tier "light"
    Then the member rows of "Opus" and "Haiku" have the model, effort and tier they joined with
    And host.info lists both members under the agent "claude"
    And host.leave for the agent "claude" without member is "bad_request"
    And host.leave for the agent "claude" with member "Haiku" removes only "Haiku"
    And each member's sessions start the fake CLI with its own model and effort as in section 10.4
    And a member with model "" and effort "" passes neither
    When the owner sends member set for "Opus" with model "sonnet"
    Then the next start of a session of "Opus", a resume, passes model "sonnet"
    When the fake claude exits and the fake codex answers thread/start with an error on a refused model
    Then each emits turn_interrupted with "crash", a task session ends in task fail, no session_replaced is sent and the session keeps its id
    And with each real CLI the model the CLI reports is the one chosen

  Scenario: The owner's edit stays (A10.16)
    Given the profiles cast with a member "Opus" whose model "opus" is suggested "strong"
    When the owner sends member set for "Opus" with tier "light" and skills "docs"
    And a refresh suggests "standard" for "opus"
    And a refresh lists no "opus"
    And a refresh lists no models
    And the node restarts
    Then the member row of "Opus" is unchanged byte for byte since the member set
    And the browser's AI member panel of "Opus" shows tier "light", the suggestion "strong" beside it and "not listed" for its model
