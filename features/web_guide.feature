@web
Feature: The first-run guide

  Scenario: A brand-new browser is welcomed and taken to pairing (G.1)
    Given a browser with no computer and no group
    When it opens the client
    Then the welcome dialog says "Welcome to SoloDawn" and lists the five steps
    When the person chooses "Start"
    Then the guide dock is open at 0 of 5 with "Connect your computer" current
    When the person chooses "Connect"
    Then the address is "#/onboarding" and neither the guide dock nor its pill is shown

  Scenario: Each step checks off from the real state (G.2)
    Given the guide is open
    When the browser has the computer "workstation" whose Claude Code has no account type
    Then step 1 is done with "Connected "workstation"" and step 2 is current
    When Claude Code gets the account type personal
    Then step 2 is done and step 3 is current, saying "Still to set: Claude Code"
    When host.info gives Claude Code defaults
    Then step 3 is done and step 4 is current
    When the group "Website" has a secretary and 1 working AI
    Then step 4 stays current and its action is "Add AI"
    When the group "Website" has a secretary and 2 working AIs besides it
    Then step 4 is done with "Created "Website"" and step 5 is current
    When the person's text message appears in the main line of "Website"
    Then the dock shows "You're all set" with 5 of 5
    When the person chooses "Done"
    Then the dock is gone and the stored guide state is "done"

  Scenario: Clicks alone do not tick a step (G.3)
    Given the guide is open at step 3
    When the person chooses "Set them"
    Then the address is "#/nodes" and the beacon surrounds Claude Code's "Set defaults" button
    And step 3 is still current

  Scenario: No AI found, and a computer that is offline (G.4)
    Given the guide is open and the computer "workstation" lists no CLI
    Then step 2 says no AI coding assistant was found and offers "Check again"
    When the person chooses "Check again"
    Then host.info is requested with refresh
    When the computer goes offline
    Then step 2 says ""workstation" is offline"

  Scenario: Skip, then reopen from the top bar and from Settings (G.5)
    Given the welcome dialog is shown
    When the person chooses "Look around first"
    Then no dock is shown and the note says the guide can be reopened in Settings › Getting started
    And the top bar has "Guide 0/5"
    When the person chooses "Guide 0/5"
    Then the dock is open
    When the person closes the guide
    And opens Settings and chooses "Open the guide"
    Then the dock is open again

  Scenario: Nobody else is welcomed (G.6)
    Given a browser with a paired computer and a group
    When it opens the client
    Then no welcome dialog, no dock and no chip are shown
    Given a browser that joined a group by invitation
    When the joined browser opens the client
    Then no welcome dialog is shown

  Scenario: Phone width (G.7)
    Given the guide is open at step 3 on a page 375 px wide
    Then the dock is a pill above the bottom navigation and no element is wider than the page
    When the person taps the pill
    Then the steps open as a bottom sheet with its close button
    When the person opens the chat of "Website"
    Then the pill is hidden

  Scenario: Reduced motion (G.8)
    Given the person prefers reduced motion
    When a step is done and the beacon is shown
    Then no element of the guide has a running animation
