// End-to-end milestone 2 (E-M2): group creation across two browsers without reload, sub-second
// live message delivery, reconnect delta sync, and >1000-row cold reset with scroll-back and
// preserved pending card (F-06, F-07, A2.1, A2.2).
import { execFile } from 'node:child_process';
import { mkdir, writeFile } from 'node:fs/promises';
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
  const repo = join(dir, 'project-repo');
  await mkdir(repo, { recursive: true });
  await exec('git', ['init', '-b', 'main'], { cwd: repo });
  await exec('git', ['config', 'user.name', 'Alex'], { cwd: repo });
  await exec('git', ['config', 'user.email', 'alex@example.test'], { cwd: repo });
  await writeFile(join(repo, 'README.md'), '# Handbook\n');
  await exec('git', ['add', 'README.md'], { cwd: repo });
  await exec('git', ['commit', '-m', 'Initial commit'], { cwd: repo });
  return repo;
}

async function pairPrimaryBrowser(page: Page, net: Net, node: Node, label: string) {
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
    label,
    '--rendezvous',
    net.rendezvous.signal,
  );
  expect(paired.code, paired.stderr).toBe(0);
  await page.locator('input[name="acct-claude"][value="personal"]').check();
  await page.locator('input[name="acct-codex"][value="api_key"]').check();
  await page.locator('a.done-btn').click();
  await expect.poll(() => page.url()).toContain('#/groups');
  await expect.poll(() => page.locator('.bar a.conn').getAttribute('data-st')).toBe('ok');
}

async function installNetSwitch(context: BrowserContext) {
  await context.addInitScript(() => {
    const state = {
      offline: false,
      sockets: new Set<WebSocket>(),
      peers: new Set<RTCPeerConnection>(),
    };
    const NativeWS = window.WebSocket;
    const NativePC = window.RTCPeerConnection;

    class GuardedWS extends NativeWS {
      constructor(url: string | URL, protocols?: string | string[]) {
        super(url, protocols);
        state.sockets.add(this);
        this.addEventListener('close', () => state.sockets.delete(this));
        if (state.offline) queueMicrotask(() => this.close());
      }
    }

    class GuardedPC extends NativePC {
      constructor(config?: RTCConfiguration) {
        super(config);
        state.peers.add(this);
        this.addEventListener('connectionstatechange', () => {
          if (this.connectionState === 'closed') state.peers.delete(this);
        });
        if (state.offline) queueMicrotask(() => this.close());
      }
    }

    window.WebSocket = GuardedWS;
    window.RTCPeerConnection = GuardedPC;
    Object.assign(window, {
      __setPeerOffline(offline: boolean) {
        state.offline = offline;
        if (offline) {
          for (const pc of [...state.peers]) pc.close();
          for (const ws of [...state.sockets]) ws.close();
        }
      },
    });
  });
}

async function setPeerOffline(page: Page, offline: boolean) {
  await page.evaluate((flag) => {
    const win = window as unknown as { __setPeerOffline: (next: boolean) => void };
    win.__setPeerOffline(flag);
  }, offline);
}

async function pairSecondBrowser(firstPage: Page, secondPage: Page, net: Net) {
  await firstPage.goto(`${net.rendezvous.url}#/nodes`);
  await firstPage.locator('button.btn', { hasText: 'Add a browser' }).click();
  const dialog = firstPage.locator('dialog.dlg[open]');
  await dialog.locator('button.btn-primary', { hasText: 'Create pairing link' }).click();
  const rawLink = ((await dialog.locator('.pair-box pre.cmd').textContent()) ?? '').trim();
  expect(rawLink).toContain('#pair=');
  const hashIndex = rawLink.indexOf('#pair=');
  const link = `${net.rendezvous.url}${rawLink.slice(hashIndex)}`;
  await dialog.locator('button.btn-primary', { hasText: 'Done' }).click();

  await secondPage.goto(link);
  await expect.poll(() => secondPage.url()).toContain('#/onboarding');
  await expect
    .poll(() => secondPage.locator('section[aria-labelledby="st-h"] .badge').textContent())
    .toContain('Connected');
  await secondPage.locator('a.done-btn').click();
  await expect.poll(() => secondPage.url()).toContain('#/groups');
  await expect.poll(() => secondPage.locator('.bar a.conn').getAttribute('data-st')).toBe('ok');
}

function decisionSecretaryScript() {
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
            name: 'request_decision',
            arguments: {
              question: 'Keep legacy archive redirects?',
              options: [
                { label: 'Keep redirects', effect: 'Old links continue working' },
                { label: 'Drop redirects', effect: 'Simpler routing table' },
              ],
              recommended: 0,
              to: ['Alex'],
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
          result: 'asked',
          session_id: '$session',
        },
      },
      { dir: 'eof' },
    ],
  };
}

describe('E-M2: multi-browser group sync, delta catch-up, and cold scroll-back', () => {
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
    'syncs group creation, sub-second chat, 20-message delta, and 1500-message reset with scroll-back',
    { timeout: 120_000 },
    async () => {
      const scriptDir = join(inject('e2e').root, `m2-scripts-${Date.now()}`);
      await mkdir(scriptDir, { recursive: true });
      const claudeScript = join(scriptDir, 'claude.json');
      await writeFile(claudeScript, JSON.stringify(decisionSecretaryScript()));

      const active = await startNet({
        nodes: 1,
        env: () => ({ SOLODAWN_FAKE_CLAUDE_SCRIPT: claudeScript }),
      });
      net = active;
      const repo = await initGitRepo(active.dir);

      const ctx1 = await browser.newContext({ viewport: { width: 1440, height: 900 } });
      const ctx2 = await browser.newContext({ viewport: { width: 1440, height: 900 } });
      contexts.push(ctx1, ctx2);
      await installNetSwitch(ctx2);

      const page1 = await ctx1.newPage();
      const page2 = await ctx2.newPage();

      await pairPrimaryBrowser(page1, active, active.nodes[0], 'home-box');
      await pairSecondBrowser(page1, page2, active);

      await page1.goto(`${active.rendezvous.url}#/groups`);
      await page1.locator('button', { hasText: 'New project group' }).first().click();

      const createDlg = page1.locator('dialog.dlg[open]');
      const textInputs = createDlg.locator('input.input');
      await textInputs.nth(0).fill('Handbook');
      await textInputs.nth(1).fill(repo);
      await textInputs.nth(2).fill('main');
      await textInputs.nth(3).fill('Alex');
      await createDlg.locator('select.input').first().selectOption('none');
      await createDlg.locator('button.btn-primary', { hasText: 'Create group' }).click();

      await expect.poll(() => page1.url()).toMatch(/#\/groups\/[^/]+$/);
      const groupHash = new URL(page1.url()).hash;

      // Second connected browser sees the new group in its list without reloading.
      await expect
        .poll(() => page2.locator('.glist a.gitem b').allTextContents())
        .toContain('Handbook');

      await page2.locator('.glist a.gitem', { hasText: 'Handbook' }).click();
      await expect.poll(() => new URL(page2.url()).hash).toBe(groupHash);

      // Post a message from Browser 1 and verify Browser 2 renders it within 1 second (A2.1).
      const composer1 = page1.locator('.composer textarea');
      await composer1.fill('Hello live across browsers');
      const sentAt = performance.now();
      await page1.locator('.composer button.send').click();
      await expect
        .poll(() => page2.locator('.line .msg .text').allTextContents(), {
          timeout: 1000,
          interval: 15,
        })
        .toContain('Hello live across browsers');
      expect(performance.now() - sentAt).toBeLessThan(1000);

      // Add a secretary AI so an old pending card exists before the 1500-message gap.
      await page1.locator('.members button', { hasText: 'Add AI' }).click();
      const addDlg = page1.locator('dialog.dlg[open]');
      await expect.poll(() => addDlg.locator('.choice').count()).toBeGreaterThan(0);
      await addDlg.locator('.sec-opt input[type="checkbox"]').check();
      await addDlg.locator('button.btn-primary', { hasText: 'Add AI' }).click();
      await expect.poll(() => page1.locator('.members .member').count()).toBe(1);

      await composer1.fill('Please check archive redirects');
      await page1.locator('.composer button.send').click();
      await expect
        .poll(() => page2.locator('.needbar').textContent())
        .toContain('Keep legacy archive redirects?');

      // Disconnect Browser 2, post 20 messages, reconnect, and verify exact delta sync (A2.2).
      await setPeerOffline(page2, true);
      await expect
        .poll(() => page2.locator('.composer .state.down').textContent())
        .toContain('Cannot reach the group home');

      for (let i = 1; i <= 20; i++) {
        const label = `Delta message ${String(i).padStart(2, '0')}`;
        await composer1.fill(label);
        await page1.locator('.composer button.send').click();
        await expect.poll(() => composer1.inputValue()).toBe('');
      }

      await setPeerOffline(page2, false);
      await expect
        .poll(() => page2.locator('.line .msg .text').allTextContents())
        .toEqual(
          expect.arrayContaining([
            'Hello live across browsers',
            'Delta message 01',
            'Delta message 20',
          ]),
        );

      const textsAfterDelta = await page2.locator('.line .msg .text').allTextContents();
      const idxDelta01 = textsAfterDelta.indexOf('Delta message 01');
      const idxDelta20 = textsAfterDelta.indexOf('Delta message 20');
      expect(idxDelta01).toBeGreaterThan(-1);
      expect(idxDelta20 - idxDelta01).toBe(19);

      // Disconnect Browser 2 again, post 1500 messages (>1000 cap), and verify cold reset + scroll-back.
      await setPeerOffline(page2, true);
      await expect
        .poll(() => page2.locator('.composer .state.down').textContent())
        .toContain('Cannot reach the group home');

      await page1.evaluate(async () => {
        const ta = document.querySelector<HTMLTextAreaElement>('.composer textarea');
        const form = document.querySelector<HTMLFormElement>('.composer form');
        if (!ta || !form) throw new Error('composer not mounted');
        for (let i = 1; i <= 1500; i++) {
          ta.value = `History line ${String(i).padStart(4, '0')}`;
          ta.dispatchEvent(new Event('input', { bubbles: true }));
          form.requestSubmit();
          while (ta.value !== '') {
            await new Promise((resolve) => setTimeout(resolve, 1));
          }
        }
      });

      await setPeerOffline(page2, false);
      await expect
        .poll(() => page2.locator('.line .msg .text').allTextContents())
        .toContain('History line 1500');

      const resetTexts = await page2.locator('.line .msg .text').allTextContents();
      expect(resetTexts).toContain('History line 1451');
      expect(resetTexts).not.toContain('History line 1450');

      // The old pending decision card from before the 1500 messages remains visible after reset.
      await expect
        .poll(() => page2.locator('.needbar').textContent())
        .toContain('Keep legacy archive redirects?');
      await expect
        .poll(() =>
          page2
            .locator('.line .card')
            .allTextContents()
            .then((all) => all.join('\n')),
        )
        .toContain('Keep legacy archive redirects?');

      // Scrolling toward the top loads the previous 50-message page in chronological order.
      await page2.locator('.scroller').evaluate((el) => {
        el.scrollTop = -el.scrollHeight;
        el.dispatchEvent(new Event('scroll'));
      });
      await expect
        .poll(() => page2.locator('.line .msg .text').allTextContents())
        .toContain('History line 1450');

      const pagedTexts = await page2.locator('.line .msg .text').allTextContents();
      const idx1401 = pagedTexts.indexOf('History line 1401');
      const idx1450 = pagedTexts.indexOf('History line 1450');
      const idx1451 = pagedTexts.indexOf('History line 1451');
      const idx1500 = pagedTexts.indexOf('History line 1500');
      expect(idx1401).toBeGreaterThan(-1);
      expect(idx1401).toBeLessThan(idx1450);
      expect(idx1450).toBeLessThan(idx1451);
      expect(idx1451).toBeLessThan(idx1500);
    },
  );
});
