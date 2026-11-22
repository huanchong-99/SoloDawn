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
    And in Chinese the banner reads "SoloDawn 已在另一个标签页中打开。" with "在此使用" and the indicator reads "未连接"
    When the person chooses "Use here"
    Then the banner is gone and the indicator has state "linking" reading "Reconnecting"

  Scenario: The states sheet lists the status mount at banner only
    When the person opens "#/states" in a development server
    Then the mounting points show "status · 10" at "banner" and do not list "status" at "chat.top"
