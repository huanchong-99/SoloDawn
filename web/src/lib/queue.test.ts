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

/** Who the message waits for, with the task. */
const said = (msg: p.MsgRow) => waiting(group, msg).map(({ ai, task }) => `${ai.id} T-${task.num}`);

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
    expect(said(msg)).toEqual(['codex T-12']);
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
    expect(waiting(without(gid), msg)).toMatchObject([{ ai: { id: 'codex' }, task: { num: 12 } }]);
  });

  it('is for the author only, for a text, while the group runs', () => {
    expect(said(say('other', { by: 'mina' }))).toEqual([]);
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
    expect(said(msg)).toEqual(['codex T-12']);
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
    expect(said(say('m1', { mentions: [] }))).toEqual(['secretary T-12']);
    expect(said(say('m2', { mentions: ['mina', 'gone'] }))).toEqual(['secretary T-12']);
    expect(said(say('m3', { mentions: ['codex'] }))).toEqual(['codex T-12']);
  });

  it('is every message, for every AI it names that is busy', () => {
    const older = say('m1');
    const newer = say('m2', { mentions: ['secretary', 'codex'] });
    expect(said(older)).toEqual(['codex T-12']);
    expect(said(newer)).toEqual(['secretary T-12', 'codex T-12']);
  });

  it('is a message in a thread for the AIs it names, unless its task holds it', () => {
    const msg = say('m1', { thread: task12 });
    expect(said(msg)).toEqual(['codex T-12']);
    expect(said(say('m2', { thread: task12, mentions: [] }))).toEqual([]);
    expect(said(say('m3', { thread: task12, mentions: ['secretary'] }))).toEqual([
      'secretary T-12',
    ]);
    write('row-task', { id: 'other', num: 13 });
    const states = { active: ['codex T-12'], queued: ['codex T-12'], paused: [], failed: [] };
    for (const [state, expected] of Object.entries(states)) {
      write('row-task', { id: 'other', num: 13, state });
      expect(said(say(`m-${state}`, { thread: 'other' })), state).toEqual(expected);
    }
    write('row-task', { id: task12, num: 12, state: 'paused' });
    expect(said(msg)).toEqual([]);
    expect(said(say('main'))).toEqual(['codex T-12']);
  });

  it('begins when the owner of the AI has confirmed it', () => {
    const msg = say('m1');
    confirm('c-other', 'm0', 'codex', 'pending');
    confirm('c-ai', 'm1', 'secretary', 'pending');
    write('row-msg-system', { id: 's', thread: null, re: 'm1', body: { ai: 'codex' } });
    expect(said(msg)).toEqual(['codex T-12']);
    confirm('c', 'm1', 'codex', 'pending');
    expect(said(msg)).toEqual([]);
    ai('codex');
    confirm('c', 'm1', 'codex', 'confirmed');
    expect(said(msg)).toEqual(['codex T-12']);
    confirm('c', 'm1', 'codex', 'denied');
    expect(said(msg)).toEqual([]);
  });

  it('counts a confirmed message from its confirmation', () => {
    const msg = say('m1');
    confirm('c', 'm1', 'codex', 'pending');
    ai('codex');
    confirm('c', 'm1', 'codex', 'confirmed');
    ai('codex');
    expect(said(msg)).toEqual([]);
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
