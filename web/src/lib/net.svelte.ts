// The connection that the shell hands to every page: how this browser reaches the homes of its
// groups. The shell decides once, at start, where the homes come from; pages read them and never
// choose. A development server answers from the node's wire examples (mock.ts), unless the
// address has `?nomock`; everywhere else the homes are the paired nodes, reached through peer.ts
// under the single-tab Web Lock of tablock.ts.
import { SvelteMap } from 'svelte/reactivity';
import type { DeviceLog } from './devlog';
import { Home, homes } from './groups.svelte';
import { loadDevice, type Device } from './identity';
import { joinGroup, JoinError, presentLog, type Invitation } from './join';
import { browserName, signalUrlOf } from './onboard.svelte';
import { loadJoined, loadOwner, saveJoined, saveOwner, type JoinedHomes } from './owner';
import { nodes } from './nodes.svelte';
import { connect, type Peer, type Session } from './peer';
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
  /** Reaches a computer to watch a terminal on it, until the function it returns is called. */
  watchHost?(id: string): () => void;
}

type Joined = Awaited<ReturnType<typeof joinGroup>>;
/** The group joined, and the person's nodes that were not told its home. */
type JoinResult = { g: string; notifications: Awaited<Joined['notices']> };
type Join = NonNullable<Net['join']>;
type WatchHost = NonNullable<Net['watchHost']>;

/** The mock answers in a development server only, and not when the address has `nomock`, nor
 *  in the demo (`demo`), whose world answers behind the live connection (web/features/sim). */
export const sourceOf = (mode: string, search: string): Source => {
  const params = new URLSearchParams(search);
  const demo = params.has('demo');
  return mode === 'development' && !params.has('nomock') && !demo ? 'mock' : 'live';
};

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
    if (await log.apply(env).catch(() => undefined)) changed = true;
  }
  return changed;
}

function startLive(
  added: Map<string, Home>,
  paths: SvelteMap<Home, Path>,
  stopSignal: AbortSignal,
  waitForUseHere: () => Promise<void>,
  setJoin: (join: Join | undefined) => void,
  setWatchHost: (watch: WatchHost | undefined) => void,
) {
  const activeTab = (device: Device, signal: AbortSignal) => {
    let session: Session | undefined;
    let joining: AbortController | undefined;
    let epoch = 0;
    const homeOf = (log: DeviceLog, id: string, name: string) => {
      let home = added.get(id);
      if (!home) {
        home = new Home({ user: log.user, sign: (payload) => device.sign(payload) });
        Object.assign(home, { device: id, name, owner: log.valid(id) });
        added.set(id, home);
        homes.push(home);
      }
      return home;
    };
    const openSession = (log: DeviceLog, addrs: Record<string, string[]>, joined: JoinedHomes) => {
      if (!pairedNodes(log).length) return undefined;
      const url = signalUrlOf();
      // This page's rendezvous and the addresses its nodes gave: a code is looked for on each
      // (contracts/step-2/people.md 4.1).
      const ownPaths = [...new Set([url, ...Object.values(addrs).flat()])];
      const urls = [...ownPaths, ...Object.values(joined).map((h) => h.url)];
      const invited = new Set<string>();
      const watches = new Map<string, number>();
      const hub = connect({
        urls,
        device,
        known: (id) =>
          log.valid(id) || Object.hasOwn(joined, id) || invited.has(id) || watches.has(id),
        name: browserName,
      });
      const peers = new Map<string, Pick<Peer, 'send' | 'close'>>();
      const controller = (joining = new AbortController());
      const onDevlog = async (frame: Devlog) => {
        if (!(await appendEntries(log, frame.entries)) || session !== hub) return;
        await saveOwner(() => indexedDB, log, addrs);
        syncPeers();
        for (const p of peers.values()) presentLog(p, log);
      };
      /** A home that this browser hears of from a node, which is dialled unless it is already. */
      const reach = (id: string) => {
        const home = homeOf(log, id, '');
        if (log.valid(id)) return home;
        if (!Object.hasOwn(joined, id)) {
          joined[id] = { name: '', url };
          void saveJoined(() => indexedDB, joined);
        }
        if (!peers.has(id) && !invited.has(id)) dial(id, home);
        return home;
      };
      const connectHome = (home: Home) => {
        home.onmove = (g, id) => home.move(g, reach(id));
        home.onhomes = (known) => {
          for (const { g, home: id, name } of known) reach(id).expect(g, name);
        };
        const link = home.connect();
        return {
          ...link,
          push(frame: Parameters<typeof link.push>[0]) {
            link.push(frame);
            switch (frame.t) {
              case 'devlog':
                if (frame.user === log.user) void onDevlog(frame);
                break;
              case 'host.info':
                nodes.receive(home, frame);
                break;
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
                presentLog(peer, log);
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
          const home = homeOf(log, id, name);
          if (!peers.has(id)) dial(id, home);
        }
      };
      syncPeers();
      for (const [id, { name }] of Object.entries(joined)) {
        if (!peers.has(id)) dial(id, homeOf(log, id, name));
      }
      const keeps = (id: string) => log.valid(id) || Object.hasOwn(joined, id) || invited.has(id);
      const releaseWatch = (id: string) => {
        if (controller.signal.aborted) return;
        const remaining = watches.get(id)! - 1;
        if (remaining) {
          watches.set(id, remaining);
          return;
        }
        watches.delete(id);
        if (keeps(id)) return;
        peers.get(id)!.close();
        peers.delete(id);
        const home = added.get(id)!;
        added.delete(id);
        homes.splice(homes.indexOf(home), 1);
      };
      setWatchHost((id) => {
        watches.set(id, (watches.get(id) ?? 0) + 1);
        // A computer that is not reached yet is dialled again, its back-off from the start: it may
        // have dropped the offers so far (contracts/step-2/watch-team.md section 3). One that this
        // browser reaches for anyway has dropped none, and may have requests to be sent again.
        const home = homeOf(log, id, id);
        if (keeps(id) ? !peers.has(id) : !home.ctl) dial(id, home);
        return () => releaseWatch(id);
      });
      setJoin(async (invitation, name, cancel) => {
        // A link names the home; a code's home is known once its pair message came.
        let id = invitation.home!;
        let trackPath: ((path: Path | undefined) => void) | undefined;
        let replaced: Home | undefined;
        try {
          const { g, peer, notices } = await joinGroup(invitation, name, {
            device,
            log,
            url,
            urls: ownPaths,
            session: hub,
            browserName,
            signal: AbortSignal.any([cancel, controller.signal]),
            dial(home, listen, onpath) {
              id = home;
              invited.add(id);
              replaced = added.get(id);
              return hub.peer(id, listen, onpath);
            },
            ownNodes: () => {
              const own = new Set(pairedNodes(log).map(([node]) => node));
              return [...added.values()].filter((home) => home.ctl && own.has(home.device));
            },
            async remember(home, at, homeName) {
              joined[home] = { name: homeName, url: at };
              await saveJoined(() => indexedDB, joined);
            },
            link: (target) => {
              const home = homeOf(log, target, joined[target].name);
              trackPath = (path) => (path ? paths.set(home, path) : paths.delete(home));
              return connectHome(home);
            },
            onpath: (path) => trackPath?.(path),
          });
          peers.set(id, peer);
          return { g, notifications: await notices };
        } catch (error) {
          if (replaced && session === hub) dial(id, replaced);
          throw error;
        } finally {
          invited.delete(id);
        }
      });
      return hub;
    };
    const close = () => {
      epoch += 1;
      setJoin(undefined);
      setWatchHost(undefined);
      joining?.abort();
      session?.close();
      session = undefined;
    };
    const syncRoute = async () => {
      if (parse(location.hash).page === 'onboarding') return close();
      if (session) return;
      const run = (epoch += 1);
      const [{ log, addrs }, joined] = await Promise.all([
        loadOwner(crypto, () => indexedDB),
        loadJoined(() => indexedDB),
      ]);
      if (run === epoch && log) session = openSession(log, addrs, joined);
    };
    addEventListener('hashchange', () => void syncRoute(), { signal });
    void syncRoute();
    return new Promise<void>((resolve) =>
      signal.addEventListener('abort', () => (close(), resolve())),
    );
  };

  loadDevice({ crypto, idb: () => indexedDB }).then(
    (device) => {
      if (stopSignal.aborted) return;
      const run = (sig: AbortSignal) => {
        const both = AbortSignal.any([sig, stopSignal]);
        return both.aborted ? Promise.resolve() : activeTab(device, both);
      };
      void runTab(device.id, run, waitForUseHere);
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
  let watchHost = $state.raw<WatchHost>();
  const net: Net = {
    source,
    homes,
    get watchHost() {
      return watchHost;
    },
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
  const waitForUseHere = () =>
    new Promise<void>((resolve) => {
      otherTab = true;
      chooseHere = () => {
        chooseHere = undefined;
        otherTab = false;
        resolve();
      };
    });
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
    startLive(
      added,
      paths,
      stopAbort.signal,
      waitForUseHere,
      (run) => (join = run),
      (run) => (watchHost = run),
    );
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
