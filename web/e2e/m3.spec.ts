// End-to-end milestone 3 (E-M3): AI member panel, add-AI dialog, mentioning Claude Code and Codex
// with session continuity, 1-minute idle dormancy, and resume with memory (U-3, A3.1, A3.4).
import { mkdir, readFile, writeFile } from 'node:fs/promises';
import { join } from 'node:path';
import type { Browser, BrowserContext, Page } from 'playwright';
import { afterAll, afterEach, beforeAll, describe, expect, inject, test } from 'vitest';
import { launch } from './config';
import {
  claudeInitSteps,
  codexInitSteps,
  exec,
  gitIdentityEnv,
  initGitRepo,
  openDirectContext,
  poll,
  readPairingCode,
  sendChatMessage,
  startDirectSurface,
  type DirectSurface,
} from './direct';
import { startNet, type Net, type Node } from './nodes';

async function setNodeIdleMinutes(node: Node, minutes: number) {
  const dbPath = join(node.dataDir, 'node.db');
  const script = `
    import { DatabaseSync } from 'node:sqlite';
    const db = new DatabaseSync(process.argv[1]);
    db.exec('PRAGMA busy_timeout = 5000');
    db.prepare("INSERT OR REPLACE INTO setting(key, value) VALUES ('idle_minutes', ?)").run(process.argv[2]);
    db.close();
  `;
  await exec(process.execPath, ['--input-type=module', '-e', script, dbPath, String(minutes)]);
}

async function pairAndCreateGroup(page: Page, surface: DirectSurface, node: Node, repo: string) {
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
  await poll(() => page.locator('a.done-btn').count()).toBe(1);
  await page.locator('a.done-btn').click();
  await poll(() => page.url()).toContain('#/groups');
  await poll(() => page.locator('.bar a.conn').getAttribute('data-st')).toBe('ok');

  await page.locator('button', { hasText: 'New group' }).first().click();
  const dlg = page.locator('dialog.dlg[open]');
  const inputs = dlg.locator('input.input');
  await inputs.nth(0).fill('Website');
  await inputs.nth(1).fill(repo);
  await inputs.nth(2).fill('main');
  await inputs.nth(3).fill('Alex');
  await dlg.locator('select.input').first().selectOption('none');
  await dlg.locator('button.btn-primary', { hasText: 'Create group' }).click();
  await poll(() => page.url()).toMatch(/#\/groups\/[^/]+$/);
}

function claudeTwoTurnScript() {
  return {
    steps: [
      ...claudeInitSteps(),
      {
        dir: 'mcp<-cli',
        msg: {
          jsonrpc: '2.0',
          id: 1,
          method: 'tools/call',
          params: { name: 'post', arguments: { text: 'Claude reply one: codeword-amber' } },
        },
      },
      { dir: 'mcp->cli', msg: { jsonrpc: '2.0', id: 1, result: { isError: false } } },
      {
        dir: 'stdout',
        msg: {
          type: 'result',
          subtype: 'success',
          is_error: false,
          result: 'done-1',
          session_id: '$session',
        },
      },
      { dir: 'stdin', msg: { type: 'user', message: { role: 'user' } } },
      {
        dir: 'mcp<-cli',
        msg: {
          jsonrpc: '2.0',
          id: 2,
          method: 'tools/call',
          params: {
            name: 'post',
            arguments: { text: 'Claude reply two in same open session: codeword-amber' },
          },
        },
      },
      { dir: 'mcp->cli', msg: { jsonrpc: '2.0', id: 2, result: { isError: false } } },
      {
        dir: 'stdout',
        msg: {
          type: 'result',
          subtype: 'success',
          is_error: false,
          result: 'done-2',
          session_id: '$session',
        },
      },
      { dir: 'eof' },
    ],
  };
}

function claudeResumedScript() {
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
            arguments: { text: 'Claude resumed after sleep with memory: codeword-amber' },
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
          result: 'resumed',
          session_id: '$session',
        },
      },
      { dir: 'eof' },
    ],
  };
}

function codexReplyScript() {
  return {
    session: '01a1f63f-701c-78c1-bbf3-f1ac61c56b59',
    steps: [
      ...codexInitSteps('turn-codex-1'),
      {
        dir: 'mcp<-cli',
        msg: {
          jsonrpc: '2.0',
          id: 1,
          method: 'tools/call',
          params: { name: 'post', arguments: { text: 'Codex reply from studio' } },
        },
      },
      { dir: 'mcp->cli', msg: { jsonrpc: '2.0', id: 1, result: { isError: false } } },
      {
        dir: 'stdout',
        msg: {
          method: 'turn/completed',
          params: {
            threadId: '$session',
            turn: { id: 'turn-codex-1', status: 'completed', error: null },
          },
        },
      },
      { dir: 'eof' },
    ],
  };
}

describe('E-M3: AI member panel, add-AI dialog, mentions, dormancy, and resume', () => {
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
    'adds Claude Code and Codex, replies to mentions, sleeps after 1 minute idle, and resumes',
    { timeout: 180_000 },
    async () => {
      const scriptDir = join(inject('e2e').root, `m3-scripts-${Date.now()}`);
      await mkdir(scriptDir, { recursive: true });
      const claudePath = join(scriptDir, 'claude.json');
      const codexPath = join(scriptDir, 'codex.json');
      await writeFile(claudePath, JSON.stringify(claudeTwoTurnScript()));
      await writeFile(codexPath, JSON.stringify(codexReplyScript()));

      const active = await startNet({
        nodes: 1,
        env: () => ({
          ...gitIdentityEnv,
          SOLODAWN_FAKE_CLAUDE_SCRIPT: claudePath,
          SOLODAWN_FAKE_CODEX_SCRIPT: codexPath,
        }),
      });
      net = active;
      const direct = await startDirectSurface(active.rendezvous, inject('browser'));
      surface = direct;
      const [node] = active.nodes;
      await setNodeIdleMinutes(node, 1);

      const repo = await initGitRepo(active.dir, 'website-repo', '# Website\n');
      const context = await openDirectContext(browser, contexts, direct);
      const page = await context.newPage();

      await pairAndCreateGroup(page, direct, node, repo);

      const members = page.locator('#chat-side .members');
      await poll(() => members.textContent()).toContain('No AI in this group yet.');
      await poll(() => members.textContent()).toContain('People · 1');
      await poll(() => members.locator('.person').textContent()).toContain('Alex');
      await poll(() => members.locator('.person').textContent()).toContain('You');
      await poll(() => members.locator('.person').textContent()).toContain('Owner');

      // Add Claude Code via AddAI dialog.
      await members.locator('button', { hasText: 'Add AI' }).click();
      const addDlg = page.locator('dialog.dlg[open]');
      await poll(() => addDlg.locator('.choice').count()).toBe(3);
      await addDlg.locator('.choice', { hasText: 'Claude Code · studio' }).locator('input').check();
      await poll(() =>
        addDlg.getByRole('textbox', { name: 'Name in the group' }).inputValue(),
      ).toBe('Claude Code');
      await addDlg.locator('button.btn-primary', { hasText: 'Add AI' }).click();
      await poll(() => addDlg.count()).toBe(0);
      await poll(() => members.locator('.member').count()).toBe(1);

      // Add Codex via AddAI dialog; Claude Code remains selectable.
      await members.locator('button', { hasText: 'Add AI' }).click();
      await poll(() =>
        addDlg.locator('.choice', { hasText: 'Claude Code · studio' }).textContent(),
      ).toContain('In this group · adds another member');
      await addDlg.locator('.choice', { hasText: 'Codex · studio' }).locator('input').check();
      await poll(() =>
        addDlg.getByRole('textbox', { name: 'Name in the group' }).inputValue(),
      ).toBe('Codex');
      await addDlg.locator('button.btn-primary', { hasText: 'Add AI' }).click();
      await poll(() => addDlg.count()).toBe(0);
      await poll(() => members.locator('.member').count()).toBe(2);

      const claudeCard = members.locator('.member', { hasText: 'Claude Code' });
      const codexCard = members.locator('.member', { hasText: 'Codex' });
      await poll(() => claudeCard.textContent()).toContain('group home');
      await poll(() => claudeCard.textContent()).toContain('Personal subscription');
      await poll(() => claudeCard.locator('header .st').textContent()).toContain(
        'Sleeping · wakes when @mentioned',
      );
      await poll(() => codexCard.textContent()).toContain('API key');

      // Mention @Claude Code using the @ completion popup (A3.1 turn 1).
      const composer = page.locator('.composer textarea');
      const sendBtn = page.locator('.composer button.send');
      await composer.fill('@Clau');
      const mentionOption = page.locator('.composer .mentions [role="option"]', {
        hasText: 'Claude Code',
      });
      await poll(() => mentionOption.count()).toBe(1);
      await mentionOption.click();
      await poll(() => composer.inputValue()).toBe('@Claude Code ');
      await composer.pressSequentially('remember codeword-amber');
      await poll(() => composer.inputValue()).toBe('@Claude Code remember codeword-amber');
      await poll(() => sendBtn.isDisabled()).toBe(false);
      await sendBtn.click();
      await poll(() => composer.inputValue()).toBe('');

      await poll(() => page.locator('.line .msg .text').allTextContents()).toContain(
        'Claude reply one: codeword-amber',
      );
      await poll(() => claudeCard.locator('header .st').textContent()).toBe('Waiting');

      // Mention @Claude Code a second time; the same process handles turn 2 without respawning.
      await sendChatMessage(page, '@Claude Code what codeword did I give you?');
      await poll(() => page.locator('.line .msg .text').allTextContents()).toContain(
        'Claude reply two in same open session: codeword-amber',
      );

      // Pause the group: a mention of @Codex waits, and resuming delivers it (A4.7).
      await page.getByRole('button', { name: 'Stop AIs' }).click();
      await page.getByRole('button', { name: 'Pause', exact: true }).click();
      const pausedNotice = page.locator('.dock .paused');
      await poll(() => pausedNotice.textContent()).toContain('You paused this group');
      await poll(() => page.locator('.glist .st').textContent()).toBe('Paused');
      await sendChatMessage(page, '@Codex say hello');
      await poll(() => page.locator('.line .msg .text').allTextContents()).toContain(
        '@Codex say hello',
      );
      expect(await page.locator('.line').textContent()).not.toContain('Codex reply from studio');
      await page.getByRole('button', { name: 'Resume' }).click();
      await poll(() => pausedNotice.count()).toBe(0);
      await poll(() => page.locator('.line .msg .text').allTextContents()).toContain(
        'Codex reply from studio',
      );

      const logBeforeIdle = await readFile(node.env.SOLODAWN_FAKECLI_LOG, 'utf8');
      const claudeStartsBeforeIdle = [...logBeforeIdle.matchAll(/^start claude (\S+)$/gm)];
      expect(claudeStartsBeforeIdle).toHaveLength(1);
      const originalSessionId = claudeStartsBeforeIdle[0][1];

      // After the 1-minute idle timeout (A3.4), the CLI process exits and the card shows dormant.
      await expect
        .poll(() => claudeCard.locator('header .st').textContent(), {
          timeout: 80_000,
          interval: 500,
        })
        .toContain('Sleeping · wakes when @mentioned');
      await poll(() => readFile(node.env.SOLODAWN_FAKECLI_LOG, 'utf8')).toContain(
        `exit claude ${originalSessionId}`,
      );

      // Swap Claude script for the resumed session and @mention Claude Code again.
      await writeFile(claudePath, JSON.stringify(claudeResumedScript()));
      await sendChatMessage(page, '@Claude Code wake up and confirm the codeword');

      await poll(() => page.locator('.line .msg .text').allTextContents()).toContain(
        'Claude resumed after sleep with memory: codeword-amber',
      );

      const logAfterResume = await readFile(node.env.SOLODAWN_FAKECLI_LOG, 'utf8');
      const claudeStartsAfterResume = [...logAfterResume.matchAll(/^start claude (\S+)$/gm)];
      expect(claudeStartsAfterResume).toHaveLength(2);
      expect(claudeStartsAfterResume[1][1]).toBe(originalSessionId);

      // Take all AIs of the computer offline, which closes the session, and bring them back (A4.8).
      await page.getByRole('button', { name: 'Stop AIs' }).click();
      await page.getByRole('button', { name: 'Take offline' }).click();
      const offlineBanner = page.locator('.ban .off');
      await poll(() => offlineBanner.textContent()).toContain('Your AIs on studio are offline.');
      await poll(() => claudeCard.locator('header .st').textContent()).toBe('Offline · by you');
      await offlineBanner.getByRole('button', { name: 'Bring back online' }).click();
      await poll(() => offlineBanner.count()).toBe(0);
      await poll(() => claudeCard.locator('header .st').textContent()).toContain('Sleeping');

      // Correct what was set at creation: the name and a check command (R-103).
      await page.getByRole('button', { name: 'Edit group settings' }).click();
      const settings = page.getByRole('dialog', { name: 'Edit group settings' });
      await settings.getByLabel('Group name').fill('Website two');
      await settings.getByLabel('Check commands').fill('git status');
      await settings.getByLabel('Pass score').fill('85');
      await settings.getByRole('button', { name: 'Save' }).click();
      await poll(() => settings.count()).toBe(0);
      await poll(() => page.locator('.chat-head h2').textContent()).toBe('Website two');
      await page.getByRole('button', { name: 'Edit group settings' }).click();
      expect(await settings.getByLabel('Check commands').inputValue()).toBe('git status');
      expect(await settings.getByLabel('Pass score').inputValue()).toBe('85');
      await settings.getByRole('button', { name: 'Cancel' }).click();

      // Correct an account type on the computers page; the member in the group follows (R-50).
      const chat = page.url();
      await page.locator('.rail a[href="#/nodes"]').click();
      await page.getByRole('button', { name: 'Account type · Codex' }).click();
      const codexRow = page.locator('.ai-row', { hasText: 'Codex' });
      await codexRow.getByRole('radio', { name: /^Team plan/ }).check();
      await poll(() => codexRow.locator('.badge').first().textContent()).toBe('Team plan');
      await page.goto(chat);
      await poll(() => codexCard.textContent()).toContain('Team plan');

      // Remove an AI that is not external, in two presses (R-11).
      await codexCard.getByRole('button', { name: 'Remove AI' }).click();
      await codexCard.getByRole('button', { name: 'Confirm removal' }).click();
      await poll(() => members.locator('.member').count()).toBe(1);
    },
  );
});
