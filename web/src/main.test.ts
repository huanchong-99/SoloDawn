import 'fake-indexeddb/auto';
import { afterEach, expect, it, vi } from 'vitest';

it('starts the client in the page body', async () => {
  await import('./main');
  await vi.waitFor(() => expect(document.body.querySelector('nav')).not.toBeNull());
});

/** Loads main.ts afresh at `search`, in development or not, and returns what it called, in order. */
async function load(search: string, dev: boolean): Promise<string[]> {
  const calls: string[] = [];
  vi.resetModules();
  vi.stubEnv('DEV', dev);
  history.replaceState(null, '', search || location.pathname);
  vi.doMock('./lib/boot', () => ({
    boot: (root: HTMLElement) => calls.push(`boot ${root.tagName}`),
  }));
  vi.doMock('../features/sim/install', () => ({ install: async () => void calls.push('install') }));
  await import('./main');
  await vi.waitFor(() => expect(calls.at(-1)).toMatch(/^boot/));
  return calls;
}

afterEach(() => {
  vi.doUnmock('./lib/boot');
  vi.doUnmock('../features/sim/install');
  vi.unstubAllEnvs();
  history.replaceState(null, '', location.pathname);
});

it('plays the demo world at ?demo on a development server, and only there', async () => {
  expect(await load('?demo', true)).toEqual(['install', 'boot BODY']);
  expect(await load('', true)).toEqual(['boot BODY']);
  expect(await load('?other', true)).toEqual(['boot BODY']);
  expect(await load('?demo', false)).toEqual(['boot BODY']);
});
