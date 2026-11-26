Feature: The folders an AI CLI was used in on a host
  host.workspaces lists, for one CLI, the folders it was used in on this
  computer, newest use first, from the CLI's own session files or server.

  Scenario: Claude Code and Codex list the folders they were used in, newest first (A3.11 host)
    Given a host on disk whose Claude Code sessions were in "api", then in "web", then in a deleted folder and under the node's data directory
    And a scripted Codex whose threads, over two pages, were in "web", then in "docs", then in a deleted folder
    When the owner asks the host for the workspaces of "claude", "codex" and "external"
    Then the host lists "web, api" for "claude", "docs, web" for "codex" and nothing for "external"

  Scenario: With the CLIs' settings and login files unreadable, the same folders are listed (A3.12 host)
    Given a host on disk whose Claude Code sessions were in "api", then in "web", then in a deleted folder and under the node's data directory
    And a scripted Codex whose threads, over two pages, were in "web", then in "docs", then in a deleted folder
    And the home folder's ".claude.json", ".codex/auth.json" and ".codex/config.toml" cannot be read
    When the owner asks the host for the workspaces of "claude", "codex" and "external"
    Then the host lists "web, api" for "claude", "docs, web" for "codex" and nothing for "external"

  Scenario: A Codex that cannot list its threads lists nothing
    Given a host on disk whose Claude Code sessions were in "api", then in "web", then in a deleted folder and under the node's data directory
    And a scripted Codex that exits without listing
    When the owner asks the host for the workspaces of "claude", "codex" and "external"
    Then the host lists "web, api" for "claude", "" for "codex" and nothing for "external"
