import { afterEach, beforeEach, expect, it } from 'vitest';
import { all, closePage, show } from '../../features/page';
import { Group, Home, homes } from '../lib/groups.svelte';
import { goldens } from '../lib/mock';
import type { Net } from '../lib/net.svelte';
import { nodes } from '../lib/nodes.svelte';
import { prefs } from '../lib/prefs.svelte';
import * as p from '../lib/proto';
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
