import 'fake-indexeddb/auto';
import { describeFeature, loadFeature } from '@amiceli/vitest-cucumber';
import { IDBFactory } from 'fake-indexeddb';
import { flushSync, mount, unmount } from 'svelte';
import { expect, vi } from 'vitest';
import App from '../src/App.svelte';
import { DeviceLog } from '../src/lib/devlog';
import { Group, Home, homes } from '../src/lib/groups.svelte';
import { Device, ed25519, encode } from '../src/lib/identity';
import { saveOwner } from '../src/lib/owner';
import { prefs } from '../src/lib/prefs.svelte';
import { browser, example, owner, TestHome, user } from './home';
import { all, closePage, field, one, openPage, texts } from './page';
import { FakePC } from './peer-fakes';

const feature = await loadFeature('../features/web_status.feature');
const website = 'rZNvy-1jH6Z-BcPqA5U5BQ';
const challengeNonce = 'Zk3aZk3aZk3aZk3aZk3aZg';

class FakeSignallingSocket {
  static opened: FakeSignallingSocket[] = [];
  closed = false;
  onmessage: ((event: MessageEvent) => void) | null = null;
  onclose: ((event: CloseEvent) => void) | null = null;

  constructor(readonly url: string) {
    FakeSignallingSocket.opened.push(this);
    this.deliver({ t: 'challenge', nonce: challengeNonce });
  }

  deliver(frame: object) {
    queueMicrotask(() => {
      if (!this.closed) {
        this.onmessage?.(new MessageEvent('message', { data: JSON.stringify(frame) }));
      }
    });
  }

  send(raw: string) {
    const parsed = JSON.parse(raw) as { t?: string; k?: string[] };
    if (parsed.t === 'auth') this.deliver({ t: 'ready', ice: [], ttl: 3600 });
    if (parsed.t === 'watch' && parsed.k?.[0]) {
      this.deliver({ t: 'presence', k: parsed.k[0], on: true });
    }
  }

  close() {
    if (this.closed) return;
    this.closed = true;
    this.onclose?.(new CloseEvent('close'));
  }
}

interface HeldLock {
  abort: AbortController;
  done: Promise<unknown>;
}

function createFakeLocks() {
  const held = new Map<string, HeldLock>();
  return {
    heldNames: () => [...held.keys()],
    request: async (
      name: string,
      options: LockOptions,
      callback: (lock: Lock | null) => Promise<unknown> | null,
    ) => {
      const prev = held.get(name);
      if (options.ifAvailable && prev) return callback(null);
      if (options.steal && prev) {
        held.delete(name);
        prev.abort.abort();
      }
      const abort = new AbortController();
      const settled = Promise.withResolvers<void>();
      const entry: HeldLock = { abort, done: settled.promise };
      held.set(name, entry);
      const broken = new Promise<never>((_, reject) => {
        abort.signal.addEventListener('abort', () =>
          reject(new DOMException('Lock broken by steal', 'AbortError')),
        );
      });
      try {
        return await Promise.race([Promise.resolve(callback({ name, mode: 'exclusive' })), broken]);
      } finally {
        if (held.get(name) === entry) held.delete(name);
        settled.resolve();
      }
    },
  };
}

async function generateDashDevice() {
  while (true) {
    const pair = (await crypto.subtle.generateKey(ed25519, false, [
      'sign',
      'verify',
    ])) as CryptoKeyPair;
    const dev = await Device.of(crypto, pair);
    if (dev.id.startsWith('-')) return { pair, dev };
  }
}

function seedDeviceKey(idb: IDBFactory, pair: CryptoKeyPair): Promise<void> {
  return new Promise((resolve, reject) => {
    const req = idb.open('solodawn', 1);
    req.onupgradeneeded = () => {
      req.result.createObjectStore('device');
    };
    req.onsuccess = () => {
      const db = req.result;
      const tx = db.transaction('device', 'readwrite');
      tx.objectStore('device').put(pair, 'key');
      tx.oncomplete = () => {
        db.close();
        resolve();
      };
      tx.onerror = () => reject(tx.error);
    };
    req.onerror = () => reject(req.error);
  });
}

async function seedPairedBrowser(idb: IDBFactory) {
  const { pair, dev } = await generateDashDevice();
  await seedDeviceKey(idb, pair);
  const nodePair = (await crypto.subtle.generateKey(ed25519, false, [
    'sign',
    'verify',
  ])) as CryptoKeyPair;
  const nodeDev = await Device.of(crypto, nodePair);
  const log = new DeviceLog(crypto, dev.id);
  const makeEntry = (ver: number, target: string, kind: 'browser' | 'node', name: string) =>
    dev.sign(
      JSON.stringify({
        t: 'device',
        n: encode(crypto.getRandomValues(new Uint8Array(16))),
        ts: Date.now(),
        user: dev.id,
        ver,
        op: 'add',
        dev: target,
        kind,
        name,
      }),
    );
  await log.apply(await makeEntry(1, dev.id, 'browser', 'Web'));
  await log.apply(await makeEntry(2, nodeDev.id, 'node', 'workstation'));
  await saveOwner(() => idb, log, { [nodeDev.id]: ['wss://rv.example/rv'] });
  return dev.id;
}

interface MountedTab {
  host: HTMLDivElement;
  app: Record<string, never>;
}

function mountTab(hash: string): MountedTab {
  location.hash = hash;
  dispatchEvent(new HashChangeEvent('hashchange', { newURL: location.href }));
  const host = document.createElement('div');
  document.body.appendChild(host);
  const app = mount(App, { target: host }) as Record<string, never>;
  flushSync();
  return { host, app };
}

function writeControlsDisabled(): boolean[] {
  const controls = all<HTMLButtonElement | HTMLInputElement | HTMLTextAreaElement>(
    '.composer button, .composer textarea, .msg-body .card button, .msg-body .card input, .msg-body .card textarea, .members .head button',
  );
  return controls.map((el) => el.disabled);
}

describeFeature(feature, ({ Scenario, AfterEachScenario }) => {
  const defaultLocks = navigator.locks;
  let tabs: MountedTab[] = [];
  let home: TestHome;
  let client: ReturnType<typeof browser>;

  async function syncHome() {
    const expected = home.synced + home.groups.size;
    await client.connect();
    await vi.waitFor(() => expect(home.synced).toBe(expected));
    await new Promise((applied) => setTimeout(applied));
    flushSync();
  }

  AfterEachScenario(async () => {
    for (const tab of tabs.splice(0)) {
      await unmount(tab.app);
      tab.host.remove();
    }
    FakeSignallingSocket.opened = [];
    FakePC.reset();
    Object.defineProperty(navigator, 'locks', { configurable: true, value: defaultLocks });
    prefs.lang = 'en';
    vi.unstubAllEnvs();
    await closePage();
  });

  Scenario(
    'A second tab shows the other-tab notice until Use here steals the lock and closes the first tab',
    ({ Given, And, When, Then }) => {
      let fakeLocks: ReturnType<typeof createFakeLocks>;
      let deviceId = '';
      let tab1: MountedTab;
      let tab2: MountedTab;
      let tab1Sockets: FakeSignallingSocket[] = [];
      let tab1Peers: FakePC[] = [];
      let countBeforeOnline = 0;
      let peersBeforeOnline = 0;

      Given(
        'a browser paired with "workstation" using a device key beginning with "-"',
        async () => {
          const idb = new IDBFactory();
          vi.stubGlobal('indexedDB', idb);
          vi.stubGlobal('WebSocket', FakeSignallingSocket);
          vi.stubGlobal('RTCPeerConnection', FakePC);
          vi.stubGlobal(
            'IntersectionObserver',
            class {
              observe() {}
              disconnect() {}
            },
          );
          vi.spyOn(window, 'matchMedia').mockImplementation(
            () =>
              ({
                matches: false,
                addEventListener() {},
                removeEventListener() {},
              }) as unknown as MediaQueryList,
          );
          fakeLocks = createFakeLocks();
          Object.defineProperty(navigator, 'locks', { configurable: true, value: fakeLocks });
          prefs.lang = 'en';
          deviceId = await seedPairedBrowser(idb);
          expect(deviceId.startsWith('-')).toBe(true);
        },
      );

      And('the first tab is open and connected', async () => {
        tab1 = mountTab('#/groups');
        tabs.push(tab1);
        await vi.waitFor(() => {
          expect(FakeSignallingSocket.opened.length).toBeGreaterThan(0);
          expect(FakePC.made.length).toBeGreaterThan(0);
        });
        tab1Sockets = [...FakeSignallingSocket.opened];
        tab1Peers = [...FakePC.made];
        expect(fakeLocks.heldNames()).toEqual([`solodawn-device-${deviceId}`]);
        expect(tab1.host.querySelector('.tab-ban')).toBeNull();
      });

      When('a second tab opens at "#/groups"', async () => {
        tab2 = mountTab('#/groups');
        tabs.push(tab2);
        await vi.waitFor(() => {
          flushSync();
          expect(tab2.host.querySelector('.tab-ban')).not.toBeNull();
        });
      });

      Then(
        'the second tab shows the other-tab notice "SoloDawn is open in another tab." with "Use here" and opens no connection',
        () => {
          const ban = tab2.host.querySelector('.tab-ban[role="status"]');
          expect(ban?.querySelector('.grow')?.textContent?.trim()).toBe(
            'SoloDawn is open in another tab.',
          );
          expect(ban?.querySelector('button')?.textContent?.trim()).toBe('Use here');
          expect(FakeSignallingSocket.opened).toHaveLength(tab1Sockets.length);
          expect(FakePC.made).toHaveLength(tab1Peers.length);
        },
      );

      And(
        'in Chinese the second tab notice reads "SoloDawn 已在另一个标签页中打开。" with "在此使用"',
        () => {
          prefs.lang = 'zh-CN';
          flushSync();
          const ban = tab2.host.querySelector('.tab-ban');
          expect(ban?.querySelector('.grow')?.textContent?.trim()).toBe(
            'SoloDawn 已在另一个标签页中打开。',
          );
          expect(ban?.querySelector('button')?.textContent?.trim()).toBe('在此使用');
          prefs.lang = 'en';
          flushSync();
        },
      );

      When('the person chooses "Use here" in the second tab', () => {
        tab2.host.querySelector<HTMLButtonElement>('.tab-ban button')?.click();
        flushSync();
      });

      Then(
        'the second tab clears the notice and connects, while the first tab closes its connections and shows the notice',
        async () => {
          await vi.waitFor(() => {
            flushSync();
            expect(tab2.host.querySelector('.tab-ban')).toBeNull();
            expect(tab1.host.querySelector('.tab-ban .grow')?.textContent?.trim()).toBe(
              'SoloDawn is open in another tab.',
            );
            expect(tab1Sockets.every((ws) => ws.closed)).toBe(true);
            expect(tab1Peers.every((pc) => pc.closes > 0)).toBe(true);
            expect(FakeSignallingSocket.opened.length).toBeGreaterThan(tab1Sockets.length);
            expect(FakePC.made.length).toBeGreaterThan(tab1Peers.length);
          });
        },
      );

      When('a network online event fires in the first tab', async () => {
        countBeforeOnline = FakeSignallingSocket.opened.length;
        peersBeforeOnline = FakePC.made.length;
        window.dispatchEvent(new Event('online'));
        await new Promise((later) => setTimeout(later, 30));
        flushSync();
      });

      Then('the first tab opens no new connection and keeps showing the notice', () => {
        expect(FakeSignallingSocket.opened).toHaveLength(countBeforeOnline);
        expect(FakePC.made).toHaveLength(peersBeforeOnline);
        expect(tab1.host.querySelector('.tab-ban .grow')?.textContent?.trim()).toBe(
          'SoloDawn is open in another tab.',
        );
      });
    },
  );

  Scenario(
    'With the home unreachable the chat shows the read-only banner, keeps cached messages, disables write controls, and resumes on return',
    ({ Given, Then, When, And }) => {
      Given(
        'the chat of "Website" on "workstation" is open with cached messages, a pending brief, a pending approval, and a pending decision',
        async () => {
          home = new TestHome('workstation');
          home.add(website, 'Website');
          home.write(
            website,
            example('row-member-ai', { id: 'sec', name: 'Secretary', secretary: true }),
          );
          home.write(website, example('row-member-ai', { id: 'codex', name: 'Codex' }));
          home.say(website, owner, 'Cached request from earlier');
          home.write(
            website,
            example('row-msg-brief-pending', {
              id: 'brief-ro',
              by: 'sec',
              thread: null,
              body: {
                ...(example('row-msg-brief-pending').body as object),
                requester: owner,
              },
            }),
          );
          home.write(
            website,
            example('row-msg-approval-pending', {
              id: 'ap-ro',
              by: 'sec',
              thread: null,
              body: {
                ...(example('row-msg-approval-pending').body as object),
                ai: 'codex',
                approvers: [user],
              },
            }),
          );
          home.write(
            website,
            example('row-msg-decision-pending', {
              id: 'dc-ro',
              by: 'sec',
              thread: null,
              body: {
                task: '',
                question: 'Keep redirects?',
                options: [{ id: 'A', label: 'Keep', effect: 'Safe.' }],
                to: [user],
              },
            }),
          );
          client = browser(home);
          client.store.owner = true;
          homes.push(client.store);
          await syncHome();
          openPage(`#/groups/${website}`);
        },
      );

      Then('the chat shows no unreachable-home banner and its write controls are enabled', () => {
        expect(all('.chat > .top .home-ban')).toEqual([]);
        expect(field().disabled).toBe(false);
        expect(one<HTMLButtonElement>('#m-brief-ro .btn-dawn').disabled).toBe(false);
        expect(one<HTMLButtonElement>('#m-ap-ro .btn-dawn').disabled).toBe(false);
        expect(one<HTMLButtonElement>('#m-dc-ro .opt').disabled).toBe(false);
        expect(one<HTMLButtonElement>('.members .head button').disabled).toBe(false);
      });

      When('the home stops', () => {
        home.stop();
        client.drop();
        flushSync();
      });

      Then(
        'the chat top banner says "Read-only · cannot reach the group home (workstation). Messages already here stay readable until it returns."',
        () => {
          expect(texts('.chat > .top .home-ban[role="status"]')).toEqual([
            'Read-only · cannot reach the group home (workstation). Messages already here stay readable until it returns.',
          ]);
        },
      );

      And(
        'in Chinese the chat top banner says "只读 · 连不上群的家（workstation）。恢复连接前仍可查看已有记录。"',
        () => {
          prefs.lang = 'zh-CN';
          flushSync();
          expect(texts('.chat > .top .home-ban[role="status"]')).toEqual([
            '只读 · 连不上群的家（workstation）。恢复连接前仍可查看已有记录。',
          ]);
          prefs.lang = 'en';
          flushSync();
        },
      );

      And(
        'the cached messages remain readable while the composer, card actions, and Add AI button are disabled',
        () => {
          expect(texts('.msg .text')).toContain('Cached request from earlier');
          const states = writeControlsDisabled();
          expect(states.length).toBeGreaterThanOrEqual(9);
          expect(states.every(Boolean)).toBe(true);
        },
      );

      When('the home returns with a message written meanwhile', async () => {
        home.say(website, owner, 'Written while offline');
        await syncHome();
      });

      Then(
        'the chat top banner is gone, the missed message appears without reload, and the write controls are enabled again',
        () => {
          expect(all('.chat > .top .home-ban')).toEqual([]);
          expect(texts('.msg .text')).toContain('Written while offline');
          expect(field().disabled).toBe(false);
          expect(one<HTMLButtonElement>('#m-brief-ro .btn-dawn').disabled).toBe(false);
          expect(one<HTMLButtonElement>('#m-ap-ro .btn-dawn').disabled).toBe(false);
          expect(one<HTMLButtonElement>('#m-dc-ro .opt').disabled).toBe(false);
          expect(one<HTMLButtonElement>('.members .head button').disabled).toBe(false);
        },
      );
    },
  );

  Scenario(
    'An unnamed unreachable home omits parentheses and the states sheet lists status mounts',
    ({ Given, When, Then }) => {
      Given('a group whose unreachable home has no name yet', () => {
        const bareHome = new Home({
          user,
          sign: async (p) => ({ p, k: 'this-browser', s: 'sig' }),
        });
        const bareGroup = new Group(bareHome, website);
        bareGroup.name = 'Website';
        bareGroup.apply([
          example('row-group', { id: website, name: 'Website' }),
          example('row-member-human', { g: website, name: 'Alex' }),
        ]);
        bareHome.groups = [bareGroup];
        homes.push(bareHome);
      });

      When('the person opens its chat', () => {
        openPage(`#/groups/${website}`);
      });

      Then(
        'the chat top banner says "Read-only · cannot reach the group home. Messages already here stay readable until it returns."',
        () => {
          expect(texts('.chat > .top .home-ban[role="status"]')).toEqual([
            'Read-only · cannot reach the group home. Messages already here stay readable until it returns.',
          ]);
        },
      );

      When('the person opens the states sheet in a development server', async () => {
        await closePage();
        vi.stubEnv('MODE', 'development');
        openPage('#/states');
        await vi.waitFor(() => {
          flushSync();
          expect(all('.states')).toHaveLength(1);
        });
      });

      Then(
        'the mounting points show "status · 10" at "banner" and "status · 10" at "chat.top"',
        () => {
          const dts = texts('.points dt');
          const dds = all('.points dd');
          expect(dds[dts.indexOf('banner')].textContent).toContain('status · 10');
          expect(dds[dts.indexOf('chat.top')].textContent).toContain('status · 10');
        },
      );
    },
  );
});
