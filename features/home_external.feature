Feature: The home keeps the external AI's tool label

  Scenario: An external AI's owner controls its tool label
    Given an external AI joins its home with tool "ZCode"
    Then the external member row has tool "ZCode" and the default profile
    When another member tries to set the external tool to "Cursor"
    Then the home refuses the external member change as "forbidden"
    And the external member row has tool "ZCode" and the default profile
    When the owner sets the external tool to "Cursor"
    Then the external member row has tool "Cursor" and the default profile
    When the owner sets the external tool to ""
    Then the external member row has tool "" and the default profile

  Scenario: An external join with a tool label of 33 code points is refused
    When an external AI joins its home with tool "文文文文文文文文文文文文文文文文文文文文文文文文文文文文文文文文文"
    Then the home refuses the external member change as "bad_request"
