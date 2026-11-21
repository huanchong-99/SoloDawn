// End-to-end milestone 3 (E-M3): AI member panel, add-AI dialog, mentioning Claude Code and Codex
// with session continuity, 1-minute idle dormancy, and resume with memory (U-3, A3.1, A3.4).
import { execFile } from 'node:child_process';
import { mkdir, readFile, writeFile } from 'node:fs/promises';
import { join } from 'node:path';
import { promisify } from 'node:util';
import type { Browser, BrowserContext, Page } from 'playwright';
import { afterAll, afterEach, beforeAll, describe, expect, inject, test, vi } from 'vitest';
import { launch } from './config';
import { startNet, type Net, type Node } from './nodes';

const exec = promisify(execFile);
const codePattern =
  /\b([23456789ABCDEFGHJKLMNPQRSTUVWXYZ]{4}(?:-[23456789ABCDEFGHJKLMNPQRSTUVWXYZ]{4}){3})\b/;

async function initGitRepo(dir: string): Promise<string> {
  const repo = join(dir, 'website-repo');
  await mkdir(repo, { recursive: true });
  await exec('git', ['init', '-b', 'main'], { cwd: repo });
  await exec('git', ['config', 'user.name', 'Alex'], { cwd: repo });
  await exec('git', ['config', 'user.email', 'alex@example.test'], { cwd: repo });
  await writeFile(join(repo, 'README.md'), '# Website\n');
  await exec('git', ['add', 'README.md'], { cwd: repo });
  await exec('git', ['commit', '-m', 'Initial commit'], { cwd: repo });
  return repo;
}

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

async function pairAndCreateGroup(page: Page, net: Net, node: Node, repo: string) {
  await page.goto(`${net.rendezvous.url}#/onboarding`);
  let code = '';
  await vi.waitFor(async () => {
    const text = (await page.locator('#prompt').textContent()) ?? '';
    const match = codePattern.exec(text);
    expect(match).not.toBeNull();
    code = match ? match[1] : '';
  });
  const paired = await node.run(
    'solodawn',
    'pair',
    code,
    '--name',
    'studio',
    '--rendezvous',
    net.rendezvous.signal,
  );
  expect(paired.code, paired.stderr).toBe(0);

  await page.locator('input[name="acct-claude"][value="personal"]').check();
  await page.locator('input[name="acct-codex"][value="api_key"]').check();
  await page.locator('a.done-btn').click();
  await expect.poll(() => page.url()).toContain('#/groups');
  await expect.poll(() => page.locator('.bar a.conn').getAttribute('data-st')).toBe('ok');

  await page.locator('button', { hasText: 'New project group' }).first().click();
  const dlg = page.locator('dialog.dlg[open]');
  const inputs = dlg.locator('input.input');
  await inputs.nth(0).fill('Website');
  await inputs.nth(1).fill(repo);
  await inputs.nth(2).fill('main');
  await inputs.nth(3).fill('Alex');
  await dlg.locator('select.input').first().selectOption('none');
  await dlg.locator('button.btn-primary', { hasText: 'Create group' }).click();
  await expect.poll(() => page.url()).toMatch(/#\/groups\/[^/]+$/);
}

function claudeTwoTurnScript() {
  return {
    steps: [
      { dir: 'stdin', msg: { type: 'user', message: { role: 'user' } } },
      {
        dir: 'mcp<-cli',
        msg: {
          jsonrpc: '2.0',
          id: 0,
          method: 'initialize',
          params: {
            protocolVersion: '2025-11-25',
            capabilities: {},
            clientInfo: { name: 'claude-code', version: '2.1.311' },
          },
        },
      },
      { dir: 'mcp->cli', msg: { jsonrpc: '2.0', id: 0, result: {} } },
      { dir: 'mcp<-cli', msg: { jsonrpc: '2.0', method: 'notifications/initialized' } },
      { dir: 'stdout', msg: { type: 'system', subtype: 'init', session_id: '$session' } },
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
      { dir: 'stdin', msg: { type: 'user', message: { role: 'user' } } },
      {
        dir: 'mcp<-cli',
        msg: {
          jsonrpc: '2.0',
          id: 0,
          method: 'initialize',
          params: {
            protocolVersion: '2025-11-25',
            capabilities: {},
            clientInfo: { name: 'claude-code', version: '2.1.311' },
          },
        },
      },
      { dir: 'mcp->cli', msg: { jsonrpc: '2.0', id: 0, result: {} } },
      { dir: 'mcp<-cli', msg: { jsonrpc: '2.0', method: 'notifications/initialized' } },
      { dir: 'stdout', msg: { type: 'system', subtype: 'init', session_id: '$session' } },
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
      { dir: 'stdin', msg: { method: 'initialize' } },
      { dir: 'stdout', msg: { id: '$id', result: {} } },
      { dir: 'stdin', msg: { method: 'initialized' } },
      { dir: 'stdin', msg: { method: 'thread/start' } },
      { dir: 'stdout', msg: { id: '$id', result: { thread: { id: '$session' } } } },
      { dir: 'stdin', msg: { method: 'turn/start', params: { threadId: '$session' } } },
      {
        dir: 'stdout',
        msg: {
          id: '$id',
          result: { turn: { id: 'turn-codex-1', status: 'inProgress', error: null } },
        },
      },
      {
        dir: 'mcp<-cli',
        msg: {
          jsonrpc: '2.0',
          id: 0,
          method: 'initialize',
          params: {
            protocolVersion: '2025-06-18',
            capabilities: {},
            clientInfo: { name: 'codex-mcp-client', version: '0.164.0' },
          },
        },
      },
      { dir: 'mcp->cli', msg: { jsonrpc: '2.0', id: 0, result: {} } },
      { dir: 'mcp<-cli', msg: { jsonrpc: '2.0', method: 'notifications/initialized' } },
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
  const contexts: BrowserContext[] = [];

  beforeAll(async () => {
    browser = await launch(inject('browser'));
  });
  afterAll(() => browser?.close());
  afterEach(() => Promise.allSettled(contexts.splice(0).map((ctx) => ctx.close())));
  afterEach(() => net?.stop());

  test(
    'adds Claude Code and Codex, replies to mentions, sleeps after 1 minute idle, and resumes',
    { timeout: 130_000 },
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
          SOLODAWN_FAKE_CLAUDE_SCRIPT: claudePath,
          SOLODAWN_FAKE_CODEX_SCRIPT: codexPath,
        }),
      });
      net = active;
      const [node] = active.nodes;
      await setNodeIdleMinutes(node, 1);

      const repo = await initGitRepo(active.dir);
      const context = await browser.newContext({ viewport: { width: 1440, height: 900 } });
      contexts.push(context);
      const page = await context.newPage();

      await pairAndCreateGroup(page, active, node, repo);

      const members = page.locator('#chat-side .members');
      await expect.poll(() => members.textContent()).toContain('No AI in this group yet.');
      await expect.poll(() => members.textContent()).toContain('People · 1');
      await expect.poll(() => members.locator('.person').textContent()).toContain('Alex');
      await expect.poll(() => members.locator('.person').textContent()).toContain('You');
      await expect.poll(() => members.locator('.person').textContent()).toContain('Owner');

      // Add Claude Code via AddAI dialog.
      await members.locator('button', { hasText: 'Add AI' }).click();
      const addDlg = page.locator('dialog.dlg[open]');
      await expect.poll(() => addDlg.locator('.choice').count()).toBe(2);
      await addDlg.locator('.choice', { hasText: 'Claude Code · studio' }).locator('input').check();
      await expect.poll(() => addDlg.locator('input.input').inputValue()).toBe('Claude Code');
      await addDlg.locator('button.btn-primary', { hasText: 'Add AI' }).click();
      await expect.poll(() => members.locator('.member').count()).toBe(1);

      // Add Codex via AddAI dialog; Claude Code now shows "Already in this group".
      await members.locator('button', { hasText: 'Add AI' }).click();
      await expect
        .poll(() => addDlg.locator('.choice', { hasText: 'Claude Code · studio' }).textContent())
        .toContain('Already in this group');
      await addDlg.locator('.choice', { hasText: 'Codex · studio' }).locator('input').check();
      await expect.poll(() => addDlg.locator('input.input').inputValue()).toBe('Codex');
      await addDlg.locator('button.btn-primary', { hasText: 'Add AI' }).click();
      await expect.poll(() => members.locator('.member').count()).toBe(2);

      const claudeCard = members.locator('.member', { hasText: 'Claude Code' });
      const codexCard = members.locator('.member', { hasText: 'Codex' });
      await expect.poll(() => claudeCard.textContent()).toContain('studio · group home');
      await expect.poll(() => claudeCard.textContent()).toContain('Personal subscription');
      await expect
        .poll(() => claudeCard.locator('header .st').textContent())
        .toContain('Sleeping · wakes when @mentioned');
      await expect.poll(() => codexCard.textContent()).toContain('API key');

      // Mention @Claude Code using the @ completion popup (A3.1 turn 1).
      const composer = page.locator('.composer textarea');
      await composer.fill('@Clau');
      const mentionOption = page.locator('.composer .mentions [role="option"]', {
        hasText: 'Claude Code',
      });
      await expect.poll(() => mentionOption.count()).toBe(1);
      await mentionOption.click();
      await composer.fill('@Claude Code remember codeword-amber');
      await page.locator('.composer button.send').click();

      await expect
        .poll(() => page.locator('.line .msg .text').allTextContents())
        .toContain('Claude reply one: codeword-amber');
      await expect.poll(() => claudeCard.locator('header .st').textContent()).toBe('Waiting');

      // Mention @Claude Code a second time; the same process handles turn 2 without respawning.
      await composer.fill('@Claude Code what codeword did I give you?');
      await page.locator('.composer button.send').click();
      await expect
        .poll(() => page.locator('.line .msg .text').allTextContents())
        .toContain('Claude reply two in same open session: codeword-amber');

      // Mention @Codex and verify Codex also replies in chat.
      await composer.fill('@Codex say hello');
      await page.locator('.composer button.send').click();
      await expect
        .poll(() => page.locator('.line .msg .text').allTextContents())
        .toContain('Codex reply from studio');

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
      await expect
        .poll(() => readFile(node.env.SOLODAWN_FAKECLI_LOG, 'utf8'), { timeout: 10_000 })
        .toContain(`exit claude ${originalSessionId}`);

      // Swap Claude script for the resumed session and @mention Claude Code again.
      await writeFile(claudePath, JSON.stringify(claudeResumedScript()));
      await composer.fill('@Claude Code wake up and confirm the codeword');
      await page.locator('.composer button.send').click();

      await expect
        .poll(() => page.locator('.line .msg .text').allTextContents())
        .toContain('Claude resumed after sleep with memory: codeword-amber');

      const logAfterResume = await readFile(node.env.SOLODAWN_FAKECLI_LOG, 'utf8');
      const claudeStartsAfterResume = [...logAfterResume.matchAll(/^start claude (\S+)$/gm)];
      expect(claudeStartsAfterResume).toHaveLength(2);
      expect(claudeStartsAfterResume[1][1]).toBe(originalSessionId);
    },
  );
});
