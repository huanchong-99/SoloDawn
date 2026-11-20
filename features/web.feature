@web
Feature: Web client
  The web client is one responsive page that a node and the rendezvous serve as static files.
  It opens in the person's language and theme, moves between its pages by the address fragment,
  and tells a browser below the supported versions to update instead of failing later. It reads
  every frame from a node against the schema of its message, and a mock connection answers from
  the node's wire examples so that screens can be built with no node.

  Background:
    Given a browser set to Chinese and a light system theme

  Scenario: The shell opens on the chats page
    Given the browser supports Ed25519 in WebCrypto
    When the person opens the client
    Then the navigation reads "群聊", "电脑和 AI", "设置"
    And the current page is "群聊"

  Scenario: The address fragment selects the page
    Given the browser supports Ed25519 in WebCrypto
    And the person opens the client
    When the address fragment becomes "#/nodes"
    Then the current page is "我的电脑和 AI"
    And focus is on the main landmark named "我的电脑和 AI"

  Scenario: The skip button moves focus into the current page
    Given the browser supports Ed25519 in WebCrypto
    And the person opens the client
    When the person activates the skip button
    Then focus is on the main landmark named "群聊"

  Scenario: An unknown address opens the chats page
    Given the browser supports Ed25519 in WebCrypto
    When the person opens the client at "#/nowhere"
    Then the current page is "群聊"

  Scenario: The language choice changes the interface texts and is remembered
    Given the browser supports Ed25519 in WebCrypto
    And the person opens the client at "#/settings"
    When the person chooses the language "English"
    Then the navigation reads "Chats", "Computers", "Settings"
    And the page is marked as "en"
    And the navigation reads "Chats", "Computers", "Settings" after the client is opened again

  Scenario: The theme choice is applied and remembered
    Given the browser supports Ed25519 in WebCrypto
    And the person opens the client at "#/settings"
    When the person chooses the theme "深色"
    Then the page uses the "dark" theme
    And the page uses the "dark" theme after the client is opened again

  Scenario: The top bar switches theme and language
    Given the browser supports Ed25519 in WebCrypto
    And the person opens the client
    When the person presses "切换深色 / 浅色"
    Then the focused theme button announces "主题: 深色"
    When the person presses "Switch language / 切换语言"
    Then the page uses the "dark" theme
    And the page is marked as "en"
    And the focused language button announces "Language: English"

  Scenario: The mark of the current page moves to the entry that is opened
    Given the browser supports Ed25519 in WebCrypto
    And the person opens the client
    When the address fragment becomes "#/settings"
    Then the mark of the current page is on entry 3 of the navigation
    And the page arrives from the side of the later entries

  Scenario: A page opened from a later one arrives from the other side
    Given the browser supports Ed25519 in WebCrypto
    And the person opens the client at "#/settings"
    When the address fragment becomes "#/nodes"
    Then the mark of the current page is on entry 2 of the navigation
    And the page arrives from the side of the earlier entries

  Scenario: The mark of the chosen option moves with the choice
    Given the browser supports Ed25519 in WebCrypto
    And the person opens the client at "#/settings"
    When the person chooses the theme "深色"
    And the person chooses the language "English"
    Then the mark of the theme is on option 3
    And the mark of the language is on option 2

  Scenario: No part of an arriving page waits unseen
    Given the browser supports Ed25519 in WebCrypto
    And the person opens the client
    When the address fragment becomes "#/settings"
    Then one frame later part 2 of the page has not moved from where it arrives
    And both parts have faded in as far as each other
    And both parts are seen in full when no animation runs

  Scenario: The page cross-fades to a new theme in a browser with view transitions
    Given the browser supports Ed25519 in WebCrypto
    And the browser has view transitions
    And the person opens the client
    When the person presses "切换深色 / 浅色"
    Then the page uses the "auto" theme until the cross-fade has its first picture
    And the page uses the "dark" theme in the cross-fade

  Scenario: A press during the cross-fade reaches the page
    Given the browser supports Ed25519 in WebCrypto
    And the browser has view transitions
    And the person opens the client
    When the person presses "切换深色 / 浅色" and again while the page cross-fades
    Then the second press came while the picture of the old page was up
    And the page uses the "light" theme

  Scenario: A cross-fade under way ends when the system starts to ask for reduced motion
    Given the browser supports Ed25519 in WebCrypto
    And the browser has view transitions
    And the person opens the client
    When the person presses "切换深色 / 浅色" and the system asks for reduced motion while the page cross-fades
    Then the picture of the old page is gone
    And the page uses the "dark" theme

  Scenario: The theme changes at once when the system asks for reduced motion
    Given the browser supports Ed25519 in WebCrypto
    And the browser has view transitions
    And the system asks for reduced motion
    And the person opens the client
    When the person presses "切换深色 / 浅色"
    Then the page uses the "dark" theme
    And the page has not cross-faded

  Scenario: The client opens when the browser blocks site storage
    Given the browser supports Ed25519 in WebCrypto
    And the browser blocks site storage
    When the person opens the client
    Then the current page is "群聊"
    And the page uses the "auto" theme

  Scenario: A browser without Ed25519 gets the upgrade notice
    Given the browser lacks Ed25519 in WebCrypto
    When the person opens the client
    Then the upgrade notice "这个浏览器需要升级" is shown
    And the application is not shown

  Scenario: A browser that cannot keep its Ed25519 key gets the upgrade notice
    Given the browser makes Ed25519 keys but cannot clone them for storage
    When the person opens the client
    Then the upgrade notice "这个浏览器需要升级" is shown
    And the application is not shown

  Scenario: A browser without light-dark() colours gets the upgrade notice
    Given the browser supports Ed25519 in WebCrypto but not light-dark() in CSS
    When the person opens the client
    Then the upgrade notice "这个浏览器需要升级" is shown
    And the application is not shown

  Scenario: Every wire example of the node is read as a typed frame
    Given the wire examples in "internal/proto/testdata"
    When the client reads each one with the schema of its file name
    Then each one is read with every field as written
    And each one is read without a field added that the client does not know

  Scenario: Screens can sync a group with no node
    Given a mock connection, with no node
    When the client sends "hello"
    Then the answer lists the group "Website"
    When the client syncs that group
    Then the pushes "reset" and "rows" arrive before the answer "synced" at 815

  Scenario: Screens can page a thread and send a command with no node
    Given a mock connection, with no node
    When the client fetches the newest page of the main line
    Then the page holds the message "Choose a location."
    When the client sends a command
    Then the command is accepted

  Scenario: A browser that blocks site storage gets the storage notice
    Given the browser supports Ed25519 in WebCrypto but blocks site storage
    When the person opens the client
    Then the notice "站点存储被阻止" is shown
    And the application is not shown

  Scenario: The browser keeps its device key between visits
    Given the browser supports Ed25519 in WebCrypto
    And the person has opened the client once
    When the person opens the client again
    Then the browser is the same device as the first time

  Scenario Outline: A receiver answers a signed command with one code
    Given a command that this browser signed for the owner's node
    When <event>
    Then the answer is "<answer>"

    Examples:
      | event                                          | answer         |
      | it arrives unchanged                           | accepted       |
      | one byte of its payload is changed on the way  | bad_sig        |
      | the browser is revoked before it arrives       | unknown_device |
      | it reaches another device                      | not_for_me     |
      | it arrives 25 hours later                      | stale          |
      | it is revoked, misdirected and late            | unknown_device |
      | it is misdirected and late                     | not_for_me     |

  Scenario: A revoked device is out of the log for good
    Given the device log of a person with this browser, a tablet and a node
    When the node revokes the tablet
    Then the valid devices are this browser and the node
    And an entry signed by the tablet is refused
    And an entry that adds the tablet again is refused
    And the same revoke arriving a second time changes nothing

  Scenario: Two devices that share a pairing secret recognise each other
    Given this browser created a pairing secret
    When a node is given the secret as a person typed it, in lower case
    Then both sides meet in the same room
    And each side accepts the other's pair message
    And neither side accepts its own pair message sent back
