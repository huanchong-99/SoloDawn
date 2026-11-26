Feature: People join and leave a home group
  The home admits a person through an open invitation, stores their device log,
  and reads roles from member rows when deciding changes.

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
