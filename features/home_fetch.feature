Feature: Fetching pages and single rows from a group home
  A client scrolls back through the main line or a task thread by asking the
  home for older pages below the lowest pos it holds, reads notes and the
  review ledger by kind, and fetches a single row by its type and id. Every
  fetched frame stays within the 60 KiB data-channel limit and omits gone
  markers, and an older fetch page arriving after a deletion never brings a
  deleted row back.

  Scenario: Scrolling back through the main line and a task thread reads older pages in creation order
    Given a group home with main-line messages, a decided card and a task thread
    When the browser fetches the main line from 0 with limit 2
    Then the fetched page holds the rows "m5, m4" with more true
    When the browser fetches the main line below the last pos with limit 2
    Then the fetched page holds the rows "c2, m1" with more false
    When the browser fetches the task thread from 0 with limit 1
    Then the fetched page holds the rows "t3" with more true
    When the browser fetches the task thread below the last pos with limit 2
    Then the fetched page holds the rows "c2" with more false

  Scenario: Notes and reviews are paged by kind and split at the ctl message limit
    Given a group home with notes, review messages across threads and four 16 KiB notes
    When the browser fetches kind "review" from 0 with limit 10
    Then the fetched page holds the rows "rev2, rev1" with more false
    When the browser fetches kind "note" from 0 with limit 10
    Then the fetched frame is at most 61440 bytes and holds "big4, big3, big2" with more true
    When the browser fetches kind "note" below the last pos with limit 10
    Then the fetched frame is at most 61440 bytes and holds "big1, n2, n1" with more false

  Scenario: Fetching one row and applying an older fetch page after a deletion never restores a deleted row
    Given a group home where the browser watches the group and holds notes "n1" and "n2"
    When the browser fetches the one row "note" "n2"
    Then the fetched page holds the rows "n2" with more false
    When an older fetch page of notes is taken before "n2" is forgotten
    And the browser applies the pushed deletion marker and then the older fetch page
    Then the browser's cache omits "n2" and keeps "n1"
    When the browser fetches the one row "note" "n2"
    Then the fetched page holds the rows "" with more false
    When the browser fetches kind "note" from 0 with limit 10
    Then the fetched page holds the rows "n1" with more false
