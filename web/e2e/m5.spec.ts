// End-to-end milestone 5 (E-M5): full main-line flow from owner request through brief card,
// confirmation, two parallel tasks with 6-step progress bars, local checks, cross-AI review,
// automatic squash-merge, task threads, and secretary summary cards (U-4, U-7, U-8, U-9, A5.1),
// plus approval card and decision card round-trip in the browser (Owner override 1).
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
  const repo = join(dir, 'm5-repo');
  await mkdir(repo, { recursive: true });
  await exec('git', ['init', '-b', 'main'], { cwd: repo });
  await exec('git', ['config', 'user.name', 'Alex'], { cwd: repo });
  await exec('git', ['config', 'user.email', 'alex@example.test'], { cwd: repo });
  await writeFile(join(repo, 'README.md'), '# Release pipeline\n');
  await exec('git', ['add', 'README.md'], { cwd: repo });
  await exec('git', ['commit', '-m', 'Initial commit'], { cwd: repo });
  return repo;
}

async function readPairingCode(page: Page): Promise<string> {
  let code = '';
  await vi.waitFor(async () => {
    const muted = await page.locator('#prompt').evaluate((el) => el.classList.contains('muted'));
    expect(muted).toBe(false);
    const text = (await page.locator('#prompt').textContent()) ?? '';
    const match = codePattern.exec(text);
    expect(match).not.toBeNull();
    code = match ? match[1] : '';
  });
  return code;
}

async function pairNode(page: Page, net: Net, node: Node, name: string) {
  await page.goto(`${net.rendezvous.url}#/onboarding`);
  const code = await readPairingCode(page);
  const paired = await node.run(
    'solodawn',
    'pair',
    code,
    '--name',
    name,
    '--rendezvous',
    net.rendezvous.signal,
  );
  expect(paired.code, paired.stderr).toBe(0);
  await page.locator('input[name="acct-claude"][value="personal"]').check();
  await page.locator('input[name="acct-codex"][value="api_key"]').check();
  await page.locator('a.done-btn').click();
  await expect.poll(() => page.url()).toContain('#/groups');
}

function claudeInitSteps() {
  return [
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
  ];
}

function codexInitSteps() {
  return [
    { dir: 'stdin', msg: { method: 'initialize' } },
    { dir: 'stdout', msg: { id: '$id', result: {} } },
    { dir: 'stdin', msg: { method: 'initialized' } },
    { dir: 'stdin', msg: { method: 'thread/start' } },
    { dir: 'stdout', msg: { id: '$id', result: { thread: { id: '$session' } } } },
    { dir: 'stdin', msg: { method: 'turn/start', params: { threadId: '$session' } } },
    {
      dir: 'stdout',
      msg: { id: '$id', result: { turn: { id: 'turn-1', status: 'inProgress', error: null } } },
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
  ];
}

function secretaryM5Script() {
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
            name: 'propose_brief',
            arguments: {
              title: 'Add release checklist and changelog generator',
              goal: 'Ship deterministic release notes and preflight verification.',
              criteria: ['Both tasks pass local checks and cross-review at 90 or above'],
              scope_in: ['Release notes builder', 'Preflight checklist'],
              scope_out: ['External deployment hooks'],
              tasks: [
                {
                  title: 'Build release checklist',
                  assignee: 'Claude Code',
                  reviewer: 'Codex',
                },
                {
                  title: 'Build changelog generator',
                  assignee: 'Codex',
                  reviewer: 'Claude Code',
                },
              ],
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
          result: 'brief-sent',
          session_id: '$session',
        },
      },
      // Turn 2: woken by task_merged notice for T-1.
      { dir: 'stdin', msg: { type: 'user', message: { role: 'user' } } },
      {
        dir: 'mcp<-cli',
        msg: {
          jsonrpc: '2.0',
          id: 2,
          method: 'tools/call',
          params: {
            name: 'post',
            arguments: {
              kind: 'summary',
              task: 'T-1',
              text: 'Release checklist merged cleanly.',
            },
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
          result: 'summary-1',
          session_id: '$session',
        },
      },
      // Turn 3: woken by task_merged notice for T-2.
      { dir: 'stdin', msg: { type: 'user', message: { role: 'user' } } },
      {
        dir: 'mcp<-cli',
        msg: {
          jsonrpc: '2.0',
          id: 3,
          method: 'tools/call',
          params: {
            name: 'post',
            arguments: {
              kind: 'summary',
              task: 'T-2',
              text: 'Changelog generator merged cleanly.',
            },
          },
        },
      },
      { dir: 'mcp->cli', msg: { jsonrpc: '2.0', id: 3, result: { isError: false } } },
      {
        dir: 'stdout',
        msg: {
          type: 'result',
          subtype: 'success',
          is_error: false,
          result: 'summary-2',
          session_id: '$session',
        },
      },
      { dir: 'eof' },
    ],
  };
}

function claudeTaskScript() {
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
              kind: 'progress',
              where: 'thread',
              text: 'Drafted checklist rules and running local checks.',
            },
          },
        },
      },
      { dir: 'mcp->cli', msg: { jsonrpc: '2.0', id: 1, result: { isError: false } } },
      {
        dir: 'mcp<-cli',
        msg: {
          jsonrpc: '2.0',
          id: 2,
          method: 'tools/call',
          params: { name: 'run_checks', arguments: {} },
        },
      },
      { dir: 'mcp->cli', msg: { jsonrpc: '2.0', id: 2, result: { isError: false } } },
      {
        dir: 'mcp<-cli',
        msg: {
          jsonrpc: '2.0',
          id: 3,
          method: 'tools/call',
          params: {
            name: 'post',
            arguments: { kind: 'progress', where: 'thread', text: 'Local checks green on T-1.' },
          },
        },
      },
      { dir: 'mcp->cli', msg: { jsonrpc: '2.0', id: 3, result: { isError: false } } },
      {
        dir: 'mcp<-cli',
        msg: {
          jsonrpc: '2.0',
          id: 4,
          method: 'tools/call',
          params: {
            name: 'submit_task',
            arguments: { summary: 'Implemented release checklist verification.' },
          },
        },
      },
      { dir: 'mcp->cli', msg: { jsonrpc: '2.0', id: 4, result: { isError: false } } },
      {
        dir: 'stdout',
        msg: {
          type: 'result',
          subtype: 'success',
          is_error: false,
          result: 'submitted-1',
          session_id: '$session',
        },
      },
      { dir: 'eof' },
    ],
  };
}

function codexTaskScript() {
  return {
    session: '01a1f63f-701c-78c1-bbf3-f1ac61c56b60',
    steps: [
      ...codexInitSteps(),
      {
        dir: 'mcp<-cli',
        msg: {
          jsonrpc: '2.0',
          id: 1,
          method: 'tools/call',
          params: {
            name: 'post',
            arguments: {
              kind: 'progress',
              where: 'thread',
              text: 'Built changelog formatter and running local checks.',
            },
          },
        },
      },
      { dir: 'mcp->cli', msg: { jsonrpc: '2.0', id: 1, result: { isError: false } } },
      {
        dir: 'mcp<-cli',
        msg: {
          jsonrpc: '2.0',
          id: 2,
          method: 'tools/call',
          params: { name: 'run_checks', arguments: {} },
        },
      },
      { dir: 'mcp->cli', msg: { jsonrpc: '2.0', id: 2, result: { isError: false } } },
      {
        dir: 'mcp<-cli',
        msg: {
          jsonrpc: '2.0',
          id: 3,
          method: 'tools/call',
          params: {
            name: 'post',
            arguments: { kind: 'progress', where: 'thread', text: 'Local checks green on T-2.' },
          },
        },
      },
      { dir: 'mcp->cli', msg: { jsonrpc: '2.0', id: 3, result: { isError: false } } },
      {
        dir: 'mcp<-cli',
        msg: {
          jsonrpc: '2.0',
          id: 4,
          method: 'tools/call',
          params: {
            name: 'submit_task',
            arguments: { summary: 'Implemented changelog generator.' },
          },
        },
      },
      { dir: 'mcp->cli', msg: { jsonrpc: '2.0', id: 4, result: { isError: false } } },
      {
        dir: 'stdout',
        msg: {
          method: 'turn/completed',
          params: {
            threadId: '$session',
            turn: { id: 'turn-1', status: 'completed', error: null },
          },
        },
      },
      { dir: 'eof' },
    ],
  };
}

function claudeReviewScript() {
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
            name: 'submit_review',
            arguments: {
              score: 94,
              items: [{ file: 'README.md', line: 1, text: 'Clear changelog output structure.' }],
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
          result: 'reviewed-2',
          session_id: '$session',
        },
      },
      { dir: 'eof' },
    ],
  };
}

function codexReviewScript() {
  return {
    session: '01a1f63f-701c-78c1-bbf3-f1ac61c56b61',
    steps: [
      ...codexInitSteps(),
      {
        dir: 'mcp<-cli',
        msg: {
          jsonrpc: '2.0',
          id: 1,
          method: 'tools/call',
          params: {
            name: 'submit_review',
            arguments: {
              score: 96,
              items: [
                { file: 'README.md', line: 1, text: 'Checklist assertions cover edge cases.' },
              ],
            },
          },
        },
      },
      { dir: 'mcp->cli', msg: { jsonrpc: '2.0', id: 1, result: { isError: false } } },
      {
        dir: 'stdout',
        msg: {
          method: 'turn/completed',
          params: {
            threadId: '$session',
            turn: { id: 'turn-1', status: 'completed', error: null },
          },
        },
      },
      { dir: 'eof' },
    ],
  };
}

function approvalAndDecisionScript() {
  const approvalArgs = {
    tool_name: 'Bash',
    input: {
      command: 'git clean -fd',
      description: 'Remove untracked build artifacts before packaging',
    },
    tool_use_id: 'toolu_approve_1',
  };
  return {
    steps: [
      ...claudeInitSteps(),
      {
        dir: 'mcp<-cli',
        msg: {
          jsonrpc: '2.0',
          id: 1,
          method: 'tools/call',
          params: { name: 'approve', arguments: approvalArgs },
        },
      },
      {
        dir: 'mcp->cli',
        msg: {
          jsonrpc: '2.0',
          id: 1,
          result: { content: [{ type: 'text', text: '{"behavior":"deny"}' }], isError: false },
        },
      },
      {
        dir: 'stdout',
        msg: {
          type: 'result',
          subtype: 'success',
          is_error: false,
          result: 'awaiting-approval',
          session_id: '$session',
        },
      },
      // Turn 2: woken after human clicks Approve; retries approve -> allow, then raises decision card.
      { dir: 'stdin', msg: { type: 'user', message: { role: 'user' } } },
      {
        dir: 'mcp<-cli',
        msg: {
          jsonrpc: '2.0',
          id: 2,
          method: 'tools/call',
          params: {
            name: 'approve',
            arguments: { ...approvalArgs, tool_use_id: 'toolu_approve_2' },
          },
        },
      },
      {
        dir: 'mcp->cli',
        msg: {
          jsonrpc: '2.0',
          id: 2,
          result: { content: [{ type: 'text', text: '{"behavior":"allow"}' }], isError: false },
        },
      },
      {
        dir: 'mcp<-cli',
        msg: {
          jsonrpc: '2.0',
          id: 3,
          method: 'tools/call',
          params: {
            name: 'request_decision',
            arguments: {
              question: 'Which caching backend should we ship?',
              context: 'The cache backs release metadata lookups.',
              options: [
                { label: 'In-memory LRU', effect: 'Zero extra storage, resets on restart' },
                { label: 'SQLite table', effect: 'Persists across restarts on the group home' },
                { label: 'Redis server', effect: 'Shared across external workers' },
                { label: 'No cache', effect: 'Reads from disk on every request' },
              ],
              recommended: 1,
              to: ['Alex'],
            },
          },
        },
      },
      { dir: 'mcp->cli', msg: { jsonrpc: '2.0', id: 3, result: { isError: false } } },
      {
        dir: 'stdout',
        msg: {
          type: 'result',
          subtype: 'success',
          is_error: false,
          result: 'awaiting-decision',
          session_id: '$session',
        },
      },
      // Turn 3: woken after human picks option B on DecisionCard.
      { dir: 'stdin', msg: { type: 'user', message: { role: 'user' } } },
      {
        dir: 'mcp<-cli',
        msg: {
          jsonrpc: '2.0',
          id: 4,
          method: 'tools/call',
          params: {
            name: 'post',
            arguments: { text: 'Confirmed approval and selected option B: SQLite table.' },
          },
        },
      },
      { dir: 'mcp->cli', msg: { jsonrpc: '2.0', id: 4, result: { isError: false } } },
      {
        dir: 'stdout',
        msg: {
          type: 'result',
          subtype: 'success',
          is_error: false,
          result: 'complete',
          session_id: '$session',
        },
      },
      { dir: 'eof' },
    ],
  };
}

describe('E-M5: full main-line task lifecycle and approval/decision round-trip', () => {
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
    'runs owner request -> brief confirm -> two checked, cross-reviewed, merged tasks -> summary cards',
    { timeout: 90_000 },
    async () => {
      const scriptDir = join(inject('e2e').root, `m5-main-${Date.now()}`);
      await mkdir(scriptDir, { recursive: true });
      const node0Claude = join(scriptDir, 'node0-claude.json');
      const node0Codex = join(scriptDir, 'node0-codex.json');
      const node1Claude = join(scriptDir, 'node1-claude.json');

      await writeFile(node0Claude, JSON.stringify(claudeTaskScript()));
      await writeFile(node0Codex, JSON.stringify(codexTaskScript()));
      await writeFile(node1Claude, JSON.stringify(secretaryM5Script()));

      const active = await startNet({
        nodes: 2,
        env: (index) =>
          index === 0
            ? {
                SOLODAWN_FAKE_CLAUDE_SCRIPT: node0Claude,
                SOLODAWN_FAKE_CODEX_SCRIPT: node0Codex,
              }
            : {
                SOLODAWN_FAKE_CLAUDE_SCRIPT: node1Claude,
              },
      });
      net = active;
      const [homeNode, secNode] = active.nodes;
      const repo = await initGitRepo(active.dir);

      const context = await browser.newContext({ viewport: { width: 1440, height: 900 } });
      contexts.push(context);
      const page = await context.newPage();

      await pairNode(page, active, homeNode, 'workstation');
      await pairNode(page, active, secNode, 'coordinator');

      await page.locator('button', { hasText: 'New project group' }).first().click();
      const createDlg = page.locator('dialog.dlg[open]');
      const inputs = createDlg.locator('input.input');
      await inputs.nth(0).fill('Release Ops');
      await inputs.nth(1).fill(repo);
      await inputs.nth(2).fill('main');
      await inputs.nth(3).fill('Alex');
      await createDlg.locator('select.input').first().selectOption('none');
      // Stagger T-1 (0.4s) and T-2 (1.3s) inside run_checks so review scripts swap cleanly
      // and task_merged notices reach Secretary in deterministic turns.
      await createDlg
        .locator('textarea.input')
        .fill('test -f README.md && case "$PWD" in */1-*) sleep 0.4;; *) sleep 1.3;; esac');
      await createDlg.locator('select.input').nth(1).selectOption({ label: 'workstation' });
      await createDlg.locator('button.btn-primary', { hasText: 'Create group' }).click();
      await expect.poll(() => page.url()).toMatch(/#\/groups\/[^/]+$/);

      const members = page.locator('#chat-side .members');

      // Add Secretary on coordinator (Node 1), Claude Code on workstation (Node 0), Codex on workstation (Node 0).
      await members.locator('button', { hasText: 'Add AI' }).click();
      const addDlg = page.locator('dialog.dlg[open]');
      await expect.poll(() => addDlg.locator('.choice').count()).toBeGreaterThanOrEqual(3);
      await addDlg
        .locator('.choice', { hasText: 'Claude Code · coordinator' })
        .locator('input')
        .check();
      await addDlg.locator('.sec-opt input[type="checkbox"]').check();
      await expect.poll(() => addDlg.locator('input.input').inputValue()).toBe('Secretary');
      await addDlg.locator('button.btn-primary', { hasText: 'Add AI' }).click();
      await expect.poll(() => members.locator('.member').count()).toBe(1);

      await members.locator('button', { hasText: 'Add AI' }).click();
      await addDlg
        .locator('.choice', { hasText: 'Claude Code · workstation' })
        .locator('input')
        .check();
      await addDlg.locator('button.btn-primary', { hasText: 'Add AI' }).click();
      await expect.poll(() => members.locator('.member').count()).toBe(2);

      await members.locator('button', { hasText: 'Add AI' }).click();
      await addDlg.locator('.choice', { hasText: 'Codex · workstation' }).locator('input').check();
      await addDlg.locator('button.btn-primary', { hasText: 'Add AI' }).click();
      await expect.poll(() => members.locator('.member').count()).toBe(3);

      // Owner sends plain request in chat -> Secretary posts BriefCard.
      await page.locator('.composer textarea').fill('Build the release checklist and changelog');
      await page.locator('.composer button.send').click();

      const briefCard = page.locator('.line section.card', { hasText: 'Task brief' });
      await expect.poll(() => briefCard.count()).toBe(1);
      await expect.poll(() => briefCard.textContent()).toContain('Needs you');
      await expect
        .poll(() => briefCard.textContent())
        .toContain('Add release checklist and changelog generator');
      await expect.poll(() => briefCard.textContent()).toContain('Build release checklist');
      await expect.poll(() => briefCard.textContent()).toContain('Build changelog generator');
      await expect
        .poll(() => page.locator('.needbar').textContent())
        .toContain('Waiting on you · 1');

      // Watch Node 0's fakecli.log so as soon as both task processes start and enter run_checks,
      // their script files switch to the cross-review scripts before submit_task spawns reviewers.
      const swapPromise = vi
        .waitFor(
          async () => {
            const log = await readFile(homeNode.env.SOLODAWN_FAKECLI_LOG, 'utf8').catch(() => '');
            expect(log).toMatch(/^start claude /m);
            expect(log).toMatch(/^start codex /m);
          },
          { timeout: 15_000, interval: 20 },
        )
        .then(async () => {
          await Promise.all([
            writeFile(node0Claude, JSON.stringify(claudeReviewScript())),
            writeFile(node0Codex, JSON.stringify(codexReviewScript())),
          ]);
        });

      // The only human action in A5.1: clicking Confirm & start on the brief card.
      await briefCard.locator('button.btn-dawn', { hasText: 'Confirm & start' }).click();
      await swapPromise;

      await expect.poll(() => briefCard.textContent()).toContain('Alex confirmed');

      const task1Card = page.locator('.line section.card', { hasText: 'T-1' }).first();
      const task2Card = page.locator('.line section.card', { hasText: 'T-2' }).first();
      await expect.poll(() => task1Card.count()).toBe(1);
      await expect.poll(() => task2Card.count()).toBe(1);

      // Verify both task cards reach Merged with 6-step bars completed, local checks passed, and review scores.
      await expect
        .poll(() => task1Card.textContent(), { timeout: 30_000 })
        .toContain('Merged · 96');
      await expect
        .poll(() => task2Card.textContent(), { timeout: 30_000 })
        .toContain('Merged · 94');

      expect(await task1Card.locator('.steps i').count()).toBe(6);
      await expect.poll(() => task1Card.textContent()).toContain('Local checks passed');
      await expect.poll(() => task1Card.textContent()).toContain('Round 1 · 96, passed');
      await expect.poll(() => task2Card.textContent()).toContain('Local checks passed');
      await expect.poll(() => task2Card.textContent()).toContain('Round 1 · 94, passed');

      // No approval cards appeared during A5.1.
      expect(await page.locator('section.card[aria-label="Approval"]').count()).toBe(0);

      // Secretary posted two Done / Merged summary cards on the main line.
      const summaryCards = page.locator('.line section.card', { hasText: 'Review score' });
      await expect.poll(() => summaryCards.count(), { timeout: 20_000 }).toBe(2);
      const summaryText = (await summaryCards.allTextContents()).join('\n');
      expect(summaryText).toContain('T-1');
      expect(summaryText).toContain('96');
      expect(summaryText).toContain('T-2');
      expect(summaryText).toContain('94');
      expect(summaryText).toContain('1 round');

      // Open T-1's thread fold and verify progress messages, ChecksRow, ReviewBlock, task_merged, and summary jump link.
      await task1Card.locator('details.fold > summary').click();
      const thread1 = task1Card.locator('.thread');
      await expect
        .poll(() => thread1.textContent())
        .toContain('Drafted checklist rules and running local checks.');
      await expect.poll(() => thread1.textContent()).toContain('Local checks passed');
      await expect
        .poll(() => thread1.locator('.review').textContent())
        .toContain('Checklist assertions cover edge cases.');
      await expect.poll(() => thread1.textContent()).toContain('Secretary merged T-1');

      const summaryJump = thread1.locator('a.tcard').first();
      await expect.poll(() => summaryJump.count()).toBe(1);
      await summaryJump.click();
      await expect.poll(() => page.url()).toMatch(/#\/groups\/[^/]+$/);
    },
  );

  test('completes approval card and 4-option decision card round-trip in the browser', async () => {
    const scriptDir = join(inject('e2e').root, `m5-cards-${Date.now()}`);
    await mkdir(scriptDir, { recursive: true });
    const claudePath = join(scriptDir, 'claude-cards.json');
    await writeFile(claudePath, JSON.stringify(approvalAndDecisionScript()));

    const active = await startNet({
      nodes: 1,
      env: () => ({ SOLODAWN_FAKE_CLAUDE_SCRIPT: claudePath }),
    });
    net = active;
    const [node] = active.nodes;
    const repo = await initGitRepo(active.dir);

    const context = await browser.newContext({ viewport: { width: 1440, height: 900 } });
    contexts.push(context);
    const page = await context.newPage();

    await pairNode(page, active, node, 'workstation');

    await page.locator('button', { hasText: 'New project group' }).first().click();
    const createDlg = page.locator('dialog.dlg[open]');
    const inputs = createDlg.locator('input.input');
    await inputs.nth(0).fill('Card Roundtrip');
    await inputs.nth(1).fill(repo);
    await inputs.nth(2).fill('main');
    await inputs.nth(3).fill('Alex');
    await createDlg.locator('select.input').first().selectOption('none');
    await createDlg.locator('button.btn-primary', { hasText: 'Create group' }).click();
    await expect.poll(() => page.url()).toMatch(/#\/groups\/[^/]+$/);

    const members = page.locator('#chat-side .members');
    await members.locator('button', { hasText: 'Add AI' }).click();
    const addDlg = page.locator('dialog.dlg[open]');
    await expect.poll(() => addDlg.locator('.choice').count()).toBe(2);
    await addDlg
      .locator('.choice', { hasText: 'Claude Code · workstation' })
      .locator('input')
      .check();
    await addDlg.locator('button.btn-primary', { hasText: 'Add AI' }).click();
    await expect.poll(() => members.locator('.member').count()).toBe(1);

    // Trigger Claude Code turn 1 -> ApprovalCard appears.
    await page.locator('.composer textarea').fill('@Claude Code clean artifacts and pick cache');
    await page.locator('.composer button.send').click();

    const approvalCard = page.locator('section.card[aria-label="Approval"]');
    await expect.poll(() => approvalCard.count()).toBe(1);
    await expect.poll(() => approvalCard.textContent()).toContain('Needs you');
    await expect.poll(() => approvalCard.textContent()).toContain('Command');
    await expect
      .poll(() => approvalCard.locator('pre.cmd').textContent())
      .toContain('git clean -fd');
    await expect
      .poll(() => approvalCard.textContent())
      .toContain('Remove untracked build artifacts before packaging');
    await expect.poll(() => page.locator('.needbar').textContent()).toContain('Approval');

    await approvalCard.locator('button.btn-dawn', { hasText: 'Approve' }).click();
    await expect.poll(() => approvalCard.textContent()).toContain('Approved by Alex');

    // Turn 2 runs approved action and posts a 4-option DecisionCard with Recommended badge on B.
    const decisionCard = page.locator('.line section.card', { hasText: 'Your call' });
    await expect.poll(() => decisionCard.count()).toBe(1);
    await expect.poll(() => decisionCard.textContent()).toContain('Needs you');
    await expect
      .poll(() => decisionCard.textContent())
      .toContain('Which caching backend should we ship?');
    await expect.poll(() => decisionCard.locator('button.opt').count()).toBe(4);
    await expect
      .poll(() => decisionCard.locator('button.opt', { hasText: 'B · SQLite table' }).textContent())
      .toContain('Recommended');
    await expect.poll(() => decisionCard.locator('.free-answer input').count()).toBe(1);
    await expect.poll(() => page.locator('.needbar').textContent()).toContain('Your call');

    await decisionCard.locator('button.opt', { hasText: 'B · SQLite table' }).click();
    await expect.poll(() => decisionCard.textContent()).toContain('Alex decided');
    await expect.poll(() => decisionCard.locator('.opt.chosen').textContent()).toContain('Chosen');
    await expect.poll(() => page.locator('.needbar').count()).toBe(0);

    // Turn 3 confirms receipt of both the approval and the chosen decision option.
    await expect
      .poll(() => page.locator('.line .msg .text').allTextContents())
      .toContain('Confirmed approval and selected option B: SQLite table.');
  });
});
