Feature: Step 3 style presets (A10.5)
  As a project group member
  I want style presets stored as typed notes and attached to task briefs
  So that interface tasks consistently receive the group's current style preset

  Scenario: A preset is written and replaced as a note of type preset and carried into every task of its brief
    Given a preset test group with retention set to 7 days
    When the owner writes the preset "Calm dashboard" with text "v1 draft"
    And the owner writes the preset "Calm dashboard" with text "Principles: one accent colour; no motion longer than 200 ms."
    And an AI writes the context note "Keep routes flat."
    Then note_list with type "preset" returns only "Calm dashboard" with its updated text and original id
    And note_list without type returns only the context note "Keep routes flat."
    When the secretary proposes a two-task brief naming preset "Calm dashboard" and the owner confirms it
    Then the assign turn of every task of that brief holds preset "Calm dashboard" unchanged and the review turn names it
    When the secretary proposes a brief without a preset and the owner confirms it
    Then the assign turn of that brief's task holds no style preset

  Scenario: Forgetting a preset after a brief is proposed notes its deletion in the task turn
    Given a preset test group with retention set to 7 days
    When the owner writes the preset "Transient" with text "Temporary palette."
    And the secretary proposes a brief naming preset "Transient"
    And the owner forgets the preset "Transient"
    And the owner confirms the pending brief
    Then the assign turn of that brief's task states that preset "Transient" was deleted since the brief was proposed

  Scenario: A brief naming a preset the group lacks is refused with no brief row
    Given a preset test group with retention set to 7 days
    When the owner writes the preset "Calm dashboard" with text "One accent colour."
    And the owner writes the preset "Airy" with text "Generous spacing."
    Then proposing a brief with unknown preset "Neon" fails with "not_found" listing "Airy, Calm dashboard" and writes no brief row

  Scenario: Presets obey A5.14 across daily retention and admin forget
    Given a preset test group with retention set to 7 days
    When 8 days ago the owner wrote the preset "Calm dashboard" and the text message "Old chatter"
    And an AI attempt to write preset "Hijack" is refused with "forbidden"
    And daily retention runs on the preset group today
    Then the old text message "Old chatter" is purged while preset "Calm dashboard" remains
    When a browser watches the preset group and the owner forgets the preset "Calm dashboard"
    Then the browser receives a gone marker for that preset within 1 second, fetch kind "preset" is empty, and the name "Calm dashboard" can be created again
