Feature: M7 terminal takeover
  An owner on a direct WebRTC path opens an interactive pseudo-terminal on a
  working AI session, holds incoming mentions until hand-back delivers them in
  order after the hand-back summary, logs non-overlapping CLI starts and exits
  across mode switches, refuses takeover on relayed paths, hands back promptly
  on orderly closes or after ICE detects a lost connection while answering
  replayed takeover.start commands with conflict afterwards, creates and
  cancels or keeps takeover-origin tasks according to worktree changes, and
  closes pty with hand-back when an open direct PeerConnection transitions to a
  TURN relay pair while keeping ctl alive.

  Scenario: Mentions during takeover are held and delivered in order after hand-back (A7.3)
    Given an M7 project group on a direct path with configured AI "Claude" on an active task
    When the M7 owner opens terminal takeover on "Claude" and attaches the pty channel
    And the M7 owner posts mentions "first held", "second held" and "third held" to "Claude" in that task thread
    Then "Claude" stays in state "takeover" with 3 queued items in host.info and no delivered reply
    When the M7 owner hands back "Claude"'s terminal and "Claude" posts handback "Terminal edits complete." and answers the held mentions
    Then "Claude" posts handback "Terminal edits complete." before posting "Ordered: first held, second held, third held" in that task thread

  Scenario: Fake CLIs log every start and exit with the session id across ten switches with no overlapping processes (A7.4)
    Given an M7 project group on a direct path with configured AIs "Claude" and "Codex" on active tasks
    When the M7 owner switches "Claude" ten times between structured turns and interactive PTY takeover
    And the M7 owner switches "Codex" ten times between structured turns and interactive PTY takeover
    Then the M7 fake CLI log records every "claude" start and exit on one session id with no start before the previous exit
    And the M7 fake CLI log records every "codex" start and exit on one session id with no start before the previous exit

  Scenario: On a relayed path takeover.start is refused with relay (A7.5)
    Given an M7 project group with configured AI "Claude" accessed over a relayed WebRTC path
    When the M7 owner sends takeover.start for "Claude" over that relayed ctl connection
    Then takeover.start is refused with error code "relay" and "Claude" starts 0 CLI processes

  Scenario: Orderly terminal closes hand back within 2 seconds, a lost connection hands back when ICE fails, and replayed takeover.start returns conflict (A7.6)
    Given an M7 project group on a direct path with configured AI "Claude" on an active task
    When the M7 owner starts takeover on "Claude", replays that takeover.start while open, and sends takeover.end
    Then the open replay succeeds, hand-back finishes within 2 seconds, and replaying that takeover.start afterwards fails with "conflict"
    When the M7 owner starts a second takeover on "Claude" and closes the pty data channel in order
    Then hand-back finishes within 2 seconds and replaying the second takeover.start fails with "conflict"
    When the M7 owner starts a third takeover on "Claude" and drops all packets on that browser connection
    Then the node hands back once its WebRTC stack reports the connection lost, and replaying the third takeover.start on a reconnected ctl fails with "conflict"

  Scenario: Takeover on a working AI with no task creates a task card, cancels it when unchanged, and keeps it with commit facts after a commit (A7.7)
    Given an M7 project group on a direct path with configured AI "Claude" and no tasks
    When the M7 owner opens terminal takeover on "Claude" with no task named
    Then an active "Owner takeover" task card exists for "Claude" and its pty channel opens in the task worktree
    When the M7 owner hands back "Claude"'s terminal with nothing changed in the worktree
    Then that takeover task is "cancelled" with no open task session and 0 handback messages
    When the M7 owner opens terminal takeover on "Claude" again, commits "fix.txt" with subject "Apply owner fix" in the task worktree, and hands back
    Then the second takeover task remains "active" and its handback message lists commit "Apply owner fix" and file "fix.txt"

  @not-windows
  Scenario: Moving an open takeover PeerConnection from direct to TURN relay closes pty, hands back, and keeps original ctl working (A7.9)
    Given an M7 project group with configured AI "Claude" on an active task over a direct path with TURN reachable
    When the M7 owner opens terminal takeover on "Claude" and verifies a non-relay selected pair in WebRTC statistics
    And direct UDP latency degrades while TURN stays reachable so the same PeerConnection selects a relayed pair
    Then WebRTC selected-pair statistics report a relay candidate, the pty channel closes, and the node hands back "Claude"
    And host.info still succeeds on the original M7 ctl connection without sending a new offer
