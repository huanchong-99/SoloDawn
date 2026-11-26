// What a person does to a task (contracts/commands.md 1.5; task-machine rows 11, 12, 13 and 15):
// pause, resume, cancel and give it to other AIs. The person's member id is the one they have in
// every group of the world. A role given to another person's AI waits for that person's word
// (contracts/step-2/confirm.md section 7), which Mina gives a moment later.
import { ids } from './ids';
import { msg, type Fields } from './rows';
import { Scene, type Task } from './scene';
import type { Tasks } from './tasks';
import type { World } from './world';

type Answer = { ok: true } | { err: { code: string; msg: string } };

const ok: Answer = { ok: true };
const refuse = (code: string): Answer => ({ err: { code, msg: `refused: ${code}` } });
const final = (t: Task) => ['done', 'cancelled'].includes(t.state);

export class Ops extends Scene {
  constructor(
    world: World,
    readonly tasks: Tasks,
  ) {
    super(world);
  }

  apply(payload: Fields): Answer {
    const g = payload.g as string;
    const t = this.task(g, payload.id as string);
    if (!t) return refuse('not_found');
    const ops: Record<string, () => Answer> = {
      pause: () => this.#pause(g, t),
      resume: () => this.#resume(g, t),
      cancel: () => this.#cancel(g, t),
      assign: () => this.#assign(g, t, payload),
    };
    return ops[payload.op as string]?.() ?? refuse('bad_request');
  }

  /** Row 12: the paused wait names who paused the task. */
  #pause(g: string, t: Task) {
    if (t.state !== 'active') return refuse('conflict');
    const kept = t.wait?.kind === 'reviewer' ? t.wait.kind : undefined;
    const wait = { kind: 'manual', since: Date.now(), kept, by: ids.me };
    this.setTask(g, t.id, { state: 'paused', wait });
    this.setAI(g, t.assignee, { state: 'paused' });
    return ok;
  }

  #resume(g: string, t: Task) {
    if (!['paused', 'failed'].includes(t.state)) return refuse('conflict');
    this.setTask(g, t.id, { state: 'active', wait: this.tasks.wait(g, t.id) });
    this.setAI(g, t.assignee, { state: 'working', task: t.id });
    this.tasks.kick(g, t.id, 1500);
    return ok;
  }

  #cancel(g: string, t: Task) {
    if (final(t)) return refuse('conflict');
    const ended = Date.now();
    this.setTask(g, t.id, { state: 'cancelled', wait: null, stats: { ...t.stats, ended } });
    for (const card of this.#pendingCards(g, t.id))
      this.decide(g, card.id as string, { status: 'expired', at: ended });
    for (const id of [t.assignee, t.reviewer])
      if (id && this.ai(g, id)?.task === t.id)
        this.setAI(g, id, { state: 'waiting', task: undefined });
    this.tasks.start(g);
    return ok;
  }

  #pendingCards(g: string, id: string) {
    return this.world
      .rows(g)
      .filter(
        (row) => row.r === 'msg' && (row.state as { status?: string } | null)?.status === 'pending',
      )
      .filter((row) => (row.body as { task?: string }).task === id);
  }

  /** Row 11. A new assignee stops the old one; its pending approvals become `reassigned`, by the
   *  person. A started task never goes back to the queue: an assignee whose folder is busy with
   *  another task is refused. */
  #assign(g: string, t: Task, payload: Fields) {
    if (final(t)) return refuse('conflict');
    const assignee = (payload.assignee as string | undefined) ?? t.assignee;
    const reviewer = (payload.reviewer as string | undefined) ?? t.reviewer;
    if (assignee === reviewer) return refuse('conflict');
    const moved = assignee !== t.assignee && t.state !== 'queued';
    if (moved && this.tasks.busy(g, assignee, t.id)) return refuse('conflict');
    if (assignee !== t.assignee) this.#handOver(g, t, assignee);
    const state = t.state === 'failed' ? 'active' : t.state;
    this.setTask(g, t.id, {
      assignee,
      reviewer,
      state,
      wait: state === 'active' ? this.tasks.wait(g, t.id) : t.wait,
    });
    for (const id of [assignee, reviewer]) this.#askOwner(g, t, id);
    this.tasks.start(g);
    this.tasks.kick(g, t.id, 1500);
    return ok;
  }

  #handOver(g: string, t: Task, assignee: string) {
    for (const card of this.#pendingCards(g, t.id))
      if (card.kind === 'approval')
        this.decide(g, card.id as string, { status: 'reassigned', by: ids.me, at: Date.now() });
    if (this.ai(g, t.assignee)?.task === t.id)
      this.setAI(g, t.assignee, { state: 'waiting', task: undefined });
    this.tasks
      .flags(t.id)
      .splice(0, Infinity, ...this.tasks.flags(t.id).filter((flag) => flag.startsWith('script:')));
    this.post('progress', {
      g,
      by: assignee,
      thread: t.id,
      text: '我接手了这个任务，先看一下前面做到哪了。',
    });
  }

  /** Another person's AI is asked for: a `direct` card for its owner, who says yes after a moment
   *  and gives up the terminal she had taken over. */
  #askOwner(g: string, t: Task, id: string | undefined) {
    const ai = this.ai(g, id);
    if (!ai || ai.user === this.world.me) return;
    const say = { g, by: 'system', main: true, re: t.brief ?? t.id };
    const card = this.world.write(
      g,
      msg('confirm', say, { what: 'direct', ai: ai.id, to: [ai.user] }, { status: 'pending' }),
    );
    const asked = [...(t.asked ?? []), ai.id];
    this.setTask(g, t.id, { asked });
    this.world.after(4000, () => {
      const by = this.world.rows(g).find((row) => row.r === 'member' && row.user === ai.user)?.id;
      this.decide(g, card.id as string, {
        status: 'confirmed',
        by,
        at: Date.now(),
        act: { p: '{}', k: ai.user, s: 'demo' },
      });
      this.setAI(g, ai.id, { state: 'waiting' });
      this.tasks.kick(g, t.id, 1500);
    });
  }
}
