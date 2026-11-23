@web
Feature: Other-tab banner and inactive connection indicator
  While another tab of this browser holds the device Web Lock, the status area mounts the
  other-tab banner with "Use here" under the top bar, and the connection indicator shows the
  inactive state instead of claiming to reconnect (planning/flows/connect.md section F; acceptance
  test A1.12).

  Scenario: An inactive tab shows the other-tab banner and the inactive indicator instead of reconnecting
    Given an online computer "workstation" with no selected candidate pair while another tab holds the lock
    When the browser opens "#/groups"
    Then the banner shows "SoloDawn is open in another tab." with "Use here" and the indicator has state "offline" reading "Inactive"
    When the person chooses "Use here"
    Then the banner is gone and the indicator has state "linking" reading "Reconnecting"

  Scenario: The composer of an inactive tab says the group is in use in another tab
    Given a group "Website" read before another tab took the lock
    When the browser opens the chat of "Website"
    Then the composer is disabled and says "This group is in use in another tab."
    And the group list says "In another tab" and neither is styled as unreachable
