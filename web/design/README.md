# SoloDawn interface design notes

Date: 2026-11-20. Status: mockups; the real front end is written after the owner has reviewed them.

Open `index.html` for the table of contents. All pages are static HTML and need no build; open them directly in a browser.

## 1. Design idea

**In one sentence: warm color is kept for the places that need you.**

This product carries a lot of information: several people, several computers and several AIs all talking at once. The user really needs to know only three things: what the AIs are doing, where they are stuck, and who they are waiting for. So the whole design follows one rule:

- The interface as a whole is a calm neutral. The dark theme is the ink blue before dawn; the light theme is paper white in morning light.
- The whole system has exactly one warm color, called "dawn". It appears only on things a person has to handle: waiting for your approval, waiting for your decision, waiting for you to confirm the start, a restricted connection that needs your attention.
- Wherever a screen is warm, something there is waiting for you. No warm color means nothing needs you right now.

This is also how the name "dawn" is used: no sun is drawn and no gradient is spread; a line of light shows only where a person is needed.

Four more practices run through everything:

1. **The main line does not flood.** The main line holds only requests, task briefs, task cards, approvals, decisions and completion summaries. The AIs' progress, their conversations with each other and the reviews are folded into the "thread" at the bottom of a task card, which shows a one-line summary by default.
2. **Waiting for you and waiting for others look different.** Waiting for you is a solid warm fill; waiting for others is a gray hollow circle that names who is being waited for.
3. **People are round, AIs are square.** The avatar shape tells a person from an AI at a glance. An AI's square holds the brand mark of its vendor; the secretary shows the mark of the model behind it, plus a "secretary" corner badge.
4. **State is not carried by color alone.** Working is a breathing dot, waiting is a hollow circle, sleeping is a crescent, paused is two vertical bars, taken over is a square. Users with color blindness can tell them apart.

## 2. Relation to pachat.ai

I looked at its home page, and took only its level of finish: whitespace, corner radii and consistent font weights. Layout, colors, brand and illustrations were not reused. It is soft pastel gradients with a cartoon mascot, for a general audience; SoloDawn is a workbench for developers to watch AIs work, so it takes a restrained, information-dense route that uses outlines instead of shadows.

## 3. Design tokens

All of them are in `tokens.css`, as CSS variables. Each color is one line written as `light-dark(light value, dark value)`, and switching themes changes only `color-scheme`. Without a `data-theme` attribute the system setting is followed.

### 3.1 Colors

| Token           | Light     | Dark      | Use                                          |
| --------------- | --------- | --------- | -------------------------------------------- |
| `--bg`          | `#F6F4EF` | `#0C0F16` | page background                              |
| `--surface`     | `#FFFFFF` | `#141822` | cards, panels                                |
| `--surface-2`   | `#EFECE5` | `#1C212D` | inset blocks, hover                          |
| `--surface-3`   | `#E5E1D8` | `#262C3A` | pressed, slider tracks                       |
| `--line`        | `#E2DED5` | `#272C39` | dividers, card borders                       |
| `--line-strong` | `#8C8982` | `#646C7E` | input borders                                |
| `--text`        | `#171A21` | `#ECEEF3` | body text                                    |
| `--text-2`      | `#4A505C` | `#B3B9C6` | secondary text                               |
| `--text-3`      | `#5F646F` | `#8E95A5` | auxiliary text                               |
| `--sky`         | `#2A55D6` | `#8CABFF` | sky: an AI is working, links, keyboard focus |
| `--dawn`        | `#F2A036` | `#F6B152` | dawn: waiting for you (fill color)           |
| `--dawn-text`   | `#8A4B00` | `#F6B152` | dawn-colored text on ordinary backgrounds    |
| `--on-dawn`     | `#2A1A00` | `#2A1A00` | text on a dawn fill                          |
| `--ok`          | `#146F44` | `#62CC91` | passed, online                               |
| `--danger`      | `#B92E27` | `#FF857D` | failed, rejected, emergency stop             |

There are only five semantic roles: neutral, sky, dawn, ok and danger. There is no separate "warning color"; reminders such as a restricted relay also use dawn, because they also mean "please take a look".

The four light backgrounds (`--sky-soft` and so on) are mixed from the semantic colors and the card color and have no values of their own. The terminal area has its own set, `--term-*`, which is dark in both themes.

The brand gradient `--horizon` (indigo, rose, amber) is used only on the logo and on a thin line on the contents page.

### 3.2 Fonts and sizes

- Latin text and numbers: Geist (Google Fonts). Chinese: the system font (PingFang, Microsoft YaHei). Code, branch names and the terminal: Geist Mono.
- Without a network everything falls back to system fonts and the pages display as usual.
- Numbers use tabular figures (`tabular-nums`), so usage, times and scores line up.

| Token      | Size    | Used for                                                    |
| ---------- | ------- | ----------------------------------------------------------- |
| `--fs-xs`  | 12px    | times, badges, captions                                     |
| `--fs-sm`  | 13px    | messages inside threads, secondary card information         |
| `--fs-md`  | 15px    | body text, main-line messages                               |
| `--fs-lg`  | 17px    | card titles                                                 |
| `--fs-xl`  | 21px    | page titles                                                 |
| `--fs-2xl` | 28px    | large section titles                                        |
| `--fs-3xl` | 30-46px | main title of the onboarding page, varies with screen width |

Body line height is 1.6 (Chinese needs to be looser than English); headings use 1.25. The main line is 15px and threads are 13px; this one-step difference in size lets threads recede naturally.

### 3.3 Spacing, radii, shadows, motion

- **Spacing:** based on 4px, nine steps in all: 4, 8, 12, 16, 20, 24, 32, 48, 64.
- **Radii:** 6 (badges, code blocks), 10 (buttons, inputs), 14 (cards), 20 (overlays), full (pills).
- **Shadows:** cards use no shadow and are told apart by a 1px outline, which is cleaner in the dark theme and cheaper to render. Only overlays use one shadow step, `--shadow-pop`.
- **Motion:** two durations, 120ms for hover and 200ms for expanding, on the same easing curve. Only one thing moves all the time: the breathing dot for "working" (besides it, the onboarding page has a spinner while connecting and the terminal has a blinking cursor). All of it is turned off when the system asks for reduced motion.

### 3.4 Layout

| Width                              | Layout                                                                                                                                                            |
| ---------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| below 768px (phone)                | top bar + content + three-item bottom navigation. The group chat and terminal pages hide the bottom navigation to give the space to the input box and the key bar |
| 768-1023px (tablet, narrow window) | left navigation + content. The member bar slides out from the icon at the top right                                                                               |
| 1024-1279px                        | left navigation + chat + member bar                                                                                                                               |
| 1280px and above                   | left navigation + group list + chat + member bar                                                                                                                  |

Chat content is at most 760px wide and is not stretched further; a line holds no more than fifty Chinese characters.

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
