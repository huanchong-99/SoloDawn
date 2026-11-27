# SoloDawn interface design notes

Date: 2026-11-20; sections 1 and 3 rewritten on 2026-11-26 for the redesign of the web client.
Status: sections 1 and 3 are the visual reference of the client in `web/src/`. The static HTML
mockups in this directory (`index.html`, `chat.html`, `groups.html`, `nodes.html`,
`onboarding.html`, `settings.html`, `terminal.html`, `components.html`, with `base.css`, `base.js`
and `tokens.css`) are superseded: they show the first design and are kept unchanged as a record.
Sections 4 to 11 describe those mockups; where they differ from sections 1 and 3, sections 1 and 3
hold.

## 1. Design idea

**In one sentence: calm neutrals, one accent, and colour only for meaning.** The client should
look premium and still be simple to anyone who sees it for the first time, with a chat that reads
as naturally as the messengers people already use.

- **Calm, tinted paper.** The page is light paper in the light theme and deep ink in the dark one,
  and both take the hue of the theme colour: a change of theme colour changes the whole page, as
  plainly as a change between light and dark. Navigation (the rail, the top bar, the group list)
  stands on the page colour and recedes; the content (the chat, the side panel, cards, dialogs)
  stands on the card colour, the lightest in the light theme, and leads.
- **One accent.** An indigo blue by default (the person may choose another theme colour, see
  "Theme colours" below), kept under the token name `--sky`, marks what can be acted on
  and what an AI is doing: primary buttons, links and mentions, the focus ring, the icon of the
  current navigation entry, an AI's working glyph, the current step of a task, the person's own
  bubbles (as the tint `--mine`) and the send button when it can send. Nowhere else.
- **Dawn is a signal.** Amber still means "a person is needed", and only that, but as a small
  signal: the pill of a card that waits, the count of a group with waiting items, the label and
  dot of the waiting strip, a tint in the border of a waiting card. Never a frame around half the
  screen and never a button: a button on a waiting card is the accent.
- **Green and red are results.** Green is something passed or finished; red is a failure, a
  refusal that is a failure, or the emergency stop. A refusal by a person and a pause are states,
  not failures: they are neutral.
- **Hierarchy by size, weight and space.** Fewer borders, more alignment, one 4 px rhythm.
  Surfaces are told apart by tone first, a hairline second, a shadow last.
- **Motion confirms.** Short, interruptible, from the place a thing comes from. Nothing waits
  for an animation, and under reduced motion nothing moves.

What stays from the first design:

1. **People are round, AIs are rounded squares** that hold the mark of their vendor; the
   secretary has its badge.
2. **State is never carried by colour alone.** Working is a breathing dot, waiting a hollow
   circle, sleeping a crescent, paused two bars, taken over a square, offline a dashed circle.
3. **The main line does not flood.** Progress, the AIs' talk with each other and reviews live in
   the thread at the foot of a task card.
4. **The thin horizon line** (`--horizon`) is the only gradient, on the logo and the onboarding
   page. No glass, no glow, no coloured card heads, no second accent.
5. **The terminal is dark in both themes.**

## 2. Relation to pachat.ai

I looked at its home page, and took only its level of finish: whitespace, corner radii and consistent font weights. Layout, colors, brand and illustrations were not reused. It is soft pastel gradients with a cartoon mascot, for a general audience; SoloDawn is a workbench for developers to watch AIs work, so it takes a restrained, information-dense route that uses outlines instead of shadows.

## 3. The design language

All tokens are in `web/src/styles/tokens.css`, the shared styles in `web/src/styles/base.css`.
Each fixed colour is one line written as `light-dark(light value, dark value)` with 6-digit hex
values. The surfaces, the hairlines and the texts are
`light-dark(oklch(L calc(C * var(--tint)) var(--hue)), oklch(…))`: a lightness and a chroma of
their own in each theme, the hue and the strength of the tint from the theme colour. Soft fills are
`color-mix(in srgb, var(--x) N%, var(--surface))`. The contrast scenario of
`features/web_a11y.feature` reads these formats. Switching themes changes only `color-scheme`.

### 3.1 Colours

The tinted tokens are given by their OKLCH lightness and chroma, and shown with the default theme
colour (hue 268).

| Token           | Light L / C   | Dark L / C    | Light     | Dark      | Use                                                                   |
| --------------- | ------------- | ------------- | --------- | --------- | --------------------------------------------------------------------- |
| `--bg`          | 0.958 / 0.017 | 0.165 / 0.030 | `#ECF1FD` | `#090E1B` | rail, top bar, group list, page                                       |
| `--surface`     | 0.991 / 0.004 | 0.205 / 0.030 | `#FBFCFF` | `#111725` | chat canvas, side panel, cards, dialogs                               |
| `--surface-2`   | 0.948 / 0.020 | 0.245 / 0.032 | `#E8EEFC` | `#1A2030` | others' bubbles, inset blocks                                         |
| `--surface-3`   | 0.928 / 0.026 | 0.290 / 0.034 | `#E0E7F9` | `#242B3D` | hover, pressed, tracks, disabled fill                                 |
| `--raised`      | 0.993 / 0.003 | 0.245 / 0.032 | `#FCFDFF` | `#1A2030` | what is lifted off its ground: a selected row, a plain button, a menu |
| `--line`        | 0.905 / 0.030 | 0.300 / 0.040 | `#D7DFF4` | `#252D42` | hairlines, card borders                                               |
| `--line-strong` | 0.600 / 0.035 | 0.560 / 0.040 | `#778096` | `#6A748D` | input borders (3:1 or more)                                           |
| `--text`        | 0.210 / 0.020 | 0.950 / 0.008 | `#141822` | `#ECEEF4` | body text                                                             |
| `--text-2`      | 0.430 / 0.035 | 0.800 / 0.025 | `#474F64` | `#B7BECE` | secondary text                                                        |
| `--text-3`      | 0.500 / 0.040 | 0.680 / 0.035 | `#59637B` | `#8F98AF` | auxiliary text (still 4.5:1 or more)                                  |
| `--sky`         |               |               | `#3A57E8` | `#8FA3FF` | the accent (the default theme colour)                                 |
| `--dawn`        |               |               | `#F2A23A` | `#F4B256` | needs a person (fill)                                                 |
| `--dawn-text`   |               |               | `#9A5300` | `#F4B256` | dawn-coloured text on an ordinary surface                             |
| `--on-dawn`     |               |               | `#2A1A00` | `#2A1A00` | text on a dawn fill                                                   |
| `--ok`          |               |               | `#117246` | `#5ED193` | passed, finished, online                                              |
| `--danger`      |               |               | `#BB2C25` | `#FF8A80` | failed, the emergency stop                                            |

Lightness alone sets the steps between the surfaces and the contrast of every text, so both are
the same in every theme colour; the hue changes, and the chroma with `--tint`. A light surface can
carry little chroma before it leaves sRGB (near white, about 0.004 for blue), a dark one much
more, so the dark theme carries the stronger tint. Every tinted token stays inside sRGB for every
theme colour (`features/web_a11y.feature`), so all browsers draw the same colour.

`--sky-soft`, `--dawn-soft`, `--ok-soft` and `--danger-soft` are mixed from the semantic colour and
the card colour; `--mine` (14% of the accent) is the person's own bubble; `--need-line` is the
border of a card that waits. In the dark theme a shadow barely shows, so a raised layer is one
step lighter (`--raised`) with its hairline.

### Theme colours

The settings page offers ten theme colours as a row of swatches; each sets the accent `--sky`,
`--on-sky` and the hue of the page `--hue` (`src/styles/accents.css`), and everything tinted or
mixed from them follows. The set covers the accent colours people know from their systems and
apps (macOS: blue, purple, pink, yellow, green, graphite; Telegram's palette adds cyan; Todoist's
themes are the same families), and one warm theme: cream paper and warm brown ink with a clay
accent. Pure red and orange are left out: red is the failure and the emergency stop, and orange
at text contrast is the same brown as amber. The names are SoloDawn's own: scenes of a day from
dawn to night, which echo the product's name, not the colour words other products use.

| Theme colour            | Id           | Light     | Dark      | Page hue | Tint |
| ----------------------- | ------------ | --------- | --------- | -------- | ---- |
| 黎明 Daybreak (default) | `daybreak`   | `#3A57E8` | `#8FA3FF` | 268      | 1    |
| 雨后 After rain         | `rain`       | `#46668C` | `#8FADD6` | 250      | 1    |
| 深潭 Deep pool          | `pool`       | `#155F72` | `#5AAEC4` | 220      | 1    |
| 浅湾 Shallows           | `shallows`   | `#00735F` | `#4FC4A2` | 178      | 1    |
| 林荫 Tree shade         | `shade`      | `#387026` | `#8ABD70` | 140      | 1    |
| 远山 Far hills          | `hills`      | `#7A4E93` | `#BB98CF` | 312      | 1    |
| 朝霞 Morning glow       | `glow`       | `#B52C62` | `#F58BAD` | 355      | 1    |
| 余晖 Afterglow          | `afterglow`  | `#A3492A` | `#E8956F` | 82       | 0.6  |
| 晨曦 First light        | `firstlight` | `#985800` | `#E0A24C` | 88       | 1    |
| 夜幕 Nightfall          | `nightfall`  | `#434A57` | `#ADB3BD` | 262      | 0.25 |

- The page hue is the accent's own OKLCH hue, except for the two warm themes: afterglow's paper is
  warm ivory (82, at 0.6 tint for low chroma) under a clay accent (40 to 45), and first light's a
  yellower gold (88) than its accent, so the two warm pages stay apart. Nightfall keeps a quarter of
  the tint: a cool grey page.
- Afterglow is the evening counterpart of morning glow, the last warm light of the day; it sits
  between morning glow and first light, by hue.

- Daybreak stays the default: it is the accent the redesign was approved with, and the first stop
  of the horizon line of the logo. `src/lib/prefs.test.ts` holds its values, so a person who never
  chose keeps its blue.
- Each theme colour tints the whole page, not only what can be acted on: the surfaces, the
  hairlines and the texts take its hue (section 3.1). The person sees the new colour at
  once when they choose one; the page cross-fades to it where the browser has view transitions,
  and changes at once under reduced motion.
- Every theme colour passes the same contrast pairs as the default in both themes: each text,
  the accent, dawn's text and the result colours 4.5:1 on every surface (`--bg`, `--surface`,
  `--surface-2`, `--surface-3`, `--raised`, `--raised-2`, `--mine`, `--sky-soft`), and the accent
  3:1 against the hairline and the inset fill, so that the neutral nightfall still shows the
  focus ring and a chosen border.
- Any two theme colours are at least 17 apart in CIE Lab (ΔE 1976) in each theme, so no two
  swatches read as one.
- With first light or afterglow as the accent, dawn turns magenta (`#D94AA3` / `#F08AC8`, text
  `#A8226F` / `#F08AC8`): still warm and loud, about 70° of hue from first light and 54° from
  afterglow. It is 40° from the red of
  a failure, a little further than the default amber is (30°). The waiting signal stays at least
  45° of hue from the accent in every theme colour (`features/a11y.spec.ts`).
- Some accents sit near a result colour: afterglow is 13° of hue from the red of a failure,
  morning glow 25°, shallows 16° from the green of a passed result, tree shade 48° from it. The state glyphs and the
  words beside them tell them apart (`issues.md`).
- A swatch is a radio named by its colour; the chosen one has a ring in its colour and a tick.

Rules:

- A selected row of the group list is neutral (`--raised` with `--shadow-1`), never the accent.
- A note (`.note`) is a neutral inset block; its tone colours its glyph and lead words only. The
  one note on dawn is the "home of N groups" line of the computers page, which asks for attention.
- Avatars and group marks are tinted from the hue of their id: `oklch(0.94 0.04 h)` with the
  initial in `oklch(0.42 0.09 h)` in the light theme, `oklch(0.32 0.05 h)` with
  `oklch(0.86 0.07 h)` in the dark one (7:1 or more). They are computed in `Avatar.svelte`.

### 3.2 Type

Geist for Latin and numbers and Geist Mono for code, branches and the terminal, both self-hosted
(OFL); Chinese comes from the system (PingFang SC, Hiragino Sans GB, Microsoft YaHei, Noto Sans
CJK SC). Weights 400, 500 and 600 only. Headings are 600 with `text-wrap: balance`; the Latin
ones are tightened by 0.01em (`:lang(en)`), Chinese ones are not. Numbers that line up are
tabular. No text is smaller than 12 px, except the 11 px labels of the navigation. A section head
(`.sec-title`) is 13/600 in `--text-2`; a Latin one is upper case at 12 px, 0.04em apart.

| Token      | Size / line height            | Use                                       |
| ---------- | ----------------------------- | ----------------------------------------- |
| `--fs-xs`  | 12 / 1.4                      | times, meta, badges                       |
| `--fs-sm`  | 13 / 1.5                      | thread rows, card meta, side panel values |
| `--fs-ui`  | 14 / 1.45                     | buttons, list previews, form labels       |
| `--fs-md`  | 15 / 1.6                      | message text, list names                  |
| `--fs-lg`  | 16 / 1.3                      | card titles, dialog titles                |
| `--fs-xl`  | 20 / 1.3                      | page titles                               |
| `--fs-2xl` | 26 / 1.25                     | summary numbers                           |
| `--fs-3xl` | clamp(30px, 5vw, 44px) / 1.15 | onboarding headline                       |

### 3.3 Space, radius, size, elevation

- Spacing is on a 4 px base (`--sp-1` to `--sp-16`).
- Radius: `--r-sm` 6 (badges, inline code, chips), `--r-md` 10 (buttons, inputs), `--r-lg` 14
  (cards, popovers), `--r-xl` 18 (bubbles, the composer, dialogs), `--r-full`.
- Controls are 36 px (`--ctl`), 30 px small (`--ctl-sm`), and 44 px under a coarse pointer.
- Widths: rail 64 (76 in English), group list 300, side panel 340, top bar 52, chat head 56,
  `--measure` 760, `--card-w` 560 (the widest card in the chat), `--bubble-w` min(72%, 600px)
  (82% on a phone).
- Elevation: `--shadow-1` (cards, a selected row), `--shadow-pop` (menus, popovers, notices),
  `--shadow-dlg` (dialogs, sheets, the panel that slides over the chat), `--scrim`.
- Icons are Lucide's paths (`web/src/components/ICONS-NOTICE.md`), stroke 1.75 at 20 px and 1.5 at
  16 px, round caps and joins; 24 px in empty states only.

### 3.4 Shared parts

- **Buttons.** Primary is the accent; `.btn-dawn` keeps its class and looks like it. The plain
  button is `--raised` with a hairline and `--shadow-1`; ghost has no border; danger is red text
  on the plain button, and `.btn-danger.solid` is the button that confirms something destructive.
  Disabled is `--surface-3` with `--text-3`. Hover is a layer whose opacity changes; a press
  sinks the button to 0.96.
- **Inputs** are 36 px on `--surface` with a `--line-strong` border. Radios and checkboxes are
  drawn at 18 px with the accent when chosen; the switch is 44 by 26.
- **Badges** are 20 px pills; `.solid` is for counts. The pill of a waiting card is soft dawn
  with a dot.
- **Segmented control**: a `--surface-2` track with a raised thumb that slides by `--at`.
- **Settings**: a grouped list on a card, rows of 52 px, hairlines inset 16 px.
- **Dialogs**: radius 18, `--shadow-dlg`, 480 px (`.wide` 640), padding 24. Below 768 px a dialog
  is a bottom sheet with a decorative grabber and safe-area padding.
- **Radio cards** (`.acct`, the options of a decision, the AIs of the add-AI dialog): 56 px,
  radius 12; the chosen one has the accent border and `--sky-soft`; a disabled one is at 0.55.
- **Empty states** (`.blank`): a 48 px circle with a 24 px icon, a title, one line, at most one
  primary button. No dashed borders.
- **Ready for lists of members and folders**: `.pick-row` (a member to pick, 52 px), `.path-row`
  (a folder path in mono with its action), `.sec-title` (a section head with its count).

### 3.5 The chat

The main line is a conversation, so it has bubbles; a card is a document with actions, so it
stands on its own surface; a thread is a work log, so it is compact flat rows.

- **Others** stand on the left: a 36 px picture (32 on a phone) at the top of a run, the name in
  13/500, badges, the time, then the bubble on `--surface-2`. A run is what the code calls
  `joined`: its later messages have no head and are 4 px apart, and the corners toward a
  neighbour on the author's side tighten to 6 px.
- **The person's own** messages stand on the right without a picture, on `--mine`; the head is
  one quiet line ("You · 19:42").
- **Cards** stand in the author's column, at most `--card-w` wide; on a phone they span it.
- **The day** is a small pill on a band of the canvas colour that stays at the top of the line.
- **Notes of the home** are centred lines of 12 px.
- **The time of a joined message** shows beside its bubble under the pointer, and for all of them
  while keyboard focus is on the main line.
- **The composer** is one raised container of radius 18: the text on top and a row with the
  mention button and the send circle under it; one row on a phone and in a thread.
- **Threads**: rows of 13/1.5 with a 20 px picture, 8 px apart, no boxes.

### 3.6 Cards

One anatomy for every card: a head (a 16 px glyph, the kind in 13/500, an optional id in mono,
the state pill on the right), a title in 16/600, key-value rows (`.kv`: labels 72 px wide in
Chinese and 96 px in English, 13 px in `--text-3`; values 14 px), actions on the left with the
primary first and a quiet note after them. The kind is told by glyph and words, never by a colour.

- **Waiting for the person**: a tint of dawn in the border and the soft pill; nothing else.
- **Settled**: no shadow, the title in `--text-2`, the pill as the outcome (ok for confirmed, a
  neutral pill with a cross for a refusal).
- **Decision**: the options are radio cards with the key in mono; the chosen one has the accent
  border and a tick.
- **Task**: the stages are one row of 4 px segments for any number of steps (done ok, current
  accent, waiting dawn, later `--surface-3`), with the stage in words beside it; questions asked
  in place and holds are inset blocks on `--surface-2`.
- **Summary**: figures in 26/600, additions green and removals red.

### 3.7 Motion

Only `transform` and `opacity` animate; a colour that changes on hover, press or focus is a layer
whose opacity changes. `base.css` holds the one `prefers-reduced-motion: reduce` block.

| Token       | Value  | Use                                      |
| ----------- | ------ | ---------------------------------------- |
| `--dur-1`   | 120 ms | hover, press, small fades                |
| `--dur-2`   | 200 ms | a change of state, a popover, a bubble   |
| `--dur-3`   | 320 ms | a panel, a sheet, a page                 |
| `--dur-4`   | 500 ms | a spring comes to rest                   |
| `--dur-out` | 140 ms | what leaves                              |
| `--travel`  | 12 px  | how far a page and its parts come        |
| `--rise`    | 8 px   | how far a bubble, a pill or a list comes |
| `--stagger` | 30 ms  | between the parts of an arriving page    |

`--ease` is for a change of state, `--ease-out` for an arrival, `--ease-in` for an exit,
`--spring` for what a person moved and for counts.

| What                                                     | How                                                                                                  |
| -------------------------------------------------------- | ---------------------------------------------------------------------------------------------------- |
| A message that arrives while the chat is open            | fade, rise 8 px and scale from 0.98, from its own side                                               |
| Messages on open and on paging                           | no animation                                                                                         |
| A card whose state changes in view                       | the new pill lands with the spring (`data-changed`)                                                  |
| A count                                                  | lands with the spring each time it changes                                                           |
| The group list reorders                                  | `flip`, 240 ms                                                                                       |
| A chip of the waiting strip                              | fade and scale from 0.96                                                                             |
| An AI at work                                            | a dot that breathes, the one motion that loops                                                       |
| The panel over the chat below 1024 px                    | slides in over a scrim in 320 ms, out in 140 ms                                                      |
| A dialog; a sheet on a phone                             | fade and scale from 0.97; up from the bottom edge                                                    |
| A menu, the stop panel, the mention list                 | fade and scale from 0.96 from the trigger's corner, 160 ms                                           |
| A notice                                                 | down from the top, fades out in 140 ms                                                               |
| A task stage advances                                    | its segment fills from the left                                                                      |
| The send button can send                                 | its glyph lands with the spring                                                                      |
| A disclosure                                             | the chevron turns                                                                                    |
| A pairing step is done                                   | its tick lands with the spring                                                                       |
| The welcome horizon, steps and guide dock                | horizon sweeps from left; steps stagger; dock fades and lifts; pill and sheet toggle scale from 0.96 |
| A guide step finishes or all five complete               | tick scales in with the spring; segment fills from left; finish horizon sweeps from left             |
| The guide beacon                                         | ring enters from 1.04 scale and pulses out to 1.12 twice, then fades                                 |
| The closed-guide note                                    | fades and lifts from the dock corner, fades out in 140 ms                                            |
| Page change, theme and language, press, logo, focus ring | as `web/README.md` (Motion) says                                                                     |

What leaves the page through a Svelte block (a menu, a list, a pill) leaves at once: the test
DOM never ends an exit transition, so only the notices and the confirm cards over the terminal,
which had one before, fade out.

## 4. Components and their states

Every state of every component is laid out in `components.html`.

### 4.1 Global state (top right of every page)

- **Connection indicator:** direct (green dot + latency), relay, restricted (warm background + diamond), reconnecting (breathing blue dot), cannot reach the group's home (red hollow circle). Clicking it opens "My computers and AIs".
- **Emergency stop "Pause all":** a red-outlined button. Clicking it opens a confirmation; choose "All AIs in this group" or "All my AIs". After a pause, a banner with "Resume" appears under the top bar. On a phone the button keeps only its icon, but keeps its screen-reader name.

### 4.2 Chat main line

- **Waiting-for-you bar:** a bar pinned at the top of the chat, listing the matters in this group that are stuck on you; one click jumps to the matching card.
- **Task brief card:** sent by the secretary. Goal, acceptance criteria, scope (do / do not), breakdown. States: waiting for your confirmation (with "Confirm start" and "Needs changes"), confirmed (collapses to one line, expandable).
- **Task card:** number, title, status badge, six-step progress bar, who is working, branch, check result, review result, and the thread at the bottom. States: queued, in progress, waiting for your approval, waiting for someone else's approval, review sent back, failed (circuit breaker), merged, taken over by the owner, paused.
- **Approval card:** which AI, what it wants to do (the original command), why, "Approve" and "Reject". States: as the owner sees it (clickable), as others see it (buttons grayed out + lock icon + a note on who alone can approve), approved, rejected, reassigned.
- **Decision card:** a trade-off question from the secretary, one button per option with its consequence written out, and one option can be marked "Secretary's suggestion".
- **Completion summary card:** a one-sentence conclusion + review score, rounds, time spent, lines changed; the thread is archived.

### 4.3 Task thread

The folded area at the bottom of a task card uses the native `<details>`, needs no script, and can be expanded and collapsed from the keyboard. Collapsed, it is one line: the count, a summary of the latest entry, and the number of AI back-and-forth rounds (the 10-round limit of the runaway-prevention rule). Expanded, the message types are: progress, question, call, completion report, review, approval request, decision request.

- **AI calling AI:** a small block with a blue left border that says who calls whom, whether it waits on the spot or gets a receipt later, time spent and call depth, with the reply right below it.
- **Review:** a large score + the passing line + point-by-point comments (with file and line number).

### 4.4 AI member card

Avatar (with a status badge), a one-line status (what it is doing or who it is waiting for), the computer and connection type, the account type (personal subscription / team plan / API key, and who may direct it), a usage bar, and "Open terminal". States: working, waiting (for you / for others), sleeping, taken over by the owner, quota used up, offline. Other people's AIs have no "Open terminal"; it becomes "Ask to watch". When the computer is on the relay, "Open terminal" is grayed out with the reason.

### 4.5 Terminal takeover

An inverted bar says "The owner is operating directly", with the number of queued messages and a "Hand back" button; below it a dark terminal area (the mockup imitates it with static text; the real version uses xterm.js); on a phone there is an extra row with Esc, Tab, Ctrl and the arrow keys. Beside it are notes: changes stay on the task branch, queued messages, watch requests, and the platform does not read terminal content.

### 4.6 My computers and AIs

One card per computer: online status, connection type, and which groups it is the home of. If it is a group's home, a note says it must not be shut down; if it is on the relay, a note says speed and traffic are restricted, with "Let an AI help me troubleshoot the network". Under each AI the permissions are listed per project, as a three-segment choice: read only / work in the workspace / full access. When "full access" is selected, that segment turns warm, as a reminder that this is a setting that needs attention.

### 4.7 Settings

Permissions (default level for new projects, full access in all projects, the repository's own configuration, who may approve for me), local data (retention days, default 90; audit records), notifications (browser notification switch, when to remind, iPhone note), appearance (theme, language), this browser (identity fingerprint, revoke devices).

### 4.8 Basic parts

Buttons (dawn, ink primary, normal, danger, ghost, small, icon, disabled), badges, status dots, avatars, notices, usage bars, switches, three-segment choices, inputs, checkboxes, overlays.

## 5. Why it is designed this way

- **Why the primary color does not fill everything with the brand color:** if buttons, links and selected states all used the warm color, "waiting for you" would no longer stand out. Saving the warm color for one purpose works better than any icon. The ordinary primary button is ink (white in the dark theme).
- **Why the thread is folded into the card instead of opening a side panel:** a phone has no room for a side panel; folded into the card, the phone and the computer share one interaction, and there is only one set of code. The space on the right of a computer is left for "what each AI is doing now", which people look at more often.
- **Why messages are not bubbles:** much of a group's content is cards, code and long progress notes, and bubbles waste width. A row layout of "avatar + name + content" lets cards attach naturally under a message; on a phone a card can also take the full row.
- **Why cards use outlines instead of shadows:** a screen holds many cards, stacked shadows get dirty, and outlines stay clear in the dark theme.
- **Why the approval card shows the original command:** the person who approves answers for that command and must see the original, not the AI's paraphrase.
- **Why buttons are grayed out instead of hidden for non-owners:** so that everyone knows the matter is stuck on an approval and who can give it; otherwise others just see a task that does not move.

## 6. Accessibility

- All text contrast is at least 4.5:1, and input borders and graphics at least 3:1. Every combination of text and background in the two themes was computed with a script; none falls short.
- Everything clickable is a native button, link, form control or `<details>`, reachable with the Tab key; focus has a 2px blue outline.
- Every page starts with a "Skip to main content" link.
- On phones and touch screens, ordinary buttons and icon buttons are at least 44px high and small buttons at least 36px.
- State is expressed with shape, color and text together.
- The emergency stop confirmation uses the native `<dialog>`, which brings focus trapping and closing with Esc.
- The connection progress on the onboarding page, the takeover bar on the terminal page and the pause banner have `role="status"`, so screen readers announce them. The connection indicator in the top bar has a full screen-reader name (for example "Connection status: direct, latency 18 milliseconds").

## 7. Chinese and English

Elements with a `data-en` attribute can be switched between Chinese and English, with a script of under ten lines. **The onboarding page and the settings page switch completely**; the other pages switch only the shell (navigation, top bar, emergency stop overlay) and some card labels. Chat content is sent by users and AIs and is not translated. English is longer than Chinese; the onboarding and settings pages were also checked at 375px width in English and do not overflow.

## 8. Files

| File              | Content                                                                                                  |
| ----------------- | -------------------------------------------------------------------------------------------------------- |
| `index.html`      | table of contents                                                                                        |
| `onboarding.html` | onboarding page                                                                                          |
| `groups.html`     | group list                                                                                               |
| `chat.html`       | group chat: main line + task thread, with all the cards and the AI member bar                            |
| `terminal.html`   | terminal takeover                                                                                        |
| `nodes.html`      | my computers and AIs                                                                                     |
| `settings.html`   | settings                                                                                                 |
| `components.html` | overview of the design system and component states                                                       |
| `tokens.css`      | design tokens (about 100 lines)                                                                          |
| `base.css`        | shared styles (about 350 lines)                                                                          |
| `base.js`         | theme, language, icons, brand marks, shell, emergency stop, copy (about 140 lines, no dependencies)      |
| `icons/`          | the original SVG brand marks (3 files), see section 9; the pages use the same paths inlined in `base.js` |

HTML, CSS and JS together are about 1900 lines. The interface icons are hand-drawn line icons embedded in `base.js`; the AI brand marks come from an open-source icon set (section 9) and are also inlined in `base.js`. No CDN script is used. The only external request is Google Fonts.

## 9. Brand marks

An AI avatar (`.av.ai`) is a square that holds the vendor's mark; a person's avatar is still a round text avatar. A mark is referenced as `<svg><use href="#b-claude"/></svg>` from the symbol table in `base.js`; to change a mark, change only the referenced name.

| Used for                             | Mark                       | File                     | Color                                                                                                                   |
| ------------------------------------ | -------------------------- | ------------------------ | ----------------------------------------------------------------------------------------------------------------------- |
| Claude Code, the secretary           | Anthropic Claude starburst | `icons/claude-color.svg` | official orange `#D97757`                                                                                               |
| Codex (also used for the GPT series) | OpenAI                     | `icons/openai.svg`       | official single-color version that follows the text color: black in the light theme, white (inverted) in the dark theme |
| Gemini (API key)                     | Google Gemini              | `icons/gemini-color.svg` | official color version                                                                                                  |

**Source, version, license:** the open-source icon set `@lobehub/icons-static-svg`, pinned at version 1.104.0, MIT license (see the `license` field of the package's `package.json`). The download address has the form `https://unpkg.com/@lobehub/icons-static-svg@1.104.0/icons/<name>.svg`. The original files are in `icons/` unchanged, and `BRANDS` in `base.js` holds the same path data (Gemini's three same-shaped layers are merged into one path and stacked three times with `<use>`; the shape is unchanged). No other AI such as Cursor appears in the mockups, so none was downloaded; when one appears, download it from the same version and the same path.

**Trademark notice:** each mark is a trademark of its owner, used only to identify the matching AI product, and does not imply endorsement.

**Usage rules:** follow each vendor's brand guidelines. Do not distort a mark (no stretching, cropping, outline or shadow); do not recolor it, use only the versions the vendor provides: color versions keep their original colors, OpenAI uses only the official single-color version, turned white (inverted) in the dark theme. A mark takes up 62% of its square, with nothing else placed in the margin around it.

**Avatar square:** the background is always the card color `--surface` (white in the light theme, ink blue in the dark theme) and the border is `--line-strong`. With this, the orange has a contrast of 3.1:1 and the Gemini blue 3.5:1 in the light theme, and 5.7:1 and 5.1:1 in the dark theme; the OpenAI single-color version is above 15:1 in both themes, and the border above 3.4:1; none is below 3:1.

**The secretary:** the secretary is an AI, its avatar is also a square showing the mark of the model it uses, with a round ink badge reading "secretary" (the Chinese character 秘) at the top right (black text on white in the dark theme). The mockups do not state which model the secretary runs on; here it is taken from the existing data: the secretary is on Alex's MacBook with a personal-subscription account, and the account registered on that computer is a Claude Code personal subscription, so the secretary uses the Claude mark. If that is not the case, change the referenced name. In group chat messages, the member bar, threads, the computers and AIs page and the queued messages on the terminal page, the text "Secretary" also sits next to the secretary's avatar; the badge is a second layer of hint.

**Sample data:** "Gemini CLI" was changed to "Gemini (API key)": Google offers only API keys, and Gemini CLI no longer serves personal subscriptions. In @ mentions the short name `@Gemini` is used.

## 10. How it was checked, and the results

- **Desktop width:** headless Chrome took screenshots of every page at 1440px width, in both dark and light; the group chat page was also viewed at 1100px (the three-column layout without the group list column).
- **Phone width:** every page was opened in the built-in browser's 375x812 phone emulation, and a script on each page found every element whose right edge goes beyond the screen or beyond its scroll container. At first, cards on the group chat page stretched the main line; that was fixed, and the result for all eight pages is now 0 overflowing elements. Every page's screenshot was viewed, and the group chat page was also checked with all folded areas expanded.
- **Tablet width:** the group chat page was checked at 820px, including the member bar sliding out, the emergency stop overlay and the pause banner.
- **English:** the overflow check was rerun on the onboarding and settings pages at 375px after switching to English.
- **Brand marks (2026-11-20):** headless Chrome opened the group chat, group list, computers and AIs, component overview and terminal pages at 375, 768 and 1440px, in the light and dark themes, and a script checked overflowing elements and the AI avatars (they must be square, with the mark not exceeding the square): all passed; at 1440px on the group list page one name is cut off with an ellipsis ("Gemini (API key) · Sam's Linux"), which is the designed behavior. The screenshots of each page's avatars, the secretary badge and the inverted OpenAI mark in the dark theme were viewed.
- **Script:** there are no console errors on any page; fonts load normally.

Checks not done: the pages were not opened on a real phone; they were not viewed in Safari and Firefox; nobody walked through them with a screen reader.

## 11. Known limitations

- `light-dark()` and CSS nesting need a browser from spring 2024 or later (Chrome 123, Safari 17.5, Firefox 120). On iOS 17.4 and earlier the pages open without color. If the real front end must support older systems, write each color as separate light and dark parts; the token names stay the same.
- The shell (navigation, top right of the top bar) is inserted by `base.js`, so that the eight static pages share one copy. In the real front end it is a component.
- The terminal area is static text and is not connected to xterm.js.
- Buttons such as "Approve" and "Confirm start" do nothing when clicked in the mockups; what really works is the theme, the language, the emergency stop, copying the prompt, expanding and collapsing threads, the member bar switch, and the various switches and choices.
- The names of people and computers, pairing codes, fingerprints and domains in the examples are all made up.
