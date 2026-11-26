Feature: Joining and leaving groups on a host

  Scenario: Joining groups on a remote home shares one connection, records each AI's workspace and reconnects on restart (A3.3 host half)
    Given a paired host on disk with declared agents "claude" and "codex" and a remote home "home-remote"
    And remote home "home-remote" hosts project groups "Website" and "Backend"
    When the owner joins "claude" as "Sec" with secretary true to "Website" on "home-remote"
    And the owner joins "codex" as "Codex" with secretary false to "Website" on "home-remote"
    And the owner joins "claude" as "Claude Backend" with secretary false to "Backend" on "home-remote"
    Then the host dialled "home-remote" 1 time and the home added each AI with the folder it was joined with
    And each group session runs in the folder its AI was joined with, and the node's data directory holds no project folder
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

  Scenario: A join names a folder that is there now, by its absolute path (A2.8 host)
    Given a paired host on disk with declared agents "claude" and "codex" and a remote home "home-remote"
    And remote home "home-remote" hosts project groups "Website" and "Backend"
    When the owner joins "claude" as "Claude" to "Website" on "home-remote" in a folder that does not exist
    Then the join command fails with code "not_found" and the home added no AI
    When the owner joins "claude" as "Claude" to "Website" on "home-remote" in the relative folder "work/website"
    Then the join command fails with code "bad_request" and the home added no AI

  Scenario: A folder of an AI of another group is refused with that group's name, whichever home it has (A2.9 host)
    Given a paired host on disk with declared agents "claude" and "codex" and a remote home "home-remote"
    And remote home "home-remote" hosts project groups "Website" and "Backend"
    And remote home "home-other" hosts project group "Docs"
    When the owner joins "claude" as "Claude" to "Website" on "home-remote" in folder "shared"
    And the owner joins "codex" as "Codex Docs" to "Docs" on "home-other" in folder "shared"
    Then the join command fails with code "conflict" naming the group "Website" and the home added no AI to "Docs"
    When the owner joins "codex" as "Codex" to "Website" on "home-remote" in folder "shared"
    Then the join succeeded and "Website" has 2 sessions in folder "shared"
    When the owner joins "claude" to "Backend" on "home-remote" and to "Docs" on "home-other" at once, both in folder "solo"
    Then exactly one of the two joins succeeded and the other failed with code "conflict"
    When "claude" and "codex" leave "Website"
    And the owner joins "codex" as "Codex Docs" to "Docs" on "home-other" in folder "shared"
    Then the join succeeded and "Docs" has a session in folder "shared"
