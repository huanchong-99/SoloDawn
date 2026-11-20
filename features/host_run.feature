Feature: Host session runtime

  Scenario: An idle session goes dormant after 1 minute and resumes with its stored id
    Given a host session runtime with idle_minutes set to 1
    When a task session completes a turn with CLI session "797fa93e-af8a-4768-b847-42582d5583a0"
    And 1 minute passes with no turn
    Then the task session is "dormant" and member status reported "dormant"
    When the task session receives another turn
    Then the driver resumes CLI session "797fa93e-af8a-4768-b847-42582d5583a0"

  Scenario: With max_open 1 a waiting AI is closed first and a working AI holds the slot
    Given a host session runtime with max_open set to 1 and two AIs
    When the first AI finishes a turn and is "waiting"
    And the second AI receives a turn
    Then the first AI becomes "dormant" before the second AI starts
    When the second AI is still "working" and the first AI receives a turn
    Then the first AI waits until the second AI finishes its turn and then starts

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

  Scenario: Jumping the queue preempts another task session and resumes it afterwards
    Given an AI running a turn in its task session and also holding a group session
    When a jump turn arrives for the group session
    Then the task session is interrupted and reported preempted while the jump turn runs
    And when the jump turn ends the host sends task resume and a "continue" turn to the task session
    When a jump turn arrives for the running task session itself
    Then the task session is interrupted and runs the jump turn without pausing the task
