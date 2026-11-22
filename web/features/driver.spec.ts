import { describeFeature, loadFeature } from '@amiceli/vitest-cucumber';
import { flushSync } from 'svelte';
import { SvelteMap } from 'svelte/reactivity';
import { expect, vi } from 'vitest';
import StatusPanel from '../src/components/pairing/StatusPanel.svelte';
import { Home, homes } from '../src/lib/groups.svelte';
import type { Path, Source } from '../src/lib/net.svelte';
import { nodes } from '../src/lib/nodes.svelte';
import type { PairingSession } from '../src/lib/onboard.svelte';
import type * as p from '../src/lib/proto';
import { example, ownedNode, user, type OwnedNode } from './home';
import { all, closePage, openPage, show, texts } from './page';

const feature = await loadFeature('../features/web_driver.feature');

const website = 'rZNvy-1jH6Z-BcPqA5U5BQ';
const device = 'J3Uuh-y1z2AX536yAFlGHJL5sjxVpWVJavvVphvdm5Q';
const paths = new SvelteMap<Home, Path | undefined>();

vi.mock('../src/lib/net.svelte', async (actual) => {
  const real = await actual<typeof import('../src/lib/net.svelte')>();
  return {
    ...real,
    start: (source: Source) => ({
      net: {
        source,
        homes,
        path: (home: Home) => (home.ctl ? paths.get(home) : undefined),
      },
      stop: () => {},
    }),
  };
});

const agent = (
  name: string,
  accounts: string[],
  takeover: string,
): p.HostInfo['agents'][number] => ({
  agent: name,
  cli_version: '1.0.0',
  account: null,
  login_ok: true,
  caps: {},
  driver: { accounts, takeover },
  groups: [],
});

const agents = [
  agent('claude', ['personal', 'team', 'api_key'], 'new'),
  agent('opencode', ['api_key'], 'none'),
];

const choices = () =>
  all('fieldset.acct').map((set) => [...set.querySelectorAll('label b')].map((b) => b.textContent));

const offered = [['Personal subscription', 'Team plan', 'API key'], ['API key']];

describeFeature(feature, ({ Background, Scenario, AfterEachScenario }) => {
  let workstation: OwnedNode;

  AfterEachScenario(async () => {
    await closePage();
    paths.clear();
    nodes.infos.clear();
  });

  Background(({ Given }) => {
    Given('a browser set to English', () => {});
  });

  Scenario(
    'A CLI that allows API keys only is offered no other account type',
    ({ Given, When, Then }) => {
      Given(
        'an owned computer "workstation" with "claude" allowing every account type and "opencode" allowing API keys only, neither declared',
        async () => {
          workstation = ownedNode('workstation', device, { agents: structuredClone(agents) });
          homes.push(workstation.store);
          await workstation.connect();
        },
      );
      When('the browser opens "#/nodes"', async () => {
        openPage('#/nodes');
        await vi.waitFor(() => {
          flushSync();
          expect(all('fieldset.acct')).toHaveLength(2);
        });
      });
      Then(
        '"claude" offers "Personal subscription", "Team plan" and "API key", and "opencode" offers "API key" only',
        () => expect(choices()).toEqual(offered),
      );
      When('pairing finds the same two AIs', async () => {
        await closePage();
        const session = { stage: 'accounts', role: 'a', enrolled: true, agents, busy: false };
        show(StatusPanel, { session: session as unknown as PairingSession });
        flushSync();
      });
      Then('the pairing page offers the same account types', () => {
        expect(texts('section.card h2.kind')).toEqual([
          'Connection status',
          'How does Claude Code sign in?',
          'How does opencode sign in?',
        ]);
        expect(choices()).toEqual(offered);
      });
    },
  );

  Scenario('An AI whose driver has no takeover offers no terminal', ({ Given, When, Then }) => {
    Given(
      'an owned computer "workstation" on a direct path, home of a group with my AIs "Claude Code" on "claude" and "OpenCode" on "opencode" without a takeover',
      async () => {
        workstation = ownedNode('workstation', device, { agents: structuredClone(agents) });
        workstation.home.add(website, 'Website');
        for (const [id, name, cli] of [
          ['ai-claude', 'Claude Code', 'claude'],
          ['ai-opencode', 'OpenCode', 'opencode'],
        ]) {
          workstation.home.write(
            website,
            example('row-member-ai', {
              id,
              g: website,
              user,
              name,
              cli,
              host: device,
              state: 'waiting',
              task: undefined,
            }),
          );
        }
        homes.push(workstation.store);
        await workstation.connect();
        paths.set(workstation.store, { kind: 'direct', rtt: 4, since: Date.now() });
      },
    );
    When('the browser opens the group', async () => {
      openPage(`#/groups/${website}`);
      await vi.waitFor(() => {
        flushSync();
        expect(nodes.infos.has(workstation.store)).toBe(true);
        expect(all('.member')).toHaveLength(2);
      });
      flushSync();
    });
    Then('"Claude Code" links to "Open terminal" and "OpenCode" offers no terminal', () => {
      const [claude, opencode] = all('.member');
      expect(claude.querySelector('h3')!.textContent).toBe('Claude Code');
      expect(claude.querySelector('.card-foot a')!.getAttribute('href')).toBe(
        `#/terminal/${website}/ai-claude`,
      );
      expect(opencode.querySelector('h3')!.textContent).toBe('OpenCode');
      expect(opencode.querySelector('.card-foot a')).toBeNull();
      expect(opencode.textContent).not.toContain('Open terminal');
    });
  });
});
