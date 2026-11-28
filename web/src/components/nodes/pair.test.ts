import { flushSync } from 'svelte';
import { SvelteMap } from 'svelte/reactivity';
import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import { all, closePage, one, show, texts } from '../../../features/page';
import { Home, homes } from '../../lib/groups.svelte';
import { goldens } from '../../lib/mock';
import type { Net, OwnerDevice } from '../../lib/net.svelte';
import { nodes } from '../../lib/nodes.svelte';
import { fingerprintOf } from '../../lib/onboard.svelte';
import { prefs } from '../../lib/prefs.svelte';
import * as p from '../../lib/proto';
import PairBrowserDialog from './PairBrowserDialog.svelte';

beforeEach(() => {
  prefs.lang = 'en';
});
afterEach(async () => {
  await closePage();
  nodes.infos.clear();
  devices.clear();
});

const me = { user: 'alex', sign: async (payload: string) => ({ p: payload, k: 'k', s: 's' }) };
// The owner's devices, as `net.devices` gives them.
const devices = new SvelteMap<string, OwnerDevice>();
const info = p.decode(p.HostInfoPush, goldens['host-info-push']);
/** A device that `by` added: the computer of the dialog, unless another is given. */
const device = (id: string, kind: OwnerDevice['kind'], by = info.device) =>
  devices.set(id, { id, kind, name: kind, by, self: false });

/** Opens the dialog on one online computer and creates the link that the computer answers with. */
async function createLink(link: string) {
  const home = new Home(me);
  const expires = Date.now() + 600_000;
  const answer = { ...p.decode(p.CmdResult, goldens['cmd-pair-link']), link, expires };
  Object.assign(home, { owner: true, ctl: { request: async () => answer } });
  nodes.infos.set(home, info);
  homes.push(home);
  const net: Net = {
    source: 'live',
    homes,
    path: () => undefined,
    get devices() {
      return [...devices.values()];
    },
  };
  show(PairBrowserDialog, { open: true, net });
  one('dialog form').dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }));
  await vi.waitFor(() => {
    flushSync();
    expect(texts('dialog .cmd')).toEqual([link]);
  });
}

it('says that a localhost link opens only on the computer that created it', async () => {
  await createLink('http://localhost:47800/#pair=7K2M-9QXD-4F8T-WB3N');
  expect(texts('dialog .pair-box .note')).toEqual([
    'This is a localhost link: it opens only in a browser on the computer that created it, not on a phone or another computer.',
  ]);
});

it('says nothing more of a link that other devices can open', async () => {
  await createLink('https://rv.example/#pair=7K2M-9QXD-4F8T-WB3N');
  expect(all('dialog .pair-box .note')).toEqual([]);
});

it('shows that the link was used, with the fingerprint of the browser that joined', async () => {
  const phone = 'J3Uuh-y1z2AX536yAFlGHJL5sjxVpWVJavvVphvdm5Q';
  device('akWV_5PtNLIGhakuMAvpD9LcG1D8TUfc1URh48VREcs', 'browser');
  await createLink('http://localhost:47800/#pair=7K2M-9QXD-4F8T-WB3N');
  expect(one('dialog .pair-box .hint').textContent).toContain('Expires in');

  // A computer that is added while the link is open used no link, and neither did a browser
  // that another device added: the computer that made the link adds the browser that uses it.
  device('rZNvy-1jH6Z-BcPqA5U5BQrZNvy-1jH6Z-BcPqA5U5BQ', 'node');
  device('7K2M9QXD4F8TWB3N7K2M9QXD4F8TWB3N7K2M9QXD4F8', 'browser', 'another-computer');
  flushSync();
  expect(one('dialog .pair-box .hint').textContent).toContain('Expires in');
  expect(all('dialog .pair-box .note code')).toEqual([]);

  device(phone, 'browser');
  flushSync();
  expect(texts('dialog .pair-box .hint')).toEqual(['This link was used and discarded.']);
  const print = await fingerprintOf(phone);
  await vi.waitFor(() => expect(texts('dialog .pair-box .note code')).toEqual([print]));
  expect(texts('dialog .pair-box .note')).toEqual([
    `The fingerprint of the browser that joined. It should match the one its page shows under “This browser”: ${print}`,
  ]);
});
