import { IDBFactory } from 'fake-indexeddb';
import { flushSync, mount, unmount } from 'svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { FakePC, transportStats } from '../../features/peer-fakes';
import App from '../App.svelte';
import Indicator from '../components/nodes/Indicator.svelte';
import { DeviceLog } from './devlog';
import { Home, homes } from './groups.svelte';
import { Device, ed25519, encode, loadDevice } from './identity';
import { goldens } from './mock';
import { mockHome, sourceOf, start } from './net.svelte';
import { loadOwner, saveOwner } from './owner';
import { prefs } from './prefs.svelte';
import * as p from './proto';

// The requests the mock connection was asked, as sent.
const asked = vi.hoisted(() => [] as unknown[]);
vi.mock('./mock', async (actual) => {
  const mock = await actual<typeof import('./mock')>();
  return {
    ...mock,
    mockCtl: (onpush: (push: p.Push) => void): p.Ctl => {
      const ctl = mock.mockCtl(onpush);
      return {
        request: (req: never) => {
          asked.push(req);
          return ctl.request(req);
        },
      } as unknown as p.Ctl;
    },
  };
});

/** Signalling clients created by peer.ts in place of rendezvous.ts (F-02 mock transport). */
const { clients, FakeClient } = vi.hoisted(() => {
  class FakeClient {
    sent: { t: string; [field: string]: unknown }[] = [];
    closed = false;
    constructor(
      readonly urls: readonly string[],
      readonly device: unknown,
      readonly known: (id: string) => boolean,
      readonly receive: (url: string, frame: unknown) => void,
    ) {
      clients.push(this);
    }
    get url() {
      return this.urls[0];
    }
    send(frame: { t: string }) {
      this.sent.push(frame);
    }
    close() {
      this.closed = true;
    }
    ready(ice: unknown[] = [], ttl = 3600) {
      this.receive(this.url, { t: 'ready', ice, ttl });
    }
    offersFor(target: string) {
      return this.sent
        .filter((f) => f.t === 'to' && f.k === target)
        .map((f) => JSON.parse((f.env as p.Envelope).p) as { t: string; conn: string })
        .filter((payload) => payload.t === 'offer');
    }
  }
  const clients: FakeClient[] = [];
  return { clients, FakeClient };
});
type FakeClient = InstanceType<typeof FakeClient>;

vi.mock('./rendezvous', () => ({
  rendezvous: (...args: ConstructorParameters<typeof FakeClient>) => new FakeClient(...args),
}));

const person: string = JSON.parse(goldens['row-member-human']).user;
const websiteId = 'rZNvy-1jH6Z-BcPqA5U5BQ';
const elsewhere = () => new Home({ user: 'u', sign: () => new Promise(() => {}) });
const makeId = () => encode(crypto.getRandomValues(new Uint8Array(16)));
const newDevice = async () =>
  Device.of(crypto, await crypto.subtle.generateKey(ed25519, false, ['sign', 'verify']));

async function signDeviceEntry(
  signer: Device,
  user: string,
  ver: number,
  op: 'add' | 'revoke',
  dev: string,
  kind?: 'node' | 'browser',
  name?: string,
) {
  return signer.sign(
    JSON.stringify({
      t: 'device',
      n: makeId(),
      ts: Date.now(),
      user,
      ver,
      op,
      dev,
      ...(op === 'add' ? { kind, name } : {}),
    }),
  );
}

/** Simple in-memory Web Locks implementation matching LockManager's ifAvailable and steal modes. */
function createFakeLocks() {
  interface Held {
    abort: AbortController;
    done: Promise<unknown>;
  }
  const locks = new Map<string, Held>();
  return {
    request: async (
      name: string,
      options: LockOptions,
      callback: (lock: Lock | null) => Promise<unknown> | null,
    ) => {
      const current = locks.get(name);
      if (options.ifAvailable && current) return callback(null);
      if (options.steal && current) {
        locks.delete(name);
        current.abort.abort();
      }
      const abort = new AbortController();
      const stolen = new Promise<never>((_, reject) => {
        abort.signal.addEventListener('abort', () =>
          reject(new DOMException('Lock broken by steal', 'AbortError')),
        );
      });
      const run = Promise.resolve(callback({ name, mode: 'exclusive' } as Lock));
      const entry: Held = { abort, done: run };
      locks.set(name, entry);
      try {
        return await Promise.race([run, stolen]);
      } finally {
        if (locks.get(name) === entry) locks.delete(name);
      }
    },
  };
}

let idb: IDBFactory;
const stops: (() => void)[] = [];
const startTracked = (source: Parameters<typeof start>[0]) => {
  const handle = start(source);
  stops.push(handle.stop);
  return handle;
};

beforeEach(() => {
  idb = new IDBFactory();
  vi.stubGlobal('indexedDB', idb);
  vi.stubGlobal('RTCPeerConnection', FakePC);
  Object.defineProperty(navigator, 'locks', {
    configurable: true,
    value: {
      request: (_name: string, _opts: unknown, cb: (lock: object) => unknown) =>
        Promise.resolve(cb({})),
    },
  });
  FakePC.reset();
  clients.length = 0;
  homes.length = 0;
  asked.length = 0;
  location.hash = '';
  prefs.lang = 'en';
});

afterEach(() => {
  while (stops.length) stops.pop()!();
  homes.length = 0;
  asked.length = 0;
  clients.length = 0;
  FakePC.reset();
  location.hash = '';
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

async function answerNode(
  client: FakeClient,
  node: Device,
  browserId: string,
  pcIndex: number,
  stats = transportStats('host', 'host', 0.008),
) {
  client.receive(client.url, { t: 'presence', k: node.id, on: true });
  await vi.waitFor(() => expect(client.offersFor(node.id).length).toBeGreaterThan(0), {
    timeout: 3000,
  });
  const { conn } = client.offersFor(node.id).at(-1)!;
  const pc = FakePC.made[pcIndex];
  pc.stats = stats;
  const env = await node.sign(
    JSON.stringify({ t: 'answer', n: makeId(), ts: Date.now(), to: browserId, conn, sdp: 'ans' }),
  );
  if (client.known(node.id)) client.receive(client.url, { t: 'from', k: node.id, env });
  await vi.waitFor(() => expect(pc.remote).toBeDefined());
  pc.ctl.open();
  await vi.waitFor(() => expect(pc.ctl.frames().some((f) => f.t === 'hello')).toBe(true));
  return pc;
}

function welcomeFrame(
  re: number,
  device: string,
  name: string,
  groups: p.Welcome['groups'] = [],
  addrs = ['wss://rv.example/rv'],
): p.Welcome {
  return {
    t: 'welcome',
    re,
    ok: true,
    v: 1,
    device,
    name,
    version: 'v0.5.0',
    time: Date.now(),
    addrs,
    groups,
    owner: true,
  };
}

describe('sourceOf', () => {
  it.each([
    ['development', '', 'mock'],
    ['development', '?a=1', 'mock'],
    ['development', '?nomockery', 'mock'],
    ['development', '?nomock', 'live'],
    ['development', '?a=1&nomock=', 'live'],
    ['production', '', 'live'],
    ['test', '', 'live'],
  ])('is, in the mode %s at %j, %s', (mode, search, source) => {
    expect(sourceOf(mode, search)).toBe(source);
  });
});

describe('mockHome', () => {
  it("is the examples' home, read by their person with this browser's key, once online", async () => {
    const { home, online } = await mockHome();
    expect(home.me.user).toBe(person);
    expect(asked).toStrictEqual([
      { t: 'hello', v: 1, kind: 'browser', name: 'development', addrs: [] },
    ]);
    const env = await home.me.sign('{"t":"cmd"}');
    expect(env.p).toBe('{"t":"cmd"}');
    expect(env.k).toMatch(/^[\w-]{43}$/);
    expect(home.ctl).toBeUndefined();
    online();
    expect(home.ctl).toBeDefined();
    expect(home.groups.map((group) => group.name)).toEqual(['Website']);
  });
});

describe('start (mock)', () => {
  it('adds the home of the mock, online and reached directly since it came online', async () => {
    const before = Date.now();
    const other = elsewhere();
    homes.push(other);
    const { net, stop } = start('mock');
    expect(net.source).toBe('mock');
    await vi.waitFor(() => expect(homes).toHaveLength(2));
    const home = homes[1];
    expect(home.groups.map((group) => group.name)).toEqual(['Website']);
    const path = net.path(home)!;
    expect(path).toStrictEqual({ kind: 'direct', rtt: undefined, since: path.since });
    expect(path.since).toBeGreaterThanOrEqual(before);
    expect(path.since).toBeLessThanOrEqual(Date.now());
    expect(net.path(other)).toBeUndefined();
    other.ctl = home.ctl;
    expect(net.path(other)).toBeUndefined();
    other.ctl = undefined;
    home.ctl = undefined;
    expect(net.path(home)).toBeUndefined();
    stop();
    expect(homes).toEqual([other]);
  });

  it('adds nothing, and says why, when this browser cannot keep its device key', async () => {
    const failed = new DOMException('The operation is insecure.', 'SecurityError');
    const said = vi.spyOn(console, 'error').mockImplementation(() => {});
    vi.spyOn(globalThis, 'indexedDB', 'get').mockImplementation(() => {
      throw failed;
    });
    start('mock');
    await vi.waitFor(() => expect(said).toHaveBeenCalledOnce());
    expect(said.mock.calls[0][0]).toBe('The mock home could not start:');
    expect(said.mock.calls[0][1]).toMatchObject({ name: 'StorageBlocked' });
    expect(homes).toEqual([]);
  });

  it('adds nothing when stopped before the mock is ready', async () => {
    const { stop } = start('mock');
    stop();
    await vi.waitFor(() => expect(asked).toHaveLength(1));
    await new Promise((later) => setTimeout(later, 20));
    expect(homes).toEqual([]);
  });
});

describe('start (live)', () => {
  it('opens no session when unpaired, when only browsers are in the log, or when storage is blocked', async () => {
    // 1. Unpaired browser.
    const first = start('live');
    await new Promise((later) => setTimeout(later, 30));
    expect(first.net.source).toBe('live');
    expect(first.net.homes).toBe(homes);
    expect(homes).toEqual([]);
    expect(clients).toEqual([]);
    first.stop();

    // 2. Owner log containing only the browser itself (no node entries).
    const realReplay = DeviceLog.replay.bind(DeviceLog);
    let entriesIter: ReturnType<typeof vi.spyOn> | undefined;
    const replaySpy = vi.spyOn(DeviceLog, 'replay').mockImplementation(async (...args) => {
      const log = await realReplay(...args);
      entriesIter = vi.spyOn(log.entries, Symbol.iterator);
      return log;
    });
    const browserDev = await loadDevice({ crypto, idb: () => idb });
    const browserOnlyLog = new DeviceLog(crypto, browserDev.id);
    await browserOnlyLog.apply(
      await signDeviceEntry(browserDev, browserDev.id, 1, 'add', browserDev.id, 'browser', 'Web'),
    );
    await saveOwner(() => idb, browserOnlyLog, {});
    const second = start('live');
    await vi.waitFor(() => expect(entriesIter).toHaveBeenCalledOnce());
    expect(homes).toEqual([]);
    expect(clients).toEqual([]);
    second.stop();
    replaySpy.mockRestore();

    // 3. Stopped before loadDevice settles, and when loadDevice rejects.
    const earlyStop = start('live');
    earlyStop.stop();
    await new Promise((later) => setTimeout(later, 20));
    expect(clients).toEqual([]);

    vi.spyOn(globalThis, 'indexedDB', 'get').mockImplementation(() => {
      throw new DOMException('blocked', 'SecurityError');
    });
    const blockedRun = start('live');
    await new Promise((later) => setTimeout(later, 20));
    expect(homes).toEqual([]);
    blockedRun.stop();
  });

  it('connects to paired non-revoked nodes, populates groups, updates from pushes and reactive path, and cleans up on stop', async () => {
    const browserDev = await loadDevice({ crypto, idb: () => idb });
    const nodeA = await newDevice();
    const nodeRevoked = await newDevice();
    const nodeLate = await newDevice();

    const log = new DeviceLog(crypto, browserDev.id);
    await log.apply(
      await signDeviceEntry(browserDev, browserDev.id, 1, 'add', browserDev.id, 'browser', 'Web'),
    );
    await log.apply(
      await signDeviceEntry(browserDev, browserDev.id, 2, 'add', nodeA.id, 'node', 'workstation'),
    );
    await log.apply(
      await signDeviceEntry(browserDev, browserDev.id, 3, 'add', nodeRevoked.id, 'node', 'old-pc'),
    );
    await log.apply(await signDeviceEntry(browserDev, browserDev.id, 4, 'revoke', nodeRevoked.id));
    await saveOwner(() => idb, log, { [nodeA.id]: ['wss://rv-a.example/rv'] });

    const preExisting = elsewhere();
    homes.push(preExisting);

    const host = document.createElement('div');
    document.body.appendChild(host);
    const { net, stop } = startTracked('live');
    const indicatorView = mount(Indicator, { target: host, props: { net } });

    await vi.waitFor(() => expect(homes).toHaveLength(2));
    const homeA = homes[1];
    expect(homeA.device).toBe(nodeA.id);
    expect(homeA.name).toBe('workstation');
    expect(homeA.owner).toBe(true);
    expect(net.path(homeA)).toBeUndefined();

    // Signalling URLs include both page origin and nodeA's remembered rendezvous.
    const urls = clients.map((c) => c.url);
    expect(urls).toContain('wss://rv-a.example/rv');
    const rvClient = clients.find((c) => c.url === 'wss://rv-a.example/rv')!;
    rvClient.ready();

    // Only nodeA is offered; the revoked node is never dialled.
    const pcA = await answerNode(rvClient, nodeA, browserDev.id, 0);
    expect(rvClient.offersFor(nodeRevoked.id)).toEqual([]);

    const helloFrame = pcA.ctl.frames().find((f) => f.t === 'hello')!;
    pcA.ctl.deliver(
      welcomeFrame(
        helloFrame.id,
        nodeA.id,
        'workstation-pro',
        [{ g: websiteId, name: 'Website', seq: 1 }],
        ['wss://rv-updated.example/rv'],
      ),
    );

    // Browser sends its owner devlog on welcome, updates saved addrs, and runs sync on Website.
    await vi.waitFor(() => {
      expect(homeA.ctl).toBeDefined();
      expect(homeA.name).toBe('workstation-pro');
      expect(homeA.groups.map((g) => g.name)).toEqual(['Website']);
      expect(pcA.ctl.frames().some((f) => f.t === 'devlog')).toBe(true);
      expect(pcA.ctl.frames().some((f) => f.t === 'sync' && f.g === websiteId)).toBe(true);
    });
    expect((await loadOwner(crypto, () => idb)).addrs[nodeA.id]).toEqual([
      'wss://rv-updated.example/rv',
    ]);

    // Path is reactive: Indicator updates from direct (8 ms) to relay (45 ms).
    await vi.waitFor(() => {
      flushSync();
      expect(net.path(homeA)).toMatchObject({ kind: 'direct', rtt: 8 });
      expect(host.querySelector('.conn')?.getAttribute('data-st')).toBe('ok');
      expect(host.querySelector('.conn')?.textContent).toContain('8 ms');
    });

    vi.useFakeTimers();
    pcA.stats = transportStats('relay', 'host', 0.045);
    await vi.advanceTimersByTimeAsync(1000);
    vi.useRealTimers();
    await vi.waitFor(() => {
      flushSync();
      expect(net.path(homeA)).toMatchObject({ kind: 'relay', rtt: 45 });
      expect(host.querySelector('.conn')?.getAttribute('data-st')).toBe('limited');
    });

    // Answer initial sync and push a message row and a groups update; verify Home routes commands.
    const syncReq = pcA.ctl.frames().find((f) => f.t === 'sync' && f.g === websiteId)!;
    pcA.ctl.deliver({ t: 'synced', re: syncReq.id, ok: true, g: websiteId, seq: 1 });
    const msgRow = { ...p.decode(p.Row, goldens['row-msg-text']), g: websiteId, seq: 2, pos: 2 };
    pcA.ctl.deliver({ t: 'rows', g: websiteId, rows: [msgRow] });
    await vi.waitFor(() => expect(homeA.groups[0].main).toHaveLength(1));

    const postPromise = homeA.groups[0].post('Hello live', []);
    await vi.waitFor(() => expect(pcA.ctl.frames().some((f) => f.t === 'cmd')).toBe(true));
    const cmdFrame = pcA.ctl.frames().find((f) => f.t === 'cmd')!;
    pcA.ctl.deliver({ re: cmdFrame.id, ok: true });
    expect(await postPromise).toEqual({ re: cmdFrame.id, ok: true });

    const secondGroup = 'aGVsbG8tZ3JvdXAtMDAwMg';
    pcA.ctl.deliver({
      t: 'groups',
      groups: [
        { g: websiteId, name: 'Website', seq: 2 },
        { g: secondGroup, name: 'Handbook', seq: 0 },
      ],
    });
    await vi.waitFor(() =>
      expect(homeA.groups.map((g) => g.name)).toEqual(['Website', 'Handbook']),
    );

    // Duplicate/unrelated/invalid devlog pushes do not trigger save or echo; a new node entry
    // dynamically adds a second Home, dials it, and forwards the updated devlog.
    const sentDevlogsBefore = pcA.ctl.frames().filter((f) => f.t === 'devlog').length;
    pcA.ctl.deliver({ t: 'devlog', user: 'someone-else', entries: [...log.entries] });
    pcA.ctl.deliver({ t: 'devlog', user: browserDev.id, entries: [...log.entries] });
    pcA.ctl.deliver({
      t: 'devlog',
      user: browserDev.id,
      entries: [{ p: 'bad', k: browserDev.id, s: 'bad' }],
    });
    await new Promise((later) => setTimeout(later, 20));
    expect(pcA.ctl.frames().filter((f) => f.t === 'devlog')).toHaveLength(sentDevlogsBefore);

    const e5 = await signDeviceEntry(
      browserDev,
      browserDev.id,
      5,
      'add',
      nodeLate.id,
      'node',
      'laptop',
    );
    pcA.ctl.deliver({ t: 'devlog', user: browserDev.id, entries: [e5] });
    await vi.waitFor(() => {
      expect(homes).toHaveLength(3);
      expect(homes[2].name).toBe('laptop');
      expect(rvClient.offersFor(nodeLate.id)).toHaveLength(1);
      expect(pcA.ctl.frames().filter((f) => f.t === 'devlog').length).toBeGreaterThan(
        sentDevlogsBefore,
      );
    });

    // Connection drop clears ctl and path; stop() closes connections and removes only added homes.
    pcA.ctl.onclose();
    await vi.waitFor(() => {
      expect(homeA.ctl).toBeUndefined();
      expect(net.path(homeA)).toBeUndefined();
    });

    stop();
    expect(rvClient.closed).toBe(true);
    expect(homes).toEqual([preExisting]);
    await unmount(indicatorView);
    host.remove();
  });

  it('drops late devlog updates if the session closes while verification is in flight', async () => {
    const browserDev = await loadDevice({ crypto, idb: () => idb });
    const nodeA = await newDevice();
    const nodeB = await newDevice();
    const log = new DeviceLog(crypto, browserDev.id);
    await log.apply(
      await signDeviceEntry(browserDev, browserDev.id, 1, 'add', browserDev.id, 'browser', 'Web'),
    );
    await log.apply(
      await signDeviceEntry(browserDev, browserDev.id, 2, 'add', nodeA.id, 'node', 'workstation'),
    );
    await saveOwner(() => idb, log, { [nodeA.id]: ['wss://rv.example/rv'] });

    // Also cover stop() firing between runTab() and the async lock grant callback.
    const grantGate = Promise.withResolvers<void>();
    let lockEntered = false;
    Object.defineProperty(navigator, 'locks', {
      configurable: true,
      value: {
        request: async (_name: string, _opts: unknown, cb: (lock: object) => unknown) => {
          lockEntered = true;
          await grantGate.promise;
          return cb({});
        },
      },
    });
    const abortedBeforeGrant = start('live');
    await vi.waitFor(() => expect(lockEntered).toBe(true));
    abortedBeforeGrant.stop();
    grantGate.resolve();
    await new Promise((later) => setTimeout(later, 20));
    expect(clients).toEqual([]);

    // Restore default granted lock stub for the live session check.
    Object.defineProperty(navigator, 'locks', {
      configurable: true,
      value: {
        request: (_name: string, _opts: unknown, cb: (lock: object) => unknown) =>
          Promise.resolve(cb({})),
      },
    });

    const { stop } = startTracked('live');
    await vi.waitFor(() => expect(clients.length).toBeGreaterThan(0));
    const rvClient = clients.find((c) => c.url === 'wss://rv.example/rv')!;
    rvClient.ready();
    const pcA = await answerNode(rvClient, nodeA, browserDev.id, 0);
    const hello = pcA.ctl.frames().find((f) => f.t === 'hello')!;
    pcA.ctl.deliver(welcomeFrame(hello.id, nodeA.id, 'workstation'));
    await vi.waitFor(() => expect(homes[0].ctl).toBeDefined());

    const e3 = await signDeviceEntry(browserDev, browserDev.id, 3, 'add', nodeB.id, 'node', 'n2');
    pcA.ctl.deliver({ t: 'devlog', user: browserDev.id, entries: [e3] });
    stop();
    await new Promise((later) => setTimeout(later, 30));
    expect((await loadOwner(crypto, () => idb)).log?.entries).toHaveLength(2);
  });

  it('pauses live session on #/onboarding and picks up newly paired nodes on route change', async () => {
    const browserDev = await loadDevice({ crypto, idb: () => idb });
    const nodeA = await newDevice();
    const nodeB = await newDevice();

    const addListenerSpy = vi.spyOn(window, 'addEventListener');
    location.hash = '#/onboarding';
    const { stop } = startTracked('live');
    await vi.waitFor(() =>
      expect(addListenerSpy.mock.calls.some(([type]) => type === 'hashchange')).toBe(true),
    );
    expect(clients).toEqual([]);

    // Pairing completes on #/onboarding and saves nodeA.
    const log = new DeviceLog(crypto, browserDev.id);
    await log.apply(
      await signDeviceEntry(browserDev, browserDev.id, 1, 'add', browserDev.id, 'browser', 'Web'),
    );
    await log.apply(
      await signDeviceEntry(browserDev, browserDev.id, 2, 'add', nodeA.id, 'node', 'workstation'),
    );
    await saveOwner(() => idb, log, { [nodeA.id]: ['wss://rv.example/rv'] });

    // Navigating from #/onboarding to #/groups starts the live session; navigating to #/nodes
    // reuses the open session without closing it.
    location.hash = '#/groups';
    dispatchEvent(new HashChangeEvent('hashchange', { newURL: location.href }));
    await vi.waitFor(() => expect(homes).toHaveLength(1));
    const firstHome = homes[0];
    const rvFirst = clients.find((c) => c.url === 'wss://rv.example/rv')!;
    expect(rvFirst.closed).toBe(false);

    location.hash = '#/nodes';
    dispatchEvent(new HashChangeEvent('hashchange', { newURL: location.href }));
    await new Promise((later) => setTimeout(later, 20));
    expect(rvFirst.closed).toBe(false);

    // Navigating back to #/onboarding to add a second computer closes the live session cleanly.
    location.hash = '#/onboarding';
    dispatchEvent(new HashChangeEvent('hashchange', { newURL: location.href }));
    expect(rvFirst.closed).toBe(true);

    // Also verify a rapid #/groups -> #/onboarding bounce while loadOwner is in flight opens nothing.
    await log.apply(
      await signDeviceEntry(browserDev, browserDev.id, 3, 'add', nodeB.id, 'node', 'laptop'),
    );
    await saveOwner(() => idb, log, {
      [nodeA.id]: ['wss://rv.example/rv'],
      [nodeB.id]: ['wss://rv.example/rv'],
    });
    const countBeforeBounce = clients.length;
    location.hash = '#/groups';
    dispatchEvent(new HashChangeEvent('hashchange', { newURL: location.href }));
    location.hash = '#/onboarding';
    dispatchEvent(new HashChangeEvent('hashchange', { newURL: location.href }));
    await new Promise((later) => setTimeout(later, 30));
    expect(clients).toHaveLength(countBeforeBounce);

    // Leaving #/onboarding for #/nodes reuses firstHome and adds laptop's Home.
    location.hash = '#/nodes';
    dispatchEvent(new HashChangeEvent('hashchange', { newURL: location.href }));
    await vi.waitFor(() => expect(homes).toHaveLength(2));
    expect(homes[0]).toBe(firstHome);
    expect(homes[1].name).toBe('laptop');

    stop();
  });

  it('enforces one active tab with runTab and renders the other-tab notice with Use here in App', async () => {
    const fakeLocks = createFakeLocks();
    Object.defineProperty(navigator, 'locks', { configurable: true, value: fakeLocks });
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

    const browserDev = await loadDevice({ crypto, idb: () => idb });
    const nodeA = await newDevice();
    const log = new DeviceLog(crypto, browserDev.id);
    await log.apply(
      await signDeviceEntry(browserDev, browserDev.id, 1, 'add', browserDev.id, 'browser', 'Web'),
    );
    await log.apply(
      await signDeviceEntry(browserDev, browserDev.id, 2, 'add', nodeA.id, 'node', 'workstation'),
    );
    await saveOwner(() => idb, log, { [nodeA.id]: ['wss://rv.example/rv'] });

    // First tab acquires the lock and opens signalling; useHere() before any conflict is a no-op.
    const tab1 = start('live');
    tab1.net.useHere?.();
    await vi.waitFor(() => expect(clients.length).toBeGreaterThan(0));
    const tab1Clients = [...clients];
    expect(tab1.net.otherTab).toBe(false);

    // Second tab mounts full App at #/onboarding: shows the other-tab banner, suppresses Onboarding
    // view, and opens zero new signalling clients until "Use here" is clicked.
    location.hash = '#/onboarding';
    const host2 = document.createElement('div');
    document.body.appendChild(host2);
    const app2 = mount(App, { target: host2 });

    await vi.waitFor(() => {
      flushSync();
      expect(host2.querySelector('.tab-ban')?.textContent).toContain(
        'SoloDawn is open in another tab.',
      );
    });
    expect(host2.querySelector('.onboarding')).toBeNull();
    expect(clients).toHaveLength(tab1Clients.length);

    // Navigate to #/groups and click "Use here" in the second tab: steals lock, closes tab1's
    // signalling clients, puts tab1 into otherTab state, and connects tab2.
    location.hash = '#/groups';
    dispatchEvent(new HashChangeEvent('hashchange', { newURL: location.href }));
    flushSync();

    host2.querySelector<HTMLButtonElement>('.tab-ban button')!.click();
    await vi.waitFor(() => {
      flushSync();
      expect(host2.querySelector('.tab-ban')).toBeNull();
      expect(tab1.net.otherTab).toBe(true);
      expect(tab1Clients.every((c) => c.closed)).toBe(true);
      expect(clients.length).toBeGreaterThan(tab1Clients.length);
    });

    tab1.stop();
    expect(tab1.net.otherTab).toBe(false);
    await unmount(app2);
    host2.remove();
  });
});
