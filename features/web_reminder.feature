@web
Feature: Browser push reminders and iOS Home Screen hint
  On Settings, a person turns browser notifications on or off and chooses which moments remind
  them (`commands.md` section 1.7). The browser creates a VAPID key pair, subscribes through its
  service worker, and hands the subscription to each connected home. A reminder carries only the
  group, card type, author name and one-line title, shows them in the reader's language, and opens
  the card when pressed (A8.3). On iPhone and iPad before adding SoloDawn to the Home Screen,
  Settings explains why adding to the Home Screen is required first (risks.md K-20).

  Background:
    Given a browser set to English

  Scenario: Enabling notifications subscribes the browser, syncs homes, updates choices, and turns off
    Given a browser that allows Web Push notifications
    And the home "workstation" holds the group "Website"
    When the person opens the client at "#/settings"
    Then the browser notifications status reads "Off in this browser"
    And the Home Screen note is not shown on a desktop browser
    And the choices read "I am @mentioned, A decision is needed, An approval is needed, A brief awaits confirmation, Someone wants to use my AI, A task is done"
    When the person turns browser notifications on
    Then the browser notifications status reads "Allowed in this browser"
    And the service worker "./sw.js?lang=en" is registered with a valid VAPID subscription
    And the home "workstation" received a push subscription for "mention, approval, decision, brief, confirm, done"
    When the person navigates to "#/groups" and a second home "mini" connects with group "Docs"
    Then the home "mini" also received the same push subscription
    When the person returns to "#/settings" and unchecks "A task is done"
    Then both homes received a push subscription for "mention, approval, decision, brief, confirm"
    When the person turns browser notifications off
    Then the browser notifications status reads "Off in this browser" again
    And the browser subscription is unsubscribed and both homes received push off

  Scenario: A closed-page push reminder holds no command or message text, shows the summary, and opens the card
    Given the home "workstation" holds the group "Website" with a pending approval card
    And the service worker receives the wire approval reminder while no window is open
    Then the pushed payload holds neither message text nor approval command
    And the browser notification reads "Website · Approval" and "Codex: Time the path change in a browser."
    When the person clicks the notification to open the closed client
    Then the client opens the chat of "Website" with focus on the approval card's "Approve" button

  Scenario: Clicking a reminder while SoloDawn is open in Chinese focuses the tab and jumps to the card
    Given the home "workstation" holds the group "Website" with a pending approval card
    And the person opens the client at "#/settings" in Chinese
    When the service worker receives the wire approval reminder in Chinese and the person clicks it
    Then the browser notification reads "Website · 审批申请" and "Codex: Time the path change in a browser."
    And the open window moves to the chat of "Website" with focus on the approval card's "批准" button

  Scenario: Clicking the reminder of a task that stopped opens its thread
    Given the home "workstation" holds the group "Website" with a task
    When the service worker receives the reminder that the task stopped and the person clicks it
    Then the browser notification reads "Website · Task stopped" and "Codex: Login page"
    And the client opens the chat of "Website" with the task's thread open and focus on its line

  Scenario: A reminder opens the closed client at a task older than what the client reads at first
    Given the home "workstation" holds the group "Website" with a task and 60 newer messages
    When the person clicks the reminder that the task stopped, with the client closed, and the first read breaks off
    Then the client has not asked for the task's message
    When the connection is back
    Then the client asks the home for the task's message once
    And the task's thread is open with focus on its line

  Scenario: A reminder of what the home no longer has leaves the person alone
    Given the home "workstation" holds the group "Website" without the task's message
    When the person clicks the reminder that the task stopped, with the client closed
    Then the client asks the home for the task's message once
    When the person writes in the composer, the task's message arrives after all and the connection is renewed
    Then focus stays in the composer and the thread is closed

  Scenario: A reminder clicked in an open window whose connection is gone opens once it is back
    Given the home "workstation" holds the group "Website" with a task
    And the client is open and its connection is gone
    When the person clicks the reminder of a task whose message came meanwhile
    And the connection is back
    Then the task's thread is open with focus on its line

  Scenario: A reminder whose message was asked for on a dead connection opens on the next one
    Given the home "workstation" holds the group "Website" with a task and 60 newer messages
    And the client is open and its connection is dead without its knowing
    When the person clicks the reminder that the task stopped
    And the request is lost
    Then the client has asked once, whatever else arrived, and the chat has no task
    When the connection is back
    Then the task's thread is open with focus on its line

  Scenario: iPhone and iPad require adding SoloDawn to the Home Screen before enabling notifications
    Given an iPhone browser tab that has not been added to the Home Screen
    When the person opens the client at "#/settings"
    Then the browser notifications switch is disabled with status "Add to Home Screen first"
    And the highlighted Home Screen note explains notifications and keeping the browser key after seven days without a visit
    When the same iPhone opens SoloDawn from the Home Screen with Web Push allowed
    Then the browser notifications switch is enabled and the Home Screen note is hidden
