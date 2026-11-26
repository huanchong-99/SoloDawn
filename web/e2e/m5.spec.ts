// End-to-end milestone 5 (E-M5): full main-line flow from owner request through brief card,
// confirmation, two tasks with 3-step progress bars, each in its assignee's folder, cross-AI review,
// done at the passed review, task threads, and secretary summary cards (U-4, U-7, U-8, U-9, A5.1),
// plus approval card and decision card round-trip in the browser (Owner override 1).
import { chmod, mkdir, readFile, readdir, writeFile } from 'node:fs/promises';
import { delimiter, join } from 'node:path';
import type { Browser, BrowserContext, Page } from 'playwright';
import { afterAll, afterEach, beforeAll, describe, expect, inject, test } from 'vitest';
import { launch } from './config';
import {
  chooseFolder,
  claudeInitSteps,
  codexInitSteps,
  createGroup,
  makeFolder,
  openDirectContext,
  poll,
  readPairingCode,
  sendChatMessage,
  startDirectSurface,
  type DirectSurface,
  finishOnboarding,
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
  await finishOnboarding(page);
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
              criteria: ['Both tasks pass cross-review at 90 or above'],
              scope_in: ['Release notes builder', 'Preflight checklist'],
              scope_out: ['External deployment hooks'],
              // T-2 waits for T-1, so the secretary is woken for T-1's end before T-2's. Every
              // reviewer's CLI starts once: each start replays its computer's script from the top.
              tasks: [
                {
                  title: 'Build release checklist',
                  assignee: 'Claude Code',
                  reviewer: 'Reviewer',
                },
                {
                  title: 'Build changelog generator',
                  assignee: 'Codex',
                  reviewer: 'Checker',
                  depends: [0],
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
      // Turn 3: woken by the notice that T-1 is done.
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
              text: 'Release checklist is done.',
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
      // Turn 4: woken by the notice that T-2 is done.
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
              text: 'Changelog generator is done.',
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
              text: 'Drafted checklist rules and ran the project tests.',
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
          params: {
            name: 'submit_task',
            arguments: { summary: 'Implemented release checklist verification.' },
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
              text: 'Built changelog formatter and ran the project tests.',
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
          params: {
            name: 'submit_task',
            arguments: { summary: 'Implemented changelog generator.' },
          },
        },
      },
      { dir: 'mcp->cli', msg: { jsonrpc: '2.0', id: 2, result: { isError: false } } },
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

const call = (id: number, name: string, args: object) => [
  {
    dir: 'mcp<-cli',
    msg: { jsonrpc: '2.0', id, method: 'tools/call', params: { name, arguments: args } },
  },
  { dir: 'mcp->cli', msg: { jsonrpc: '2.0', id, result: { isError: false } } },
];
const turnEnd = (result: string) => ({
  dir: 'stdout',
  msg: { type: 'result', subtype: 'success', is_error: false, result, session_id: '$session' },
});
const turn = { dir: 'stdin', msg: { type: 'user', message: { role: 'user' } } };

/** A group session: a first turn that does nothing, and two that each start a direct task. */
function directTaskScript() {
  return {
    steps: [
      ...claudeInitSteps(),
      turnEnd('ready'),
      turn,
      ...call(1, 'start_task', { title: 'Dismiss the notice', plan: 'Add a close button.' }),
      turnEnd('started'),
      turn,
      ...call(2, 'start_task', { title: 'Fade the notice', plan: 'Fade it out on close.' }),
      turnEnd('queued'),
      { dir: 'eof' },
    ],
  };
}

/** The session of that task: it asks a person in the thread, and works on with the answer. */
function askingTaskScript() {
  return {
    steps: [
      ...claudeInitSteps(),
      ...call(1, 'post', { kind: 'question', where: 'thread', text: '@Alex keep the field?' }),
      turnEnd('asked'),
      turn,
      ...call(2, 'post', { kind: 'progress', where: 'thread', text: 'Keeping the field.' }),
      turnEnd('answered'),
      { dir: 'eof' },
    ],
  };
}

/** The session of the second task, which starts once the folder is free. */
function queuedTaskScript() {
  return {
    steps: [
      ...claudeInitSteps(),
      ...call(1, 'post', { kind: 'progress', where: 'thread', text: 'Fading the notice out.' }),
      turnEnd('started'),
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
    'runs owner request -> brief confirm -> two cross-reviewed tasks done in their folders -> summary cards',
    { timeout: 120_000 },
    async () => {
      const scriptDir = join(inject('e2e').root, `m5-main-${Date.now()}`);
      await mkdir(scriptDir, { recursive: true });
      const script = (name: string, steps: object) => {
        const path = join(scriptDir, `${name}.json`);
        return writeFile(path, JSON.stringify(steps)).then(() => path);
      };
      const workstation = {
        SOLODAWN_FAKE_CLAUDE_SCRIPT: await script('workstation-claude', claudeTaskScript()),
        SOLODAWN_FAKE_CODEX_SCRIPT: await script('workstation-codex', codexTaskScript()),
      };
      const coordinator = {
        SOLODAWN_FAKE_CLAUDE_SCRIPT: await script('coordinator-claude', secretaryM5Script()),
        SOLODAWN_FAKE_CODEX_SCRIPT: await script('coordinator-codex', codexReviewScript()),
      };
      const laptop = {
        SOLODAWN_FAKE_CLAUDE_SCRIPT: await script('laptop-claude', claudeReviewScript()),
      };

      const active = await startNet({
        nodes: 3,
        env: (index) => [workstation, coordinator, laptop][index],
      });
      net = active;
      const direct = await startDirectSurface(active.rendezvous, inject('browser'));
      surface = direct;
      const folder = (name: string) => makeFolder(active.dir, name);
      const folders = {
        checklist: await folder('release-checklist'),
        changelog: await folder('changelog'),
        secretary: await folder('release-notes'),
        reviewer: await folder('release-review'),
        checker: await folder('release-check'),
      };

      const context = await openDirectContext(browser, contexts, direct);
      const page = await context.newPage();

      const names = ['workstation', 'coordinator', 'laptop'];
      for (const [i, node] of active.nodes.entries()) await pairNode(page, direct, node, names[i]);

      // The group is created with its AIs, each in its own folder on its computer.
      await page.locator('button', { hasText: 'New group' }).first().click();
      await createGroup(page, {
        name: 'Release Ops',
        member: 'Alex',
        home: 'workstation',
        ais: [
          {
            cli: 'Claude Code',
            computer: 'workstation',
            name: 'Claude Code',
            folder: folders.checklist,
          },
          { cli: 'Codex', computer: 'workstation', name: 'Codex', folder: folders.changelog },
          {
            cli: 'Claude Code',
            computer: 'coordinator',
            name: 'Secretary',
            folder: folders.secretary,
            secretary: true,
          },
          { cli: 'Codex', computer: 'coordinator', name: 'Reviewer', folder: folders.reviewer },
          { cli: 'Claude Code', computer: 'laptop', name: 'Checker', folder: folders.checker },
        ],
      });
      const members = page.locator('#chat-side .members');
      await poll(() => members.locator('.member').count()).toBe(5);

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
      const summaryCards = page.locator('.line section.card', { hasText: 'Review score' });

      // T-1 is done at Reviewer's passed review; T-2, which waits for it, at Checker's.
      await poll(() => task1Card.textContent(), 30_000).toContain('Done · 96');
      await poll(() => task2Card.textContent(), 30_000).toContain('Done · 94');
      await poll(() => summaryCards.count(), 20_000).toBe(2);

      for (const [card, score, workspace] of [
        [task1Card, 96, folders.checklist],
        [task2Card, 94, folders.changelog],
      ] as const) {
        expect(await card.locator('.steps i').count()).toBe(3);
        await poll(() => card.textContent()).toContain(`Round 1 · ${score}, passed`);
        // Each task ran in its assignee's folder.
        expect(await card.locator('.kv code').textContent()).toBe(workspace);
      }

      // No approval cards appeared during A5.1, and no computer made a project folder of its own.
      expect(await page.locator('section.card[aria-label="Approval"]').count()).toBe(0);
      for (const node of active.nodes)
        expect(await readdir(node.dataDir)).not.toContain('projects');

      const summaryText = (await summaryCards.allTextContents()).join('\n');
      expect(summaryText).toContain('T-1');
      expect(summaryText).toContain('96');
      expect(summaryText).toContain('T-2');
      expect(summaryText).toContain('94');
      expect(summaryText).toContain('1 round');

      // Open T-1's thread fold: progress, the report, the review, task_done under the secretary's
      // name, and the summary jump link.
      await task1Card.locator('details.fold > summary').click();
      const thread1 = task1Card.locator('.thread');
      await poll(() => thread1.textContent()).toContain(
        'Drafted checklist rules and ran the project tests.',
      );
      await poll(() => thread1.textContent()).toContain(
        'Implemented release checklist verification.',
      );
      await poll(() => thread1.locator('.review').textContent()).toContain(
        'Checklist assertions cover edge cases.',
      );
      const done = thread1.locator('li.ti', { hasText: 'T-1 passed review and is done' });
      await poll(() => done.textContent()).toContain('Secretary');

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
          PATH: `${wrapperDir}${delimiter}${bin}${delimiter}${process.env.PATH ?? ''}`,
          SOLODAWN_FAKE_CLAUDE_SCRIPT: claudePath,
        }),
      });
      net = active;
      const direct = await startDirectSurface(active.rendezvous, inject('browser'));
      surface = direct;
      const [node] = active.nodes;
      const workspace = await makeFolder(active.dir, 'cards');

      const context = await openDirectContext(browser, contexts, direct);
      const page = await context.newPage();

      await pairNode(page, direct, node, 'workstation');

      await page.locator('button', { hasText: 'New group' }).first().click();
      await createGroup(page, { name: 'Card Roundtrip', member: 'Alex' });

      const members = page.locator('#chat-side .members');
      await members.locator('button', { hasText: 'Add AI' }).click();
      const addDlg = page.locator('dialog.dlg[open]');
      await poll(() => addDlg.locator('.choice').count()).toBe(3);
      await addDlg
        .locator('.choice', { hasText: 'Claude Code · workstation' })
        .locator('input')
        .check();
      await chooseFolder(addDlg, workspace);
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

  test(
    "answers an AI's question in its task's thread, pauses and cancels the task from its card, and the task that waited for its folder starts",
    { timeout: 90_000 },
    async () => {
      const scriptDir = join(inject('e2e').root, `m5-thread-${Date.now()}`);
      await mkdir(scriptDir, { recursive: true });
      const claudePath = join(scriptDir, 'claude.json');
      await writeFile(claudePath, JSON.stringify(directTaskScript()));

      const active = await startNet({
        nodes: 1,
        env: () => ({ SOLODAWN_FAKE_CLAUDE_SCRIPT: claudePath }),
      });
      net = active;
      const direct = await startDirectSurface(active.rendezvous, inject('browser'));
      surface = direct;
      const [node] = active.nodes;
      const workspace = await makeFolder(active.dir, 'thread');
      const context = await openDirectContext(browser, contexts, direct);
      const page = await context.newPage();
      await pairNode(page, direct, node, 'workstation');

      await page.locator('button', { hasText: 'New group' }).first().click();
      await createGroup(page, { name: 'Thread', member: 'Alex' });
      const members = page.locator('#chat-side .members');
      await members.locator('button', { hasText: 'Add AI' }).click();
      const addDlg = page.locator('dialog.dlg[open]');
      await poll(() => addDlg.locator('.choice').count()).toBe(3);
      await addDlg
        .locator('.choice', { hasText: 'Claude Code · workstation' })
        .locator('input')
        .check();
      await chooseFolder(addDlg, workspace);
      await addDlg.locator('button.btn-primary', { hasText: 'Add AI' }).click();
      await poll(() => members.locator('.member').count()).toBe(1);

      // The group session has read its script once it has ended a turn; the task's reads the next.
      await sendChatMessage(page, '@Claude Code are you there?');
      await poll(() => members.locator('.member').textContent(), 30_000).toContain('Waiting');
      await writeFile(claudePath, JSON.stringify(askingTaskScript()));
      await sendChatMessage(page, '@Claude Code let the notice be dismissed');

      const taskCard = (num: string) =>
        page
          .locator('.line section.card')
          .filter({ has: page.locator('.card-head .mono', { hasText: num }) });
      const card = taskCard('T-1');
      await poll(() => card.locator('.card-head').textContent(), 30_000).toContain('In progress');
      await card.locator('details.fold > summary').click();
      const thread = card.locator('.thread');
      await poll(() => thread.textContent(), 30_000).toContain('@Alex keep the field?');

      const reply = card.getByRole('combobox', { name: "Message in T-1's thread" });
      await reply.fill('@Claude Code yes, keep it.');
      await reply.press('Enter');
      await poll(() => reply.inputValue()).toBe('');
      await poll(() => thread.textContent(), 30_000).toContain('@Claude Code yes, keep it.');
      // The task's own session got the answer: it posts what it does with it.
      await poll(() => thread.textContent(), 30_000).toContain('Keeping the field.');
      expect(await page.locator('.line .msg-body > .text').allTextContents()).not.toContain(
        '@Claude Code yes, keep it.',
      );

      // A second direct request while T-1 holds the folder: T-2 waits for the folder (A5.20).
      await writeFile(claudePath, JSON.stringify(queuedTaskScript()));
      await sendChatMessage(page, '@Claude Code also let the notice fade out');
      const queued = taskCard('T-2');
      await poll(() => queued.locator('.card-head').textContent(), 30_000).toContain('Queued');
      await poll(() => queued.locator('.hold').textContent()).toContain(
        'Waits for its folder, which T-1 is using.',
      );

      const more = card.getByRole('button', { name: 'Actions on T-1' });
      const item = (name: string) => card.locator('.menu .item', { hasText: name });
      await more.click();
      await item('Pause').click();
      await poll(() => card.locator('.hold').textContent()).toContain('Alex paused the task.');
      await poll(() => card.locator('.card-head').textContent()).toContain('Paused');
      expect(await queued.locator('.card-head').textContent()).toContain('Queued');

      await more.click();
      await item('Cancel task').click();
      const question = card.locator('.ask');
      await poll(() => question.textContent()).toContain('Cancel T-1 for good?');
      expect(await card.locator('.card-head').textContent()).toContain('Paused');
      await question.getByRole('button', { name: 'Cancel task' }).click();
      await poll(() => card.locator('.card-head').textContent()).toContain('Cancelled');
      await poll(() => more.count()).toBe(0);

      // The folder is free: T-2 starts by itself, in a session of its own.
      await poll(() => queued.locator('.card-head').textContent(), 30_000).toContain('In progress');
      await queued.locator('details.fold > summary').click();
      await poll(() => queued.locator('.thread').textContent(), 30_000).toContain(
        'Fading the notice out.',
      );
    },
  );
});
