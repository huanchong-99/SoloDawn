import 'fake-indexeddb/auto';
import { expect, it, vi } from 'vitest';

it('starts the client in the page body', async () => {
  await import('./main');
  await vi.waitFor(() => expect(document.body.querySelector('nav')).not.toBeNull());
});
