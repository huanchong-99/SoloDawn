Feature: Inbox wire delivery and wake-up cap in the home
  The home sends queued items to each AI's host in wire inbox frames with copies
  of the rows they name, sends unacknowledged items again on hello until their
  host acknowledges them, and holds excess wakes under the group's hourly cap.

  Scenario: An unacknowledged inbox item is sent again on reconnect, and not after ack
    Given a group home with a secretary and a working AI on the owner's laptop
    When a brief of 1 task for the working AI is posted and confirmed
    Then the laptop receives the confirmed rows and then an inbox frame with "assign" and "confirmed", carrying their task and message rows
    When the laptop reconnects and says hello before acknowledging the items
    Then the laptop receives the same "assign" and "confirmed" items again in 1 inbox frame
    When the owner's browser sends an ack for item 1
    And the laptop reconnects and says hello
    Then the laptop receives the same "assign" and "confirmed" items again in 1 inbox frame
    When the laptop sends an ack for item 1 and item 2
    And the laptop reconnects and says hello
    Then the laptop receives no inbox frame

  Scenario: Excess wakes for an AI wait for the hour, while resends count once
    Given a group home with a wake-up cap of 2 per hour and a working AI on the owner's laptop
    When 2 mentions are queued for the working AI
    Then the laptop receives 2 "mention" items
    When the laptop reconnects and says hello before acknowledging the items
    Then the laptop receives 2 "mention" items in 1 inbox frame
    When another mention is queued for the working AI 10 seconds later
    Then the laptop receives no inbox frame before 1 hour has passed since the 1st mention
    When 1 hour has passed since the 1st mention
    Then the laptop receives item 3
