// A person's messages that wait for an AI, and letting one go first (planning/product-rules.md
// R-72, planning/contracts/commands.md 1.4 `jump`). No row names the items a home has queued, so
// what still waits is worked out from the rows.
import type { Group } from './groups.svelte';
import { aiMembers } from './members';
import type { AIMemberRow, MsgRow, TaskRow } from './proto';

/** A message that waits for an AI, with the task the AI is busy with when that is why. */
export interface Wait {
  msg: MsgRow;
  ai: AIMemberRow;
  task: TaskRow | undefined;
  /** When it was queued, in the group's sequence. */
  at: number;
}

/** The AIs a person's main-line message goes to: those it mentions, else the secretary. */
function addressed(group: Group, msg: MsgRow) {
  const ais = aiMembers(group);
  const named = msg.mentions.flatMap((id) => ais.filter((ai) => ai.id === id));
  return named.length ? named : ais.filter((ai) => ai.secretary);
}

/** When the message was queued for the AI: when it was written, or when the AI's owner confirmed
 *  it; undefined while they have not (contracts/step-2/confirm.md). */
function queuedAt(group: Group, msg: MsgRow, ai: AIMemberRow) {
  const card = group.main.find(
    (each) => each.kind === 'confirm' && each.re === msg.id && each.body.ai === ai.id,
  );
  if (!card) return msg.pos;
  return card.state!.status === 'confirmed' ? card.seq : undefined;
}

/** What an AI does that a message for it waits for. One that is waiting or asleep takes it. */
const holds = ['working', 'takeover', 'limited', 'offline', 'paused'];

/** Who a message still waits for. An AI's row changes when it starts or ends a turn, so a row
 *  older than the message says that the AI has not taken it. */
function waitsOf(group: Group, msg: MsgRow): Wait[] {
  return addressed(group, msg).flatMap((ai) => {
    const at = queuedAt(group, msg, ai);
    if (at === undefined || ai.seq >= at || !holds.includes(ai.state)) return [];
    return [{ msg, ai, at, task: ai.state === 'working' ? group.task(ai.task) : undefined }];
  });
}

const mine = (group: Group, msg: MsgRow) =>
  msg.kind === 'text' && msg.thread === null && msg.by === group.me?.id;

/** What the person's message waits for, when an AI is busy with a task, and whether it can be let
 *  go first. A `jump` moves the newest message of its author that waits, so only that one can; and
 *  it interrupts nothing in an AI that the node does not start (contracts/step-3/external.md). */
export function waiting(group: Group, msg: MsgRow): { wait: Wait; first: boolean }[] {
  if (!mine(group, msg) || group.info?.paused) return [];
  const busy = waitsOf(group, msg).filter(({ ai, task }) => task && ai.cli !== 'external');
  if (!busy.length) return [];
  const last = group.main
    .filter((each) => mine(group, each))
    .flatMap((each) => waitsOf(group, each))
    .reduce((newest, each) => (each.at >= newest.at ? each : newest));
  return busy.map((wait) => ({ wait, first: wait.msg === last.msg && wait.ai === last.ai }));
}

/** Lets the message go first. */
export const goFirst = (group: Group, msg: MsgRow) =>
  group.home.command({ t: 'jump', g: group.g, msg: msg.id });
