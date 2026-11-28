import { describe, expect, it } from 'vitest';
import { Group, Home } from './groups.svelte';
import { goldens } from './mock';
import { prefs } from './prefs.svelte';
import { decode, Row, type TaskRow } from './proto';
import { reviewLine, taskBadge, taskSteps, threadPreview, waitBadge } from './task';

const row = <T extends Row = Row>(name: string, changes: object = {}) =>
  ({ ...decode(Row, goldens[name]), ...changes }) as T;

function makeGroup(withGroupRow = true) {
  const owner = row<Extract<Row, { r: 'member'; kind: 'human' }>>('row-member-human');
  const home = new Home({ user: owner.user, sign: () => new Promise(() => {}) });
  const group = new Group(home, owner.g);
  const initial: Row[] = [
    owner,
    row('row-member-ai'),
    row('row-member-ai', { id: 'LXCZmuGAXkvO-bSrOkuCfw', name: 'Claude Code', cli: 'claude' }),
    row('row-member-human', { id: 'mina', name: 'Mina', user: 'mina-user' }),
  ];
  if (withGroupRow) initial.unshift(row('row-group'));
  group.apply(initial);
  return { group, owner };
}

describe('task helpers', () => {
  prefs.lang = 'en';

  it('computes badges across terminal, paused, queued, waiting and active states', () => {
    const { group, owner } = makeGroup();
    const base = row<TaskRow>('row-task');

    expect(taskBadge(group, { ...base, state: 'done' })).toEqual({
      text: 'Done · 86',
      tone: 'ok',
    });
    expect(taskBadge(group, { ...base, state: 'done', review: undefined })).toEqual({
      text: 'Done',
      tone: 'ok',
    });
    expect(taskBadge(group, { ...base, state: 'cancelled' })).toEqual({ text: 'Cancelled' });
    expect(taskBadge(group, { ...base, state: 'failed', failures: 1 })).toEqual({
      text: 'Stopped after 1 failures',
      tone: 'danger',
      solid: true,
    });
    expect(taskBadge(group, { ...base, state: 'failed', failures: 0 })).toEqual({
      text: 'Stopped after 1 failed reviews',
      tone: 'danger',
      solid: true,
    });
    expect(
      taskBadge(group, {
        ...base,
        state: 'failed',
        failures: 0,
        review: { ...base.review!, failed: 0 },
      }),
    ).toEqual({ text: 'Stopped', tone: 'danger', solid: true });
    expect(taskBadge(group, { ...base, state: 'failed', failures: 0, review: undefined })).toEqual({
      text: 'Stopped',
      tone: 'danger',
      solid: true,
    });

    expect(taskBadge(group, { ...base, state: 'paused' })).toEqual({
      text: 'Paused',
      dot: 'paused',
    });
    expect(taskBadge(group, { ...base, state: 'queued' })).toEqual({ text: 'Queued' });
    const busy = { kind: 'busy', since: 1 };
    expect(taskBadge(group, { ...base, state: 'queued', wait: busy })).toEqual({ text: 'Queued' });

    expect(taskBadge(group, base)).toEqual({ text: 'Sent back · round 1', tone: 'danger' });
    expect(taskBadge(group, { ...base, review: { ...base.review!, ok: true } })).toEqual({
      text: 'In progress',
      tone: 'sky',
      dot: 'working',
    });
    expect(taskBadge(group, { ...base, wait: { kind: 'reviewer', since: 1 } })).toEqual({
      text: 'Waiting for a reviewer',
    });
    expect(taskBadge(group, { ...base, assignee: 'departed', review: undefined })).toEqual({
      text: 'In progress',
      tone: 'sky',
      dot: 'working',
    });

    expect(waitBadge(group, null)).toBeUndefined();
    expect(waitBadge(group, { kind: 'depends', since: 1 })).toBeUndefined();
    expect(waitBadge(group, { kind: 'approval', who: [owner.user], since: 1 })).toEqual({
      text: 'Waiting for your approval',
      tone: 'dawn',
      solid: true,
    });
    expect(waitBadge(group, { kind: 'approval', who: ['mina-user'], since: 1 })).toEqual({
      text: "Waiting for Mina's approval",
    });
    expect(waitBadge(group, { kind: 'approval', since: 1 })).toEqual({
      text: 'Waiting for approval',
    });
    expect(waitBadge(group, { kind: 'decision', who: [owner.user], since: 1 })).toEqual({
      text: 'Waiting for your decision',
      tone: 'dawn',
      solid: true,
    });
    expect(
      waitBadge(group, { kind: 'decision', who: ['stranger', 'mina-user'], since: 1 }),
    ).toEqual({
      text: "Waiting for Mina's decision",
    });
    expect(waitBadge(group, { kind: 'decision', who: ['stranger'], since: 1 })).toEqual({
      text: 'Waiting for a decision',
    });
    expect(waitBadge(group, { kind: 'confirm', who: [owner.user], since: 1 })).toEqual({
      text: 'Waiting for your confirmation',
      tone: 'dawn',
      solid: true,
    });
    expect(waitBadge(group, { kind: 'confirm', who: ['mina-user'], since: 1 })).toEqual({
      text: "Waiting for Mina's confirmation",
    });
    expect(waitBadge(group, { kind: 'confirm', who: ['stranger'], since: 1 })).toEqual({
      text: 'Waiting for confirmation',
    });
    expect(taskBadge(group, row<TaskRow>('row-task-confirm'))).toEqual({
      text: 'Waiting for your confirmation',
      tone: 'dawn',
      solid: true,
    });
  });

  it('shows the takeover of the assignee after a final state and before every other', () => {
    const { group } = makeGroup();
    const task = row<TaskRow>('row-task');
    const ai = (changes: object) => group.apply([row('row-member-ai', changes)]);
    const takeover = { text: "AI's owner is driving", dot: 'takeover' };
    const rejected = { text: 'Sent back · round 1', tone: 'danger' };

    ai({ seq: 900, state: 'takeover', task: undefined });
    expect(taskBadge(group, task)).toEqual(rejected);
    ai({ seq: 901, state: 'takeover', task: 'another' });
    expect(taskBadge(group, task)).toEqual(rejected);
    ai({ seq: 902, state: 'waiting', task: task.id });
    expect(taskBadge(group, task)).toEqual(rejected);

    ai({ seq: 903, state: 'takeover', task: task.id });
    expect(taskBadge(group, task)).toEqual(takeover);
    expect(taskBadge(group, { ...task, state: 'paused' })).toEqual(takeover);
    expect(taskBadge(group, { ...task, wait: { kind: 'reviewer', since: 1 } })).toEqual(takeover);
    expect(taskBadge(group, { ...task, state: 'cancelled' })).toEqual({ text: 'Cancelled' });
    expect(taskBadge(group, { ...task, assignee: 'departed' })).toEqual(rejected);
    prefs.lang = 'zh-CN';
    expect(taskBadge(group, task)).toEqual({ text: '主人正在直接操作', dot: 'takeover' });
    prefs.lang = 'en';
  });

  it('puts the stronger-AI badge after waits and before rejected review, only while active', () => {
    const { group } = makeGroup();
    const task = row<TaskRow>('row-task', {
      stuck: { kind: 'review', n: 2, ask: true },
    });
    expect(taskBadge(group, task)).toEqual({ text: 'Must ask a stronger AI', tone: 'dawn' });
    expect(taskBadge(group, { ...task, stuck: { ...task.stuck!, ask: false } })).toEqual({
      text: 'Sent back · round 1',
      tone: 'danger',
    });
    expect(taskBadge(group, { ...task, wait: { kind: 'reviewer', since: 1 } })).toEqual({
      text: 'Waiting for a reviewer',
    });
    expect(taskBadge(group, { ...task, state: 'queued' })).toEqual({ text: 'Queued' });
    expect(taskBadge(group, { ...task, state: 'paused' })).toEqual({
      text: 'Paused',
      dot: 'paused',
    });
    expect(taskBadge(group, { ...task, state: 'cancelled' })).toEqual({ text: 'Cancelled' });
    prefs.lang = 'zh-CN';
    expect(taskBadge(group, task)).toEqual({ text: '须先请教更强的 AI', tone: 'dawn' });
    prefs.lang = 'en';
  });

  it('maps the three steps across all states and steps', () => {
    const { group, owner } = makeGroup(false);
    const base = row<TaskRow>('row-task-waiting', { wait: null });
    const marks = (task: TaskRow, status = 'In progress') =>
      taskSteps(group, task, status).items.map((item) => item.mark);

    const queued = taskSteps(group, { ...base, state: 'queued' }, 'Queued');
    expect(queued.summary).toBe('Queued · 0 / 3');
    expect(queued.aria).toBe('Progress: 0 of 3 steps, Queued');
    expect(queued.items.map((item) => [item.key, item.label, item.mark])).toEqual([
      ['work', 'Work', undefined],
      ['review', 'Review', undefined],
      ['done', 'Done', undefined],
    ]);

    const done = taskSteps(group, { ...base, state: 'done', step: 'review' }, 'Done');
    expect(done.summary).toBe('Done · 3 / 3');
    expect(done.aria).toBe('Progress: 3 of 3 steps, Done');
    expect(done.items.map((item) => item.mark)).toEqual(['done', 'done', 'done']);

    const work = taskSteps(group, { ...base, step: 'work' }, 'In progress');
    expect(work.summary).toBe('Work · 1 / 3');
    expect(work.aria).toBe('Progress: step 1 of 3, Work (In progress)');
    expect(work.items.map((item) => item.mark)).toEqual(['now', undefined, undefined]);

    const review = taskSteps(group, { ...base, step: 'review' }, 'In progress');
    expect(review.summary).toBe('Review · 2 / 3');
    expect(review.items.map((item) => item.mark)).toEqual(['done', 'now', undefined]);
    // A review that sent the work back is the badge's to say; the bar shows where the task is.
    expect(marks(row<TaskRow>('row-task'), 'Sent back')).toEqual(['done', 'now', undefined]);
    expect(marks({ ...base, step: 'work', review: row<TaskRow>('row-task').review })).toEqual([
      'now',
      undefined,
      undefined,
    ]);

    for (const kind of ['approval', 'decision', 'confirm'])
      expect(marks({ ...base, wait: { kind, who: [owner.user], since: 1 } }, 'Waiting')).toEqual([
        'wait',
        undefined,
        undefined,
      ]);
    for (const wait of [
      { kind: 'confirm', who: ['mina-user'], since: 1 },
      { kind: 'decision', since: 1 },
      { kind: 'reviewer', who: [owner.user], since: 1 },
    ])
      expect(marks({ ...base, step: 'review', wait }, 'Waiting')).toEqual([
        'done',
        'now',
        undefined,
      ]);

    expect(marks({ ...base, state: 'paused', step: 'review' }, 'Paused')).toEqual([
      'done',
      undefined,
      undefined,
    ]);
    expect(marks({ ...base, state: 'cancelled' }, 'Cancelled')).toEqual([
      undefined,
      undefined,
      undefined,
    ]);
    expect(marks({ ...base, state: 'failed', step: 'review' }, 'Stopped')).toEqual([
      'done',
      'fail',
      undefined,
    ]);
  });

  it('formats review metadata and thread previews across present and missing members', () => {
    const { group } = makeGroup();
    const base = row<TaskRow>('row-task');

    expect(reviewLine(group, { ...base, review: { ...base.review!, ok: true } })).toEqual({
      verdict: 'Round 1 · 86, passed',
      tone: 'ok',
      meta: 'Claude Code · pass 90',
    });
    expect(reviewLine(group, { ...base, review: { ...base.review!, by: 'departed' } })).toEqual({
      verdict: 'Round 1 · 86, sent back',
      tone: 'danger',
      meta: 'Pass 90',
    });
    expect(reviewLine(group, { ...base, review: undefined })).toEqual({
      meta: 'Not started · Claude Code to review · pass 90',
    });
    expect(reviewLine(group, { ...base, review: undefined, reviewer: 'departed' })).toEqual({
      meta: 'Not started · pass 90',
    });
    expect(reviewLine(group, { ...base, review: undefined, reviewer: undefined })).toEqual({
      meta: 'Not started · pass 90',
    });

    expect(threadPreview(group, { ...base, thread: { count: 0, rounds: 0 } })).toBe(
      'No messages yet',
    );
    expect(
      threadPreview(group, { ...base, thread: { count: 1, rounds: 0, last: undefined } }),
    ).toBe('');
    expect(threadPreview(group, base)).toBe('Latest: Codex · Added the path indicator.');
    expect(
      threadPreview(group, {
        ...base,
        thread: {
          count: 1,
          rounds: 0,
          last: { by: 'system', kind: 'approval', text: '', ts: 1 },
        },
      }),
    ).toBe('Latest: System · Approval');
    expect(
      threadPreview(group, {
        ...base,
        thread: {
          count: 1,
          rounds: 0,
          last: { by: 'departed', kind: 'review', text: '', ts: 1 },
        },
      }),
    ).toBe('Latest: Former member · Review');

    const withSystemLast = (ts: number): TaskRow => ({
      ...base,
      thread: {
        count: 1,
        rounds: 0,
        last: { by: 'system', kind: 'system', text: '', ts },
      },
    });

    // Not loaded yet: falls back to the generic kind label.
    expect(threadPreview(group, withSystemLast(1000))).toBe('Latest: System · Notice');
    prefs.lang = 'zh-CN';
    expect(threadPreview(group, withSystemLast(1000))).toBe('最新：系统 · 通知');
    prefs.lang = 'en';

    // Loaded single-line system message matching ts.
    group.apply([
      base,
      row('row-msg-system', {
        id: 'sys-single',
        seq: 10,
        pos: 10,
        thread: base.id,
        ts: 1000,
        body: {
          code: 'turn_interrupted',
          ai: base.assignee,
          reason: 'timeout',
          task: base.id,
          detail: 'Turn deadline reached.',
        },
      }),
    ]);
    expect(threadPreview(group, withSystemLast(1000))).toBe(
      "Latest: System · Codex's turn timed out. · T-12 · Turn deadline reached.",
    );

    // Different ts, non-system message with matching ts, or system message in another thread still falls back.
    group.apply([
      row('row-msg-text', {
        id: 'text-same-ts',
        seq: 11,
        pos: 11,
        thread: base.id,
        ts: 2000,
        text: 'Not a system row',
      }),
      row('row-msg-system', {
        id: 'sys-other-thread',
        seq: 12,
        pos: 12,
        thread: 'other-task',
        ts: 2000,
        body: { code: 'no_secretary' },
      }),
    ]);
    expect(threadPreview(group, withSystemLast(2000))).toBe('Latest: System · Notice');

    // Multiple system messages with matching ts: picks the last one in the thread.
    group.apply([
      row('row-msg-system', {
        id: 'sys-first',
        seq: 13,
        pos: 13,
        thread: base.id,
        ts: 3000,
        body: { code: 'no_secretary' },
      }),
      row('row-msg-system', {
        id: 'sys-second',
        seq: 14,
        pos: 14,
        thread: base.id,
        ts: 3000,
        body: { code: 'task_done', task: base.id },
      }),
      row('row-msg-text', {
        id: 'text-after-sys',
        seq: 15,
        pos: 15,
        thread: base.id,
        ts: 3000,
        text: 'Ignored non-system message',
      }),
    ]);
    expect(threadPreview(group, withSystemLast(3000))).toBe(
      'Latest: System · T-12 passed review and is done',
    );
    prefs.lang = 'zh-CN';
    expect(threadPreview(group, withSystemLast(3000))).toBe('最新：系统 · T-12 通过评审，已完成');
    prefs.lang = 'en';

    // Multi-line system text: keeps only the first line.
    group.apply([
      row('row-member-human', {
        id: 'multiline-member',
        seq: 16,
        user: 'multiline-user',
        name: 'Mina\nsecond line',
      }),
      row('row-msg-system', {
        id: 'sys-multiline',
        seq: 17,
        pos: 17,
        thread: base.id,
        ts: 4000,
        body: { code: 'member_joined', member: base.assignee, by: 'multiline-member' },
      }),
    ]);
    expect(threadPreview(group, withSystemLast(4000))).toBe(
      'Latest: System · Codex joined, invited by Mina',
    );

    // Non-system rows keep exact existing behavior even when matching system messages exist.
    expect(
      threadPreview(group, {
        ...base,
        thread: {
          count: 2,
          rounds: 0,
          last: { by: base.assignee, kind: 'text', text: 'Custom message', ts: 3000 },
        },
      }),
    ).toBe('Latest: Codex · Custom message');
    expect(
      threadPreview(group, {
        ...base,
        thread: {
          count: 2,
          rounds: 0,
          last: { by: 'LXCZmuGAXkvO-bSrOkuCfw', kind: 'review', text: '', ts: 3000 },
        },
      }),
    ).toBe('Latest: Claude Code · Review');
  });

  it('shows the group-paused badge and clears active step marks while keeping queued, final and takeover states', () => {
    const { group, owner } = makeGroup();
    const base = row<TaskRow>('row-task');
    const active: TaskRow = {
      ...base,
      state: 'active',
      step: 'work',
      wait: null,
      review: undefined,
    };
    const marks = (task: TaskRow, status = taskBadge(group, task).text) =>
      taskSteps(group, task, status).items.map((item) => item.mark);

    group.apply([row('row-group-paused')]);

    // Active task across work, review, waits, and rejected review.
    expect(taskBadge(group, active)).toEqual({ text: 'Group paused', dot: 'paused' });
    expect(marks(active)).toEqual([undefined, undefined, undefined]);
    expect(taskSteps(group, active, 'Group paused')).toMatchObject({
      summary: 'Work · 1 / 3',
      aria: 'Progress: step 1 of 3, Work (Group paused)',
    });

    const atReview: TaskRow = { ...active, step: 'review' };
    expect(taskBadge(group, atReview)).toEqual({ text: 'Group paused', dot: 'paused' });
    expect(marks(atReview)).toEqual(['done', undefined, undefined]);

    const waitViewer: TaskRow = {
      ...active,
      wait: { kind: 'approval', who: [owner.user], since: 1 },
    };
    expect(taskBadge(group, waitViewer)).toEqual({ text: 'Group paused', dot: 'paused' });
    expect(marks(waitViewer)).toEqual([undefined, undefined, undefined]);

    const waitOther: TaskRow = {
      ...active,
      wait: { kind: 'decision', who: ['mina-user'], since: 1 },
    };
    expect(taskBadge(group, waitOther)).toEqual({ text: 'Group paused', dot: 'paused' });
    expect(marks(waitOther)).toEqual([undefined, undefined, undefined]);

    expect(taskBadge(group, base)).toEqual({ text: 'Group paused', dot: 'paused' });
    expect(marks(base)).toEqual(['done', undefined, undefined]);

    // Task-paused under a paused group.
    const taskPaused: TaskRow = { ...base, state: 'paused', step: 'review' };
    expect(taskBadge(group, taskPaused)).toEqual({ text: 'Group paused', dot: 'paused' });
    expect(marks(taskPaused)).toEqual(['done', undefined, undefined]);

    // Queued task stays Queued with -, -, -.
    const queued: TaskRow = { ...active, state: 'queued', step: 'work' };
    expect(taskBadge(group, queued)).toEqual({ text: 'Queued' });
    expect(taskSteps(group, queued, 'Queued')).toMatchObject({
      summary: 'Queued · 0 / 3',
      items: [
        expect.objectContaining({ key: 'work', mark: undefined }),
        expect.objectContaining({ key: 'review', mark: undefined }),
        expect.objectContaining({ key: 'done', mark: undefined }),
      ],
    });

    // Final states keep their final badges and step marks.
    expect(taskBadge(group, { ...base, state: 'done' })).toEqual({
      text: 'Done · 86',
      tone: 'ok',
    });
    expect(marks({ ...base, state: 'done' })).toEqual(['done', 'done', 'done']);

    expect(taskBadge(group, { ...base, state: 'cancelled' })).toEqual({ text: 'Cancelled' });
    expect(marks({ ...base, state: 'cancelled' })).toEqual(['done', undefined, undefined]);

    const failed: TaskRow = { ...base, state: 'failed', step: 'review', failures: 2 };
    expect(taskBadge(group, failed)).toEqual({
      text: 'Stopped after 2 failures',
      tone: 'danger',
      solid: true,
    });
    expect(marks(failed)).toEqual(['done', 'fail', undefined]);

    // Localization in zh-CN.
    prefs.lang = 'zh-CN';
    expect(taskBadge(group, active)).toEqual({ text: '群已暂停', dot: 'paused' });
    expect(taskBadge(group, queued)).toEqual({ text: '排队中' });
    prefs.lang = 'en';

    // Takeover still wins over group pause and keeps the active step mark.
    group.apply([row('row-member-ai', { seq: 903, state: 'takeover', task: active.id })]);
    expect(taskBadge(group, active)).toEqual({ text: "AI's owner is driving", dot: 'takeover' });
    expect(marks(active)).toEqual(['now', undefined, undefined]);
    expect(marks(atReview)).toEqual(['done', 'now', undefined]);

    // Resuming the group restores active badge and step mark.
    group.apply([row('row-member-ai', { seq: 904, state: 'waiting', task: undefined })]);
    group.apply([row('row-group', { seq: 999 })]);
    expect(taskBadge(group, active)).toEqual({
      text: 'In progress',
      tone: 'sky',
      dot: 'working',
    });
    expect(marks(active)).toEqual(['now', undefined, undefined]);
  });
});
