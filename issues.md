# Issues

The issue record of this project: bugs, deviations from a requirement or from the owner's expectation, and known limitations, open and resolved. Read it before implementing or fixing anything. Add an entry for every such finding, not for every step; update the entry when it is resolved, and keep resolved entries.

Entry format: `### YYYY-MM-DD · type · title`, where type is `bug`, `deviation` (a feature does not do what was required or expected) or `limitation` (known, accepted for now). Then the symptom, the cause with its location (`file:line` where it is code), and the status (open, or resolved with the version or commit). Link a lesson, decision or postmortem instead of repeating it.

## Open

### 2026-11-20 · limitation · Web client can paint one frame in the wrong theme

- Symptom: when the theme chosen in the browser (`sd-theme` = `light` or `dark`) is the opposite of the system setting, the page can paint one frame in the system theme while it loads, then switches.
- Cause: `data-theme` is set on `<html>` by the module script after the bundle has loaded (`web/src/lib/prefs.svelte.ts:40-47`); until then `tokens.css` follows the system (`web/src/styles/tokens.css:6`). `web/index.html` has no inline script that sets it earlier.
- Status: open. No inline script for now, by the coordinator's decision of 2026-11-20 (one inline line is the usual fix; no gate measures it and a later Content-Security-Policy may forbid it).

### 2026-11-20 · limitation · Older browsers get an unstyled notice or a blank page

- Symptom: a browser below the supported versions (Chrome and Edge 137, Firefox 136, Safari 17.5; [decision](docs/decisions/implemented/architecture/2026-11-20-browser-baseline.md)) sees the upgrade notice. Chrome and Edge 123 to 136 and Firefox 120 to 135 show it styled. A browser without `light-dark()` (Chrome below 123, Firefox below 120, Safari below 17.5) shows it with default colours, and one without nested rules that start with a type selector (Chrome below 120, Firefox below 117, Safari below 17.2) also loses the layout those rules give; a browser that cannot run the bundle shows a blank page.
- Cause: the notice is mounted by the same bundle (`web/src/lib/boot.ts:32-34`) and styled by the same tokens (`web/src/styles/tokens.css`, every colour a `light-dark()` pair) and nested rules (`web/src/styles/base.css`) as the application, and the build targets the supported versions (`web/vite.config.ts:21-25`), so the CSS ships as written.
- History: until the baseline was set, `web/vite.config.ts` had no `build.target`, so Vite 8.3.8's default (`chrome111`, `edge111`, `firefox114`, `safari16.4`, `ios16.4`) applied, and Lightning CSS, which minifies the CSS for the build target, rewrote every `light-dark()` pair into two custom properties and flattened the nesting. The built notice was then styled down to those versions; only the development server served the CSS as written, which is what the first version of this entry described.
- Status: open, accepted by the coordinator on 2026-11-20 as a known limitation; restated the same day when the coordinator chose to ship the CSS as written.

### 2026-11-20 · limitation · Safari deletes the device key after seven days without a visit

- Symptom: in Safari on macOS and iOS, when the person does not interact with the SoloDawn page during seven days of Safari use, Safari deletes the origin's IndexedDB, and with it the device key once the identity module keeps it there (F-01b). The browser then starts as a new device and has to be paired again. A web app added to the home screen keeps its own count and is not affected; Lockdown Mode disables IndexedDB altogether.
- Cause: WebKit's tracking prevention deletes all script-writable storage of a site after seven days of browser use without interaction with it ([WebKit blog](https://webkit.org/blog/10218/full-third-party-cookie-blocking-and-more/)). Not a SoloDawn defect; [decision](docs/decisions/implemented/architecture/2026-11-20-browser-baseline.md), Consequences.
- Mitigation: F-13 shows the iOS add-to-home-screen hint; F-01b can request `navigator.storage.persist()`, which Safari grants by heuristics such as being opened as a home screen web app ([WebKit blog](https://webkit.org/blog/14403/updates-to-storage-policy/)).
- Status: open, accepted as a limitation of the platform.

### 2026-11-20 · limitation · Browsers accept small-order Ed25519 public keys

- Symptom: `crypto.subtle.verify` returns true for the signature R = identity, S = 0 under the identity point as public key, over any message, in Chromium 159, Firefox 159 and WebKit 27.2 (spike S-7, 2026-11-20). WebCrypto requires false for small-order keys; web-platform-tests `eddsa_small_order_points` passes 8, 9 and 9 of 14 in Chrome 158, Firefox 160 and Safari 27.2 (wpt.fyi, 2026-11-19).
- Cause: the engines' Ed25519 libraries do not make the small-order checks ([Firefox bug 1929022](https://bugzilla.mozilla.org/show_bug.cgi?id=1929022), [WebKit bug 280673](https://bugs.webkit.org/show_bug.cgi?id=280673), both open).
- Effect: a device id that is a small-order point would verify forged signatures, if a device log or a pairing ever accepted one. The envelope contract does not reject such ids today.
- Status: open. Rejecting them needs a contract change that the Go node and the browser apply alike; referred to the planning lead on 2026-11-20.

### 2026-11-20 · limitation · Safari's verify() fails every signature over an empty message

- Symptom: WebKit 27.2 (Playwright build, macOS) returns false from `crypto.subtle.verify` for a valid Ed25519 signature over an empty message, RFC 8032 test 1 included; its own signature of the empty message is valid (OpenSSL verifies it). Chromium 159 and Firefox 159 verify both.
- Cause: in WebKit's `Source/WebCore/PAL/pal/crypto/CryptoKitShim.swift`, `isValidSignature` returns false when the data is empty. No bug report found; the first affected Safari version is unknown.
- Effect: none on the protocol, whose signed bytes always start with `solodawn/1\n`; `boot` signs a non-empty message (`web/src/lib/boot.ts:8-9`). Browser tests must not verify an empty message, such as RFC 8032 test 1, in WebKit.
- Status: open, external.

### 2026-11-20 · deviation · Web mock does not answer `host.info` or push `groups`

- Symptom: F-02 in `planning/steps/step-1-frontend.md` asks for a mock that answers `hello`, `sync`, `fetch` and `host.info`; the settled plan (PR #58) adds that it pushes `groups`. `mockCtl` answers `hello`, `sync`, `fetch` and `cmd`, and pushes only `reset` and `rows`. `proto.ts` has no `host.info` type; it has a `Groups` type, written from the contract.
- Cause: neither frame has a Go type or a wire example yet, and the mock answers from wire examples only (`web/src/lib/mock.ts:63-75`, `web/src/lib/proto.ts:94-96`, `web/src/lib/proto.ts:372-377`). Both are frozen by backend task B-10 (`planning/steps/step-1-backend.md`; PR #58, `contracts/conventions.md` section 5), not by a slice of B-01.
- Status: open. `host.info` was left out by the coordinator's decision of 2026-11-20, until it has an example; the `groups` push was found in the review of PR #61 and not changed there.

### 2026-11-20 · deviation · Web note rows refuse `pos` until the Go type has it

- Symptom: the settled row contract (PR #58) gives note rows a `pos`; `decode` refuses a note row that carries one.
- Cause: `NoteRow` mirrors the merged Go `NoteRow` and `internal/proto/testdata/row-note.json`, which have no `pos` (`web/src/lib/proto.ts:261-270`).
- Status: open. Changes with the backend change that adds the field to the Go type and the example.

## Resolved

### 2026-11-20 · limitation · Web client does not start when the browser blocks site storage

- Symptom: expected blank page when the person has blocked site data for the page's origin. Checked under vitest with a `localStorage` that throws (the import of `prefs` rejects); not reproduced in a real browser.
- Cause: `prefs` reads `localStorage` while its module loads (`web/src/lib/prefs.svelte.ts:12-16` before the fix), and both the application and the upgrade notice import it. A browser that refuses storage access throws on `localStorage` (Chrome raises a `SecurityError`), so the import fails before `boot` mounts anything.
- Status: resolved in 815f0547d (PR #61). Storage is read and written behind a guard (`web/src/lib/prefs.svelte.ts:17-35`): a failed read, or a value the client does not know, gives the default; a failed write is ignored. Checked under vitest with a `localStorage` getter that throws a `SecurityError`; still not tried in a real browser.

### 2026-11-20 · bug · Go acceptance suite ran the web feature

- Symptom: `go test -tags acceptance -count=1 ./features/...` failed with `step is undefined` for seven scenarios of `features/web.feature` (for example `the current page is "群聊"`), so the merge gate failed.
- Cause: `features/acceptance_test.go:44` sets `Paths: []string{"."}`, so godog runs every `.feature` in `features/`, including `web.feature`, whose steps are implemented in `web/features/web.spec.ts` (vitest-cucumber) and not in Go; `Strict: true` fails the undefined steps. Each branch passed on its own (the F-00 branch had no `go.mod`, so Go acceptance was skipped; the B-00 and B-01 branches had no `web.feature`); the conflict appeared only after both were merged.
- Status: resolved in PR #59: `web.feature` is tagged `@web` and the godog run uses `Tags: "~@web"`.
