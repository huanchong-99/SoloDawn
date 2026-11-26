Feature: Tasks from brief to done
  End-to-end task flows on an in-process node with fake Claude Code and Codex
  CLIs, each AI in a workspace of its own: briefs, submission by report,
  scored reviews, done at review, failures, dependencies, direct tasks, queue
  jumps, idempotency and stale-item retirement. The node runs no checks,
  pushes nothing and merges nothing.

  Scenario: Two tasks run from brief to done, each reviewed by the other's AI, with no human action beyond confirm (A5.1, A5.2, A5.12)
    Given a group with a secretary and working AIs "Claude" and "Codex", each in a workspace of its own
    When the owner posts a feature request in the main line and the secretary proposes a two-task brief
    Then before confirm no assign item was queued on the home and no task session exists
    When the owner confirms the brief and both tasks are submitted by report, reviewed by the other's AI and summarized
    Then both tasks are done with two summary cards in the main line and no approval card
    And the owner saw no confirm card (A9.40)
    And each passing review made its task done with no AI turn in between and a task_done row in the thread
    And each task ran in its own session in the workspace of its assignee, after it was done its sessions are gone, and the node's data directory holds no project folder

  Scenario: Review scores, vetoes, the sixth failed review and invalid reviews follow the task machine (A5.4)
    Given a brief task assigned to "Claude" and reviewed by "Codex" that "Claude" submitted twice
    When "Claude" reviews the report under review and "Codex" reviews the earlier report
    Then the review by the assignee is refused with "forbidden" and the review of the earlier report with "conflict"
    When "Codex" reviews with score 86, then with a veto at score 95, and then four more failing reviews on resubmissions
    Then the 86 review returned the task to work, the veto recorded score 0, and the sixth failed review made the task failed

  Scenario Outline: Three consecutive crashed turns with no submission in between stop the task until a person takes it up (A5.5)
    Given an active brief task assigned to "Claude" and reviewed by "Codex"
    When the work of "Claude" on the task ends in turn in "<endings>"
    Then the third failure in a row made the task failed, and its thread mentions the owner
    When the secretary tries to resume the failed task and to reassign it to "Codex"
    Then the home refuses both with "forbidden" and the task stays failed
    When the owner <takes it up>
    Then the task is active without failures, and "<AI>" continues it in its own workspace

    Examples:
      | endings                                                    | takes it up                          | AI     |
      | a crash, a crash, a crash                                  | resumes the failed task              | Claude |
      | a crash, a crash, a crash                                  | reassigns the failed task to "Codex" | Codex  |
      | a crash, a crash, a submission, a crash, a crash, a crash  | resumes the failed task              | Claude |

  Scenario: A dependent task stays queued across reassignment and starts after its dependency is done (A5.6)
    Given a confirmed brief where task 2 depends on task 1
    When the secretary reassigns task 2 to "Codex" and reviewer "Claude" while task 2 is queued
    Then task 2 remains queued with no task session
    When task 1 is submitted and passes its review
    Then task 1 is done, and task 2 becomes active and runs its task session for "Codex" in the workspace of "Codex"

  Scenario: An owner's direct request leaves the group session read-only and moves the change to a task session in the same folder (A5.7)
    Given a group with a secretary and working AIs "Claude" and "Codex", each in a workspace of its own
    When the owner mentions "Claude" directly for a change and "Claude" calls start_task
    Then the group session listed start_task and no tool that submits, a task card exists, and the secretary was notified
    And the change was made in a new task session in the folder of the group session of "Claude"

  Scenario: Jumping the queue on a direct request interrupts the running task turn, pre-empts the task and lets it continue by itself (A5.11)
    Given an active brief task assigned to "Claude" and reviewed by "Codex"
    When "Claude" is running its task turn and the owner jumps the queue on a direct request to it
    Then the running turn is interrupted and the task shows as pre-empted while "Claude" answers the request in its group session
    And the task continues by itself with one turn of its own, and nothing is reported as a failure or delivered twice

  Scenario: A jump aimed at the session that is running interrupts it and goes first without pausing the task (A5.11)
    Given an active brief task assigned to "Claude" and reviewed by "Codex"
    When "Claude" is running its task turn and the owner jumps the queue on a message in the task thread
    Then the running turn is interrupted and the jumped message goes first in the next turn of the same session
    And the session continues by itself with one turn of its own, before the message that waited
    And the task was never paused, and nothing is reported as a failure

  Scenario: Repeated act confirm and repeated task fail with the same nonce apply once (A5.15)
    Given a group with a secretary, working AIs and a pending one-task brief
    When the owner sends the same signed confirm twice and the host sends the same signed task fail twice
    Then both confirms answered ok creating one task, and both task fails answered ok counting one failure

  Scenario: An AI that is assignee or reviewer of an unfinished task cannot be removed (A5.18)
    Given a submitted brief task assigned to "Claude" and reviewed by "Codex"
    Then removing "Claude" or "Codex" from the group is refused with "conflict"

  Scenario: Submitting again retires the queued review on the home and refuses a held review on the host (A5.19)
    Given an active brief task assigned to "Claude" with reviewer "Codex" held on the host
    When "Claude" submits report 1 and then report 2 before "Codex" is released
    Then the home has retired the review item for report 1 and keeps only report 2 queued
    When the host hold on "Codex" is released and "Codex" runs its review turn
    Then the host refused the held review item for report 1, and "Codex" reviewed only report 2 and the task is done

  Scenario: A summary that holds a test secret pattern fails with secret and the task is not submitted (A5.22)
    Given an active brief task assigned to "Claude" and reviewed by "Codex"
    When "Claude" calls submit_task with a summary that holds a test secret
    Then submit_task failed with "secret", the task is still at step work, and the thread holds no report
