// End-to-end milestone 5 (E-M5): full main-line flow from owner request through brief card,
// confirmation, two parallel tasks with 6-step progress bars, local checks, cross-AI review,
// automatic squash-merge, task threads, and secretary summary cards (U-4, U-7, U-8, U-9, A5.1),
// plus approval card and decision card round-trip in the browser (Owner override 1).
import { chmod, mkdir, readFile, writeFile } from 'node:fs/promises';
import { delimiter, join } from 'node:path';
import type { Browser, BrowserContext, Page } from 'playwright';
import { afterAll, afterEach, beforeAll, describe, expect, inject, test, vi } from 'vitest';
import { launch } from './config';
import {
  claudeInitSteps,
  codexInitSteps,
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

async function pairNode(page: Page, surface: DirectSurface, node: Node, name: string) {
  await page.goto(`${surface.url}#/onboarding`);
  const code = await readPairingCode(page, surface.signal);
  const paired = await node.run(
    'solodawn',
    'pair',
    code,
    '--name',
    name,
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
                // T-1 uses coordinator Codex ("Reviewer"): T-2 stays in gateT2 until T-1's summary card, and
                // deliver.go (aiGroupKey) serializes turns per AI member, so workstation Codex is busy.
                {
                  title: 'Build release checklist',
                  assignee: 'Claude Code',
                  reviewer: 'Reviewer',
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
      // Turn 2: woken when owner confirms the brief.
      { dir: 'stdin', msg: { type: 'user', message: { role: 'user' } } },
      {
        dir: 'stdout',
        msg: {
          type: 'result',
          subtype: 'success',
          is_error: false,
          result: 'brief-confirmed',
          session_id: '$session',
        },
      },
      // Turn 3: woken by task_merged notice for T-1.
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
      // Turn 4: woken by task_merged notice for T-2.
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
  return {
    steps: [
      ...claudeInitSteps(),
      // Turn 1: approve blocks until human clicks Approve, returns allow, then raises DecisionCard.
      {
        dir: 'mcp<-cli',
        msg: {
          jsonrpc: '2.0',
          id: 1,
          method: 'tools/call',
          params: {
            name: 'approve',
            arguments: {
              tool_name: 'Bash',
              input: {
                command: 'git clean -fd',
                description: 'Remove untracked build artifacts before packaging',
              },
              tool_use_id: 'toolu_approve_1',
            },
          },
        },
      },
      {
        dir: 'mcp->cli',
        msg: {
          jsonrpc: '2.0',
          id: 1,
          result: { content: [{ type: 'text', text: '{"behavior":"allow"}' }], isError: false },
        },
      },
      {
        dir: 'mcp<-cli',
        msg: {
          jsonrpc: '2.0',
          id: 2,
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
      { dir: 'mcp->cli', msg: { jsonrpc: '2.0', id: 2, result: { isError: false } } },
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
      // Turn 2: woken after human picks option B on DecisionCard.
      { dir: 'stdin', msg: { type: 'user', message: { role: 'user' } } },
      {
        dir: 'mcp<-cli',
        msg: {
          jsonrpc: '2.0',
          id: 3,
          method: 'tools/call',
          params: {
            name: 'post',
            arguments: { text: 'Confirmed approval and selected option B: SQLite table.' },
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
    'runs owner request -> brief confirm -> two checked, cross-reviewed, merged tasks -> summary cards',
    { timeout: 120_000 },
    async () => {
      const scriptDir = join(inject('e2e').root, `m5-main-${Date.now()}`);
      await mkdir(scriptDir, { recursive: true });
      const node0Claude = join(scriptDir, 'node0-claude.json');
      const node0Codex = join(scriptDir, 'node0-codex.json');
      const node1Claude = join(scriptDir, 'node1-claude.json');
      const node1Codex = join(scriptDir, 'node1-codex.json');
      const gateT1 = join(scriptDir, 'release-t1');
      const gateT2 = join(scriptDir, 'release-t2');

      await writeFile(node0Claude, JSON.stringify(claudeTaskScript()));
      await writeFile(node0Codex, JSON.stringify(codexTaskScript()));
      await writeFile(node1Claude, JSON.stringify(secretaryM5Script()));
      await writeFile(node1Codex, JSON.stringify(codexReviewScript()));

      const active = await startNet({
        nodes: 2,
        env: (index) =>
          index === 0
            ? {
                ...gitIdentityEnv,
                SOLODAWN_FAKE_CLAUDE_SCRIPT: node0Claude,
                SOLODAWN_FAKE_CODEX_SCRIPT: node0Codex,
              }
            : {
                ...gitIdentityEnv,
                SOLODAWN_FAKE_CLAUDE_SCRIPT: node1Claude,
                SOLODAWN_FAKE_CODEX_SCRIPT: node1Codex,
              },
      });
      net = active;
      const direct = await startDirectSurface(active.rendezvous, inject('browser'));
      surface = direct;
      const [homeNode, secNode] = active.nodes;
      const repo = await initGitRepo(active.dir, 'm5-repo', '# Release pipeline\n');

      const context = await openDirectContext(browser, contexts, direct);
      const page = await context.newPage();

      await pairNode(page, direct, homeNode, 'workstation');
      await pairNode(page, direct, secNode, 'coordinator');

      await page.locator('button', { hasText: 'New project group' }).first().click();
      const createDlg = page.locator('dialog.dlg[open]');
      const inputs = createDlg.locator('input.input');
      await inputs.nth(0).fill('Release Ops');
      await inputs.nth(1).fill(repo);
      await inputs.nth(2).fill('main');
      await inputs.nth(3).fill('Alex');
      await createDlg.locator('select.input').first().selectOption('none');
      // Gate T-1 and T-2 inside run_checks on marker files (`*/1-*` relies on the host's
      // `wt/<num>-<slug>` worktree folder naming) with a 60 s bound so orphaned shells exit.
      const checkCmd = [
        'test -f README.md',
        `i=0; case "$(pwd)" in */1-*) until [ -f "${gateT1}" ] || [ $i -ge 1200 ]; do sleep 0.05; i=$((i+1)); done; test -f "${gateT1}";; *) until [ -f "${gateT2}" ] || [ $i -ge 1200 ]; do sleep 0.05; i=$((i+1)); done; test -f "${gateT2}";; esac`,
      ].join(' && ');
      await createDlg.locator('textarea.input').fill(checkCmd);
      await createDlg.locator('select.input').nth(1).selectOption({ label: 'workstation' });
      await createDlg.locator('button.btn-primary', { hasText: 'Create group' }).click();
      await poll(() => page.url()).toMatch(/#\/groups\/[^/]+$/);

      const members = page.locator('#chat-side .members');

      // Add Secretary on coordinator (Node 1), Claude Code on workstation (Node 0), Codex on workstation (Node 0).
      await members.locator('button', { hasText: 'Add AI' }).click();
      const addDlg = page.locator('dialog.dlg[open]');
      await poll(() => addDlg.locator('.choice').count()).toBe(4);
      await addDlg
        .locator('.choice', { hasText: 'Claude Code · coordinator' })
        .locator('input')
        .check();
      await addDlg.locator('.sec-opt input[type="checkbox"]').check();
      await poll(() =>
        addDlg.getByRole('textbox', { name: 'Name in the group' }).inputValue(),
      ).toBe('Secretary');
      await addDlg.locator('button.btn-primary', { hasText: 'Add AI' }).click();
      await poll(() => addDlg.count()).toBe(0);
      await poll(() => members.locator('.member').count()).toBe(1);

      await members.locator('button', { hasText: 'Add AI' }).click();
      await poll(() => addDlg.locator('.choice').count()).toBe(4);
      await addDlg
        .locator('.choice', { hasText: 'Claude Code · workstation' })
        .locator('input')
        .check();
      await addDlg.locator('button.btn-primary', { hasText: 'Add AI' }).click();
      await poll(() => addDlg.count()).toBe(0);
      await poll(() => members.locator('.member').count()).toBe(2);

      await members.locator('button', { hasText: 'Add AI' }).click();
      await poll(() => addDlg.locator('.choice').count()).toBe(4);
      await addDlg.locator('.choice', { hasText: 'Codex · workstation' }).locator('input').check();
      await addDlg.locator('button.btn-primary', { hasText: 'Add AI' }).click();
      await poll(() => addDlg.count()).toBe(0);
      await poll(() => members.locator('.member').count()).toBe(3);

      await members.locator('button', { hasText: 'Add AI' }).click();
      await poll(() => addDlg.locator('.choice').count()).toBe(4);
      await addDlg.locator('.choice', { hasText: 'Codex · coordinator' }).locator('input').check();
      await addDlg.getByRole('textbox', { name: 'Name in the group' }).fill('Reviewer');
      await addDlg.locator('button.btn-primary', { hasText: 'Add AI' }).click();
      await poll(() => addDlg.count()).toBe(0);
      await poll(() => members.locator('.member').count()).toBe(4);

      // Owner sends plain request in chat -> Secretary posts BriefCard.
      await sendChatMessage(page, 'Build the release checklist and changelog');

      const briefCard = page.locator('.line section.card', { hasText: 'Task brief' });
      await poll(() => briefCard.count()).toBe(1);
      await poll(() => briefCard.textContent()).toContain('Needs you');
      await poll(() => briefCard.textContent()).toContain(
        'Add release checklist and changelog generator',
      );
      await poll(() => briefCard.textContent()).toContain('Build release checklist');
      await poll(() => briefCard.textContent()).toContain('Build changelog generator');
      await poll(() => page.locator('.chat nav.needbar').textContent()).toContain(
        'Waiting on you · 1',
      );

      // The only human action in A5.1: clicking Confirm & start on the brief card.
      await briefCard.locator('button.btn-dawn', { hasText: 'Confirm & start' }).click();
      await poll(() => briefCard.textContent()).toContain('Alex confirmed');

      const task1Card = page.locator('.line section.card', { hasText: 'T-1' }).first();
      const task2Card = page.locator('.line section.card', { hasText: 'T-2' }).first();
      await poll(() => task1Card.count()).toBe(1);
      await poll(() => task2Card.count()).toBe(1);

      // Wait until both task CLIs have started, swap workstation Claude to its reviewer script for
      // T-2, then release T-1 (reviewed by Reviewer on coordinator while T-2 stays held at gateT2).
      await vi.waitFor(
        async () => {
          const log = await readFile(homeNode.env.SOLODAWN_FAKECLI_LOG, 'utf8').catch(() => '');
          expect(log).toMatch(/^start claude /m);
          expect(log).toMatch(/^start codex /m);
        },
        { timeout: 15_000, interval: 25 },
      );
      await writeFile(node0Claude, JSON.stringify(claudeReviewScript()));
      await writeFile(gateT1, 'ok');

      // Wait until T-1 merges and Secretary posts SummaryCard #1, then release T-2.
      const summaryCards = page.locator('.line section.card', { hasText: 'Review score' });
      await poll(() => task1Card.textContent(), 30_000).toContain('Merged · 96');
      await poll(() => summaryCards.count(), 20_000).toBe(1);
      await writeFile(gateT2, 'ok');

      // Verify T-2 also completes local checks, passes review, merges, and gets SummaryCard #2.
      await poll(() => task2Card.textContent(), 30_000).toContain('Merged · 94');
      await poll(() => summaryCards.count(), 20_000).toBe(2);

      expect(await task1Card.locator('.steps i').count()).toBe(6);
      await poll(() => task1Card.textContent()).toContain('Local checks passed');
      await poll(() => task1Card.textContent()).toContain('Round 1 · 96, passed');
      await poll(() => task2Card.textContent()).toContain('Local checks passed');
      await poll(() => task2Card.textContent()).toContain('Round 1 · 94, passed');

      // No approval cards appeared during A5.1.
      expect(await page.locator('section.card[aria-label="Approval"]').count()).toBe(0);

      const summaryText = (await summaryCards.allTextContents()).join('\n');
      expect(summaryText).toContain('T-1');
      expect(summaryText).toContain('96');
      expect(summaryText).toContain('T-2');
      expect(summaryText).toContain('94');
      expect(summaryText).toContain('1 round');

      // Open T-1's thread fold and verify progress messages, ChecksRow, ReviewBlock, task_merged, and summary jump link.
      await task1Card.locator('details.fold > summary').click();
      const thread1 = task1Card.locator('.thread');
      await poll(() => thread1.textContent()).toContain(
        'Drafted checklist rules and running local checks.',
      );
      await poll(() => thread1.textContent()).toContain('Local checks passed');
      await poll(() => thread1.locator('.review').textContent()).toContain(
        'Checklist assertions cover edge cases.',
      );
      await poll(() => thread1.textContent()).toContain('Secretary merged T-1');

      const summaryJump = thread1.locator('a.tcard').first();
      await poll(() => summaryJump.count()).toBe(1);
      await summaryJump.click();
      await poll(() => page.url()).toMatch(/#\/groups\/[^/]+$/);
    },
  );

  test(
    'completes approval card and 4-option decision card round-trip in the browser',
    { timeout: 90_000 },
    async () => {
      const { root, bin } = inject('e2e');
      const scriptDir = join(root, `m5-cards-${Date.now()}`);
      const wrapperDir = join(scriptDir, 'bin');
      await mkdir(wrapperDir, { recursive: true });
      const claudePath = join(scriptDir, 'claude-cards.json');
      const stdinLog = join(scriptDir, 'claude-stdin.log');
      const claudeWrapper = join(wrapperDir, 'claude');
      await writeFile(claudePath, JSON.stringify(approvalAndDecisionScript()));
      await writeFile(
        claudeWrapper,
        [
          `#!${process.execPath}`,
          "const { spawn } = require('node:child_process');",
          "const { appendFileSync } = require('node:fs');",
          `const child = spawn(${JSON.stringify(join(bin, 'claude'))}, process.argv.slice(2), {`,
          "  stdio: ['pipe', 'inherit', 'inherit'],",
          '});',
          "process.stdin.on('data', (chunk) => {",
          `  appendFileSync(${JSON.stringify(stdinLog)}, chunk);`,
          '  child.stdin.write(chunk);',
          '});',
          "process.stdin.on('end', () => child.stdin.end());",
          "child.on('exit', (code) => process.exit(code ?? 1));",
          '',
        ].join('\n'),
      );
      await chmod(claudeWrapper, 0o755);

      const active = await startNet({
        nodes: 1,
        env: () => ({
          ...gitIdentityEnv,
          PATH: `${wrapperDir}${delimiter}${bin}${delimiter}${process.env.PATH ?? ''}`,
          SOLODAWN_FAKE_CLAUDE_SCRIPT: claudePath,
        }),
      });
      net = active;
      const direct = await startDirectSurface(active.rendezvous, inject('browser'));
      surface = direct;
      const [node] = active.nodes;
      const repo = await initGitRepo(active.dir, 'cards-repo', '# Card Roundtrip\n');

      const context = await openDirectContext(browser, contexts, direct);
      const page = await context.newPage();

      await pairNode(page, direct, node, 'workstation');

      await page.locator('button', { hasText: 'New project group' }).first().click();
      const createDlg = page.locator('dialog.dlg[open]');
      const inputs = createDlg.locator('input.input');
      await inputs.nth(0).fill('Card Roundtrip');
      await inputs.nth(1).fill(repo);
      await inputs.nth(2).fill('main');
      await inputs.nth(3).fill('Alex');
      await createDlg.locator('select.input').first().selectOption('none');
      await createDlg.locator('button.btn-primary', { hasText: 'Create group' }).click();
      await poll(() => page.url()).toMatch(/#\/groups\/[^/]+$/);

      const members = page.locator('#chat-side .members');
      await members.locator('button', { hasText: 'Add AI' }).click();
      const addDlg = page.locator('dialog.dlg[open]');
      await poll(() => addDlg.locator('.choice').count()).toBe(2);
      await addDlg
        .locator('.choice', { hasText: 'Claude Code · workstation' })
        .locator('input')
        .check();
      await addDlg.locator('button.btn-primary', { hasText: 'Add AI' }).click();
      await poll(() => addDlg.count()).toBe(0);
      await poll(() => members.locator('.member').count()).toBe(1);

      // Trigger Claude Code turn 1 -> ApprovalCard appears while approve waits for owner action.
      await sendChatMessage(page, '@Claude Code clean artifacts and pick cache');

      const approvalCard = page.locator('section.card[aria-label="Approval"]');
      await poll(() => approvalCard.count()).toBe(1);
      await poll(() => approvalCard.textContent()).toContain('Needs you');
      await poll(() => approvalCard.textContent()).toContain('Command');
      await poll(() => approvalCard.locator('pre.cmd').textContent()).toContain('git clean -fd');
      await poll(() => approvalCard.textContent()).toContain(
        'Remove untracked build artifacts before packaging',
      );
      await poll(() => page.locator('.chat nav.needbar').textContent()).toContain('Approval');

      await approvalCard.locator('button.btn-dawn', { hasText: 'Approve' }).click();
      await poll(() => approvalCard.textContent()).toContain('Approved by Alex');

      // approve resolves with allow in turn 1 and Claude raises a 4-option DecisionCard.
      const decisionCard = page.locator('.line section.card', { hasText: 'Your call' });
      await poll(() => decisionCard.count()).toBe(1);
      await poll(() => decisionCard.textContent()).toContain('Needs you');
      await poll(() => decisionCard.textContent()).toContain(
        'Which caching backend should we ship?',
      );
      await poll(() => decisionCard.locator('button.opt').count()).toBe(4);
      await poll(() =>
        decisionCard.locator('button.opt', { hasText: 'B · SQLite table' }).textContent(),
      ).toContain('Recommended');
      await poll(() => decisionCard.locator('.free-answer input').count()).toBe(1);
      await poll(() => page.locator('.chat nav.needbar').textContent()).toContain('Your call');

      await decisionCard.locator('button.opt', { hasText: 'B · SQLite table' }).click();
      await poll(() => decisionCard.textContent()).toContain('Alex decided');
      await poll(() => decisionCard.locator('.opt.chosen').textContent()).toContain('Chosen');
      await poll(() => page.locator('.chat nav.needbar').count()).toBe(0);

      // Turn 2 confirms receipt of both the approval and the chosen decision option, and the
      // underlying CLI stdin received the formatted decision choice.
      await poll(() => page.locator('.line .msg .text').allTextContents()).toContain(
        'Confirmed approval and selected option B: SQLite table.',
      );
      const stdinLines = (await readFile(stdinLog, 'utf8'))
        .trim()
        .split('\n')
        .filter((line) => !line.includes('"control_request"'));
      expect(stdinLines).toHaveLength(2);
      expect(stdinLines[1]).toContain('Which caching backend should we ship?');
      expect(stdinLines[1]).toContain('Choice: B (SQLite table)');
    },
  );
});
