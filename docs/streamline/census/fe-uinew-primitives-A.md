# Census: frontend/src/components/ui-new/primitives/ — First Half (A–R)

Unit: fe-uinew-primitives-A  
Branch: refactor/streamline-quality-gates  
Date: 2026-06-14

## Scope

Files 1–20 of 40 (alphabetical first half):  
Button, Card, ChatBoxBase, Command, CommandBar, CommentCard, ContextBar, CreateChatBox, Dialog, Dropdown, ErrorAlert, Field, IconButton, IconButtonGroup, IconListItem, Label, PlanningChat, PrimaryButton, ProcessListItem, RecentReposList

---

## Module Map

| File | Purpose | Public Surface | Relations | Notes |
|---|---|---|---|---|
| `Button.tsx` | Generic CVA-styled button with 7 variants (primary/secondary/outline/ghost/destructive/glass/link) and 5 sizes. Supports asChild via Radix Slot. | `Button`, `ButtonProps`, `buttonVariants` | Imported by: McpSettingsNew, AgentSettingsNew, OrganizationSettingsNew, ProjectSettingsNew, WorkflowSidebar | Core design-system primitive; widely used |
| `Card.tsx` | Radix-style layout card with header/title/description/content/footer sub-components. | `Card`, `CardHeader`, `CardTitle`, `CardDescription`, `CardContent`, `CardFooter` | **Zero production importers found** — no file imports from `ui-new/primitives/Card`. The settings pages all use `SettingsCard.tsx` (separate file). | CANDIDATE: dead — appears to be a shadcn/Radix boilerplate leftover never wired into the new design |
| `ChatBoxBase.tsx` | Shared chat input layout shell — WYSIWYG editor, header/footer slots, banner, running border animation, variant dropdown. Houses `VisualVariant` enum (NORMAL/FEEDBACK/EDIT/PLAN). | `ChatBoxBase`, `EditorProps`, `VariantProps`, `VisualVariant` | Imported by CreateChatBox, SessionChatBox, PlanningChat (all within primitives). Uses WYSIWYGEditor, Toolbar, DropdownMenuItem. | Base layer for all chat inputs; PLAN variant feeds PlanningChat |
| `Command.tsx` | cmdk-based command palette primitives (Command, CommandInput, CommandList, CommandEmpty, CommandGroup, CommandItem, CommandSeparator, CommandShortcut, CommandDialog). | 9 exports: `Command`, `CommandDialog`, `CommandInput`, `CommandList`, `CommandEmpty`, `CommandGroup`, `CommandItem`, `CommandShortcut`, `CommandSeparator` | CommandBar.tsx imports all; CommandBarDialog.tsx imports `CommandDialog` | Thin styled wrapper over cmdk library |
| `CommandBar.tsx` | Rendered command palette UI — groups, pages, back navigation, action items with IdeIcon/CopyIcon special handling. Dispatches `onSelect(item)`. | `CommandBar` | Imported by CommandBarDialog.tsx | Contains `ide-icon` special case — G1 deletion candidate (the `ActionItemIcon` branch for `ide-icon`) |
| `CommentCard.tsx` | Shared presentational card for diff-view comments (user/github/input variants). | `CommentCard`, `CommentCardVariant` | Imported by ReviewCommentRenderer, GitHubCommentRenderer, CommentWidgetLine, PrCommentsDialog, ui/pr-comment-card.tsx | Active; used in 4+ production containers |
| `ContextBar.tsx` | Floating draggable sidebar with primary/secondary action icon groups. Handles `ide-icon` and `copy-icon` special icon dispatch. Uses `useContextBarPosition` for drag. | `ContextBar`, `ContextBarProps` | Imported by ContextBarContainer; ContextBarContainer mounted in WorkspacesMain | Contains two G1 concerns: (1) `ide-icon` branch renders `IdeIcon` for `open-in-ide` action; (2) `toggle-dev-server` spin/error state |
| `CreateChatBox.tsx` | Create-mode chat box with executor/model dropdowns, file attach, agent icon, save-as-default checkbox. | `CreateChatBox`, `ExecutorProps`, `ModelConfigProps`, `SaveAsDefaultProps` | Imported by CreateChatBoxContainer; mounted in WorkspacesLayout | Stateless; delegates to ChatBoxBase |
| `Dialog.tsx` | Radix Dialog primitives styled for new design system. Reads `PortalContainerContext` for portal target. | `Dialog`, `DialogPortal`, `DialogOverlay`, `DialogTrigger`, `DialogClose`, `DialogContent`, `DialogHeader`, `DialogFooter`, `DialogTitle`, `DialogDescription` | Imported by Command.tsx (CommandDialog), Step3Models workflow step | Core modal primitive |
| `Dropdown.tsx` | Radix DropdownMenu styled components with SearchInput, TriggerButton, SubContent etc. | `DropdownMenu`, `DropdownMenuTrigger`, `DropdownMenuTriggerButton`, `DropdownMenuContent`, `DropdownMenuItem`, `DropdownMenuCheckboxItem`, `DropdownMenuRadioItem`, `DropdownMenuLabel`, `DropdownMenuSeparator`, `DropdownMenuShortcut`, `DropdownMenuGroup`, `DropdownMenuPortal`, `DropdownMenuSub`, `DropdownMenuSubContent`, `DropdownMenuSubTrigger`, `DropdownMenuRadioGroup`, `DropdownMenuSearchInput` | Imported by Toolbar, SearchableDropdown, RepoCardSimple, ProjectSelectorContainer, RepoCard, AgentSettingsNew, CreateChatBox | `DropdownMenuSubContent`, `DropdownMenuSubTrigger`, `DropdownMenuCheckboxItem`, `DropdownMenuRadioItem`, `DropdownMenuRadioGroup`, `DropdownMenuShortcut` appear only self-defined in this file — no external callers for those specific sub-exports |
| `ErrorAlert.tsx` | Simple error alert box with multiline support. | `ErrorAlert` | Imported by McpSettingsNew, FeishuSettingsNew, AgentSettingsNew, ReposSettingsNew, ProjectSettingsNew, OrganizationSettingsNew, Step4Terminals, GitPanel | Widely used |
| `Field.tsx` | Form field primitives: Field, FieldLabel, FieldGroup, FieldSet, FieldLegend, FieldContent, FieldTitle, FieldDescription, FieldSeparator, FieldError. Uses CVA for orientation variants. | `Field`, `FieldLabel`, `FieldDescription`, `FieldError`, `FieldGroup`, `FieldLegend`, `FieldSeparator`, `FieldSet`, `FieldContent`, `FieldTitle` | Only `Field`, `FieldLabel`, `FieldError` imported externally (workflow steps Step0–Step6). The other 6 exports (`FieldSet`, `FieldLegend`, `FieldContent`, `FieldTitle`, `FieldDescription`, `FieldSeparator`) have no external callers. | CANDIDATE: FieldSet/FieldLegend/FieldContent/FieldTitle/FieldDescription/FieldSeparator are dead exports — shadcn boilerplate never wired |
| `IconButton.tsx` | Small icon-only button with default/tertiary variants and disabled state. | `IconButton` | Imported by ProjectSettingsNew, Step3Models | Simple utility |
| `IconButtonGroup.tsx` | Horizontal icon button group with connected border, supporting active/disabled/tooltip states per item. | `IconButtonGroup`, `IconButtonGroupItem` | Imported by PreviewBrowser.tsx | Used for preview browser screen-size toggle buttons |
| `IconListItem.tsx` | Clickable or static list item with icon, label, loading spinner, disabled state. | `IconListItem` | Imported by BrowseRepoButtonContainer, CreateRepoButtonContainer, RecentReposList | Simple list row |
| `Label.tsx` | Thin Radix Label wrapper with CVA variant. | `Label` | Imported by Field.tsx (FieldLabel), McpSettingsNew, AgentSettingsNew | Core form primitive |
| `PlanningChat.tsx` | Presentational planning chat UI — multi-turn message thread + status badge + confirm/materialize action buttons. Maps draft.status to button states. | `PlanningChat` | Imported by PlanningChatContainer only | Planning draft confirm→materialize flow (System B AuditPlan concern) |
| `PrimaryButton.tsx` | Brand-colored action button with optional leading icon/spinner. 3 variants (default/secondary/tertiary). | `PrimaryButton` | Imported by CreateChatBox, SessionChatBox, PlanningChat, FeishuSettingsNew, Step0Project | Submit/CTA button |
| `ProcessListItem.tsx` | Sidebar list row for execution processes (codingagent/setupscript/cleanupscript/devserver/qualityscan). Shows running dots or status dot. | `ProcessListItem` | Imported by ProcessListContainer; container used in RightSidebar | `qualityscan` run reason present — directly visible Quality Gate System A concern |
| `RecentReposList.tsx` | Display list for recently used repos with loading/error/empty states. | `RecentReposList`, `RecentRepoEntry` | Imported by RecentReposListContainer; container used in GitPanelCreate.tsx | Purely presentational |

---

## Candidates Summary

| # | File | Kind | Why | Hint |
|---|---|---|---|---|
| 1 | `Card.tsx` | dead | No production importer found outside the file itself | delete |
| 2 | `CommandBar.tsx` lines 21–31 | legacy | `ide-icon` branch in `ActionItemIcon` renders `IdeIcon` — part of G1 open-in-IDE deletion surface | refactor (remove branch) |
| 3 | `ContextBar.tsx` lines 163–180 | legacy | `ide-icon` special case renders `IdeIcon` for `open-in-ide` action — G1 deletion surface | refactor (remove branch) |
| 4 | `Field.tsx` — FieldSet, FieldLegend, FieldContent, FieldTitle, FieldDescription, FieldSeparator | dead | 6 exported sub-components with no callers outside Field.tsx itself | delete (trim exports) |
| 5 | `Dropdown.tsx` — DropdownMenuSubContent/SubTrigger/CheckboxItem/RadioItem/RadioGroup/Shortcut | redundant | These sub-exports are not imported by any production file (only the legacy ui/dropdown-menu.tsx which is the old design system) | investigate |

---

## Invisible Features

- **PlanningChat / PlanningChatContainer**: planning-draft confirm→materialize flow. `PlanningChatContainer` calls `planningDraftsApi.create`, `sendMessage`, `confirmDraft`, `materializeDraft`. Currently only reachable if `PlanningChatContainer` is mounted (check if it's actually in a page route).
- **ProcessListItem `qualityscan` run reason**: hardcoded label "Quality Scan" maps to `qualityscan` `ExecutionProcessRunReason` — this is the Quality Gate System A process type visible in the right sidebar.
- **ContextBar drag handle**: `useContextBarPosition` hook drives a draggable floating bar — position persisted via store (`useUiPreferencesStore`).
- **ChatBoxBase `isRunning` animated border**: CSS class `chat-box-running` adds animated border when workspace is active.
- **`VisualVariant.PLAN`** in `ChatBoxBase`: triggers brand-colored border in planning mode — bridges into the planning-draft confirm flow.
