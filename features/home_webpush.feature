Feature: Encrypted Web Push reminders from the home
  A browser subscribes to reminders from a home with its own push endpoint and VAPID key pair.
  When an enabled event concerns its person and that browser is offline, the home sends an RFC 8291
  encrypted short summary that holds neither message text nor an approval command; other connected
  devices of the same person do not suppress it.

  Scenario: Offline browser gets an encrypted approval summary while the owner's node and another browser stay connected
    Given a home with the group "Website", two browsers of the owner, and the owner's node hosting "Codex"
    When the first browser subscribes to Web Push for "mention, approval, decision, brief, done"
    And the second browser subscribes to Web Push for "mention, approval, decision, brief, done"
    And the first browser disconnects while the second browser and the owner's node stay connected
    And "Codex" posts an approval card in "Website" for command "pnpm exec playwright install chromium" with why "Install the browser test dependencies" and text "Need Chromium to time the path change."
    Then the push endpoint of the first browser receives 1 encrypted notification
    And the wire bytes received by the push endpoint contain none of "Website", "Install the browser", "pnpm exec playwright" or "Need Chromium"
    And decrypted by the first browser, the reminder is for group "Website", kind "approval", who "Codex" and title "Install the browser test dependencies", with none of "pnpm exec playwright" or "Need Chromium"
    And the push endpoint of the second browser receives 0 notifications

  Scenario: A subscription covers every group on the home, survives a restart, and ends on off
    Given a home with the group "Website", two browsers of the owner, and the owner's node hosting "Codex"
    When the first browser subscribes to Web Push for "mention, approval"
    And the second browser creates the group "Mobile" on the same home with "Codex"
    And the home restarts and loads its push subscriptions while only the owner's node reconnects
    And "Codex" posts a text message "Confidential release notes" mentioning the owner in "Mobile"
    Then the push endpoint of the first browser receives 1 encrypted notification
    And decrypted by the first browser, the reminder is for group "Mobile", kind "mention", who "Codex" and title "", with none of "Confidential release notes"
    When the first browser reconnects, turns Web Push off, and disconnects
    And "Codex" posts a text message "Second mention after off" mentioning the owner in "Mobile"
    Then the push endpoint of the first browser receives no further notifications
