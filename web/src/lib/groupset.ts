// The settings of a group that an admin can change (planning/contracts/commands.md 1.1 `group set`,
// planning/contracts/rows.md section 1).
import type { GroupRow, GroupSet } from './proto';

type Limits = GroupRow['limits'];

interface LimitField {
  key: keyof Limits;
  unit?: 'min' | 's';
  /** How many of the row's units are one of the unit shown. */
  per: number;
  /** The most the rules allow, in the unit shown. */
  max?: number;
  /** Set when 0 is a choice of its own (the default is 1). */
  min?: 0;
}

/** The limits, with the everyday ones first. `ask_depth` is R-114's; an answer to `ask` must come
 *  before a CLI gives up on the tool call (flows/ask.md D), and one to an approval within what the
 *  CLIs were seen to wait for (flows/approval.md H). */
export const limitFields: LimitField[] = [
  { key: 'rounds', per: 1 },
  { key: 'wakes_per_hour', per: 1 },
  { key: 'turn_minutes', unit: 'min', per: 1 },
  { key: 'failures', per: 1 },
  { key: 'review_rounds', per: 1, min: 0 },
  { key: 'idle_notice_minutes', unit: 'min', per: 1 },
  { key: 'approval_wait_s', unit: 'min', per: 60, max: 10 },
  { key: 'ask_wait_s', unit: 's', per: 1, max: 55 },
  { key: 'ask_depth', per: 1, min: 0, max: 4 },
];

/** The commands of a text with one on each line. */
export const commandsIn = (text: string) =>
  text
    .split('\n')
    .map((line) => line.trim())
    .filter(Boolean);

const scaled = (limits: Limits, by: (value: number, per: number) => number) =>
  Object.fromEntries(limitFields.map(({ key, per }) => [key, by(limits[key], per)])) as Limits;

/** The settings of a group as its dialog edits them. */
export const draftOf = (info: GroupRow) => ({
  name: info.name,
  checks: info.checks.join('\n'),
  pass: info.pass,
  pass_foundation: info.pass_foundation,
  limits: scaled(info.limits, (value, per) => value / per),
  secretary: info.secretary ?? '',
  templates: structuredClone(info.templates),
});

type Draft = ReturnType<typeof draftOf>;

const fieldsOf = (draft: Draft) => ({
  ...draft,
  name: draft.name.trim(),
  checks: commandsIn(draft.checks),
  limits: scaled(draft.limits, (value, per) => Math.round(value * per)),
});

function changed<T extends object>(before: T, after: T) {
  const same = (key: keyof T) => JSON.stringify(after[key]) === JSON.stringify(before[key]);
  return Object.fromEntries(
    Object.entries(after).filter(([key]) => !same(key as keyof T)),
  ) as Partial<T>;
}

/** The fields of `group set` that the draft changes. Limits are sent whole, so those the person
 *  left alone are the ones the group has `now`. */
export function changesOf(
  before: Draft,
  draft: Draft,
  now: Limits,
): Partial<Omit<GroupSet, 't' | 'n' | 'ts' | 'g' | 'op'>> {
  const was = fieldsOf(before);
  const is = fieldsOf(draft);
  const fields = changed(was, is);
  if (!fields.limits) return fields;
  return { ...fields, limits: { ...now, ...changed(was.limits, is.limits) } };
}
