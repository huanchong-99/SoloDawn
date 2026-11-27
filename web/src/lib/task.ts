// Pure display derivations for TaskCard, reading only cached rows (contracts/rows.md section 4,
// contracts/task-machine.md section 1).
import type { Group } from './groups.svelte';
import type { Kind } from './mounts.svelte';
import { t } from './prefs.svelte';
import type { TaskRow } from './proto';

/** Highlight state of one segment in the three-step task bar. */
export type StepMark = 'done' | 'now' | 'wait' | 'fail' | undefined;

/** Visual model of the task status pill. */
export interface Badge {
  text: string;
  tone?: 'sky' | 'dawn' | 'ok' | 'danger';
  solid?: boolean;
  dot?: 'working' | 'paused' | 'takeover';
}

/** The three positions the card draws: work, review, done (contracts/task-machine.md section 1). */
const stepKeys = ['work', 'review', 'done'] as const;
const idleStates = new Set(['paused', 'cancelled']);

type CardWait = 'approval' | 'decision' | 'confirm';
const isCardWait = (kind: string): kind is CardWait =>
  ['approval', 'decision', 'confirm'].includes(kind);

function cardWaitBadge(group: Group, kind: CardWait, who?: string[]): Badge {
  if (who?.includes(group.home.me.user)) {
    return { text: t(`task.wait.${kind}.you`), tone: 'dawn', solid: true };
  }
  const person = who?.map((u) => group.personName(u)).find(Boolean);
  const text = person
    ? t(`task.wait.${kind}.who`).replace('{who}', person)
    : t(`task.wait.${kind}.other`);
  return { text };
}

/** Words `task.wait` for the status badge when an active wait is present. */
export function waitBadge(group: Group, wait: TaskRow['wait']): Badge | undefined {
  if (!wait) return undefined;
  if (isCardWait(wait.kind)) return cardWaitBadge(group, wait.kind, wait.who);
  return wait.kind === 'reviewer' ? { text: t('task.badge.reviewer') } : undefined;
}

function failedBadge(task: TaskRow): Badge {
  if (task.failures > 0) {
    const text = t('task.badge.failed.count').replace('{n}', `${task.failures}`);
    return { text, tone: 'danger', solid: true };
  }
  const reviews = task.review?.failed ?? 0;
  const text =
    reviews > 0
      ? t('task.badge.failed.reviews').replace('{n}', `${reviews}`)
      : t('task.badge.failed');
  return { text, tone: 'danger', solid: true };
}

function finalBadge(task: TaskRow): Badge | undefined {
  if (task.state === 'done') {
    const text = task.review
      ? t('task.badge.done.score').replace('{n}', `${task.review.score}`)
      : t('task.badge.done');
    return { text, tone: 'ok' };
  }
  if (task.state === 'cancelled') return { text: t('task.badge.cancelled') };
  return task.state === 'failed' ? failedBadge(task) : undefined;
}

function activeBadge(group: Group, task: TaskRow): Badge {
  const waiting = waitBadge(group, task.wait);
  if (waiting) return waiting;
  if (task.stuck?.ask) return { text: t('task.badge.stuck'), tone: 'dawn' };
  if (task.review?.ok === false) {
    return {
      text: t('task.badge.rejected').replace('{n}', `${task.review.round}`),
      tone: 'danger',
    };
  }
  return { text: t('task.badge.active'), tone: 'sky', dot: 'working' };
}

function takenOver(group: Group, task: TaskRow): boolean {
  const assignee = group.member(task.assignee);
  return assignee?.kind === 'ai' && assignee.state === 'takeover' && assignee.task === task.id;
}

/** Selects the task's single status badge in the order of contracts/rows.md section 4. */
export function taskBadge(group: Group, task: TaskRow): Badge {
  const ended = finalBadge(task);
  if (ended) return ended;
  if (takenOver(group, task)) return { text: t('task.badge.takeover'), dot: 'takeover' };
  if (task.state === 'paused') return { text: t('task.badge.paused'), dot: 'paused' };
  return task.state === 'queued' ? { text: t('task.badge.queued') } : activeBadge(group, task);
}

function waitsOnViewer(group: Group, task: TaskRow): boolean {
  const { wait } = task;
  return wait !== null && isCardWait(wait.kind) && Boolean(wait.who?.includes(group.home.me.user));
}

function currentMark(group: Group, task: TaskRow): StepMark {
  if (task.state === 'failed') return 'fail';
  if (idleStates.has(task.state)) return undefined;
  return waitsOnViewer(group, task) ? 'wait' : 'now';
}

/** Computes the three-step bar segments, trailing step text, and accessible summary. A done task
 *  has every segment done; a queued one, none. */
export function taskSteps(group: Group, task: TaskRow, status: string) {
  const at = task.state === 'done' ? stepKeys.length : Number(task.step === 'review');
  const cur = task.state === 'queued' ? undefined : currentMark(group, task);
  const items = stepKeys.map((key, i) => ({
    key,
    label: t(`task.step.${key}`),
    mark: (i < at ? 'done' : i === at && cur) || undefined,
  }));
  if (task.state === 'queued') {
    const aria = t('task.steps.queued').replace('{status}', status);
    return { items, summary: `${t('task.step.queued')} · 0 / 3`, aria };
  }
  if (task.state === 'done') {
    const aria = t('task.steps.done').replace('{status}', status);
    return { items, summary: `${items[2].label} · 3 / 3`, aria };
  }
  const step = items[at].label;
  const aria = t('task.steps.at')
    .replace('{n}', `${at + 1}`)
    .replace('{step}', step)
    .replace('{status}', status);
  return { items, summary: `${step} · ${at + 1} / 3`, aria };
}

/** Formats the review line of `<dl class="kv">` from `task.review`, `task.reviewer` and `task.pass`. */
export function reviewLine(
  group: Group,
  task: TaskRow,
): { meta: string; verdict?: string; tone?: 'ok' | 'danger' } {
  const pass = `${task.pass}`;
  return task.review ? reviewed(group, task.review, pass) : pending(group, task, pass);
}

function reviewed(group: Group, review: NonNullable<TaskRow['review']>, pass: string) {
  const verdict = t(review.ok ? 'task.review.ok' : 'task.review.fail')
    .replace('{round}', `${review.round}`)
    .replace('{score}', `${review.score}`);
  const who = group.member(review.by)?.name;
  const meta = who
    ? t('task.review.by').replace('{who}', who).replace('{pass}', pass)
    : t('task.review.pass').replace('{pass}', pass);
  return { verdict, tone: (review.ok ? 'ok' : 'danger') as 'ok' | 'danger', meta };
}

function pending(group: Group, task: TaskRow, pass: string) {
  const who = task.reviewer ? group.member(task.reviewer)?.name : undefined;
  const meta = who
    ? t('task.review.pending').replace('{who}', who).replace('{pass}', pass)
    : t('task.review.none').replace('{pass}', pass);
  return { meta };
}

/** Formats the collapsed thread preview line from `task.thread`. */
export function threadPreview(group: Group, task: TaskRow): string {
  const last = task.thread.last;
  if (!last) return task.thread.count === 0 ? t('task.thread.empty') : '';
  const who = last.by === 'system' ? t('chat.system') : group.nameOf(last.by);
  const said = last.text || t(`chat.kind.${last.kind as Kind}`);
  return t('task.thread.last').replace('{who}', who).replace('{text}', said);
}
