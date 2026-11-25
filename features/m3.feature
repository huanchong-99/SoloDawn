Feature: M3 AI sessions on the local link
  Claude Code and Codex sessions run over a node's in-process local link and
  local MCP socket with the scripted fake CLIs, preserving session memory
  across turns and across idle dormancy, enforcing max_open, reporting mid-turn
  crashes, keeping unposted output and secrets out of the group, deduplicating
  repeated inbox items across restarts, and listing the contract's MCP tools.
  After each scenario its one-person group shows no confirm card (A9.40).

  Scenario: Live MCP tool lists match the frozen contract across session kinds
    Given an in-process M3 node with a local MCP socket
    When MCP clients list tools through the shim for each session kind
    Then every Codex session lists the main-line tools and schemas of tools.json
    And structured Claude Code sessions add only approve

  Scenario: Mentioning a Claude Code member twice runs in one CLI process and remembers the first mention
    Given an in-process M3 node with group "Website" and configured AI "Claude Code" on "claude"
    And "Claude Code" is scripted for two turns that post "Noted alpha-7." and "First mention said alpha-7."
    When the owner posts "@Claude Code remember alpha-7" in "Website"
    Then "Claude Code" posts "Noted alpha-7." in "Website" and enters state "waiting"
    When the owner posts "@Claude Code what did I say" in "Website"
    Then "Claude Code" posts "First mention said alpha-7." in "Website"
    And the CLI lifecycle log records 1 "claude" process for the whole conversation

  Scenario: Mentioning a Codex member twice runs in one CLI process and remembers the first mention
    Given an in-process M3 node with group "Website" and configured AI "Codex" on "codex"
    And "Codex" is scripted for two turns on thread "01a1f63f-701c-78c1-bbf3-f1ac61c56b59" that post "Noted beta-9." and "First mention said beta-9."
    When the owner posts "@Codex remember beta-9" in "Website"
    Then "Codex" posts "Noted beta-9." in "Website" and enters state "waiting"
    When the owner posts "@Codex what did I say" in "Website"
    Then "Codex" posts "First mention said beta-9." in "Website"
    And the CLI lifecycle log records 1 "codex" process on session "01a1f63f-701c-78c1-bbf3-f1ac61c56b59"

  Scenario: A configured AI goes dormant after the 1-minute idle period and resumes its session id
    Given an in-process M3 node with idle_minutes 1, group "Website", and configured AI "Claude Code" on "claude"
    And "Claude Code" is scripted to post "Before idle." and wait for stdin close
    When the owner posts "@Claude Code store gamma-3" in "Website"
    Then "Claude Code" posts "Before idle." in "Website" and enters state "waiting"
    When the 1-minute idle period elapses
    Then the first "claude" process has exited and "Claude Code" has state "dormant" in "Website"
    When "Claude Code" is scripted for a resumed turn that posts "Resumed gamma-3."
    And the owner posts "@Claude Code recall gamma-3" in "Website"
    Then "Claude Code" posts "Resumed gamma-3." in "Website"
    And the CLI lifecycle log records 2 "claude" starts with the same session id

  Scenario: With max_open 1 a waiting AI goes dormant first and a working AI finishes before the second starts
    Given an in-process M3 node with max_open 1, group "Website", configured AI "Claude Code" on "claude", and configured AI "Codex" on "codex"
    And "Claude Code" is scripted to post "Claude turn 1." and wait for stdin close
    And "Codex" is scripted on thread "01a1f63f-701c-78c1-bbf3-f1ac61c56b60" for a first turn that posts "Codex turn 1." and a second turn that waits on command approval before posting "Codex turn 2."
    When the owner posts "@Claude Code first" in "Website"
    Then "Claude Code" posts "Claude turn 1." in "Website" and enters state "waiting"
    When the owner posts "@Codex second" in "Website"
    Then "Claude Code" enters state "dormant" before "Codex" posts "Codex turn 1." in "Website"
    When "Claude Code" is scripted for a resumed turn that posts "Claude after Codex."
    And the owner posts "@Codex hold the slot" in "Website"
    And the owner waits for the approval card from "Codex" in "Website"
    And the owner posts "@Claude Code wait your turn" in "Website"
    Then "Claude Code" has not started a second process while "Codex" is working
    When the owner approves that card in "Website"
    Then "Codex" posts "Codex turn 2." before "Claude Code" posts "Claude after Codex." in "Website"

  Scenario: A CLI exiting mid-turn produces a turn_interrupted row and the next mention resumes
    Given an in-process M3 node with group "Website" and configured AI "Claude Code" on "claude"
    And "Claude Code" is scripted to exit mid-turn after reporting its session id
    When the owner posts "@Claude Code crash now" in "Website"
    Then a "turn_interrupted" system row for "Claude Code" with reason "crash" appears in "Website"
    And "Claude Code" has state "dormant" in "Website"
    When "Claude Code" is scripted for a resumed turn that posts "Back online."
    And the owner posts "@Claude Code try again" in "Website"
    Then "Claude Code" posts "Back online." in "Website"
    And the CLI lifecycle log records 2 "claude" starts with the same session id

  Scenario: Text printed by the model outside tools does not appear in the group
    Given an in-process M3 node with group "Website" and configured AI "Claude Code" on "claude"
    And "Claude Code" is scripted for two turns that print "UNPOSTED_ALPHA" with post "Visible reply." and then print "UNPOSTED_BETA" without calling post
    When the owner posts "@Claude Code speak once" in "Website"
    Then "Claude Code" posts "Visible reply." in "Website" and enters state "waiting"
    When the owner posts "@Claude Code think silently" in "Website"
    Then "Claude Code" completes the second turn without posting a second message
    And no message in "Website" contains "UNPOSTED_ALPHA" or "UNPOSTED_BETA"

  Scenario: A post from an AI containing a secret fails with secret and writes no row on the home
    Given an in-process M3 node with group "Website" and configured AI "Claude Code" on "claude"
    And "Claude Code" is scripted to post a test secret and expect tool error "secret"
    When the owner posts "@Claude Code check token" in "Website"
    Then "Claude Code" finishes the turn after receiving the "secret" tool error
    And no message from "Claude Code" exists on the home for "Website"

  Scenario: An inbox item sent twice by the home produces one turn, also across a host restart
    Given an in-process M3 node with group "Website", configured AI "Claude Code" on "claude", and acknowledgements held on the link
    And "Claude Code" is scripted for one turn that posts "Delivered once."
    When the owner posts "@Claude Code once" in "Website"
    Then "Claude Code" posts "Delivered once." in "Website" and queues 1 acknowledgement
    When the home sends its due inbox items again on a new connection
    Then the host queues a 2nd acknowledgement without starting another CLI process
    When the host restarts on the node's store and attaches to the home
    Then the restarted host acknowledges the resent item and "Claude Code" still ran 1 CLI process and posted 1 message
