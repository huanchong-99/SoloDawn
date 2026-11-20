# Issues

The issue record of this project: bugs, deviations from a requirement or from the owner's expectation, and known limitations, open and resolved. Read it before implementing or fixing anything. Add an entry for every such finding, not for every step; update the entry when it is resolved, and keep resolved entries.

Entry format: `### YYYY-MM-DD · type · title`, where type is `bug`, `deviation` (a feature does not do what was required or expected) or `limitation` (known, accepted for now). Then the symptom, the cause with its location (`file:line` where it is code), and the status (open, or resolved with the version or commit). Link a lesson, decision or postmortem instead of repeating it.

## Open

### 2026-11-20 · limitation · Fake CLIs lack invalid-login and exhausted-limit captures

- Symptom: neither scripted CLI covers an invalid-login startup error or an exhausted rate limit.
- Cause: the [macOS driver spike](https://github.com/huanchong-99/SoloDawn/pull/57) captured signed-in sessions and rate-limit updates, not these failures.
- Status: open. Add scripts from real captures before B-14 handles either failure.

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
- Mitigation: F-13 shows the iOS add-to-home-screen hint. F-04 asks for persistent storage (`navigator.storage.persist()`) once the device is enrolled, after pairing; Safari grants it by heuristics such as being opened as a home screen web app ([WebKit blog](https://webkit.org/blog/14403/updates-to-storage-policy/)). `loadDevice` does not ask on first run, because Firefox answers the call with a permission prompt (`dom/quota/StorageManager.cpp`, `PersistentStoragePermissionRequest::Start`, asks through `nsContentPermissionUtils::AskPermission`; MDN, [Storage quotas and eviction criteria](https://developer.mozilla.org/en-US/docs/Web/API/Storage_API/Storage_quotas_and_eviction_criteria)).
- Status: open, accepted as a limitation of the platform.

### 2026-11-20 · limitation · Safari's verify() fails every signature over an empty message

- Symptom: WebKit 27.2 (Playwright build, macOS) returns false from `crypto.subtle.verify` for a valid Ed25519 signature over an empty message, RFC 8032 test 1 included; its own signature of the empty message is valid (OpenSSL verifies it). Chromium 159 and Firefox 159 verify both.
- Cause: in WebKit's `Source/WebCore/PAL/pal/crypto/CryptoKitShim.swift`, `isValidSignature` returns false when the data is empty. No bug report found; the first affected Safari version is unknown.
- Effect: none on the protocol; envelopes and challenges have non-empty context prefixes. F-01c asserts WebKit's false result on RFC 8032 test 1 explicitly, separately from the protocol vectors.
- Status: open, external.

### 2026-11-20 · deviation · Web mock does not answer `host.info` or push `groups`

- Symptom: F-02 in `planning/steps/step-1-frontend.md` asks for a mock that answers `hello`, `sync`, `fetch` and `host.info`; the settled plan (PR #58) adds that it pushes `groups`. `mockCtl` answers `hello`, `sync`, `fetch` and `cmd`, and pushes only `reset` and `rows`. `proto.ts` has no `host.info` type; it has a `Groups` type, written from the contract.
- Cause: neither frame has a Go type or a wire example yet, and the mock answers from wire examples only (`web/src/lib/mock.ts:63-75`, `web/src/lib/proto.ts:94-96`, `web/src/lib/proto.ts:372-377`). Both are frozen by backend task B-10 (`planning/steps/step-1-backend.md`; PR #58, `contracts/conventions.md` section 5), not by a slice of B-01.
- Status: open. `host.info` was left out by the coordinator's decision of 2026-11-20, until it has an example; the `groups` push was found in the review of PR #61 and not changed there.

### 2026-11-20 · limitation · Random ids are checked in `n` and `g` of a signed payload only

- Symptom: `contracts/conventions.md` section 2 makes any text that is no random id a wrong type in every field that holds one. `proto.Decode` and `decode` check it for `n` and `g` of a signed payload, which is what the envelope check reads (`contracts/envelope.md` section 3, check 3). A row, a frame or a command field that holds an id (`g` of `sync` or of a row, `thread`, `re`, `task`, `mentions`, a row's own `id`) still takes any text.
- Cause: only those two fields have the type (`internal/proto/envelope.go:12-23`, `web/src/lib/proto.ts:17-22`). The contracts type `n` and `g` as ids in a field table; for the other fields `contracts/rows.md` says "random ids unless stated" without saying per field which values stand beside the id (the `by` of a message row may be `"system"`, a device row's `id` is `<user id>:<ver>`).
- Status: open. A field gets the type (`proto.ID` in Go, `Id` in `proto.ts`) in the task that first reads it from the wire.

### 2026-11-20 · limitation · A message cut between fragments is forwarded as if it were complete

- Symptom: when a client sends a WebSocket message in several fragments and its transport closes between two of them, the rendezvous reads the fragments received so far as one complete message. If they happen to form a valid frame, it is handled like any other: a `to` is forwarded as `from`. `planning/contracts/rendezvous.md` section 2 says that a frame the server cannot read closes its sender.
- Cause: `github.com/coder/websocket` v1.8.15, `read.go:447-449`: `msgReader.Read` takes any `io.EOF` for the end of the message, and the header of the next fragment, read by `readFrameHeader` (`frame.go:52-58`), fails with a plain `io.EOF` when the transport has closed. `Conn.Read` then returns the bytes so far without an error, to `internal/rendezvous/server.go:128`.
- Why it is harmless: what is forwarded is a complete, valid frame that the same authenticated sender could have sent on purpose; the sender's connection is already gone; and the receiver checks the signed envelope itself (`planning/contracts/envelope.md` section 3).
- Status: open, accepted by the coordinator on 2026-11-20 with no code change. Revisit if the library changes, or if the server ever acts on a frame beyond forwarding it.

### 2026-11-20 · limitation · A late `offline` can retire a newer unanswered offer

- Symptom: a node that offers again soon after an offer to an absent device can retire the new offer at once, as if every path had reported the target offline, and wait out a back-off before the next one.
- Cause: the rendezvous answers each `to` it cannot deliver with `{"t":"offline","k":…}`, which names the target but not the `conn` (`planning/contracts/rendezvous.md` section 2.2). `internal/peer/path.go:140-145` passes every such report to `Peer.Offline`, which takes it for each unanswered offer to that target and removes the reporting path (`internal/peer/unanswered.go:38-43`). A report for an earlier frame (the earlier offer or one of its candidates) still on its way when the next offer leaves therefore counts against the new one. The back-off of at least one second makes this rare; a presence report can end the back-off sooner (`planning/flows/connect.md` step 16).
- Status: open, accepted by the coordinator on 2026-11-20 with no contract change.

## Resolved

### 2026-11-20 · deviation · Two host check tests skipped themselves on Windows

- Symptom: `TestChecksRequireBothGitDiffReads` and `TestCheckCancellationStopsForegroundPipeline` in `internal/host/checks_test.go` called `t.Skip` on Windows. `t.Skip(` is a listed directive under `[tamper]` in `.project-standard/gates.toml` with no exception, yet both merged with HS-5 (#94).
- Cause: both tests need a POSIX shell (a fake `git` written as a shell script; a pipeline with a FIFO), and the author guarded them with a runtime skip. Neither CI nor the reviews of #94 ran the directive check.
- Status: resolved. Both tests and their FIFO helper moved to `internal/host/checks_unix_test.go` under `//go:build !windows`, so they are not compiled where they cannot run; no listed directive remains in `cmd/`, `internal/` or `features/`.

### 2026-11-20 · bug · Wire types read seven things differently from the contracts

- Symptom: an audit of the merged wire layer found (1) `proto.ts` typed some 50 integer fields (`seq`, `re`, `id`, every time and count outside a message row) as any number, so the browser read `"seq":805.5` where `proto.Decode` refuses it; (2) `decode(Frame, …)` read a `welcome`, `synced` or `fetched` answer that lacks a field as a `cmd` answer `{re, ok}` instead of refusing it; (3) neither language had the optional `full` of an `approval` body (`contracts/messages.md` 2.3, added in PR #71 after the B-01d freeze), so it was dropped; (4) Go's `MessageRow[ApprovalBody, …]` took `"state":null` for a card, and `MessageRow[EmptyBody, StatusOnly]` took a state for a text message, both of which `proto.ts` refuses (`contracts/rows.md` section 5); (5) `testdata/welcome.json` gave the node version as `0.3.1`, where `contracts/local.md` section 9 and the `welcome` example (PR #112) make it the release tag, `v0.3.1`; (6) Go's `Success` took `"ok":false`, which `proto.ts` refuses (`contracts/conventions.md` section 3); (7) the web read a number beyond the largest double in a fraction field (`"window_pct":1e400`) as `Infinity`, which `proto.Decode` refuses.
- Cause: (1) `num` in place of `int` (`web/src/lib/proto.ts`); (2) the `Frame` union tried `CmdResult`, which has no `t` and drops unknown fields, after the typed answers; (3) the structs were frozen before the field existed; (4) `MessageRow` held its state as `*S` for every kind; (5) the example predates PR #112; (6) `Success.OK` was a plain `bool`; (7) `v.number()` accepts `Infinity`.
- Status: resolved in branch `fix/audit-proto`: integers are `int` except the four `usage` fractions (`web/src/lib/proto.ts:10-12`); the `cmd` answer in `Frame` has no `t` (`web/src/lib/proto.ts:562`); `full` is in both types and the approval examples (`internal/proto/messages.go:41`, `web/src/lib/proto.ts:317`); a card's state is a struct and a non-card's is `NoCard`, which is `null` only (`internal/proto/rows.go:98-115`, `internal/proto/messages.go:198-207`); the `welcome` example has `v0.3.1`; `Success.OK` has the type `True`, which takes `true` only (`internal/proto/session.go`); every number in `proto.ts` is finite.

### 2026-11-20 · bug · The language button was not named by its visible text

- Symptom: the top-bar language button shows "EN" (or "中") but was named "Switch language / 切换语言" by its `aria-label`, so a speech-input user saying "click EN" reached nothing (WCAG 2.2 SC 2.5.3), and the half of the name in the other language had no language of its own (SC 3.1.2). Found in the audit of the web shell.
- Cause: `aria-label={t('bar.lang')}` replaced the button's content as its name (`web/src/App.svelte:49` before the fix).
- Status: resolved on branch `fix/audit-web-shell-2`: no `aria-label`; the name is the visible text, marked with its language, then a visually hidden "Switch language / 切换语言" whose halves carry `en` and `zh-CN` (`web/src/App.svelte`, `textIn` and `otherLang` in `web/src/lib/prefs.svelte.ts`). The look is unchanged. Scenario "The language button is named by its visible text, each part in its own language".

### 2026-11-20 · deviation · The browser bar kept the system colour after a theme was chosen

- Symptom: with a light system and the dark theme chosen, the browser's bar (Android Chrome, Safari) stayed white: `<meta name="theme-color">` in `web/index.html` follows the system scheme only.
- Status: resolved on branch `fix/audit-web-shell-2`: an effect in `web/src/lib/prefs.svelte.ts` gives both tags the colour `index.html` gives the theme shown. The manifest colours stay fixed. Unit tests "the colour of the browser bar".

### 2026-11-20 · bug · Under forced colours a choice was told by the weight of its text alone

- Symptom: in a Windows contrast theme (forced colours), the sliding mark of the navigation and the thumb of each segmented control disappear, because the system drops backgrounds and shadows; the current page and the chosen theme and language were shown only in bold.
- Status: resolved on branch `fix/audit-web-shell-2`: `web/src/styles/base.css` section 8 draws them in `Highlight` and `HighlightText` with `forced-color-adjust: none` (otherwise the system's text backplate hides the text) and hides the hover layer over the current entry. Scenario "Under forced colours the chosen options and the current page are marked in system colours".

### 2026-11-20 · deviation · Defaults were stored on the first visit

- Symptom: the first load wrote the default theme and the language guessed from the browser to `localStorage`, so a person who never chose a language kept it after changing the browser's language.
- Cause: one effect wrote `<html>` and storage on every run, the first included (`web/src/lib/prefs.svelte.ts:40-47` before the fix).
- Status: resolved on branch `fix/audit-web-shell-2`: storage is written only when the person changes a setting (`set` in `web/src/lib/prefs.svelte.ts`). Unit test "writes nothing to this browser until the person changes a setting".

### 2026-11-20 · bug · Web device log refused the entry it held when `p` carried a lone surrogate

- Symptom: a browser holding a device-log entry whose `p` has U+FFFD refused with `fork` the same entry arriving with the escape `\ud800` in that place, and the reverse; a Go node, for the same bytes, does not refuse it (`contracts/envelope.md` section 4: the entry held at a version, the same `p`, `k` and `s`, is not refused). Both spellings are one signed text: the signature covers `p` as `TextEncoder` encodes it, a lone surrogate as U+FFFD, and `encoding/json` reads the escape as U+FFFD. A node passes entries on with U+FFFD, so a browser that wrote the escape and later sends its log again after a `welcome` has its own entry refused by a browser that got it through a node. Found in the audit of the identity code.
- Cause: `same` compared `p` as JavaScript strings (`web/src/lib/devlog.ts:111` before the fix).
- Status: resolved in PR #115: `same` compares `p.toWellFormed()` (`web/src/lib/devlog.ts:110-113`). `crypto.json` holds the case in `devlogs` (its entries are JSON text, so that `p` can carry the escape), and both vector runners pass it; `web/src/lib/devlog.test.ts` and `internal/identity/devlog_test.go` (`TestLogTakesTheSameEntryWithALoneSurrogateAgain`) test it too.

### 2026-11-20 · deviation · A device-log entry of more than 2 KiB is refused before the contract says so

- Symptom: `Log.Apply` and `DeviceLog.apply` refuse an entry whose `p` is more than 2,048 bytes as UTF-8. `planning/contracts/envelope.md` on `rebuild` has no such rule yet.
- Cause: the rule was settled by the coordinator (`planning/risks.md` Q-23) and is written in planning pull request #71, which is not merged: `contracts/envelope.md` section 4, rule 2, and the two cases of `contracts/vectors.md`. The code follows that text (`internal/identity/devlog.go:27-29`, `:89`; `web/src/lib/devlog.ts:118-128`), and `crypto.json` holds the cases.
- Status: resolved: PR #71 (4ac3e10f5) merged the rule into `planning/contracts/envelope.md` section 4, rule 2, so the code now follows the contract.

### 2026-11-20 · bug · Language options were spoken in the language of the page

- Symptom: on the settings page of the English interface, the option "简体中文" had no language of its own, so a screen reader spoke it with the English voice (eSpeak in NVDA reads no Chinese); in the Chinese interface "English" was given the Chinese voice. WCAG 2.2 SC 3.1.2 (Language of Parts, AA) asks that each passage's language be programmatically determined. Found in the audit of the web shell.
- Cause: the radio labels inherited `lang` from `<html>`, which follows the interface language (`web/src/routes/Settings.svelte:39-40` before the fix).
- Status: resolved on branch `fix/audit-web-shell`: each option's `<label>` carries the `lang` of the language it names (`web/src/routes/Settings.svelte:41`); the scenario "Each language option is marked with the language it is written in" of `features/web.feature` covers both interfaces.

### 2026-11-20 · deviation · Identity code was behind the contract settled in PR #69

- Symptom: after PR #69 the contract and both implementations differed: the pairing `mac` covered the role and the device id only, so the rendezvous service could change `kind` and `name` of a `pair` message; a `pair` message of any `kind` was accepted; `n` and `g` of a signed payload took any text, an empty one and the escape of a lone surrogate included, which a node and a browser read as different nonces; `crypto.json` had no `pair_messages` and no `kind` and `name` in `pairing`.
- Cause: the rules were settled after B-02b and F-01b merged (`internal/identity/pairing.go:60-64`, `internal/identity/envelope.go:45-51`, `web/src/lib/pairing.ts:46-56`, `web/src/lib/payload.ts:32-38`, all before the change).
- Status: resolved in PR #72 (B-02c with F-01d): the `mac` covers `role`, `k`, `kind` and `name` as `TextEncoder` encodes them (`internal/identity/pairing.go:57-75`, `web/src/lib/pairing.ts:46-72`); a random id is a type that `proto.Decode` and `proto.ts` check (`internal/proto/decode.go:152-157`, `web/src/lib/proto.ts:17-22`); `crypto.json` holds every case `contracts/vectors.md` names and both test suites pass on it.

### 2026-11-20 · deviation · Go identity and the web client answer differently until B-02b

- Symptom: for the same bytes, `internal/identity` and `web/src/lib/identity.ts` can give different answers. The web client follows the contract as settled in PR #58 and the coordinator's rules of 2026-11-20; the Go package follows the contract as it was. Known differences: weak keys as `k`, `dev`, `user`, pair `k` or signature R (web refuses, Go accepts); an empty `to` (web: a value, so `not_for_me`; Go: no `to`); signalling without `to` (web: `not_for_me`; Go accepts); a payload without `t`, `n` or `ts`, or with `null` in a field read (web: `bad_request`; Go: zero values); a key in another case, such as `TO` (web ignores it; Go reads it); an integer written `2.0` (web: 2; Go refuses) and one above 2^53 (web refuses; Go reads it); an `add` without `kind` `node` or `browser` or without a string `name` (web refuses; Go accepts); a `revoke` of the last valid device (web refuses; Go accepts); a long s in a pairing secret (web refuses; Go reads it as S); a pair message whose `k` is not a device id (web ignores it; Go checks only the mac); a signature whose S is not below L (both refuse, the web client before WebCrypto).
- Cause: the rules changed after B-02 merged; the Go package keeps the old behaviour (`internal/identity/key.go:87-91` signature check, `internal/identity/envelope.go:51-62` payload fields, `internal/identity/devlog.go:63-78` and `:93-104` log rules, `internal/identity/pairing.go:36-37` and `:66-68` secret and pair message). The web side: `web/src/lib/payload.ts`, `web/src/lib/identity.ts:126-136`, `:213-219`, `web/src/lib/devlog.ts:98-101`, `web/src/lib/pairing.ts:22-29`.
- Status: resolved in PR #67: `internal/identity` follows the settled contract, `internal/proto/testdata/crypto.json` holds the matching vectors, and `web/src/lib/vectors.test.ts` passes on them. The line references above are those before the change.

### 2026-11-20 · limitation · An empty `to` in a signed payload is read as no `to`

- Symptom: an envelope whose payload carries `"to":""` passes the `not_for_me` check on every receiver, although `contracts/envelope.md` section 3 says a `to` that is present must equal the receiver's device id.
- Cause: `proto.PayloadHeader.To` is a plain string (`internal/proto/envelope.go:17`), so `addressed` cannot tell an empty `to` from a missing one (`internal/identity/envelope.go:56`). Only a device the receiver already listens to can sign such a payload.
- Status: resolved in PR #67, by the rule the contract settled on (`contracts/envelope.md` section 3, check 4): an empty `to` is a value like any other and is nobody's device id, so it is `not_for_me` for signalling and commands alike, and `null` is `bad_request`. The payload is read into a pointer field, which tells an empty `to` from a missing one (`internal/identity/envelope.go:49`, `addressed` at `internal/identity/envelope.go:82`).

### 2026-11-20 · limitation · Go and browser JSON parsers can disagree on a signed payload

- Symptom: not observed. A log entry written by hand by one of the person's own valid devices could be accepted by a node and refused by a browser, or the reverse, which leaves their logs at different versions. Examples: `"ver":2.0` (refused by Go, read as 2 by `JSON.parse`), a key spelled `"VER"` (matched by Go, ignored by a browser). The common fields that `OpenSignal` and `OpenCommand` read disagree the same way: for `{"to":"A","TO":"B"}` Go reads `to` as B and `JSON.parse` as A, and Go also matches Unicode case folds, so `"tſ"` (long s) sets `ts`. Go and `JSON.parse` both keep the last of two equal keys.
- Cause: `entry` reads the payload with `encoding/json` into `proto.DeviceEntry` (`internal/identity/devlog.go:68`), and `addressed` into `proto.PayloadHeader` (`internal/identity/envelope.go:53`); `encoding/json` matches keys without regard to case and refuses a fraction for an integer, and the contract has no canonical JSON. The shared vectors (`internal/proto/testdata/crypto.json`) hold no such payload.
- Status: resolved in PR #67. `internal/identity` reads every payload with `proto.Decode` (`internal/proto/decode.go:40`), as every later task that reads the wire does; it reads as `contracts/conventions.md` section 2 says: names are exact, a field that is always present must be there, `null` is taken only by a field marked "or `null`", and an integer is a number whose nearest double is a whole number of at most 2^53 − 1, so `2.0` is 2. `internal/proto/testdata/crypto.json` holds such payloads in `received` and `devlogs`. What remains is the depth of nesting each parser accepts (`planning/risks.md` K-16).

### 2026-11-20 · limitation · Go verifies signatures of small-order public keys that OpenSSL refuses

- Symptom: for the device id `AQAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA` (the identity point), the signature made of the identity point and a zero scalar verifies over any text in `crypto/ed25519`, so `VerifyChallenge` and `OpenCommand` (with a `known` that admits the id) accept it without any private key. The other small-order points verify such forgeries for part of all texts. Node 24 (OpenSSL) refuses the same signature, so a Go receiver and another Ed25519 implementation can disagree on whether an envelope or log entry verifies.
- Cause: `ed25519.Verify` (`internal/identity/key.go:90`) follows RFC 8032 without refusing small-order or non-canonical public keys, and `contracts/envelope.md` section 2 does not say whether such a key is a device id.
- Status: resolved in PR #67. The fourteen encodings of small-order points are no device id and no first half of a signature (`contracts/envelope.md` sections 2 and 3); `verify` compares the bytes itself before it asks the library (`internal/identity/key.go:91-137`). As `k` such a value is `bad_sig`, as `dev` it makes a log entry refused, and `Secret.Verify` refuses a `pair` message that carries it. `crypto.json` holds a forged signature under each of the fourteen and one by a real key that begins with the identity point. A real key with a small-order point added to it still passes (`planning/risks.md` K-15).

### 2026-11-20 · deviation · Web note rows refuse `pos` until the Go type has it

- Symptom: the settled row contract (PR #58) gives note rows a `pos`; `decode` refuses a note row that carries one.
- Cause: `NoteRow` mirrors the merged Go `NoteRow` and `internal/proto/testdata/row-note.json`, which have no `pos` (`web/src/lib/proto.ts:261-270`).
- Status: resolved in B-01 slice (d). Both wire types require the creation position and the shared note example includes it.

### 2026-11-20 · bug · Web `verify` threw for a `k` or `s` of some hundred thousand characters

- Symptom: `verify`, `isDevice`, `verifyPair` and `DeviceLog.apply` threw `RangeError: Maximum call stack size exceeded` for a `k`, `s`, `dev` or `mac` of 400 000 base64url characters, where the contract wants `bad_sig`, a refused entry or an ignored `pair` message. Under Node 24 a text of 133 000 characters still passed and one of 267 000 threw; the limit differs by engine and was not measured in the browsers. Nothing was accepted that should have been refused. Found in the review of PR #66, before the code was merged.
- Cause: `decode` encodes the bytes it decoded to compare the spelling, and `encode` spread them all into the arguments of one `String.fromCharCode` call (`web/src/lib/identity.ts:18` before the fix).
- Status: resolved in PR #66: `encode` makes the text byte by byte (`web/src/lib/identity.ts:18-19`). Checked under vitest with 400 000 characters.

### 2026-11-20 · limitation · Browsers accept small-order Ed25519 public keys

- Symptom: `crypto.subtle.verify` returns true for the signature R = identity, S = 0 under the identity point as public key, over any message, in Chromium 159, Firefox 159 and WebKit 27.2 (spike S-7, 2026-11-20). WebCrypto requires false for small-order keys; web-platform-tests `eddsa_small_order_points` passes 8, 9 and 9 of 14 in Chrome 158, Firefox 160 and Safari 27.2 (wpt.fyi, 2026-11-19).
- Cause: the engines' Ed25519 libraries do not make the small-order checks ([Firefox bug 1929022](https://bugzilla.mozilla.org/show_bug.cgi?id=1929022), [WebKit bug 280673](https://bugs.webkit.org/show_bug.cgi?id=280673), both open).
- Effect: relying on WebCrypto alone would admit invalid signers. The envelope contract requires rejection before WebCrypto.
- Status: resolved for the web client in F-01b: the settled contract (PR #58, `contracts/envelope.md` section 2) makes the fourteen weak values no device id, and the client compares the bytes itself before WebCrypto is asked, for `k`, `dev`, `user`, a pair message's `k` and the first half of a signature (`web/src/lib/identity.ts:104-136`). The Go side follows in B-02b.

F-01c measurements. The first two results are native `subtle.verify`; the last is SoloDawn's receiver result for **each** of the fourteen weak-key vectors. CI runs four builds on every pull request; the others ran by hand on macOS 26.6.2.

| Browser | Run by | Identity-point vector | Valid empty message (RFC 8032 §7.1 test 1) | Receiver |
| --- | --- | --- | --- | --- |
| Chrome for Testing 137.0.7151.119 | CI, Ubuntu 24.04; by hand, macOS | true | true | `bad_sig` |
| Edge 137.0.3296.93 | CI, Ubuntu 24.04 | true | true | `bad_sig` |
| Firefox 136.0 (Playwright build 1478) | CI, Ubuntu 24.04; by hand, macOS | true | true | `bad_sig` |
| WebKit 27.2 (Playwright build 2370) | CI, macOS 26; by hand, macOS | true | false | `bad_sig` |
| Chromium 156.0.8078.4 (Playwright) | by hand, macOS | true | true | `bad_sig` |
| Firefox 157.0 (Playwright) | by hand, macOS | true | true | `bad_sig` |
| Safari 26.6.2 | by hand, macOS, once, with Vitest's preview provider, which is not in the repository | true | false | `bad_sig` |

No Safari release runs in CI, and Safari 17.5, the baseline, was not run at all. Playwright's WebKit is a current build of Apple's port, so its row shows that the vectors pass on Apple's Ed25519 implementation of today, not on that of Safari 17.5. For Safari 17.5 the baseline still rests on the sources cited in the browser-baseline decision, not on a run.

### 2026-11-20 · limitation · Firefox 136 cannot export the JWK of a seed imported as PKCS #8

- Symptom: on Firefox 136.0, importing an Ed25519 seed succeeds and it signs, but exporting the imported private key as JWK rejects with `OperationError`. The other builds above export the expected public coordinate.
- Effect: the original shared test used that export to construct a key pair. SoloDawn does not export its private device keys.
- Status: handled in F-01c. The suite does not export: it imports the public key of the vector and proves that **each** seed signs for it. No signature bytes are compared.

### 2026-11-20 · limitation · Web client does not start when the browser blocks site storage

- Symptom: expected blank page when the person has blocked site data for the page's origin. Checked under vitest with a `localStorage` that throws (the import of `prefs` rejects); not reproduced in a real browser.
- Cause: `prefs` reads `localStorage` while its module loads (`web/src/lib/prefs.svelte.ts:12-16` before the fix), and both the application and the upgrade notice import it. A browser that refuses storage access throws on `localStorage` (Chrome raises a `SecurityError`), so the import fails before `boot` mounts anything.
- Status: resolved in 815f0547d (PR #61). Storage is read and written behind a guard (`web/src/lib/prefs.svelte.ts:17-35`): a failed read, or a value the client does not know, gives the default; a failed write is ignored. Checked under vitest with a `localStorage` getter that throws a `SecurityError`; still not tried in a real browser.

### 2026-11-20 · bug · Go acceptance suite ran the web feature

- Symptom: `go test -tags acceptance -count=1 ./features/...` failed with `step is undefined` for seven scenarios of `features/web.feature` (for example `the current page is "群聊"`), so the merge gate failed.
- Cause: `features/acceptance_test.go:44` sets `Paths: []string{"."}`, so godog runs every `.feature` in `features/`, including `web.feature`, whose steps are implemented in `web/features/web.spec.ts` (vitest-cucumber) and not in Go; `Strict: true` fails the undefined steps. Each branch passed on its own (the F-00 branch had no `go.mod`, so Go acceptance was skipped; the B-00 and B-01 branches had no `web.feature`); the conflict appeared only after both were merged.
- Status: resolved in PR #59: `web.feature` is tagged `@web` and the godog run uses `Tags: "~@web"`.
