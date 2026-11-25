import { beforeEach, describe, expect, it } from 'vitest';
import { Group, Home } from './groups.svelte';
import { goldens } from './mock';
import { prefs } from './prefs.svelte';
import * as p from './proto';
import { systemText, taskTag } from './system';

const g = 'rZNvy-1jH6Z-BcPqA5U5BQ';
const task: string = JSON.parse(goldens['row-task']).id;
const codex: string = JSON.parse(goldens['row-member-ai']).id;
const ts = new Date(2026, 10, 22, 9, 58).getTime();

let group: Group;

const row = (name: string, changes: object = {}) =>
  ({ ...p.decode(p.Row, goldens[name]), g, ...changes }) as p.Row;
const said = (body: object) =>
  systemText(group, row('row-msg-system', { ts, body }) as Extract<p.MsgRow, { kind: 'system' }>);

beforeEach(() => {
  prefs.lang = 'en';
  const home = new Home({ user: 'u', sign: () => new Promise(() => {}) });
  group = new Group(home, g);
  group.apply([
    row('row-task', { num: 12 }),
    row('row-member-ai', { name: "Codex $& $'" }),
    row('row-member-human', { id: 'mina', name: "Mina $& $'" }),
  ]);
});

describe('systemText', () => {
  it.each([
    [
      { code: 'task_failed', task, reason: 'failures' },
      'Task T-12 stopped after repeated failures.',
    ],
    [
      { code: 'task_failed', task, reason: 'review_rounds' },
      'Task T-12 stopped after repeated failed reviews.',
    ],
    [
      { code: 'task_failed', task: 'DrtCn6htSBw', reason: 'failures' },
      'Task DrtCn6h stopped after repeated failures.',
    ],
    [
      { code: 'turn_interrupted', ai: codex, reason: 'crash', task, detail: 'signal 9' },
      "Codex $& $''s turn crashed. · T-12 · signal 9",
    ],
    [{ code: 'turn_interrupted', ai: codex, reason: 'timeout' }, "Codex $& $''s turn timed out."],
    [
      { code: 'turn_interrupted', ai: codex, reason: 'stop', task },
      "Codex $& $''s turn was stopped. · T-12",
    ],
    [
      { code: 'login_invalid', ai: codex, detail: 'expired token' },
      "Codex $& $''s sign-in is no longer valid. · expired token",
    ],
    [{ code: 'login_invalid', ai: 'gone' }, "Former member's sign-in is no longer valid."],
    [
      { code: 'session_replaced', ai: codex, task },
      "Codex $& $' started a new session from the task notes. · T-12",
    ],
    [
      { code: 'session_replaced', ai: codex },
      "Codex $& $' started a new session from the task notes.",
    ],
    [
      { code: 'rounds_limit', task, n: 10 },
      'T-12 is paused: the AIs went 10 rounds without a person. Reply in its thread to carry on.',
    ],
    [
      { code: 'preempted', task, by: 'mina' },
      "Mina $& $' jumped the queue. T-12 is paused and carries on afterwards.",
    ],
    [
      { code: 'rate_limited', ai: codex, resets: new Date(2026, 10, 22, 15).getTime() },
      "Codex $& $' is out of quota until 15:00, then carries on.",
    ],
    [
      { code: 'rate_limited', ai: codex, resets: new Date(2026, 10, 23, 3).getTime() },
      "Codex $& $' is out of quota until Nov 23, 03:00, then carries on.",
    ],
    [
      { code: 'group_paused', by: 'mina' },
      "Mina $& $' paused the group. All its AIs stop for now.",
    ],
    [{ code: 'group_resumed', by: 'mina' }, "Mina $& $' resumed the group. Its AIs carry on."],
    [
      { code: 'member_joined', member: 'mina', by: codex },
      "Mina $& $' joined, invited by Codex $& $'.",
    ],
    [
      { code: 'member_removed', user: 'u', name: "$& $'", by: 'mina' },
      "Mina $& $' removed $& $' from the group.",
    ],
    [{ code: 'member_removed', user: 'u', name: "$& $'" }, "$& $' left the group."],
    [{ code: 'no_secretary' }, 'This group has no secretary AI.'],
    [{ code: 'cap_reached', ai: codex, cap: 'day' }, 'cap_reached'],
  ])('words %j', (body, text) => expect(said(body)).toBe(text));

  it('is in the language of the reader', () => {
    prefs.lang = 'zh-CN';
    expect(said({ code: 'task_failed', task, reason: 'failures' })).toBe(
      '任务 T-12 连续失败，已停下。',
    );
  });
});

describe('taskTag', () => {
  it('is the number of a task that is held, and else the start of its id', () => {
    expect(taskTag(group, task)).toBe('T-12');
    expect(taskTag(group, 'DrtCn6htSBw')).toBe('DrtCn6h');
  });
});
