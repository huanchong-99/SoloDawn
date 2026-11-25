// A9.1 in the real client: P creates an invitation at the real node P1, and Q's browser, which has
// met only Q's own node, opens its `#join=` link and joins. Nothing is reloaded.
import type { Browser, BrowserContext, Page } from 'playwright';
import { afterAll, afterEach, beforeAll, describe, expect, inject, test } from 'vitest';
import { launch } from './config';
import {
  gitIdentityEnv,
  initGitRepo,
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

async function createProject(page: Page, repo: string) {
  await visit(page, '#/groups');
  await page.locator('button', { hasText: 'New group' }).first().click();
  const form = page.locator('dialog.dlg[open]');
  const inputs = form.locator('input.input');
  await inputs.nth(0).fill('Handbook');
  await inputs.nth(1).fill(repo);
  await inputs.nth(2).fill('main');
  await inputs.nth(3).fill('Alex');
  await form.locator('select.input').first().selectOption('none');
  await form.getByRole('button', { name: 'Create group' }).click();
  await poll(() => page.url()).toMatch(/#\/groups\/[^/]+$/);
  return new URL(page.url()).hash.slice('#/groups/'.length);
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
    const active = await startNet({ nodes: 2, env: () => ({ ...gitIdentityEnv }) });
    net = active;
    surface = await startDirectSurface(active.rendezvous, inject('browser'));
    const [p1, q1] = active.nodes;
    const repo = await initGitRepo(active.dir, 'project-repo', '# Handbook\n');
    const p = await (await openDirectContext(browser, contexts, surface)).newPage();
    const q = await (await openDirectContext(browser, contexts, surface)).newPage();
    await pair(p, surface, p1, 'P1');
    await pair(q, surface, q1, 'Q1');

    const g = await createProject(p, repo);
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
