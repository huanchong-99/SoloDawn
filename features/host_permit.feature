Feature: macOS folder permission checks in host.join and session start

  Scenario: host.join refuses a folder when macOS refuses or times out after 2 seconds (A11.5)
    Given a paired host for permit checks with declared agent "claude" and remote group "Website" on "home-remote"
    When macOS refuses access to folder "website"
    And the owner tries to join "claude" as "Claude" to "Website" on "home-remote" in folder "website"
    Then the permit join fails with code "permission" naming folder "website" and adds no member
    When macOS does not answer within 2 seconds for folder "website"
    And the owner tries to join "claude" as "Claude" to "Website" on "home-remote" in folder "website"
    Then the permit join fails with code "permission" saying macOS is waiting and adds no member
    When macOS allows access to folder "website"
    And the owner tries to join "claude" as "Claude" to "Website" on "home-remote" in folder "website"
    Then the permit join succeeds with 1 session in folder "website"

  Scenario: A session whose folder macOS refuses starts no CLI and reports the folder in detail (A11.6)
    Given a task session in folder "website" on a host whose folder access macOS refuses
    When that task session receives a turn
    Then no CLI started and the host reports turn_interrupted "crash" naming the folder in detail and task fail "crash"
    When macOS allows access to that task folder and the session receives a turn
    Then the CLI started once in that folder
