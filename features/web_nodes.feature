@web
Feature: Connection indicator and my computers and AIs
  The connection indicator shows how this browser reaches the home of the open group, or its
  computers overall, in four states: direct with latency, relayed and limited, reconnecting, and
  unreachable. On a phone an open chat hides the top bar, so the indicator moves into the chat
  head. Opening the indicator or the Computers tab shows each owned computer with its system, node
  version, link path, hosted groups and registered AIs, declares an unset AI account type with
  agent.set, and starts pairing for another computer or another browser.

  Background:
    Given a browser set to English

  Scenario: The indicator shows all four states, names them for screen readers, and shows in phone chats
    Given an owned computer "workstation" that hosts the group "Website"
    When the browser opens "#/groups/rZNvy-1jH6Z-BcPqA5U5BQ" while selecting a candidate pair
    Then the indicator in the top bar has state "linking" and reads "Reconnecting"
    And the indicator links to "#/nodes"
    When the path to "workstation" becomes direct with latency 18 ms
    Then the indicator in the top bar has state "ok", reads "Direct 18 ms" and is named "Connection status: direct, 18 ms"
    When the path to "workstation" becomes relayed
    Then the indicator in the top bar has state "limited" and reads "Relayed · limited"
    When "workstation" stops
    Then the indicator in the top bar has state "down" and reads "Cannot reach group home"
    When the browser opens "#/nodes"
    Then the indicator in the top bar has state "down" and reads "Cannot reach computers"
    When the browser opens "#/groups/rZNvy-1jH6Z-BcPqA5U5BQ" on a 390 px phone
    Then the indicator in the chat head has state "down" and reads "Cannot reach group home"

  Scenario: Two computers are listed with direct, relayed, limited and offline states, and setting an account type updates the card
    Given an owned computer "workstation" on "macOS 15.2" running "v0.3.1" with direct path 4 ms and group "Website"
    And an owned computer "windows-box" on "Windows 11" running "v0.3.1" on a relayed path for 26 minutes with a limited AI in "Website"
    When the browser opens "#/nodes"
    Then the page lists "workstation" as "Online" on "macOS 15.2 · Node v0.3.1 · 2 of 4 AIs open" with link "Direct · 4 ms"
    And "workstation" warns "This computer is the home of 1 group — keep it on."
    And "workstation" lists "Claude Code" as "Personal subscription" and "Signed in", working in "Website" in the folder "/home/dev/website"
    And "workstation" lists "Codex" as "Account type not set" and "Not signed in" with the three account choices
    When the person chooses "Team plan" for "Codex" on "workstation"
    Then "workstation" receives a signed "agent.set" command setting "codex" to "team" and lists "Codex" as "Team plan"
    And the page lists "windows-box" as "Online" with link "Relay · 210 ms · 26 min", relay notice "Using a relay — speed and data are limited." and "Limited" AI state in the folder "/home/dev/office" of "Website"
    When "windows-box" stops
    Then "windows-box" is marked "Offline" and its AIs say "Offline · wakes when the computer returns"
    And the indicator in the top bar has state "some" and reads "Some computers unreachable"

  Scenario: An account type is corrected, with its members in the groups
    Given an owned computer "workstation" on "macOS 15.2" running "v0.3.1" with direct path 4 ms and group "Website"
    When the browser opens "#/nodes" and the person opens "Account type" of "Claude Code"
    Then "Personal subscription" is checked among the three choices
    When the group home refuses member commands and the person chooses "API key" for "Claude Code"
    Then "workstation" receives "agent.set" with "api_key" and "member set" of "Claude Code" with "api_key", and the choice stays
    And the card says "Not changed yet in Website." and why
    When the group home takes them again and the person presses "Try again"
    Then the note is gone and the member in "Website" has "api_key"
    When "workstation" refuses "agent.set" and the person chooses "Team plan" for "Claude Code"
    Then "API key" is checked again and no member command is sent

  Scenario: Adding a computer opens onboarding and adding a browser requests a one-time pairing link
    Given an owned computer "workstation" on "macOS 15.2" running "v0.3.1" with direct path 4 ms and group "Website"
    When the browser opens "#/nodes"
    Then the "Add a computer" link leads to "#/onboarding"
    When the person opens "Add a browser" and creates a pairing link on "workstation"
    Then "workstation" receives a signed "pair.browser" command for its device id
    And the dialog shows the pairing link "https://rv.example/#pair=7K2M-9QXD-4F8T-WB3N" with a copy button

  Scenario: An empty computers page shows a single Add a computer action and updates when a computer connects
    When the browser opens "#/nodes"
    Then the page shows the empty notice "No computers connected yet" and one "Add a computer" link to "#/onboarding"
    When an owned computer "workstation" connects with 3 open AIs
    Then the page lists "workstation" with "3 of 4 AIs open"
