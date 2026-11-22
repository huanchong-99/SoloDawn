import 'fake-indexeddb/auto';
import { describeFeature, loadFeature } from '@amiceli/vitest-cucumber';
import { flushSync } from 'svelte';
import { SvelteMap } from 'svelte/reactivity';
import { expect, vi } from 'vitest';
import { homes } from '../src/lib/groups.svelte';
import type { Home } from '../src/lib/groups.svelte';
import type { Source } from '../src/lib/net.svelte';
import { prefs } from '../src/lib/prefs.svelte';
import { browser, TestHome } from './home';
import { all, closePage, one, openPage, texts } from './page';

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
    prefs.lang = 'en';
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

      And(
        'in Chinese the banner reads "SoloDawn 已在另一个标签页中打开。" with "在此使用" and the indicator reads "未连接"',
        () => {
          prefs.lang = 'zh-CN';
          flushSync();
          const ban = one('.ban .tab-ban[role="status"]');
          expect(ban.querySelector('.grow')?.textContent?.trim()).toBe(
            'SoloDawn 已在另一个标签页中打开。',
          );
          expect(ban.querySelector('button')?.textContent?.trim()).toBe('在此使用');
          const conn = one<HTMLAnchorElement>('.bar .conn');
          expect(texts('.bar .conn')).toEqual(['未连接']);
          expect(conn.getAttribute('aria-label')).toBe('连接状态：未连接');
          prefs.lang = 'en';
          flushSync();
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

  Scenario('The states sheet lists the status mount at banner only', ({ When, Then }) => {
    When('the person opens "#/states" in a development server', async () => {
      vi.stubEnv('MODE', 'development');
      openPage('#/states');
      await vi.waitFor(() => {
        flushSync();
        expect(all('.states')).toHaveLength(1);
      });
    });

    Then(
      'the mounting points show "status · 10" at "banner" and do not list "status" at "chat.top"',
      () => {
        const dts = texts('.points dt');
        const dds = all('.points dd');
        expect(dds[dts.indexOf('banner')].textContent).toContain('status · 10');
        expect(dds[dts.indexOf('chat.top')].textContent).not.toContain('status');
      },
    );
  });
});
