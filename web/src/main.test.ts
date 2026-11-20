import 'fake-indexeddb/auto';
import { expect, it, vi } from 'vitest';

it('starts the client in the page body', async () => {
  // happy-dom has no navigator.storage, which every browser that passes the boot check has.
  const persist = vi.fn(() => Promise.resolve(false));
  Object.assign(navigator, { storage: { persist } });
  await import('./main');
  await vi.waitFor(() => expect(document.body.querySelector('nav')).not.toBeNull());
  expect(persist).toHaveBeenCalledOnce();
});
