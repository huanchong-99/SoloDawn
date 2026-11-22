import { describe, expect, it, vi } from 'vitest';
import { Group, Home } from '../../lib/groups.svelte';
import { goldens } from '../../lib/mock';
import * as p from '../../lib/proto';
import { requestWatch, stopWatch, Watch, watchers } from './watch.svelte';

const g = 'rZNvy-1jH6Z-BcPqA5U5BQ';
const member = '1PC8WineBrUQ-apCjx7tug';
const direct = { kind: 'direct' as const, rtt: 1, since: 1 };
const relay = { ...direct, kind: 'relay' as const };
const me = {
  user: 'viewer',
  sign: async (payload: string) => ({ p: payload, k: 'key', s: 'sig' }),
};

function setup() {
  const channel: p.PtyChannel = {
    binaryType: 'blob',
    readyState: 'open',
    onmessage: null,
    onclose: null,
    send: vi.fn(),
    close: vi.fn(),
  };
  const request = vi.fn().mockResolvedValue({ re: 1, ok: true });
  const open = vi.fn().mockReturnValue(channel);
  const home = new Home(me);
  home.device = 'host';
  home.ctl = { request: request as p.Ctl['request'], channel: open };
  const target = { host: home, to: 'host' };
  const write = vi.fn();
  const watch = new Watch(g, member, write);
  return { channel, request, open, home, target, write, watch };
}

describe('receive-only watch', () => {
  it('asks on ctl before opening a binary channel, passes exact bytes and never sends', async () => {
    const s = setup();
    s.watch.follow(undefined, direct);
    s.watch.follow(s.target, undefined);
    expect([s.watch.phase, s.request.mock.calls]).toEqual(['idle', []]);
    s.watch.follow(s.target, direct);
    expect([s.watch.phase, s.open.mock.calls]).toEqual(['starting', []]);
    await vi.waitFor(() => expect(s.watch.phase).toBe('active'));
    expect(s.request.mock.calls).toEqual([[{ t: 'watch.start', g, member }]]);
    expect(s.open.mock.calls).toEqual([[`watch:${g}:${member}`]]);
    expect(s.channel.binaryType).toBe('arraybuffer');
    s.watch.follow(s.target, direct);
    s.channel.onmessage!({ data: new Uint8Array([0, 255]) });
    s.channel.onmessage!({ data: new Uint8Array([27, 91]).buffer });
    s.channel.onmessage!({ data: 'not binary' });
    expect(s.write.mock.calls).toEqual([[new Uint8Array([0, 255])], [new Uint8Array([27, 91])]]);
    expect(s.channel.send).not.toHaveBeenCalled();
    s.channel.onclose!({});
    s.watch.handBack();
    expect([s.watch.phase, s.channel.onmessage, s.channel.onclose]).toEqual(['ended', null, null]);
    expect(s.channel.close).toHaveBeenCalledExactlyOnceWith();
    expect(s.request).toHaveBeenCalledTimes(1);
  });

  it('refuses relay before opening and closes on a changed path without ending the grant', async () => {
    const s = setup();
    s.watch.follow(s.target, relay);
    expect([s.watch.phase, s.request.mock.calls, s.open.mock.calls]).toEqual(['relay', [], []]);
    const live = new Watch(g, member, s.write);
    live.follow(s.target, direct);
    await vi.waitFor(() => expect(live.phase).toBe('active'));
    live.follow(s.target, relay);
    expect(live.phase).toBe('relay');
    expect(s.channel.close).toHaveBeenCalledExactlyOnceWith();
    expect(s.request.mock.calls).toEqual([[{ t: 'watch.start', g, member }]]);
  });

  it('does not open after cancellation or after a pending request changes to relay', async () => {
    const s = setup();
    let finish!: (value: unknown) => void;
    s.request.mockImplementation(
      () =>
        new Promise((resolve) => {
          finish = resolve;
        }),
    );
    s.watch.follow(s.target, direct);
    s.watch.handBack();
    finish({ re: 1, ok: true });
    await Promise.resolve();
    await Promise.resolve();
    expect([s.watch.phase, s.open.mock.calls]).toEqual(['ended', []]);
    const next = new Watch(g, member, s.write);
    next.follow(s.target, direct);
    next.follow(s.target, relay);
    finish({ re: 1, ok: true });
    await Promise.resolve();
    await Promise.resolve();
    expect([next.phase, s.open.mock.calls]).toEqual(['relay', []]);
  });

  it.each(['forbidden', 'relay'])('shows a %s refusal without opening', async (code) => {
    const s = setup();
    s.request.mockResolvedValue({ re: 1, err: { code, msg: '' } });
    s.watch.follow(s.target, direct);
    await vi.waitFor(() => expect(s.watch.phase).toBe('error'));
    expect([s.watch.errorCode, s.open.mock.calls]).toEqual([code, []]);
  });

  it('shows connection loss and a missing byte channel', async () => {
    const s = setup();
    s.request.mockRejectedValue(new Error('disconnected'));
    s.watch.follow(s.target, direct);
    await vi.waitFor(() => expect(s.watch.phase).toBe('error'));
    expect(s.watch.errorCode).toBe('lost');
    s.request.mockResolvedValue({ re: 1, ok: true });
    s.open.mockReturnValue(undefined);
    const next = new Watch(g, member, s.write);
    next.follow(s.target, direct);
    await vi.waitFor(() => expect(next.phase).toBe('error'));
    expect(next.errorCode).toBe('lost');
    s.home.ctl = undefined;
    const gone = new Watch(g, member, s.write);
    const answer = Promise.withResolvers<unknown>();
    s.home.ctl = { request: vi.fn(() => answer.promise) as p.Ctl['request'] };
    gone.follow(s.target, direct);
    s.home.ctl = undefined;
    answer.resolve({ re: 1, ok: true });
    await vi.waitFor(() => expect(gone.phase).toBe('error'));
    expect(gone.errorCode).toBe('lost');
  });
});

describe('watch grants and requests', () => {
  it('dispatches existing resize frames and host.info only from the current connection', () => {
    const home = new Home(me);
    const welcome = { ...p.decode(p.Welcome, goldens.welcome), groups: [] };
    const ctl = { request: vi.fn() as p.Ctl['request'] };
    const old = home.connect();
    old.online(ctl, welcome);
    const current = home.connect();
    current.online(ctl, welcome);
    const size = { t: 'pty.resize' as const, g, ai: member, cols: 100, rows: 30 };
    expect(p.decode(p.Frame, JSON.stringify(size))).toEqual(size);
    old.push(size);
    expect(home.terminalSizes.size).toBe(0);
    current.push(size);
    expect(home.terminalSizes.get(`${g}:${member}`)).toEqual(size);
    const info = p.decode(p.HostInfoPush, goldens['host-info-push']);
    current.push(info);
    expect(home.hostInfo).toEqual(info);
  });

  it('reads the matching AI and group, including absent and empty lists', () => {
    const info = p.decode(p.HostInfoPush, goldens['host-info-push']);
    expect(watchers(undefined, g, member)).toEqual([]);
    expect(watchers(info, 'other', member)).toEqual([]);
    expect(watchers(info, g, 'other')).toEqual([]);
    const entry = info.agents[0].groups[0];
    expect(watchers(info, entry.g, entry.member)).toEqual(entry.watchers);
    entry.watchers = ['viewer'];
    expect(watchers(info, entry.g, entry.member)).toEqual(['viewer']);
  });

  it('signs the exact watch request and owner stop command', async () => {
    const s = setup();
    const group = new Group(s.home, g);
    const ai = p.decode(p.AIMemberRow, goldens['row-member-ai']);
    await requestWatch(group, ai);
    await stopWatch(s.home, g, member, 'viewer');
    const payloads = s.request.mock.calls.map(([req]) => {
      const { n, ts, ...payload } = JSON.parse(req.env.p);
      expect(typeof n).toBe('string');
      expect(typeof ts).toBe('number');
      return payload;
    });
    expect(payloads).toEqual([
      {
        t: 'post',
        g,
        kind: 'confirm',
        thread: null,
        main: true,
        text: '',
        mentions: [],
        re: null,
        body: { what: 'watch', ai: ai.id },
      },
      { t: 'watch.end', g, to: 'host', member, user: 'viewer' },
    ]);
  });
});
