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
    And the person presses "Switch language / 切换语言"
    Then the page uses the "dark" theme
    And the page is marked as "en"

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
    And each one is refused with one unknown field added

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
