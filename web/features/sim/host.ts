// The demo computers as hosts of AIs (contracts/commands.md section 2, datachannel.md section 7):
// `host.info`, the folders each CLI was used in, the account type of a CLI, taking every AI of a
// computer offline and back, adding an AI to a group with its folder and removing it, the pairing
// link of another browser, and the terminal of a takeover with its hand-back (flows/takeover.md),
// which Mina asks to watch.
import type * as p from '../../src/lib/proto';
import { newId } from '../../src/lib/groups.svelte';
import { newSecret, shown } from '../../src/lib/pairing';
import { folders, homeDirs, words } from './content';
import type { SimNode } from './node';
import { here, type People } from './people';
import { ai, msg, type Fields, type Row } from './rows';
import { Scene, type AI } from './scene';
import type { Talk } from './talk';
import type { Tasks } from './tasks';
import { takeoverTerminal, watchedTerminal } from './terminal';
import type { World } from './world';

type Answer = { ok: true; [field: string]: unknown } | { err: { code: string; msg: string } };

const ok: Answer = { ok: true };
const refuse = (code: string, msg = `refused: ${code}`): Answer => ({ err: { code, msg } });
const hour = 3_600_000;
const accounts = ['personal', 'team', 'api_key'];

export class Host extends Scene {
  constructor(
    world: World,
    readonly tasks: Tasks,
    readonly talk: Talk,
    readonly people: People,
  ) {
    super(world);
  }

  /** The AIs that run on a computer, in every group, with their group. */
  #members(node: SimNode) {
    return this.world
      .groups()
      .flatMap((g) => this.ais(g).map((each) => ({ g, ai: each })))
      .filter(({ ai }) => ai.host === node.device);
  }

  info(node: SimNode): p.HostInfoPush {
    const members = this.#members(node);
    const groups = (agent: string) =>
      members
        .filter(({ ai }) => ai.cli === agent)
        .map(({ g, ai }) => ({
          g,
          member: ai.id,
          workspace: ai.workspace,
          level: 3,
          state: ai.state,
          task: ai.task,
          queued: (this.world.memory.queue[ai.id] ?? []).length,
          watchers: this.world.memory.grants
            .filter((each) => each.g === g && each.member === ai.id)
            .map((each) => each.user),
        }));
    const homes = [...new Set([...node.groups.keys(), ...members.map(({ g }) => g)])].map((g) => ({
      g,
      home: this.world.home(g)!.device,
      name: this.world.home(g)!.groups.get(g)!.name,
    }));
    return {
      t: 'host.info',
      device: node.device,
      name: node.name,
      os: node.os,
      version: 'v1.0.0',
      exe: '/usr/local/bin/solodawn',
      open: members.filter(({ ai }) => ai.state === 'working').length,
      max_open: 4,
      offline: node.offline,
      update: null,
      homes,
      relay: { used: 3_250_000, limit: 52_428_800 },
      agents: node.agents.map((agent) => ({ ...agent, groups: groups(agent.agent) })),
      settings: {
        retention_days: 90,
        idle_minutes: 15,
        default_level: 3,
        all_level3: false,
        approvers: [],
      },
    };
  }

  /** `host.workspaces`: the folders the CLI was used in here, newest first, those of its AIs among
   *  them. None for an agent the node does not start; an agent it does not know is refused. */
  workspaces(node: SimNode, agent: string): Answer {
    if (agent === 'external') return { ok: true, workspaces: [] };
    if (!node.agents.some((each) => each.agent === agent)) return refuse('bad_request');
    const used = this.#members(node)
      .filter(({ ai }) => ai.cli === agent)
      .map(({ ai }) => ai.workspace);
    const paths = [...new Set([...used, ...(folders[node.device]?.[agent] ?? [])])];
    const workspaces = paths.map((path, i) => ({ path, used: Date.now() - (i * 26 + 1) * hour }));
    return { ok: true, workspaces };
  }

  /** A host command for this computer. */
  command(node: SimNode, payload: Fields): Answer {
    if (payload.to !== node.device) return refuse('not_for_me');
    const commands: Record<string, () => Answer> = {
      'agent.set': () => this.#account(node, payload),
      'host.offline': () => this.#offline(node, true),
      'host.online': () => this.#offline(node, false),
      'pair.browser': () => ({
        ok: true,
        link: here(`#pair=${shown(newSecret(crypto))}`),
        expires: Date.now() + 600_000,
      }),
      'host.join': () => this.#join(node, payload),
      'host.leave': () => this.#leave(payload),
      'watch.end': () => this.#unwatch(node, payload),
      'takeover.start': () => this.#takeover(node, payload),
      'takeover.end': () => this.#handback(payload),
    };
    return commands[payload.t as string]?.() ?? ok;
  }

  #account(node: SimNode, payload: Fields): Answer {
    const agent = node.agents.find((each) => each.agent === payload.agent);
    if (!agent) return refuse('conflict');
    if (!accounts.includes(payload.account as string)) return refuse('bad_request');
    Object.assign(agent, { account: payload.account, login_ok: true });
    this.world.changed();
    return ok;
  }

  /** R-64: every AI of the computer goes offline in every group, or comes back. */
  #offline(node: SimNode, offline: boolean): Answer {
    node.offline = offline;
    for (const { g, ai } of this.#members(node))
      this.setAI(g, ai.id, { state: offline ? 'offline' : this.#back(g, ai) });
    this.world.changed();
    if (!offline) this.#resume();
    node.broadcast(this.info(node));
    return ok;
  }

  #back(g: string, member: AI) {
    const t = this.task(g, member.task);
    return t?.state === 'active' ? 'working' : 'waiting';
  }

  #resume() {
    this.tasks.kickAll();
    for (const g of this.world.groups()) this.talk.drainAll(g);
  }

  /** The folder of `host.join` (contracts/commands.md section 2): an absolute path under the
   *  person's own folder of this computer, with no `/` at its end, and no AI's of another group. */
  #folder(node: SimNode, g: string, path: unknown): Answer | string {
    if (typeof path !== 'string' || !path.startsWith('/'))
      return refuse('bad_request', 'not an absolute path');
    const workspace = path.replace(/(.)\/+$/, '$1');
    if (!`${workspace}/`.startsWith(homeDirs[node.device])) return refuse('not_found', path);
    const other = this.#members(node).find(
      (each) => each.ai.workspace === workspace && each.g !== g,
    );
    if (!other) return workspace;
    const name = this.world.home(other.g)!.groups.get(other.g)!.name;
    return refuse('conflict', `the folder is the workspace of an AI of the group "${name}"`);
  }

  /** `host.join`: the computer adds its CLI to the group in the folder given, and it says hello a
   *  moment later. */
  #join(node: SimNode, payload: Fields): Answer {
    const g = payload.g as string;
    const workspace = this.#folder(node, g, payload.workspace);
    if (typeof workspace !== 'string') return workspace;
    const home = this.world.home(g);
    if (!home || home.down) return refuse('home_unreachable');
    const agent = node.agents.find((each) => each.agent === payload.agent);
    if (!agent) return refuse('conflict');
    if (this.world.rows(g).some((row) => row.r === 'member' && row.name === payload.name))
      return refuse('conflict');
    const id = newId();
    const secretary = Boolean(payload.secretary);
    const spec = {
      id,
      name: payload.name as string,
      cli: agent.agent,
      user: node.owner,
      host: node.device,
      hostName: node.name,
      workspace,
      secretary,
    };
    const profile = {
      account: (payload.account as string) ?? agent.account ?? 'personal',
      model: payload.model as string,
      effort: payload.effort as string,
      tier: payload.tier as string,
      skills: payload.skills as string,
    };
    if (secretary) this.people.group({ g, secretary: id });
    this.world.write(g, ai(g, { ...spec, ...profile }));
    if (secretary) this.world.change(g, 'group', g, { secretary: id });
    this.world.after(2500, () =>
      this.post('text', {
        g,
        by: id,
        text: `大家好，我是 ${spec.name}，跑在 ${node.name} 上。有事 @ 我。`,
      }),
    );
    return ok;
  }

  #leave(payload: Fields): Answer {
    const g = payload.g as string;
    const member =
      this.ai(g, payload.member as string) ??
      this.ais(g).find((each) => each.cli === payload.agent);
    return member ? this.people.removeAI(g, member) : refuse('not_found');
  }

  #unwatch(node: SimNode, payload: Fields): Answer {
    const { g, member, user } = payload;
    this.world.memory.grants = this.world.memory.grants.filter(
      (each) => !(each.g === g && each.member === member && each.user === user),
    );
    node.broadcast(this.info(node));
    return ok;
  }

  /** `takeover.start` (flows/takeover.md A and B): the AI's task session, or its group session
   *  when it has no unfinished task, in its folder. A takeover creates no task. */
  #takeover(node: SimNode, payload: Fields): Answer {
    const g = payload.g as string;
    const member = this.ai(g, payload.member as string);
    if (!member || member.host !== node.device) return refuse('not_found');
    if (member.state === 'takeover') return refuse('busy');
    const current = this.task(g, (payload.task as string) ?? member.task);
    const task = current && ['active', 'paused'].includes(current.state) ? current.id : undefined;
    this.world.memory.takeovers[`${g}:${member.id}`] = { task };
    this.setAI(g, member.id, { state: 'takeover', task });
    this.world.after(6000, () => this.#minaWatches(g, member.id));
    return ok;
  }

  #minaWatches(g: string, member: string) {
    const open = this.world.memory.takeovers[`${g}:${member}`];
    if (!open || this.ai(g, member)?.state !== 'takeover') return;
    this.people.minaAsks(g, member);
  }

  /** `takeover.end` (flows/takeover.md D): the AI posts its account of what was done, in the
   *  task's thread or the main line; the grants of the takeover end. */
  #handback(payload: Fields): Answer {
    const g = payload.g as string;
    const key = `${g}:${payload.member}`;
    const open = this.world.memory.takeovers[key];
    const member = this.ai(g, payload.member as string);
    if (!open || !member) return ok;
    delete this.world.memory.takeovers[key];
    this.world.memory.grants = this.world.memory.grants.filter(
      (each) => !(each.g === g && each.member === member.id),
    );
    this.#handbackPost(g, member, open.task);
    const t = this.task(g, open.task);
    this.setAI(g, member.id, {
      state: t?.state === 'active' ? 'working' : 'waiting',
      task: t?.state === 'active' ? t.id : undefined,
    });
    if (t) this.tasks.kick(g, t.id);
    this.talk.drainAll(g);
    return ok;
  }

  /** The hand-back message is the AI's own account: the node reads nothing from the folder. */
  #handbackPost(g: string, member: AI, id: string | undefined) {
    const say = { g, by: member.id, thread: id ?? null, text: words.handback };
    if (id) this.post('handback', say);
    else this.world.write(g, msg('handback', say) as Row);
  }

  /** A browser of the person opens a terminal channel on this computer. */
  channel(node: SimNode, label: string) {
    const [kind, g, member] = label.split(':');
    if (kind === 'watch') return this.#watch(g, member);
    const cli = this.ai(g, member)?.cli ?? 'claude';
    return this.world.memory.takeovers[`${g}:${member}`] ? takeoverTerminal(cli) : undefined;
  }

  #watch(g: string, member: string) {
    return this.#granted(g, member) ? watchedTerminal() : undefined;
  }

  #granted(g: string, member: string) {
    return this.world.memory.grants.some(
      (each) => each.g === g && each.member === member && each.user === this.world.me,
    );
  }

  watchStart(_node: SimNode, g: string, member: string) {
    return this.#granted(g, member) ? ok : refuse('forbidden');
  }

  /** The device log a browser presents: the computer is paired once the log names it. */
  heard(node: SimNode, frame: p.Devlog | p.PTYResize) {
    if (frame.t !== 'devlog' || node.paired) return;
    node.paired = frame.entries.some((env) => env.p.includes(node.device));
    this.world.changed();
  }
}
