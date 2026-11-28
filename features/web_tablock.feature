@web
Feature: One active tab per browser
  The tabs of one browser share its device key, and a node keeps one inbound connection per device,
  so only the tab that holds the device's Web Lock connects. Another tab says that SoloDawn is open
  in another tab and connects only when the person chooses "Use here" in it.

  Background:
    Given a browser whose device id begins with "-"

  Scenario: The second tab offers no connection
    Given the first tab has connected
    When a second tab opens
    Then the second tab shows the other-tab notice and has opened nothing

  Scenario: "Use here" moves the connection to the second tab
    Given the first tab has connected
    And a second tab shows the other-tab notice
    When the person chooses "Use here" in the second tab
    Then the first tab closes its signalling and peer connections and shows the other-tab notice
    And the second tab connects

  Scenario: A tab that lost the lock does not connect again on its own
    Given the second tab took the connection from the first
    When the first tab is asked to reconnect and to probe
    Then the first tab opens no signalling or peer connection

  Scenario: The inactive second tab shows only the other-tab placeholder in an empty group list
    Given the first tab has connected
    When a second tab opens its empty group list
    Then the group list shows "SoloDawn is open in another tab." and hides the empty-state prompt
    When the person chooses "Use here" in the second tab
    Then the group list returns to the normal empty state
