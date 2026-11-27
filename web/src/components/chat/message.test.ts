import { flushSync } from 'svelte';
import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import { browser, example, owner, TestHome } from '../../../features/home';
import { all, closePage, openPage, scrollToTop, show } from '../../../features/page';
import { Group, Home, homes } from '../../lib/groups.svelte';
import { goldens } from '../../lib/mock';
import { prefs } from '../../lib/prefs.svelte';
import * as p from '../../lib/proto';
import Message from './Message.svelte';

beforeEach(() => {
  prefs.lang = 'en';
});
afterEach(closePage);

it("draws the picture of a member that left from the name it left with, not '?'", () => {
  const me = { user: 'alex', sign: async (payload: string) => ({ p: payload, k: 'k', s: 's' }) };
  const person = p.decode(p.HumanMemberRow, goldens['row-member-human']);
  const decoded = p.decode(p.MsgRow, goldens['row-msg-text']);
  const text = { ...decoded, by: 'mina' };
  const group = new Group(new Home(me), person.g);
  group.apply([
    { ...person, id: 'mina', name: 'Mina', seq: 1 },
    { ...text, seq: 2 },
  ]);
  group.apply([{ r: 'member', id: 'mina', seq: 3, gone: true }]);
  show(Message, { group, msg: text, joined: false });
  expect(all('.av').map((av) => av.textContent?.trim())).toEqual(['M']);
});

it('marks messages above a gap on each message shape', () => {
  const me = { user: 'alex', sign: async (payload: string) => ({ p: payload, k: 'k', s: 's' }) };
  const groupRow = p.decode(p.GroupRow, goldens['row-group']);
  const group = new Group(new Home(me), groupRow.id);
  const plain = { ...p.decode(p.MsgRow, goldens['row-msg-text']), id: 'plain-above' };
  const below = { ...p.decode(p.MsgRow, goldens['row-msg-text']), id: 'plain-below' };
  const note = {
    ...p.decode(p.MsgRow, goldens['row-msg-progress']),
    id: 'note-above',
    by: 'system',
  };
  const sys = { ...p.decode(p.MsgRow, goldens['row-msg-system']), id: 'sys-above', by: 'system' };
  group.apply([groupRow]);
  show(Message, { group, msg: plain, joined: false, above: true });
  show(Message, { group, msg: below, joined: false });
  show(Message, { group, msg: note, joined: false, above: true });
  show(Message, { group, msg: sys, joined: false, above: true });
  expect(all('.msg').map((el) => [el.tagName, el.id, el.classList.contains('above')])).toEqual([
    ['ARTICLE', 'm-plain-above', true],
    ['ARTICLE', 'm-plain-below', false],
    ['P', 'm-note-above', true],
    ['DIV', 'm-sys-above', true],
  ]);
});

it('marks only messages and whole days above group.gap and clears once the gap closes', async () => {
  const g = 'rZNvy-1jH6Z-BcPqA5U5BQ';
  const home = new TestHome();
  home.add(g, 'Website');
  const today = new Date().setHours(0, 0, 0, 0);
  home.write(g, example('row-msg-approval-pending', { id: 'old-day-card', ts: today - 60_000 }));
  home.write(g, example('row-msg-approval-pending', { id: 'same-day-card', ts: today + 10_000 }));
  for (let i = 0; i < 50; i += 1) {
    home.say(g, owner, `message ${i + 1}`, { ts: today + 20_000 + i * 1_000 });
  }
  const reader = browser(home);
  homes.push(reader.store);
  await reader.connect();
  await vi.waitFor(() => expect(home.synced).toBe(1));
  await new Promise((done) => setTimeout(done));
  flushSync();

  const group = reader.store.groups[0];
  openPage(`#/groups/${g}`);
  expect(group.gap).toBe(group.main[2].pos);
  expect(all('.day').map((day) => day.classList.contains('above'))).toEqual([true, false]);
  expect(all('.msg.above').map((el) => el.id)).toEqual(['m-old-day-card', 'm-same-day-card']);

  scrollToTop();
  await vi.waitFor(() => expect(group.paging().more).toBe(false));
  flushSync();
  expect(group.gap).toBe(0);
  expect(all('.day.above')).toEqual([]);
  expect(all('.msg.above')).toEqual([]);
});

it('keeps group.gap fixed and marks nothing above when live messages arrive in ordinary reading', async () => {
  const g = 'rZNvy-1jH6Z-BcPqA5U5BQ';
  const home = new TestHome();
  home.add(g, 'Website');
  const today = new Date().setHours(0, 0, 0, 0);
  for (let i = 0; i < 50; i += 1) {
    home.say(g, owner, `message ${i + 1}`, { ts: today + 20_000 + i * 1_000 });
  }
  const reader = browser(home);
  homes.push(reader.store);
  await reader.connect();
  await vi.waitFor(() => expect(home.synced).toBe(1));
  await new Promise((done) => setTimeout(done));
  flushSync();

  const group = reader.store.groups[0];
  const initialGap = group.gap;
  openPage(`#/groups/${g}`);
  expect(all('.day.above')).toEqual([]);
  expect(all('.msg.above')).toEqual([]);

  for (let i = 0; i < 3; i += 1) home.say(g, owner, `live ${i + 1}`);
  await new Promise((done) => setTimeout(done));
  flushSync();
  expect(group.gap).toBe(initialGap);
  expect(all('.day.above')).toEqual([]);
  expect(all('.msg.above')).toEqual([]);
});
