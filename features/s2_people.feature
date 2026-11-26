Feature: Step 2 people, invitations, roles, removal and host surfaces
  A group owner or admin invites a second person through a one-time #join link,
  manages member roles and removal, admits the member's host AIs, and enforces
  per-group home and host surface boundaries.

  Scenario: Invitation link and joining across people without reload (A9.1)
    Given the people cast with P's group "Alpha" on P1 and pre-join history "Welcome before Q"
    When P creates an invitation for the people group on P1
    Then the #join invitation link carries the group id, P1's device id and a valid one-time secret
    When Q's browser joins the people group with that #join link as "Q"
    Then Q has a human member row with role "member" and Q's device log is on P1 as device rows
    And both P's and Q's browsers show Q and the pre-join history without a reload

  Scenario: A group of people without an AI has a main line, notes and members, and a later joiner reads its history (A9.22)
    Given the people cast with P's group "Alpha" on P1 and pre-join history "Welcome before Q"
    And P writes the note "Shared plan" in the people group
    When P creates an invitation for the people group on P1
    And Q's browser joins the people group with that #join link as "Q"
    Then Q's browser reads the main line "Welcome before Q", the note "Shared plan" and the members P and Q, and the group has no AI

  Scenario: A person who is already in another group on the home is added without a link (A9.46)
    Given the people cast where Q has joined P's group "Alpha" on P1 as "Q"
    When P creates group "Beta" on P1 and adds Q to it as "Q"
    Then Q's connected browser lists "Beta" without a reload
    And a browser of Q that connects to P1 later lists "Beta"
    And add_person on P1 for a person who is in no group there is refused with "not_found"

  Scenario: Invalid device log, second use, expired invitation and member invite refusal (A9.2)
    Given an in-process home node P1 for P with group "Alpha" under a controllable people clock
    When P creates an invitation with nonce "inv-1" on the controllable home
    And a joiner browser presents a device log that does not replay and sends join
    Then that join is refused with "forbidden" and no member row is added
    When Q's browser joins with that same invitation link as "Q"
    Then using that invitation link a second time is refused
    When P creates a second invitation with nonce "inv-2" and 10 minutes pass
    Then using the expired invitation link is refused
    When Q as a member sends invite for the controllable group
    Then Q's invite is refused with "forbidden"

  Scenario: Admin promotion, demotion and owner-only role rules (A9.3)
    Given the people cast where Q has joined P's group "Alpha" on P1 as "Q"
    When P sets Q's role to "admin" in the people group
    Then group set and invite by Q succeed in the people group
    And member set with role by admin Q is refused with "forbidden"
    When P sets Q's role back to "member" in the people group
    Then group set and invite by Q are refused with "forbidden"
    And setting a second owner, changing P's owner role and role changes by Q are refused

  Scenario: Removing a member, unfinished-task conflict and owner removal refusal (A9.4)
    Given the people cast where Q joined P's group "Alpha" on P1 and joined Q-team and Q-personal from Q1
    Then removing the owner P is refused with "forbidden"
    When an unfinished task has Q-team as assignee and then as reviewer
    Then removing Q while Q-team holds either unfinished task role was refused with "conflict"
    When P cancels that task and removes Q from the people group
    Then Q's browser receives groups without the group and P sees gone markers for Q and Q's AIs

  Scenario: A removed member's connections close and are refused (A9.4)
    Given the people cast where Q joined P's group "Alpha" on P1 and joined Q-team and Q-personal from Q1
    When P removes Q from the people group
    Then Q's connections to P1 close within 1 second and Q's next connection attempt to P1 is refused
    And Q1 has ended the sessions of Q's AIs in the people group and forgotten its home

  Scenario: Joining Q-team from Q1 and refusing non-member or duplicate-secretary add_ai (A9.5)
    Given the people cast where P's group "Alpha" on P1 has a secretary, P also has group "Solo" and Q has joined only the first as "Q"
    When Q1 joins Q-team to the people group with host.join
    Then Q-team's member row has host Q1, user Q and the folder Q chose as its workspace
    And Q's host.join to Q1 for P's other group is refused with "forbidden"
    And host.join from Q with secretary true while the group has a secretary is refused with "forbidden"

  Scenario: Node surface boundaries between P's and Q's devices and nodes (A9.6)
    Given the people cast where Q joined "Alpha" on P1 with Q-team on Q1 and P has a second group "Solo" on P1
    Then on P1 Q's browser may use the shared group's home surface while host commands, host.info and sync of P's other group are refused with "forbidden"
    And Q1 accepts no host command and no takeover.start from a device of P
