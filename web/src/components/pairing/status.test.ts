import { afterEach, expect, it } from 'vitest';
import { all, closePage, show } from '../../../features/page';
import type { PairingSession } from '../../lib/onboard.svelte';
import type { Path } from '../../lib/peer';
import { prefs } from '../../lib/prefs.svelte';
import StatusPanel from './StatusPanel.svelte';

afterEach(closePage);

/** The title and the line under it of the connection step, as the panel draws them. */
function connectStep(enrolled: boolean, kind?: Path['kind'], lang: typeof prefs.lang = 'en') {
  prefs.lang = lang;
  const path = kind && { kind, rtt: 4, since: 0 };
  const session = { stage: enrolled ? 'ready' : 'linking', role: 'a', enrolled, path, agents: [] };
  show(StatusPanel, { session: session as unknown as PairingSession });
  const step = all('.link-steps li').at(-2)!;
  return [step.querySelector('b')!.textContent, step.querySelector('.muted')!.textContent];
}

it('names the connection step by the path that carries the connection once it is made', () => {
  expect(connectStep(false, 'direct')).toEqual([
    'Opening a direct connection',
    'Trying UDP, TCP and IPv6 at once. The first that works wins.',
  ]);
  expect(connectStep(true)).toEqual(['Opening a direct connection', 'Enrolled as your device']);
  expect(connectStep(true, 'direct')).toEqual([
    'Direct connection established',
    'Enrolled as your device',
  ]);
  expect(connectStep(true, 'relay')).toEqual([
    'Connected through encrypted relay',
    'Enrolled as your device',
  ]);
  expect(connectStep(true, 'direct', 'zh-CN')).toEqual(['已建立直连', '已登记为你的设备']);
});
