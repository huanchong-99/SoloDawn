# Census — Unit `fe-vscode-ide-focused`

Branch: `refactor/streamline-quality-gates` (worktree label `refactor/streamline-and-quality-gate-rules`)
Scope: `frontend/src/vscode/` (bridge.ts, ContextMenu.tsx) + `frontend/src/components/ide/` (IdeIcon.tsx, OpenInIdeButton.tsx)
Date: 2026-06-14

## TL;DR — two DIFFERENT features, do not conflate

| Feature | Files | Runtime status |
|---|---|---|
| **A. "Open in external IDE"** (spawn `code`/`cursor` on host, render IDE logo + button) | `components/ide/IdeIcon.tsx`, `components/ide/OpenInIdeButton.tsx` | **LIVE & wired** (Navbar, DiffViewCardWithComments, NextActionCard, ContextBar, CommandBar, actions registry). This is the G1 deletion target. |
| **B. "Whole app embedded in a VS Code webview iframe"** (keyboard/clipboard postMessage bridge) | `vscode/bridge.ts`, `vscode/ContextMenu.tsx` | **DORMANT** — no host, no webview build target, no `acquireVsCodeApi`, no host-side message handler exists in this repo. Only `wysiwyg.tsx` consumes one exported helper (`writeClipboardViaBridge`) which degrades gracefully outside an iframe. `ContextMenu.tsx` is fully ORPHANED (0 importers). |

The external `solodawn-vscode` extension is real but lives in a **separate repo**; it only calls the HTTP route `/containers/info` (see `crates/server/src/routes/containers.rs:74`). It does NOT embed this React app in a webview here — i.e. nothing in *this* repo activates the bridge.ts iframe protocol.

## Module map

| File | Purpose | Public surface | Relations | Notes |
|---|---|---|---|---|
| `frontend/src/vscode/bridge.ts` | Keyboard + clipboard bridge for when the app runs inside a VS Code webview iframe. Forwards keydown/keyup/keypress to `parent` via `postMessage`; handles Copy/Cut/Paste/Undo/Redo locally; bridges clipboard read/write when `navigator.clipboard` is restricted; handles parent→iframe `VIBE_ADD_TO_INPUT` (insert text at caret). **Auto-installs on import** (L487). | `inIframe()`, `parentClipboardWrite()`, `parentClipboardRead()`, `installVSCodeIframeKeyboardBridge()`, `writeClipboardViaBridge()`, `readClipboardViaBridge()` | Imported by: `components/ui/wysiwyg.tsx` (only `writeClipboardViaBridge`, L45/108); `vscode/ContextMenu.tsx` (`readClipboardViaBridge`+`writeClipboardViaBridge`). Uses `@/utils/id` `secureRandomIdFragment`. | iframe protocol strings (`vscode-iframe-*`, `VIBE_ADD_TO_INPUT`) appear ONLY here — no host/listener in-repo. Uses plain `globalThis.parent.postMessage`, NOT `acquireVsCodeApi` (corrects fe-ui-wysiwyg census claim). `VSCODE_PARENT_ORIGIN = location.origin` so postMessage targets same-origin parent. |
| `frontend/src/vscode/ContextMenu.tsx` | `WebviewContextMenu` React component: custom right-click menu (Copy/Cut/Paste/Undo/Redo/Select All) for the webview-iframe scenario, using `execCommand` legacy fallbacks + the bridge clipboard helpers. Only activates `if (inIframe())`. | `WebviewContextMenu: React.FC` | Imports `readClipboardViaBridge`/`writeClipboardViaBridge` from `@/vscode/bridge`. | **0 production importers** — grep across whole repo finds only its own definition + the R1 audit doc. Never mounted in App/layout. Orphan. |
| `frontend/src/components/ide/IdeIcon.tsx` | Renders the per-IDE logo SVG (vscode/cursor/windsurf/intellij/zed/xcode/antigravity) themed light/dark; `Code2` fallback for CUSTOM/none. `getIdeName()` maps `EditorType`→display name. | `IdeIcon({editorType,className})`, `getIdeName(editorType)` | Imports `EditorType`,`ThemeMode` from `shared/types`, `useTheme`. Consumed by: `OpenInIdeButton.tsx`, `NextActionCard.tsx:22/191/362`, `ui-new/primitives/CommandBar.tsx:15/23`, `ui-new/primitives/ContextBar.tsx:6/173`, `ui-new/actions/index.ts:56/646`. | Part of feature A (open-in-IDE). Heavily reused across both workspaces; not a whole-file orphan. Loads `/ide/*.svg` public assets. |
| `frontend/src/components/ide/OpenInIdeButton.tsx` | Ghost icon button "Open in <IDE>"; reads `config.editor.editorType`, renders `IdeIcon` + a11y label. Caller supplies `onClick`. | `OpenInIdeButton({onClick,disabled,className})` | Imports `IdeIcon`,`getIdeName` (sibling), `Button`, `useUserSystem`. Consumed by: `layout/Navbar.tsx:28/192` (`handleOpenInIDE`, gated by `isSingleRepoProject`), `ui-new/containers/DiffViewCardWithComments.tsx:31/433`. | Part of feature A. onClick chains: Navbar→`useOpenProjectInEditor`→`projectsApi.openEditor`→`POST /api/projects/{id}/open-editor`; DiffView→`useOpenInEditor`→`attemptsApi.openEditor`→`POST /api/task-attempts/{id}/open-editor`. |

## Invisible features

1. **VS Code webview iframe keyboard/clipboard bridge** (`bridge.ts`, auto-installs on import). Forwards keystrokes to a parent webview and bridges clipboard via postMessage. **Seems unused at runtime** — no in-repo host listens for `vscode-iframe-*`/`VIBE_ADD_TO_INPUT`, no webview build target, no `acquireVsCodeApi`. It is dormant infra for an embed scenario that this repo does not build. Only `writeClipboardViaBridge` is live (wysiwyg copy), and it works fine outside any iframe.
2. **`WebviewContextMenu`** — a complete custom context menu, but never mounted. Dead UI.
3. **`solodawn-vscode` external extension touchpoint** — out of this unit's files, but relevant: `crates/server/src/routes/containers.rs:74` + `crates/db/src/models/workspace.rs:477` are kept "for the VSCode extension". That extension consumes HTTP routes only; it is not the webview-iframe host and does not exercise bridge.ts.

## Keep/cut evidence map

| Item | Disposition | Confidence | Evidence |
|---|---|---|---|
| `ContextMenu.tsx` (whole file) | delete | high | 0 importers repo-wide (grep). Not mounted. Pure orphan. Blast radius: none. |
| `bridge.ts` iframe-only surface (`installVSCodeIframeKeyboardBridge`, key forwarding, `parentClipboard*`, `VIBE_ADD_TO_INPUT`, auto-install L487) | refactor (extract clipboard helper, drop iframe path) — investigate webview product intent first | medium | iframe protocol strings exist only in this file; no host in repo. BUT cannot 100% disprove an out-of-repo `solodawn-vscode` webview host depends on it → investigate, don't blind-delete. |
| `bridge.ts` clipboard helpers `writeClipboardViaBridge`/`readClipboardViaBridge` | keep (re-home into clipboard util if bridge deleted) | high | `wysiwyg.tsx:45/108` live import; wysiwyg is used by ~10 chat boxes in both workspaces. Deleting bridge.ts without re-homing breaks copy everywhere. |
| `IdeIcon.tsx` + `OpenInIdeButton.tsx` | delete (feature A / G1) — but cascades | high | Both implement the "open in external IDE" feature. 6+ live consumers (Navbar, DiffView, NextActionCard, ContextBar, CommandBar, actions registry). Blast radius is large: removing them requires the coordinated edits enumerated in `docs/audit/R1-ide-editor-connection-deletion-audit.md` §1-§9 (api.ts methods, backend routes, EditorType, ContextBar/CommandBar `ide-icon` branch, i18n). |

## Watch-list relevance
- (a) Open in external IDE [G1]: feature A files (IdeIcon, OpenInIdeButton) are the frontend leaf of it. Full deletion map already exists at `docs/audit/R1-ide-editor-connection-deletion-audit.md`.
- (b) VS Code webview bridge: feature B (bridge.ts, ContextMenu.tsx). Dormant; see above.
- (c) Quality Gate System A / (d) planning-draft + AuditPlan System B: NOT present in any file of this unit.

## Tool notes
fast-context (`mcp__fast-context__fast_context_search`) returned `resource_exhausted` on every attempt (3 retries incl. reduced tree_depth + exclude_paths). Fell back to Grep + Read for all cross-file usage tracing, per the tooling fallback rule.
