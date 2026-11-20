Feature: Protocol wire compatibility
  Nodes and browsers exchange the same signed entries and group rows.

  Scenario: A forwarded device log preserves the bytes its author signed
    When a peer decodes and re-encodes the enrollment log
    Then both signatures verify for the original additions of "phone browser" and "workstation"

  Scenario: A client reads different row types in one update
    When a client decodes the mixed group update
    Then it sees task 12 at step "review"
    And the decision created at position 790 is now "decided" at sequence 814
    And the note at sequence 815 is a deletion marker
