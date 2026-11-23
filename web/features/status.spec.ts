import 'fake-indexeddb/auto';
import { describeFeature, loadFeature } from '@amiceli/vitest-cucumber';
import { flushSync } from 'svelte';
import { SvelteMap } from 'svelte/reactivity';
import { expect, vi } from 'vitest';
import { homes } from '../src/lib/groups.svelte';
import type { Home } from '../src/lib/groups.svelte';
import type { Source } from '../src/lib/net.svelte';
import { browser, TestHome } from './home';
import { all, closePage, field, one, openPage, texts } from './page';

const website = 'rZNvy-1jH6Z-BcPqA5U5BQ';
const feature = await loadFeature('../features/web_status.feature');
const tabState = new SvelteMap<'otherTab', boolean>();

vi.mock('../src/lib/net.svelte', async (actual) => {
  const real = await actual<typeof import('../src/lib/net.svelte')>();
  return {
    ...real,
    start: (source: Source) => ({
      net: {
        source,
        homes,
        path: (_home: Home) => undefined,
        get otherTab() {
          return tabState.get('otherTab') ?? false;
        },
        useHere: () => {
          tabState.set('otherTab', false);
        },
      },
      stop: () => {},
    }),
  };
});

describeFeature(feature, ({ Scenario, AfterEachScenario }) => {
  AfterEachScenario(async () => {
    tabState.clear();
    vi.unstubAllEnvs();
    await closePage();
  });

  Scenario(
    'An inactive tab shows the other-tab banner and the inactive indicator instead of reconnecting',
    ({ Given, When, Then, And }) => {
      Given(
        'an online computer "workstation" with no selected candidate pair while another tab holds the lock',
        async () => {
          const home = new TestHome('workstation');
          const client = browser(home);
          homes.push(client.store);
          await client.connect();
          tabState.set('otherTab', true);
        },
      );

      When('the browser opens "#/groups"', () => {
        openPage('#/groups');
      });

      Then(
        'the banner shows "SoloDawn is open in another tab." with "Use here" and the indicator has state "offline" reading "Inactive"',
        () => {
          const ban = one('.ban .tab-ban[role="status"]');
          expect(ban.querySelector('.grow')?.textContent?.trim()).toBe(
            'SoloDawn is open in another tab.',
          );
          expect(ban.querySelector('button')?.textContent?.trim()).toBe('Use here');
          const conn = one<HTMLAnchorElement>('.bar .conn');
          expect(conn.dataset.st).toBe('offline');
          expect(texts('.bar .conn')).toEqual(['Inactive']);
          expect(conn.getAttribute('aria-label')).toBe('Connection status: inactive');
        },
      );

      When('the person chooses "Use here"', () => {
        one<HTMLButtonElement>('.tab-ban button').click();
        flushSync();
      });

      Then(
        'the banner is gone and the indicator has state "linking" reading "Reconnecting"',
        () => {
          expect(all('.tab-ban')).toEqual([]);
          const conn = one<HTMLAnchorElement>('.bar .conn');
          expect(conn.dataset.st).toBe('linking');
          expect(texts('.bar .conn')).toEqual(['Reconnecting']);
        },
      );
    },
  );

  Scenario(
    'The composer of an inactive tab says the group is in use in another tab',
    ({ Given, When, Then, And }) => {
      Given('a group "Website" read before another tab took the lock', async () => {
        const home = new TestHome('workstation');
        home.add(website, 'Website');
        const client = browser(home);
        homes.push(client.store);
        await client.connect();
        await vi.waitFor(() =>
          expect(client.store.groups.map((group) => group.g)).toEqual([website]),
        );
        client.drop();
        tabState.set('otherTab', true);
      });

      When('the browser opens the chat of "Website"', () => {
        openPage(`#/groups/${website}`);
      });

      Then('the composer is disabled and says "This group is in use in another tab."', () => {
        expect(field().disabled).toBe(true);
        expect(texts('.composer [role=status]')).toEqual(['This group is in use in another tab.']);
      });

      And(
        'the group list says "In another tab" and neither is styled as unreachable',
        () => {
          expect(texts('.gdown')).toEqual(['In another tab']);
          expect(one('.composer .state').className).toContain('away');
          expect(one('.gdown').className).toContain('away');
          expect(all('.composer .state.down, .gdown:not(.away)')).toEqual([]);
        },
      );
    },
  );
});
