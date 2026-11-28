import { mkdir, writeFile } from 'node:fs/promises';
import { join } from 'node:path';
import type { Browser, BrowserContext } from 'playwright';
import { afterAll, afterEach, beforeAll, describe, expect, inject, test } from 'vitest';
import { launch } from './config';
import {
  chooseFolder,
  claudeInitSteps,
  finishOnboarding,
  makeFolder,
  openDirectContext,
  poll,
  readPairingCode,
  sendChatMessage,
  startDirectSurface,
  type DirectSurface,
} from './direct';
import { startNet, type Net } from './nodes';

function secretaryMainLineReplyScript() {
  return {
    steps: [
      ...claudeInitSteps(),
      {
        dir: 'mcp<-cli',
        msg: {
          jsonrpc: '2.0',
          id: 1,
          method: 'tools/call',
          params: {
            name: 'post',
            arguments: {
              kind: 'text',
              text: 'I need at least 2 working AIs besides the secretary before proposing a brief, so assignee and reviewer are different AIs. Click Add AI to add another CLI or add the same CLI twice with a new name and folder.',
            },
          },
        },
      },
      { dir: 'mcp->cli', msg: { jsonrpc: '2.0', id: 1, result: { isError: false } } },
      {
        dir: 'stdout',
        msg: {
          type: 'result',
          subtype: 'success',
          is_error: false,
          result: 'told-person-in-main-line',
          session_id: '$session',
        },
      },
      { dir: 'eof' },
    ],
  };
}

describe('V3: Three AIs requirement', () => {
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
    'shows the 3-AI hint in CreateGroup and Chat, secretary replies plainly in main line, and warning clears at 3 AIs',
    { timeout: 120_000 },
    async () => {
      const scriptDir = join(inject('e2e').root, `three-ais-${Date.now()}`);
      await mkdir(scriptDir, { recursive: true });
      const claudePath = join(scriptDir, 'claude.json');
      await writeFile(claudePath, JSON.stringify(secretaryMainLineReplyScript()));

      const active = await startNet({
        nodes: 1,
        env: () => ({ SOLODAWN_FAKE_CLAUDE_SCRIPT: claudePath }),
      });
      net = active;
      const direct = await startDirectSurface(active.rendezvous, inject('browser'));
      surface = direct;
      const [node] = active.nodes;

      const secFolder = await makeFolder(active.dir, 'sec-ws');
      const codexFolder = await makeFolder(active.dir, 'codex-ws');
      const workerFolder = await makeFolder(active.dir, 'worker-ws');

      const context = await openDirectContext(browser, contexts, direct);
      const page = await context.newPage();
      await page.setViewportSize({ width: 1280, height: 860 });

      await page.goto(`${direct.url}#/onboarding`);
      const code = await readPairingCode(page, direct.signal);
      const paired = await node.run(
        'solodawn',
        'pair',
        code,
        '--name',
        'studio',
        '--rendezvous',
        direct.signal,
      );
      expect(paired.code, paired.stderr).toBe(0);

      await poll(() => page.locator('input[name="acct-claude"][value="personal"]').count()).toBe(1);
      await page.locator('input[name="acct-claude"][value="personal"]').check();
      await poll(() => page.locator('fieldset.acct').first().isDisabled()).toBe(false);
      await page.locator('input[name="acct-codex"][value="api_key"]').check();
      await finishOnboarding(page, true);
      await poll(() => page.locator('.bar a.conn').getAttribute('data-st')).toBe('ok');

      // FinishOnboarding opens CreateGroup automatically; verify 3-AI hint shows while < 2 working AIs are ticked.
      const dialog = page.getByRole('dialog', { name: 'New group' });
      await dialog.getByLabel('Group name').fill('Three AIs Group');
      await dialog.getByLabel('Your name in the group').fill('Alex');
      await poll(() => dialog.locator('.three-ais').textContent()).toContain(
        '3 AIs: 1 secretary + at least 2 working AIs',
      );

      const claudeRow = dialog
        .locator('.ai')
        .filter({ hasText: 'Claude Code' })
        .filter({ hasText: 'studio' });
      await claudeRow.getByRole('checkbox').check();
      await claudeRow.getByLabel('Name of Claude Code in the group').fill('Secretary');
      await chooseFolder(claudeRow, secFolder);
      await claudeRow.getByRole('radio', { name: 'Make Secretary the secretary' }).check();

      const codexRow = dialog
        .locator('.ai')
        .filter({ hasText: 'Codex' })
        .filter({ hasText: 'studio' });
      await codexRow.getByRole('checkbox').check();
      await codexRow.getByLabel('Name of Codex in the group').fill('Codex');
      await chooseFolder(codexRow, codexFolder);

      // 1 secretary + 1 working AI => workingCount is 1 < 2: hint stays visible, submit stays enabled.
      expect(await dialog.locator('.three-ais').textContent()).toContain('Add AI');
      const submitBtn = dialog.getByRole('button', { name: 'Create group' });
      expect(await submitBtn.isDisabled()).toBe(false);

      await submitBtn.click();
      await poll(() => page.url()).toMatch(/#\/groups\/[^/]+$/);

      // Group has 1 secretary + 1 working AI: inline need-ais warning is shown in Chat.
      const members = page.locator('#chat-side .members');
      await poll(() => members.locator('.member').count()).toBe(2);
      await poll(() => page.locator('.line .need-ais').textContent()).toContain(
        'Add at least 2 working AIs besides the secretary via Add AI',
      );

      // Request triggers secretary, which posts a plain message in main line (not a tool error).
      await sendChatMessage(page, 'Build the onboarding checklist');
      await poll(() => page.locator('.line .msg .text').allTextContents()).toContain(
        'I need at least 2 working AIs besides the secretary before proposing a brief, so assignee and reviewer are different AIs. Click Add AI to add another CLI or add the same CLI twice with a new name and folder.',
      );
      expect(await page.locator('.line section.card', { hasText: 'Task brief' }).count()).toBe(0);

      // Add the same CLI (Claude Code) a 2nd time via Add AI with a new name and folder.
      await members.locator('button', { hasText: 'Add AI' }).click();
      const addDlg = page.locator('dialog.dlg[open]');
      await addDlg.locator('.choice', { hasText: 'Claude Code · studio' }).locator('input').check();
      await addDlg.getByRole('textbox', { name: 'Name in the group' }).fill('Worker');
      await chooseFolder(addDlg, workerFolder);
      await addDlg.locator('button.btn-primary', { hasText: 'Add AI' }).click();
      await poll(() => addDlg.count()).toBe(0);

      await poll(() => members.locator('.member').count()).toBe(3);
      await poll(() => page.locator('.line .need-ais').count()).toBe(0);
    },
  );
});
