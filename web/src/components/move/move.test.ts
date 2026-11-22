import { afterEach, beforeEach, expect, it } from 'vitest';
import { all, closePage, one, show } from '../../../features/page';
import { Group, Home } from '../../lib/groups.svelte';
import { goldens } from '../../lib/mock';
import type { Net } from '../../lib/net.svelte';
import { prefs } from '../../lib/prefs.svelte';
import * as p from '../../lib/proto';
import MoveInfo from './MoveInfo.svelte';
import mounts from './mounts';

beforeEach(() => {
  prefs.lang = 'en';
});
afterEach(closePage);

function setup(role = 'owner') {
  const human = p.decode(p.HumanMemberRow, goldens['row-member-human']);
  const me = {
    user: human.user,
    sign: async (payload: string) => ({ p: payload, k: 'key', s: 'signature' }),
  };
  const source = new Home(me);
  source.device = 'old-node';
  source.name = 'Current computer';
  source.owner = true;
  const target = new Home(me);
  target.device = 'new-node';
  target.name = 'Other computer';
  target.owner = true;
  const foreign = new Home(me);
  foreign.device = 'foreign-node';
  foreign.name = 'Foreign computer';
  const group = new Group(source, human.g);
  group.apply([{ ...human, role }]);
  const net: Net = { source: 'mock', homes: [source, target, foreign], path: () => undefined };
  return { group, net };
}

it('mounts after group info and lists only other owned computers with exact CLI lines', () => {
  expect(mounts['chat.side']).toEqual([{ order: 30, component: MoveInfo }]);
  const { group, net } = setup();
  show(MoveInfo, { group, net });
  expect(all('h3').map((node) => node.textContent)).toEqual(['Other computer']);
  expect(all('code').map((node) => node.textContent)).toEqual([
    `solodawn group export ${group.g} group.sqlite --to new-node`,
    'solodawn group import group.sqlite',
  ]);
  expect(one('.note').textContent).toBe(
    'Export with --to stops this computer from being the home immediately. The group is unavailable until import finishes. Do not resend an unanswered command: receipts stay on the old computer, so it could run twice.',
  );
});

it('shows the group id and an explicit device-id placeholder when no other computer is known', () => {
  const { group, net } = setup();
  net.homes.splice(1);
  show(MoveInfo, { group, net });
  expect(all('code').map((node) => node.textContent)).toEqual([
    `solodawn group export ${group.g} group.sqlite --to DEVICE_ID`,
    'solodawn group import group.sqlite',
  ]);
  expect(all('.hint').map((node) => node.textContent)).toEqual([
    'Run these commands in the local command line, not in this browser. A group can move only to another computer of its owner.',
    "No other computer of yours is known. Replace DEVICE_ID with the destination computer's device id.",
    'For a safekeeping copy, omit --to and the device id; the current computer stays the home. If import fails, you can import the file on the old computer again.',
  ]);
});

it.each(['admin', 'member'])('does not offer the commands to a human %s', (role) => {
  const props = setup(role);
  show(MoveInfo, props);
  expect(document.querySelector('section')).toBeNull();
});

it('does not offer commands before membership arrives or to an AI', () => {
  const { group, net } = setup();
  group.rows.clear();
  show(MoveInfo, { group, net });
  expect(document.querySelector('section')).toBeNull();
});

it('provides the owner guidance in Chinese', () => {
  prefs.lang = 'zh-CN';
  show(MoveInfo, setup());
  expect(one('h2').textContent).toBe('迁移群组');
  expect(one('summary').textContent).toBe('导出与导入');
});
