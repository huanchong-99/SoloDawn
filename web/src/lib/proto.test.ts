import * as v from 'valibot';
import { describe, expect, it } from 'vitest';
import { examples, goldens, schemaOf } from './mock';
import * as p from './proto';

/** Fields of the examples that the contracts mark optional and Go tags omitempty: the only ones
 *  that may be left out. */
const omitempty: Record<string, string[]> = {
  'rv-ready': ['ice.username', 'ice.credential'],
  'rv-ice-answer': ['ice.username', 'ice.credential'],
  answer: ['to'],
  ice: ['to'],
  offer: ['to'],
  'cmd-pair-link': ['link', 'expires'],
  'cmd-reviewed': ['passed', 'round'],
  'cmd-started': ['num'],
  'device-add': ['kind', 'name'],
  'row-device': ['kind', 'name'],
  'row-group': ['repo', 'secretary'],
  'row-group-paused': ['repo', 'secretary'],
  'row-member-ai': ['task', ...usage()],
  'row-msg-brief-revise': ['state.note'],
  // A superseded brief keeps whatever the state it replaced held.
  'row-msg-brief-superseded': ['state.by', 'state.at', 'state.act', 'state.note'],
  'row-msg-decision-decided': [
    'body.task',
    'body.context',
    'body.recommended',
    'state.choice',
    'state.text',
  ],
  'row-msg-decision-pending': ['body.task', 'body.context', 'body.recommended'],
  'row-msg-checks': ['body.items.detail'],
  'row-msg-report': ['body.pr'],
  'row-msg-review': ['body.items.file', 'body.items.line'],
  'row-msg-summary': ['body.pr'],
  'row-msg-system': ['body.task', 'body.detail'],
  'row-task': [
    'brief',
    'commit',
    'reviewer',
    'branch',
    'pr',
    'pr_url',
    'review',
    'checks.local',
    'checks.cloud',
    'thread.last',
  ],
  'row-task-waiting': ['wait.msg', 'wait.who'],
};

function usage() {
  const keys = ['window_pct', 'window_resets', 'day_cap_pct', 'spend_today', 'spend_cap'];
  return [...keys, 'turns_today'].map((k) => `usage.${k}`);
}

type Obj = Record<string, unknown>;

/** Paths to every object, including objects inside arrays. */
function objects(value: unknown, path: string[] = []): string[][] {
  if (typeof value !== 'object' || value === null) return [];
  const inner = Object.entries(value).flatMap(([k, x]) => objects(x, [...path, k]));
  return Array.isArray(value) ? inner : [path, ...inner];
}

function schemaObjects(name: string) {
  // Room data belongs to the pairing peers and can contain arbitrary keys.
  return objects(JSON.parse(goldens[name])).filter(
    (path) => name !== 'rv-room' || path[0] !== 'data',
  );
}

const at = (value: unknown, path: string[]) => path.reduce((o, k) => (o as Obj)[k], value) as Obj;
const named = (path: string[]) => path.filter((k) => !/^\d+$/.test(k)).join('.');
const accepts = (name: string, value: unknown) => v.safeParse(schemaOf[name], value).success;

/** The example with one change made to a copy of the object at `path`. */
function variant(name: string, path: string[], change: (o: Obj) => void) {
  const value = JSON.parse(goldens[name]);
  change(at(value, path));
  return value;
}

describe('wire examples', () => {
  it('maps every example to a schema, and every schema to an example', () => {
    expect(examples.toSorted()).toEqual(Object.keys(schemaOf).toSorted());
  });

  it.each(examples)('%s parses to exactly its JSON', (name) => {
    expect(p.decode(schemaOf[name], goldens[name])).toStrictEqual(JSON.parse(goldens[name]));
  });

  it.each(examples)('%s drops an unknown field in any object', (name) => {
    for (const path of schemaObjects(name)) {
      const extra = variant(name, path, (o) => (o.extra = 1));
      const read = p.decode(schemaOf[name], JSON.stringify(extra));
      expect(read, `extra field at ${path.join('.')}`).toStrictEqual(JSON.parse(goldens[name]));
      // So a golden file with a field that the schema lacks fails the exact test above.
      expect(read, `extra field at ${path.join('.')}`).not.toStrictEqual(extra);
    }
  });

  it.each(examples)('%s is refused without a field that Go always writes', (name) => {
    // Rows inside rows and fetched are checked through their own examples.
    const paths = schemaObjects(name).filter((path) => path[0] !== 'rows');
    for (const path of paths) {
      for (const key of Object.keys(at(JSON.parse(goldens[name]), path))) {
        const field = named([...path, key]);
        const missing = variant(name, path, (o) => delete o[key]);
        const optional =
          (omitempty[name] ?? []).includes(field) ||
          (name.startsWith('row-msg-') && field === 'env') ||
          (name.startsWith('row-msg-approval-') && field === 'body.why');
        expect(accepts(name, missing), `without ${field}`).toBe(optional);
      }
    }
  });

  it('keeps a signed payload as the exact text received, leading space included', () => {
    const { entries } = p.decode(p.Devlog, goldens.devlog);
    expect(entries[1].p.slice(0, 15)).toBe(' {"t":"device",');
    expect(p.decode(p.DeviceRow, goldens['row-device']).env.p).toBe(entries[1].p);
  });

  it('refuses null for a field that Go omits when empty', () => {
    expect(
      accepts(
        'row-task',
        variant('row-task', [], (o) => (o.review = null)),
      ),
    ).toBe(false);
  });
});

const messageFiles = examples.filter((name) => name.startsWith('row-msg-'));

describe('message contracts', () => {
  it.each(messageFiles)('%s names its kind and card status', (name) => {
    const row = p.decode(p.MsgRow, goldens[name]);
    expect(name).toBe(['row-msg', row.kind, ...(row.state ? [row.state.status] : [])].join('-'));
  });

  it.each(messageFiles)('%s permits null only where the contract names it', (name) => {
    const row = JSON.parse(goldens[name]);
    const nullable = ['thread', 're'];
    if (row.state === null) nullable.push('state');
    if (row.kind === 'brief') nullable.push('body.replaces');
    for (const path of objects(row)) {
      for (const key of Object.keys(at(row, path))) {
        const field = named([...path, key]);
        const changed = variant(name, path, (o) => (o[key] = null));
        expect(accepts(name, changed), field).toBe(nullable.includes(field));
      }
    }
  });

  it.each([null, [], [1, 'a'], '', 0])('rejects a non-object empty body (%j)', (body) => {
    expect(
      accepts(
        'row-msg-text',
        variant('row-msg-text', [], (o) => (o.body = body)),
      ),
    ).toBe(false);
  });

  it('requires the state of the right card kind', () => {
    const row = JSON.parse(goldens['row-msg-call-waiting']);
    for (const state of [null, { status: 'pending' }, { status: 'new' }]) {
      expect(v.safeParse(p.MsgRow, { ...row, state }).success).toBe(false);
    }
    expect(v.safeParse(p.MsgRow, { ...row, kind: 'unknown' }).success).toBe(false);
    expect(v.safeParse(p.MsgRow, { ...row, kind: 'text', body: {} }).success).toBe(false);
  });

  it('keeps the state a superseded brief replaced, pending included', () => {
    const row = JSON.parse(goldens['row-msg-brief-superseded']);
    const { status, by, at, act } = row.state;
    for (const state of [{ status }, { status, by, at, act }]) {
      expect(v.parse(p.MsgRow, { ...row, state })).toStrictEqual({ ...row, state });
    }
  });

  it.each([0.5, Number.MAX_SAFE_INTEGER + 1])('rejects %s as a wire integer', (ms) => {
    const row = JSON.parse(goldens['row-msg-call-answered']);
    row.state.ms = ms;
    expect(v.safeParse(p.MsgRow, row).success).toBe(false);
  });

  it.each([
    { code: 'rounds_limit', task: 't', n: 3 },
    { code: 'turn_interrupted', ai: 'a', reason: 'crash', task: 't', detail: 'killed' },
    ...['group_paused', 'group_resumed'].map((code) => ({ code, by: 'm' })),
    { code: 'preempted', task: 't', by: 'm' },
    { code: 'cap_reached', ai: 'a', cap: 'day', task: 't' },
    { code: 'rate_limited', ai: 'a', resets: 17 },
    { code: 'login_invalid', ai: 'a', detail: 'signed out' },
    { code: 'task_failed', task: 't', reason: 'failures' },
    { code: 'task_merged', task: 't', commit: 'abc', merge_commit: 'def' },
    { code: 'no_secretary' },
    { code: 'session_replaced', ai: 'a', task: 't' },
  ])('reads the fields of system code $code', (body) => {
    const row = { ...JSON.parse(goldens['row-msg-system']), body };
    expect(v.parse(p.MsgRow, row)).toStrictEqual(row);
    const optional: Record<string, string[]> = {
      turn_interrupted: ['detail', 'task'],
      cap_reached: ['task'],
      login_invalid: ['detail'],
      session_replaced: ['task'],
    };
    for (const key of Object.keys(body)) {
      const missing: Obj = { ...body };
      delete missing[key];
      expect(v.safeParse(p.MsgRow, { ...row, body: missing }).success, key).toBe(
        (optional[body.code] ?? []).includes(key),
      );
    }
  });

  it('rejects an unknown system code', () => {
    const row = JSON.parse(goldens['row-msg-system']);
    expect(accepts('row-msg-system', { ...row, body: { code: 'unknown' } })).toBe(false);
  });
});

describe('rendezvous frames', () => {
  it('preserves the entire room data object and refuses other JSON types', () => {
    const room = JSON.parse(goldens['rv-room']);
    room.data = JSON.parse('{"constructor":{"Unknown":null},"__proto__":["pair"]}');
    expect(JSON.stringify(p.decode(p.RVRoom, JSON.stringify(room)))).toBe(JSON.stringify(room));
    for (const data of [null, false, 2, 'pair', []]) {
      expect(accepts('rv-room', { ...room, data })).toBe(false);
    }
  });

  it.each(['rv-join', 'rv-joined', 'rv-room'])('%s takes a 32-digit lowercase hex room', (name) => {
    const frame = JSON.parse(goldens[name]);
    const room = '0123456789abcdef'.repeat(2);
    expect(accepts(name, { ...frame, room })).toBe(true);
    for (const invalid of [
      room.slice(1),
      'z' + room,
      room + 'z',
      room.toUpperCase(),
      room.slice(1) + 'g',
    ]) {
      expect(accepts(name, { ...frame, room: invalid })).toBe(false);
    }
  });

  it('limits watch to 64 ids', () => {
    const watch = JSON.parse(goldens['rv-watch']);
    expect(accepts('rv-watch', { ...watch, k: Array(64).fill(watch.k[0]) })).toBe(true);
    expect(accepts('rv-watch', { ...watch, k: Array(65).fill(watch.k[0]) })).toBe(false);
  });

  it.each(['rv-ready', 'rv-ice-answer'])('%s reads ttl as a safe integer', (name) => {
    const frame = JSON.parse(goldens[name]);
    for (const ttl of [0.5, 2 ** 53]) {
      expect(accepts(name, { ...frame, ttl })).toBe(false);
    }
  });
});

describe('decode', () => {
  const frames = ['welcome', 'synced', 'fetched', 'cmd-ok', 'cmd-pair-link', 'cmd-error'];

  it.each([...frames, 'rows', 'reset', 'devlog'])('reads the %s frame', (name) => {
    expect(p.decode(p.Frame, goldens[name])).toStrictEqual(JSON.parse(goldens[name]));
  });

  it.each(Object.keys(schemaOf).filter((name) => name.startsWith('row-')))(
    'reads the %s example as a row',
    (name) => {
      expect(p.decode(p.Row, goldens[name])).toStrictEqual(JSON.parse(goldens[name]));
    },
  );

  it('reads the groups push, which has no wire example yet', () => {
    const groups = '{"t":"groups","groups":[{"g":"Qm1","name":"Website","seq":812}]}';
    expect(p.decode(p.Frame, groups)).toStrictEqual(JSON.parse(groups));
    expect(() => p.decode(p.Frame, '{"t":"groups","groups":[{"g":"Qm1"}]}')).toThrow(v.ValiError);
  });

  it('refuses a frame a node does not send', () => {
    expect(() => p.decode(p.Frame, goldens.hello)).toThrow(v.ValiError);
  });

  it('refuses text that is not JSON', () => {
    expect(() => p.decode(p.Frame, '{"t":')).toThrow(SyntaxError);
  });
});
