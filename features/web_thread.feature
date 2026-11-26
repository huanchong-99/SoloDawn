@web
Feature: Task thread, review block and system notices
  A task's thread pages from its newest messages back, renders every message kind with its badge,
  mentions, code spans and body block, links cards raised to the main line instead of drawing them
  twice, shows when the task's assignee AI is working on it, draws scored review blocks with pass
  thresholds and veto items, and words the home's system notices in the reader's language, showing
  done tasks under the secretary's name. A report is the assignee's own account, as written.

  Background:
    Given a group "Website" with a secretary, two AIs and task 12 "Show connection path"

  Scenario: Opening a thread fetches its newest page, and older pages load on demand
    Given the thread of task 12 holds 75 progress messages on the home
    When the thread of task 12 is mounted
    Then the browser fetches the newest 50 messages of task 12 and shows them in order
    When the reader presses "Load earlier messages"
    Then the browser fetches the page before the oldest message held and shows all 75 in order
    And the button "Load earlier messages" is gone

  Scenario: An unreachable home stops thread paging with a notice
    Given the thread of task 12 holds 75 progress messages on the home
    When the home stops and the thread of task 12 is mounted
    Then no thread page is fetched
    And the thread says "Earlier messages cannot be loaded right now."

  Scenario: A thread renders message kinds, the report as written, one-line links for main-line cards and the working indicator
    Given the thread of task 12 holds every thread message kind
    And the assignee "Codex" is working on task 12 for 6 minutes
    When the thread of task 12 is mounted
    Then the thread shows the badges for progress, question, report, approval, decision and summary
    And only cards waiting on the reader carry the dawn badge, and every main-line card links to its main-line message without duplicate ids
    And the summary line shows "T-12 · score 94" in English and "T-12 · 94 分" in Chinese
    When the reader clicks a main-line card link in the thread
    Then the hash route stays on the chat of "Website" and scrolls to the main-line message
    And the question marks the mention "@Secretary" and the code span "path"
    And the report shows the assignee's own account and no line of figures
    And the bottom of the thread shows "Codex is working · 6 min"
    When "Codex" works on another task or is waiting
    Then the working indicator is gone

  Scenario: Review blocks show pass, sent-back and veto states with round numbers and file locations
    Given the thread of task 12 holds a sent-back review, a veto review and a passed review
    When the thread of task 12 is mounted
    Then the sent-back review has tone "danger", badge "Review · Round 1 · Sent back", score "86", "Pass 90" and locations "path.go:88" and "conn.go"
    And the veto review shows the "Veto" badge and score "0"
    And the passed review has tone "ok", badge "Review · Round 3 · Passed" and score "94"

  Scenario: Done tasks appear under the secretary's name, 1.0 system codes have client wording and postponed codes fall back cleanly
    Given the thread of task 12 holds a task_done notice, every 1.0 system code and a postponed system code
    And the main line also holds a task_done notice
    When the thread of task 12 is mounted
    Then the task_done row reads "Secretary T-12 passed review and is done · Show connection path" with an ok check icon
    And every 1.0 system row is worded in English and the postponed row shows its code
    And only the round limit of a task that waits for a person is warm
    And switching the language to Chinese words task_done under "Secretary" as "T-12 通过评审，已完成" and every other row in Chinese
    When the reader opens the main line of "Website"
    Then the main line also renders the task_done notice under "Secretary"

  Scenario: The states sheet renders every wire example with the thread area's mounted cards
    When the reader opens "#/states" in a development server
    Then the states sheet marks "review" and "system" as mounted by "thread"
    And none of "task", "approval", "brief", "decision", "summary", "call", "report" or "handback" is mounted by "thread"
    And every message wire example is rendered on the sheet
