// The connection that the shell hands to every page: how this browser reaches the homes of its
// groups. The shell decides once, at start, where the homes come from; pages read them and never
// choose. A development server answers from the node's wire examples (mock.ts), unless the
// address has `?nomock`; everywhere else the homes are the paired nodes, reached through peer.ts
// under the single-tab Web Lock of tablock.ts.
import { SvelteMap } from 'svelte/reactivity';
import type { DeviceLog } from './devlog';
import { Home, homes, type Link } from './groups.svelte';
import { loadDevice, type Device } from './identity';
import { joinGroup, JoinError, presentLog, type Invitation } from './join';
import { browserName, signalUrlOf } from './onboard.svelte';
import { loadJoined, loadOwner, saveJoined, saveOwner, type JoinedHomes } from './owner';
import { connect, type Peer } from './peer';
import type { Devlog, Welcome } from './proto';
import { parse } from './route.svelte';
import { runTab } from './tablock';
import { hello } from './version';

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
  /** True while another tab of this browser holds the device's connection lock. */
  readonly otherTab?: boolean;
  /** Steals the device's connection lock for this tab after the person chooses "Use here". */
  useHere?(): void;
  join?(invitation: Invitation, name: string, signal: AbortSignal): Promise<JoinResult>;
}

type JoinResult = { notifications: Awaited<Awaited<ReturnType<typeof joinGroup>>['notices']> };
type Join = NonNullable<Net['join']>;

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
  const welcome = (await hello(ctl.request, 'development')) as Welcome;
  return { home, online: () => link.online(ctl, welcome) };
}

function pairedNodes(log: DeviceLog): [string, string][] {
  const found = new Map<string, string>();
  for (const env of log.entries) {
    const entry = JSON.parse(env.p) as { dev: string; kind?: string; name?: string };
    if (entry.kind === 'node' && log.valid(entry.dev)) found.set(entry.dev, entry.name!);
  }
  return [...found];
}

async function appendEntries(log: DeviceLog, entries: Devlog['entries']): Promise<boolean> {
  let changed = false;
  for (const env of entries) {
    if (await log.apply(env).catch(() => {})) changed = true;
  }
  return changed;
}

function startLive(
  added: Map<string, Home>,
  paths: SvelteMap<Home, Path>,
  stopSignal: AbortSignal,
  waitForUseHere: () => Promise<void>,
  setJoin: (join: Join | undefined) => void,
) {
  const activeTab = (device: Device, signal: AbortSignal) => {
    if (signal.aborted) return Promise.resolve();
    let session: (() => void) | undefined;
    let pending: symbol | undefined;
    const homeOf = (user: string, id: string, name: string, owner = false) => {
      let home = added.get(id);
      if (!home) {
        home = new Home({ user, sign: (payload) => device.sign(payload) });
        Object.assign(home, { device: id, name, owner });
        added.set(id, home);
        homes.push(home);
      }
      return home;
    };
    const openSession = (log: DeviceLog, addrs: Record<string, string[]>, joined: JoinedHomes) => {
      if (!pairedNodes(log).length) return undefined;
      const urls = [
        signalUrlOf(),
        ...Object.values(addrs).flat(),
        ...Object.values(joined).map((home) => home.url),
      ];
      const pendingHomes = new Set<string>();
      const hub = connect({
        urls,
        device,
        known: (id) => log.valid(id) || Object.hasOwn(joined, id) || pendingHomes.has(id),
        name: browserName,
      });
      const peers = new Map<string, Pick<Peer, 'send'>>();
      const joining = new AbortController();
      const pushLog = (): Devlog => ({ t: 'devlog', user: log.user, entries: [...log.entries] });
      const onDevlog = async (frame: Devlog) => {
        if (!(await appendEntries(log, frame.entries)) || joining.signal.aborted) return;
        await saveOwner(() => indexedDB, log, addrs);
        syncPeers();
        for (const peer of peers.values()) presentLog(peer, log);
      };
      const connectHome = (home: Home): Link => {
        const link = home.connect();
        return {
          ...link,
          push(frame) {
            link.push(frame);
            switch (frame.t) {
              case 'devlog':
                if (frame.user === log.user) void onDevlog(frame);
            }
          },
        };
      };
      const dial = (id: string, home: Home) => {
        const peer = hub.peer(
          id,
          () => {
            const link = connectHome(home);
            return {
              online: (ctl, welcome) => {
                addrs[id] = welcome.addrs;
                void saveOwner(() => indexedDB, log, addrs);
                peer.send(pushLog());
                link.online(ctl, welcome);
              },
              push: link.push,
              offline: link.offline,
            };
          },
          (path) => (path ? paths.set(home, path) : paths.delete(home)),
        );
        peers.set(id, peer);
      };
      const syncPeers = () => {
        for (const [id, name] of pairedNodes(log)) {
          const home = homeOf(log.user, id, name, true);
          if (!peers.has(id)) dial(id, home);
        }
      };
      syncPeers();
      for (const [id, { name }] of Object.entries(joined)) {
        if (!peers.has(id)) dial(id, homeOf(log.user, id, name));
      }
      setJoin(async (invitation, name, cancel) => {
        const id = invitation.home;
        const url = signalUrlOf();
        if (pendingHomes.has(id)) throw new JoinError('room', 'conflict');
        pendingHomes.add(id);
        let trackPath: ((path: Path | undefined) => void) | undefined;
        let replaced: Home | undefined;
        try {
          const { peer, notices } = await joinGroup(invitation, name, {
            device,
            log,
            url,
            session: hub,
            dial(listen, onpath) {
              replaced = added.get(id);
              return hub.peer(id, listen, onpath);
            },
            browserName,
            signal: AbortSignal.any([cancel, signal, joining.signal]),
            ownNodes: () => {
              const own = new Set(pairedNodes(log).map(([id]) => id));
              return [...added.values()].filter((home) => home.ctl && own.has(home.device));
            },
            async remember(homeName) {
              joined[id] = { name: homeName, url };
              await saveJoined(() => indexedDB, joined);
            },
            link: () => {
              const home = homeOf(log.user, id, joined[id].name);
              trackPath = (path) => (path ? paths.set(home, path) : paths.delete(home));
              return connectHome(home);
            },
            onpath: (path) => trackPath?.(path),
          });
          peers.set(id, peer);
          return { notifications: await notices };
        } catch (error) {
          if (replaced && !joining.signal.aborted) dial(id, replaced);
          throw error;
        } finally {
          pendingHomes.delete(id);
        }
      });
      return () => {
        joining.abort();
        hub.close();
      };
    };
    const close = () => {
      pending = undefined;
      setJoin(undefined);
      session?.();
      session = undefined;
    };
    const syncRoute = async () => {
      if (parse(location.hash).page === 'onboarding') return close();
      if (session) return;
      const run = (pending = Symbol());
      const [{ log, addrs }, joined] = await Promise.all([
        loadOwner(crypto, () => indexedDB),
        loadJoined(() => indexedDB),
      ]);
      if (run === pending && log) session = openSession(log, addrs, joined);
    };
    addEventListener('hashchange', () => void syncRoute(), { signal });
    void syncRoute();
    return new Promise<void>((resolve) =>
      signal.addEventListener('abort', () => (close(), resolve())),
    );
  };

  loadDevice({ crypto, idb: () => indexedDB }).then(
    (device) => {
      const run = (sig: AbortSignal) => activeTab(device, AbortSignal.any([sig, stopSignal]));
      if (!stopSignal.aborted) void runTab(device.id, run, waitForUseHere);
    },
    () => {},
  );
}

/** Starts reaching the homes from a source; `stop` takes back what it added. Only a development
 *  build has the mock: the build leaves it out, and the wire examples with it. */
export function start(source: Source) {
  const paths = new SvelteMap<Home, Path>();
  const added = new Map<string, Home>();
  const stopAbort = new AbortController();
  let otherTab = $state(false);
  let chooseHere: (() => void) | undefined;
  let join: Join | undefined;
  const net: Net = {
    source,
    homes,
    path: (home) => (home.ctl ? paths.get(home) : undefined),
    get otherTab() {
      return otherTab;
    },
    useHere: () => chooseHere?.(),
    join: (invitation, name, signal) => {
      if (!join) return Promise.reject(new JoinError('onboarding', 'unpaired'));
      return join(invitation, name, signal);
    },
  };
  function waitForUseHere() {
    const { promise, resolve } = Promise.withResolvers<void>();
    otherTab = true;
    chooseHere = () => {
      chooseHere = undefined;
      otherTab = false;
      resolve();
    };
    return promise;
  }
  if (import.meta.env.DEV && source === 'mock') {
    mockHome().then(
      ({ home, online }) => {
        if (stopAbort.signal.aborted) return;
        paths.set(home, { kind: 'direct', rtt: undefined, since: Date.now() });
        added.set(home.me.user, home);
        homes.push(home);
        online();
      },
      // This browser's storage failed: the development server shows no group, and says why.
      (error: unknown) => console.error('The mock home could not start:', error),
    );
  } else {
    startLive(added, paths, stopAbort.signal, waitForUseHere, (run) => (join = run));
  }
  const stop = () => {
    stopAbort.abort();
    chooseHere = undefined;
    otherTab = false;
    for (const home of added.values()) homes.splice(homes.indexOf(home), 1);
    paths.clear();
  };
  return { net, stop };
}
