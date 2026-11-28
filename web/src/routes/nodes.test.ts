import { flushSync } from 'svelte';
import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import { all, closePage, show } from '../../features/page';
import { Group, Home, homes } from '../lib/groups.svelte';
import { goldens } from '../lib/mock';
import type { Net } from '../lib/net.svelte';
import { fingerprintOf } from '../lib/onboard.svelte';
import { nodes } from '../lib/nodes.svelte';
import { prefs } from '../lib/prefs.svelte';
import * as p from '../lib/proto';
import { clockOn } from '../lib/when';
import Nodes from './Nodes.svelte';

beforeEach(() => {
  prefs.lang = 'en';
});
afterEach(async () => {
  await closePage();
  homes.length = 0;
  nodes.infos.clear();
});

const me = { user: 'alex', sign: async (payload: string) => ({ p: payload, k: 'k', s: 's' }) };

function computer(device: string, name: string) {
  const home = new Home(me);
  Object.assign(home, { device, name, owner: true });
  return home;
}

it('lists a group that moved on the computer it moved to, and not on the one it left', () => {
  const old = computer('old-node', 'Old computer');
  const fresh = computer('new-node', 'New computer');
  const human = p.decode(p.HumanMemberRow, goldens['row-member-human']);
  const group = new Group(fresh, human.g);
  group.name = 'Website';
  fresh.groups = [group];
  // Both computers still hold the `host.info` read before the move: it says the old one is the home.
  const info = p.decode(p.HostInfoPush, goldens['host-info-push']);
  const homed = [{ g: human.g, home: 'old-node', name: 'Website' }];
  nodes.infos.set(old, { ...info, device: 'old-node', name: 'Old computer', homes: homed });
  nodes.infos.set(fresh, { ...info, device: 'new-node', name: 'New computer', homes: homed });
  homes.push(old, fresh);
  const net: Net = { source: 'mock', homes, path: () => undefined };
  show(Nodes, { net });
  const hosted = all('section.node').map((card) => card.querySelector('.kv dd')?.textContent);
  expect(hosted).toEqual([undefined, 'Website']);
});

it('distinguishes expired login from unconfigured account across pushed host.info without re-fetching on open', async () => {
  const box = computer('node-1', 'Workstation');
  const { permissions: _permissions, ...answer } = p.decode(
    p.HostInfo,
    goldens['host-info-answer'],
  );
  let calls = 0;
  box.ctl = {
    request: vi.fn(async () => {
      calls += 1;
      return answer;
    }) as unknown as p.Ctl['request'],
  };
  homes.push(box);

  nodes.sync([box]);
  await vi.waitFor(() => expect(nodes.infos.get(box)).toBeDefined());
  expect(calls).toBe(1);

  const net: Net = { source: 'mock', homes, path: () => undefined };
  show(Nodes, { net });
  flushSync();
  expect(calls).toBe(1);
  expect(
    all('.card.node .ai-row')
      .find((el) => el.textContent?.includes('Claude Code'))
      ?.querySelector('.st')
      ?.textContent?.trim(),
  ).toBe('Signed in');

  const push = p.decode(p.HostInfoPush, goldens['host-info-push']);
  nodes.receive(box, {
    ...push,
    device: answer.device,
    name: answer.name,
    offline: false,
    agents: answer.agents.map((a) =>
      a.agent === 'claude' ? { ...a, account: 'personal', login_ok: false } : a,
    ),
  });
  flushSync();

  const cards = all('.card.node .ai-row');
  const claudeRow = cards.find((el) => el.textContent?.includes('Claude Code'))!;
  const codexRow = cards.find((el) => el.textContent?.includes('Codex'))!;
  expect(claudeRow.querySelector('.st')!.textContent!.trim()).toBe('Sign-in expired');
  expect(claudeRow.querySelector('.note[data-tone="danger"]')?.textContent).toContain(
    'Claude Code needs to sign in again.',
  );
  expect(claudeRow.querySelector('.note[data-tone="danger"]')?.textContent).toContain(
    'Run it directly on workstation',
  );
  // Codex has account: null and login_ok: false -> stays Not signed in without reauth alert.
  expect(codexRow.querySelector('.st')!.textContent!.trim()).toBe('Not signed in');
  expect(codexRow.querySelector('.note[data-tone="danger"]')).toBeNull();

  // Pushing updated host.info with login_ok = true clears the alert immediately.
  nodes.receive(box, {
    ...push,
    device: answer.device,
    name: answer.name,
    offline: false,
  });
  flushSync();
  const updatedClaude = all('.card.node .ai-row').find((el) =>
    el.textContent?.includes('Claude Code'),
  )!;
  expect(updatedClaude.querySelector('.st')!.textContent!.trim()).toBe('Signed in');
  expect(updatedClaude.querySelector('.note[data-tone="danger"]')).toBeNull();
});

it("lists the owner's browsers, each with its fingerprint and when it was added", async () => {
  const [web, phone, node] = [
    'akWV_5PtNLIGhakuMAvpD9LcG1D8TUfc1URh48VREcs',
    'J3Uuh-y1z2AX536yAFlGHJL5sjxVpWVJavvVphvdm5Q',
    'rZNvy-1jH6Z-BcPqA5U5BQrZNvy-1jH6Z-BcPqA5U5BQ',
  ];
  const added = 1763600000000;
  const net: Net = {
    source: 'live',
    homes,
    path: () => undefined,
    devices: [
      { id: web, kind: 'browser', name: 'Browser', by: web, self: true },
      { id: node, kind: 'node', name: 'workstation', by: web, added, self: false },
      { id: phone, kind: 'browser', name: 'Phone', by: node, added, self: false },
    ],
  };
  show(Nodes, { net });
  const prints = [await fingerprintOf(web), await fingerprintOf(phone)];
  await vi.waitFor(() => expect(all('.browsers code').map((el) => el.textContent)).toEqual(prints));
  const rows = all('.browsers li').map((row) => row.textContent!.replace(/\s+/g, ' ').trim());
  expect(rows).toEqual([
    `Browser This browser Fingerprint ${prints[0]}`,
    `Phone Fingerprint ${prints[1]} Added ${clockOn(added, Date.now(), 'en')}`,
  ]);
  expect(all('.browsers h3').map((el) => el.textContent)).toEqual(['Browsers and phones']);
});

it('shows no list of browsers before the owner has devices', () => {
  show(Nodes, { net: { source: 'live', homes, path: () => undefined, devices: [] } });
  expect(all('.browsers')).toEqual([]);
});
