import { beforeEach, describe, expect, it, vi } from 'vitest';
import { Group, Home } from './groups.svelte';
import { goldens } from './mock';
import { prefs } from './prefs.svelte';
import * as p from './proto';
import {
  activeSummary,
  applyCtrl,
  binaryBytes,
  bridgePty,
  endTakeover,
  handbackFilesLabel,
  headerSubtitle,
  hostHomeOf,
  linkBadgeOf,
  phoneKeys,
  ptyLabel,
  queuedCountOf,
  sendResize,
  sessionTaskOf,
  startTakeover,
  takeoverBarState,
  takeoverErrorText,
  toTerminalChunk,
} from './terminal';

const gid = 'rZNvy-1jH6Z-BcPqA5U5BQ';
const memberId = '1PC8WineBrUQ-apCjx7tug';
const taskId = 'DrtCn6htSBwmMPrFPbHJHA';
const hostId = 'J3Uuh-y1z2AX536yAFlGHJL5sjxVpWVJavvVphvdm5Q';

const signed: string[] = [];
const me = {
  user: 'alex-user',
  sign: async (payload: string) => {
    signed.push(payload);
    return { p: payload, k: 'dev-key', s: 'dev-sig' };
  },
};

function row<T extends p.Row = p.Row>(name: string, changes: Record<string, unknown> = {}): T {
  return { ...p.decode(p.Row, goldens[name]), ...changes } as T;
}

function onlineHome(device = hostId, handler?: (req: unknown) => Promise<unknown>): Home {
  const home = new Home(me);
  const link = home.connect();
  const ctl: p.Ctl = {
    request: (async (req: unknown) =>
      handler ? handler(req) : { re: 1, ok: true }) as p.Ctl['request'],
  };
  const welcome = p.decode(p.Welcome, goldens.welcome);
  link.online(ctl, { ...welcome, device, groups: [] });
  return home;
}

beforeEach(() => {
  prefs.lang = 'en';
  signed.length = 0;
});

describe('ptyLabel, phoneKeys, hostHomeOf, sessionTaskOf and queuedCountOf', () => {
  it('formats ptyLabel and lists the eight phone toolbar keys in order', () => {
    expect(ptyLabel(gid, memberId)).toBe(`pty:${gid}:${memberId}`);
    expect(phoneKeys).toEqual([
      { label: 'Esc', seq: '\x1b' },
      { label: 'Tab', seq: '\t' },
      { label: 'Ctrl', ctrl: true },
      { label: '↑', seq: '\x1b[A', ariaKey: 'terminal.key.up' },
      { label: '↓', seq: '\x1b[B', ariaKey: 'terminal.key.down' },
      { label: '/', seq: '/' },
      { label: '@', seq: '@' },
      { label: '⏎', seq: '\r', ariaKey: 'terminal.key.enter' },
    ]);
  });

  it('finds hostHomeOf by device, hostInfo.device, group.info.home, or returns undefined', () => {
    const byDevice = onlineHome('dev-a');
    const byPush = new Home(me);
    byPush.hostInfo = { ...p.decode(p.HostInfoPush, goldens['host-info-push']), device: 'dev-b' };

    const groupHome = onlineHome('home-dev');
    const group = new Group(groupHome, gid);
    group.apply([row('row-group', { id: gid, home: 'group-host-id' })]);

    expect(hostHomeOf([byDevice, byPush], group, 'dev-a')).toBe(byDevice);
    expect(hostHomeOf([byDevice, byPush], group, 'dev-b')).toBe(byPush);
    expect(hostHomeOf([byDevice], group, 'group-host-id')).toBe(groupHome);
    expect(hostHomeOf([byDevice], group, 'missing-id')).toBeUndefined();
    expect(hostHomeOf([], undefined, 'group-host-id')).toBeUndefined();

    const groupWithoutInfo = new Group(groupHome, gid);
    expect(hostHomeOf([], groupWithoutInfo, 'group-host-id')).toBeUndefined();
  });

  it('resolves sessionTaskOf from route taskParam, ai.task, or returns undefined', () => {
    const home = onlineHome();
    const group = new Group(home, gid);
    const t1 = row<p.TaskRow>('row-task', { id: 't-1', g: gid, num: 1 });
    const t2 = row<p.TaskRow>('row-task', { id: 't-2', g: gid, num: 2 });
    const ai = row<p.AIMemberRow>('row-member-ai', { id: memberId, g: gid, task: 't-1' });
    const idleAi = row<p.AIMemberRow>('row-member-ai', { id: memberId, g: gid, task: undefined });
    group.apply([t1, t2, ai]);

    expect(sessionTaskOf(undefined, ai, 't-2')).toBeUndefined();
    expect(sessionTaskOf(group, ai, 't-2')).toBe(t2);
    expect(sessionTaskOf(group, ai, undefined)).toBe(t1);
    expect(sessionTaskOf(group, idleAi, undefined)).toBeUndefined();
    expect(sessionTaskOf(group, undefined, undefined)).toBeUndefined();
  });

  it('reads queuedCountOf across agents and groups', () => {
    expect(queuedCountOf(undefined, gid, memberId)).toBe(0);
    const base = p.decode(p.HostInfoPush, goldens['host-info-push']);
    const info: p.HostInfoPush = {
      ...base,
      agents: [
        {
          agent: 'claude',
          cli_version: '2.0.0',
          account: 'personal',
          login_ok: true,
          caps: {},
          groups: [
            { g: gid, member: 'other-member', level: 3, state: 'working', queued: 9 },
            { g: 'other-group', member: memberId, level: 3, state: 'working', queued: 7 },
          ],
        },
        {
          agent: 'codex',
          cli_version: '0.1.0',
          account: 'team',
          login_ok: true,
          caps: {},
          groups: [{ g: gid, member: memberId, level: 3, state: 'takeover', queued: 4 }],
        },
      ],
    };
    expect(queuedCountOf(info, gid, memberId)).toBe(4);
    expect(queuedCountOf(info, gid, 'nobody')).toBe(0);
  });
});

describe('headerSubtitle, linkBadgeOf, activeSummary, takeoverErrorText and takeoverBarState', () => {
  it('builds headerSubtitle with and without ai, task and branch', () => {
    const ai = row<p.AIMemberRow>('row-member-ai', { host_name: 'MacBook' });
    const taskWithBranch = row<p.TaskRow>('row-task', { num: 12, branch: 'task/12-conn' });
    const taskNoBranch = { ...taskWithBranch, branch: undefined };

    expect(headerSubtitle(undefined, taskWithBranch)).toBe('');
    expect(headerSubtitle(undefined, taskWithBranch, 'Website')).toBe('Website');
    expect(headerSubtitle(ai, undefined, 'Website')).toBe('MacBook');
    expect(headerSubtitle(ai, taskNoBranch)).toBe('MacBook · T-12');
    expect(headerSubtitle(ai, taskWithBranch)).toBe('MacBook · T-12 · task/12-conn');
  });

  it('formats linkBadgeOf for offline, linking, relay and direct with or without rtt', () => {
    expect(linkBadgeOf(false, { kind: 'direct', rtt: 4, since: 1 })).toEqual({
      st: 'down',
      text: 'Offline',
    });
    expect(linkBadgeOf(true, undefined)).toEqual({ st: 'linking', text: 'Connecting' });
    expect(linkBadgeOf(true, { kind: 'relay', rtt: 120, since: 1 })).toEqual({
      st: 'limited',
      text: 'Relayed · limited',
    });
    expect(linkBadgeOf(true, { kind: 'direct', rtt: undefined, since: 1 })).toEqual({
      st: 'ok',
      text: 'Direct',
    });
    expect(linkBadgeOf(true, { kind: 'direct', rtt: 0, since: 1 })).toEqual({
      st: 'ok',
      text: 'Direct · 0 ms',
    });
    expect(linkBadgeOf(true, { kind: 'direct', rtt: 4, since: 1 })).toEqual({
      st: 'ok',
      text: 'Direct · 4 ms',
    });
  });

  it('formats activeSummary across exact minute and queued boundaries', () => {
    const start = 1_000_000;
    expect(activeSummary(start, start - 500, 0)).toBe('You took over just now.');
    expect(activeSummary(start, start + 59_999, 0)).toBe('You took over just now.');
    expect(activeSummary(start, start + 60_000, 0)).toBe('You took over 1 min ago.');
    expect(activeSummary(start, start + 60_000, -1)).toBe('You took over 1 min ago.');
    expect(activeSummary(start, start + 360_000, 1)).toBe(
      'You took over 6 min ago. 1 message is queued until you hand back.',
    );
    expect(activeSummary(start, start + 360_000, 2)).toBe(
      'You took over 6 min ago. 2 messages are queued until you hand back.',
    );
  });

  it('maps known takeoverErrorText codes and derives takeoverBarState across every phase', () => {
    expect(takeoverErrorText('busy')).toBe(
      'This AI is busy with another terminal or a merge right now.',
    );
    expect(takeoverErrorText('cli')).toBe(
      'The AI CLI could not open the interactive session on its computer.',
    );
    expect(takeoverErrorText('conflict')).toBe(
      'This session cannot be taken over in its current state.',
    );
    expect(takeoverErrorText('home_unreachable')).toBe(
      "The AI's computer cannot reach this group's home right now.",
    );
    expect(takeoverErrorText('not_found')).toBe('This AI or task was not found on its computer.');
    expect(takeoverErrorText('lost')).toBe("Cannot reach this AI's computer.");
    expect(takeoverErrorText('forbidden')).toBeUndefined();

    const ai = row<p.AIMemberRow>('row-member-ai', { state: 'working' });
    const offlineAi = row<p.AIMemberRow>('row-member-ai', { state: 'offline' });
    const direct = { kind: 'direct' as const, rtt: 5, since: 1 };
    const relay = { kind: 'relay' as const, rtt: 90, since: 1 };
    const base = {
      phase: 'idle' as const,
      ai,
      owned: true,
      online: true,
      path: direct,
      errorCode: '',
      startedAt: 1000,
      nowMs: 121_000,
      queued: 2,
    };

    expect(takeoverBarState({ ...base, ai: undefined })).toEqual({
      title: 'AI not found',
      detail: 'This AI member has not been read yet or has left the group.',
      handback: false,
    });
    expect(takeoverBarState({ ...base, owned: false })).toEqual({
      title: 'Owner only',
      detail: "Only this AI's owner can open its terminal.",
      handback: false,
    });
    expect(takeoverBarState({ ...base, online: false })).toEqual({
      title: 'Computer offline',
      detail: "Cannot reach this AI's computer right now.",
      handback: false,
    });
    expect(takeoverBarState({ ...base, ai: offlineAi })).toEqual({
      title: 'Computer offline',
      detail: "Cannot reach this AI's computer right now.",
      handback: false,
    });
    expect(takeoverBarState({ ...base, path: undefined })).toEqual({
      title: 'Connecting…',
      detail: 'Waiting for a direct connection to this computer.',
      handback: false,
    });
    expect(takeoverBarState({ ...base, path: relay })).toEqual({
      title: 'Direct connection required',
      detail: 'Terminal video uses a direct link only. Establish a direct connection first.',
      handback: false,
    });
    expect(takeoverBarState(base)).toEqual({
      title: 'Opening terminal…',
      detail: 'Switching the AI session into interactive terminal mode.',
      handback: true,
    });
    expect(takeoverBarState({ ...base, phase: 'starting' })).toEqual({
      title: 'Opening terminal…',
      detail: 'Switching the AI session into interactive terminal mode.',
      handback: true,
    });
    expect(takeoverBarState({ ...base, phase: 'relay' })).toEqual({
      title: 'Direct connection required',
      detail: 'Terminal video uses a direct link only. Establish a direct connection first.',
      handback: false,
    });
    expect(takeoverBarState({ ...base, phase: 'relayed' })).toEqual({
      title: 'Handed back · switched to relay',
      detail:
        'The connection switched to a relay, so the terminal closed and handed back to the AI.',
      handback: false,
    });
    expect(takeoverBarState({ ...base, phase: 'ended' })).toEqual({
      title: 'Handed back',
      detail: 'The interactive session ended and control returned to the AI.',
      handback: false,
    });
    expect(takeoverBarState({ ...base, phase: 'active' })).toEqual({
      title: 'Owner is driving',
      detail: 'You took over 2 min ago. 2 messages are queued until you hand back.',
      handback: true,
    });
    expect(takeoverBarState({ ...base, phase: 'error', errorCode: 'busy' })).toEqual({
      title: 'Cannot open terminal',
      detail: 'This AI is busy with another terminal or a merge right now.',
      handback: false,
    });
    expect(takeoverBarState({ ...base, phase: 'error', errorCode: 'internal' })).toEqual({
      title: 'Cannot open terminal',
      errorCode: 'internal',
      handback: false,
    });
  });
});

describe('applyCtrl, binaryBytes, toTerminalChunk and handbackFilesLabel', () => {
  it('applies sticky Ctrl across exact ASCII boundaries', () => {
    expect(applyCtrl('c', false)).toEqual({ out: 'c', ctrl: false });
    expect(applyCtrl('', true)).toEqual({ out: '', ctrl: true });
    expect(applyCtrl('?', true)).toEqual({ out: '?', ctrl: false });
    expect(applyCtrl('@', true)).toEqual({ out: '\x00', ctrl: false });
    expect(applyCtrl('c', true)).toEqual({ out: '\x03', ctrl: false });
    expect(applyCtrl('C', true)).toEqual({ out: '\x03', ctrl: false });
    expect(applyCtrl('cd', true)).toEqual({ out: '\x03d', ctrl: false });
    expect(applyCtrl('~', true)).toEqual({ out: '\x1e', ctrl: false });
    expect(applyCtrl('\x7f', true)).toEqual({ out: '\x7f', ctrl: false });
  });

  it('converts binary strings, normalizes chunks and formats handback file counts', () => {
    expect([...binaryBytes('\x00\x7f\xff\u0102')]).toEqual([0, 0x7f, 0xff, 0x02]);

    const bytes = new Uint8Array([65, 66]);
    expect(toTerminalChunk(bytes)).toBe(bytes);
    expect(toTerminalChunk(bytes.buffer)).toEqual(bytes);
    expect(toTerminalChunk('hello')).toBe('hello');
    expect(toTerminalChunk(null)).toBeUndefined();
    expect(toTerminalChunk(42)).toBeUndefined();

    expect(handbackFilesLabel(-1)).toBe('No file changes');
    expect(handbackFilesLabel(0)).toBe('No file changes');
    expect(handbackFilesLabel(1)).toBe('1 file');
    expect(handbackFilesLabel(3)).toBe('3 files');
  });
});

describe('startTakeover, endTakeover, sendResize and bridgePty', () => {
  it('signs takeover.start and takeover.end and handles offline, refusal and rejection', async () => {
    vi.spyOn(Date, 'now').mockReturnValue(1763600000000);
    const offline = new Home(me);
    expect(await startTakeover(undefined, hostId, gid, memberId)).toBe('lost');
    expect(await startTakeover(offline, hostId, gid, memberId)).toBe('lost');
    await endTakeover(undefined, hostId, gid, memberId);
    await endTakeover(offline, hostId, gid, memberId);
    expect(signed).toEqual([]);

    const sent: unknown[] = [];
    const host = onlineHome(hostId, async (req) => {
      sent.push(req);
      return { re: 1, ok: true };
    });

    expect(await startTakeover(host, hostId, gid, memberId)).toBe('');
    const first = JSON.parse(signed[0]) as Record<string, unknown>;
    expect(first).toEqual({
      t: 'takeover.start',
      n: expect.stringMatching(/^[A-Za-z0-9_-]{21}[AQgw]$/),
      ts: 1763600000000,
      to: hostId,
      g: gid,
      member: memberId,
    });
    expect('task' in first).toBe(false);

    expect(await startTakeover(host, hostId, gid, memberId, taskId)).toBe('');
    expect(p.decode(p.TakeoverStart, signed[1])).toMatchObject({
      t: 'takeover.start',
      to: hostId,
      g: gid,
      member: memberId,
      task: taskId,
    });

    await endTakeover(host, hostId, gid, memberId);
    expect(p.decode(p.TakeoverEnd, signed[2])).toMatchObject({
      t: 'takeover.end',
      ts: 1763600000000,
      to: hostId,
      g: gid,
      member: memberId,
    });
    expect(sent).toHaveLength(3);

    const refusing = onlineHome(hostId, async () => ({
      re: 2,
      err: { code: 'relay', msg: 'relayed' },
    }));
    expect(await startTakeover(refusing, hostId, gid, memberId)).toBe('relay');

    const rejecting = onlineHome(hostId, async () => {
      throw new Error('dropped');
    });
    expect(await startTakeover(rejecting, hostId, gid, memberId)).toBe('lost');
    await expect(endTakeover(rejecting, hostId, gid, memberId)).resolves.toBeUndefined();
    vi.restoreAllMocks();
  });

  it('pushes pty.resize only when ctl.send exists and cols and rows are at least 1', () => {
    const pushed: unknown[] = [];
    const withoutSend = { request: vi.fn() } as unknown as p.Ctl;
    const withSend = {
      request: vi.fn(),
      send: (frame: unknown) => pushed.push(frame),
    } as unknown as p.Ctl;

    sendResize(undefined, gid, memberId, 80, 24);
    sendResize(withoutSend, gid, memberId, 80, 24);
    sendResize(withSend, gid, memberId, 0, 24);
    sendResize(withSend, gid, memberId, 80, 0);
    expect(pushed).toEqual([]);

    sendResize(withSend, gid, memberId, 1, 1);
    sendResize(withSend, gid, memberId, 120.8, 32.4);
    expect(pushed).toEqual([
      { t: 'pty.resize', g: gid, ai: memberId, cols: 1, rows: 1 },
      { t: 'pty.resize', g: gid, ai: memberId, cols: 120, rows: 32 },
    ]);
  });

  it('bridges PtyChannel open, incoming chunks, outgoing bytes/text, remote close and local close', () => {
    function makeFakePty(initialState = 'connecting') {
      let state = initialState;
      const sent: Uint8Array[] = [];
      let closes = 0;
      const hooks = {
        onopen: null as (() => void) | null,
        onmessage: null as ((event: { data: unknown }) => void) | null,
        onclose: null as (() => void) | null,
      };
      const channel: p.PtyChannel = {
        binaryType: 'blob',
        get readyState() {
          return state;
        },
        get onopen() {
          return hooks.onopen;
        },
        set onopen(fn: (() => void) | null) {
          hooks.onopen = fn;
        },
        get onmessage() {
          return hooks.onmessage;
        },
        set onmessage(fn: ((event: { data: unknown }) => void) | null) {
          hooks.onmessage = fn;
        },
        get onclose() {
          return hooks.onclose;
        },
        set onclose(fn: (() => void) | null) {
          hooks.onclose = fn;
        },
        send(data) {
          sent.push(data);
        },
        close() {
          closes += 1;
          state = 'closed';
          hooks.onclose?.();
        },
      };
      return {
        channel,
        hooks,
        sent,
        get closes() {
          return closes;
        },
        setState(next: string) {
          state = next;
        },
      };
    }

    // 1. Channel starts connecting, opens, streams both ways, then closes from remote.
    const first = makeFakePty('connecting');
    const opened = vi.fn();
    const chunks: (Uint8Array | string)[] = [];
    const remoteClosed = vi.fn();
    const bridge = bridgePty(first.channel, {
      onOpen: opened,
      onChunk: (c) => chunks.push(c),
      onClose: remoteClosed,
    });
    expect(first.channel.binaryType).toBe('arraybuffer');
    expect(opened).not.toHaveBeenCalled();

    // Outgoing sends are dropped until readyState is 'open'.
    bridge.send(new Uint8Array([1]));
    bridge.sendText('ignored');
    expect(first.sent).toEqual([]);

    first.setState('open');
    first.hooks.onopen?.();
    expect(opened).toHaveBeenCalledTimes(1);

    bridge.sendText('');
    bridge.sendText('ls\r');
    bridge.send(new Uint8Array([3]));
    expect(first.sent.map((b) => [...b])).toEqual([[108, 115, 13], [3]]);

    first.hooks.onmessage?.({ data: new Uint8Array([65]).buffer });
    first.hooks.onmessage?.({ data: 'prompt> ' });
    first.hooks.onmessage?.({ data: null });
    expect(chunks).toEqual([new Uint8Array([65]), 'prompt> ']);

    first.setState('closed');
    first.hooks.onclose?.();
    first.hooks.onclose?.();
    expect(remoteClosed).toHaveBeenCalledTimes(1);
    first.setState('open');
    bridge.send(new Uint8Array([9]));
    expect(first.sent).toHaveLength(2);

    // 2. Channel already open on attach, no onOpen callback, and closed locally.
    const second = makeFakePty('open');
    const secondClosed = vi.fn();
    const bridge2 = bridgePty(second.channel, {
      onChunk: () => {},
      onClose: secondClosed,
    });
    second.hooks.onopen?.();
    bridge2.close();
    bridge2.close();
    expect(second.closes).toBe(1);
    expect(second.hooks.onopen).toBeNull();
    expect(second.hooks.onmessage).toBeNull();
    expect(second.hooks.onclose).toBeNull();
    expect(secondClosed).not.toHaveBeenCalled();
  });
});
