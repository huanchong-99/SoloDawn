---
description: 'The SoloDawn web client: how to build and check it, what its shell, routing, preferences and texts guarantee, and what a new screen has to follow.'
kind: reference
---

# Web client

## Summary

One responsive Svelte 5 page, built by Vite into `internal/node/webdist/`, from where the node
embeds it and the rendezvous serves it as static files. It holds no group logic: it renders rows
and sends commands (`planning/steps/step-1-frontend.md`). What exists today is the frame every
screen lives in: navigation, top bar, themes, languages, hash routing, the web manifest and the
notices for browsers that cannot run the client or that block its storage; this browser's device
key with signed envelopes, the device log and pairing; the wire types with their decoding; and a
mock connection that answers from the node's wire examples.

The interface mockups are in `design/`. They are the visual reference for the interface: implement
screens to match them, and add the motion they leave out.

## Use this capability

Run these in `web/` with pnpm; versions are pinned in `package.json` and `pnpm-lock.yaml`.

| Command            | Effect                                                             |
| ------------------ | ------------------------------------------------------------------ |
| `pnpm install`     | installs the dependencies                                          |
| `pnpm dev`         | development server with hot reload                                 |
| `pnpm build`       | production build into `../internal/node/webdist/` (ignored by Git) |
| `pnpm check`       | `svelte-check --fail-on-warnings`: types and Svelte warnings       |
| `pnpm test`        | unit tests in `src/` and the acceptance feature in `features/`     |
| `pnpm format`      | Prettier                                                           |
| `pnpm stryker run` | mutation tests of every `.ts` module outside routes and components |

Before the first `pnpm test`, install the headless browser with
`pnpm exec playwright install --with-deps --only-shell chromium`. The five motion scenarios run
against a temporary production build in Chromium.

The one runtime dependency is `valibot`, for the wire types. Signatures, hashes and macs use the
browser's WebCrypto only; no signature library is bundled (R-164). `fake-indexeddb` is a
development dependency, an IndexedDB for the tests.

## Contract

- **Static hosting.** Asset paths are relative (`base: './'`) and pages are address fragments, so
  the build works from any path and needs no server rule. The fragment is `#/<page>/…`; the pages
  are `groups`, `nodes` and `settings`; anything else opens `groups`.
- **Build output.** `vite build` empties `internal/node/webdist/` and writes its `.gitignore`
  back, so the directory stays out of Git and invisible to the gates.
- **Browser support (C-9).** Chrome and Edge 137, Firefox 136, Safari 17.5 (macOS and iOS) and
  later; the build targets exactly these versions, so the CSS ships as written
  ([decision](../docs/decisions/implemented/architecture/2026-11-20-browser-baseline.md)). `boot`
  asks WebCrypto for a non-extractable Ed25519 key pair, signs a message with a structured clone of
  the private key (how IndexedDB stores it) and verifies it with the public key, and asks CSS for
  `light-dark()`. When any step fails, or the page has no WebCrypto, the upgrade notice is mounted
  in place of the application. Only then does `boot` load the device key (next item); when
  IndexedDB fails it mounts the storage notice instead, which is not an old browser.
- **Device key.** `loadDevice` keeps this browser's non-extractable Ed25519 key pair in IndexedDB
  (database `solodawn`, store `device`, key `key`) and creates it on first run. Each call makes a
  candidate key and stores it only when none is stored, in one transaction, so two pages opened at
  once keep one key; the transaction is durable (`durability: 'strict'`, ignored by a browser
  without the option), so a new key is not lost to a power cut. It does not ask for persistent
  storage: in Firefox `navigator.storage.persist()` opens a permission prompt, which a first
  visit should not show. Onboarding (F-04) asks for it once the device is enrolled. When
  IndexedDB cannot be reached, opened, read or written it rejects with `StorageBlocked`. A `Device` has the `id` (the public key,
  base64url, 43 characters) and the `fingerprint` of `contracts/envelope.md` section 2, and signs
  envelopes and rendezvous answers. `boot` loads it and drops it; the connection code will load it
  again.
- **Envelopes, device log, pairing.** `src/lib/identity.ts`, `devlog.ts` and `pairing.ts`
  implement sections 1 to 4 and 6 of `contracts/envelope.md` as settled in PR #58, where it differs
  from `internal/identity` (`issues.md`): canonical base64url only; the fourteen weak keys are never
  a device id and a signature whose R is one, or whose S is not below L, fails before WebCrypto is
  asked; `openSignal` and `openCommand` answer the first failing check (`bad_sig`,
  `unknown_device`, `bad_request`, `not_for_me`, `stale`), with both ends of the window accepted;
  `DeviceLog.apply` accepts or refuses an entry by section 4 and leaves the log unchanged when it
  refuses; `adopts` is the log's half of adoption; a pairing secret is read by removing hyphens and
  upper-casing a to z only. `src/lib/payload.ts` reads the fields those checks need, with
  `JSON.parse`: exact names, the last of a repeated name, integers as safe integers by value, and
  no `null`. Signing is left to WebCrypto, so in Safari two signatures of one payload differ.
- **Preferences.** `prefs.theme` is `auto`, `light` or `dark`; `prefs.lang` is `zh-CN` or `en`.
  A first visit follows the system theme and the browser language (any `zh` locale opens Chinese).
  Every change is written to `<html data-theme lang>` and to `localStorage` (`sd-theme`,
  `sd-lang`). A stored value the client does not know, from another version for instance, is
  ignored. When the browser blocks site storage, the client opens with the defaults and applies
  changes without remembering them. `tokens.css` turns `data-theme` into `color-scheme`; every
  colour is a `light-dark()` pair.
- **Wire types.** `src/lib/proto.ts` mirrors `internal/proto`: one `valibot` schema per message,
  with its TypeScript type inferred under the same name. An unknown field is dropped, as Go's
  `encoding/json` does, so a node that adds a field does not break an older client; a missing
  field is refused unless Go tags it `omitempty`; a Go pointer without
  `omitempty` must be there and may be `null`. Message bodies are any JSON until a contract types
  them. `decode(schema, text)` is the only place where received text becomes a typed value;
  `Frame` is what a node may send on `ctl`. Code after it trusts the type. An envelope's `p` stays
  the exact string received. `Ctl` is the interface of a `ctl` connection: numbered requests,
  their answers, and pushes.
- **Mock connection.** `mockCtl(onpush)` in `src/lib/mock.ts` is a `Ctl` that answers from the
  files of `internal/proto/testdata`: `hello` with `welcome`; `sync` with the pushes `reset` and
  `rows`, then `synced`; `fetch` with `fetched`; every `cmd` with `ok`. It knows the examples'
  one group. It is for development and tests only: import it dynamically behind
  `import.meta.env.DEV`, so that the build leaves it and the examples out. `vite.config.ts` names
  that directory `@testdata` and lets Vite read it (`server.fs.allow`). Stryker runs the tests in
  a copy of `web/` under `.stryker-tmp/`; there the name still points at the repository's
  directory.
- **Fonts.** Geist and Geist Mono 1.9.2, one variable `woff2` each, are served from
  `public/fonts/` with their licence (OFL-1.1); the page requests nothing from a third party. A
  browser downloads a face only when text uses it and shows the system fonts until it arrives.
- **Texts.** Every interface string is a key in both `src/messages/zh-CN.json` and
  `src/messages/en.json` and is read with `t(key)`; a test fails when the key sets differ. Chat
  content is never translated.
- **Motion.** No animation library is bundled: CSS transitions and keyframes move the parts, the
  browser's View Transitions cross-fade the page, and `svelte/motion` reads the system setting.
  The tokens are in `tokens.css` beside the others: four durations (`--dur-1` 120 ms hover and
  press, `--dur-2` 200 ms switch and focus, `--dur-3` 320 ms arrival and cross-fade, `--dur-4`
  500 ms spring), `--ease` for a change of state, `--ease-out` for an arrival, `--spring` for what
  a person moved, `--travel` and `--stagger`. Only `transform` and `opacity` are animated; a colour
  that changes on hover, press or focus is a layer under or around the part whose opacity changes.
  `route.dir` is the way the last page change went along the navigation (1, -1, or 0 within a
  page) and `--at` on the navigation and on a segmented control is the index of the current entry;
  the styles turn them into the side a page arrives from and the place of the sliding mark. The
  parts of a page arrive one after another, but only their travel waits: all of them fade in
  from the first frame, and a part whose animation does not run is seen in full.
  `choose(key, value)` changes a preference through `shift` of `src/lib/motion.ts`, which
  cross-fades the page where the browser has view transitions (Chrome and Edge, Safari 18,
  Firefox 144) and changes it at once elsewhere. The fade takes a picture of the old page only
  and fades it out over the new page: a page in a picture takes no pointer input, so `shift`
  marks the new page `data-live`, which keeps it out of the pictures, and a press during the fade
  reaches it. When the system asks for reduced motion, nothing moves: no transition, no
  keyframes, no cross-fade, and a cross-fade under way ends at once.
- **Where code goes.** Logic lives in `.ts` and `.svelte.ts` modules, which the gates measure
  (CRAP and mutation); components stay thin. Shared styles are global classes in
  `src/styles/base.css`, taken from the design; a component's own layout goes in its `<style>`.

## Understand the implementation

| Path                        | Role                                                            |
| --------------------------- | --------------------------------------------------------------- |
| `index.html`, `src/main.ts` | entry: styles, then `boot` with the page's crypto, CSS, storage |
| `src/lib/boot.ts`           | browser check, device key; mounts `App` or `Notice`             |
| `src/lib/identity.ts`       | device key in IndexedDB, base64url, sign, verify, open          |
| `src/lib/devlog.ts`         | `DeviceLog`: apply, replay, valid devices, adoption             |
| `src/lib/pairing.ts`        | pairing secret, room id, mac, pair message check                |
| `src/lib/payload.ts`        | the payload fields the envelope and log checks read             |
| `src/lib/prefs.svelte.ts`   | theme and language state, their persistence, `t`                |
| `src/lib/route.svelte.ts`   | current page from the address fragment, and the way it changed  |
| `src/lib/motion.ts`         | `spring`, the curve of `--spring`; `shift`, the page cross-fade |
| `src/lib/proto.ts`          | wire schemas and types, `decode`, the `Ctl` interface           |
| `src/lib/mock.ts`           | wire examples by name, their schemas, `mockCtl`                 |
| `src/App.svelte`            | shell: skip link, navigation, top bar, the current route        |
| `src/Notice.svelte`         | the upgrade notice and the storage notice                       |
| `src/routes/`               | one component per page                                          |
| `src/components/`           | shared parts; `Icon.svelte` holds the line icons                |
| `src/styles/`               | `tokens.css` (design tokens) and `base.css` (shared classes)    |
| `public/`                   | manifest, icon and `fonts/`, copied as they are                 |
| `features/web.spec.ts`      | steps of `../features/web.feature`                              |

Below 768 px the navigation is the bottom bar and the theme and language buttons of the top bar
are hidden; both choices are on the settings page at every width.

## Known limitations

- The theme is applied by the module script, so a browser forced to the theme opposite to its
  system setting can paint one frame in the system theme while loading (`issues.md`).
- The upgrade notice uses the same tokens as the application. A browser without `light-dark()` or
  CSS nesting shows it with default colours; a browser too old to run the bundle shows nothing
  (`issues.md`).
- Safari deletes the site's storage, and with it the device key, after seven days of use without
  a visit, unless the client was added to the home screen (`issues.md`).
- Nothing yet stores a device log or uses the device key beyond `boot`; the connection code
  (F-03) and onboarding (F-04) will.
- The manifest has an SVG icon only and there is no service worker yet.

## Verification and sources

- Unit tests sit beside the modules (`src/**/*.test.ts`); `../features/web.feature` is the
  acceptance feature. `stryker.config.json` breaks below a mutation score of 100.
- The shell was checked in headless Chrome at 375, 820, 1100 and 1440 px in both themes and both
  languages on every page (no element wider than the viewport) on 2026-11-20. It has not been
  opened on a real phone, in Safari or in Firefox.
- The motion was checked there too: while pages changed, the theme and language switched and
  parts were hovered, pressed and focused, the page's animations (`document.getAnimations()`)
  changed `opacity` and `transform` only, and with reduced motion emulated there were none. A test
  holds `--spring` to `spring()`. Frame rate was not measured, and the fallback without view
  transitions was exercised only in the tests.
- `src/lib/proto.test.ts` reads every wire example with the schema of its file name and fails on a
  file that has none (`crypto.json`, which holds test vectors, excepted); it adds an unknown field
  to every object, which is dropped (so a golden file with a field the schema lacks fails the exact
  comparison), and removes every field in turn. Fields that appear in no example are not
  covered by those variants.
- `src/lib/vectors.test.ts` holds the client to every part of
  `internal/proto/testdata/crypto.json` (`planning/contracts/vectors.md`). It never compares
  signature bytes, since Safari's signatures are random: it verifies the vectors' signatures and
  the ones the client makes itself. The vectors, and every other identity test,
  have run in Node only; IndexedDB has run as `fake-indexeddb` only. Running them in the three
  browsers is task F-01c.
- Importing `decode` and `Frame` adds about 7.6 KiB to the bundle, 2.6 KiB gzipped (measured
  with `valibot` 1.5.0 on 2026-11-20). Nothing in the application imports them yet.
- Rules: `planning/architecture.md` section 8, `planning/product-rules.md` R-160 to R-164.
