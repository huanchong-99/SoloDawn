@web
Feature: Connection indicator and my computers and AIs
  The top bar shows how this browser reaches the home of the open group, or its computers overall,
  in four states: direct with latency, relayed and limited, reconnecting, and unreachable. Opening
  the indicator or the Computers tab shows each owned computer with its system, node version, link
  path, relay quota, hosted groups and registered AIs, and starts pairing for another computer or
  another browser.

  Background:
    Given a browser set to English

  Scenario: The top-bar indicator shows all four connection states and links to the computers page
    Given an owned computer "workstation" that hosts the group "Website"
    When the browser opens "#/groups/rZNvy-1jH6Z-BcPqA5U5BQ" while selecting a candidate pair
    Then the indicator in the top bar has state "linking" and reads "Reconnecting"
    And the indicator links to "#/nodes"
    When the path to "workstation" becomes direct with latency 18 ms
    Then the indicator in the top bar has state "ok" and reads "Direct 18 ms"
    When the path to "workstation" becomes relayed
    Then the indicator in the top bar has state "limited" and reads "Relayed · limited"
    When "workstation" stops
    Then the indicator in the top bar has state "down" and reads "Cannot reach group home"

  Scenario: Two computers are listed with direct, relayed and offline states, relay quota and AIs
    Given an owned computer "workstation" on "macOS 15.2" running "v0.3.1" with direct path 4 ms and group "Website"
    And an owned computer "windows-box" on "Windows 11" running "v0.3.1" on a relayed path for 26 minutes with quota 3.1 MB of 50 MB
    When the browser opens "#/nodes"
    Then the page lists "workstation" as "Online" on "macOS 15.2 · Node v0.3.1 · 2 of 4 AIs open" with link "Direct · 4 ms"
    And "workstation" warns "This computer is the home of 1 group — keep it on."
    And "workstation" lists "Claude Code" as "Personal subscription" and "Signed in", working in "Website"
    And "workstation" lists "Codex" as "Account type not set" and "Not signed in"
    And the page lists "windows-box" as "Online" with link "Relay · 210 ms · 26 min" and relay notice "Relay data today: 3.1 MB / 50 MB"
    When "windows-box" stops
    Then "windows-box" is marked "Offline" and its AIs say "Offline · wakes when the computer returns"

  Scenario: Adding a computer opens onboarding and adding a browser requests a one-time pairing link
    Given an owned computer "workstation" on "macOS 15.2" running "v0.3.1" with direct path 4 ms and group "Website"
    When the browser opens "#/nodes"
    Then the "Add a computer" link leads to "#/onboarding"
    When the person opens "Add a browser" and creates a pairing link on "workstation"
    Then "workstation" receives a signed "pair.browser" command for its device id
    And the dialog shows the pairing link "https://rv.example/#pair=7K2M-9QXD-4F8T-WB3N" with a copy button

  Scenario: An empty computers page prompts to add a computer, and a host.info push updates a card live
    When the browser opens "#/nodes"
    Then the page shows the empty notice "No computers connected yet" with a link to "#/onboarding"
    When an owned computer "workstation" connects and pushes a host.info update with 3 open AIs
    Then the page lists "workstation" with "3 of 4 AIs open"

