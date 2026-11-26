// The scripts of the demo world, driven through a computer's connection as the client drives
// them: a confirmed brief becomes tasks that reach `merged`, a denied approval stays denied and the
// task goes on without it, a message let go first pauses the busy AI's task, and a saved world
// comes back as it was. Every row the world writes has the shape the client reads.
import * as v from 'valibot';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { newId } from '../../src/lib/groups.svelte';
import * as p from '../../src/lib/proto';
import { user } from '../home';
import { words } from './content';
import { Director } from './director';
import { devices, ids } from './ids';
import type { Line } from './node';
import type { Fields, Row } from './rows';
import { seed } from './seed';
import { World } from './world';

const g = ids.site;
let world: World;
let line: Line;

beforeEach(() => {
  vi.useFakeTimers();
  world = new World(user, 'full');
  seed(world);
  new Director(world);
  line = world.node(devices.macbook)!.open(() => {});
});

afterEach(() => {
  world.stop();
  vi.useRealTimers();
});

/** Sends a command of the person, signed as the client signs it, and waits for its answer. */
async function send(fields: Fields) {
  const payload = JSON.stringify({ n: newId(), ts: Date.now(), g, ...fields });
  const answer = line.request({ t: 'cmd', env: { p: payload, k: user, s: 'signature' } });
  await vi.advanceTimersByTimeAsync(10);
  return (await answer) as { ok?: true; err?: { code: string } };
}

const say = (text: string, mentions: string[] = []) =>
  send({ t: 'post', kind: 'text', thread: null, main: false, text, mentions, re: null, body: {} });

const rows = (kind: string) => world.rows(g).filter((row) => row.kind === kind) as Row[];
const status = (row: Row) => (row.state as { status?: string } | null)?.status;
const pending = (kind: string) => rows(kind).filter((row) => status(row) === 'pending');
const tasks = () => world.rows(g).filter((row) => row.r === 'task' && row.state !== 'merged');

/** Lets the scripts run, deciding each card that waits for the person as `decide` says, until
 *  every task of the group is merged or a minute has passed. */
async function run(decide: (card: Row) => Fields) {
  for (let i = 0; i < 60 && tasks().length; i += 1) {
    for (const card of [...pending('approval'), ...pending('decision')])
      await send({ t: 'act', msg: card.id, ...decide(card) });
    await vi.advanceTimersByTimeAsync(1000);
  }
  await vi.advanceTimersByTimeAsync(2000);
}

async function confirmedBrief() {
  await say('首页加一个导出价格表的按钮。');
  await vi.advanceTimersByTimeAsync(3000);
  const [brief] = pending('brief');
  expect(await send({ t: 'act', msg: brief.id, action: 'confirm' })).toMatchObject({ ok: true });
}

const approve = (card: Row) =>
  card.kind === 'approval' ? { action: 'approve' } : { action: 'choose', arg: 'B' };

describe('the demo world', () => {
  it('takes a confirmed brief to merged tasks, each with its summary', async () => {
    await confirmedBrief();
    await run(approve);
    const made = world.rows(g).filter((row) => row.r === 'task' && (row.num as number) > 1);
    expect(made.map((row) => row.state)).toEqual(['merged', 'merged']);
    const summaries = rows('summary').map((row) => (row.body as { task: string }).task);
    expect(summaries).toEqual(expect.arrayContaining(made.map((row) => row.id)));
    const failed = rows('checks').filter((row) => !(row.body as { ok: boolean }).ok);
    expect(failed).toHaveLength(1);
    for (const row of world.rows(g))
      expect(v.safeParse(p.Row, structuredClone(row)).issues).toBe(undefined);
  });

  it('shows a denied approval as denied, and the AI works on without it', async () => {
    await confirmedBrief();
    const deny = (card: Row) =>
      card.kind === 'approval' ? { action: 'deny' } : { action: 'choose', text: '都给' };
    await run(deny);
    expect(rows('approval').map(status)).toEqual(['denied']);
    expect(rows('progress').map((row) => row.text)).toContain(words.denied);
    expect(tasks()).toEqual([]);
  });

  it('pauses the task of a busy AI for a message let go first, then goes on', async () => {
    await confirmedBrief();
    await vi.advanceTimersByTimeAsync(9000);
    await send({ t: 'act', msg: pending('approval')[0].id, action: 'approve' });
    await vi.advanceTimersByTimeAsync(1000);
    await say('线上首页打不开了，先看这个。', [ids.claude]);
    const asked = rows('text').at(-1)!;
    expect(await send({ t: 'jump', msg: asked.id })).toMatchObject({ ok: true });
    const paused = world.rows(g).find((row) => row.r === 'task' && row.state === 'paused');
    expect((paused?.wait as { kind: string }).kind).toBe('preempted');
    await vi.advanceTimersByTimeAsync(6000);
    expect(rows('text').at(-1)!.text).toContain(words.preemptedAnswer);
    expect(world.rows(g).some((row) => row.r === 'task' && row.state === 'paused')).toBe(false);
  });

  it('comes back from what it saved as it was', async () => {
    await confirmedBrief();
    const saved = JSON.parse(JSON.stringify(world.saved()));
    const again = new World(user, 'full');
    seed(again);
    again.restore(saved);
    expect(again.rows(g)).toEqual(world.rows(g));
    expect(again.memory.flags).toEqual(world.memory.flags);
    again.stop();
  });
});
