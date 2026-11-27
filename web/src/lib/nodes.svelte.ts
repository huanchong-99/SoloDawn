// Host information, AI account declaration and browser pairing per owned node
// (planning/contracts/datachannel.md section 7, planning/contracts/commands.md section 2), and
// the four states of the connection indicator (planning/flows/connect.md step 8 and section E).
import { SvelteMap } from 'svelte/reactivity';
import { homes, lost, newId, type Group, type Home } from './groups.svelte';
import { addHome } from './join';
import { aiMembers } from './members';
import type { Net, Path } from './net.svelte';
import { setAccount, signalUrlOf, type AccountKind } from './onboard.svelte';
import type * as p from './proto';

/** Visual states of the connection indicator. */
export type ConnKind = 'ok' | 'some' | 'limited' | 'linking' | 'down' | 'offline';

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

/** A folder of the owner's computer that a CLI was used in, and who has it as a workspace there:
 *  an AI of the group being added to, which can share it, or another group, whose folder it stays
 *  (planning/contracts/local.md section 7). */
export interface Folder {
  path: string;
  used: number;
  /** The names of the AIs of this group that have it: they share it. */
  ais?: string[];
  /** The name of the other group whose AI has it: it cannot be chosen. */
  other?: string;
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
 *  lock; for an open group chat, its home; otherwise the summary across `net.homes`, `some` while
 *  only part of them is connected, or `undefined` before any home is known. */
export function indicator(net: Net, group?: Group): ConnStatus | undefined {
  if (net.otherTab) return { st: 'offline' };
  const targets = group ? [group.home] : net.homes;
  if (targets.length === 0) return undefined;
  const online = targets.filter((home) => home.ctl);
  if (online.length === 0) return { st: 'down' };
  if (online.length < targets.length) return { st: 'some' };
  return summary(online.map((home) => net.path(home)));
}

function activeCtl(home: Home): p.Ctl | undefined {
  return home.owner ? home.ctl : undefined;
}

/** Whether a computer of the owner learns the home of `group` from this browser alone: another
 *  person's computer is known by `home.add` only (contracts/step-2/people.md section 7, point 2). */
const unheard = ({ home }: Group) => !home.owner;

/** The members that run `agent` on the computer `device` and have another account type in their
 *  group than `account`, which is declared for that CLI (R-50). */
export const outdated = (device: string, agent: string, account: string) =>
  homes
    .flatMap((home) => home.groups)
    .flatMap((group) => aiMembers(group).map((ai) => ({ group, ai })))
    .filter(({ ai }) => ai.host === device && ai.cli === agent && ai.account !== account);

/** Holds the latest `host.info` of each owned computer, refreshes it when an owner node connects
 *  or after a command that changes it, and sends `pair.browser`. */
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
    for (const [node] of this.infos) for (const group of groups) this.#tell(node, group);
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

  /** Gives the members that run `agent` on `device` its declared account type; the first refusal,
   *  when there is one (contracts/commands.md 1.2 `set`). */
  async correct(device: string, agent: string, account: string) {
    const answers = await Promise.all(
      outdated(device, agent, account).map(({ group, ai }) =>
        group.home.command({ t: 'member', g: group.g, op: 'set', id: ai.id, account }),
      ),
    );
    return answers.find((answer) => answer.err)?.err;
  }

  /** The folders that the CLI `agent` of the owned computer `home` was used in, newest first
   *  (`host.workspaces`), each with who has it by the computer's last `host.info`; `group` is the
   *  group the AI is being added to, none while it is being created. */
  async folders(home: Home, agent: string, group?: Group): Promise<Folder[] | p.ErrorResponse> {
    const ctl = home.ctl;
    const answer = await (ctl ? ctl.request({ t: 'host.workspaces', agent }).catch(lost) : lost());
    if (answer.err) return answer;
    return answer.workspaces.map((folder) => ({
      ...folder,
      ...this.holder(home, folder.path, group),
    }));
  }

  /** Who has `path` as a workspace on the owned computer `home`: another group, by its name, or
   *  the AIs of `group`, by their names. Nobody: an empty object. A folder is the workspace of AIs
   *  of one group only (contracts/local.md section 7). */
  holder(home: Home, path: string, group?: Group): Pick<Folder, 'ais' | 'other'> {
    const info = this.infos.get(home);
    const entries = (info?.agents ?? []).flatMap(({ groups }) => groups);
    const here = entries.filter((entry) => entry.workspace === path);
    const other = here.find((entry) => entry.g !== group?.g);
    if (other) return { other: info!.homes.find(({ g }) => g === other.g)?.name ?? other.g };
    return here.length ? { ais: here.map((entry) => group!.nameOf(entry.member)) } : {};
  }

  /** Asks an online owner node (`to`) for a one-time browser pairing link (`pair.browser`). */
  async pairBrowser(home: Home, to: string): Promise<PairLink | p.ErrorResponse> {
    const answer = await this.#command(home, to, 'pair.browser');
    if (answer.err) return answer;
    return { link: answer.link!, expires: answer.expires! };
  }

  /** Takes every AI of the owned node `home` offline, or back online (R-64), then reads its
   *  `host.info` again: the node pushes it only when the watchers change. */
  async setOffline(home: Home, offline: boolean): Promise<p.CmdResult | p.ErrorResponse> {
    const answer = await this.#command(home, home.device, `host.${offline ? 'offline' : 'online'}`);
    if (!answer.err) await this.#read(home, home.ctl!);
    return answer;
  }

  /** Sends a host command that carries nothing of its own (`HostCommandOnly`). */
  async #command(home: Home, to: string, t: p.HostCommandOnly['t']) {
    const env = await home.me.sign(JSON.stringify({ t, n: newId(), ts: Date.now(), to }));
    const ctl = home.ctl;
    return ctl ? ctl.request({ t: 'cmd', env }).catch(lost) : lost();
  }

  /** Asks the online owner node `home` for `host.info` with `refresh`: its models listed again. */
  refresh(home: Home) {
    return this.#read(home, home.ctl!, true);
  }

  async #read(home: Home, ctl: p.Ctl, refresh?: true) {
    const answer = await ctl.request({ t: 'host.info', ...(refresh && { refresh }) }).catch(lost);
    if (home.ctl !== ctl || answer.err) return;
    const { re: _re, ok: _ok, ...info } = answer;
    this.#learn(ctl, info);
    this.infos.set(home, info);
    home.onhomes?.(info.homes);
  }

  #learn(ctl: p.Ctl, { homes }: p.HostInfoPush) {
    const known = this.#known.get(ctl) ?? new Set();
    for (const { g } of homes) known.add(g);
    this.#known.set(ctl, known);
  }
}

/** The client's shared `Nodes` store. */
export const nodes = new Nodes();
