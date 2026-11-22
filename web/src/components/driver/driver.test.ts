import { afterEach, expect, test } from 'vitest';
import { Home } from '../../lib/groups.svelte';
import { goldens } from '../../lib/mock';
import { nodes } from '../../lib/nodes.svelte';
import * as p from '../../lib/proto';
import { offeredAccounts, offersTerminal } from './driver';

const sign = async (payload: string) => ({ p: payload, k: 'k', s: 's' });

afterEach(() => nodes.infos.clear());

test('offeredAccounts keeps the known types the driver allows, in the client order', () => {
  expect(offeredAccounts({ accounts: ['api_key', 'team', 'other'], takeover: 'none' })).toEqual([
    'team',
    'api_key',
  ]);
  expect(offeredAccounts({ accounts: ['api_key'], takeover: 'none' })).toEqual(['api_key']);
});

test('offersTerminal is false when a connected host declares no takeover for the agent', () => {
  const home = new Home({ user: 'u', sign });
  expect(offersTerminal(undefined, 'claude')).toBe(true);
  expect(offersTerminal(home, 'claude')).toBe(true);
  home.ctl = {} as p.Ctl;
  expect(offersTerminal(home, 'claude')).toBe(false);
  const info = p.decode(p.HostInfoPush, goldens['host-info-push']);
  info.agents[0].driver.takeover = 'none';
  nodes.infos.set(home, info);
  expect(offersTerminal(home, info.agents[0].agent)).toBe(false);
  expect(offersTerminal(home, info.agents[1].agent)).toBe(true);
  expect(offersTerminal(home, 'opencode')).toBe(false);
});
