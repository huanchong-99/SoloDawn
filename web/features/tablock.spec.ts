import { locks } from 'node:worker_threads';
import { describeFeature, loadFeature } from '@amiceli/vitest-cucumber';
import { flushSync, mount, unmount } from 'svelte';
import { SvelteMap } from 'svelte/reactivity';
import { expect, vi } from 'vitest';
import GroupList from '../src/components/groups/GroupList.svelte';
import TabBanner from '../src/components/status/TabBanner.svelte';
import { homes } from '../src/lib/groups.svelte';
import { isDevice } from '../src/lib/identity';
import type { Net } from '../src/lib/net.svelte';
import { prefs } from '../src/lib/prefs.svelte';
import { runTab } from '../src/lib/tablock';

const feature = await loadFeature('../features/web_tablock.feature');

const opened = ['open signalling', 'open peer'];
const closed = ['close signalling', 'close peer'];

/** One tab of the browser: what it opened, closed and showed, in order. */
function tab(device: string) {
  const log: string[] = [];
  const triggers = new EventTarget();
  const useHere = Promise.withResolvers<void>();
  const end = Promise.withResolvers<void>();
  const active = (signal: AbortSignal) => {
    log.push(...opened);
    for (const trigger of ['reconnect', 'probe'])
      triggers.addEventListener(trigger, () => log.push(...opened), { signal });
    signal.addEventListener('abort', () => log.push(...closed));
    return end.promise;
  };
  const notice = () => {
    log.push('other-tab notice');
    return useHere.promise;
  };
  void runTab(device, active, notice);
  return { log, triggers, useHere, end };
}

describeFeature(feature, ({ Background, Scenario, BeforeEachScenario, AfterEachScenario }) => {
  let device: string;
  let first: ReturnType<typeof tab> | undefined;
  let second: ReturnType<typeof tab> | undefined;
  let endActive: (() => void) | undefined;
  let releaseWait: (() => void) | undefined;
  const mounted: Record<string, never>[] = [];

  BeforeEachScenario(() => {
    Object.defineProperty(navigator, 'locks', {
      configurable: true,
      value: locks,
    });
  });

  AfterEachScenario(async () => {
    for (const part of mounted.splice(0)) await unmount(part);
    document.body.innerHTML = '';
    homes.length = 0;
    first?.end.resolve();
    second?.end.resolve();
    endActive?.();
    releaseWait?.();
    first = second = endActive = releaseWait = undefined;
    await vi.waitFor(async () =>
      expect(await navigator.locks.query()).toStrictEqual({ held: [], pending: [] }),
    );
    vi.restoreAllMocks();
  });

  Background(({ Given }) => {
    Given('a browser whose device id begins with "-"', () => {
      device = '-mLU3DYJV6Ej75jYvS8F5Zre6xyO33qzmpZHe4KZ0xg';
      expect(isDevice(device)).toBe(true);
    });
  });

  async function connectFirst() {
    first = tab(device);
    await vi.waitFor(() => expect(first!.log).toStrictEqual(opened));
  }

  async function openSecond() {
    second = tab(device);
    await vi.waitFor(() => expect(second!.log).toStrictEqual(['other-tab notice']));
  }

  async function takeOver() {
    second!.useHere.resolve();
    await vi.waitFor(() => expect(second!.log).toStrictEqual(['other-tab notice', ...opened]));
  }

  Scenario('The second tab offers no connection', ({ Given, When, Then }) => {
    Given('the first tab has connected', connectFirst);
    When('a second tab opens', openSecond);
    Then('the second tab shows the other-tab notice and has opened nothing', async () => {
      expect(second!.log).toStrictEqual(['other-tab notice']);
      expect(first!.log).toStrictEqual(opened);
      expect((await navigator.locks.query()).held).toStrictEqual([
        { name: `solodawn-device-${device}`, mode: 'exclusive', clientId: expect.any(String) },
      ]);
    });
  });

  Scenario('"Use here" moves the connection to the second tab', ({ Given, When, Then, And }) => {
    Given('the first tab has connected', connectFirst);
    And('a second tab shows the other-tab notice', openSecond);
    When('the person chooses "Use here" in the second tab', takeOver);
    Then(
      'the first tab closes its signalling and peer connections and shows the other-tab notice',
      () => {
        expect(first!.log).toStrictEqual([...opened, ...closed, 'other-tab notice']);
      },
    );
    And('the second tab connects', () => {
      expect(second!.log).toStrictEqual(['other-tab notice', ...opened]);
    });
  });

  Scenario(
    'A tab that lost the lock does not connect again on its own',
    ({ Given, When, Then }) => {
      Given('the second tab took the connection from the first', async () => {
        await connectFirst();
        await openSecond();
        await takeOver();
      });
      When('the first tab is asked to reconnect and to probe', () => {
        first!.triggers.dispatchEvent(new Event('reconnect'));
        first!.triggers.dispatchEvent(new Event('probe'));
      });
      Then('the first tab opens no signalling or peer connection', () => {
        expect(first!.log).toStrictEqual([...opened, ...closed, 'other-tab notice']);
      });
    },
  );

  Scenario(
    'The inactive second tab shows only the other-tab placeholder in an empty group list',
    ({ Given, When, Then }) => {
      const tabState = new SvelteMap<'otherTab', boolean>();
      let chooseHere: (() => void) | undefined;
      const net: Net = {
        source: 'live',
        homes,
        path: () => undefined,
        get otherTab() {
          return tabState.get('otherTab') ?? false;
        },
        useHere: () => chooseHere?.(),
      };

      Given('the first tab has connected', connectFirst);
      When('a second tab opens its empty group list', async () => {
        prefs.lang = 'en';
        const session = Promise.withResolvers<void>();
        endActive = () => session.resolve();
        void runTab(
          device,
          () => session.promise,
          () =>
            new Promise<void>((resolve) => {
              tabState.set('otherTab', true);
              chooseHere = () => {
                chooseHere = undefined;
                tabState.set('otherTab', false);
                resolve();
              };
              releaseWait = chooseHere;
            }),
        );
        const main = document.createElement('main');
        main.tabIndex = -1;
        document.body.appendChild(main);
        mounted.push(mount(TabBanner, { target: document.body, props: { net } }) as never);
        mounted.push(
          mount(GroupList, { target: main, props: { net, oncreate: () => {} } }) as never,
        );
        await vi.waitFor(() => {
          flushSync();
          expect(net.otherTab).toBe(true);
        });
      });
      Then(
        'the group list shows "SoloDawn is open in another tab." and hides the empty-state prompt',
        () => {
          const empty = document.querySelector('.empty')!;
          expect(empty.textContent).toContain('SoloDawn is open in another tab.');
          expect(empty.querySelector('h3')).toBeNull();
          expect(empty.textContent).not.toContain('No groups yet');
          expect(empty.textContent).not.toContain(
            'No computer of yours is connected yet. A group lives on one of them.',
          );
          expect(empty.textContent).not.toContain('New group');
          expect(empty.textContent).not.toContain('Open Computers');
        },
      );
      When('the person chooses "Use here" in the second tab', async () => {
        document.querySelector<HTMLButtonElement>('.tab-ban button')!.click();
        await vi.waitFor(() => {
          flushSync();
          expect(net.otherTab).toBe(false);
          expect(first!.log).toStrictEqual([...opened, ...closed, 'other-tab notice']);
        });
      });
      Then('the group list returns to the normal empty state', () => {
        const empty = document.querySelector('.empty')!;
        expect(empty.querySelector('h3')?.textContent).toBe('No groups yet');
        expect(empty.textContent).toContain(
          'No computer of yours is connected yet. A group lives on one of them.',
        );
        expect(empty.textContent).toContain('Open Computers');
        expect(empty.textContent).not.toContain('SoloDawn is open in another tab.');
      });
    },
  );
});
