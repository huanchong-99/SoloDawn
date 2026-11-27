import { describe, expect, it, vi } from 'vitest';
import type { DeviceLog } from './devlog';
import type { Home } from './groups.svelte';
import { encode, type Device } from './identity';
import { addHome, joinGroup, presentLog, readCode, readInvitation, type Invitation } from './join';
import { mac, parseSecret, room } from './pairing';
import type { Listener, Session } from './peer';
import type { Ctl, Devlog, Envelope, Welcome } from './proto';

const g = encode(new Uint8Array(16));
const secret = parseSecret('7K2M-9QXD-4F8T-WB3N');
const home = encode(new Uint8Array(32).fill(2));

describe('readCode', () => {
  it('removes every white space and reads the rest as a typed secret', () => {
    expect(readCode(' 7k2m 9qxd\t4f8t\nwb3n\r\n')).toEqual({ secret });
    expect(readCode('7K2M-9QXD-4F8T-WB3N')).toEqual({ secret });
  });

  it.each(['7K2M-9QXD-4F8T-WB3O', '7K2M9QXD4F8TWB3', '7K2M_9QXD_4F8T_WB3N', ''])(
    'refuses %j, which is no code',
    (text) => expect(readCode(text)).toBeUndefined(),
  );
});

describe('readInvitation', () => {
  it('reads the three parts of an invitation, decoding the URL and typed secret', () => {
    expect(readInvitation('#/groups')).toBeUndefined();
    expect(readInvitation(`#join=7k2m%2D9qxd-4f8t-wb3n&g=${g}&home=${home}`)).toEqual({
      secret: parseSecret('7K2M-9QXD-4F8T-WB3N'),
      g,
      home,
    });
  });

  it.each([
    `#join=wrong&g=${g}&home=${home}`,
    `#join=7K2M-9QXD-4F8T-WB3N&g=wrong&home=${home}`,
    `#join=7K2M-9QXD-4F8T-WB3N&g=${g}&home=${encode(new Uint8Array(32))}`,
    `#join=7K2M-9QXD-4F8T-WB3N&g=${g}`,
  ])('refuses malformed invitation %s before making a connection', (hash) => {
    expect(() => readInvitation(hash)).toThrow('link: invalid');
  });
});

it('presents the log in ordered frames no larger than the ctl byte limit', () => {
  const entries = ['雪'.repeat(15_000), 'a'.repeat(16_000), 'b'.repeat(1000)].map((p) => ({
    p,
    k: home,
    s: 'signature',
  }));
  const log = { user: home, entries } as DeviceLog;
  const frames: Devlog[] = [];
  const send = vi.fn((frame) => frames.push(structuredClone(frame)));
  presentLog({ send }, log);
  expect(frames).toStrictEqual([
    { t: 'devlog', user: home, entries: entries.slice(0, 2) },
    { t: 'devlog', user: home, entries: entries.slice(2) },
  ]);
});

it('keeps a frame exactly at the ctl byte limit intact', () => {
  const frame: Devlog = {
    t: 'devlog',
    user: home,
    entries: [{ p: '', k: home, s: 'signature' }],
  };
  frame.entries[0].p = 'a'.repeat(61_440 - JSON.stringify(frame).length);
  const send = vi.fn((value: Devlog) => structuredClone(value));
  presentLog({ send }, { user: home, entries: frame.entries } as DeviceLog);
  expect(send.mock.results.map(({ value }) => value)).toEqual([frame]);
});

const invitation = { g, home, secret };
const browser = encode(new Uint8Array(32).fill(3));
const url = 'wss://rv.example/rv';
const node = 'ws://192.0.2.10:47800/rv';
const welcome: Welcome = {
  t: 'welcome',
  re: 1,
  ok: true,
  v: 3,
  name: 'Home',
  version: 'test',
  time: 1,
  device: home,
  groups: [],
  owner: false,
  addrs: [],
};

function attempt(
  valid = true,
  signal = new AbortController().signal,
  nodes: Home[] = [],
  given: Invitation = invitation,
  roomWait?: number,
) {
  const options = Promise.withResolvers<{ heard: Parameters<Session['watchRoom']>[2] }>();
  const listen = Promise.withResolvers<Listener>();
  const close = vi.fn();
  const send = vi.fn();
  const peer = vi.fn((_home: string, fn: () => Listener) => {
    listen.resolve(fn());
    return { close, send };
  });
  const stop = vi.fn();
  const watchRoom = vi.fn((_: string, _id: string, heard: Parameters<Session['watchRoom']>[2]) => {
    options.resolve({ heard });
    return stop;
  });
  const remember = vi.fn(async (_home: string, _url: string, _name: string) => {});
  const link = { online: vi.fn(), push: vi.fn(), offline: vi.fn() };
  const push = vi.fn();
  const request = vi.fn();
  const result = joinGroup(given, 'Joiner', {
    device: {
      id: browser,
      sign: (p: string) => Promise.resolve({ p, k: browser, s: 's' }),
    } as Device,
    log: {
      user: browser,
      valid: () => valid,
      entries: [{ p: 'entry', k: browser, s: 's' }],
    } as unknown as DeviceLog,
    url,
    urls: [url, node],
    session: { send, watchRoom, roomWait },
    dial: peer,
    signal,
    browserName: 'Web',
    remember,
    link: () => link,
    ownNodes: () => nodes,
    onpath: vi.fn(),
  });
  void result.catch(() => {});
  const offer = async (k = home) => {
    const opts = await options.promise;
    const id = await room(crypto, secret);
    const fields = { role: 'a', k, kind: 'node', name: 'Home' };
    const data = { t: 'pair' as const, ...fields, mac: await mac(crypto, secret, fields) };
    return { opts, id, data };
  };
  const matched = async () => {
    const { opts, id, data } = await offer();
    opts.heard!(url, { t: 'room', room: id, data });
    return listen.promise;
  };
  return {
    result,
    matched,
    offer,
    peer,
    send,
    close,
    stop,
    watchRoom,
    request,
    push,
    remember,
    link,
    options: options.promise,
  };
}

it('does not contact rendezvous before onboarding', async () => {
  const run = attempt(false);
  await expect(run.result).rejects.toMatchObject({
    step: 'onboarding',
    code: 'unpaired',
  });
  expect(run.watchRoom).not.toHaveBeenCalled();
});

it('does not contact rendezvous after cancellation', async () => {
  const run = attempt(true, AbortSignal.abort());
  await expect(run.result).rejects.toMatchObject({
    step: 'room',
    code: 'cancelled',
  });
  expect(run.watchRoom).not.toHaveBeenCalled();
});

it.each([
  [2, { re: 2, ok: true }, { step: 'hello', code: 'version' }],
  [
    3,
    { re: 2, err: { code: 'forbidden', msg: 'invalid log' } },
    { step: 'join', code: 'forbidden' },
  ],
])('closes the connection when a v%i home does not admit the browser', async (v, answer, error) => {
  const run = attempt();
  const listener = await run.matched();
  run.request.mockResolvedValue(answer);
  listener.online({ request: run.request, send: run.push } as Ctl, { ...welcome, v });
  await expect(run.result).rejects.toMatchObject(error);
  expect(run.close).toHaveBeenCalledOnce();
  expect(run.remember).not.toHaveBeenCalled();
});

it('closes a cancelled room without sending join', async () => {
  const cancel = new AbortController();
  const run = attempt(true, cancel.signal);
  const { opts, id } = await run.offer();
  cancel.abort();
  expect(run.stop).toHaveBeenCalledExactlyOnceWith();
  await expect(run.result).rejects.toMatchObject({ step: 'room', code: 'cancelled' });
  expect(run.stop).toHaveBeenCalled();
  expect(run.close).not.toHaveBeenCalled();
  expect(run.request).not.toHaveBeenCalled();
  opts.heard!(url, { t: 'joined', room: id });
  expect(run.send).not.toHaveBeenCalled();
});

it('dials only the invited node, from a matching authenticated room message', async () => {
  const run = attempt();
  const { opts, id, data } = await run.offer();
  const foreign = { role: 'a', k: browser, kind: 'node', name: 'Foreign' };
  const otherKind = { role: data.role, k: data.k, name: data.name, kind: 'browser' };
  const foreignMac = await mac(crypto, secret, foreign);
  const otherKindMac = await mac(crypto, secret, otherKind);
  const importKey = vi.spyOn(crypto.subtle, 'importKey');
  expect(run.watchRoom).toHaveBeenCalledExactlyOnceWith(url, id, opts.heard);
  opts.heard('wss://other.example/rv', { t: 'joined', room: id });
  opts.heard!(url, { t: 'presence', k: home, on: true });
  opts.heard!(url, { t: 'joined', room: 'another room' });
  opts.heard!(url, { t: 'room', room: 'another room', data });
  opts.heard!(url, {
    t: 'room',
    room: id,
    data: { t: 'pair', ...foreign, mac: foreignMac },
  });
  opts.heard!(url, {
    t: 'room',
    room: id,
    data: { t: 'pair', ...otherKind, mac: otherKindMac },
  });
  expect(importKey).not.toHaveBeenCalled();
  expect(run.send).not.toHaveBeenCalled();
  await Promise.resolve();
  expect(run.peer).not.toHaveBeenCalled();
  const listener = await run.matched();
  expect(importKey).toHaveBeenCalledTimes(2);
  opts.heard!(url, { t: 'joined', room: id });
  importKey.mockRestore();
  opts.heard!(url, { t: 'ready', ice: [], ttl: 0 });
  const refusal = { step: 'hello', code: 'forbidden' };
  listener.refused!('forbidden');
  await expect(run.result).rejects.toMatchObject(refusal);
  expect(run.peer).toHaveBeenCalledExactlyOnceWith(
    home,
    expect.any(Function),
    expect.any(Function),
  );
  expect(run.send).not.toHaveBeenCalled();
});

it.each([
  ['devlog', new Error('send failed')],
  ['join', new Error('request failed')],
])('reports transport errors at the %s step', async (step, error) => {
  const run = attempt();
  const listener = await run.matched();
  if (step === 'devlog')
    run.push.mockImplementation(() => {
      throw error;
    });
  else run.request.mockRejectedValue(error);
  listener.online({ request: run.request, send: run.push } as Ctl, welcome);
  await expect(run.result).rejects.toMatchObject({ step, code: 'lost' });
  expect(run.close).toHaveBeenCalledOnce();
});

it('reports a connection loss before the home confirms the group', async () => {
  const run = attempt();
  const listener = await run.matched();
  run.request.mockResolvedValue({ re: 2, ok: true });
  listener.online({ request: run.request, send: run.push } as Ctl, welcome);
  await vi.waitFor(() => expect(run.remember).toHaveBeenCalledWith(home, url, 'Home'));
  listener.offline();
  await expect(run.result).rejects.toMatchObject({ step: 'groups', code: 'lost' });
  expect(run.close).toHaveBeenCalledOnce();
});

it('waits for a groups push containing the invitation despite unrelated groups', async () => {
  const run = attempt();
  const listener = await run.matched();
  const other = { g: encode(new Uint8Array(16).fill(1)), name: 'Other', seq: 1 };
  const shared = { g, name: 'Shared', seq: 2 };
  const ctl = { request: run.request, send: run.push } as Ctl;
  run.request.mockResolvedValue({ re: 2, ok: true });
  listener.online(ctl, { ...welcome, groups: [other] });
  await vi.waitFor(() => expect(run.remember).toHaveBeenCalledWith(home, url, 'Home'));
  listener.push({ t: 'groups', groups: [other, shared] });
  const result = await run.result;
  expect(result.g).toBe(g);
  expect(await result.notices).toEqual([]);
  expect(run.stop).toHaveBeenCalledOnce();
  expect(run.link.online).toHaveBeenCalledExactlyOnceWith(ctl, { ...welcome, groups: [other] });
  expect(run.link.push).toHaveBeenCalledExactlyOnceWith({
    t: 'groups',
    groups: [other, shared],
  });
});

it('reports a failed write while remembering an admitted home', async () => {
  const run = attempt();
  const listener = await run.matched();
  run.request.mockResolvedValue({ re: 2, ok: true });
  run.remember.mockRejectedValue(new Error('write failed'));
  listener.online({ request: run.request, send: run.push } as Ctl, welcome);
  await expect(run.result).rejects.toMatchObject({ step: 'remember', code: 'lost' });
  expect(run.close).toHaveBeenCalledOnce();
  expect(run.link.online).not.toHaveBeenCalled();
});

it('closes on cancellation during persistence and ignores further room messages', async () => {
  const cancel = new AbortController();
  const run = attempt(true, cancel.signal);
  const { opts } = await run.offer();
  const listener = await run.matched();
  const write = Promise.withResolvers<void>();
  run.request.mockResolvedValue({ re: 2, ok: true });
  run.remember.mockReturnValue(write.promise);
  listener.online({ request: run.request, send: run.push } as Ctl, welcome);
  await vi.waitFor(() => expect(run.remember).toHaveBeenCalledWith(home, url, 'Home'));
  cancel.abort();
  expect(run.close).toHaveBeenCalledOnce();
  opts.heard!(url, { t: 'ready', ice: [], ttl: 0 });
  expect(run.send).not.toHaveBeenCalled();
  write.resolve();
  await expect(run.result).rejects.toMatchObject({ step: 'remember', code: 'cancelled' });
});

it('clears the invitation timer and cancellation listener after joining', async () => {
  vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout'] });
  try {
    const cancel = new AbortController();
    const run = attempt(true, cancel.signal);
    const listener = await run.matched();
    run.request.mockResolvedValue({ re: 2, ok: true });
    listener.online({ request: run.request, send: run.push } as Ctl, {
      ...welcome,
      groups: [{ g, name: 'Group', seq: 1 }],
    });
    await run.result;
    expect(vi.getTimerCount()).toBe(0);
    cancel.abort();
    expect(run.close).not.toHaveBeenCalled();
  } finally {
    vi.useRealTimers();
  }
});

it.each(['cancelled', 'lost'])('reports %s while signing home.add', async (code) => {
  const cancel = new AbortController();
  const signed = Promise.withResolvers<Envelope>();
  const sign = vi.fn(() => signed.promise);
  const run = attempt(true, cancel.signal, [{ me: { sign }, device: home } as unknown as Home]);
  const listener = await run.matched();
  const ctl = { request: run.request, send: run.push } as Ctl;
  run.request.mockResolvedValue({ re: 2, ok: true });
  listener.online(ctl, { ...welcome, groups: [{ g, name: 'Shared', seq: 1 }] });
  await vi.waitFor(() => expect(sign).toHaveBeenCalledOnce());
  if (code === 'cancelled') {
    cancel.abort();
    signed.resolve({ p: 'notice', k: browser, s: 's' });
  } else signed.reject(new Error('sign failed'));
  await expect(run.result).rejects.toMatchObject({ step: 'home.add', code });
  expect(run.link.online).not.toHaveBeenCalled();
});

it('keeps the admitted group when its own nodes refuse or lose the notification', async () => {
  const sign = async (p: string) => ({ p, k: browser, s: 's' });
  const refuse = vi.fn(async (_request: { env: Envelope }) => ({ err: { code: 'bad_request' } }));
  const nodes = [
    {
      device: 'older',
      me: { sign },
      ctl: { request: refuse },
    },
    {
      device: 'lost',
      me: { sign },
      ctl: { request: vi.fn().mockRejectedValue(new Error('lost')) },
    },
    { device: 'offline', me: { sign } },
  ] as unknown as Home[];
  const run = attempt(true, undefined, nodes);
  const listener = await run.matched();
  const groups = [
    { g: encode(new Uint8Array(16).fill(1)), name: 'Other', seq: 2 },
    { g, name: 'Group', seq: 1 },
  ];
  run.request.mockResolvedValue({ re: 2, ok: true });
  listener.online({ request: run.request, send: run.push } as Ctl, { ...welcome, groups });
  const { notices } = await run.result;
  expect(await notices).toMatchObject([
    { node: 'older', error: { step: 'home.add', code: 'bad_request' } },
    { node: 'lost', error: { step: 'home.add', code: 'lost' } },
    { node: 'offline', error: { step: 'home.add', code: 'lost' } },
  ]);
  expect(run.link.push).toHaveBeenCalledWith({ t: 'groups', groups });
  expect(JSON.parse(refuse.mock.calls[0][0].env.p)).toMatchObject({ name: 'Group' });
  expect(run.close).not.toHaveBeenCalled();
});

it.each([
  { re: 1, ok: true },
  { re: 1, err: { code: 'bad_request', msg: '' } },
])('tells a node the home of a group, whether it takes it or not', async (answer) => {
  const sign = async (p: string) => ({ p, k: browser, s: 's' });
  const request = vi.fn(async (_request: { t: string; env: Envelope }) => answer);
  const node = { device: 'node', me: { sign }, ctl: { request } } as unknown as Home;
  vi.spyOn(Date, 'now').mockReturnValue(42);
  await addHome(node, { g, home }, 'Studio', url);
  vi.restoreAllMocks();
  expect(request).toHaveBeenCalledOnce();
  const [{ t, env }] = request.mock.calls[0];
  expect([t, env.k]).toEqual(['cmd', browser]);
  expect(JSON.parse(env.p)).toEqual({
    t: 'home.add',
    n: expect.stringMatching(/^[A-Za-z0-9_-]{22}$/),
    ts: 42,
    to: 'node',
    g,
    home,
    name: 'Studio',
    addrs: [url],
  });
});

it('closes the room when nobody arrives before the invitation window ends', async () => {
  vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout'] });
  try {
    const run = attempt();
    await run.options;
    await vi.advanceTimersByTimeAsync(599_999);
    expect(run.close).not.toHaveBeenCalled();
    await vi.advanceTimersByTimeAsync(1);
    await expect(run.result).rejects.toMatchObject({ step: 'room', code: 'timeout' });
  } finally {
    vi.useRealTimers();
  }
});

it('waits as long as the session says, when it says', async () => {
  vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout'] });
  try {
    const run = attempt(true, undefined, [], { secret }, 15_000);
    await run.options;
    await vi.advanceTimersByTimeAsync(14_999);
    expect(run.stop).not.toHaveBeenCalled();
    await vi.advanceTimersByTimeAsync(1);
    await expect(run.result).rejects.toMatchObject({ step: 'room', code: 'timeout' });
  } finally {
    vi.useRealTimers();
  }
});

describe('joining by code', () => {
  const host = encode(new Uint8Array(32).fill(4));
  const code = { secret };
  const self = async () => {
    const fields = { role: 'b', k: browser, kind: 'browser', name: 'Web' };
    return { t: 'pair', ...fields, mac: await mac(crypto, secret, fields) };
  };

  it('looks on every path and joins the home whose pair message answers, with the group of the welcome', async () => {
    const run = attempt(true, undefined, [], code);
    const { id, data } = await run.offer(host);
    expect(run.watchRoom.mock.calls).toEqual([
      [url, id, expect.any(Function)],
      [node, id, expect.any(Function)],
    ]);
    const [[, , onUrl], [, , onNode]] = run.watchRoom.mock.calls;
    onNode(url, { t: 'joined', room: id });
    expect(run.send).not.toHaveBeenCalled();
    onNode(node, { t: 'joined', room: id });
    expect(run.send).toHaveBeenCalledExactlyOnceWith(node, {
      t: 'room',
      room: id,
      data: await self(),
    });
    onNode(node, { t: 'room', room: id, data });
    const listener = await vi.waitFor(() => run.peer.mock.calls[0][1]());
    expect(run.peer).toHaveBeenCalledExactlyOnceWith(
      host,
      expect.any(Function),
      expect.any(Function),
    );
    onUrl(url, { t: 'room', room: id, data });
    const groups = [{ g, name: 'Shared', seq: 1 }];
    const ctl = { request: run.request, send: run.push } as Ctl;
    run.request.mockResolvedValue({ re: 2, ok: true });
    listener.online(ctl, { ...welcome, device: host, groups, join: g });
    const result = await run.result;
    expect(result.g).toBe(g);
    expect(JSON.parse(run.request.mock.calls[0][0].env.p)).toMatchObject({
      t: 'join',
      g,
      name: 'Joiner',
    });
    expect(run.remember).toHaveBeenCalledExactlyOnceWith(host, node, 'Home');
    expect(run.stop).toHaveBeenCalledTimes(2);
    expect(run.peer).toHaveBeenCalledOnce();
  });

  it('tells the nodes the home and the path on which the code answered', async () => {
    const sign = async (p: string) => ({ p, k: browser, s: 's' });
    const request = vi.fn(async (_request: { env: Envelope }) => ({ ok: true }));
    const own = { device: 'own', me: { sign }, ctl: { request } } as unknown as Home;
    const run = attempt(true, undefined, [own], code);
    const { id, data } = await run.offer(host);
    run.watchRoom.mock.calls[1][2](node, { t: 'room', room: id, data });
    const listener = await vi.waitFor(() => run.peer.mock.calls[0][1]());
    run.request.mockResolvedValue({ re: 2, ok: true });
    listener.online({ request: run.request, send: run.push } as Ctl, {
      ...welcome,
      groups: [{ g, name: 'Shared', seq: 1 }],
      join: g,
    });
    expect(await (await run.result).notices).toEqual([]);
    expect(JSON.parse(request.mock.calls[0][0].env.p)).toMatchObject({
      t: 'home.add',
      g,
      home: host,
      addrs: [node],
    });
  });

  it('ignores a pair message that does not prove the code', async () => {
    const run = attempt(true, undefined, [], code);
    const { id, data } = await run.offer(host);
    const heard = run.watchRoom.mock.calls[0][2];
    heard(url, { t: 'room', room: id, data: { ...data, k: home } });
    heard(url, { t: 'room', room: id, data: { ...(await run.offer(home)).data, role: 'b' } });
    heard(url, { t: 'room', room: id, data });
    await vi.waitFor(() => expect(run.peer).toHaveBeenCalledOnce());
    expect(run.peer.mock.calls[0][0]).toBe(host);
  });

  it('keeps the group of a link whatever the welcome names', async () => {
    const run = attempt();
    const listener = await run.matched();
    const other = encode(new Uint8Array(16).fill(1));
    run.request.mockResolvedValue({ re: 2, ok: true });
    listener.online({ request: run.request, send: run.push } as Ctl, {
      ...welcome,
      groups: [{ g, name: 'Shared', seq: 1 }],
      join: other,
    });
    expect((await run.result).g).toBe(g);
    expect(JSON.parse(run.request.mock.calls[0][0].env.p)).toMatchObject({ t: 'join', g });
  });
});
