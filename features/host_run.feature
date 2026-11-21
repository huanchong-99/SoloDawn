Feature: Host session runtime

  Scenario: An idle session goes dormant after 1 minute and resumes with its stored id
    Given a host session runtime with idle_minutes set to 1
    When a task session with CLI session "797fa93e-af8a-4768-b847-42582d5583a0" completes a turn, stays idle for 1 minute and receives another turn
    Then the task session became "dormant" and member status reported "dormant" before the second turn
    And the driver resumed CLI session "797fa93e-af8a-4768-b847-42582d5583a0"

  Scenario: With max_open 1 a waiting AI is closed first and a working AI holds the slot
    Given a host session runtime with max_open set to 1 and two AIs
    When the first AI finishes a turn, the second AI starts a turn, and the first AI requests a turn before the second finishes
    Then the first AI became "dormant" before the second AI started
    And the first AI waited while the second AI was working and started once it finished

  Scenario: A CLI exiting mid-turn reports a crash and the next turn resumes
    Given a task session backed by the scripted Claude CLI
    When the scripted Claude CLI exits mid-turn after reporting session "887fa93e-af8a-4768-b847-42582d5583a1"
    Then the host reports turn_interrupted "crash" and task fail "crash"
    And the next turn on the task session resumes "887fa93e-af8a-4768-b847-42582d5583a1" and completes

  Scenario: Ten session switches close the old CLI process before starting the next
    Given two scripted Claude sessions under max_open 1 sharing a lifecycle log
    When turns alternate between the two sessions 10 times
    Then the lifecycle log records 11 non-overlapping start-exit pairs

  Scenario: A failed resume starts a fresh session with a replaced turn
    Given a task session whose stored CLI session id is no longer resumable
    When the task session receives a turn
    Then the host reports session_replaced and delivers a "replaced" turn on a fresh session
