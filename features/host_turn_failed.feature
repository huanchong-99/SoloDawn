Feature: Failed turns report visibly while keeping the open CLI waiting

  Scenario: A failed turn in the group session posts turn_interrupted and stays waiting
    Given a group "Website" with a configured "Claude Code" member on "claude" whose failed turn keeps the CLI open
    And "Claude Code" is scripted to fail once with "API error 503: upstream down\nretry later" and later post "Recovered reply"
    When the owner sends "@Claude Code please check the build" to "Website"
    Then a "turn_interrupted" crash row for "Claude Code" with detail "API error 503: upstream down" is posted in "Website"
    And "Claude Code" stays in state "waiting" in "Website"
    When the owner sends "@Claude Code try once more" to "Website"
    Then "Claude Code" replies with "Recovered reply" in "Website"
    And the fake "claude" log records 1 process for that group conversation

  Scenario: A turn ending with login_invalid posts a login_invalid notice and notifies LoginChanged after Dormant and limited
    Given a group "Website" with a configured "Claude Code" member on "claude" whose failed turn keeps the CLI open
    And the host records LoginChanged snapshots for "Claude Code" in "Website"
    And "Claude Code" is scripted to fail sign-in with "Failed to authenticate: OAuth token has expired"
    When the owner sends "@Claude Code please check the build" to "Website"
    Then a "login_invalid" notice for "Claude Code" with detail "Failed to authenticate: OAuth token has expired" is posted in "Website"
    And "Claude Code" enters state "limited" in "Website"
    And LoginChanged fired once with session "dormant", 0 open sessions, member state "limited", and "claude" login_ok false
    When owner device "owner-1" sends agent.recheck for "claude"
    Then "Claude Code" enters state "dormant" in "Website"
    And LoginChanged fired twice with session "dormant", 0 open sessions, member state "dormant", and "claude" login_ok true
