import { SvelteMap } from 'svelte/reactivity';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { watch } from '../../../features/watch.svelte';
import { Group, Home } from '../../lib/groups.svelte';
import { goldens } from '../../lib/mock';
import type { Path } from '../../lib/net.svelte';
import { prefs } from '../../lib/prefs.svelte';
import * as p from '../../lib/proto';
import { requestWatch, stopWatch, Watch, watchBar, watchers } from './watch.svelte';

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
    expect(s.write.mock.calls).toEqual([
      [new Uint8Array([0, 255])],
      [new Uint8Array([27, 91])],
      ['not binary'],
    ]);
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

describe('after the consent', () => {
  beforeEach(() => void vi.useFakeTimers());
  afterEach(() => void vi.useRealTimers());

  it('tries a refused start again after the back-off of a connection, from its start', async () => {
    const s = setup();
    const stop = watch(() => s.watch.follow(s.target, direct));
    const starts = async (ms: number) => {
      await vi.advanceTimersByTimeAsync(ms);
      return s.request.mock.calls.length;
    };
    const refused = { re: 1, err: { code: 'forbidden', msg: '' } };
    s.request.mockResolvedValue(refused);
    // Without consent a refusal stands.
    expect(await starts(600_000)).toBe(1);
    s.watch.consented();
    expect(await starts(0)).toBe(2);
    expect([await starts(999), await starts(1)]).toEqual([2, 3]);
    expect([await starts(1999), await starts(1)]).toEqual([3, 4]);
    expect([await starts(3999), await starts(1)]).toEqual([4, 5]);
    s.watch.consented();
    expect(await starts(0)).toBe(6);
    expect([await starts(999), await starts(1)]).toEqual([6, 7]);
    s.request.mockResolvedValue({ re: 1, ok: true });
    expect([await starts(2000), s.watch.phase, s.watch.errorCode]).toEqual([8, 'active', '']);
    s.watch.consented();
    expect([await starts(600_000), s.watch.phase]).toEqual([8, 'active']);
    stop();
  });

  it('retries a refusal that arrives after the consent, and nothing once it is closed', async () => {
    const s = setup();
    const stop = watch(() => s.watch.follow(s.target, direct));
    const answer = Promise.withResolvers<unknown>();
    s.request.mockReturnValueOnce(answer.promise);
    await vi.advanceTimersByTimeAsync(0);
    s.watch.consented();
    answer.resolve({ re: 1, err: { code: 'forbidden', msg: '' } });
    await vi.advanceTimersByTimeAsync(0);
    expect(s.watch.phase).toBe('error');
    s.watch.handBack();
    await vi.advanceTimersByTimeAsync(600_000);
    expect([s.watch.phase, s.request.mock.calls.length]).toEqual(['ended', 1]);
    stop();
  });

  it('tries again in one series however often the path changes', async () => {
    const s = setup();
    const paths = new SvelteMap<string, Path>([['', direct]]);
    const stop = watch(() => s.watch.follow(s.target, paths.get('')));
    s.request.mockResolvedValue({ re: 1, err: { code: 'forbidden', msg: '' } });
    s.watch.consented();
    await vi.advanceTimersByTimeAsync(0);
    for (let i = 0; i < 5; i += 1) {
      paths.set('', relay);
      await vi.advanceTimersByTimeAsync(0);
      expect([s.watch.phase, s.watch.errorCode]).toEqual(['relay', '']);
      paths.set('', direct);
      await vi.advanceTimersByTimeAsync(0);
    }
    expect(s.request).toHaveBeenCalledTimes(6);
    await vi.advanceTimersByTimeAsync(1000);
    expect(s.request).toHaveBeenCalledTimes(7);
    await vi.advanceTimersByTimeAsync(1999);
    expect(s.request).toHaveBeenCalledTimes(7);
    stop();
  });

  it('does not start an ended watch again', async () => {
    const s = setup();
    s.watch.follow(s.target, direct);
    await vi.advanceTimersByTimeAsync(0);
    s.channel.onclose!({});
    s.watch.follow(s.target, relay);
    s.watch.follow(s.target, direct);
    await vi.advanceTimersByTimeAsync(0);
    expect([s.watch.phase, s.request.mock.calls.length]).toEqual(['ended', 1]);
  });

  it('counts the refusals since the consent, and keeps the last one while it tries again', async () => {
    const s = setup();
    const stop = watch(() => s.watch.follow(s.target, direct));
    const said = () => [s.watch.phase, s.watch.errorCode, s.watch.refusals];
    s.request.mockResolvedValue({ re: 1, err: { code: 'forbidden', msg: '' } });
    await vi.advanceTimersByTimeAsync(0);
    expect(said()).toEqual(['error', 'forbidden', 1]);
    s.watch.consented();
    expect(said()).toEqual(['idle', '', 0]);
    await vi.advanceTimersByTimeAsync(0);
    expect(said()).toEqual(['error', 'forbidden', 1]);
    const answer = Promise.withResolvers<unknown>();
    s.request.mockReturnValueOnce(answer.promise);
    await vi.advanceTimersByTimeAsync(1000);
    expect(said()).toEqual(['starting', 'forbidden', 1]);
    answer.resolve({ re: 1, ok: true });
    await vi.advanceTimersByTimeAsync(0);
    expect(said()).toEqual(['active', '', 1]);
    stop();
  });

  it('starts once a relayed path is direct', async () => {
    const s = setup();
    s.watch.follow(s.target, relay);
    s.watch.follow(s.target, relay);
    expect(s.watch.phase).toBe('relay');
    s.watch.follow(s.target, direct);
    await vi.advanceTimersByTimeAsync(0);
    expect(s.watch.phase).toBe('active');
  });
});

describe('what the bar says to a watcher', () => {
  const at = new Date(2026, 10, 26, 10, 32).getTime();
  const ai = {
    ...p.decode(p.AIMemberRow, goldens['row-member-ai']),
    id: member,
    state: 'takeover',
    since: at - 1,
  };
  const person = p.decode(p.HumanMemberRow, goldens['row-member-human']);
  const card = (state: object, changes: object = {}) =>
    ({
      ...p.decode(p.Row, goldens['row-msg-confirm-pending']),
      id: 'ask',
      by: 'me',
      body: { what: 'watch', ai: member, to: [person.user] },
      state,
      ...changes,
    }) as p.Row;
  const none = "The owner's consent is needed";

  function bar(watch: Partial<Watch>, rows: p.Row[], changes: Partial<p.AIMemberRow> = {}) {
    const group = new Group(new Home(me), g);
    group.apply([{ ...person, id: 'me', user: me.user }, person, ...rows]);
    const said = watchBar({ errorCode: '', refusals: 0, ...watch } as Watch, group, {
      ...ai,
      ...changes,
    });
    return [said.title, said.detail, said.ask ?? false];
  }

  const confirmed = { status: 'confirmed', by: person.id, at };
  const pending = card({ status: 'pending' });

  it.each([
    ['no request', [], none, true],
    ['a request of someone else', [card({ status: 'pending' }, { by: 'sam' })], none, true],
    [
      'a request for another AI',
      [card({}, { body: { what: 'watch', ai: 'x', to: [] } })],
      none,
      true,
    ],
    [
      'another kind of request',
      [card({}, { body: { what: 'ask', ai: member, to: [] } })],
      none,
      true,
    ],
    ['a consent to an earlier takeover', [card({ ...confirmed, at: at - 2 })], none, true],
    ['a pending request', [pending], 'Waiting for owner', false],
    ['a consent', [card(confirmed)], 'Confirmed by owner · 10:32', false],
    ['a refusal', [card({ ...confirmed, status: 'denied' })], 'Denied by owner · 10:32', true],
    ['an expired request', [card({ status: 'expired', at })], 'Expired · 10:32', true],
    [
      'the last request',
      [pending, card(confirmed, { id: 'b', pos: 900 })],
      'Confirmed by owner · 10:32',
      false,
    ],
  ])('reads %s', (_, rows, title, ask) => {
    prefs.lang = 'en';
    expect(bar({ phase: 'idle' }, rows)).toEqual([title, expect.any(String), ask]);
  });

  it('keeps what it says while a start is asked for, and offers to ask after a refusal', () => {
    prefs.lang = 'en';
    const said = 'Confirmed by owner · 10:32';
    const soon =
      'The terminal opens here by itself once the computer has the consent. If it does not, ask again.';
    const refused = { phase: 'error', errorCode: 'forbidden', refusals: 2 } as const;
    expect(bar({ phase: 'starting' }, [card(confirmed)])).toEqual([said, soon, false]);
    // The grant may be on its way.
    expect(bar({ ...refused, refusals: 1 }, [card(confirmed)])).toEqual([said, soon, false]);
    for (const phase of ['error', 'idle', 'starting'] as const) {
      expect(bar({ ...refused, phase }, [card(confirmed)])).toEqual([said, soon, true]);
      expect(bar({ phase, errorCode: 'lost', refusals: 2 }, [card(confirmed)])[0]).toBe(
        'Could not open the watch channel',
      );
    }
    expect(bar(refused, [pending])[2]).toBe(false);
    expect(bar(refused, [])[0]).toBe(none);
    expect(bar({ phase: 'starting' }, [])[0]).toBe('Opening the watch channel');
    expect(bar({ phase: 'active' }, [card(confirmed)])[0]).toBe('Watching · receive only');
  });

  it('says that the takeover has ended', () => {
    prefs.lang = 'en';
    const over = [
      'The takeover has ended',
      'There is nothing to watch until the owner opens the terminal again.',
      false,
    ];
    expect(bar({ phase: 'idle' }, [pending], { state: 'waiting' })).toEqual(over);
    expect(bar({ phase: 'idle' }, [], { state: 'waiting' })).toEqual(over);
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
