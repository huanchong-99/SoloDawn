// The scripts of the demo world, driven through a computer's connection as the client drives
// them: a group made with a person and connected AIs, each in a folder of its computer; a
// confirmed brief whose tasks go through work, report and review to done, one task at a time per
// folder; a denied approval that stays denied while the task goes on without it; a message let go
// first that pauses the busy AI's task; who paused or reassigned a task; the project memory and
// knowledge base, written, edited, searched and paged; and a saved world that comes back as it was. Every row the world writes has the shape the client reads.
import * as v from 'valibot';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { newId } from '../../src/lib/groups.svelte';
import * as p from '../../src/lib/proto';
import { user } from '../home';
import { words } from './content';
import { Director } from './director';
import { devices, ids, users } from './ids';
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
const tasks = () => world.rows(g).filter((row) => row.r === 'task' && row.state !== 'done');
const made = () => world.rows(g).filter((row) => row.r === 'task' && (row.num as number) > 1);
const parses = () => {
  for (const each of world.groups())
    for (const row of world.rows(each))
      expect(v.safeParse(p.Row, structuredClone(row)).issues).toBe(undefined);
};

/** Lets the scripts run, deciding each card that waits for the person as `decide` says, until
 *  every task of the group is done or a minute has passed. */
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
  it('takes a confirmed brief through work, report and review to done, one task per folder', async () => {
    await confirmedBrief();
    const [first, second, third] = made();
    expect(made().map((row) => row.state)).toEqual(['active', 'active', 'queued']);
    expect(third.wait).toMatchObject({ kind: 'busy' });
    expect([first.assignee, second.assignee, third.assignee]).toEqual([
      ids.claude,
      ids.codex,
      ids.claude,
    ]);
    await run(approve);
    expect(made().map((row) => row.state)).toEqual(['done', 'done', 'done']);
    const ended = (row: Row) => (row.stats as { started: number; ended: number }).ended;
    const started = (row: Row) => (row.stats as { started: number }).started;
    expect(started(world.find<Row>(g, 'task', third.id)!)).toBeGreaterThanOrEqual(
      ended(world.find<Row>(g, 'task', first.id)!),
    );
    const summaries = rows('summary').map((row) => (row.body as { task: string }).task);
    expect(summaries).toEqual(expect.arrayContaining(made().map((row) => row.id)));
    const reviews = rows('review').map((row) => row.body as { ok: boolean; report: string });
    expect(reviews.filter((review) => !review.ok)).toHaveLength(1);
    const reports = rows('report').map((row) => row.id);
    expect(reports).toHaveLength(made().length + 2);
    for (const review of reviews) expect(reports).toContain(review.report);
    for (const row of made()) expect(reports).toContain(row.report);
    const done = rows('system').filter(
      (row) => (row.body as { code: string }).code === 'task_done',
    );
    expect(done).toHaveLength(made().length + 1);
    parses();
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

  it('says who paused a task and who gave it to another AI; a busy folder takes none', async () => {
    await confirmedBrief();
    const [first] = made();
    await vi.advanceTimersByTimeAsync(9000);
    const [approval] = pending('approval');
    expect(await send({ t: 'task', op: 'pause', id: first.id })).toMatchObject({ ok: true });
    expect(world.find<Row>(g, 'task', first.id)!.wait).toMatchObject({
      kind: 'manual',
      by: ids.me,
    });
    expect(await send({ t: 'task', op: 'resume', id: first.id })).toMatchObject({ ok: true });
    const codex = { t: 'task', op: 'assign', id: first.id, assignee: ids.codex };
    expect(await send(codex)).toMatchObject({ err: { code: 'conflict' } });
    const mina = { ...codex, assignee: ids.minaAi };
    expect(await send(mina)).toMatchObject({ ok: true });
    expect(world.find<Row>(g, 'msg', approval.id)!.state).toMatchObject({
      status: 'reassigned',
      by: ids.me,
    });
    parses();
  });

  it('makes a group with a person and a connected AI in a folder of its computer', async () => {
    const docs = newId();
    const create = { t: 'group', op: 'create', g: docs, name: '博客', member_name: 'Alex' };
    expect(await send(create)).toMatchObject({ ok: true });
    const add = (user: string, name: string) =>
      send({ t: 'member', op: 'add_person', g: docs, user, name });
    expect(await add(users.mina, 'Mina')).toMatchObject({ ok: true });
    expect(await add(users.mina, 'Mina 2')).toMatchObject({ err: { code: 'conflict' } });
    expect(await add(users.leo, 'Leo')).toMatchObject({ err: { code: 'not_found' } });
    await vi.advanceTimersByTimeAsync(3000);
    expect(world.rows(docs).map((row) => [row.r, row.kind])).toEqual([
      ['group', undefined],
      ['member', 'human'],
      ['member', 'human'],
      ['msg', 'system'],
      ['msg', 'text'],
    ]);

    const listed = (await line.request({ t: 'host.workspaces', agent: 'claude' })) as {
      workspaces: { path: string }[];
    };
    expect(listed.workspaces.map((folder) => folder.path)).toEqual([
      '/Users/alex/dev/solodawn-site',
      '/Users/alex/dev/blog',
      '/Users/alex/dev/design-system',
      '/Users/alex/notes',
    ]);
    const none = await line.request({ t: 'host.workspaces', agent: 'external' });
    expect(none).toMatchObject({ ok: true, workspaces: [] });
    const unknown = await line.request({ t: 'host.workspaces', agent: 'gemini' });
    expect(unknown).toMatchObject({ err: { code: 'bad_request' } });

    const join = (workspace: string) =>
      send({
        t: 'host.join',
        to: devices.macbook,
        g: docs,
        home: devices.macbook,
        agent: 'claude',
        name: 'Claude Code',
        secretary: true,
        workspace,
      });
    expect(await join('dev/blog')).toMatchObject({ err: { code: 'bad_request' } });
    expect(await join(undefined as never)).toMatchObject({ err: { code: 'bad_request' } });
    expect(await join('/tmp/blog')).toMatchObject({ err: { code: 'not_found' } });
    expect(await join('/Users/alex/dev/solodawn-site/')).toMatchObject({
      err: {
        code: 'conflict',
        msg: 'the folder is the workspace of an AI of the group "官网改版"',
      },
    });
    expect(await join('/Users/alex/dev/blog/')).toMatchObject({ ok: true });
    const ai = world.rows(docs).find((row) => row.kind === 'ai')!;
    expect(ai.workspace).toBe('/Users/alex/dev/blog');
    const info = world.info(world.node(devices.macbook)!);
    const entry = info.agents.flatMap((agent) => agent.groups).find((each) => each.g === docs);
    expect(entry).toMatchObject({ member: ai.id, workspace: '/Users/alex/dev/blog' });
    parses();
  });

  it('writes, edits, searches and pages the project memory and the knowledge base', async () => {
    const notes = (type?: string) =>
      world.rows(g).filter((row) => row.r === 'note' && row.type === type) as Row[];
    const fetch = (fields: Fields) =>
      line.request({ t: 'fetch', g, before: 0, limit: 50, ...fields } as never) as Promise<{
        rows: Row[];
        more: boolean;
      }>;
    expect(notes().map((row) => row.text)).toHaveLength(3);
    const [decision, convention] = notes('kb');
    expect((await fetch({ kind: 'kb' })).rows.map((row) => row.id)).toEqual([
      convention.id,
      decision.id,
    ]);
    const page = await fetch({ kind: 'note', limit: 2 });
    expect([page.rows.length, page.more]).toEqual([2, true]);
    const older = await fetch({ kind: 'note', before: page.rows[1].pos });
    expect([older.rows.length, older.more]).toEqual([1, false]);
    expect((await fetch({ kind: 'review' })).rows.map((row) => row.kind)).toEqual(['review']);
    expect((await fetch({ kind: 'kb', q: '静态  CDN' })).rows).toEqual([decision]);

    const note = notes()[0];
    const edit = { t: 'note_edit', id: note.id, text: '断点用 `640px`。' };
    expect(await send(edit)).toMatchObject({ ok: true });
    expect(world.find(g, 'note', note.id)).toMatchObject({ pos: note.pos, text: edit.text });
    const rename = { t: 'note_edit', id: decision.id, text: 'x', name: convention.name };
    expect(await send(rename)).toMatchObject({ err: { code: 'conflict' } });
    expect(await send({ ...rename, name: '决策：静态页' })).toMatchObject({ ok: true });
    expect(world.find(g, 'note', decision.id)).toMatchObject({
      name: '决策：静态页',
      pos: decision.pos,
    });

    const put = { t: 'note', id: newId(), task: null, type: 'kb', name: '决策：静态页', text: 'y' };
    expect(await send(put)).toMatchObject({ ok: true });
    expect(notes('kb').map((row) => [row.id, row.text])).toEqual([
      [decision.id, 'y'],
      [convention.id, convention.text],
    ]);
    expect(await send({ t: 'forget', r: 'note', id: decision.id })).toMatchObject({ ok: true });
    expect(await send({ ...edit, id: decision.id })).toMatchObject({ err: { code: 'not_found' } });
    parses();
  });
});
