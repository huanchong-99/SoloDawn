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
import States from './States.svelte';
import StateSheet from '../../routes/dev/States.svelte';

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

it('includes the preset area in the development states sheet', () => {
  show(StateSheet, {});
  expect(texts('.preset summary b')).toEqual(['Style presets']);
  expect(all('.preset h2')).toEqual([]);
});

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

async function submit(selector = '.preset form') {
  one(selector).dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }));
  await tick();
  flushSync();
  await vi.waitFor(() => {
    flushSync();
    if (selector === 'dialog form') expect(all('dialog')).toEqual([]);
    else expect(one<HTMLTextAreaElement>('textarea').disabled).toBe(false);
  });
}

it('shows fetch refusal, an empty list, and the human-member restriction', async () => {
  request.mockResolvedValueOnce({ re: 1, err: { code: 'forbidden' } });
  show(PresetPanel, { group });
  await open();
  expect(texts('.preset [role=alert]')).toEqual(['Not sent: this device may not do that.']);
  await closePage();
  group.apply([{ ...group.me!, online: false, seq: group.me!.seq + 1 }]);
  show(PresetPanel, { group });
  await open();
  expect(texts('.preset .hint')).toEqual([
    'Only an online human member can write presets.',
    'No presets yet.',
  ]);
});

it('retains text on write refusal, cancels an edit and reuses the forget dialog', async () => {
  group.apply([row('row-note-preset')]);
  show(PresetPanel, { group });
  await open();
  one<HTMLButtonElement>('.mem-item button').click();
  flushSync();
  input('textarea', 'Changed');
  request.mockResolvedValueOnce({ re: 1, err: { code: 'bad_request' } });
  await submit();
  expect(texts('.preset [role=alert]')).toEqual([
    'Not sent: the home refused it as too long or malformed.',
  ]);
  expect(one<HTMLTextAreaElement>('textarea').value).toBe('Changed');
  all<HTMLButtonElement>('.preset form button')[1].click();
  flushSync();
  expect(one<HTMLInputElement>('input').value).toBe('');
  expect(one<HTMLTextAreaElement>('textarea').value).toBe('');
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

it('renders the interactive states demo and creates, replaces and deletes a preset', async () => {
  show(States, {});
  await open();
  input('input', 'New');
  input('textarea', 'First');
  await submit();
  expect(texts('.mem-item > b')).toEqual(['Calm dashboard', 'New']);
  all<HTMLButtonElement>('.mem-item button')[2].click();
  flushSync();
  input('textarea', 'Second');
  await submit();
  expect(texts('.mem-item .mem-text')).toEqual([
    'Principles: one accent colour; no motion longer than 200 ms.',
    'Second',
  ]);
  all<HTMLButtonElement>('.del')[1].click();
  flushSync();
  await submit('dialog form');
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
