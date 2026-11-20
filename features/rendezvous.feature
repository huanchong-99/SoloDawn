Feature: Rendezvous signalling
  Two authenticated devices exchange a signed offer through a local rendezvous.
  The answering device checks the offer before applying the remote description.

  Scenario Outline: A1.5 checks the description received through signalling
    Given two devices authenticated to a local rendezvous
    When the first offers a connection and the rendezvous <action> the SDP fingerprint in transit
    Then the second device's offer check returns "<result>"

    Examples:
      | action    | result   |
      | preserves | accepted |
      | changes   | bad_sig  |
