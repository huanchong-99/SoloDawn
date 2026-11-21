import { describe, expect, it } from 'vitest';
import { Group, Home } from '../../lib/groups.svelte';
import { goldens } from '../../lib/mock';
import { prefs } from '../../lib/prefs.svelte';
import { decode, Row, type TaskRow } from '../../lib/proto';
import {
  findTask,
  reviewLine,
  safePrUrl,
  taskBadge,
  taskSteps,
  threadPreview,
  waitBadge,
} from './task';

const row = <T extends Row = Row>(name: string, changes: object = {}) =>
  ({ ...decode(Row, goldens[name]), ...changes }) as T;

function makeGroup(forge = 'github', withGroupRow = true) {
  const owner = row<Extract<Row, { r: 'member'; kind: 'human' }>>('row-member-human');
  const home = new Home({ user: owner.user, sign: () => new Promise(() => {}) });
  const group = new Group(home, owner.g);
  const initial: Row[] = [
    owner,
    row('row-member-ai'),
    row('row-member-ai', { id: 'LXCZmuGAXkvO-bSrOkuCfw', name: 'Claude Code', cli: 'claude' }),
    row('row-member-human', { id: 'mina', name: 'Mina', user: 'mina-user' }),
  ];
  if (withGroupRow) initial.unshift(row('row-group', { forge }));
  group.apply(initial);
  return { group, owner };
}

describe('task helpers', () => {
  prefs.lang = 'en';

  it('finds a live task row and ignores missing or deleted entries', () => {
    const { group } = makeGroup();
    const task = row<TaskRow>('row-task');
    expect(findTask(group, task.id)).toBeUndefined();
    group.apply([task]);
    expect(findTask(group, task.id)).toEqual(task);
    group.apply([{ r: 'task', id: task.id, seq: 9999, gone: true }]);
    expect(findTask(group, task.id)).toBeUndefined();
  });

  it('accepts only http and https pull-request URLs', () => {
    expect(safePrUrl(undefined)).toBeUndefined();
    expect(safePrUrl('')).toBeUndefined();
    expect(safePrUrl('not a url')).toBeUndefined();
    expect(safePrUrl('javascript:alert(1)')).toBeUndefined();
    expect(safePrUrl('ftp://git.example/pr/1')).toBeUndefined();
    expect(safePrUrl('http://git.example/pr/1')).toBe('http://git.example/pr/1');
    expect(safePrUrl('https://git.example/pr/1')).toBe('https://git.example/pr/1');
  });

  it('computes badges across terminal, takeover, paused, queued, waiting and active states', () => {
    const { group, owner } = makeGroup();
    const base = row<TaskRow>('row-task');

    expect(taskBadge(group, { ...base, state: 'merged' })).toEqual({
      text: 'Merged · 86',
      tone: 'ok',
    });
    expect(taskBadge(group, { ...base, state: 'merged', review: undefined })).toEqual({
      text: 'Merged',
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

    // Sent back by review, waiting, and normal active badges.
    expect(taskBadge(group, base)).toEqual({ text: 'Sent back · round 1', tone: 'danger' });
    expect(taskBadge(group, { ...base, wait: { kind: 'reviewer', since: 1 } })).toEqual({
      text: 'Waiting for a reviewer',
    });
    expect(taskBadge(group, { ...base, wait: { kind: 'conflict', since: 1 } })).toEqual({
      text: 'Merge conflict',
      tone: 'danger',
    });
    expect(taskBadge(group, { ...base, assignee: 'departed', review: undefined })).toEqual({
      text: 'In progress',
      tone: 'sky',
      dot: 'working',
    });

    // Takeover applies only when the assignee is an AI in takeover on this exact task.
    group.apply([row('row-member-ai', { seq: 900, state: 'takeover', task: 'other-task' })]);
    expect(taskBadge(group, { ...base, review: undefined }).text).toBe('In progress');
    group.apply([row('row-member-ai', { seq: 901, state: 'takeover', task: base.id })]);
    expect(taskBadge(group, base)).toEqual({
      text: 'Owner is operating directly',
      dot: 'takeover',
    });

    expect(waitBadge(group, null)).toBeUndefined();
    expect(waitBadge(group, { kind: 'depends', since: 1 })).toBeUndefined();
    expect(waitBadge(group, { kind: 'approval', who: [owner.user], since: 1 })).toEqual({
      text: 'Waiting for your approval',
      tone: 'dawn',
      solid: true,
    });
    expect(waitBadge(group, { kind: 'approval', since: 1 })).toEqual({
      text: 'Waiting for approval',
    });
    expect(waitBadge(group, { kind: 'decision', who: [owner.user], since: 1 })).toEqual({
      text: 'Waiting for your decision',
      tone: 'dawn',
      solid: true,
    });
    expect(waitBadge(group, { kind: 'decision', who: ['mina-user'], since: 1 })).toEqual({
      text: "Waiting for Mina's decision",
    });
    expect(waitBadge(group, { kind: 'decision', who: [], since: 1 })).toEqual({
      text: 'Waiting for a decision',
    });
  });

  it('maps step segments across all states, steps and forge modes', () => {
    const { group, owner } = makeGroup('github', false);
    const base = row<TaskRow>('row-task-waiting', { wait: null });

    expect(taskSteps(group, { ...base, state: 'queued' }, 'Queued').summary).toBe('0 / 6');
    expect(taskSteps(group, { ...base, state: 'merged' }, 'Merged').summary).toBe('6 / 6');
    expect(taskSteps(group, { ...base, step: 'unknown' }, 'In progress').summary).toBe(
      'Work · 1 / 6',
    );

    // Local check transitions: failing check, ready for PR, already has PR, rejected review, conflict.
    const localFail = taskSteps(
      group,
      { ...base, step: 'local_check', checks: { local: { ok: false, at: 1, commit: 'c1' } } },
      'In progress',
    );
    expect(localFail.items.map((item) => item.mark)).toEqual([
      'done',
      'fail',
      undefined,
      undefined,
      undefined,
      undefined,
    ]);

    const openPr = taskSteps(
      group,
      { ...base, step: 'local_check', checks: { local: { ok: true, at: 1, commit: 'c1' } } },
      'In progress',
    );
    expect(openPr.summary).toBe('Open PR · 3 / 6');

    const withPr = taskSteps(
      group,
      {
        ...base,
        step: 'local_check',
        pr: 9,
        checks: { local: { ok: true, at: 1, commit: 'c1' } },
      },
      'In progress',
    );
    expect(withPr.summary).toBe('Local checks · 2 / 6');

    const conflictStep = taskSteps(
      group,
      {
        ...base,
        step: 'local_check',
        checks: { local: { ok: true, at: 1, commit: 'c1' } },
        wait: { kind: 'conflict', since: 1 },
      },
      'Merge conflict',
    );
    expect(conflictStep.items[1].mark).toBe('fail');

    // Cloud check: failing vs running, plus waiting on viewer vs non-viewer.
    expect(
      taskSteps(
        group,
        {
          ...base,
          step: 'cloud_check',
          pr: 9,
          checks: { cloud: { ok: false, at: 1, commit: 'c1' } },
        },
        'In progress',
      ).items[3].mark,
    ).toBe('fail');
    expect(
      taskSteps(group, { ...base, step: 'cloud_check', pr: 9 }, 'In progress').items[3].mark,
    ).toBe('now');
    expect(
      taskSteps(
        group,
        { ...base, wait: { kind: 'approval', who: [owner.user], since: 1 } },
        'Waiting',
      ).items[0].mark,
    ).toBe('wait');
    expect(
      taskSteps(group, { ...base, wait: { kind: 'decision', since: 1 } }, 'Waiting').items[0].mark,
    ).toBe('now');

    // Review, paused, failed, takeover, merge, and forge: none.
    expect(
      taskSteps(
        group,
        {
          ...base,
          step: 'review',
          pr: undefined,
          checks: { cloud: { ok: true, at: 1, commit: 'c1' } },
        },
        'In progress',
      ).items.map((item) => item.mark),
    ).toEqual(['done', 'done', undefined, 'done', 'now', undefined]);

    expect(taskSteps(group, row<TaskRow>('row-task'), 'Sent back').items[4].mark).toBe('fail');
    expect(
      taskSteps(group, { ...base, state: 'paused', step: 'merge', pr: 9 }, 'Paused').items[5].mark,
    ).toBeUndefined();
    expect(
      taskSteps(group, { ...base, state: 'failed', step: 'branch' }, 'Stopped').items[0].mark,
    ).toBe('fail');

    group.apply([row('row-member-ai', { seq: 950, state: 'takeover', task: base.id })]);
    expect(taskSteps(group, base, 'Takeover').items[0].mark).toBe('now');

    const localGroup = makeGroup('none').group;
    expect(
      taskSteps(
        localGroup,
        {
          ...base,
          state: 'merged',
          checks: { local: { ok: true, at: 1, commit: 'c1' } },
        },
        'Merged',
      ).items.map((item) => [item.mark, item.skipped]),
    ).toEqual([
      ['done', false],
      ['done', false],
      [undefined, true],
      [undefined, true],
      ['done', false],
      ['done', false],
    ]);
    expect(
      taskSteps(
        localGroup,
        { ...base, step: 'local_check', checks: { local: { ok: true, at: 1, commit: 'c1' } } },
        'In progress',
      ).summary,
    ).toBe('Local checks · 2 / 6');
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
      threadPreview(group, { ...base, thread: { count: 2, rounds: 0, last: undefined } }),
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
          last: { by: 'departed', kind: 'custom', text: '', ts: 1 },
        },
      }),
    ).toBe('Latest: Former member · custom');
  });
});
