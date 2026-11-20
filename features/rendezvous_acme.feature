Feature: Rendezvous certificates by ACME
  A rendezvous started with one command gets the certificates for its two names from an ACME
  server, which checks them on the shared TLS port (TLS-ALPN-01), and renews each one before it
  expires while it keeps serving. Pebble, a local ACME test server, stands in for Let's Encrypt;
  its DNS answers 127.0.0.1 for every name.

  Scenario: A8.4 One command gets certificates for both names and renews them
    Given an ACME test server whose certificates last 30 seconds
    When the command "solodawn rendezvous" starts for "rv.localhost" with that ACME server
    Then "rv.localhost" and "turn.rv.localhost" present certificates that the ACME test server issued
    And HTTPS on the shared port answers under the certificate of "rv.localhost"
    And the browser signs the challenge and relays data through TURN over TLS
    When two thirds of the certificates' lifetime have passed
    Then both names present new certificates from the ACME test server before the first ones expire
    And the browser signs the challenge and relays data through TURN over TLS
