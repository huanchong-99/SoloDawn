Feature: Scripted CLI session ownership
  Scenario Outline: A test can observe every process switch without a vendor account
    Given "<cli>" is found first on PATH with version "<version>"
    When I switch between structured and terminal starts 10 times on session "797fa93e-af8a-4768-b847-42582d5583a0"
    Then the lifecycle log contains 11 start-exit pairs for that session in order

    Examples:
      | cli    | version               |
      | claude | 2.1.311 (Claude Code) |
      | codex  | codex-cli 0.164.0     |
