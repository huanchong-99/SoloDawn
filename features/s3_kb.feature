Feature: Group knowledge base (step 3)
  AIs store and replace long-lived documents per group with kb_put and search
  them with kb_search over FTS5, isolated across groups on the same home and
  preserved across home restarts and daily retention until forgotten by an
  admin, with outbound secret checks on both name and text.

  Scenario: An AI stores and searches a document, isolated across groups and preserved across a home restart (A10.6)
    Given an S3 knowledge-base node with groups "Alpha" and "Beta" and configured AIs "Claude Alpha" and "Claude Beta"
    When "Claude Alpha" stores the knowledge-base document "Decision: single writer" in "Alpha" and searches for "writer" and "absentword"
    Then kb_put created the document in "Alpha", kb_search for "writer" returned it, and kb_search for "absentword" returned an empty list
    When "Claude Beta" searches the knowledge base in "Beta" for "writer"
    Then kb_search in "Beta" returned an empty list without "Alpha"'s document
    When the S3 home restarts and "Claude Alpha" searches "Alpha" for "writer" again
    Then kb_search after the restart answers with the same document

  Scenario: Knowledge-base documents survive retention, vanish from search on forget, and refuse outbound secrets (A10.7)
    Given an S3 knowledge-base node with retention set to 7 days, group "Alpha", two browsers and configured AI "Claude Alpha"
    And 8 days ago "Alpha" received an old text message and "Claude Alpha" stored the document "Module:连接路径"
    When daily retention runs on the S3 node today
    Then the old text message in "Alpha" has expired while "Module:连接路径" remains searchable by "路径"
    When the first browser forgets "Module:连接路径" in "Alpha"
    Then the second browser receives a gone marker for "Module:连接路径" within 1 second and kb_search for "路径" returns an empty list
    When "Claude Alpha" calls kb_put with a test secret in text and in name
    Then both kb_put calls fail with "secret" and the home has no row for either secret document
