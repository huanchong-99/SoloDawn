# Agent Note: Interactive `?demo` simulation world and redesigned workbench interface

Status: implemented

## Problem

Evaluating full multi-computer workflows (onboarding, group creation, member confirmation, brief
approval, task assignment, review, takeover, group export, and invitation codes) previously
required booting local nodes, WebRTC links, and AI CLIs. Simultaneously, the initial prototype UI
needed a cleaner, restrained visual hierarchy that stayed readable as a multi-AI developer
workbench on both desktop and mobile viewports.

## Decision

The owner and coordinator introduced an interactive `?demo` simulation mode alongside a full
interface redesign (`web/src/`, `web/design/README.md` sections 1 and 3):

1. **In-browser `?demo` simulation (`web/features/sim/`):** Launching the web client with `?demo`
   mounts a scripted, deterministic multi-computer simulation world (owner node, remote member,
   secretary, working AIs, simulated approvals/decisions, terminal takeover, group move, and
   invitation codes) connected through the same `Net` / `Peer` client abstractions without modifying
   production sync logic.
2. **Restrained developer-workbench visual redesign (`web/design/README.md`):** Replaced heavy
   borders and scattered card styling with calm tinted neutrals (`--bg`, `--surface`, `--raised`),
   a single interactive accent (`--sky`), small semantic signals (`--dawn` for attention, green
   for pass/online, red for failures/stop), dark terminal styling in both themes, tabular metrics,
   and 4 px spacing rhythm, leaving superseded static HTML mockups in `web/design/` only as
   historical records.
3. **Simulation bug fixes kept separate from production client rules:** Walkthrough fixes (such as
   per-task closing requests, making the waiting bar label a link, routing host commands past
   moved-group checks, parsing member mentions without eating Chinese prose, and avoiding double-
   concatenating pasted pairing codes) were isolated to simulation/UI layers and pinned by tests.

## Alternatives considered

- **Maintaining separate static HTML mockups or recording screenshots for evaluation:** quickly drifted
  from live Svelte components and could not validate interactive flows such as invitation codes,
  takeover, or scroll position.
- **Special-casing demo logic directly inside production store modules:** rejected in favor of a
  modular `SimNode` / `World` driver plugged beneath `Peer` / `Net`.

## Consequences

- Interactive product walkthroughs can reproduce real-user bugs rapidly in a browser, exposing
  issues later fixed for both `?demo` and production components (`fecea3145`, `9cf60fbb8`).
- The simulation director and scripted timelines must be kept aligned whenever core flows change,
  or retired explicitly when superseded by live-node guides.

## Verification

- Implemented in commits `3308fafa4` (`Web: a clickable demo world at ?demo`), `6453c7d85` (`Web: a premium, simple redesign of every screen`), `fecea3145` (`Web: fix the rough edges found in the demo walk-through`), `9cf60fbb8` (M5 demo closing/move/takeover fixes), and `6c2f014f1` (demo pairing code input fix), covered by `web/features/sim/sim.test.ts`, `panel.test.ts`, `web/e2e/m5.spec.ts`, and `web/features/a11y.spec.ts`.

## Related

- [web design README](../../../../web/design/README.md)
- [web README](../../../../web/README.md)
