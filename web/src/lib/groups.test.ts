import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { find, Group, Home, homes, lost, newId, recent, subject } from './groups.svelte';
import { goldens } from './mock';
import * as p from './proto';

const user = 'akWV_5PtNLIGhakuMAvpD9LcG1D8TUfc1URh48VREcs';
const website = 'rZNvy-1jH6Z-BcPqA5U5BQ';
const task = 'DrtCn6htSBwmMPrFPbHJHA';
const codex = '1PC8WineBrUQ-apCjx7tug';

/** A wire example as a row, with some fields changed. */
const row = (name: string, changes: object = {}) =>
  ({ ...p.decode(p.Row, goldens[name]), ...changes }) as p.Row;
const text = (id: string, seq: number, changes: object = {}) =>
  row('row-msg-text', { id, seq, pos: seq, ...changes }) as p.MsgRow;
const gone = (r: string, id: string, seq: number): p.GoneRow => ({ r, id, seq, gone: true });
const rows = (...of: p.Row[]): p.Push => ({ t: 'rows', g: website, rows: of });
const reset: p.Push = { t: 'reset', g: website };

interface Asked {
  req: Record<string, unknown>;
  answer(frame: object): void;
  fail(): void;
}

/** A connection whose requests the test answers by hand, in its own time. */
function connection() {
  const asked: Asked[] = [];
  const ctl = {
    request: (req: Record<string, unknown>) =>
      new Promise((answer, fail) => asked.push({ req, answer, fail: () => fail(new Error()) })),
  } as p.Ctl;
  return { ctl, asked };
}

/** Lets what waits on an answer run. */
const settle = () => new Promise((done) => setTimeout(done));

const signed: string[] = [];
const me = {
  user,
  sign: async (payload: string) => {
    signed.push(payload);
    return { p: payload, k: 'device', s: 'signature' };
  },
};
const welcome = (groups: p.Welcome['groups'], changes: object = {}): p.Welcome => ({
  ...p.decode(p.Welcome, goldens.welcome),
  groups,
  ...changes,
});
const listed = (g: string, name = 'Website') => ({ g, name, seq: 0 });
const synced = (seq: number) => ({ t: 'synced', re: 1, ok: true, g: website, seq });
const fetched = (page: p.Row[], more: boolean) => ({
  t: 'fetched',
  re: 2,
  ok: true,
  rows: page,
  more,
});
const sync = (since: number, g = website) => ({ t: 'sync', g, since, state_only: false });
const fetch = (before: number, thread: string | null = null) => ({
  t: 'fetch',
  g: website,
  thread,
  before,
  limit: 50,
});

/** A new connection of a home, on which the node has answered `hello`. */
function connect(home: Home, groups = [listed(website)], changes: object = {}) {
  const { ctl, asked } = connection();
  const link = home.connect();
  link.online(ctl, welcome(groups, changes));
  return { ctl, asked, link };
}

/** A home whose one group has been listed, and the request of its first read. */
function joined() {
  const home = new Home(me);
  return { home, ...connect(home), group: home.groups[0] };
}

/** The same, read to the end at sequence number S. */
async function read(seq = 815) {
  const all = joined();
  all.asked[0].answer(synced(seq));
  await settle();
  return all;
}

// Before any test adds one.
const homesAtFirst = [...homes];

beforeEach(() => {
  signed.length = 0;
  homes.length = 0;
});
afterEach(() => vi.useRealTimers());

describe('rows', () => {
  it('are held by type and id', () => {
    const { group } = joined();
    const member = row('row-member-human', { id: 'same' });
    const note = row('row-note', { id: 'same' });
    group.apply([member, note]);
    expect([...group.rows]).toEqual([
      ['member same', member],
      ['note same', note],
    ]);
  });

  it('replace the copy held only with a higher seq', () => {
    const { group } = joined();
    const first = text('a', 5);
    const equal = text('a', 5, { text: 'equal' });
    const lower = text('a', 4, { text: 'lower' });
    const higher = text('a', 6, { text: 'higher' });
    group.apply([first]);
    group.apply([equal]);
    expect(group.rows.get('msg a')).toBe(first);
    group.apply([lower]);
    expect(group.rows.get('msg a')).toBe(first);
    group.apply([higher]);
    expect(group.rows.get('msg a')).toBe(higher);
  });

  it('keep a deletion marker, unseen, so that an older copy cannot bring the row back', async () => {
    const { group, asked, link } = await read();
    const message = text('a', 700, { thread: task });
    const member = row('row-member-ai', { seq: 700 });
    group.apply([message, member]);
    expect(group.main).toEqual([]);
    expect(group.thread(task)).toEqual([message]);
    expect(group.members).toEqual([member]);

    link.push(rows(gone('msg', 'a', 816), gone('member', member.id, 817)));
    expect(group.rows.get('msg a')).toEqual(gone('msg', 'a', 816));
    expect(group.thread(task)).toEqual([]);
    expect(group.members).toEqual([]);
    expect(group.member(member.id)).toBeUndefined();

    // A page asked for before the deletion arrives after it.
    void group.older(task);
    asked[1].answer(fetched([message], false));
    await settle();
    expect(group.rows.get('msg a')).toEqual(gone('msg', 'a', 816));
    expect(group.thread(task)).toEqual([]);
  });

  it('of a marker for a row never seen are kept too', () => {
    const { group } = joined();
    group.apply([gone('msg', 'a', 9)]);
    group.apply([text('a', 8)]);
    expect(group.main).toEqual([]);
  });
});

describe('the cursor', () => {
  it('is 0 at the start and asks for everything', () => {
    const { group, asked } = joined();
    expect(group.cursor).toBe(0);
    expect(asked.map((ask) => ask.req)).toEqual([sync(0)]);
  });

  it('is not moved by rows that arrive before synced, and becomes S at synced', async () => {
    const { group, asked, link } = joined();
    link.push(reset);
    link.push(rows(text('a', 700), text('b', 710)));
    expect(group.main.map((msg) => msg.id)).toEqual(['a', 'b']);
    expect(group.cursor).toBe(0);
    asked[0].answer(synced(815));
    await settle();
    expect(group.cursor).toBe(815);
  });

  it('then follows each live row applied, and no row that changes nothing', async () => {
    const { group, link } = await read();
    link.push(rows(text('a', 816), text('b', 817)));
    expect(group.cursor).toBe(817);
    link.push(rows(text('a', 816)));
    expect(group.cursor).toBe(817);
    link.push(rows(text('b', 800)));
    expect(group.cursor).toBe(817);
    link.push(rows(gone('msg', 'a', 818)));
    expect(group.cursor).toBe(818);
  });

  it('is never moved by a page', async () => {
    const { group, asked } = await read();
    void group.older();
    asked[1].answer(fetched([text('z', 900)], false));
    await settle();
    expect(group.main.map((msg) => msg.id)).toEqual(['z']);
    expect(group.cursor).toBe(815);
  });

  it('returns to 0 with the cache at a reset, also when the read breaks off after its first batch', async () => {
    const { home, group, link } = await read();
    link.push(rows(text('a', 816)));
    link.offline();

    const again = connect(home);
    expect(again.asked[0].req).toEqual(sync(816));
    again.link.push(reset);
    expect(group.rows.size).toBe(0);
    expect(group.cursor).toBe(0);
    again.link.push(rows(text('n', 2400), text('o', 2401)));
    // The connection is lost here: no synced.
    again.link.offline();
    expect(group.cursor).toBe(0);

    const third = connect(home);
    expect(third.asked[0].req).toEqual(sync(0));
  });

  it('keeps its place when a delta breaks off', async () => {
    const { home, group, link } = await read();
    link.offline();
    const again = connect(home);
    again.link.push(rows(text('a', 816), text('b', 817)));
    again.link.offline();
    expect(group.main.map((msg) => msg.id)).toEqual(['a', 'b']);
    expect(group.cursor).toBe(815);
  });

  it('keeps its place when the home refuses the read or the connection loses it', async () => {
    const { home, group, asked, link } = await read();
    void group.sync(home.ctl!);
    asked[1].answer({ re: 2, err: { code: 'forbidden', msg: 'no' } });
    await settle();
    expect(group.cursor).toBe(815);
    link.push(rows(text('a', 816)));
    expect(group.cursor).toBe(815);

    void group.sync(home.ctl!);
    asked[2].fail();
    await settle();
    expect(group.cursor).toBe(815);
    link.push(rows(text('b', 817)));
    expect(group.cursor).toBe(815);
  });

  it('marks the paging as failed when the home refuses the read, so that the top does not wait', async () => {
    const { home, group, asked } = await read();
    expect(group.paging()).toMatchObject({ more: true, failed: false });
    void group.sync(home.ctl!);
    asked[1].answer({ re: 2, err: { code: 'forbidden', msg: 'no' } });
    await settle();
    expect(group.paging().failed).toBe(true);
    expect(group.paging().ready).toBe(false);
    // No page is asked for: only the read was.
    await group.older();
    expect(asked).toHaveLength(2);
  });

  it('does not mark the paging when a refusal comes too late', async () => {
    const { home, group, asked } = await read();
    void group.sync(home.ctl!);
    void group.sync(home.ctl!);
    asked[1].answer({ re: 2, err: { code: 'forbidden', msg: 'no' } });
    await settle();
    expect(group.paging().failed).toBe(false);
  });
});

describe('an answer that comes too late', () => {
  // Round 3, R3-5, with two connections: the first is replaced while its read is still open.
  it('of a connection that was replaced does not end the reset of the one that replaced it', async () => {
    const { home, group, asked } = joined();
    const second = connect(home);
    second.link.push(reset);
    second.link.push(rows(text('n', 2400), text('o', 2401)));
    asked[0].answer(synced(815));
    await settle();
    expect(group.cursor).toBe(0);
    // Nothing is live yet either: the second read has no synced.
    second.link.push(rows(text('p', 2402)));
    expect(group.cursor).toBe(0);
    second.link.offline();

    const third = connect(home);
    expect(third.asked[0].req).toEqual(sync(0));
  });

  it('to a read that was overtaken on the same connection moves nothing', async () => {
    const { home, group, asked, link } = joined();
    void group.sync(home.ctl!);
    asked[0].answer(synced(700));
    await settle();
    expect(group.cursor).toBe(0);
    link.push(rows(text('a', 701)));
    expect(group.cursor).toBe(0);

    asked[1].answer(synced(815));
    await settle();
    expect(group.cursor).toBe(815);
    link.push(rows(text('b', 816)));
    expect(group.cursor).toBe(816);
  });

  it('to a read whose connection is gone moves nothing', async () => {
    const { group, asked, link } = joined();
    link.offline();
    asked[0].answer(synced(815));
    await settle();
    expect(group.cursor).toBe(0);
  });

  it('with a page is dropped when the group is read again, on the same connection or another', async () => {
    const { home, group, asked } = await read();
    const before = group.paging();
    const sameConnection = group.older();
    void group.sync(home.ctl!);
    asked[1].answer(fetched([text('a', 300)], false));
    await sameConnection;
    expect([...group.rows.keys()]).toEqual([]);
    // Until the read ends the paging is the one of before, and as it was.
    expect(group.paging()).toBe(before);
    expect(before).toMatchObject({ before: undefined, more: true, busy: false, failed: false });
    asked[2].answer(synced(900));
    await settle();
    const fresh = group.paging();
    expect(fresh).not.toBe(before);

    const otherConnection = group.older();
    const again = connect(home);
    asked[3].answer(fetched([text('b', 400)], false));
    await otherConnection;
    expect([...group.rows.keys()]).toEqual([]);
    expect(group.paging()).toBe(fresh);
    expect(fresh).toMatchObject({ before: undefined, more: true, busy: false, failed: false });
    again.asked[0].answer(synced(950));
    await settle();
    expect(group.paging()).not.toBe(fresh);
    void group.older();
    expect(again.asked[1].req).toEqual(fetch(0));
  });

  it('with a page is dropped when the connection is gone, and the paging stays as it was', async () => {
    const { group, asked, link } = await read();
    const page = group.older();
    const paging = group.paging();
    link.offline();
    asked[1].answer(fetched([text('a', 300)], false));
    await page;
    expect([...group.rows.keys()]).toEqual([]);
    expect(group.paging()).toBe(paging);
    expect([paging.before, paging.more, paging.busy, paging.failed]).toEqual([
      undefined,
      true,
      false,
      false,
    ]);
  });
});

describe('pages', () => {
  it('are asked for one before the other, each before the lowest pos of the last', async () => {
    const { group, asked } = await read();
    const paging = group.paging();
    expect([paging.before, paging.more, paging.busy, paging.failed, paging.ready]).toEqual([
      undefined,
      true,
      false,
      false,
      true,
    ]);

    const first = group.older();
    expect(asked[1].req).toEqual(fetch(0));
    expect([paging.busy, paging.ready]).toEqual([true, false]);
    asked[1].answer(fetched([text('c', 790), text('b', 760), text('a', 720)], true));
    await first;
    expect([paging.before, paging.more, paging.busy, paging.ready]).toEqual([
      720,
      true,
      false,
      true,
    ]);
    expect(group.main.map((msg) => msg.id)).toEqual(['a', 'b', 'c']);

    void group.older();
    expect(asked[2].req).toEqual(fetch(720));
    asked[2].answer(fetched([text('0', 700)], false));
    await settle();
    expect([paging.before, paging.more, paging.busy, paging.ready]).toEqual([
      700,
      false,
      false,
      false,
    ]);
  });

  it('of the main line start before the oldest of the newest 50 held, whatever older card is held', async () => {
    const { group, asked } = await read();
    // A cold read: a card still pending from long ago, and the newest 50 of the main line.
    const card = row('row-msg-decision-pending', { id: 'card', seq: 5, pos: 5, thread: null });
    const newest = Array.from({ length: 50 }, (_, i) => text(`m${i}`, 700 + i));
    group.apply([card, ...newest]);
    void group.older();
    expect(asked[1].req).toEqual(fetch(700));
    // A message that arrives afterwards does not move the start of a page already asked for.
    group.apply([text('late', 800)]);
    asked[1].answer(fetched([text('l', 690)], true));
    await settle();
    expect(group.paging().before).toBe(690);
  });

  // The probe of the review: 100 messages read, 1500 written while away, and a page asked for the
  // moment the browser is back. The rows held then are about to be dropped by a reset.
  it('are not asked for while the group is being read, and then start from what the read brought', async () => {
    const { home, group, link } = await read();
    link.push(rows(...Array.from({ length: 100 }, (_, i) => text(`old${i}`, 816 + i))));
    const held = group.paging();
    link.offline();

    const again = connect(home);
    void group.older();
    expect(again.asked.map((ask) => ask.req)).toEqual([sync(915)]);
    expect(group.paging()).toBe(held);
    again.link.push(reset);
    void group.older();
    again.link.push(rows(...Array.from({ length: 50 }, (_, i) => text(`new${i}`, 2366 + i))));
    void group.older();
    expect(again.asked).toHaveLength(1);

    again.asked[0].answer(synced(2415));
    await settle();
    const fresh = group.paging();
    expect(fresh).not.toBe(held);
    void group.older();
    expect(again.asked[1].req).toEqual(fetch(2366));
    again.asked[1].answer(fetched([text('between', 2365)], true));
    await settle();
    expect([fresh.before, fresh.more]).toEqual([2365, true]);
  });

  it('of the main line start before the oldest message held when fewer than 50 are held', async () => {
    const { group, asked } = await read();
    group.apply([text('b', 720), text('c', 790), text('a', 705)]);
    void group.older();
    expect(asked[1].req).toEqual(fetch(705));
    // An empty page is the end, and leaves the place where it was asked from.
    asked[1].answer(fetched([], false));
    await settle();
    expect(group.paging()).toMatchObject({ before: 705, more: false, busy: false });
  });

  it('end at an empty page, whatever it says of more', async () => {
    const { group, asked } = await read();
    const page = group.older();
    asked[1].answer(fetched([], true));
    await page;
    expect(group.paging()).toMatchObject({ before: 0, more: false, busy: false, failed: false });
  });

  it('of a thread are read from its newest page, apart from the main line', async () => {
    const { group, asked } = await read();
    group.apply([text('main', 600), text('held', 650, { thread: task })]);
    void group.older(task);
    expect(asked[1].req).toEqual(fetch(0, task));
    const page = [text('b', 760, { thread: task }), text('a', 720, { thread: task })];
    asked[1].answer(fetched(page, true));
    await settle();
    expect(group.paging(task).before).toBe(720);
    expect(group.paging(task)).toBe(group.paging(task));
    expect(group.paging()).toBe(group.paging(null));
    expect(group.paging().before).toBeUndefined();
    expect(group.thread(task).map((msg) => msg.id)).toEqual(['held', 'a', 'b']);

    void group.older(task);
    expect(asked[2].req).toEqual(fetch(720, task));
  });

  it('are not asked for while one is on its way, when there are no more, or with no connection', async () => {
    const { group, asked, link } = await read();
    void group.older();
    void group.older();
    expect(asked).toHaveLength(2);
    asked[1].answer(fetched([text('a', 720)], false));
    await settle();
    void group.older();
    expect(asked).toHaveLength(2);

    void group.older(task);
    expect(asked).toHaveLength(3);
    link.offline();
    await group.older('another');
    expect(asked).toHaveLength(3);
    expect(group.paging('another').busy).toBe(false);
  });

  it('stop, with nothing changed, when the home refuses one or the connection fails it', async () => {
    const { group, asked } = await read();
    group.apply([text('a', 720)]);
    const refused = group.older();
    asked[1].answer({ re: 2, err: { code: 'not_found', msg: 'no such group' } });
    await refused;
    expect(group.paging()).toMatchObject({
      before: undefined,
      more: true,
      busy: false,
      failed: true,
      ready: false,
    });
    void group.older();
    expect(asked).toHaveLength(2);

    const failing = group.older(task);
    asked[2].fail();
    await failing;
    expect(group.paging(task)).toMatchObject({ more: true, busy: false, failed: true });
    expect([...group.rows.keys()]).toEqual(['msg a']);
  });

  it('start again when a read of the group ends, not before', async () => {
    const { home, group, asked, link } = await read();
    const refused = group.older();
    asked[1].answer({ re: 2, err: { code: 'internal', msg: 'no' } });
    await refused;
    const before = group.paging();
    expect(before.failed).toBe(true);

    link.offline();
    const again = connect(home);
    expect(group.paging()).toBe(before);
    again.asked[0].answer(synced(900));
    await settle();
    const fresh = group.paging();
    expect(fresh).not.toBe(before);
    expect([fresh.before, fresh.more, fresh.busy, fresh.failed]).toEqual([
      undefined,
      true,
      false,
      false,
    ]);
    void group.older();
    expect(again.asked[1].req).toEqual(fetch(0));
  });

  it('do not start again when the read is refused', async () => {
    const { home, group, asked } = await read();
    const before = group.paging();
    void group.sync(home.ctl!);
    asked[1].answer({ re: 2, err: { code: 'forbidden', msg: 'no' } });
    await settle();
    expect(group.paging()).toBe(before);
    // Nor is a page asked for after a read that did not end: nothing says what is held is whole.
    void group.older();
    expect(asked).toHaveLength(2);
  });
});

describe('the lists of a group', () => {
  it('order messages by pos, whatever order they arrive in', () => {
    const { group } = joined();
    group.apply([text('b', 20), row('row-member-human'), text('c', 30), text('a', 10)]);
    expect(group.main.map((msg) => msg.id)).toEqual(['a', 'b', 'c']);
    // Neither does a marker among them disturb the order.
    const marked = joined().group;
    marked.apply([text('b', 20), gone('msg', 'x', 25), text('c', 30), text('a', 10)]);
    expect(marked.main.map((msg) => msg.id)).toEqual(['a', 'b', 'c']);
    // A card that is decided gets a new seq and keeps its pos.
    group.apply([text('a', 40, { pos: 10 })]);
    expect(group.main.map((msg) => msg.id)).toEqual(['a', 'b', 'c']);
  });

  it('put in the main line what is outside a thread or raised from one', () => {
    const { group } = joined();
    group.apply([
      text('plain', 1),
      text('raised', 2, { thread: task, main: true }),
      text('threaded', 3, { thread: task }),
      text('other', 4, { thread: 'another-task' }),
      row('row-note', { seq: 5 }),
    ]);
    expect(group.main.map((msg) => msg.id)).toEqual(['plain', 'raised']);
    expect(group.thread(task).map((msg) => msg.id)).toEqual(['raised', 'threaded']);
  });

  it('list the members by name, and find one by id', () => {
    const { group } = joined();
    const owner = row('row-member-human') as p.HumanMemberRow;
    const ai = row('row-member-ai') as p.AIMemberRow;
    const claude = row('row-member-ai', { id: 'claude', name: 'Claude Code' });
    group.apply([owner, ai, text('a', 1), claude]);
    expect(group.members).toEqual([claude, ai, owner]);
    expect(group.member(ai.id)).toBe(ai);
    expect(group.member('system')).toBeUndefined();
  });

  it('know the person at this browser among the members, and find a person name or task by id', () => {
    const { group } = joined();
    expect(group.me).toBeUndefined();
    expect(group.personName(user)).toBeUndefined();
    expect(group.task(task)).toBeUndefined();
    // An AI carries its owner's user id too, and comes first by name.
    const ai = row('row-member-ai', { name: 'Alpha AI' });
    const aiOnly = row('row-member-ai', { id: 'solo-ai', user: 'ai-owner', name: 'Beta AI' });
    const stranger = row('row-member-human', { id: 'other', user: 'someone', name: 'Mina' });
    const owner = row('row-member-human', { name: 'Zoe' }) as p.HumanMemberRow;
    const taskRow = row('row-task') as p.TaskRow;
    const sameIdNote = row('row-note', { id: 'same' });
    group.apply([ai, aiOnly, stranger, owner, taskRow, sameIdNote]);
    expect(group.me).toBe(owner);
    expect(group.personName(user)).toBe('Zoe');
    expect(group.personName('someone')).toBe('Mina');
    expect(group.personName('ai-owner')).toBeUndefined();
    expect(group.personName('nobody')).toBeUndefined();
    expect(group.task(taskRow.id)).toBe(taskRow);
    expect(group.task('same')).toBeUndefined();
    expect(group.task('missing')).toBeUndefined();

    group.apply([gone('member', owner.id, 900), gone('task', taskRow.id, 901)]);
    expect(group.personName(user)).toBeUndefined();
    expect(group.task(taskRow.id)).toBeUndefined();
  });

  it('hold the group row and the time of the last main-line message', () => {
    const { group } = joined();
    expect(group.info).toBeUndefined();
    expect(group.latest).toBe(0);
    const info = row('row-group');
    group.apply([info, row('row-group', { id: 'another-group', created: 1 })]);
    expect(group.info).toBe(info);
    expect(group.latest).toBe(1763599995000);
    group.apply([text('a', 10, { ts: 30 }), text('b', 20, { ts: 50 }), text('c', 30, { ts: 40 })]);
    expect(group.latest).toBe(40);
    group.apply([text('d', 40, { ts: 90, thread: task })]);
    expect(group.latest).toBe(40);
  });

  it('take the name from the list of the node', () => {
    const { home, group } = joined();
    expect(group.name).toBe('Website');
    expect(group.g).toBe(website);
    expect(group.home).toBe(home);
  });
});

describe('waiting for you', () => {
  const brief = (id: string, seq: number, changes: object = {}) =>
    row('row-msg-brief-pending', { id, seq, pos: seq, ...changes });
  const approval = (id: string, seq: number, changes: object = {}) =>
    row('row-msg-approval-pending', { id, seq, pos: seq, ...changes });
  const decision = (id: string, seq: number, changes: object = {}) =>
    row('row-msg-decision-pending', { id, seq, pos: seq, ...changes });
  const confirm = (id: string, seq: number, to: string[]) =>
    row('row-msg-confirm-pending', {
      id,
      seq,
      pos: seq,
      by: 'system',
      ...body('row-msg-confirm-pending', { to }),
    });
  const body = (name: string, changes: object) => ({
    body: { ...(p.decode(p.MsgRow, goldens[name]).body as object), ...changes },
  });

  function withMe() {
    const { group } = joined();
    group.apply([row('row-member-human')]);
    return group;
  }

  it('lists the pending cards that this person has to act on, oldest first', () => {
    const group = withMe();
    group.apply([
      confirm('confirm', 40, [user]),
      decision('decision', 30),
      brief('brief', 10),
      approval('approval', 20),
    ]);
    expect(group.waiting.map((msg) => msg.id)).toEqual([
      'brief',
      'approval',
      'decision',
      'confirm',
    ]);
  });

  it('leaves out the cards of other people', () => {
    const group = withMe();
    group.apply([
      brief('brief', 10, body('row-msg-brief-pending', { requester: 'someone-else' })),
      approval('approval', 20, body('row-msg-approval-pending', { approvers: ['someone'] })),
      decision('decision', 30, body('row-msg-decision-pending', { to: ['someone'] })),
      confirm('confirm', 40, ['someone']),
    ]);
    expect(group.waiting).toEqual([]);
  });

  it('leaves out a brief while this person is no member of the group', () => {
    const { group } = joined();
    group.apply([brief('mine', 10)]);
    expect(group.waiting).toEqual([]);
  });

  it('leaves out what is decided, and what is no card to decide', () => {
    const group = withMe();
    group.apply([
      row('row-msg-brief-confirmed', { id: 'confirmed' }),
      row('row-msg-approval-approved', { id: 'approved' }),
      row('row-msg-decision-decided', { id: 'decided' }),
      row('row-msg-confirm-denied', {
        id: 'denied',
        ...body('row-msg-confirm-denied', { to: [user] }),
      }),
      row('row-msg-call-waiting', { id: 'call' }),
      text('text', 5),
      // A kind that is no card is never waited for, whatever it carries.
      text('odd', 6, { state: { status: 'pending' }, body: { to: [user], approvers: [user] } }),
    ]);
    expect(group.waiting).toEqual([]);
  });
});

describe('the subject of a message', () => {
  const group = joined().group;
  const msg = (name: string, changes: object = {}) => row(name, changes) as p.MsgRow;
  const without = (name: string, field: string) => {
    const { [field]: _dropped, ...body } = p.decode(p.MsgRow, goldens[name]).body as Record<
      string,
      unknown
    >;
    return msg(name, { body });
  };

  it('is the title of a brief and the question of a decision', () => {
    expect(subject(group, msg('row-msg-brief-pending', { text: 'Here is the brief.' }))).toBe(
      'Show connection paths',
    );
    expect(subject(group, msg('row-msg-decision-pending'))).toBe('Where should the path appear?');
  });

  it('is the reason of an approval, or else the request itself', () => {
    expect(subject(group, msg('row-msg-approval-pending'))).toBe(
      'Time the path change in a browser.',
    );
    expect(subject(group, without('row-msg-approval-pending', 'why'))).toBe(
      'pnpm exec playwright install chromium',
    );
  });

  it('is the request of a confirmation', () => {
    group.apply([row('row-member-ai', { id: 'TEcBBAeLwcuEpP9Z0Sekug', name: 'Claude Code' })]);
    group.apply([row('row-member-human', { name: 'Pat' })]);
    expect(subject(group, msg('row-msg-confirm-pending'))).toBe(
      'Let Pat watch Claude Code’s terminal.',
    );
  });

  it('is the text of any other message', () => {
    expect(subject(group, text('a', 1))).toBe('The path is direct.');
    expect(subject(group, msg('row-msg-question'))).toBe('Should the path include latency?');
  });
});

describe('a home', () => {
  it('lists the groups of its node and reads each', () => {
    const home = new Home(me);
    expect([home.ctl, home.groups, home.device, home.name, home.owner]).toEqual([
      undefined,
      [],
      '',
      '',
      false,
    ]);
    expect(home.me).toBe(me);
    const { ctl, asked } = connect(home, [listed('one', 'One'), listed('two', 'Two')]);
    expect(home.ctl).toBe(ctl);
    expect([home.device, home.name, home.owner]).toEqual([
      'J3Uuh-y1z2AX536yAFlGHJL5sjxVpWVJavvVphvdm5Q',
      'workstation',
      true,
    ]);
    expect(home.groups.map((group) => [group.g, group.name])).toEqual([
      ['one', 'One'],
      ['two', 'Two'],
    ]);
    expect(asked.map((ask) => ask.req)).toEqual([sync(0, 'one'), sync(0, 'two')]);
  });

  it('says whose node it is not', () => {
    const home = new Home(me);
    connect(home, [], { name: 'laptop', owner: false });
    expect([home.name, home.owner]).toEqual(['laptop', false]);
  });

  it('shows its groups from the rows held while the node cannot be reached, and reads on when it returns', async () => {
    const { home, group, link } = await read();
    link.push(rows(text('a', 816)));
    link.offline();
    expect(home.ctl).toBeUndefined();
    expect(home.groups).toEqual([group]);
    expect(group.main.map((msg) => msg.id)).toEqual(['a']);

    const again = connect(home, [listed(website, 'Website 2')]);
    expect(home.ctl).toBe(again.ctl);
    expect(home.groups).toEqual([group]);
    expect(group.name).toBe('Website 2');
    expect(again.asked.map((ask) => ask.req)).toEqual([sync(816)]);
  });

  it('replaces its list at a groups push and reads only the group that is new', async () => {
    const { home, group, asked, link } = await read();
    link.push({ t: 'groups', groups: [listed('new', 'Docs'), listed(website, 'Site')] });
    expect(home.groups.map((each) => [each.g, each.name])).toEqual([
      ['new', 'Docs'],
      [website, 'Site'],
    ]);
    expect(home.groups[1]).toBe(group);
    expect(asked.slice(1).map((ask) => ask.req)).toEqual([sync(0, 'new')]);

    link.push({ t: 'groups', groups: [] });
    expect(home.groups).toEqual([]);
    expect(asked).toHaveLength(2);
  });

  it('gives each push to the group it names and drops the others', async () => {
    const home = new Home(me);
    const { link } = connect(home, [listed('one'), listed('two')]);
    const [one, two] = home.groups;
    link.push({ t: 'rows', g: 'two', rows: [text('a', 5)] });
    link.push({ t: 'rows', g: 'one', rows: [text('b', 6)] });
    link.push({ t: 'rows', g: 'gone', rows: [text('c', 7)] });
    link.push({ t: 'reset', g: 'gone' });
    link.push({ t: 'devlog', user, entries: [] });
    expect([...one.rows.keys()]).toEqual(['msg b']);
    expect([...two.rows.keys()]).toEqual(['msg a']);
    link.push({ t: 'reset', g: 'two' });
    expect([one.rows.size, two.rows.size]).toEqual([1, 0]);
    expect(home.hostInfo).toBeUndefined();
    expect([...home.terminalSizes]).toEqual([]);
  });

  it('reads a fresh group again when an unlisted group is listed again', async () => {
    const { home, group, link, asked } = await read(815);
    link.push({ t: 'groups', groups: [] });
    expect(home.groups).toEqual([]);
    link.push({ t: 'groups', groups: [listed(website, 'Listed again')] });
    expect(home.groups).toHaveLength(1);
    expect(home.groups[0]).not.toBe(group);
    expect([home.groups[0].name, home.groups[0].cursor, home.groups[0].rows.size]).toEqual([
      'Listed again',
      0,
      0,
    ]);
    expect(asked.map(({ req }) => req)).toEqual([sync(0), sync(0)]);
  });

  it('takes pushes from the last connection to come online and from no other', async () => {
    const home = new Home(me);
    const early = home.connect();
    const first = connect(home);
    const group = home.groups[0];
    // A connection on which the node has not answered hello is not the home's yet.
    early.push(rows(text('early', 1)));
    expect(group.rows.size).toBe(0);
    first.link.push(rows(text('a', 2)));
    expect([...group.rows.keys()]).toEqual(['msg a']);

    const second = connect(home);
    first.link.push(rows(text('stale', 3)));
    first.link.push(reset);
    first.link.push({ t: 'groups', groups: [] });
    expect([...group.rows.keys()]).toEqual(['msg a']);
    expect(home.groups).toEqual([group]);
    second.link.push(rows(text('b', 4)));
    expect([...group.rows.keys()]).toEqual(['msg a', 'msg b']);
  });

  it('stays online when a connection that was replaced goes, and takes nothing from one that is gone', () => {
    const home = new Home(me);
    const first = connect(home);
    const second = connect(home);
    first.link.offline();
    expect(home.ctl).toBe(second.ctl);

    second.link.offline();
    expect(home.ctl).toBeUndefined();
    second.link.push(rows(text('a', 2)));
    expect(home.groups[0].rows.size).toBe(0);
  });
});

describe('commands', () => {
  beforeEach(() => {
    vi.useFakeTimers({ toFake: ['Date'] });
    vi.setSystemTime(1763600000000);
  });

  const keysOf = (example: string) => Object.keys(JSON.parse(goldens[example]));

  it('are signed by this device and answered by the node', async () => {
    const { home, group, asked } = joined();
    const sending = group.post('Hello @Codex', [codex]);
    await vi.waitFor(() => expect(asked).toHaveLength(2));
    const payload = p.decode(p.Post, signed[0]);
    expect(payload).toEqual({
      t: 'post',
      n: expect.stringMatching(/^[A-Za-z0-9_-]{21}[AQgw]$/),
      ts: 1763600000000,
      g: website,
      kind: 'text',
      thread: null,
      main: false,
      text: 'Hello @Codex',
      mentions: [codex],
      re: null,
      body: {},
    });
    // The fields of the wire example of a post, in its order; only an AI's post has `as`.
    expect(Object.keys(JSON.parse(signed[0]))).toEqual(
      keysOf('post-text').filter((field) => field !== 'as'),
    );
    expect(asked[1].req).toEqual({
      t: 'cmd',
      env: { p: signed[0], k: 'device', s: 'signature' },
    });
    asked[1].answer({ re: 2, ok: true });
    expect(await sending).toEqual({ re: 2, ok: true });

    const refused = home.command({ t: 'jump', g: website, msg: 'a' });
    await vi.waitFor(() => expect(asked).toHaveLength(3));
    asked[2].answer({ re: 3, err: { code: 'conflict', msg: 'nothing to jump' } });
    expect(await refused).toEqual({ re: 3, err: { code: 'conflict', msg: 'nothing to jump' } });
  });

  it('create a project group that the nonce names, as the wire example of the command has it', async () => {
    const { home, asked } = joined();
    const creating = home.create({
      name: 'Docs',
      repo: 'https://git.example/team/docs.git',
      base: 'main',
      forge: 'github',
      checks: ['pnpm test'],
      member_name: 'Alex',
    });
    await vi.waitFor(() => expect(asked).toHaveLength(2));
    const payload = p.decode(p.GroupCreate, signed[0]);
    expect(payload).toEqual({
      t: 'group',
      n: expect.any(String),
      ts: 1763600000000,
      g: payload.n,
      op: 'create',
      name: 'Docs',
      kind: 'project',
      repo: 'https://git.example/team/docs.git',
      base: 'main',
      forge: 'github',
      checks: ['pnpm test'],
      member_name: 'Alex',
    });
    expect(Object.keys(JSON.parse(signed[0]))).toEqual(keysOf('group-create'));
    asked[1].answer({ re: 2, ok: true });
    expect(await creating).toEqual({ g: payload.n, answer: { re: 2, ok: true } });
  });

  it('give each command a nonce of its own, or the one handed to them', async () => {
    const { home, asked } = joined();
    void home.command({ t: 'jump', g: website, msg: 'a' });
    void home.command({ t: 'jump', g: website, msg: 'a' });
    void home.command({ t: 'jump', g: website, msg: 'a' }, 'again');
    await vi.waitFor(() => expect(asked).toHaveLength(4));
    const nonces = signed.map((payload) => JSON.parse(payload).n);
    expect(nonces[0]).not.toBe(nonces[1]);
    expect(nonces[2]).toBe('again');
  });

  it('are answered as lost when the node cannot be reached or the connection breaks', async () => {
    const { group, asked, link } = joined();
    const breaking = group.post('one', []);
    await vi.waitFor(() => expect(asked).toHaveLength(2));
    asked[1].fail();
    expect(await breaking).toEqual(lost());

    link.offline();
    expect(await group.post('two', [])).toEqual(lost());
    expect(signed).toHaveLength(2);
    expect(asked).toHaveLength(2);
  });
});

describe('every group', () => {
  const home = (...groups: string[]) => {
    const made = new Home(me);
    connect(
      made,
      groups.map((g) => listed(g, g)),
    );
    homes.push(made);
    return made;
  };

  it('starts with no home', () => {
    expect(homesAtFirst).toEqual([]);
  });

  it('is listed with the newest main line first', () => {
    const first = home('quiet', 'old');
    const second = home('busy', 'new');
    expect(recent().map((group) => group.g)).toEqual(['quiet', 'old', 'busy', 'new']);
    first.groups[1].apply([text('a', 1, { ts: 100 })]);
    second.groups[0].apply([text('a', 1, { ts: 300 })]);
    second.groups[1].apply([text('a', 1, { ts: 200 })]);
    expect(recent().map((group) => group.g)).toEqual(['busy', 'new', 'old', 'quiet']);
    expect(first.groups.map((group) => group.g)).toEqual(['quiet', 'old']);
  });

  it('is found by its id', () => {
    const first = home('one');
    const second = home('two', 'three');
    expect(find('three')).toBe(second.groups[1]);
    expect(find('one')).toBe(first.groups[0]);
    expect(find('four')).toBeUndefined();
    expect(find(undefined)).toBeUndefined();
  });
});

describe('following a moved group', () => {
  const moved = { re: 1, err: { code: 'moved', msg: 'The group has moved.', home: 'destination' } };

  function destination(home: Home) {
    home.device = 'source';
    const target = new Home(me);
    target.device = 'destination';
    home.onmove = vi.fn((g, id) => {
      expect(id).toBe('destination');
      home.move(g, target);
    });
    return target;
  }

  it('keeps cached rows and the route while following sync, and reads again after import', async () => {
    const { home, group, asked, link } = joined();
    group.apply([text('kept', 4)]);
    const target = destination(home);
    const read = vi.spyOn(group, 'sync').mockResolvedValue(undefined);
    homes.push(home, target);
    asked[0].answer(moved);
    await settle();
    expect(read).toHaveBeenCalledTimes(0);
    read.mockRestore();
    expect(home.groups).toEqual([]);
    expect(target.groups).toEqual([group]);
    expect(find(website)).toBe(group);
    expect(group.home).toBe(target);
    expect(group.main.map((msg) => msg.id)).toEqual(['kept']);
    link.push(rows(text('late', 5)));
    expect(group.main.map((msg) => msg.id)).toEqual(['kept']);
    const next = connect(target, [], { device: 'destination' });
    expect(target.groups).toEqual([group]);
    expect(next.asked.map(({ req }) => req)).toEqual([sync(0)]);
    next.asked[0].answer({ re: 1, err: { code: 'not_found', msg: 'not imported' } });
    await settle();
    expect(group.paging().failed).toBe(true);
    next.link.push({ t: 'groups', groups: [listed(website, 'Imported')] });
    expect(next.asked.map(({ req }) => req)).toEqual([sync(0), sync(0)]);
    next.asked[1].answer(synced(6));
    await settle();
    expect([group.name, group.paging().failed, group.cursor]).toEqual(['Imported', false, 6]);
    expect(target.groups).toEqual([group]);
    next.link.push({ t: 'groups', groups: [] });
    expect(target.groups).toEqual([]);
  });

  it('follows fetch responses and reads the retained cursor on an already-connected destination', async () => {
    const { home, group, asked, link } = await read(815);
    link.push({ t: 'groups', groups: [listed(website), listed('stays')] });
    asked[1].answer({ ...synced(0), g: 'stays' });
    await settle();
    const target = destination(home);
    const existing = connect(target, [listed('other'), listed(website)]);
    const page = group.older();
    asked[2].answer(moved);
    await page;
    expect(home.groups.map((item) => item.g)).toEqual(['stays']);
    expect(target.groups.map((item) => item.g)).toEqual(['other', website]);
    expect(target.groups[1]).toBe(group);
    expect(existing.asked.map(({ req }) => req)).toEqual([sync(0, 'other'), sync(0), sync(815)]);
    home.follow(website, moved.err);
    expect(existing.asked.map(({ req }) => req)).toEqual([sync(0, 'other'), sync(0), sync(815)]);
    home.move('absent', target);
    expect(target.groups.map((item) => item.g)).toEqual(['other', website]);
    existing.link.push({ t: 'groups', groups: [] });
    expect(target.groups).toEqual([]);
  });

  it('does not resurrect an unlisted group when an old command answers moved', async () => {
    const { home, group, asked, link } = await read();
    const target = destination(home);
    const reread = vi.spyOn(group, 'sync').mockResolvedValue(undefined);
    const command = group.post('once', []);
    await settle();
    link.push({ t: 'groups', groups: [] });
    asked[1].answer(moved);
    expect(await command).toEqual(moved);
    expect(reread).toHaveBeenCalledTimes(0);
    reread.mockRestore();
    expect(target.groups).toEqual([]);
    expect(home.onmove).toHaveBeenCalledExactlyOnceWith(website, 'destination');
    expect(asked.map(({ req }) => req.t)).toEqual(['sync', 'cmd']);
    expect(group.home).toBe(home);
    expect(home.groups).toEqual([]);
  });

  it('ignores missing, empty and self destinations and other errors', () => {
    const { home } = joined();
    destination(home);
    for (const err of [
      { code: 'forbidden', home: 'destination' },
      { code: 'moved' },
      { code: 'moved', home: '' },
      { code: 'moved', home: 'source' },
    ])
      home.follow(website, { ...err, msg: 'refused' });
    expect(home.onmove).toHaveBeenCalledTimes(0);
    const standalone = new Home(me);
    standalone.follow(website, moved.err);
    expect(standalone.groups).toEqual([]);
  });

  it('does not follow a command response from a connection that was replaced or lost', async () => {
    const { home, group, asked, link } = await read();
    destination(home);
    const command = group.post('late', []);
    await settle();
    link.offline();
    asked[1].answer(moved);
    expect(await command).toEqual(moved);
    expect(home.onmove).toHaveBeenCalledTimes(0);
    expect(group.home).toBe(home);
  });
});

describe('ids and lost answers', () => {
  it('makes a new random id each time', () => {
    const ids = [newId(), newId()];
    expect(ids[0]).not.toBe(ids[1]);
    for (const id of ids) expect(p.decode(p.Id, JSON.stringify(id))).toBe(id);
  });

  it('answers a lost request with a code of its own', () => {
    expect(lost()).toEqual({ re: 0, err: { code: 'lost', msg: 'connection lost' } });
  });

  it('makes a group for a home', () => {
    const home = new Home(me);
    const group = new Group(home, 'g');
    expect([group.home, group.g, group.name, group.cursor]).toEqual([home, 'g', '', 0]);
    // Nothing is live before the first synced.
    group.apply([text('a', 7)]);
    expect(group.cursor).toBe(0);
  });
});
