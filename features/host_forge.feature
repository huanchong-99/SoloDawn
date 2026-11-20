Feature: Push a task branch through the host
  Background:
    Given a task worktree with a bare origin and another unpublished branch

  Scenario: The level 2 host push publishes only the assigned branch
    When the host pushes the assigned task branch
    Then origin contains only that branch at the task commit

  Scenario: A host push cannot overwrite a newer remote task branch
    Given origin has a newer commit on the assigned branch
    When the host pushes the assigned task branch
    Then Git rejects the push and origin keeps the newer commit
