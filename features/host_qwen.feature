Feature: The host drives Qwen Code over qwen serve
  The Qwen Code driver is declared from the documentation only (issues.md):
  the fake Qwen Code replays the documented daemon API, not a capture.

  Scenario: Two turns with a declined command approval run in one Qwen Code daemon
    Given a scripted Qwen Code daemon that asks to run "pnpm test" in the first of two turns
    When the host starts Qwen Code at level 1 and declines the command
    Then both Qwen Code turns end on session "qs-accept" in 1 process

  Scenario: Qwen Code reports a session it cannot resume and refuses takeover
    Given a scripted Qwen Code daemon without session "qs-gone"
    When the host resumes Qwen Code session "qs-gone"
    Then the Qwen Code driver reports no session and refuses terminal takeover
