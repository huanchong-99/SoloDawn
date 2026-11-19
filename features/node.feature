Feature: Node
  The node assembles the SoloDawn runtime. So far it only carries the build version;
  a build without a release version reports "dev".

  Scenario: A development build reports its version
    When I run "solodawn version"
    Then it prints "dev" on stdout and exits with code 0
