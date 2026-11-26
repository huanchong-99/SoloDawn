// The "In the browser" sentences of step 2 (planning/steps/step-2-3-acceptance.md), each person in
// a page of their own whose homes are the test homes of features/s2_cast.ts; watching runs against
// real nodes.
import { mkdir, writeFile } from 'node:fs/promises';
import { delimiter, join } from 'node:path';
import type { Browser, BrowserContext, Page } from 'playwright';
import { createServer, type ViteDevServer } from 'vite';
import { afterAll, afterEach, beforeAll, describe, expect, inject, test } from 'vitest';
import type {} from '../features/s2_cast';
import { launch } from './config';
import {
  chooseFolder,
  createGroup,
  makeFolder,
  openDirectContext,
  pairPrimaryBrowser,
  poll,
  startDirectSurface,
  type DirectSurface,
} from './direct';
import { startNet, type Net, type Node } from './nodes';

type Id = keyof Window['s2']['ids'];

let server: ViteDevServer;
let engine: Browser;

async function open(as: 'P' | 'Q', hash: string) {
  const page = await engine.newPage({ viewport: { width: 1440, height: 900 } });
  await page.goto(`${server.resolvedUrls!.local[0]}?nomock`);
  await page.addScriptTag({ type: 'module', url: '/features/s2_cast.ts' });
  await page.waitForFunction(() => window.s2);
  await page.evaluate((as) => window.s2.cast(as), as);
  await page.evaluate(
    (hash) => (location.hash = hash.replace(/\{(\w+)\}/g, (_, id: Id) => window.s2.ids[id])),
    hash,
  );
  return page;
}

const ids = (page: Page) => page.evaluate(() => window.s2.ids);
const cards = '.card:has-text("Let Q-personal")';
const sent = (page: Page) => page.evaluate(() => window.s2.sent());
const update = (page: Page, r: string, id: Id, changes: Record<string, unknown>) =>
  page.evaluate(([r, id, changes]) => window.s2.update(r, id, changes), [r, id, changes] as const);
const texts = (page: Page, selector: string) =>
  expect.poll(() => page.locator(selector).allInnerTexts(), { timeout: 5000 });

describe('step 2 in the browser', () => {
  beforeAll(async () => {
    server = await createServer({ logLevel: 'warn', server: { host: '127.0.0.1', port: 0 } });
    await server.listen();
    engine = await launch(inject('browser'));
  });
  afterAll(async () => {
    await engine.close();
    await server.close();
  });
  afterEach(async () => {
    for (const context of engine.contexts()) await context.close();
  });

  test('A9.4 roles, invitation, removal', async () => {
    const p = await open('P', '#/groups/{g}');
    await p.getByRole('button', { name: 'Invite', exact: true }).click();
    await p.getByRole('button', { name: 'Create link' }).click();
    await texts(p, 'pre.cmd').toEqual(['https://p1.example/#join=one-time']);
    await p.getByRole('button', { name: 'Done' }).click();
    const row = p.locator('.people li', { hasText: /^Q/ });
    await row.getByRole('button', { name: 'Make admin' }).click();
    await expect.poll(() => sent(p), { timeout: 5000 }).toHaveLength(2);
    await update(p, 'member', 'q', { role: 'admin' });
    await row.getByRole('button', { name: 'Make member' }).waitFor();
    await row.getByRole('button', { name: 'Remove' }).click();
    await row.getByRole('button', { name: 'Confirm removal' }).click();
    await expect.poll(() => sent(p), { timeout: 5000 }).toHaveLength(3);
    await p.evaluate(() => window.s2.remove('member', 'q'));
    await texts(p, '.people li').toEqual(['A\nAlex · You · Owner']);
    const { g, q: id } = await ids(p);
    expect(await sent(p)).toEqual([
      { t: 'invite', g },
      { t: 'member', g, op: 'set', id, role: 'admin' },
      { t: 'member', g, op: 'remove', id },
    ]);
  });

  test('A9.11 the card, its actions for Q only, each end state', async () => {
    const p = await open('P', '#/groups/{g}');
    const waiting = 'Confirmation\nWaiting for Q\n\nLet Q-personal act on the request from Alex.';
    await texts(p, cards).toEqual([waiting, waiting]);
    expect(await p.locator(cards).getByRole('button').count()).toBe(0);
    const q = await open('Q', '#/groups/{g}');
    await texts(q, 'nav.needbar b').toEqual(['Waiting on you · 2']);
    await q.locator(cards).nth(0).getByRole('button', { name: 'Confirm' }).click();
    await q.locator(cards).nth(1).getByRole('button', { name: 'Deny' }).click();
    const { g, c1, c2, q: by } = await ids(q);
    await expect
      .poll(() => sent(q), { timeout: 5000 })
      .toEqual([
        { t: 'act', g, msg: c1, action: 'confirm' },
        { t: 'act', g, msg: c2, action: 'deny' },
      ]);
    const at = Date.now();
    await update(q, 'msg', 'c1', { state: { status: 'confirmed', by, at } });
    await update(q, 'msg', 'c2', { state: { status: 'denied', by, at } });
    await texts(q, cards).toEqual([
      expect.stringContaining('Confirmed by Q'),
      expect.stringContaining('Denied by Q'),
    ]);
  });

  test('A9.14 Q edits the list and delegates of each AI; P only sees them', async () => {
    const q = await open('Q', '#/groups/{g}');
    for (const name of ['Q-team', 'Q-personal']) {
      const card = q.locator('.member', { hasText: name });
      await card.getByRole('button', { name: 'Edit access' }).click();
      for (const box of await card.getByRole('checkbox', { name: 'Alex' }).all()) await box.check();
      await card.getByRole('button', { name: 'Save access' }).click();
    }
    const { g, qTeam, qPersonal, user } = await ids(q);
    await expect
      .poll(() => sent(q), { timeout: 5000 })
      .toEqual([
        { t: 'member', g, op: 'set', id: qTeam, allow: [user], approvers: [user] },
        { t: 'member', g, op: 'set', id: qPersonal, allow: [user], approvers: [user] },
      ]);
    const p = await open('P', '#/groups/{g}');
    for (const id of ['qTeam', 'qPersonal'] as const)
      await update(p, 'member', id, { allow: [user], approvers: [user] });
    for (const list of ['Allow list', 'Approval delegates'])
      await texts(p, `.member dt:text-is("${list}") + dd`).toEqual(['Alex', 'Alex']);
    expect(await p.getByRole('button', { name: 'Edit access' }).count()).toBe(0);
  });

  test('R-60, R-103 P corrects a limit and has no check commands, Q changes nothing', async () => {
    const p = await open('P', '#/groups/{g}');
    await p.getByRole('button', { name: 'Edit group settings' }).click();
    const dialog = p.getByRole('dialog', { name: 'Edit group settings' });
    const save = dialog.getByRole('button', { name: 'Save' });
    expect(await save.isDisabled()).toBe(true);
    expect(await dialog.getByLabel('Check commands').count()).toBe(0);
    const limits = dialog.locator('details', { hasText: 'Limits' });
    await limits.locator('summary').click();
    const depth = limits.getByLabel('Chain of questions');
    await depth.fill('5');
    await limits.locator('summary').click();
    expect(await depth.isVisible()).toBe(false);
    await save.click();
    // The browser refuses the field, in a fold that opens for it.
    await depth.waitFor();
    expect(await sent(p)).toEqual([]);
    await depth.fill('4');
    await limits.getByLabel('Wake-ups per hour').fill('40');
    await save.click();
    await dialog.waitFor({ state: 'detached' });
    const { g } = await ids(p);
    expect(await sent(p)).toEqual([
      {
        t: 'group',
        g,
        op: 'set',
        limits: {
          rounds: 10,
          wakes_per_hour: 40,
          turn_minutes: 60,
          failures: 3,
          review_rounds: 5,
          ask_depth: 4,
          ask_wait_s: 45,
          approval_wait_s: 600,
          idle_notice_minutes: 30,
        },
      },
    ]);
    const q = await open('Q', '#/groups/{g}');
    await q.locator('.group-details').waitFor();
    expect(await q.getByRole('button', { name: 'Edit group settings' }).count()).toBe(0);
  });

  test('A9.27 the group info shows the two commands to P only', async () => {
    const p = await open('P', '#/groups/{g}');
    const { g, p2 } = await ids(p);
    await p.locator('.move summary').click();
    await texts(p, '.move code').toEqual([
      `solodawn group export ${g} group.sqlite --to ${p2}`,
      'solodawn group import group.sqlite',
    ]);
    const q = await open('Q', '#/groups/{g}');
    await q.locator('.people li', { hasText: 'Alex' }).waitFor();
    expect(await q.locator('.move').count()).toBe(0);
  });
});

// The fake Claude Code replays scripts and has no terminal: this one echoes what is typed there.
const cli = `#!/bin/sh
case " $* " in
  " --version ") echo "2.1.311 (Claude Code)"; exit;;
  *" -p "*) exit 1;;
esac
echo terminal ready
while read -r line; do echo "got $line"; done
`;

const screen = (page: Page) => poll(() => page.locator('.xterm-rows').innerText());
const bar = (page: Page) => poll(() => page.locator('.takeover b').innerText(), 30_000);
const watching = (page: Page) =>
  poll(() => page.locator('section[aria-label="Watching"]').innerText());

// Who watches reaches no computer of the owner before the consent, and the owner has one device,
// whose terminal page is open.
describe('watching a terminal, with real nodes', () => {
  let browser: Browser;
  let net: Net | undefined;
  let surface: DirectSurface | undefined;
  const contexts: BrowserContext[] = [];

  beforeAll(async () => {
    browser = await launch(inject('browser'));
  });
  afterAll(() => browser?.close());
  afterEach(async () => {
    await Promise.allSettled(contexts.splice(0).map((context) => context.close()));
    await surface?.close();
    await net?.stop();
  });

  test(
    'A9.20 A9.21 the owner answers on the terminal page, and the watcher starts by itself',
    { timeout: 180_000 },
    async () => {
      const bin = join(inject('e2e').root, 'watch-bin');
      await mkdir(bin);
      await writeFile(join(bin, 'claude'), cli, { mode: 0o700 });
      const PATH = [bin, inject('e2e').bin, process.env.PATH].join(delimiter);
      const active = (net = await startNet({ nodes: 3, env: () => ({ PATH }) }));
      const direct = (surface = await startDirectSurface(active.rendezvous, inject('browser')));
      const open = async () => (await openDirectContext(browser, contexts, direct)).newPage();

      const h = await open();
      await pairPrimaryBrowser(h, direct, active.nodes[0], 'H1');
      await h.locator('button', { hasText: 'New group' }).first().click();
      await createGroup(h, { name: 'Website', member: 'Mina' });
      const member = async (name: string, node: Node) => {
        await h.getByRole('button', { name: 'Invite', exact: true }).click();
        await h.getByRole('button', { name: 'Create link' }).click();
        const link = await h.locator('pre.cmd').innerText();
        await h.getByRole('button', { name: 'Done' }).click();
        const page = await open();
        await pairPrimaryBrowser(page, direct, node, `${name}'s computer`);
        await page.goto(`${direct.url}${link.slice(link.indexOf('#join='))}`);
        await page.getByLabel('Your name in this group').fill(name);
        await page.locator('form button.btn-primary').click();
        await poll(() => page.locator('.chat-head h2').allTextContents()).toEqual(['Website']);
        return page;
      };
      const p = await member('Alex', active.nodes[1]);
      const q = await member('Quinn', active.nodes[2]);
      await q.locator('.members button', { hasText: 'Add AI' }).click();
      const add = q.locator('dialog.dlg[open]');
      await add.locator('.choice', { hasText: 'Claude Code' }).locator('input').check();
      await chooseFolder(add, await makeFolder(active.dir, 'website'));
      await add.locator('button.btn-primary', { hasText: 'Add AI' }).click();
      await q.locator('.member').getByRole('link', { name: 'Open terminal' }).click();
      await bar(q).toBe('Owner is driving');
      await screen(q).toContain('terminal ready');
      await watching(q).toContain('Nobody is watching.');

      await q.locator('.xterm-helper-textarea').focus();
      await p.locator('.member').getByRole('button', { name: 'Ask to watch' }).click();
      await p.locator('.member').getByRole('link', { name: 'Watch', exact: true }).click();
      await bar(p).toBe('Waiting for Quinn');
      const card = q.locator('.asked .card');
      await poll(() => card.locator('p').innerText()).toBe(
        'Let Alex watch Claude Code’s terminal.',
      );
      expect(await q.locator('.toast').count()).toBe(0);
      // The terminal has kept the keyboard, and gets it back after the answer.
      await q.keyboard.type('before\n');
      await card.getByRole('button', { name: 'Confirm' }).click();
      await poll(() => card.count()).toBe(0);
      await bar(p).toBe('Watching · receive only');
      expect(await p.locator('.keys').count()).toBe(0);
      await watching(q).toMatch(/Alex\s+Stop watching/);
      // The owner's size arrives with the channel: from here the watcher misses nothing.
      const rows = await q.locator('.xterm-rows > div').count();
      await poll(() => p.locator('.xterm-rows > div').count()).toBe(rows);
      await q.keyboard.type('after\n');
      await screen(q).toMatch(/got before[^]*got after/);
      await screen(p).toContain('got after');
      await bar(q).toBe('Owner is driving');

      await q.getByRole('button', { name: 'Stop watching' }).click();
      await watching(q).toContain('Nobody is watching.');
      await bar(p).toBe('Watching ended');
      await bar(q).toBe('Owner is driving');
    },
  );
});
