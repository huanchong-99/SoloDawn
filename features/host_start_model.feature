Feature: Host model choice
  Scenario Outline: Server CLI choice is sent at its supported process start (A10.15 host)
    Given a model-choice host with fake OpenCode and Qwen Code
    When the model-choice host joins "<agent>" as "One" with model "provider/unlisted", effort "custom" and tier "standard"
    And model-choice member "One" starts a turn
    Then the model-choice process received model "provider/unlisted" and effort "custom"
    When the home publishes model "provider/next" and effort "next-effort" for model-choice member "One"
    Then the model-choice running process is unchanged
    When model-choice member "One" resumes in a new process
    Then the model-choice process received model "provider/next" and effort "next-effort"
    And the model-choice session retained its CLI id
    Examples:
      | agent    |
      | opencode |
      | qwen     |

  Scenario Outline: Empty choices pass no server CLI configuration (A10.15 host)
    Given a model-choice host with fake OpenCode and Qwen Code
    When the model-choice host joins "<agent>" as "One" with model "", effort "" and tier "standard"
    And model-choice member "One" starts a turn
    Then the model-choice process received model "" and effort ""
    Examples:
      | agent    |
      | opencode |
      | qwen     |

  Scenario Outline: A refused server CLI choice crashes a task and keeps its session (A10.15 host)
    Given a model-choice host with fake OpenCode and Qwen Code
    When the model-choice host joins "<agent>" as "One" with model "provider/refused", effort "high" and tier "standard"
    And model-choice member "One" starts a task whose model is refused
    Then model-choice reports a crash and task fail with the original session id
    Examples:
      | agent    |
      | opencode |
      | qwen     |

  Scenario: Several members share a CLI account and leave individually (A10.15 host)
    Given a model-choice host with fake Claude Code and Codex
    When the model-choice host joins "claude" as "One" with model "opus", effort "high" and tier "strong"
    And the model-choice host joins "claude" as "Two" with model "haiku", effort "" and tier "light"
    Then both model-choice profiles are forwarded unchanged to the home
    And model-choice host.info lists both members under claude
    And model-choice leave without member is bad_request
    When model-choice leave selects "Two"
    Then only model-choice member "One" remains

  Scenario Outline: Choice applies at the next process start, including resume (A10.15 host)
    Given a model-choice host with fake Claude Code and Codex
    When the model-choice host joins "<agent>" as "One" with model "unlisted", effort "custom" and tier "standard"
    And model-choice member "One" starts a turn
    Then the model-choice process received model "unlisted" and effort "custom"
    When the home publishes model "next-model" and effort "next-effort" for model-choice member "One"
    Then the model-choice running process is unchanged
    When model-choice member "One" resumes in a new process
    Then the model-choice process received model "next-model" and effort "next-effort"
    And the model-choice session retained its CLI id
    Examples:
      | agent  |
      | claude |
      | codex  |

  Scenario Outline: Empty choices pass no model or effort (A10.15 host)
    Given a model-choice host with fake Claude Code and Codex
    When the model-choice host joins "<agent>" as "One" with model "", effort "" and tier "standard"
    And model-choice member "One" starts a turn
    Then the model-choice process received model "" and effort ""
    Examples:
      | agent  |
      | claude |
      | codex  |

  Scenario Outline: A refused model crashes a task without replacing its session (A10.15 host)
    Given a model-choice host with fake Claude Code and Codex
    When the model-choice host joins "<agent>" as "One" with model "refused", effort "high" and tier "standard"
    And model-choice member "One" starts a task whose model is refused
    Then model-choice reports a crash and task fail with the original session id
    Examples:
      | agent  |
      | claude |
      | codex  |
