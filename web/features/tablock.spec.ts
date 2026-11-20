// @vitest-environment node
import { describeFeature, loadFeature } from '@amiceli/vitest-cucumber';
import { expect, vi } from 'vitest';
import { isDevice } from '../src/lib/identity';
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

describeFeature(feature, ({ Background, Scenario, AfterEachScenario }) => {
  let device: string;
  let first: ReturnType<typeof tab>;
  let second: ReturnType<typeof tab>;

  AfterEachScenario(async () => {
    first.end.resolve();
    second.end.resolve();
    await vi.waitFor(async () =>
      expect(await navigator.locks.query()).toStrictEqual({ held: [], pending: [] }),
    );
  });

  Background(({ Given }) => {
    Given('a browser whose device id begins with "-"', () => {
      device = '-mLU3DYJV6Ej75jYvS8F5Zre6xyO33qzmpZHe4KZ0xg';
      expect(isDevice(device)).toBe(true);
    });
  });

  async function connectFirst() {
    first = tab(device);
    await vi.waitFor(() => expect(first.log).toStrictEqual(opened));
  }

  async function openSecond() {
    second = tab(device);
    await vi.waitFor(() => expect(second.log).toStrictEqual(['other-tab notice']));
  }

  async function takeOver() {
    second.useHere.resolve();
    await vi.waitFor(() => expect(second.log).toStrictEqual(['other-tab notice', ...opened]));
  }

  Scenario('The second tab offers no connection', ({ Given, When, Then }) => {
    Given('the first tab has connected', connectFirst);
    When('a second tab opens', openSecond);
    Then('the second tab shows the other-tab notice and has opened nothing', async () => {
      expect(second.log).toStrictEqual(['other-tab notice']);
      expect(first.log).toStrictEqual(opened);
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
        expect(first.log).toStrictEqual([...opened, ...closed, 'other-tab notice']);
      },
    );
    And('the second tab connects', () => {
      expect(second.log).toStrictEqual(['other-tab notice', ...opened]);
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
        first.triggers.dispatchEvent(new Event('reconnect'));
        first.triggers.dispatchEvent(new Event('probe'));
      });
      Then('the first tab opens no signalling or peer connection', () => {
        expect(first.log).toStrictEqual([...opened, ...closed, 'other-tab notice']);
      });
    },
  );
});
