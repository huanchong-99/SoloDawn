Feature: Command line
  The solodawn command only parses its arguments and hands the work to the node.
  Arguments that are not a known command get the usage line and a failing exit code.

  Scenario: An incomplete TLS key pair is rejected before listening
    When I run "solodawn rendezvous --domain localhost --no-relay --cert cert.pem"
    Then it prints "--cert and --key must be supplied together" on stderr and exits with code 1

  Scenario Outline: Arguments that are not a known command print the usage
    When I run "solodawn <arguments>"
    Then it prints "usage: solodawn <command> [arguments]" on stderr and exits with code 2

    Examples:
      | arguments         |
      |                   |
      | unknown           |
      | --unknown         |
      | version extra     |
      | help extra        |
      | --help extra      |
      | -h extra          |
      | node extra        |
      | node --help extra |
      | node -h extra     |
      | node --unknown    |

  Scenario Outline: Top-level help explains how to start the node and where the local page is
    When I run "solodawn <arguments>"
    Then it prints on stdout with exit code 0 including:
      | usage: solodawn <command> [arguments] |
      | solodawn node                         |
      | http://localhost:47800                |

    Examples:
      | arguments |
      | --help    |
      | -h        |
      | help      |

  Scenario Outline: Node help explains how to start the node and where the local page is
    When I run "solodawn <arguments>"
    Then it prints on stdout with exit code 0 including:
      | usage: solodawn node   |
      | http://localhost:47800 |
      | solodawn pair          |

    Examples:
      | arguments   |
      | node --help |
      | node -h     |
