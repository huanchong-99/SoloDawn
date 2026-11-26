import { expect, it } from 'vitest';
import { changesOf, draftOf, limitFields } from './groupset';
import { goldens } from './mock';
import * as p from './proto';

const info = p.decode(p.GroupRow, goldens['row-group']);
const limits = {
  rounds: 10,
  wakes_per_hour: 20,
  turn_minutes: 60,
  failures: 3,
  review_rounds: 5,
  ask_depth: 4,
  ask_wait_s: 45,
  approval_wait_s: 600,
  idle_notice_minutes: 30,
};

/** The fields of the command after a change of the draft, when the row now has `now`. */
function after(change: (draft: ReturnType<typeof draftOf>) => void, now = limits) {
  const row = { ...info, limits };
  const draft = draftOf(row);
  change(draft);
  return changesOf(draftOf(row), draft, now);
}

it('shows every limit of the row once, the wait for an approval in minutes', () => {
  expect(limitFields).toEqual([
    { key: 'rounds', per: 1 },
    { key: 'wakes_per_hour', per: 1 },
    { key: 'turn_minutes', unit: 'min', per: 1 },
    { key: 'failures', per: 1 },
    { key: 'review_rounds', per: 1, min: 0 },
    { key: 'idle_notice_minutes', unit: 'min', per: 1 },
    { key: 'approval_wait_s', unit: 'min', per: 60, max: 10 },
    { key: 'ask_wait_s', unit: 's', per: 1, max: 55 },
    { key: 'ask_depth', per: 1, min: 0, max: 4 },
  ]);
  expect(draftOf({ ...info, limits }).limits).toEqual({ ...limits, approval_wait_s: 10 });
});

it('drafts the row as it is, in a copy', () => {
  const draft = draftOf({ ...info, name: 'Website', secretary: 'm7' });
  expect(draft).toStrictEqual({
    name: 'Website',
    pass: info.pass,
    pass_foundation: info.pass_foundation,
    limits: { ...info.limits, approval_wait_s: info.limits.approval_wait_s / 60 },
    secretary: 'm7',
  });
  expect(draft.limits).not.toBe(info.limits);
  expect(draftOf({ ...info, secretary: undefined }).secretary).toBe('');
});

it('sends only what changed', () => {
  expect(after(() => {})).toStrictEqual({});
  expect(after((draft) => (draft.name = ` ${info.name} `))).toStrictEqual({});
  expect(after((draft) => (draft.name = ' Docs '))).toStrictEqual({ name: 'Docs' });
  expect(after((draft) => (draft.pass = 80))).toStrictEqual({ pass: 80 });
  expect(after((draft) => (draft.pass_foundation = 60))).toStrictEqual({ pass_foundation: 60 });
  expect(after((draft) => (draft.secretary = 'm8'))).toStrictEqual({ secretary: 'm8' });
});

it('sends the limits whole, with those of the row now where the person left them', () => {
  expect(after((draft) => (draft.limits.rounds = 12))).toStrictEqual({
    limits: { ...limits, rounds: 12 },
  });
  const now = { ...limits, rounds: 7, failures: 2 };
  expect(after((draft) => (draft.limits.rounds = 12), now)).toStrictEqual({
    limits: { ...now, rounds: 12 },
  });
  expect(after((draft) => (draft.limits.approval_wait_s = 2.51))).toStrictEqual({
    limits: { ...limits, approval_wait_s: 151 },
  });
});
