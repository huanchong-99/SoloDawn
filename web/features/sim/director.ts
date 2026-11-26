// The hands of the demo world: what its computers do with each command beyond what a test home
// applies (features/home.ts), and what follows from it. A home command reaches the scripts of
// tasks, talk and people; a host command, the host. Commands a test home applies itself (posting
// text, deciding a card, creating, pausing and resuming a group) are followed up here after it.
import type * as p from '../../src/lib/proto';
import { Host } from './host';
import { ids } from './ids';
import type { SimNode } from './node';
import { Ops } from './ops';
import { People } from './people';
import { group, person, row, type Fields } from './rows';
import { Scene, type AI, type Msg } from './scene';
import { Talk } from './talk';
import { Tasks } from './tasks';
import type { Hands, World } from './world';

type Answer = object | undefined;

export class Director extends Scene implements Hands {
  readonly tasks: Tasks;
  readonly ops: Ops;
  readonly talk: Talk;
  readonly people: People;
  readonly host: Host;

  constructor(world: World) {
    super(world);
    this.tasks = new Tasks(world);
    this.ops = new Ops(world, this.tasks);
    this.talk = new Talk(world, this.tasks);
    this.people = new People(world);
    this.host = new Host(world, this.tasks, this.talk, this.people);
    world.hands = this;
  }

  /** What was going on when the page was loaded again goes on. */
  resume() {
    this.tasks.kickAll();
    for (const g of this.world.groups()) this.talk.drainAll(g);
  }

  command(node: SimNode, payload: Fields): Answer {
    if ('to' in payload) return this.host.command(node, payload);
    const g = payload.g as string;
    const homed = node.groups.has(g) || (payload.t === 'group' && payload.op === 'create');
    if (!homed) return { err: { code: 'not_found', msg: 'no such group' } };
    return this.#home(payload);
  }

  #home(payload: Fields): Answer {
    const g = payload.g as string;
    const commands: Record<string, () => Answer> = {
      task: () => this.ops.apply(payload),
      jump: () => this.talk.jump(payload),
      member: () => this.people.member(payload),
      invite: () => this.people.invite(payload),
      join: () => this.people.join(payload),
      note: () => this.#note(payload),
      forget: () => this.#forget(payload),
      push: () => ({ ok: true }),
      group: () => (payload.op === 'set' ? this.people.group(payload) : undefined),
      post: () => (payload.kind === 'confirm' ? this.people.askToWatch(g, payload) : undefined),
    };
    return commands[payload.t as string]?.();
  }

  #note(payload: Fields) {
    const g = payload.g as string;
    const fields = {
      id: payload.id,
      g,
      task: payload.task ?? null,
      type: payload.type,
      name: payload.name,
    };
    this.world.write(
      g,
      row('row-note', { ...fields, text: payload.text, by: ids.me, ts: Date.now() }),
    );
    return { ok: true };
  }

  #forget(payload: Fields) {
    const g = payload.g as string;
    this.world.home(g)!.remove(g, payload.r as string, payload.id as string);
    this.world.changed();
    return { ok: true };
  }

  /** After a command that the test home applied. */
  applied(node: SimNode, payload: Fields) {
    const g = payload.g as string;
    const after: Record<string, () => void> = {
      post: () => this.talk.heard(g, payload.n as string),
      act: () => this.#acted(g, payload),
      group: () => this.#group(node, payload),
    };
    after[payload.t as string]?.();
  }

  #group(node: SimNode, payload: Fields) {
    const g = payload.g as string;
    if (payload.op === 'create') return this.#created(node, payload);
    const paused = payload.op === 'pause';
    this.post(
      'system',
      { g, by: 'system' },
      { code: paused ? 'group_paused' : 'group_resumed', by: ids.me },
    );
    for (const ai of this.ais(g)) this.#hold(g, ai, paused);
    if (!paused) this.resume();
  }

  /** The hosts interrupt the turns of a paused group, and give them again when it resumes. */
  #hold(g: string, ai: AI, paused: boolean) {
    const active = this.task(g, ai.task)?.state === 'active';
    if (paused && ai.state === 'working') this.setAI(g, ai.id, { state: 'paused' });
    if (!paused && ai.state === 'paused' && active) this.setAI(g, ai.id, { state: 'working' });
  }

  /** `group create`: the test home wrote example rows; they become those of this person. */
  #created(node: SimNode, payload: Fields) {
    const g = payload.g as string;
    const kind = payload.kind === 'team' ? 'team' : 'project';
    this.world.write(
      g,
      group({
        g,
        name: payload.name as string,
        home: node.device,
        kind,
        repo: payload.repo as string,
        checks: payload.checks as string[],
      }),
    );
    this.world.write(g, person(g, ids.me, this.world.me, payload.member_name as string, 'owner'));
  }

  /** A card was decided: the task, the secretary or the people of the card act on it. */
  #acted(g: string, payload: Fields) {
    const card = this.message(g, payload.msg as string);
    if (!card) return;
    const by: Record<string, () => void> = {
      brief: () => this.#brief(g, card),
      approval: () => this.tasks.acted(g, card),
      decision: () => this.tasks.acted(g, card),
      confirm: () => this.people.decided(g, card),
    };
    by[card.kind]?.();
  }

  #brief(g: string, card: Msg) {
    const state = card.state as { status: string; note?: string };
    const secretary = card.by;
    if (state.status === 'confirmed') {
      this.post('text', {
        g,
        by: secretary,
        text: '收到，已经派下去了。进度会在各自的任务里更新。',
      });
      return this.tasks.create(g, card);
    }
    const body = card.body as { goal: string; requester: string };
    const request = { text: body.goal, requester: body.requester };
    this.world.after(3000, () => this.talk.brief(g, secretary, request, card, state.note ?? ''));
  }

  info(node: SimNode) {
    return this.host.info(node);
  }
  watchStart(node: SimNode, g: string, member: string) {
    return this.host.watchStart(node, g, member);
  }
  channel(node: SimNode, label: string) {
    return this.host.channel(node, label);
  }
  heard(node: SimNode, frame: p.Devlog | p.PTYResize) {
    this.host.heard(node, frame);
  }
}
