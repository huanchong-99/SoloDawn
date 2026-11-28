import { describe, expect, it } from 'vitest';
import { href, pages, parse, routes, tabHref, tabs } from './route.svelte';

describe('the table', () => {
  it('has the design pages and invitation entry point', () => {
    expect(pages).toEqual([
      'onboarding',
      'join',
      'groups',
      'chat',
      'terminal',
      'nodes',
      'settings',
      'states',
    ]);
  });

  it('puts every page of a navigation entry under that entry', () => {
    expect(tabs).toEqual(['groups', 'nodes', 'settings']);
    for (const tab of tabs) expect(routes[tab]).toMatchObject({ path: tab, tab });
  });
});

describe('parse', () => {
  it.each([
    ['#/onboarding', 'onboarding', {}],
    ['#join=secret&g=group&home=node', 'join', { link: '#join=secret&g=group&home=node' }],
    ['#/join', 'join', {}],
    ['#/groups', 'groups', {}],
    ['#/groups/', 'groups', {}],
    ['#/groups/a-group', 'chat', { g: 'a-group' }],
    ['#/groups/a-group/more', 'groups', {}],
    ['#/groups/a-group/', 'chat', { g: 'a-group' }],
    ['#/groups//', 'groups', {}],
    ['#/terminal/g1/m1', 'terminal', { g: 'g1', member: 'm1' }],
    ['#/terminal/g1/m1/', 'terminal', { g: 'g1', member: 'm1' }],
    ['#/terminal/g1/m1/t1', 'terminal', { g: 'g1', member: 'm1', task: 't1' }],
    ['#/terminal/g1/m1/t1/', 'terminal', { g: 'g1', member: 'm1', task: 't1' }],
    ['#/terminal/g1', 'groups', {}],
    ['#/terminal//m1', 'groups', {}],
    ['#/nodes', 'nodes', {}],
    ['#/nodes/a-node', 'groups', {}],
    ['#/nodes/', 'nodes', {}],
    ['#/settings/', 'settings', {}],
    ['#/onboarding/', 'onboarding', {}],
    ['#/terminal/g1/m1/t1/more', 'groups', {}],
    ['#/settings', 'settings', {}],
    ['#/states', 'states', {}],
    ['', 'groups', {}],
    ['#/', 'groups', {}],
    ['#/nowhere', 'groups', {}],
    ['#settings', 'groups', {}],
    ['x/settings', 'groups', {}],
    ['#xnodes', 'groups', {}],
    ['#xgroups/a-group', 'groups', {}],
    ['#/settingsx', 'groups', {}],
    ['#/groups/%E2%9C%93%20ok', 'chat', { g: '✓ ok' }],
    ['#/groups/%E0%A4%A', 'groups', {}],
  ])('opens %j on the page %s with %j', (hash, page, params) => {
    expect(parse(hash)).toStrictEqual({ page, params });
  });

  it('opens the page of a development server in the build as the chats', () => {
    expect(parse('#/states', false)).toStrictEqual({ page: 'groups', params: {} });
    expect(parse('#/settings', false)).toStrictEqual({ page: 'settings', params: {} });
  });

  it('opens the development page in a development build by default', () => {
    expect(import.meta.env.DEV).toBe(true);
    expect(parse('#/states').page).toBe('states');
  });
});

describe('href', () => {
  it('writes the address of a page', () => {
    expect(href('groups')).toBe('#/groups');
    expect(href('settings')).toBe('#/settings');
    expect(href('chat', { g: 'abc' })).toBe('#/groups/abc');
    expect(href('terminal', { g: 'g1', member: 'm1' })).toBe('#/terminal/g1/m1');
    expect(href('terminal', { g: 'g1', member: 'm1', task: 't1' })).toBe('#/terminal/g1/m1/t1');
  });

  it('escapes what a parameter holds, and parse reads it back', () => {
    const address = href('chat', { g: 'a/b c?' });
    expect(address).toBe('#/groups/a%2Fb%20c%3F');
    expect(parse(address)).toStrictEqual({ page: 'chat', params: { g: 'a/b c?' } });
  });

  it('is read back as the same page for every page', () => {
    const params = { g: 'g1', member: 'm1', task: 't1' };
    for (const page of pages) {
      const address = href(page as 'terminal', params);
      expect(parse(address).page).toBe(page);
    }
  });
});

describe('tabHref', () => {
  it('reopens the remembered group only on wide screens while it still exists', () => {
    expect(tabHref('groups', true, (g) => g === 'g1', 'g1')).toBe('#/groups/g1');
    expect(tabHref('groups', true, () => true, 'a/b c?')).toBe('#/groups/a%2Fb%20c%3F');
    expect(tabHref('groups', true, () => false, 'g1')).toBe('#/groups');
    expect(tabHref('groups', false, () => true, 'g1')).toBe('#/groups');
    expect(tabHref('groups', true, () => true, undefined)).toBe('#/groups');
    expect(tabHref('nodes', true, () => true, 'g1')).toBe('#/nodes');
    expect(tabHref('settings', true, () => true, 'g1')).toBe('#/settings');
  });
});
