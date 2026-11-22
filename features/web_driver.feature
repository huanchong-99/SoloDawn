@web
Feature: What a CLI's driver lets the browser offer
  Each entry of `agents` in `host.info` carries the driver's declaration, `driver`: the account
  types its terms allow and whether it has a takeover (planning/contracts/step-3/drivers.md
  section 6). The browser offers only those account types, and no "Open terminal" for an AI whose
  driver declares `takeover` `none` (acceptance tests A10.1 and A10.2 as seen in the client).

  Background:
    Given a browser set to English

  Scenario: A CLI that allows API keys only is offered no other account type
    Given an owned computer "workstation" with "claude" allowing every account type and "opencode" allowing API keys only, neither declared
    When the browser opens "#/nodes"
    Then "claude" offers "Personal subscription", "Team plan" and "API key", and "opencode" offers "API key" only
    When pairing finds the same two AIs
    Then the pairing page offers the same account types

  Scenario: An AI whose driver has no takeover offers no terminal
    Given an owned computer "workstation" on a direct path, home of a group with my AIs "Claude Code" on "claude" and "OpenCode" on "opencode" without a takeover
    When the browser opens the group
    Then "Claude Code" links to "Open terminal" and "OpenCode" offers no terminal
