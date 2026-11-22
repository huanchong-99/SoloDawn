Feature: Host model discovery
  Scenario: OpenCode lists provider/model ids and variants while Qwen lists nothing (A10.13 host)
    Given a model-discovery host with scripted OpenCode and Qwen Code
    When the model-discovery host receives host.info
    Then the model-discovery host lists OpenCode variants and tiers and no Qwen models
    When the OpenCode listing has an unknown shape
    And the model-discovery host receives host.info with refresh
    Then both model lists are empty

  Scenario: Discovery preserves the CLI listing and applies word-based tier suggestions (A10.13 host)
    Given a model-discovery host with scripted Claude Code and Codex
    When the model-discovery host receives host.info
    Then the model-discovery host lists the scripted ids, names, efforts, fast flags and tiers
    When the scripted codex model listing fails
    And the model-discovery host receives host.info with refresh
    Then the codex models are empty and the other host.info fields are unchanged

  Scenario: Unknown listing shapes give an empty list (A10.13 host)
    Given a model-discovery host with scripted Claude Code and Codex
    When both model listings have an unknown shape
    And the model-discovery host receives host.info
    Then both model lists are empty

  Scenario: A refresh skips the shared 60 second detection and model cache (A10.14 host)
    Given a model-discovery host with scripted Claude Code and Codex
    And the model-discovery host receives host.info
    When the scripted claude list gains a model
    And 59 seconds pass for model discovery
    And the model-discovery host receives host.info
    Then the model-discovery host answers its old list
    When the model-discovery host receives host.info with refresh
    Then the model-discovery host answers the new list

  Scenario: Detection and model listings expire together after 60 seconds (A10.14 host)
    Given a model-discovery host with scripted Claude Code and Codex
    And the model-discovery host receives host.info
    When the scripted claude list gains a model
    And 60 seconds pass for model discovery
    And the model-discovery host receives host.info
    Then the model-discovery host answers the new list
