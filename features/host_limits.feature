Feature: Host turn limits, vendor rate limits and offline mode

  Scenario: All my AIs offline closes sessions, holds items across a stopped home, and online delivers them
    Given a host with an active task session and a waiting group session connected to its home
    When the owner sends host.offline
    Then every session on the host is dormant and the home received turn_interrupted "stop" and state "offline"
    When the home pushes a mention item while the host is offline
    Then the item remains unacknowledged on the home and starts no turn
    When the home stops, the owner sends host.online and host.offline, the home reconnects, and the owner sends host.online
    Then host.offline succeeded while the home was stopped and host.online delivered and acknowledged the held item

  Scenario: With turn_minutes 1 a long turn is interrupted with reason timeout
    Given a host task session in a group whose turn_minutes is 1
    When the task session starts a turn that does not finish within 1 minute
    Then the CLI receives an interrupt and the home receives turn_interrupted "timeout" and task fail "timeout"

  Scenario: Vendor rate limits hold deliveries until the reset timestamp
    Given a host AI with a held follow-up mention
    When a turn reports vendor rate_limited until 30 seconds later
    Then member status reports rate_limited and state "limited" and holds the mention until the reset time passes
