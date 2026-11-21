// Host information, AI account declaration and browser pairing per owned node
// (planning/contracts/datachannel.md section 7, planning/contracts/commands.md section 2), and
// the four states of the connection indicator (planning/flows/connect.md step 8 and section E).
import { SvelteMap } from 'svelte/reactivity';
import { lost, newId, type Group, type Home } from './groups.svelte';
import type { Net, Path } from './net.svelte';
import { t } from './prefs.svelte';
import type * as p from './proto';

/** The four visual states of the connection indicator. */
export type ConnKind = 'ok' | 'limited' | 'linking' | 'down';

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

/** Account types declared with `agent.set` (planning/contracts/commands.md section 2). */
export const accounts = ['personal', 'team', 'api_key'] as const;
/** One of the three AI account types accepted by `agent.set`. */
export type AccountKind = (typeof accounts)[number];

const cliLabels: Record<string, string> = { claude: 'Claude Code', codex: 'Codex' };

/** Display label of an AI CLI identifier. */
export const cliName = (agent: string) => cliLabels[agent] ?? agent;

const accountKeys: Record<string, `nodes.account.${AccountKind}` | undefined> = {
  personal: 'nodes.account.personal',
  team: 'nodes.account.team',
  api_key: 'nodes.account.api_key',
};

/** Localized label of a declared AI account type, or the unset label when `null` or unknown. */
export const accountLabel = (account: string | null) => {
  const key = accountKeys[account as string];
  return t(key ?? 'nodes.account.unset');
};

/** Sends a signed `agent.set` host command (planning/contracts/commands.md section 2). */
export async function setAccount(
  ctl: p.Ctl | undefined,
  signer: { sign(p: string): Promise<p.Envelope> },
  to: string,
  agent: string,
  account: AccountKind,
): Promise<p.CmdResult | p.ErrorResponse> {
  const env = await signer.sign(
    JSON.stringify({ t: 'agent.set', n: newId(), ts: Date.now(), to, agent, account }),
  );
  return ctl ? ctl.request({ t: 'cmd', env }).catch(lost) : lost();
}

function summary(paths: (Path | undefined)[]): ConnStatus {
  if (paths.some((path) => !path)) return { st: 'linking' };
  const live = paths as Path[];
  if (live.some((path) => path.kind === 'relay')) return { st: 'limited' };
  const measured = live.find((path) => path.rtt !== undefined);
  return { st: 'ok', rtt: measured?.rtt };
}

/** The connection state to show in the indicator: for an open group chat, its home; otherwise the
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

/** Holds the latest `host.info` of each owned computer, refreshes it when an owner node connects
 *  or after `agent.set`, and sends `pair.browser`. */
export class Nodes {
  /** The latest `host.info` state for each owned `Home`. */
  readonly infos = new SvelteMap<Home, p.HostInfoPush>();
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

  async #read(home: Home, ctl: p.Ctl) {
    const answer = await ctl.request({ t: 'host.info' }).catch(lost);
    if (home.ctl !== ctl || answer.err) return;
    const { re: _re, ok: _ok, ...info } = answer;
    this.infos.set(home, info);
  }
}

/** The client's shared `Nodes` store. */
export const nodes = new Nodes();
