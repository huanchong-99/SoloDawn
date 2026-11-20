import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { find, Group, Home, homes, lost, newId, recent, subject } from './groups.svelte';
import { goldens } from './mock';
import * as p from './proto';

const user = 'akWV_5PtNLIGhakuMAvpD9LcG1D8TUfc1URh48VREcs';
const website = 'rZNvy-1jH6Z-BcPqA5U5BQ';
const task = 'DrtCn6htSBwmMPrFPbHJHA';

/** A wire example as a row, with some fields changed. */
const row = (name: string, changes: object = {}) =>
  ({ ...p.decode(p.Row, goldens[name]), ...changes }) as p.Row;
const text = (id: string, seq: number, changes: object = {}) =>
  row('row-msg-text', { id, seq, pos: seq, ...changes }) as p.MsgRow;
const gone = (r: string, id: string, seq: number): p.GoneRow => ({ r, id, seq, gone: true });

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

/** A home whose one group has been listed, and the request of its first read. */
function joined() {
  const home = new Home(me);
  const { ctl, asked } = connection();
  home.online(ctl, welcome([listed(website)]));
  return { home, group: home.groups[0], ctl, asked };
}

/** The same, read to the end at sequence number S. */
async function synced(seq = 815) {
  const all = joined();
  all.asked[0].answer({ t: 'synced', re: 1, ok: true, g: website, seq });
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
    const { home, group, asked } = await synced();
    const message = text('a', 700, { thread: task });
    const member = row('row-member-ai', { seq: 700 });
    group.apply([message, member]);
    expect(group.main).toEqual([]);
    expect(group.thread(task)).toEqual([message]);
    expect(group.members).toEqual([member]);

    home.push({
      t: 'rows',
      g: website,
      rows: [gone('msg', 'a', 816), gone('member', member.id, 817)],
    });
    expect(group.rows.get('msg a')).toEqual(gone('msg', 'a', 816));
    expect(group.thread(task)).toEqual([]);
    expect(group.members).toEqual([]);
    expect(group.member(member.id)).toBeUndefined();

    // A page asked for before the deletion arrives after it.
    void group.older(task);
    asked[1].answer({ t: 'fetched', re: 2, ok: true, rows: [message], more: false });
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
    expect(asked.map((ask) => ask.req)).toEqual([
      { t: 'sync', g: website, since: 0, state_only: false },
    ]);
  });

  it('is not moved by rows that arrive before synced, and becomes S at synced', async () => {
    const { home, group, asked } = joined();
    home.push({ t: 'reset', g: website });
    home.push({ t: 'rows', g: website, rows: [text('a', 700), text('b', 710)] });
    expect(group.main.map((msg) => msg.id)).toEqual(['a', 'b']);
    expect(group.cursor).toBe(0);
    asked[0].answer({ t: 'synced', re: 1, ok: true, g: website, seq: 815 });
    await settle();
    expect(group.cursor).toBe(815);
  });

  it('then follows each live row applied, and no row that changes nothing', async () => {
    const { home, group } = await synced();
    home.push({ t: 'rows', g: website, rows: [text('a', 816), text('b', 817)] });
    expect(group.cursor).toBe(817);
    home.push({ t: 'rows', g: website, rows: [text('a', 816)] });
    expect(group.cursor).toBe(817);
    home.push({ t: 'rows', g: website, rows: [text('b', 800)] });
    expect(group.cursor).toBe(817);
    home.push({ t: 'rows', g: website, rows: [gone('msg', 'a', 818)] });
    expect(group.cursor).toBe(818);
  });

  it('is never moved by a page', async () => {
    const { group, asked } = await synced();
    void group.older();
    asked[1].answer({ t: 'fetched', re: 2, ok: true, rows: [text('z', 900)], more: false });
    await settle();
    expect(group.main.map((msg) => msg.id)).toEqual(['z']);
    expect(group.cursor).toBe(815);
  });

  it('returns to 0 with the cache at a reset, also when the read breaks off after its first batch', async () => {
    const { home, group } = await synced();
    home.push({ t: 'rows', g: website, rows: [text('a', 816)] });
    home.offline();

    const again = connection();
    home.online(again.ctl, welcome([listed(website)]));
    expect(again.asked[0].req).toEqual({ t: 'sync', g: website, since: 816, state_only: false });
    home.push({ t: 'reset', g: website });
    expect(group.rows.size).toBe(0);
    expect(group.cursor).toBe(0);
    home.push({ t: 'rows', g: website, rows: [text('n', 2400), text('o', 2401)] });
    // The connection is lost here: no synced.
    home.offline();
    expect(group.cursor).toBe(0);

    const third = connection();
    home.online(third.ctl, welcome([listed(website)]));
    expect(third.asked[0].req).toEqual({ t: 'sync', g: website, since: 0, state_only: false });
  });

  it('keeps its place when a delta breaks off', async () => {
    const { home, group } = await synced();
    home.offline();
    const again = connection();
    home.online(again.ctl, welcome([listed(website)]));
    home.push({ t: 'rows', g: website, rows: [text('a', 816), text('b', 817)] });
    home.offline();
    expect(group.main.map((msg) => msg.id)).toEqual(['a', 'b']);
    expect(group.cursor).toBe(815);
  });

  it('keeps its place when the home refuses the read or the connection loses it', async () => {
    const { home, group, asked } = await synced();
    void group.sync(home.ctl!);
    asked[1].answer({ re: 2, err: { code: 'forbidden', msg: 'no' } });
    await settle();
    expect(group.cursor).toBe(815);
    home.push({ t: 'rows', g: website, rows: [text('a', 816)] });
    expect(group.cursor).toBe(815);

    void group.sync(home.ctl!);
    asked[2].fail();
    await settle();
    expect(group.cursor).toBe(815);
    home.push({ t: 'rows', g: website, rows: [text('b', 817)] });
    expect(group.cursor).toBe(815);
  });
});

describe('pages', () => {
  it('ask for the newest first, then for what is before the lowest pos received', async () => {
    const { group, asked } = await synced();
    const paging = group.paging();
    expect([paging.before, paging.more, paging.busy]).toEqual([0, true, false]);

    const first = group.older();
    expect(asked[1].req).toEqual({ t: 'fetch', g: website, thread: null, before: 0, limit: 50 });
    expect(paging.busy).toBe(true);
    const page = [text('c', 790), text('b', 760), text('a', 720)];
    asked[1].answer({ t: 'fetched', re: 2, ok: true, rows: page, more: true });
    await first;
    expect([paging.before, paging.more, paging.busy]).toEqual([720, true, false]);
    expect(group.main.map((msg) => msg.id)).toEqual(['a', 'b', 'c']);

    void group.older();
    expect(asked[2].req).toEqual({ t: 'fetch', g: website, thread: null, before: 720, limit: 50 });
    asked[2].answer({ t: 'fetched', re: 3, ok: true, rows: [], more: false });
    await settle();
    expect([paging.before, paging.more, paging.busy]).toEqual([720, false, false]);
  });

  it('end at an empty page, whatever it says of more', async () => {
    const { group, asked } = await synced();
    const page = group.older();
    asked[1].answer({ t: 'fetched', re: 2, ok: true, rows: [], more: true });
    await page;
    expect(group.paging()).toMatchObject({ before: 0, more: false, busy: false });
  });

  it('of a thread are read apart from the main line', async () => {
    const { group, asked } = await synced();
    void group.older(task);
    expect(asked[1].req).toEqual({ t: 'fetch', g: website, thread: task, before: 0, limit: 50 });
    asked[1].answer({
      t: 'fetched',
      re: 2,
      ok: true,
      rows: [text('b', 760, { thread: task }), text('a', 720, { thread: task })],
      more: true,
    });
    await settle();
    expect(group.paging(task).before).toBe(720);
    expect(group.paging(task)).toBe(group.paging(task));
    expect(group.paging()).toBe(group.paging(null));
    expect(group.paging().before).toBe(0);
    expect(group.thread(task).map((msg) => msg.id)).toEqual(['a', 'b']);
  });

  it('are not asked for while one is on its way, when there are no more, or with no connection', async () => {
    const { home, group, asked } = await synced();
    void group.older();
    void group.older();
    expect(asked).toHaveLength(2);
    asked[1].answer({ t: 'fetched', re: 2, ok: true, rows: [text('a', 720)], more: false });
    await settle();
    void group.older();
    expect(asked).toHaveLength(2);

    void group.older(task);
    expect(asked).toHaveLength(3);
    home.offline();
    await group.older('another');
    expect(asked).toHaveLength(3);
    expect(group.paging('another').busy).toBe(false);
  });

  it('end, with nothing changed, when the home refuses one or the connection loses it', async () => {
    const { group, asked } = await synced();
    group.apply([text('a', 720)]);
    const refused = group.older();
    asked[1].answer({ re: 2, err: { code: 'not_found', msg: 'no such group' } });
    await refused;
    expect(group.paging()).toMatchObject({ before: 0, more: false, busy: false });

    const lostPage = group.older(task);
    asked[2].fail();
    await lostPage;
    expect(group.paging(task)).toMatchObject({ before: 0, more: false, busy: false });
    expect([...group.rows.keys()]).toEqual(['msg a']);
  });

  it('start again when the group is read again, whatever a page from before brings', async () => {
    const { home, group, asked } = await synced();
    const before = group.paging();
    const stale = group.older();
    home.offline();
    const again = connection();
    home.online(again.ctl, welcome([listed(website)]));
    const fresh = group.paging();
    expect(fresh).not.toBe(before);

    asked[1].answer({ t: 'fetched', re: 2, ok: true, rows: [text('a', 300)], more: false });
    await stale;
    expect(group.paging()).toBe(fresh);
    expect([fresh.before, fresh.more, fresh.busy]).toEqual([0, true, false]);
    void group.older();
    expect(again.asked[1].req).toMatchObject({ t: 'fetch', before: 0 });
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
    const codex = row('row-member-ai') as p.AIMemberRow;
    const claude = row('row-member-ai', { id: 'claude', name: 'Claude Code' });
    group.apply([owner, codex, text('a', 1), claude]);
    expect(group.members).toEqual([claude, codex, owner]);
    expect(group.member(codex.id)).toBe(codex);
    expect(group.member('system')).toBeUndefined();
  });

  it('know the person at this browser among the members', () => {
    const { group } = joined();
    expect(group.me).toBeUndefined();
    // An AI carries its owner's user id too, and comes first by name.
    const codex = row('row-member-ai');
    const stranger = row('row-member-human', { id: 'other', user: 'someone', name: 'mina' });
    const owner = row('row-member-human');
    group.apply([codex, stranger, owner]);
    expect(group.me).toBe(owner);
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
    group.apply([decision('decision', 30), brief('brief', 10), approval('approval', 20)]);
    expect(group.waiting.map((msg) => msg.id)).toEqual(['brief', 'approval', 'decision']);
  });

  it('leaves out the cards of other people', () => {
    const group = withMe();
    group.apply([
      brief('brief', 10, body('row-msg-brief-pending', { requester: 'someone-else' })),
      approval('approval', 20, body('row-msg-approval-pending', { approvers: ['someone'] })),
      decision('decision', 30, body('row-msg-decision-pending', { to: ['someone'] })),
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
      row('row-msg-call-waiting', { id: 'call' }),
      text('text', 5),
      // A kind that is no card is never waited for, whatever it carries.
      text('odd', 6, { state: { status: 'pending' }, body: { to: [user], approvers: [user] } }),
    ]);
    expect(group.waiting).toEqual([]);
  });
});

describe('the subject of a message', () => {
  const msg = (name: string, changes: object = {}) => row(name, changes) as p.MsgRow;
  const without = (name: string, field: string) => {
    const { [field]: _dropped, ...body } = p.decode(p.MsgRow, goldens[name]).body as Record<
      string,
      unknown
    >;
    return msg(name, { body });
  };

  it('is the title of a brief and the question of a decision', () => {
    expect(subject(msg('row-msg-brief-pending', { text: 'Here is the brief.' }))).toBe(
      'Show connection paths',
    );
    expect(subject(msg('row-msg-decision-pending'))).toBe('Where should the path appear?');
  });

  it('is the reason of an approval, or else the request itself', () => {
    expect(subject(msg('row-msg-approval-pending'))).toBe('Time the path change in a browser.');
    expect(subject(without('row-msg-approval-pending', 'why'))).toBe(
      'pnpm exec playwright install chromium',
    );
  });

  it('is the text of any other message', () => {
    expect(subject(text('a', 1))).toBe('The path is direct.');
    expect(subject(msg('row-msg-question'))).toBe('Should the path include latency?');
  });
});

describe('a home', () => {
  it('lists the groups of its node and reads each', () => {
    const home = new Home(me);
    expect([home.ctl, home.groups, home.name, home.owner]).toEqual([undefined, [], '', false]);
    expect(home.me).toBe(me);
    const { ctl, asked } = connection();
    home.online(ctl, welcome([listed('one', 'One'), listed('two', 'Two')]));
    expect(home.ctl).toBe(ctl);
    expect([home.name, home.owner]).toEqual(['workstation', true]);
    expect(home.groups.map((group) => [group.g, group.name])).toEqual([
      ['one', 'One'],
      ['two', 'Two'],
    ]);
    expect(asked.map((ask) => ask.req)).toEqual([
      { t: 'sync', g: 'one', since: 0, state_only: false },
      { t: 'sync', g: 'two', since: 0, state_only: false },
    ]);
  });

  it('says whose node it is not', () => {
    const home = new Home(me);
    home.online(connection().ctl, welcome([], { name: 'laptop', owner: false }));
    expect([home.name, home.owner]).toEqual(['laptop', false]);
  });

  it('shows its groups from the rows held while the node cannot be reached, and reads on when it returns', async () => {
    const { home, group } = await synced();
    home.push({ t: 'rows', g: website, rows: [text('a', 816)] });
    home.offline();
    expect(home.ctl).toBeUndefined();
    expect(home.groups).toEqual([group]);
    expect(group.main.map((msg) => msg.id)).toEqual(['a']);

    const again = connection();
    home.online(again.ctl, welcome([listed(website, 'Website 2')]));
    expect(home.groups).toEqual([group]);
    expect(group.name).toBe('Website 2');
    expect(again.asked.map((ask) => ask.req)).toEqual([
      { t: 'sync', g: website, since: 816, state_only: false },
    ]);
  });

  it('replaces its list at a groups push and reads only the group that is new', async () => {
    const { home, group, asked } = await synced();
    home.push({ t: 'groups', groups: [listed('new', 'Docs'), listed(website, 'Site')] });
    expect(home.groups.map((each) => [each.g, each.name])).toEqual([
      ['new', 'Docs'],
      [website, 'Site'],
    ]);
    expect(home.groups[1]).toBe(group);
    expect(asked.slice(1).map((ask) => ask.req)).toEqual([
      { t: 'sync', g: 'new', since: 0, state_only: false },
    ]);

    home.push({ t: 'groups', groups: [] });
    expect(home.groups).toEqual([]);
    expect(asked).toHaveLength(2);
  });

  it('gives each push to the group it names and drops the others', async () => {
    const home = new Home(me);
    home.online(connection().ctl, welcome([listed('one'), listed('two')]));
    const [one, two] = home.groups;
    home.push({ t: 'rows', g: 'two', rows: [text('a', 5)] });
    home.push({ t: 'rows', g: 'one', rows: [text('b', 6)] });
    home.push({ t: 'rows', g: 'gone', rows: [text('c', 7)] });
    home.push({ t: 'reset', g: 'gone' });
    home.push({ t: 'devlog', user, entries: [] });
    expect([...one.rows.keys()]).toEqual(['msg b']);
    expect([...two.rows.keys()]).toEqual(['msg a']);
    home.push({ t: 'reset', g: 'two' });
    expect([one.rows.size, two.rows.size]).toEqual([1, 0]);
  });
});

describe('commands', () => {
  beforeEach(() => {
    vi.useFakeTimers({ toFake: ['Date'] });
    vi.setSystemTime(1763600000000);
  });

  it('are signed by this device and answered by the node', async () => {
    const { home, group, asked } = joined();
    const sending = group.post('Hello @Codex', ['codex']);
    await vi.waitFor(() => expect(asked).toHaveLength(2));
    const payload = JSON.parse(signed[0]);
    expect(Object.keys(payload).slice(0, 4)).toEqual(['t', 'n', 'ts', 'g']);
    expect(payload).toEqual({
      t: 'post',
      n: expect.stringMatching(/^[A-Za-z0-9_-]{21}[AQgw]$/),
      ts: 1763600000000,
      g: website,
      kind: 'text',
      thread: null,
      main: false,
      text: 'Hello @Codex',
      mentions: ['codex'],
      re: null,
      body: {},
    });
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

  it('create a project group that the nonce names', async () => {
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
    const payload = JSON.parse(signed[0]);
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
    expect(p.decode(p.Id, JSON.stringify(payload.g))).toBe(payload.n);
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
    const { home, group, asked } = joined();
    const breaking = group.post('one', []);
    await vi.waitFor(() => expect(asked).toHaveLength(2));
    asked[1].fail();
    expect(await breaking).toEqual(lost());

    home.offline();
    expect(await group.post('two', [])).toEqual(lost());
    expect(signed).toHaveLength(2);
    expect(asked).toHaveLength(2);
  });
});

describe('every group', () => {
  const home = (...groups: string[]) => {
    const made = new Home(me);
    made.online(connection().ctl, welcome(groups.map((g) => listed(g, g))));
    homes.push(made);
    return made;
  };

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

  it('starts with no home', () => {
    expect(homesAtFirst).toEqual([]);
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
