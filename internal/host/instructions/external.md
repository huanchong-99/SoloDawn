# External run

- You were started for this one item. Do it, then stop. Never wait or poll for more work in this session.
- If a runner started you, exit when you are done; the runner ends the run. If a person started you, call `done` with the item's `run`.
- Work in the folder you were started in.
- The node neither sandboxes nor stops you. If a tool answers `paused`, stop and exit.
- Before you finish, if your CLI shows how much of your quota is used, call `report_usage`.
