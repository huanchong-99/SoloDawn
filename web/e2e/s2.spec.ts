// The "In the browser" sentences of step 2 (planning/steps/step-2-3-acceptance.md), each person in
// a page of their own whose homes are the test homes of features/s2_cast.ts.
import type { Browser, Page } from 'playwright';
import { createServer, type ViteDevServer } from 'vite';
import { afterAll, afterEach, beforeAll, describe, expect, inject, test } from 'vitest';
import type {} from '../features/s2_cast';
import { launch } from './config';

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

  test("A9.21 P's terminal takes no input, and Q sees that P is watching", async () => {
    const p = await open('P', '#/terminal/{g}/{qTeam}');
    await p
      .getByText("You can watch, but cannot type or resize the owner's terminal.")
      .first()
      .waitFor();
    expect(await p.locator('.keys').count()).toBe(0);
    const q = await open('Q', '#/terminal/{g}/{qTeam}');
    await texts(q, 'section[aria-label="Watching"] .row').toEqual(['A\nAlex\nStop watching']);
  });

  test('A9.23 the team group has no task list, no brief and no AI cards', async () => {
    const p = await open('P', '#/groups/{team}');
    await texts(p, '[aria-label="Bulletin"]').toEqual([
      'Claude Code\nWebsite\n\nOn a computer belonging to Alex\n\nThe website is ready.',
    ]);
    expect(await p.getByRole('combobox', { name: 'Message' }).getAttribute('placeholder')).toBe(
      'Message the team',
    );
    expect(await p.locator('.member').count()).toBe(0);
    expect(await p.getByRole('button', { name: 'Add AI' }).count()).toBe(0);
  });

  test('A9.23 P creates a team group in the dialog, and a bulletin shows there', async () => {
    const p = await open('P', '#/groups');
    await p.getByRole('button', { name: 'New group' }).click();
    await p.getByRole('radio', { name: 'Team group' }).check();
    const dialog = p.getByRole('dialog', { name: 'New group' });
    expect(await dialog.getByLabel('Git repository').count()).toBe(0);
    await dialog.getByLabel('Group name').fill('Studio');
    await dialog.getByLabel('Your name in the group').fill('Alex');
    await dialog.getByLabel('Home computer').selectOption('P1');
    await dialog.getByRole('button', { name: 'Create group' }).click();
    await texts(p, '.chat-head h2').toEqual(['Studio']);
    const g = new URL(p.url()).hash.split('/')[2];
    expect(await sent(p)).toEqual([
      {
        t: 'group',
        g,
        op: 'create',
        name: 'Studio',
        kind: 'team',
        base: '',
        forge: 'none',
        checks: [],
        member_name: 'Alex',
      },
    ]);
    // P2 listed neither team group: it was told the first as it connected, then the new one.
    const { p2, team } = await ids(p);
    const { host } = new URL(p.url());
    const notice = (g: string, name: string) => ({
      t: 'home.add',
      to: p2,
      g,
      home: expect.any(String),
      name,
      addrs: [`ws://${host}/rv`],
    });
    await expect
      .poll(() => p.evaluate(() => window.s2.told()), { timeout: 5000 })
      .toEqual([notice(team, 'Team'), notice(g, 'Studio')]);
    await texts(p, '#chat-side h2, #chat-side summary b').toEqual([
      'PEOPLE · 1',
      'GROUP DETAILS',
      'MOVE GROUP',
    ]);
    await p.evaluate((g) => window.s2.bulletin(g), g);
    await texts(p, '[aria-label="Bulletin"] p:last-child').toEqual(['The website is ready.']);
    await p.getByRole('button', { name: 'Invite', exact: true }).waitFor();
    expect(await p.getByRole('button', { name: 'Add AI' }).count()).toBe(0);
    await p.getByRole('button', { name: 'Edit group settings' }).click();
    const settings = p.getByRole('dialog', { name: 'Edit group settings' });
    expect(await settings.locator('input, textarea, select, summary').count()).toBe(1);
    await settings.getByLabel('Group name').fill('Studio notices');
    await settings.getByRole('button', { name: 'Save' }).click();
    await settings.waitFor({ state: 'detached' });
    expect((await sent(p)).at(-1)).toEqual({ t: 'group', g, op: 'set', name: 'Studio notices' });
  });

  test('A9.24 the dialog that joins an AI shows the check commands and each template', async () => {
    const p = await open('P', '#/groups/{g}');
    await p.getByRole('button', { name: 'Add AI' }).click();
    await p.getByRole('dialog', { name: 'Add an AI' }).locator('summary').click();
    await texts(p, 'dialog [aria-label="Group commands"] code').toEqual([
      'https://git.example/team/website.git',
      'main',
      'pnpm check',
      'pnpm test',
      ...['git', 'push', 'origin', '{branch}'],
      ...['forge', 'open', '--title={title}', '--body={summary}', '{base}', '{commit}'],
      ...['forge', 'checks', '{request}', '{commit}'],
      ...['forge', 'merge', '{request}', '{commit}'],
    ]);
  });

  test('R-60, R-103 P corrects a check command and a limit, Q changes nothing', async () => {
    const p = await open('P', '#/groups/{g}');
    await p.getByRole('button', { name: 'Edit group settings' }).click();
    const dialog = p.getByRole('dialog', { name: 'Edit group settings' });
    const save = dialog.getByRole('button', { name: 'Save' });
    expect(await save.isDisabled()).toBe(true);
    await dialog.getByLabel('Check commands').fill('pnpm check\npnpm vitest run');
    const limits = dialog.locator('details', { hasText: 'Limits' });
    await limits.locator('summary').click();
    const depth = limits.getByLabel('Questions within questions');
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
        checks: ['pnpm check', 'pnpm vitest run'],
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
