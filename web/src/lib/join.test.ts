import { describe, expect, it, vi } from 'vitest';
import type { DeviceLog } from './devlog';
import type { Home } from './groups.svelte';
import { encode, type Device } from './identity';
import { joinGroup, presentLog, readInvitation } from './join';
import { mac, parseSecret, room } from './pairing';
import { connect, type Listener, type Options, type Session } from './peer';
import type { Ctl, Devlog, Envelope, Welcome } from './proto';

vi.mock('./peer', () => ({ connect: vi.fn() }));

const g = encode(new Uint8Array(16));
const home = encode(new Uint8Array(32).fill(2));

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

const secret = parseSecret('7K2M-9QXD-4F8T-WB3N');
const invitation = { g, home, secret };
const browser = encode(new Uint8Array(32).fill(3));
const url = 'wss://rv.example/rv';
const welcome: Welcome = {
  t: 'welcome',
  re: 1,
  ok: true,
  v: 2,
  name: 'Home',
  version: 'test',
  time: 1,
  device: home,
  groups: [],
  owner: false,
  addrs: [],
};

function attempt(valid = true, signal = new AbortController().signal, nodes: Home[] = []) {
  vi.mocked(connect).mockReset();
  const options = Promise.withResolvers<Options>();
  const listen = Promise.withResolvers<Listener>();
  const close = vi.fn();
  const send = vi.fn();
  const peer = vi.fn((_: string, fn: () => Listener) => listen.resolve(fn()));
  vi.mocked(connect).mockImplementation((opts) => {
    options.resolve(opts);
    return { peer, send, close } as unknown as Session;
  });
  const remember = vi.fn(async (_name: string) => {});
  const link = { online: vi.fn(), push: vi.fn(), offline: vi.fn() };
  const push = vi.fn();
  const request = vi.fn();
  const result = joinGroup(invitation, 'Joiner', {
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
    signal,
    browserName: 'Web',
    remember,
    link: () => link,
    ownNodes: () => nodes,
    onpath: vi.fn(),
  });
  void result.catch(() => {});
  const offer = async () => {
    const opts = await options.promise;
    const id = await room(crypto, secret);
    const fields = { role: 'a', k: home, kind: 'node', name: 'Home' };
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
    request,
    push,
    remember,
    link,
    options: options.promise,
  };
}

it('does not contact rendezvous before onboarding', async () => {
  await expect(attempt(false).result).rejects.toMatchObject({
    step: 'onboarding',
    code: 'unpaired',
  });
  expect(connect).not.toHaveBeenCalled();
});

it('does not contact rendezvous after cancellation', async () => {
  await expect(attempt(true, AbortSignal.abort()).result).rejects.toMatchObject({
    step: 'room',
    code: 'cancelled',
  });
  expect(connect).not.toHaveBeenCalled();
});

it.each([
  [1, { re: 2, ok: true }, { step: 'hello', code: 'version' }],
  [
    2,
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
  await run.options;
  cancel.abort();
  await expect(run.result).rejects.toMatchObject({ step: 'room', code: 'cancelled' });
  expect(run.close).toHaveBeenCalled();
  expect(run.request).not.toHaveBeenCalled();
});

it('dials only the invited node, from a matching authenticated room message', async () => {
  const run = attempt();
  const { opts, id, data } = await run.offer();
  const foreign = { role: 'a', k: browser, kind: 'node', name: 'Foreign' };
  const otherKind = { ...data, kind: 'browser' };
  expect(opts.known(home)).toBe(true);
  expect(opts.known(browser)).toBe(false);
  opts.heard!(url, { t: 'presence', k: home, on: true });
  opts.heard!(url, { t: 'joined', room: 'another room' });
  opts.heard!(url, { t: 'room', room: 'another room', data });
  opts.heard!(url, {
    t: 'room',
    room: id,
    data: { t: 'pair', ...foreign, mac: await mac(crypto, secret, foreign) },
  });
  opts.heard!(url, {
    t: 'room',
    room: id,
    data: { ...otherKind, mac: await mac(crypto, secret, otherKind) },
  });
  expect(run.send).not.toHaveBeenCalled();
  expect(run.peer).not.toHaveBeenCalled();
  const listener = await run.matched();
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
  await vi.waitFor(() => expect(run.remember).toHaveBeenCalledWith('Home'));
  listener.offline();
  await expect(run.result).rejects.toMatchObject({ step: 'groups', code: 'lost' });
  expect(run.close).toHaveBeenCalledOnce();
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
  await vi.waitFor(() => expect(run.remember).toHaveBeenCalledWith('Home'));
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
