import * as v from 'valibot';
import { describe, expect, it } from 'vitest';
import { examples, goldens, mockCtl, schemaOf } from './mock';
import * as p from './proto';

/** What the two readers of the wire agree on: internal/proto/testdata/wire.json, which the Go
 *  tests write from the tags of the Go types and judge `proto.Decode` by. For the members each
 *  example has, `optional` may be left out, `nullable` may be null, `fraction` are the numbers that
 *  need not be integers, `id` the texts that are random ids, and `opaque` the members that the Go
 *  type of the frame keeps as JSON. A path names its members with dots and leaves out the places
 *  in a list. `cases` are examples with one value replaced. */
interface Wire {
  shapes: Record<string, Record<'optional' | 'nullable' | 'fraction' | 'id' | 'opaque', string[]>>;
  cases: { name: string; file: string; at: string[]; value: unknown; ok: boolean }[];
}

const wire: Wire = JSON.parse(goldens.wire);

type Obj = Record<string, unknown>;

/** Paths to every object, including objects inside arrays. */
function objects(value: unknown, path: string[] = []): string[][] {
  if (typeof value !== 'object' || value === null) return [];
  const inner = Object.entries(value).flatMap(([k, x]) => objects(x, [...path, k]));
  return Array.isArray(value) ? inner : [path, ...inner];
}

const at = (value: unknown, path: string[]) => path.reduce((o, k) => (o as Obj)[k], value) as Obj;
const named = (path: string[]) => path.filter((k) => !/^\d+$/.test(k)).join('.');
const accepts = (name: string, value: unknown) =>
  v.safeParse(schemaOf[name], value, { abortEarly: true }).success;

/** The example with one change made to a copy of the object at `path`. */
function variant(name: string, path: string[], change: (o: Obj) => void) {
  const value = JSON.parse(goldens[name]);
  change(at(value, path));
  return value;
}

/** The example with the value at `path` replaced, or left out when there is no `change`. */
function replaced(name: string, path: string[], change?: () => unknown) {
  const key = path[path.length - 1];
  return variant(name, path.slice(0, -1), (parent) => {
    if (change) parent[key] = change();
    else delete parent[key];
  });
}

describe('wire examples', () => {
  it('maps every example to a schema, and every schema to an example', () => {
    expect(examples.toSorted()).toEqual(Object.keys(schemaOf).toSorted());
  });

  it.each(examples)('%s parses to exactly its JSON', (name) => {
    expect(p.decode(schemaOf[name], goldens[name])).toStrictEqual(JSON.parse(goldens[name]));
  });

  it.each(examples)('%s drops an unknown field in any object', (name) => {
    for (const path of objects(JSON.parse(goldens[name]))) {
      const extra = variant(name, path, (o) => (o.extra = 1));
      const read = p.decode(schemaOf[name], JSON.stringify(extra));
      expect(read, `extra field at ${path.join('.')}`).toStrictEqual(JSON.parse(goldens[name]));
      // So a golden file with a field that the schema lacks fails the exact test above.
      expect(read, `extra field at ${path.join('.')}`).not.toStrictEqual(extra);
    }
  });

  it('has the shared facts of every example, and of nothing else', () => {
    expect(Object.keys(wire.shapes).toSorted()).toEqual(examples.toSorted());
  });

  // The Go tests make the same changes and expect the same of proto.Decode.
  it.each(examples)('%s is read as the shared facts say, whichever value is changed', (name) => {
    const facts = wire.shapes[name];
    const read = (what: string, path: string[], want: boolean, change?: () => unknown) =>
      expect(accepts(name, replaced(name, path, change)), `${named(path)} ${what}`).toBe(want);
    const refused = (what: string, path: string[], value: unknown) => {
      // The whole example is not replaced.
      if (path.length > 0) read(what, path, false, () => value);
    };
    const visitArray = (value: unknown[], path: string[]) => {
      refused('an object', path, {});
      value.forEach((element, i) => {
        const inner = [...path, `${i}`];
        read('null in the list', inner, false, () => null);
        visit(element, inner);
      });
    };
    const visitObject = (value: Obj, path: string[]) => {
      refused('a list', path, []);
      for (const [key, member] of Object.entries(value)) {
        const inner = [...path, key];
        const innerField = named(inner);
        read('left out', inner, facts.optional.includes(innerField));
        if (!facts.opaque.includes(innerField)) {
          read('null', inner, facts.nullable.includes(innerField), () => null);
        }
        visit(member, inner);
      }
    };
    const scalars: Record<string, (field: string, path: string[]) => void> = {
      string: (field, path) => {
        refused('a number', path, 7);
        if (facts.id.includes(field)) refused('a text that is no id', path, 'system');
      },
      number: (field, path) => {
        const fraction = facts.fraction.includes(field);
        refused('a text', path, '7');
        read('a fraction', path, fraction, () => 0.5);
        read('2^53', path, fraction, () => 2 ** 53);
      },
      boolean: (_, path) => {
        refused('a text', path, 'true');
      },
    };
    const visit = (value: unknown, path: string[]) => {
      const field = named(path);
      if (value === null || facts.opaque.includes(field)) return;
      if (Array.isArray(value)) return visitArray(value, path);
      if (typeof value === 'object') return visitObject(value as Obj, path);
      scalars[typeof value](field, path);
    };
    expect(accepts(name, JSON.parse(goldens[name]))).toBe(true);
    visit(JSON.parse(goldens[name]), []);
  });

  it.each(wire.cases)('$file: $name', ({ file, at, value, ok }) => {
    expect(
      accepts(
        file,
        replaced(file, at, () => value),
      ),
    ).toBe(ok);
  });

  // JSON.parse reads a number beyond the largest double as Infinity; proto.Decode refuses it.
  it.each(['1e400', '-1e400'])('refuses %s in a fraction and in an integer field', (literal) => {
    for (const field of ['window_pct', 'turns_today']) {
      const text = goldens['row-member-ai'].replace(/("window_pct": )[^,]+/, `$1${literal}`);
      const changed =
        field === 'window_pct'
          ? text
          : goldens['row-member-ai'].replace(/("turns_today": )\d+/, `$1${literal}`);
      expect(changed, field).not.toBe(goldens['row-member-ai']);
      expect(() => p.decode(p.AIMemberRow, changed), field).toThrow(v.ValiError);
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

  it.each([0.5, Number.MAX_SAFE_INTEGER + 1, Number.MIN_SAFE_INTEGER - 1])(
    'refuses %s as a payload timestamp or device-log version',
    (value) => {
      expect(accepts('offer', { ...JSON.parse(goldens.offer), ts: value })).toBe(false);
      expect(accepts('device-add', { ...JSON.parse(goldens['device-add']), ver: value })).toBe(
        false,
      );
    },
  );
});

describe('Id', () => {
  const is = (text: unknown) => v.is(p.Id, text);
  const encode = (bytes: number[]) =>
    btoa(String.fromCharCode(...bytes))
      .replaceAll('+', '-')
      .replaceAll('/', '_')
      .replaceAll('=', '');
  const id = 'Zk3-Zk3_Zk3aZk3aZk30Zg';

  it('is what 16 bytes encode to in base64url without padding', () => {
    for (let b = 0; b < 256; b++) {
      for (const at of [0, 7, 15]) {
        const bytes = new Array<number>(16).fill(0);
        bytes[at] = b;
        expect(is(encode(bytes)), encode(bytes)).toBe(true);
      }
    }
    expect(is(id)).toBe(true);
    expect(is(encode(new Array<number>(16).fill(255)))).toBe(true);
  });

  it('ends on one of the four characters whose unused bits are zero', () => {
    const alphabet = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_';
    const last = [...alphabet].filter((c) => is(id.slice(0, 21) + c));
    expect(last.join('')).toBe('AQgw');
  });

  it.each([
    ['an empty text', ''],
    ['21 characters', id.slice(1)],
    ['23 characters, one before', 'A' + id],
    ['23 characters, one after', id + 'A'],
    ['padding', id + '=='],
    ['a line break after it', id + '\n'],
    ['a line break before it', '\n' + id],
    ['the standard alphabet', '++++++++++++++++++++/w'],
    ['a character outside the alphabet at the start', '.' + id.slice(1)],
    ['a character outside the alphabet near the end', id.slice(0, 20) + '.' + id.slice(21)],
    ['a lone surrogate', '\ud800'],
    ['22 characters outside the alphabet', 'é'.repeat(21) + 'w'],
    ['a device id', 'akWV_5PtNLIGhakuMAvpD9LcG1D8TUfc1URh48VREcs'],
    ['a value that only some fields allow', 'system'],
    ['a number', 5],
    ['null', null],
    ['a list of one id', [id]],
  ])('is not %s', (_, text) => {
    expect(is(text)).toBe(false);
  });

  it('is the type of n, g and conn in a signed payload, and not of every field', () => {
    const offer = JSON.parse(goldens.offer);
    expect(accepts('offer', offer)).toBe(true);
    expect(accepts('offer', { ...offer, g: id })).toBe(true);
    expect(accepts('offer', { ...offer, n: 'offer-1' })).toBe(false);
    expect(accepts('offer', { ...offer, g: 'Website' })).toBe(false);
    expect(accepts('offer', { ...offer, g: '' })).toBe(false);
    expect(accepts('offer', { ...offer, to: 'anyone' })).toBe(true);
    // A conn that is no random id is refused, as internal/peer/signal.go refuses it.
    expect(accepts('offer', { ...offer, conn: 'c1' })).toBe(false);
    for (const name of ['answer', 'ice']) {
      const signal = JSON.parse(goldens[name]);
      expect(accepts(name, signal)).toBe(true);
      expect(accepts(name, { ...signal, conn: 'c' })).toBe(false);
    }
    // The by of a message row may be "system", and a device row's id is no random id.
    const msg = JSON.parse(goldens['row-msg-system']);
    expect(msg.by).toBe('system');
    expect(accepts('row-msg-system', msg)).toBe(true);
    const device = p.decode(p.DeviceRow, goldens['row-device']);
    expect(device.id).toBe(`${device.user}:${device.ver}`);
  });
});

const messageFiles = examples.filter((name) => name.startsWith('row-msg-'));

describe('message contracts', () => {
  it.each(messageFiles)('%s names its kind and card status', (name) => {
    const row = p.decode(p.MsgRow, goldens[name]);
    const suffix =
      row.kind === 'brief' && row.body.preset !== undefined
        ? ['preset']
        : row.state
          ? [row.state.status]
          : [];
    expect(name).toBe(['row-msg', row.kind, ...suffix].join('-'));
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
    { code: 'task_done', task: 't' },
    { code: 'no_secretary' },
    { code: 'session_replaced', ai: 'a', task: 't' },
    { code: 'member_joined', member: 'm2', by: 'm1' },
    { code: 'member_removed', user: 'u2', name: 'Mina', by: 'm1' },
  ])('reads the fields of system code $code', (body) => {
    const row = { ...JSON.parse(goldens['row-msg-system']), body };
    expect(v.parse(p.MsgRow, row)).toStrictEqual(row);
    const optional: Record<string, string[]> = {
      turn_interrupted: ['detail', 'task'],
      cap_reached: ['task'],
      login_invalid: ['detail'],
      session_replaced: ['task'],
      member_removed: ['by'],
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
  it.each(['rv-join', 'rv-join-host', 'rv-joined', 'rv-leave', 'rv-room'])(
    '%s takes a 32-digit lowercase hex room',
    (name) => {
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
    },
  );

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

  it.each(['used', 'limit'])('reads quota %s as a safe integer', (field) => {
    const frame = JSON.parse(goldens['rv-quota-answer']);
    for (const n of [0.5, 2 ** 53]) {
      expect(accepts('rv-quota-answer', { ...frame, [field]: n })).toBe(false);
    }
  });
});

describe('decode', () => {
  it('reads watch success, moved errors and the mock watch answer', async () => {
    expect(p.decodeAnswer('watch.start', '{"re":7,"ok":true}')).toStrictEqual({ re: 7, ok: true });
    expect(p.decodeAnswer('sync', goldens['cmd-moved'])).toStrictEqual(
      JSON.parse(goldens['cmd-moved']),
    );
    const request = p.decode(p.WatchStart, goldens['watch-start']);
    const ctl = mockCtl(() => {});
    expect(await ctl.request(request)).toStrictEqual({ re: 1, ok: true });
    expect(() => p.decodeAnswer('watch.start', '{"re":7}')).toThrow(v.ValiError);
  });

  /** Each answer example, and the type of the request it answers. */
  const answers: [string, p.RequestType][] = [
    ['welcome', 'hello'],
    ['synced', 'sync'],
    ['fetched', 'fetch'],
    ['cmd-ok', 'cmd'],
    ['cmd-ok', 'watch.start'],
    ['cmd-started', 'cmd'],
    ['cmd-reviewed', 'cmd'],
    ['cmd-pair-link', 'cmd'],
    ['cmd-invite-link', 'cmd'],
    ['cmd-noted', 'cmd'],
    ['host-info-answer', 'host.info'],
    ['host-workspaces-answer', 'host.workspaces'],
  ];
  const pushes = ['rows', 'reset', 'devlog', 'groups', 'host-info-push'];

  it('has an answer example for every request type', () => {
    expect([...new Set(answers.map(([, t]) => t))].toSorted()).toEqual(
      Object.keys(p.AnswerOf).toSorted(),
    );
  });

  it.each(answers)('reads %s as the answer to %s', (name, t) => {
    expect(p.decodeAnswer(t, goldens[name])).toStrictEqual(JSON.parse(goldens[name]));
    expect(schemaOf[name]).toBe(p.AnswerOf[t]);
  });

  it.each(Object.keys(p.AnswerOf) as p.RequestType[])(
    'reads an error as the answer to %s, whatever else it holds',
    (t) => {
      const error = JSON.parse(goldens['cmd-error']);
      expect(p.decodeAnswer(t, goldens['cmd-error'])).toStrictEqual(error);
      const text = JSON.stringify({ ...error, ok: true, t: 'welcome' });
      expect(p.decodeAnswer(t, text)).toStrictEqual(error);
    },
  );

  // Only an object can be an error answer; any other JSON value is refused by the answer schema,
  // a ValiError like any frame that does not match, never a TypeError of looking for `err` in it.
  it.each(Object.keys(p.AnswerOf) as p.RequestType[])(
    'refuses a JSON value that is no object as the answer to %s',
    (t) => {
      for (const text of ['null', '4', '"err"', '[]', '[{"err":{}}]', 'true']) {
        expect(() => p.decodeAnswer(t, text), text).toThrow(v.ValiError);
      }
    },
  );

  it.each([...pushes, 'cmd-error'])('reads the %s frame, which needs no request', (name) => {
    expect(p.decode(p.Frame, goldens[name])).toStrictEqual(JSON.parse(goldens[name]));
  });

  it.each(Object.keys(schemaOf).filter((name) => name.startsWith('row-')))(
    'reads the %s example as a row',
    (name) => {
      expect(p.decode(p.Row, goldens[name])).toStrictEqual(JSON.parse(goldens[name]));
    },
  );

  it('reads the groups push with the entries of welcome', () => {
    const groups = p.decode(p.Groups, goldens.groups);
    expect(groups.groups).toStrictEqual(p.decode(p.Welcome, goldens.welcome).groups);
    expect(() => p.decode(p.Frame, '{"t":"groups","groups":[{"g":"Qm1"}]}')).toThrow(v.ValiError);
  });

  it('reads host.info as an answer by its request, and as a push without one', () => {
    const answer = JSON.parse(goldens['host-info-answer']);
    const push = JSON.parse(goldens['host-info-push']);
    expect(p.decode(p.Frame, goldens['host-info-push'])).toStrictEqual(push);
    expect(() => p.decodeAnswer('host.info', goldens['host-info-push'])).toThrow(v.ValiError);
    expect(() => p.decodeAnswer('host.info', JSON.stringify({ ...answer, ok: false }))).toThrow(
      v.ValiError,
    );
  });

  // Read by the request they answer, the answer to cmd ignores what it does not read,
  // as Go does (contracts/conventions.md section 2).
  it('ignores in a cmd answer the members it does not read, rows and t included', () => {
    for (const extra of [{ rows: null }, { rows: [{ ts: 1 }] }, { t: 'sync' }, { t: null }]) {
      const text = JSON.stringify({ re: 4, ok: true, ...extra });
      expect(p.decodeAnswer('cmd', text), text).toStrictEqual({ re: 4, ok: true });
    }
  });

  it.each(answers)('refuses %s, the answer to %s, without one of its fields', (name, t) => {
    for (const key of Object.keys(JSON.parse(goldens[name]))) {
      const missing = JSON.parse(goldens[name]);
      delete missing[key];
      const read = () => p.decodeAnswer(t, JSON.stringify(missing));
      if (schemaOf[name] === p.CmdResult && !['re', 'ok'].includes(key)) {
        expect(read, `without ${key}`).not.toThrow();
      } else {
        expect(read, `without ${key}`).toThrow(v.ValiError);
      }
    }
  });

  it('reads a node version as the release tag it was built from (contracts/local.md)', () => {
    expect(p.decode(p.Welcome, goldens.welcome).version).toMatch(/^v\d+\.\d+\.\d+$/);
  });

  it('refuses a frame a node does not send, and an answer as a push', () => {
    expect(() => p.decode(p.Frame, goldens.hello)).toThrow(v.ValiError);
    for (const name of ['welcome', 'synced', 'fetched', 'cmd-ok']) {
      expect(() => p.decode(p.Frame, goldens[name]), name).toThrow(v.ValiError);
    }
  });

  it('refuses text that is not JSON', () => {
    expect(() => p.decode(p.Frame, '{"t":')).toThrow(SyntaxError);
  });
});

describe('objects and errors as Go reads them', () => {
  it('refuses an array for usage, whose fields are all optional', () => {
    const member = JSON.parse(goldens['row-member-ai']);
    expect(accepts('row-member-ai', member)).toBe(true);
    expect(accepts('row-member-ai', { ...member, usage: [] })).toBe(false);
    expect(accepts('row-member-ai', { ...member, usage: {} })).toBe(true);
  });

  it('reads a frame with err as an error, whatever ok says', () => {
    const text = '{"re":1,"ok":true,"err":{"code":"conflict","msg":"Decided."}}';
    expect(p.decode(p.Frame, text)).toStrictEqual({
      re: 1,
      err: { code: 'conflict', msg: 'Decided.' },
    });
    expect(v.is(p.CmdResult, JSON.parse(text))).toBe(false);
    expect(v.is(p.CmdResult, { re: 1, ok: true })).toBe(true);
    expect(() => p.decode(p.Frame, '{"re":1,"ok":true,"err":"conflict"}')).toThrow();
  });
});

describe('commands', () => {
  const posts = examples.filter((name) => name.startsWith('post-'));

  it.each(posts)('%s is the post its message row carries, signed', (name) => {
    const post = p.decode(p.Post, goldens[name]);
    const person = p.decode(p.HumanMemberRow, goldens['row-member-human']);
    expect(name).toBe(`post-${post.kind}`);
    const rows = examples
      .filter((row) => row.startsWith(`row-msg-${post.kind}`))
      .map((row) => p.decode(p.MsgRow, goldens[row]))
      .filter((row) => row.by !== 'system')
      .filter((row) => row.kind !== 'brief' || row.body.preset === undefined);
    expect(rows.length).toBeGreaterThan(0);
    for (const row of rows) {
      expect(JSON.parse(row.env!.p)).toStrictEqual(post);
      expect([
        post.n,
        post.as ?? person.id,
        post.text,
        post.thread,
        post.main,
        post.mentions,
        post.re,
      ]).toEqual([row.id, row.by, row.text, row.thread, row.main, row.mentions, row.re]);
    }
  });

  it.each(['task', 'checks', 'bulletin', 'system', 'unknown'])(
    'refuses a post of kind %s',
    (kind) => {
      const post = JSON.parse(goldens['post-text']);
      expect(v.is(p.Post, post)).toBe(true);
      expect(v.is(p.Post, { ...post, kind })).toBe(false);
    },
  );

  it('mentions the people who may decide an approval', () => {
    const row = p.decode(p.MsgRow, goldens['row-msg-approval-pending']);
    const person = p.decode(p.HumanMemberRow, goldens['row-member-human']);
    expect(row.mentions).toEqual([person.id]);
    expect(row.kind === 'approval' && row.body.approvers).toEqual([person.user]);
  });

  it.each(examples.filter((name) => name.startsWith('act-')))('%s has no empty field', (name) => {
    const act = p.decode(p.Act, goldens[name]);
    expect(name).toBe(`act-${act.action}`);
    expect(Object.values(act)).not.toContain('');
    const has = Object.keys(act).filter((key) => key === 'arg' || key === 'text');
    expect(has).toEqual({ choose: ['arg', 'text'], revise: ['text'] }[act.action] ?? []);
  });

  // A host event is the body of a system message without ai, which the home adds.
  it.each(examples.filter((name) => name.startsWith('member-status-')))(
    '%s reports a fact that is a system body once it has ai',
    (name) => {
      const status = p.decode(p.MemberStatus, goldens[name]);
      expect(name).toBe(`member-status-${status.event!.code}`);
      const row = JSON.parse(goldens['row-msg-system']);
      expect(v.is(p.MsgRow, { ...row, body: { ...status.event, ai: status.id } })).toBe(true);
      expect(v.is(p.MsgRow, { ...row, body: status.event })).toBe(false);
    },
  );

  it('refuses a host event with a code the contract does not have', () => {
    const status = JSON.parse(goldens['member-status-rate_limited']);
    expect(accepts('member-status', status)).toBe(true);
    expect(accepts('member-status', { ...status, event: { code: 'task_done' } })).toBe(false);
  });

  // commands.md 1.7: off: true ends the reminders, whatever else the payload holds.
  it('reads a push as off only when off is true', () => {
    const on = JSON.parse(goldens['push-on']);
    for (const off of [false, 'true', 1]) {
      expect(accepts('push-off', { ...on, off })).toBe(false);
      expect(accepts('push-on', { ...on, off })).toBe(true);
    }
    expect(accepts('push-off', { ...on, off: true })).toBe(true);
  });

  it.each(examples.filter((name) => /^(group|member|task)-/.test(name)))(
    '%s is named by its op, which selects its schema',
    (name) => {
      const command = JSON.parse(goldens[name]);
      expect(name.split('-').slice(0, 2).join('-')).toBe(`${command.t}-${command.op}`);
      expect(accepts(name, { ...command, op: 'unknown' })).toBe(false);
    },
  );
});

describe('host surface', () => {
  it('reads the profile golden files with their command and row schemas', () => {
    expect(schemaOf['member-add_ai-profile']).toBe(p.MemberAddAI);
    expect(schemaOf['member-set-profile']).toBe(p.MemberSet);
    expect(schemaOf['row-task-stuck']).toBe(p.TaskRow);
    const row = p.decode(p.AIMemberRow, goldens['row-member-ai']);
    expect([row.model, row.effort, row.tier, row.skills]).toEqual([
      'gpt-5.4',
      'high',
      'strong',
      'Go backend, protocol design',
    ]);
    expect(p.decode(p.TaskRow, goldens['row-task-stuck']).stuck).toEqual({
      kind: 'review',
      n: 2,
      ask: true,
    });
    expect(p.decode(p.TaskRow, goldens['row-task']).stuck).toBeUndefined();
    expect(p.decode(p.HostInfo, goldens['host-info-answer']).agents.map((a) => a.models)).toEqual([
      [
        {
          model: 'opus',
          name: 'Opus',
          efforts: ['low', 'medium', 'high', 'xhigh', 'max'],
          fast: true,
          tier: 'strong',
        },
        { model: 'haiku', name: 'Haiku', efforts: [], tier: 'light' },
      ],
      [],
      [],
    ]);
    expect(p.decode(p.HostInfo, goldens['host-info-answer']).agents.map((a) => a.defaults)).toEqual(
      [
        { model: 'opus', effort: 'high', tier: 'strong', skills: 'Go backend' },
        undefined,
        undefined,
      ],
    );
    expect(p.decode(p.AIMemberRow, goldens['row-member-ai-follow']).follow).toBe(true);
    expect(p.decode(p.MemberAddAI, goldens['member-add_ai-follow']).follow).toBe(true);
    expect(p.decode(p.MemberSet, goldens['member-set-follow']).follow).toBe(true);
    expect(p.decode(p.MemberSet, goldens['member-set-follow-host']).follow).toBe(true);
    expect(p.decode(p.HostJoin, goldens['host-join-follow']).follow).toBe(true);
    expect(p.decode(p.AgentDefaults, goldens['agent-defaults'])).toMatchObject({
      t: 'agent.defaults',
      agent: 'claude',
      model: 'opus',
      effort: 'high',
      tier: 'strong',
      skills: 'Go backend, protocol design',
    });
    for (const [name, schema] of [
      ['row-member-ai-follow', p.AIMemberRow],
      ['member-add_ai-follow', p.MemberAddAI],
      ['member-set-follow', p.MemberSet],
      ['host-join-follow', p.HostJoin],
    ] as const) {
      const bad = { ...JSON.parse(goldens[name]), follow: false };
      expect(v.safeParse(schema, bad).success, name).toBe(false);
    }
  });

  // A host command has `to` and a `t` with a dot; the frames of the host surface have no `to`.
  const hostCommands = examples.filter((name) => {
    const { t, to } = JSON.parse(goldens[name]);
    return typeof t === 'string' && t.includes('.') && to !== undefined;
  });

  it.each(hostCommands)('%s is named by its t, and is for one node', (name) => {
    const command = JSON.parse(goldens[name]);
    expect(name.replace(/-(external|follow)$/, '')).toBe(command.t.replace('.', '-'));
    expect(command.to).toBe(p.decode(p.Welcome, goldens.welcome).device);
    expect(accepts(name, { ...command, t: 'host.unknown' })).toBe(false);
  });

  it('holds the agent of host.info as settings.set and agent.set name it', () => {
    const info = p.decode(p.HostInfo, goldens['host-info-answer']);
    const set = p.decode(p.SettingsSet, goldens['settings-set']);
    const managed = info.agents.filter((agent) => agent.agent !== 'external');
    expect(managed.map((agent) => agent.agent)).toEqual(Object.keys(set.caps!));
    expect(managed.map((agent) => agent.account)).toEqual(['personal', null]);
    expect(p.decode(p.AgentSet, goldens['agent-set'])).toMatchObject({ agent: 'codex' });
    expect(info.homes).toEqual([
      { g: info.agents[0].groups[0].g, home: info.device, name: 'Website' },
    ]);
    expect(managed.map((agent) => agent.driver)).toEqual([
      {
        accounts: ['personal', 'team', 'api_key'],
        takeover: 'new',
      },
      {
        accounts: ['personal', 'team', 'api_key'],
        takeover: 'resume',
      },
    ]);
  });

  it('matches the external host agent to its join command and member row', () => {
    const info = p.decode(p.HostInfo, goldens['host-info-answer']);
    const join = p.decode(p.HostJoin, goldens['host-join-external']);
    const add = p.decode(p.MemberAddAI, goldens['member-add_ai-external']);
    const row = p.decode(p.AIMemberRow, goldens['row-member-ai-external']);
    const set = p.decode(p.MemberSet, goldens['member-set-tool']);
    const external = info.agents.find((agent) => agent.agent === row.cli);
    expect(info.exe).toBe('/usr/local/bin/solodawn');
    expect(join).toMatchObject({ agent: row.cli, account: row.account, tool: row.tool });
    expect(add).toMatchObject({ id: row.id, cli: row.cli, account: row.account, tool: row.tool });
    expect(set).toMatchObject({ id: row.id, tool: '' });
    expect(external).toStrictEqual({
      agent: 'external',
      cli_version: '',
      account: null,
      login_ok: true,
      caps: {},
      driver: { accounts: ['personal', 'team', 'api_key'], takeover: 'none' },
      groups: [
        {
          g: row.g,
          member: row.id,
          workspace: row.workspace,
          level: 3,
          state: row.state,
          queued: 0,
          watchers: [],
        },
      ],
      models: [],
    });
  });

  it('reads the step-2 invite link and the act a decided confirm card carries', () => {
    const invite = p.decode(p.Invite, goldens.invite);
    const inviteLink = p.decode(p.CmdResult, goldens['cmd-invite-link']);
    const welcome = p.decode(p.Welcome, goldens.welcome);
    expect(inviteLink.link).toBe(
      `https://rv.example/#join=7K2M-9QXD-4F8T-WB3N&g=${invite.g}&home=${welcome.device}`,
    );
    for (const [example, action] of [
      ['row-msg-confirm-confirmed', 'confirm'],
      ['row-msg-confirm-denied', 'deny'],
    ] as const) {
      const row = p.decode(p.MsgRow, goldens[example]);
      if (row.kind !== 'confirm' || !('act' in row.state)) throw new Error(`${example} has no act`);
      expect(p.decode(p.Act, row.state.act.p)).toMatchObject({ msg: row.id, action });
    }
  });

  it('reads step-3 preset and kb notes, brief preset, note answer, and kb search fetch', () => {
    const presetRow = p.decode(p.NoteRow, goldens['row-note-preset']);
    const kbRow = p.decode(p.NoteRow, goldens['row-note-kb']);
    expect([presetRow.type, presetRow.name, presetRow.task]).toEqual([
      'preset',
      'Calm dashboard',
      null,
    ]);
    expect([kbRow.type, kbRow.name, kbRow.task]).toEqual([
      'kb',
      'Decision: the home is the single writer',
      null,
    ]);
    expect(p.decode(p.Note, goldens['note-preset'])).toMatchObject({
      type: 'preset',
      name: 'Calm dashboard',
      task: null,
    });
    expect(p.decode(p.Note, goldens['note-kb'])).toMatchObject({
      type: 'kb',
      name: 'Decision: the home is the single writer',
      task: null,
    });
    expect(p.decode(p.CmdResult, goldens['cmd-noted'])).toStrictEqual({
      re: 9,
      ok: true,
      id: presetRow.id,
      replaced: false,
    });
    const briefRow = p.decode(p.MsgRow, goldens['row-msg-brief-preset']);
    const briefPost = p.decode(p.Post, briefRow.env!.p);
    expect(briefRow.kind === 'brief' && briefRow.body.preset).toBe('Calm dashboard');
    expect(briefPost.kind === 'brief' && briefPost.body.preset).toBe('Calm dashboard');
    expect(p.decode(p.FetchKind, goldens['fetch-preset'])).toMatchObject({
      kind: 'preset',
      before: 0,
      limit: 50,
    });
    expect(p.decode(p.FetchKind, goldens['fetch-kb'])).toMatchObject({
      kind: 'kb',
      q: 'single writer 路径',
      before: 0,
      limit: 5,
    });
  });

  it('reads caps only of the agents it knows, and refuses a cap without its max', () => {
    const set = JSON.parse(goldens['settings-set']);
    const read = (caps: unknown) => v.safeParse(p.SettingsSet, { ...set, caps });
    expect(read({ Claude: { day: null } }).output).toMatchObject({ caps: {} });
    expect(read({ codex: { day: { unit: 'turns' } } }).success).toBe(false);
    expect(read({ codex: null }).success).toBe(false);
    expect(read([]).success).toBe(false);
  });

  it('takes a takeover member and task, and a terminal member, only as ids', () => {
    const start = JSON.parse(goldens['takeover-start']);
    const resize = JSON.parse(goldens['pty-resize']);
    expect(start.member).toBe(resize.ai);
    expect(accepts('takeover-start', { ...start, member: 'codex' })).toBe(false);
    expect(accepts('pty-resize', { ...resize, ai: 'codex' })).toBe(false);
  });
});

describe('MCP tool lists', () => {
  const lists = p.decode(p.ToolLists, goldens.tools);
  const names = (kind: keyof p.ToolLists) => lists[kind].tools.map((tool) => tool.name);

  it('reads tools.json as it is written, and nothing it does not name', () => {
    expect(lists).toStrictEqual(JSON.parse(goldens.tools));
    // The secretary posts in its own group only: no tool names another one.
    for (const kind of Object.keys(lists) as (keyof p.ToolLists)[]) {
      expect(lists[kind].tools[0].inputSchema.properties).not.toHaveProperty('group');
    }
    const described = JSON.parse(goldens.tools);
    described.group.tools[0].description = 'Post a message.';
    expect(v.is(p.ToolLists, described)).toBe(false);
  });

  it('lists the tools of section 1 to every session, and none twice', () => {
    for (const kind of Object.keys(lists) as (keyof p.ToolLists)[]) {
      expect(names(kind), kind).toEqual(expect.arrayContaining(['post', 'ask', 'net_status']));
      expect(new Set(names(kind)).size, kind).toBe(names(kind).length);
      expect(names(kind), kind).not.toContain('approve');
    }
    expect(names('secretary')).toEqual(expect.arrayContaining(['propose_brief', 'set_task']));
    expect(names('task')).toContain('submit_task');
    expect(names('review')).toContain('submit_review');
  });

  it('lists the external tools with task and decision recipients optional', () => {
    const tools = Object.fromEntries(
      lists.external.tools.map((tool) => [tool.name, tool.inputSchema]),
    );
    expect(names('external')).toEqual([
      'post',
      'ask',
      'reply',
      'read_thread',
      'list_members',
      'list_tasks',
      'note_put',
      'note_list',
      'kb_put',
      'kb_search',
      'submit_task',
      'start_task',
      'submit_review',
      'request_decision',
      'net_status',
      'next_item',
      'done',
      'report_usage',
    ]);
    expect(tools.post.properties.kind.enum).toEqual(['text', 'progress', 'question']);
    expect(tools.read_thread.required).toBeUndefined();
    expect(tools.request_decision.required).toEqual(['question', 'options']);
    expect(tools.next_item.properties).toEqual({});
    expect(tools.done.properties).toEqual({ run: { type: 'integer' } });
    expect(tools.done.required).toEqual(['run']);
    expect(tools.report_usage.properties).toEqual({
      window_pct: { type: 'number', minimum: 0, maximum: 100 },
      window_resets: { type: 'integer' },
    });
    expect(tools.report_usage.required).toEqual(['window_pct']);
  });
});

describe('mock connection', () => {
  it('pushes reset and rows before it answers sync', async () => {
    const seen: string[] = [];
    const ctl = mockCtl((push) => seen.push(push.t));
    const synced = await ctl.request({ t: 'sync', g: 'g', since: 0, state_only: false });
    expect([...seen, synced]).toEqual(['reset', 'rows', { ...JSON.parse(goldens.synced), re: 1 }]);
  });

  it('answers host.info with the example, numbered', async () => {
    const ctl = mockCtl(() => {});
    await ctl.request({ t: 'host.info' });
    const info = await ctl.request({ t: 'host.info' });
    expect(info).toStrictEqual({ ...JSON.parse(goldens['host-info-answer']), re: 2 });
  });

  it('answers an external host.join command with success', async () => {
    const ctl = mockCtl(() => {});
    const answer = await ctl.request({
      t: 'cmd',
      env: { p: goldens['host-join-external'], k: 'k', s: 's' },
    });
    expect(answer).toStrictEqual({ re: 1, ok: true });
  });

  it('pushes groups before group create and join, and answers typed commands with their examples', async () => {
    const seen: string[] = [];
    const ctl = mockCtl((push) => seen.push(push.t));
    const sign = (payload: unknown) => ({ p: JSON.stringify(payload), k: 'k', s: 's' });
    await ctl.request({ t: 'cmd', env: p.decode(p.Cmd, goldens.cmd).env });
    expect(seen).toEqual([]);
    await ctl.request({ t: 'cmd', env: sign(JSON.parse(goldens['group-pause'])) });
    expect(seen).toEqual([]);
    const plainNote = await ctl.request({ t: 'cmd', env: sign(JSON.parse(goldens.note)) });
    expect(plainNote).toStrictEqual({ re: 3, ok: true });
    const created = await ctl.request({ t: 'cmd', env: sign(JSON.parse(goldens['group-create'])) });
    expect(seen).toEqual(['groups']);
    expect(created).toStrictEqual({ re: 4, ok: true });
    const joined = await ctl.request({ t: 'cmd', env: sign(JSON.parse(goldens.join)) });
    expect(seen).toEqual(['groups', 'groups']);
    expect(joined).toStrictEqual({ re: 5, ok: true });
    const invited = await ctl.request({ t: 'cmd', env: sign(JSON.parse(goldens.invite)) });
    expect(invited).toStrictEqual({ ...JSON.parse(goldens['cmd-invite-link']), re: 6 });
    const noted = await ctl.request({ t: 'cmd', env: sign(JSON.parse(goldens['note-preset'])) });
    expect(noted).toStrictEqual({ ...JSON.parse(goldens['cmd-noted']), re: 7 });
    for (const [i, name] of ['code-new', 'code-show'].entries()) {
      const code = await ctl.request({ t: 'cmd', env: sign(JSON.parse(goldens[name])) });
      expect(code).toStrictEqual({ ...JSON.parse(goldens['cmd-code']), re: 8 + i });
    }
    const off = await ctl.request({ t: 'cmd', env: sign(JSON.parse(goldens['code-off'])) });
    expect(off).toStrictEqual({ re: 10, ok: true });
    expect(seen).toEqual(['groups', 'groups']);
  });
});

describe('inbox', () => {
  it('carries plain items that name the rows beside them', () => {
    const { items } = p.decode(p.Inbox, goldens.inbox);
    expect(items.map((entry) => Object.keys(entry))).toEqual([
      ['item', 'msg', 'task'],
      ['item', 'msg', 'task'],
    ]);
    for (const { item, msg, task } of items) {
      expect(item.task).toBe(task!.id);
      expect(item.msg).toBe(msg?.id);
    }
    expect(items[0].item).toStrictEqual(JSON.parse(goldens['item-assign']));
    expect(p.decode(p.Ack, goldens.ack).item).toBe(items[0].item.item);
  });

  it.each(examples.filter((name) => name.startsWith('item-')))('%s is named by its why', (name) => {
    expect(name).toBe(`item-${p.decode(p.Item, goldens[name]).why}`);
  });
});
