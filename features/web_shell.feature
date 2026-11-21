@web
Feature: Shell of the web client
  The shell is the frame of every page and the seams through which each area of the interface
  adds its parts without editing the shell or another area: the route table with the seven pages
  of the design and their parameters, the connection the shell chooses and every page is given,
  the mounting points in the top bar, under it, above the page, in the chat and for the card of
  each kind of message, and one loader for the texts of every area.

  Background:
    Given a browser set to English

  Scenario Outline: Each page of the design opens at its address
    When the person opens the client at "<address>"
    Then the page "<title>" is open, drawn by "<drawn by>"
    And the navigation marks "<entry>"

    Examples:
      | address                         | title                    | drawn by         | entry     |
      | #/onboarding                    | Connect a computer       | the onboarding page | nothing   |
      | #/groups                        | Chats                    | the group list   | Chats     |
      | #/groups/rZNvy-1jH6Z-BcPqA5U5BQ | Chats                    | the group list   | Chats     |
      | #/terminal/g/m                  | Terminal takeover        | a coming page    | Chats     |
      | #/nodes                         | My computers and AIs     | its own page     | Computers |
      | #/settings                      | Settings                 | its own page     | Settings  |
      | #/states                        | Design system and states | the states sheet | nothing   |

  Scenario: A page that is not built yet says so and leads back to the chats
    When the person opens the client at "#/terminal/g/m"
    Then the page says "This page is not built yet"
    And its link "Back to the chats" leads to "#/groups"

  Scenario: The address gives the page its parameters
    Given the home of the group "Website" is reached
    When the person opens the client at "#/groups/rZNvy-1jH6Z-BcPqA5U5BQ"
    Then the chat of "Website" is open
    When the address becomes "#/groups"
    Then no chat is open

  Scenario: The address of a group that has not been read keeps the chat's place, with a way back
    When the person opens the client at "#/groups/not-read-yet"
    Then the page says "This group has not been read yet. It is read once the computer that is its home can be reached."
    And its link "Back to the group list" leads to "#/groups"
    When the home of the group "not-read-yet" is reached
    Then the chat of "not-read-yet" is open

  Scenario Outline: A page has the screen to itself where its route asks for it
    When the person opens the client at "<address>"
    Then the shell gives the page the whole screen "<where>"

    Examples:
      | address        | where          |
      | #/groups/g     | on a phone     |
      | #/terminal/g/m | on a phone     |
      | #/onboarding   | at every width |
      | #/groups       | nowhere        |
      | #/settings     | nowhere        |

  Scenario: In a development server every page is given the group of the wire examples
    Given a development server
    When the person opens the client at "#/groups"
    Then the group list shows "Website" from the computer "workstation"

  Scenario Outline: Elsewhere no page is given a home until the connections to the nodes are wired
    Given <where>
    When the person opens the client at "#/groups"
    Then the group list is empty

    Examples:
      | where                                              |
      | a development server, at an address with "?nomock" |
      | a build                                            |

  Scenario: What areas mount in the top bar comes first, in order, at every page
    Given an area that mounts "Pause all" at 20 and another that mounts the indicator at 10
    When the person opens the client at "#/settings"
    Then the top bar reads the indicator "live", "Pause all", then the theme and language buttons

  Scenario: An area's banner shows under the top bar and its layer after the page
    Given an area that mounts a notice as a banner and as a layer
    When the person opens the client at "#/nodes"
    Then a notice stands between the top bar and the page, and another after the page

  Scenario: A card mounted for its kind replaces the one-line stub
    Given an area that mounts the card of approvals and of the home's notices
    And the home of the group "Website" holds a pending approval, a pending brief and a notice of the home
    When the person opens the client at "#/groups/rZNvy-1jH6Z-BcPqA5U5BQ"
    Then the approval is drawn by its card under the head and text of its message
    And the brief is still the one-line stub
    And the notice of the home is drawn whole by its card, with no line of its own

  Scenario: A chat holds what areas mount in its head, above its main line and beside it
    Given an area that mounts a part in the head, above the main line and beside it
    And the home of the group "Website" is reached
    When the person opens the client at "#/groups/rZNvy-1jH6Z-BcPqA5U5BQ"
    Then the head, the place above the main line and the side panel each hold the part for "Website"
    And the head has a button "People and details" that opens the side panel

  Scenario: Below 1024 px the side panel opens over the chat, holds the focus, and closes
    Given an area that mounts a part in the head, above the main line and beside it
    And the home of the group "Website" is reached
    And the person opens the client at "#/groups/rZNvy-1jH6Z-BcPqA5U5BQ" on a screen 820 px wide
    When the person presses "People and details"
    Then the side panel is open, with focus on its button "Close the panel"
    And the head, the main line and the composer behind it cannot be reached
    When the person presses Escape
    Then the side panel is closed and focus is on "People and details"
    And the chat can be reached again
    When the person presses "People and details" and then "Close the panel"
    Then the side panel is closed again, with focus back on "People and details"

  Scenario: An Escape that a dialog takes, or that a part already used, leaves the side panel open
    Given an area that mounts a part in the head, above the main line and beside it
    And the home of the group "Website" is reached
    And the person opens the client at "#/groups/rZNvy-1jH6Z-BcPqA5U5BQ" on a screen 820 px wide
    And the person has opened the side panel
    When the person presses Escape in a dialog
    Then the side panel is open
    When the person presses Escape where a part has already used it
    Then the side panel is still open

  Scenario: From 1024 px the side panel stands beside the chat and takes no focus
    Given an area that mounts a part in the head, above the main line and beside it
    And the home of the group "Website" is reached
    And the person opens the client at "#/groups/rZNvy-1jH6Z-BcPqA5U5BQ" on a screen 1440 px wide
    When the person presses "People and details"
    Then focus is still on "People and details" and the chat can be reached
    When the person presses Escape
    Then the side panel is still marked open

  Scenario: Two areas cannot mount the card of one kind
    When two areas each mount a card for approvals
    Then gathering the mounts fails, naming both areas

  Scenario Outline: The texts of an area cannot replace another text
    When an area adds the file "<file>" with the key "<key>"
    Then loading the texts fails: "<problem>"

    Examples:
      | file             | key        | problem                                     |
      | page.en.json     | page.x     | the shell has the namespace page            |
      | approval.en.json | chat.send  | chat.send is not a key of the area approval |
      | approval.fr.json | approval.x | no language fr                              |

  Scenario: Each web slice runs its own feature file
    Then every web feature file is tagged "@web"
    And every web feature file but the end-to-end one is run by the spec file of its slice
