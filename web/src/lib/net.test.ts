import { IDBFactory } from 'fake-indexeddb';
import { flushSync, mount, unmount } from 'svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { FakePC, transportStats } from '../../features/peer-fakes';
import App from '../App.svelte';
import Indicator from '../components/nodes/Indicator.svelte';
import { DeviceLog } from './devlog';
import { Home, homes } from './groups.svelte';
import { Device, ed25519, encode, loadDevice } from './identity';
import { readCode, readInvitation } from './join';
import { goldens } from './mock';
import { mockHome, sourceOf, start } from './net.svelte';
import { nodes } from './nodes.svelte';
import * as owner from './owner';
import { loadJoined, loadOwner, saveJoined, saveOwner } from './owner';
import { mac, parseSecret, room, shown } from './pairing';
import { Peer } from './peer';
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
        .map((f) => JSON.parse((f.env as p.Envelope).p) as { t: string; conn: string; sdp: string })
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
  nodes.infos.clear();
  asked.length = 0;
  location.hash = '';
  prefs.lang = 'en';
});

afterEach(() => {
  while (stops.length) stops.pop()!();
  homes.length = 0;
  nodes.infos.clear();
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
  pcIndex: number | undefined,
  stats = transportStats('host', 'host', 0.008),
) {
  client.receive(client.url, { t: 'presence', k: node.id, on: true });
  await vi.waitFor(() => expect(client.offersFor(node.id).length).toBeGreaterThan(0), {
    timeout: 3000,
  });
  const { conn, sdp } = client.offersFor(node.id).at(-1)!;
  const pc = FakePC.made.find(({ local }) => local?.sdp === sdp)!;
  if (pcIndex !== undefined) expect(pc).toBe(FakePC.made[pcIndex]);
  pc.stats = stats;
  const env = await node.sign(
    JSON.stringify({ t: 'answer', n: makeId(), ts: Date.now(), to: browserId, conn, sdp: 'ans' }),
  );
  if (client.known(node.id)) client.receive(client.url, { t: 'from', k: node.id, env });
  await vi.waitFor(() =>
    expect(pc.remote, `answer to connection ${conn} for ${sdp} (closed: ${pc.closes})`).toEqual({
      type: 'answer',
      sdp: 'ans',
    }),
  );
  pc.ctl.open();
  await vi.waitFor(() => expect(pc.ctl.frames().some((f) => f.t === 'hello')).toBe(true));
  return pc;
}

/** The home's list no longer has its one group, and the home says that it did not move. */
async function left(pc: FakePC, home: Home) {
  const syncs = () => pc.ctl.frames().filter((frame) => frame.t === 'sync');
  await vi.waitFor(() => expect(syncs()).not.toHaveLength(0));
  const before = syncs().length;
  pc.ctl.deliver({ t: 'groups', groups: [] });
  await vi.waitFor(() => expect(syncs()).toHaveLength(before + 1));
  pc.ctl.deliver({ re: syncs().at(-1)!.id, err: { code: 'not_found', msg: 'no such group' } });
  await vi.waitFor(() => expect(home.groups).toEqual([]));
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
    v: 3,
    device,
    name,
    version: 'v0.5.0',
    time: Date.now(),
    addrs,
    groups,
    owner: true,
  };
}

it('does not open a superseded route read when storage finishes after leaving and returning', async () => {
  const browser = await loadDevice({ crypto, idb: () => idb });
  const node = await newDevice();
  const log = new DeviceLog(crypto, browser.id);
  await log.apply(
    await signDeviceEntry(browser, browser.id, 1, 'add', browser.id, 'browser', 'Web'),
  );
  await log.apply(
    await signDeviceEntry(browser, browser.id, 2, 'add', node.id, 'node', 'Computer'),
  );
  await saveOwner(() => idb, log, {});
  const state = await loadOwner(crypto, () => idb);
  const reads: ReturnType<typeof Promise.withResolvers<typeof state>>[] = [];
  vi.spyOn(owner, 'loadOwner').mockImplementation(() => {
    const read = Promise.withResolvers<typeof state>();
    reads.push(read);
    return read.promise;
  });
  vi.spyOn(owner, 'loadJoined').mockResolvedValue({});
  vi.stubGlobal('location', new URL('https://client.example/#/groups'));
  startTracked('live');
  await vi.waitFor(() => expect(reads).toHaveLength(1));
  location.hash = '#/onboarding';
  dispatchEvent(new HashChangeEvent('hashchange', { newURL: location.href }));
  location.hash = '#/groups';
  dispatchEvent(new HashChangeEvent('hashchange', { newURL: location.href }));
  expect(reads).toHaveLength(2);
  reads[0].resolve(state);
  await reads[0].promise;
  await Promise.resolve();
  expect(homes).toEqual([]);
  expect(clients).toEqual([]);
  reads[1].resolve(state);
  await vi.waitFor(() => expect(homes.map((home) => home.device)).toEqual([node.id]));
  expect(clients).toHaveLength(1);
});

it('follows moved to an unknown node on the retained signalling URLs, persists it, and follows back without redialling', async () => {
  const browser = await loadDevice({ crypto, idb: () => idb });
  const source = await newDevice();
  const target = await newDevice();
  const log = new DeviceLog(crypto, browser.id);
  await log.apply(
    await signDeviceEntry(browser, browser.id, 1, 'add', browser.id, 'browser', 'Web'),
  );
  await log.apply(
    await signDeviceEntry(browser, browser.id, 2, 'add', source.id, 'node', 'Source'),
  );
  await saveOwner(() => idb, log, { [source.id]: ['wss://old-rv.example/rv'] });
  const client = startTracked('live');
  await vi.waitFor(() => expect(clients).toHaveLength(2));
  const urls = clients.map((way) => way.url);
  const rv = clients.find((way) => way.url === 'wss://old-rv.example/rv')!;
  rv.ready();
  const old = await answerNode(rv, source, browser.id, 0);
  old.ctl.deliver(
    welcomeFrame(1, source.id, 'Source', [{ g: websiteId, name: 'Project', seq: 3 }]),
  );
  await vi.waitFor(() =>
    expect(old.ctl.frames().filter((frame) => frame.t === 'sync')).toHaveLength(1),
  );
  const home = client.net.homes.find((item) => item.device === source.id)!;
  const group = home.groups[0];
  old.ctl.deliver({
    t: 'rows',
    g: websiteId,
    rows: [{ ...JSON.parse(goldens['row-member-human']), user: browser.id }],
  });
  old.ctl.deliver({
    t: 'synced',
    re: old.ctl.frames().find((frame) => frame.t === 'sync')!.id,
    ok: true,
    g: websiteId,
    seq: 3,
  });
  await vi.waitFor(() => expect(group.cursor).toBe(3));
  const reread = vi.spyOn(group, 'sync').mockResolvedValue(undefined);
  const command = group.post('do not replay', []);
  await vi.waitFor(() =>
    expect(old.ctl.frames().filter((frame) => frame.t === 'cmd')).toHaveLength(1),
  );
  const moved = {
    re: old.ctl.frames().find((frame) => frame.t === 'cmd')!.id,
    err: { code: 'moved', msg: 'The group has moved.', home: target.id },
  };
  old.ctl.deliver(moved);
  expect(await command).toEqual(moved);
  expect(reread).toHaveBeenCalledTimes(0);
  reread.mockRestore();
  const destination = client.net.homes.find((item) => item.device === target.id)!;
  expect([group.home, destination.groups[0]]).toEqual([destination, group]);
  expect(home.groups).toEqual([]);
  expect(rv.known(target.id)).toBe(true);
  expect((await loadOwner(crypto, () => idb)).addrs[target.id]).toBeUndefined();
  expect(await loadJoined(() => idb)).toEqual({ [target.id]: { name: '', url: urls[0] } });
  expect((await loadOwner(crypto, () => idb)).log!.valid(target.id)).toBe(false);
  const next = await answerNode(rv, target, browser.id, undefined);
  next.ctl.deliver(welcomeFrame(1, target.id, 'Destination', [], ['wss://new-rv.example/rv']));
  await vi.waitFor(async () =>
    expect((await loadOwner(crypto, () => idb)).addrs[target.id]).toEqual([
      'wss://new-rv.example/rv',
    ]),
  );
  await vi.waitFor(() =>
    expect(next.ctl.frames().filter((frame) => frame.t === 'sync')).toHaveLength(1),
  );
  const first = next.ctl.frames().find((frame) => frame.t === 'sync')!;
  expect({ ...first, id: 1 }).toEqual({
    id: 1,
    t: 'sync',
    g: websiteId,
    since: 3,
    state_only: false,
  });
  next.ctl.deliver({ re: first.id, err: { code: 'not_found', msg: 'not imported' } });
  await vi.waitFor(() => expect(group.paging().failed).toBe(true));
  expect(destination.groups).toEqual([group]);
  next.ctl.deliver({ t: 'groups', groups: [{ g: websiteId, name: 'Imported', seq: 3 }] });
  await vi.waitFor(() =>
    expect(next.ctl.frames().filter((frame) => frame.t === 'sync')).toHaveLength(2),
  );
  const read = next.ctl.frames().filter((frame) => frame.t === 'sync')[1];
  next.ctl.deliver({ t: 'synced', re: read.id, ok: true, g: websiteId, seq: 3 });
  await vi.waitFor(() => expect(group.paging().failed).toBe(false));
  expect([group.name, destination.name]).toEqual(['Imported', 'Destination']);
  expect(next.ctl.frames().filter((frame) => frame.t === 'cmd')).toEqual([]);
  const offers = rv.offersFor(source.id).length;
  const sourceCtl = home.ctl;
  const back = group.post('move back', []);
  await vi.waitFor(() =>
    expect(next.ctl.frames().filter((frame) => frame.t === 'cmd')).toHaveLength(1),
  );
  next.ctl.deliver({
    re: next.ctl.frames().find((frame) => frame.t === 'cmd')!.id,
    err: { code: 'moved', msg: 'moved back', home: source.id },
  });
  await back;
  expect([group.home, home.groups[0]]).toEqual([home, group]);
  expect(destination.groups).toEqual([]);
  expect(rv.offersFor(source.id).length).toBe(offers);
  expect(old.closes).toBe(0);
  expect(home.ctl).toBe(sourceCtl);
  expect(await loadJoined(() => idb)).toEqual({ [target.id]: { name: '', url: urls[0] } });
});

describe('the homes that an own computer knows', () => {
  const other = 'ZG9jcy1ncm91cC0wMDAwMQ';
  const { re: _re, ok: _ok, ...info } = p.decode(p.HostInfo, goldens['host-info-answer']);
  const syncsOf = (pc: FakePC) => pc.ctl.frames().filter((frame) => frame.t === 'sync');

  /** A browser of a person with one computer, which is online, and what its `host.info` says. */
  async function secondBrowser() {
    const browser = await loadDevice({ crypto, idb: () => idb });
    const own = await newDevice();
    const foreign = await newDevice();
    const log = new DeviceLog(crypto, browser.id);
    await log.apply(
      await signDeviceEntry(browser, browser.id, 1, 'add', browser.id, 'browser', 'Web'),
    );
    await log.apply(
      await signDeviceEntry(browser, browser.id, 2, 'add', own.id, 'node', 'Computer'),
    );
    await saveOwner(() => idb, log, {});
    const { net } = startTracked('live');
    await vi.waitFor(() => expect(clients).toHaveLength(1));
    const [rv] = clients;
    rv.ready();
    const ownPC = await answerNode(rv, own, browser.id, undefined);
    ownPC.ctl.deliver(welcomeFrame(1, own.id, 'Computer'));
    await vi.waitFor(() => expect(net.homes[0].ctl).toBeDefined());
    const knows = (homes: p.HostInfo['homes']) =>
      ownPC.ctl.deliver({ ...info, t: 'host.info', homes });
    const homeOf = (id: string) => net.homes.find(({ device }) => device === id);
    return { browser, own, foreign, net, rv, ownPC, knows, homeOf };
  }

  it('are dialled once and remembered; a group of one that is away is shown as away', async () => {
    const { browser, own, foreign, net, rv, ownPC, knows, homeOf } = await secondBrowser();
    expect(rv.known(foreign.id)).toBe(false);
    const entries = [
      { g: websiteId, home: foreign.id, name: 'Website' },
      { g: other, home: own.id, name: 'Own' },
    ];
    knows(entries);
    expect(net.homes.map(({ device }) => device)).toEqual([own.id, foreign.id]);
    const home = homeOf(foreign.id)!;
    expect([home.owner, home.name, home.ctl, net.path(home)]).toEqual([
      false,
      '',
      undefined,
      undefined,
    ]);
    expect(home.groups.map(({ g, name }) => [g, name])).toEqual([[websiteId, 'Website']]);
    // An own computer that names itself is asked, not dialled again.
    expect(homeOf(own.id)!.groups.map(({ g }) => g)).toEqual([other]);
    expect(syncsOf(ownPC)).toMatchObject([{ g: other, since: 0 }]);
    expect(rv.known(foreign.id)).toBe(true);
    const remembered = { [foreign.id]: { name: '', url: rv.url } };
    await vi.waitFor(async () => expect(await loadJoined(() => idb)).toEqual(remembered));
    const save = vi.spyOn(owner, 'saveJoined');
    const closes = vi.spyOn(Peer.prototype, 'close');
    knows(entries);
    expect(net.homes).toHaveLength(2);
    expect(closes).not.toHaveBeenCalled();
    expect(save).not.toHaveBeenCalled();

    const pc = await answerNode(rv, foreign, browser.id, undefined);
    expect(rv.offersFor(foreign.id)).toHaveLength(1);
    const listed = [{ g: websiteId, name: 'Website', seq: 1 }];
    pc.ctl.deliver({ ...welcomeFrame(1, foreign.id, "Q's computer", listed), owner: false });
    await vi.waitFor(() => expect(syncsOf(pc)).toHaveLength(1));
    expect([home.name, home.groups.map(({ g }) => g)]).toEqual(["Q's computer", [websiteId]]);
    knows([...entries, { g: 'ZG9jcy1ncm91cC0wMDAwMw', home: foreign.id, name: 'Docs' }]);
    await vi.waitFor(() => expect(syncsOf(pc)).toHaveLength(2));
    pc.ctl.deliver({ re: syncsOf(pc)[1].id, err: { code: 'forbidden', msg: 'not a member' } });
    await vi.waitFor(() => expect(home.groups.map(({ g }) => g)).toEqual([websiteId]));
    expect(rv.offersFor(foreign.id)).toHaveLength(1);
    expect([pc.closes, ownPC.closes]).toEqual([0, 0]);
  });

  it('keeps a home that no longer speaks to this browser, with its groups as away', async () => {
    const { browser, foreign, rv, knows, homeOf } = await secondBrowser();
    knows([{ g: websiteId, home: foreign.id, name: 'Website' }]);
    const home = homeOf(foreign.id)!;
    const pc = await answerNode(rv, foreign, browser.id, undefined);
    const refuse = (re: number) =>
      pc.ctl.deliver({ re, err: { code: 'version', msg: 'update needed' } });
    refuse(1);
    await vi.waitFor(() => expect(pc.closes).toBe(1));
    expect(pc.ctl.frames()).toHaveLength(1);
    expect([homeOf(foreign.id), home.ctl, home.groups.map(({ g }) => g)]).toEqual([
      home,
      undefined,
      [websiteId],
    ]);
  });

  it('keeps a watched computer that is also the home of a group', async () => {
    const { foreign, net, knows, homeOf } = await secondBrowser();
    const end = net.watchHost!(foreign.id);
    const watched = homeOf(foreign.id)!;
    knows([{ g: websiteId, home: foreign.id, name: 'Website' }]);
    end();
    expect(homeOf(foreign.id)).toBe(watched);
    expect(watched.groups.map(({ g }) => g)).toEqual([websiteId]);
  });

  it('follows a group that its home no longer lists, and dials the new home', async () => {
    const { browser, foreign, rv, knows, homeOf } = await secondBrowser();
    knows([{ g: websiteId, home: foreign.id, name: 'Website' }]);
    const group = homeOf(foreign.id)!.groups[0];
    const pc = await answerNode(rv, foreign, browser.id, undefined);
    pc.ctl.deliver({
      ...welcomeFrame(1, foreign.id, 'P1', [{ g: websiteId, name: 'Website', seq: 1 }]),
      owner: false,
    });
    await vi.waitFor(() => expect(syncsOf(pc)).toHaveLength(1));
    pc.ctl.deliver({ t: 'synced', re: syncsOf(pc)[0].id, ok: true, g: websiteId, seq: 7 });
    await vi.waitFor(() => expect(group.cursor).toBe(7));
    const next = await newDevice();
    pc.ctl.deliver({ t: 'groups', groups: [] });
    await vi.waitFor(() => expect(syncsOf(pc)).toHaveLength(2));
    pc.ctl.deliver({
      re: syncsOf(pc)[1].id,
      err: { code: 'moved', msg: 'The group has moved.', home: next.id },
    });
    await vi.waitFor(() => expect(homeOf(next.id)?.groups).toEqual([group]));
    expect([group.home, homeOf(foreign.id)!.groups]).toEqual([homeOf(next.id), []]);
    const there = await answerNode(rv, next, browser.id, undefined);
    there.ctl.deliver({
      ...welcomeFrame(1, next.id, 'P2', [{ g: websiteId, name: 'Website', seq: 7 }]),
      owner: false,
    });
    await vi.waitFor(() => expect(syncsOf(there)).toMatchObject([{ since: 7 }]));
  });

  it('are not dialled while this browser joins there, and take over a group that was lost', async () => {
    const { browser, foreign, rv, knows, homeOf, net } = await secondBrowser();
    knows([{ g: websiteId, home: foreign.id, name: 'Website' }]);
    const lost = homeOf(foreign.id)!;
    const group = lost.groups[0];
    const restored = await newDevice();
    const secret = parseSecret('7K2M-9QXD-4F8T-WB3N');
    const invitation = readInvitation(`#join=${shown(secret)}&g=${websiteId}&home=${restored.id}`)!;
    const joined = net.join!(invitation, 'Q', new AbortController().signal);
    const id = await room(crypto, secret);
    await vi.waitFor(() => expect(rv.sent).toContainEqual({ t: 'join', room: id }));
    const remote = { role: 'a' as const, k: restored.id, kind: 'node' as const, name: 'P2' };
    rv.receive(rv.url, {
      t: 'room',
      room: id,
      data: { t: 'pair', ...remote, mac: await mac(crypto, secret, remote) },
    });
    const pc = await answerNode(rv, restored, browser.id, undefined);
    knows([
      { g: websiteId, home: foreign.id, name: 'Website' },
      { g: other, home: restored.id, name: 'Docs' },
    ]);
    expect(pc.closes).toBe(0);
    const groups = [{ g: websiteId, name: 'Website', seq: 3 }];
    pc.ctl.deliver({ ...welcomeFrame(1, restored.id, 'P2', groups), owner: false, v: 3 });
    await vi.waitFor(() => expect(pc.ctl.frames().at(-1)?.t).toBe('cmd'));
    pc.ctl.deliver({ t: 'cmd', re: pc.ctl.frames().at(-1)!.id, ok: true });
    const home = homeOf(restored.id)!;
    await vi.waitFor(() => expect(home.groups[0]).toBe(group));
    expect([group.home, lost.groups]).toEqual([home, []]);
    await vi.waitFor(() => expect(syncsOf(pc)[0]).toMatchObject({ g: websiteId, since: 0 }));
    await vi.waitFor(async () =>
      expect((await loadJoined(() => idb))[restored.id]).toEqual({ name: 'P2', url: rv.url }),
    );
    void joined;
  });
});

it('shares a temporary watch-host connection and closes it only after the last watch ends', async () => {
  const browser = await loadDevice({ crypto, idb: () => idb });
  const own = await newDevice();
  const foreign = await newDevice();
  const log = new DeviceLog(crypto, browser.id);
  await log.apply(
    await signDeviceEntry(browser, browser.id, 1, 'add', browser.id, 'browser', 'Web'),
  );
  await log.apply(await signDeviceEntry(browser, browser.id, 2, 'add', own.id, 'node', 'Computer'));
  await saveOwner(() => idb, log, {});
  const { net, stop } = startTracked('live');
  await vi.waitFor(() => expect(net.watchHost).toBeTypeOf('function'));
  clients[0].ready();
  const ownPC = await answerNode(clients[0], own, browser.id, 0);
  ownPC.ctl.deliver(welcomeFrame(1, own.id, 'Computer'));
  await vi.waitFor(() => expect(net.homes[0].ctl).toBeDefined());
  const ownCtl = net.homes[0].ctl;
  net.watchHost!(own.id)();
  expect(net.homes[0].ctl).toBe(ownCtl);
  expect(ownPC.closes).toBe(0);
  // One that is being reconnected anyway keeps its connection and the requests in it.
  net.homes[0].ctl = undefined;
  net.watchHost!(own.id)();
  expect([ownPC.closes, clients[0].offersFor(own.id).length]).toEqual([0, 1]);
  net.homes[0].ctl = ownCtl;
  const early = net.watchHost!(foreign.id);
  expect(clients[0].known(foreign.id)).toBe(true);
  await vi.waitFor(() => expect(clients[0].offersFor(foreign.id)).toHaveLength(1));
  const host = net.homes.find((home) => home.device === foreign.id)!;
  // The computer did not answer: reached for again, it is offered a new connection at once.
  const first = net.watchHost!(foreign.id);
  early();
  expect(net.homes.filter((home) => home.device === foreign.id)).toEqual([host]);
  await vi.waitFor(() => expect(clients[0].offersFor(foreign.id)).toHaveLength(2));
  expect([FakePC.made[1].closes, FakePC.made[2].closes]).toEqual([1, 0]);
  const pc = await answerNode(clients[0], foreign, browser.id, 2);
  pc.ctl.deliver({ ...welcomeFrame(1, foreign.id, 'AI host'), owner: false });
  await vi.waitFor(() => expect(host.ctl).toBeDefined());
  const second = net.watchHost!(foreign.id);
  expect(clients[0].offersFor(foreign.id)).toHaveLength(2);
  first();
  expect([pc.closes, clients[0].known(foreign.id)]).toEqual([0, true]);
  expect(host.ctl).toBeDefined();
  second();
  expect([pc.closes, clients[0].known(foreign.id)]).toEqual([1, false]);
  expect([host.ctl, net.path(host)]).toEqual([undefined, undefined]);
  expect(net.homes.map((home) => home.device)).toEqual([own.id]);
  expect(await loadJoined(() => idb)).toEqual({});
  const last = net.watchHost!(foreign.id);
  expect(net.homes.map((home) => home.device)).toEqual([own.id, foreign.id]);
  expect(net.homes[1]).not.toBe(host);
  stop();
  const kept = elsewhere();
  homes.push(kept);
  last();
  expect(homes).toEqual([kept]);
  expect(net.watchHost).toBeUndefined();
});

it('dials a computer that is paired after a watch of it has ended', async () => {
  const browser = await loadDevice({ crypto, idb: () => idb });
  const own = await newDevice();
  const foreign = await newDevice();
  const log = new DeviceLog(crypto, browser.id);
  await log.apply(
    await signDeviceEntry(browser, browser.id, 1, 'add', browser.id, 'browser', 'Web'),
  );
  await log.apply(await signDeviceEntry(browser, browser.id, 2, 'add', own.id, 'node', 'Computer'));
  await saveOwner(() => idb, log, {});
  const { net } = startTracked('live');
  await vi.waitFor(() => expect(net.watchHost).toBeTypeOf('function'));
  clients[0].ready();
  const ownPC = await answerNode(clients[0], own, browser.id, 0);
  ownPC.ctl.deliver(welcomeFrame(1, own.id, 'Computer'));
  await vi.waitFor(() => expect(net.homes[0].ctl).toBeDefined());
  const end = net.watchHost!(foreign.id);
  await vi.waitFor(() => expect(clients[0].offersFor(foreign.id)).toHaveLength(1));
  end();
  const entry = await signDeviceEntry(browser, browser.id, 3, 'add', foreign.id, 'node', 'AI host');
  ownPC.ctl.deliver({ t: 'devlog', user: browser.id, entries: [entry] });
  await vi.waitFor(() => expect(clients[0].offersFor(foreign.id)).toHaveLength(2));
});

it('reuses and keeps the group home when a watch ends on an already joined node', async () => {
  const browser = await loadDevice({ crypto, idb: () => idb });
  const own = await newDevice();
  const home = await newDevice();
  const log = new DeviceLog(crypto, browser.id);
  await log.apply(
    await signDeviceEntry(browser, browser.id, 1, 'add', browser.id, 'browser', 'Web'),
  );
  await log.apply(await signDeviceEntry(browser, browser.id, 2, 'add', own.id, 'node', 'Computer'));
  await saveOwner(() => idb, log, {});
  await saveJoined(() => idb, { [home.id]: { name: 'Group home', url: 'wss://rv.example/rv' } });
  const { net } = startTracked('live');
  await vi.waitFor(() => expect(net.watchHost).toBeTypeOf('function'));
  clients[0].ready();
  const pc = await answerNode(clients[0], home, browser.id, undefined);
  pc.ctl.deliver({
    ...welcomeFrame(1, home.id, 'Group home', [{ g: websiteId, name: 'Project', seq: 1 }]),
    owner: false,
  });
  const host = net.homes.find((item) => item.device === home.id)!;
  await vi.waitFor(() => expect(host.ctl).toBeDefined());
  const ctl = host.ctl;
  net.watchHost!(home.id)();
  expect(clients[0].offersFor(home.id)).toHaveLength(1);
  expect(pc.closes).toBe(0);
  expect(host.ctl).toBe(ctl);
  expect(host.groups.map((group) => group.g)).toEqual([websiteId]);
  expect(net.homes).toContain(host);
});

it('joins by code on every path, at the home that answers on one of them, and remembers that path', async () => {
  const browser = await loadDevice({ crypto, idb: () => idb });
  const own = await newDevice();
  const home = await newDevice();
  const log = new DeviceLog(crypto, browser.id);
  await log.apply(
    await signDeviceEntry(browser, browser.id, 1, 'add', browser.id, 'browser', 'Web'),
  );
  await log.apply(await signDeviceEntry(browser, browser.id, 2, 'add', own.id, 'node', 'Own node'));
  const page = 'ws://localhost:3000/rv';
  const lan = 'ws://192.0.2.10:47800/rv';
  await saveOwner(() => idb, log, { [own.id]: [lan, page] });
  const client = startTracked('live');
  await vi.waitFor(() => expect(clients.map((way) => way.url)).toEqual([page, lan]));
  const [rv, nodeWay] = clients;
  rv.ready();
  nodeWay.ready();
  const code = readCode('7k2m 9qxd 4f8t wb3n')!;
  const joined = client.net.join!(code, 'Joiner', new AbortController().signal);
  const id = await room(crypto, code.secret);
  const enter = { t: 'join', room: id };
  await vi.waitFor(() => expect(nodeWay.sent).toContainEqual(enter));
  expect(rv.sent.filter((frame) => frame.t === 'join')).toEqual([enter]);
  expect(nodeWay.sent.filter((frame) => frame.t === 'join')).toEqual([enter]);
  expect(rv.known(home.id)).toBe(false);
  nodeWay.receive(nodeWay.url, { t: 'joined', room: id });
  const remote = { role: 'a', k: home.id, kind: 'node', name: 'Foreign home' };
  nodeWay.receive(nodeWay.url, {
    t: 'room',
    room: id,
    data: { t: 'pair', ...remote, mac: await mac(crypto, code.secret, remote) },
  });
  const pc = await answerNode(nodeWay, home, browser.id, undefined);
  expect(rv.known(home.id)).toBe(true);
  pc.ctl.deliver({
    ...welcomeFrame(1, home.id, 'Foreign home'),
    owner: false,
    v: 3,
    join: websiteId,
  });
  await vi.waitFor(() => expect(pc.ctl.frames().at(-1)?.t).toBe('cmd'));
  const command = pc.ctl.frames().at(-1)!;
  expect(JSON.parse(command.env.p)).toMatchObject({ t: 'join', g: websiteId, name: 'Joiner' });
  pc.ctl.deliver({ t: 'groups', groups: [{ g: websiteId, name: 'Shared', seq: 3 }] });
  pc.ctl.deliver({ t: 'cmd', re: command.id, ok: true });
  await expect(joined).resolves.toEqual({ g: websiteId, notifications: [] });
  expect(await loadJoined(() => idb)).toEqual({
    [home.id]: { name: 'Foreign home', url: nodeWay.url },
  });
  expect(homes.find((each) => each.device === home.id)?.groups.map(({ g }) => g)).toEqual([
    websiteId,
  ]);
  expect(rv.known(home.id)).toBe(true);
});

it('joins an invitation without adopting the home and reconnects to it after reload', async () => {
  const browser = await loadDevice({ crypto, idb: () => idb });
  const own = await newDevice();
  const offline = await newDevice();
  const home = await newDevice();
  const log = new DeviceLog(crypto, browser.id);
  await log.apply(
    await signDeviceEntry(browser, browser.id, 1, 'add', browser.id, 'browser', 'Web'),
  );
  await log.apply(await signDeviceEntry(browser, browser.id, 2, 'add', own.id, 'node', 'Own node'));
  await log.apply(
    await signDeviceEntry(browser, browser.id, 3, 'add', offline.id, 'node', 'Offline node'),
  );
  await saveOwner(() => idb, log, {});
  const client = startTracked('live');
  const secret = parseSecret('7K2M-9QXD-4F8T-WB3N');
  const invitation = readInvitation(`#join=${shown(secret)}&g=${websiteId}&home=${home.id}`)!;
  expect(invitation).toEqual({ secret, g: websiteId, home: home.id });
  await expect(
    client.net.join!(invitation, 'Joiner', new AbortController().signal),
  ).rejects.toMatchObject({ step: 'onboarding', code: 'unpaired' });
  await vi.waitFor(() => expect(clients).toHaveLength(1));
  clients[0].ready();
  const ownPC = await answerNode(clients[0], own, browser.id, 0);
  ownPC.ctl.deliver(welcomeFrame(1, own.id, 'Own node'));
  await vi.waitFor(() => expect(homes[0].ctl).toBeDefined());
  expect(ownPC.ctl.frames()).toContainEqual({
    t: 'devlog',
    user: browser.id,
    entries: log.entries,
  });

  const cancel = new AbortController();
  cancel.abort();
  await expect(client.net.join!(invitation, 'Joiner', cancel.signal)).rejects.toMatchObject({
    step: 'room',
    code: 'cancelled',
  });
  expect(clients).toHaveLength(1);

  const joined = client.net.join!(invitation, 'Joiner', new AbortController().signal);
  const rv = clients[0];
  const id = await room(crypto, secret);
  const enter = { t: 'join', room: id };
  await vi.waitFor(() => expect(rv.sent).toContainEqual(enter));
  expect(clients).toEqual([rv]);
  expect(rv.closed).toBe(false);
  expect(rv.known(home.id)).toBe(false);
  expect(rv.known(offline.id)).toBe(true);
  rv.receive(rv.url, { t: 'joined', room: id });
  const self = { role: 'b', k: browser.id, kind: 'browser', name: 'Browser' };
  expect(rv.sent).toContainEqual({
    t: 'room',
    room: id,
    data: { t: 'pair', ...self, mac: await mac(crypto, secret, self) },
  });
  const remote = { role: 'a', k: home.id, kind: 'node', name: 'Foreign home' };
  rv.receive(rv.url, {
    t: 'room',
    room: id,
    data: { t: 'pair', ...remote, mac: await mac(crypto, secret, remote) },
  });
  const pc = await answerNode(rv, home, browser.id, 1);
  expect(rv.known(home.id)).toBe(true);
  pc.ctl.deliver({ ...welcomeFrame(1, home.id, 'Foreign home'), owner: false, v: 3 });
  await vi.waitFor(() => expect(pc.ctl.frames().at(-1)?.t).toBe('cmd'));
  const [hello, devlog, command] = pc.ctl.frames();
  expect(hello).toMatchObject({ t: 'hello', v: 3 });
  expect(devlog).toEqual({ t: 'devlog', user: browser.id, entries: log.entries });
  expect(JSON.parse(command.env.p)).toMatchObject({ t: 'join', g: websiteId, name: 'Joiner' });
  expect(command.env.k).toBe(browser.id);
  const groups = [{ g: websiteId, name: 'Shared', seq: 3 }];
  pc.ctl.deliver({ t: 'devlog', user: home.id, entries: [] });
  pc.ctl.deliver({ t: 'groups', groups: [{ g: makeId(), name: 'Other', seq: 1 }] });
  pc.ctl.deliver({ t: 'groups', groups });
  pc.ctl.deliver({ t: 'cmd', re: command.id, ok: true });
  await vi.waitFor(() => expect(ownPC.ctl.frames().at(-1)?.t).toBe('cmd'));
  const notice = ownPC.ctl.frames().at(-1)!;
  expect(JSON.parse(notice.env.p)).toMatchObject({
    t: 'home.add',
    to: own.id,
    g: websiteId,
    home: home.id,
    name: 'Shared',
    addrs: [rv.url],
  });
  expect(notice.env.k).toBe(browser.id);
  ownPC.ctl.deliver({ t: 'cmd', re: notice.id, ok: true });
  await expect(joined).resolves.toEqual({ g: websiteId, notifications: [] });
  const sharedHome = homes[2];
  expect(sharedHome.owner).toBe(false);
  expect(sharedHome.groups.map((group) => group.g)).toEqual([websiteId]);
  await vi.waitFor(() => expect(pc.ctl.frames().at(-1)?.t).toBe('sync'));
  await left(pc, sharedHome);
  expect((await loadOwner(crypto, () => idb)).log?.entries).toEqual(log.entries);
  expect(await loadJoined(() => idb)).toEqual({ [home.id]: { name: 'Foreign home', url: rv.url } });
  const anotherBrowser = await newDevice();
  const entry = await signDeviceEntry(
    browser,
    browser.id,
    4,
    'add',
    anotherBrowser.id,
    'browser',
    'Another browser',
  );
  const apply = vi.spyOn(DeviceLog.prototype, 'apply');
  ownPC.ctl.deliver({ t: 'devlog', user: home.id, entries: [entry] });
  const updated = { t: 'devlog', user: browser.id, entries: [...log.entries, entry] };
  expect(apply).not.toHaveBeenCalled();
  apply.mockRestore();
  expect(await loadOwner(crypto, () => idb)).toMatchObject({ log: { entries: log.entries } });
  ownPC.ctl.deliver({ t: 'devlog', user: browser.id, entries: [entry] });
  await vi.waitFor(() => expect(pc.ctl.frames()).toContainEqual(updated));

  pc.becomes('failed');
  await vi.waitFor(() => expect(sharedHome.ctl).toBeUndefined());
  const offlineEntry = await signDeviceEntry(browser, browser.id, 5, 'revoke', anotherBrowser.id);
  ownPC.ctl.deliver({ t: 'devlog', user: browser.id, entries: [offlineEntry] });
  await vi.waitFor(() =>
    expect(ownPC.ctl.frames().at(-1)).toEqual({
      ...updated,
      entries: [...updated.entries, offlineEntry],
    }),
  );
  rv.receive(rv.url, { t: 'presence', k: home.id, on: true });
  await vi.waitFor(() => expect(rv.offersFor(home.id)).toHaveLength(2));
  const reconnected = await answerNode(rv, home, browser.id, 2);
  reconnected.ctl.deliver({
    ...welcomeFrame(1, home.id, 'Foreign home', groups),
    owner: false,
    v: 3,
  });
  await vi.waitFor(() =>
    expect(reconnected.ctl.frames()).toContainEqual({
      ...updated,
      entries: [...updated.entries, offlineEntry],
    }),
  );
  await left(reconnected, sharedHome);
  await vi.waitFor(() => expect(client.net.path(sharedHome)).toMatchObject({ kind: 'direct' }));

  const newNode = await newDevice();
  const nodeEntry = await signDeviceEntry(
    browser,
    browser.id,
    6,
    'add',
    newNode.id,
    'node',
    'New computer',
  );
  expect(rv.known(newNode.id)).toBe(false);
  reconnected.ctl.deliver({ t: 'devlog', user: browser.id, entries: [nodeEntry] });
  const extended = [...updated.entries, offlineEntry, nodeEntry];
  await vi.waitFor(() =>
    expect(ownPC.ctl.frames().at(-1)).toEqual({ ...updated, entries: extended }),
  );
  expect(rv.known(newNode.id)).toBe(true);
  expect(homes.at(-1)).toMatchObject({ device: newNode.id, name: 'New computer', owner: true });
  expect((await loadOwner(crypto, () => idb)).log?.entries).toEqual(extended);

  const otherHome = await newDevice();
  const otherInvitation = { ...invitation, home: otherHome.id };
  const abort = new AbortController();
  const other = client.net.join!(otherInvitation, 'Joiner', abort.signal);
  const rejected = expect(other).rejects.toMatchObject({ step: 'hello', code: 'cancelled' });
  const joins = () => rv.sent.filter((frame) => frame.t === 'join');
  await vi.waitFor(() => expect(joins()).toEqual([enter, enter]));
  const otherPair = { ...remote, k: otherHome.id };
  rv.receive(rv.url, {
    t: 'room',
    room: id,
    data: { t: 'pair', ...otherPair, mac: await mac(crypto, secret, otherPair) },
  });
  await vi.waitFor(() => expect(rv.known(otherHome.id)).toBe(true));
  abort.abort();
  await rejected;
  expect(rv.known(otherHome.id)).toBe(false);

  const refused = client.net.join!(invitation, 'Joiner', new AbortController().signal);
  const refusal = expect(refused).rejects.toMatchObject({ step: 'hello', code: 'version' });
  await vi.waitFor(() => expect(joins()).toEqual([enter, enter, enter]));
  const pairing = {
    t: 'room',
    room: id,
    data: { t: 'pair', ...remote, mac: await mac(crypto, secret, remote) },
  };
  rv.receive(rv.url, pairing);
  await vi.waitFor(() => expect(rv.offersFor(home.id)).toHaveLength(3));
  const wrongVersion = await answerNode(rv, home, browser.id, undefined);
  wrongVersion.ctl.deliver({ ...welcomeFrame(1, home.id, 'Foreign home'), owner: false, v: 2 });
  await refusal;
  expect(rv.closed).toBe(false);
  await vi.waitFor(() => expect(rv.offersFor(home.id)).toHaveLength(4));
  const restored = await answerNode(rv, home, browser.id, undefined);
  restored.ctl.deliver({ ...welcomeFrame(1, home.id, 'Foreign home', groups), owner: false, v: 3 });
  await vi.waitFor(() => expect(sharedHome.groups.map(({ g }) => g)).toEqual([websiteId]));

  const pending = client.net.join!(invitation, 'Joiner', new AbortController().signal);
  const cancelled = expect(pending).rejects.toMatchObject({ step: 'hello', code: 'cancelled' });
  await vi.waitFor(() => expect(joins()).toEqual([enter, enter, enter, enter]));
  rv.receive(rv.url, pairing);
  await vi.waitFor(() => expect(rv.offersFor(home.id)).toHaveLength(5));
  await answerNode(rv, home, browser.id, undefined);
  expect(clients).toEqual([rv]);
  location.hash = '#/onboarding';
  dispatchEvent(new HashChangeEvent('hashchange', { newURL: location.href }));
  const framesAfterClose = rv.sent.length;
  await cancelled;
  expect(rv.closed).toBe(true);
  expect(rv.sent).toHaveLength(framesAfterClose);
  client.stop();
  await expect(
    client.net.join!(invitation, 'Joiner', new AbortController().signal),
  ).rejects.toMatchObject({ step: 'onboarding', code: 'unpaired' });
  await saveJoined(() => idb, {
    ...(await loadJoined(() => idb)),
    [own.id]: { name: 'Also joined', url: 'wss://saved.example/rv' },
  });
  const close = vi.spyOn(Peer.prototype, 'close');
  location.hash = '#/groups';
  startTracked('live');
  await vi.waitFor(() =>
    expect(homes.map((node) => node.device)).toEqual([own.id, offline.id, newNode.id, home.id]),
  );
  expect(close).not.toHaveBeenCalled();
  expect(homes[0].owner).toBe(true);
  expect(homes.at(-1)?.owner).toBe(false);
  expect(clients[1].known(home.id)).toBe(true);
  expect(clients[1].known(anotherBrowser.id)).toBe(false);
  expect(clients.slice(1).map((client) => client.url)).toContain('wss://saved.example/rv');
});

describe('sourceOf', () => {
  it.each([
    ['development', '', 'mock'],
    ['development', '?a=1', 'mock'],
    ['development', '?nomockery', 'mock'],
    ['development', '?nomock', 'live'],
    ['development', '?a=1&nomock=', 'live'],
    ['development', '?demo', 'live'],
    ['development', '?demo=fresh', 'live'],
    ['development', '?demonstration', 'mock'],
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
      { t: 'hello', v: 3, kind: 'browser', name: 'development', addrs: [] },
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
    const ctl = home.ctl;
    home.ctl = undefined;
    expect(net.path(home)).toBeUndefined();
    home.ctl = ctl;
    stop();
    expect(homes).toEqual([other]);
    expect(net.path(home)).toBeUndefined();
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
    expect(second.net.devices).toEqual([]);
    second.stop();
    replaySpy.mockRestore();

    // 3. Stopped before loadDevice settles, and when loadDevice rejects.
    const request = vi.spyOn(navigator.locks, 'request');
    const earlyStop = start('live');
    earlyStop.stop();
    await new Promise((later) => setTimeout(later, 20));
    expect(clients).toEqual([]);
    expect(request).not.toHaveBeenCalled();

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

    // The owner's devices are the valid ones of the log, in its order: not the revoked node.
    const added = (env: { p: string }) => (JSON.parse(env.p) as { ts: number }).ts;
    const by = browserDev.id;
    const web = { id: browserDev.id, kind: 'browser', name: 'Web', by, self: true };
    const workstation = { id: nodeA.id, kind: 'node', name: 'workstation', by, self: false };
    expect(net.devices).toEqual([
      { ...web, added: added(log.entries[0]) },
      { ...workstation, added: added(log.entries[1]) },
    ]);

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

    // One of the two homes is reached: the indicator says some are not.
    await vi.waitFor(() => {
      flushSync();
      expect(host.querySelector('.conn')?.getAttribute('data-st')).toBe('some');
    });
    homes.splice(homes.indexOf(preExisting), 1);

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
    homes.unshift(preExisting);

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

    // A browser that a node added is among the owner's devices once its entry arrives.
    const phone = await newDevice();
    const e6 = await signDeviceEntry(
      nodeA,
      browserDev.id,
      6,
      'add',
      phone.id,
      'browser',
      'Browser',
    );
    pcA.ctl.deliver({ t: 'devlog', user: browserDev.id, entries: [e6] });
    await vi.waitFor(() => expect(net.devices).toHaveLength(4));
    expect(net.devices![3]).toEqual({
      id: phone.id,
      kind: 'browser',
      name: 'Browser',
      by: nodeA.id,
      added: added(e6),
      self: false,
    });
    expect(net.devices!.map((device) => device.name)).toEqual([
      'Web',
      'workstation',
      'laptop',
      'Browser',
    ]);

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

  it('forwards incoming host.info pushes to nodes.infos for connected owner nodes', async () => {
    const browserDev = await loadDevice({ crypto, idb: () => idb });
    const nodeA = await newDevice();
    const log = new DeviceLog(crypto, browserDev.id);
    await log.apply(
      await signDeviceEntry(browserDev, browserDev.id, 1, 'add', browserDev.id, 'browser', 'Web'),
    );
    await log.apply(
      await signDeviceEntry(browserDev, browserDev.id, 2, 'add', nodeA.id, 'node', 'Studio'),
    );
    await saveOwner(() => idb, log, { [nodeA.id]: ['wss://rv.example/rv'] });
    const client = startTracked('live');
    await vi.waitFor(() =>
      expect(clients.some((way) => way.url === 'wss://rv.example/rv')).toBe(true),
    );
    const rv = clients.find((way) => way.url === 'wss://rv.example/rv')!;
    rv.ready();
    const pc = await answerNode(rv, nodeA, browserDev.id, 0);
    const hello = pc.ctl.frames().find((frame) => frame.t === 'hello')!;
    pc.ctl.deliver(welcomeFrame(hello.id, nodeA.id, 'Studio'));
    await vi.waitFor(() => expect(client.net.homes[0]?.ctl).toBeDefined());
    const home = client.net.homes[0];
    expect(home.owner).toBe(true);
    const base = p.decode(p.HostInfoPush, goldens['host-info-push']);
    pc.ctl.deliver({
      ...base,
      device: nodeA.id,
      name: 'Studio',
      agents: base.agents.map((a) => (a.agent === 'claude' ? { ...a, login_ok: false } : a)),
    });
    expect(nodes.infos.get(home)?.agents.find((a) => a.agent === 'claude')?.login_ok).toBe(false);
    expect(nodes.infos.get(home)?.device).toBe(nodeA.id);
  });

  it('enforces one active tab with runTab and renders the other-tab notice with Use here in App', async () => {
    const fakeLocks = createFakeLocks();
    const steals = [Promise.withResolvers<void>(), Promise.withResolvers<void>()];
    let nextSteal = 0;
    Object.defineProperty(navigator, 'locks', {
      configurable: true,
      value: {
        async request(...args: Parameters<typeof fakeLocks.request>) {
          if (args[1].steal) await steals[Math.min(nextSteal++, 1)].promise;
          return fakeLocks.request(...args);
        },
      },
    });
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
    steals[0].resolve();
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
