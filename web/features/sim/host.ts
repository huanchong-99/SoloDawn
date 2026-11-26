// The demo computers as hosts of AIs (contracts/commands.md section 2, datachannel.md section 7):
// `host.info`, the account type of a CLI, taking every AI of a computer offline and back, adding an
// AI to a group and removing it, the pairing link of another browser, and the terminal of a
// takeover with its hand-back (flows/takeover.md), which Mina asks to watch.
import type * as p from '../../src/lib/proto';
import { newId } from '../../src/lib/groups.svelte';
import { newSecret, shown } from '../../src/lib/pairing';
import { words } from './content';
import type { SimNode } from './node';
import { here, type People } from './people';
import { ai, msg, sha, task, type Fields, type Row } from './rows';
import { Scene, type AI } from './scene';
import type { Talk } from './talk';
import type { Tasks } from './tasks';
import { takeoverTerminal, watchedTerminal } from './terminal';
import type { World } from './world';

type Answer = { ok: true; [field: string]: unknown } | { err: { code: string; msg: string } };

const ok: Answer = { ok: true };
const refuse = (code: string): Answer => ({ err: { code, msg: `refused: ${code}` } });
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

  /** `host.join`: the computer adds its CLI to the group, which says hello a moment later. */
  #join(node: SimNode, payload: Fields): Answer {
    const g = payload.g as string;
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

  /** `takeover.start` (flows/takeover.md A and B): the AI's task session, the secretary's group
   *  session, or a new task of origin `takeover` for a working AI that has none. */
  #takeover(node: SimNode, payload: Fields): Answer {
    const g = payload.g as string;
    const member = this.ai(g, payload.member as string);
    if (!member || member.host !== node.device) return refuse('not_found');
    if (member.state === 'takeover') return refuse('busy');
    const current = this.task(g, (payload.task as string) ?? member.task);
    const session =
      current && ['active', 'paused'].includes(current.state) ? current.id : undefined;
    const made = !session && !member.secretary ? this.#ownTask(g, member) : undefined;
    const id = session ?? made;
    this.world.memory.takeovers[`${g}:${member.id}`] = {
      task: id,
      typed: false,
      made: Boolean(made),
    };
    this.setAI(g, member.id, { state: 'takeover', task: id });
    this.world.after(6000, () => this.#minaWatches(g, member.id));
    return ok;
  }

  #ownTask(g: string, member: AI) {
    const num = Math.max(0, ...this.tasksOf(g).map((each) => each.num)) + 1;
    const made = this.world.write(
      g,
      task({
        g,
        num,
        title: 'Owner takeover',
        slug: 'owner-takeover',
        assignee: member.id,
        origin: 'takeover',
      }),
    );
    this.post('task', { g, by: 'system', id: made.id as string }, { task: made.id });
    return made.id as string;
  }

  #minaWatches(g: string, member: string) {
    const open = this.world.memory.takeovers[`${g}:${member}`];
    if (!open || this.ai(g, member)?.state !== 'takeover') return;
    this.people.minaAsks(g, member);
  }

  /** `takeover.end` (flows/takeover.md D): the AI posts what was done, unless the takeover made
   *  its own task and nothing was typed, which cancels it; the grants of the takeover end. */
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
    if (open.made && !open.typed) this.#drop(g, open.task!);
    else this.#report(g, member, open.task);
    const t = this.task(g, open.task);
    this.setAI(g, member.id, {
      state: t?.state === 'active' ? 'working' : 'waiting',
      task: t?.state === 'active' ? t.id : undefined,
    });
    if (t) this.tasks.kick(g, t.id);
    this.talk.drainAll(g);
    return ok;
  }

  #drop(g: string, id: string) {
    const t = this.task(g, id);
    if (t)
      this.setTask(g, id, {
        state: 'cancelled',
        wait: null,
        stats: { ...t.stats, ended: Date.now() },
      });
  }

  #report(g: string, member: AI, id: string | undefined) {
    const body = {
      files: ['src/routes/Home.svelte'],
      commits: [{ sha: sha().slice(0, 7), subject: '主人在终端里调整首页文案' }],
      added: 6,
      removed: 2,
    };
    const say = { g, by: member.id, thread: id ?? null, text: words.handback };
    if (id) this.post('handback', say, body);
    else this.world.write(g, msg('handback', say, body) as Row);
  }

  /** A browser of the person opens a terminal channel on this computer. */
  channel(node: SimNode, label: string) {
    const [kind, g, member] = label.split(':');
    if (kind === 'watch') return this.#watch(g, member);
    const open = this.world.memory.takeovers[`${g}:${member}`];
    const cli = this.ai(g, member)?.cli ?? 'claude';
    if (!open) return undefined;
    return takeoverTerminal(cli, () => {
      open.typed = true;
      this.world.changed();
    });
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
