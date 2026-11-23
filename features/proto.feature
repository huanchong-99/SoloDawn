Feature: Protocol wire compatibility
  Nodes and browsers exchange the same signed entries and group rows.

  Scenario: A client reads the quota wire example
    Then the quota answer example reports 3250000 of 52428800 bytes

  Scenario: A forwarded device log preserves the bytes its author signed
    When a peer decodes and re-encodes the enrollment log
    Then both signatures verify for the original additions of "phone browser" and "workstation"

  Scenario: A client reads different row types in one update
    When a client decodes the mixed group update
    Then it sees task 12 at step "review"
    And the decision created at position 790 is now "decided" at sequence 814
    And the note at sequence 815 is a deletion marker

  Scenario: An approval keeps its original request after a person approves it
    When a client reads the pending approval and its approved update
    Then both show the same signed request for "pnpm exec playwright install chromium"
    And the update carries the approver's signed "approve" action for that card

  Scenario: A client matches an external AI in host info to its member row
    When a client reads the external AI member and host info examples
    Then the host advertises the member with tool "ZCode" and no takeover
