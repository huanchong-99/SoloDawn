@web
Feature: The emergency stop in the web client
  An admin pauses every AI of a group and resumes it (R-64, A4.7); the owner of a computer takes all
  of their AIs there offline and back (A4.8). "Stop AIs" is in the top bar of every page and, on a
  phone, in the head of a chat. Its panel is the confirmation. While a group is paused everybody sees
  it above the composer and in the group list; while AIs are offline their owner sees it under the
  top bar, on the computer and on the AIs.

  Background:
    Given a browser of Alex, who owns "workstation", the home of "Website" with a working AI

  Scenario: The owner pauses the group and resumes it
    When Alex opens "Website" and then "Stop AIs"
    Then the panel offers to pause the group, where one AI is working, and to take the AIs on workstation offline
    When Alex presses "Pause"
    Then the home receives "group pause" and the panel closes
    And above the composer it says that Alex paused the group, what becomes of new messages, and "Resume"
    And the group list marks "Website" as paused, and "Stop AIs" no longer offers to pause it
    When Alex opens the thread of a task and presses "Resume"
    Then the home receives "group resume", the notice and the mark are gone and focus is in the main composer

  Scenario: A refused pause stays in the panel with its reason
    Given the home refuses commands with "forbidden"
    When Alex opens "Website" and then "Stop AIs"
    And Alex presses "Pause"
    Then the panel is still open and says that this is not allowed

  Scenario: A panel closed while its action runs opens again on a refusal, and else leaves the person alone
    When Alex opens "Website", presses "Pause" in the panel and goes on writing
    And the home refuses the pause
    Then the panel opens again and says that this is not allowed
    When Alex presses "Pause" again and goes on writing, and the home takes it
    Then focus stays where Alex writes

  Scenario: A refused resume says why beside the button
    Given Mina paused the group
    And the home refuses commands with "forbidden"
    When Alex opens "Website"
    Then above the composer it says that Mina paused the group
    When Alex presses "Resume"
    Then the notice says that this is not allowed

  Scenario: A member sees who paused the group and cannot resume it
    Given Alex is an ordinary member of a group on a computer of someone else
    And Mina paused the group
    When Alex opens "Website"
    Then above the composer it says that Mina paused the group and that an admin resumes it, without a button
    And there is no "Stop AIs"

  Scenario: The home is away
    Given "workstation" stops
    When Alex opens "Website" and then "Stop AIs"
    Then both actions are disabled and the panel says that the computer cannot be reached

  Scenario: The owner takes the AIs on their computers offline from any page, and back
    Given a second computer "mac-mini" of Alex
    When Alex opens the settings page and then "Stop AIs"
    Then the panel offers only to take the AIs on workstation and mac-mini offline
    When Alex presses "Take offline"
    Then both computers receive "host.offline", the panel closes and "Stop AIs" is gone
    And under the top bar it says that the AIs on workstation and mac-mini are offline
    And the AIs of "Website" read "Offline · by you"
    When Alex presses "Bring back online"
    Then both computers receive "host.online" and the banner is gone

  Scenario: A person who left the panel keeps their place when the button goes away with the action
    When Alex opens the settings page and then "Stop AIs"
    And Alex presses "Take offline" and moves on to the top bar
    Then "Stop AIs" is gone and focus stays in the top bar

  Scenario: One computer of two cannot be reached
    Given a second computer "mac-mini" of Alex
    And "mac-mini" stops
    When Alex opens the settings page and then "Stop AIs"
    Then the panel says that mac-mini cannot be reached now
    When Alex presses "Take offline"
    Then only workstation receives "host.offline"

  Scenario: A computer that refuses is told in the panel and under the top bar
    Given "workstation" refuses host commands
    When Alex opens the settings page and then "Stop AIs"
    And Alex presses "Take offline"
    Then the panel is still open and says that this is not allowed
    When the AIs of "workstation" are offline and Alex presses "Bring back online"
    Then the banner says that this is not allowed

  Scenario: On the computers page the owner confirms in place
    When Alex opens the computers page
    And Alex presses "Take offline" on "workstation"
    Then the card says what happens and focus is on its strip
    When Alex presses "Cancel"
    Then the card is as before and "workstation" has received nothing
    When Alex presses "Take offline" on "workstation" twice
    Then "workstation" receives "host.offline" and its card and its AIs say that they are offline
    And no banner repeats it on this page
    When Alex presses "Bring back online"
    Then "workstation" receives "host.online" and the card is as before

  Scenario: Another browser of the owner takes the AIs offline
    When Alex opens "Website"
    And another browser takes the AIs on "workstation" offline
    Then under the top bar it says that the AIs on workstation are offline

  Scenario: After a reload a computer that connects later is read on every page
    Given the AIs of "workstation" are offline, with no AI in a session, and its connection is down
    When Alex opens the settings page and then "workstation" connects
    Then under the top bar it says that the AIs on workstation are offline
    And "Stop AIs" is gone, for there is nothing to stop

  Scenario: The panel opens under its button wherever the button is
    When Alex opens "Website" and the button is pushed down to 155 px
    And Alex opens "Stop AIs"
    Then the panel starts 8 px under the button

  Scenario: On a phone the button is in the head of the chat
    When Alex opens "Website" on a screen 390 px wide
    Then "Stop AIs" is in the head of the chat and not in the top bar
