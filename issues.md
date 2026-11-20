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
- Mitigation: F-13 shows the iOS add-to-home-screen hint. `loadDevice` asks for persistent storage once after it creates a key (`web/src/lib/identity.ts:246-249`); Safari grants it by heuristics such as being opened as a home screen web app ([WebKit blog](https://webkit.org/blog/14403/updates-to-storage-policy/)), and the answer is not checked.
- Status: open, accepted as a limitation of the platform.

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

### 2026-11-20 · deviation · Go identity and the web client answer differently until B-02b

- Symptom: for the same bytes, `internal/identity` and `web/src/lib/identity.ts` can give different answers. The web client follows the contract as settled in PR #58 and the coordinator's rules of 2026-11-20; the Go package follows the contract as it was. Known differences: weak keys as `k`, `dev`, `user`, pair `k` or signature R (web refuses, Go accepts); an empty `to` (web: a value, so `not_for_me`; Go: no `to`); signalling without `to` (web: `not_for_me`; Go accepts); a payload without `t`, `n` or `ts`, or with `null` in a field read (web: `bad_request`; Go: zero values); a key in another case, such as `TO` (web ignores it; Go reads it); an integer written `2.0` (web: 2; Go refuses) and one above 2^53 (web refuses; Go reads it); an `add` without `kind` `node` or `browser` or without a string `name` (web refuses; Go accepts); a `revoke` of the last valid device (web refuses; Go accepts); a long s in a pairing secret (web refuses; Go reads it as S); a pair message whose `k` is not a device id (web ignores it; Go checks only the mac); a signature whose S is not below L (both refuse, the web client before WebCrypto).
- Cause: the rules changed after B-02 merged; the Go package keeps the old behaviour (`internal/identity/key.go:87-91` signature check, `internal/identity/envelope.go:51-62` payload fields, `internal/identity/devlog.go:63-78` and `:93-104` log rules, `internal/identity/pairing.go:36-37` and `:66-68` secret and pair message). The web side: `web/src/lib/payload.ts`, `web/src/lib/identity.ts:125-135`, `:212-218`, `web/src/lib/devlog.ts:98-101`, `web/src/lib/pairing.ts:22-29`.
- Status: open until B-02b brings Go to the settled contract and adds the matching vectors to `internal/proto/testdata/crypto.json`; the web client then rebases and must pass them.

### 2026-11-20 · limitation · An empty `to` in a signed payload is read as no `to`

- Symptom: an envelope whose payload carries `"to":""` passes the `not_for_me` check on every receiver, although `contracts/envelope.md` section 3 says a `to` that is present must equal the receiver's device id.
- Cause: `proto.PayloadHeader.To` is a plain string (`internal/proto/envelope.go:17`), so `addressed` cannot tell an empty `to` from a missing one (`internal/identity/envelope.go:56`). Only a device the receiver already listens to can sign such a payload.
- Status: open.

### 2026-11-20 · limitation · Go and browser JSON parsers can disagree on a signed payload

- Symptom: not observed. A log entry written by hand by one of the person's own valid devices could be accepted by a node and refused by a browser, or the reverse, which leaves their logs at different versions. Examples: `"ver":2.0` (refused by Go, read as 2 by `JSON.parse`), a key spelled `"VER"` (matched by Go, ignored by a browser). The common fields that `OpenSignal` and `OpenCommand` read disagree the same way: for `{"to":"A","TO":"B"}` Go reads `to` as B and `JSON.parse` as A, and Go also matches Unicode case folds, so `"tſ"` (long s) sets `ts`. Go and `JSON.parse` both keep the last of two equal keys.
- Cause: `entry` reads the payload with `encoding/json` into `proto.DeviceEntry` (`internal/identity/devlog.go:68`), and `addressed` into `proto.PayloadHeader` (`internal/identity/envelope.go:53`); `encoding/json` matches keys without regard to case and refuses a fraction for an integer, and the contract has no canonical JSON. The shared vectors (`internal/proto/testdata/crypto.json`) hold no such payload.
- Status: open. Only the signer can write such a payload, and it gains no authority it does not already have; what it can do is make a Go receiver and a browser read its own payload differently. No software of this project writes one.

### 2026-11-20 · limitation · Go verifies signatures of small-order public keys that OpenSSL refuses

- Symptom: for the device id `AQAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA` (the identity point), the signature made of the identity point and a zero scalar verifies over any text in `crypto/ed25519`, so `VerifyChallenge` and `OpenCommand` (with a `known` that admits the id) accept it without any private key. The other small-order points verify such forgeries for part of all texts. Node 24 (OpenSSL) refuses the same signature, so a Go receiver and another Ed25519 implementation can disagree on whether an envelope or log entry verifies.
- Cause: `ed25519.Verify` (`internal/identity/key.go:90`) follows RFC 8032 without refusing small-order or non-canonical public keys, and `contracts/envelope.md` section 2 does not say whether such a key is a device id.
- Status: open, for the planning lead: whether a device id must not be a small-order point, so that both implementations refuse it. Such a key is trusted only after a valid device adds it or a secret holder pairs it, so nobody can use it against a device that is not theirs.

## Resolved

### 2026-11-20 · limitation · Browsers accept small-order Ed25519 public keys

- Symptom: `crypto.subtle.verify` returns true for the signature R = identity, S = 0 under the identity point as public key, over any message, in Chromium 159, Firefox 159 and WebKit 27.2 (spike S-7, 2026-11-20). WebCrypto requires false for small-order keys; web-platform-tests `eddsa_small_order_points` passes 8, 9 and 9 of 14 in Chrome 158, Firefox 160 and Safari 27.2 (wpt.fyi, 2026-11-19).
- Cause: the engines' Ed25519 libraries do not make the small-order checks ([Firefox bug 1929022](https://bugzilla.mozilla.org/show_bug.cgi?id=1929022), [WebKit bug 280673](https://bugs.webkit.org/show_bug.cgi?id=280673), both open).
- Effect: a device id that is a small-order point would verify forged signatures, if a device log or a pairing ever accepted one. The envelope contract does not reject such ids today.
- Status: resolved for the web client in F-01b: the settled contract (PR #58, `contracts/envelope.md` section 2) makes the fourteen weak values no device id, and the client compares the bytes itself before WebCrypto is asked, for `k`, `dev`, `user`, a pair message's `k` and the first half of a signature (`web/src/lib/identity.ts:103-135`). The Go side follows in B-02b (entry "Go verifies signatures of small-order public keys" below).

### 2026-11-20 · limitation · Web client does not start when the browser blocks site storage

- Symptom: expected blank page when the person has blocked site data for the page's origin. Checked under vitest with a `localStorage` that throws (the import of `prefs` rejects); not reproduced in a real browser.
- Cause: `prefs` reads `localStorage` while its module loads (`web/src/lib/prefs.svelte.ts:12-16` before the fix), and both the application and the upgrade notice import it. A browser that refuses storage access throws on `localStorage` (Chrome raises a `SecurityError`), so the import fails before `boot` mounts anything.
- Status: resolved in 815f0547d (PR #61). Storage is read and written behind a guard (`web/src/lib/prefs.svelte.ts:17-35`): a failed read, or a value the client does not know, gives the default; a failed write is ignored. Checked under vitest with a `localStorage` getter that throws a `SecurityError`; still not tried in a real browser.

### 2026-11-20 · bug · Go acceptance suite ran the web feature

- Symptom: `go test -tags acceptance -count=1 ./features/...` failed with `step is undefined` for seven scenarios of `features/web.feature` (for example `the current page is "群聊"`), so the merge gate failed.
- Cause: `features/acceptance_test.go:44` sets `Paths: []string{"."}`, so godog runs every `.feature` in `features/`, including `web.feature`, whose steps are implemented in `web/features/web.spec.ts` (vitest-cucumber) and not in Go; `Strict: true` fails the undefined steps. Each branch passed on its own (the F-00 branch had no `go.mod`, so Go acceptance was skipped; the B-00 and B-01 branches had no `web.feature`); the conflict appeared only after both were merged.
- Status: resolved in PR #59: `web.feature` is tagged `@web` and the godog run uses `Tags: "~@web"`.
