Feature: People join and leave a home group
  The home admits a person through an open invitation or the group's open
  invitation code, stores their device log, and reads roles from member rows
  when deciding changes.

  Scenario: Joining and restoring a member's connection (A9.3 home half)
    Given a people home with an invited visitor
    When the visitor joins the people group
    Then the visitor is a member with a device row and a joined message
    When the visitor joins the people group again after a restore
    Then the join writes no rows and leaves the invitation open

  Scenario: Only the owner sets roles and an admin cannot remove an admin (A9.4 home half)
    Given a people home with an invited visitor
    When the visitor joins the people group
    And the owner promotes the visitor to admin
    Then the visitor's stored role is admin
    When another admin removes the visitor
    Then the people command answers forbidden
    When the visitor leaves the people group
    Then the visitor and device are gone and a departure message has no remover

  Scenario: Leaving one group retires its devices without forgetting another local membership
    Given a people home with an invited visitor
    When the visitor joins the people group
    And the visitor also belongs to another group homed here
    And the visitor leaves the people group
    Then only this group's device rows and device log are retired

  Scenario: An admin adds a person of another group on this home without an invitation (A9.46 home half)
    Given a people home with an invited visitor
    And the visitor also belongs to another group homed here
    When the owner adds the visitor with add_person as "visitor"
    Then the visitor is a member with one device row per entry of their log and a joined message by the owner
    When the owner adds the visitor with add_person as "visitor again"
    Then the people command answers "conflict"
    When the owner adds a person this home does not know with add_person
    Then the people command answers "not_found"

  Scenario: One invitation code admits several people until it is turned off (A9.47, A9.48 home half)
    Given a people home with an invited visitor
    When the owner makes an invitation code for 24 hours
    Then the code has 16 characters in four groups and ends 24 hours on
    When the owner sends that code command again
    Then the answer is ok with no code and the code is still open
    When the visitor joins the people group by the code
    Then the joiner is a member let in by the owner
    When a second person joins the people group by the code
    Then the joiner is a member let in by the owner
    When the owner turns the code off
    Then the people group has no open code
    When a third person joins the people group by the code
    Then the people command answers "forbidden"
    And the visitor and the second person are still members

  Scenario: A later code and the end of its time end a code, and only an admin handles one (A9.48 home half)
    Given a people home with an invited visitor
    When the owner makes an invitation code for 1 hour
    And the owner makes an invitation code for 24 hours
    And the visitor joins the people group by the first code
    Then the people command answers "forbidden"
    When the visitor joins the people group by the code
    Then the joiner is a member let in by the owner
    When the home's clock reaches the end of the code
    Then the people group has no open code
    When a second person joins the people group by the code
    Then the people command answers "forbidden"
    When the visitor, a member, makes an invitation code for 24 hours
    Then the people command answers forbidden
    When the owner makes an invitation code for 0 hours
    Then the people command answers "bad_request"
    When the owner makes an invitation code for 169 hours
    Then the people command answers "bad_request"

  Scenario: Removing a person ends the code, leaving does not, and a link and a member still join (A9.50, A9.52 home half)
    Given a people home with an invited visitor
    When the owner makes an invitation code for 24 hours
    And the visitor joins the people group
    Then the visitor is a member with a device row and a joined message
    When the visitor joins the people group by the code
    Then the join writes no rows and leaves the invitation open
    When a second person joins the people group by the code
    And the second person leaves the people group
    Then the code is still open
    When the owner removes the visitor
    Then the people group has no open code
