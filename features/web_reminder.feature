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
    When the person turns browser notifications on
    Then the browser notifications status reads "Allowed in this browser"
    And the service worker "./sw.js?lang=en" is registered with a valid VAPID subscription
    And the home "workstation" received a push subscription for "mention, approval, decision, brief, done"
    When the person navigates to "#/groups" and a second home "mini" connects with group "Docs"
    Then the home "mini" also received the same push subscription
    When the person returns to "#/settings" and unchecks "A task is done"
    Then both homes received a push subscription for "mention, approval, decision, brief"
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

  Scenario: iPhone and iPad require adding SoloDawn to the Home Screen before enabling notifications
    Given an iPhone browser tab that has not been added to the Home Screen
    When the person opens the client at "#/settings"
    Then the browser notifications switch is disabled with status "Add to Home Screen first"
    And the highlighted Home Screen note explains notifications and keeping the browser key after seven days without a visit
    When the same iPhone opens SoloDawn from the Home Screen with Web Push allowed
    Then the browser notifications switch is enabled and the Home Screen note is hidden
