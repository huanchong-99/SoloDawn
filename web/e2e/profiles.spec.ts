import { mkdir, readFile, writeFile } from 'node:fs/promises';
import { delimiter, join } from 'node:path';
import type { Browser, BrowserContext, Page } from 'playwright';
import { createServer, type ViteDevServer } from 'vite';
import { afterAll, afterEach, beforeAll, describe, expect, inject, test } from 'vitest';
import type {} from '../features/s2_cast';
import { launch } from './config';
import {
  chooseFolder,
  createGroup,
  exec,
  makeFolder,
  openDirectContext,
  poll,
  readPairingCode,
  startDirectSurface,
  type DirectSurface,
  finishOnboarding,
} from './direct';
import { leader, startNet, stop, type Net, type Node, type Proc } from './nodes';

async function pairAndCreateGroup(page: Page, surface: DirectSurface, node: Node) {
  await page.goto(`${surface.url}#/onboarding`);
  const code = await readPairingCode(page, surface.signal);
  const paired = await node.run(
    'solodawn',
    'pair',
    code,
    '--name',
    'studio',
    '--rendezvous',
    surface.signal,
  );
  expect(paired.code, paired.stderr).toBe(0);
  await poll(() => page.locator('input[name="acct-claude"][value="personal"]').count()).toBe(1);
  await page.locator('input[name="acct-claude"][value="personal"]').check();
  await poll(() => page.locator('fieldset.acct').first().isDisabled()).toBe(false);
  await page.locator('input[name="acct-codex"][value="api_key"]').check();
  await page.locator('input[name="acct-opencode"][value="api_key"]').check();
  await page.locator('input[name="acct-qwen"][value="api_key"]').check();
  await finishOnboarding(page, true);
  await poll(() => page.locator('.bar a.conn').getAttribute('data-st')).toBe('ok');
  await createGroup(page, { name: 'Profiles', member: 'Owner' });
}

describe('AI profiles with fake CLIs', () => {
  let browser: Browser;
  let net: Net | undefined;
  let surface: DirectSurface | undefined;
  let server: ViteDevServer;
  const restarted: Proc[] = [];
  const contexts: BrowserContext[] = [];
  let bin: string;

  beforeAll(async () => {
    bin = join(inject('e2e').root, 'profiles-bin');
    await mkdir(bin);
    await exec('go', [
      'build',
      '-o',
      `${bin}/`,
      '../internal/host/fakecli/opencode',
      '../internal/host/fakecli/qwen',
    ]);
    browser = await launch(inject('browser'));
    server = await createServer({ logLevel: 'warn', server: { host: '127.0.0.1', port: 0 } });
    await server.listen();
  });
  afterAll(async () => {
    await browser.close();
    await server.close();
  });
  afterEach(async () => {
    await Promise.all(contexts.splice(0).map((context) => context.close()));
    await surface?.close();
    await Promise.all(restarted.splice(0).map(stop));
    await net?.stop();
    surface = undefined;
    net = undefined;
  });

  test('A10.8/A10.13–A10.16: owner edits, repeated CLI joins, catalogs, refresh and restart', async () => {
    const scripts = join(inject('e2e').root, `profiles-${Date.now()}`);
    await mkdir(scripts);
    const claude = join(scripts, 'claude.json');
    const codex = join(scripts, 'codex.json');
    const opencode = join(scripts, 'opencode.json');
    const original = await readFile('../internal/host/fakecli/testdata/claude-models.json', 'utf8');
    await writeFile(claude, original);
    await writeFile(
      codex,
      await readFile('../internal/host/fakecli/testdata/codex-models.json', 'utf8'),
    );
    await writeFile(
      opencode,
      await readFile('../internal/host/fakecli/testdata/opencode-models.json', 'utf8'),
    );
    net = await startNet({
      nodes: 1,
      env: () => ({
        PATH: [bin, inject('e2e').bin, process.env.PATH ?? ''].join(delimiter),
        SOLODAWN_FAKE_CLAUDE_SCRIPT: claude,
        SOLODAWN_FAKE_CODEX_SCRIPT: codex,
        SOLODAWN_FAKE_OPENCODE_SCRIPT: opencode,
      }),
    });
    surface = await startDirectSurface(net.rendezvous, inject('browser'));
    const context = await openDirectContext(browser, contexts, surface);
    const page = await context.newPage();
    await pairAndCreateGroup(page, surface, net.nodes[0]);
    await page.locator('.members').getByRole('button', { name: 'Add AI', exact: true }).click();
    const dialog = page.locator('dialog.dlg[open]');
    await poll(() => dialog.locator('.choice').count()).toBe(5);
    await dialog.locator('.choice', { hasText: 'Codex · studio' }).locator('input').check();
    await poll(() =>
      dialog
        .getByRole('combobox', { name: 'Model', exact: true })
        .locator('option')
        .allTextContents(),
    ).toEqual(["The CLI's default", 'Mini', 'Luna']);
    await dialog.getByRole('combobox', { name: 'Model', exact: true }).selectOption('gpt-6-mini');
    await poll(() => dialog.locator('input[value="light"]').isChecked()).toBe(true);
    await poll(() => dialog.locator('.fast').textContent()).toBe('Fast mode');
    await poll(() =>
      dialog
        .getByRole('combobox', { name: 'Thinking level', exact: true })
        .locator('option')
        .allTextContents(),
    ).toEqual(["The CLI's default", 'medium', 'high']);
    await dialog.getByRole('combobox', { name: 'Model', exact: true }).selectOption('gpt-6-luna');
    await poll(() => dialog.locator('input[value="standard"]').isChecked()).toBe(true);
    await dialog.locator('.choice', { hasText: 'OpenCode · studio' }).locator('input').check();
    await dialog
      .getByRole('combobox', { name: 'Model', exact: true })
      .selectOption('anthropic/claude-opus-5');
    await poll(() =>
      dialog
        .getByRole('combobox', { name: 'Thinking level', exact: true })
        .locator('option')
        .allTextContents(),
    ).toEqual(["The CLI's default", 'low', 'high']);
    await poll(() => dialog.locator('.fast').count()).toBe(0);
    await dialog.locator('.choice', { hasText: 'Qwen Code · studio' }).locator('input').check();
    await poll(() => dialog.getByRole('textbox', { name: 'Model', exact: true }).count()).toBe(1);
    await poll(() =>
      dialog.getByRole('textbox', { name: 'Thinking level', exact: true }).count(),
    ).toBe(1);
    await dialog.locator('.choice', { hasText: 'Claude Code · studio' }).locator('input').check();
    await poll(() =>
      dialog
        .getByRole('combobox', { name: 'Model', exact: true })
        .locator('option')
        .allTextContents(),
    ).toEqual(["The CLI's default", 'Default', 'Haiku']);
    await dialog.getByRole('combobox', { name: 'Model', exact: true }).selectOption('default');
    await poll(() =>
      dialog
        .getByRole('combobox', { name: 'Thinking level', exact: true })
        .locator('option')
        .allTextContents(),
    ).toEqual(["The CLI's default", 'low', 'high']);
    await poll(() => dialog.locator('input[type="radio"][value="strong"]').isChecked()).toBe(true);
    await dialog
      .getByRole('combobox', { name: 'Thinking level', exact: true })
      .selectOption('high');
    await dialog.getByRole('textbox', { name: 'Name in the group', exact: true }).fill('Opus');
    await dialog.getByPlaceholder('What this AI is good at, in one line').fill('Go backend');
    await chooseFolder(dialog, await makeFolder(net.dir, 'opus'));
    await dialog.getByRole('button', { name: 'Add AI', exact: true }).click();
    await poll(() => dialog.count()).toBe(0);
    const member = page
      .locator('.member')
      .filter({ has: page.getByRole('heading', { name: 'Opus', exact: true }) });
    await poll(() => member.locator('.skills').textContent()).toBe('Go backend');
    await poll(() => member.locator('.tier-row').innerText()).toBe('More');
    await member.getByRole('button', { name: 'Edit profile', exact: true }).click();
    const editor = member.getByRole('form', { name: 'Edit profile', exact: true });
    await editor.locator('input[value="light"]').check();
    await editor.getByPlaceholder('What this AI is good at, in one line').fill('x'.repeat(201));
    await poll(() => editor.getByRole('button', { name: 'Save', exact: true }).isDisabled()).toBe(
      true,
    );
    await editor.getByPlaceholder('What this AI is good at, in one line').fill('docs');
    await editor.getByRole('button', { name: 'Save', exact: true }).click();
    await poll(() => member.locator('.skills').textContent()).toBe('docs');

    await page.locator('.members').getByRole('button', { name: 'Add AI', exact: true }).click();
    await dialog.locator('.choice', { hasText: 'Claude Code · studio' }).locator('input').check();
    await dialog.getByRole('combobox', { name: 'Model', exact: true }).selectOption('haiku');
    await poll(() => dialog.locator('input[value="light"]').isChecked()).toBe(true);
    await dialog.getByRole('textbox', { name: 'Name in the group', exact: true }).fill('Haiku');
    await chooseFolder(dialog, await makeFolder(net.dir, 'haiku'));
    await dialog.getByRole('button', { name: 'Add AI', exact: true }).click();
    await poll(() => dialog.count()).toBe(0);
    await poll(() => page.locator('.member h3').allTextContents()).toEqual(['Haiku', 'Opus']);
    const haiku = page
      .locator('.member')
      .filter({ has: page.getByRole('heading', { name: 'Haiku', exact: true }) });
    await poll(() => haiku.locator('.tier').getAttribute('data-tier')).toBe('light');

    const gained = original.replace(
      '"supportsEffort":false}',
      '"supportsEffort":false},{"value":"gemini-3-pro","displayName":"Pro"}',
    );
    await writeFile(claude, gained);
    await member.getByRole('button', { name: 'Refresh the models', exact: true }).click();
    await member.getByRole('button', { name: 'Edit profile', exact: true }).click();
    await poll(() =>
      member
        .getByRole('combobox', { name: 'Model', exact: true })
        .locator('option')
        .allTextContents(),
    ).toEqual(["The CLI's default", 'Default', 'Haiku', 'Pro']);
    await member.getByRole('button', { name: 'Cancel', exact: true }).click();
    await poll(() => member.locator('.suggest span').textContent()).toBe('More');

    await writeFile(codex, '{"session":"models","exit":1}');
    await writeFile(claude, original.replace('"claude-opus-5"', '"gemini-3-pro"'));
    await member.getByRole('button', { name: 'Refresh the models', exact: true }).click();
    await poll(() => member.locator('.suggest span').textContent()).toBe('Normal');
    await poll(() => member.locator('.tier-row > .tier').getAttribute('data-tier')).toBe('light');
    await poll(() => member.locator('.skills').textContent()).toBe('docs');

    await page.locator('.members').getByRole('button', { name: 'Add AI', exact: true }).click();
    await dialog.locator('.choice', { hasText: 'Codex · studio' }).locator('input').check();
    await poll(() => dialog.getByRole('textbox', { name: 'Model', exact: true }).count()).toBe(1);
    await poll(() =>
      dialog.getByRole('textbox', { name: 'Thinking level', exact: true }).count(),
    ).toBe(1);
    await dialog.getByRole('button', { name: 'Cancel', exact: true }).click();

    await writeFile(claude, '{"session":"models","exit":1}');
    await member.getByRole('button', { name: 'Refresh the models', exact: true }).click();
    await poll(() => member.locator('.what .badge').textContent()).toBe('Not listed');
    await poll(() => member.locator('.suggest').count()).toBe(0);
    await poll(() => member.locator('.tier-row').innerText()).toBe('Less');
    await poll(() => member.locator('.skills').textContent()).toBe('docs');

    const [node] = net.nodes;
    process.kill(-node.pid, 'SIGTERM');
    await poll(() => {
      try {
        process.kill(node.pid, 0);
        return false;
      } catch {
        return true;
      }
    }).toBe(true);
    restarted.push(
      leader(
        'restarted profiles node',
        [join(inject('e2e').bin, 'solodawn'), 'node'],
        node.env,
        node.home,
        join(inject('e2e').root, 'pids'),
      ),
    );
    await poll(() => page.locator('.bar a.conn').getAttribute('data-st'), 30_000).toBe('ok');
    await page.reload();
    await poll(() => member.locator('.what .badge').textContent()).toBe('Not listed');
    await poll(() => member.locator('.suggest').count()).toBe(0);
    await poll(() => member.locator('.tier-row').innerText()).toBe('Less');
    await poll(() => member.locator('.skills').textContent()).toBe('docs');
  });

  async function cast(as: 'P' | 'Q') {
    const page = await browser.newPage({ viewport: { width: 1440, height: 900 } });
    contexts.push(page.context());
    await page.goto(`${server.resolvedUrls!.local[0]}?nomock`);
    await page.addScriptTag({ type: 'module', url: '/features/s2_cast.ts' });
    await page.waitForFunction(() => window.s2);
    await page.evaluate(async (as) => {
      await window.s2.cast(as);
      window.s2.update('member', 'qPersonal', {
        tier: 'strong',
        skills: 'Go backend',
        state: 'waiting',
      });
      location.hash = `#/groups/${window.s2.ids.g}`;
    }, as);
    return page;
  }

  test('A10.8: only the AI owner sees its profile editor', async () => {
    const other = await cast('P');
    const otherCard = other
      .locator('.member')
      .filter({ has: other.getByRole('heading', { name: 'Q-personal', exact: false }) });
    await poll(() => otherCard.locator('.skills').textContent()).toBe('Go backend');
    await poll(() =>
      otherCard.getByRole('button', { name: 'Edit profile', exact: true }).count(),
    ).toBe(0);
    await poll(() =>
      otherCard.getByRole('button', { name: 'Refresh the models', exact: true }).count(),
    ).toBe(0);
    const owner = await cast('Q');
    const ownCard = owner
      .locator('.member')
      .filter({ has: owner.getByRole('heading', { name: 'Q-personal', exact: true }) });
    await poll(() => ownCard.locator('.skills').textContent()).toBe('Go backend');
    await poll(() =>
      ownCard.getByRole('button', { name: 'Edit profile', exact: true }).count(),
    ).toBe(1);
  });

  test('A10.10–A10.12: the task badge follows authoritative review escalation rows', async () => {
    const page = await cast('P');
    const task = JSON.parse(
      await readFile('../internal/proto/testdata/row-task-stuck.json', 'utf8'),
    );
    const notice = JSON.parse(
      await readFile('../internal/proto/testdata/row-msg-task.json', 'utf8'),
    );
    await page.evaluate(
      ({ task, notice }) => {
        const { g, qPersonal, qTeam } = window.s2.ids;
        window.s2.update('task', 'g', {
          ...task,
          id: g,
          g,
          assignee: qPersonal,
          reviewer: qTeam,
          stuck: { kind: 'review', n: 1, ask: false },
        });
        window.s2.update('msg', 'c1', {
          ...notice,
          id: window.s2.ids.c1,
          g,
          thread: null,
          main: true,
          body: { task: g },
        });
      },
      { task, notice },
    );
    const badge = page.getByText('Must ask a stronger AI', { exact: true });
    await poll(() => page.locator('.card .steps i').count()).toBe(3);
    await poll(() => badge.count()).toBe(0);
    for (const stuck of [
      { kind: 'review', n: 2, ask: true },
      { kind: 'review', n: 2, ask: false },
      { kind: 'review', n: 3, ask: true },
    ]) {
      await page.evaluate((stuck) => window.s2.update('task', 'g', { stuck }), stuck);
      await poll(() => badge.count()).toBe(stuck.ask ? 1 : 0);
    }
  });
});
