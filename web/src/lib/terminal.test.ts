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
  sendResize,
  sessionTaskOf,
  startTakeover,
  Takeover,
  takeoverBarState,
  takeoverErrorText,
  targetOf,
  terminalTask,
  toTerminalChunk,
} from './terminal.svelte';

const gid = 'rZNvy-1jH6Z-BcPqA5U5BQ';
const memberId = '1PC8WineBrUQ-apCjx7tug';
const taskId = 'DrtCn6htSBwmMPrFPbHJHA';
const hostId = 'J3Uuh-y1z2AX536yAFlGHJL5sjxVpWVJavvVphvdm5Q';
const direct = { kind: 'direct' as const, rtt: 5, since: 1 };
const relay = { kind: 'relay' as const, rtt: 90, since: 1 };

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

function online(home: Home, ctl: p.Ctl, device = hostId) {
  const welcome = p.decode(p.Welcome, goldens.welcome);
  home.connect().online(ctl, { ...welcome, device, groups: [] });
  return home;
}

function onlineHome(device = hostId, handler?: (req: unknown) => Promise<unknown>): Home {
  const request = (async (req: unknown) =>
    handler ? handler(req) : { re: 1, ok: true }) as p.Ctl['request'];
  return online(new Home(me), { request }, device);
}

/** A data channel: what was sent, how often it was closed, and the far end's acts. */
function fakeChannel(state = 'open') {
  const sent: number[][] = [];
  let closes = 0;
  const channel: p.PtyChannel = {
    binaryType: 'blob',
    get readyState() {
      return state;
    },
    onmessage: null,
    onclose: null,
    send: (data) => sent.push([...data]),
    close() {
      closes += 1;
      state = 'closed';
      channel.onclose?.({});
    },
  };
  return {
    channel,
    sent,
    get closes() {
      return closes;
    },
    open: () => (state = 'open'),
    deliver: (data: unknown) => channel.onmessage?.({ data }),
    remoteClose() {
      state = 'closed';
      channel.onclose?.({});
    },
  };
}

/** A computer that answers `takeover.start` when the test says so, and records the rest. */
function computer() {
  const commands: Record<string, unknown>[] = [];
  const resizes: unknown[] = [];
  const channels: { label: string; fake: ReturnType<typeof fakeChannel> }[] = [];
  const answers: ((answer: unknown) => void)[] = [];
  const ctl: p.Ctl = {
    request: (async (req: { env: p.Envelope }): Promise<unknown> => {
      const cmd = JSON.parse(req.env.p) as Record<string, unknown>;
      commands.push(cmd);
      if (cmd.t !== 'takeover.start') return { re: 1, ok: true };
      return new Promise((resolve) => answers.push(resolve));
    }) as unknown as p.Ctl['request'],
    send: (frame) => resizes.push(frame),
    channel(label) {
      const fake = fakeChannel();
      channels.push({ label, fake });
      return fake.channel;
    },
  };
  const home = online(new Home(me), ctl);
  return {
    home,
    target: { host: home, to: hostId },
    commands,
    sent: (t: string) => commands.filter((cmd) => cmd.t === t),
    resizes,
    channels,
    noChannel: () => delete ctl.channel,
    answer: async (answer: object = { re: 1, ok: true }) => {
      await vi.waitFor(() => expect(answers.length).toBeGreaterThan(0));
      answers.shift()!(answer);
      await new Promise((later) => setTimeout(later));
    },
  };
}

beforeEach(() => {
  prefs.lang = 'en';
  signed.length = 0;
});

describe('the parts of a terminal page', () => {
  it('names the channel and lists the eight phone keys in order', () => {
    expect(ptyLabel(gid, memberId)).toBe(`pty:${gid}:${memberId}`);
    expect(phoneKeys).toEqual([
      { label: 'Esc', seq: '\x1b' },
      { label: 'Tab', seq: '\t' },
      { label: 'Ctrl', seq: '', ctrl: true },
      { label: '↑', seq: '\x1b[A', ariaKey: 'terminal.key.up' },
      { label: '↓', seq: '\x1b[B', ariaKey: 'terminal.key.down' },
      { label: '/', seq: '/' },
      { label: '@', seq: '@' },
      { label: '⏎', seq: '\r', ariaKey: 'terminal.key.enter' },
    ]);
  });

  it('finds the computer by its device id, and a target once the AI is read and online', () => {
    const a = onlineHome('dev-a');
    const b = onlineHome('dev-b');
    expect(hostHomeOf([a, b], 'dev-b')).toBe(b);
    expect(hostHomeOf([a, b], 'dev-c')).toBeUndefined();

    const ai = row<p.AIMemberRow>('row-member-ai', { host: 'dev-a', state: 'working' });
    expect(targetOf(a, ai)).toEqual({ host: a, to: 'dev-a' });
    expect(targetOf(undefined, ai)).toBeUndefined();
    expect(targetOf(new Home(me), ai)).toBeUndefined();
    expect(targetOf(a, undefined)).toBeUndefined();
    expect(targetOf(a, { ...ai, state: 'offline' })).toBeUndefined();
  });

  it('takes the task the address names, else the AI’s', () => {
    const group = new Group(onlineHome(), gid);
    const t1 = row<p.TaskRow>('row-task', { id: 't-1', g: gid, num: 1 });
    const t2 = row<p.TaskRow>('row-task', { id: 't-2', g: gid, num: 2 });
    const ai = row<p.AIMemberRow>('row-member-ai', { id: memberId, g: gid, task: 't-1' });
    group.apply([t1, t2, ai]);

    expect(sessionTaskOf(undefined, ai, 't-2')).toBeUndefined();
    expect(sessionTaskOf(group, ai, 't-2')).toBe(t2);
    expect(sessionTaskOf(group, ai, undefined)).toBe(t1);
    expect(sessionTaskOf(group, { ...ai, task: undefined }, undefined)).toBeUndefined();
    expect(sessionTaskOf(group, undefined, undefined)).toBeUndefined();
  });

  it('opens on the task the AI is on, while a terminal can open on it', () => {
    const group = new Group(new Home(me), gid);
    const ai = row<p.AIMemberRow>('row-member-ai', { task: taskId });
    expect(terminalTask(group, ai)).toBeUndefined();
    const states = ['active', 'paused', 'failed', 'queued', 'merged', 'cancelled'];
    for (const [i, state] of states.entries()) {
      group.apply([row('row-task', { id: taskId, seq: 900 + i, state })]);
      expect(terminalTask(group, ai)).toBe(i < 3 ? taskId : undefined);
    }
    group.apply([row('row-task', { id: taskId, seq: 950, state: 'active' })]);
    expect(terminalTask(group, { ...ai, task: undefined })).toBeUndefined();
  });

  it('writes the header line and the link line', () => {
    const ai = row<p.AIMemberRow>('row-member-ai', { host_name: 'MacBook' });
    const task = row<p.TaskRow>('row-task', { num: 12 });

    expect(headerSubtitle(undefined, task)).toBe('');
    expect(headerSubtitle(undefined, task, 'Website')).toBe('Website');
    expect(headerSubtitle(ai, undefined, 'Website')).toBe('MacBook');
    expect(headerSubtitle(ai, task)).toBe('MacBook · T-12');

    expect(linkBadgeOf(false, direct)).toEqual({ st: 'down', text: 'Offline' });
    expect(linkBadgeOf(true, undefined)).toEqual({ st: 'linking', text: 'Connecting' });
    expect(linkBadgeOf(true, relay)).toEqual({ st: 'limited', text: 'Relayed · limited' });
    expect(linkBadgeOf(true, { ...direct, rtt: undefined })).toEqual({ st: 'ok', text: 'Direct' });
    expect(linkBadgeOf(true, { ...direct, rtt: 0 })).toEqual({ st: 'ok', text: 'Direct · 0 ms' });
  });

  it('says since when the owner drives, and that messages wait, in both languages', () => {
    const start = 1_000_000;
    const now = 'You took over just now. Messages to this AI are queued until you hand back.';
    expect(activeSummary(start, start - 500)).toBe(now);
    expect(activeSummary(start, start + 59_999)).toBe(now);
    expect(activeSummary(start, start + 60_000)).toBe(
      'You took over 1 min ago. Messages to this AI are queued until you hand back.',
    );
    prefs.lang = 'zh-CN';
    expect(activeSummary(start, start + 360_000)).toBe(
      '你已接管 6 分钟。发给它的消息在排队，交还后送达。',
    );
  });

  it('words the refusals it knows and says what the bar shows in every phase', () => {
    const busy =
      'This AI is busy: its terminal is already open, one of its tasks is merging, or it is about to start a turn (perhaps waiting for a free slot on its computer). Try again soon.';
    for (const [code, text] of [
      ['busy', busy],
      ['cli', 'The AI CLI could not open the interactive session on its computer.'],
      ['conflict', 'This session cannot be taken over in its current state.'],
      ['home_unreachable', "The AI's computer cannot reach this group's home right now."],
      ['not_found', 'This AI or task was not found on its computer.'],
      ['lost', "Cannot reach this AI's computer."],
    ]) {
      expect(takeoverErrorText(code)).toBe(text);
    }
    expect(takeoverErrorText('forbidden')).toBeUndefined();
    prefs.lang = 'zh-CN';
    expect(takeoverErrorText('busy')).toBe(
      '这个 AI 正忙：它的终端已经开着，或它的任务正在合并，或它马上要开始一轮（也许在等它的电脑有空位）。请稍后再试。',
    );
    prefs.lang = 'en';

    const ai = row<p.AIMemberRow>('row-member-ai', { state: 'working' });
    const base = {
      phase: 'idle' as const,
      ai,
      online: true,
      path: direct,
      errorCode: '',
      startedAt: 1000,
      nowMs: 121_000,
    };
    const bar = (title: string, detail: string, handback = false) => ({
      title,
      detail,
      handback,
    });
    const offline = bar('Computer offline', "Cannot reach this AI's computer right now.");
    const opening = bar(
      'Opening terminal…',
      'Switching the AI session into interactive terminal mode.',
      true,
    );
    const direct1 = bar(
      'Direct connection required',
      'The terminal works over a direct connection only. Establish one to this computer first.',
    );

    expect(takeoverBarState({ ...base, ai: undefined })).toEqual(
      bar('AI not found', 'This AI member has not been read yet or has left the group.'),
    );
    expect(takeoverBarState({ ...base, online: false })).toEqual(offline);
    expect(takeoverBarState({ ...base, ai: { ...ai, state: 'offline' } })).toEqual(offline);
    expect(takeoverBarState({ ...base, path: undefined })).toEqual(
      bar('Connecting…', 'Waiting for a direct connection to this computer.'),
    );
    expect(takeoverBarState({ ...base, path: relay })).toEqual(direct1);
    expect(takeoverBarState(base)).toEqual(opening);
    expect(takeoverBarState({ ...base, phase: 'starting' })).toEqual(opening);
    expect(takeoverBarState({ ...base, phase: 'relay' })).toEqual(direct1);
    expect(takeoverBarState({ ...base, phase: 'relayed' })).toEqual(
      bar(
        'Handed back · switched to relay',
        'The connection switched to a relay, so the terminal closed and handed back to the AI.',
      ),
    );
    expect(takeoverBarState({ ...base, phase: 'ended' })).toEqual(
      bar('Handed back', 'The interactive session ended and control returned to the AI.'),
    );
    expect(takeoverBarState({ ...base, phase: 'active' })).toEqual(
      bar(
        'Owner is driving',
        'You took over 2 min ago. Messages to this AI are queued until you hand back.',
        true,
      ),
    );
    expect(takeoverBarState({ ...base, phase: 'error', errorCode: 'busy' })).toEqual({
      title: 'Cannot open terminal',
      detail: busy,
      errorCode: undefined,
      handback: false,
    });
    expect(takeoverBarState({ ...base, phase: 'error', errorCode: 'internal' })).toEqual({
      title: 'Cannot open terminal',
      detail: undefined,
      errorCode: 'internal',
      handback: false,
    });
  });

  it('applies a sticky Ctrl to the first character from @ to ~', () => {
    expect(applyCtrl('c', false)).toEqual({ out: 'c', ctrl: false });
    expect(applyCtrl('', true)).toEqual({ out: '', ctrl: true });
    expect(applyCtrl('?', true)).toEqual({ out: '?', ctrl: false });
    expect(applyCtrl('@', true)).toEqual({ out: '\x00', ctrl: false });
    expect(applyCtrl('C', true)).toEqual({ out: '\x03', ctrl: false });
    expect(applyCtrl('cd', true)).toEqual({ out: '\x03d', ctrl: false });
    expect(applyCtrl('~', true)).toEqual({ out: '\x1e', ctrl: false });
    expect(applyCtrl('\x7f', true)).toEqual({ out: '\x7f', ctrl: false });
  });

  it('reads binary strings and channel messages, and counts the files of a hand-back', () => {
    expect([...binaryBytes('\x00\x7f\xffĂ')]).toEqual([0, 0x7f, 0xff, 0x02]);
    const bytes = new Uint8Array([65, 66]);
    expect(toTerminalChunk(bytes)).toBe(bytes);
    expect(toTerminalChunk(bytes.buffer)).toEqual(bytes);
    expect(toTerminalChunk('hello')).toBe('hello');
    expect(toTerminalChunk(42)).toBeUndefined();

    expect(handbackFilesLabel(-1)).toBe('No file changes');
    expect(handbackFilesLabel(0)).toBe('No file changes');
    expect(handbackFilesLabel(1)).toBe('1 file');
    expect(handbackFilesLabel(3)).toBe('3 files');
  });
});

describe('the takeover commands and the channel', () => {
  it('signs takeover.start and takeover.end, and words a lost or refused start', async () => {
    vi.spyOn(Date, 'now').mockReturnValue(1763600000000);
    const offline = new Home(me);
    expect(await startTakeover(offline, hostId, gid, memberId)).toBe('lost');
    await endTakeover(undefined, hostId, gid, memberId);
    await endTakeover(offline, hostId, gid, memberId);
    expect(signed).toEqual([]);

    const asked: unknown[] = [];
    const host = onlineHome(hostId, async (req) => {
      asked.push(req);
      return { re: 1, ok: true };
    });
    expect(await startTakeover(host, hostId, gid, memberId)).toBe('');
    expect(JSON.parse(signed[0])).toEqual({
      t: 'takeover.start',
      n: expect.stringMatching(/^[A-Za-z0-9_-]{21}[AQgw]$/),
      ts: 1763600000000,
      to: hostId,
      g: gid,
      member: memberId,
    });
    expect(await startTakeover(host, hostId, gid, memberId, taskId)).toBe('');
    expect(p.decode(p.TakeoverStart, signed[1]).task).toBe(taskId);
    await endTakeover(host, hostId, gid, memberId);
    expect(asked.map((req) => (req as { t: string }).t)).toEqual(['cmd', 'cmd', 'cmd']);
    expect(p.decode(p.TakeoverEnd, signed[2])).toMatchObject({
      t: 'takeover.end',
      ts: 1763600000000,
      to: hostId,
      g: gid,
      member: memberId,
    });

    const refusing = onlineHome(hostId, async () => ({ re: 2, err: { code: 'busy', msg: '' } }));
    expect(await startTakeover(refusing, hostId, gid, memberId)).toBe('busy');
    const dropping = onlineHome(hostId, () => Promise.reject(new Error('dropped')));
    expect(await startTakeover(dropping, hostId, gid, memberId)).toBe('lost');
    await expect(endTakeover(dropping, hostId, gid, memberId)).resolves.toBeUndefined();
    vi.restoreAllMocks();
  });

  it('pushes pty.resize for a size of at least one column and one row', () => {
    const pushed: unknown[] = [];
    const ctl = { request: vi.fn(), send: (frame: unknown) => pushed.push(frame) } as p.Ctl;
    sendResize(undefined, gid, memberId, 80, 24);
    sendResize({ request: vi.fn() } as p.Ctl, gid, memberId, 80, 24);
    sendResize(ctl, gid, memberId, 0, 24);
    sendResize(ctl, gid, memberId, 80, 0);
    expect(pushed).toEqual([]);
    sendResize(ctl, gid, memberId, 1, 1);
    sendResize(ctl, gid, memberId, 120.8, 32.4);
    expect(pushed).toEqual([
      { t: 'pty.resize', g: gid, ai: memberId, cols: 1, rows: 1 },
      { t: 'pty.resize', g: gid, ai: memberId, cols: 120, rows: 32 },
    ]);
  });

  it('carries bytes both ways, sends only on an open channel, and tells a close from afar', () => {
    const far = fakeChannel('connecting');
    const chunks: unknown[] = [];
    const closed = vi.fn();
    const bridge = bridgePty(far.channel, (chunk) => chunks.push(chunk), closed);
    expect(far.channel.binaryType).toBe('arraybuffer');
    bridge.sendText('early');
    expect(far.sent).toEqual([]);

    far.open();
    bridge.sendText('');
    bridge.sendText('ls\r');
    bridge.send(new Uint8Array([3]));
    expect(far.sent).toEqual([[108, 115, 13], [3]]);
    far.deliver(new Uint8Array([65]).buffer);
    far.deliver('prompt> ');
    far.deliver(null);
    expect(chunks).toEqual([new Uint8Array([65]), 'prompt> ']);

    far.remoteClose();
    far.remoteClose();
    expect(closed).toHaveBeenCalledTimes(1);
    far.open();
    bridge.send(new Uint8Array([9]));
    bridge.close();
    expect([far.sent.length, far.closes]).toEqual([2, 0]);

    const near = fakeChannel();
    const nearClosed = vi.fn();
    const own = bridgePty(near.channel, () => {}, nearClosed);
    own.close();
    own.close();
    expect([near.closes, near.channel.onmessage, near.channel.onclose]).toEqual([1, null, null]);
    expect(nearClosed).not.toHaveBeenCalled();
  });
});

describe('a takeover', () => {
  const ctrlC = { label: 'Ctrl', seq: '', ctrl: true } as const;

  it('starts on a direct path, opens the channel, sends its size, and hands back', async () => {
    const pc = computer();
    const written: unknown[] = [];
    const tk = new Takeover(gid, memberId, taskId, (chunk) => written.push(chunk));
    tk.follow(undefined, direct);
    tk.follow(pc.target, undefined);
    expect([tk.phase, pc.commands]).toEqual(['idle', []]);

    tk.resize(100, 30);
    vi.spyOn(Date, 'now').mockReturnValue(5000);
    tk.follow(pc.target, direct);
    expect(tk.phase).toBe('starting');
    tk.follow(pc.target, direct);
    await pc.answer();
    expect(pc.sent('takeover.start')).toEqual([
      expect.objectContaining({ to: hostId, g: gid, member: memberId, task: taskId }),
    ]);
    expect([tk.phase, tk.startedAt, tk.errorCode]).toEqual(['active', 5000, '']);
    expect(pc.channels.map((c) => c.label)).toEqual([`pty:${gid}:${memberId}`]);
    expect(pc.resizes).toEqual([{ t: 'pty.resize', g: gid, ai: memberId, cols: 100, rows: 30 }]);
    tk.resize(120, 32);
    expect(pc.resizes.at(-1)).toEqual({
      t: 'pty.resize',
      g: gid,
      ai: memberId,
      cols: 120,
      rows: 32,
    });

    const far = pc.channels[0].fake;
    far.deliver('claude> ');
    tk.press(ctrlC);
    expect(tk.ctrl).toBe(true);
    tk.type('c');
    expect(tk.ctrl).toBe(false);
    tk.press(phoneKeys[7]);
    tk.typeBinary('\x80');
    expect([written, far.sent]).toEqual([['claude> '], [[3], [13], [0x80]]]);

    tk.handBack();
    tk.handBack();
    await vi.waitFor(() => expect(pc.sent('takeover.end')).toHaveLength(1));
    expect(pc.sent('takeover.end')[0]).toMatchObject({ to: hostId, g: gid, member: memberId });
    expect([tk.phase, far.closes]).toEqual(['ended', 1]);
    tk.resize(90, 20);
    tk.follow(pc.target, direct);
    expect([pc.resizes.length, pc.sent('takeover.start').length]).toEqual([2, 1]);
    vi.restoreAllMocks();
  });

  it('sends takeover.end after the answer when handed back while still opening (A7.6)', async () => {
    const pc = computer();
    const tk = new Takeover(gid, memberId, undefined, () => {});
    tk.follow(pc.target, direct);
    tk.handBack();
    expect(tk.phase).toBe('ended');
    await new Promise((later) => setTimeout(later));
    expect(pc.sent('takeover.end')).toEqual([]);

    await pc.answer();
    await vi.waitFor(() => expect(pc.sent('takeover.end')).toHaveLength(1));
    expect(pc.commands.map((cmd) => cmd.t)).toEqual(['takeover.start', 'takeover.end']);
    expect([tk.phase, pc.channels]).toEqual(['ended', []]);

    const refused = new Takeover(gid, memberId, undefined, () => {});
    refused.follow(pc.target, direct);
    refused.handBack();
    await pc.answer({ re: 1, err: { code: 'busy', msg: '' } });
    expect(pc.sent('takeover.end')).toHaveLength(1);
  });

  it('refuses a relayed path, and closes without takeover.end when the path becomes relayed', async () => {
    const pc = computer();
    const tk = new Takeover(gid, memberId, undefined, () => {});
    tk.follow(pc.target, relay);
    expect([tk.phase, pc.commands]).toEqual(['idle', []]);

    const live = new Takeover(gid, memberId, undefined, () => {});
    live.follow(pc.target, direct);
    live.follow(pc.target, relay);
    expect(live.phase).toBe('starting');
    await pc.answer();
    live.follow(pc.target, relay);
    expect([live.phase, pc.channels[0].fake.closes]).toEqual(['relayed', 1]);
    live.handBack();
    live.follow(pc.target, relay);
    await new Promise((later) => setTimeout(later));
    expect([live.phase, pc.sent('takeover.end')]).toEqual(['relayed', []]);
  });

  it('starts when a relayed path recovers to direct', async () => {
    const pc = computer();
    const tk = new Takeover(gid, memberId, undefined, () => {});
    tk.follow(pc.target, relay);
    tk.follow(pc.target, direct);
    expect(tk.phase).toBe('starting');
    await pc.answer();
    expect([tk.phase, pc.channels.length]).toEqual(['active', 1]);
  });

  it('starts again when a path refused as relayed turns relay and recovers to direct', async () => {
    const pc = computer();
    const tk = new Takeover(gid, memberId, undefined, () => {});
    tk.follow(pc.target, direct);
    await pc.answer({ re: 1, err: { code: 'relay', msg: '' } });
    tk.follow(pc.target, relay);
    tk.follow(pc.target, direct);
    expect(tk.phase).toBe('starting');
  });

  it('shows a refusal, a lost channel and the computer closing the terminal', async () => {
    const pc = computer();
    const relayed = new Takeover(gid, memberId, undefined, () => {});
    relayed.follow(pc.target, direct);
    await pc.answer({ re: 1, err: { code: 'relay', msg: '' } });
    expect([relayed.phase, relayed.errorCode]).toEqual(['relay', 'relay']);

    const busy = new Takeover(gid, memberId, undefined, () => {});
    busy.follow(pc.target, direct);
    await pc.answer({ re: 1, err: { code: 'busy', msg: '' } });
    expect([busy.phase, busy.errorCode]).toEqual(['error', 'busy']);
    busy.handBack();
    expect(busy.phase).toBe('error');

    const closed = new Takeover(gid, memberId, undefined, () => {});
    closed.follow(pc.target, direct);
    await pc.answer();
    pc.channels[0].fake.remoteClose();
    closed.type('x');
    closed.typeBinary('x');
    closed.handBack();
    await new Promise((later) => setTimeout(later));
    expect([closed.phase, pc.sent('takeover.end'), pc.channels[0].fake.sent]).toEqual([
      'ended',
      [],
      [],
    ]);

    pc.noChannel();
    const lost = new Takeover(gid, memberId, undefined, () => {});
    lost.follow(pc.target, direct);
    await pc.answer();
    expect([lost.phase, lost.errorCode]).toEqual(['error', 'lost']);

    const dropped = new Takeover(gid, memberId, undefined, () => {});
    dropped.follow(pc.target, direct);
    pc.home.ctl = undefined;
    await pc.answer();
    expect([dropped.phase, dropped.errorCode]).toEqual(['error', 'lost']);
  });
});
