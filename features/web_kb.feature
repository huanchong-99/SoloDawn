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

  Scenario: A person changes a document's name and text in place, and adds one
    Given an online group with two knowledge-base documents
    When the reader changes the first document to "Decision: one writer" and "The home writes."
    Then a note_edit command keeps its id, focus returns to its button and the list shows the change
    When the home refuses the next edit with conflict and the reader saves a second name
    Then the field stays open and says "Not saved: another document already has that name."
    When the reader presses Escape in the field
    Then nothing more is sent and the second document reads as before
    When the reader adds a document named "Decision 2"
    Then nothing is sent and the new document's field says "Not saved: another document already has that name."
    When the reader names it "Convention: commits" and saves it
    Then a kb note command creates it with a new id and the list ends with it
    When the home disconnects
    Then the change and new-document buttons are disabled
