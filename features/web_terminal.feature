@web
Feature: Terminal takeover page and handback summary
  The owner opens the interactive terminal of one of their own AIs over a direct WebRTC connection:
  the browser sends takeover.start, opens the binary channel pty:<g>:<member>, pushes pty.resize,
  streams bytes in both directions, says that messages to the AI wait until hand-back, has
  narrow-screen keys, refuses or closes cleanly on a relayed path, and sends takeover.end on
  hand-back or tab close, after the answer to takeover.start when that has not come yet
  (planning/flows/takeover.md, planning/contracts/datachannel.md sections 1 and 7,
  planning/contracts/commands.md section 2; acceptance tests A7.1, A7.5, A7.6, A7.7, A7.9).

  Background:
    Given a group "Website" with an owned AI "Claude Code" on "workstation", in English

  Scenario: Opening an owned AI terminal with a task streams terminal bytes, resizes, says messages wait and renders handback
    Given "Claude Code" is working on task 12 "Fix path events" on branch "task/12-conn-events" with a direct path of 4 ms
    When the owner clicks "Open terminal" on the "Claude Code" member card
    Then "workstation" receives a signed "takeover.start" for "Claude Code" on task 12
    And the browser opens channel "pty:rZNvy-1jH6Z-BcPqA5U5BQ:1PC8WineBrUQ-apCjx7tug" and pushes "pty.resize"
    And the terminal header reads "Claude Code · Terminal" and "workstation · T-12 · task/12-conn-events"
    And the takeover bar reads "Owner is driving" and the session card shows "T-12 Fix path events", "task/12-conn-events" and "Direct · 4 ms"
    When the host streams prompt bytes and the owner presses "Ctrl" then types "c" and presses "Enter"
    Then the takeover bar and the queue card say that messages wait until hand-back, and the channel receives Ctrl-C and Enter
    When the owner clicks "Hand back" and the home writes a handback message in the main line
    Then the channel closes, "workstation" receives a signed "takeover.end", and the chat renders the handback card with "path.go", "9f2c1ab", "Emit path changes" and "+40 −3"

  Scenario: Opening an AI without a task explains new-task creation, route task ids are forwarded, and the secretary is read-only
    Given "Claude Code" has no task and "Secretary" is the group's secretary on a direct path
    When the owner opens "#/terminal/rZNvy-1jH6Z-BcPqA5U5BQ/1PC8WineBrUQ-apCjx7tug"
    Then the session card says "Opening the terminal of an AI without a task creates one so changes stay on a task branch."
    When the host creates task 15 "Owner takeover" on branch "task/15-owner-takeover" for "Claude Code"
    Then the terminal header updates to "workstation · T-15 · task/15-owner-takeover"
    When the owner navigates to "#/terminal/rZNvy-1jH6Z-BcPqA5U5BQ/1PC8WineBrUQ-apCjx7tug/O21RWBqThVezWCWeVoSoSQ"
    Then "workstation" receives "takeover.end" for the previous session and "takeover.start" naming task "O21RWBqThVezWCWeVoSoSQ"
    When the owner navigates to "#/terminal/rZNvy-1jH6Z-BcPqA5U5BQ/qBSFMsr2hHYKOMblEA_kdA"
    Then the session card shows "Read-only group session" and "The secretary's session is read-only and changes no files."

  Scenario: A task card opens the terminal on its own task
    Given "Claude Code" is working on task 12 "Fix path events" on branch "task/12-conn-events" with a direct path of 4 ms
    And "Claude Code" is also the assignee of task 15, whose card is in the main line
    When the owner opens the chat of "Website" over a relayed path
    Then the menu of T-15 says "Open Claude Code's terminal" and "Direct link only" without a link
    When the path is direct again and the owner follows "Open Claude Code's terminal" of T-15
    Then "workstation" receives a signed "takeover.start" for "Claude Code" on task 15

  Scenario: Relayed paths and takeover refusals block terminal takeover clearly
    Given the path to "workstation" is relayed
    When the owner opens the chat of "Website"
    Then the "Open terminal" button on "Claude Code" is disabled with reason "Direct link only"
    When the owner opens "#/terminal/rZNvy-1jH6Z-BcPqA5U5BQ/1PC8WineBrUQ-apCjx7tug" directly
    Then no "takeover.start" is sent and the takeover bar says "Direct connection required"
    When the path becomes direct and "workstation" refuses "takeover.start" with "relay", then "busy", then "internal"
    Then the takeover bar shows "Direct connection required", then "This AI is busy: its terminal is already open, one of its tasks is merging, or it is about to start a turn (perhaps waiting for a free slot on its computer). Try again soon.", then error code "internal"

  Scenario: A relayed path that recovers to direct opens the terminal without reopening the page
    Given the path to "workstation" is relayed
    When the owner opens "#/terminal/rZNvy-1jH6Z-BcPqA5U5BQ/1PC8WineBrUQ-apCjx7tug" and the path becomes direct
    Then one "takeover.start" is sent, the pty channel opens and the takeover bar no longer says "Direct connection required"

  Scenario: Handing back while the terminal is still opening ends the takeover once the start is answered
    Given "Claude Code" is working on task 12 "Fix path events" on branch "task/12-conn-events" with a direct path of 4 ms
    And "workstation" holds its answer to "takeover.start"
    When the owner opens "#/terminal/rZNvy-1jH6Z-BcPqA5U5BQ/1PC8WineBrUQ-apCjx7tug" and clicks "Hand back" while the bar says "Opening terminal…"
    Then no "takeover.end" is sent yet and the chat of "Website" is open
    When "workstation" answers "takeover.start" with ok
    Then "workstation" receives one signed "takeover.end" after it, and no pty channel is opened

  Scenario: The terminal is named and can be left from the keyboard
    Given "Claude Code" is working on task 12 "Fix path events" on branch "task/12-conn-events" with a direct path of 4 ms
    When the owner opens "#/terminal/rZNvy-1jH6Z-BcPqA5U5BQ/1PC8WineBrUQ-apCjx7tug" on a 1280 px desktop
    Then the takeover bar is a status region, the key row is no tab stop of its own, the details column takes keyboard focus, and the terminal's input is described by the note on leaving it
    And only Ctrl+M is kept from the terminal, and it moves focus to "Hand back"

  Scenario: The card of an AI in takeover offers to hand it back
    Given "Claude Code" is in takeover on a direct path
    When the owner opens the chat of "Website"
    Then the "Claude Code" card reads "Owner is driving" and offers "Hand back" with "Messages to it are queued until you hand back"
    When the owner clicks "Hand back" on the card
    Then "workstation" receives a signed "takeover.end" for "Claude Code"

  Scenario: Tab close via pagehide and remote channel close end takeover cleanly
    Given "Claude Code" is working on task 12 "Fix path events" on branch "task/12-conn-events" with a direct path of 4 ms
    When the owner opens "#/terminal/rZNvy-1jH6Z-BcPqA5U5BQ/1PC8WineBrUQ-apCjx7tug" and closes the tab
    Then the channel closes and "workstation" receives exactly one signed "takeover.end"
    When the owner reopens "#/terminal/rZNvy-1jH6Z-BcPqA5U5BQ/1PC8WineBrUQ-apCjx7tug" and the host closes the pty channel
    Then the takeover bar updates to "Handed back" without sending a duplicate "takeover.end"

  Scenario: An active terminal session closes pty and keeps ctl open when the path switches to relay
    Given "Claude Code" is working on task 12 "Fix path events" on branch "task/12-conn-events" with a direct path of 4 ms
    When the owner opens "#/terminal/rZNvy-1jH6Z-BcPqA5U5BQ/1PC8WineBrUQ-apCjx7tug" and the path to "workstation" switches to relay
    Then the pty channel is closed, the takeover bar says "Handed back · switched to relay", and "host.info" still succeeds on the existing ctl connection

  Scenario: The development states sheet mounts the handback card for the wire example
    Given a development server
    When the owner opens "#/states"
    Then the "handback" section is marked "Mounted by terminal" and renders "path.go", "9f2c1ab", "Emit path changes" and "+40 −3"
