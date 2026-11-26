import { expect, it, vi } from 'vitest';
import { hello } from './version';

it('says hello with version 3 once, and takes a refusal of the version as the answer', async () => {
  const refusal = { re: 1, err: { code: 'version', msg: 'unsupported version' } };
  const send = vi.fn(async () => refusal);
  expect(await hello(send, 'phone browser')).toBe(refusal);
  expect(send.mock.calls).toEqual([
    [{ t: 'hello', v: 3, kind: 'browser', name: 'phone browser', addrs: [] }],
  ]);
});
