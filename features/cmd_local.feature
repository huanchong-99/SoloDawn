Feature: Local command socket
  Scenario: Status prints the fingerprint reported by the running node
    Given a local node socket that reports its device fingerprint
    When the owner asks solodawn for that node's status
    Then the command prints the node's fingerprint and succeeds
