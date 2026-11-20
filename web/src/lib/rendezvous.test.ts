import { afterEach, beforeAll, beforeEach, expect, it, vi, type Mock } from 'vitest';
import { Device, ed25519, verifyChallenge } from './identity';
import { rendezvous } from './rendezvous';

const local = 'ws://localhost:47800/rv';
const remote = 'wss://rv.example/rv';
const refused = 'ws://192.0.2.1:47800/rv'; // as a page served over HTTPS sees it
const room = '0123456789abcdef0123456789abcdef';
const data = { t: 'pair', k: 'key', kind: 'browser', name: 'web', role: 'a', mac: 'mac' } as const;
const ready = { t: 'ready', ice: [], ttl: 3600 };
const ice = [{ urls: ['turn:rv.example:3478'], username: 'user', credential: 'pass' }];
const conn = 'BBBBBBBBBBBBBBBBBBBBBA';
const sockets = new Map<string, FakeSocket>();

class FakeSocket {
  static OPEN = 1;
  static CLOSING = 2;
  readyState = FakeSocket.OPEN;
  /** Everything the client handed to send, in order. */
  sent: string[] = [];
  onmessage!: (event: { data: string }) => unknown;
  onclose!: () => void;
  constructor(readonly url: string) {
    // A browser throws here for a URL it refuses (SecurityError, SyntaxError).
    if (url === refused) throw new DOMException('insecure', 'SecurityError');
    expect(sockets.has(url)).toBe(false);
    sockets.set(url, this);
  }
  send(text: string) {
    this.sent.push(text);
  }
  close() {
    this.readyState = FakeSocket.CLOSING;
  }
  disconnected() {
    this.readyState = 3;
    this.onclose();
  }
  deliver(frame: unknown) {
    return this.onmessage({ data: JSON.stringify(frame) });
  }
  async authenticate(nonce = 'LdAL134CIs7YgmZUganB2Q') {
    await this.deliver({ t: 'challenge', nonce });
    await this.deliver(ready);
  }
  frames() {
    return this.sent.map((text) => JSON.parse(text));
  }
}

let browser: Device;
let node: Device;
const newDevice = async () =>
  Device.of(crypto, await crypto.subtle.generateKey(ed25519, false, ['sign', 'verify']));
beforeAll(async () => {
  [browser, node] = await Promise.all([newDevice(), newDevice()]);
});
beforeEach(() => {
  sockets.clear();
  vi.stubGlobal('WebSocket', FakeSocket);
});
afterEach(() => {
  vi.useRealTimers();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

function connect(urls = [remote]) {
  const receive = vi.fn();
  const client = rendezvous(urls, browser, (key) => key === node.id, receive);
  return { client, receive, rv: sockets.get(remote)! };
}

let markers = 0;
const isMarker = (frame: { t: string; k?: string }) => frame.k?.startsWith('marker ') === true;

/** Delivers frames on a socket and then a marker, and waits until the marker is received: every
 *  frame before it has been handled by then, as one path delivers in order. */
async function through(socket: FakeSocket, receive: Mock, ...frames: unknown[]) {
  for (const frame of frames) void socket.deliver(frame);
  const marker = { t: 'offline', k: `marker ${++markers}` };
  void socket.deliver(marker);
  await vi.waitFor(() => expect(receive.mock.calls.at(-1)).toStrictEqual([socket.url, marker]));
}

/** What receive got, without the markers. */
const got = (receive: Mock) => receive.mock.calls.filter(([, frame]) => !isMarker(frame));

/** A signalling payload from node to the browser, signed by node. */
const signal = (fields: Record<string, unknown> = {}) =>
  node.sign(
    JSON.stringify({
      t: 'answer',
      n: 'AAAAAAAAAAAAAAAAAAAAAA',
      ts: Date.now(),
      to: browser.id,
      conn,
      sdp: 'original',
      ...fields,
    }),
  );

it('signs the nonce text separately on each path and fans out without waiting for the slow one', async () => {
  const { client, receive, rv } = connect([local, remote, remote]);
  const lan = sockets.get(local)!;
  const header = { to: node.id, n: 'AAAAAAAAAAAAAAAAAAAAAA', ts: Date.now(), conn };
  const env = await browser.sign(
    JSON.stringify({ ...header, t: 'offer', probe: false, sdp: 'original description' }),
  );
  const offer = { t: 'to', k: node.id, env } as const;
  const candidate = {
    ...offer,
    env: await browser.sign(
      JSON.stringify({ ...header, n: 'CCCCCCCCCCCCCCCCCCCCCA', t: 'ice', cand: 'c', mid: '0' }),
    ),
  };
  client.send({ t: 'watch', k: [node.id] });
  await rv.authenticate();
  expect(lan.sent).toStrictEqual([]);
  client.send(offer);
  client.send(candidate);
  const [auth] = rv.frames();
  expect(rv.frames()).toStrictEqual([
    { t: 'auth', k: browser.id, s: auth.s },
    { t: 'watch', k: [node.id] },
    offer,
    candidate,
  ]);
  expect(await verifyChallenge(crypto, browser.id, 'LdAL134CIs7YgmZUganB2Q', auth.s)).toBe(true);
  await lan.authenticate('AAAAAAAAAAAAAAAAAAAAAA');
  const [lanAuth] = lan.frames();
  expect(await verifyChallenge(crypto, browser.id, 'AAAAAAAAAAAAAAAAAAAAAA', lanAuth.s)).toBe(true);
  expect(lan.sent.slice(1)).toStrictEqual(rv.sent.slice(1));
  await vi.waitFor(() =>
    expect(receive.mock.calls).toStrictEqual([
      [remote, ready],
      [local, ready],
    ]),
  );
});

it('carries watch replacement, pairing and credential refresh in both directions on each path', async () => {
  const { client, receive, rv } = connect([local, remote]);
  const lan = sockets.get(local)!;
  await lan.authenticate();
  await rv.deliver({ t: 'challenge', nonce: 'x' });
  await rv.deliver({ ...ready, ice });
  const commands: Parameters<typeof client.send>[0][] = [
    { t: 'watch', k: [node.id] },
    { t: 'watch', k: [] },
    { t: 'join', room },
    { t: 'room', room, data },
    { t: 'ice' },
  ];
  for (const command of commands) client.send(command);
  const events = [
    { t: 'presence', k: node.id, on: false },
    { t: 'offline', k: node.id },
    { t: 'joined', room },
    { t: 'room', room, data },
    { t: 'ice', ice, ttl: 7200 },
  ];
  for (const socket of [rv, lan]) {
    expect(socket.frames().slice(1)).toStrictEqual(commands);
    await through(socket, receive, ...events.map((event) => ({ ...event, future: 42 })));
  }
  expect(got(receive)).toStrictEqual([
    [local, ready],
    [remote, { ...ready, ice }],
    ...[remote, local].flatMap((url) => events.map((frame) => [url, frame])),
  ]);
});

it('A1.5: a changed answer never reaches the peer; the intact signed answer still does', async () => {
  const { receive, rv } = connect();
  await rv.authenticate();
  const env = await signal();
  const from = { t: 'from', k: node.id, env };
  await through(
    rv,
    receive,
    { ...from, env: { ...env, p: env.p.replace('original', 'altered') } },
    { ...from, k: browser.id, env: await browser.sign(env.p) },
    // A relay that names another sender than the signer gets nothing through.
    { ...from, k: browser.id },
    { ...from, env: await signal({ to: node.id }) },
    { ...from, env: await signal({ ts: 0 }) },
    // A conn that is no random id, as internal/peer/signal.go drops it.
    { ...from, env: await signal({ conn: 'c' }) },
    { ...from, env: await signal({ conn: undefined }) },
  );
  expect(got(receive)).toStrictEqual([[remote, ready]]);
  await through(rv, receive, from);
  expect(got(receive)).toStrictEqual([
    [remote, ready],
    [remote, from],
  ]);
});

it('delivers the frames of one path in the order they arrived, a from that is checked included', async () => {
  const { receive, rv } = connect();
  await rv.authenticate();
  const from = { t: 'from', k: node.id, env: await signal() };
  const offline = { t: 'offline', k: node.id };
  void rv.deliver(from);
  void rv.deliver(offline);
  rv.disconnected();
  await vi.waitFor(() =>
    expect(receive.mock.calls).toStrictEqual([
      [remote, ready],
      [remote, from],
      [remote, offline],
      [remote, { t: 'closed' }],
    ]),
  );
});

it('rejects a frame out of handshake order on just its path', async () => {
  const { client, receive, rv } = connect([local, remote]);
  const lan = sockets.get(local)!;
  await rv.deliver(ready);
  expect(rv.readyState).toBe(FakeSocket.CLOSING);
  await lan.authenticate();
  client.send({ t: 'ice' });
  expect(lan.frames().slice(1)).toStrictEqual([{ t: 'ice' }]);
  rv.disconnected();
  client.send({ t: 'watch', k: [] });
  expect(rv.sent).toStrictEqual([]);
  await vi.waitFor(() =>
    expect(receive.mock.calls).toStrictEqual([
      [local, ready],
      [remote, { t: 'closed' }],
    ]),
  );
  expect(lan.frames().slice(1)).toStrictEqual([{ t: 'ice' }, { t: 'watch', k: [] }]);
});

it('requires ready after auth, including the ICE list and a ttl of at least 1', async () => {
  for (const frame of [
    { t: 'ready', ice: [] },
    { t: 'ready', ice: [], ttl: 0 },
    { t: 'ready', ice: [], ttl: -5 },
  ]) {
    sockets.clear();
    const { client, receive, rv } = connect();
    await rv.deliver({ t: 'challenge', nonce: 'x' });
    await rv.deliver(frame);
    expect(rv.readyState).toBe(FakeSocket.CLOSING);
    client.send({ t: 'ice' });
    expect(rv.frames().map(({ t }) => t)).toStrictEqual(['auth']);
    await new Promise((resolve) => setTimeout(resolve));
    expect(receive.mock.calls).toStrictEqual([]);
  }
  const { receive } = connect([local]);
  const lan = sockets.get(local)!;
  await lan.deliver({ t: 'challenge', nonce: 'x' });
  await lan.deliver({ t: 'ready', ice: [], ttl: 1 });
  await vi.waitFor(() =>
    expect(receive.mock.calls).toStrictEqual([[local, { t: 'ready', ice: [], ttl: 1 }]]),
  );
});

it('passes on only the ICE entries the browser can use, and drops an ice answer with ttl 0', async () => {
  const stun = { urls: ['stun:rv.example:3478'] };
  const keep = [
    stun,
    { urls: ['stun:rv.example', 'stuns:[2001:db8::1]:5349'] },
    { urls: ['stun:rv.example:1', 'stun:rv.example:9', 'stun:rv.example:65535'] },
    { urls: ['stun:rv.example:10000', 'stun:rv.example:59999', 'stun:rv.example:64999'] },
    { urls: ['stun:rv.example:65499', 'stun:rv.example:65529'] },
    {
      urls: ['turn:rv.example:3478?transport=udp', 'turns:turn.rv.example:443?transport=tcp'],
      username: 'u',
      credential: 'c',
    },
    { urls: ['stun:rv.example'], username: '', credential: '' },
  ];
  const drop = [
    { urls: [] },
    { urls: ['stun:rv.example:3478?transport=udp'] },
    { urls: ['stun:'] },
    { urls: ['stun:rv.example:'] },
    { urls: ['stun:rv.example:port'] },
    { urls: ['stun:rv.example:0'] },
    { urls: ['stun:rv.example:080'] },
    { urls: ['stun:rv.example:65536'] },
    { urls: ['stun:rv.example:65540'] },
    { urls: ['stun:rv.example:65600'] },
    { urls: ['stun:rv.example:66000'] },
    { urls: ['stun:rv.example:70000'] },
    { urls: ['stun:rv.example:99999'] },
    { urls: ['stun:rv.example:100000'] },
    { urls: ['STUN:rv.example'] },
    { urls: ['http://rv.example'] },
    { urls: ['stun:rv.example', 'bogus'] },
    { urls: ['turn:rv.example'], credential: 'c' },
    { urls: ['turn:rv.example'], username: '', credential: 'c' },
    { urls: ['turns:rv.example'], username: 'u' },
    { urls: ['turn:rv.example'], username: 'u', credential: '' },
    { urls: ['turn:rv.example?transport=sctp'], username: 'u', credential: 'c' },
    { urls: ['stun:rv.example', 'turn:rv.example'], credential: 'c' },
  ];
  const { receive, rv } = connect();
  await rv.deliver({ t: 'challenge', nonce: 'x' });
  await rv.deliver({ t: 'ready', ice: [...drop, ...keep], ttl: 3600 });
  await through(
    rv,
    receive,
    { t: 'ice', ice: [...keep, ...drop], ttl: 0 },
    { t: 'ice', ice: [...drop, stun], ttl: 1 },
  );
  expect(got(receive)).toStrictEqual([
    [remote, { t: 'ready', ice: keep, ttl: 3600 }],
    [remote, { t: 'ice', ice: [stun], ttl: 1 }],
  ]);
});

it('ignores a frame that is no JSON or matches no schema', async () => {
  const { receive, rv } = connect();
  await rv.authenticate();
  for (const data of ['{', '{}', '{"t":"ice","ttl":1}', '{"t":"future"}']) rv.onmessage({ data });
  const event = { t: 'offline', k: node.id };
  await through(rv, receive, event);
  expect(got(receive)).toStrictEqual([
    [remote, ready],
    [remote, event],
  ]);
});

const pacedClock = () =>
  vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout', 'performance'] });

it('sends at most 90 frames in any 11 seconds on a path, in order, and queues the rest', async () => {
  pacedClock();
  const { client, rv } = connect();
  const joins = (from: number, to: number) =>
    Array.from({ length: to - from }, (_, i) => ({
      t: 'join' as const,
      room: (from + i).toString(16).padStart(32, '0'),
    }));
  for (const frame of joins(0, 150)) client.send(frame);
  await rv.authenticate();
  // The auth counts: 89 frames go with it.
  expect(rv.frames()).toStrictEqual([rv.frames()[0], ...joins(0, 89)]);
  // Not at 10 seconds, where the server's window ends: a second later.
  vi.advanceTimersByTime(10_999);
  client.send(joins(150, 151)[0]);
  expect(rv.sent).toHaveLength(90);
  vi.advanceTimersByTime(1);
  expect(rv.frames().slice(1)).toStrictEqual(joins(0, 151));
  // The window slides: the 28 places of the frames sent at 0 are free, the rest in 11 seconds.
  for (const frame of joins(151, 191)) client.send(frame);
  expect(rv.frames().slice(1)).toStrictEqual(joins(0, 179));
  vi.advanceTimersByTime(10_999);
  expect(rv.sent).toHaveLength(180);
  vi.advanceTimersByTime(1);
  expect(rv.frames().slice(1)).toStrictEqual(joins(0, 191));
  // The 90 frames of 11 seconds ago free their places; what waits when the client closes is
  // never sent.
  for (const frame of joins(191, 300)) client.send(frame);
  expect(rv.frames().slice(1)).toStrictEqual(joins(0, 269));
  client.close();
  vi.advanceTimersByTime(30_000);
  expect(rv.frames().slice(1)).toStrictEqual(joins(0, 269));
});

it('frees each place 11 seconds after its own frame, not all at once', async () => {
  pacedClock();
  const { client, rv } = connect();
  await rv.authenticate();
  const ice = () => client.send({ t: 'ice' });
  for (let i = 0; i < 44; i++) ice(); // with the auth, 45 frames at 0 s
  vi.advanceTimersByTime(5_000);
  for (let i = 0; i < 145; i++) ice(); // 45 go at 5 s, 100 wait
  expect(rv.sent).toHaveLength(90);
  vi.advanceTimersByTime(5_999);
  expect(rv.sent).toHaveLength(90);
  vi.advanceTimersByTime(1);
  expect(rv.sent).toHaveLength(135); // at 11 s, the 45 places of 0 s
  vi.advanceTimersByTime(4_999);
  expect(rv.sent).toHaveLength(135);
  vi.advanceTimersByTime(1);
  expect(rv.sent).toHaveLength(180); // at 16 s, the 45 places of 5 s
  vi.advanceTimersByTime(6_000);
  expect(rv.sent).toHaveLength(190); // at 22 s, 10 of the places of 11 s
});

it('paces by a clock that a wall clock set back does not hold up', async () => {
  pacedClock();
  const { client, rv } = connect();
  await rv.authenticate();
  for (let i = 0; i < 150; i++) client.send({ t: 'ice' });
  expect(rv.sent).toHaveLength(90);
  vi.spyOn(Date, 'now').mockReturnValue(Date.now() - 3_600_000);
  vi.advanceTimersByTime(11_000);
  expect(rv.sent).toHaveLength(151);
});

it('refuses a frame of more than 16 KiB in UTF-8 on every path, and sends one of exactly 16 KiB', async () => {
  const { client, rv } = connect([local, remote]);
  const lan = sockets.get(local)!;
  await rv.authenticate();
  await lan.authenticate();
  const frame = { t: 'room', room, data: { ...data, name: '' } } as const;
  const room16 = (bytes: number) => {
    const free = bytes - JSON.stringify(frame).length;
    return {
      ...frame,
      data: { ...frame.data, name: 'é'.repeat(free >> 1) + 'a'.repeat(free % 2) },
    };
  };
  expect(new TextEncoder().encode(JSON.stringify(room16(16_384)))).toHaveLength(16_384);
  expect(JSON.stringify(room16(16_384)).length).toBeLessThan(16_384);
  client.send(room16(16_384));
  expect(() => client.send(room16(16_385))).toThrow(
    new RangeError('signalling frame exceeds 16 KiB'),
  );
  for (const socket of [rv, lan]) expect(socket.frames().slice(1)).toStrictEqual([room16(16_384)]);
});

it('refuses a to of more than 16,382 bytes, whose forwarded from would exceed 16 KiB', async () => {
  const { client, rv } = connect();
  await rv.authenticate();
  const to = (bytes: number) => {
    const frame = { t: 'to' as const, k: node.id, env: { p: '', k: browser.id, s: 's' } };
    const p = 'a'.repeat(bytes - JSON.stringify(frame).length);
    return { ...frame, env: { ...frame.env, p } };
  };
  expect(JSON.stringify(to(16_382))).toHaveLength(16_382);
  client.send(to(16_382));
  for (const bytes of [16_383, 16_384]) {
    expect(() => client.send(to(bytes))).toThrow(new RangeError('signalling frame exceeds 16 KiB'));
  }
  expect(rv.frames().slice(1)).toStrictEqual([to(16_382)]);
});

it('reports a receive that throws and goes on delivering the frames after it', async () => {
  const failure = new Error('receive failed');
  const reported = vi.fn();
  vi.stubGlobal('reportError', reported);
  const receive = vi
    .fn()
    .mockImplementationOnce(() => {})
    .mockImplementationOnce(() => {
      throw failure;
    });
  rendezvous([remote], browser, (key) => key === node.id, receive);
  const rv = sockets.get(remote)!;
  await rv.authenticate();
  const presence = { t: 'presence', k: node.id, on: true };
  const offline = { t: 'offline', k: node.id };
  void rv.deliver(presence);
  void rv.deliver(offline);
  rv.disconnected();
  await vi.waitFor(() =>
    expect(receive.mock.calls).toStrictEqual([
      [remote, ready],
      [remote, presence],
      [remote, offline],
      [remote, { t: 'closed' }],
    ]),
  );
  expect(reported.mock.calls).toStrictEqual([[failure]]);
});

it('drops a from whose check throws, reports it, and delivers the frames after it', async () => {
  const failure = new Error('known failed');
  const reported = vi.fn();
  vi.stubGlobal('reportError', reported);
  const known = vi
    .fn()
    .mockImplementationOnce(() => {
      throw failure;
    })
    .mockImplementation((key: string) => key === node.id);
  const receive = vi.fn();
  rendezvous([remote], browser, known, receive);
  const rv = sockets.get(remote)!;
  await rv.authenticate();
  const from = { t: 'from', k: node.id, env: await signal() };
  const offline = { t: 'offline', k: node.id };
  void rv.deliver(from);
  void rv.deliver(from);
  void rv.deliver(offline);
  rv.disconnected();
  await vi.waitFor(() =>
    expect(receive.mock.calls).toStrictEqual([
      [remote, ready],
      [remote, from],
      [remote, offline],
      [remote, { t: 'closed' }],
    ]),
  );
  expect(reported.mock.calls).toStrictEqual([[failure]]);
});

it('reports a URL the browser refuses as closed, after the call, and keeps the other paths', async () => {
  const { client, receive, rv } = connect([refused, remote]);
  expect(receive.mock.calls).toStrictEqual([]);
  await rv.authenticate();
  client.send({ t: 'ice' });
  expect(rv.frames().slice(1)).toStrictEqual([{ t: 'ice' }]);
  await vi.waitFor(() =>
    expect(receive.mock.calls).toStrictEqual([
      [refused, { t: 'closed' }],
      [remote, ready],
    ]),
  );
  client.close();
  expect(rv.readyState).toBe(FakeSocket.CLOSING);
});

it('delivers nothing once closed, not even the closed of a refused URL', async () => {
  const { client, receive, rv } = connect([refused, remote]);
  client.close();
  await rv.deliver({ t: 'challenge', nonce: 'x' });
  await new Promise((resolve) => setTimeout(resolve));
  expect(receive.mock.calls).toStrictEqual([]);
});

it('delivers nothing after close(), and sends nothing more on that path', async () => {
  const { client, receive, rv } = connect();
  await rv.authenticate();
  await vi.waitFor(() => expect(receive.mock.calls).toStrictEqual([[remote, ready]]));
  const from = { t: 'from', k: node.id, env: await signal() };
  void rv.deliver(from);
  client.close();
  // The socket is still closing: its close event comes later.
  client.send({ t: 'ice' });
  void rv.deliver({ t: 'offline', k: node.id });
  rv.disconnected();
  client.send({ t: 'ice' });
  await new Promise((resolve) => setTimeout(resolve, 50));
  expect(receive.mock.calls).toStrictEqual([[remote, ready]]);
  expect(rv.frames().map(({ t }) => t)).toStrictEqual(['auth']);
});

it('sends nothing on a path the server closed, before or after ready', async () => {
  const { client, receive, rv } = connect([local, remote]);
  const lan = sockets.get(local)!;
  client.send({ t: 'ice' });
  rv.disconnected();
  await lan.authenticate();
  lan.disconnected();
  client.send({ t: 'watch', k: [] });
  expect(rv.sent).toStrictEqual([]);
  expect(lan.frames().map(({ t }) => t)).toStrictEqual(['auth', 'ice']);
  await vi.waitFor(() =>
    expect(receive.mock.calls).toStrictEqual([
      [remote, { t: 'closed' }],
      [local, ready],
      [local, { t: 'closed' }],
    ]),
  );
});
