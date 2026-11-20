Feature: Identity
  Every device has one key and everything it sends is signed with it. A receiver
  refuses a signed envelope with one error code, a person's devices follow from
  their device log, and two devices that have never met prove to each other that
  they hold the same pairing secret.

  Scenario: A node keeps its device key between starts
    Given a node that started for the first time in an empty data directory
    When the node starts again
    Then it has the same device id and fingerprint as before
    And its key file can be read by the user only

  Scenario Outline: A receiver answers a signed command with one code
    Given a command that the owner's browser signed for the owner's node
    When <what happens>
    Then the answer is "<answer>"

    Examples:
      | what happens                                    | answer         |
      | it arrives unchanged                            | accepted       |
      | one byte of its payload is changed on the way   | bad_sig        |
      | the browser is revoked before it arrives        | unknown_device |
      | it reaches another device                       | not_for_me     |
      | it arrives 25 hours later                       | stale          |
      | it is changed, revoked, misdirected and late    | bad_sig        |
      | it is revoked, misdirected and late             | unknown_device |
      | it is misdirected and late                      | not_for_me     |

  Scenario: Signalling goes stale sooner than a command
    Given a payload that the owner's browser signed 3 minutes ago
    Then read as signalling the answer is "stale"
    And read as a command the answer is "accepted"

  Scenario: A revoked device is out of the log for good
    Given the device log of a person with a browser, a tablet and a node
    When the node revokes the tablet
    Then the valid devices are the browser and the node
    And an entry signed by the tablet is refused
    And an entry that adds the tablet again is refused
    And the same revoke arriving a second time changes nothing
    And replaying the log from its first entry gives the same valid devices

  Scenario: Two devices that share a pairing secret recognise each other
    Given a browser that created a pairing secret
    When a node is given the secret as a person typed it, in lower case
    Then both sides meet in the same room
    And each side accepts the other's pair message
    And neither side accepts its own pair message sent back
    And a pair message made with another secret is refused

  Scenario: A new device adopts only the person its pairing peer enrolled it in
    Given a node that belongs to nobody and has paired with a browser
    Then it adopts the person of a log in which that browser added it
    But it does not adopt the person of a log in which another device added it
