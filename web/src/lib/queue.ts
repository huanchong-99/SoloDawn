// A person's messages that wait for an AI, and letting one go first (planning/product-rules.md
// R-72, planning/contracts/commands.md 1.4 `jump`). No row names the items a home has queued, so
// what still waits is worked out from the rows.
import type { Group } from './groups.svelte';
import { aiMembers } from './members';
import type { AIMemberRow, MsgRow, TaskRow } from './proto';

/** An AI that a message waits for, and the task it is busy with. */
export interface Wait {
  ai: AIMemberRow;
  task: TaskRow;
}

/** The AIs a person's message goes to: those it mentions, else in the main line the secretary. */
function addressed(group: Group, msg: MsgRow) {
  const ais = aiMembers(group);
  const named = ais.filter((ai) => msg.mentions.includes(ai.id));
  return named.length || msg.thread !== null ? named : ais.filter((ai) => ai.secretary);
}

/** When the message was queued for the AI, in the group's sequence: when it was written, or when
 *  the AI's owner confirmed it; 0 while they have not (contracts/step-2/confirm.md). */
function queuedAt(group: Group, msg: MsgRow, ai: AIMemberRow) {
  const card = group.main.find(
    (each) => each.kind === 'confirm' && each.re === msg.id && each.body.ai === ai.id,
  );
  if (!card) return msg.pos;
  return card.state!.status === 'confirmed' ? card.seq : 0;
}

/** Whether an AI is at work, since before it was sent the message. Its row changes when it starts
 *  or ends a turn, so a row older than the message says that the AI has not taken it. A `jump`
 *  interrupts nothing in an AI that the node does not start (contracts/step-3/external.md). */
const busy = (group: Group, msg: MsgRow, ai: AIMemberRow) =>
  ai.state === 'working' && ai.cli !== 'external' && ai.seq < queuedAt(group, msg, ai);

/** Whether the message is held, whatever the AIs do: the group is paused, or the task in whose
 *  thread it stands is paused or failed (contracts/task-machine.md section 2). */
function held(group: Group, { thread }: MsgRow) {
  const state = group.task(thread ?? undefined)?.state;
  return group.info?.paused || state === 'paused' || state === 'failed';
}

/** The AIs busy with a task that the person's message waits for. A `jump` of the message goes
 *  first with all of them. */
export function waiting(group: Group, msg: MsgRow): Wait[] {
  if (msg.kind !== 'text' || msg.by !== group.me?.id || held(group, msg)) return [];
  return addressed(group, msg)
    .filter((ai) => busy(group, msg, ai))
    .flatMap((ai) => {
      const task = group.task(ai.task);
      return task ? [{ ai, task }] : [];
    });
}

/** Lets the message go first. */
export const goFirst = (group: Group, msg: MsgRow) =>
  group.home.command({ t: 'jump', g: group.g, msg: msg.id });
