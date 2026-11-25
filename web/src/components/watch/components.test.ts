import { flushSync } from 'svelte';
import { SvelteMap } from 'svelte/reactivity';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { closePage, show } from '../../../features/page';
import { Group, Home } from '../../lib/groups.svelte';
import { goldens } from '../../lib/mock';
import type { Path } from '../../lib/net.svelte';
import { prefs } from '../../lib/prefs.svelte';
import * as p from '../../lib/proto';
import AskToWatch from './AskToWatch.svelte';
import Watchers from './Watchers.svelte';
import MemberPanel from '../member/MemberPanel.svelte';

const g = 'rZNvy-1jH6Z-BcPqA5U5BQ';
const me = {
  user: 'viewer',
  sign: async (payload: string) => ({ p: payload, k: 'key', s: 'sig' }),
};

function setup() {
  const home = new Home(me);
  const group = new Group(home, g);
  const ai = { ...p.decode(p.AIMemberRow, goldens['row-member-ai']), state: 'takeover' as const };
  group.apply([p.decode(p.Row, goldens['row-member-human']), ai]);
  const request = vi.fn().mockResolvedValue({ re: 1, ok: true });
  home.ctl = { request: request as p.Ctl['request'] };
  return { home, group, ai, request };
}

const text = () => document.body.textContent!.replace(/\s+/g, ' ').trim();
const click = () => (document.querySelector('button') as HTMLButtonElement).click();

beforeEach(() => {
  prefs.lang = 'en';
});
afterEach(closePage);

it('offers request and watch links only on another person’s taken-over AI', () => {
  const { group, ai } = setup();
  show(MemberPanel, { group, net: { source: 'mock', homes: [], path: () => undefined } });
  expect(
    document.querySelector('.member .card-foot')!.textContent!.replace(/\s+/g, ' ').trim(),
  ).toBe('Ask to watch Watch');
  expect(document.querySelector('.member a')!.getAttribute('href')).toBe(
    `#/terminal/${group.g}/${ai.id}`,
  );
  group.apply([{ ...ai, seq: ai.seq + 1, state: 'working' }]);
  flushSync();
  expect(document.querySelector('.member .card-foot')).toBeNull();
});

it('disables the watch link on a relayed path and says why', () => {
  const { home, group, ai } = setup();
  const far = new Home(me);
  far.device = ai.host;
  far.ctl = home.ctl;
  const paths = new SvelteMap<Home, Path>([[far, { kind: 'relay', rtt: 90, since: 1 }]]);
  show(MemberPanel, {
    group,
    net: { source: 'mock', homes: [far], path: (of: Home) => paths.get(of) },
  });
  expect(document.querySelector('.member a')).toBeNull();
  const watch = document.querySelectorAll<HTMLButtonElement>('.member .card-foot button')[1];
  expect(watch.textContent!.trim()).toBe('Watch');
  expect(watch.disabled).toBe(true);
  expect(document.getElementById(watch.getAttribute('aria-describedby')!)!.textContent).toBe(
    'Watching requires a direct connection',
  );
  paths.set(far, { kind: 'direct', rtt: 9, since: 2 });
  flushSync();
  expect(document.querySelector('.member a')!.textContent!.trim()).toBe('Watch');
});

describe('ask to watch action', () => {
  it('submits once and displays success', async () => {
    const s = setup();
    show(AskToWatch, s);
    expect(text()).toBe('Ask to watch');
    click();
    await vi.waitFor(() => expect(text()).toBe('Watch request sent'));
    expect(document.querySelector('button')!.disabled).toBe(true);
    expect(s.request).toHaveBeenCalledTimes(1);
  });

  it('disables the action offline and outside a takeover', () => {
    const s = setup();
    s.home.ctl = undefined;
    show(AskToWatch, s);
    expect(document.querySelector('button')!.disabled).toBe(true);
  });

  it('shows a refusal and allows another request', async () => {
    const s = setup();
    s.request.mockResolvedValue({ re: 1, err: { code: 'forbidden', msg: '' } });
    show(AskToWatch, s);
    click();
    await vi.waitFor(() => expect(document.querySelector('.fail')).not.toBeNull());
    expect(document.querySelector('button')!.disabled).toBe(false);
    expect(s.request).toHaveBeenCalledTimes(1);
  });
});

describe('watcher list', () => {
  it('queries the AI owner’s host even without host ownership and offers stop controls', () => {
    const s = setup();
    s.request.mockResolvedValue({ re: 1, err: { code: 'forbidden', msg: 'not permitted' } });
    show(Watchers, { group: s.group, host: s.home, member: s.ai.id });
    expect(text()).toBe('Watching Nobody is watching.');
    expect(s.request.mock.calls).toEqual([[{ t: 'host.info' }]]);
    const info = p.decode(p.HostInfoPush, goldens['host-info-push']);
    Object.assign(info.agents[0].groups[0], { g, member: s.ai.id, watchers: ['absent'] });
    s.home.hostInfo = info;
    flushSync();
    expect(text()).toBe('Watching F Former member Stop watching');
    expect(document.querySelector('button')!.textContent!.trim()).toBe('Stop watching');
  });

  it('loads host.info, stops the named person, and reflects a pushed removal', async () => {
    const s = setup();
    const info = p.decode(p.HostInfoPush, goldens['host-info-push']);
    const user = p.decode(p.HumanMemberRow, goldens['row-member-human']);
    Object.assign(info.agents[0].groups[0], { g, member: s.ai.id, watchers: [user.user] });
    s.home.owner = true;
    s.request.mockResolvedValue({ ...info, re: 1, ok: true });
    show(Watchers, { group: s.group, host: s.home, member: s.ai.id });
    await vi.waitFor(() => expect(document.querySelector('button')).not.toBeNull());
    expect(text()).toBe(`Watching O ${user.name} Stop watching`);
    click();
    await vi.waitFor(() => expect(s.request).toHaveBeenCalledTimes(2));
    const payload = JSON.parse(s.request.mock.calls[1][0].env.p);
    expect({ t: payload.t, g: payload.g, member: payload.member, user: payload.user }).toEqual({
      t: 'watch.end',
      g,
      member: s.ai.id,
      user: user.user,
    });
    s.home.hostInfo = { ...info, agents: [] };
    flushSync();
    expect(text()).toBe('Watching Nobody is watching.');
  });

  it('keeps the list and displays a stop refusal', async () => {
    const s = setup();
    const info = p.decode(p.HostInfoPush, goldens['host-info-push']);
    Object.assign(info.agents[0].groups[0], { g, member: s.ai.id, watchers: ['absent'] });
    s.home.owner = true;
    s.request.mockResolvedValueOnce({ ...info, re: 1, ok: true });
    s.request.mockResolvedValue({ re: 1, err: { code: 'not_found', msg: '' } });
    show(Watchers, { group: s.group, host: s.home, member: s.ai.id });
    await vi.waitFor(() => expect(document.querySelector('button')).not.toBeNull());
    click();
    await vi.waitFor(() => expect(document.querySelector('.fail')).not.toBeNull());
    expect(document.querySelector('button')!.disabled).toBe(false);
  });

  it('does not apply an old reply after reconnecting and displays connection loss', async () => {
    const s = setup();
    s.home.owner = true;
    const answer = Promise.withResolvers<unknown>();
    s.request.mockReturnValue(answer.promise);
    show(Watchers, { group: s.group, host: s.home, member: s.ai.id });
    s.home.ctl = undefined;
    flushSync();
    answer.resolve({ ...p.decode(p.HostInfoPush, goldens['host-info-push']), re: 1, ok: true });
    await answer.promise;
    await Promise.resolve();
    expect(s.home.hostInfo).toBeUndefined();
    s.request.mockRejectedValue(new Error('connection lost'));
    s.home.ctl = { request: s.request as p.Ctl['request'] };
    flushSync();
    await vi.waitFor(() => expect(document.querySelector('.fail')).not.toBeNull());
    expect(s.home.hostInfo).toBeUndefined();
  });

  it('ignores a refused host.info reply', async () => {
    const s = setup();
    s.home.owner = true;
    s.request.mockResolvedValue({ re: 1, err: { code: 'forbidden', msg: '' } });
    show(Watchers, { group: s.group, host: s.home, member: s.ai.id });
    await Promise.resolve();
    await Promise.resolve();
    expect(s.home.hostInfo).toBeUndefined();
    expect(text()).toBe('Watching Nobody is watching.');
  });
});
