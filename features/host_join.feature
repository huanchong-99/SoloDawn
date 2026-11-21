Feature: Joining and leaving groups on a host

  Scenario: Joining groups on a remote home shares one connection and clone and reconnects on restart (A3.3 host half)
    Given a paired host on disk with declared agents "claude" and "codex" and a remote home "home-remote"
    And remote home "home-remote" hosts project groups "Website" and "Backend"
    When the owner joins "claude" as "Sec" with secretary true to "Website" on "home-remote"
    And the owner joins "codex" as "Codex" with secretary false to "Website" on "home-remote"
    And the owner joins "claude" as "Claude Backend" with secretary false to "Backend" on "home-remote"
    Then the host dialled "home-remote" 1 time and cloned "Website" and "Backend" once each
    And the host holds 3 dormant group sessions and 2 stored homes entries
    When the host restarts on its store and re-establishes home connections
    Then the restarted host dialled "home-remote" 1 time and holds 3 dormant group sessions

  Scenario: Refusing an undeclared agent and leaving groups preserves active homes until the last group leaves
    Given a paired host on disk with declared agents "claude" and "codex" and a remote home "home-remote"
    And remote home "home-remote" hosts project groups "Website" and "Backend"
    When the owner joins undeclared agent "codex" after clearing its account to "Website" on "home-remote"
    Then the join command fails with code "conflict" and the host dialled "home-remote" 0 times
    When the owner declares "codex" as "team" and joins both "claude" and "codex" to "Website" on "home-remote"
    And "home-remote" refuses removing "claude" with "conflict" because of an unfinished task
    And the owner asks "claude" to leave "Website"
    Then the leave command fails with code "conflict" and "Website" still has 2 sessions and an open home connection
    When "home-remote" accepts member removals and "claude" leaves "Website"
    Then "Website" has 1 session and remains in stored homes with its home connection open
    When "codex" leaves "Website"
    Then "Website" has 0 sessions, stored homes is empty, and the connection to "home-remote" is closed
