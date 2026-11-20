Feature: Command line
  The solodawn command only parses its arguments and hands the work to the node.
  Arguments that are not a known command get the usage line and a failing exit code.

  Scenario: An incomplete TLS key pair is rejected before listening
    When I run "solodawn rendezvous --domain localhost --no-relay --cert cert.pem"
    Then it prints "--cert and --key must be supplied together" on stderr and exits with code 1

  Scenario Outline: Arguments that are not a known command print the usage
    When I run "solodawn <arguments>"
    Then it prints "usage: solodawn version | rendezvous [--listen address]" on stderr and exits with code 2

    Examples:
      | arguments     |
      |               |
      | unknown       |
      | version extra |
