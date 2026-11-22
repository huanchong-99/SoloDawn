Feature: Team group home commands
  Scenario: A team home stores person bulletins and empty text without waking an AI
    Given a team home named "Team"
    When its owner posts a bulletin from project "Website" and secretary "Secretary"
    Then the team stores that signed bulletin as its owner without inbox items
    When its owner posts empty team text
    Then the team accepts the post without inbox items

  Scenario Outline: A team home refuses project work and settings
    Given a team home named "Team"
    When its owner sends team command '<fields>'
    Then the team command is refused with "<code>"

    Examples:
      | fields                                      | code        |
      | "t":"task","op":"start"                 | conflict    |
      | "t":"post","kind":"brief"               | conflict    |
      | "t":"post","kind":"confirm"             | conflict    |
      | "t":"member","op":"add_ai"              | conflict    |
      | "t":"group","op":"set","checks":[]    | bad_request |

  Scenario: A project home refuses a bulletin
    Given a project bulletin home named "Website"
    When its owner posts a bulletin from project "Website" and secretary "Secretary"
    Then the team command is refused with "conflict"
