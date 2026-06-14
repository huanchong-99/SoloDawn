# Census: fe-rjsf (frontend/src/components/rjsf/)

Module map for the React-jsonschema-form (RJSF) shadcn/ui theme layer.

## File Map

| File | Purpose | Public Surface | Relations | Notes |
|------|---------|---------------|-----------|-------|
| `index.ts` | Module barrel re-export | `shadcnTheme`, `customWidgets`, `customTemplates`, `customFields`; re-exports from `widgets/`, `templates/`, `fields/` | Consumed only by `ExecutorConfigForm.tsx` (imports `shadcnTheme`) | Individual named exports (`customWidgets` etc.) have 0 direct external consumers; only `shadcnTheme` is used |
| `theme.ts` | Assembles the RJSF theme registry | `shadcnTheme`, `customWidgets`, `customTemplates`, `customFields` | Imports all widgets, templates, fields; consumed by `index.ts` barrel | `textarea` alias for `TextareaWidget` registered at key `"textarea"` — no uiSchema in codebase sets `ui:widget: "textarea"`, so the alias is unused |
| `widgets/index.ts` | Barrel for widgets | Re-exports `TextWidget`, `SelectWidget`, `CheckboxWidget`, `TextareaWidget` | Imported by `theme.ts` | — |
| `widgets/TextWidget.tsx` | RJSF text input widget wrapping shadcn `Input` | `TextWidget` (React component, WidgetProps) | Registered in `customWidgets.TextWidget`; dispatched by RJSF for `"string"` schema fields | — |
| `widgets/SelectWidget.tsx` | RJSF select widget wrapping shadcn `Select` | `SelectWidget` (React component, WidgetProps) | Registered in `customWidgets.SelectWidget`; handles nullable enum via `__null__` sentinel | Non-trivial logic: null-type filtering, `__null__` sentinel value, i18n via `useTranslation` |
| `widgets/CheckboxWidget.tsx` | RJSF checkbox widget wrapping shadcn `Checkbox` | `CheckboxWidget` (React component, WidgetProps) | Registered in `customWidgets.CheckboxWidget` | — |
| `widgets/TextareaWidget.tsx` | RJSF textarea widget wrapping shadcn `Textarea` | `TextareaWidget` (React component, WidgetProps) | Registered as `customWidgets.TextareaWidget` AND `customWidgets.textarea` alias | Dynamic `rows` derived from `schema.title` containing "prompt" — implicit convention |
| `templates/index.ts` | Barrel for templates | Re-exports `ArrayFieldTemplate`, `ArrayFieldItemTemplate`, `FieldTemplate`, `ObjectFieldTemplate`, `FormTemplate` | Imported by `theme.ts` | — |
| `templates/ArrayFieldTemplate.tsx` | RJSF array field container + item templates | `ArrayFieldTemplate`, `ArrayFieldItemTemplate` (React components) | Registered in `customTemplates`; uses `buttonsProps.hasRemove`, `buttonsProps.onRemoveItem` from `@rjsf/utils` `ArrayFieldItemButtonsTemplateProps` | `item as unknown as { key? }` cast is a hacky but valid React element key access |
| `templates/FieldTemplate.tsx` | RJSF field wrapper: two-column label+input layout | `FieldTemplate` (React component, FieldTemplateProps) | Registered in `customTemplates.FieldTemplate`; renders all non-object fields | Object-type fields short-circuit to `return children` (no wrapper) |
| `templates/FormTemplate.tsx` | Minimal form container wrapper | `FormTemplate` (React component) | Registered in `customTemplates.FormTemplate` | Trivial — just `<div className="w-full">{children}</div>` |
| `templates/ObjectFieldTemplate.tsx` | RJSF object field container (divide-y layout) | `ObjectFieldTemplate` (React component, ObjectFieldTemplateProps) | Registered in `customTemplates.ObjectFieldTemplate` | Strips all RJSF title/description/add-property from objects |
| `fields/index.ts` | Barrel for custom fields | Re-exports `KeyValueField` | Imported by `theme.ts` | — |
| `fields/KeyValueField.tsx` | RJSF custom field for `Record<string,string>` env vars | `KeyValueField` (React component, FieldProps) | Registered in `customFields.KeyValueField`; activated via `uiSchema: { env: { "ui:field": "KeyValueField" } }` in `ExecutorConfigForm`; communicates up via `registry.formContext.onEnvChange` | Bypasses normal RJSF onChange — updates go through `formContext.onEnvChange` callback, not RJSF's own data path |

## Callers

- **Only external caller**: `frontend/src/components/ExecutorConfigForm.tsx` — imports `shadcnTheme` and registers all widgets/templates/fields on the `@rjsf/core` `Form`.
- `ExecutorConfigForm` is itself used in `frontend/src/pages/ui-new/settings/AgentSettingsNew.tsx` (executor config section).

## Candidates

| File | Kind | Evidence | Disposition |
|------|------|---------|-------------|
| `theme.ts` `textarea` alias | redundant | `customWidgets.textarea = TextareaWidget` but no uiSchema in codebase sets `"ui:widget": "textarea"` | investigate |
| `customWidgets`/`customTemplates`/`customFields` named exports | redundant | Only `shadcnTheme` (the composite) is imported externally; individual named exports from `index.ts` have no consumers | refactor (remove named exports from barrel or keep for future consumers) |
| `FormTemplate` | stub | Simply `<div className="w-full">{children}</div>`; provides no real customization | investigate (could be deleted with RJSF default) |
