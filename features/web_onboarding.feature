@web
Feature: Onboarding and pairing in the web client
  A fresh browser pairs with a node through a one-time 16-character pairing secret and a 10-minute
  room exchange, shows the node's label and 12-digit fingerprint, enrols the newcomer through the
  signed device log and a second hello, asks the browser to keep its storage, and declares the
  account type of each AI CLI found on the computer.

  Background:
    Given a browser set to English

  Scenario: A fresh browser pairs a fresh node, enrols it, asks to keep storage and sets CLI accounts
    Given a fresh browser and a fresh node on a rendezvous
    When the person opens the client at "#/onboarding"
    Then the prompt gives "install.md", "--rendezvous" and a 16-character code in four groups
    And the countdown starts at "10 min 0 s"
    When the person copies the prompt and gives the code to the node
    Then the clipboard holds the prompt
    And the page shows the node "Alex's MacBook" with its 12-digit fingerprint
    When the connection opens and the node welcomes the browser before and after enrolment
    Then the browser sent a two-entry device log without a request id and asked to keep storage
    And the page lists "Claude Code" and "Codex" for account declaration
    When the person declares "Personal subscription" for "Claude Code" and "API key" for "Codex"
    Then the node received signed "agent.set" commands for "claude" and "codex"
    And "Next: set your AIs' defaults" leads to "#/nodes"

  Scenario: The pairing secret expires after 10 minutes and can be replaced
    Given a fresh browser and a fresh node on a rendezvous
    When the person opens the client at "#/onboarding"
    And 9 minutes and 40 seconds pass
    Then the countdown reads "20 s"
    When 20 more seconds pass
    Then the pairing code is discarded and the page says it expired
    When the person presses "New pairing code"
    Then the prompt gives a new 16-character code and a fresh "10 min 0 s" countdown

  Scenario: A new browser opened from a pairing link adopts the node's device log
    Given a node that already belongs to the owner and created a browser pairing secret
    When a fresh browser opens the pairing link
    Then the address becomes "#/onboarding" and the browser joins the room in role "b"
    When the node pairs, welcomes the browser and pushes the owner's device log
    Then the browser adopts the log, sends a second "hello" and asks to keep storage

  Scenario: A malformed pairing link shows its error
    Given a fresh browser and a fresh node on a rendezvous
    When the person opens a pairing link with secret "not-a-secret"
    Then the page shows that the pairing code in the link is not valid
