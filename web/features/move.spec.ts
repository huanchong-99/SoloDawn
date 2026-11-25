import { describeFeature, loadFeature } from '@amiceli/vitest-cucumber';
import { flushSync } from 'svelte';
import { expect, vi } from 'vitest';
import { Group, Home, homes, find, type Link } from '../src/lib/groups.svelte';
import { goldens } from '../src/lib/mock';
import type { Net } from '../src/lib/net.svelte';
import { prefs } from '../src/lib/prefs.svelte';
import * as p from '../src/lib/proto';
import MoveInfo from '../src/components/move/MoveInfo.svelte';
import { all, closePage, one, show } from './page';

const feature = await loadFeature('../features/web_move.feature');
describeFeature(feature, ({ AfterEachScenario, Scenario }) => {
  AfterEachScenario(closePage);
  let source: Home;
  let destination: Home;
  let group: Group;
  let link: Link;
  const moved = { re: 1, err: { code: 'moved', msg: 'The group has moved.', home: 'destination' } };
  let request: ReturnType<typeof vi.fn>;

  function setup() {
    prefs.lang = 'en';
    const human = p.decode(p.HumanMemberRow, goldens['row-member-human']);
    const me = {
      user: human.user,
      sign: async (payload: string) => ({ p: payload, k: 'key', s: 'signature' }),
    };
    source = new Home(me);
    destination = new Home(me);
    Object.assign(source, { device: 'source', name: 'Source', owner: true });
    Object.assign(destination, { device: 'destination', name: 'Destination', owner: true });
    link = source.connect();
    request = vi.fn().mockResolvedValue({ t: 'synced', re: 1, ok: true, g: human.g, seq: 3 });
    link.online(
      { request: request as p.Ctl['request'] },
      {
        ...p.decode(p.Welcome, goldens.welcome),
        device: 'source',
        groups: [{ g: human.g, name: 'Project', seq: 3 }],
      },
    );
    group = source.groups[0];
    group.apply([human]);
    homes.push(source, destination);
    source.onmove = (g) => source.move(g, destination);
  }
  const followed = () => {
    expect(find(group.g)).toBe(group);
    expect(group.home).toBe(destination);
    expect(group.members.map((member) => member.name)).toEqual(['owner']);
  };
  const readsOn = () => {
    const next = vi.fn().mockResolvedValue({ t: 'synced', re: 1, ok: true, g: group.g, seq: 3 });
    destination.connect().online(
      { request: next as p.Ctl['request'] },
      {
        ...p.decode(p.Welcome, goldens.welcome),
        groups: [{ g: group.g, name: 'Project', seq: 3 }],
      },
    );
    expect(next.mock.calls.map(([req]) => req)).toEqual([
      { t: 'sync', g: group.g, since: 3, state_only: false },
    ]);
  };
  Scenario(
    'A browser follows moved without sending an unanswered command twice',
    ({ Given, When, Then, And }) => {
      Given('a browser reading a project on its old home', setup);
      When('a command response says that the group moved to another computer', async () => {
        request.mockResolvedValueOnce(moved);
        expect(await group.post('once', [])).toEqual(moved);
        expect(request).toHaveBeenCalledTimes(1);
      });
      Then('the group stays on the same route with its cached rows and its new home', followed);
      And('the browser reads from the new home without replaying the command', readsOn);
    },
  );
  Scenario(
    'An idle browser follows a group that its home no longer lists',
    ({ Given, When, Then, And }) => {
      Given('a browser reading a project on its old home', setup);
      When('the old home no longer lists the group, and says where it went', async () => {
        request.mockResolvedValueOnce(moved);
        link.push({ t: 'groups', groups: [] });
        expect(find(group.g)).toBe(group);
        await vi.waitFor(() => expect(source.groups).toEqual([]));
      });
      Then('the group stays on the same route with its cached rows and its new home', followed);
      And('the browser reads from the new home without replaying the command', readsOn);
    },
  );
  Scenario('Only the owner sees local move commands', ({ Given, When, Then, And }) => {
    Given('the owner reads a project and has another computer', () => {
      setup();
      const net: Net = { source: 'mock', homes, path: () => undefined };
      show(MoveInfo, { group, net });
    });
    When('the owner opens the export and import guidance', () => {
      one<HTMLDetailsElement>('details').open = true;
      flushSync();
    });
    Then("it shows the group's export command naming that computer and the import command", () => {
      expect(all('code').map((node) => node.textContent)).toEqual([
        `solodawn group export ${group.g} group.sqlite --to destination`,
        'solodawn group import group.sqlite',
      ]);
    });
    And('it explains the unavailable interval and duplicate-command risk', () => {
      expect(one('.note').textContent).toBe(
        'Export with --to stops this computer from being the home immediately. The group is unavailable until import finishes. Do not resend an unanswered command: receipts stay on the old computer, so it could run twice.',
      );
    });
  });
});
