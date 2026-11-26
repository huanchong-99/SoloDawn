// How the AIs of the demo world answer what people write (contracts/commands.md 1.3, 1.4;
// flows/agent-turn.md C). A message goes to the AIs it mentions, and in the main line to the
// secretary when it mentions none. The secretary turns a request into a brief, answers a question,
// and writes a new brief when one is sent back. An AI that is busy with a task answers at its next
// pause, unless the person lets the message go first (`jump`), which pauses the task meanwhile.
import { titleOf, words } from './content';
import type { Fields } from './rows';
import { Scene, type AI, type Msg } from './scene';
import type { Tasks } from './tasks';
import type { World } from './world';

const reply = 2500;
const isQuestion = (text: string) =>
  /[?？]\s*$/.test(text.trim()) ||
  /^(怎么|为什么|是否|能不能|有没有)/.test(text.replace(/@\S+\s*/g, ''));

export class Talk extends Scene {
  #turn = 0;

  constructor(
    world: World,
    readonly tasks: Tasks,
  ) {
    super(world);
    tasks.drain = (g, ai) => this.drain(g, ai);
  }

  /** A person posted a text message. */
  heard(g: string, id: string) {
    const said = this.message(g, id);
    if (!said || said.kind !== 'text') return;
    if (said.thread) this.#thread(g, said);
    for (const ai of this.#addressed(g, said)) this.#deliver(g, ai, said);
  }

  /** Task-machine row 22: a person's message in a thread starts the rounds again. */
  #thread(g: string, said: Msg) {
    const t = this.task(g, said.thread!);
    if (t) this.setTask(g, t.id, { thread: { ...t.thread, rounds: 0 } });
  }

  #addressed(g: string, said: Msg): AI[] {
    const named = this.ais(g).filter((ai) => said.mentions.includes(ai.id));
    if (named.length || said.thread) return named;
    const secretary = this.secretary(g);
    return secretary ? [secretary] : [];
  }

  #deliver(g: string, ai: AI, said: Msg) {
    if (ai.user !== this.world.me && ai.account === 'personal') return this.#askOwner(g, ai, said);
    if (this.#busy(g, ai)) {
      (this.world.memory.queue[ai.id] ??= []).push({ g, msg: said.id });
      return;
    }
    this.world.after(reply, () => this.answer(g, ai.id, said.id));
  }

  /** A personal AI of another person acts only on its owner's word (contracts/step-2/confirm.md
   *  section 2): a `direct` card for her, which Mina confirms a moment later, giving up the
   *  terminal she had taken over. */
  #askOwner(g: string, ai: AI, said: Msg) {
    const say = { g, by: 'system', main: true, re: said.id };
    const card = this.post(
      'confirm',
      say,
      { what: 'direct', ai: ai.id, to: [ai.user] },
      { status: 'pending' },
    );
    this.world.after(4000, () => {
      const by = this.world.rows(g).find((row) => row.r === 'member' && row.user === ai.user)?.id;
      const act = { p: '{}', k: ai.user, s: 'demo' };
      this.decide(g, card.id, { status: 'confirmed', by, at: Date.now(), act });
      this.setAI(g, ai.id, { state: 'waiting' });
      this.world.after(reply, () => this.answer(g, ai.id, said.id));
    });
  }

  /** Whether the AI will not take a message now: the group is paused, it is busy with a task, it
   *  is offline or taken over. */
  #busy(g: string, ai: AI) {
    return this.paused(g) || ['working', 'offline', 'takeover'].includes(ai.state);
  }

  /** The AI answers a message, unless what it would answer is no longer there. */
  answer(g: string, id: string, msgId: string, lead = '') {
    const ai = this.ai(g, id);
    const said = this.message(g, msgId);
    if (!ai || !said) return;
    this.touch(g, id);
    if (ai.secretary && !said.thread) return this.#secretary(g, ai, said);
    const text = lead + words.mention[this.#turn++ % words.mention.length];
    const author = said.by;
    this.post('text', { g, by: id, thread: said.thread, text, mentions: [author], re: null });
  }

  #secretary(g: string, ai: AI, said: Msg) {
    if (isQuestion(said.text)) return this.post('text', { g, by: ai.id, text: words.answer });
    if (this.#pair(g).length < 2) return this.post('text', { g, by: ai.id, text: words.oneAI });
    this.brief(g, ai.id, { text: said.text, requester: said.by });
  }

  /** The two AIs a brief gives the work to: the person's own first. */
  #pair(g: string) {
    const working = this.working(g).filter((ai) => ai.state !== 'offline');
    const own = working.filter((ai) => ai.user === this.world.me);
    return [...own, ...working.filter((ai) => !own.includes(ai))].slice(0, 2);
  }

  /** What a brief says: a title and goal from the request, the person's note when the brief is
   *  written again, and two tasks for the two AIs, each reviewed by the other. */
  #body(g: string, text: string, note: string) {
    const [first, second] = this.#pair(g);
    const entry = (title: string, assignee: string, reviewer: string) => ({
      title,
      assignee,
      reviewer,
      foundation: false,
      depends: [],
    });
    const tasks = [
      entry(`实现：${titleOf(text)}`, first.id, second.id),
      entry(words.testsTitle, second.id, first.id),
    ];
    const again = note
      ? { title: `${titleOf(text)}（已按意见修改）`, criteria: [...words.criteria, note] }
      : {};
    const goal = text.replace(/@\S+\s*/g, '');
    const scope = { in: words.scopeIn, out: words.scopeOut };
    return {
      title: titleOf(text),
      goal,
      criteria: words.criteria,
      scope,
      tasks,
      pass: 90,
      ...again,
    };
  }

  /** The secretary posts a brief for a request; one written again replaces the brief sent back. */
  brief(
    g: string,
    secretary: string,
    request: { text: string; requester: string },
    replaces?: Msg,
    note = '',
  ) {
    const body = {
      ...this.#body(g, request.text, note),
      requester: request.requester,
      replaces: replaces?.id ?? null,
    };
    if (replaces)
      this.decide(g, replaces.id, { ...(replaces.state as Fields), status: 'superseded' });
    const text = replaces ? words.briefAgain : words.briefLead;
    this.post('brief', { g, by: secretary, text }, body, { status: 'pending' });
  }

  /** Answers the messages queued for an AI while it was busy; whether there were any. */
  drain(g: string, id: string) {
    const queued = this.world.memory.queue[id] ?? [];
    const mine = queued.filter((each) => each.g === g);
    if (!mine.length) return false;
    this.world.memory.queue[id] = queued.filter((each) => each.g !== g);
    for (const each of mine)
      this.world.after(2000, () => this.answer(g, id, each.msg, words.busyAnswer));
    return true;
  }

  /** Answers what waits for any AI of a group that can answer now: after a resume, after the
   *  computer is back online, after a hand-back. */
  drainAll(g: string) {
    for (const ai of this.ais(g)) {
      const t = this.task(g, ai.task);
      if (
        !this.paused(g) &&
        !['offline', 'takeover'].includes(ai.state) &&
        (!t || t.state !== 'active')
      )
        this.drain(g, ai.id);
    }
  }

  /** `jump` (contracts/commands.md 1.4): the AIs the message waits for answer it now. One that
   *  works on a task pauses it meanwhile (task-machine rows 16 and 13b). */
  jump(payload: Fields) {
    const g = payload.g as string;
    const waiting = Object.entries(this.world.memory.queue).filter(([, list]) =>
      list.some((each) => each.msg === payload.msg),
    );
    if (!waiting.length) return { err: { code: 'conflict', msg: 'nothing waits' } };
    for (const [ai] of waiting) this.#first(g, ai, payload.msg as string);
    return { ok: true };
  }

  #first(g: string, id: string, msgId: string) {
    this.world.memory.queue[id] = this.world.memory.queue[id].filter((each) => each.msg !== msgId);
    const t = this.#preempt(g, id, msgId);
    this.world.after(reply, () => {
      this.answer(g, id, msgId, words.preemptedAnswer);
      if (t) this.world.after(reply, () => this.#back(g, t.id));
    });
  }

  /** Task-machine row 16: the task the AI works on pauses for the message, unless it merges. */
  #preempt(g: string, id: string, msgId: string) {
    const t = this.task(g, this.ai(g, id)?.task);
    if (t?.assignee !== id || t.state !== 'active' || t.step === 'merge') return undefined;
    this.setTask(g, t.id, { state: 'paused', wait: { kind: 'preempted', since: Date.now() } });
    const by = this.message(g, msgId)!.by;
    this.post('system', { g, by: 'system', thread: t.id }, { code: 'preempted', task: t.id, by });
    return t;
  }

  /** Task-machine row 13b: the host resumes the task it pre-empted. */
  #back(g: string, id: string) {
    const t = this.task(g, id);
    if (t?.state !== 'paused' || t.wait?.kind !== 'preempted') return;
    this.setTask(g, id, { state: 'active', wait: this.tasks.wait(g, id) });
    this.tasks.kick(g, id, 1500);
  }
}
