// The connection that the shell hands to every page: how this browser reaches the homes of its
// groups. The shell decides once, at start, where the homes come from; pages read them and never
// choose. A development server answers from the node's wire examples (mock.ts), unless the
// address has `?nomock`; everywhere else the homes are the nodes themselves, which the browser's
// connections (peer.ts, unit W-2) reach once they are wired here.
import { Home, homes } from './groups.svelte';
import { loadDevice } from './identity';
import type { Welcome } from './proto';

/** Where the homes come from: the wire examples, or the nodes. */
export type Source = 'mock' | 'live';

/** How a connection reaches its node, as the browser's own statistics show it
 *  (planning/flows/connect.md step 8); the shape of `Path` in peer.ts. */
export interface Path {
  /** `relay` when either candidate of the selected pair is a relay candidate. */
  kind: 'direct' | 'relay';
  /** The latest round trip in milliseconds, where the browser reports one. */
  rtt: number | undefined;
  /** When a path of this kind was first read, in Unix milliseconds. */
  since: number;
}

/** What every page is given. */
export interface Net {
  readonly source: Source;
  /** Every node this browser connects to, with the groups homed on it: `homes` of groups.svelte. */
  readonly homes: Home[];
  /** How this browser reaches a home now; undefined while it does not. */
  path(home: Home): Path | undefined;
}

/** The mock answers in a development server only, and not when the address has `nomock`. */
export const sourceOf = (mode: string, search: string): Source =>
  mode === 'development' && !new URLSearchParams(search).has('nomock') ? 'mock' : 'live';

/** One home answered from the wire examples, online: the examples' one group, read by the
 *  person of the examples with this browser's device key. */
export async function mockHome() {
  const { mockCtl, goldens } = await import('./mock');
  const device = await loadDevice({ crypto, idb: () => indexedDB });
  const user: string = JSON.parse(goldens['row-member-human']).user;
  const home = new Home({ user, sign: (payload) => device.sign(payload) });
  const link = home.connect();
  const ctl = mockCtl(link.push);
  // The mock answers every hello with the welcome of the examples.
  const welcome = (await ctl.request({
    t: 'hello',
    v: 1,
    kind: 'browser',
    name: 'development',
    addrs: [],
  })) as Welcome;
  return { home, online: () => link.online(ctl, welcome) };
}

/** Starts reaching the homes from a source; `stop` takes back what it added. Only a development
 *  build has the mock: the build leaves it out, and the wire examples with it. */
export function start(source: Source) {
  // When each home of the mock came online; the mock's connection is direct.
  const since = new Map<Home, number>();
  const net: Net = {
    source,
    homes,
    path(home) {
      const at = since.get(home);
      return at !== undefined && home.ctl
        ? { kind: 'direct', rtt: undefined, since: at }
        : undefined;
    },
  };
  const added: Home[] = [];
  let stopped = false;
  if (import.meta.env.DEV && source === 'mock') {
    mockHome().then(
      ({ home, online }) => {
        if (stopped) return;
        since.set(home, Date.now());
        added.push(home);
        homes.push(home);
        online();
      },
      // This browser's storage failed: the development server shows no group, and says why.
      (error: unknown) => console.error('The mock home could not start:', error),
    );
  }
  const stop = () => {
    stopped = true;
    for (const home of added) homes.splice(homes.indexOf(home), 1);
  };
  return { net, stop };
}
