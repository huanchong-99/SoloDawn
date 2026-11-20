Feature: Command line
  The solodawn command only parses its arguments and hands the work to the node.
  Arguments that are not a known command get the usage line and a failing exit code.

  Scenario Outline: Arguments that are not a known command print the usage
    When I run "solodawn <arguments>"
    Then it prints "usage: solodawn version | rendezvous [--listen address]" on stderr and exits with code 2

    Examples:
      | arguments     |
      |               |
      | unknown       |
      | version extra |
