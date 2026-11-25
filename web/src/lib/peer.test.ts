import { afterEach, beforeEach, describe, expect, expectTypeOf, it, vi, type Mock } from 'vitest';
import { FakePC, transportStats } from '../../features/peer-fakes';
import type { Device } from './identity';
import type { Link } from './groups.svelte';
import { connect, type Listener, type Options, type Session } from './peer';
import { goldens } from './mock';
import type { Ctl, RequestType, Welcome } from './proto';

/** The signalling clients made, oldest first, in place of rendezvous.ts. */
const { clients, FakeClient } = vi.hoisted(() => {
  class FakeClient {
    sent: { t: string; [field: string]: unknown }[] = [];
    closed = false;
    constructor(
      readonly urls: readonly string[],
      readonly device: unknown,
      readonly known: unknown,
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
    hear(frame: unknown) {
      this.receive(this.url, frame);
    }
    /** Signs the path in. */
    ready(ice: unknown[] = [], ttl = 3600) {
      this.hear({ t: 'ready', ice, ttl });
    }
    /** The payloads of the signals sent on this path, of one kind. */
    signals(t: string) {
      return this.sent
        .filter((frame) => frame.t === 'to')
        .map((frame) => JSON.parse((frame.env as { p: string }).p))
        .filter((payload) => payload.t === t);
    }
    count(t: string) {
      return this.sent.filter((frame) => frame.t === t).length;
    }
  }
  const clients: FakeClient[] = [];
  return { clients, FakeClient };
});
type FakeClient = InstanceType<typeof FakeClient>;

vi.mock('./rendezvous', () => ({
  rendezvous: (...args: ConstructorParameters<typeof FakeClient>) => new FakeClient(...args),
}));

const rv = 'wss://rv.example/rv';
const lan = 'ws://localhost:47800/rv';
const node = 'node-a';
const other = 'node-b';
const now = 1_763_600_000_000;
const id = /^[A-Za-z0-9_-]{21}[AQgw]$/;
const stun = { urls: ['stun:rv.example:3478'] };
const turn = {
  urls: ['turn:rv.example:3478?transport=udp', 'turns:turn.rv.example:443?transport=tcp'],
  username: 'user',
  credential: 'pass',
};
const welcome: Omit<Welcome, 're'> = {
  t: 'welcome',
  ok: true,
  v: 1,
  device: node,
  name: 'workstation',
  version: 'v0.3.1',
  time: now,
  addrs: [],
  groups: [],
  owner: true,
};
const rows = { t: 'rows', g: 'group', rows: [] };

const sign = vi.fn(async (p: string) => ({ p, k: 'browser', s: 'signed' }));
const device = { id: 'browser', sign } as unknown as Device;
const known = () => true;

/** Lets every promise that is ready run; the clock does not move. */
const flush = () => new Promise<void>((resolve) => setImmediate(resolve));

/** Moves the clock, letting what each timer starts run before the next one is due. */
async function wait(ms: number) {
  await vi.advanceTimersByTimeAsync(ms);
  await flush();
}

let session: Session;

it('joins a room on its existing path and only rejoins it while watched', async () => {
  session = connect({ urls: [rv, lan], device, known, name: 'Browser' });
  const [remote, local] = clients;
  const heard = vi.fn();
  const enter = { t: 'join', room: 'invite-room' };
  const stop = session.watchRoom(rv, enter.room, heard);
  expect(clients).toEqual([remote, local]);
  expect(remote.sent.at(-1)).toEqual(enter);
  expect(local.sent.map(({ t }) => t)).toEqual(['watch']);
  local.hear({ t: 'closed' });
  dispatchEvent(new Event('online'));
  await vi.waitFor(() => expect(clients).toHaveLength(3));
  expect(clients[2].sent.map(({ t }) => t)).toEqual(['watch']);

  remote.hear({ t: 'closed' });
  dispatchEvent(new Event('online'));
  await vi.waitFor(() => expect(clients).toHaveLength(4));
  const reopened = clients[3];
  expect(reopened.url).toBe(rv);
  expect(reopened.sent.at(-1)).toEqual(enter);
  const joined = { t: 'joined', room: enter.room };
  reopened.hear(joined);
  expect(heard).toHaveBeenLastCalledWith(rv, joined);

  stop();
  reopened.hear({ t: 'closed' });
  dispatchEvent(new Event('online'));
  await vi.waitFor(() => expect(clients).toHaveLength(5));
  expect(clients[4].sent.map(({ t }) => t)).toEqual(['watch']);
  clients[4].hear(joined);
  expect(heard.mock.calls).toEqual([
    [lan, { t: 'closed' }],
    [rv, { t: 'closed' }],
    [rv, joined],
  ]);
});
function start(urls = [rv], more: Partial<Options> = {}) {
  session = connect({ urls, device, known, name: 'phone browser', ...more });
  return session;
}

/** A signal of a node about one of the browser's connections, as rendezvous.ts hands it on. */
const from = (conn: string, fields: object, k = node) => ({
  t: 'from',
  k,
  env: {
    p: JSON.stringify({ n: 'AAAAAAAAAAAAAAAAAAAAAA', ts: now, to: 'browser', conn, ...fields }),
    k,
    s: 'signed',
  },
});
const answer = (conn: string, sdp = 'answer', k = node) => from(conn, { t: 'answer', sdp }, k);
const candidate = (conn: string, cand: string, mid = '0', k = node) =>
  from(conn, { t: 'ice', cand, mid }, k);

/** What hears a peer: one listener for each connection that came to serve. */
function hearing() {
  const listeners: { online: Mock; push: Mock; offline: Mock }[] = [];
  const listen = vi.fn(() => {
    const listener = { online: vi.fn(), push: vi.fn(), offline: vi.fn() };
    listeners.push(listener);
    return listener;
  });
  return { listen, listeners, onpath: vi.fn() };
}

const pcs = () => FakePC.made;
const last = () => FakePC.made.at(-1)!;
/** The conn of the offer a connection sent, by the order the connections were made. */
const connOf = (pc: FakePC, client = clients[0]) =>
  client.signals('offer').find((offer) => offer.sdp === pc.local!.sdp)!.conn as string;

/** Answers a connection, opens its ctl and welcomes its hello. */
async function serve(pc: FakePC, client = clients[0]) {
  client.hear(answer(connOf(pc, client)));
  await flush();
  pc.ctl.open();
  await flush();
  pc.ctl.deliver({ ...welcome, re: 1 });
  await flush();
}

/** A session with one signed-in path and a peer whose first connection serves. */
async function served(target = node) {
  start();
  clients[0].ready([stun, turn]);
  const heard = hearing();
  const peer = session.peer(target, heard.listen, heard.onpath);
  await flush();
  const pc = last();
  await serve(pc);
  return { peer, pc, ...heard };
}

/** A connection that no peer adopts, dialled by a peer whose own connection serves. */
async function dialled(probe = false) {
  const { peer, listen, listeners, onpath } = await served();
  const conn = peer.dial(probe);
  await flush();
  return { peer, conn, pc: last(), listen, listeners, onpath };
}

/** A dialled connection that the node answered and whose ctl is open. */
async function opened() {
  const made = await dialled();
  clients[0].hear(answer(made.conn.conn));
  await flush();
  made.pc.ctl.open();
  await flush();
  return made;
}

beforeEach(() => {
  clients.length = 0;
  FakePC.reset();
  sign.mockClear();
  vi.stubGlobal('RTCPeerConnection', FakePC);
  vi.useFakeTimers({
    toFake: ['setTimeout', 'clearTimeout', 'setInterval', 'clearInterval', 'performance', 'Date'],
  });
  vi.setSystemTime(now);
});
afterEach(() => {
  session.close();
  vi.useRealTimers();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe('signalling paths', () => {
  it('opens one client for each distinct address and tells every frame on', () => {
    const heard = vi.fn();
    start([rv, lan, rv], { heard });
    expect(clients.map((client) => client.urls)).toStrictEqual([[rv], [lan]]);
    expect(clients.map((client) => [client.device, client.known])).toStrictEqual([
      [device, known],
      [device, known],
    ]);
    expect(clients.map((client) => client.sent)).toStrictEqual([
      [{ t: 'watch', k: [] }],
      [{ t: 'watch', k: [] }],
    ]);
    const frames = [
      { t: 'ready', ice: [], ttl: 3600 },
      { t: 'joined', room: '0123456789abcdef0123456789abcdef' },
      { t: 'presence', k: node, on: true },
    ];
    for (const frame of frames) clients[1].hear(frame);
    expect(heard.mock.calls).toStrictEqual(frames.map((frame) => [lan, frame]));
  });

  it('needs nobody to tell the frames to', () => {
    start();
    expect(() => clients[0].ready()).not.toThrow();
  });

  it('sends a pairing frame on the one path named', () => {
    start([rv, lan]);
    const join = { t: 'join', room: '0123456789abcdef0123456789abcdef' } as const;
    session.send(lan, join);
    session.send('wss://unknown.example/rv', join);
    expect(clients[0].sent).toStrictEqual([{ t: 'watch', k: [] }]);
    expect(clients[1].sent).toStrictEqual([{ t: 'watch', k: [] }, join]);
  });

  it('watches the nodes it keeps a connection to, 64 at most', () => {
    start([rv, lan]);
    const { listen } = hearing();
    const peer = session.peer(node, listen);
    session.peer(other, listen);
    for (const client of clients)
      expect(client.sent).toStrictEqual([
        { t: 'watch', k: [] },
        { t: 'watch', k: [node] },
        { t: 'watch', k: [node, other] },
      ]);
    peer.close();
    expect(clients[0].sent.at(-1)).toStrictEqual({ t: 'watch', k: [other] });
    const many = Array.from({ length: 64 }, (_, i) => `node-${i}`);
    for (const target of many) session.peer(target, listen);
    expect(clients[1].sent.at(-1)).toStrictEqual({ t: 'watch', k: [other, ...many.slice(0, 63)] });
  });

  it('asks for fresh ICE servers at half their ttl, and every 10 seconds until they come', () => {
    start([rv, lan]);
    clients[0].ready([turn], 3600);
    vi.advanceTimersByTime(1_799_999);
    expect(clients[0].count('ice')).toBe(0);
    vi.advanceTimersByTime(1);
    expect(clients[0].sent.at(-1)).toStrictEqual({ t: 'ice' });
    expect(clients[1].count('ice')).toBe(0);
    vi.advanceTimersByTime(9_999);
    expect(clients[0].count('ice')).toBe(1);
    vi.advanceTimersByTime(1);
    expect(clients[0].count('ice')).toBe(2);
    clients[0].hear({ t: 'ice', ice: [stun], ttl: 100 });
    vi.advanceTimersByTime(49_999);
    expect(clients[0].count('ice')).toBe(2);
    vi.advanceTimersByTime(1);
    expect(clients[0].count('ice')).toBe(3);
  });

  it('waits no longer for a refresh than a timer can', () => {
    start();
    clients[0].ready([], 5_000_000);
    vi.advanceTimersByTime(2 ** 31 - 2);
    expect(clients[0].count('ice')).toBe(0);
    vi.advanceTimersByTime(1);
    expect(clients[0].count('ice')).toBe(1);
  });

  it('opens a closed path again after 1, 2, 4 … 60 seconds, with the watch list', () => {
    start();
    session.peer(node, hearing().listen);
    for (const seconds of [1, 2, 4, 8, 16, 32, 60, 60]) {
      const made = clients.length;
      clients.at(-1)!.hear({ t: 'closed' });
      expect(clients.at(-1)!.closed).toBe(true);
      vi.advanceTimersByTime(seconds * 1000 - 1);
      expect(clients).toHaveLength(made);
      vi.advanceTimersByTime(1);
      expect(clients).toHaveLength(made + 1);
      expect(clients.at(-1)!.urls).toStrictEqual([rv]);
      expect(clients.at(-1)!.sent).toStrictEqual([{ t: 'watch', k: [node] }]);
    }
  });

  it('starts the series again after a path that stayed signed in for 60 seconds', () => {
    start();
    const reopensAfter = (ms: number) => {
      const made = clients.length;
      clients.at(-1)!.hear({ t: 'closed' });
      vi.advanceTimersByTime(ms - 1);
      expect(clients).toHaveLength(made);
      vi.advanceTimersByTime(1);
      expect(clients).toHaveLength(made + 1);
    };
    reopensAfter(1000);
    reopensAfter(2000);
    clients.at(-1)!.ready();
    vi.advanceTimersByTime(59_999);
    reopensAfter(4000);
    clients.at(-1)!.ready();
    vi.advanceTimersByTime(60_000);
    reopensAfter(1000);
    reopensAfter(2000);
  });

  it('drops the refresh and the ICE servers of a path that closed', async () => {
    start([rv, lan]);
    clients[0].ready([turn], 100);
    clients[1].ready([stun], 3600);
    clients[0].hear({ t: 'closed' });
    vi.advanceTimersByTime(50_000);
    expect(clients[0].count('ice')).toBe(0);
    expect(clients[2].count('ice')).toBe(0);
    session.peer(node, hearing().listen);
    expect(last().config).toStrictEqual({ iceServers: [stun] });
    await flush();
    expect(clients[1].signals('offer')).toHaveLength(1);
    expect(clients[2].signals('offer')).toHaveLength(0);
  });

  it('opens its paths at once when the network changes, and only those that wait', () => {
    start([rv, lan]);
    clients[0].hear({ t: 'closed' });
    vi.advanceTimersByTime(1000);
    clients[2].hear({ t: 'closed' });
    vi.advanceTimersByTime(1000);
    dispatchEvent(new Event('online'));
    expect(clients.map((client) => client.url)).toStrictEqual([rv, lan, rv, rv]);
    // The series started again: the next wait is one second, not four.
    clients[3].hear({ t: 'closed' });
    vi.advanceTimersByTime(1000);
    expect(clients).toHaveLength(5);
  });

  it('closes every path and connection for good', async () => {
    const listened = vi.spyOn(globalThis, 'addEventListener');
    const unlistened = vi.spyOn(globalThis, 'removeEventListener');
    const { pc, listeners } = await served();
    clients[0].hear({ t: 'closed' });
    session.close();
    expect(unlistened.mock.calls).toStrictEqual([['online', listened.mock.calls[0][1]]]);
    expect(listened.mock.calls).toStrictEqual([['online', expect.any(Function)]]);
    expect(clients.map((client) => client.closed)).toStrictEqual([true]);
    expect(pc.closes).toBe(1);
    expect(listeners[0].offline).toHaveBeenCalledTimes(1);
    dispatchEvent(new Event('online'));
    await wait(3_600_000);
    expect(clients).toHaveLength(1);
    expect(pcs()).toHaveLength(1);
    expect(clients[0].count('ice')).toBe(0);
  });
});

describe('a session that closes', () => {
  it('closes a path that is signed in', () => {
    start([rv, lan]);
    clients[0].ready([turn]);
    session.close();
    expect(clients.map((client) => client.closed)).toStrictEqual([true, true]);
  });

  it('stops asking a path that is signed in for ICE servers', async () => {
    start();
    clients[0].ready([turn], 3600);
    await wait(1_799_999);
    session.close();
    await wait(3_600_000);
    expect(clients[0].count('ice')).toBe(0);
  });
});

describe('an offer', () => {
  it('waits for a signalling path, and goes out at once when the node is reported present', async () => {
    start([rv, lan]);
    session.peer(node, hearing().listen);
    await wait(1000);
    await wait(2000);
    expect(pcs()).toHaveLength(0);
    clients[0].ready([stun]);
    clients[0].hear({ t: 'presence', k: other, on: true });
    clients[0].hear({ t: 'presence', k: node, on: false });
    expect(pcs()).toHaveLength(0);
    clients[0].hear({ t: 'presence', k: node, on: true });
    expect(pcs()).toHaveLength(1);
  });

  it('creates ctl, and is signed and sent on every path that is signed in', async () => {
    start([rv, lan, 'wss://slow.example/rv']);
    clients[0].ready([stun]);
    clients[1].ready([turn]);
    session.peer(node, hearing().listen);
    await flush();
    const [pc] = pcs();
    expect(pc.config).toStrictEqual({ iceServers: [stun, turn] });
    expect(pc.channels.map((channel) => channel.label)).toStrictEqual(['ctl']);
    expect(pc.local).toStrictEqual({ type: 'offer', sdp: 'offer 1' });
    const [sent] = clients[0].sent.filter((frame) => frame.t === 'to');
    expect(sent).toStrictEqual({ t: 'to', k: node, env: await sign.mock.results[0].value });
    expect(clients[1].sent.at(-1)).toStrictEqual(sent);
    expect(clients[2].count('to')).toBe(0);
    const [offer] = clients[0].signals('offer');
    expect(offer).toStrictEqual({
      t: 'offer',
      sdp: 'offer 1',
      probe: false,
      n: expect.stringMatching(id),
      ts: now,
      to: node,
      conn: expect.stringMatching(id),
    });
    expect(offer.n).not.toBe(offer.conn);
  });

  it('trickles each candidate on every path, and no end of candidates', async () => {
    const { pc, conn } = await dialled();
    pc.gather({ candidate: 'candidate:1 1 udp 1 192.0.2.1 9 typ host', sdpMid: '0' });
    pc.gather({ candidate: 'candidate:2 1 udp 1 192.0.2.2 9 typ host', sdpMid: null });
    pc.gather({ candidate: '', sdpMid: '0' });
    pc.gather(null);
    await flush();
    expect(clients[0].signals('ice')).toStrictEqual([
      {
        t: 'ice',
        cand: 'candidate:1 1 udp 1 192.0.2.1 9 typ host',
        mid: '0',
        n: expect.stringMatching(id),
        ts: now,
        to: node,
        conn: conn.conn,
      },
      {
        t: 'ice',
        cand: 'candidate:2 1 udp 1 192.0.2.2 9 typ host',
        mid: '',
        n: expect.stringMatching(id),
        ts: now,
        to: node,
        conn: conn.conn,
      },
    ]);
  });

  it('is given up when it cannot be made or signed', async () => {
    const { peer } = await dialled();
    FakePC.refuseOffer = true;
    peer.dial(false);
    await flush();
    expect(last().closes).toBe(1);
    FakePC.refuseOffer = false;
    peer.dial(false);
    await flush();
    sign.mockRejectedValueOnce(new Error('no key'));
    last().gather({ candidate: 'candidate:1', sdpMid: '0' });
    await flush();
    expect(last().closes).toBe(1);
  });

  it('is retired when no answer comes within 10 seconds, and its late answer is ignored', async () => {
    const { pc, conn } = await dialled();
    await wait(9_999);
    expect(pc.closes).toBe(0);
    await wait(1);
    expect(pc.closes).toBe(1);
    expect(await conn.closed).toBe(0);
    clients[0].hear(answer(conn.conn));
    await flush();
    expect(pc.remote).toBeUndefined();
  });

  it('is retired at once when every path it went out on reports the node offline', async () => {
    start([rv, lan]);
    clients[0].ready();
    clients[1].ready();
    const peer = session.peer(node, hearing().listen);
    const waiting = Promise.withResolvers<{ p: string; k: string; s: string }>();
    sign.mockReturnValueOnce(waiting.promise);
    await flush();
    const [pc] = pcs();
    // Before the offer has left, a report is about an earlier one.
    clients[0].hear({ t: 'offline', k: node });
    expect(pc.closes).toBe(0);
    waiting.resolve({ p: '{}', k: 'browser', s: 'signed' });
    await flush();
    clients[0].hear({ t: 'offline', k: other });
    clients[1].hear({ t: 'offline', k: other });
    clients[0].hear({ t: 'offline', k: node });
    clients[0].hear({ t: 'offline', k: node });
    expect(pc.closes).toBe(0);
    clients[1].hear({ t: 'offline', k: node });
    expect(pc.closes).toBe(1);
    peer.close();
  });

  it('counts a path that closed as one that reported the node offline', async () => {
    const { pc } = await dialled();
    clients[0].hear({ t: 'closed' });
    expect(pc.closes).toBe(1);
  });

  it('is left to ICE once it is answered', async () => {
    const { pc, conn } = await dialled();
    clients[0].hear(answer(conn.conn));
    await flush();
    clients[0].hear({ t: 'offline', k: node });
    await wait(10_000);
    expect(pc.closes).toBe(0);
    pc.becomes('disconnected');
    expect(pc.closes).toBe(0);
    pc.becomes('failed');
    expect(pc.closes).toBe(1);
  });
});

describe('an answer and its candidates', () => {
  it('uses the first copy of the answer of its own node and connection', async () => {
    const { pc, conn, peer } = await dialled();
    const second = peer.dial(false);
    await flush();
    clients[0].hear(answer(conn.conn, 'another signer', other));
    clients[0].hear(from(conn.conn, { t: 'answer' }));
    clients[0].hear(from(conn.conn, { t: 'offer', sdp: 'offer', probe: false }));
    await flush();
    expect(pc.remote).toBeUndefined();
    clients[0].hear(answer(conn.conn, 'first'));
    clients[0].hear(answer(conn.conn, 'second'));
    await flush();
    expect(pc.remote).toStrictEqual({ type: 'answer', sdp: 'first' });
    expect(last().remote).toBeUndefined();
    clients[0].hear(answer(second.conn, 'its own'));
    await flush();
    expect(last().remote).toStrictEqual({ type: 'answer', sdp: 'its own' });
    expect(pc.closes + last().closes).toBe(0);
  });

  it('gives up a connection whose answer the browser cannot read', async () => {
    const { pc, conn } = await dialled();
    clients[0].hear(answer(conn.conn, 'unreadable'));
    await flush();
    expect(pc.closes).toBe(1);
  });

  it('holds candidates until the answer and applies each distinct one once (A1.11)', async () => {
    const { pc, conn } = await dialled();
    clients[0].hear(candidate(conn.conn, 'candidate:1'));
    clients[0].hear(candidate(conn.conn, 'candidate:1'));
    clients[0].hear(candidate(conn.conn, 'candidate:1', '1'));
    clients[0].hear(from(conn.conn, { t: 'ice', cand: 'candidate:9' }));
    clients[0].hear(candidate(conn.conn, 'candidate:8', '0', other));
    await flush();
    expect(pc.added).toStrictEqual([]);
    // A candidate that comes while the answer is being applied waits for it too.
    const applied = Promise.withResolvers<void>();
    pc.hold = applied.promise;
    clients[0].hear(answer(conn.conn));
    clients[0].hear(candidate(conn.conn, 'candidate:2'));
    await flush();
    expect(pc.added).toStrictEqual([]);
    applied.resolve();
    await flush();
    const first = [
      { candidate: 'candidate:1', sdpMid: '0' },
      { candidate: 'candidate:1', sdpMid: '1' },
      { candidate: 'candidate:2', sdpMid: '0' },
    ];
    expect(pc.added).toStrictEqual(first);
    clients[0].hear(candidate(conn.conn, 'candidate:1'));
    clients[0].hear(candidate(conn.conn, 'candidate:3'));
    clients[0].hear(candidate(conn.conn, 'refused'));
    await flush();
    expect(pc.added).toStrictEqual([
      ...first,
      { candidate: 'candidate:3', sdpMid: '0' },
      { candidate: 'refused', sdpMid: '0' },
    ]);
    expect(pc.early).toBe(0);
    expect(pc.closes).toBe(0);
  });

  it('takes 64 candidates for one connection, held or applied', async () => {
    const { pc, conn } = await dialled();
    for (let i = 0; i < 63; i++) clients[0].hear(candidate(conn.conn, `candidate:${i}`));
    clients[0].hear(answer(conn.conn));
    await flush();
    clients[0].hear(candidate(conn.conn, 'candidate:63'));
    clients[0].hear(candidate(conn.conn, 'candidate:64'));
    expect(pc.added).toHaveLength(64);
    expect(pc.added.at(-1)).toStrictEqual({ candidate: 'candidate:63', sdpMid: '0' });
  });
});

describe('ctl', () => {
  const kinds: { t: RequestType; right: object }[] = [
    { t: 'hello', right: welcome },
    { t: 'sync', right: { t: 'synced', ok: true, g: 'group', seq: 7 } },
    { t: 'fetch', right: { t: 'fetched', ok: true, rows: [], more: false } },
    { t: 'host.info', right: JSON.parse(goldens['host-info-answer']) },
  ];

  it('numbers its requests and gives each the response that names it', async () => {
    const { pc, conn } = await opened();
    await expect(conn.opened).resolves.toBeUndefined();
    const env = { p: '{}', k: 'browser', s: 'signed' };
    const one = conn.request({ t: 'cmd', env });
    const two = conn.request({ t: 'fetch', g: 'group', thread: null, before: 0, limit: 50 });
    expect(pc.ctl.frames()).toStrictEqual([
      { t: 'cmd', env, id: 1 },
      { t: 'fetch', g: 'group', thread: null, before: 0, limit: 50, id: 2 },
    ]);
    pc.ctl.deliver({ re: 3, ok: true });
    pc.ctl.deliver({ t: 'fetched', re: 2, ok: true, rows: [], more: false });
    pc.ctl.deliver({ re: 1, ok: true, num: 14 });
    expect(await two).toStrictEqual({ t: 'fetched', re: 2, ok: true, rows: [], more: false });
    expect(await one).toStrictEqual({ re: 1, ok: true, num: 14 });
  });

  it.each(kinds)('reads the answer to $t as that request has it', async ({ t, right }) => {
    const { pc, conn } = await opened();
    const unreadable = conn.request({ t });
    const answered = conn.request({ t });
    const refused = conn.request({ t });
    pc.ctl.deliver({ re: 1, ok: true });
    pc.ctl.deliver({ ...right, re: 1 });
    pc.ctl.deliver({ ...right, re: 2 });
    pc.ctl.deliver({ re: 3, ok: true, err: { code: 'forbidden', msg: 'no' } });
    await expect(unreadable).rejects.toThrow(new Error('unreadable answer'));
    expect(await answered).toStrictEqual({ ...right, re: 2 });
    expect(await refused).toStrictEqual({ re: 3, err: { code: 'forbidden', msg: 'no' } });
    expect(pc.closes).toBe(0);
  });

  // An answer is read by its request, so the members a result does not have are ignored, a `t`
  // included (contracts/conventions.md section 2; proto.test.ts, "ignores in a cmd answer the
  // members it does not read"). Only a frame without the result fails the command.
  it('answers a cmd with a synced frame as a plain result, its t and other members ignored', async () => {
    const { pc, conn } = await opened();
    const [unreadable, ok, other] = [1, 2, 3].map(() => conn.request({ t: 'cmd' }));
    pc.ctl.deliver({ ...rows, re: 1 });
    pc.ctl.deliver({ re: 2, ok: true, num: 14 });
    pc.ctl.deliver({ t: 'synced', re: 3, ok: true, g: 'group', seq: 7 });
    await expect(unreadable).rejects.toThrow(new Error('unreadable answer'));
    expect(await ok).toStrictEqual({ re: 2, ok: true, num: 14 });
    expect(await other).toStrictEqual({ re: 3, ok: true });
  });

  it('drops what is no frame and stays open', async () => {
    const { pc, conn, peer } = await opened();
    const push = vi.fn();
    conn.onpush = push;
    const asked = conn.request({ t: 'cmd' });
    for (const message of ['no json', 'null', '5', '"text"', '[]', new ArrayBuffer(4)])
      expect(() => pc.ctl.deliver(message)).not.toThrow();
    pc.ctl.deliver({ t: 'unknown' });
    pc.ctl.deliver({ re: 'one', ok: true });
    pc.ctl.deliver({ re: 7, ok: true });
    expect(push).not.toHaveBeenCalled();
    pc.ctl.deliver(rows);
    expect(push.mock.calls).toStrictEqual([[rows]]);
    pc.ctl.deliver({ re: 1, ok: true });
    expect(await asked).toStrictEqual({ re: 1, ok: true });
    expect(pc.closes).toBe(0);
    peer.close();
  });

  it('answers bad_request to a request, which a browser never serves', async () => {
    const { pc, conn } = await opened();
    const push = vi.fn();
    conn.onpush = push;
    const asked = vi.fn();
    conn.request({ t: 'cmd' }).then(asked, () => {});
    pc.ctl.deliver({ ...rows, id: 5 });
    pc.ctl.deliver({ re: 1, ok: true, id: 6 });
    for (const bad of [1.5, 'seven', null, 2 ** 53]) pc.ctl.deliver({ t: 'sync', id: bad });
    await flush();
    const refusal = { code: 'bad_request', msg: 'the web client serves no request' };
    expect(pc.ctl.frames().slice(1)).toStrictEqual([
      { re: 5, err: refusal },
      { re: 6, err: refusal },
    ]);
    expect(push).not.toHaveBeenCalled();
    expect(asked).not.toHaveBeenCalled();
  });

  it('refuses a request that cannot fit into 60 KiB under any id', async () => {
    const { pc, conn } = await opened();
    const fixed = JSON.stringify({ t: 'cmd', pad: '', id: Number.MAX_SAFE_INTEGER }).length;
    conn.request({ t: 'cmd', pad: 'x'.repeat(61_440 - fixed) }).catch(() => {});
    expect(pc.ctl.sent).toHaveLength(1);
    expect(() => conn.request({ t: 'cmd', pad: 'x'.repeat(61_441 - fixed) })).toThrow(
      new RangeError('ctl frame exceeds 60 KiB'),
    );
    // The limit counts bytes: é is two.
    expect(() => conn.request({ t: 'cmd', pad: 'é'.repeat((61_442 - fixed) / 2) })).toThrow(
      RangeError,
    );
    expect(pc.ctl.sent).toHaveLength(1);
  });

  it('sends a push without an id up to 60 KiB, and closes when the channel cannot send', async () => {
    const { pc, conn } = await opened();
    // fixed is 37 bytes, so 61_439 - fixed and 61_441 - fixed are even multiples of two-byte 'é'.
    const fixed = JSON.stringify({ t: 'devlog', user: '', entries: [] }).length;
    const exact = {
      t: 'devlog' as const,
      user: 'x' + 'é'.repeat((61_439 - fixed) / 2),
      entries: [],
    };
    conn.send(exact);
    expect(pc.ctl.frames()).toStrictEqual([exact]);
    const overAscii = { t: 'devlog' as const, user: 'x'.repeat(61_441 - fixed), entries: [] };
    expect(() => conn.send(overAscii)).toThrow(new RangeError('ctl frame exceeds 60 KiB'));
    const overUtf8 = { t: 'devlog' as const, user: 'é'.repeat((61_441 - fixed) / 2), entries: [] };
    expect(() => conn.send(overUtf8)).toThrow(new RangeError('ctl frame exceeds 60 KiB'));
    expect(pc.ctl.sent).toHaveLength(1);
    pc.ctl.closing = true;
    conn.send({ t: 'devlog', user: 'u', entries: [] });
    expect(conn.gone).toBe(true);
    expect(pc.closes).toBe(1);
    const sendSpy = vi.spyOn(pc.ctl, 'send');
    conn.send({ t: 'devlog', user: 'u', entries: [] });
    expect(sendSpy).not.toHaveBeenCalled();
  });

  it('is gone when its channel cannot send, and rejects what waits', async () => {
    const { pc, conn } = await opened();
    const first = conn.request({ t: 'cmd' });
    pc.ctl.closing = true;
    const second = conn.request({ t: 'cmd' });
    await expect(first).rejects.toThrow(new Error('connection lost'));
    await expect(second).rejects.toThrow(new Error('connection lost'));
    expect(pc.closes).toBe(1);
    pc.ctl.closing = false;
    await expect(conn.request({ t: 'cmd' })).rejects.toThrow(new Error('connection lost'));
    expect(pc.ctl.sent).toHaveLength(1);
  });

  it('lets go of its deadline when it closes before the answer comes', async () => {
    const { peer } = await served();
    const timers = vi.getTimerCount();
    const conn = peer.dial(false);
    await flush();
    expect(vi.getTimerCount()).toBe(timers + 1);
    conn.close();
    expect(vi.getTimerCount()).toBe(timers);
  });

  it('closes once, says how long ctl was open, and applies nothing afterwards', async () => {
    const { pc, conn } = await dialled();
    clients[0].hear(answer(conn.conn));
    await wait(500);
    pc.ctl.open();
    const push = vi.fn();
    conn.onpush = push;
    await wait(1234);
    pc.ctl.onclose();
    conn.close();
    expect(pc.closes).toBe(1);
    expect(await conn.closed).toBe(1234);
    pc.ctl.deliver(rows);
    expect(push).not.toHaveBeenCalled();
    clients[0].hear(candidate(conn.conn, 'candidate:1'));
    expect(pc.added).toStrictEqual([]);
  });

  it('opens other channels on the connection that serves', async () => {
    start();
    clients[0].ready();
    const peer = session.peer(node, hearing().listen);
    await flush();
    expect(peer.channel('pty:group:member')).toBeUndefined();
    expect(() => peer.send({ t: 'devlog', user: 'u', entries: [] })).not.toThrow();
    await serve(last());
    const channel = peer.channel('pty:group:member');
    expect(channel).toBe(last().channels[1]);
    expect(last().channels[1].label).toBe('pty:group:member');
    const push = { t: 'devlog' as const, user: 'u', entries: [] };
    peer.send(push);
    expect(last().ctl.frames().at(-1)).toStrictEqual(push);
    const resize = { t: 'pty.resize' as const, g: 'group', ai: 'member', cols: 120, rows: 32 };
    peer.send(resize);
    expect(last().ctl.frames().at(-1)).toStrictEqual(resize);
    const fixed = JSON.stringify({ t: 'devlog', user: '', entries: [] }).length;
    expect(() => peer.send({ t: 'devlog', user: 'x'.repeat(61_441 - fixed), entries: [] })).toThrow(
      new RangeError('ctl frame exceeds 60 KiB'),
    );
    last().ctl.onclose();
    await flush();
    expect(() => peer.send(push)).not.toThrow();
  });
});

describe('hello', () => {
  it('has in the link of a group home a listener', () => {
    expectTypeOf<Link>().toExtend<Listener>();
  });

  it('says who the browser is and hands the welcome to a new listener', async () => {
    const { peer, pc, listen, listeners } = await served();
    expect(pc.ctl.frames()).toStrictEqual([
      { t: 'hello', v: 2, kind: 'browser', name: 'phone browser', addrs: [], id: 1 },
    ]);
    expect(listen).toHaveBeenCalledTimes(1);
    expect(listeners[0].online.mock.calls).toStrictEqual([[peer, { ...welcome, re: 1 }]]);
    await wait(10_000);
    expect(pc.closes).toBe(0);
  });

  it('gives up a connection that is not welcomed within 10 seconds', async () => {
    const { pc, conn } = await opened();
    const greeted = conn.hello();
    greeted.catch(() => {});
    await wait(9_999);
    expect(pc.closes).toBe(0);
    await wait(1);
    expect(pc.closes).toBe(1);
    await expect(greeted).rejects.toThrow(new Error('connection lost'));
  });

  it('gives up a connection whose hello the node refuses', async () => {
    const { pc, conn } = await opened();
    const greeted = conn.hello();
    pc.ctl.deliver({ re: 1, err: { code: 'version', msg: 'too new' } });
    await flush();
    expect(pc.ctl.frames().at(-1)).toStrictEqual({
      t: 'hello',
      v: 1,
      kind: 'browser',
      name: 'phone browser',
      addrs: [],
      id: 2,
    });
    pc.ctl.deliver({ re: 2, err: { code: 'version', msg: 'unsupported' } });
    await expect(greeted).rejects.toThrow(new Error('hello refused: version'));
    expect(pc.closes).toBe(1);
    expect(conn.refused).toBe('version');
  });

  it('uses the same connection when the node only supports v1', async () => {
    const { pc, conn } = await opened();
    const greeted = conn.hello();
    pc.ctl.deliver({ re: 1, err: { code: 'version', msg: 'too new' } });
    await flush();
    pc.ctl.deliver({ ...welcome, re: 2 });
    await expect(greeted).resolves.toStrictEqual({ ...welcome, re: 2 });
    expect(pc.closes).toBe(0);
  });

  it('gives up a connection whose welcome it cannot read', async () => {
    const { pc, conn } = await opened();
    const greeted = conn.hello();
    pc.ctl.deliver({ t: 'welcome', re: 1, ok: true });
    await expect(greeted).rejects.toThrow(new Error('unreadable answer'));
    expect(pc.closes).toBe(1);
    await wait(10_000);
    expect(pc.closes).toBe(1);
  });
});

describe('a peer', () => {
  /** Lets the current attempt fail at once and says when the next one was made. */
  async function failsAndRetriesAfter(ms: number) {
    const made = pcs().length;
    clients[0].hear({ t: 'offline', k: node });
    await flush();
    await wait(ms - 1);
    expect(pcs()).toHaveLength(made);
    await wait(1);
    expect(pcs()).toHaveLength(made + 1);
  }

  async function attempting() {
    start();
    clients[0].ready();
    const heard = hearing();
    const peer = session.peer(node, heard.listen, heard.onpath);
    await flush();
    return { peer, ...heard };
  }

  it('offers again after 1, 2, 4 … 60 seconds, each time with a new conn', async () => {
    await attempting();
    for (const seconds of [1, 2, 4, 8, 16, 32, 60, 60]) await failsAndRetriesAfter(seconds * 1000);
    const conns = clients[0].signals('offer').map((offer) => offer.conn);
    expect(new Set(conns).size).toBe(9);
  });

  it('starts the series again only after a connection whose ctl stayed open 60 seconds', async () => {
    const { listeners } = await attempting();
    await failsAndRetriesAfter(1000);
    await failsAndRetriesAfter(2000);
    await serve(last());
    await wait(59_999);
    last().ctl.onclose();
    await flush();
    expect(listeners[0].offline).toHaveBeenCalledTimes(1);
    await wait(3_999);
    expect(pcs()).toHaveLength(3);
    await wait(1);
    expect(pcs()).toHaveLength(4);
    await serve(last());
    await wait(60_000);
    last().becomes('failed');
    await flush();
    await wait(999);
    expect(pcs()).toHaveLength(4);
    await wait(1);
    expect(pcs()).toHaveLength(5);
    await failsAndRetriesAfter(2000);
  });

  it('skips the back-off when the node is reported present or the network changes', async () => {
    await attempting();
    await failsAndRetriesAfter(1000);
    await failsAndRetriesAfter(2000);
    clients[0].hear({ t: 'offline', k: node });
    await flush();
    clients[0].hear({ t: 'presence', k: node, on: true });
    expect(pcs()).toHaveLength(4);
    await flush();
    // The series started again, and nothing was left waiting.
    await failsAndRetriesAfter(1000);
    clients[0].hear({ t: 'offline', k: node });
    await flush();
    dispatchEvent(new Event('online'));
    expect(pcs()).toHaveLength(6);
    await wait(9_999);
    expect(pcs()).toHaveLength(6);
  });

  it('offers nothing new while it attempts or serves', async () => {
    await attempting();
    clients[0].hear({ t: 'presence', k: node, on: true });
    await failsAndRetriesAfter(1000);
    await serve(last());
    clients[0].hear({ t: 'presence', k: node, on: true });
    dispatchEvent(new Event('online'));
    await wait(120_000);
    expect(pcs()).toHaveLength(2);
  });

  /** The node answers the attempt's offer and refuses its hello. */
  async function refuseHello(code: string) {
    clients[0].hear(answer(connOf(last())));
    await flush();
    last().ctl.open();
    await flush();
    last().ctl.deliver({ re: 1, err: { code, msg: 'no' } });
    await flush();
    if (code === 'version') {
      last().ctl.deliver({ re: 2, err: { code, msg: 'no' } });
      await flush();
    }
  }

  it('retries a connection whose hello fails for a reason that may pass', async () => {
    const { listen } = await attempting();
    await refuseHello('internal');
    expect(last().closes).toBe(1);
    expect(listen).not.toHaveBeenCalled();
    await wait(1000);
    expect(pcs()).toHaveLength(2);
  });

  it.each(['version', 'forbidden'])(
    'stops offering to a node that refuses its hello with %s, and says why',
    async (code) => {
      start();
      clients[0].ready();
      const refused = vi.fn();
      const listener = { online: vi.fn(), push: vi.fn(), offline: vi.fn(), refused };
      const peer = session.peer(node, () => listener);
      const hold = peer.hold();
      await flush();
      await refuseHello(code);
      expect(last().closes).toBe(1);
      expect(refused.mock.calls).toStrictEqual([[code]]);
      expect(listener.online).not.toHaveBeenCalled();
      expect(listener.offline).not.toHaveBeenCalled();
      hold();
      clients[0].hear({ t: 'presence', k: node, on: true });
      dispatchEvent(new Event('online'));
      await wait(120_000);
      expect(pcs()).toHaveLength(1);
      await expect(peer.request({ t: 'cmd', env: { p: '', k: '', s: '' } })).rejects.toThrow(
        new Error('connection lost'),
      );
    },
  );

  it('rejects the requests that waited for a welcome when the node refuses for good', async () => {
    const { peer, pc } = await served();
    const cmd = peer.request({ t: 'cmd', env: { p: '', k: '', s: '' } });
    const settled = vi.fn();
    cmd.then(settled, settled);
    pc.ctl.onclose();
    await flush();
    await wait(1000);
    expect(settled).not.toHaveBeenCalled();
    await refuseHello('version');
    await expect(cmd).rejects.toThrow(new Error('connection lost'));
  });

  it('stops as well when its listener does not hear refusals', async () => {
    const unhandled = vi.fn();
    process.on('unhandledRejection', unhandled);
    const { listen } = await attempting();
    await refuseHello('forbidden');
    process.off('unhandledRejection', unhandled);
    expect(listen).toHaveBeenCalledTimes(1);
    expect(unhandled).not.toHaveBeenCalled();
    await wait(120_000);
    expect(pcs()).toHaveLength(1);
  });

  it('hands pushes to the listener of the connection that serves, and to none before', async () => {
    const { listeners } = await attempting();
    const pc = last();
    clients[0].hear(answer(connOf(pc)));
    await flush();
    pc.ctl.open();
    pc.ctl.deliver(rows);
    await flush();
    pc.ctl.deliver({ ...welcome, re: 1 });
    await flush();
    expect(listeners[0].push).not.toHaveBeenCalled();
    pc.ctl.deliver(rows);
    expect(listeners[0].push.mock.calls).toStrictEqual([[rows]]);
  });

  it('rejects a request while no connection serves', async () => {
    const { peer } = await attempting();
    await expect(peer.request({ t: 'cmd', env: { p: '', k: '', s: '' } })).rejects.toThrow(
      new Error('connection lost'),
    );
    expect(last().ctl.sent).toStrictEqual([]);
  });

  it('sends unanswered requests again after the next welcome, but not a sync', async () => {
    const { peer, pc, listeners, listen } = await served();
    const env = (p: string) => ({ p, k: 'browser', s: 'signed' });
    const answered = peer.request({ t: 'cmd', env: env('answered') });
    const cmd = peer.request({ t: 'cmd', env: env('kept') });
    const sync = peer.request({ t: 'sync', g: 'group', since: 3, state_only: false });
    const fetch = peer.request({ t: 'fetch', g: 'group', thread: null, before: 0, limit: 50 });
    pc.ctl.deliver({ re: 2, ok: true });
    expect(await answered).toStrictEqual({ re: 2, ok: true });
    pc.ctl.onclose();
    await expect(sync).rejects.toThrow(new Error('connection lost'));
    expect(listeners[0].offline).toHaveBeenCalledTimes(1);
    await expect(peer.request({ t: 'cmd', env: env('late') })).rejects.toThrow('connection lost');
    // The listener of the next connection reads again before the old requests go out.
    listen.mockImplementationOnce(() => ({
      online: vi.fn((ctl: Ctl) => {
        ctl.request({ t: 'sync', g: 'group', since: 9, state_only: false }).catch(() => {});
      }),
      push: vi.fn(),
      offline: vi.fn(),
    }));
    await wait(1000);
    await serve(last());
    expect(last().ctl.frames().slice(1)).toStrictEqual([
      { t: 'sync', g: 'group', since: 9, state_only: false, id: 2 },
      { t: 'cmd', env: env('kept'), id: 3 },
      { t: 'fetch', g: 'group', thread: null, before: 0, limit: 50, id: 4 },
    ]);
    last().ctl.deliver({ re: 3, ok: true, num: 14 });
    last().ctl.deliver({ t: 'fetched', re: 4, ok: true, rows: [], more: false });
    expect(await cmd).toStrictEqual({ re: 3, ok: true, num: 14 });
    expect(await fetch).toStrictEqual({ t: 'fetched', re: 4, ok: true, rows: [], more: false });
    // What was answered is not asked a third time.
    last().ctl.onclose();
    await flush();
    await wait(2000);
    await serve(last());
    expect(pcs()).toHaveLength(3);
    expect(last().ctl.frames().slice(1)).toStrictEqual([]);
  });

  it('does not keep a request that cannot be sent or whose answer cannot be read', async () => {
    const { peer, pc } = await served();
    const env = { p: 'x'.repeat(61_440), k: 'browser', s: 'signed' };
    await expect(peer.request({ t: 'cmd', env })).rejects.toThrow(RangeError);
    const unreadable = peer.request({ t: 'cmd', env: { p: '', k: '', s: '' } });
    pc.ctl.deliver({ re: 2 });
    await expect(unreadable).rejects.toThrow(new Error('unreadable answer'));
    pc.ctl.onclose();
    await flush();
    await wait(1000);
    await serve(last());
    expect(last().ctl.sent).toHaveLength(1);
  });

  it('closes for good: its requests reject and nothing reconnects', async () => {
    const { peer, pc, listeners, onpath } = await served();
    const cmd = peer.request({ t: 'cmd', env: { p: '', k: '', s: '' } });
    const sync = peer.request({ t: 'sync', g: 'group', since: 0, state_only: false });
    onpath.mockClear();
    peer.close();
    await expect(cmd).rejects.toThrow(new Error('connection lost'));
    await expect(sync).rejects.toThrow(new Error('connection lost'));
    expect(pc.closes).toBe(1);
    expect(listeners[0].offline).toHaveBeenCalledTimes(1);
    expect(onpath.mock.calls).toStrictEqual([[undefined]]);
    expect(clients[0].sent.at(-1)).toStrictEqual({ t: 'watch', k: [] });
    await wait(120_000);
    expect(pcs()).toHaveLength(1);
  });

  it('closes an attempt and a wait for the next one', async () => {
    const { peer } = await attempting();
    peer.close();
    expect(last().closes).toBe(1);
    await wait(120_000);
    const again = session.peer(node, hearing().listen);
    clients[0].hear({ t: 'offline', k: node });
    await flush();
    again.close();
    await wait(120_000);
    expect(pcs()).toHaveLength(2);
  });

  it('closes while its reconnect waits for the back-off: nothing is offered then', async () => {
    const { peer } = await attempting();
    clients[0].hear({ t: 'offline', k: node });
    await flush();
    const offered = pcs().length;
    peer.close();
    await wait(120_000);
    expect(pcs()).toHaveLength(offered);
    expect(clients[0].signals('offer')).toHaveLength(offered);
  });

  it('gives way to a second peer for the same node', async () => {
    const { peer, pc } = await served();
    const second = session.peer(node, hearing().listen);
    await flush();
    expect(pc.closes).toBe(1);
    expect(pcs()).toHaveLength(2);
    // Closing the first again takes nothing from the second.
    const watches = clients[0].count('watch');
    peer.close();
    expect(clients[0].count('watch')).toBe(watches);
    expect(pcs()[1].closes).toBe(0);
    clients[0].hear({ t: 'offline', k: node });
    await flush();
    clients[0].hear({ t: 'presence', k: node, on: true });
    expect(pcs()).toHaveLength(3);
    second.close();
  });
});

describe('a probe and a move', () => {
  const next = { ...welcome, name: 'moved', re: 1 };

  /** A probe the node answered and whose ctl is open: what a move adopts. */
  async function probing(peer: Awaited<ReturnType<typeof served>>['peer']) {
    const probe = peer.dial(true);
    await flush();
    clients[0].hear(answer(probe.conn));
    await flush();
    last().ctl.open();
    await flush();
    return probe;
  }

  it('offers a probe without the relays, and its end starts nothing', async () => {
    const { pc, peer } = await dialled(true);
    expect(pcs()[0].config).toStrictEqual({ iceServers: [stun, turn] });
    expect(pc.config).toStrictEqual({ iceServers: [stun] });
    // An entry that names a relay among its addresses is one.
    const mixed = { ...turn, urls: ['stun:rv.example:3478', 'turn:rv.example:3478'] };
    clients[0].hear({ t: 'ice', ice: [mixed, stun], ttl: 3600 });
    peer.dial(true).close();
    expect(last().config).toStrictEqual({ iceServers: [stun] });
    expect(clients[0].signals('offer').map((offer) => offer.probe)).toStrictEqual([false, true]);
    pc.becomes('failed');
    await flush();
    await wait(120_000);
    expect(pcs()).toHaveLength(3);
  });

  it('adopts a connection: the one before it is closed and its requests move', async () => {
    const { peer, pc, listeners, onpath } = await served();
    const cmd = peer.request({ t: 'cmd', env: { p: '', k: '', s: '' } });
    const sync = peer.request({ t: 'sync', g: 'group', since: 3, state_only: false });
    const probe = await probing(peer);
    const moved = last();
    moved.stats = transportStats('host', 'srflx');
    await wait(1000);
    onpath.mockClear();
    peer.adopt(probe, next);
    expect(pc.closes).toBe(1);
    expect(moved.closes).toBe(0);
    expect(listeners[0].offline).toHaveBeenCalledTimes(1);
    expect(listeners[1].online.mock.calls).toStrictEqual([[peer, next]]);
    expect(listeners[0].offline.mock.invocationCallOrder[0]).toBeLessThan(
      listeners[1].online.mock.invocationCallOrder[0],
    );
    expect(onpath.mock.calls).toStrictEqual([
      [undefined],
      [{ kind: 'direct', rtt: undefined, since: now + 1000 }],
    ]);
    await expect(sync).rejects.toThrow(new Error('connection lost'));
    expect(moved.ctl.frames()).toStrictEqual([{ t: 'cmd', env: { p: '', k: '', s: '' }, id: 1 }]);
    moved.ctl.deliver(rows);
    pc.ctl.deliver(rows);
    expect(listeners[1].push.mock.calls).toStrictEqual([[rows]]);
    expect(listeners[0].push).not.toHaveBeenCalled();
    moved.ctl.deliver({ re: 1, ok: true });
    expect(await cmd).toStrictEqual({ re: 1, ok: true });
    await wait(1000);
    expect(onpath).toHaveBeenCalledTimes(3);
    expect(onpath.mock.calls[2]).toStrictEqual([
      { kind: 'direct', rtt: undefined, since: now + 1000 },
    ]);
    // The old connection's end is no loss, the new one's is.
    await wait(120_000);
    expect(pcs()).toHaveLength(2);
    moved.ctl.onclose();
    await flush();
    expect(listeners[1].offline).toHaveBeenCalledTimes(1);
    await wait(1000);
    expect(pcs()).toHaveLength(3);
  });

  it('adopts over a reconnect that waits or is on its way', async () => {
    const { peer, pc } = await served();
    const probe = await probing(peer);
    pc.ctl.onclose();
    await flush();
    peer.adopt(probe, next);
    await wait(120_000);
    expect(pcs()).toHaveLength(2);
    last().ctl.onclose();
    await flush();
    await wait(1000);
    const attempt = last();
    const second = await probing(peer);
    peer.adopt(second, next);
    expect(attempt.closes).toBe(1);
    expect(last().closes).toBe(0);
    await wait(120_000);
    expect(pcs()).toHaveLength(4);
  });

  it('keeps its own attempt open when that is the one adopted', async () => {
    const { pc } = await served();
    expect(pc.closes).toBe(0);
  });

  it('starts no reconnect while a move holds it', async () => {
    const { peer, pc } = await served();
    const first = peer.hold();
    const second = peer.hold();
    pc.ctl.onclose();
    await flush();
    await wait(120_000);
    first();
    await wait(120_000);
    expect(pcs()).toHaveLength(1);
    second();
    await wait(999);
    expect(pcs()).toHaveLength(1);
    await wait(1);
    expect(pcs()).toHaveLength(2);
  });

  it('starts none after a hold in which nothing was lost, or a connection was adopted', async () => {
    const { peer, pc } = await served();
    peer.hold()();
    await wait(120_000);
    expect(pcs()).toHaveLength(1);
    const release = peer.hold();
    const probe = await probing(peer);
    pc.ctl.onclose();
    await flush();
    peer.adopt(probe, next);
    release();
    await wait(120_000);
    expect(pcs()).toHaveLength(2);
  });

  it('releases a hold once, however often its release is called', async () => {
    const { peer, pc } = await served();
    const release = peer.hold();
    release();
    release();
    // The next move holds the peer as the first did.
    const second = peer.hold();
    pc.ctl.onclose();
    await flush();
    await wait(120_000);
    expect(pcs()).toHaveLength(1);
    second();
    await wait(1000);
    expect(pcs()).toHaveLength(2);
  });

  it('closes what it dialled when the session closes, the answered probe too', async () => {
    const { peer, pc } = await served();
    await probing(peer);
    const probe = last();
    await wait(2000);
    session.close();
    expect([pc.closes, probe.closes]).toStrictEqual([1, 1]);
    const asked = [pc.statsAsked, probe.statsAsked];
    await wait(30_000);
    expect([pc.statsAsked, probe.statsAsked]).toStrictEqual(asked);
  });

  it('closes every other connection it dialled when it adopts one', async () => {
    const { peer, pc } = await served();
    await probing(peer);
    const stale = last();
    const probe = await probing(peer);
    peer.adopt(probe, next);
    expect([pc.closes, stale.closes, last().closes]).toStrictEqual([1, 1, 0]);
  });

  it('does not adopt a connection that is already gone: the one that serves stays', async () => {
    const { peer, pc, listen, listeners } = await served();
    const probe = await probing(peer);
    expect(probe.gone).toBe(false);
    last().becomes('failed');
    expect(probe.gone).toBe(true);
    peer.adopt(probe, next);
    expect(pc.closes).toBe(0);
    expect(listen).toHaveBeenCalledTimes(1);
    expect(listeners[0].offline).not.toHaveBeenCalled();
    peer.request({ t: 'cmd', env: { p: '', k: '', s: '' } }).catch(() => {});
    expect(pc.ctl.frames().at(-1)).toStrictEqual({ t: 'cmd', env: { p: '', k: '', s: '' }, id: 2 });
  });

  it('cannot be brought back once it is closed', async () => {
    const { peer, listen } = await served();
    const probe = await probing(peer);
    const greeted = probe.hello();
    greeted.catch(() => {});
    const elsewhere = await probing(session.peer(other, hearing().listen));
    const foreign = last();
    peer.close();
    expect(pcs()[1].closes).toBe(1);
    await expect(greeted).rejects.toThrow(new Error('connection lost'));
    // A welcome that comes late: the peer adopts nothing and closes what it is handed.
    peer.adopt(probe, next);
    peer.adopt(elsewhere, next);
    expect(listen).toHaveBeenCalledTimes(1);
    expect(foreign.closes).toBe(1);
    expect(() => peer.dial(true)).toThrow(new Error('peer closed'));
    // Nothing more is offered to its node, whatever happens to the connections it had.
    const offered = () => clients[0].signals('offer').filter((offer) => offer.to === node);
    const before = offered().length;
    pcs()[1].becomes('failed');
    clients[0].hear({ t: 'presence', k: node, on: true });
    await wait(120_000);
    expect(offered()).toHaveLength(before);
  });

  it('forgets a loss when it closes', async () => {
    const { peer, pc } = await served();
    const release = peer.hold();
    pc.ctl.onclose();
    await flush();
    peer.close();
    release();
    await wait(120_000);
    expect(pcs()).toHaveLength(1);
  });
});

describe('the path', () => {
  it('is read from the selected pair when ctl opens and every second after', async () => {
    const { pc, conn } = await dialled();
    const onpath = vi.fn();
    conn.onpath = onpath;
    pc.stats = transportStats('host', 'prflx', 0.0456);
    clients[0].hear(answer(conn.conn));
    await wait(5000);
    expect(pc.statsAsked).toBe(0);
    expect(conn.path).toBeUndefined();
    pc.ctl.open();
    await flush();
    const direct = { kind: 'direct', rtt: 46, since: now + 5000 };
    expect(onpath.mock.calls).toStrictEqual([[direct]]);
    expect(conn.path).toStrictEqual(direct);
    await wait(999);
    expect(pc.statsAsked).toBe(1);
    pc.stats = transportStats('host', 'prflx');
    await wait(1);
    expect(onpath.mock.calls.at(-1)).toStrictEqual([{ ...direct, rtt: undefined }]);
    pc.stats = transportStats('relay', 'host', 0.2);
    await wait(1000);
    const relay = { kind: 'relay', rtt: 200, since: now + 7000 };
    expect(onpath.mock.calls.at(-1)).toStrictEqual([relay]);
    pc.stats = transportStats('srflx', 'relay', 0.2);
    await wait(1000);
    expect(onpath.mock.calls.at(-1)).toStrictEqual([relay]);
    pc.stats = transportStats('srflx', 'srflx', 0.2);
    await wait(1000);
    expect(onpath.mock.calls.at(-1)).toStrictEqual([
      { ...relay, kind: 'direct', since: now + 9000 },
    ]);
    expect(onpath).toHaveBeenCalledTimes(5);
  });

  it('is the pair marked selected where the statistics have no transport', async () => {
    const { pc, conn } = await opened();
    pc.stats = new Map<string, Record<string, unknown>>([
      ['S', { type: 'stream', id: 'S', selected: true, localCandidateId: 'H' }],
      ['P1', { type: 'candidate-pair', id: 'P1', selected: false, localCandidateId: 'H' }],
      ['P2', { type: 'candidate-pair', id: 'P2', selected: true, localCandidateId: 'R' }],
      ['H', { type: 'local-candidate', id: 'H', candidateType: 'host' }],
      ['R', { type: 'local-candidate', id: 'R', candidateType: 'relay' }],
    ]);
    await wait(1000);
    expect(conn.path).toStrictEqual({ kind: 'relay', rtt: undefined, since: now + 1000 });
  });

  it('is direct when the candidates of the pair are hidden, and nothing while no pair is selected', async () => {
    const { pc, conn } = await opened();
    const onpath = vi.fn();
    conn.onpath = onpath;
    const stats = transportStats('relay', 'relay');
    stats.delete('L');
    stats.delete('M');
    pc.stats = stats;
    await wait(1000);
    expect(conn.path).toStrictEqual({ kind: 'direct', rtt: undefined, since: now + 1000 });
    const none = transportStats('host', 'host');
    none.set('T', { type: 'transport', id: 'T' });
    pc.stats = none;
    await wait(1000);
    expect(conn.path).toBeUndefined();
    expect(onpath.mock.calls.at(-1)).toStrictEqual([undefined]);
  });

  it('is told to the peer for the connection that serves, and undefined when it is lost', async () => {
    const { pc, onpath } = await served();
    expect(onpath.mock.calls).toStrictEqual([[undefined]]);
    pc.stats = transportStats('host', 'host', 0.004);
    await wait(1000);
    expect(onpath.mock.calls.at(-1)).toStrictEqual([{ kind: 'direct', rtt: 4, since: now + 1000 }]);
    pc.ctl.onclose();
    await flush();
    expect(onpath.mock.calls.at(-1)).toStrictEqual([undefined]);
  });

  it('is not read after the connection is gone, and a failed reading changes nothing', async () => {
    const { pc, conn } = await opened();
    const onpath = vi.fn();
    conn.onpath = onpath;
    pc.stats = new Map([['broken', {}]]);
    await wait(1000);
    expect(onpath).not.toHaveBeenCalled();
    pc.stats = transportStats('host', 'host');
    const reading = Promise.withResolvers<void>();
    pc.hold = reading.promise;
    await wait(1000);
    const asked = pc.statsAsked;
    conn.close();
    reading.resolve();
    await wait(5000);
    expect(onpath).not.toHaveBeenCalled();
    expect(pc.statsAsked).toBe(asked);
  });
});
