import { expect, it, vi } from 'vitest';

/** A page load at the given address. */
async function load(hash: string) {
  vi.resetModules();
  location.hash = hash;
  return (await import('./route.svelte')).route;
}

it.each([
  ['#/nodes', 'nodes'],
  ['#/settings', 'settings'],
  ['#/groups/a-group', 'groups'],
  ['', 'groups'],
  ['#/nowhere', 'groups'],
  ['#settings', 'groups'],
])('opens %j on the %s page', async (hash, page) => {
  expect((await load(hash)).page).toBe(page);
});

it('follows the address when it changes', async () => {
  const route = await load('');
  location.hash = '#/settings';
  dispatchEvent(new HashChangeEvent('hashchange'));
  expect(route.page).toBe('settings');
});
