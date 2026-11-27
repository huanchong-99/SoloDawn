Feature: Room pairing, pair.browser, the host surface and the rooms of invitation codes
  A fresh browser and an unowned node pair through a rendezvous room, and an
  owner browser adds another browser with pair.browser under once-only receipts.
  A home node hosts the room of each open invitation code on its rendezvous:
  people who type the code meet the home there and join its group.

  Scenario: Onboarding room pairing, fingerprint, bootstrap rule and adoption (A1.1)
    Given an in-process rendezvous and an unowned pairing node
    When a fresh browser creates a pairing secret and joins its room
    And the pairing node runs pair with that secret and label "workstation"
    Then the node fingerprint seen in the room equals the node key fingerprint
    When the pairing browser opens ctl and sends hello before any device log
    Then the pairing welcome reports owner false
    And host.info before enrolment is refused with "forbidden"
    When the pairing browser sends a log whose add entry for the node was signed by another device
    Then the pairing node still has no owner and refuses host.info with "forbidden"
    When the pairing browser sends a valid device log adding the node signed by the pairing browser
    Then pair completes with the node fingerprint and active path
    When the pairing browser sends hello again
    Then the pairing welcome reports owner true
    And host.info succeeds for the owner browser

  Scenario: pair.browser returns the same link on replay until used or expired (A1.9)
    Given an owned node with a connected owner browser over the rendezvous
    When the owner browser sends pair.browser with nonce "first"
    And the owner browser repeats pair.browser with nonce "first" while the link is valid
    Then both answers return ok with the same #pair link and expiry
    When a second browser pairs using that #pair link
    Then the second browser receives the device log, adopts the owner and reads owner true
    When the owner browser repeats pair.browser with nonce "first" after the link was used
    Then the pair.browser repeat is refused with "conflict"
    When the owner browser sends pair.browser with nonce "second"
    And 10 minutes pass so the second link expires
    And the owner browser repeats pair.browser with nonce "second"
    Then the pair.browser repeat is refused with "conflict"

  Scenario Outline: People who type one invitation code meet the home in its room and join, <how> (A9.47)
    Given a code home with a group, its owner's browser and an invitation code for 24 hours
    When "Q" and "R" type the code and join <how>
    Then the room showed each of them the home node, and its welcome named the group
    And each of them has a member row with role "member" and their device log on the code home, let in by the owner

    Examples:
      | how                 |
      | one after the other |
      | at once             |

  Scenario Outline: When its owner <ends>, the code that was open answers nobody and the connection that waited on it is closed (A9.48)
    Given a code home with a group, its owner's browser and an invitation code for 24 hours
    And "Q" has joined by the code and "R" has typed it without joining
    When the code home's owner <ends>
    Then a device that types the ended code gets no answer
    And the connection of "R" to the code home is closed
    And "Q" is still a member and still connected

    Examples:
      | ends               |
      | turns the code off |
      | makes a new code   |

  Scenario: At the end of its time a code refuses a join, and then answers nobody and closes the connection that waited (A9.48)
    Given a code home with a group, its owner's browser and an invitation code for 24 hours
    And "Q" has joined by the code and "R" has typed it without joining
    When the code home's clock reaches the end of the code
    Then a join on the connection of "R" is refused with "forbidden"
    When the code home's timers run out
    Then a device that types the ended code gets no answer
    And the connection of "R" to the code home is closed
    And "Q" is still a member and still connected

  Scenario: A code answers after its home restarted, after the rendezvous connection was made again, and once a rendezvous connects (A9.49)
    Given a code home with a group, its owner's browser and an invitation code for 24 hours
    When the code home restarts
    Then "Q" types the code and joins
    When the code home's rendezvous connection is cut and made again
    Then "R" types the code and joins
    When the code home's rendezvous connection is cut and its owner makes a new code
    Then a device that types the code gets no answer
    When the code home connects to the rendezvous again
    Then "S" types the code and joins

  Scenario: After a move the code has ended and its room is left (A9.49)
    Given a code home with a group, its owner's browser and an invitation code for 24 hours
    When the group of the code home moves to another computer of its owner
    Then a device that types the ended code gets no answer

  Scenario: Removing a person ends the code, and a person who leaves does not (A9.50)
    Given a code home with a group, its owner's browser and an invitation code for 24 hours
    And "Q" and "R" have joined by the code
    When "R" leaves the group of the code home
    Then a device that types the code is shown the home node
    When the code home's owner removes "Q" from the group
    Then a device that types the ended code gets no answer

  Scenario: A person who typed the code while its home was off joins when it starts, and a wrong code is never answered (A9.51)
    Given a code home with a group, its owner's browser and an invitation code for 24 hours
    When the code home stops
    And "Q" types the code and waits, and nothing answers a wrong code
    And the code home starts again
    Then "Q" is shown the home node and joins without typing again
    And nothing answers a wrong code

  Scenario: A rendezvous that never says hosts is sent no hosted room and no leave, and pairing and invitations go on there (A9.55)
    Given a code home on a rendezvous that never says hosts, with a group, its owner's browser and an invitation code for 24 hours
    Then a device that types the code gets no answer
    When the code home's owner sends pair.browser and invite
    And the code home's owner turns the code off
    Then a browser that opens each of those links meets the home node in its room and connects to it
    And that rendezvous was sent no join with data and no leave, on the one connection the code home made

  Scenario: Where another device hosts the room, the home is answered full, is found on its other rendezvous, and hosts there once its connection is made again (A9.55)
    Given a code home with a group, its owner's browser and an invitation code for 24 hours
    And a holder of the code hosts its room on a second rendezvous
    When the code home connects to the second rendezvous
    Then the code home is answered full there and its connection stays
    And a device that types the code is shown the home node
    And a device that types the code on the second rendezvous is shown the holder
    When the holder leaves and the code home's connection to the second rendezvous is made again
    Then "Q" types the code on the second rendezvous and joins

  Scenario: A code made and a code ended after the home took fresh ICE servers is hosted and left (A9.55)
    Given a code home whose rendezvous gives ICE servers for one second, with a group and its owner's browser
    When the code home has asked for fresh ICE servers and taken them
    And the code home's owner makes an invitation code for 24 hours
    Then the code home's rendezvous was sent the hosting join of the code's room
    And a device that types the code is shown the home node
    When the code home's owner turns the code off
    Then the code home's rendezvous was sent the leave of the ended code's room
    And a device that types the ended code gets no answer
