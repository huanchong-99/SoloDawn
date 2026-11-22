@web
Feature: Knowledge base in the web client
  Scenario: Documents load on opening and older pages load on demand
    Given an online group with 51 knowledge-base documents
    When the reader opens Knowledge base
    Then the browser lists the newest 50 documents with names authors times and text
    When the reader loads earlier documents
    Then all 51 documents appear in creation order
    And typed documents stay out of Project memory

  Scenario: Search preserves the home ranking and clearing restores the list
    Given an online group with two knowledge-base documents
    When the reader searches for writer
    Then the browser shows the home matches best first and asks for a narrower query
    When the reader searches for absent
    Then the browser shows No matching documents
    When the reader clears the search
    Then both documents are listed in creation order

  Scenario: Forgetting removes a document from results and the list
    Given an online group with two knowledge-base documents
    When the reader searches for writer
    And the admin deletes the first result
    Then that document is absent from the results and focus returns to the fold
    When the reader clears the search
    Then that document is absent from the list
    When the reader searches again for writer
    Then the home no longer returns the forgotten document

  Scenario: Offline documents are read-only
    Given an online group with two knowledge-base documents
    When the home disconnects
    Then cached documents remain readable and search paging and deletion are disabled
