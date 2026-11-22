# Contract: presets and the knowledge base (step 3)

Two kinds of long-lived document per group, both stored as notes: **style
presets**, which a brief names and a task session is given, and the
**knowledge base**, which AIs and people search. This file settles design
points D-17 and D-18 of `steps/parallel-plan-step-2-3.md` and refines
3.2 and 3.3 of `steps/step-2-3.md`.

It adds to step 1 contracts without editing them; each section names the
file it adds to. No step 1 shape changes meaning: every addition is an
optional field, a new value of an existing field, or a new tool. Presence
follows `contracts/conventions.md` section 2.

## 1. The note row (adds to `contracts/rows.md` section 6)

A `note` row gains two optional fields:

```json
{"r":"note","id":"n7…","seq":902,"pos":871,"g":"Qm1…","task":null,
 "type":"preset","name":"Calm dashboard",
 "text":"Principles: one accent colour; no motion longer than 200 ms. …",
 "by":"<member id>","ts":1763700000000}
```

```json
{"r":"note","id":"n9…","seq":915,"pos":915,"g":"Qm1…","task":null,
 "type":"kb","name":"Decision: the home is the single writer",
 "text":"Every change to a group goes through its home. Reason: …",
 "by":"<member id>","ts":1763701000000}
```

| Field | Value |
| --- | --- |
| `type` | optional: `preset` or `kb`. A note without it is a context note, as in step 1 |
| `name` | optional: present exactly when `type` is. What the document is called: 1 to 80 characters, none of them a control character. Compared exactly, character by character |
| `task` | `null` on a note that has a `type` |
| `by`, `ts` | who wrote the current text, and when |
| `pos` | set when the note is created; it stays when the text is replaced |

- **One note per name.** In a group, at most one note of a given `type` has
  a given `name`. A preset and a knowledge-base document may share a name.
- **Size.** A document is one row, so it obeys the row limit of
  `contracts/conventions.md` section 4 (16 KiB): about 16,000 characters of
  English or 5,300 of Chinese, at 3 bytes each. A longer knowledge-base
  document is written as several documents; a longer preset is refused.
  No document is made of several rows.
- **Kept.** What `contracts/rows.md` section 6 and `contracts/storage.md`
  section 4 say of notes holds: `keep = 1`, never removed by retention,
  not part of `sync`, changes pushed like any row, deleted by an admin
  with `forget`, which leaves a `gone` marker and frees the name.
- A client keeps notes that have a `type` out of its list of context notes
  ("Project memory"), also when they arrive in a `rows` push.

## 2. Writing (adds to `contracts/commands.md` 1.6)

The `note` command gains two optional fields, `type` and `name`, sent
together or not at all. Without them it is the command of step 1.

| `type` | By | Effect |
| --- | --- | --- |
| `preset` | a person | the preset `name` is created or replaced |
| `kb` | a person, or an AI | the document `name` is created or replaced |

Result, only when `type` is sent: `id`, `replaced`.

- **Create.** If the group has no note of that `type` and `name`, the home
  writes a new row with the command's `id`; `replaced` is `false`.
- **Replace.** If it has one, that row gets the command's `text` and `ts`,
  the member id of the person or AI that sent it as `by`, and a new `seq`.
  Its `id` and `pos` stay; the command's own `id` is not used. The result
  has the row's `id` and `replaced: true`. No history: the old text is gone.
- A document is renamed by writing the new name and forgetting the old one.

Refused:

| Case | Code |
| --- | --- |
| one of `type`, `name` without the other; a `type` other than `preset` and `kb`; a `name` outside the rule of section 1; `task` not `null` | `bad_request` |
| the row would exceed the row limit (the message names the limit and the size) | `bad_request` |
| `type: preset` signed for an AI (`as` present) | `forbidden` |
| a new command whose `id` is already the id of another note of the group | `conflict` |

A preset is written by a person only, and by no tool: a person confirms
the brief that names it, so an AI must not be able to replace its text.

## 3. Reading (adds to `contracts/datachannel.md` section 4)

`fetch` gains two values of `kind` and one optional field.

```json
{"t":"fetch","id":7,"g":"Qm1…","kind":"preset","before":0,"limit":50}
{"t":"fetch","id":8,"g":"Qm1…","kind":"kb","before":915,"limit":50}
{"t":"fetch","id":9,"g":"Qm1…","kind":"kb","q":"single writer 路径","before":0,"limit":5}
```

| Request | Answer |
| --- | --- |
| `kind: note` | the context notes only: notes without a `type`. This is what a host puts into a first turn, so presets and documents never enter it this way |
| `kind: preset`, `kind: kb` | the notes of that type, paged by `pos`, newest first, as for `kind: note` |
| `kind: kb` with `q` | a search (section 6): the documents that match, best first |

`q` is optional and allowed only with `kind: kb`. With `q`:

- A search is not paged: `before` is 0, and `q` holds a character that is
  not white space; else `bad_request`.
- The answer is the usual `fetched`: `rows` holds the matching rows, best
  first, as many as fit one message, at most `limit`. A row is at most
  16 KiB, so the best three always fit. `more` is `true` when other
  documents matched and were left out; the way to them is a narrower query.
- No match is `rows: []` with `more: false`, not an error.

There is no read by name: a reader that needs one preset reads the presets
and takes the one with that name. A group has few.

## 4. The brief's preset (adds to `contracts/messages.md` 2.1)

The body of a `brief` gains one optional field:

```json
{"title":"Show the connection path; warn when relayed", …,
 "pass":90,"requester":"<member id>","replaces":null,
 "preset":"Calm dashboard"}
```

| Field | Value |
| --- | --- |
| `preset` | optional: the `name` of a preset of the group. Absent when the brief names none |

- Signed with the brief and fixed with its body: a brief names the same
  preset from proposal to merge. One preset per brief, for all its tasks;
  a task of `origin` `direct` or `takeover` has none.
- **Check** on `post` kind `brief` (`contracts/commands.md` 1.3): the
  group has a preset of that name, else `not_found`, and the message
  lists the names of the group's presets.
- **Card.** The brief card shows the name; the client reads the text with
  `fetch` `kind: preset`. A person who wants another preset, or none, sends
  the brief back (`act revise`), as for any other part of a brief; `act
  confirm` carries nothing about the preset.

### 4.1 In the turn (adds to `contracts/drivers.md` section 3)

The first turn of a task session carries the brief's goal and criteria
(`flows/task.md` step 7). When that brief has `preset`, the same turn
carries the preset, after the reviewer line and before the notes:

```
Reviewer: Codex   Pass score: 90
Style preset "Calm dashboard":
Principles: one accent colour; no motion longer than 200 ms. …
Notes of this group:
- Path events use fields path, rtt_ms, since, limited.
```

- When it composes the turn, the host reads the brief row, named by the task
  row's `brief`, with `fetch` `row` (`contracts/datachannel.md` section 4),
  for the goal, the criteria and the preset's name, and the text with `fetch`
  `kind: preset`. So the text is the one of that moment, not of the
  confirmation: a task that starts weeks later gets the current preset.
- The text is included whole, outside the 8 KiB of the notes.
- If the preset was forgotten since, the turn has one line in its place
  and the task goes on:
  `Style preset "Calm dashboard": deleted since the brief was proposed.`
- A review turn of that task carries the name only, one line after the
  pass score: `Style preset: "Calm dashboard"`. The reviewer reads the text
  with `note_list` `type: preset`. No other turn carries the preset.
- The preset is content, like the notes: the node checks nothing against
  it. Its authority is that of the `assign` item it is part of.

## 5. Tools (adds to `contracts/mcp-tools.md`)

Two new tools, for every AI in every kind of session (section 1 of that
file), and one new argument on each of two existing tools.

| Tool | Arguments | Returns | Maps to |
| --- | --- | --- | --- |
| `kb_put` | `name`; `text` | `{"id":"…","replaced":false}`, the command's result | `note` with `type: kb` |
| `kb_search` | `query`; `limit` (default 3, at most 20) | `{"documents":[{"id","name","text","by","at"}],"more":false}`, best first | `fetch` with `kind: kb` and `q` |
| `note_list` | adds `type` (optional): `preset` lists the presets, each entry with `name` beside the step 1 fields. Without `type`: the context notes, as in step 1 | `{"notes":[{"id","task","name","text","by","at"}]}` | `fetch` with `kind: preset`; without `type`, `kind: note` as in step 1 |
| `propose_brief` | adds `preset` (optional): a preset's name | as in step 1 | `post` kind `brief` |

- `kb_put` with a name that exists replaces that document; the result says
  so. The secret check (`contracts/local.md` section 8) reads both fields.
- `kb_search` returns whole documents, since no tool reads one. The result
  is one `fetched` answer, so at most 60 KiB.
- No tool lists the knowledge base: `note_list` takes no `type: kb`.
- An unknown preset in `propose_brief` is the home's `not_found`, whose
  text lists the valid names (the convention of that file for names).
- `tools.json` (section 5 of that file) gains `kb_put` and `kb_search` in
  all five lists, `type` on `note_list`, and `preset` on `propose_brief`.

Instructions (adds to `contracts/drivers.md` section 4, in its 60 lines):

- **All**: the group has a knowledge base of decisions, module contracts
  and conventions. Search it with `kb_search` before a design choice.
  Record a decision with `kb_put` under a name that says what it is about;
  the same name replaces the document. Notes (`note_put`) are short
  lessons every new task is given; the knowledge base is looked up.
- **Task session**: a style preset in the turn is followed for interface
  work.
- **Secretary**: for interface work, name a style preset in the brief when
  the group has one that fits (`note_list` with `type: preset`).

## 6. The index (adds to `contracts/storage.md` section 1)

One more home table, the full-text index of the knowledge base. It holds
only notes of type `kb`; presets are not searched.

```sql
CREATE VIRTUAL TABLE kb USING fts5(
  g UNINDEXED, id UNINDEXED, name, text, tokenize = 'unicode61'
);
CREATE UNIQUE INDEX row_named ON row (g, data->>'type', data->>'name')
  WHERE r = 'note' AND gone = 0 AND data->>'type' IS NOT NULL;
```

- `row` stays the record; `kb` is derived from it. The `note` and `forget`
  commands change both in one transaction: a replaced or forgotten
  document leaves the index, a written one enters it. The migration that
  creates `kb` fills it from `row`. Retention never touches it.
- `row_named` is the rule "one note per name" and the lookup of section 2
  and of the check of section 4.
- `fetch` `kind: note`, `preset` and `kb` without `q` read through
  `row_keep` with a condition on `data->>'type'`.

**Spreading.** FTS5's `unicode61` splits text at spaces and punctuation, so
a run of Chinese is one token and a word inside it is not found
`[verified]`. Its `trigram` tokenizer finds nothing shorter than three
characters, which rules out most Chinese words and `db` `[verified]`. So
before text enters the index, and before a search term is used, the home
writes a space on both sides of every character of the scripts Han,
Hiragana, Katakana and Hangul. Each such character is then a token, and a
word is a phrase of them.

**Matching.** The home builds the FTS5 query; no syntax of `q` reaches it:

1. split `q` at white space into terms;
2. spread each term, double every `"` in it and wrap it in `"`: a phrase;
3. join the phrases with ` OR `;
4. `SELECT id FROM kb WHERE kb MATCH ? AND g = ? ORDER BY bm25(kb)`.

| What follows, each row `[verified]` (section 9) | Example |
| --- | --- |
| a document matches when any term is in its name or text; more and rarer matches rank higher | `retention database` |
| letter case and accents do not matter | `resume` finds `RÉSUMÉ` |
| a term matches whole words, in order: no prefixes, no word stems | `path` does not find `paths` |
| punctuation inside a term separates words | `rtt_ms` is `rtt` followed by `ms` |
| Chinese is found by any run of characters, one included | `路径` finds `连接路径事件` |
| `AND`, `NOT`, `*`, `name:` and quotes are text, not operators | `a AND` is two terms |
| a term of punctuation only matches nothing | `---` |

## 7. Shapes to freeze (unit P-5)

P-5 freezes these in `internal/proto`, with golden files and the web
schema (`contracts/conventions.md` section 5): a `note` row of each new
type; a `brief` row whose body has `preset`; the `note` payload with
`type` and `name`, and its answer; `fetch` with `kind: preset`, and with
`kind: kb` and `q`; the tool lists in `tools.json`. The existing
`row-note.json` stays: a context note has neither field.

## 8. A home that predates step 3

Receivers ignore unknown fields (`contracts/conventions.md` section 2).
An older home sent `note` with `type` writes a context note, which then
enters first turns: wrong, and silent. It answers `bad_request` to `fetch`
with `kind: preset` or `kb`, and stores a brief with `preset` unchecked. So
a client offers presets and the knowledge base, and a host lists `kb_put`,
`kb_search` and the new arguments, only for a group whose home knows
step 3: a home that answers `welcome` with `v` 2. Decided: one protocol
step, `v` 2, covers every addition of steps 2 and 3. A client sends `hello`
with `v` 2 and, answered `version`, again with `v` 1, as a host does; it
then offers nothing of steps 2 and 3. P-4a freezes it; T2-1 builds it.

## 9. Verified, not verified, decided

`[verified]`, by a throwaway Go program against `modernc.org/sqlite`
v1.46.2, not kept in the repository:

- FTS5 is built in; `bm25` orders results; a `DELETE` by `g` and `id`
  removes a document from the index.
- The tokenizer facts and every row of the table in section 6, with
  Chinese, Japanese and Korean text.
- A raw `q` passed to `MATCH` is a syntax error for `a"b`, `a AND` and
  `---`; built as in section 6 all three are valid queries.
- `row_named`: a second note of the same type and name is refused, a
  `gone` marker is not, the lookup by name uses the index, and the three
  paged reads use `row_keep`.

`[unverified]`:

- That Claude Code and Codex accept a tool result of 60 KiB. If one does
  not, the default `limit` of `kb_search` goes down.
- Search speed, and whether 16 KiB is enough for real presets.
- Scripts without spaces outside section 6 (Thai, for one): not tried.

Decided by the coordinator on 2026-11-20: the read of one row by id
(section 4.1, built by HM-3); the preset's name in the review turn;
changes of preset through `revise` only, so U3-1 is "preset shown in the
brief card; editor"; protocol step `v` 2 (section 8).
