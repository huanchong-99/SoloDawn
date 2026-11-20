# Agent Note: Browser baseline Chrome and Edge 137, Firefox 136, Safari 17.5

Status: implemented

## Problem

The web client signs as a device with a non-extractable Ed25519 key kept in IndexedDB, with
WebCrypto only and no bundled signature library ([envelope contract](../../../../planning/contracts/envelope.md)
section 2, `planning/architecture.md` C-9, `planning/product-rules.md` R-164). The plan left the
lowest browser versions that can do this unverified and gave the question to spike S-7
([step 1](../../../../planning/steps/step-1.md), M0; `planning/risks.md` Q-3), with the rule that
those versions, or the design's baseline where it is higher, become the supported baseline. Step 1
is accepted on exactly these browsers: Safari, Chrome and Firefox on macOS, Chrome and Firefox on
Windows and Linux, Safari on iOS, Chrome on Android.

## Decision

The supported browsers are:

| Browser                                                | Lowest version |
| ------------------------------------------------------ | -------------- |
| Chrome (macOS, Windows, Linux, Android), Edge          | 137            |
| Firefox (macOS, Windows, Linux)                        | 136            |
| Safari (macOS and iOS)                                 | 17.5           |

The build targets exactly these versions (`web/vite.config.ts`:
`['chrome137', 'edge137', 'firefox136', 'safari17.5', 'ios17.5']`). CSS is minified for the same
targets, so `light-dark()` and nested rules ship as written. At start, `boot`
(`web/src/lib/boot.ts`) mounts the upgrade notice instead of the application unless all of these
hold: WebCrypto makes a non-extractable Ed25519 key pair (`{name: 'Ed25519'}`, usages `sign` and
`verify`); a structured clone of the private key signs a non-empty message; the original public
key verifies that signature; and `CSS.supports('color', 'light-dark(#000, #fff)')` is true. `boot`
does not touch IndexedDB: storage that the person or the browser blocks is not an old browser, and
the identity module handles it.

Origin: spike S-7 measured and researched the versions on 2026-11-20 and reported two build
options; the coordinator (the main session) chose this one, acting under the owner's written
authorization that delegates to the coordinator every decision that needs the owner's assent. The
owner did not decide it in person.

### Why each version

Each capability below is the first version that has it; the baseline is the highest of them.

| Capability                                                                | Chrome, Edge | Firefox | Safari |
| ------------------------------------------------------------------------- | ------------ | ------- | ------ |
| Non-extractable Ed25519 key from `generateKey`; `sign`, `verify`          | 137          | 129     | 17.0   |
| `exportKey('raw')` of the public key (32 bytes), `importKey('raw')`       | 137          | 129     | 17.0   |
| The key read back from IndexedDB (structured clone) still signs           | 137          | **136** | 17.0   |
| `light-dark()` colours (every colour in `web/src/styles/tokens.css`)      | 123          | 120     | **17.5** |
| Nested rules that start with a type selector (`web/src/styles/base.css`)  | 120          | 117     | 17.2   |
| `:has()`, `color-mix()`, `dvh` units                                      | 111          | 121     | 16.2   |

- Chrome 137 shipped Ed25519 in WebCrypto on desktop and Android
  ([release notes](https://developer.chrome.com/release-notes/137),
  [Chrome Platform Status](https://chromestatus.com/feature/4913922408710144),
  [tracking bug](https://issues.chromium.org/issues/1370697)); before it, Ed25519 was behind the
  experimental web platform features flag. The Chromium 137 source already has the structured
  clone tags for Ed25519 keys (`web_crypto_sub_tags.h`, `kEd25519Tag`, `kEd25519KeyTag`). Edge 137
  follows Chromium.
- Firefox shipped Ed25519 in 129 ([developer release notes](https://developer.mozilla.org/en-US/docs/Mozilla/Firefox/Releases/129),
  [bug 1804788](https://bugzilla.mozilla.org/show_bug.cgi?id=1804788)), but a stored key cannot be
  read back before 136 (next section).
- Safari 17.0 added Ed25519
  ([Safari 17 release notes](https://developer.apple.com/documentation/safari-release-notes/safari-17-release-notes),
  "Added support for Ed25519 cryptography"); the structured clone of Ed25519 keys is part of
  WebKit's first Ed25519 commit (3abe0e4531), before that release. Safari is at 17.5 only because
  of `light-dark()`.
- The CSS versions come from MDN browser-compat-data (`css/types/color.json` for `light-dark()`
  and `color-mix()`, `css/selectors/nesting.json`, `css/selectors/has.json`,
  `css/types/length.json`); the WebCrypto versions also match its `api/SubtleCrypto.json`.

### Firefox 129 to 135 cannot read a stored key back

Firefox 129 to 135 makes the key and writes it to IndexedDB, but `get()` fails with "An attempt
was made to use an object that is not, or is no longer, usable", and `structuredClone()` of the key
fails the same way ([bug 1939993](https://bugzilla.mozilla.org/show_bug.cgi?id=1939993), fixed in
136). The cause is in `dom/crypto/KeyAlgorithmProxy.cpp`: `WriteStructuredClone` handles Ed25519
keys, but `ReadStructuredClone` has no case for them until the fix (checked in the release tags
`FIREFOX_129_0_RELEASE` to `FIREFOX_135_0_1_RELEASE`, and `FIREFOX_136_0_RELEASE`). A browser
that loses its device key at every reload cannot be a device, so `boot` signs with a clone of the
key, which these versions fail. Firefox 129 to 133 also rejects `importKey('raw', …, 'Ed25519', …)`
with the algorithm given as a bare string ([bug 1924951](https://bugzilla.mozilla.org/show_bug.cgi?id=1924951),
fixed in 134); the client gives algorithms as objects.

## Alternatives considered

**The same versions, with Safari at 17.0 and the CSS lowered.** With the build target at
`safari17`, Lightning CSS rewrites every `light-dark()` pair into two custom properties per colour
and flattens the nested rules; S-7 built the client this way, and in the output `data-theme` still
switches those properties (read in the CSS, not run in a browser). It would add Safari 17.0 to
17.4. The coordinator chose to ship the CSS as written instead; no further reason was recorded.

**Firefox 129, with the key kept another way.** Not possible within the contract: a key that
IndexedDB cannot return would have to be extractable to be stored as bytes, and the contract
requires a non-extractable key.

**A bundled Ed25519 library for older browsers.** Excluded by R-164 before S-7; not reconsidered.

## Consequences

- Browsers below the baseline get the upgrade notice, which names these versions. Without
  `light-dark()` (Chrome below 123, Firefox below 120, Safari below 17.5) the notice shows with
  default colours, and a browser that cannot run the bundle shows a blank page (`issues.md`).
- Safari signs Ed25519 with random signatures: two signatures of the same message differ (MDN
  browser-compat-data note on `SubtleCrypto.sign`; [WebKit bug 262499](https://bugs.webkit.org/show_bug.cgi?id=262499),
  open). Chrome and Firefox sign deterministically as RFC 8032 specifies. Tests must not expect
  fixed signature bytes from a browser: they verify fixed signatures from test vectors, and verify
  the browser's own signatures, and nothing else.
- Safari deletes all script-writable storage of a site, IndexedDB included, after seven days of
  Safari use without interaction with the site ([WebKit blog](https://webkit.org/blog/10218/full-third-party-cookie-blocking-and-more/));
  a web app added to the home screen keeps its own count and is exempt. For SoloDawn that loses
  the device key, and the browser has to be paired again (`issues.md`). Safari grants
  `navigator.storage.persist()` by heuristics, such as being opened as a home screen web app
  ([WebKit blog](https://webkit.org/blog/14403/updates-to-storage-policy/)). Lockdown Mode
  disables IndexedDB (Safari 17 release notes).
- Current WebKit returns false from `verify()` for every signature over an empty message
  (`isValidSignature` in WebKit's `CryptoKitShim.swift` returns false when the data is empty);
  no bug report was found and the first affected version is unknown. Every signed SoloDawn
  message starts with `solodawn/1\n`, and `boot` signs a non-empty message.
- None of the three engines rejects small-order Ed25519 public keys in `verify()`, as WebCrypto
  requires (web-platform-tests `eddsa_small_order_points`: 8, 9 and 9 of 14 passing in Chrome
  158, Firefox 160 and Safari 27.2 on 2026-11-19). Whether the protocol rejects such keys itself is
  open (`issues.md`).
- WebKit on Linux (the GTK and WPE ports, which end-to-end tests may use) had its own Ed25519
  defects: `generateKey()` failed for about one key in 128, fixed by WebKit commit e6eb9ebd41
  (2026-10-09), and `verify()` with a small-order key crashed the process, fixed by 183d2f8db4
  (2026-10-12). Which Playwright WebKit build contains them is not verified.
- The development server and the build both serve the CSS as written, so what a developer sees
  is what ships.

## Verification

- S-7, on macOS (arm64), 2026-11-20: Playwright 1.65.1 with Chromium 159.0.8154.6, Firefox 159.0
  and WebKit 27.2, a test page on `http://localhost`, and a persistent profile per engine. In all
  three: the private key is not extractable (`raw`, `pkcs8` and `jwk` export throw
  `InvalidAccessError`); the raw public key is 32 bytes and, imported again, verifies; the key
  pair stored in IndexedDB and read back after closing the page, and again after quitting and
  relaunching the browser on the same profile, signs, verifies and is still not extractable.
  RFC 8032 test vectors 1 to 3 (checked against OpenSSL first) verify in Chromium and Firefox, and
  2 and 3 in WebKit (1 is the empty message). Twenty signatures of one message gave one value in
  Chromium and Firefox and twenty in WebKit.
- The versions of Chrome 137, Firefox 129 to 136 and Safari 17.0 to 17.5 were not run; they rest
  on the release notes, bug trackers, source and compatibility data linked above. In particular,
  that `structuredClone()` of a key fails in Firefox 129 to 135 is stated in bug 1939993 and
  follows from the source; it was not observed.
- F-01c, CI run 44687053047 (2026-11-20): the shared identity vectors (`crypto.json`) passed in
  Chrome for Testing 137.0.7151.119, Edge 137.0.3296.93 and Firefox 136.0 on Ubuntu 24.04, and in
  Playwright WebKit 27.2 on macOS 26.7.2; no Safari release ran, so Safari 17.5 still rests on the
  sources above.
- The build with these targets keeps `light-dark()` and nesting in the CSS output (checked in
  `internal/node/webdist/assets/*.css` after `vite build`). `web/src/lib/boot.test.ts` and
  `features/web.feature` cover each reason for the upgrade notice with stand-ins for the browser.
