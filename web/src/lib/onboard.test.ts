import { IDBFactory } from 'fake-indexeddb';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { DeviceLog } from './devlog';
import { Device, ed25519, encode, loadDevice, verifyEnvelope } from './identity';
import {
  accountLabel,
  accounts,
  capturePairHash,
  cliName,
  PairingSession,
  promptFor,
  setAccount,
  signalUrlOf,
  takeLinkSecret,
} from './onboard.svelte';
import { loadOwner, saveOwner } from './owner';
import { mac, newSecret, parseSecret, room, shown, verifyPair } from './pairing';
import type { Listener, Options, Path } from './peer';
import { choose } from './prefs.svelte';
import type { Ctl, Devlog, HostInfo, Pair, Push, Welcome } from './proto';

interface FakePeer {
  target: string;
  listener: Listener;
  onpath: (path: Path | undefined) => void;
  sent: Devlog[];
}

interface FakeHub {
  options: Options;
  sent: { url: string; frame: unknown }[];
  peers: FakePeer[];
  closed: number;
}

const hubs = vi.hoisted(() => [] as FakeHub[]);
const mockPushes = vi.hoisted(() => [] as ((push: Push) => void)[]);

const makeCtl = (fn: (req: { t: string }) => Promise<unknown>): Ctl => ({
  request: vi.fn(fn) as unknown as Ctl['request'],
});

vi.mock('./peer', () => ({
  connect: (options: Options) => {
    const hub: FakeHub = { options, sent: [], peers: [], closed: 0 };
    hubs.push(hub);
    return {
      send: (url: string, frame: unknown) => hub.sent.push({ url, frame }),
      close: () => {
        hub.closed += 1;
      },
      peer: (
        target: string,
        listen: () => Listener,
        onpath: (path: Path | undefined) => void = () => {},
      ) => {
        const entry: FakePeer = { target, listener: listen(), onpath, sent: [] };
        hub.peers.push(entry);
        return { send: (frame: Devlog) => entry.sent.push(frame) };
      },
    };
  },
}));

vi.mock('./mock', async (actual) => {
  const real = await actual<typeof import('./mock')>();
  return {
    ...real,
    mockCtl: (onpush: (push: Push) => void) => {
      mockPushes.push(onpush);
      return real.mockCtl(onpush);
    },
  };
});

const newDevice = async () =>
  Device.of(crypto, await crypto.subtle.generateKey(ed25519, false, ['sign', 'verify']));
const makeId = () => encode(crypto.getRandomValues(new Uint8Array(16)));

async function signDeviceAdd(
  signer: Device,
  user: string,
  ver: number,
  dev: string,
  kind: 'node' | 'browser',
  name: string,
) {
  return signer.sign(
    JSON.stringify({
      t: 'device',
      n: makeId(),
      ts: Date.now(),
      user,
      ver,
      op: 'add',
      dev,
      kind,
      name,
    }),
  );
}

function welcomeOf(device: string, owner: boolean, name = 'MacBook'): Welcome {
  return {
    t: 'welcome',
    re: 1,
    ok: true,
    v: 1,
    device,
    name,
    version: 'v0.4.0',
    time: 1000,
    addrs: ['wss://rv.example/rv'],
    groups: [],
    owner,
  };
}

function hostInfoOf(device: string, agents: HostInfo['agents'], name = 'MacBook'): HostInfo {
  return {
    t: 'host.info',
    re: 2,
    ok: true,
    device,
    name,
    os: 'macOS 15',
    version: 'v0.4.0',
    exe: '/usr/local/bin/solodawn',
    open: 0,
    max_open: 4,
    offline: false,
    update: null,
    homes: [],
    relay: null,
    agents,
    settings: {
      retention_days: 90,
      idle_minutes: 15,
      default_level: 3,
      all_level3: false,
      approvers: [],
    },
  };
}

let idb: IDBFactory;

beforeEach(() => {
  hubs.length = 0;
  mockPushes.length = 0;
  idb = new IDBFactory();
  vi.stubGlobal('indexedDB', idb);
  location.hash = '';
  choose('lang', 'en');
});

afterEach(() => {
  vi.useRealTimers();
  vi.unstubAllEnvs();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
  location.hash = '';
  localStorage.clear();
});

describe('labels, URLs, prompts and #pair= capture', () => {
  it('maps known CLI and account identifiers and keeps unknown ones verbatim', () => {
    expect(accounts).toEqual(['personal', 'team', 'api_key']);
    expect(cliName('claude')).toBe('Claude Code');
    expect(cliName('codex')).toBe('Codex');
    expect(cliName('custom')).toBe('custom');

    choose('lang', 'en');
    expect(accountLabel('personal')).toBe('Personal subscription');
    expect(accountLabel('team')).toBe('Team plan');
    expect(accountLabel('api_key')).toBe('API key');
    expect(accountLabel('enterprise')).toBe('enterprise');

    choose('lang', 'zh-CN');
    expect(accountLabel('personal')).toBe('个人订阅');
    expect(accountLabel('team')).toBe('团队方案');
    expect(accountLabel('api_key')).toBe('API key');
  });

  it('always emits --rendezvous with ws: or wss: matching the page origin', () => {
    const code = '7K2M-9QXD-4F8T-WB3N';
    expect(signalUrlOf({ protocol: 'https:', host: 'solodawn.org' })).toBe('wss://solodawn.org/rv');
    expect(signalUrlOf({ protocol: 'http:', host: 'localhost:47800' })).toBe(
      'ws://localhost:47800/rv',
    );
    expect(signalUrlOf()).toBe(`ws://${location.host}/rv`);

    const enPrompt = promptFor('en', code, {
      origin: 'https://solodawn.org',
      protocol: 'https:',
      host: 'solodawn.org',
    });
    expect(enPrompt).toContain('https://solodawn.org/install.md');
    expect(enPrompt).toContain('git config --global user.email');
    expect(enPrompt).toContain(code);
    expect(enPrompt).toContain('--rendezvous wss://solodawn.org/rv');

    const zhPrompt = promptFor('zh-CN', code, {
      origin: 'http://127.0.0.1:47800',
      protocol: 'http:',
      host: '127.0.0.1:47800',
    });
    expect(zhPrompt).toContain('http://127.0.0.1:47800/install.md');
    expect(zhPrompt).toContain('git config --global user.email');
    expect(zhPrompt).toContain(code);
    expect(zhPrompt).toContain('--rendezvous ws://127.0.0.1:47800/rv');

    expect(promptFor('en', code)).toContain(`${location.origin}/install.md`);
  });

  it('captures only #pair=<secret> fragments and claims each secret once', () => {
    location.hash = '#/groups';
    capturePairHash();
    expect(location.hash).toBe('#/groups');
    expect(takeLinkSecret()).toBeUndefined();

    location.hash = '#pair=7K2M%2D9QXD-4F8T-WB3N';
    capturePairHash();
    expect(location.hash).toBe('#/onboarding');
    expect(takeLinkSecret()).toBe('7K2M-9QXD-4F8T-WB3N');
    expect(takeLinkSecret()).toBeUndefined();

    location.hash = '#pair=%E0%A4%A';
    expect(takeLinkSecret()).toBe('%E0%A4%A');
    expect(location.hash).toBe('#/onboarding');
    expect(takeLinkSecret()).toBeUndefined();
  });
});

describe('setAccount', () => {
  it('signs agent.set and returns the node answer or lost when disconnected', async () => {
    const signer = await newDevice();
    const target = await newDevice();
    expect(await setAccount(undefined, signer, target.id, 'claude', 'personal')).toStrictEqual({
      re: 0,
      err: { code: 'lost', msg: 'connection lost' },
    });

    const sent: unknown[] = [];
    const okCtl = {
      request: vi.fn(async (req: unknown) => {
        sent.push(req);
        return { re: 1, ok: true };
      }),
    } as unknown as Ctl;

    const before = Date.now();
    const res = await setAccount(okCtl, signer, target.id, 'codex', 'api_key');
    expect(res).toStrictEqual({ re: 1, ok: true });
    const frame = sent[0] as { t: string; env: { p: string; k: string; s: string } };
    expect(frame.t).toBe('cmd');
    expect(await verifyEnvelope(crypto, frame.env)).toBe(true);
    const payload = JSON.parse(frame.env.p) as Record<string, unknown>;
    expect(payload).toMatchObject({
      t: 'agent.set',
      to: target.id,
      agent: 'codex',
      account: 'api_key',
    });
    expect(typeof payload.n).toBe('string');
    expect(payload.ts as number).toBeGreaterThanOrEqual(before);

    const dropCtl = {
      request: () => Promise.reject(new Error('closed')),
    } as unknown as Ctl;
    expect(await setAccount(dropCtl, signer, target.id, 'codex', 'team')).toStrictEqual({
      re: 0,
      err: { code: 'lost', msg: 'connection lost' },
    });
  });
});

describe('PairingSession mock, countdown and storage checks', () => {
  it('loads mock state, reacts to host.info pushes, and declares accounts', async () => {
    const session = new PairingSession('mock');
    await vi.waitFor(() => expect(session.stage).toBe('accounts'));
    expect(session.role).toBe('a');
    expect(session.code).toMatch(/^([0-9A-HJKMNP-TV-Z]{4}-){3}[0-9A-HJKMNP-TV-Z]{4}$/);
    expect(session.enrolled).toBe(true);
    expect(session.browserFingerprint).toMatch(/^[0-9A-F]{4} [0-9A-F]{4} [0-9A-F]{4}$/);
    expect(session.node?.name).toBe('workstation');
    expect(session.path?.kind).toBe('direct');
    expect(session.path?.rtt).toBe(12);
    expect(session.agents?.map((a) => [a.agent, a.account])).toEqual([
      ['claude', 'personal'],
      ['codex', null],
    ]);

    await session.setAccount('codex', 'team');
    expect(session.agents?.map((a) => [a.agent, a.account])).toEqual([
      ['claude', 'personal'],
      ['codex', 'team'],
    ]);
    expect(session.stage).toBe('ready');

    const pushInfo = {
      ...hostInfoOf(session.node!.id, [], 'renamed-mac'),
      re: undefined,
      ok: undefined,
    };
    mockPushes[0](pushInfo as never);
    expect(session.node?.name).toBe('renamed-mac');
    expect(session.agents).toEqual([]);
    session.destroy();
  });

  it('stops mock setup cleanly on destroy or blocked IndexedDB, and skips mock outside DEV', async () => {
    const early = new PairingSession('mock');
    early.destroy();
    await new Promise((r) => setTimeout(r, 20));
    expect(early.node).toBeUndefined();

    vi.stubGlobal('indexedDB', {
      open: () => {
        throw new DOMException('Blocked', 'SecurityError');
      },
    });
    const blockedMock = new PairingSession('mock');
    await new Promise((r) => setTimeout(r, 20));
    expect(blockedMock.node).toBeUndefined();
    blockedMock.destroy();

    vi.stubGlobal('indexedDB', idb);
    vi.stubEnv('DEV', false);
    const prod = new PairingSession('mock');
    await vi.waitFor(() => expect(hubs).toHaveLength(1));
    prod.destroy();
  });

  it('counts down by whole seconds, expires at 0, and clamps past-deadline jumps to 0', async () => {
    vi.useFakeTimers();
    vi.setSystemTime(1_000_000);
    const session = new PairingSession('live');
    await vi.waitFor(() => expect(hubs).toHaveLength(1));
    await vi.advanceTimersByTimeAsync(0);
    expect(session.remaining).toBe(600);
    expect(session.code).not.toBe('');

    vi.setSystemTime(1_000_000 + 598_500);
    await vi.advanceTimersByTimeAsync(1000);
    expect(session.remaining).toBe(1);
    expect(session.stage).toBe('waiting');

    vi.setSystemTime(1_000_000 + 605_000);
    await vi.advanceTimersByTimeAsync(1000);
    expect(session.remaining).toBe(0);
    expect(session.stage).toBe('expired');
    expect(session.code).toBe('');
    expect(hubs[0].closed).toBe(1);
  });

  it('distinguishes StorageBlocked from other loadOwner failures and bad #pair= secrets', async () => {
    location.hash = '#pair=not-valid';
    const bad = new PairingSession('live');
    expect(bad.role).toBe('b');
    expect(bad.stage).toBe('failed');
    expect(bad.error).toStrictEqual({ step: 1, key: 'bad_secret', code: undefined });
    bad.destroy();

    vi.stubGlobal('indexedDB', {
      open: () => {
        throw new DOMException('Blocked', 'SecurityError');
      },
    });
    const noStorage = new PairingSession('live');
    await vi.waitFor(() => expect(noStorage.stage).toBe('failed'));
    expect(noStorage.error).toStrictEqual({ step: 1, key: 'storage', code: undefined });
    noStorage.destroy();

    const newerIdb = new IDBFactory();
    await loadDevice({ crypto, idb: () => newerIdb });
    const realOpen = newerIdb.open.bind(newerIdb);
    let opens = 0;
    vi.spyOn(newerIdb, 'open').mockImplementation(((name: string, version?: number) => {
      opens += 1;
      return opens === 1 ? realOpen(name, version) : realOpen(name, 99);
    }) as typeof newerIdb.open);
    await new Promise<void>((resolve) => {
      const bump = realOpen('solodawn', 100);
      bump.onsuccess = () => {
        bump.result.close();
        resolve();
      };
    });
    opens = 0;
    vi.spyOn(newerIdb, 'open').mockImplementation(((name: string) =>
      opens++ === 0 ? realOpen(name) : realOpen(name, 1)) as typeof newerIdb.open);
    vi.stubGlobal('indexedDB', newerIdb);
    const otherErr = new PairingSession('live');
    await vi.waitFor(() => expect(otherErr.stage).toBe('failed'));
    expect(otherErr.error).toStrictEqual({ step: 1, key: 'lost', code: undefined });
    otherErr.destroy();
  });
});

describe('PairingSession live role a and role b flows', () => {
  it('enrols a first node and then a second node, filtering room messages and known keys', async () => {
    let persistCalls = 0;
    Object.defineProperty(navigator, 'storage', {
      configurable: true,
      value: {
        persist: async () => {
          persistCalls += 1;
          throw new Error('ignored');
        },
      },
    });

    const node1 = await newDevice();
    const node2 = await newDevice();
    const stranger = await newDevice();

    const session = new PairingSession('live');
    await vi.waitFor(() => expect(hubs).toHaveLength(1));
    const hub = hubs[0];
    expect(hub.options.urls).toStrictEqual([`ws://${location.host}/rv`]);
    expect(hub.options.name).toBe('Browser');
    expect(hub.options.known(stranger.id)).toBe(false);

    const secret = parseSecret(session.code);
    const roomId = await room(crypto, secret);
    const url = hub.options.urls[0];

    hub.options.heard!(url, { t: 'presence', k: node1.id, on: true });
    hub.options.heard!(url, { t: 'ready', ice: [], ttl: 60 });
    expect(hub.sent).toStrictEqual([{ url, frame: { t: 'join', room: roomId } }]);

    hub.options.heard!(url, { t: 'joined', room: 'other-room' });
    expect(hub.sent).toHaveLength(1);

    hub.options.heard!(url, { t: 'joined', room: roomId });
    expect(hub.sent).toHaveLength(2);
    const ourPair = (hub.sent[1].frame as { data: Pair }).data;
    expect(await verifyPair(crypto, secret, ourPair, 'a')).toBe(true);

    // Ignore wrong room, non-node kind, wrong role, and forged mac; dial only once on race.
    const validSelf = { role: 'b' as const, k: node1.id, kind: 'node' as const, name: 'mac-1' };
    const validPair: Pair = { t: 'pair', ...validSelf, mac: await mac(crypto, secret, validSelf) };
    const wrongRole = { role: 'a' as const, k: node1.id, kind: 'node' as const, name: 'mac-1' };
    hub.options.heard!(url, { t: 'room', room: 'other', data: validPair });
    hub.options.heard!(url, { t: 'room', room: roomId, data: ourPair });
    hub.options.heard!(url, {
      t: 'room',
      room: roomId,
      data: { t: 'pair', ...wrongRole, mac: await mac(crypto, secret, wrongRole) },
    });
    hub.options.heard!(url, { t: 'room', room: roomId, data: { ...validPair, mac: 'bad' } });
    await new Promise((r) => setTimeout(r, 15));
    expect(hub.peers).toHaveLength(0);

    hub.options.heard!(url, { t: 'room', room: roomId, data: validPair });
    hub.options.heard!(url, { t: 'room', room: roomId, data: validPair });
    await vi.waitFor(() => expect(hub.peers).toHaveLength(1));
    expect(session.stage).toBe('linking');
    expect(session.node?.id).toBe(node1.id);
    expect(hub.options.known(node1.id)).toBe(true);

    // Once paired, further ready frames do not join another room.
    hub.options.heard!(url, { t: 'ready', ice: [], ttl: 60 });
    expect(hub.sent).toHaveLength(2);

    const peer = hub.peers[0];
    peer.onpath({ kind: 'relay', rtt: 45, since: 1 });
    expect(session.path?.kind).toBe('relay');

    const requests: unknown[] = [];
    const ctl = makeCtl(async (req) => {
      requests.push(req);
      if (req.t === 'hello') {
        return 'v' in req && req.v === 2
          ? { err: { code: 'version', msg: 'unsupported version' } }
          : welcomeOf(node1.id, true, 'Mac One');
      }
      return hostInfoOf(node1.id, [
        {
          agent: 'claude',
          cli_version: '1.0',
          account: null,
          login_ok: true,
          driver: { accounts: ['api_key'], takeover: 'none' },
          models: [],
          caps: {},
          groups: [],
        },
        {
          agent: 'codex',
          cli_version: '1.0',
          account: 'api_key',
          login_ok: true,
          driver: { accounts: ['api_key'], takeover: 'none' },
          models: [],
          caps: {},
          groups: [],
        },
      ]);
    });

    peer.listener.online(ctl, welcomeOf(node1.id, false, 'Mac One'));
    await vi.waitFor(() => expect(session.stage).toBe('accounts'));
    expect(peer.sent).toHaveLength(1);
    expect(peer.sent[0].entries).toHaveLength(2);
    expect(persistCalls).toBe(1);
    expect(session.code).toBe('');
    expect(session.enrolled).toBe(true);

    const hello = { t: 'hello', kind: 'browser', name: 'Browser', addrs: [] };
    expect(requests.slice(0, 2)).toEqual([
      { ...hello, v: 2 },
      { ...hello, v: 1 },
    ]);

    // Late signalling frames after enrolment are ignored; non-onboarding pushes and devlog in role a are ignored.
    hub.options.heard!(url, { t: 'ready', ice: [], ttl: 60 });
    peer.listener.push({ t: 'groups', groups: [] });
    peer.listener.push({ t: 'devlog', user: node1.id, entries: [] });
    peer.listener.online(ctl, welcomeOf(node1.id, true, 'Mac One v2'));
    expect(session.node?.name).toBe('Mac One v2');
    expect(peer.sent).toHaveLength(1);
    peer.listener.offline();
    expect(session.stage).toBe('accounts');

    // Step-4 refusal keeps connection alive; retry clears error; lost connection fails session.
    let nextCmd: unknown = { re: 3, err: { code: 'bad_request', msg: 'bad' } };
    const cmdCtl = makeCtl(async () => nextCmd);
    peer.listener.online(cmdCtl, welcomeOf(node1.id, true, 'Mac One v2'));
    await session.setAccount('claude', 'personal');
    expect(session.error).toStrictEqual({ step: 4, key: 'refused', code: 'bad_request' });
    expect(session.stage).toBe('accounts');

    nextCmd = { re: 4, ok: true };
    await session.setAccount('claude', 'personal');
    expect(session.error).toBeUndefined();
    expect(session.stage).toBe('ready');

    const failCtl = makeCtl(() => Promise.reject(new Error('gone')));
    peer.listener.online(failCtl, welcomeOf(node1.id, true, 'Mac One v2'));
    await session.setAccount('claude', 'team');
    expect(session.stage).toBe('failed');
    expect(session.error).toStrictEqual({ step: 4, key: 'lost', code: undefined });

    // Pairing a second computer appends entry 3 to the stored owner log; re-pairing node2 adds nothing.
    await session.start();
    await vi.waitFor(() => expect(hubs).toHaveLength(2));
    const hub2 = hubs[1];
    expect(hub2.options.known(node1.id)).toBe(true);
    expect(hub2.options.known(node2.id)).toBe(false);

    const secret2 = parseSecret(session.code);
    const room2 = await room(crypto, secret2);
    const self2 = { role: 'b' as const, k: node2.id, kind: 'node' as const, name: 'mac-2' };
    hub2.options.heard!(url, {
      t: 'room',
      room: room2,
      data: { t: 'pair', ...self2, mac: await mac(crypto, secret2, self2) },
    });
    await vi.waitFor(() => expect(hub2.peers).toHaveLength(1));
    const ctl2 = makeCtl(async (req) =>
      req.t === 'hello' ? welcomeOf(node2.id, true, 'Mac Two') : hostInfoOf(node2.id, []),
    );
    hub2.peers[0].listener.online(ctl2, welcomeOf(node2.id, false, 'Mac Two'));
    await vi.waitFor(() => expect(session.stage).toBe('ready'));
    expect(hub2.peers[0].sent[0].entries).toHaveLength(3);

    // Re-pairing an already valid node sends the 3-entry log unchanged.
    await session.start();
    await vi.waitFor(() => expect(hubs).toHaveLength(3));
    const secret3 = parseSecret(session.code);
    const room3 = await room(crypto, secret3);
    hubs[2].options.heard!(url, {
      t: 'room',
      room: room3,
      data: { t: 'pair', ...self2, mac: await mac(crypto, secret3, self2) },
    });
    await vi.waitFor(() => expect(hubs[2].peers).toHaveLength(1));
    hubs[2].peers[0].listener.online(ctl2, welcomeOf(node2.id, false, 'Mac Two'));
    await vi.waitFor(() => expect(session.stage).toBe('ready'));
    expect(hubs[2].peers[0].sent[0].entries).toHaveLength(3);
    session.destroy();
  });

  it('adopts a device log in role b once log.valid(me) holds, and handles welcome.owner shortcuts', async () => {
    const owner = await newDevice();
    const node = await newDevice();
    const link1 = newSecret(crypto);
    location.hash = `#pair=${shown(link1)}`;

    const session = new PairingSession('live');
    await vi.waitFor(() => expect(hubs).toHaveLength(1));
    expect(session.role).toBe('b');
    const browserDev = await loadDevice({ crypto, idb: () => idb });
    const room1 = await room(crypto, link1);
    const nodeSelf = { role: 'a' as const, k: node.id, kind: 'node' as const, name: 'desk' };
    hubs[0].options.heard!(hubs[0].options.urls[0], {
      t: 'room',
      room: room1,
      data: { t: 'pair', ...nodeSelf, mac: await mac(crypto, link1, nodeSelf) },
    });
    await vi.waitFor(() => expect(hubs[0].peers).toHaveLength(1));

    const ctl = makeCtl(async (req) =>
      req.t === 'hello' ? welcomeOf(node.id, true) : hostInfoOf(node.id, []),
    );
    const peer = hubs[0].peers[0];
    peer.listener.online(ctl, welcomeOf(node.id, false));

    // Frame 1 omits this browser; frame 2 adds it, signed by another owner device.
    const e1 = await signDeviceAdd(owner, owner.id, 1, owner.id, 'browser', 'main');
    const e2 = await signDeviceAdd(owner, owner.id, 2, node.id, 'node', 'desk');
    const e3 = await signDeviceAdd(owner, owner.id, 3, browserDev.id, 'browser', 'Browser');
    peer.listener.push({ t: 'devlog', user: owner.id, entries: [e1, e2] });
    await new Promise((r) => setTimeout(r, 15));
    expect(session.enrolled).toBe(false);

    peer.listener.push({ t: 'devlog', user: owner.id, entries: [e3] });
    await vi.waitFor(() => expect(session.stage).toBe('ready'));
    const stored = await loadOwner(crypto, () => idb);
    expect(stored.log?.valid(browserDev.id)).toBe(true);

    // Role b when node already welcomes with owner: true finishes immediately;
    // when welcome.owner is false and browser already holds an ownerLog, fails unadopted.
    const link2 = newSecret(crypto);
    location.hash = `#pair=${shown(link2)}`;
    await session.start();
    await vi.waitFor(() => expect(hubs).toHaveLength(2));
    const room2 = await room(crypto, link2);
    hubs[1].options.heard!(hubs[1].options.urls[0], {
      t: 'room',
      room: room2,
      data: { t: 'pair', ...nodeSelf, mac: await mac(crypto, link2, nodeSelf) },
    });
    await vi.waitFor(() => expect(hubs[1].peers).toHaveLength(1));
    hubs[1].peers[0].listener.online(ctl, welcomeOf(node.id, false));
    await vi.waitFor(() => expect(session.stage).toBe('failed'));
    expect(session.error).toStrictEqual({ step: 3, key: 'unadopted', code: undefined });

    // Fresh browser with welcome.owner === true skips devlog wait, even without navigator.storage.
    Object.defineProperty(navigator, 'storage', { configurable: true, value: undefined });
    idb = new IDBFactory();
    vi.stubGlobal('indexedDB', idb);
    const link3 = newSecret(crypto);
    location.hash = `#pair=${shown(link3)}`;
    const quick = new PairingSession('live');
    await vi.waitFor(() => expect(hubs).toHaveLength(3));
    const room3 = await room(crypto, link3);
    hubs[2].options.heard!(hubs[2].options.urls[0], {
      t: 'room',
      room: room3,
      data: { t: 'pair', ...nodeSelf, mac: await mac(crypto, link3, nodeSelf) },
    });
    await vi.waitFor(() => expect(hubs[2].peers).toHaveLength(1));
    hubs[2].peers[0].listener.online(ctl, welcomeOf(node.id, true));
    await vi.waitFor(() => expect(quick.stage).toBe('ready'));
    quick.destroy();
    // Stale callbacks after destroy leave state untouched.
    hubs[2].peers[0].onpath({ kind: 'direct', rtt: 5, since: 2 });
    hubs[2].peers[0].listener.online(ctl, welcomeOf(node.id, true, 'stale'));
    hubs[2].peers[0].listener.push({ t: 'groups', groups: [] });
    hubs[2].peers[0].listener.offline();
    hubs[2].peers[0].listener.refused!('forbidden');
    expect(quick.stage).toBe('ready');
    expect(quick.path).toBeUndefined();
    session.destroy();
  });

  it('reports peer refusals, bad devlogs, confirmOwner failures, offline drops and host.info errors', async () => {
    const node = await newDevice();

    async function dialRoleB() {
      idb = new IDBFactory();
      vi.stubGlobal('indexedDB', idb);
      const link = newSecret(crypto);
      location.hash = `#pair=${shown(link)}`;
      const s = new PairingSession('live');
      const count = hubs.length + 1;
      await vi.waitFor(() => expect(hubs).toHaveLength(count));
      const hub = hubs.at(-1)!;
      const roomId = await room(crypto, link);
      const self = { role: 'a' as const, k: node.id, kind: 'node' as const, name: 'desk' };
      hub.options.heard!(hub.options.urls[0], {
        t: 'room',
        room: roomId,
        data: { t: 'pair', ...self, mac: await mac(crypto, link, self) },
      });
      await vi.waitFor(() => expect(hub.peers).toHaveLength(1));
      return { session: s, peer: hub.peers[0] };
    }

    // 1. setAccount before agents or node are known is a no-op; peer refusal fails step 3.
    {
      const { session } = await dialRoleB();
      await session.setAccount('claude', 'personal');
      expect(session.stage).toBe('linking');
      session.destroy();
    }
    {
      const idle = new PairingSession('live');
      await idle.setAccount('claude', 'personal');
      expect(idle.stage).toBe('waiting');
      idle.destroy();
    }
    // Destroy mid-flight during live startup, dial, enrol, finishEnrolment and fetchHostInfo.
    {
      const cancelledStart = new PairingSession('live');
      cancelledStart.destroy();
      await new Promise((r) => setTimeout(r, 15));
      expect(cancelledStart.stage).toBe('waiting');
    }
    {
      Object.defineProperty(navigator, 'storage', { configurable: true, value: {} });
      const s = new PairingSession('live');
      const count = hubs.length + 1;
      await vi.waitFor(() => expect(hubs).toHaveLength(count));
      const hub = hubs.at(-1)!;
      const secret = parseSecret(s.code);
      const roomId = await room(crypto, secret);
      const self = { role: 'b' as const, k: node.id, kind: 'node' as const, name: 'desk' };
      const pair: Pair = { t: 'pair', ...self, mac: await mac(crypto, secret, self) };
      hub.options.heard!(hub.options.urls[0], { t: 'room', room: roomId, data: pair });
      s.destroy();
      await new Promise((r) => setTimeout(r, 15));
      expect(hub.peers).toHaveLength(0);
    }
    {
      const { session, peer } = await dialRoleB();
      const gate = Promise.withResolvers<unknown>();
      const slowCtl = makeCtl(() => gate.promise);
      peer.listener.online(slowCtl, welcomeOf(node.id, true));
      await new Promise((r) => setTimeout(r, 10));
      session.destroy();
      gate.resolve(hostInfoOf(node.id, []));
      await new Promise((r) => setTimeout(r, 10));
      expect(session.agents).toBeUndefined();
    }
    {
      const s = new PairingSession('live');
      const count = hubs.length + 1;
      await vi.waitFor(() => expect(hubs).toHaveLength(count));
      const hub = hubs.at(-1)!;
      const secret = parseSecret(s.code);
      const roomId = await room(crypto, secret);
      const self = { role: 'b' as const, k: node.id, kind: 'node' as const, name: 'desk' };
      hub.options.heard!(hub.options.urls[0], {
        t: 'room',
        room: roomId,
        data: { t: 'pair', ...self, mac: await mac(crypto, secret, self) },
      });
      await vi.waitFor(() => expect(hub.peers).toHaveLength(1));
      const enrolCtl = makeCtl(async (req) => {
        if (req.t === 'hello') {
          queueMicrotask(() => s.destroy());
          return welcomeOf(node.id, true);
        }
        return hostInfoOf(node.id, []);
      });
      hub.peers[0].listener.online(enrolCtl, welcomeOf(node.id, false));
      await new Promise((r) => setTimeout(r, 15));
      expect(s.enrolled).toBe(false);
    }
    {
      const { session, peer } = await dialRoleB();
      peer.listener.refused!('forbidden');
      expect(session.error).toStrictEqual({ step: 3, key: 'refused', code: 'forbidden' });
      session.destroy();
    }

    // 2. Offline before agents loaded
    {
      const { session, peer } = await dialRoleB();
      peer.listener.offline();
      expect(session.error).toStrictEqual({ step: 3, key: 'lost', code: undefined });
      session.destroy();
    }

    // 3. Invalid devlog entry
    {
      const { session, peer } = await dialRoleB();
      const ctl = makeCtl(async () => ({}));
      peer.listener.online(ctl, welcomeOf(node.id, false));
      peer.listener.push({ t: 'devlog', user: node.id, entries: [{ p: '{}', k: 'x', s: 'y' }] });
      await vi.waitFor(() => expect(session.stage).toBe('failed'));
      expect(session.error).toStrictEqual({ step: 3, key: 'unadopted', code: undefined });
      session.destroy();
    }

    // 4. confirmOwner: lost, refused, not_owner; and host.info: lost, refused
    for (const [label, helloRes, infoRes, expected] of [
      ['hello lost', null, null, { step: 3, key: 'lost', code: undefined }],
      [
        'hello refused',
        { re: 1, err: { code: 'forbidden', msg: 'no' } },
        null,
        { step: 3, key: 'refused', code: 'forbidden' },
      ],
      [
        'not owner',
        welcomeOf(node.id, false),
        null,
        { step: 3, key: 'not_owner', code: undefined },
      ],
      ['host.info lost', welcomeOf(node.id, true), null, { step: 4, key: 'lost', code: undefined }],
      [
        'host.info refused',
        welcomeOf(node.id, true),
        { re: 2, err: { code: 'internal', msg: 'boom' } },
        { step: 4, key: 'refused', code: 'internal' },
      ],
    ] as const) {
      idb = new IDBFactory();
      vi.stubGlobal('indexedDB', idb);
      const s = new PairingSession('live');
      const count = hubs.length + 1;
      await vi.waitFor(() => expect(hubs).toHaveLength(count));
      const hub = hubs.at(-1)!;
      const secret = parseSecret(s.code);
      const roomId = await room(crypto, secret);
      const self = { role: 'b' as const, k: node.id, kind: 'node' as const, name: 'desk' };
      hub.options.heard!(hub.options.urls[0], {
        t: 'room',
        room: roomId,
        data: { t: 'pair', ...self, mac: await mac(crypto, secret, self) },
      });
      await vi.waitFor(() => expect(hub.peers).toHaveLength(1));
      const ctl = makeCtl(async (req) => {
        const res = req.t === 'hello' ? helloRes : infoRes;
        if (!res) throw new Error('dropped');
        return res;
      });
      hub.peers[0].listener.online(ctl, welcomeOf(node.id, false));
      await vi.waitFor(() => expect(s.stage, label).toBe('failed'));
      expect(s.error, label).toStrictEqual(expected);
      s.destroy();
    }
  });
});
