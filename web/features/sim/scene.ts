// What every script of the demo world reads and writes: the members and tasks of a group, and the
// messages its people and AIs post. The scripts write rows as the home would after the commands
// of planning/contracts/commands.md; they skip the signing, which the browser never sees.
import type * as p from '../../src/lib/proto';
import { msg, type Fields, type Row, type Say } from './rows';
import type { World } from './world';

export type Task = p.TaskRow & Fields;
export type AI = p.AIMemberRow & Fields;
export type Msg = p.MsgRow & Fields;

export class Scene {
  constructor(readonly world: World) {}

  groupRow(g: string) {
    return this.world.find<p.GroupRow>(g, 'group', g);
  }

  ai(g: string, id: string | undefined) {
    const member = this.world.find<AI>(g, 'member', id);
    return member?.kind === 'ai' ? member : undefined;
  }

  ais(g: string) {
    return this.world.rows(g).filter((row) => row.r === 'member' && row.kind === 'ai') as AI[];
  }

  /** The AIs that do the work: all but the secretary. */
  working(g: string) {
    return this.ais(g).filter((each) => !each.secretary);
  }

  secretary(g: string) {
    return this.ai(g, this.groupRow(g)?.secretary);
  }

  task(g: string, id: string | undefined) {
    return this.world.find<Task>(g, 'task', id);
  }

  tasksOf(g: string) {
    return this.world.rows(g).filter((row) => row.r === 'task') as Task[];
  }

  message(g: string, id: string | undefined) {
    return this.world.find<Msg>(g, 'msg', id);
  }

  paused(g: string) {
    return Boolean(this.groupRow(g)?.paused);
  }

  /** Writes a new message; one in a task's thread also moves the task's thread summary. */
  post(kind: string, say: Say, body: Fields = {}, state: Fields | null = null) {
    const written = this.world.write(say.g, msg(kind, say, body, state)) as Msg;
    if (say.thread) this.#threadMoved(say.g, written);
    return written;
  }

  #threadMoved(g: string, written: Msg) {
    const held = this.task(g, written.thread!);
    if (!held) return;
    const last = { by: written.by, kind: written.kind, text: written.text, ts: written.ts };
    const thread = { ...held.thread, count: held.thread.count + 1, last };
    this.world.write(g, { ...held, thread } as Row);
  }

  /** Changes a task. */
  setTask(g: string, id: string, fields: Fields) {
    return this.world.change(g, 'task', id, fields) as Task | undefined;
  }

  /** Changes what an AI is doing; a row that would not change is not written again, so that a
   *  message sent to a busy AI is still seen to wait (web/src/lib/queue.ts). */
  setAI(g: string, id: string, fields: Fields) {
    const held = this.ai(g, id);
    if (!held || Object.entries(fields).every(([key, value]) => held[key] === value)) return;
    const since = fields.state && fields.state !== held.state ? Date.now() : held.since;
    this.world.write(g, { ...held, ...fields, since, last_seen: Date.now() } as Row);
  }

  /** The AI took a message: its row is written again, as its host reports a new turn. */
  touch(g: string, id: string) {
    const held = this.ai(g, id);
    if (held) this.world.write(g, { ...held, last_seen: Date.now() } as Row);
  }

  /** Changes a card's state. */
  decide(g: string, id: string, state: Fields) {
    return this.world.change(g, 'msg', id, { state });
  }
}
