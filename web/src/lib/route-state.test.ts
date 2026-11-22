import { describe, expect, it, vi } from 'vitest';

/** A page load at the given address. */
async function load(hash: string) {
  vi.resetModules();
  location.hash = hash;
  return (await import('./route.svelte')).route;
}

/** The address changes, as the browser tells it. */
function go(hash: string) {
  location.hash = hash;
  dispatchEvent(new HashChangeEvent('hashchange', { newURL: location.href }));
}

describe('route', () => {
  it.each([
    ['#/nodes', 'nodes', {}],
    ['#/groups/a-group', 'chat', { g: 'a-group' }],
    ['#/nowhere', 'groups', {}],
  ])('opens %j on the %s page', async (hash, page, params) => {
    expect(await load(hash)).toEqual({ page, params, dir: 1 });
  });

  it('follows the address when it changes', async () => {
    const route = await load('');
    go('#/groups/g2');
    expect(route).toEqual({ page: 'chat', params: { g: 'g2' }, dir: 0 });
    go('#/settings');
    expect(route).toEqual({ page: 'settings', params: {}, dir: 1 });
  });

  it('knows which way along the navigation each change went', async () => {
    const route = await load('#/nodes');
    for (const [hash, dir] of [
      ['#/groups', -1],
      ['#/settings', 1],
      ['#/nodes', -1],
      ['#/nodes', 0],
      ['#/terminal/g/m', -1],
      ['#/groups/g', 0],
      ['#/onboarding', -1],
      ['#/groups', 1],
      ['#/states', 1],
      ['#/settings', -1],
    ] as const) {
      go(hash);
      expect([hash, route.dir]).toEqual([hash, dir]);
    }
  });

  it('follows each destination when two hash changes are queued', async () => {
    const route = await load('#/groups');
    location.hash = '#/settings';
    const first = new HashChangeEvent('hashchange', { newURL: location.href });
    location.hash = '#/nodes';
    const second = new HashChangeEvent('hashchange', { newURL: location.href });
    dispatchEvent(first);
    expect(route).toEqual({ page: 'settings', params: {}, dir: 1 });
    dispatchEvent(second);
    expect(route).toEqual({ page: 'nodes', params: {}, dir: -1 });
  });
});
