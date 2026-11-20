import { afterEach, beforeAll, beforeEach, expect, it, vi } from 'vitest';
import { Device, ed25519, verifyChallenge } from './identity';
import { rendezvous } from './rendezvous';

const local = 'ws://localhost:47800/rv';
const remote = 'wss://rv.example/rv';
const room = '0123456789abcdef0123456789abcdef';
const data = { t: 'pair', k: 'key', kind: 'browser', name: 'web', role: 'a', mac: 'mac' } as const;
const ready = { t: 'ready', ice: [], ttl: 3600 };
const ice = [{ urls: ['turn:rv.example:3478'], username: 'user', credential: 'pass' }];
const sockets = new Map<string, FakeSocket>();

class FakeSocket {
  static OPEN = 1;
  static CLOSING = 2;
  readyState = FakeSocket.OPEN;
  sent: string[] = [];
  onmessage!: (event: { data: unknown }) => unknown;
  onclose!: () => void;
  constructor(url: string) {
    expect(sockets.has(url)).toBe(false);
    sockets.set(url, this);
  }
  send(text: string) {
    expect(this.readyState).toBe(FakeSocket.OPEN);
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
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

function connect(urls = [remote]) {
  const receive = vi.fn();
  const client = rendezvous(urls, browser, (key) => key === node.id, receive);
  return { client, receive, rv: sockets.get(remote)! };
}

it('signs the nonce text separately on each path and fans out without waiting for the slow one', async () => {
  const { client, receive, rv } = connect([local, remote, remote]);
  const lan = sockets.get(local)!;
  const header = {
    to: node.id,
    n: 'AAAAAAAAAAAAAAAAAAAAAA',
    ts: Date.now(),
    conn: 'BBBBBBBBBBBBBBBBBBBBBA',
  };
  const env = await browser.sign(
    JSON.stringify({ ...header, t: 'offer', probe: false, sdp: 'original description' }),
  );
  const offer = { t: 'to', k: node.id, env } as const;
  const candidate = {
    ...offer,
    env: await browser.sign(
      JSON.stringify({
        ...header,
        n: 'CCCCCCCCCCCCCCCCCCCCCA',
        t: 'ice',
        cand: 'candidate',
        mid: '0',
      }),
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
  expect(receive.mock.calls).toStrictEqual([
    [remote, ready],
    [local, ready],
  ]);
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
    for (const event of events) await socket.deliver({ ...event, future: 42 });
  }
  expect(receive.mock.calls).toStrictEqual([
    [local, ready],
    [remote, { ...ready, ice }],
    ...[remote, local].flatMap((url) => events.map((frame) => [url, frame])),
  ]);
});

it('A1.5: a changed answer never reaches the peer; the intact signed answer still does', async () => {
  const { receive, rv } = connect();
  await rv.authenticate();
  const p = {
    t: 'answer',
    n: 'AAAAAAAAAAAAAAAAAAAAAA',
    ts: Date.now(),
    to: browser.id,
    conn: 'c',
    sdp: 'original',
  };
  const env = await node.sign(JSON.stringify(p));
  const from = { t: 'from', k: node.id, env };
  await rv.deliver({ ...from, env: { ...env, p: env.p.replace('original', 'altered') } });
  await rv.deliver({ ...from, k: browser.id });
  await rv.deliver({ ...from, k: browser.id, env: await browser.sign(env.p) });
  await rv.deliver({ ...from, env: await node.sign(JSON.stringify({ ...p, to: node.id })) });
  await rv.deliver({ ...from, env: await node.sign(JSON.stringify({ ...p, ts: 0 })) });
  expect(receive.mock.calls).toStrictEqual([[remote, ready]]);
  await rv.deliver(from);
  expect(receive.mock.calls).toStrictEqual([
    [remote, ready],
    [remote, from],
  ]);
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
  expect(receive.mock.calls).toStrictEqual([
    [local, ready],
    [remote, { t: 'closed' }],
  ]);
  client.close();
  expect(lan.readyState).toBe(FakeSocket.CLOSING);
  client.send({ t: 'ice' });
  expect(lan.frames().slice(1)).toStrictEqual([{ t: 'ice' }, { t: 'watch', k: [] }]);
});

it('closes the path when its challenge cannot be signed', async () => {
  vi.spyOn(browser, 'signChallenge').mockRejectedValue(new Error('key unavailable'));
  const { receive, rv } = connect();
  await rv.deliver({ t: 'challenge', nonce: 'LdAL134CIs7YgmZUganB2Q' });
  expect(rv.readyState).toBe(FakeSocket.CLOSING);
  expect(rv.sent).toStrictEqual([]);
  expect(receive.mock.calls).toStrictEqual([]);
});

it('does not authenticate if a premature ready arrives during signing', async () => {
  const { receive, rv } = connect();
  const signing = rv.deliver({ t: 'challenge', nonce: 'x' });
  await rv.deliver(ready);
  await signing;
  expect(rv.readyState).toBe(FakeSocket.CLOSING);
  expect(rv.sent).toStrictEqual([]);
  expect(receive.mock.calls).toStrictEqual([]);
});

it('requires ready after auth, including the ICE list and TTL', async () => {
  const { receive, rv } = connect();
  await rv.deliver({ t: 'challenge', nonce: 'x' });
  await rv.deliver({ t: 'ready', ice: [] });
  expect(rv.readyState).toBe(FakeSocket.CLOSING);
  expect(receive.mock.calls).toStrictEqual([]);
});

it('ignores malformed frames, binary messages and text larger than 16 KiB in UTF-8', async () => {
  const { receive, rv } = connect();
  await rv.authenticate();
  const event = { t: 'offline', k: '' };
  event.k = 'é'.repeat((16_384 - JSON.stringify(event).length) >> 1);
  const text = JSON.stringify(event).padEnd(16_384 - event.k.length);
  expect(new TextEncoder().encode(text)).toHaveLength(16_384);
  for (const data of [
    text + ' ',
    new Blob([text]),
    { toString: () => text },
    '{',
    '{"t":"ice","ttl":1}',
    '{}',
  ]) {
    await rv.onmessage({ data });
  }
  await rv.onmessage({ data: text });
  expect(receive.mock.calls).toStrictEqual([
    [remote, ready],
    [remote, event],
  ]);
});

it('does not deliver a signal if the path closes while its signature is checked', async () => {
  const { client, rv, receive } = connect();
  await rv.authenticate();
  const env = await node.sign(
    JSON.stringify({
      t: 'ice',
      to: browser.id,
      n: 'AAAAAAAAAAAAAAAAAAAAAA',
      ts: Date.now(),
      conn: 'c',
      mid: '0',
      cand: 'candidate',
    }),
  );
  const verifying = rv.deliver({ t: 'from', k: node.id, env });
  client.close();
  await verifying;
  expect(receive.mock.calls).toStrictEqual([[remote, ready]]);
});

it('enforces the outbound watch and UTF-8 frame limits at the boundary', async () => {
  const { client, rv } = connect();
  await rv.authenticate();
  const watch = { t: 'watch', k: Array<string>(64).fill(node.id) } as const;
  client.send(watch);
  expect(() => client.send({ t: 'watch', k: [...watch.k, node.id] })).toThrow(
    'watch exceeds 64 ids',
  );
  const frame = { t: 'room', room, data: { ...data, name: '' } } as const;
  const remaining = 16_384 - JSON.stringify(frame).length;
  const large = {
    ...frame,
    data: { ...frame.data, name: 'é'.repeat(remaining >> 1) + 'a'.repeat(remaining % 2) },
  };
  client.send(large);
  expect(() =>
    client.send({ ...large, data: { ...large.data, name: large.data.name + 'a' } }),
  ).toThrow('frame exceeds 16 KiB');
  expect(rv.frames().slice(1)).toStrictEqual([watch, large]);
});
