// End-to-end regression for Unit V1: turn failures and OAuth expiry surface in chat,
// the AI member card, and the Computers page instead of disappearing silently.
import { mkdir, writeFile } from 'node:fs/promises';
import { join } from 'node:path';
import type { Browser, BrowserContext, Page } from 'playwright';
import { afterAll, afterEach, beforeAll, describe, expect, inject, test } from 'vitest';
import { launch } from './config';
import {
  chooseFolder,
  claudeInitSteps,
  codexInitSteps,
  createGroup,
  finishOnboarding,
  makeFolder,
  openDirectContext,
  poll,
  readPairingCode,
  sendChatMessage,
  startDirectSurface,
  type DirectSurface,
} from './direct';
import { startNet, type Net, type Node } from './nodes';

function claudeFailureThenOAuthScript(failedResult: string, oauthResult: string) {
  return {
    steps: [
      ...claudeInitSteps(),
      {
        dir: 'stdout',
        msg: {
          type: 'result',
          subtype: 'error_during_execution',
          is_error: true,
          api_error_status: 400,
          result: failedResult,
          session_id: '$session',
        },
      },
      { dir: 'stdin', msg: { type: 'user', message: { role: 'user' } } },
      {
        dir: 'stdout',
        msg: {
          type: 'result',
          subtype: 'success',
          is_error: true,
          result: oauthResult,
          session_id: '$session',
        },
      },
      { dir: 'eof' },
    ],
  };
}

function codexIdleScript() {
  return {
    session: '01a1f63f-701c-78c1-bbf3-f1ac61c56b59',
    steps: [...codexInitSteps('turn-codex-1'), { dir: 'eof' }],
  };
}

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
  await finishOnboarding(page, true);
  await poll(() => page.locator('.bar a.conn').getAttribute('data-st')).toBe('ok');
  await createGroup(page, { name: 'Website', member: 'Alex' });
}

async function saveShot(page: Page, name: string) {
  const dir = process.env.V1_SHOT_DIR;
  if (!dir) return;
  await mkdir(dir, { recursive: true });
  await page.emulateMedia({ reducedMotion: 'reduce' });
  await page.evaluate(
    () => new Promise<void>((r) => requestAnimationFrame(() => requestAnimationFrame(() => r()))),
  );
  await page.screenshot({ path: join(dir, name), fullPage: true, animations: 'disabled' });
}

describe('V1: turn failures and OAuth expiration surface visibly', () => {
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

  test(
    'surfaces failed turn crashes and OAuth expiration in chat, member panel, and Computers',
    { timeout: 120_000 },
    async () => {
      const scriptDir = join(inject('e2e').root, `v1-scripts-${Date.now()}`);
      await mkdir(scriptDir, { recursive: true });
      const claudePath = join(scriptDir, 'claude.json');
      const codexPath = join(scriptDir, 'codex.json');
      const oauthError =
        'Failed to authenticate. API Error: 401 {"error":{"message":"OAuth token has expired"}}';
      await writeFile(
        claudePath,
        JSON.stringify(
          claudeFailureThenOAuthScript('API error 400: overloaded\nretry later', oauthError),
        ),
      );
      await writeFile(codexPath, JSON.stringify(codexIdleScript()));

      const active = await startNet({
        nodes: 1,
        env: () => ({
          SOLODAWN_FAKE_CLAUDE_SCRIPT: claudePath,
          SOLODAWN_FAKE_CODEX_SCRIPT: codexPath,
        }),
      });
      net = active;
      const direct = await startDirectSurface(active.rendezvous, inject('browser'));
      surface = direct;
      const [node] = active.nodes;

      const folder = await makeFolder(active.dir, 'website');
      const context = await openDirectContext(browser, contexts, direct);
      const page = await context.newPage();
      await page.setViewportSize({ width: 1280, height: 840 });

      await pairAndCreateGroup(page, direct, node);

      const members = page.locator('#chat-side .members');
      await members.locator('button', { hasText: 'Add AI' }).click();
      const addDlg = page.locator('dialog.dlg[open]');
      await poll(() => addDlg.locator('.choice').count()).toBe(3);
      await addDlg.locator('.choice', { hasText: 'Claude Code · studio' }).locator('input').check();
      await chooseFolder(addDlg, folder);
      await addDlg.locator('button.btn-primary', { hasText: 'Add AI' }).click();
      await poll(() => addDlg.count()).toBe(0);

      const claudeCard = members.locator('.member', { hasText: 'Claude Code' });
      await poll(() => claudeCard.locator('header .st').textContent()).toContain('Sleeping');

      // 1. Non-auth failed result surfaces turn_interrupted crash + detail and keeps the open CLI in Waiting.
      await sendChatMessage(page, '@Claude Code run status check');
      await poll(() => page.locator('.line').textContent()).toContain(
        "Claude Code's turn crashed. · API error 400: overloaded",
      );
      await poll(() => claudeCard.locator('header .st').textContent()).toBe('Waiting');
      await saveShot(page, 'v1-failed-turn-chat.png');

      // 2. OAuth expiry on the follow-up turn surfaces login_invalid, member card re-auth prompt, and Computers re-auth alert.
      await sendChatMessage(page, '@Claude Code try again');

      await poll(() => page.locator('.line').textContent()).toContain(
        "Claude Code's sign-in is no longer valid.",
      );
      await poll(() => page.locator('.line').textContent()).toContain('Failed to authenticate.');
      await poll(() => claudeCard.locator('header .st').textContent()).toBe(
        'Limited · sign-in expired',
      );
      const memberAlert = claudeCard.locator('.note[data-tone="danger"]');
      await poll(() => memberAlert.textContent()).toContain('Claude Code needs to sign in again.');
      await poll(() => memberAlert.textContent()).toContain('Run it directly on studio');
      await saveShot(page, 'v1-reauth-member.png');

      // Navigate via the alert link to #/nodes and verify the live host.info update shows Sign-in expired.
      await memberAlert.getByRole('link', { name: 'Computers' }).click();
      const claudeRow = page.locator('.card.node .ai-row', { hasText: 'Claude Code' });
      await poll(() => claudeRow.locator('.st').first().textContent()).toContain('Sign-in expired');
      const nodeAlert = claudeRow.locator('.note[data-tone="danger"]');
      await poll(() => nodeAlert.textContent()).toContain('Claude Code needs to sign in again.');
      await poll(() => nodeAlert.textContent()).toContain('Run it directly on studio');
      await saveShot(page, 'v1-reauth-nodes.png');
    },
  );
});
