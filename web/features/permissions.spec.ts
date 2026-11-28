import 'fake-indexeddb/auto';
import { describeFeature, loadFeature } from '@amiceli/vitest-cucumber';
import { IDBFactory } from 'fake-indexeddb';
import { chromium } from 'playwright';
import { flushSync, tick } from 'svelte';
import { SvelteMap } from 'svelte/reactivity';
import { createServer } from 'vite';
import { afterAll, afterEach, beforeAll, describe, expect, it, vi } from 'vitest';
import { find, Home, homes } from '../src/lib/groups.svelte';
import { Device, ed25519, encode } from '../src/lib/identity';
import type { Path, Source } from '../src/lib/net.svelte';
import { nodes } from '../src/lib/nodes.svelte';
import { mac, parseSecret, room } from '../src/lib/pairing';
import { announcement, prefs, t } from '../src/lib/prefs.svelte';
import * as p from '../src/lib/proto';
import { ownedNode, type OwnedNode } from './home';
import { all, closePage, fill, go, one, openPage, read, texts } from './page';
import { FakePC } from './peer-fakes';

const feature = await loadFeature('../features/web_permissions.feature');

const device = 'J3Uuh-y1z2AX536yAFlGHJL5sjxVpWVJavvVphvdm5Q';
const winDevice = 'W3Uuh-y1z2AX536yAFlGHJL5sjxVpWVJavvVphvdm5Q';
const linuxDevice = 'L3Uuh-y1z2AX536yAFlGHJL5sjxVpWVJavvVphvdm5Q';
const website = 'rZNvy-1jH6Z-BcPqA5U5BQ';
const paths = new SvelteMap<Home, Path | undefined>();
let sourceOverride: Source | undefined;

vi.mock('../src/lib/net.svelte', async (actual) => {
  const real = await actual<typeof import('../src/lib/net.svelte')>();
  return {
    ...real,
    start: (source: Source) => ({
      net: {
        source: sourceOverride ?? source,
        homes,
        path: (home: Home) => (home.ctl ? paths.get(home) : undefined),
      },
      stop: () => {},
    }),
  };
});

type PermitReq = { t: string; open?: string; lang?: string; path?: string };
type PermitHandler = (req: PermitReq) => Promise<p.CmdResult | p.ErrorResponse>;

function attachPermit(node: OwnedNode, sent: PermitReq[], handler: () => PermitHandler) {
  const ctl = node.store.ctl!;
  const orig = ctl.request.bind(ctl);
  ctl.request = ((req: PermitReq) => {
    if (req.t === 'host.permit') {
      sent.push(req);
      return handler()(req);
    }
    return orig(req as never);
  }) as p.Ctl['request'];
}

function cardFor(name: string): HTMLElement {
  return all<HTMLElement>('.card.node').find(
    (card) => card.querySelector('h3')?.textContent?.trim() === name,
  )!;
}

function rowFor(card: HTMLElement, title: string): HTMLElement {
  return [...card.querySelectorAll<HTMLElement>('.perm-rows > li')].find(
    (li) => li.querySelector('b')?.textContent?.trim() === title,
  )!;
}

function buttonIn(root: ParentNode, textMatch: string): HTMLButtonElement {
  return [...root.querySelectorAll<HTMLButtonElement>('button')].find((b) =>
    b.textContent?.includes(textMatch),
  )!;
}

describeFeature(feature, ({ Background, Scenario, AfterEachScenario }) => {
  let workstation: OwnedNode;
  let permitCalls: PermitReq[];
  let permitHandler: PermitHandler;
  let pendingPermit:
    { resolve: (res: p.CmdResult | p.ErrorResponse) => void; path?: string } | undefined;
  let platformBackup: PropertyDescriptor | undefined;
  let uaBackup: PropertyDescriptor | undefined;
  let uadBackup: PropertyDescriptor | undefined;

  async function mountMac(perms: NonNullable<p.HostInfo['permissions']>) {
    workstation = ownedNode('workstation', device, {
      os: 'darwin/arm64',
      permissions: perms,
    });
    workstation.home.add(website, 'Website');
    homes.push(workstation.store);
    await workstation.connect();
    await vi.waitFor(() => expect(find(website)?.info).toBeDefined());
    attachPermit(workstation, permitCalls, () => permitHandler);
    paths.set(workstation.store, { kind: 'direct', rtt: 12, since: Date.now() });
  }

  async function openNodesWithRows(name = 'workstation') {
    openPage('#/nodes');
    await vi.waitFor(() => {
      flushSync();
      expect(cardFor(name)?.querySelectorAll('.perm-rows > li')).toHaveLength(3);
    });
  }

  function stubBrowser(uaDataPlatform: string | undefined, platform: string, userAgent: string) {
    if (!platformBackup) {
      platformBackup = Object.getOwnPropertyDescriptor(Navigator.prototype, 'platform');
    }
    if (!uaBackup) {
      uaBackup = Object.getOwnPropertyDescriptor(Navigator.prototype, 'userAgent');
    }
    if (!uadBackup) {
      uadBackup = Object.getOwnPropertyDescriptor(Navigator.prototype, 'userAgentData');
    }
    Object.defineProperty(navigator, 'userAgentData', {
      configurable: true,
      value: uaDataPlatform === undefined ? undefined : { platform: uaDataPlatform },
    });
    Object.defineProperty(navigator, 'platform', { configurable: true, value: platform });
    Object.defineProperty(navigator, 'userAgent', { configurable: true, value: userAgent });
  }

  function restoreBrowser() {
    for (const [key, desc] of [
      ['platform', platformBackup],
      ['userAgent', uaBackup],
      ['userAgentData', uadBackup],
    ] as const) {
      if (desc) Object.defineProperty(navigator, key, desc);
      else if (key === 'userAgentData') Reflect.deleteProperty(navigator, key);
    }
  }

  AfterEachScenario(async () => {
    vi.useRealTimers();
    restoreBrowser();
    await closePage();
    paths.clear();
    nodes.infos.clear();
    prefs.lang = 'en';
    announcement.text = '';
    pendingPermit = undefined;
  });

  Background(({ Given }) => {
    Given('the owner\'s Mac "workstation" whose node reports permissions', async () => {
      permitCalls = [];
      permitHandler = async () => ({ re: 1, ok: true });
      await mountMac({ local_network: false, incoming: true, full_disk: false });
    });
  });

  Scenario('The card shows what macOS allows (WP.1)', ({ Given, Then, And }) => {
    Given('local network is not allowed, incoming is allowed and full disk is off', () =>
      openNodesWithRows(),
    );

    Then('the Computers page shows "macOS permissions" open, with "Needs you"', () => {
      const card = cardFor('workstation');
      expect(card.querySelector('.perm-head h3')?.textContent?.trim()).toBe('macOS permissions');
      expect(card.querySelector('.perm-head .badge')?.textContent?.trim()).toBe('Needs you');
      expect(card.querySelectorAll('.perm-rows > li')).toHaveLength(3);
    });

    And('"Local network" says "Not allowed yet" and offers "Allow"', () => {
      const row = rowFor(cardFor('workstation'), 'Local network');
      expect(row.textContent).toContain('Not allowed yet');
      expect(buttonIn(row, 'Allow')).toBeDefined();
    });

    And('"Incoming connections" has a tick', () => {
      const row = rowFor(cardFor('workstation'), 'Incoming connections');
      expect(row.querySelector('.mark.ok')).not.toBeNull();
    });

    And('"Files" offers "Allow all folders…"', () => {
      const row = rowFor(cardFor('workstation'), 'Files');
      expect(buttonIn(row, 'Allow all folders…')).toBeDefined();
    });

    And('exactly one host.permit without fields was sent when the card appeared', () => {
      expect(permitCalls).toEqual([{ t: 'host.permit' }]);
    });
  });

  Scenario('Asking, and the tick when macOS allows (WP.2)', ({ When, Then, And }) => {
    When('the owner chooses "Allow"', async () => {
      await openNodesWithRows();
      buttonIn(rowFor(cardFor('workstation'), 'Local network'), 'Allow').click();
      flushSync();
    });

    Then('one more host.permit without fields is sent', () => {
      expect(permitCalls).toEqual([{ t: 'host.permit' }, { t: 'host.permit' }]);
    });

    And('the coach says to look at the screen of "workstation" and click Allow', () => {
      const row = rowFor(cardFor('workstation'), 'Local network');
      expect(row.querySelector('.coach [role="status"]')?.textContent).toContain(
        'Look at the screen of “workstation”',
      );
      expect(row.querySelector('.coach [role="status"]')?.textContent).toContain('Click Allow');
    });

    When('host.info arrives with local network allowed', () => {
      const next: p.HostInfoPush = {
        ...nodes.infos.get(workstation.store)!,
        permissions: { local_network: true, incoming: true, full_disk: false },
      };
      nodes.receive(workstation.store, next);
      flushSync();
    });

    Then('"Local network" has a tick, the coach is gone and the badge says "Ready"', () => {
      const card = cardFor('workstation');
      const row = rowFor(card, 'Local network');
      expect(row.querySelector('.mark.ok')).not.toBeNull();
      expect(row.querySelector('.coach')).toBeNull();
      expect(card.querySelector('.perm-head .badge')?.textContent?.trim()).toBe('Ready');
      expect(card.querySelector('.perm-head h3')?.textContent?.trim()).toBe('macOS permissions');
      expect(card.querySelectorAll('.perm-rows > li')).toHaveLength(3);
    });

    And('"Allowed: Local network" is announced', () => {
      expect(announcement.text).toBe('Allowed: Local network');
    });
  });

  Scenario('No window, so System Settings (WP.3)', ({ Given, When, Then, And }) => {
    Given('the coach of "Local network" is open', async () => {
      await openNodesWithRows();
      buttonIn(rowFor(cardFor('workstation'), 'Local network'), 'Allow').click();
      flushSync();
    });

    When('the owner chooses "No window? Open System Settings"', () => {
      buttonIn(
        rowFor(cardFor('workstation'), 'Local network'),
        'No window? Open System Settings',
      ).click();
      flushSync();
    });

    Then('host.permit is sent with open "local_network"', () => {
      expect(permitCalls.at(-1)).toEqual({ t: 'host.permit', open: 'local_network' });
    });

    And('the coach says to scroll down to Local Network and switch SoloDawn on', () => {
      const text = rowFor(cardFor('workstation'), 'Local network').querySelector(
        '.coach [role="status"]',
      )?.textContent;
      expect(text).toContain('Scroll down to Local Network and switch SoloDawn on');
    });
  });

  Scenario('A firewall that blocks (WP.4)', ({ Given, When, Then, And }) => {
    Given('incoming is not allowed', async () => {
      workstation.info.permissions = { local_network: true, incoming: false, full_disk: false };
      await openNodesWithRows();
    });

    When('the owner chooses "Open System Settings" in "Incoming connections"', () => {
      buttonIn(
        rowFor(cardFor('workstation'), 'Incoming connections'),
        'Open System Settings',
      ).click();
      flushSync();
    });

    Then('host.permit is sent with open "incoming"', () => {
      expect(permitCalls.at(-1)).toEqual({ t: 'host.permit', open: 'incoming' });
    });

    And('the coach names Options… and "Allow incoming connections"', () => {
      const text = rowFor(cardFor('workstation'), 'Incoming connections').querySelector(
        '.coach [role="status"]',
      )?.textContent;
      expect(text).toContain('Options…');
      expect(text).toContain('Allow incoming connections');
    });
  });

  Scenario('Full Disk Access by the drag panel (WP.5)', ({ When, Then, And }) => {
    When('the owner chooses "Allow all folders…"', async () => {
      await openNodesWithRows();
      buttonIn(rowFor(cardFor('workstation'), 'Files'), 'Allow all folders…').click();
      flushSync();
    });

    Then('host.permit is sent with open "full_disk" and the page\'s language', () => {
      expect(permitCalls.at(-1)).toEqual({ t: 'host.permit', open: 'full_disk', lang: 'en' });
    });

    And('the coach names the panel under System Settings and the + button', () => {
      const coach = rowFor(cardFor('workstation'), 'Files').querySelector('.coach')!;
      expect(coach.textContent).toContain('a small panel sits under System Settings');
      expect(coach.textContent).toContain('Click + under the list');
    });

    When('host.info arrives with full disk on', () => {
      nodes.receive(workstation.store, {
        ...nodes.infos.get(workstation.store)!,
        permissions: { local_network: true, incoming: true, full_disk: true },
      });
      flushSync();
    });

    Then('"Files" has a tick and says Full Disk Access is on', () => {
      const row = rowFor(cardFor('workstation'), 'Files');
      expect(row.querySelector('.mark.ok')).not.toBeNull();
      expect(row.textContent).toContain('Full Disk Access is on');
    });
  });

  Scenario('A folder macOS must allow (WP.6)', ({ Given, When, Then, And }) => {
    let expectedFolder = '';

    async function useFolder(path: string) {
      expectedFolder = path;
      const input = one<HTMLInputElement>('dialog dialog input.input');
      fill(input, path);
      flushSync();
      one<HTMLButtonElement>('dialog dialog footer button.btn-primary').click();
      flushSync();
    }

    Given('the folder dialog of Add AI for Claude Code on "workstation"', async () => {
      permitHandler = (req) =>
        new Promise((resolve) => {
          pendingPermit = { resolve, path: req.path };
        });
      openPage(`#/groups/${website}`);
      await vi.waitFor(() => expect(find(website)?.info).toBeDefined());
      one<HTMLButtonElement>('.members .head button').click();
      await vi.waitFor(() => expect(all('dialog .choice').length).toBeGreaterThan(0));
      one<HTMLButtonElement>('dialog .folder .change').click();
      await tick();
      flushSync();
      await vi.waitFor(() => expect(one('dialog dialog input.input')).not.toBeNull());
      vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout'] });
    });

    When(
      'the owner uses "/Users/me/Documents/website" and the node has not answered after 500 ms',
      async () => {
        await useFolder('/Users/me/Documents/website');
        expect(pendingPermit?.path).toBe('/Users/me/Documents/website');
        vi.advanceTimersByTime(500);
        flushSync();
      },
    );

    Then('the dialog says macOS is asking on "workstation"', () => {
      expect(one('dialog dialog .gate')?.textContent).toContain(
        'Look at the screen of “workstation”: macOS is asking whether SoloDawn may use this folder',
      );
    });

    When('the node answers ok', async () => {
      pendingPermit!.resolve({ re: 1, ok: true });
      await tick();
      flushSync();
    });

    Then('the dialog closes with that folder', () => {
      expect(document.querySelector('dialog dialog')).toBeNull();
      expect(read(one('dialog .path-row code'))).toBe('/Users/me/Documents/website');
    });

    When('the owner uses "/Users/me/Downloads/x" and the node answers "permission"', async () => {
      one<HTMLButtonElement>('dialog .folder .change').click();
      await tick();
      flushSync();
      await useFolder('/Users/me/Downloads/x');
      pendingPermit!.resolve({ re: 2, err: { code: 'permission', msg: 'denied' } });
      await tick();
      flushSync();
    });

    Then(
      'the dialog stays open and offers "Open System Settings", "Allow all folders…" and "Try again"',
      () => {
        const dlg = one('dialog dialog');
        const gateEl = dlg.querySelector('.gate.no')!;
        expect(texts('dialog dialog .gate.no button')).toEqual([
          'Open System Settings',
          'Allow all folders…',
          'Try again',
        ]);
        expect(gateEl.textContent).toContain('macOS has not allowed SoloDawn to use this folder');

        buttonIn(gateEl, 'Open System Settings').click();
        flushSync();
        expect(permitCalls.at(-1)).toEqual({ t: 'host.permit', open: 'folders' });
        expect(gateEl.textContent).toContain('Files and Folders is open on “workstation”');

        buttonIn(gateEl, 'Allow all folders…').click();
        flushSync();
        expect(permitCalls.at(-1)).toEqual({ t: 'host.permit', open: 'full_disk', lang: 'en' });
      },
    );

    When('the owner chooses "Try again" and the node answers ok', async () => {
      buttonIn(one('dialog dialog .gate.no'), 'Try again').click();
      flushSync();
      expect(pendingPermit?.path).toBe('/Users/me/Downloads/x');
      pendingPermit!.resolve({ re: 3, ok: true });
      await tick();
      flushSync();
    });

    And('the dialog closes with that folder', () => {
      expect(document.querySelector('dialog dialog')).toBeNull();
      expect(read(one('dialog .path-row code'))).toBe(expectedFolder);
    });
  });

  Scenario('Other systems (WP.7)', ({ Given, Then }) => {
    let linuxSent: PermitReq[] = [];

    Given('the owner\'s computer "tower" on "linux/amd64" without permissions', async () => {
      const tower = ownedNode('tower', linuxDevice, { os: 'linux/amd64' });
      tower.home.add(website, 'Website');
      homes.push(tower.store);
      await tower.connect();
      linuxSent = [];
      attachPermit(tower, linuxSent, () => async () => ({ re: 1, ok: true }));
      paths.set(tower.store, { kind: 'direct', rtt: 10, since: Date.now() });
    });

    Then('its card shows no permissions and its folder dialog sends no host.permit', async () => {
      openPage('#/nodes');
      await vi.waitFor(() => expect(cardFor('tower')).toBeDefined());
      const card = cardFor('tower');
      expect(card.querySelector('.perm-plain, .perm-card, .win-relay')).toBeNull();
      expect(linuxSent).toEqual([]);

      go(`#/groups/${website}`);
      await vi.waitFor(() => {
        flushSync();
        expect(find(website)?.info).toBeDefined();
        expect(one('.members .head button')).toBeDefined();
      });
      one<HTMLButtonElement>('.members .head button').click();
      await vi.waitFor(() =>
        expect(all<HTMLInputElement>('dialog input[name="candidate"]').length).toBeGreaterThan(1),
      );
      const towerChoice = all<HTMLLabelElement>('dialog label.choice').find((c) =>
        c.textContent?.includes('tower'),
      )!;
      towerChoice.querySelector<HTMLInputElement>('input')!.click();
      flushSync();
      one<HTMLButtonElement>('dialog .folder .change').click();
      await tick();
      flushSync();
      await vi.waitFor(() => expect(one('dialog dialog input.input')).not.toBeNull());
      fill(one<HTMLInputElement>('dialog dialog input.input'), '/home/me/app');
      flushSync();
      one<HTMLButtonElement>('dialog dialog footer button.btn-primary').click();
      flushSync();
      expect(document.querySelector('dialog dialog')).toBeNull();
      expect(linuxSent).toEqual([]);
    });

    Given('the owner\'s computer "desk" on "windows/amd64" on a relayed path', async () => {
      await closePage();
      const desk = ownedNode('desk', winDevice, { os: 'windows/amd64' });
      homes.push(desk.store);
      await desk.connect();
      paths.set(desk.store, { kind: 'relay', rtt: 140, since: Date.now() });
      openPage('#/nodes');
      await vi.waitFor(() => {
        flushSync();
        expect(cardFor('desk')).toBeDefined();
      });
    });

    Then('its card names Windows Security › Firewall & network protection', () => {
      const card = cardFor('desk');
      expect(card.textContent).toContain('Windows Security › Firewall & network protection');
    });
  });

  Scenario("Before pairing, by the browser's system (WP.8)", ({ Then, And }) => {
    Then('the onboarding page of a Mac browser lists the three things macOS shows', async () => {
      stubBrowser('macOS', 'MacIntel', 'Mozilla/5.0 (Macintosh; Intel Mac OS X 14_5)');
      openPage('#/onboarding');
      await tick();
      flushSync();
      const note = one('.sys-asks');
      expect(texts('.sys-asks li')).toHaveLength(3);
      expect(note.textContent).toContain('Background Items Added');
      expect(note.textContent).not.toContain('Windows Security');
    });

    And('that of a Windows browser lists the Windows Security window', async () => {
      await closePage();
      stubBrowser('Windows', 'Win32', 'Mozilla/5.0 (Windows NT 10.0; Win64; x64)');
      openPage('#/onboarding');
      await tick();
      flushSync();
      const note = one('.sys-asks');
      expect(note.textContent).toContain('Windows Security');
      expect(note.textContent).not.toContain('Background Items Added');
    });

    And('that of a Linux browser shows no such note', async () => {
      await closePage();
      stubBrowser('Linux', 'Linux x86_64', 'Mozilla/5.0 (X11; Linux x86_64)');
      openPage('#/onboarding');
      await tick();
      flushSync();
      expect(document.querySelector('.sys-asks')).toBeNull();
    });
  });

  Scenario('Phone width and reduced motion (WP.9)', ({ When, Then, And }) => {
    let server: Awaited<ReturnType<typeof createServer>>;
    let browser: Awaited<ReturnType<typeof chromium.launch>>;
    let check: {
      scrollWidth: number;
      overflows: { tag: string; cls: string; right: number }[];
      buttonHeights: number[];
      stacked: boolean;
      runningAnimations: number;
    };

    beforeAll(async () => {
      server = await createServer({
        mode: 'development',
        server: { host: '127.0.0.1', port: 0 },
        logLevel: 'silent',
      });
      await server.listen();
      browser = await chromium.launch({ executablePath: process.env.CHROMIUM_PATH });
    }, 30_000);

    afterAll(async () => {
      await browser?.close();
      await server?.close();
    });

    When('the page is 375 px wide with a coach open', async () => {
      const page = await browser.newPage({
        viewport: { width: 375, height: 812 },
        locale: 'en-US',
      });
      try {
        await page.goto(`${server.resolvedUrls!.local[0]}#/nodes`);
        await page.locator('.perm-plain .turn').first().waitFor();
        await page.locator('.perm-plain .turn').first().click();
        await page.locator('.perm-plain .perm-rows .act').first().waitFor();
        await page.locator('.perm-plain .perm-rows .act').first().click();
        await page.locator('.perm-plain .coach').waitFor();

        const layout = await page.evaluate(() => {
          const vw = document.documentElement.clientWidth;
          const overflows = [...document.querySelectorAll('body *')]
            .map((el) => ({
              tag: el.tagName,
              cls: el.className?.toString() ?? '',
              rect: el.getBoundingClientRect(),
            }))
            .filter(({ rect }) => rect.width > 0 && rect.height > 0 && rect.right > vw + 0.5)
            .map(({ tag, cls, rect }) => ({ tag, cls, right: rect.right }));
          const buttonHeights = [
            ...document.querySelectorAll<HTMLButtonElement>('.perm-plain button'),
          ].map((btn) => btn.getBoundingClientRect().height);
          const sk = document.querySelector('.perm-plain .coach .sk')?.getBoundingClientRect();
          const body = document
            .querySelector('.perm-plain .coach .coach-body')
            ?.getBoundingClientRect();
          const stacked = Boolean(sk && body && body.top >= sk.bottom - 1);
          return {
            scrollWidth: document.documentElement.scrollWidth,
            overflows,
            buttonHeights,
            stacked,
          };
        });

        await page.emulateMedia({ reducedMotion: 'reduce' });
        await page.waitForTimeout(50);
        const runningAnimations = await page.evaluate(() => {
          const el = document.querySelector('.perm-plain');
          return el
            ? el.getAnimations({ subtree: true }).filter((a) => a.playState === 'running').length
            : -1;
        });

        check = { ...layout, runningAnimations };
      } finally {
        await page.close();
      }
    });

    Then('no element is wider than the page and the buttons are at least 44 px high', () => {
      expect(check.scrollWidth).toBeLessThanOrEqual(375);
      expect(check.overflows).toEqual([]);
      expect(check.stacked).toBe(true);
      expect(check.buttonHeights.length).toBeGreaterThanOrEqual(3);
      for (const h of check.buttonHeights) expect(h).toBeGreaterThanOrEqual(44);
    });

    And('with reduced motion no element of the card has a running animation', () => {
      expect(check.runningAnimations).toBe(0);
    });
  });
});

describe('permission wiring guards', () => {
  afterEach(async () => {
    sourceOverride = undefined;
    vi.unstubAllGlobals();
    await closePage();
    paths.clear();
    nodes.infos.clear();
    prefs.lang = 'en';
    history.replaceState(null, '', '/');
  });

  it('mounts open Permit between StatusPanel status card and account card on onboarding (#/onboarding)', async () => {
    class FakeSocket {
      static made: FakeSocket[] = [];
      sent: Record<string, unknown>[] = [];
      onmessage: ((event: { data: string }) => void | Promise<void>) | null = null;
      onclose: (() => void) | null = null;
      constructor(readonly url: string) {
        FakeSocket.made.push(this);
      }
      send(text: string) {
        this.sent.push(JSON.parse(text) as Record<string, unknown>);
      }
      close() {}
      async deliver(frame: unknown) {
        await this.onmessage?.({ data: JSON.stringify(frame) });
      }
    }

    FakePC.reset();
    sourceOverride = 'live';
    const idb = new IDBFactory();
    vi.stubGlobal('indexedDB', idb);
    vi.stubGlobal('WebSocket', FakeSocket);
    vi.stubGlobal('RTCPeerConnection', FakePC);
    const node = await Device.of(
      crypto,
      await crypto.subtle.generateKey(ed25519, false, ['sign', 'verify']),
    );
    const makeId = () => encode(crypto.getRandomValues(new Uint8Array(16)));

    openPage('#/onboarding');
    await vi.waitFor(() => expect(FakeSocket.made.length).toBeGreaterThan(0));
    const ws = FakeSocket.made.at(-1)!;
    await ws.deliver({ t: 'challenge', nonce: 'LdAL134CIs7YgmZUganB2Q' });
    await vi.waitFor(() => expect(ws.sent.some((f) => f.t === 'auth')).toBe(true));
    await ws.deliver({ t: 'ready', ice: [], ttl: 3600 });

    const codeMatch = /([0-9A-HJKMNP-TV-Z]{4}-){3}[0-9A-HJKMNP-TV-Z]{4}/.exec(
      one('#prompt').textContent!,
    )!;
    const secret = parseSecret(codeMatch[0]);
    const roomId = await room(crypto, secret);
    await vi.waitFor(() => expect(ws.sent).toContainEqual({ t: 'join', room: roomId }));
    await ws.deliver({ t: 'joined', room: roomId });
    await vi.waitFor(() =>
      expect(ws.sent.some((f) => f.t === 'room' && f.room === roomId)).toBe(true),
    );
    const browserPair = (ws.sent.find((f) => f.t === 'room') as { data: p.Pair }).data;
    const nodeSelf = { role: 'b' as const, k: node.id, kind: 'node' as const, name: 'workstation' };
    await ws.deliver({
      t: 'room',
      room: roomId,
      data: { t: 'pair', ...nodeSelf, mac: await mac(crypto, secret, nodeSelf) },
    });

    await vi.waitFor(() =>
      expect(
        ws.sent.some((f) => f.t === 'to' && JSON.parse((f.env as { p: string }).p).t === 'offer'),
      ).toBe(true),
    );
    const offerFrame = ws.sent.find(
      (f) => f.t === 'to' && JSON.parse((f.env as { p: string }).p).t === 'offer',
    )!;
    const conn = JSON.parse((offerFrame.env as { p: string }).p).conn as string;
    const pc = FakePC.made.at(-1)!;
    const answerEnv = await node.sign(
      JSON.stringify({
        t: 'answer',
        n: makeId(),
        ts: Date.now(),
        to: browserPair.k,
        conn,
        sdp: 'ans',
      }),
    );
    await ws.deliver({ t: 'from', k: node.id, env: answerEnv });
    await vi.waitFor(() => expect(pc.remote).toBeDefined());
    pc.ctl.open();

    const welcome = (re: number, isOwner: boolean) => ({
      t: 'welcome',
      re,
      ok: true,
      v: 1,
      device: node.id,
      name: 'workstation',
      version: 'v0.3.1',
      time: Date.now(),
      addrs: ['wss://rv.example/rv'],
      groups: [],
      owner: isOwner,
    });
    await vi.waitFor(() => expect(pc.ctl.frames().some((f) => f.t === 'hello')).toBe(true));
    pc.ctl.deliver(welcome(1, false));
    await vi.waitFor(() => expect(pc.ctl.frames().filter((f) => f.t === 'hello')).toHaveLength(2));
    const secondHello = pc.ctl.frames().filter((f) => f.t === 'hello')[1];
    pc.ctl.deliver(welcome(secondHello.id, true));
    await vi.waitFor(() => expect(pc.ctl.frames().some((f) => f.t === 'host.info')).toBe(true));
    const infoReq = pc.ctl.frames().find((f) => f.t === 'host.info')!;
    pc.ctl.deliver({
      t: 'host.info',
      re: infoReq.id,
      ok: true,
      device: node.id,
      name: 'workstation',
      os: 'darwin/arm64',
      version: 'v0.3.1',
      exe: '/Applications/SoloDawn.app/Contents/MacOS/solodawn',
      open: 0,
      max_open: 4,
      offline: false,
      update: null,
      homes: [],
      relay: null,
      permissions: { local_network: false, incoming: true, full_disk: false },
      agents: [
        {
          agent: 'claude',
          cli_version: '1.0',
          account: null,
          login_ok: true,
          driver: { accounts: ['personal'], takeover: 'none' },
          models: [],
          caps: {},
          groups: [],
        },
      ],
      settings: {
        retention_days: 90,
        idle_minutes: 15,
        default_level: 2,
        all_level3: false,
        approvers: [],
      },
    });

    await vi.waitFor(() => {
      flushSync();
      expect(one('.perm-card .coach .sk-alert')).not.toBeNull();
    });
    const colCards = all('.onboarding .wrap > div:nth-child(2) > section.card');
    expect(colCards[0].querySelector('#st-h')).not.toBeNull();
    expect(colCards[1].classList.contains('perm-card')).toBe(true);
    expect(colCards[1].querySelector('.perm-head h2')?.textContent?.trim()).toBe(
      'Permissions on this Mac',
    );
    expect(colCards[2].querySelector('h2.kind')?.textContent).toContain('Claude Code');
    await vi.waitFor(() =>
      expect(pc.ctl.frames().filter((f) => f.t === 'host.permit')).toHaveLength(1),
    );
    const permitReq = pc.ctl.frames().find((f) => f.t === 'host.permit')!;
    expect(permitReq).toEqual({ t: 'host.permit', id: permitReq.id });
  });

  it('re-probes host.permit when a computer reconnects with a new ctl on Computers', async () => {
    const macNode = ownedNode('workstation', device, {
      os: 'darwin/arm64',
      permissions: { local_network: true, incoming: true, full_disk: false },
    });
    homes.push(macNode.store);
    const sent: PermitReq[] = [];
    const origConnect = macNode.store.connect.bind(macNode.store);
    macNode.store.connect = () => {
      const link = origConnect();
      const origOnline = link.online;
      link.online = (ctl, welcome) => {
        const origReq = ctl.request.bind(ctl);
        ctl.request = ((req: PermitReq) => {
          if (req.t === 'host.permit') {
            sent.push(req);
            return Promise.resolve({ re: 1, ok: true as const });
          }
          return origReq(req as never);
        }) as p.Ctl['request'];
        origOnline(ctl, welcome);
      };
      return link;
    };

    await macNode.connect();
    paths.set(macNode.store, { kind: 'direct', rtt: 12, since: Date.now() });
    openPage('#/nodes');
    await vi.waitFor(() => {
      flushSync();
      expect(cardFor('workstation')?.querySelector('.perm-plain')).not.toBeNull();
    });
    expect(sent).toEqual([{ t: 'host.permit' }]);

    await macNode.connect();
    flushSync();
    await vi.waitFor(() => expect(sent).toHaveLength(2));
    expect(sent).toEqual([{ t: 'host.permit' }, { t: 'host.permit' }]);
  });

  it('shows non-permission errors from host.permit in FolderPicker (known and fallback)', async () => {
    const macNode = ownedNode('workstation', device, {
      os: 'darwin/arm64',
      permissions: { local_network: true, incoming: true, full_disk: false },
    });
    macNode.home.add(website, 'Website');
    homes.push(macNode.store);
    await macNode.connect();
    let answer: p.CmdResult | p.ErrorResponse = {
      re: 1,
      err: { code: 'not_found', msg: 'missing' },
    };
    const sent: PermitReq[] = [];
    attachPermit(macNode, sent, () => async () => answer);

    openPage(`#/groups/${website}`);
    await vi.waitFor(() => expect(find(website)?.info).toBeDefined());
    one<HTMLButtonElement>('.members .head button').click();
    await vi.waitFor(() => expect(all('dialog .choice').length).toBeGreaterThan(0));
    one<HTMLButtonElement>('dialog .folder .change').click();
    await tick();
    flushSync();
    await vi.waitFor(() => expect(one('dialog dialog input.input')).not.toBeNull());

    fill(one<HTMLInputElement>('dialog dialog input.input'), '/Users/me/missing');
    flushSync();
    one<HTMLButtonElement>('dialog dialog footer button.btn-primary').click();
    await vi.waitFor(() => {
      flushSync();
      expect(read(one('dialog dialog p.fail[role="alert"]'))).toBe(
        t('member.join.error.not_found'),
      );
    });

    answer = { re: 2, err: { code: 'weird', msg: 'x' } };
    one<HTMLButtonElement>('dialog dialog footer button.btn-primary').click();
    await vi.waitFor(() => {
      flushSync();
      expect(read(one('dialog dialog p.fail[role="alert"]'))).toBe(
        `${t('chat.error.other')} weird`,
      );
    });
  });
});
