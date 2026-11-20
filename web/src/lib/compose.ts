// What the composer and a message's text need from a text: the mention being typed, the members
// a text mentions, its parts for display, and how much of a row it would fill.
import { utf8 } from './identity';

/** Someone who can be mentioned: a member's id and name. */
export interface Named {
  id: string;
  name: string;
}

/** The mention being typed at the caret: where its `@` is and what was typed after it. A name
 *  may hold spaces, so only a line break ends it. */
export function typing(text: string, caret: number) {
  const before = text.slice(0, caret);
  const at = before.lastIndexOf('@');
  const query = before.slice(at + 1);
  return at < 0 || query.includes('\n') ? undefined : { at, query };
}

/** The members whose name starts with what was typed, whatever its case. */
export const matching = <M extends Named>(members: M[], query: string) =>
  members.filter(({ name }) => name.toLowerCase().startsWith(query.toLowerCase()));

/** The text with the mention typed from `at` to the caret replaced by the member's name and a
 *  space, and the caret after it. */
export function complete(text: string, caret: number, at: number, name: string) {
  const head = `${text.slice(0, at)}@${name} `;
  return { text: head + text.slice(caret), caret: head.length };
}

/** A part of a text as it is shown: plain text, a code span, or the mention of a member. */
export interface Part {
  kind: 'text' | 'code' | 'at';
  text: string;
  /** The member an `at` names. */
  id?: string;
}

/** The member whose name follows the `@` at `at`: the longest, when one name begins another. A
 *  name that runs on into a letter, a digit or an underscore is another word. */
function named(text: string, at: number, members: Named[]) {
  const start = at + 1;
  return members
    .filter(
      ({ name }) => text.startsWith(name, start) && !/\w/.test(text.charAt(start + name.length)),
    )
    .sort((a, b) => b.name.length - a.name.length)
    .at(0);
}

/** Text outside code spans, cut at the mentions of the members given: what is before the first
 *  mention, the mention, and the rest in the same way. */
function plain(text: string, members: Named[]): Part[] {
  const out: Part[] = [];
  let from = 0;
  for (let at = text.indexOf('@'); at >= 0; at = text.indexOf('@', at + 1)) {
    const member = named(text, at, members);
    if (!member) continue;
    const end = at + 1 + member.name.length;
    out.push(
      { kind: 'text', text: text.slice(from, at) },
      { kind: 'at', text: text.slice(at, end), id: member.id },
    );
    from = end;
    at = end - 1;
  }
  out.push({ kind: 'text', text: text.slice(from) });
  return out;
}

/** A text as its parts: the code spans between backticks, the mentions of the members given, and
 *  the text between them. A backtick that nothing closes is text. */
export function parts(text: string, members: Named[]): Part[] {
  const pieces = text.split('`');
  if (pieces.length % 2 === 0) {
    const open = pieces.pop();
    pieces.push(`${pieces.pop()}\`${open}`);
  }
  return pieces
    .flatMap((piece, i): Part[] =>
      i % 2 ? [{ kind: 'code', text: piece }] : plain(piece, members),
    )
    .filter((part) => part.text !== '');
}

/** The ids of the members a text mentions, each once, in the order of their first mention. */
export const mentioned = (text: string, members: Named[]) => [
  ...new Set(parts(text, members).flatMap((part) => part.id ?? [])),
];

/** What is left in the field once `taken`, the text it held when a message was sent, is sent:
 *  what the writer typed after it since. A writer who changed the sent text itself keeps it all. */
export const unsent = (text: string, taken: string) =>
  text.startsWith(taken) ? text.slice(taken.length) : text;

/** The most a row may hold: 16 KiB (contracts/conventions.md section 4). */
export const rowLimit = 16 * 1024;
/** From here on the composer tells the writer how full the row is: three quarters of the limit. */
export const nearLimit = 12 * 1024;
/** What the row of a person's text message holds besides its text and its mentions, measured on
 *  the wire example row-msg-text.json. That row is 673 bytes: 38 are its text and 34 the `as`
 *  that only an AI's post has, which leaves 601. Its `seq` and `pos` have three digits each and
 *  can have sixteen, 26 bytes more. */
const fixed = 627;
/** What each mention adds: the member's id once in the row's `mentions` and once more, escaped,
 *  inside `env.p`. The first costs 50 bytes and each one after it 52. */
const perMention = 52;

const size = (text: string) => utf8(text).length;

/** The size of the row that a text message would make, by the rule of contracts/rows.md section
 *  5. The row holds the text twice: as `text`, and inside `env.p`, where it is escaped once more,
 *  so a letter costs 2 bytes, a line break 5, a quote or a backslash 6, a Chinese character 6.
 *  An estimate that is never under a row with those numbers of digits: the size the node measures
 *  decides. */
export function rowSize(text: string, mentions = 0) {
  const once = JSON.stringify(text);
  // Without the quotes around each: two bytes outside, and inside p two escaped ones more.
  const said = size(once) - 2 + size(JSON.stringify(once)) - 6;
  return fixed + perMention * mentions + said;
}

/** How full the row of a text would be, in hundredths of the limit, rounded up: a text that is
 *  over the limit is never told as 100. */
export const fill = (text: string, mentions = 0) =>
  Math.ceil((rowSize(text, mentions) / rowLimit) * 100);

/** Whether a text fits a row with room to spare (`ok`), fits but nears the limit (`near`), or
 *  is too long to send (`over`). */
export function room(text: string, mentions = 0) {
  const bytes = rowSize(text, mentions);
  if (bytes > rowLimit) return 'over';
  return bytes >= nearLimit ? 'near' : 'ok';
}
