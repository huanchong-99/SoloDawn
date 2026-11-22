import { flushSync } from 'svelte';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { watch } from '../../features/watch.svelte';
import { callView, formatElapsed, withoutInlineReplies, type CallMsg } from './call';
import { Group, Home } from './groups.svelte';
import { goldens } from './mock';
import { prefs } from './prefs.svelte';
import { decode, Row, type MsgRow } from './proto';

const row = <T extends Row = Row>(name: string, changes: object = {}) =>
  ({ ...decode(Row, goldens[name]), ...changes }) as T;

const codexId = '1PC8WineBrUQ-apCjx7tug';
const claudeId = 'LXCZmuGAXkvO-bSrOkuCfw';
const replyId = 'V4Kxhofmz4pIL8MtLbWxlg';

function makeGroup() {
  const owner = row<Extract<Row, { r: 'member'; kind: 'human' }>>('row-member-human');
  const home = new Home({ user: owner.user, sign: () => new Promise(() => {}) });
  const group = new Group(home, owner.g);
  group.apply([
    row('row-group'),
    owner,
    row('row-member-ai'),
    row('row-member-ai', { id: claudeId, name: 'Claude Code', cli: 'claude' }),
  ]);
  return { group, owner };
}

describe('call helpers', () => {
  afterEach(() => vi.useRealTimers());

  it('formats elapsed durations in whole units across every boundary and clamps negative values', () => {
    prefs.lang = 'en';

    expect(formatElapsed(-5_000)).toBe('0 s');
    expect(formatElapsed(0)).toBe('0 s');
    expect(formatElapsed(999)).toBe('0 s');
    expect(formatElapsed(1_000)).toBe('1 s');
    expect(formatElapsed(1_900)).toBe('1 s');
    expect(formatElapsed(38_000)).toBe('38 s');
    expect(formatElapsed(59_999)).toBe('59 s');
    expect(formatElapsed(60_000)).toBe('1 min');
    expect(formatElapsed(179_999)).toBe('2 min');
    expect(formatElapsed(3_599_999)).toBe('59 min');
    expect(formatElapsed(3_600_000)).toBe('1 h');
    expect(formatElapsed(10_799_999)).toBe('2 h');
    expect(formatElapsed(86_399_999)).toBe('23 h');
    expect(formatElapsed(86_400_000)).toBe('1 d');
    expect(formatElapsed(259_199_999)).toBe('2 d');

    prefs.lang = 'zh-CN';
    expect(formatElapsed(38_000)).toBe('38 秒');
    expect(formatElapsed(120_000)).toBe('2 分钟');
    expect(formatElapsed(7_200_000)).toBe('2 小时');
    expect(formatElapsed(172_800_000)).toBe('2 天');
    prefs.lang = 'en';
  });

  it('draws a waiting call again every second while it is under a minute old, then every minute', async () => {
    prefs.lang = 'en';
    vi.useFakeTimers();
    const { group } = makeGroup();
    const waiting = row<CallMsg>('row-msg-call-waiting');
    vi.setSystemTime(waiting.ts + 58_000);
    const seen: string[] = [];
    const stop = watch(() => void seen.push(callView(group, waiting).timing));
    flushSync();
    vi.advanceTimersByTime(1_000);
    flushSync();
    expect(seen).toEqual(['Waiting for reply · 58 s', 'Waiting for reply · 59 s']);
    expect(vi.getTimerCount()).toBe(2);

    vi.advanceTimersByTime(1_000);
    flushSync();
    await Promise.resolve();
    expect(seen.at(-1)).toBe('Waiting for reply · 1 min');
    // Only the minute clock is left.
    expect(vi.getTimerCount()).toBe(1);
    vi.advanceTimersByTime(57_999);
    flushSync();
    expect(seen).toHaveLength(3);
    vi.advanceTimersByTime(1);
    flushSync();
    expect(seen.at(-1)).toBe('Waiting for reply · 1 min');

    stop();
    await Promise.resolve();
    expect(vi.getTimerCount()).toBe(0);
  });

  it('starts no clock for an answered call', () => {
    vi.useFakeTimers();
    const { group } = makeGroup();
    const stop = watch(() => void callView(group, row<CallMsg>('row-msg-call-answered')));
    flushSync();
    expect(vi.getTimerCount()).toBe(0);
    stop();
  });

  it('derives waiting and answered call views and missing or deleted rows', () => {
    prefs.lang = 'en';
    const { group, owner } = makeGroup();
    const waiting = row<CallMsg>('row-msg-call-waiting');

    vi.useFakeTimers({ toFake: ['Date'] });
    vi.setSystemTime(waiting.ts + 15_000);
    const wView = callView(group, waiting);
    expect(wView).toEqual({
      caller: 'Codex',
      callee: 'Claude Code',
      mode: 'Waits for answer',
      waiting: true,
      timing: 'Waiting for reply · 15 s',
      depth: 'Depth 1',
      reply: undefined,
    });

    const asyncWaiting: CallMsg = {
      ...waiting,
      by: 'departed-caller',
      body: { to: 'departed-callee', depth: 3, wait: false },
    };
    vi.setSystemTime(waiting.ts + 360_000);
    const asyncView = callView(group, asyncWaiting);
    expect(asyncView.caller).toBe('Former member');
    expect(asyncView.callee).toBe('Former member');
    expect(asyncView.mode).toBe('Gets a receipt');
    expect(asyncView.timing).toBe('Waiting for reply · 6 min');
    expect(asyncView.depth).toBe('Depth 3');

    const answered = row<CallMsg>('row-msg-call-answered');
    expect(callView(group, answered).reply).toBeUndefined();

    group.apply([
      row('row-msg-reply', {
        text: '@Codex use `limited`',
        mentions: [codexId],
      }),
    ]);
    const aView = callView(group, answered);
    expect(aView.waiting).toBe(false);
    expect(aView.timing).toBe('Took 0 s');
    expect(aView.reply).toEqual({
      by: claudeId,
      name: 'Claude Code',
      cli: 'claude',
      text: '@Codex use `limited`',
      mentions: [codexId],
    });

    prefs.lang = 'zh-CN';
    const zhAnswered = callView(group, {
      ...answered,
      state: { status: 'answered', reply: replyId, ms: 38_000 },
    });
    expect([zhAnswered.mode, zhAnswered.timing, zhAnswered.depth].join(' · ')).toBe(
      '当场等结果 · 用时 38 秒 · 调用第 1 层',
    );
    prefs.lang = 'en';

    group.apply([
      row('row-msg-reply', { id: 'rep-human', seq: 901, by: owner.id, text: 'Noted' }),
      row('row-msg-reply', { id: 'rep-gone-member', seq: 902, by: 'left-ai', text: 'Done' }),
      { r: 'msg', id: replyId, seq: 950, gone: true },
    ]);
    expect(callView(group, answered).reply).toBeUndefined();
    expect(
      callView(group, { ...answered, state: { status: 'answered', reply: 'rep-human', ms: 10 } })
        .reply,
    ).toMatchObject({ by: owner.id, name: owner.name, cli: undefined });
    expect(
      callView(group, {
        ...answered,
        state: { status: 'answered', reply: 'rep-gone-member', ms: 10 },
      }).reply,
    ).toMatchObject({ by: 'left-ai', name: 'Former member', cli: undefined });
  });

  it('filters inline reply rows only when the call card is mounted and the call is answered', () => {
    const waiting = row<MsgRow>('row-msg-call-waiting', { id: 'c-wait' });
    const answered = row<MsgRow>('row-msg-call-answered', { id: 'c-ans' });
    const inlineReply = row<MsgRow>('row-msg-reply', { id: replyId, re: 'c-ans' });
    const orphanReply = row<MsgRow>('row-msg-reply', { id: 'rep-orphan', re: 'c-missing' });
    const waitingReply = row<MsgRow>('row-msg-reply', { id: 'rep-wait', re: 'c-wait' });
    const progress = row<MsgRow>('row-msg-progress', { id: 'prog-1' });
    const list = [progress, waiting, waitingReply, answered, inlineReply, orphanReply];

    expect(withoutInlineReplies(list, false)).toEqual(list);
    expect(withoutInlineReplies(list, true).map((m) => m.id)).toEqual([
      'prog-1',
      'c-wait',
      'rep-wait',
      'c-ans',
      'rep-orphan',
    ]);
  });
});
