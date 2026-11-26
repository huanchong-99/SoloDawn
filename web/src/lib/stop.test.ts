import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import { Group, Home } from './groups.svelte';
import { goldens } from './mock';
import { nodes } from './nodes.svelte';
import { prefs } from './prefs.svelte';
import * as p from './proto';
import {
  computers,
  isOffline,
  mayPause,
  names,
  offlineAIs,
  setPaused,
  statusIn,
  working,
} from './stop';

const gid = 'rZNvy-1jH6Z-BcPqA5U5BQ';
const user: string = JSON.parse(goldens['row-member-human']).user;
const signed: string[] = [];
const sign = async (payload: string) => {
  signed.push(payload);
  return { p: payload, k: 'dev-k', s: 'dev-s' };
};

let seq = 1000;
const row = <T extends p.Row>(name: string, changes: object = {}) =>
  ({ ...p.decode(p.Row, goldens[name]), seq: (seq += 1), ...changes }) as T;
const ai = (id: string, changes: object = {}) =>
  row<p.AIMemberRow>('row-member-ai', { id, name: id, ...changes });
const info = (offline: boolean): p.HostInfoPush => ({
  ...p.decode(p.HostInfoPush, goldens['host-info-push']),
  offline,
});

/** A computer of the person, or of someone else. */
function computer(name: string, owner = true) {
  const home = new Home({ user, sign });
  const ctl = { request: async () => ({ re: 1, ok: true }) } as unknown as p.Ctl;
  const welcome = p.decode(p.Welcome, goldens.welcome);
  home.connect().online(ctl, { ...welcome, device: name, name, owner, groups: [] });
  return home;
}

function groupOf(home: Home, changes: object = {}, role = 'owner') {
  const group = new Group(home, gid);
  group.apply([
    row('row-group', { id: gid, ...changes }),
    row('row-member-human', { id: 'alex', user, role }),
  ]);
  home.groups = [group];
  return group;
}

beforeEach(() => {
  prefs.lang = 'en';
});
afterEach(() => {
  nodes.infos.clear();
  signed.length = 0;
});

describe('setPaused', () => {
  it('sends group pause and group resume to the home', async () => {
    const group = groupOf(computer('workstation'));
    expect(await setPaused(group, true)).toEqual({ re: 1, ok: true });
    expect(await setPaused(group, false)).toEqual({ re: 1, ok: true });
    expect(signed.map((text) => p.decode(p.GroupCommand, text))).toEqual(
      ['pause', 'resume'].map((op) => ({
        t: 'group',
        n: expect.any(String),
        ts: expect.any(Number),
        g: gid,
        op,
      })),
    );
  });
});

describe('mayPause', () => {
  it('is for the owner and the admins of a group that has AIs', () => {
    const home = computer('workstation');
    const withAI = (role: string) => {
      const group = groupOf(home, {}, role);
      group.apply([ai('codex')]);
      return group;
    };
    expect(mayPause(withAI('owner'))).toBe(true);
    expect(mayPause(withAI('admin'))).toBe(true);
    expect(mayPause(withAI('member'))).toBe(false);
    expect(mayPause(groupOf(home))).toBe(false);
    expect(mayPause(new Group(home, gid))).toBe(false);
  });
});

describe('working', () => {
  it('counts the AIs that run a turn', () => {
    const group = groupOf(computer('workstation'));
    expect(working(group)).toBe(0);
    group.apply([
      ai('a', { state: 'working' }),
      ai('b', { state: 'waiting' }),
      ai('c', { state: 'dormant' }),
    ]);
    expect(working(group)).toBe(1);
  });
});

describe('computers', () => {
  it("tells the person's computers whose AIs are offline from the others", () => {
    const on = computer('on');
    const off = computer('off');
    const unread = computer('unread');
    const foreign = computer('foreign', false);
    nodes.infos.set(on, info(false));
    nodes.infos.set(off, info(true));
    nodes.infos.set(foreign, info(true));
    expect([on, off, unread].map(isOffline)).toEqual([false, true, false]);
    expect(computers([on, off, unread, foreign])).toEqual({
      stopped: [off],
      running: [on, unread],
    });
  });
});

describe('names', () => {
  it('joins the names as the language does', () => {
    const [a, b, c] = ['workstation', '和平', 'mac-mini'].map((name) => computer(name));
    expect(names([a], 'en')).toBe('workstation');
    expect(names([a, c], 'en')).toBe('workstation and mac-mini');
    expect(names([a, b, c], 'zh-CN')).toBe('workstation、和平 和 mac-mini');
  });
});

describe('offlineAIs', () => {
  it('lists the AIs of one computer that their groups show as offline', () => {
    const home = computer('workstation');
    const other = computer('mac-mini');
    groupOf(home).apply([
      ai('a', { host: 'workstation', state: 'offline' }),
      ai('b', { host: 'workstation', state: 'dormant' }),
      ai('c', { host: 'mac-mini', state: 'offline' }),
    ]);
    groupOf(other).apply([ai('d', { host: 'workstation', state: 'offline' })]);
    expect(offlineAIs(home, [home, other])).toBe('a d');
    expect(offlineAIs(other, [home, other])).toBe('c');
  });
});

describe('statusIn', () => {
  it('says that the owner took the AI offline, unless it runs a turn or is taken over', () => {
    const home = computer('workstation');
    const group = groupOf(home);
    const said = (state: string, host: Home | undefined = home) =>
      statusIn(group, ai('a', { state, task: undefined }), host);
    expect(said('offline')).toEqual({ st: 'offline', text: 'Offline' });
    nodes.infos.set(home, info(false));
    expect(said('offline')).toEqual({ st: 'offline', text: 'Offline' });
    nodes.infos.set(home, info(true));
    for (const state of ['offline', 'dormant'])
      expect(said(state)).toEqual({ st: 'offline', text: 'Offline · by you' });
    expect(said('working')).toEqual({ st: 'working', text: 'Working' });
    expect(said('takeover')).toEqual({ st: 'takeover', text: 'Owner is driving' });
    expect(statusIn(group, ai('a', { state: 'offline' }), undefined)).toEqual({
      st: 'offline',
      text: 'Offline',
    });
  });
});
