import 'fake-indexeddb/auto';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { Home, homes } from './groups.svelte';
import { goldens } from './mock';
import { mockHome, sourceOf, start } from './net.svelte';
import * as p from './proto';

// The requests the mock connection was asked, as sent.
const asked = vi.hoisted(() => [] as unknown[]);
vi.mock('./mock', async (actual) => {
  const mock = await actual<typeof import('./mock')>();
  return {
    ...mock,
    mockCtl: (onpush: (push: p.Push) => void): p.Ctl => {
      const ctl = mock.mockCtl(onpush);
      return {
        request: (req: never) => {
          asked.push(req);
          return ctl.request(req);
        },
      } as unknown as p.Ctl;
    },
  };
});

const person: string = JSON.parse(goldens['row-member-human']).user;
const elsewhere = () => new Home({ user: 'u', sign: () => new Promise(() => {}) });

afterEach(() => {
  homes.length = 0;
  asked.length = 0;
});

describe('sourceOf', () => {
  it.each([
    ['development', '', 'mock'],
    ['development', '?a=1', 'mock'],
    ['development', '?nomockery', 'mock'],
    ['development', '?nomock', 'live'],
    ['development', '?a=1&nomock=', 'live'],
    ['production', '', 'live'],
    ['test', '', 'live'],
  ])('is, in the mode %s at %j, %s', (mode, search, source) => {
    expect(sourceOf(mode, search)).toBe(source);
  });
});

describe('mockHome', () => {
  it("is the examples' home, read by their person with this browser's key, once online", async () => {
    const { home, online } = await mockHome();
    expect(home.me.user).toBe(person);
    expect(asked).toStrictEqual([
      { t: 'hello', v: 1, kind: 'browser', name: 'development', addrs: [] },
    ]);
    const env = await home.me.sign('{"t":"cmd"}');
    expect(env.p).toBe('{"t":"cmd"}');
    expect(env.k).toMatch(/^[\w-]{43}$/);
    expect(home.ctl).toBeUndefined();
    online();
    expect(home.ctl).toBeDefined();
    expect(home.groups.map((group) => group.name)).toEqual(['Website']);
  });
});

describe('start', () => {
  it('reaches no home from the nodes until the connection code is wired', async () => {
    const { net, stop } = start('live');
    await new Promise((later) => setTimeout(later, 50));
    expect(net.source).toBe('live');
    expect(net.homes).toBe(homes);
    expect(homes).toEqual([]);
    expect(asked).toEqual([]);
    stop();
  });

  it('adds the home of the mock, online and reached directly since it came online', async () => {
    const before = Date.now();
    const other = elsewhere();
    homes.push(other);
    const { net, stop } = start('mock');
    expect(net.source).toBe('mock');
    await vi.waitFor(() => expect(homes).toHaveLength(2));
    const home = homes[1];
    expect(home.groups.map((group) => group.name)).toEqual(['Website']);
    const path = net.path(home)!;
    expect(path).toStrictEqual({ kind: 'direct', rtt: undefined, since: path.since });
    expect(path.since).toBeGreaterThanOrEqual(before);
    expect(path.since).toBeLessThanOrEqual(Date.now());
    expect(net.path(other)).toBeUndefined();
    home.ctl = undefined;
    expect(net.path(home)).toBeUndefined();
    stop();
    expect(homes).toEqual([other]);
  });

  it('adds nothing when stopped before the mock is ready', async () => {
    const { stop } = start('mock');
    stop();
    await vi.waitFor(() => expect(asked).toHaveLength(1));
    await new Promise((later) => setTimeout(later, 20));
    expect(homes).toEqual([]);
  });
});
