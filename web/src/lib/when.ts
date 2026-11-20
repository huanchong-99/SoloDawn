// Times as the chat shows them, in the reader's language and time zone.

const day = 24 * 60 * 60 * 1000;

/** The local midnight a time follows. */
export const dayOf = (ts: number) => new Date(ts).setHours(0, 0, 0, 0);

/** How many calendar days before today a time is. A day of a clock change is not 24 hours. */
const daysAgo = (ts: number, now: number) => Math.round((dayOf(now) - dayOf(ts)) / day);

const sameYear = (ts: number, now: number) =>
  new Date(ts).getFullYear() === new Date(now).getFullYear();

const format = (ts: number, lang: string, options: Intl.DateTimeFormatOptions) =>
  new Intl.DateTimeFormat(lang, options).format(ts);

/** "today" or "yesterday", in the language. */
const relative = (ago: number, lang: string) =>
  new Intl.RelativeTimeFormat(lang, { numeric: 'auto' }).format(0 - ago, 'day');

/** The time of day, as 09:58. */
export const clock = (ts: number, lang: string) =>
  format(ts, lang, { hour: '2-digit', minute: '2-digit', hourCycle: 'h23' });

/** The heading of a day in the main line: "today · November 20", "yesterday · November 19",
 *  "Wednesday, November 18", and "November 18, 2025" for a day of another year. */
export function dayLabel(ts: number, now: number, lang: string) {
  const ago = daysAgo(ts, now);
  if (ago === 0 || ago === 1)
    return `${relative(ago, lang)} · ${format(ts, lang, { month: 'long', day: 'numeric' })}`;
  return sameYear(ts, now)
    ? format(ts, lang, { weekday: 'long', month: 'long', day: 'numeric' })
    : format(ts, lang, { year: 'numeric', month: 'long', day: 'numeric' });
}

/** When something happened, in a few characters, for a list: the time today, "yesterday", the
 *  weekday within the last week, then the date, with its year for another year. */
export function shortWhen(ts: number, now: number, lang: string) {
  const ago = daysAgo(ts, now);
  if (ago < 0 || ago > 6)
    return sameYear(ts, now)
      ? format(ts, lang, { month: 'short', day: 'numeric' })
      : format(ts, lang, { year: 'numeric', month: 'short', day: 'numeric' });
  if (ago > 1) return format(ts, lang, { weekday: 'short' });
  return ago ? relative(ago, lang) : clock(ts, lang);
}

/** How long after a message the next one of the same author still belongs to it. */
const together = 5 * 60 * 1000;

/** Messages by the day they were written on, in the order given. `joined` marks a message that
 *  follows one of the same author within five minutes on that day: it is drawn under it, with no
 *  head of its own. A day can come twice, when a writer's clock was off. */
export function days<M extends { ts: number; by: string }>(msgs: M[]) {
  const out: { day: number; msgs: { msg: M; joined: boolean }[] }[] = [];
  let last: M | undefined;
  for (const msg of msgs) {
    const day = dayOf(msg.ts);
    if (out.at(-1)?.day !== day) {
      out.push({ day, msgs: [] });
      last = undefined;
    }
    const joined = last?.by === msg.by && msg.ts - last.ts < together;
    out.at(-1)!.msgs.push({ msg, joined });
    last = msg;
  }
  return out;
}
