---
description: 'The SoloDawn web client: how to build and check it, what its shell, route table, connection, mounting points, preferences, texts, group caches, group list and chat guarantee, and how an area of the interface adds its parts.'
kind: reference
---

# Web client

## Summary

One responsive Svelte 5 page, built by Vite into `internal/node/webdist/`, from where the node
embeds it and the rendezvous serves it as static files. It holds no group logic: it renders rows
and sends commands (`planning/steps/step-1-frontend.md`). What exists today is the frame every
screen lives in: navigation, top bar, themes, languages, the route table, the connection the
shell chooses for every page, the mounting points through which each area of the
interface adds its parts, the web manifest and the notices for browsers that cannot run the
client or that block its storage; this browser's device
key with signed envelopes, the device log and pairing; the wire types with their decoding; a
mock connection that answers from the node's wire examples; the row cache of every group with the
lists drawn from it; the group list with its create-group dialog; the main line of a group's
chat with its composer; the terminal takeover page and hand-back card; the connection indicator and the computers and AIs page; and the connection code, signalling and
connections to paired nodes under the single-tab Web Lock. A development server shows the one
computer and the one group of the wire examples unless the address carries `?nomock`; everywhere
else the shell connects to each paired node over its rendezvous.

The visual reference for the interface is `design/README.md`, sections 1 and 3: the design
language (colour, type, the chat layout, the anatomy of a card, the motion catalogue). The static
HTML mockups beside it show the first design and are superseded.

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

`pnpm test:vectors` runs the unit suite for `crypto.json` in Playwright browsers. Install
Chromium and Firefox with `pnpm exec playwright install --with-deps --only-shell chromium firefox`;
on macOS also install `webkit`. Select one engine with `--project chromium|firefox|webkit`.
WebKit is available only on macOS. CI pins Chrome for Testing and Edge 137 (`CHROMIUM_PATH`),
and Playwright Firefox 136 (`FIREFOX136_PATH`) on Linux; on macOS it uses Playwright's bundled
WebKit, not a version-matched Safari release. Firefox 136 needs a separate Playwright driver:
the current driver's viewport and WebSocket protocol differs from that build's.

`pnpm exec vitest run --config e2e/config.ts --project chromium|firefox|webkit` runs the
end-to-end tests in Node against real processes, with the browsers of `test:vectors`; it also
needs Go. Its global setup (`e2e/nodes.ts`) builds the client into `solodawn`, and the fake Claude
Code and Codex beside it, once per run into a short directory under `/tmp`; a Go overlay embeds the
build, so nothing is written into `internal/node/webdist/`. A test calls `startNet({ nodes })`: a
rendezvous on a free 127.0.0.1 port and nodes, each process the leader of its own process group,
each node with a HOME, a data directory and a `SOLODAWN_FAKECLI_LOG` of its own and the binaries
first on its PATH. It waits until every node answers on its local socket, unless `awaitNodes` is
false, and hands over the addresses: the rendezvous's `url` and `signal`, each node's `home`,
`dataDir`, `socket` and `env`, and `run` for any program on the node's PATH. The test stops them in
an after-hook (`AfterEachScenario`, `afterEach`), which also runs when it fails or times out:
`stop` sends each group SIGTERM, then SIGKILL once its leader has exited or 3 s have passed, and
removes the directory. The run's teardown kills what a test process that died left running.
`launch(inject('browser'))` from `e2e/config.ts` opens the project's browser, and `config.ts` sets
`fileParallelism: false` so two Playwright WebKit UIProcesses never race to create the macOS
Keychain WebCrypto master key on a fresh runner. `pnpm test` leaves `e2e/` out. The tests change
nothing of the test process's own environment; the scenarios of `../features/web_e2e.feature` check
that. `e2e/s2.spec.ts` instead runs the step 2 cast of `features/s2_cast.ts` in each page against a
Vite development server with `?nomock`, except watching, which three people do with real nodes; the
rest of the node side of step 2 is tested by integration.

The runtime dependencies are `valibot`, for the wire types, and `@xterm/xterm` 6.0.0 with
`@xterm/addon-fit` 0.11.0 (MIT) for the terminal page. It is about
337 KiB minified (86 KiB gzipped) with a 7 KiB stylesheet, so `routes/Terminal.svelte` loads it with a
dynamic `import()` when it opens, and the rest of the client never carries it. Signatures, hashes and macs use the
browser's WebCrypto only; no signature library is bundled (R-164). `fake-indexeddb` is a
development dependency, an IndexedDB for the tests.

### Add an area of the interface

A unit of the interface owns its folder `src/components/<area>/`, its texts and, for a page, its
file in `src/routes/`. It edits neither the shell nor another area:

1. **Texts.** `src/messages/<area>.zh-CN.json` and `<area>.en.json`, every key `<area>.…`, and
   `src/messages/<area>.d.ts`, as `chat.d.ts` with its own name. No import to add: the loader
   finds the files, and `t('<area>.…')` is checked by `pnpm check`.
2. **Parts.** `src/components/<area>/mounts.ts`, default export `{ … } satisfies Mounts`, with
   entries at the points it needs and the cards of its kinds. No import to add: `gather` finds
   it, and `#/states` of a development server shows where each entry and card went.
   That sheet shows the messages of the wire examples and the mounting points, not the design
   sheet's colours, type, buttons or the states of the connection and the emergency stop: an
   area that adds a component adds its states there.
3. **A page.** `src/routes/<View>.svelte`, named as its route's `view`; it is given
   `{ params, net }` (`PageProps<'terminal'>` types them). A page that needs another address
   or another layout asks for a change of the route table.
4. **A preference of this browser.** `preference('<area>-…', allowed, fallback)` of
   `prefs.svelte.ts`.
5. **Acceptance.** `../features/web_<slice>.feature`, tagged `@web` on its first line, and
   `features/<slice>.spec.ts`, which loads it with `loadFeature('../features/web_<slice>.feature')`.
   A scenario of `web_shell.feature` fails when a web feature file has no such spec or tag; the
   end-to-end steps (`web_e2e.feature`) are the one exception, run by `e2e/config.ts` from
   `features/e2e.ts`. The steps open the client with `openPage(address, width)` of
   `features/page.ts`, which mounts the shell in the test DOM with English texts and answers the
   media queries of script for a screen that wide, and give it homes with `TestHome`
   and `browser` of `features/home.ts`; `show(component, props)` mounts one part alone. The
   development server's `net` is not used there: a test puts its homes in `homes` itself.

## Contract

- **Static hosting.** Asset paths are relative (`base: './'`) and pages are address fragments, so
  the build works from any path and needs no server rule.
- **Route table.** `routes` in `src/lib/route.svelte.ts` holds the pages of the design
  (`design/index.html`) and the invitation entry point. Each has a path, a file in
  `src/routes/`, a navigation entry (if any), and a whole-screen setting:

  | Page         | Address                            | Drawn by            | Entry     | Whole screen   |
  | ------------ | ---------------------------------- | ------------------- | --------- | -------------- |
  | `onboarding` | `#/onboarding`                     | `Onboarding.svelte` | none      | at every width |
  | `join`       | `#join=<invitation>`, `#/join`     | `Join.svelte`       | none      | at every width |
  | `groups`     | `#/groups`                         | `Groups.svelte`     | chats     | no             |
  | `chat`       | `#/groups/<g>`                     | `Groups.svelte`     | chats     | on a phone     |
  | `terminal`   | `#/terminal/<g>/<member>[/<task>]` | `Terminal.svelte`   | chats     | on a phone     |
  | `nodes`      | `#/nodes`                          | `Nodes.svelte`      | computers | no             |
  | `settings`   | `#/settings`                       | `Settings.svelte`   | settings  | no             |
  | `states`     | `#/states`                         | `dev/States.svelte` | none      | no             |

  An address fits a path when it has as many segments, every word is equal and every parameter
  is given, except an optional last one (such as `:task?`); no address fits two paths. A `#join=`
  fragment is passed intact to the join form; `#/join` alone, which "Join a group" of the group
  list opens, asks for an invitation code. Parameters are
  read with `decodeURIComponent`; `href(page, params)` writes an address with
  `encodeURIComponent`, and `parse` reads it back. An address that fits no path, one with a
  malformed escape, and `#/states` in the build open `groups`. `route` is `{ page, params, dir }`;
  `Params<'chat'>` is `{ g: string }`. The navigation has three entries (`tabs`); a page outside
  them marks none. The shell finds a page's component by file name. Focus goes to the main
  landmark when another page component is shown;
  between the group list and a chat, one component, the chat moves focus itself.

- **Whole screen.** A route's `full` is written to `<div class="app" data-full>`: `phone` hides
  the top bar and the bottom navigation below 768 px (an open chat, whose head is its bar, and the
  terminal), `always` hides them and the side navigation at every width (onboarding and join).
  The banners stay in both.
- **Connection.** The shell decides once, when it starts, where the homes come from, and hands
  the result to every page as `net` (`src/lib/net.svelte.ts`): `sourceOf(mode, search)` is
  `mock` in a development server (`import.meta.env.MODE === 'development'`) unless the address
  has `?nomock` or `?demo`, and `live` everywhere else, the tests included (`?demo` connects live
  to the in-browser simulation of `features/sim/`). `mock` adds one home answered by
  `mockCtl`, read by the person of the wire examples with this browser's device key, and online;
  only a development build has this code, so the build carries neither the mock nor the examples
  (checked in the output of `vite build`). `live` acquires the single-tab Web Lock (`runTab` of
  `tablock.ts`, showing the other-tab banner with "Use here" through `components/status/` at
  `banner` while another tab holds it; "Use here" moves focus to the main landmark, as the banner
  goes; meanwhile the composer says the group is in use in another tab and the group list row says
  so in short, in the secondary text colour rather than the danger colour of a home that cannot be
  reached) and connects to each paired node from `owner.ts` through `peer.ts` (W-2), one
  `session.peer(node, ...)` per node, pausing while `#/onboarding` runs a pairing session and
  forwarding updates to this person's device log on both paired and joined connections.
  The browser sends `hello` with `v: 3` without falling back to older protocol versions
  (`src/lib/version.ts`). `net.homes` is `homes` of `groups.svelte.ts`; `net.path(home)`
  is how this browser reaches a home now (`direct` or `relay`, the round trip, since when), and
  undefined while it does not; the mock's path is direct. `net.source` says which it is.
- **Joining a group.** `readInvitation` in `src/lib/join.ts` reads a `#join=` fragment without
  contacting its home; `readCode` reads a typed invitation code: it removes white space and reads
  the rest as a typed pairing secret, and the page refuses any other text before anything is sent
  (`contracts/step-2/people.md` 2.6). The join page (U2-1) passes the invitation or the code,
  chosen display name and a cancellation signal to `net.join`. The active browser enters the
  room without adopting either device: a link's on this page's rendezvous alone, taking only the
  `pair` message of the home it names; a code's on every signalling path it has, this page's
  rendezvous and the addresses its nodes gave in `welcome`, taking the first node that proves the
  code as the home and the path it answered on (section 4.1). While nothing answers, the page says
  so, for up to 10 minutes. The browser presents its own log and asks the home to admit it to the
  link's group, or for a code to the group `welcome.join` names. On success, it remembers the home
  and that path in IndexedDB (store `device`, key `joined`), notifies its connected own nodes with
  `home.add`, and starts reading the new group on the same connection.
- **Homes heard of.** Each `host.info` of an own node names the groups whose home it knows
  (`contracts/step-2/people.md` section 7, point 2): `net.svelte.ts` dials and remembers those
  homes (`Home.onhomes`), so another browser of the person shows the groups they joined. Until its
  home can be reached such a group is in the list by its name, as one whose home is away; a home
  that can be reached is asked for it at once, so a move made while the tab was closed is followed.
  A group that its home refuses is dropped without a word; a home that ignores this browser stays
  away. The computer name of a home never reached is not known, and left out.
- **Mounting points.** An area of the interface mounts its components without editing the shell,
  the chat or another area: it adds `src/components/<area>/mounts.ts`, whose default export is a
  `Mounts` (`src/lib/mounts.svelte.ts`), and `gather` finds every such file by name. The points,
  and what a component there is given:

  | Point           | Where                                                                      | Props                         |
  | --------------- | -------------------------------------------------------------------------- | ----------------------------- |
  | `bar`           | the top bar, before the theme and language buttons, on every page          | `{ net }`                     |
  | `banner`        | under the top bar, across the page; a banner comes down from under the bar | `{ net }`                     |
  | `layer`         | after the page, above it: dialogs and notifications                        | `{ net }`                     |
  | `chat.head`     | the head of a chat, after the group's name                                 | `{ group }`                   |
  | `chat.top`      | between the head of a chat and its main line                               | `{ group }`                   |
  | `chat.write`    | above the composer: what the writer should know before writing             | `{ group }`                   |
  | `chat.side`     | beside the main line from 1024 px; below, a panel that slides over         | `{ group, net }`              |
  | `member.ai`     | in the rows of each AI card, before the profile rows: `dt` and `dd` pairs  | `{ group, ai }`               |
  | `member.add`    | in Add AI, between the computer choices and the name field                 | `{ group, candidate, draft }` |
  | `member.joined` | after Add AI successfully sends `host.join`                                | `{ group, candidate, name }`  |
  | `cards`         | the card of a message, by kind, in the main line and in threads            | `CardProps`                   |

  Parts at `member.ai` show or edit more of the member. A part at `member.add` may set the draft's
  `model`, `effort` and `tier` before `host.join` sends it; `candidate` is undefined until chosen.
  The model area uses both points at order 10: `MemberModel` displays the member's model and
  level and refreshes its computer's list (editing is in `Profile.svelte`), and `AddModel`
  supplies the chosen candidate's model/level fields.

  The external AI area adds an account type and optional tool label to the same draft. A part
  at `member.joined` receives `{ group, candidate, name }` after a successful `host.join`; the
  external AI area uses it to open setup when the new member row reaches the browser.
  Removing an external AI takes an inline confirmation, which Escape cancels.

  An entry is `{ order, component }`: the lower `order` comes first, then the area's name. A
  component with nothing to show draws nothing; an empty banner takes no room. While any area
  mounts at `chat.side`, the chat has a side column from 1024 px and, below it, a button "People
  and details" in its head that opens the panel over the chat: focus moves to the panel's close
  button, and the head, the main line and the composer behind it are `inert` until a press of
  that button or Escape closes it and focus returns to the head's button. An Escape that a
  `<dialog>` receives, or that a part has already used (`defaultPrevented`), leaves the panel
  open; from 1024 px Escape does not touch it. `cards` maps a kind of message (`MsgRow['kind']`) to a component that
  is given `{ group, msg }`, with `msg` of that kind; `card(kind)` finds it, for a thread too. In
  a message written by a member it is drawn in the body, under the head and the text, in place of
  the one-line stub; a row the home wrote (`by: 'system'`) it draws whole, in place of the quiet
  line. Two areas that mount the card of one kind fail at start, naming both.

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
  implement sections 1 to 4 and 6 of `contracts/envelope.md`, as `internal/identity` does:
  canonical base64url only; the fourteen weak keys are never a device id and a signature whose R
  is one, or whose S is not below L, fails before WebCrypto is asked; `openSignal` and
  `openCommand` answer the first failing check (`bad_sig`, `unknown_device`, `bad_request`,
  `not_for_me`, `stale`), with both ends of the window accepted; `DeviceLog.apply` accepts or
  refuses an entry by section 4 and leaves the log unchanged when it refuses: an entry whose `p`
  is more than 2,048 bytes as UTF-8 is refused, and a `dev` is judged by its bytes and never
  imported as a key, so one that is no point of the curve is added; the entry already held at a
  version is recognised by `p` as it is signed, a lone surrogate counting as U+FFFD, as a Go node
  reads it, and by `k` and `s`; `adopts` is the log's half of
  adoption; a pairing secret is read by removing hyphens and upper-casing a to z only; the `mac`
  of a pair message covers its `role`, `k`, `kind` and `name` as `TextEncoder` encodes them, and
  `verifyPair` ignores a message whose `kind` is neither `node` nor `browser`.
  `src/lib/payload.ts` reads the fields those checks need, with `JSON.parse`: exact names, the
  last of a repeated name, integers as safe integers by value, no `null`, and `n` and `g` as
  random ids. Signing is left to WebCrypto, so in Safari two signatures of one payload differ.
- **Preferences.** `prefs.theme` is `auto`, `light` or `dark`; `prefs.accent`, the theme colour,
  is one of `accents` (`daybreak`, the default, `rain`, `pool`, `shallows`, `shade`, `hills`,
  `glow`, `afterglow`, `firstlight`, `nightfall`); `prefs.lang` is `zh-CN` or `en`.
  Until the person changes a setting, the client follows the system theme and the browser
  language (any `zh` locale opens Chinese) on every visit: nothing is stored on load.
  `localStorage` (`sd-theme`, `sd-accent`, `sd-lang`) gets a value only when the person changes
  that setting, on the settings page or with a top-bar button. The preferences are always written
  to `<html data-theme data-accent lang>`. A stored value the client does not know, from another version for
  instance, is ignored. When the browser blocks site storage, the client opens with the defaults
  and applies changes without remembering them. A part of the client keeps a preference of its own
  the same way with `preference(name, allowed, fallback)`: its `value` is the default until set,
  and setting it stores `sd-<name>`. Made again with the same values, as a module the development
  server reloads is, it is the same preference; made with other values, or as `theme`, `accent`
  or `lang`, it throws. `tokens.css` turns `data-theme` into
  `color-scheme`; every colour is a `light-dark()` pair. `accents.css` turns `data-accent` into
  `--hue` (with `--tint` for `afterglow` and `nightfall`), `--sky` and `--on-sky` (and, for `afterglow` and
  `firstlight`, the waiting colours). `tokens.css` gives every surface (`--bg`, `--surface` to
  `--surface-3`, `--raised`, `--raised-2`), the hairlines (`--line`, `--line-strong`) and the texts
  (`--text` to `--text-3`) an OKLCH lightness and chroma of its own, with the hue `--hue`, so a
  theme colour changes the whole page as plainly as light and dark do (`design/README.md`,
  section 3.1), and mixes the soft fills, `--mine` and the focus ring from `--sky`; a swatch carries its own `data-accent`. The browser's own bar
  (`<meta name="theme-color">`) takes the colour `index.html` gives the theme shown, so a chosen
  theme wins over the system scheme; that colour is the untinted page grey of before, and the
  manifest colours stay fixed (`issues.md`).
- **Wire types.** `src/lib/proto.ts` mirrors `internal/proto`: one `valibot` schema per message,
  with its TypeScript type inferred under the same name. An unknown field is dropped, as Go's
  `encoding/json` does, so a node that adds a field does not break an older client; a missing
  field is refused unless Go tags it `omitempty`; a Go pointer without
  `omitempty` must be there and may be `null`. Message bodies are any JSON until a contract types
  them. `decode(schema, text)` is the only place where received text becomes a typed value;
  `Frame` is what a node may send on `ctl` unasked, its pushes and the error answer; a frame with
  `re` is read with `decodeAnswer(t, text)`, by the type of the request it answers (`AnswerOf`).
  Code after it trusts the type. An envelope's `p` stays the exact string received. `Id` is a
  random id, the canonical base64url spelling of 16 bytes; `n`, `g` and `conn` of a signed payload
  have this type, and so has every id in a command payload and in an inbox item, the `task` in the
  body of a review post included; rows and the other message bodies still hold ids as strings
  (`issues.md`).
  The payload of each command has its
  schema (`GroupCreate`, `Post`, `Act`, `TaskStart`); a schema names the `op`, `kind` or `code`
  that selects it. `Ctl` is the interface of a `ctl` connection: numbered requests, their
  answers, and pushes.
- **Demo.** For clicking through every flow with no node, run `pnpm dev` and open `?demo`: a
  whole simulated world answers behind the live connection (`src/main.ts` loads
  `features/sim/install.ts` before `boot`, which installs a fake `connect` through `simulate` of
  `peer.ts`, a stand-in service worker registration and notifications for reminders). The "演示"
  panel stands for what happens outside the browser: other people, the AIs and the computers.
  Everything above the connection is the client's own code. `?demo=fresh` starts before
  onboarding, `?demo=pairfail` with a computer the browser cannot reach, and `&reset` starts the
  world again. The production build contains none of it. The demo writes the same IndexedDB
  database and `localStorage` as the client on that origin, so a later plain `pnpm dev` session on
  the same port starts paired with the demo's computers, unless you open `?demo&reset` first or
  clear the site data.
- **Mock connection.** `mockCtl(onpush)` in `src/lib/mock.ts` is a `Ctl` that answers from the
  files of `internal/proto/testdata`: `hello` with `welcome`; `sync` with the pushes `reset` and
  `rows`, then `synced`; `fetch` with `fetched`; `host.info` from the examples' node;
  every `cmd` with `ok`, a `group create` after the push `groups`. It knows the examples' one
  group. It is for development and tests only: import it dynamically behind
  `import.meta.env.DEV`, so that the build leaves it and the examples out. `vite.config.ts` names
  that directory `@testdata` and lets Vite read it (`server.fs.allow`). Stryker runs the tests in
  a copy of `web/` under `.stryker-tmp/`; there the name still points at the repository's
  directory.
- **Connections.** `connect(options)` in `src/lib/peer.ts` is the offering side of
  `planning/flows/connect.md` sections A and C, as `internal/peer` is the node's. Only the tab
  that holds the device's lock calls it (`tablock.ts`), and closes the session when it loses the
  lock. It needs the signalling URLs, the device, `known` and the browser's name.
  - **Signalling.** One `rendezvous.ts` client per URL, so that a path opens again by itself:
    after 1, 2, 4 … 60 seconds, from 1 second again after 60 seconds signed in, and at once on
    the window's `online` event. Each path gets the watch list (the nodes that have a peer, 64 at
    most) when it opens and whenever the list changes, and is asked for fresh ICE servers at half
    their `ttl`, then every 10 seconds until they come. `heard(url, frame)` gets every frame of
    every path and `session.send(url, frame)` sends a `join` or `room` on one: that is all this
    module knows of pairing.
  - **A peer.** `session.peer(node, listen, onpath)` keeps one connection to a node. It offers on
    the paths that are signed in (none: the attempt has failed), with one `ctl` channel and a new
    `conn`; the offer and each candidate are signed and sent on every such path, and no end of
    candidates is sent. An offer without an answer after 10 seconds, or reported offline by every
    path it went out on (a path that closes counts), is retired, and its late answer ignored. The
    first copy of the answer is used. A candidate of the node waits for the answer and is applied
    once, 64 at most for a connection; one for a connection the browser did not offer is dropped,
    as a browser answers none. When `ctl` opens the peer says `hello`; a refusal, an unreadable
    `welcome` or 10 seconds without one closes the connection. A `hello` refused with `version`
    or `forbidden` also stops the peer: the requests still waiting reject, a listener from
    `listen()` hears `refused(code)`, when it has one, and that node is offered nothing more until
    a new peer is made for it. After any
    other failure or a loss (`ctl` closed, the connection `failed`) it offers again after 1, 2,
    4 … 60 seconds, from 1 second again only after `ctl` stayed open for 60 seconds, and at once
    when a path reports the node present or the window gets `online`.
  - **Listeners.** `listen()` is called for each connection the node has welcomed and returns
    what hears it: `online(ctl, welcome)`, `push(frame)`, `offline()`. A `Home.connect` of the
    group caches is one. `ctl` is the peer itself, the same for every connection.
  - **Requests.** `peer.request` numbers a request per connection, keeps its type under that
    number and resolves with the response that names it, read with `decodeAnswer` of `proto.ts`
    as the answer to a request of that type or as an error of the node; `peer.ts` holds no answer
    shape of its own. It rejects, and never invents an answer of the node: with
    `connection lost` while no connection serves, when the peer closes, and for a `sync` whose
    connection is lost (the listener reads again from its cursor at `online`); with
    `unreadable answer` for a response `decodeAnswer` refuses; with a `RangeError` for a request
    over 60 KiB. Every other request whose connection is lost is sent again after the next `welcome`,
    after the listener's own `sync` requests, in the order first made. The group caches word a
    rejection as `lost`; that code is theirs, not the connection's. `connection.send(frame)` and
    `peer.send(frame)` send a `devlog` or `pty.resize` push without a request `id`, throwing a
    `RangeError` above 60 KiB.
  - **What arrives on `ctl`.** A message with an `id` is a request, which a browser never
    serves: `bad_request`. A message with `re` is a response; one that names no waiting request
    is dropped. Anything else is a push when `Frame` reads it and is dropped when not. `ctl` stays
    open through all of it.
  - **Path.** `onpath` gets `{ kind, rtt, since }` when `ctl` opens and every second after, and
    `undefined` while no pair is selected or no connection serves. `kind` is `relay` when either
    candidate of the selected pair is a relay candidate and `direct` otherwise; the pair is the
    one the transport statistics name, or the one marked `selected` where there are none.
  - **For the probe and the move** (unit W-3, not built: the shell dials with `probe` false only):
    `peer.dial(probe)` offers a connection
    that is not the peer's own, a probe without TURN entries; `connection.opened`, `closed`,
    `hello()` and `request()` drive it; `peer.adopt(connection, welcome)` makes it the one that
    serves and closes every other connection the peer dialled; `peer.hold()` keeps a loss from
    starting a reconnect until its release, which counts once. `peer.channel(label)` opens a data
    channel beside `ctl` (the terminal).
  - **Closing.** `peer.close()` and `session.close()` close every connection a peer dialled,
    probes included. A closed peer stays closed: `dial` throws, and `adopt` closes the connection
    it is handed, so a welcome that comes after the tab lost its lock connects nothing. A live
    peer does not adopt a connection that is already gone (`connection.gone`): the one that
    serves stays.
- **Fonts.** Geist and Geist Mono 1.9.2, one variable `woff2` each, are served from
  `public/fonts/` with their licence (OFL-1.1); the page requests nothing from a third party. A
  browser downloads a face only when text uses it and shows the system fonts until it arrives.
  Chinese is drawn by the system's font (`--font-sans` in `tokens.css` names PingFang SC,
  Hiragino Sans GB, Microsoft YaHei and Noto Sans CJK SC). The weights in use are 400, 500 and 600.
- **Icons.** `src/components/Icon.svelte` holds the line icons as path strings copied from Lucide
  (ISC, parts MIT from Feather), under the names the client uses; no icon package is installed or
  loaded. The licence text is in `src/components/ICONS-NOTICE.md`. The vendor marks in
  `Avatar.svelte` are those of `design/icons/NOTICE.md`.
- **Texts.** Every interface string is a key in both languages and is read with `t(key)`. The
  shell's are in `src/messages/zh-CN.json` and `src/messages/en.json`, under the namespaces
  `app`, `nav`, `bar`, `page` (the title of each page), `pref` (the names of the preferences),
  `upgrade` and `storage`. Each area of the interface has `src/messages/<area>.zh-CN.json` and
  `<area>.en.json`, whose keys all begin with `<area>.`, and `<area>.d.ts`, which adds the keys
  of its English file to the type `Key`. Each area has its own pair of files in `src/messages/`
  (`<area>.en.json` and `<area>.zh-CN.json`); the folder shows which areas exist.
  `src/messages/files.glob.ts` finds every file by
  its name, and `src/messages/texts.ts` merges them. It throws, naming the file, when an area takes a
  namespace of the shell, holds a key outside its own name, holds a value that is no text or an
  empty text, or has keys that differ between its two languages, and naming the area when it has
  no file in one of the languages. No key can therefore be given twice: a collision fails at start and in the tests,
  never replacing a text. A text with a number in it carries `{n}`, which its component replaces. Chat
  content is never translated. The options of the language choice are named in their own
  language in both files and carry its `lang`, so a screen reader speaks each in its language
  (WCAG 2.1 SC 3.1.2). The language button of the top bar has no `aria-label`: its name is its
  visible text (`bar.lang.other`, marked with the language it names) followed by a visually
  hidden "Switch language / 切换语言", each half in its own language from its own file
  (`textIn`), so speech input can use the visible text (SC 2.5.3).
- **Motion.** No animation library is bundled: CSS transitions and keyframes move the parts, the
  browser's View Transitions cross-fade the page, and `svelte/motion` reads the system setting.
  `svelte/animate`'s `flip`, part of Svelte, moves a group to its new place in the list; it runs
  in script, which the styles cannot still, so its duration is 0 under reduced motion. What
  arrives rises from `--rise` below its place (`rise` in `base.css`): the rows of the group list
  one after another, a pill over the composer, a question asked in a task card. A message drawn
  within 3 s of being written comes in from its own side (`live` in `Message.svelte`); what a chat
  opens with and a page of older messages are simply there. A layer that opens from a trigger (a
  menu, the stop panel, the list of mentions) grows from the trigger's corner (`popin`); a dialog
  fades in a little smaller than it is, and below 768 px it is a sheet that comes up from the
  bottom edge. A count that changes lands with `--spring`, and so does the outcome pill of a card
  whose state changes while it is in view (`components/changed.ts` marks the card
  `data-changed`). What leaves through a Svelte block leaves at once, except a notice and a
  confirm card over the terminal, which fade out in 140 ms: the test DOM never ends an exit
  transition. Nothing loops but the dot of an AI at work and the spinner of a pairing step.
  The tokens are in `tokens.css` beside the others: five durations (`--dur-1` 120 ms hover and
  press, `--dur-2` 200 ms a change of state, `--dur-3` 320 ms arrival and cross-fade, `--dur-4`
  500 ms spring, `--dur-out` 140 ms what leaves), `--ease` for a change of state, `--ease-out` for
  an arrival, `--ease-in` for an exit, `--spring` for what a person moved, `--travel` (12 px, a
  page and its parts), `--rise` (8 px) and `--stagger`. Only `transform` and `opacity` are animated; a colour
  that changes on hover, press or focus is a layer under or around the part whose opacity changes.
  Keyboard focus is seen in full the moment it lands: the skip button is in place and the ring
  is opaque at once, and only the ring's settling onto the part is animated.
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
  keyframes, no cross-fade, and a cross-fade under way ends at once (the rule for its pictures is apart
  from the rule for every part, which a browser without view transitions would otherwise drop); the logo does not turn to
  the pointer and a press does not sink a button, which shows the press by its layer alone.
- **Group caches.** `src/lib/groups.svelte.ts` holds what `planning/flows/sync.md` asks of a
  client. The code that owns the connections makes one `Home` per node, with the person's user id
  and a `sign` for their commands, and puts it in `homes`. For every connection to that node it
  calls `home.connect()` before the connection exists, gives the link's `push` to the connection
  as its push handler, and calls the link's `online(ctl, welcome)` after `welcome` and its
  `offline()` when the connection is gone.
  - Only the last connection to call `online` is the home's: what an earlier one still pushes is
    dropped, and its going leaves the home online, so a move to a better path cannot mix two
    reads. The owner of the connections therefore calls `online` only for the connection that is
    to become the home's: for a probe, when it takes over. `online` lists the node's groups and reads each with `sync` from its cursor; a `groups`
    push replaces the list, keeps the rows of a group listed before and reads a new one at once.
    A group is in the list of one home: of two within reach that list it the first keeps it, and
    from a home that is out of reach, or no longer lists it, the one that lists it takes it over.
  - A `Group` applies a row only with a higher `seq` than the copy it holds, and keeps a `gone`
    marker in place of its row, unseen. Its `cursor`, the `since` of its next `sync`, is 0 at the
    start and from every `reset`, S at `synced`, then the `seq` of each live row applied; rows
    before `synced` and pages never move it, so a reset that breaks off starts again from nothing
    and a delta that breaks off keeps its place. The answer to a read moves the cursor only when
    no later read of the group was started and its connection is still the home's. After `synced`
    it fetches, one row each, the requests of the waiting confirm cards that the read did not bring.
  - `older(thread)` fetches one page of a thread, or of the main line, and then the page before
    the lowest `pos` received. No page is asked for while the group is being read: the paging
    starts again at `synced`, from the rows the read brought. The main line starts before the oldest of the newest 50 messages
    held, which a cold read brings without a gap; not before the oldest message held, which can be
    a card still pending from long ago. A thread starts at its newest page, `before: 0`. An empty
    page ends the paging; a page the home refuses stops it (`failed`) until the group is read
    again; a page that arrives after a later read, or after its connection is gone, is dropped.
    `paging(thread)` says whether more may exist, whether a page is on its way, and whether one
    can be asked for (`ready`). `gap` is the `pos` where the next page of the main line starts
    (starting from the oldest of the newest 50 held), or `0` once its last page is in; messages of
    the main line with a lower `pos` stand above a gap.
  - The lists and lookups: `main` and `thread(task)` in `pos` order, `members` by name,
    `member(id)`, `nameOf(id)` (the name to show: of a member that left, the name of the last row
    this page held of it, else "Former member"), `personName(user)` (also of a person who left
    while this page held their row, by the name they left with last), `task(id)`, `me`, `waiting` (the
    pending brief the person asked for, the pending approvals, decisions and confirmations put to
    them), `info`
    (the group row), `latest`. `recent()` is every group, the newest main line first.
  - `post` and `create` send signed commands, shaped as the wire examples `post-text.json` and
    `group-create.json`, and answer what the node answered; the client draws from the rows that
    come back, never from its own command. A request whose connection is lost is answered
    `lost()`, a code of the client's own. While `home.ctl` is undefined the group is shown from
    the rows held, read-only.
- **Group pages.** `#/groups` is the group list and `#/groups/<group id>` the chat of that group;
  `routes/Groups.svelte` draws both, from `params.g`. While the group the address names has not
  been read, the chat's place says so, with the way back to the list.
  - Width: from 1280 px the list stays beside an open chat; below it an open chat has the page
    and its back button leads to the list. With no group open the list has the page below 900 px,
    with the cards that wait as a bar above it; from 900 px they are an overview beside the list.
    On a phone an open chat also takes the place of the top bar and the bottom navigation: its
    route asks the shell for the whole screen.
  - The main line is a reversed scroller with one child, so the newest message is in view when it
    opens, and a page added above moves nothing. While the top of what is read is in view, or
    within 600 px of it, the page before is fetched; when it cannot be, because the home refused
    a page or cannot be reached, the top says so and never claims to be the start. Messages stand
    under the day they were written on. A day keeps its key when messages are added before or
    after it, so the messages already drawn stay as they are. Its name stays at the top of the
    line on a band of the page's colour, under which the messages pass. A jump to a message puts
    it in the middle of the main line, as far as the line reaches, and keeps it there while pages
    and messages arrive above and below it, until the person scrolls the line, sends a message or
    jumps elsewhere; the browser's scroll anchoring is off meanwhile. The top edge always, and
    while `group.gap` is non-zero any message with `pos < group.gap` and any day whose messages all
    lie above the gap, disable scroll anchoring (`overflow-anchor: none`), so a page landing at the
    top or in a gap below a card fetched alone leaves what is read below it in place. A message the
    jump puts at the end of the line is not held: the line keeps it and the newest in view. A message within five
    minutes of the one before it, by the same author, is drawn under it without a head; its time
    shows under the pointer, and for all such messages while keyboard focus is on the main line
    or on a button of the chat. That, and the focus ring of the main line, are for someone who
    has pressed a key since they last used the pointer, not for the focus the main line is given
    when a chat opens.
  - "Today" and "yesterday" are read from `now()` of `when.ts`, one clock that moves once a
    minute while something drawn reads it.
  - A text shows its code spans and the mentions of the members its row names, drawn by the
    shared `Rich.svelte` in the main line, the thread and the call block. The composer
    completes a mention from the members as it is typed (arrow keys, Enter or Tab, Escape),
    sends with Enter where the person has a keyboard and with the button on a touch screen, and
    never on a key that an input method is composing with. What is typed while a message is on
    its way stays in the field. It estimates the row by `contracts/rows.md` section 5 (`rowSize`
    in `compose.ts`): the text twice, 52 bytes for each member mentioned, and 627 for the rest of
    a row, measured on the wire example with the longest sequence numbers. It tells the writer
    from three quarters of the 16 KiB and does not send beyond them; the size the node measures
    decides.
  - The create-group dialog holds the computer chosen as the home, not its place in the list, and
    sends nothing when that computer is no longer among those that can be reached. A computer
    that appears while there was none to choose from is chosen. The dialog also lists the people who are
    already in one of the person's groups on that computer and the person's connected managed AIs
    (never an external one), each with a tick and, for each ticked one, its folder
    (`FolderPicker.svelte`), name, model, thinking level, work share (not for the secretary) and a
    secretary choice, and "Also make an invitation code" with its lifetime, off by default. It creates the group first, then adds
    each person and each AI, then sends `code new` when the box is ticked (`found` in
    `src/lib/members.ts`), and says beside any of them that was refused. A code that was made is
    shown with its copies before the dialog closes; a refused code is said under the box.
  - The shell keeps the person's computers told the homes they need (`tell` in
    `src/lib/nodes.svelte.ts`): those of groups on another person's computer.
    What a computer's `host.info.homes` lacks is sent with `home.add`, once per connection,
    whatever it answers.
  - Only text messages are drawn in full. A card is one line that names its kind and its subject,
    with the dawn edge when it waits for the reader, and a row the home wrote is a quiet line;
    the cards the areas mount replace both. An AI-to-AI `call` (`src/components/call/`) draws its
    route, wait mode, depth, question and either its `waiting` indicator with elapsed time since
    `ts`, counted every second while it is under a minute and every minute after, or its
    `answered` duration with the `reply` folded inside, hiding the standalone `reply` row when
    both are in the same list. A denied call shows “Denied” instead of a timer or reply.
  - Confirmation cards (`src/components/confirm/`) name who asks to act, answer a call, or watch a
    terminal (`confirmSentence` in `src/lib/confirm.ts`, also the card's subject in the lists of what
    waits). The watch card names its author; other cards name the author of the referenced request,
    or who confirmed it when it is a brief, and the task in place of a person when it is the `task`
    message of a task without a brief. Only a reader named in `body.to` sees the pending card's
    Confirm and Deny actions; everyone sees whether it is pending, confirmed, denied or expired.
  - The member panel (`src/components/member/`) mounts at `chat.side` (`order: 10`). It lists the
    group's AI members, placing the secretary first with its corner mark, with each AI's state
    (`working`, `waiting` on the reader in dawn or on another member, `dormant` as sleeping,
    `takeover`, `limited`, `offline`), computer with path (`local`, `direct`, `relay`, `offline`),
    account type, a usage bar (`.meter` in `src/styles/base.css`, from `usageMeter` in
    `src/lib/members.ts`: the rate-limit window percentage with its reset time, in danger tone at
    100 percent and omitted when `usage` has no `window_pct`; a `meter` to assistive technology,
    named by its label, with the percentage and the reset time as its value text), and
    for owned AIs whose driver has a takeover (`offersTerminal` of `src/components/driver/`) an
    "Open terminal" link enabled only when the browser's own path to that computer is `direct`, or,
    while the AI is in `takeover`, a "Hand back" button that sends `takeover.end` and says that
    messages to it wait until then; then the human members with their "You" and role tags;
    actions and errors go below the person's name. Owners and admins open "Invite"
    (`InvitationDialog.svelte`): first the group's invitation code (`contracts/step-2/people.md`
    2.6), read with `code show` when the dialog opens and again each time the home is reached; with
    none open, a lifetime of 1 hour, 24 hours (chosen) or 7 days and "Create code"; with one open,
    the code (`CodeCard.svelte`), until when it is valid, "Copy code", "Copy invitation" (one line
    of instructions with the date, the code on a line of its own), "New code", which says first
    that the current code stops working, and "Turn off". `sendCode` in `src/lib/members.ts` follows
    a `new` answered without the code, a repeat answered from its receipt, with `show`. Below the
    code come the people of the person's other groups, then a one-time link with a countdown. The
    confirmation of removing another person says that an open code is ended with it; leaving says
    nothing of it. The owner's row has no role or removal action. Owners can promote, demote, and
    remove other people; admins can remove ordinary members; non-owners can leave. Signed commands
    go to the group home, and its row pushes update the panel. The access editor
    (`src/components/allow/`) places each AI's allow list and approval delegates alongside its
    computer and account. Only its owner sees "Edit access" next to the terminal action. The home
    applies the allow list only on a team or API-key account and always retains the owner as an
    approver. The panel's "Add AI" button requires a reachable group home, a group member, and at
    least one connected owned computer, and shows its reason when disabled. The add-AI dialog reads
    `host.info` from the owner's connected computers, suggests an unused member name in the group,
    offers the secretary switch when the group has none or the reader is an admin, asks for the folder
    the AI works in (`FolderPicker.svelte`: the folders its CLI has used there, or one typed), signs
    `host.join` for the chosen computer, and words `home_unreachable`, `conflict` and a lost computer directly
    before falling back to `Failure`.
  - AI profiles (`src/components/profile/`) show on every AI card, first under its name, the tier
    worded as the work share ("Less / Normal / More": how much work the secretary gives it) in one
    block with its usage line (`Quota.svelte`: window used and its reset, turns today, or none
    reported), and its skills; a secretary takes no tasks, so its card shows the usage line alone,
    headed "Quota", and no editor or dialog offers it a work share. Only the AI's owner can open
    the inline editor of model and thinking level (not for an external AI:
    `contracts/step-3/external.md` section 11), work share (with the usage line, not for the
    secretary) and skills, or accept a differing model-tier suggestion from `host.info`; accepting
    sends only the tier, and saving sends only what changed. The add-AI dialog uses the same fields
    and sends tier and skills with the chosen model and level, and the create-group dialog gives
    each ticked managed AI model, level and work share (no skills), sent in its `host.join`. The
    skills counter counts code points; over 200, Save and Add are disabled.
  - Model choices (`src/components/model/`) show the CLI's model names, levels and fast-mode
    capability. The thinking level is always shown beside the model: the chosen model's levels and
    "The CLI's default", disabled with its reason when the model lists none or none is chosen, and
    a stored level the list does not hold as "Not listed". Empty catalogs offer model and level text inputs; stored models absent from a
    catalog remain visible as "Not listed". Choosing a model in Add AI fills its suggested tier,
    which the owner can override. The same CLI can be selected repeatedly with distinct member
    names; the candidate reads "In this group · adds another member". Computers lists each hosted
    member separately. Add AI passes the draft through the mount with a getter/setter, and the
    model fields bind it back to the dialog. Empty-catalog text inputs use native pattern validation
    for the model's 100-code-point and level's 32-code-point limits, including astral characters.
    The card's refresh action sits beside the model value. Only the AI's owner can edit model/level
    (in the profile editor) or refresh its catalog. Refresh sends `host.info` with `refresh: true`, updates the live list
    without restarting, and never changes the stored model, level, tier or skills.
  - Task cards (`src/components/task/`, `src/lib/task.ts`) show "Must ask a stronger AI" in dawn
    when an active task has `stuck.ask` true, after wait badges and before a rejected-review
    badge. A false or absent `ask` leaves the normal badge selection unchanged.
  - The group info panel (`src/components/groupinfo/`, `src/lib/groupinfo.svelte.ts`) mounts at
    `chat.side` (`order: 20`) below the member panel. Its folded "Project memory" section fetches
    `note` and `review` ledger pages on demand (`fetch` with `kind`), omits `gone` markers, and
    lets an admin delete a note or review entry with a signed `forget` command after a `<dialog>`
    confirmation. Any person of the group adds a note (`note`) and changes one in place
    (`note_edit`); the group details (order 19) let an admin change the group's name and
    secretary in place (`group set`). An edit in place is `EditButton` and `EditField`: Enter in a
    line, Ctrl or ⌘ with Enter in a text, or Save sends it; Escape or Cancel leaves the value; a
    refusal stays beside the field; focus returns to the button that opened it. Review ledger
    entries are signed by the reviewing AI and are not edited.
- **Terminal takeover.** `#/terminal/<g>/<member>[/<task>]` (`routes/Terminal.svelte`,
  `lib/terminal.svelte.ts`, `components/terminal/`) loads `@xterm/xterm` and `@xterm/addon-fit`
  with a dynamic `import()`. `Takeover` in `lib/terminal.svelte.ts` is the takeover of one
  address: it refuses a relayed path, signs `takeover.start` (with `task` when the address names
  one), opens the binary `pty:<g>:<member>` channel, pushes `pty.resize` when the channel opens
  and when xterm's size changes, carries bytes both ways with the phone key row and a sticky
  `Ctrl`, and closes the channel, keeping `ctl`, when the path becomes `relay` (A7.9). It sends
  `takeover.end` on "Hand back", on `pagehide` and when the page closes or moves to another
  address; while `takeover.start` is still unanswered, `takeover.end` waits for the answer, so a
  hand-back cannot reach the computer before its terminal opens (A7.6). The page says that
  messages to the AI wait until hand-back, without a count: the node pushes no `host.info`
  (`../issues.md`). Tab and Shift+Tab are typed into the terminal, so Ctrl+M, which a terminal
  reads as Enter, is kept from it and moves focus to the takeover bar's button; a note in the
  details column says so and describes the terminal's input field. Faint text in the default
  colour is drawn in `--term-dim`, 5.6 to 1 on `--term-bg`, where xterm's own half opacity gives
  4.1 to 1 (`../issues.md` for faint text in a colour). The key row scrolls; the details column
  scrolls and takes keyboard focus for that.
- **Connection indicator and computers page.** `indicator(net, group)` in `src/lib/nodes.svelte.ts`
  gives the pill mounted at `bar` (and at `chat.head` below 768 px, where an open chat hides the
  top bar) its state, linking to `#/nodes` with an accessible `Connection status: …`
  label: `offline` (`Inactive`) while `net.otherTab` is true; for an open `#/groups/<g>` chat,
  the path to that group's home; elsewhere, the summary across `net.homes` (`undefined` before
  any home is known, `down` when no target home is
  connected, `some` (`Some computers unreachable`) when only some are, `linking` while any connected target has no selected pair yet, `limited` when any
  uses a relay, and `ok` with the first measured `rtt` when all are direct). `routes/Nodes.svelte`
  calls `nodes.sync(net.homes)` to read `host.info` once per online owner connection, lists owned
  computers with their link path, hosted groups and registered AIs, sends `agent.set` followed by
  a fresh `host.info` read when the owner chooses an unset AI account type, offering only the
  types in the AI's `driver.accounts` (`offeredAccounts`, as the pairing cards do), and opens
  `PairBrowserDialog` (`pair.browser`).
- **Emergency stop (R-64).** `src/components/stop/` and `src/lib/stop.ts`. "Stop AIs" is in the top
  bar (`bar`, `order: 20`) and, below 768 px, in a chat's head (`chat.head`). It shows while there
  is something to stop: for an admin, the open group while it is not paused; for an owner,
  a computer whose AIs are not offline. Its panel (a `popover`) is the confirmation: each action
  says what it does and is sent at once. `setPaused` sends `group pause` and `group resume` to the
  home; `nodes.setOffline` sends `host.offline` and `host.online` to a computer of the person and
  reads its `host.info` again, whose `offline` is what the page shows. A paused group says so to
  everybody above the composer (`chat.write`), with "Resume" for an admin, and in the group list.
  A computer with its AIs offline says so under the top bar (`banner`, with "Bring back online",
  not on the computers page), on its card, which also takes them offline with a confirmation in
  place, and on the owner's AI cards. The node pushes no `host.info` (`../issues.md`): when the
  groups show an owned computer's AIs going offline or coming back, it is read again, so another
  browser of the owner follows.
- **Waiting for a busy AI (R-72).** `src/components/queue/Waits.svelte` and `src/lib/queue.ts`. Under a
  message of the reader's own in the main line or a thread: that an AI busy with a task has not taken it, and
  "Go first, pausing T-12", which sends `jump`. No row names what a home has queued, so it is worked
  out: a message waits for an AI it went to while the AI's row is older than the message (or than
  its owner's confirmation) and the AI is at work. Every waiting message offers it, once for all
  the AIs it waits for, as one `jump` sends all its items again; not for an external AI, which a
  jump does not interrupt, nor in a paused group or the thread of a paused or failed task, where the
  message is held. The choice says what it does: it pauses the task, or interrupts a review, the
  session of the thread it stands in, or several tasks. `conflict` means that it was taken meanwhile,
  and the line goes; any other refusal shows while the message waits. After a jump the line says that
  the AI takes the message next, then that the task is paused, until it continues.
- **What a person does with a task.** `src/lib/taskops.ts`, `TaskMenu.svelte` and
  `TaskActions.svelte`. The menu in the head of a task card (a `popover`) holds what the reader may
  do: for an admin pause, resume, reassign and cancel as `contracts/task-machine.md` allows them in
  the task's state, and for an owner the terminals of their own AIs on the task, with the task in
  the address. Without either there is no menu. Pause and resume are sent at once; a cancel asks on
  the card and says what it does; a reassignment names assignee and reviewer and says whose AI stops
  and which owner is asked. A line on the card says why a paused, stopped, queued or waiting task
  stands still (`taskHold`; a wait for a card is the badge's) and holds the one action that moves it
  on. A refusal is shown and changes nothing. The menu reads the connection from a context
  (`components/net.ts`, given by the shell). In an open thread the same `Composer` posts into the
  thread; it offers the task's own AIs first and says when a message wakes nobody, waits for a
  paused task, or carries on one that the round limit paused (`replyNote`). A thread once opened is
  kept while it is folded, with its draft.
- **In-page notifications.** `src/components/notify/` mounts `Notifications.svelte` at `layer`
  (`order: 10`), backed by `Notifier` in `src/lib/notify.svelte.ts`. After a group's first sync it
  shows the live events for the reader: `approval`, `decision`, `brief` and `confirm` waiting on the
  reader, in dawn, until opened, dismissed or resolved; `done` summaries and `mention`s, in calm
  neutral, and `alert`s, for 8 seconds. An alert is a notice of the home that mentions the reader
  (a task that stopped, an AI whose sign-in failed), in the words of its row (`src/lib/system.ts`)
  and the red of its mark. The 8 seconds are held while focus or the pointer is on the notification
  and start again in full when both leave. Main-line messages of the chat open on screen stay quiet,
  as does a request to watch the takeover whose terminal page is open (`onOpenTerminal`): that page
  shows the card, and leaving it would hand the terminal back;
  thread mentions and other groups still notify. Open jumps to the card or message. At most three are shown: a
  newcomer past that drops the oldest self-closing one, or the oldest waiting card when none is
  self-closing. The stack sits at the top right of the page's area, under the top bar and the
  banners, so it never covers the composer or "Use here". On a chat it hangs under the chat's
  head at every width. From 1024 px it is as wide as the cards of the side column it lies over,
  so it covers neither the head's buttons, the waiting bar nor "Add AI"; this holds only at
  1024 px and wider. Below 1024 px the side panel opens over it, and while the panel is open the
  stack is inert, so Tab does not reach it. Its polite live region is always in the page, apart from the shell's
  status region, and it shows nothing while another tab holds the connection. "Open" and the
  close button of a notification are described by its title. Closing one from the keyboard moves
  focus to the next notification's close button, else to the one before, else to the main
  landmark; a close by pointer leaves focus alone. Notifications slide into place and fade out,
  both at once under reduced motion.
- **Web Push reminders.** `src/sw.ts` (built by `vite.config.ts` into a self-contained `sw.js` at
  the root of `webdist/`), `src/lib/push.ts`, and `src/components/reminder/` implement the browser
  side of `planning/contracts/commands.md` section 1.7 and A8.3. `ReminderCard.svelte` on
  `routes/Settings.svelte` turns browser notifications on or off, selects which of the six
  event kinds (`mention`, `approval`, `decision`, `brief`, `confirm`, `done`, kept via
  `preference`) send reminders, in a group named by "Notify me when", and shows the iPhone/iPad Home Screen hint
  (`risks.md` K-20). `loadVapid` keeps a
  P-256 VAPID key pair in IndexedDB (`solodawn`, store `device`, key `vapid`); `Reminders`
  subscribes through `PushManager` and sends `push` (`PushOn` or `{ off: true }`) to connected
  homes, while `ReminderSync.svelte` (mounted at `layer`) keeps newly connected homes in sync and
  jumps to `#m-<msg>` when a notification is clicked, in the chat or, for a request to watch, on the
  terminal page that is open, fetching that one message when the read did not bring it
  (`Group.seek`); of a task's card the thread opens. A request chosen on the terminal page that this
  browser does not hold is fetched, if need be after the home is reached again, before the page is
  left.
  `sw.ts` decodes `Reminder`, displays `group · kind` with `who: title` in the reader's language,
  with words for `code` in place of the kind when it has some (without a code, a mention with a title
  is the notice of a task that stopped, and says so), alerts again for another reminder of the same
  message or thread, and focuses an open tab or opens `#/groups/<g>` at the card.
- **First-run guide.** `guide` is mounted at `bar` (`GuideChip`) and `layer` (`Welcome` and
  `GuideDock`), persisting `sd-guide-state` (`new`, `open`, `closed`, `done`) via `preference`.
  Step state is derived in `src/lib/guide.svelte.ts` from live computers and groups; `Welcome`
  checks IndexedDB (`isBrandNew`) so neither invited peers nor already paired browsers see it,
  and step 4 uses shared `creating` state on `groups.svelte.ts` to open the Create Group dialog on
  `#/groups`. `src/components/guide/beacon.ts` highlights step targets (`[data-guide="defaults"]`,
  `.ai-row fieldset.acct`, `.composer:not(.in-thread)`) inside the guide layer without modifying
  target pages.
- **Where code goes.** Logic lives in `.ts` and `.svelte.ts` modules, which the gates measure
  (CRAP and mutation); components stay thin. A module that only holds an `import.meta.glob` is
  named `*.glob.ts` and is not mutated; it holds no logic. Stryker would wrap the glob's
  pattern, which Vite must read as written, so the glob lives alone in such a module
  (`src/messages/files.glob.ts`, `src/lib/mounts.glob.ts`; `stryker.config.json` leaves out
  `src/**/*.glob.ts` beside `mock.ts`), and the logic that reads its result takes it as an
  input, which the tests replace. Shared styles are global classes in
  `src/styles/base.css`, taken from the design; a component's own layout goes in its `<style>`.

## Understand the implementation

| Path                          | Role                                                                                |
| ----------------------------- | ----------------------------------------------------------------------------------- |
| `index.html`, `src/main.ts`   | entry: styles, then `boot` with the page's crypto, CSS, storage                     |
| `src/lib/boot.ts`             | browser check, device key; mounts `App` or `Notice`                                 |
| `src/lib/idbtx.ts`            | IndexedDB open, transaction and close for `solodawn` store `device`                 |
| `src/lib/identity.ts`         | device key in IndexedDB, base64url, sign, verify, open                              |
| `src/lib/devlog.ts`           | `DeviceLog`: apply, replay, valid devices, adoption                                 |
| `src/lib/pairing.ts`          | pairing secret, room id, mac, pair message check                                    |
| `src/lib/owner.ts`            | owner device log and node signalling URLs in IndexedDB                              |
| `src/lib/join.ts`             | invitation reader, room pairing, log presentation, `join` and `home.add`            |
| `src/lib/version.ts`          | hello protocol version 3                                                            |
| `src/lib/onboard.svelte.ts`   | onboarding pairing session, `#pair=` capture, `agent.set`, CLI and account labels   |
| `src/lib/payload.ts`          | the payload fields the envelope and log checks read                                 |
| `src/lib/prefs.svelte.ts`     | theme, language and `preference`, their persistence, `t`                            |
| `src/lib/guide.svelte.ts`     | five-step guide state, step derivation, and brand-new check                         |
| `src/components/guide/`       | `Welcome`, `GuideDock` (pill + sheet), `GuideChip`, `GuideCard`, `beacon`, `mounts` |
| `src/messages/texts.ts`       | the one loader of the texts: checks and merges the files                            |
| `src/**/*.glob.ts`            | an `import.meta.glob` alone: the areas' texts and mounts                            |
| `src/lib/route.svelte.ts`     | route table, `parse`, `href`, the page open, the way it went                        |
| `src/lib/net.svelte.ts`       | where the homes come from: `sourceOf`, `start` (`mock` and `live`), `runTab` lock   |
| `src/lib/mounts.svelte.ts`    | mounting points, `gather` of the areas' mounts, `card(kind)`                        |
| `src/lib/motion.ts`           | `spring`, the curve of `--spring`; `shift`, the page cross-fade                     |
| `src/lib/jump.ts`             | `jumpToMessage`: scroll, focus and hold `#m-<id>` or a task's thread; `release`     |
| `src/lib/proto.ts`            | wire schemas and types, `decode`, the `Ctl` interface                               |
| `src/lib/mock.ts`             | wire examples by name, their schemas, `mockCtl`                                     |
| `src/lib/rendezvous.ts`       | signalling client: sign-in, pacing, the receiver's checks                           |
| `src/lib/peer.ts`             | signalling paths, connections to nodes, `ctl`, path, reconnect                      |
| `features/peer-fakes.ts`      | a peer connection and its channels for the tests of `peer.ts`                       |
| `src/lib/groups.svelte.ts`    | `Home`, `Group`: row caches, cursor, paging, lists, commands                        |
| `src/lib/members.ts`          | AI card status, path, candidate list, free name, `host.join`, invitation code       |
| `src/lib/stop.ts`             | emergency stop: pause a group, which computers have their AIs offline               |
| `src/lib/queue.ts`            | which of the person's messages wait for a busy AI; `jump`                           |
| `src/lib/groupinfo.svelte.ts` | memory paging; `note`, `note_edit`, `forget`, `group set`                           |
| `src/lib/nodes.svelte.ts`     | connection indicator, `host.info` cache, `agent.set`, pairing                       |
| `src/lib/system.ts`           | the words of a notice of the home, by its code                                      |
| `src/lib/notify.svelte.ts`    | live in-page notification store, classification, dismissal and jump                 |
| `src/lib/compose.ts`          | mention being typed, mentions and parts of a text, row size                         |
| `src/lib/when.ts`             | times in the reader's language; messages by day                                     |
| `src/lib/task.ts`             | task card derivations: badge, three-step bar, review and thread                     |
| `src/lib/taskops.ts`          | what a person may do to a task, why it stands still, replies in its thread          |
| `src/lib/call.ts`             | AI-to-AI call derivations: elapsed time, route, question and inline reply           |
| `src/lib/terminal.svelte.ts`  | terminal takeover: commands, `pty` channel, `Takeover`, what the page shows         |
| `src/lib/push.ts`             | Web Push capability, VAPID keys in IndexedDB, `PushManager`, `push` commands        |
| `src/sw.ts`                   | service worker: `Reminder` push notifications and `notificationclick` card open     |
| `src/App.svelte`              | shell: navigation, top bar, banners, the page of the route                          |
| `src/Notice.svelte`           | the upgrade notice and the storage notice                                           |
| `src/routes/`                 | one component per page; `Groups.svelte` is the list and a chat                      |
| `src/routes/Join.svelte`      | invitation landing page, using the onboarding header and hero styles                |
| `src/routes/dev/`             | `States.svelte`: the message examples, for a development server                     |
| `src/components/`             | shared parts: `Icon` (line icons), `Avatar`, `Failure`, `Mount`, `Rich` (a text)    |
| `src/components/groups/`      | the group list, the cards that wait, the create-group dialog                        |
| `src/components/chat/`        | the chat of a group: main line, message, composer                                   |
| `src/components/task/`        | `TaskCard.svelte`, mounted for kind `task`, with its menu and actions               |
| `src/components/thread/`      | a task's thread and its composer, scored review block and system notices            |
| `src/components/call/`        | `CallBlock.svelte`, mounted for kind `call` in threads and the main line            |
| `src/components/confirm/`     | `ConfirmCard.svelte`, mounted for kind `confirm`                                    |
| `src/components/allow/`       | `AllowEditor.svelte`, lists and editor for each AI in the member panel              |
| `src/components/member/`      | the chat's member panel (`chat.side`) and the add-AI dialog                         |
| `src/components/profile/`     | work share and usage block, owner editor, model-tier suggestion, profile fields     |
| `src/components/model/`       | model/level fields, display and catalog refresh                                     |
| `src/components/people/`      | invitation dialog and code card, role/removal actions and join form                 |
| `src/messages/people.*`       | English and Chinese invitation, member-action and join texts                        |
| `src/components/groupinfo/`   | home, settings (order 19) and project memory (order 20) at `chat.side`              |
| `src/components/preset/`      | style preset editor (`chat.side`, `order: 21`) and brief preset reader              |
| `src/components/kb/`          | knowledge-base list, search and admin deletion (`chat.side`, `order: 22`)           |
| `src/components/move/`        | owner's local export/import guidance (`chat.side`, `order: 30`)                     |
| `src/components/terminal/`    | terminal session sidebar                                                            |
| `src/components/watch/`       | receive-only watch channel, standalone request action and watcher stop controls     |
| `src/components/nodes/`       | top-bar and phone-chat indicator, add-browser pairing dialog                        |
| `src/components/stop/`        | "Stop AIs" and its panel, the paused notice, the offline banner and card strip      |
| `src/components/queue/`       | `Waits.svelte`: a message waits for a busy AI, and can go first                     |
| `src/components/notify/`      | live in-page notifications mounted at `layer`                                       |
| `src/components/driver/`      | account types and terminal action offered by an AI's `driver`                       |
| `src/components/pairing/`     | onboarding status and CLI account cards, `#pair=` mount                             |
| `src/components/reminder/`    | settings notifications card with iOS hint (`ReminderCard`), `ReminderSync` mount    |
| `src/components/status/`      | other-tab notice mounted at `banner`                                                |
| `src/styles/`                 | `tokens.css`, `base.css` and `onboarding.css` (shared pairing and join layout)      |
| `public/`                     | manifest, icon and `fonts/`, copied as they are                                     |
| `features/web.spec.ts`        | steps of `../features/web.feature`                                                  |
| `features/shell.spec.ts`      | steps of `../features/web_shell.feature`; `fixtures/` stand-ins                     |
| `features/groups.spec.ts`     | steps of `../features/web_groups.feature`                                           |
| `features/chat.spec.ts`       | steps of `../features/web_chat.feature`                                             |
| `features/status.spec.ts`     | steps of `../features/web_status.feature`                                           |
| `features/task.spec.ts`       | steps of `../features/web_task.feature`                                             |
| `features/taskops.spec.ts`    | steps of `../features/web_taskops.feature`                                          |
| `features/profile.spec.ts`    | steps of `../features/web_profile.feature`                                          |
| `features/model.spec.ts`      | steps of `../features/web_model.feature`                                            |
| `features/thread.spec.ts`     | steps of `../features/web_thread.feature`                                           |
| `features/call.spec.ts`       | steps of `../features/web_call.feature`                                             |
| `features/confirm.spec.ts`    | steps of `../features/web_confirm.feature`                                          |
| `features/member.spec.ts`     | steps of `../features/web_member.feature`                                           |
| `features/people.spec.ts`     | steps of `../features/web_people.feature`                                           |
| `features/people.test.ts`     | focused tests for member actions and the join page                                  |
| `features/groupinfo.spec.ts`  | steps of `../features/web_groupinfo.feature`                                        |
| `features/preset.spec.ts`     | steps of `../features/web_preset.feature`                                           |
| `features/kb.spec.ts`         | steps of `../features/web_kb.feature`                                               |
| `features/move.spec.ts`       | steps of `../features/web_move.feature`                                             |
| `features/trust.spec.ts`      | steps of `../features/web_trust.feature`                                            |
| `features/terminal.spec.ts`   | steps of `../features/web_terminal.feature`                                         |
| `features/watch.spec.ts`      | steps of `../features/web_watch.feature`                                            |
| `features/nodes.spec.ts`      | steps of `../features/web_nodes.feature`                                            |
| `features/stop.spec.ts`       | steps of `../features/web_stop.feature`                                             |
| `features/queue.spec.ts`      | steps of `../features/web_queue.feature`                                            |
| `features/notify.spec.ts`     | steps of `../features/web_notify.feature`                                           |
| `features/onboarding.spec.ts` | steps of `../features/web_onboarding.feature`                                       |
| `features/driver.spec.ts`     | steps of `../features/web_driver.feature`                                           |
| `features/reminder.spec.ts`   | steps of `../features/web_reminder.feature` (A8.3 browser half)                     |
| `features/guide.spec.ts`      | steps of `../features/web_guide.feature` (first-run guide scenarios G.1–G.8)        |
| `features/a11y.spec.ts`       | steps of `../features/web_a11y.feature`                                             |
| `features/home.ts`            | a home and an owned computer in memory for the steps; `page.ts` drives the page     |
| `features/s2_cast.ts`         | the step 2 cast in a page, as `window.s2`                                           |
| `features/demo.ts`            | groups with a morning of talk, for a development page                               |
| `features/sim/`               | the world of `?demo`: computers, people, AIs and their scripts                      |
| `features/watch.svelte.ts`    | an effect outside a component, for tests of what is reactive                        |
| `features/shots.ts`           | screenshots of the pages from that demo, four widths                                |
| `e2e/config.ts`               | end-to-end projects per engine, `launch`, the build of a run                        |
| `e2e/nodes.ts`                | `startNet`: rendezvous and nodes per test, stop, teardown sweep                     |
| `e2e/s2.spec.ts`              | the In the browser sentences of step 2, one page per person                         |
| `features/e2e.ts`             | steps of `../features/web_e2e.feature`, run by `e2e/config.ts`                      |

Below 768 px the navigation is the bottom bar and the theme and language buttons of the top bar
are hidden; both choices are on the settings page at every width.

Under forced colours (Windows contrast themes) the system drops the sliding marks of the
navigation and of the segmented controls; the current entry and the chosen option are then drawn
in `Highlight` and `HighlightText` instead (`base.css` section 8).

## Known limitations

- A `moved` response to `cmd`, `sync` or `fetch` switches a group to the named home and
  preserves its cache. An existing destination connection is reused; an unknown destination is
  remembered for reconnecting on the current signalling paths. Its welcome supplies its remembered
  addresses. A group that leaves its home's list is read once more, so that an idle browser gets
  that answer; any other refusal discards it. Commands are
  not replayed across a move: their receipts stay on the old home. Before import the cached
  group remains visible; the destination's `groups` push starts a new read after import.
- A group in the list of a node that a home out of reach still holds was restored there from a
  copy (`contracts/step-2/export.md` section 4): it is taken over and read from the start, so an
  open chat goes on there. While the old home can be reached nothing is taken over.
- A browser that stays connected hears of a group joined in another one at its next `host.info`:
  the node pushes none (`../issues.md`).
- Only a group's owner sees the local `solodawn group export` / `group import` guidance. The
  browser neither runs these commands nor transfers the file. A copy omits `--to`; scheduling
  copies belongs to the operating system, not the browser.

- Watching reuses `#/terminal/<g>/<member>` for another person's AI. The terminal acquires its
  host through the existing signalling session, reuses a connected home or node, and releases a
  temporary connection when the last watch ends. Another person's taken-over AI offers Ask to
  watch and Watch. The terminal bar says how the reader's request stands (`watchBar`) and offers to
  ask; a request settled before the takeover began does not count. With the consent the page
  reaches for the computer again, with a new back-off, and a refused `watch.start` is tried again
  by the back-off of a connection, since the grant arrives a moment later. The bar changes only with
  an answer, and after a second refusal it offers to ask again (the owner may have stopped the
  watcher). An ended watch is not started again, and the computer is let go when the takeover ends,
  unless the watch is open. The owner's terminal
  shows the watchers, and over the terminal each pending request as the `ConfirmCard` of the chat:
  it takes no focus when it comes and gives it to the terminal when it goes.
- Neither group settings nor the add-AI dialog offers an external AI as secretary.
- The add-AI dialog warns when a personal-subscription secretary would serve another person. The
  invitation dialog warns before inviting the first other person.
- A shared secretary should use a team plan or an API key, with an explicit allow-list. On a
  personal subscription every request from someone other than its owner waits for that owner's
  confirmation, every time; this cannot be switched off.
- A foreign home stores the group, names its members and decides which requests reach your AIs.
  Join only groups of people you trust. Group details attribute its computer using a matching
  device row; if none is available, the owner is shown as not provided, never inferred from a
  member's group role.

- The theme is applied by the module script, so a browser forced to the theme opposite to its
  system setting can paint one frame in the system theme while loading (`issues.md`).
- The upgrade notice uses the same tokens as the application. A browser without `light-dark()` or
  CSS nesting shows it with default colours; a browser too old to run the bundle shows nothing
  (`issues.md`).
- Safari deletes the site's storage, and with it the device key, after seven days of use without
  a visit, unless the client was added to the home screen (`issues.md`).
- Onboarding (`routes/Onboarding.svelte`, `lib/onboard.svelte.ts`, `lib/owner.ts`,
  `components/pairing/`) pairs a node or joins from a `#pair=<secret>` link, stores the owner's
  device log in IndexedDB (`solodawn`, store `device`, key `owner`), asks for persistent storage
  once enrolled, and sends `agent.set`; the other pages reach their nodes through `peer.ts`, by way
  of `net.svelte.ts`.
- `peer.ts` runs against real nodes in the end-to-end tests (`e2e/connect.spec.ts`), in the three
  browsers.
- The manifest has an SVG icon only.
- A development server (`pnpm dev`) shows the one computer and the one group of
  the wire examples, unless the address has `?nomock` or `?demo`; the mock answers every command with a bare
  `ok` and pushes no row, so a message sent there does not appear, an account choice does not
  update `host.info`, and `pair.browser` returns no link.
- From 768 px the top bar of an open chat says "Chats", and the chat's own
  head with the group's name is below it, where the design has one bar with the group's name.
- The preset list and brief preset reader load only the first 50 presets; there is
  no pagination or read by name. Typed notes stay out of Project memory. A person adds a preset
  and changes its name and text in place (`note_edit`); a new preset whose name is held is
  refused before it is sent.
- Knowledge-base lists load in pages of 50; searches return up to 20 documents in the home's
  ranking order. If more
  match, narrow the query rather than paging results. A person adds a document and changes its
  name and text in place (`note_edit`); a new document whose name this browser already holds is
  refused before it is sent, and one held only by the home replaces that document, as `kb_put`
  does.
- The texts of the `states` area are in the build, although only a development server shows
  their page.
- Safari moves the main line when a message arrives while the reader is scrolled back
  (`issues.md`).
- The end-to-end harness runs on macOS and Linux only (`issues.md`).

## Verification and sources

- `e2e/external.spec.ts` covers the external AI add dialog, its folder in every hand-out, setup
  prompt, reported usage and removal (A10.17, A10.18, A10.20, A10.25 and A10.27).
- Unit tests sit beside the modules (`src/**/*.test.ts`); `../features/web.feature` is the
  acceptance feature. `stryker.config.json` breaks below a mutation score of 100.
- `src/lib/peer.test.ts` runs `peer.ts` against a fake `RTCPeerConnection` and fake signalling
  clients, with the clock in the test's hands. `../features/web_peer.feature` is the browser half
  of A1.11 (`planning/steps/step-1-acceptance.md`) and one reconnect, through the real
  `rendezvous.ts` with signed envelopes; its steps are `features/peer.spec.ts`.
- The shell was checked in headless Chrome at 375, 820, 1100 and 1440 px in both themes and both
  languages on every page (no element wider than the viewport) on 2026-11-20. It has not been
  opened on a real phone, in Safari or in Firefox.
- The seams of the shell have unit tests beside `route.svelte.ts`, `net.svelte.ts`,
  `mounts.svelte.ts`, `prefs.svelte.ts` and `texts.ts`, and the acceptance feature
  `../features/web_shell.feature`, whose steps mount stand-in areas (`features/fixtures/`) into
  the points. The test DOM lays nothing out, so the whole-screen modes are checked by the
  attribute the styles read. On 2026-11-21 the settings page, a page then still to come and the states sheet
  were looked at in headless Chromium at 375, 820, 1100 and 1440 px in both themes, and an open
  chat with a stand-in area at the top bar, the banner, above the main line and beside it, with
  the side panel open below 1024 px; no element but the visually hidden half of the language
  button's name reached past the viewport.
- The motion was checked there too: while pages changed, the theme and language switched and
  parts were hovered, pressed and focused, the page's animations (`document.getAnimations()`)
  changed `opacity` and `transform` only, and with reduced motion emulated there were none. A test
  holds `--spring` to `spring()`. Frame rate was not measured, and the fallback without view
  transitions was exercised only in the tests.
- `src/lib/proto.test.ts` reads every wire example with the schema of its file name and fails on a
  file that has none (`crypto.json` and `wire.json`, which hold test vectors, excepted); it adds
  an unknown field to every object, which is dropped (so a golden file with a field the schema
  lacks fails the exact comparison). It then holds the schemas to `internal/proto/testdata/wire.json`,
  which the Go tests write from the Go types and judge `proto.Decode` by: every value of every
  example is changed in turn (left out, `null`, another type, a fraction, 2^53, a text that is no
  id) and must be read or refused as that file says, and so must its cases.
  Fields that appear in no example are not covered by those variants.
- `src/lib/vectors.test.ts` holds the client to every part of
  `internal/proto/testdata/crypto.json` (`planning/contracts/vectors.md`). It never compares
  signature bytes, since Safari's signatures are random: it verifies the vectors' signatures and
  the ones the client makes itself. `vitest.vectors.ts` runs only `src/lib/vectors.test.ts` and
  `features/browser-crypto.ts` in Chromium (Chrome 137 and Edge 137 in CI), Firefox 136 and (on
  macOS) WebKit (`.github/workflows/browser-vectors.yml`); neither file touches IndexedDB. The
  other identity tests, and IndexedDB, run in Node only (`fake-indexeddb`).
- Importing `decode` and `Frame` adds about 7.6 KiB to the bundle, 2.6 KiB gzipped (measured
  with `valibot` 1.5.0 on 2026-11-20). `peer.ts` imports `Frame`.
- The group list and the chat have unit tests beside `groups.svelte.ts`, `compose.ts` and
  `when.ts`, and two acceptance features, `../features/web_groups.feature` (A2.1, A2.2, A2.6 and
  the reset of round 3, R3-5) and `../features/web_chat.feature`. Their steps mount
  `routes/Groups.svelte` in the test DOM against `features/home.ts`, which answers `sync`,
  `fetch` and `cmd` as `planning/flows/sync.md` says; no node ran. The test DOM lays nothing out:
  whether the top of the main line is in view, and whether the person has a keyboard, are stood
  in for (`features/page.ts`); it does not report the chosen option of a select either, so the
  steps choose one by disabling the others for that moment. `compose.test.ts` holds `rowSize` to
  rows built from the wire example, with up to 20 members mentioned: never under, at most 2 bytes
  over.
- Both pages were looked at in headless Chromium at 375, 820, 1100 and 1440 px in both themes,
  with `features/demo.ts` loaded into a development page, on 2026-11-20. They have not been opened
  in Safari or Firefox, nor on a real phone, and nobody walked through them with a screen reader.
- Accessibility (U-16, U-16b): on 2026-11-22 axe-core 4.10.3 (tags wcag2a, wcag2aa, wcag21a,
  wcag21aa) ran in the system Chrome on the group list, the chat with calls and a thread, the
  member panel, the project memory and its delete dialog, the add-AI dialog, the terminal (active
  and refused), settings (also as an iPhone), computers, the add-a-browser and create-group
  dialogs, onboarding, the states sheet and the other-tab notice, each with notifications where
  they show, in both themes at 1280 by 800 and 390 by 844 with a touch pointer: no violation.
  Tab order, focus after each close and the way out of the terminal were walked with real key
  presses there. `../features/web_a11y.feature` holds what the test DOM can check; it has no
  layout, no native dialog focus and no colour, so the placement of notifications, the focus a
  `<dialog>` gives and returns, and contrast on the page were checked in Chrome only. Nobody
  walked the pages with a screen reader.
- Rules: `planning/architecture.md` section 8, `planning/product-rules.md` R-160 to R-164.
