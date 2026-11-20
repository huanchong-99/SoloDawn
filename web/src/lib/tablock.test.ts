// @vitest-environment node
import { afterEach, expect, it, vi } from 'vitest';
import { runTab } from './tablock';

const device = '-mLU3DYJV6Ej75jYvS8F5Zre6xyO33qzmpZHe4KZ0xg';
const name = `solodawn-device-${device}`;

function tab() {
  const triggers = new EventTarget();
  const log: string[] = [];
  const click = Promise.withResolvers<void>();
  const end = Promise.withResolvers<void>();
  const active = (signal: AbortSignal) => {
    log.push('open signalling', 'open peer');
    triggers.addEventListener('reconnect', () => log.push('reconnect'), { signal });
    triggers.addEventListener('probe', () => log.push('probe'), { signal });
    signal.addEventListener('abort', () => log.push('close signalling', 'close peer'));
    return end.promise;
  };
  const otherTab = () => {
    log.push('other tab');
    return click.promise;
  };
  return { log, triggers, click, end, run: () => runTab(device, active, otherTab) };
}

afterEach(() => vi.restoreAllMocks());

it('waits for the choice even when the lock becomes free before Use here', async () => {
  const request = vi.spyOn(navigator.locks, 'request').mockResolvedValueOnce(null);
  const click = Promise.withResolvers<void>();
  const otherTab = vi.fn().mockReturnValueOnce(click.promise).mockRejectedValue(new Error('ended'));
  const active = vi.fn().mockResolvedValue(undefined);
  const session = runTab(device, active, otherTab);
  await vi.waitFor(() => expect(otherTab.mock.calls).toStrictEqual([[]]));
  expect(active.mock.calls).toStrictEqual([]);
  expect(request).toHaveBeenCalledTimes(1);
  click.resolve();
  await session;
  expect(active).toHaveBeenCalledTimes(1);
  expect(otherTab.mock.calls).toStrictEqual([[]]);
});

it('A1.12: grants only one session, aborts the former holder and never reconnects on its own', async () => {
  const requests = vi.spyOn(navigator.locks, 'request');
  const first = tab();
  const second = tab();
  void first.run();
  const opened = ['open signalling', 'open peer'];
  await vi.waitFor(() => expect(first.log).toStrictEqual(opened));
  const secondRun = second.run();
  await vi.waitFor(() => expect(second.log).toStrictEqual(['other tab']));
  first.triggers.dispatchEvent(new Event('reconnect'));
  first.triggers.dispatchEvent(new Event('probe'));
  expect(first.log).toStrictEqual([...opened, 'reconnect', 'probe']);

  second.click.resolve();
  const retired = [...opened, 'reconnect', 'probe', 'close signalling', 'close peer', 'other tab'];
  await vi.waitFor(() => expect(first.log).toStrictEqual(retired));
  await vi.waitFor(() => expect(second.log).toStrictEqual(['other tab', ...opened]));
  first.triggers.dispatchEvent(new Event('reconnect'));
  first.triggers.dispatchEvent(new Event('probe'));
  expect(first.log).toStrictEqual(retired);

  second.end.resolve();
  await secondRun;
  expect((await navigator.locks.query()).held).not.toContainEqual(
    expect.objectContaining({ name }),
  );
  expect(requests.mock.calls.map(([key, options]) => [key, options])).toStrictEqual([
    [name, { ifAvailable: true }],
    [name, { ifAvailable: true }],
    [name, { steal: true }],
  ]);
  expect(first.log).toStrictEqual(retired);
  first.end.resolve();
});

it('propagates a session’s own AbortError without reporting another tab', async () => {
  const failure = new DOMException('session ended', 'AbortError');
  const otherTab = vi.fn().mockRejectedValue(new Error('unexpected other-tab notice'));
  await expect(runTab(device, () => Promise.reject(failure), otherTab)).rejects.toBe(failure);
  expect(otherTab.mock.calls).toStrictEqual([]);
});

it.each([
  new DOMException('blocked', 'SecurityError'),
  Object.assign(new Error('not a DOM exception'), { name: 'AbortError' }),
])('propagates a lock request failure (%s) without starting a session', async (failure) => {
  vi.spyOn(navigator.locks, 'request').mockRejectedValue(failure);
  const active = vi.fn();
  const otherTab = vi.fn().mockRejectedValue(new Error('unexpected other-tab notice'));
  await expect(runTab(device, active, otherTab)).rejects.toBe(failure);
  expect(active.mock.calls).toStrictEqual([]);
  expect(otherTab.mock.calls).toStrictEqual([]);
});
