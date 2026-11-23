Feature: External AIs on a host

  Scenario: Adding external AIs needs an account and no agent.set (A10.17 host)
    Given a host on disk whose remote home "home-remote" hosts the project group "Website"
    When the owner joins the external AI "Zed" with account "team" and tool "ZCode" to "Website"
    And the owner joins the external AI "Yan" with account "api_key" to "Website"
    Then joining an external AI without an account, with account "free" or as secretary is "bad_request"
    And the home added 2 members with cli "external" and the host holds 2 external group sessions with no CLI session, both "offline"
    And the host's agents end with the agent "external" with takeover "none" and no models

  Scenario: An external AI takes one item per run and every run ends (A10.18, A10.23 host)
    Given a host whose external AI "Scout" is present and whose group has turn_minutes 1
    When the home mentions "Scout" twice and the runner takes, ends, fails and abandons runs
    Then the first hand-out is run 1 of kind "group" in the clone, with the group and external instructions in its context and the items in its text
    And while run 1 was open nothing more was handed out and "Scout" was "working"
    And after run 1 ended "Scout" was "waiting", run 2 was handed out once and a late done for run 1 changed nothing
    And the failed run 2 was reported as turn_interrupted "crash" with "exit 1"
    And the run nobody ended was reported as turn_interrupted "timeout" after 1 minute and "Scout" was "waiting"

  Scenario: Nothing is handed out while held, and an absent AI is offline (A10.24, A10.27 host)
    Given a host whose external AI "Scout" is present and whose group has turn_minutes 1
    When the group is paused, all AIs go offline and the runner stops, each with an item waiting
    Then "Scout" was "paused", "offline" and "offline" in turn, and no item was handed out
    And after the resume, the return online and the runner's return each waiting item was handed out
