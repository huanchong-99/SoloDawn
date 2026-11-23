Feature: Tasks from brief to merge
  End-to-end task flows on in-process nodes with fake Claude Code, Codex and gh
  CLIs, covering briefs, worktrees, local and hosted checks, reviews, merges,
  conflicts, direct tasks, idempotency and stale-item retirement.

  Scenario Outline: Two tasks run from brief to merge without a remote, with no human action beyond confirm (A5.1, A5.2, A5.12, sample <sample>)
    Given a project group with forge "none", a secretary and working AIs "Claude" and "Codex"
    When the owner posts a feature request in the main line and the secretary proposes a two-task brief
    Then before confirm no task worktree exists and no assign item is queued on the home
    When the owner confirms the brief and both tasks run through checks, cross-AI review, merge and summary
    Then both tasks are merged onto base with two summary cards in the main line and no approval card
    And the owner saw no confirm card (A9.40)
    And each passing review was followed by its merge with no AI turn in between and a task_merged row in the thread
    And each task ran in its own session and worktree, and after the merge those sessions and folders are gone

    Examples:
      | sample |
      | 1      |
      | 2      |
      | 3      |
      | 4      |
      | 5      |
      | 6      |
      | 7      |
      | 8      |
      | 9      |
      | 10     |
      | 11     |
      | 12     |
      | 13     |
      | 14     |
      | 15     |
      | 16     |
      | 17     |
      | 18     |
      | 19     |
      | 20     |

  Scenario: Two tasks run from brief to merge with a bare remote and fake gh, including push_branch (A5.1, A5.12, A5.13)
    Given a project group with forge "github", a bare remote, fake gh, a secretary and working AIs "Claude" and "Codex"
    When the owner posts a feature request in the main line and the secretary proposes a two-task brief
    And the owner confirms the brief and both tasks push their branches, pass local and hosted checks, review each other, merge and summarize
    Then push_branch pushed the assigned branch to origin with no approval card
    And both tasks are merged on origin with two summary cards carrying pull-request numbers and no approval card
    And the owner saw no confirm card (A9.40)
    And each passing review was followed by its merge with no AI turn in between and a task_merged row in the thread
    And each task ran in its own session and worktree, and after the merge those sessions and folders are gone

  Scenario: Clean-tree, passing-check and unchanged-head rules govern submission and merge (A5.3)
    Given an active brief task on forge "none" assigned to "Claude" and reviewed by "Codex"
    When "Claude" runs checks on a dirty worktree, submits after failing checks, submits after passing checks, and submits again after an unchecked commit
    Then run_checks refused the dirty worktree, submit_task failed after failing checks, succeeded after passing checks, and failed after the extra commit
    When the review passes while merge is held and a new commit advances the task branch before merge runs
    Then the unreviewed commit is not merged onto base and the task returns to local_check waiting on conflict

  Scenario: Review scores, vetoes, the sixth failed review and invalid reviews follow the task machine (A5.4)
    Given a submitted brief task on forge "none" assigned to "Claude" and reviewed by "Codex"
    When the assignee and a wrong-commit checkout attempt reviews on the submitted task
    Then the assignee review is refused with "forbidden" and the wrong-commit review is refused with "conflict"
    When "Codex" reviews with score 86, then with a veto at score 95, and then four more failing reviews on resubmissions
    Then the 86 review returned the task to local_check, the veto recorded score 0, and the sixth failed review made the task failed

  Scenario: A dependent task stays queued without a worktree across reassignment and starts after its dependency merges (A5.6)
    Given a confirmed brief where task 2 depends on task 1 on forge "none"
    When the secretary reassigns task 2 to "Codex" and reviewer "Claude" while task 2 is queued
    Then task 2 remains queued with no worktree and no task session
    When task 1 completes checks, passes review and merges
    Then task 2 becomes active, creates its worktree, and runs its task session for "Codex"

  Scenario: An owner direct request leaves the group session read-only and moves to a new task session (A5.7)
    Given a project group with forge "none", a secretary and working AIs "Claude" and "Codex"
    When the owner mentions "Claude" directly for a code change and "Claude" calls start_task
    Then the group session listed start_task without task submission tools, created a task card, notified the secretary, and committed the change in a new task session

  Scenario: A diff containing a test secret pattern fails the hard check (A5.9)
    Given an active brief task on forge "none" assigned to "Claude" and reviewed by "Codex"
    When "Claude" commits a test secret block with a test file and calls run_checks
    Then run_checks completes with ok false and a failing "secrets" hard-check item

  Scenario: A project check command sees PATH and HOME and does not see a node token variable (A5.17)
    Given a project group with check "m5envcheck" and a test token variable set in the node environment
    When an active task on that group commits a change and calls run_checks
    Then run_checks passes with "m5envcheck" seeing PATH and HOME and not seeing the test token variable

  Scenario: A merge conflict notifies the secretary and merges after resolution, re-check and re-review (A5.10)
    Given a submitted brief task on forge "none" whose base branch gained a conflicting commit
    When "Codex" passes the review and the host attempts the merge
    Then the task returns to local_check with wait "conflict" and the secretary receives a conflict notice
    When the secretary names "Claude" on the conflicted task and "Claude" resolves, re-checks, resubmits, passes review and merges
    Then the task reaches state "merged" and base holds the resolved content

  Scenario: Repeated act confirm and repeated task fail with the same nonce apply once (A5.15)
    Given a project group with forge "none", a secretary, working AIs and a pending one-task brief
    When the owner sends the same signed confirm twice and the host sends the same signed task fail twice
    Then both confirms answered ok creating one task, and both task fails answered ok counting one failure

  Scenario: Busy AIs cannot leave, and at step merge only an admin cancel stops the task before the host drops the merge (A5.18)
    Given a submitted brief task on forge "none" assigned to "Claude" and reviewed by "Codex"
    Then removing "Claude" or "Codex" from the group is refused with "conflict"
    When "Codex" passes the review while the host holds the merge item
    Then reassigning the task and submitting a new commit at step merge are refused with "conflict"
    When the owner cancels the task at step merge and the host releases and processes the held merge item
    Then the task is cancelled, base is unchanged, and the host recorded the merge item as refused

  Scenario: Resubmitting with a newer commit retires the queued review on the home and refuses a held review on the host (A5.19)
    Given an active brief task on forge "none" assigned to "Claude" with reviewer "Codex" held on the host
    When "Claude" submits commit 1 and then commits, checks and submits commit 2 before "Codex" is released
    Then the home has retired the review item for commit 1 and keeps only commit 2 queued
    When the host hold on "Codex" is released and "Codex" runs its review turn
    Then the host refused the held review item for commit 1 and delivered only commit 2 to "Codex"
