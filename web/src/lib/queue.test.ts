import { beforeEach, describe, expect, it } from 'vitest';
import { Group, Home } from './groups.svelte';
import { goldens } from './mock';
import * as p from './proto';
import { goFirst, waiting } from './queue';

const gid = 'rZNvy-1jH6Z-BcPqA5U5BQ';
const user: string = JSON.parse(goldens['row-member-human']).user;
const task12: string = JSON.parse(goldens['row-task']).id;
const signed: string[] = [];
const sign = async (payload: string) => {
  signed.push(payload);
  return { p: payload, k: 'dev-k', s: 'dev-s' };
};

let group: Group;
let seq = 0;

/** Writes a row as the home does: the next sequence number, which a new message has as `pos`. */
function write(name: string, changes: object = {}) {
  seq += 1;
  const row = { ...p.decode(p.Row, goldens[name]), seq, ...changes };
  const key = `${row.r} ${row.id}`;
  const held = group.rows.get(key) as p.MsgRow | undefined;
  group.apply([('pos' in row ? { ...row, pos: held?.pos ?? seq } : row) as p.Row]);
  return group.rows.get(key)!;
}

const ai = (id: string, changes: object = {}) =>
  write('row-member-ai', { id, name: id, state: 'working', task: task12, ...changes });
const say = (id: string, changes: object = {}) =>
  write('row-msg-text', { id, by: 'me', mentions: ['codex'], ...changes }) as p.MsgRow;
const confirm = (id: string, re: string, aiId: string, status: string) =>
  write('row-msg-confirm-pending', {
    id,
    by: 'system',
    re,
    thread: null,
    main: true,
    body: { what: 'direct', ai: aiId, to: [user] },
    state: { status },
  });

/** Who the message waits for, with the task, and a mark where it can go first. */
const said = (msg: p.MsgRow) =>
  waiting(group, msg).map(
    ({ wait, first }) => `${wait.ai.id} T-${wait.task!.num}${first ? ' first' : ''}`,
  );

beforeEach(() => {
  seq = 0;
  signed.length = 0;
  const home = new Home({ user, sign });
  home.ctl = { request: async () => ({ re: 1, ok: true }) } as unknown as p.Ctl;
  group = new Group(home, gid);
  write('row-group', { id: gid });
  write('row-member-human', { id: 'me', user });
  write('row-task', { id: task12, num: 12 });
  ai('codex');
  ai('secretary', { secretary: true });
});

describe('waiting', () => {
  it('is a message of the person to an AI that has been busy with a task since before it', () => {
    const msg = say('m1');
    expect(said(msg)).toEqual(['codex T-12 first']);
    expect(waiting(group, msg)[0].wait).toMatchObject({ msg: { id: 'm1' }, at: msg.seq });
  });

  it('is over when the row of the AI changes after the message', () => {
    const msg = say('m1');
    ai('codex');
    expect(said(msg)).toEqual([]);
    // Not older than the message is not before it.
    const next = say('m2');
    ai('codex', { seq: next.seq });
    expect(said(next)).toEqual([]);
  });

  it('waits for the rows of the group and of the person', () => {
    const msg = say('m1');
    const rows = [...group.rows.values()];
    const without = (id: string) => {
      const part = new Group(group.home, gid);
      part.apply(rows.filter((row) => row.id !== id));
      return part;
    };
    expect(waiting(without('me'), msg)).toEqual([]);
    expect(waiting(without(gid), msg)).toMatchObject([{ first: true }]);
  });

  it('is for the author only, for a text in the main line, while the group runs', () => {
    expect(said(say('other', { by: 'mina' }))).toEqual([]);
    expect(said(say('thread', { thread: task12 }))).toEqual([]);
    expect(
      said(
        write('row-msg-progress', {
          id: 'p',
          by: 'me',
          thread: null,
          mentions: ['codex'],
        }) as p.MsgRow,
      ),
    ).toEqual([]);
    const msg = say('m1');
    expect(said(msg)).toEqual(['codex T-12 first']);
    write('row-group-paused', { id: gid });
    expect(said(msg)).toEqual([]);
  });

  it('is for an AI that is busy with a task the client holds, and that a node starts', () => {
    for (const changes of [
      { state: 'waiting' },
      { state: 'dormant' },
      { state: 'limited' },
      { state: 'offline' },
      { task: undefined },
      { task: 'unknown' },
      { cli: 'external' },
    ]) {
      ai('codex', changes);
      expect(said(say(`m-${seq}`)), JSON.stringify(changes)).toEqual([]);
    }
  });

  it('goes to the secretary when it mentions no AI', () => {
    expect(said(say('m1', { mentions: [] }))).toEqual(['secretary T-12 first']);
    expect(said(say('m2', { mentions: ['mina', 'gone'] }))).toEqual(['secretary T-12 first']);
    expect(said(say('m3', { mentions: ['codex'] }))).toEqual(['codex T-12 first']);
  });

  it('lets only the newest that waits go first, and of two AIs the one named last', () => {
    const older = say('m1');
    const newer = say('m2', { mentions: ['secretary', 'codex'] });
    expect(said(older)).toEqual(['codex T-12']);
    expect(said(newer)).toEqual(['secretary T-12', 'codex T-12 first']);
    // A newer one that an AI has not taken for another reason is the one a jump would move.
    ai('off', { state: 'offline', task: undefined });
    for (const state of ['takeover', 'limited', 'offline', 'paused']) {
      ai('off', { state, task: undefined });
      const id = `m-${state}`;
      say(id, { mentions: ['off'] });
      expect(said(newer), state).toEqual(['secretary T-12', 'codex T-12']);
      ai('off', { state: 'waiting', task: undefined });
      expect(said(newer), state).toEqual(['secretary T-12', 'codex T-12 first']);
    }
    // One of another person's does not count.
    say('theirs', { by: 'mina' });
    expect(said(newer)).toEqual(['secretary T-12', 'codex T-12 first']);
  });

  it('begins when the owner of the AI has confirmed it', () => {
    const msg = say('m1');
    confirm('c-other', 'm0', 'codex', 'pending');
    confirm('c-ai', 'm1', 'secretary', 'pending');
    write('row-msg-system', { id: 's', thread: null, re: 'm1', body: { ai: 'codex' } });
    expect(said(msg)).toEqual(['codex T-12 first']);
    confirm('c', 'm1', 'codex', 'pending');
    expect(said(msg)).toEqual([]);
    ai('codex');
    confirm('c', 'm1', 'codex', 'confirmed');
    expect(said(msg)).toEqual(['codex T-12 first']);
    confirm('c', 'm1', 'codex', 'denied');
    expect(said(msg)).toEqual([]);
  });

  it('counts a confirmed message from its confirmation', () => {
    const first = say('m1');
    confirm('c', 'm1', 'codex', 'pending');
    const second = say('m2', { mentions: ['secretary'] });
    expect(said(second)).toEqual(['secretary T-12 first']);
    confirm('c', 'm1', 'codex', 'confirmed');
    expect(said(second)).toEqual(['secretary T-12']);
    expect(said(first)).toEqual(['codex T-12 first']);
  });
});

describe('goFirst', () => {
  it('sends jump for the message', async () => {
    const msg = say('m1');
    expect(await goFirst(group, msg)).toEqual({ re: 1, ok: true });
    expect(JSON.parse(signed[0])).toEqual({
      t: 'jump',
      n: expect.any(String),
      ts: expect.any(Number),
      g: gid,
      msg: 'm1',
    });
  });
});
