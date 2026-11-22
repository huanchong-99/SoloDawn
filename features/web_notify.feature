@web
Feature: In-page notifications of the web client
  Live events that need or mention this browser's reader appear in the shell's overlay layer while
  the reader is outside that group's main line: approvals, decisions and briefs waiting on the
  reader in dawn, and mentions and completed tasks in calm neutral (planning/product-rules.md R-34,
  planning/contracts/commands.md section 1.7). Opening a notification navigates to the group's chat
  and focuses its card or message; resolving a card, dismissing a notification, or waiting out a
  transient notice removes it cleanly.

  Background:
    Given the groups "Website" and "Docs" read in a browser of their owner, set to English

  Scenario: Blocking cards appear in dawn on other pages, jump to the card on Open, and clear when resolved
    Given the person opens the client at "#/nodes"
    Then the polite live region of notifications is in the page, empty, for the rows already held
    When "Codex" posts a pending approval in "Website" and a pending approval for "Mina"
    Then the notification layer shows one warm notification "Approval · Time the path change in a browser." for "Website · Codex"
    When "Secretary" posts a pending decision and a pending brief in "Website"
    Then the notification layer shows the approval, "Your call · Where should the path appear?" and "Confirm start · Show connection paths"
    When the approval card in "Website" is approved on another device
    Then the approval notification disappears, leaving the decision and the brief
    When the person clicks "Open" on the brief notification
    Then the chat of "Website" opens with focus on the "Confirm & start" button and no notifications remain

  Scenario: Mentions and completed tasks pause under focus or pointer, auto-dismiss after leaving, and yield to blocking cards
    Given the person opens the client at "#/settings"
    When "Codex" mentions the owner in "Website" with "Check `path.go` first" and "Secretary" posts a summary for task 12
    Then the notification layer shows two calm notifications: "Mentioned you · Check `path.go` first" and "Task done · T-12 Show connection path"
    When the person clicks "Dismiss notification" on the mention notification
    Then only the task-done notification remains
    When keyboard focus lands on "Open" of the task-done notification and 8 seconds pass
    Then the task-done notification stays visible with focus on its "Open" link
    When the pointer enters the task-done notification, focus leaves it, and 8 seconds pass
    Then the task-done notification stays visible
    When the pointer leaves the task-done notification and 8 seconds pass
    Then no notification is shown and the empty live region stays
    When "Codex" posts a pending approval in "Website", mentions the owner with "Look here", and "Secretary" posts a pending decision and a pending brief
    Then the mention yields its place so the approval, the decision and the brief stay visible

  Scenario: Main-line arrivals in the open chat stay quiet while thread mentions and other groups notify
    Given the person opens the client at "#/groups/rZNvy-1jH6Z-BcPqA5U5BQ"
    When "Codex" posts a pending approval in "Website" and mentions the owner in the main line of "Website"
    Then no notification is shown
    When "Codex" mentions the owner inside task 12's thread in "Website" and "Mina" mentions the owner in "Docs"
    Then the notification layer shows the thread mention for "Website · Codex" and the mention for "Docs · Mina"
    When the person clicks "Open" on the thread mention notification
    Then the chat stays on "Website" and focus moves to task 12's card

  Scenario: The states sheet lists notify at the shell's layer mounting point
    Given a development server
    When the person opens the states sheet
    Then the mounting points show "notify · 10" at "layer" and notifications translate to Chinese
