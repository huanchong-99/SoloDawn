import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import { Group, Home } from './groups.svelte';
import { goldens } from './mock';
import { prefs } from './prefs.svelte';
import { decode, Row, TaskAssign, TaskCommand, type TaskRow } from './proto';
import {
  assignNotes,
  cancelNotes,
  changedRoles,
  ownTerminals,
  replyNote,
  sendTask,
  taskHold,
  taskOps,
  threadMembers,
  workingAIs,
} from './taskops';

const codex = '1PC8WineBrUQ-apCjx7tug';
const claude = 'LXCZmuGAXkvO-bSrOkuCfw';
const secretary = 'qBSFMsr2hHYKOMblEA_kdA';
const qwen = 'cXdlbi1tZW1iZXItMDAwMQ';

const row = <T extends Row = Row>(name: string, changes: object = {}) =>
  ({ ...decode(Row, goldens[name]), ...changes }) as T;
const owner = row<Extract<Row, { kind: 'human' }>>('row-member-human', { name: 'Alex' });
const base = row<TaskRow>('row-task', { assignee: codex, reviewer: claude });
const task = (changes: Partial<TaskRow> = {}): TaskRow => ({ ...base, ...changes });
const wait = (kind: string) => ({ kind, since: 1 });

const sent: string[] = [];

function makeGroup(role = 'owner', ...more: Row[]) {
  const sign = async (p: string) => {
    sent.push(p);
    return { p, k: 'this-browser', s: 'signature' };
  };
  const group = new Group(new Home({ user: owner.user, sign }), owner.g);
  group.apply([
    row('row-group'),
    { ...owner, role },
    row('row-member-human', { id: 'mina', name: 'Mina', user: 'mina-user', role: 'member' }),
    row('row-member-ai', { id: codex, name: 'Codex', state: 'waiting' }),
    row('row-member-ai', { id: claude, name: 'Claude Code', state: 'waiting' }),
    row('row-member-ai', { id: qwen, name: 'Qwen Code', user: 'mina-user', state: 'waiting' }),
    row('row-member-ai', { id: secretary, name: 'Secretary', secretary: true }),
    ...more,
  ]);
  return group;
}

beforeEach(() => {
  prefs.lang = 'en';
  sent.length = 0;
});
afterEach(() => {
  prefs.lang = 'en';
});

describe('what a person may do to a task', () => {
  it('is for an admin, by its state, and only the cancel once it is being merged', () => {
    for (const role of ['owner', 'admin']) {
      const group = makeGroup(role);
      expect(taskOps(group, task({ state: 'queued', step: 'branch' }))).toEqual([
        'assign',
        'cancel',
      ]);
      expect(taskOps(group, task({ state: 'active' }))).toEqual(['pause', 'assign', 'cancel']);
      expect(taskOps(group, task({ state: 'paused' }))).toEqual(['resume', 'assign', 'cancel']);
      expect(taskOps(group, task({ state: 'failed' }))).toEqual(['resume', 'assign', 'cancel']);
      expect(taskOps(group, task({ state: 'active', step: 'merge' }))).toEqual(['cancel']);
      expect(taskOps(group, task({ state: 'merged', step: 'merge' }))).toEqual([]);
      expect(taskOps(group, task({ state: 'cancelled' }))).toEqual([]);
    }
    const member = makeGroup('member');
    for (const state of ['queued', 'active', 'paused', 'failed'])
      expect(taskOps(member, task({ state }))).toEqual([]);
  });

  it('is sent as the commands of the contract, with the roles of an assign', async () => {
    const group = makeGroup();
    await sendTask(group, task(), 'resume');
    expect(decode(TaskCommand, sent[0])).toEqual({
      t: 'task',
      n: expect.any(String),
      ts: expect.any(Number),
      g: owner.g,
      op: 'resume',
      id: base.id,
    });
    await sendTask(group, task(), 'assign', { reviewer: qwen });
    expect(JSON.parse(sent[1])).not.toHaveProperty('assignee');
    expect(decode(TaskAssign, sent[1])).toMatchObject({
      op: 'assign',
      id: base.id,
      reviewer: qwen,
    });
  });

  it('gives a role to any AI but the secretary', () => {
    const ids = workingAIs(makeGroup()).map((ai) => ai.id);
    expect(ids).toEqual([claude, codex, qwen]);
    const none = new Group(new Home({ user: owner.user, sign: async () => ({}) as never }), 'g');
    none.apply([row('row-member-ai', { id: codex })]);
    expect(workingAIs(none).map((ai) => ai.id)).toEqual([codex]);
  });

  it('names in an assign only the roles that change', () => {
    expect(changedRoles(base, { assignee: codex, reviewer: claude })).toEqual({});
    expect(changedRoles(base, { assignee: qwen, reviewer: claude })).toEqual({ assignee: qwen });
    expect(changedRoles(base, { assignee: codex, reviewer: qwen })).toEqual({ reviewer: qwen });
    expect(changedRoles(base, { assignee: claude, reviewer: codex })).toEqual({
      assignee: claude,
      reviewer: codex,
    });
    // Nobody is named to review, and nobody is chosen.
    expect(changedRoles(task({ reviewer: undefined }), { assignee: codex, reviewer: '' })).toEqual(
      {},
    );
  });

  it('says what an assign does: who stops, and which owners are asked', () => {
    const group = makeGroup();
    expect(assignNotes(group, base, {})).toEqual([]);
    expect(assignNotes(group, base, { assignee: claude })).toEqual([
      'Codex stops here. Changes it has not committed are lost; its commits carry over.',
    ]);
    expect(assignNotes(group, base, { assignee: qwen })).toEqual([
      'Codex stops here. Changes it has not committed are lost; its commits carry over.',
      'Mina is asked first: Qwen Code is theirs.',
    ]);
    expect(assignNotes(group, base, { reviewer: qwen })).toEqual([
      'Mina is asked first: Qwen Code is theirs.',
    ]);
    prefs.lang = 'zh-CN';
    expect(assignNotes(group, base, { assignee: qwen })).toEqual([
      'Codex 就此停下。没提交的改动会丢，提交过的会接着用。',
      'Qwen Code 是 Mina 的，要先等对方确认。',
    ]);
    prefs.lang = 'en';
    // Before the branch there is nothing to lose; an AI that left is nobody's to stop.
    expect(assignNotes(group, task({ branch: undefined }), { assignee: claude })).toEqual([]);
    expect(assignNotes(group, task({ assignee: 'gone' }), { assignee: claude })).toEqual([]);
    const stray = makeGroup('owner', row('row-member-ai', { id: 'stray', name: 'S', user: 'x' }));
    expect(assignNotes(stray, base, { reviewer: 'stray' })).toEqual([
      'Former member is asked first: S is theirs.',
    ]);
  });

  it('says what a cancel does', () => {
    const queued = (id: string, changes: object) =>
      row('row-task', { id, state: 'queued', depends: [base.id], ...changes });
    const group = makeGroup(
      'owner',
      queued('a', { num: 20 }),
      queued('b', { num: 21 }),
      queued('c', { num: 22, depends: ['other'] }),
      queued('d', { num: 23, state: 'active' }),
      { r: 'task', id: 'e', seq: 900, gone: true },
    );
    const tell = [
      'Its AIs stop and its open cards close.',
      'Changes not committed yet are lost; the branch stays.',
    ];
    expect(cancelNotes(makeGroup(), base)).toEqual(tell);
    expect(cancelNotes(group, base)).toEqual([
      ...tell,
      'Tasks that wait for it could then never start: T-20, T-21.',
    ]);
    expect(cancelNotes(makeGroup(), task({ step: 'merge' }))).toEqual([
      'It has passed review and may be merging right now.',
      ...tell,
    ]);
    const before = task({ state: 'queued', step: 'branch', branch: undefined });
    expect(cancelNotes(makeGroup(), before)).toEqual(['It will never start.']);
  });
});

describe('why a task stands still', () => {
  it('is said for every wait that no card carries', () => {
    const group = makeGroup();
    const paused = (kind: string) => task({ state: 'paused', wait: wait(kind) });
    expect(taskHold(group, paused('manual'))).toEqual({
      kind: 'manual',
      text: 'The secretary or an admin paused it. Its AIs stay stopped until one of them resumes it.',
      tone: undefined,
    });
    expect(taskHold(group, { ...paused('rounds'), thread: { count: 12, rounds: 10 } })).toEqual({
      kind: 'rounds',
      text: 'The AIs went 10 rounds without a person. Reply in its thread to carry on.',
      tone: 'dawn',
    });
    expect(taskHold(group, paused('preempted'))).toEqual({
      kind: 'preempted',
      text: 'A message jumped the queue. It carries on by itself afterwards.',
      tone: undefined,
    });
    expect(taskHold(group, task({ wait: wait('reviewer') }))).toEqual({
      kind: 'reviewer',
      text: 'Ready for review, and nobody is named to do it. The secretary or an admin names a reviewer.',
      tone: undefined,
    });
    expect(taskHold(group, task({ wait: wait('conflict') }))).toEqual({
      kind: 'conflict',
      text: 'The merge did not go through. The secretary or an admin hands it back to be fixed.',
      tone: undefined,
    });
    for (const kind of ['approval', 'decision', 'confirm'])
      expect(taskHold(group, task({ wait: wait(kind) }))).toBeUndefined();
    expect(taskHold(group, base)).toBeUndefined();
    expect(taskHold(group, task({ state: 'merged' }))).toBeUndefined();
  });

  it('is said of a stopped task by what its reader can do', () => {
    const stopped = task({ state: 'failed', wait: null });
    expect(taskHold(makeGroup(), stopped)).toEqual({
      kind: 'failed',
      text: 'Its AIs stay stopped until you resume it or give it to another AI. The count then starts again.',
      tone: 'danger',
    });
    expect(taskHold(makeGroup('member'), stopped)).toEqual({
      kind: 'failed',
      text: 'Its AIs stay stopped until an admin resumes it or gives it to another AI.',
      tone: 'danger',
    });
    expect(
      taskHold(makeGroup('member'), task({ state: 'paused', wait: wait('manual') }))!.kind,
    ).toBe('manual');
  });

  it('is said of a queued task by the tasks it waits for', () => {
    const group = makeGroup(
      'owner',
      row('row-task', { id: 'a', num: 3, state: 'merged' }),
      row('row-task', { id: 'b', num: 4, state: 'active' }),
      row('row-task', { id: 'c', num: 5, state: 'failed' }),
      row('row-task', { id: 'd', num: 6, state: 'cancelled' }),
    );
    const queued = (...depends: string[]) =>
      taskHold(group, task({ state: 'queued', depends, wait: wait('depends') }));
    expect(queued('a', 'b', 'c')).toEqual({
      kind: 'depends',
      text: 'Waits for T-4, T-5 to be merged.',
      tone: undefined,
    });
    expect(queued('b', 'd')!.text).toBe('Cannot start: a task it waits for was cancelled (T-6).');
    expect(queued('a')).toBeUndefined();
    expect(queued('unknown')).toBeUndefined();
    prefs.lang = 'zh-CN';
    expect(queued('b', 'c')!.text).toBe('等 T-4、T-5 合并以后开工。');
  });
});

describe('a role that an owner denied', () => {
  const text =
    'Mina said no to Qwen Code here. The secretary or an admin gives the role to another AI.';
  const card = (changes: object = {}, ai = qwen) =>
    row('row-msg-confirm-denied', {
      ...{ id: 'card', re: base.id, body: { what: 'direct', ai, to: ['mina-user'] } },
      ...changes,
    });
  const given = task({ assignee: qwen, brief: undefined });
  const said = (changes: Partial<TaskRow> = {}, ...cards: Row[]) =>
    taskHold(makeGroup('owner', ...(cards.length ? cards : [card()])), { ...given, ...changes });

  it('is said of its assignee at every step, and of its reviewer at the review', () => {
    for (const step of ['branch', 'local_check', 'review'])
      expect(said({ step })).toEqual({ kind: 'denied', text });
    const reviews = { assignee: codex, reviewer: qwen };
    expect(said({ ...reviews, step: 'review' })?.text).toBe(text);
    expect(said({ ...reviews, step: 'local_check' })).toBeUndefined();
    expect(said({ assignee: codex, step: 'review' })).toBeUndefined();
    prefs.lang = 'zh-CN';
    expect(said()!.text).toBe('Mina 没同意 Qwen Code 参与。由秘书或管理员换一个 AI。');
  });

  it('is of the request that the task came from', () => {
    expect(said({ brief: 'the-brief' })).toBeUndefined();
    expect(said({ brief: 'the-brief' }, card({ re: 'the-brief' }))!.text).toBe(text);
  });

  it('is not said of any other card or message, of another AI, or beside another reason', () => {
    for (const status of ['pending', 'confirmed', 'expired'])
      expect(said({}, card({ state: { status } }))).toBeUndefined();
    expect(said({}, card({ kind: 'approval' }))).toBeUndefined();
    expect(said({}, card({}, codex))).toBeUndefined();
    expect(said({}, card({}, 'gone'))).toBeUndefined();
    expect(said({ wait: wait('confirm') })).toBeUndefined();
    expect(said({ state: 'merged' })).toBeUndefined();
    expect(said({ state: 'failed', wait: null })!.kind).toBe('failed');
    expect(said({ wait: wait('reviewer') })!.kind).toBe('reviewer');
  });
});

describe('the terminals of a task', () => {
  const names = (changes: Partial<TaskRow>, group = makeGroup()) =>
    ownTerminals(group, task(changes)).map((ai) => ai.name);

  it("are of the reader's own assignee, and of their reviewer from the first review on", () => {
    expect(names({ step: 'review' })).toEqual(['Claude Code', 'Codex']);
    expect(names({ step: 'local_check' })).toEqual(['Claude Code', 'Codex']);
    expect(names({ step: 'local_check', review: undefined })).toEqual(['Codex']);
    expect(names({ step: 'review', review: undefined })).toEqual(['Claude Code', 'Codex']);
    expect(names({ step: 'local_check', reviewer: qwen })).toEqual(['Codex']);
    expect(names({ reviewer: qwen })).toEqual(['Codex']);
    expect(names({ assignee: qwen, reviewer: undefined })).toEqual([]);
    expect(names({ state: 'paused' })).toEqual(['Claude Code', 'Codex']);
    for (const state of ['queued', 'merged', 'cancelled']) expect(names({ state })).toEqual([]);
  });

  it('are not offered again while one is open', () => {
    const taken = row('row-member-ai', { id: codex, name: 'Codex', state: 'takeover', seq: 900 });
    expect(names({}, makeGroup('owner', taken))).toEqual(['Claude Code']);
  });
});

describe("a message in a task's thread", () => {
  it("can mention every member, the task's own AIs first", () => {
    const of = (changes: Partial<TaskRow>) =>
      threadMembers(makeGroup(), task(changes)).map((member) => member.name);
    expect(of({})).toEqual(['Codex', 'Claude Code', 'Alex', 'Mina', 'Qwen Code', 'Secretary']);
    expect(of({ reviewer: undefined })).toEqual([
      'Codex',
      'Alex',
      'Claude Code',
      'Mina',
      'Qwen Code',
      'Secretary',
    ]);
  });

  it('says what becomes of it when it does not simply reach the AIs it names', () => {
    const group = makeGroup();
    expect(replyNote(group, base, [codex])).toBeUndefined();
    expect(replyNote(group, base, ['mina', codex])).toBeUndefined();
    const silent = '@ an AI if one should answer. Otherwise this just stays in the thread.';
    expect(replyNote(group, base, [])).toBe(silent);
    expect(replyNote(group, base, ['mina'])).toBe(silent);
    expect(replyNote(group, base, ['gone'])).toBe(silent);
    const paused = task({ state: 'paused', wait: wait('manual') });
    expect(replyNote(group, paused, [])).toBe(silent);
    expect(replyNote(group, paused, [codex])).toBe(
      'T-12 is paused: its AIs get this once it resumes.',
    );
    expect(replyNote(group, task({ state: 'failed', wait: null }), [codex])).toBe(
      'T-12 is stopped: its AIs get this once it resumes.',
    );
    const rounds = task({ state: 'paused', wait: wait('rounds') });
    expect(replyNote(group, rounds, [])).toBe('Sending this carries the task on.');
    expect(replyNote(group, rounds, [codex])).toBe('Sending this carries the task on.');
  });
});
