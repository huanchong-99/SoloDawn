import { describe, expect, it } from 'vitest';
import { readEntry, readHeader } from './payload';

// Two random ids: 16 bytes in their canonical spelling.
const n = 'Zk3aZk3aZk3aZk3aZk3aZg';
const g = 'Qm1bQm1bQm1bQm1bQm1bQw';

describe('readHeader', () => {
  it('gives the payload when t, n and ts are there and g and to are as they must be when there', () => {
    const p = `{"t":"post","n":"${n}","ts":1763600000000,"g":"${g}","to":"A","text":"hi"}`;
    expect(readHeader(p)).toStrictEqual(JSON.parse(p));
    expect(readHeader(`{"t":"","n":"${n}","ts":-3}`)).toStrictEqual({ t: '', n, ts: -3 });
    expect(readHeader(`{"t":"a","n":"${n}","ts":9007199254740991}`)?.ts).toBe(2 ** 53 - 1);
    expect(readHeader(`{"t":"a","n":"${n}","ts":2.0000000000000001}`)?.ts).toBe(2);
    expect(readHeader(`{"t":"a","n":"${n}","ts":1,"to":""}`)?.to).toBe('');
    expect(readHeader(`{"t":"a","n":"x","n":"${g}","ts":1}`)?.n).toBe(g);
  });

  it.each([
    ['g', `{"t":"a","n":"${n}","ts":1,"g":5}`],
    ['g', `{"t":"a","n":"${n}","ts":1,"g":null}`],
    ['g', `{"t":"a","n":"${n}","ts":1,"g":""}`],
    ['g', `{"t":"a","n":"${n}","ts":1,"g":"Website"}`],
    ['g', `{"t":"a","n":"${n}","ts":1,"g":"${g.slice(0, 21)}x"}`],
    ['to', `{"t":"a","n":"${n}","ts":1,"to":5}`],
    ['t', `{"n":"${n}","ts":1}`],
    ['n', '{"t":"a","ts":1}'],
    ['n', '{"t":"a","n":5,"ts":1}'],
    ['n', '{"t":"a","n":"","ts":1}'],
    ['n', '{"t":"a","n":"b","ts":1}'],
    ['n', '{"t":"a","n":"\\ud800","ts":1}'],
    ['n', `{"t":"a","n":"${n}","n":"b","ts":1}`],
    ['n', `{"t":"a","n":["${n}"],"ts":1}`],
    ['ts', `{"t":"a","n":"${n}"}`],
    ['ts', `{"t":"a","n":"${n}","ts":1.5}`],
    ['ts', `{"t":"a","n":"${n}","ts":null}`],
    ['ts', `{"t":"a","n":"${n}","ts":9007199254740992}`],
    ['ts', `{"t":"a","n":"${n}","ts":-9007199254740992}`],
    ['ts', `{"t":"a","n":"${n}","ts":1e20}`],
    ['to', `{"t":"a","n":"${n}","ts":1,"to":null}`],
  ])('is undefined for a wrong or missing %s', (_, p) => {
    expect(readHeader(p)).toBeUndefined();
  });

  it.each(['null', '[]', '"a"', '5', 'true', 'not json'])('is undefined for %s', (p) => {
    expect(readHeader(p)).toBeUndefined();
  });
});

describe('readEntry', () => {
  const add = '{"t":"device","user":"U","ver":2,"op":"add","dev":"D","kind":"node","name":"w"}';

  it('reads an add of a node or a browser, and a revoke', () => {
    expect(readEntry(add)).toStrictEqual(JSON.parse(add));
    expect(readEntry(add.replace('node', 'browser'))).toMatchObject({ op: 'add' });
    const revoke = '{"t":"device","user":"U","ver":3,"op":"revoke","dev":"D"}';
    expect(readEntry(revoke)).toStrictEqual(JSON.parse(revoke));
  });

  it.each([
    ['another t', add.replace('device', 'post')],
    ['a user that is not a string', add.replace('"U"', '1')],
    ['a dev that is not a string', add.replace('"D"', '1')],
    ['a ver that is not an integer', add.replace('2', '2.5')],
    ['a ver above 2^53', add.replace('2', '9007199254740993')],
    ['a user that is null', add.replace('"U"', 'null')],
    ['another op', add.replace('"add"', '"rename"')],
    ['an add of another kind', add.replace('node', 'phone')],
    ['an add without a kind', add.replace(',"kind":"node"', '')],
    ['an add without a name', add.replace(',"name":"w"', '')],
    ['an add whose name is not a string', add.replace('"w"', 'null')],
    ['not an object', '[]'],
  ])('is undefined for %s', (_, p) => {
    expect(readEntry(p)).toBeUndefined();
  });
});
