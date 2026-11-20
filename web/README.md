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
notice for browsers that cannot run the client.

## Use this capability

Run these in `web/` with pnpm; versions are pinned in `package.json` and `pnpm-lock.yaml`.

| Command            | Effect                                                             |
| ------------------ | ------------------------------------------------------------------ |
| `pnpm install`     | installs the development dependencies                              |
| `pnpm dev`         | development server with hot reload                                 |
| `pnpm build`       | production build into `../internal/node/webdist/` (ignored by Git) |
| `pnpm check`       | `svelte-check --fail-on-warnings`: types and Svelte warnings       |
| `pnpm test`        | unit tests in `src/` and the acceptance feature in `features/`     |
| `pnpm format`      | Prettier                                                           |
| `pnpm stryker run` | mutation tests of every `.ts` module outside routes and components |

## Contract

- **Static hosting.** Asset paths are relative (`base: './'`) and pages are address fragments, so
  the build works from any path and needs no server rule. The fragment is `#/<page>/…`; the pages
  are `groups`, `nodes` and `settings`; anything else opens `groups`.
- **Build output.** `vite build` empties `internal/node/webdist/` and writes its `.gitignore`
  back, so the directory stays out of Git and invisible to the gates.
- **Browser support (C-9).** `boot` asks WebCrypto for a non-extractable Ed25519 signing key. When
  that fails, or the page has no WebCrypto, the upgrade notice is mounted in place of the
  application.
- **Preferences.** `prefs.theme` is `auto`, `light` or `dark`; `prefs.lang` is `zh-CN` or `en`.
  A first visit follows the system theme and the browser language (any `zh` locale opens Chinese).
  Every change is written to `<html data-theme lang>` and to `localStorage` (`sd-theme`,
  `sd-lang`). A stored value the client does not know, from another version for instance, is
  ignored. When the browser blocks site storage, the client opens with the defaults and applies
  changes without remembering them. `tokens.css` turns `data-theme` into `color-scheme`; every
  colour is a `light-dark()` pair.
- **Fonts.** Geist and Geist Mono 1.9.2, one variable `woff2` each, are served from
  `public/fonts/` with their licence (OFL-1.1); the page requests nothing from a third party. A
  browser downloads a face only when text uses it and shows the system fonts until it arrives.
- **Texts.** Every interface string is a key in both `src/messages/zh-CN.json` and
  `src/messages/en.json` and is read with `t(key)`; a test fails when the key sets differ. Chat
  content is never translated.
- **Where code goes.** Logic lives in `.ts` and `.svelte.ts` modules, which the gates measure
  (CRAP and mutation); components stay thin. Shared styles are global classes in
  `src/styles/base.css`, taken from the design; a component's own layout goes in its `<style>`.

## Understand the implementation

| Path                        | Role                                                         |
| --------------------------- | ------------------------------------------------------------ |
| `index.html`, `src/main.ts` | entry: styles, then `boot(document.body, crypto)`            |
| `src/lib/boot.ts`           | Ed25519 detection; mounts `App` or `Upgrade`                 |
| `src/lib/prefs.svelte.ts`   | theme and language state, their persistence, `t`             |
| `src/lib/route.svelte.ts`   | current page from the address fragment                       |
| `src/App.svelte`            | shell: skip link, navigation, top bar, the current route     |
| `src/Upgrade.svelte`        | the upgrade notice                                           |
| `src/routes/`               | one component per page                                       |
| `src/components/`           | shared parts; `Icon.svelte` holds the line icons             |
| `src/styles/`               | `tokens.css` (design tokens) and `base.css` (shared classes) |
| `public/`                   | manifest, icon and `fonts/`, copied as they are              |
| `features/web.spec.ts`      | steps of `../features/web.feature`                           |

Below 768 px the navigation is the bottom bar and the theme and language buttons of the top bar
are hidden; both choices are on the settings page at every width.

## Known limitations

- The theme is applied by the module script, so a browser forced to the theme opposite to its
  system setting can paint one frame in the system theme while loading (`issues.md`).
- The upgrade notice uses the same tokens as the application. A browser without `light-dark()` or
  CSS nesting shows it with default colours; a browser too old to run the bundle shows nothing
  (`issues.md`).
- The manifest has an SVG icon only and there is no service worker yet.

## Verification and sources

- Unit tests sit beside the modules (`src/**/*.test.ts`); `../features/web.feature` is the
  acceptance feature. `stryker.config.json` breaks below a mutation score of 100.
- The shell was checked in headless Chrome at 375, 820, 1100 and 1440 px in both themes and both
  languages on every page (no element wider than the viewport) on 2026-11-20. It has not been
  opened on a real phone, in Safari or in Firefox.
- Rules: `planning/architecture.md` section 8, `planning/product-rules.md` R-160 to R-164.
