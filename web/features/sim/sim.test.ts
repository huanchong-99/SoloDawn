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
import { Events } from './events';
import { devices, ids, users } from './ids';
import type { Line } from './node';
import { task as taskRow, type Fields, type Row } from './rows';
import { seed } from './seed';
import { mac, room } from '../../src/lib/pairing';
import type { Heard, Listener } from '../../src/lib/peer';
import { endCode } from './people';
import { fakeConnect } from './session';
import { World } from './world';

const g = ids.site;
let world: World;
let director: Director;
let line: Line;

beforeEach(() => {
  vi.useFakeTimers();
  world = new World(user, 'full');
  seed(world);
  director = new Director(world);
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

/** The secretary asked the assignee of a task, in the task's thread, and the assignee answered
 *  there from its own folder. */
function closedIn(row: Row) {
  const thread = rows('text').filter((each) => each.thread === row.id);
  expect(thread.map((each) => [each.by, each.text])).toEqual([
    [ids.secretary, words.closeAsk],
    [row.assignee, words.closed],
  ]);
  expect(thread[0].mentions).toEqual([row.assignee]);
}

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
    await vi.advanceTimersByTimeAsync(3000);
    const notices = rows('text').filter((row) => row.text === words.allDone);
    expect(notices.map((row) => row.by)).toEqual([ids.secretary]);
    for (const each of made()) closedIn(each);
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

  it('asks the assignee of a task with no brief to close it, too', async () => {
    const spec = { g, num: 9, title: '改一处文案', assignee: ids.claude, reviewer: ids.codex };
    const lone = world.write(g, taskRow(spec)) as Row;
    director.tasks.kick(g, lone.id as string);
    await run(approve);
    expect(world.find<Row>(g, 'task', lone.id as string)!.state).toBe('done');
    await vi.advanceTimersByTimeAsync(3000);
    closedIn(lone);
    expect(rows('text').map((row) => row.text)).not.toContain(words.allDone);
  });

  it('titles the brief of a request that runs on straight after @秘书', async () => {
    await say('@秘书首页加一个导出价格表的按钮。', [ids.secretary]);
    await vi.advanceTimersByTimeAsync(3000);
    const [brief] = pending('brief');
    expect(brief.body).toMatchObject({
      title: '首页加一个导出价格表的按钮',
      goal: '首页加一个导出价格表的按钮。',
    });
    await say('@秘书怎么部署的');
    await vi.advanceTimersByTimeAsync(3000);
    expect(rows('text').at(-1)!.text).toBe(words.answer);
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

  it('adds Nora, who is in the blog group on the same computer, without an invitation', async () => {
    const people = (group: string) =>
      world.rows(group).filter((row) => row.r === 'member' && row.kind === 'human');
    const offered = people(ids.blog).filter(
      (row) => row.user !== world.me && !people(g).some((each) => each.user === row.user),
    );
    expect(offered.map((row) => row.name)).toEqual(['Nora']);
    expect(world.home(ids.blog)).toBe(world.home(g));
    const add = { t: 'member', op: 'add_person', user: users.nora, name: 'Nora' };
    expect(await send(add)).toMatchObject({ ok: true });
    expect(await send(add)).toMatchObject({ err: { code: 'conflict' } });
    expect(people(g).map((row) => row.name)).toContain('Nora');
    const nora = people(g).find((row) => row.user === users.nora)!;
    await vi.advanceTimersByTimeAsync(3000);
    expect(rows('text').at(-1)).toMatchObject({ by: nora.id, text: words.addedHello });
    expect(await say('@Nora 欢迎', [nora.id as string])).toMatchObject({ ok: true });
    expect(rows('text').at(-1)!.mentions).toEqual([nora.id]);
    parses();
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

  it('does not ask a question again that was answered when a task goes to another AI', async () => {
    await confirmedBrief();
    const second = made()[1];
    for (let i = 0; i < 20 && !pending('decision').length; i += 1)
      await vi.advanceTimersByTimeAsync(1000);
    await send({ t: 'act', msg: pending('decision')[0].id, action: 'choose', arg: 'B' });
    expect(await send({ t: 'task', op: 'pause', id: second.id })).toMatchObject({ ok: true });
    const mina = { t: 'task', op: 'assign', id: second.id, assignee: ids.minaAi };
    expect(await send(mina)).toMatchObject({ ok: true });
    expect(await send({ t: 'task', op: 'resume', id: second.id })).toMatchObject({ ok: true });
    await run(approve);
    expect(world.find<Row>(g, 'task', second.id)!.state).toBe('done');
    expect(rows('decision')).toHaveLength(1);
  });

  it('keeps the AIs on their computers while a group moves, and moves the group', async () => {
    const office = world.node(devices.office)!;
    const mac = world.node(devices.macbook)!;
    const aisOf = (device: string) =>
      world
        .info(world.node(device)!)
        .agents.flatMap((agent) => agent.groups)
        .filter((entry) => entry.g === g).length;
    const before = [aisOf(devices.macbook), aisOf(devices.office)];
    office.paired = true;
    mac.paired = true;
    new Events(world, director).move(g);
    expect(mac.listed().map((each) => each.g)).not.toContain(g);
    expect([aisOf(devices.macbook), aisOf(devices.office)]).toEqual(before);
    await vi.advanceTimersByTimeAsync(2500);
    expect(office.listed().map((each) => each.g)).toContain(g);
    expect(mac.groups.has(g)).toBe(false);
    expect([aisOf(devices.macbook), aisOf(devices.office)]).toEqual(before);
    expect(world.info(mac).homes.find((home) => home.g === g)?.home).toBe(devices.office);
  });

  it('still starts a takeover on an AI of the computer after its group moved', async () => {
    for (const node of world.nodes.values()) node.paired = true;
    new Events(world, director).move(g);
    await vi.advanceTimersByTimeAsync(2500);
    const home = { t: 'post', kind: 'text', thread: null, main: false, text: 'x', mentions: [] };
    expect(await send(home)).toMatchObject({ err: { code: 'moved' } });
    const start = { t: 'takeover.start', to: devices.macbook, member: ids.claude };
    expect(await send(start)).toMatchObject({ ok: true });
    expect(world.find<Row>(g, 'member', ids.claude)).toMatchObject({ state: 'takeover' });
  });

  it('imports a group that was exported when the world was saved', () => {
    for (const node of world.nodes.values()) node.paired = true;
    new Events(world, director).move(g);
    const again = new World(user, 'full');
    seed(again);
    again.restore(JSON.parse(JSON.stringify(world.saved())));
    expect(again.home(g)?.device).toBe(devices.office);
    expect(again.find<Row>(g, 'group', g)).toMatchObject({ home: devices.office });
    again.stop();
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

describe('the invitation code of the demo world', () => {
  const hour = 3_600_000;
  const code = (fields: Fields) => send({ t: 'code', ...fields });
  const members = () => world.rows(g).filter((row) => row.r === 'member' && row.kind === 'human');
  const nora = () => members().find((row) => row.user === users.nora);

  it('makes, shows, replaces and turns off a code as a home does, and keeps it out of every row', async () => {
    for (const hours of [0, 169, 1.5, undefined])
      expect(await code({ op: 'new', hours })).toMatchObject({ err: { code: 'bad_request' } });
    expect(await code({ op: 'mend' })).toMatchObject({ err: { code: 'bad_request' } });
    expect(await code({ op: 'show' })).toEqual({ re: 0, ok: true });
    const before = Date.now();
    const made = (await code({ op: 'new', hours: 168 })) as { code: string; expires: number };
    expect(made.code).toMatch(/^[0-9A-HJKMNP-TV-Z]{4}(-[0-9A-HJKMNP-TV-Z]{4}){3}$/);
    expect(made.expires).toBe(before + 168 * hour);
    expect(await code({ op: 'show' })).toMatchObject({ ok: true, ...made });
    const rows = JSON.stringify(world.groups().map((each) => world.rows(each)));
    expect(rows).not.toContain(made.code.replaceAll('-', ''));
    expect(rows).not.toContain(made.code);
    const again = (await code({ op: 'new', hours: 1 })) as { code: string };
    expect(again.code).not.toBe(made.code);
    expect(await code({ op: 'show' })).toMatchObject({ code: again.code });
    expect(await code({ op: 'off' })).toEqual({ re: 0, ok: true });
    expect(await code({ op: 'show' })).toEqual({ re: 0, ok: true });
    const me = members().find((row) => row.user === world.me)!;
    world.write(g, { ...me, role: 'member' });
    expect(await code({ op: 'show' })).toMatchObject({ err: { code: 'forbidden' } });
  });

  it('lets Nora join by the code a moment later, joined by the admin who made it, not once it ended', async () => {
    await code({ op: 'new', hours: 24 });
    await vi.advanceTimersByTimeAsync(7900);
    expect(nora()).toBeUndefined();
    await vi.advanceTimersByTimeAsync(100);
    const joined = rows('system').at(-1)!;
    expect(joined.body).toEqual({ code: 'member_joined', member: nora()!.id, by: ids.me });
    expect(nora()!.role).toBe('member');
    await vi.advanceTimersByTimeAsync(2500);
    expect(rows('text').at(-1)).toMatchObject({ by: nora()!.id, text: words.addedHello });
    parses();
    expect(await send({ t: 'member', op: 'remove', id: nora()!.id })).toMatchObject({ ok: true });
    expect(await code({ op: 'show' })).toEqual({ re: 0, ok: true });
    await code({ op: 'new', hours: 24 });
    await code({ op: 'off' });
    await vi.advanceTimersByTimeAsync(9000);
    expect(nora()).toBeUndefined();
  });

  it('says nothing for a person removed before their hello', async () => {
    const said = (by: unknown) => rows('text').filter((row) => row.by === by);
    await code({ op: 'new', hours: 24 });
    await vi.advanceTimersByTimeAsync(8000);
    const id = nora()!.id;
    expect(await send({ t: 'member', op: 'remove', id })).toMatchObject({ ok: true });
    await vi.advanceTimersByTimeAsync(3000);
    expect(said(id)).toEqual([]);
    const add = { t: 'member', op: 'add_person', user: users.nora, name: 'Nora' };
    expect(await send(add)).toMatchObject({ ok: true });
    const added = nora()!.id;
    expect(await send({ t: 'member', op: 'remove', id: added })).toMatchObject({ ok: true });
    await vi.advanceTimersByTimeAsync(3000);
    expect(said(added)).toEqual([]);
  });

  it('ends a code at its end, when the group moves and from the panel, and lets members in', async () => {
    await code({ op: 'new', hours: 1 });
    await vi.advanceTimersByTimeAsync(hour - 1000);
    expect(world.code(g)).toBeDefined();
    await vi.advanceTimersByTimeAsync(1000);
    expect(await code({ op: 'show' })).toEqual({ re: 0, ok: true });
    await code({ op: 'new', hours: 24 });
    endCode(world, g);
    expect(world.code(g)).toBeUndefined();
    expect(await send({ t: 'join', name: 'Alex' })).toMatchObject({ ok: true });
    await code({ op: 'new', hours: 24 });
    // A person who leaves by themself does not end it: they gave nobody else's code away.
    const me = members().find((row) => row.user === world.me)!;
    world.write(g, { ...me, role: 'admin' });
    expect(await send({ t: 'member', op: 'remove', id: me.id })).toMatchObject({ ok: true });
    expect(world.code(g)).toBeDefined();
    world.arrive(g, world.node(devices.office)!);
    expect(world.memory.codes[g]).toBeUndefined();
  });

  it('answers a typed code in its room from its home, which names the group, and gives up a room nobody answers', async () => {
    const made = (await code({ op: 'new', hours: 24 })) as { code: string };
    const secret = made.code.replaceAll('-', '');
    const url = 'wss://rv.example/rv';
    const session = fakeConnect(world)({
      urls: [url],
      device: {} as never,
      known: () => true,
      name: 'Browser',
    });
    expect(session.roomWait).toBe(20_000);
    world.pace = 0.5;
    expect(session.roomWait).toBe(10_000);
    world.pace = 1;
    const fields = { role: 'b', k: user, kind: 'browser', name: 'Browser' };
    /** Joins the room of a typed text and sends the browser's pair message there: what is heard. */
    const enter = async (text: string) => {
      const heard: Heard[] = [];
      const id = await room(crypto, text.replaceAll('-', ''));
      session.watchRoom(url, id, (_, frame) => {
        if ('room' in frame && frame.room === id) heard.push(frame);
      });
      await vi.advanceTimersByTimeAsync(100);
      expect(heard).toEqual([{ t: 'joined', room: id }]);
      const self = { t: 'pair' as const, ...fields, mac: await mac(crypto, text, fields) };
      session.send(url, { t: 'room', room: id, data: self });
      return heard;
    };
    const wrong = await enter('0000-0000-0000-0000');
    const right = await enter(secret);
    await vi.waitFor(() => expect(right).toHaveLength(2));
    expect(right[1]).toMatchObject({
      t: 'room',
      data: { role: 'a', k: devices.macbook, kind: 'node' },
    });
    expect(wrong).toHaveLength(1);
    const welcome = Promise.withResolvers<p.Welcome>();
    const listener = {
      online: (_: unknown, w: p.Welcome) => welcome.resolve(w),
      push() {},
      offline: vi.fn(),
    };
    const onpath = vi.fn();
    session.peer(devices.macbook, () => listener as unknown as Listener, onpath);
    await vi.advanceTimersByTimeAsync(300);
    expect((await welcome.promise).join).toBe(g);
    // The path is told again, for a join that follows it only once it was admitted.
    expect(onpath).toHaveBeenCalledOnce();
    await vi.advanceTimersByTimeAsync(1000);
    expect(onpath.mock.calls).toEqual([
      [expect.objectContaining({ kind: 'direct' })],
      onpath.mock.calls[0],
    ]);
    const later = Promise.withResolvers<p.Welcome>();
    session.peer(
      devices.macbook,
      () =>
        ({
          ...listener,
          online: (_: unknown, w: p.Welcome) => later.resolve(w),
        }) as unknown as Listener,
    );
    // The peer it replaced reports its connection lost, as peer.ts does.
    expect(listener.offline).toHaveBeenCalledOnce();
    expect(onpath.mock.lastCall).toEqual([undefined]);
    await vi.advanceTimersByTimeAsync(300);
    expect((await later.promise).join).toBeUndefined();
    session.close();
  });
});
