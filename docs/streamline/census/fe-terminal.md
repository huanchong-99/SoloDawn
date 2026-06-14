# fe-terminal Module Map

Unit: fe-terminal
Scope: frontend/src/components/terminal/ (4 files)

## File Summary Table

| File | Purpose | Public Surface | Relations | Notes |
|------|---------|----------------|-----------|-------|
| TerminalEmulator.tsx | xterm.js-based PTY terminal component with WebSocket I/O, auto-reconnect, keepalive, and pending-input buffer | `TerminalEmulator` (React component, forwardRef), `TerminalEmulatorRef` (interface: `write`, `clear`, `reconnect`) | Consumed by: `TerminalDebugView.tsx` (only internal caller); mocked in `WorkflowDebugPage.test.tsx` test. Imports: `@xterm/xterm`, `@xterm/addon-fit`, `@/types/websocket` (isWsOutputMessage, isWsErrorMessage) | Display options (fontSize, fontFamily, scrollback, cursorBlink) are compile-time constants with extension point documented in NOTE(E15-09). WS URL constructed as `${wsUrl}/terminal/${terminalId}` |
| TerminalDebugView.tsx | Multi-terminal debug UI: sidebar list + active emulator or history view, with auto-start/restart orchestration, terminal switching, and inline Quality Gate badge | `TerminalDebugView` (React component); private helpers `TerminalQualityBadgeInline`, `StatusDot` | Consumed by: `WorkflowDebugPage.tsx` (only production caller). Imports: `TerminalEmulator`, `QualityBadge`, `QualityReportPanel`, `useTerminalLatestQuality`, Dialog, Button, `stripAnsi` | Contains HTTP calls to `/api/terminals/{id}/start`, `/api/terminals/{id}/stop`, `/api/terminals/{id}/logs?limit=1000`. Quality Gate System A integration via `useTerminalLatestQuality` + `QualityBadge` + `QualityReportPanel` dialog. ANSI sanitization pipeline (stripAnsi -> stripControlCharacters) in terminal history view. Auto-start/restart with MAX_RESTART_ATTEMPTS=3. |
| TerminalEmulator.test.tsx | Vitest unit tests for `TerminalEmulator` component | n/a (test file) | Tests target: `TerminalEmulator.tsx`. Uses local `MockWebSocket` class, mocks xterm and FitAddon. | Tests cover: rendering, init, ref methods, WS connection/validation, keepalive, error handling, cleanup, race conditions. Some stubs are minimal (e.g. "should handle malformed WebSocket messages" only asserts onError is defined). |
| TerminalDebugView.test.tsx | Vitest integration tests for `TerminalDebugView` component | n/a (test file) | Tests target: `TerminalDebugView.tsx`. Mocks `TerminalEmulator`, xterm, FitAddon, `useQualityGate`. Uses `renderWithI18n`. | Covers: terminal selection, auto-start, restart, history loading, ANSI sanitization, waiting status, switching. |

## Key Observations

### Duplicate Name Issue
`frontend/src/components/debug/TerminalDebugView.tsx` is a separate component with the same export name (`TerminalDebugView`) but a completely different interface (`{ terminalId, terminals, onClose }`). It has zero production imports — only its own co-located test imports it. This is an orphaned legacy component (new design system, uses `text-low`/`bg-primary` design tokens from `tailwind.new.config.js`).

### Quality Gate System A Integration
`TerminalDebugView.tsx` integrates Quality Gate System A:
- `useTerminalLatestQuality(terminalId)` hook fetches gate status per terminal
- `QualityBadge` displays per-terminal gate status in the sidebar list and header
- `QualityReportPanel` opens in a Dialog on badge click

### In-flight relevance
- G1 ("open in external IDE/editor") — not present in these files.
- VS Code webview bridge — not present.
- Quality Gate System A — actively used via `useTerminalLatestQuality` + `QualityBadge` + `QualityReportPanel`.
- AuditPlan System B / planning-draft confirm->materialize — not present.

### WebSocket Protocol
TerminalEmulator sends typed JSON messages: `{ type: 'input', data }`, `{ type: 'resize', cols, rows }`, `{ type: 'heartbeat' }`. Receives `WsOutputMessage` and `WsErrorMessage` (type-guarded via `@/types/websocket`).

### API Calls in TerminalDebugView
- `POST /api/terminals/{id}/start` — auto-start / manual restart
- `POST /api/terminals/{id}/stop` — conflict recovery (409 handling)
- `GET /api/terminals/{id}/logs?limit=1000` — history load for completed/failed terminals
