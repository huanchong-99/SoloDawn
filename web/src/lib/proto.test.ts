import * as v from 'valibot';
import { describe, expect, it } from 'vitest';
import { examples, goldens, schemaOf } from './mock';
import * as p from './proto';

/** Fields of the examples that the contracts mark optional and Go tags omitempty: the only ones
 *  that may be left out. */
const omitempty: Record<string, string[]> = {
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
  'row-msg': ['env', 'state.by', 'state.at', 'state.choice', 'state.text', 'state.act'],
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

/** Paths to every object in an example, except message bodies, which are any JSON. */
function objects(value: unknown, path: string[] = []): string[][] {
  if (typeof value !== 'object' || value === null) return [];
  const inner = Object.entries(value)
    .filter(([k]) => k !== 'body')
    .flatMap(([k, x]) => objects(x, [...path, k]));
  return Array.isArray(value) ? inner : [path, ...inner];
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
    for (const path of objects(JSON.parse(goldens[name]))) {
      const extra = variant(name, path, (o) => (o.extra = 1));
      const read = p.decode(schemaOf[name], JSON.stringify(extra));
      expect(read, `extra field at ${path.join('.')}`).toStrictEqual(JSON.parse(goldens[name]));
      // So a golden file with a field that the schema lacks fails the exact test above.
      expect(read, `extra field at ${path.join('.')}`).not.toStrictEqual(extra);
    }
  });

  it.each(examples)('%s is refused without a field that Go always writes', (name) => {
    // Rows inside rows and fetched are checked through their own examples.
    const paths = objects(JSON.parse(goldens[name])).filter((path) => path[0] !== 'rows');
    for (const path of paths) {
      for (const key of Object.keys(at(JSON.parse(goldens[name]), path))) {
        const field = named([...path, key]);
        const missing = variant(name, path, (o) => delete o[key]);
        const optional = (omitempty[name] ?? []).includes(field);
        expect(accepts(name, missing), `without ${field}`).toBe(optional);
      }
    }
  });

  it('keeps a signed payload as the exact text received, leading space included', () => {
    const { entries } = p.decode(p.Devlog, goldens.devlog);
    expect(entries[1].p.slice(0, 15)).toBe(' {"t":"device",');
    expect(p.decode(p.DeviceRow, goldens['row-device']).env.p).toBe(entries[1].p);
  });

  it('takes any JSON as a message body, null included', () => {
    expect(
      accepts(
        'row-msg',
        variant('row-msg', [], (o) => (o.body = null)),
      ),
    ).toBe(true);
    expect(
      accepts(
        'row-msg',
        variant('row-msg', [], (o) => (o.body = [1, 'a'])),
      ),
    ).toBe(true);
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

  it('is the type of n and g in a signed payload, and of no other example field', () => {
    const offer = JSON.parse(goldens.offer);
    expect(accepts('offer', offer)).toBe(true);
    expect(accepts('offer', { ...offer, g: id })).toBe(true);
    expect(accepts('offer', { ...offer, n: 'offer-1' })).toBe(false);
    expect(accepts('offer', { ...offer, g: 'Website' })).toBe(false);
    expect(accepts('offer', { ...offer, g: '' })).toBe(false);
    expect(accepts('offer', { ...offer, conn: 'c1', to: 'anyone' })).toBe(true);
    // The by of a message row may be "system", and a device row's id is no random id.
    const msg = JSON.parse(goldens['row-msg']);
    expect(accepts('row-msg', { ...msg, by: 'system' })).toBe(true);
    expect(JSON.parse(goldens['row-device']).id).toContain(':');
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
