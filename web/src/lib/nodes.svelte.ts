// Host information and browser pairing per owned node (planning/contracts/datachannel.md section
// 7, planning/contracts/commands.md section 2), and the four states of the connection indicator
// (planning/flows/connect.md step 8 and section E).
import { SvelteMap } from 'svelte/reactivity';
import { lost, newId, type Group, type Home } from './groups.svelte';
import type { Net, Path } from './net.svelte';
import type * as p from './proto';

/** The four visual states of the top-bar connection indicator. */
export type ConnKind = 'ok' | 'limited' | 'linking' | 'down';

/** What the connection indicator shows: its state, and the round-trip latency when direct. */
export interface ConnStatus {
  st: ConnKind;
  rtt?: number;
}

/** One computer of the owner, combining its live home connection, its last `host.info` and path. */
export interface Computer {
  home: Home | undefined;
  info: p.HostInfoPush | undefined;
  path: Path | undefined;
  online: boolean;
}

/** The one-time pairing link returned by `pair.browser` (planning/flows/pairing.md section C). */
export interface PairLink {
  link: string;
  expires: number;
}

function summary(paths: (Path | undefined)[]): ConnStatus {
  if (paths.some((path) => !path)) return { st: 'linking' };
  const live = paths as Path[];
  if (live.some((path) => path.kind === 'relay')) return { st: 'limited' };
  const measured = live.find((path) => path.rtt !== undefined);
  return { st: 'ok', rtt: measured ? measured.rtt : undefined };
}

/** The connection state to show in the top bar: for an open group chat, its home; otherwise the
 *  summary across `net.homes`, or `undefined` before any home is known. */
export function indicator(net: Net, group?: Group): ConnStatus | undefined {
  const targets = group ? [group.home] : net.homes;
  if (targets.length === 0) return undefined;
  const online = targets.filter((home) => home.ctl);
  if (online.length === 0) return { st: 'down' };
  return summary(online.map((home) => net.path(home)));
}

function activeCtl(home: Home): p.Ctl | undefined {
  return home.owner ? home.ctl : undefined;
}

/** Holds the latest `host.info` of each owned computer, refreshes it when an owner node connects,
 *  applies `host.info` pushes, and sends `pair.browser`. */
export class Nodes {
  /** The latest `host.info` state per node device id. */
  readonly infos = new SvelteMap<string, p.HostInfoPush>();
  /** The device id reported by `host.info` for each connected owner `Home`. */
  readonly devices = new SvelteMap<Home, string>();
  #asked = new WeakMap<Home, p.Ctl | undefined>();

  /** Requests `host.info` once for each newly online owner `Home` in `homes`. */
  sync(homes: readonly Home[]) {
    for (const home of homes) {
      const ctl = activeCtl(home);
      if (this.#asked.get(home) === ctl) continue;
      this.#asked.set(home, ctl);
      if (ctl) void this.#read(home, ctl);
    }
  }

  /** Applies a `host.info` push from a node; ignores other pushes. */
  push(frame: p.Push) {
    if (frame.t === 'host.info') this.infos.set(frame.device, frame);
  }

  /** Lists the owner's computers: owned homes first, then any cached node without a home. */
  list(net: Net): Computer[] {
    const seen = new Set<string | undefined>();
    const fromHomes = net.homes
      .filter((home) => home.owner)
      .map((home) => {
        const device = this.devices.get(home);
        seen.add(device);
        return {
          home,
          info: this.infos.get(device as string),
          path: net.path(home),
          online: home.ctl !== undefined,
        };
      });
    const offline = [...this.infos.values()]
      .filter((info) => !seen.has(info.device))
      .map((info) => ({ home: undefined, info, path: undefined, online: false }));
    return [...fromHomes, ...offline];
  }

  /** Asks an online owner node (`to`) for a one-time browser pairing link (`pair.browser`). */
  async pairBrowser(home: Home, to: string, n = newId()): Promise<PairLink | p.ErrorResponse> {
    const env = await home.me.sign(JSON.stringify({ t: 'pair.browser', n, ts: Date.now(), to }));
    const ctl = home.ctl;
    const answer = await (ctl ? ctl.request({ t: 'cmd', env }).catch(lost) : lost());
    if (answer.err) return answer;
    return { link: answer.link!, expires: answer.expires! };
  }

  async #read(home: Home, ctl: p.Ctl) {
    const answer = await ctl.request({ t: 'host.info' }).catch(lost);
    if (home.ctl !== ctl || answer.err) return;
    const { re: _re, ok: _ok, ...info } = answer;
    this.devices.set(home, info.device);
    this.infos.set(info.device, info);
  }
}

/** The client's shared `Nodes` store. */
export const nodes = new Nodes();
