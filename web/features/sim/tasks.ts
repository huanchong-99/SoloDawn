// The tasks of the demo world, moved through their steps as planning/contracts/task-machine.md
// says: work in the assignee's folder, a report, a review by the other AI, done, and the
// secretary's summary. One folder takes one task at a time: the next one waits, queued. Each task
// has a script: the first of a brief asks for an approval, fails a test once and is sent back by
// its first review, the second puts a decision to the person, any other just works.
import type * as p from '../../src/lib/proto';
import { words } from './content';
import { system, task as taskRow, type Fields } from './rows';
import { Scene, type Msg, type Task } from './scene';

const step = 3000;
const scripts = ['build', 'decide'];
/** The stages of the work step, by script, in order. Work sent back by a review is revised. */
const stages: Record<string, string[]> = {
  build: ['ask', 'coded', 'fail', 'fixed', 'report'],
  decide: ['coded', 'ask', 'report'],
  plain: ['coded', 'report'],
  again: ['revised', 'resubmit'],
};
const started = ['active', 'paused', 'failed'];

type Brief = Extract<p.MsgRow, { kind: 'brief' }>['body'];
const isCard = (kind: string) => ['approval', 'decision', 'confirm'].includes(kind);
const pending = (card: Msg) => (card.state as { status?: string } | null)?.status === 'pending';

export class Tasks extends Scene {
  #scheduled = new Set<string>();
  /** Answers the messages queued for an AI while it was busy, once its turn ends: when it waits
   *  for a card, or hands the task on. Set by the talk. */
  drain: (g: string, ai: string) => boolean = () => false;

  // The script of a task, and the stages it has passed, are kept with the world.
  flags(id: string) {
    return (this.world.memory.flags[id] ??= []);
  }

  #script(id: string) {
    return (
      this.flags(id)
        .find((flag) => flag.startsWith('script:'))
        ?.slice(7) ?? 'plain'
    );
  }

  #has(id: string, flag: string) {
    return this.flags(id).includes(flag);
  }

  #mark(id: string, flag: string) {
    this.flags(id).push(flag);
  }

  /** A new assignee starts the work over, but does not put a question again that a card of the task
   *  holds, answered or still waiting; an approval that was reassigned is asked again. */
  restart(g: string, id: string) {
    const asked = this.world
      .rows(g)
      .some(
        (row) =>
          ['approval', 'decision'].includes(row.kind as string) &&
          (row.body as { task?: string }).task === id &&
          (row.state as { status: string }).status !== 'reassigned',
      );
    this.world.memory.flags[id] = this.flags(id).filter(
      (flag) => flag.startsWith('script:') || (flag === 'ask' && asked),
    );
  }

  /** The folder of an AI: its computer and its workspace there. */
  #folder(g: string, ai: string) {
    const member = this.ai(g, ai);
    return member && `${member.host} ${member.workspace}`;
  }

  /** Whether another started task has the folder of `ai` (task-machine section 2). */
  busy(g: string, ai: string, self?: string) {
    const folder = this.#folder(g, ai);
    return this.tasksOf(g).some(
      (t) => t.id !== self && started.includes(t.state) && this.#folder(g, t.assignee) === folder,
    );
  }

  /** What a task that is not started waits for: a task it depends on, or its folder. */
  #waits(g: string, t: { id?: string; assignee: string; depends: string[] }) {
    if (t.depends.some((id) => this.task(g, id)?.state !== 'done')) return 'depends';
    return this.busy(g, t.assignee, t.id) ? 'busy' : undefined;
  }

  /** Task-machine row 1: one task per entry of a confirmed brief, judged in its order. */
  create(g: string, brief: Msg) {
    const body = brief.body as Brief;
    const made: string[] = [];
    body.tasks.forEach((entry, i) => made.push(this.#make(g, brief, entry, i, made)));
    for (const id of made) this.kick(g, id);
  }

  #make(g: string, brief: Msg, entry: Brief['tasks'][number], i: number, made: string[]) {
    const depends = entry.depends.map((index) => made[index]);
    const num = Math.max(0, ...this.tasksOf(g).map((each) => each.num)) + 1;
    const roles = { assignee: entry.assignee, reviewer: entry.reviewer, brief: brief.id, depends };
    const spec = { g, num, title: entry.title, ...roles };
    const row = this.world.write(g, taskRow(spec, this.#waits(g, roles)));
    const id = row.id as string;
    this.world.memory.flags[id] = [`script:${scripts[i] ?? 'plain'}`];
    this.post('task', { g, by: 'system', id }, { task: id });
    return id;
  }

  /** Starting the queued tasks (task-machine section 2): in the order of their numbers, each judged
   *  against the tasks as the ones before it left them. */
  start(g: string) {
    const queued = this.tasksOf(g)
      .filter((t) => t.state === 'queued')
      .sort((a, b) => a.num - b.num);
    for (const t of queued) {
      const waits = this.#waits(g, t);
      if (waits && waits !== t.wait?.kind)
        this.setTask(g, t.id, { wait: { kind: waits, since: Date.now() } });
      if (waits) continue;
      const stats = { ...t.stats, started: Date.now() };
      this.setTask(g, t.id, { state: 'active', wait: this.wait(g, t.id), stats });
      this.kick(g, t.id);
    }
  }

  /** Whether the task can move now: active, with nothing to wait for, in a group that is not
   *  paused, and its AI at work (not offline, not taken over, not waiting for its owner). */
  #runnable(g: string, t: Task | undefined): t is Task {
    const moving = t?.state === 'active' && !t.wait && !this.paused(g);
    return moving && this.#present(g, t) && !this.#asked(g, t);
  }

  /** Whether the AI the task's step needs is there to act: neither offline nor taken over. */
  #present(g: string, t: Task) {
    const actor = this.ai(g, t.step === 'review' ? t.reviewer : t.assignee);
    return actor !== undefined && !['offline', 'takeover'].includes(actor.state);
  }

  /** A card that asks the owner of the task's assignee whether it may take part. */
  #asked(g: string, t: Task) {
    return this.world
      .rows(g)
      .some(
        (row) =>
          row.kind === 'confirm' &&
          pending(row as Msg) &&
          (row.body as { ai: string }).ai === t.assignee &&
          row.re === (t.brief ?? t.id),
      );
  }

  /** Moves the task on after a while, once. */
  kick(g: string, id: string, ms = step) {
    if (this.#scheduled.has(id)) return;
    this.#scheduled.add(id);
    this.world.after(ms, () => {
      this.#scheduled.delete(id);
      this.advance(g, id);
    });
  }

  /** Every task that can move, after a reload or when what held it is gone. */
  kickAll(g?: string) {
    const groups = g ? [g] : this.world.groups();
    for (const each of groups)
      for (const t of this.tasksOf(each)) {
        this.#reviewer(each, t);
        if (this.#runnable(each, t)) this.kick(each, t.id);
        else this.#othersCard(each, t);
      }
  }

  /** A task at review without a reviewer gets one, as the secretary would give it. */
  #reviewer(g: string, t: Task) {
    if (t.state !== 'active' || t.step !== 'review' || t.reviewer) return;
    const reviewer = this.working(g).find((ai) => ai.id !== t.assignee)?.id;
    t.reviewer = this.setTask(g, t.id, { reviewer })!.reviewer;
  }

  /** A card of the task that waits for another person, as after a reload: she answers again. */
  #othersCard(g: string, t: Task) {
    const card = this.message(g, t.wait?.msg);
    const owner = this.ai(g, t.assignee);
    if (!card || !owner || owner.user === this.world.me || !pending(card)) return;
    const by = this.world.rows(g).find((row) => row.r === 'member' && row.user === owner.user);
    this.world.after(4000, () => this.#ownerDecides(g, card, by?.id as string));
  }

  /** One step of a task's script. */
  advance(g: string, id: string) {
    const t = this.task(g, id);
    if (!this.#runnable(g, t)) return;
    if (t.step === 'review') this.#review(g, t);
    else this.#work(g, t);
  }

  #say(g: string, t: Task, text: string, by = t.assignee) {
    this.post('progress', { g, by, thread: t.id, text });
  }

  /** Step `work`, in the assignee's folder: it starts, then takes the next stage of its script. */
  #work(g: string, t: Task) {
    if (!this.#has(t.id, 'started')) {
      this.#mark(t.id, 'started');
      this.setAI(g, t.assignee, { state: 'working', task: t.id });
      this.#say(g, t, words.start(t.title));
      return this.kick(g, t.id);
    }
    const script = t.review?.ok === false ? 'again' : this.#script(t.id);
    const stage = stages[script].find((each) => !this.#has(t.id, each)) ?? 'report';
    this.#mark(t.id, stage);
    const run: Record<string, () => void> = {
      ask: () => this.#ask(g, t),
      coded: () => this.#say(g, t, words.coded),
      fail: () => this.#say(g, t, words.testFailed),
      fixed: () => this.#say(g, t, words.fixed),
      revised: () => this.#say(g, t, words.revised),
      report: () => this.#report(g, t),
      resubmit: () => this.#report(g, t),
    };
    run[stage]();
    if (stage !== 'ask') this.kick(g, t.id);
  }

  #ask(g: string, t: Task) {
    const ai = this.ai(g, t.assignee)!;
    const me = this.world.rows(g).find((row) => row.r === 'member' && row.user === ai.user);
    const say = { g, by: t.assignee, thread: t.id, main: true };
    const card =
      this.#script(t.id) === 'build'
        ? this.post(
            'approval',
            { ...say, mentions: me ? [me.id as string] : [] },
            this.#approval(t, ai.user),
            { status: 'pending' },
          )
        : this.post('decision', say, this.#decision(t, ai.user), { status: 'pending' });
    const kind = card.kind;
    this.setTask(g, t.id, { wait: { kind, msg: card.id, who: [ai.user], since: card.ts } });
    if (ai.user !== this.world.me)
      this.world.after(4000, () => this.#ownerDecides(g, card, me?.id as string));
    this.drain(g, t.assignee);
  }

  /** Another person's AI asked its owner, who answers by herself: she approves, or takes the
   *  recommended option. */
  #ownerDecides(g: string, card: Msg, by: string) {
    if (!pending(this.message(g, card.id) ?? card)) return;
    const act = { p: '{}', k: by, s: 'demo' };
    const state =
      card.kind === 'approval'
        ? { status: 'approved', by, at: Date.now(), act }
        : { status: 'decided', by, at: Date.now(), act, choice: 'B' };
    this.acted(g, this.decide(g, card.id, state) as Msg);
  }

  #approval(t: Task, user: string) {
    const command = words.approvalCommand;
    return {
      task: t.id,
      ai: t.assignee,
      tool: 'Bash',
      command,
      why: words.approvalWhy,
      category: 'command',
      approvers: [user],
    };
  }

  #decision(t: Task, user: string) {
    const { question, context, options } = words;
    return { task: t.id, question, context, options, recommended: 'B', to: [user] };
  }

  /** Task-machine row 5: the assignee's report is the submission; its reviewer takes over. */
  #report(g: string, t: Task) {
    const report = this.post('report', { g, by: t.assignee, thread: t.id, text: words.report });
    const reviewer = t.reviewer ?? this.working(g).find((ai) => ai.id !== t.assignee)?.id;
    this.setTask(g, t.id, { step: 'review', report: report.id, failures: 0, reviewer });
    this.setAI(g, t.assignee, { state: 'waiting', task: undefined });
    this.drain(g, t.assignee);
    this.setAI(g, reviewer!, { state: 'working', task: t.id });
  }

  /** Task-machine row 7: the reviewer scores the report. The first review of the first task of a
   *  brief sends the work back; any other passes, and the task is done. */
  #review(g: string, t: Task) {
    const ok = this.#script(t.id) !== 'build' || t.review !== undefined;
    const review = this.#scored(g, t, ok);
    this.setAI(g, t.reviewer!, { state: 'waiting', task: undefined });
    this.drain(g, t.reviewer!);
    if (ok) return this.#done(g, { ...t, review });
    this.setTask(g, t.id, { step: 'work', review });
    this.setAI(g, t.assignee, { state: 'working', task: t.id });
    this.kick(g, t.id);
  }

  /** Posts the reviewer's review of the task's report, and answers the task's `review` field. */
  #scored(g: string, t: Task, ok: boolean) {
    const score = ok ? 92 : 78;
    const round = (t.review?.round ?? 0) + 1;
    const failed = (t.review?.failed ?? 0) + Number(!ok);
    const items = ok ? words.reviewItems : words.sentBackItems;
    const { num, title, pass } = t;
    const body = { task: t.id, num, title, report: t.report, round, score, pass, ok, items };
    const text = ok ? words.passed : words.sentBack;
    this.post('review', { g, by: t.reviewer!, thread: t.id, text }, body);
    return { round, failed, score, ok, by: t.reviewer!, report: t.report!, at: Date.now() };
  }

  /** A passed review ends the task: its cards expire, the tasks that waited for it or its folder
   *  start, and the secretary sums it up. */
  #done(g: string, t: Task) {
    this.post('system', { g, by: 'system', thread: t.id }, { code: 'task_done', task: t.id });
    const stats = { ...t.stats, ended: Date.now() };
    const done = this.setTask(g, t.id, { state: 'done', wait: null, review: t.review, stats })!;
    this.#expire(g, t.id);
    this.world.after(1500, () => this.#summary(g, done));
    this.start(g);
  }

  #summary(g: string, t: Task) {
    const secretary = this.secretary(g);
    if (!secretary) return;
    const minutes = Math.max(1, Math.round((t.stats.ended! - t.stats.started) / 60_000));
    const { score, round: rounds } = t.review!;
    const body = { task: t.id, score, rounds, minutes };
    this.post('summary', { g, by: secretary.id, thread: t.id, main: true }, body);
    this.#close(g, t, secretary.id);
    const left = this.tasksOf(g).filter((each) => each.brief === t.brief && each.state !== 'done');
    if (t.brief && !left.length) this.post('text', { g, by: secretary.id, text: words.allDone });
  }

  /** Flow task.md step 17: the secretary asks the assignee, in the task's thread, for the closing
   *  step the project needs, and the assignee answers there. */
  #close(g: string, t: Task, secretary: string) {
    const ask = { g, by: secretary, thread: t.id, mentions: [t.assignee] };
    this.post('text', { ...ask, text: words.closeAsk });
    this.world.after(2500, () =>
      this.post('text', { g, by: t.assignee, thread: t.id, text: words.closed }),
    );
  }

  /** The pending cards of a task that ended become `expired`. */
  #expire(g: string, id: string) {
    for (const card of this.#cards(g, id))
      this.decide(g, card.id, { status: 'expired', at: Date.now() });
  }

  #cards(g: string, id: string) {
    return this.world
      .rows(g)
      .filter((row) => row.r === 'msg' && isCard(row.kind as string) && pending(row as Msg))
      .filter((row) => (row.body as { task?: string }).task === id) as Msg[];
  }

  /** The wait of an active task, recomputed: its oldest pending approval, else its oldest pending
   *  decision, else none (task-machine section 2). */
  wait(g: string, id: string): Fields | null {
    const cards = this.#cards(g, id);
    const card =
      cards.find((each) => each.kind === 'approval') ??
      cards.find((each) => each.kind === 'decision');
    if (!card) return null;
    const who =
      (card.body as { approvers?: string[]; to?: string[] }).approvers ??
      (card.body as { to: string[] }).to;
    return { kind: card.kind, msg: card.id, who, since: card.ts };
  }

  /** Task-machine rows 18 and 20: a card of the task was decided; its AI says what it does now. */
  acted(g: string, card: Msg) {
    const id = (card.body as { task?: string }).task;
    const t = this.task(g, id);
    if (!t) return;
    this.#say(g, t, this.#answer(card), (card.body as { ai?: string }).ai ?? t.assignee);
    if (t.state !== 'active') return;
    this.setTask(g, t.id, { wait: this.wait(g, t.id) });
    this.kick(g, t.id);
  }

  #answer(card: Msg) {
    const state = card.state as { status: string; choice?: string; text?: string };
    if (card.kind === 'approval')
      return state.status === 'approved' ? words.approved : words.denied;
    const option = words.options.find((each) => each.id === state.choice);
    return option ? words.decided(option.label) : words.decidedText(state.text ?? '');
  }
}
