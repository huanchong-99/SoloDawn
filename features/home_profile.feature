Feature: AI profiles at the home

  Scenario: The owner edits the defaults on a new AI (A10.8)
    Given an AI has joined the profile home without profile fields
    Then the AI profile is model "", effort "", tier "standard" and skills ""
    When the profile owner sets model "opus", effort "high", tier "strong" and skills "Go backend"
    Then the AI profile is model "opus", effort "high", tier "strong" and skills "Go backend"
    And the AI's host is forbidden to set its tier
    And the profile owner cannot set tier "expert"

  Scenario: A stronger secretary can answer the assignee (A10.10 and A10.11)
    Given a profile task with secretary tier "strong"
    And the profile assignee has called the secretary in the task thread
    When the profile task fails review on "c1"
    Then the profile task is stuck on "review" at 1 with ask false
    When the profile task fails review on "c2"
    Then the profile task is stuck on "review" at 2 with ask true
    And the profile home refuses submission of "c3" until the assignee asks
    When the profile secretary answers the call
    Then the profile task is stuck on "review" at 2 with ask false
    When the profile task fails review on "c4"
    Then the profile task is stuck on "review" at 3 with ask true

  Scenario: Equal tiers do not block submission (A10.11)
    Given a profile task with secretary tier "standard"
    When the profile task fails review on "c1"
    And the profile task fails review on "c2"
    Then the profile task is stuck on "review" at 2 with ask false
    When the profile task passes review on "c3"
    Then the profile task has no stuck counter

  Scenario: One failed review asks nothing, and another assignee ends the count (A10.12)
    Given a profile task with secretary tier "strong"
    When the profile task fails review on "c1"
    Then the profile task is stuck on "review" at 1 with ask false
    When the profile owner gives the task to the reviewer
    Then the profile task has no stuck counter

  Scenario: An edit in the group ends following (A10.29)
    Given an AI has joined the profile home following defaults "opus", "high", "strong" and "Go backend"
    Then the AI row follows the defaults
    When the profile owner sets tier "light"
    Then the AI profile is model "opus", effort "high", tier "light" and skills "Go backend"
    And the AI row has its own profile

  Scenario: The owner restores the defaults and the host fills them (A10.30)
    Given an AI with its own profile "haiku", "", "light" and "" at the profile home
    When the profile owner sets follow
    Then the AI row follows the defaults
    And the AI profile is model "haiku", effort "", tier "light" and skills ""
    When the AI's host sets follow with "opus", "high", "strong" and "Go backend"
    Then the AI profile is model "opus", effort "high", tier "strong" and skills "Go backend"
    And the AI row follows the defaults

  Scenario: The host writes only a row that follows, and only the profile (A10.31)
    Given an AI with its own profile "haiku", "", "light" and "" at the profile home
    Then the AI's host setting follow with "opus", "high", "strong" and "" is refused with "conflict"
    And the AI's host setting tier "strong" without follow is refused with "forbidden"
    When the profile owner sets follow
    Then the AI's host setting follow and name "Opus" is refused with "forbidden"
    And another person setting follow is refused with "forbidden"
    And a member set with follow false is refused with "bad_request"
