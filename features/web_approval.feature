@web
Feature: Approval card and waiting bar of the web client
  An approval card shows a permission request from an AI in all five states, labelled with the
  request category and the justification the CLI attached, and warns when only the beginning of a
  long request is shown (planning/contracts/messages.md sections 2.3 and 3.2; planning/risks.md
  Q-18 and K-21; acceptance tests A4.1 and A4.2). Above the chat's main line, a bar lists the
  cards in that group that wait on the reader and jumps to each one.

  Background:
    Given the chat of the group "Website" open in a browser of its owner, set to English

  Scenario: The waiting bar lists the cards waiting on the reader and jumps to a card
    Given the group holds a pending brief, approval, decision and confirmation for the reader, and an approval for "Mina"
    Then the chat waiting bar reads "Waiting on you · 4" with chips for the brief, the approval, the decision and the confirmation
    When the person clicks the approval chip in the waiting bar
    Then the chat stays open on "Website" and focus moves to the Approve button of that card
    When the person clicks the brief chip in the waiting bar
    Then focus moves to the brief message in the main line
    When the person clicks the decision chip in the waiting bar
    Then focus moves to the decision message in the main line
    When the person clicks the confirmation chip in the waiting bar
    Then focus moves to the Confirm button of that card
    When the four cards waiting on the reader are no longer pending
    Then the chat has no waiting bar

  Scenario: Approving a pending card sends act approve and shows the approved state
    Given a pending command approval for "Codex" on task 12 without truncation
    Then the card has the dawn edge, "Needs you", category "Command", "workstation (your computer) · Task T-12", the command and the reason
    And the card shows no truncation warning
    When the person clicks "Approve"
    Then the Approve and Deny buttons are disabled while the home has not answered
    When the home answers and writes the approved row
    Then the home received an act command of action "approve" for that card
    And the card has no dawn edge, reads "Approved by Alex" and says "Approved for this one request. Codex can continue."

  Scenario: Denying a truncated request shows the K-21 warning first and then the denied state
    Given a pending file approval of 4096 bytes for "Codex" with no reason
    Then the card shows category "File", omits the "Why" row, and warns that only the beginning of 4096 bytes is shown
    And the chat waiting bar names the command as the chip subject
    When the person clicks "Deny" and the home writes the denied row
    Then the home received an act command of action "deny" for that card
    And the card reads "Denied by Alex" and says "Codex will try another way without this permission, or report back."

  Scenario: A pending approval for another member is shown without warm emphasis and cannot be clicked
    Given "Mina" owns "Claude Code" on "Mina's MacBook" and a pending network approval waits on "Mina"
    Then the card has no dawn edge, reads "Waiting on Mina" and category "Network", and shows "Mina's MacBook" without "your computer"
    And the Approve and Deny buttons are disabled with the lock hint "Only the computer's owner Mina can approve"
    When a pending approval waits on an unknown user with category "other"
    Then that card reads "Waiting for owner", category "Other", and "Only the computer's owner can approve"

  Scenario: Offline and refused actions disable or report errors cleanly
    Given a pending command approval for "Codex" on task 12 without truncation
    When the home stops
    Then the Approve and Deny buttons are disabled and the card says "Cannot reach the group home"
    When the home returns, refuses act commands with "conflict", and the person clicks "Approve"
    Then the card alerts "Not sent. The home answered: conflict" and enables the buttons again

  Scenario: The states sheet renders all five wire examples of approval cards
    Given a development server
    When the person opens the states sheet
    Then the mounting points show "approval · 20" at "chat.top" and the approval section is mounted by "approval"
    And the five approval examples show "Approved by owner", "Denied by owner", "Expired", "Needs you" and "Reassigned"
