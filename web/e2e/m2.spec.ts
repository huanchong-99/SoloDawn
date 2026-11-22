// End-to-end milestone 2 (E-M2): group creation across two browsers without reload, sub-second
// live message delivery, reconnect delta sync, and >1000-row cold reset with scroll-back and
// preserved pending card (F-06, F-07, A2.1, A2.2).
import { mkdir, writeFile } from 'node:fs/promises';
import { join } from 'node:path';
import type { Browser, BrowserContext, Page } from 'playwright';
import { afterAll, afterEach, beforeAll, describe, expect, inject, test } from 'vitest';
import { launch } from './config';
import {
  claudeInitSteps,
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

async function pairPrimaryBrowser(page: Page, surface: DirectSurface, node: Node, label: string) {
  await page.goto(`${surface.url}#/onboarding`);
  const code = await readPairingCode(page, surface.signal);
  const paired = await node.run(
    'solodawn',
    'pair',
    code,
    '--name',
    label,
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

// E-M2 approximates A2.6 in the browser by cutting Browser 2's WebSocket and WebRTC transports;
// full group-home stop and restart is covered in features/m2.feature.
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
        if (!offline) {
          window.dispatchEvent(new Event('online'));
          return;
        }
        for (const pc of [...state.peers]) pc.close();
        for (const ws of [...state.sockets]) ws.close();
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

async function installBulkPoster(context: BrowserContext) {
  await context.addInitScript(() => {
    const orig = RTCPeerConnection.prototype.createDataChannel;
    RTCPeerConnection.prototype.createDataChannel = function (
      this: RTCPeerConnection,
      label: string,
      options?: RTCDataChannelInit,
    ) {
      const channel = orig.call(this, label, options);
      if (label === 'ctl') {
        channel.addEventListener('open', () => {
          Object.assign(window, { __activeCtl: channel });
        });
      }
      return channel;
    };
  });
}

async function postBulkHistory(page: Page, groupId: string, count: number) {
  await page.evaluate(
    async ({ g, total }) => {
      type Reply = { re?: number; ok?: boolean; err?: { code?: string; msg?: string } };

      const b64url = (bytes: Uint8Array) =>
        btoa(Array.from(bytes, (b) => String.fromCharCode(b)).join(''))
          .replaceAll('+', '-')
          .replaceAll('/', '_')
          .replaceAll('=', '');

      const readDeviceKeys = () =>
        new Promise<CryptoKeyPair>((resolve, reject) => {
          const openReq = indexedDB.open('solodawn', 1);
          openReq.onerror = () => reject(new Error('failed to open solodawn IndexedDB'));
          openReq.onsuccess = () => {
            const db = openReq.result;
            const getReq = db.transaction('device', 'readonly').objectStore('device').get('key');
            getReq.onerror = () => {
              db.close();
              reject(new Error('failed to read device key from IndexedDB'));
            };
            getReq.onsuccess = () => {
              db.close();
              resolve(getReq.result as CryptoKeyPair);
            };
          };
        });

      const enc = new TextEncoder();
      const signPost = async (privateKey: CryptoKey, k: string, index: number, ts: number) => {
        const n = b64url(crypto.getRandomValues(new Uint8Array(16)));
        const p = JSON.stringify({
          t: 'post',
          n,
          ts,
          g,
          kind: 'text',
          thread: null,
          main: false,
          text: `History line ${String(index).padStart(4, '0')}`,
          mentions: [],
          re: null,
          body: {},
        });
        const sig = await crypto.subtle.sign(
          { name: 'Ed25519' },
          privateKey,
          enc.encode(`solodawn/1\n${p}`),
        );
        const id = 1_000_000 + index;
        const env = { p, k, s: b64url(new Uint8Array(sig)) };
        return { id, raw: JSON.stringify({ t: 'cmd', id, env }) };
      };

      const parseReply = (data: unknown): Reply => {
        if (typeof data !== 'string') return {};
        try {
          return JSON.parse(data) as Reply;
        } catch {
          return {};
        }
      };

      const replyError = (msg: Reply): string | null => {
        if (msg.ok === true && !msg.err) return null;
        return msg.err ? `${msg.err.code}: ${msg.err.msg}` : 'bulk post command failed';
      };

      const sendBatch = async (ctl: RTCDataChannel, frames: Array<{ id: number; raw: string }>) => {
        const pending = new Set(frames.map((f) => f.id));
        let onMsg = (_ev: MessageEvent) => {};
        let onClose = () => {};
        const done = new Promise<void>((resolve, reject) => {
          onClose = () => reject(new Error('ctl channel closed during bulk post'));
          onMsg = (ev: MessageEvent) => {
            const msg = parseReply(ev.data);
            if (typeof msg.re !== 'number' || !pending.delete(msg.re)) return;
            const err = replyError(msg);
            if (err !== null) reject(new Error(err));
            else if (pending.size === 0) resolve();
          };
        });
        ctl.addEventListener('message', onMsg);
        ctl.addEventListener('close', onClose);
        try {
          for (const frame of frames) ctl.send(frame.raw);
          await done;
        } finally {
          ctl.removeEventListener('message', onMsg);
          ctl.removeEventListener('close', onClose);
        }
      };

      const ctl = (window as unknown as { __activeCtl?: RTCDataChannel }).__activeCtl;
      if (!ctl || ctl.readyState !== 'open') throw new Error('ctl channel not open');
      const pair = await readDeviceKeys();
      const k = b64url(new Uint8Array(await crypto.subtle.exportKey('raw', pair.publicKey)));
      const batchSize = 50;
      for (let start = 1; start <= total; start += batchSize) {
        const len = Math.min(batchSize, total - start + 1);
        const ts = Date.now();
        const indices = Array.from({ length: len }, (_, idx) => start + idx);
        const frames = await Promise.all(indices.map((i) => signPost(pair.privateKey, k, i, ts)));
        await sendBatch(ctl, frames);
      }
    },
    { g: groupId, total: count },
  );
}

async function pairSecondBrowser(firstPage: Page, secondPage: Page, surface: DirectSurface) {
  await firstPage.locator('.rail a[href="#/nodes"]').click();
  await poll(() => firstPage.url()).toContain('#/nodes');
  await poll(() => firstPage.locator('button.btn', { hasText: 'Add a browser' }).isDisabled()).toBe(
    false,
  );
  await firstPage.locator('button.btn', { hasText: 'Add a browser' }).click();
  const dialog = firstPage.locator('dialog.dlg[open]');
  await dialog.locator('button.btn-primary', { hasText: 'Create pairing link' }).click();
  await poll(() => dialog.locator('.pair-box pre.cmd').count()).toBe(1);
  const rawLink = ((await dialog.locator('.pair-box pre.cmd').textContent()) ?? '').trim();
  expect(rawLink).toContain('#pair=');
  const link = `${surface.url}${rawLink.slice(rawLink.indexOf('#pair='))}`;
  await dialog.locator('button.btn-primary', { hasText: 'Done' }).click();

  await secondPage.goto(link);
  await poll(() => secondPage.url()).toContain('#/onboarding');
  await poll(() =>
    secondPage.locator('section[aria-labelledby="st-h"] .badge').textContent(),
  ).toContain('Connected');
  await poll(() => secondPage.locator('a.done-btn').count()).toBe(1);
  await secondPage.locator('a.done-btn').click();
  await poll(() => secondPage.url()).toContain('#/groups');
  await poll(() => secondPage.locator('.bar a.conn').getAttribute('data-st')).toBe('ok');
}

function decisionSecretaryScript() {
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

function assertOrderedDelta(textsAfterDelta: string[], deltaLabels: string[]) {
  const idxDelta01 = textsAfterDelta.indexOf(deltaLabels[0]);
  expect(idxDelta01).toBeGreaterThan(-1);
  expect(textsAfterDelta.slice(idxDelta01, idxDelta01 + deltaLabels.length)).toEqual(deltaLabels);
  for (const label of deltaLabels) {
    expect(textsAfterDelta.filter((item) => item === label)).toHaveLength(1);
  }
}

describe('E-M2: multi-browser group sync, delta catch-up, and cold scroll-back', () => {
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
    'syncs group creation, sub-second chat, 20-message delta, and 1500-message reset with scroll-back',
    { timeout: 180_000 },
    async () => {
      const scriptDir = join(inject('e2e').root, `m2-scripts-${Date.now()}`);
      await mkdir(scriptDir, { recursive: true });
      const claudeScript = join(scriptDir, 'claude.json');
      await writeFile(claudeScript, JSON.stringify(decisionSecretaryScript()));

      const active = await startNet({
        nodes: 1,
        env: () => ({ ...gitIdentityEnv, SOLODAWN_FAKE_CLAUDE_SCRIPT: claudeScript }),
      });
      net = active;
      const direct = await startDirectSurface(active.rendezvous, inject('browser'));
      surface = direct;
      const repo = await initGitRepo(active.dir, 'project-repo', '# Handbook\n');

      const ctx1 = await openDirectContext(browser, contexts, direct);
      const ctx2 = await openDirectContext(browser, contexts, direct);
      await installBulkPoster(ctx1);
      await installNetSwitch(ctx2);

      const page1 = await ctx1.newPage();
      const page2 = await ctx2.newPage();

      await pairPrimaryBrowser(page1, direct, active.nodes[0], 'home-box');
      await pairSecondBrowser(page1, page2, direct);

      await page1.locator('.rail .tabs a[href="#/groups"]').click();
      await poll(() => page1.url()).toContain('#/groups');
      await page1.locator('button', { hasText: 'New project group' }).first().click();

      const createDlg = page1.locator('dialog.dlg[open]');
      const textInputs = createDlg.locator('input.input');
      await textInputs.nth(0).fill('Handbook');
      await textInputs.nth(1).fill(repo);
      await textInputs.nth(2).fill('main');
      await textInputs.nth(3).fill('Alex');
      await createDlg.locator('select.input').first().selectOption('none');
      await createDlg.locator('button.btn-primary', { hasText: 'Create group' }).click();

      await poll(() => page1.url()).toMatch(/#\/groups\/[^/]+$/);
      const groupHash = new URL(page1.url()).hash;
      const groupId = groupHash.slice('#/groups/'.length);

      // Second connected browser sees the new group in its list without reloading.
      await poll(() => page2.locator('.glist a.gitem b').allTextContents()).toContain('Handbook');

      await page2.locator('.glist a.gitem', { hasText: 'Handbook' }).click();
      await poll(() => new URL(page2.url()).hash).toBe(groupHash);

      // Post a message from Browser 1 and verify Browser 2 renders it within 1 second (A2.1).
      await poll(() => page1.locator('dialog.dlg[open]').count()).toBe(0);
      const composer1 = page1.locator('.composer textarea');
      await composer1.fill('Hello live across browsers');
      await poll(() => page1.locator('.composer button.send').isDisabled()).toBe(false);
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
      await poll(() => addDlg.locator('.choice').count()).toBeGreaterThan(0);
      await addDlg.locator('.sec-opt input[type="checkbox"]').check();
      await addDlg.locator('button.btn-primary', { hasText: 'Add AI' }).click();
      await poll(() => addDlg.count()).toBe(0);
      await poll(() => page1.locator('.members .member').count()).toBe(1);

      await sendChatMessage(page1, 'Please check archive redirects');
      await poll(() => page2.locator('.chat nav.needbar').textContent()).toContain(
        'Keep legacy archive redirects?',
      );

      // Cut Browser 2's transports (browser-side approximation of A2.6), post 20 messages,
      // reconnect, and verify exact ordered delta sync with each message appearing once (A2.2).
      await setPeerOffline(page2, true);
      await poll(() => page2.locator('.composer .state.down').textContent()).toContain(
        'Cannot reach the group home',
      );

      const deltaLabels = Array.from(
        { length: 20 },
        (_, idx) => `Delta message ${String(idx + 1).padStart(2, '0')}`,
      );
      for (const label of deltaLabels) {
        await sendChatMessage(page1, label);
      }

      await setPeerOffline(page2, false);
      await poll(() => page2.locator('.line .msg .text').allTextContents()).toEqual(
        expect.arrayContaining(['Hello live across browsers', ...deltaLabels]),
      );

      const textsAfterDelta = await page2.locator('.line .msg .text').allTextContents();
      assertOrderedDelta(textsAfterDelta, deltaLabels);

      // Disconnect Browser 2 again, post 1500 messages (>1000 cap), and verify cold reset + scroll-back.
      await setPeerOffline(page2, true);
      await poll(() => page2.locator('.composer .state.down').textContent()).toContain(
        'Cannot reach the group home',
      );

      await page1.locator('.rail a[href="#/settings"]').click();
      await poll(() => page1.url()).toContain('#/settings');
      await postBulkHistory(page1, groupId, 1500);

      await setPeerOffline(page2, false);
      await poll(() => page2.locator('.line .msg .text').allTextContents(), 30_000).toContain(
        'History line 1500',
      );

      const resetTexts = await page2.locator('.line .msg .text').allTextContents();
      expect(resetTexts).toContain('History line 1451');
      expect(resetTexts).not.toContain('History line 0001');
      expect(resetTexts).not.toContain('Delta message 20');
      const firstHistory = resetTexts.find((item) => item.startsWith('History line ')) ?? '';
      const oldestLoaded = Number(firstHistory.slice('History line '.length));
      expect(oldestLoaded).toBeGreaterThan(50);

      // The old pending decision card from before the 1500 messages remains visible after reset.
      await poll(() => page2.locator('.chat nav.needbar').textContent()).toContain(
        'Keep legacy archive redirects?',
      );
      await poll(() =>
        page2
          .locator('.line .card')
          .allTextContents()
          .then((all) => all.join('\n')),
      ).toContain('Keep legacy archive redirects?');

      // Scrolling toward the top loads the previous 50-message page in chronological order.
      const prevPageEnd = `History line ${String(oldestLoaded - 1).padStart(4, '0')}`;
      const prevPageStart = `History line ${String(oldestLoaded - 50).padStart(4, '0')}`;
      await page2.locator('.edge').evaluate((el) => el.scrollIntoView({ block: 'start' }));
      await poll(() => page2.locator('.line .msg .text').allTextContents()).toContain(prevPageEnd);

      const pagedTexts = await page2.locator('.line .msg .text').allTextContents();
      const idxPrevStart = pagedTexts.indexOf(prevPageStart);
      const idxPrevEnd = pagedTexts.indexOf(prevPageEnd);
      const idxOldestBefore = pagedTexts.indexOf(firstHistory);
      const idx1500 = pagedTexts.indexOf('History line 1500');
      expect(idxPrevStart).toBeGreaterThan(-1);
      expect(idxPrevStart).toBeLessThan(idxPrevEnd);
      expect(idxPrevEnd).toBeLessThan(idxOldestBefore);
      expect(idxOldestBefore).toBeLessThan(idx1500);
    },
  );
});
