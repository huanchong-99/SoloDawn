@web
Feature: Task card in the chat main line
  A message of kind "task" is written by the group home and drawn whole by the task card from the
  live task row in the group cache: its number, title and status badge, the six-step progress bar,
  the assignee, branch and pull request, local and hosted check outcomes, the review verdict or
  named reviewer, and the folded thread preview (planning/contracts/rows.md section 4;
  planning/contracts/task-machine.md section 1; acceptance test A5.1).

  Background:
    Given the chat of the group "Website" open in a browser of its owner, set to English

  Scenario: A task message shows a minimal card until its task row arrives, and when it is removed
    When the home writes a task message for "DrtCn6htSBwmMPrFPbHJHA" before the task row arrives
    Then the main line shows a minimal task card naming "DrtCn6htSBwmMPrFPbHJHA"
    When the home writes the wire example "row-task"
    Then the task card shows "T-12" and "Show connection path"
    When the home removes the task row "DrtCn6htSBwmMPrFPbHJHA"
    Then the main line shows the minimal task card naming "DrtCn6htSBwmMPrFPbHJHA" again

  Scenario: A task sent back by review shows its six-step bar, checks, review verdict and thread preview
    When the home writes the wire examples "row-task" and "row-msg-task"
    Then the task card shows "T-12", "Show connection path" and the danger badge "Sent back · round 1"
    And the six-step bar marks "done, done, done, done, fail, -" and reads "Review · 5 / 6"
    And the metadata shows owner "Codex · workstation" and branch "task/12-show-connection-path" with link "PR #31"
    And the checks line shows "Local checks passed" and "Hosted checks passed"
    And the review line shows "Round 1 · 86, sent back" and "Claude Code · pass 90"
    And the folded thread summary shows "Thread · 7" and "Latest: Codex · Added the path indicator."
    When the reader switches the language to Chinese and expands the thread fold
    Then the task card shows "打回 · 第 1 轮", "评审 · 5 / 6", "第 1 轮 86 分，打回" and an open thread fold

  Scenario: A task moves from queued through each step to merged, with and without a remote forge
    When a queued task "T-15" "Add path badge" is created without a branch yet
    Then the task card shows badge "Queued", steps "-, -, -, -, -, -" with "0 / 6", no branch row, "Not run yet" and "Not started · Claude Code to review · pass 90"
    When the task becomes active at step "branch"
    Then the task card shows working badge "In progress" and steps "now, -, -, -, -, -" with "Work · 1 / 6"
    When the task advances to "local_check" on branch "task/15-add-path-badge" with a failing local check, then a passing one
    Then the six-step bar first marks "done, fail, -, -, -, -" with "Local checks failed", then "done, done, now, -, -, -" with "Open PR · 3 / 6" and "Local checks passed"
    When the task advances to "cloud_check" on PR 44 without a link yet and a failing hosted check
    Then the branch line shows plain text "PR #44" with no link, and the six-step bar marks "done, done, done, fail, -, -" with "Hosted checks · 4 / 6" and "Hosted checks failed"
    When hosted checks pass, the pull request URL arrives, the review passes with score 94 in round 2, and the task enters "merge"
    Then the six-step bar marks "done, done, done, done, done, now" with "Merge · 6 / 6" and the review line shows "Round 2 · 94, passed"
    When the task becomes "merged"
    Then the task card shows ok badge "Merged · 94" and steps "done, done, done, done, done, done" with "6 / 6"
    When the group has forge "none"
    Then the merged six-step bar skips pull request and hosted checks as "done, done, -, -, done, done"

  Scenario: Waiting tasks distinguish waits on the reader from waits on another person, a reviewer or a conflict
    When the home writes "row-task-waiting" and a task message for it
    Then the task card shows solid dawn badge "Waiting for your decision", steps "wait, -, -, -, -, -" with "Work · 1 / 6", and a checks link to "#m-TDRDfXwvOO-iOegSUoyidg"
    When the task instead waits on an approval from the reader, then from "Mina", then from an unknown user
    Then the task card shows solid dawn "Waiting for your approval" with step "wait", then neutral "Waiting for Mina's approval" with step "now", then "Waiting for approval"
    When the task waits for a reviewer, and then hits a merge conflict
    Then the task card shows "Waiting for a reviewer", and then danger "Merge conflict" with step "fail"

  Scenario: Paused, stopped and cancelled tasks show their state
    When the home writes "row-task-paused" and a task message for it
    Then the task card shows paused badge "Paused", steps "done, done, -, -, -, -" with "Review · 5 / 6", and "Local checks passed"
    When the task stops in state "failed" after 3 consecutive failures, and then after 6 failed reviews
    Then the task card shows solid danger badge "Stopped after 3 failures" with step "fail", and then "Stopped after 6 failed reviews"
    When the task becomes "cancelled"
    Then the task card shows "Cancelled" with no active step marker

  Scenario: The development states sheet mounts the task card for the wire example
    Given a development server
    When the person opens the client at "#/states"
    Then the "task" section is marked "Mounted by task" and renders "T-12" "Show connection path" with six steps
