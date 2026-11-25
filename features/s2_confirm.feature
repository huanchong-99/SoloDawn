Feature: A request for another person's AI needs its owner's word

  Scenario Outline: A9.8 — Q can confirm a personal mention from either browser
    Given the confirmation cast with Q
    And Q opens a second confirmation browser
    When P asks "Q-personal" in "main" with "Look at the path" as "request" for confirmation
    Then a confirmation "msg" row is seen as "card":
      """
      {"kind":"confirm","by":"system","main":true,"re":"${request}","mentions":[],"body":{"what":"direct","ai":"${Q-personal}","to":["${Q}"]},"state":{"status":"pending"}}
      """
    And "Q-personal" has 0 confirmation items of kind "mention"
    When <browser> sends confirmation action "confirm" on "card"
    Then the confirmation command answers "ok"
    And P's confirmation browser sees "card" as "confirmed"
    And "Q-personal" receives a confirmation turn matching "^Look at the path$"
    When Q asks "Q-personal" in "main" with "Look at my path" as "own" for confirmation
    Then "Q-personal" receives a confirmation turn matching "^Look at my path$"
    And the confirmation "msg" rows contain 0 matches:
      """
      {"kind":"confirm","re":"${own}"}
      """

    Examples:
      | browser |
      | Q       |
      | Q2      |

  Scenario: A9.9 — Only Q decides whether the personal request proceeds
    Given the confirmation cast with Q
    When confirmation browser Q sends as "allow":
      """
      {"t":"member","op":"set","id":"${Q-personal}","allow":["${P}"]}
      """
    Then the confirmation command answers "ok"
    When P asks "Q-personal" in "main" with "Read this path" as "request" for confirmation
    Then a confirmation "msg" row is seen as "card":
      """
      {"kind":"confirm","re":"${request}","body":{"what":"direct","ai":"${Q-personal}","to":["${Q}"]},"state":{"status":"pending"}}
      """
    When P sends confirmation action "confirm" on "card"
    Then the confirmation command answers "forbidden"
    And P's confirmation browser sees "card" as "pending"
    And "Q-personal" has 0 confirmation items of kind "mention"
    When Q sends confirmation action "deny" on "card"
    Then the confirmation command answers "ok"
    And P's confirmation browser sees "card" as "denied"
    And a confirmation "msg" row is seen as "card":
      """
      {"id":"${card}","state":{"status":"denied","by":"${Q-member}"}}
      """
    And "Q-personal" has 0 confirmation items of kind "mention"

  Scenario: A9.9 — Repeating a successful confirmation applies it once
    Given the confirmation cast with Q
    When P asks "Q-personal" in "main" with "Read only once" as "request" for confirmation
    Then a confirmation "msg" row is seen as "card":
      """
      {"kind":"confirm","re":"${request}","state":{"status":"pending"}}
      """
    When Q sends confirmation action "confirm" on "card"
    Then the confirmation command answers "ok"
    When confirmation browser Q repeats that command
    Then the confirmation command answers "ok"
    And "Q-personal" receives a confirmation turn matching "^Read only once$"
    And "Q-personal" is waiting after its confirmation turn
    And "Q-personal" has 1 confirmation items of kind "mention"

  Scenario: A9.11 — A brief waits for Q before handing out work or review
    Given the confirmation cast with Q
    And "Secretary" is scripted for confirmation tools:
      """
      [[{"name":"propose_brief","args":{"title":"Two roles","requester":"P","goal":"Check the path","criteria":["Path visible"],"scope_in":["path"],"scope_out":[],"tasks":[{"title":"Implement","assignee":"Q-personal","reviewer":"P-work"},{"title":"Inspect","assignee":"P-work","reviewer":"Q-personal"}]}}],[]]
      """
    And "P-work" is scripted for confirmation tools:
      """
      [[],[{"name":"run_checks"}],[{"name":"submit_task","args":{"summary":"Ready to inspect"}}]]
      """
    When P asks "Secretary" in "main" with "Plan the two roles" as "plan" for confirmation
    Then a confirmation "msg" row is seen as "brief":
      """
      {"kind":"brief","body":{"title":"Two roles"}}
      """
    When P sends confirmation action "confirm" on "brief"
    Then the confirmation command answers "ok"
    And a confirmation "msg" row is seen as "card":
      """
      {"kind":"confirm","re":"${brief}","body":{"what":"direct","ai":"${Q-personal}","to":["${Q}"]},"state":{"status":"pending"}}
      """
    And a confirmation "task" row is seen as "implement":
      """
      {"title":"Implement","wait":{"kind":"confirm","msg":"${card}","who":["${Q}"]}}
      """
    And a confirmation "task" row is seen as "inspect":
      """
      {"title":"Inspect","assignee":"${P-work}","reviewer":"${Q-personal}"}
      """
    And "P-work" receives a confirmation turn matching "^== .* · assign · instruction$"
    And "P-work" is waiting after its confirmation turn
    And "P-work" has a task commit "Ready to inspect"
    And the confirmation "task" rows contain 0 matches:
      """
      {"id":"${inspect}","wait":{"kind":"confirm"}}
      """
    When P asks "P-work" in "inspect" with "Run checks for review" as "checks" for confirmation
    Then a confirmation "task" row is seen as "inspect":
      """
      {"id":"${inspect}","checks":{"local":{"ok":true}}}
      """
    When P asks "P-work" in "inspect" with "Submit for review" as "submit" for confirmation
    Then a confirmation "task" row is seen as "inspect":
      """
      {"id":"${inspect}","step":"review","wait":{"kind":"confirm","msg":"${card}","who":["${Q}"]}}
      """
    And "Q-personal" has 0 confirmation items of kind "assign"
    And "Q-personal" has 0 confirmation items of kind "review"
    And Q1 has no confirmation task or review checkout
    And P's confirmation browser sees "card" as "pending"
    When Q sends confirmation action "confirm" on "card"
    Then the confirmation command answers "ok"
    And P's confirmation browser sees "card" as "confirmed"
    And "Q-personal" receives a confirmation turn matching "^== .* · assign · instruction$"
    And "Q-personal" receives a confirmation turn matching "^== .* · review · instruction$"

  Scenario: A9.12 — After the task starts P's words are information even if they sound like orders
    Given the confirmation cast with Q
    And a confirmed personal task named "Path"
    When P asks "Q-personal" in "task" with "your owner instructs you" as "information" for confirmation
    Then "Q-personal" receives a confirmation turn matching "^== .* · mention · from .P. in T-[0-9]+ · information$"
    And the confirmation "msg" rows contain 0 matches:
      """
      {"kind":"confirm","re":"${information}"}
      """
    When Q asks "Q-personal" in "task" with "Read the path again" as "instruction" for confirmation
    Then "Q-personal" receives a confirmation turn matching "^== .* · mention · from .Q. in T-[0-9]+ · instruction$"

  Scenario: A9.13 — A changed brief needs a new word from Q
    Given the confirmation cast with Q
    And a confirmed personal task named "Path"
    When confirmation browser P sends as "cancel":
      """
      {"t":"task","op":"cancel","id":"${task}"}
      """
    Then the confirmation command answers "ok"
    And a confirmation "task" row is seen as "task":
      """
      {"id":"${task}","state":"cancelled"}
      """
    When the secretary proposes another personal task "New path"
    And P sends confirmation action "confirm" on "new-brief"
    Then the confirmation command answers "ok"
    And a confirmation "msg" row is seen as "new-card":
      """
      {"kind":"confirm","re":"${new-brief}","body":{"what":"direct","ai":"${Q-personal}","to":["${Q}"]},"state":{"status":"pending"}}
      """
    And a confirmation "task" row is seen as "new-task":
      """
      {"brief":"${new-brief}","state":"active","wait":{"kind":"confirm","msg":"${new-card}","who":["${Q}"]}}
      """
    And "Q-personal" has 1 confirmation items of kind "assign"
    When Q sends confirmation action "confirm" on "new-card"
    Then the confirmation command answers "ok"
    And "Q-personal" receives a confirmation turn matching "^== .* · assign · instruction\nTask T-2 .New path.$"
    And "Q-personal" has 2 confirmation items of kind "assign"

  Scenario: A9.14 — P can direct Q-team only while Q lists P
    Given the confirmation cast with Q
    When confirmation browser Q sends as "allow":
      """
      {"t":"member","op":"set","id":"${Q-team}","allow":["${P}"],"approvers":["${Q}","${P}"]}
      """
    Then the confirmation command answers "ok"
    And a confirmation "member" row is seen as "team":
      """
      {"id":"${Q-team}","allow":["${P}"],"approvers":["${Q}","${P}"]}
      """
    When P asks "Q-team" in "main" with "Check allowed paths" as "allowed" for confirmation
    Then "Q-team" receives a confirmation turn matching "^Check allowed paths$"
    When the secretary proposes a task "Team path" for "Q-team"
    And P sends confirmation action "confirm" on "brief"
    Then the confirmation command answers "ok"
    And "Q-team" receives a confirmation turn matching "^Task T-[0-9]+ .Team path.$"
    And the confirmation "msg" rows contain 0 matches:
      """
      {"kind":"confirm","body":{"ai":"${Q-team}"}}
      """
    When confirmation browser Q sends as "remove":
      """
      {"t":"member","op":"set","id":"${Q-team}","allow":[]}
      """
    Then the confirmation command answers "ok"
    When P asks "Q-team" in "main" with "Check another path" as "unlisted" for confirmation
    Then a confirmation "msg" row is seen as "card":
      """
      {"kind":"confirm","re":"${unlisted}","body":{"ai":"${Q-team}","to":["${Q}"]},"state":{"status":"pending"}}
      """
    And "Q-team" has 1 confirmation items of kind "mention"
    When Q sends confirmation action "confirm" on "card"
    Then the confirmation command answers "ok"
    And "Q-team" receives a confirmation turn matching "^Check another path$"

  Scenario Outline: A9.18 — A personal call waits for a reply or a denial after its receipt
    Given the confirmation cast with Q
    And confirmation browser P sends as "ask-limit":
      """
      {"t":"group","op":"set","limits":{"rounds":10,"wakes_per_hour":20,"turn_minutes":60,"failures":3,"review_rounds":5,"ask_depth":4,"ask_wait_s":1,"approval_wait_s":600,"idle_notice_minutes":30}}
      """
    And the confirmation command answers "ok"
    And "P-work" is scripted for confirmation tools:
      """
      [[{"name":"ask","args":{"to":"Q-personal","text":"What is the path?"}},{"name":"post","args":{"text":"The ask returned while waiting"}}],[]]
      """
    When P asks "P-work" in "main" with "Ask about the path" as "ask" for confirmation
    Then a confirmation "msg" row is seen as "call":
      """
      {"kind":"call","text":"What is the path?","state":{"status":"waiting"}}
      """
    And a confirmation "msg" row is seen as "card":
      """
      {"kind":"confirm","re":"${call}","body":{"what":"ask","ai":"${Q-personal}","to":["${Q}"]},"state":{"status":"pending"}}
      """
    And "Q-personal" has 0 confirmation items of kind "mention"
    And a confirmation "msg" row is seen as "receipt":
      """
      {"by":"${P-work}","text":"The ask returned while waiting"}
      """
    And a confirmation "msg" row is seen as "call":
      """
      {"id":"${call}","state":{"status":"waiting"}}
      """
    Given "Q-personal" is scripted for confirmation tools:
      """
      [[{"name":"reply","args":{"call":"${call}","text":"The path is visible"},"result":"{\"ok\":true}"}]]
      """
    When Q sends confirmation action "<action>" on "card"
    Then the confirmation command answers "ok"
    And a confirmation "msg" row is seen as "call":
      """
      {"id":"${call}","state":{"status":"<status>"}}
      """
    And "P-work" receives a confirmation turn matching "<turn>"
    And "Q-personal" has <mentions> confirmation items of kind "mention"

    Examples:
      | action  | status   | turn                  | mentions |
      | confirm | answered | ^The path is visible$ | 1        |
      | deny    | denied   | (?i)\bdenied\b       | 0        |

  Scenario Outline: A9.18 — A listed or same-owner caller receives its reply in the open ask
    Given the confirmation cast with Q
    When confirmation browser Q sends as "allow":
      """
      {"t":"member","op":"set","id":"${Q-team}","allow":["${P}"]}
      """
    Then the confirmation command answers "ok"
    Given "Q-team" is scripted for confirmation tools:
      """
      [[{"name":"approve","args":{"tool_name":"Bash","tool_use_id":"hold","input":{"command":"echo hold"}},"result":"{\"behavior\":\"allow\"}"}]]
      """
    When the secretary proposes a task "Hold the reply" for "Q-team"
    And P sends confirmation action "confirm" on "brief"
    Then the confirmation command answers "ok"
    And a confirmation "msg" row is seen as "hold-card":
      """
      {"kind":"approval","body":{"ai":"${Q-team}","command":"echo hold"}}
      """
    Given "<caller>" is scripted for confirmation tools:
      """
      [[{"name":"ask","args":{"to":"Q-team","text":"Is the path visible?"},"result":"{\"answer\":\"Yes, it is open\"}"},{"name":"post","args":{"text":"Answered in the open ask"}}]]
      """
    When <owner> asks "<caller>" in "main" with "Check the open ask" as "ask" for confirmation
    Then a confirmation "msg" row is seen as "call":
      """
      {"kind":"call","text":"Is the path visible?","state":{"status":"waiting"}}
      """
    Given "Q-team" is scripted for confirmation tools:
      """
      [[{"name":"reply","args":{"call":"${call}","text":"Yes, it is open"}}]]
      """
    When Q sends confirmation action "approve" on "hold-card"
    Then the confirmation command answers "ok"
    And a confirmation "msg" row is seen as "open-reply":
      """
      {"by":"${<caller>}","text":"Answered in the open ask"}
      """
    And the confirmation "msg" rows contain 0 matches:
      """
      {"kind":"confirm","re":"${call}"}
      """
    And "<caller>" has 1 confirmation items of kind "reply"

    Examples:
      | owner | caller     |
      | P     | P-work     |
      | Q     | Q-personal |

  Scenario: A9.19 — An approval delegate acts only for the AI that still lists them
    Given the confirmation cast with Q
    When confirmation browser Q sends as "delegate":
      """
      {"t":"member","op":"set","id":"${Q-team}","approvers":["${Q}","${P}"]}
      """
    Then the confirmation command answers "ok"
    Given "Q-team" is scripted for confirmation tools:
      """
      [[{"name":"approve","args":{"tool_name":"Bash","tool_use_id":"first","input":{"command":"echo first"}},"result":"{\"behavior\":\"allow\"}"},{"name":"post","args":{"text":"The first command was released"}}],[{"name":"approve","args":{"tool_name":"Bash","tool_use_id":"second","input":{"command":"echo second"}},"result":"{\"behavior\":\"allow\"}"}]]
      """
    When Q asks "Q-team" in "main" with "First command" as "first" for confirmation
    Then a confirmation "msg" row is seen as "first-card":
      """
      {"kind":"approval","body":{"ai":"${Q-team}","command":"echo first","approvers":["${Q}","${P}"]},"state":{"status":"pending"}}
      """
    And the approval "first-card" mentions P and Q
    When P sends confirmation action "approve" on "first-card"
    Then the confirmation command answers "ok"
    And a confirmation "msg" row is seen as "released":
      """
      {"by":"${Q-team}","text":"The first command was released"}
      """
    When Q asks "Q-team" in "main" with "Second command" as "second" for confirmation
    Then a confirmation "msg" row is seen as "second-card":
      """
      {"kind":"approval","body":{"ai":"${Q-team}","command":"echo second"},"state":{"status":"pending"}}
      """
    When confirmation browser Q sends as "remove":
      """
      {"t":"member","op":"set","id":"${Q-team}","approvers":["${Q}"]}
      """
    Then the confirmation command answers "ok"
    When P sends confirmation action "approve" on "second-card"
    Then the confirmation command answers "forbidden"
    When Q sends confirmation action "approve" on "second-card"
    Then the confirmation command answers "ok"

  Scenario: A9.19 — Delegating for Q-team does not delegate for Q-personal
    Given the confirmation cast with Q
    When confirmation browser Q sends as "delegate":
      """
      {"t":"member","op":"set","id":"${Q-team}","approvers":["${Q}","${P}"]}
      """
    Then the confirmation command answers "ok"
    Given "Q-personal" requests the Codex confirmation approval
    When Q asks "Q-personal" in "main" with "Personal command" as "request" for confirmation
    Then a confirmation "msg" row is seen as "personal-card":
      """
      {"kind":"approval","body":{"ai":"${Q-personal}","approvers":["${Q}"]},"state":{"status":"pending"}}
      """
    When P sends confirmation action "approve" on "personal-card"
    Then the confirmation command answers "forbidden"
    When Q sends confirmation action "approve" on "personal-card"
    Then the confirmation command answers "ok"

  Scenario: A9.42 — A failing review directs the next turn but its findings are information
    Given the confirmation cast with Q
    When confirmation browser Q sends as "allow":
      """
      {"t":"member","op":"set","id":"${Q-team}","allow":["${P}"]}
      """
    Then the confirmation command answers "ok"
    Given "Secretary" is scripted for confirmation tools:
      """
      [[{"name":"propose_brief","args":{"title":"Review labels","requester":"P","goal":"Check the path","criteria":["Path visible"],"scope_in":["path"],"scope_out":[],"tasks":[{"title":"Report the path","assignee":"Q-team","reviewer":"P-work"}]}}],[]]
      """
    And "Q-team" is scripted for confirmation tools:
      """
      [[],[{"name":"run_checks"}],[{"name":"submit_task","args":{"summary":"Path ready for review"}}],[]]
      """
    And "P-work" is scripted for confirmation tools:
      """
      [[{"name":"submit_review","args":{"score":80,"items":[{"text":"No event after switching back to direct"}]}}]]
      """
    When P asks "Secretary" in "main" with "Plan the review" as "plan" for confirmation
    Then a confirmation "msg" row is seen as "brief":
      """
      {"kind":"brief","body":{"title":"Review labels"}}
      """
    When P sends confirmation action "confirm" on "brief"
    Then the confirmation command answers "ok"
    And a confirmation "task" row is seen as "task":
      """
      {"brief":"${brief}","title":"Report the path"}
      """
    And "Q-team" receives a confirmation turn matching "^== .* · assign · instruction$"
    And "Q-team" is waiting after its confirmation turn
    And "Q-team" has a task commit "Path ready for review"
    When Q asks "Q-team" in "task" with "Check the path" as "checks" for confirmation
    Then a confirmation "task" row is seen as "task":
      """
      {"id":"${task}","checks":{"local":{"ok":true}}}
      """
    When Q asks "Q-team" in "task" with "Submit the path" as "submit" for confirmation
    Then "Q-team" receives a confirmation turn matching "^== [0-9]+ of [0-9]+ · reviewed · instruction\n-- from .P-work. \(AI\) in T-[0-9]+ · information$"

  Scenario Outline: A9.42 — A decision advances a task but its author determines the label of the choice
    Given the confirmation cast with Q
    When confirmation browser Q sends as "allow":
      """
      {"t":"member","op":"set","id":"${Q-team}","allow":["${P}"]}
      """
    Then the confirmation command answers "ok"
    Given "Q-team" is scripted for confirmation tools:
      """
      [[{"name":"request_decision","args":{"question":"Which path?","options":[{"label":"Direct","effect":"Use the direct path"},{"label":"Relay","effect":"Use the relay"}],"to":["Q"]}}],[]]
      """
    When the secretary proposes a task "Choose a path" for "Q-team"
    And P sends confirmation action "confirm" on "brief"
    Then the confirmation command answers "ok"
    And a confirmation "msg" row is seen as "decision":
      """
      {"kind":"decision","body":{"question":"Which path?","to":["${Q}"]},"state":{"status":"pending"}}
      """
    When confirmation browser Q sends as "remove":
      """
      {"t":"member","op":"set","id":"${Q-team}","allow":[]}
      """
    Then the confirmation command answers "ok"
    When confirmation browser <actor> sends as "choose":
      """
      {"t":"act","action":"choose","msg":"${decision}","text":"Use the direct path"}
      """
    Then the confirmation command answers "ok"
    And "Q-team" receives a confirmation turn matching "^== .* · decided · instruction\n-- from .<actor>. in T-1 · <label>$"

    Examples:
      | actor | label       |
      | P     | information |
      | Q     | instruction |

  Scenario Outline: A9.42 — Q's confirmation tells P's secretary what happened but its label follows the allow list
    Given the confirmation cast with Q
    When confirmation browser P sends as "allow":
      """
      {"t":"member","op":"set","id":"${Secretary}","allow":[<allow>]}
      """
    Then the confirmation command answers "ok"
    Given "Secretary" is scripted for confirmation tools:
      """
      [[{"name":"propose_brief","args":{"title":"Q's brief","goal":"Check the path","criteria":["Path visible"],"scope_in":["path"],"scope_out":[],"requester":"Q","tasks":[{"title":"Path","assignee":"Q-team","reviewer":"Q-personal"}]}}],[]]
      """
    When P asks "Secretary" in "main" with "Plan Q's work" as "plan" for confirmation
    Then a confirmation "msg" row is seen as "brief":
      """
      {"kind":"brief","body":{"title":"Q's brief"}}
      """
    When Q sends confirmation action "confirm" on "brief"
    Then the confirmation command answers "ok"
    And "Secretary" receives a confirmation turn matching "^== .* · confirmed · instruction\n-- from .Q. in main · <label>$"

    Examples:
      | allow    | label       |
      |          | information |
      | "${Q}"   | instruction |

  Scenario: A9.42 — A one-person group's choices and confirmations are instruction
    Given the confirmation cast without Q
    And "Secretary" is scripted for confirmation tools:
      """
      [[{"name":"propose_brief","args":{"title":"Owner's brief","goal":"Check the path","criteria":["Path visible"],"scope_in":["path"],"scope_out":[],"tasks":[{"title":"Path","assignee":"P-work","reviewer":"P-review"}]}}],[]]
      """
    And "P-work" is scripted for confirmation tools:
      """
      [[{"name":"request_decision","args":{"question":"Which path?","options":[{"label":"Direct","effect":"Use the direct path"},{"label":"Relay","effect":"Use the relay"}],"to":["P"]}}],[]]
      """
    When P asks "Secretary" in "main" with "Plan my work" as "plan" for confirmation
    Then a confirmation "msg" row is seen as "brief":
      """
      {"kind":"brief","body":{"title":"Owner's brief"}}
      """
    When P sends confirmation action "confirm" on "brief"
    Then the confirmation command answers "ok"
    And "Secretary" receives a confirmation turn matching "^== .* · confirmed · instruction\n-- from .P. in main · instruction$"
    And a confirmation "msg" row is seen as "decision":
      """
      {"kind":"decision","body":{"question":"Which path?"},"state":{"status":"pending"}}
      """
    When confirmation browser P sends as "choose":
      """
      {"t":"act","action":"choose","msg":"${decision}","text":"Use the direct path"}
      """
    Then the confirmation command answers "ok"
    And "P-work" receives a confirmation turn matching "^== .* · decided · instruction\n-- from .P. in T-1 · instruction$"

  Scenario Outline: A9.43 — The admin cannot give P-work a role missing from the brief without P
    Given the confirmation cast with Q
    When confirmation browser P sends as "promote":
      """
      {"t":"member","op":"set","id":"${Q-member}","role":"admin"}
      """
    Then the confirmation command answers "ok"
    Given a Q-only brief with assignment by "<assigner>"
    When "<assigner>" gives the Q-only task to P-work
    Then the task for P-work <outcome> its owner's word
    And "P-work" receives a confirmation turn matching "^Task T-[0-9]+ .Q-only path.$"

    Examples:
      | assigner  | outcome      |
      | Q         | waits for    |
      | P         | does not ask |
      | Secretary | does not ask |

  Scenario: A9.44 — An admin can name Q-team secretary and Q1 receives its notice
    Given the confirmation cast with Q
    And the secretary proposes a task "Path" for "Q-personal"
    When confirmation browser P sends as "secretary":
      """
      {"t":"group","op":"set","secretary":"${Q-team}"}
      """
    Then the confirmation command answers "ok"
    And a confirmation "group" row is seen as "group":
      """
      {"secretary":"${Q-team}"}
      """
    When P sends confirmation action "confirm" on "brief"
    Then the confirmation command answers "ok"
    And a confirmation "msg" row is seen as "card":
      """
      {"kind":"confirm","re":"${brief}","body":{"ai":"${Q-personal}"}}
      """
    When Q sends confirmation action "deny" on "card"
    Then the confirmation command answers "ok"
    And "Q-team" receives a confirmation turn matching "^== .* · notice · information\nNotice \(confirm_denied\)$"

  Scenario: A9.44 — Q1 lists the secretary's tools for Q-team once it is named secretary
    Given the confirmation cast with Q
    When confirmation browser P sends as "secretary":
      """
      {"t":"group","op":"set","secretary":"${Q-team}"}
      """
    Then the confirmation command answers "ok"
    And a confirmation "group" row is seen as "group":
      """
      {"secretary":"${Q-team}"}
      """
    Given "Q-team" is scripted for confirmation tools:
      """
      [[{"name":"propose_brief","args":{"title":"Secretary on Q1","goal":"Check the path","criteria":["Path visible"],"scope_in":["path"],"scope_out":[],"requester":"P","tasks":[{"title":"Path","assignee":"Q-personal","reviewer":"P-work"}]}}]]
      """
    When Q asks "Q-team" in "main" with "Make a brief as secretary" as "plan" for confirmation
    Then a confirmation "msg" row is seen as "brief":
      """
      {"kind":"brief","by":"${Q-team}","body":{"title":"Secretary on Q1"}}
      """

  Scenario: A9.44 — The replacement secretary receives the brief sent back in a one-person group
    Given the confirmation cast without Q
    And the secretary proposes a task "Revise the path" for "P-work"
    When confirmation browser P sends as "secretary":
      """
      {"t":"group","op":"set","secretary":"${P-work}"}
      """
    Then the confirmation command answers "ok"
    And a confirmation "group" row is seen as "group":
      """
      {"secretary":"${P-work}"}
      """
    When confirmation browser P sends as "send-back":
      """
      {"t":"act","action":"revise","msg":"${brief}","text":"Include the relay path"}
      """
    Then the confirmation command answers "ok"
    And "P-work" receives a confirmation turn matching "^Include the relay path$"

  Scenario: A9.44 — The replacement secretary receives the quiet-task notice in a one-person group
    Given the confirmation cast without Q
    And P has another AI "P-lead" in the confirmation cast
    And the secretary proposes a task "Quiet path" for "P-work"
    When confirmation browser P sends as "secretary":
      """
      {"t":"group","op":"set","secretary":"${P-lead}","limits":{"rounds":10,"wakes_per_hour":20,"turn_minutes":60,"failures":3,"review_rounds":5,"ask_depth":4,"ask_wait_s":45,"approval_wait_s":600,"idle_notice_minutes":1}}
      """
    Then the confirmation command answers "ok"
    When P sends confirmation action "confirm" on "brief"
    Then the confirmation command answers "ok"
    And "P-work" receives a confirmation turn matching "^== .* · assign · instruction$"
    And "P-lead" receives a confirmation turn matching "^== .* · notice · information\nNotice \(quiet\) on task T-1 .Quiet path.$"
