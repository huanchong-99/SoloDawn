@web
Feature: macOS permissions

  Background:
    Given the owner's Mac "workstation" whose node reports permissions

  Scenario: The card shows what macOS allows (WP.1)
    Given local network is not allowed, incoming is allowed and full disk is off
    Then the Computers page shows "macOS permissions" open, with "Needs you"
    And "Local network" says "Not allowed yet" and offers "Allow"
    And "Incoming connections" has a tick
    And "Files" offers "Allow all folders…"
    And exactly one host.permit without fields was sent when the card appeared

  Scenario: Asking, and the tick when macOS allows (WP.2)
    When the owner chooses "Allow"
    Then one more host.permit without fields is sent
    And the coach says to look at the screen of "workstation" and click Allow
    When host.info arrives with local network allowed
    Then "Local network" has a tick, the coach is gone and the badge says "Ready"
    And "Allowed: Local network" is announced

  Scenario: No window, so System Settings (WP.3)
    Given the coach of "Local network" is open
    When the owner chooses "No window? Open System Settings"
    Then host.permit is sent with open "local_network"
    And the coach says to scroll down to Local Network and switch SoloDawn on

  Scenario: A firewall that blocks (WP.4)
    Given incoming is not allowed
    When the owner chooses "Open System Settings" in "Incoming connections"
    Then host.permit is sent with open "incoming"
    And the coach names Options… and "Allow incoming connections"

  Scenario: Full Disk Access by the drag panel (WP.5)
    When the owner chooses "Allow all folders…"
    Then host.permit is sent with open "full_disk" and the page's language
    And the coach names the panel under System Settings and the + button
    When host.info arrives with full disk on
    Then "Files" has a tick and says Full Disk Access is on

  Scenario: A folder macOS must allow (WP.6)
    Given the folder dialog of Add AI for Claude Code on "workstation"
    When the owner uses "/Users/me/Documents/website" and the node has not answered after 500 ms
    Then the dialog says macOS is asking on "workstation"
    When the node answers ok
    Then the dialog closes with that folder
    When the owner uses "/Users/me/Downloads/x" and the node answers "permission"
    Then the dialog stays open and offers "Open System Settings", "Allow all folders…" and "Try again"
    When the owner chooses "Try again" and the node answers ok
    And the dialog closes with that folder

  Scenario: Other systems (WP.7)
    Given the owner's computer "tower" on "linux/amd64" without permissions
    Then its card shows no permissions and its folder dialog sends no host.permit
    Given the owner's computer "desk" on "windows/amd64" on a relayed path
    Then its card names Windows Security › Firewall & network protection

  Scenario: Before pairing, by the browser's system (WP.8)
    Then the onboarding page of a Mac browser lists the three things macOS shows
    And that of a Windows browser lists the Windows Security window
    And that of a Linux browser shows no such note

  Scenario: Phone width and reduced motion (WP.9)
    When the page is 375 px wide with a coach open
    Then no element is wider than the page and the buttons are at least 44 px high
    And with reduced motion no element of the card has a running animation
