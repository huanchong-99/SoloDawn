import { flushSync, tick } from 'svelte';
import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import { Group, Home } from '../../lib/groups.svelte';
import { goldens } from '../../lib/mock';
import { prefs } from '../../lib/prefs.svelte';
import { decode, Row, type Ctl, type HumanMemberRow, type MsgRow } from '../../lib/proto';
import { all, closePage, one, show, texts } from '../../../features/page';
import BriefCard from '../brief/BriefCard.svelte';
import BriefPreset from './BriefPreset.svelte';
import PresetPanel from './PresetPanel.svelte';

const row = (name: string, overrides: object = {}) =>
  ({ ...decode(Row, goldens[name]), ...overrides }) as Row;
let group: Group;
let request: ReturnType<typeof vi.fn>;

beforeEach(() => {
  prefs.lang = 'en';
  const owner = decode(Row, goldens['row-member-human']) as HumanMemberRow;
  const home = new Home({ user: owner.user, sign: async (p) => ({ p, k: 'key', s: 'signature' }) });
  group = new Group(home, owner.g);
  group.apply([owner, row('row-group')]);
  request = vi.fn(async () => ({ re: 1, ok: true, rows: [], more: false }));
  home.ctl = { request: request as Ctl['request'] };
});
afterEach(closePage);

async function open(selector = '.preset details') {
  one<HTMLDetailsElement>(selector).open = true;
  one(selector).dispatchEvent(new Event('toggle'));
  await tick();
  flushSync();
  await vi.waitFor(() => {
    flushSync();
    expect(all('[role=status]')).toEqual([]);
  });
}

function input(selector: string, value: string) {
  const el = one<HTMLInputElement>(selector);
  el.value = value;
  el.dispatchEvent(new Event('input', { bubbles: true }));
  flushSync();
}

async function confirmDelete() {
  one('dialog form').dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }));
  await tick();
  flushSync();
  await vi.waitFor(() => {
    flushSync();
    expect(all('dialog')).toEqual([]);
  });
}

it('shows fetch refusal and an empty list without requiring member presence', async () => {
  request.mockResolvedValueOnce({ re: 1, err: { code: 'forbidden' } });
  show(PresetPanel, { group });
  await open();
  expect(texts('.preset [role=alert]')).toEqual(['Not sent: this device may not do that.']);
  await closePage();
  group.apply([{ ...group.me!, online: false, seq: group.me!.seq + 1 }]);
  show(PresetPanel, { group });
  await open();
  expect(texts('.preset .hint')).toEqual(['No presets yet.']);
  expect(one<HTMLButtonElement>('.preset .add').disabled).toBe(false);
});

const field = (label: string) => one<HTMLInputElement>(`.preset [aria-label="${label}"]`);

async function save(count: number) {
  one<HTMLFormElement>('.preset .edit-field').requestSubmit();
  await vi.waitFor(() => {
    flushSync();
    expect(request.mock.calls.filter(([req]) => req.t === 'cmd')).toHaveLength(count);
  });
  await tick();
  flushSync();
}

it('keeps a refused change open, cancels it and reuses the forget dialog', async () => {
  group.apply([row('row-note-preset')]);
  show(PresetPanel, { group });
  await open();
  one<HTMLButtonElement>('.preset .edit').click();
  flushSync();
  expect(texts('.preset .mem-item > b')).toEqual([]);
  input('.preset textarea', 'Changed');
  request.mockResolvedValueOnce({ re: 1, err: { code: 'bad_request' } });
  await save(1);
  expect(texts('.preset [role=alert]')).toEqual([
    'Not sent: the home refused it as too long or malformed.',
  ]);
  expect(field('Preset text').value).toBe('Changed');
  one<HTMLButtonElement>('.preset .edit-field .btn-ghost').click();
  await tick();
  flushSync();
  expect(all('.preset .edit-field')).toEqual([]);
  expect(texts('.preset .mem-text')).toEqual([
    'Principles: one accent colour; no motion longer than 200 ms.',
  ]);
  one<HTMLButtonElement>('.del').click();
  flushSync();
  expect(texts('dialog .preview')).toEqual(['Calm dashboard']);
  one<HTMLButtonElement>('dialog .btn-ghost').click();
  await tick();
  flushSync();
  expect(all('dialog')).toEqual([]);
});

it('shows brief fetch refusal and offline state', async () => {
  request.mockResolvedValueOnce({ re: 1, err: { code: 'forbidden' } });
  show(BriefPreset, { group, name: 'Calm dashboard' });
  await open('details');
  expect(texts('[role=alert]')).toEqual(['Not sent: this device may not do that.']);
  await closePage();
  group.home.ctl = undefined;
  show(BriefPreset, { group, name: 'Calm dashboard' });
  await open('details');
  expect(texts('.hint')).toEqual(['Cannot reach the group home. Presets are read-only.']);
});

it('creates, renames and deletes a preset, and refuses a second of one name', async () => {
  group.apply([row('row-note-preset')]);
  let seq = 10000;
  request.mockImplementation(async (req) => {
    if (req.t === 'cmd') {
      const cmd = JSON.parse(req.env.p);
      if (cmd.t === 'forget') group.apply([{ r: 'note', id: cmd.id, seq: ++seq, gone: true }]);
      else group.apply([{ ...row('row-note-preset'), ...cmd, r: 'note', seq: ++seq, pos: seq }]);
    }
    return { re: 1, ok: true, rows: [], more: false };
  });
  show(PresetPanel, { group });
  await open();
  one<HTMLButtonElement>('.preset .add').click();
  flushSync();
  input('.preset input', ' Calm dashboard ');
  input('.preset textarea', 'First');
  one<HTMLFormElement>('.preset .edit-field').requestSubmit();
  await vi.waitFor(() =>
    expect(texts('.preset [role=alert]')).toEqual([
      'Not saved: another preset already has that name.',
    ]),
  );
  input('.preset input', ' New ');
  await save(1);
  expect(texts('.mem-item > b')).toEqual(['Calm dashboard', 'New']);
  one<HTMLButtonElement>('button[aria-label="Change preset: New"]').click();
  flushSync();
  input('.preset input', 'Newer');
  input('.preset textarea', 'Second');
  await save(2);
  const [, edit] = request.mock.calls
    .filter(([req]) => req.t === 'cmd')
    .map(([req]) => JSON.parse(req.env.p));
  expect(edit).toMatchObject({ t: 'note_edit', name: 'Newer', text: 'Second' });
  expect(texts('.mem-item > b')).toEqual(['Calm dashboard', 'Newer']);
  expect(texts('.mem-item .mem-text')).toEqual([
    'Principles: one accent colour; no motion longer than 200 ms.',
    'Second',
  ]);
  all<HTMLButtonElement>('.del')[1].click();
  flushSync();
  await confirmDelete();
  expect(texts('.mem-item > b')).toEqual(['Calm dashboard']);
  expect(document.activeElement).toBe(one('.preset summary'));
});

it('shows a brief foundation pass fallback and closes revision with Escape', async () => {
  const msg = decode(Row, goldens['row-msg-brief-preset']) as Extract<MsgRow, { kind: 'brief' }>;
  group.apply([row('row-group', { seq: 1000, pass_foundation: undefined })]);
  show(BriefCard, { group, msg });
  expect(texts('.split .muted')).toContain('Pass score 90');
  const change = all<HTMLButtonElement>('button').find(
    (el) => el.textContent?.trim() === 'Change it',
  )!;
  change.click();
  flushSync();
  one('textarea').dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }));
  await tick();
  flushSync();
  expect(all('textarea')).toEqual([]);
});

it('retains a revision draft across a temporary loss of permission', async () => {
  const original = decode(Row, goldens['row-msg-brief-pending']) as Extract<
    MsgRow,
    { kind: 'brief' }
  >;
  const msg = { ...original, body: { ...original.body, requester: 'another-member' } };
  show(BriefCard, { group, msg });
  all<HTMLButtonElement>('button')
    .find((button) => button.textContent?.trim() === 'Change it')!
    .click();
  flushSync();
  input('textarea', 'Draft');
  const member = group.me!;
  group.apply([{ ...member, role: 'member', seq: member.seq + 1 }]);
  flushSync();
  expect(all('textarea')).toEqual([]);
  group.apply([{ ...member, seq: member.seq + 2 }]);
  flushSync();
  expect(one<HTMLTextAreaElement>('textarea').value).toBe('Draft');
});

it('returns focus to the brief when membership disappears during an act', async () => {
  const msg = decode(Row, goldens['row-msg-brief-pending']) as Extract<MsgRow, { kind: 'brief' }>;
  request.mockImplementation(async () => {
    const member = group.me!;
    group.apply([{ r: 'member', id: member.id, seq: member.seq + 1, gone: true }]);
    return { re: 1, ok: true };
  });
  show(BriefCard, { group, msg });
  const confirm = all<HTMLButtonElement>('button').find(
    (button) => button.textContent?.trim() === 'Confirm & start',
  )!;
  confirm.focus();
  confirm.click();
  await vi.waitFor(() => {
    flushSync();
    expect(all<HTMLButtonElement>('button').map((button) => button.disabled)).toEqual([true, true]);
    expect(document.activeElement).toBe(one('.card'));
  });
});
