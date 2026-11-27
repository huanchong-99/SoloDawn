// What a person may do to a task, why it stands still, and who a message in its thread reaches
// (planning/contracts/task-machine.md sections 2 and 3, planning/contracts/commands.md 1.5).
import type { Group, Member } from './groups.svelte';
import { aiMembers, isAdmin } from './members';
import { t } from './prefs.svelte';
import type { AIMemberRow, TaskRow } from './proto';
import { hasSession } from './terminal.svelte';

/** What an admin does to a task (contracts/commands.md 1.5). */
export type TaskOp = 'pause' | 'resume' | 'assign' | 'cancel';

const opsOf: Record<string, TaskOp[]> = {
  queued: ['assign', 'cancel'],
  active: ['pause', 'assign', 'cancel'],
  paused: ['resume', 'assign', 'cancel'],
  failed: ['resume', 'assign', 'cancel'],
};

/** What the reader may do to the task as it is: an admin, rows 11 to 13 and 15 of
 *  contracts/task-machine.md section 3. */
export const taskOps = (group: Group, task: TaskRow): TaskOp[] =>
  isAdmin(group) ? (opsOf[task.state] ?? []) : [];

/** Who does the work and who reviews it. */
export type Roles = Partial<Pick<TaskRow, 'assignee' | 'reviewer'>>;

/** Sends one of them; `roles` is who an `assign` names. The task's row comes back from the home. */
export const sendTask = (group: Group, task: TaskRow, op: TaskOp, roles: Roles = {}) =>
  group.home.command({ t: 'task', g: group.g, op, id: task.id, ...roles });

/** The AIs that can be a task's assignee or reviewer: all but the secretary. */
export const workingAIs = (group: Group) =>
  aiMembers(group).filter((ai) => ai.id !== group.info?.secretary);

/** Why a task stands still and what moves it on, when no card says it. */
export interface Hold {
  kind: 'manual' | 'rounds' | 'preempted' | 'failed' | 'depends' | 'busy' | 'reviewer' | 'denied';
  text: string;
  tone?: 'dawn' | 'danger';
}

const num = (task: TaskRow) => `T-${task.num}`;
const nums = (tasks: TaskRow[]) => tasks.map(num).join(t('task.sep'));
const tasksOf = (group: Group) =>
  [...group.rows.values()].filter((row): row is TaskRow => row.r === 'task');

function dependsHold(group: Group, task: TaskRow): string | undefined {
  const open = task.depends.flatMap((id) => group.task(id) ?? []);
  const never = open.filter(({ state }) => state === 'cancelled');
  if (never.length) return t('task.hold.depends.never').replace('{tasks}', nums(never));
  const left = open.filter(({ state }) => state !== 'done');
  return left.length ? t('task.hold.depends').replace('{tasks}', nums(left)) : undefined;
}

/** The folder of a task: its assignee's computer and workspace (contracts/task-machine.md
 *  section 2). */
function folderOf(group: Group, task: TaskRow) {
  const ai = group.member(task.assignee) as AIMemberRow;
  return `${ai.host} ${ai.workspace}`;
}

/** The started tasks that keep a queued task's folder busy. */
function busyHold(group: Group, task: TaskRow): string | undefined {
  const folder = folderOf(group, task);
  const holders = tasksOf(group).filter(
    (other) => hasSession(other) && folderOf(group, other) === folder,
  );
  return holders.length ? t('task.hold.busy').replace('{tasks}', nums(holders)) : undefined;
}

const holdTones: Partial<Record<Hold['kind'], Hold['tone']>> = {
  rounds: 'dawn',
  failed: 'danger',
};
const waitHolds = ['manual', 'rounds', 'preempted', 'reviewer', 'depends', 'busy'] as const;

const holdKind = (task: TaskRow): Hold['kind'] | undefined =>
  task.state === 'failed' ? 'failed' : waitHolds.find((kind) => kind === task.wait?.kind);

/** The holds whose words need more than the task's rounds. A manual pause names who paused the
 *  task, also a member who has left since (contracts/rows.md section 4). */
const holdTexts: Partial<
  Record<Hold['kind'], (group: Group, task: TaskRow) => string | undefined>
> = {
  depends: dependsHold,
  busy: busyHold,
  manual: (group, task) =>
    t('task.hold.manual').replace('{who}', () => group.nameOf(task.wait!.by)),
  failed: (group) => t(isAdmin(group) ? 'task.hold.failed.admin' : 'task.hold.failed'),
};

function holdText(group: Group, task: TaskRow, kind: Hold['kind']): string | undefined {
  const words = holdTexts[kind];
  return words
    ? words(group, task)
    : t(`task.hold.${kind}`).replace('{n}', `${task.thread.rounds}`);
}

const ownerName = (group: Group, ai: AIMemberRow) => group.personName(ai.user) ?? t('chat.gone');

/** The AIs whose owner denied them a role in what the task came from. Such a card is newer than the
 *  task's message, so it is held wherever that message is drawn. */
function deniedAIs(group: Group, task: TaskRow) {
  const request = task.brief ?? task.id;
  return group.main.map(
    (msg) =>
      msg.kind === 'confirm' && msg.re === request && msg.state.status === 'denied' && msg.body.ai,
  );
}

/** An AI that gets no work in the task because its owner said no, which no wait tells. A role that
 *  the owner gave needs no word of theirs (contracts/step-2/confirm.md section 6). */
function deniedHold(group: Group, task: TaskRow): Hold | undefined {
  if (task.state !== 'active' || task.wait) return undefined;
  const denied = deniedAIs(group, task);
  const roles = [task.assignee, task.step === 'review' ? task.reviewer : undefined];
  const ai = workingAIs(group).find(
    ({ id }) => roles.includes(id) && task.asked?.includes(id) && denied.includes(id),
  );
  if (!ai) return undefined;
  const text = t('task.hold.denied')
    .replace('{ai}', () => ai.name)
    .replace('{who}', () => ownerName(group, ai));
  return { kind: 'denied', text };
}

/** The waits of contracts/rows.md section 4, `failed` among them, that no card carries. */
export function taskHold(group: Group, task: TaskRow): Hold | undefined {
  const kind = holdKind(task);
  if (!kind) return deniedHold(group, task);
  const text = holdText(group, task, kind);
  return text ? { kind, text, tone: holdTones[kind] } : undefined;
}

/** What cancelling the task does, in sentences. */
export function cancelNotes(group: Group, task: TaskRow): string[] {
  const waiting = tasksOf(group).filter(
    ({ state, depends }) => state === 'queued' && depends.includes(task.id),
  );
  return [
    t(task.state === 'queued' ? 'task.cancel.queued' : 'task.cancel.body'),
    ...(waiting.length ? [t('task.cancel.depends').replace('{tasks}', nums(waiting))] : []),
  ];
}

/** The roles of `next` that differ from the task's: what an `assign` names. */
export function changedRoles(task: TaskRow, next: Roles): Roles {
  const changed = (['assignee', 'reviewer'] as const).filter(
    (role) => next[role] && next[role] !== task[role],
  );
  return Object.fromEntries(changed.map((role) => [role, next[role]]));
}

/** What giving the task to other AIs does, in sentences: the assignee of a started task that stops,
 *  and the owners who are asked (contracts/step-2/confirm.md section 6). */
export function assignNotes(group: Group, task: TaskRow, roles: Roles): string[] {
  const old = group.member(task.assignee)?.name;
  const stops = roles.assignee && hasSession(task) && old;
  const asked = workingAIs(group).filter(
    (ai) => ai.user !== group.home.me.user && Object.values(roles).includes(ai.id),
  );
  return [
    ...(stops ? [t('task.assign.stops').replace('{ai}', () => stops)] : []),
    ...asked.map((ai) =>
      t('task.assign.asked')
        .replace('{ai}', () => ai.name)
        .replace('{who}', () => ownerName(group, ai)),
    ),
  ];
}

/** The reader's own AIs with a session on the task, whose terminal can open on it
 *  (flows/takeover.md A): its assignee, and its reviewer from the first review on. */
export function ownTerminals(group: Group, task: TaskRow): AIMemberRow[] {
  if (!hasSession(task)) return [];
  const reviewing = task.step === 'review' || task.review?.by === task.reviewer;
  const roles = [task.assignee, reviewing ? task.reviewer : undefined];
  return aiMembers(group).filter(
    (ai) => roles.includes(ai.id) && ai.user === group.home.me.user && ai.state !== 'takeover',
  );
}

/** The members a message in the task's thread can mention, its own AIs first. */
export function threadMembers(group: Group, task: TaskRow): Member[] {
  const own = [task.assignee, task.reviewer];
  const first = own.flatMap((id) => group.members.filter((member) => member.id === id));
  return [...first, ...group.members.filter((member) => !own.includes(member.id))];
}

/** What becomes of a person's message in the task's thread that mentions `mentions`, when it does
 *  not simply reach the AIs it names (contracts/task-machine.md row 22 and section 2). */
export function replyNote(group: Group, task: TaskRow, mentions: string[]): string | undefined {
  if (task.wait?.kind === 'rounds') return t('thread.reply.resumes');
  if (!mentions.some((id) => group.member(id)?.kind === 'ai')) return t('thread.reply.silent');
  if (task.state === 'paused' || task.state === 'failed')
    return t(`thread.reply.${task.state}`).replace('{task}', num(task));
  return undefined;
}
