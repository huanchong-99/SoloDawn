@web
Feature: Connection and tab status notices of the web client
  Only one tab of a browser connects at a time under the device Web Lock; an inactive tab shows
  that SoloDawn is open in another tab with a "Use here" button that steals the lock and closes
  the former holder's connections (planning/flows/connect.md section F; acceptance test A1.12).
  While a group's home cannot be reached, the chat pins a read-only banner above its main line,
  keeps cached messages readable, disables interactive write controls, and resumes sync without a
  reload when the home returns (planning/flows/sync.md section H; acceptance test A2.6).

  Scenario: A second tab shows the other-tab notice until Use here steals the lock and closes the first tab
    Given a browser paired with "workstation" using a device key beginning with "-"
    And the first tab is open and connected
    When a second tab opens at "#/groups"
    Then the second tab shows the other-tab notice "SoloDawn is open in another tab." with "Use here" and opens no connection
    And in Chinese the second tab notice reads "SoloDawn 已在另一个标签页中打开。" with "在此使用"
    When the person chooses "Use here" in the second tab
    Then the second tab clears the notice and connects, while the first tab closes its connections and shows the notice
    When a network online event fires in the first tab
    Then the first tab opens no new connection and keeps showing the notice

  Scenario: With the home unreachable the chat shows the read-only banner, keeps cached messages, disables write controls, and resumes on return
    Given the chat of "Website" on "workstation" is open with cached messages, a pending brief, a pending approval, and a pending decision
    Then the chat shows no unreachable-home banner and its write controls are enabled
    When the home stops
    Then the chat top banner says "Read-only · cannot reach the group home (workstation). Messages already here stay readable until it returns."
    And in Chinese the chat top banner says "只读 · 连不上群的家（workstation）。恢复连接前仍可查看已有记录。"
    And the cached messages remain readable while the composer, card actions, and Add AI button are disabled
    When the home returns with a message written meanwhile
    Then the chat top banner is gone, the missed message appears without reload, and the write controls are enabled again

  Scenario: An unnamed unreachable home omits parentheses and the states sheet lists status mounts
    Given a group whose unreachable home has no name yet
    When the person opens its chat
    Then the chat top banner says "Read-only · cannot reach the group home. Messages already here stay readable until it returns."
    When the person opens the states sheet in a development server
    Then the mounting points show "status · 10" at "banner" and "status · 10" at "chat.top"
