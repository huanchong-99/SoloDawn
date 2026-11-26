// The tasks of the demo world, moved through their steps as planning/contracts/task-machine.md
// says: branch, local checks, pull request, hosted checks, review by the other AI, merge, and the
// secretary's summary. Each task has a script: the first of a brief asks for an approval and fails
// a test once, the second puts a decision to the person, any other just works.
import type * as p from '../../src/lib/proto';
import { words } from './content';
import { sha, system, task as taskRow, type Fields } from './rows';
import { Scene, type Msg, type Task } from './scene';

const step = 3000;
const scripts = ['build', 'decide'];
const slugs: Record<string, string> = { build: 'feature', decide: 'tests-docs' };
const stats: Record<string, { files: number; added: number; removed: number }> = {
  build: { files: 4, added: 86, removed: 14 },
  decide: { files: 3, added: 120, removed: 8 },
  plain: { files: 2, added: 40, removed: 6 },
};
/** The stages of the local step, by script, in order. */
const stages: Record<string, string[]> = {
  build: ['ask', 'coded', 'fail', 'fixed', 'pass'],
  decide: ['coded', 'ask', 'pass'],
  plain: ['coded', 'pass'],
};

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

  /** Task-machine row 1: one task per entry of a confirmed brief. */
  create(g: string, brief: Msg) {
    const body = brief.body as Brief;
    const made: string[] = [];
    body.tasks.forEach((entry, i) => made.push(this.#make(g, brief, entry, i, made)));
    for (const id of made) this.kick(g, id);
  }

  #make(g: string, brief: Msg, entry: Brief['tasks'][number], i: number, made: string[]) {
    const depends = entry.depends.map((index) => made[index]);
    const waits = depends.some((id) => this.task(g, id)?.state !== 'merged');
    const num = Math.max(0, ...this.tasksOf(g).map((each) => each.num)) + 1;
    const script = scripts[i] ?? 'plain';
    const spec = { g, num, title: entry.title, slug: `${slugs[script] ?? 'part'}-${num}` };
    const roles = { assignee: entry.assignee, reviewer: entry.reviewer, brief: brief.id, depends };
    const row = this.world.write(g, taskRow({ ...spec, ...roles }, waits ? 'queued' : 'active'));
    const id = row.id as string;
    this.world.memory.flags[id] = [`script:${script}`];
    this.post('task', { g, by: 'system', id }, { task: id });
    return id;
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
    const steps: Record<string, (t: Task) => void> = {
      branch: (t) => this.#branch(g, t),
      local_check: (t) => this.#local(g, t),
      cloud_check: (t) => this.#cloud(g, t),
      review: (t) => this.#review(g, t),
      merge: (t) => this.#merge(g, t),
    };
    steps[t.step](t);
  }

  #say(g: string, t: Task, text: string, by = t.assignee) {
    this.post('progress', { g, by, thread: t.id, text });
  }

  #branch(g: string, t: Task) {
    if (!this.#has(t.id, 'started')) {
      this.#mark(t.id, 'started');
      this.setAI(g, t.assignee, { state: 'working', task: t.id });
      this.#say(g, t, words.start(t.title));
    } else {
      this.setTask(g, t.id, { step: 'local_check', branch: `task/${t.num}-${t.slug}` });
    }
    this.kick(g, t.id);
  }

  #local(g: string, t: Task) {
    const stage = stages[this.#script(t.id)].find((each) => !this.#has(t.id, each)) ?? 'pass';
    this.#mark(t.id, stage);
    const run: Record<string, () => void> = {
      ask: () => this.#ask(g, t),
      coded: () => this.#say(g, t, words.coded),
      fail: () => this.#checks(g, t, false),
      fixed: () => this.#say(g, t, words.fixed),
      pass: () => this.#submit(g, t),
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

  #checks(g: string, t: Task, ok: boolean, stage = 'local') {
    const commit = (t.commit as string | undefined) ?? sha();
    const items = [
      { name: 'pnpm check', ok: true },
      ok
        ? { name: 'pnpm test', ok, detail: '51 passed' }
        : { name: 'pnpm test', ok, detail: '1 failed: pricing.test.ts › 手机宽度下按钮不换行' },
    ];
    this.post('checks', { g, by: 'system', thread: t.id }, { stage, ok, commit, items });
    const checks = {
      ...t.checks,
      [stage === 'cloud' ? 'cloud' : 'local']: { ok, at: Date.now(), commit },
    };
    if (!ok) this.#say(g, t, words.testFailed);
    return { commit, checks };
  }

  #submit(g: string, t: Task) {
    const { commit, checks } = this.#checks(g, t, true);
    const size = stats[this.#script(t.id)];
    const pr = 12 + t.num;
    this.post(
      'report',
      { g, by: t.assignee, thread: t.id, text: words.submitted },
      { commit, ...size, pr },
    );
    const pr_url = `https://github.com/example/solodawn-site/pull/${pr}`;
    const stats_ = { ...t.stats, added: size.added, removed: size.removed };
    this.setTask(g, t.id, { step: 'cloud_check', commit, checks, pr, pr_url, stats: stats_ });
  }

  #cloud(g: string, t: Task) {
    const { checks } = this.#checks(g, t, true, 'cloud');
    const reviewer = t.reviewer ?? this.working(g).find((ai) => ai.id !== t.assignee)?.id;
    this.setTask(g, t.id, { step: 'review', checks, failures: 0, reviewer });
    t = { ...t, reviewer };
    this.setAI(g, t.assignee, { state: 'waiting', task: undefined });
    this.drain(g, t.assignee);
    this.setAI(g, t.reviewer!, { state: 'working', task: t.id });
    this.kick(g, t.id, step + 1000);
  }

  #review(g: string, t: Task) {
    const score = 92;
    const round = (t.review?.round ?? 0) + 1;
    const body = {
      task: t.id,
      num: t.num,
      title: t.title,
      commit: t.commit,
      round,
      score,
      pass: t.pass,
      ok: true,
      items: words.reviewItems,
    };
    this.post('review', { g, by: t.reviewer!, thread: t.id, text: '看过了，可以合并。' }, body);
    const review = {
      round,
      failed: t.review?.failed ?? 0,
      score,
      ok: true,
      by: t.reviewer!,
      commit: t.commit!,
      at: Date.now(),
    };
    this.setTask(g, t.id, { step: 'merge', review });
    this.setAI(g, t.reviewer!, { state: 'waiting', task: undefined });
    this.drain(g, t.reviewer!);
    this.kick(g, t.id);
  }

  #merge(g: string, t: Task) {
    const ended = Date.now();
    this.post(
      'system',
      { g, by: 'system', thread: t.id },
      { code: 'task_merged', task: t.id, commit: t.commit, merge_commit: sha() },
    );
    const merged = this.setTask(g, t.id, {
      state: 'merged',
      wait: null,
      stats: { ...t.stats, ended },
    })!;
    this.#expire(g, t.id);
    this.setAI(g, t.assignee, { state: 'waiting', task: undefined });
    this.drain(g, t.assignee);
    this.world.after(1500, () => this.#summary(g, merged));
    this.#unblock(g);
  }

  #summary(g: string, t: Task) {
    const secretary = this.secretary(g);
    if (!secretary) return;
    const minutes = Math.max(1, Math.round((t.stats.ended! - t.stats.started) / 60_000));
    const { added, removed } = t.stats;
    const { score, round: rounds } = t.review!;
    const body = { task: t.id, score, rounds, minutes, added, removed, pr: t.pr };
    this.post('summary', { g, by: secretary.id, thread: t.id, main: true }, body);
    const left = this.tasksOf(g).filter(
      (each) => each.brief === t.brief && each.state !== 'merged',
    );
    if (t.brief && !left.length) this.post('text', { g, by: secretary.id, text: words.allMerged });
  }

  /** Task-machine row 8: a queued task whose dependencies all merged becomes active. */
  #unblock(g: string) {
    for (const t of this.tasksOf(g)) {
      if (t.state !== 'queued') continue;
      if (t.depends.every((id) => this.task(g, id)?.state === 'merged')) {
        this.setTask(g, t.id, { state: 'active', wait: null });
        this.kick(g, t.id);
      }
    }
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
