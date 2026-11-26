@web
Feature: Task card in the chat main line
  A message of kind "task" is written by the group home and drawn whole by the task card from the
  live task row in the group cache: its number, title and status badge, the three-step progress
  bar (work, review, done), the assignee and the folder it works in, what the task waits for, the
  review verdict or named reviewer, and the folded thread preview. One folder takes one task at a
  time: a task whose folder is busy waits, queued, and says which task has it
  (planning/contracts/rows.md section 4; planning/contracts/task-machine.md sections 1 and 2;
  acceptance tests A5.1 and A5.20).

  Background:
    Given the chat of the group "Website" open in a browser of its owner, set to English

  Scenario: A task message shows a minimal card until its task row arrives, and when it is removed
    When the home writes a task message for "DrtCn6htSBwmMPrFPbHJHA" before the task row arrives
    Then the main line shows a minimal task card naming "DrtCn6htSBwmMPrFPbHJHA"
    When the home writes the wire example "row-task"
    Then the task card shows "T-12" and "Show connection path"
    When the home removes the task row "DrtCn6htSBwmMPrFPbHJHA"
    Then the main line shows the minimal task card naming "DrtCn6htSBwmMPrFPbHJHA" again

  Scenario: A task sent back by review shows its three-step bar, folder, review verdict and thread preview
    When the home writes the wire examples "row-task" and "row-msg-task"
    Then the task card shows "T-12", "Show connection path" and the danger badge "Sent back · round 1"
    And the three-step bar marks "done, now, -" and reads "Review · 2 / 3"
    And the metadata shows owner "Codex · workstation" and folder "/home/dev/website"
    And the review line shows "Round 1 · 86, sent back" and "Claude Code · pass 90"
    And the folded thread summary shows "Thread · 7" and "Latest: Codex · Added the path indicator."
    When the reader switches the language to Chinese and expands the thread fold
    Then the task card shows "打回 · 第 1 轮", "评审 · 2 / 3", "第 1 轮 86 分，打回" and an open thread fold

  Scenario: A task moves from queued through work and review to done
    When a queued task "T-15" "Add path badge" waits for T-12
    Then the task card shows badge "Queued", steps "-, -, -" with "Queued · 0 / 3", "Waits for T-12 to be done." and "Not started · Claude Code to review · pass 90"
    When the task becomes active at step "work"
    Then the task card shows working badge "In progress" and steps "now, -, -" with "Work · 1 / 3"
    When the assignee submits its report and the task is at step "review"
    Then the three-step bar marks "done, now, -" with "Review · 2 / 3"
    When the review passes with score 94 in round 2, and the task is done
    Then the task card shows ok badge "Done · 94", steps "done, done, done" with "Done · 3 / 3" and the review line "Round 2 · 94, passed"

  Scenario: A task waits for its folder and starts when the folder is free
    Given Codex works on T-12 in "/home/dev/website"
    When the home writes T-15 for Codex, queued until its folder is free, and a task message for it
    Then the card of T-15 shows "Queued", the folder "/home/dev/website" and "Waits for its folder, which T-12 is using."
    When T-12 is done and the home starts T-15
    Then the card of T-15 shows "In progress" with "Work · 1 / 3" and no longer waits

  Scenario: A task held for its AI's owner asks that owner, and shows when the owner has taken over its terminal
    When the home writes "row-task-confirm", its pending confirmation and a task message for it
    Then the task card shows solid dawn badge "Waiting for your confirmation", step "wait" and a link to the confirmation
    When the confirmation waits on "Mina" instead
    Then the task card shows neutral "Waiting for Mina's confirmation" with step "now"
    When the card is confirmed and the assignee's owner takes over its terminal for this task
    Then the task card shows the takeover badge "AI's owner is driving" with step "now"

  Scenario: Waiting tasks distinguish waits on the reader from waits on another person or a reviewer
    When the home writes "row-task-waiting" and a task message for it
    Then the task card shows solid dawn badge "Waiting for your decision", steps "wait, -, -" with "Work · 1 / 3", and a link to "#m-TDRDfXwvOO-iOegSUoyidg"
    When the task instead waits on an approval from the reader, then from "Mina", then from an unknown user
    Then the task card shows solid dawn "Waiting for your approval" with step "wait", then neutral "Waiting for Mina's approval" with step "now", then "Waiting for approval"
    When the reader clicks "Waiting for your approval" on the task card for main-line card "ap-card-1"
    Then the chat stays open at the group's address, the thread stays closed, and "#m-ap-card-1" scrolls into view
    When the task waits for a reviewer
    Then the task card shows "Waiting for a reviewer" at step "review"

  Scenario: Paused, stopped and cancelled tasks show their state
    When the home writes "row-task-paused" and a task message for it
    Then the task card shows paused badge "Paused" and steps "done, -, -" with "Review · 2 / 3"
    When the task stops in state "failed" after 3 consecutive failures, and then after 6 failed reviews
    Then the task card shows solid danger badge "Stopped after 3 failures" with step "fail", and then "Stopped after 6 failed reviews"
    When the task becomes "cancelled"
    Then the task card shows "Cancelled" with no active step marker

  Scenario: The development states sheet mounts the task card for the wire example
    Given a development server
    When the person opens the client at "#/states"
    Then the "task" section is marked "Mounted by task" and renders "T-12" "Show connection path" with three steps
