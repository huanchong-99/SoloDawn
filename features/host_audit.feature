Feature: Host audit surface
  Scenario: Approval request and decision with the deciding device are returned newest first
    Given an empty audit host store
    When the audit host records a "requested" entry in group "rZNvy-1jH6Z-BcPqA5U5BQ" for agent "claude" with detail "pnpm add -D @playwright/test" by "" with result "" at 1763600001000
    And the audit host records an "approval" entry in group "rZNvy-1jH6Z-BcPqA5U5BQ" for agent "claude" with detail "pnpm add -D @playwright/test" by "akWV_5PtNLIGhakuMAvpD9LcG1D8TUfc1URh48VREcs" with result "approved" at 1763600002000
    And an owner device sends audit surface request `{"t":"audit","id":6,"before":0,"limit":50}`
    Then the audit surface answers request 6 with 2 rows:
      | id | ts            | g                      | ai     | what      | detail                        | by                                          | result   |
      | 2  | 1763600002000 | rZNvy-1jH6Z-BcPqA5U5BQ | claude | approval  | pnpm add -D @playwright/test | akWV_5PtNLIGhakuMAvpD9LcG1D8TUfc1URh48VREcs | approved |
      | 1  | 1763600001000 | rZNvy-1jH6Z-BcPqA5U5BQ | claude | requested | pnpm add -D @playwright/test |                                             |          |

  Scenario: Paging by before across identical timestamps neither skips nor repeats rows
    Given an empty audit host store
    And 5 setting audit entries recorded at identical timestamp 1763600009000
    Then paging the audit surface with limit 2 from before 0 yields row ids "5,4"
    And paging the audit surface with limit 2 from before 4 yields row ids "3,2"
    And paging the audit surface with limit 2 from before 2 yields row ids "1"
    And paging the audit surface with limit 2 from before 1 yields row ids ""

  Scenario: Empty audit table answers with an empty JSON array
    Given an empty audit host store
    When an owner device sends audit surface request `{"t":"audit","id":9,"before":0,"limit":10}`
    Then the audit surface raw result is `{"rows":[]}`

  Scenario Outline: Audit surface enforces request shape and bounds
    Given an empty audit host store
    And 3 setting audit entries recorded at identical timestamp 1763600009000
    When an owner device sends audit surface request `<request>`
    Then the audit surface outcome is `<outcome>`

    Examples:
      | request                                       | outcome     |
      | {"t":"audit","id":1,"before":-1,"limit":1}    | bad_request |
      | {"t":"audit","id":1,"before":0,"limit":-1}    | bad_request |
      | {"t":"audit","id":1,"before":0,"limit":0}     | bad_request |
      | {"t":"audit","id":1,"before":0,"limit":101}   | bad_request |
      | {"t":"host.info","id":1,"before":0,"limit":1} | bad_request |
      | {"t":"audit","id":1,"before":0}               | bad_request |
      | {"t":"audit","id":1,"before":0,"limit":1}     | ok:1        |
      | {"t":"audit","id":1,"before":0,"limit":100}   | ok:3        |
