@web
Feature: Brief, decision and summary cards of the web client
  The secretary proposes a brief with its goal, acceptance criteria, scope and task split, and
  posts a summary card once a task is done; AIs ask for a decision with options and a free-text
  answer field. Each card renders every wire state and sends signed act commands to the group home
  (planning/contracts/messages.md sections 2.1, 2.4, 2.5, 3.1, 3.3; planning/contracts/commands.md
  section 1.4; acceptance test A5.1).

  Background:
    Given the chat of "Website" open with its secretary, working AIs and task "T-12"

  Scenario: Confirming a pending brief sends act confirm and folds the confirmed brief
    Given the secretary has posted a pending brief
    Then the brief card stands expanded, marked "Needs you", with its goal, criteria, scope and split
    When the owner presses "Confirm & start" on the brief card
    Then the home receives a signed "confirm" act on the brief with no extra fields
    And the brief card shows "Alex confirmed" and folds its details

  Scenario: Sending a brief back with changes checks the 1 KB note limit and shows superseded state
    Given the secretary has posted a pending brief
    When the owner presses "Change it" on the brief card
    And the owner types a note of 1023 letters on the brief card
    Then the brief card warns that the note is too long and keeps "Send changes" disabled
    When the owner types "Include the relay path." and sends the changes
    Then the home receives a signed "revise" act on the brief with text "Include the relay path."
    And the brief card shows "Alex asked for changes" with the note "Include the relay path."
    When the home marks the brief superseded
    Then the brief card shows "Superseded" and keeps the note "Include the relay path."

  Scenario: Picking an option on a decision card sends act choose, alone or with a note
    Given an AI has posted a pending decision with four options recommending "B"
    Then the decision card shows all four options, marks "B" as "Recommended", and offers a free-text field
    When the owner picks option "A" without typing a note
    Then the home receives a signed "choose" act with arg "A" and no text
    And the decision card shows "Alex decided" and marks option "A" as "Chosen"
    Given an AI has posted a second pending decision with options "A" and "B"
    When the owner types "Quiet indicator." in the decision field and picks option "B"
    Then the home receives a signed "choose" act with arg "B" and text "Quiet indicator."
    And the second decision card marks option "B" as "Chosen" and shows "Quiet indicator."

  Scenario: Answering a decision card in free text works both with options and without options
    Given an AI has posted a pending decision with four options recommending "B"
    When the owner types a free-text answer of 1023 letters on the decision card
    Then the decision card warns that the answer is too long and disables its options and send button
    When the owner types "Put a subtle dot in the footer instead." and submits the free-text answer
    Then the home receives a signed "choose" act with text "Put a subtle dot in the footer instead." and no arg
    And the decision card shows "Alex decided" and "Put a subtle dot in the footer instead." with no option marked "Chosen"
    Given an AI has posted a pending free-text decision with no options
    When the owner types "Keep logs for 30 days." and submits the free-text answer
    Then the home receives a signed "choose" act with text "Keep logs for 30 days." and no arg

  Scenario: Cards waiting on another person, expired decisions, offline state and refused actions stay clear
    Given "Mina" is a non-admin member of "Website"
    And an AI has posted a pending decision addressed only to "Mina", and an expired decision
    And the home also holds the golden confirmed brief, revise brief and decided decision
    Then the decision for "Mina" says "Waiting on Mina", disables its options and hides the answer form
    And the expired decision shows "Expired"
    And the golden confirmed brief, revise brief and decided decision show their acted states
    Given the secretary has posted a pending brief for the owner, and the home refuses acts with "conflict"
    When the owner presses "Confirm & start" on the brief card
    Then the brief card reports the refusal "conflict"
    When the connection to the home is lost
    Then the pending brief disables its action buttons while offline

  Scenario: Summary cards show the done task, review score, rounds and formatted duration
    Given the secretary has posted the golden summary for "T-12" with text "All criteria passed."
    And the secretary has posted summaries of 1 round in 45 minutes and of 3 rounds in 2 hours for a task the browser does not hold
    Then the first summary card shows "Done", "T-12", "Show connection path", "94", "2 rounds" and "1h 12m"
    And the second summary card shows "1 round" and "45m" without a title, and the third "2h"
