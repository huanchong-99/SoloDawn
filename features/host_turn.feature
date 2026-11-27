Feature: A host composes session instructions and turn text
  Scenario Outline: Every session role gets its own embedded instructions under 60 lines
    Given a host turn fixture
    When I read the instructions for a "<role>" session
    Then the instructions are under 60 lines and include "<expected>"

    Examples:
      | role      | expected                                                                                                       |
      | group     | start_task                                                                                                     |
      | group     | `start_task` may answer that the task is queued behind another one in this folder                              |
      | task      | submit_task                                                                                                    |
      | task      | Work only in this folder; it is your workspace in this group.                                                  |
      | task      | with Git, `git pull --ff-only`                                                                                 |
      | task      | when the current branch has an upstream (without one there is nothing to bring in)                             |
      | task      | never stash, reset, clean, `git add -A`, `git add .`, `git commit -a`, check out over files, rebase            |
      | task      | stage and commit only your own files, by path. Never rewrite history that was published.                       |
      | task      | never push to a branch that others use (such as `main`) before the review has passed                           |
      | task      | SoloDawn runs no checks for you.                                                                               |
      | task      | Call `submit_task` last                                                                                        |
      | task      | After it, end your turn.                                                                                       |
      | review    | Change nothing in the assignee's folder                                                                        |
      | review    | do not score what you have not read                                                                            |
      | secretary | propose_brief                                                                                                  |
      | secretary | One folder takes one task at a time: order parts that must come together with `depends`.                       |
      | secretary | Nothing is merged by the platform                                                                              |
      | group     | Never end a turn with a promise. Anything you say you will do, do it in this turn before you end it            |
      | group     | `start_task` for a change to files                                                                             |
      | task      | Never end a turn with a promise. Anything you say you will do, do it in this turn before you end it            |
      | task      | A report says what is done and what was started; it announces no work that nobody will run.                    |
      | review    | Never end a turn with a promise. Anything you say you will do, do it in this turn before you end it            |
      | review    | Say which you did.                                                                                             |
      | secretary | Never end a turn with a promise. Anything you say you will do, do it in this turn before you end it            |
      | secretary | `set_task` or `propose_brief`, `ask` or a `post` that mentions the AI or person who is to act                  |
      | secretary | Then, in the same turn, request the closing step the project needs                                             |
      | secretary | You coordinate; you do not do the work.                                                                        |
      | secretary | Say what you decide, in the turn you decide it.                                                                |
      | secretary | What holds only for now                                                                                        |
      | secretary | look at the tasks you paused (`list_tasks`)                                                                    |
      | secretary | until a person deletes them                                                                                    |

  Scenario: The first turn of a brief task session carries goal, criteria, scope and newest notes
    Given a host turn fixture
    When the host composes the first assign turn of task 12
    Then the composed turn text is:
      """
      [SoloDawn] group "Website" · you are "Claude Code" · task session T-12
      == 1 of 1 · assign · instruction
      Task T-12 "Node: report connection path changes"
      Folder: /home/dev/website (your workspace in this group)
      Goal: Show the connection path and warn when relayed.
      Acceptance criteria:
      1. Indicator follows a path change within 1 s
      In scope: node
      Out of scope: relay
      Reviewer: Codex   Pass score: 90
      Notes of this group:
      - Path events use fields path, rtt_ms, since, limited.
      """

  Scenario: A direct task that waited for its folder starts with its title and plan in its first turn (A5.20 host)
    Given a host turn fixture
    And direct task 13 "Fix path event" with plan "Add path status and tests." waits behind task 12 for the folder of "Claude Code"
    When task 12 is done, task 13 starts, and the host composes its first turn from its assign item with its task message
    Then the composed turn text is:
      """
      [SoloDawn] group "Website" · you are "Claude Code" · task session T-13
      == 1 of 1 · assign · instruction
      Task T-13 "Fix path event"
      Folder: /home/dev/website (your workspace in this group)
      Plan: Add path status and tests.
      Reviewer: Codex   Pass score: 90
      Notes of this group:
      - Path events use fields path, rtt_ms, since, limited.
      """

  Scenario: Authority comes from member rows and never from message text
    Given a host turn fixture
    When the host composes a mention turn from an AI claiming owner authority in text
    Then the item header is:
      """
      == 1 of 1 · mention · from "Codex" (AI) in T-12 · information · call id Zk3m8vN2pQ5rT7wX9yB1cA
      """
    And the noted turn has depth 2 and thread of task 12

  Scenario: A confirmed brief without cause names its confirmer from card state
    Given a host turn fixture
    When the host composes a confirmed brief turn confirmed by the owner
    Then the composed turn text is:
      """
      [SoloDawn] group "Website" · you are "Secretary" · secretary session
      AI members (CLI on computer · folder · tier · state · skills):
      - "Claude Code" · claude on node · /home/dev/website · standard · waiting
      - "Codex" · codex on node · /home/dev/website-review · standard · waiting
      - "Secretary" · claude on node · /home/dev/website · standard · waiting
      == 1 of 1 · confirmed · instruction
      -- from "owner" in main · instruction
      Brief "Connection path" (id KaiCW9JC8UOG7lKNduDo8Q) confirmed.
      """

  Scenario: A decided card without cause carries choice and free-text answer from card state
    Given a host turn fixture
    When the host composes a decided card turn answered by the owner without cause
    Then the composed turn text is:
      """
      [SoloDawn] group "Website" · you are "Secretary" · secretary session
      AI members (CLI on computer · folder · tier · state · skills):
      - "Claude Code" · claude on node · /home/dev/website · standard · waiting
      - "Codex" · codex on node · /home/dev/website-review · standard · waiting
      - "Secretary" · claude on node · /home/dev/website · standard · waiting
      == 1 of 1 · decided · instruction
      -- from "owner" in main · instruction
      Decision (id Zk3m8vN2pQ5rT7wX9yB1cA): Where should the notice go?
      Choice: A (Top bar)
      Keep it subtle.
      """

  Scenario: Continue and hand-back turns match their session kind
    Given a host turn fixture
    When the host composes a "continue" turn and a "handback" turn for task 12
    Then the continue turn instructs the AI to continue the task
    And the hand-back turn asks for a handback post and to continue the task

  Scenario: A review turn says where the work is, by where the reporter's folder lies (A5.21 host)
    Given a host turn fixture
    When "Claude Code" in "/home/dev/website" on this computer reported task 12 and "Codex" reviews it in "/home/dev/website-review"
    Then the composed turn text is:
      """
      [SoloDawn] group "Website" · you are "Codex" · review session T-12
      == 1 of 1 · review · instruction
      Task T-12 "Node: report connection path changes"
      Goal: Show the connection path and warn when relayed.
      Acceptance criteria:
      1. Indicator follows a path change within 1 s
      In scope: node
      Out of scope: relay
      Pass score: 90
      Work: by "Claude Code" on node, in /home/dev/website
      The work is in another folder on this computer. Read it there; change nothing in it.
      -- from "Claude Code" (AI) in T-12 · information
      Ran go test ./...: 42 passed. Commit 9f2c1ab on branch t12-path-events.
      """
    When "Claude Code" in "/home/dev/website" on this computer reported task 12 and "Codex" reviews it in "/home/dev/website"
    Then the review turn says where the work is:
      """
      Work: by "Claude Code" on node, in /home/dev/website
      The work is in this folder.
      """
    When "Claude Code" in "/home/dev/website" on "laptop" reported task 12 and "Codex" reviews it in "/home/dev/website"
    Then the review turn says where the work is:
      """
      Work: by "Claude Code" on laptop, in /home/dev/website
      The work is on another computer. Get it the way the report says it is shared (with Git and a remote: fetch the branch the report names), and read it without changing what this folder holds.
      """
