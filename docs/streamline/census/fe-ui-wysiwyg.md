# Census: fe-ui-wysiwyg

Unit: `fe-ui-wysiwyg`  
Scope: `frontend/src/components/ui/wysiwyg/` (plugins/, transformers/, nodes/, lib/, context/)  
Entry point (not in scope but aggregates all): `frontend/src/components/ui/wysiwyg.tsx`

## Module Map

| File | Purpose | Public Surface | Relations | Notes |
|------|---------|---------------|-----------|-------|
| `context/task-attempt-context.tsx` | React contexts for task/attempt IDs and local image metadata; allows deep tree access without prop drilling | `TaskAttemptContext`, `TaskContext`, `LocalImagesContext`, `useTaskAttemptId()`, `useTaskId()`, `useLocalImages()`, `LocalImageMetadata` (type) | Consumed by `image-node.tsx` (hooks); provided in `wysiwyg.tsx` wrapper; `LocalImageMetadata` type imported by `TaskFormDialog`, `useCreateAttachments`, `useImageMetadata`, `useSessionAttachments`, `ChatBoxBase`, `CreateChatBox`, `SessionChatBox` | The three contexts are always provided together in the LexicalComposer tree |
| `lib/code-highlight-theme.ts` | Shared Prism token → Tailwind CSS-variable class mapping for code syntax highlighting | `CODE_HIGHLIGHT_CLASSES: Record<string, string>` | Consumed by `wysiwyg.tsx` (`theme.codeHighlight`); used indirectly by any InlineCodeNode rendering | Uses `var(--syntax-*)` CSS variables; must be in sync with global CSS |
| `lib/create-decorator-node.tsx` | Generic factory that generates a full Lexical `DecoratorNode` class + transformers from a config object; supports inline (regex) and fenced (``` language ```) serialization | `createDecoratorNode<T>()`, `InlineSerialization<T>`, `FencedSerialization<T>`, `SerializationConfig<T>`, `GeneratedDecoratorNode<T>` (interface), `GeneratedDecoratorNodeClass<T>` (type), `DecoratorNodeConfig<T>`, `DecoratorNodeResult<T>` | Used by `image-node.tsx` and `pr-comment-node.tsx`; no external consumers outside wysiwyg scope | Core abstraction; double-click reverts node to raw markdown text for editing |
| `nodes/image-node.tsx` | Lexical DecoratorNode for inline markdown images (`![alt](src)`); renders as a thumbnail pill with lazy metadata fetch; click opens preview dialog | `ImageNode`, `ImageNodeInstance` (type), `ImageData` (interface), `SerializedImageNode` (type), `$createImageNode(src, altText)`, `$isImageNode()`, `IMAGE_TRANSFORMER` | Builds on `createDecoratorNode`; uses `useTaskAttemptId`/`useTaskId`/`useLocalImages`; calls `useImageMetadata` hook; opens `ImagePreviewDialog`; `$isImageNode` consumed by `image-keyboard-plugin`; all exports consumed by `wysiwyg.tsx` | `.vibe-images/` path detection is an invisible feature — routes to proxy API |
| `nodes/pr-comment-node.tsx` | Lexical DecoratorNode for GitHub PR comments embedded as fenced code blocks (` ```gh-comment ``` `); renders as a `PrCommentCard` | `PrCommentNode`, `PrCommentNodeInstance` (type), `NormalizedComment` (interface), `SerializedPrCommentNode` (type), `$createPrCommentNode()`, `$isPrCommentNode()`, `PR_COMMENT_EXPORT_TRANSFORMER`, `PR_COMMENT_TRANSFORMER` | Builds on `createDecoratorNode`; renders `PrCommentCard`; `NormalizedComment` imported by `TaskFollowUpSection`; all node/transformer exports consumed by `wysiwyg.tsx` | Export transformer (TextMatch) must be registered before import transformer (MultilineElement) |
| `plugins/clickable-code-plugin.tsx` | In read-only mode, makes inline `code` spans clickable when their text matches a diff file path; uses MutationObserver for dynamic content | `ClickableCodePlugin` | Props: `findMatchingDiffPath`, `onCodeClick`; source provided by `ChangesViewContext`; active only when `disabled && findMatchingDiffPath && onCodeClick`; consumed by `ChatMarkdown.tsx` (new design) | G1-adjacent: enables navigation to a file in the Changes/Diff panel from chat messages |
| `plugins/code-block-shortcut-plugin.tsx` | Detects typed ``` ``` ``` pattern (open + close backticks) and converts intermediate paragraphs into a Lexical `CodeNode` | `CodeBlockShortcutPlugin` | Complementary to `CODE_BLOCK_TRANSFORMER` (paste); consumed by `wysiwyg.tsx` (edit mode only) | Typing path only; paste path handled by transformer |
| `plugins/code-highlight-plugin.tsx` | Thin wrapper around `@lexical/code`'s `registerCodeHighlighting` | `CodeHighlightPlugin` | Consumed by `wysiwyg.tsx` (always, read and edit) | Always active, not gated on edit mode |
| `plugins/file-tag-typeahead-plugin.tsx` | `@` typeahead in the editor: searches tags and files, inserts tag content or filename-as-code + appends full path at bottom | `FileTagTypeaheadPlugin` | Props: `workspaceId`, `projectId`; calls `searchTagsAndFiles`; uses `PortalContainerContext`; edit mode only | Unique behavior: file selection inserts name inline and appends full path as a separate paragraph |
| `plugins/image-keyboard-plugin.tsx` | Handles Backspace/Delete keys for selected `ImageNode`s | `ImageKeyboardPlugin` | Imports `$isImageNode`; edit mode only; consumed by `wysiwyg.tsx` | Needed because DecoratorNodes don't have default keyboard deletion |
| `plugins/keyboard-commands-plugin.tsx` | Wires Cmd+Enter (send), Shift+Cmd+Enter, and optionally plain Enter (when `sendOnEnter` pref is on) to callbacks; flushes markdown state synchronously before firing | `KeyboardCommandsPlugin` | Props: `onCmdEnter`, `onShiftCmdEnter`, `onChange`, `transformers`; reads `useUiPreferencesStore` for `sendOnEnter`; edit mode only; consumed by `wysiwyg.tsx` | `flushSync` ensures onChange has latest content before the send callback fires |
| `plugins/markdown-sync-plugin.tsx` | Bidirectional controlled sync: external `value` → editor (import) and editor changes → `onChange` (export); guards against infinite loops via `lastSerializedRef` | `MarkdownSyncPlugin` | Props: `value`, `onChange`, `onEditorStateChange`, `editable`, `transformers`; always active; consumed by `wysiwyg.tsx` | Central to controlled-component pattern; sets `editor.setEditable()` |
| `plugins/read-only-link-plugin.tsx` | In read-only mode, sanitizes all link hrefs: blocks `javascript:`, `vbscript:`, `data:` protocols; only HTTPS links are clickable; others rendered disabled | `ReadOnlyLinkPlugin` | Read-only mode only; uses `registerMutationListener` on `LinkNode`; consumed by `wysiwyg.tsx` | Security-critical; prevents XSS from markdown links |
| `plugins/toolbar-plugin.tsx` | Floating formatting toolbar (Bold/Italic/Underline/Strikethrough/Code) that appears above text selection in edit mode | `ToolbarPlugin` | Uses `PortalContainerContext`; edit mode only; consumed by `wysiwyg.tsx` | Portals into document body; positions relative to browser selection `Range.getBoundingClientRect()` |
| `transformers/code-block-transformer.ts` | Lexical `MultilineElementTransformer` for pasted markdown code fences (``` ``` ```); export serializes `CodeNode` back to markdown | `CODE_BLOCK_TRANSFORMER` | Consumed by `wysiwyg.tsx` in `extendedTransformers`; paste only (`isImport` guard) | Requires closing backticks; typing detection is in `CodeBlockShortcutPlugin` |
| `transformers/table-transformer.ts` | Lexical `ElementTransformer` for GFM-style markdown tables; handles import (row-by-row merge), header detection (separator row), and export | `TABLE_TRANSFORMER` | Consumed by `wysiwyg.tsx` in `extendedTransformers`; `@lexical/table` nodes as dependencies | Internal `$convertFromMarkdownString` call on each cell allows rich cell content |

## VS Code Bridge

`wysiwyg.tsx` imports `writeClipboardViaBridge` from `frontend/src/vscode/bridge.ts` to copy markdown to clipboard. The bridge detects the VS Code webview environment and uses `acquireVsCodeApi().postMessage` if available, falling back to the standard Clipboard API. This is an **invisible feature** — the copy button silently upgrades behavior inside VS Code without user-visible UI change.

## Invisible Features / Invisible Behaviors

1. **`.vibe-images/` proxy path** — `image-node.tsx` detects paths starting with `.vibe-images/` and routes them through `/api/images/{id}/file` proxy, enabling images uploaded to agent workspaces to render inline without exposing raw filesystem paths.

2. **VS Code clipboard bridge** — Copy button in read-only mode routes through `writeClipboardViaBridge` which can POST to the VS Code extension host when running inside a webview (G1-adjacent: VS Code webview bridge path).

3. **`gh-comment` fenced code block** — PR comments can be embedded as ` ```gh-comment\n{json}\n``` ` blocks in any markdown value; `pr-comment-node.tsx` deserializes and renders them as interactive cards. External source (`TaskFollowUpSection`) constructs `NormalizedComment` payloads and passes them to `$createPrCommentNode`.

4. **Double-click to edit DecoratorNodes** — Both `ImageNode` and `PrCommentNode` support double-clicking to revert the node to its raw markdown text (handled generically in `create-decorator-node.tsx`). This is not documented in props.

5. **`sendOnEnter` preference** — `KeyboardCommandsPlugin` reads the Zustand store `useUiPreferencesStore` to optionally treat plain Enter as send. This is controlled by a user preference, not a prop, making it an invisible behavioral variant.

## Candidates

| Path | Kind | Evidence | Disposition | Confidence | Blast Radius |
|------|------|----------|-------------|------------|--------------|
| `nodes/image-node.tsx` – `SerializedImageNode` type (line 23–29) | dead (type export) | Never imported outside the file itself; not referenced in any consumer | investigate | medium | Zero; type-only, tree-shaken |
| `nodes/image-node.tsx` – `ImageNodeInstance` type (line 216) | dead (type export) | Never imported outside the file | investigate | medium | Zero; type-only |
| `nodes/pr-comment-node.tsx` – `SerializedPrCommentNode` type (line 27–30) | dead (type export) | Never imported outside the file | investigate | medium | Zero; type-only |
| `nodes/pr-comment-node.tsx` – `PrCommentNodeInstance` type (line 90) | dead (type export) | Never imported outside the file | investigate | medium | Zero; type-only |
