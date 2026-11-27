// A9.1 in the real client: P creates an invitation at the real node P1, and Q's browser, which has
// met only Q's own node, opens its `#join=` link and joins. Nothing is reloaded.
// A9.47, A9.48, A9.50, A9.52 and A9.54 in the real client: the same with an invitation code, which people type
// on the join page of their own client; the rendezvous, the nodes and the browsers are real.
import type { Browser, BrowserContext, Locator, Page } from 'playwright';
import { afterAll, afterEach, beforeAll, describe, expect, inject, test } from 'vitest';
import { launch } from './config';
import {
  createGroup,
  openDirectContext,
  poll,
  readNodeDeviceId,
  readPairingCode,
  sendChatMessage,
  startDirectSurface,
  type DirectSurface,
} from './direct';
import { startNet, type Net, type Node } from './nodes';

async function pair(page: Page, surface: DirectSurface, node: Node, name: string) {
  await page.goto(`${surface.url}#/onboarding`);
  const code = await readPairingCode(page, surface.signal);
  const args = ['pair', code, '--name', name, '--rendezvous', surface.signal];
  const paired = await node.run('solodawn', ...args);
  expect(paired.code, paired.stderr).toBe(0);
  await poll(() => page.locator('.link-steps').textContent()).toContain(name);
}

/** Changes the address inside the open client, as following a link there does. */
async function visit(page: Page, hash: string) {
  await page.evaluate((to) => {
    location.hash = to;
  }, hash);
}

async function createHandbook(page: Page) {
  await visit(page, '#/groups');
  await page.locator('button', { hasText: 'New group' }).first().click();
  return createGroup(page, { name: 'Handbook', member: 'Alex' });
}

async function invite(page: Page) {
  await page.getByRole('button', { name: 'Invite', exact: true }).click();
  await page.getByRole('button', { name: 'Create link' }).click();
  const link = page.locator('dialog.dlg[open] pre.cmd');
  await poll(() => link.count()).toBe(1);
  const text = ((await link.textContent()) ?? '').trim();
  await page.getByRole('button', { name: 'Done' }).click();
  return new URL(text);
}

const people = (page: Page) => page.locator('.people li').allInnerTexts();
const lines = (page: Page) => page.locator('.line .msg .text').allTextContents();

describe('A9.1 joining by invitation', () => {
  let browser: Browser;
  let net: Net | undefined;
  let surface: DirectSurface | undefined;
  const contexts: BrowserContext[] = [];

  beforeAll(async () => {
    browser = await launch(inject('browser'));
  });
  afterAll(() => browser?.close());
  afterEach(async () => {
    await Promise.allSettled(contexts.splice(0).map((ctx) => ctx.close()));
    await surface?.close();
    surface = undefined;
  });
  afterEach(() => net?.stop());

  test('a person joins with the #join link, and both browsers show the person and the history', async () => {
    const active = await startNet({ nodes: 2 });
    net = active;
    surface = await startDirectSurface(active.rendezvous, inject('browser'));
    const [p1, q1] = active.nodes;
    const p = await (await openDirectContext(browser, contexts, surface)).newPage();
    const q = await (await openDirectContext(browser, contexts, surface)).newPage();
    await pair(p, surface, p1, 'P1');
    await pair(q, surface, q1, 'Q1');

    const g = await createHandbook(p);
    await sendChatMessage(p, 'Welcome before Q');
    const link = await invite(p);
    const fields = new URLSearchParams(link.hash.slice(1));
    expect([...fields.keys()]).toEqual(['join', 'g', 'home']);
    expect(fields.get('g')).toBe(g);
    expect(fields.get('home')).toBe(await readNodeDeviceId(p1));
    expect(fields.get('join')).not.toBe('');

    expect(link.href.startsWith(surface.url)).toBe(true);

    // A reload would lose these marks.
    for (const page of [p, q]) await page.evaluate(() => void (document.body.dataset.mark = 'set'));
    await visit(q, link.hash);
    await q.getByLabel('Your name in this group').fill('Quinn');
    await q.getByRole('button', { name: 'Join group' }).click();

    await poll(() => new URL(q.url()).hash).toBe(`#/groups/${g}`);
    await poll(() => lines(q)).toEqual(['Welcome before Q']);
    await poll(() => people(q)).toEqual([
      expect.stringMatching(/Alex · Owner/),
      expect.stringMatching(/Quinn · You/),
    ]);
    const row = p.locator('.people li', { hasText: 'Quinn' });
    await row.getByRole('button', { name: 'Make admin' }).waitFor();

    await sendChatMessage(q, 'Thanks for the invitation');
    await poll(() => lines(p)).toEqual(['Welcome before Q', 'Thanks for the invitation']);
    for (const page of [p, q]) {
      expect(await page.evaluate(() => document.body.dataset.mark)).toBe('set');
    }
  });
});

/** The `join` frames a page sends to the rendezvous from now on: a code that is no code sends none. */
function watchJoins(page: Page) {
  const sent: string[] = [];
  page.on('websocket', (socket) =>
    socket.on('framesent', ({ payload }) => {
      if (String(payload).includes('"t":"join"')) sent.push(String(payload));
    }),
  );
  return sent;
}

const codeShape = /^[0-9A-Z]{4}(-[0-9A-Z]{4}){3}$/;

/** The code as a person pastes it: lower case, in groups with spaces, with no hyphens. */
const pasted = (code: string) =>
  code
    .replaceAll('-', '')
    .toLowerCase()
    .replace(/(.{4})(?=.)/g, '$1 ');

async function readCode(scope: Locator) {
  const code = scope.locator('output.code');
  await poll(() => code.count()).toBe(1);
  const text = ((await code.textContent()) ?? '').trim();
  expect(text).toMatch(codeShape);
  return text;
}

/** Creates the group `Handbook` with "Also make an invitation code" ticked; returns the group and
 *  the code, which is shown before the dialog closes. */
async function createHandbookWithCode(page: Page) {
  await visit(page, '#/groups');
  await page.locator('button', { hasText: 'New group' }).first().click();
  const dialog = page.getByRole('dialog', { name: 'New group' });
  await dialog.getByLabel('Group name').fill('Handbook');
  await dialog.getByLabel('Your name in the group').fill('Alex');
  await dialog.getByLabel('Also make an invitation code').check();
  await dialog.getByRole('button', { name: 'Create group' }).click();
  const code = await readCode(dialog);
  await dialog.getByRole('button', { name: 'Open the group' }).click();
  await poll(() => page.url()).toMatch(/#\/groups\/[^/]+$/);
  return { g: new URL(page.url()).hash.slice('#/groups/'.length), code };
}

async function openInvitation(page: Page) {
  await page.getByRole('button', { name: 'Invite', exact: true }).click();
  const dialog = page.locator('dialog.dlg[open]');
  await dialog.waitFor();
  return dialog;
}

/** The join page with the code and the name filled in; submitting is left to the caller. */
async function fillJoin(page: Page, code: string, name: string) {
  await visit(page, '#/groups');
  await page.getByRole('link', { name: 'Join a group' }).click();
  await page.getByLabel('Invitation code', { exact: true }).fill(code);
  await page.getByLabel('Your name in this group').fill(name);
}

async function typeCode(page: Page, code: string, name: string) {
  await fillJoin(page, code, name);
  await page.getByRole('button', { name: 'Join group' }).click();
}

describe('A9.47 to A9.54 joining by invitation code', () => {
  let browser: Browser;
  let net: Net | undefined;
  let surface: DirectSurface | undefined;
  const contexts: BrowserContext[] = [];

  beforeAll(async () => {
    browser = await launch(inject('browser'));
  });
  afterAll(() => browser?.close());
  afterEach(async () => {
    await Promise.allSettled(contexts.splice(0).map((ctx) => ctx.close()));
    await surface?.close();
    surface = undefined;
  });
  afterEach(() => net?.stop());

  test('people type one code in their own clients, it is replaced and turned off, and removing a person ends it', async () => {
    const active = await startNet({ nodes: 4 });
    net = active;
    surface = await startDirectSurface(active.rendezvous, inject('browser'));
    const [p, q, r, s] = await Promise.all(
      active.nodes.map(async () =>
        (await openDirectContext(browser, contexts, surface!)).newPage(),
      ),
    );
    const joins = watchJoins(q);
    await Promise.all(
      active.nodes.map((node, i) => pair([p, q, r, s][i], surface!, node, `Node ${i}`)),
    );

    // A9.54: the code is shown before the dialog closes.
    const { g, code } = await createHandbookWithCode(p);
    await sendChatMessage(p, 'Welcome before the code');
    for (const page of [p, q, r, s])
      await page.evaluate(() => void (document.body.dataset.mark = 'set'));

    // A9.47: a text with the letter O is refused on the page and nothing is sent.
    await fillJoin(q, `O${code.slice(1)}`, 'Quinn');
    joins.length = 0;
    await q.getByRole('button', { name: 'Join group' }).click();
    await q.getByText('This is not an invitation code').waitFor();
    expect(joins).toEqual([]);

    // Q and R paste it in lower case, with spaces and no hyphens, both at once.
    await fillJoin(q, pasted(code), 'Quinn');
    await fillJoin(r, pasted(code), 'Rae');
    await Promise.all(
      [q, r].map((page) => page.getByRole('button', { name: 'Join group' }).click()),
    );
    for (const page of [q, r]) {
      await poll(() => new URL(page.url()).hash).toBe(`#/groups/${g}`);
      await poll(() => lines(page)).toEqual(['Welcome before the code']);
    }
    await poll(() => people(p)).toEqual(
      expect.arrayContaining([
        expect.stringMatching(/Alex · (You · )?Owner|Owner/),
        expect.stringMatching(/Quinn/),
        expect.stringMatching(/Rae/),
      ]),
    );
    await p.getByText(/^Quinn joined, invited by Alex\./).waitFor();
    await p.getByText(/^Rae joined, invited by Alex\./).waitFor();

    // A9.52: the one-time link works while the code is open, and a member who types the code of
    // their group is taken to it with no second row.
    let dialog = await openInvitation(p);
    expect(await readCode(dialog)).toBe(code);
    await dialog.getByText(/Valid until/).waitFor();
    await dialog.getByRole('button', { name: 'Create link' }).click();
    const link = dialog.locator('pre.cmd');
    await poll(() => link.count()).toBe(1);
    const url = new URL(((await link.textContent()) ?? '').trim());
    await visit(s, url.hash);
    await s.getByLabel('Your name in this group').fill('Sam');
    await s.getByRole('button', { name: 'Join group' }).click();
    await poll(() => new URL(s.url()).hash).toBe(`#/groups/${g}`);
    await typeCode(q, code, 'Quinn');
    await poll(() => new URL(q.url()).hash).toBe(`#/groups/${g}`);
    await poll(() => people(p)).toHaveLength(4);

    // A9.48: a new code replaces the first, which then gets no answer.
    await dialog.getByRole('button', { name: 'New code' }).click();
    await dialog.getByText('The current code stops working at once.').waitFor();
    await dialog.getByRole('button', { name: 'New code' }).click();
    await poll(() => readCode(dialog)).not.toBe(code);
    const second = await readCode(dialog);
    await typeCode(r, code, 'Rae');
    await r.getByText('Looking for the group').waitFor();
    expect(new URL(r.url()).hash).toBe('#/join');
    await visit(r, `#/groups/${g}`);

    // Turn off shows none, and a code made again is ended by removing a person.
    await dialog.getByRole('button', { name: 'Turn off' }).click();
    await dialog.getByRole('button', { name: 'Create code' }).waitFor();
    expect(await dialog.locator('output.code').count()).toBe(0);
    await dialog.getByRole('button', { name: 'Create code' }).click();
    const third = await readCode(dialog);
    expect(third).not.toBe(second);
    await dialog.getByRole('button', { name: 'Done' }).click();
    const row = p.locator('.people li', { hasText: 'Quinn' });
    await row.getByRole('button', { name: 'Remove' }).click();
    await row.getByText('If the group has an invitation code open, it is ended as well.').waitFor();
    await row.getByRole('button', { name: 'Confirm removal' }).click();
    await poll(() => people(p)).toHaveLength(3);
    dialog = await openInvitation(p);
    // The dialog asks `code show` as it opens and offers a disabled Create code until it is
    // answered: count the code only once that button is enabled.
    await poll(() => dialog.getByRole('button', { name: 'Create code' }).isEnabled()).toBe(true);
    expect(await dialog.locator('output.code').count()).toBe(0);
    await typeCode(r, third, 'Rae');
    await r.getByText('Looking for the group').waitFor();
    expect(new URL(r.url()).hash).toBe('#/join');

    // Nothing was reloaded.
    for (const page of [p, q, r, s]) {
      expect(await page.evaluate(() => document.body.dataset.mark)).toBe('set');
    }
  }, 180_000);
});
