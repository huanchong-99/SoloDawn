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

it('knows which way along the navigation each change went', async () => {
  const route = await load('#/nodes');
  expect(route.dir).toBe(1);
  for (const [hash, dir] of [
    ['#/groups', -1],
    ['#/settings', 1],
    ['#/nodes', -1],
    ['#/nodes/a-node', 0],
  ] as const) {
    location.hash = hash;
    dispatchEvent(new HashChangeEvent('hashchange'));
    expect(route.dir).toBe(dir);
  }
});
