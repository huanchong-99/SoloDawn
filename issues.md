# Issues

The issue record of this project: bugs, deviations from a requirement or from the owner's expectation, and known limitations, open and resolved. Read it before implementing or fixing anything. Add an entry for every such finding, not for every step; update the entry when it is resolved, and keep resolved entries.

Entry format: `### YYYY-MM-DD · type · title`, where type is `bug`, `deviation` (a feature does not do what was required or expected) or `limitation` (known, accepted for now). Then the symptom, the cause with its location (`file:line` where it is code), and the status (open, or resolved with the version or commit). Link a lesson, decision or postmortem instead of repeating it.

## Open

### 2026-11-20 · limitation · Web client can paint one frame in the wrong theme

- Symptom: when the theme chosen in the browser (`sd-theme` = `light` or `dark`) is the opposite of the system setting, the page can paint one frame in the system theme while it loads, then switches.
- Cause: `data-theme` is set on `<html>` by the module script after the bundle has loaded (`web/src/lib/prefs.svelte.ts:19-26`); until then `tokens.css` follows the system (`web/src/styles/tokens.css:6`). `web/index.html` has no inline script that sets it earlier.
- Status: open. No inline script for now, by the coordinator's decision of 2026-11-20 (one inline line is the usual fix; no gate measures it and a later Content-Security-Policy may forbid it).

### 2026-11-20 · limitation · Older browsers get an unstyled notice or a blank page

- Symptom: a browser without Ed25519 in WebCrypto sees the upgrade notice (C-9). If it also lacks `light-dark()` or CSS nesting, the notice shows as plain text with default colours; a browser too old to run the bundle shows a blank page.
- Cause: the notice is mounted by the same bundle (`web/src/lib/boot.ts:17-19`) and styled by the same tokens (`web/src/styles/tokens.css`, every colour a `light-dark()` pair) and nested rules (`web/src/styles/base.css`) as the application.
- Status: open, accepted by the coordinator on 2026-11-20 as a known limitation.

### 2026-11-20 · limitation · Web client does not start when the browser blocks site storage

- Symptom: expected blank page when the person has blocked site data for the page's origin. Checked under vitest with a `localStorage` that throws (the import of `prefs` rejects); not reproduced in a real browser.
- Cause: `prefs` reads `localStorage` while its module loads (`web/src/lib/prefs.svelte.ts:12-16`), and both the application and the upgrade notice import it. A browser that refuses storage access throws on `localStorage` (Chrome raises a `SecurityError`), so the import fails before `boot` mounts anything.
- Status: open. Reported in the review of PR #53; not changed there.

### 2026-11-20 · deviation · Web mock does not answer `host.info`

- Symptom: F-02 in `planning/steps/step-1-frontend.md` asks for a mock that answers `hello`, `sync`, `fetch` and `host.info`. `mockCtl` answers `hello`, `sync`, `fetch` and `cmd`, and `proto.ts` has no `host.info` type.
- Cause: `host.info` is in batch (c) of B-01, which has no Go type and no wire example yet (`web/src/lib/mock.ts:63-75`, `web/src/lib/proto.ts:372-377`).
- Status: open. Left out by the coordinator's decision of 2026-11-20; to be added once batch (c) is merged.

### 2026-11-20 · deviation · Web note rows refuse `pos` until the Go type has it

- Symptom: the settled row contract (PR #58) gives note rows a `pos`; `decode` refuses a note row that carries one.
- Cause: `NoteRow` mirrors the merged Go `NoteRow` and `internal/proto/testdata/row-note.json`, which have no `pos` (`web/src/lib/proto.ts:261-270`).
- Status: open. Changes with the backend change that adds the field to the Go type and the example.

## Resolved

### 2026-11-20 · bug · Go acceptance suite ran the web feature

- Symptom: `go test -tags acceptance -count=1 ./features/...` failed with `step is undefined` for seven scenarios of `features/web.feature` (for example `the current page is "群聊"`), so the merge gate failed.
- Cause: `features/acceptance_test.go:44` sets `Paths: []string{"."}`, so godog runs every `.feature` in `features/`, including `web.feature`, whose steps are implemented in `web/features/web.spec.ts` (vitest-cucumber) and not in Go; `Strict: true` fails the undefined steps. Each branch passed on its own (the F-00 branch had no `go.mod`, so Go acceptance was skipped; the B-00 and B-01 branches had no `web.feature`); the conflict appeared only after both were merged.
- Status: resolved in PR #59: `web.feature` is tagged `@web` and the godog run uses `Tags: "~@web"`.
