// Host information, AI account declaration and browser pairing per owned node
// (planning/contracts/datachannel.md section 7, planning/contracts/commands.md section 2), and
// the four states of the connection indicator (planning/flows/connect.md step 8 and section E).
import { SvelteMap } from 'svelte/reactivity';
import { lost, newId, type Group, type Home } from './groups.svelte';
import { addHome } from './join';
import type { Net, Path } from './net.svelte';
import { setAccount, signalUrlOf, type AccountKind } from './onboard.svelte';
import type * as p from './proto';

/** Visual states of the connection indicator. */
export type ConnKind = 'ok' | 'limited' | 'linking' | 'down' | 'offline';

/** What the connection indicator shows: its state, and the round-trip latency when direct. */
export interface ConnStatus {
  st: ConnKind;
  rtt?: number;
}

/** One computer of the owner, combining its live home connection, its last `host.info` and path. */
export interface Computer {
  home: Home;
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
  return { st: 'ok', rtt: measured?.rtt };
}

/** The connection state to show in the indicator: `offline` while another tab holds the device
 *  lock; for an open group chat, its home; otherwise the summary across `net.homes`, or `undefined`
 *  before any home is known. */
export function indicator(net: Net, group?: Group): ConnStatus | undefined {
  if (net.otherTab) return { st: 'offline' };
  const targets = group ? [group.home] : net.homes;
  if (targets.length === 0) return undefined;
  const online = targets.filter((home) => home.ctl);
  if (online.length === 0) return { st: 'down' };
  return summary(online.map((home) => net.path(home)));
}

function activeCtl(home: Home): p.Ctl | undefined {
  return home.owner ? home.ctl : undefined;
}

/** Whether a computer of the owner learns the home of `group` from this browser alone: no AI
 *  joins a team group, and another person's computer is known by `home.add` only. */
const unheard = ({ home, info }: Group) => !home.owner || info?.kind === 'team';

/** Holds the latest `host.info` of each owned computer, refreshes it when an owner node connects
 *  or after `agent.set`, and sends `pair.browser`. */
export class Nodes {
  /** The latest `host.info` state for each owned `Home`. */
  readonly infos = new SvelteMap<Home, p.HostInfoPush>();
  #asked = new WeakMap<Home, p.Ctl | undefined>();
  // Per connection, the groups whose home the node knows: those of its `host.info`, and those it
  // was told since, whatever it answered.
  #known = new WeakMap<p.Ctl, Set<string>>();

  /** Requests `host.info` for each newly online owner `Home` in `homes`, or every online owner
   *  `Home` when `again` is true. */
  sync(homes: readonly Home[], again = false) {
    for (const home of homes) {
      const ctl = activeCtl(home);
      if (!again && this.#asked.get(home) === ctl) continue;
      this.#asked.set(home, ctl);
      if (ctl) void this.#read(home, ctl);
    }
  }

  /** Tells each owned computer the homes it needs and does not list in `host.info`, once per
   *  connection (contracts/step-2/people.md section 7, point 2). */
  tell(homes: readonly Home[]) {
    const groups = homes.flatMap((home) => home.groups).filter(unheard);
    for (const [node] of this.infos)
      for (const group of groups) if (group.home !== node) this.#tell(node, group);
  }

  #tell(node: Home, { g, name, home }: Group) {
    const known = node.ctl && this.#known.get(node.ctl);
    if (!known || known.has(g)) return;
    known.add(g);
    void addHome(node, { g, home: home.device }, name, signalUrlOf());
  }

  /** Lists the owner's computers from `net.homes`, with each node's latest `host.info` and path. */
  list(net: Net): Computer[] {
    return net.homes
      .filter((home) => home.owner)
      .map((home) => ({
        home,
        info: this.infos.get(home),
        path: net.path(home),
        online: home.ctl !== undefined,
      }));
  }

  /** Declares `account` for `agent` on `to` with `agent.set`, then refreshes `host.info`. */
  async setAccount(
    home: Home,
    to: string,
    agent: string,
    account: AccountKind,
  ): Promise<p.CmdResult | p.ErrorResponse> {
    const ctl = home.ctl;
    const answer = await setAccount(ctl, home.me, to, agent, account);
    if (!answer.err) await this.#read(home, ctl!);
    return answer;
  }

  /** Asks an online owner node (`to`) for a one-time browser pairing link (`pair.browser`). */
  async pairBrowser(home: Home, to: string): Promise<PairLink | p.ErrorResponse> {
    const env = await home.me.sign(
      JSON.stringify({ t: 'pair.browser', n: newId(), ts: Date.now(), to }),
    );
    const ctl = home.ctl;
    const answer = await (ctl ? ctl.request({ t: 'cmd', env }).catch(lost) : lost());
    if (answer.err) return answer;
    return { link: answer.link!, expires: answer.expires! };
  }

  /** Asks the online owner node `home` for `host.info` with `refresh`: its models listed again. */
  refresh(home: Home) {
    return this.#read(home, home.ctl!, true);
  }

  async #read(home: Home, ctl: p.Ctl, refresh?: true) {
    const answer = await ctl.request({ t: 'host.info', ...(refresh && { refresh }) }).catch(lost);
    if (home.ctl !== ctl || answer.err) return;
    const { re: _re, ok: _ok, ...info } = answer;
    const known = this.#known.get(ctl) ?? new Set();
    for (const { g } of info.homes) known.add(g);
    this.#known.set(ctl, known);
    this.infos.set(home, info);
  }
}

/** The client's shared `Nodes` store. */
export const nodes = new Nodes();
