import { chromium } from 'playwright';
import { flushSync } from 'svelte';
import { createServer } from 'vite';
import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import { browser, example, owner, TestHome } from '../../../features/home';
import { all, closePage, one, openPage, read, scrollToTop, show } from '../../../features/page';
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

it('keeps the separator and time together in a single nowrap span on system rows', async () => {
  const g = 'sys-dom-g';
  const home = new TestHome();
  home.add(g, 'Website');
  home.write(g, example('row-member-ai', { id: 'codex', name: 'Codex' }));
  const longDetail = `/very/long/unbroken/workspace/segment/${'x'.repeat(120)}/output.log`;
  home.write(
    g,
    example('row-msg-system', {
      id: 'sys-long',
      thread: null,
      ts: Date.parse('2026-11-28T09:58:00Z'),
      body: { code: 'turn_interrupted', ai: 'codex', reason: 'crash', detail: longDetail },
    }),
  );
  const client = browser(home);
  homes.push(client.store);
  await client.connect();
  await vi.waitFor(() => expect(home.synced).toBe(1));
  await new Promise((done) => setTimeout(done));
  flushSync();

  openPage(`#/groups/${g}`);
  const row = one('.sys .sys-row');
  const when = one('.sys .sys-row p .when');
  const time = one('.sys .sys-row p .when time');
  expect(read(when).startsWith('· ')).toBe(true);
  expect(read(when)).toBe(`· ${read(time)}`);
  expect(read(row)).toContain(longDetail);
  expect(read(row)).toMatch(/ · \d{1,2}:\d{2}(?: [AP]M)?$/);
});

it('wraps long system failure lines in Chromium without clipping <time> at 1440x900 or 375x812', async () => {
  const server = await createServer({
    logLevel: 'warn',
    server: { host: '127.0.0.1', port: 0 },
  });
  await server.listen();
  const baseUrl = server.resolvedUrls!.local[0];
  const execPath = process.env.CHROMIUM_PATH;
  const browserInst = await chromium.launch(execPath ? { executablePath: execPath } : {});
  try {
    const context = await browserInst.newContext({ viewport: { width: 1440, height: 900 } });
    const page = await context.newPage();
    await page.goto(`${baseUrl}?nomock`, { waitUntil: 'domcontentloaded' });
    await page.evaluate(`(async () => {
      const [{ ownedNode, example: ex }, { homes: h }, { guideState }, { prefs: pr }] =
        await Promise.all([
          import('/features/home.ts'),
          import('/src/lib/groups.svelte.ts'),
          import('/src/lib/guide.svelte.ts'),
          import('/src/lib/prefs.svelte.ts'),
        ]);
      pr.lang = 'en';
      guideState.value = 'open';
      const node = ownedNode('workstation', 'dev-ws');
      const g = 'sys-layout-g';
      node.home.add(g, 'Website', { secretary: 'sec' });
      node.home.write(g, ex('row-member-ai', { id: 'sec', name: 'Secretary', role: 'secretary' }));
      node.home.write(g, ex('row-member-ai', { id: 'codex', name: 'Codex' }));
      node.home.write(
        g,
        ex('row-msg-system', {
          id: 'sys-long',
          thread: null,
          ts: Date.parse('2026-11-28T09:58:00Z'),
          body: {
            code: 'turn_interrupted',
            ai: 'codex',
            reason: 'crash',
            detail:
              'exit code 1 while opening /extremely/long/unbroken/filesystem/path/with_a_very_long_segment_1234567890_abcdefghijklmnopqrstuvwxyz_0123456789/workspace/file.ts and extra trailing words to guarantee wrapping near the right edge',
          },
        }),
      );
      h.push(node.store);
      await node.connect();
      location.hash = '#/groups/' + g;
    })()`);

    await page.waitForSelector('.sys .sys-row time');
    await page.waitForSelector('.guide-dock');

    for (const viewport of [
      { width: 1440, height: 900 },
      { width: 375, height: 812 },
    ]) {
      await page.setViewportSize(viewport);
      const metrics = await page.evaluate(() => {
        const row = document.querySelector('.sys .sys-row') as HTMLElement;
        const pEl = row.querySelector('p') as HTMLElement;
        const when = row.querySelector('.when') as HTMLElement;
        const time = row.querySelector('time') as HTMLElement;
        const scroller = document.querySelector('.scroller') as HTMLElement;
        const cardW = parseFloat(
          getComputedStyle(document.documentElement).getPropertyValue('--card-w'),
        );
        const rowRect = row.getBoundingClientRect();
        const scrollerRect = scroller.getBoundingClientRect();
        const whenRect = when.getBoundingClientRect();
        const timeRect = time.getBoundingClientRect();
        const pRect = pEl.getBoundingClientRect();
        return {
          cardW,
          rowMaxWidth: parseFloat(getComputedStyle(row).maxWidth),
          rowScrollW: row.scrollWidth,
          rowClientW: row.clientWidth,
          pScrollW: pEl.scrollWidth,
          pClientW: pEl.clientWidth,
          timeWidth: timeRect.width,
          timeHeight: timeRect.height,
          whenHeight: whenRect.height,
          whenTopDiff: Math.abs(whenRect.top - timeRect.top),
          whenWhiteSpace: getComputedStyle(when).whiteSpace,
          timeWhiteSpace: getComputedStyle(time).whiteSpace,
          pHeight: pRect.height,
          timeRight: timeRect.right,
          rowRight: rowRect.right,
          scrollerRight: scrollerRect.right,
          timeRects: time.getClientRects().length,
        };
      });
      expect(metrics.rowMaxWidth).toBe(metrics.cardW);
      expect(metrics.rowScrollW).toBeLessThanOrEqual(metrics.rowClientW);
      expect(metrics.pScrollW).toBeLessThanOrEqual(metrics.pClientW);
      expect(metrics.timeWidth).toBeGreaterThan(0);
      expect(metrics.timeRight).toBeLessThanOrEqual(metrics.rowRight + 0.5);
      expect(metrics.timeRight).toBeLessThanOrEqual(metrics.scrollerRight + 0.5);
      expect(metrics.whenWhiteSpace).toBe('nowrap');
      expect(metrics.timeWhiteSpace).toBe('nowrap');
      expect(metrics.whenHeight).toBeLessThanOrEqual(metrics.timeHeight * 1.4);
      expect(metrics.whenTopDiff).toBeLessThan(4);
      expect(metrics.timeRects).toBe(1);
      expect(metrics.pHeight).toBeGreaterThan(metrics.timeHeight * 1.5);
    }
  } finally {
    await browserInst.close();
    await server.close();
  }
}, 20_000);

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
