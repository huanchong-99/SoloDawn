@web
Feature: Task thread, review block and system notices
  A task's thread pages from its newest messages back, renders every message kind with its badge,
  mentions, code spans and body block, shows when the task's assignee AI is working on it, draws
  scored review blocks with pass thresholds and veto items, and words every system notice of the
  home in the reader's language, showing task merges under the secretary's name.

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

  Scenario: A thread renders every message kind, report, checks, hand-back, call fallback and working indicator
    Given the thread of task 12 holds every thread message kind
    And the assignee "Codex" is working on task 12 for 6 minutes
    When the thread of task 12 is mounted
    Then the thread shows the badges for progress, question, call, reply, report, hand-back, approval and decision
    And the question marks the mention "@Secretary" and the code span "path"
    And the report metadata shows PR 31, short commit "9f2c1ab", "4 files" and "+212 −18"
    And the check rows show passing local checks, failing cloud checks and a merge conflict
    And the hand-back metadata shows "1 file", "path.go", "+40 −3" and commit "9f2c1ab Emit path changes"
    And the bottom of the thread shows "Codex is working · 6 min"
    When "Codex" works on another task or is waiting
    Then the working indicator is gone

  Scenario: Review blocks show pass, sent-back and veto states with file locations
    Given the thread of task 12 holds a sent-back review, a veto review and a passed review
    When the thread of task 12 is mounted
    Then the sent-back review has tone "danger", badge "Review · Sent back", score "86", "Pass 90" and locations "path.go:88" and "conn.go"
    And the veto review shows the "Veto" badge and score "0"
    And the passed review has tone "ok", badge "Review · Passed" and score "94"

  Scenario: Task merges appear under the secretary's name and every system code has client wording
    Given the thread of task 12 holds a task_merged notice and every other system code
    And the main line also holds a task_merged notice
    When the thread of task 12 is mounted
    Then the task_merged row names "Secretary", "T-12", "Show connection path" and commits "9f2c1ab → def4567" with an ok check icon
    And none of the 12 system rows shows a raw system code
    And switching the language to Chinese words task_merged under "Secretary" as "已合并 T-12"
    When the reader opens the main line of "Website"
    Then the main line also renders the task_merged notice under "Secretary"

  Scenario: The states sheet renders every wire example with the thread area's mounted cards
    When the reader opens "#/states" in a development server
    Then the states sheet marks "review", "checks", "system", "report" and "handback" as mounted by "thread"
    And none of "task", "approval", "brief", "decision", "summary" or "call" is mounted by "thread"
    And every message wire example is rendered on the sheet
