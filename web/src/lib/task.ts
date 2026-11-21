// Pure display derivations for TaskCard, reading only cached rows (contracts/rows.md section 4,
// contracts/task-machine.md section 1).
import type { Group } from './groups.svelte';
import type { Kind } from './mounts.svelte';
import { t } from './prefs.svelte';
import type { TaskRow } from './proto';

/** Highlight state of one segment in the six-step task bar. */
export type StepMark = 'done' | 'now' | 'wait' | 'fail' | undefined;

/** Visual model of the task status pill. */
export interface Badge {
  text: string;
  tone?: 'sky' | 'dawn' | 'ok' | 'danger';
  solid?: boolean;
  dot?: 'working' | 'paused';
}

const stepKeys = ['work', 'checks', 'pr', 'cloud', 'review', 'merge'] as const;
const stepPos: Record<string, number> = {
  branch: 0,
  local_check: 1,
  cloud_check: 3,
  review: 4,
  merge: 5,
};
const idleStates = new Set(['paused', 'cancelled']);

function cardWaitBadge(group: Group, kind: 'approval' | 'decision', who?: string[]): Badge {
  if (who?.includes(group.home.me.user)) {
    return { text: t(`task.wait.${kind}.you`), tone: 'dawn', solid: true };
  }
  const person = who?.map((u) => group.personName(u)).find(Boolean);
  const text = person
    ? t(`task.wait.${kind}.who`).replace('{who}', person)
    : t(`task.wait.${kind}.other`);
  return { text };
}

/** Words `task.wait` for the status badge and the checks link when an active wait is present. */
export function waitBadge(group: Group, wait: TaskRow['wait']): Badge | undefined {
  if (!wait) return undefined;
  if (wait.kind === 'approval' || wait.kind === 'decision') {
    return cardWaitBadge(group, wait.kind, wait.who);
  }
  if (wait.kind === 'reviewer') return { text: t('task.badge.reviewer') };
  return wait.kind === 'conflict' ? { text: t('task.badge.conflict'), tone: 'danger' } : undefined;
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
  if (task.state === 'merged') {
    const text = task.review
      ? t('task.badge.merged.score').replace('{n}', `${task.review.score}`)
      : t('task.badge.merged');
    return { text, tone: 'ok' };
  }
  if (task.state === 'cancelled') return { text: t('task.badge.cancelled') };
  return task.state === 'failed' ? failedBadge(task) : undefined;
}

function activeBadge(group: Group, task: TaskRow): Badge {
  const waiting = waitBadge(group, task.wait);
  if (waiting) return waiting;
  if (task.review?.ok === false) {
    return {
      text: t('task.badge.rejected').replace('{n}', `${task.review.round}`),
      tone: 'danger',
    };
  }
  return { text: t('task.badge.active'), tone: 'sky', dot: 'working' };
}

/** Selects the task's single status badge in the order of contracts/rows.md section 4. */
export function taskBadge(group: Group, task: TaskRow): Badge {
  const ended = finalBadge(task);
  if (ended) return ended;
  if (task.state === 'paused') return { text: t('task.badge.paused'), dot: 'paused' };
  return task.state === 'queued' ? { text: t('task.badge.queued') } : activeBadge(group, task);
}

const isRemote = (index: number) => index === 2 || index === 3;

function readyForPr(task: TaskRow, noForge: boolean): boolean {
  if (noForge || task.pr !== undefined || task.checks.local?.ok !== true) return false;
  return task.review?.ok !== false && task.wait?.kind !== 'conflict';
}

function stepIndex(task: TaskRow, noForge: boolean): number {
  return task.step === 'local_check' && readyForPr(task, noForge) ? 2 : (stepPos[task.step] ?? 0);
}

function activeStepFailed(task: TaskRow, at: number): boolean {
  if (task.wait?.kind === 'conflict') return true;
  if (at === 1) return task.checks.local?.ok === false;
  if (at === 3) return task.checks.cloud?.ok === false;
  return at === 4 && task.review?.ok === false;
}

function waitsOnViewer(group: Group, task: TaskRow): boolean {
  const kind = task.wait?.kind;
  return (
    (kind === 'approval' || kind === 'decision') &&
    Boolean(task.wait?.who?.includes(group.home.me.user))
  );
}

function currentMark(group: Group, task: TaskRow, at: number): StepMark {
  if (task.state === 'failed') return 'fail';
  if (idleStates.has(task.state)) return undefined;
  if (waitsOnViewer(group, task)) return 'wait';
  return activeStepFailed(task, at) ? 'fail' : 'now';
}

function remoteStepDone(task: TaskRow, index: number): boolean {
  if (index === 2) return task.pr !== undefined;
  return index === 3 ? task.checks.cloud !== undefined || task.pr !== undefined : true;
}

function pastStepDone(task: TaskRow, index: number, noForge: boolean): boolean {
  if (noForge) return !isRemote(index);
  return task.state === 'merged' || remoteStepDone(task, index);
}

function segmentMark(
  task: TaskRow,
  index: number,
  at: number,
  cur: StepMark,
  noForge: boolean,
): StepMark {
  if (task.state === 'merged' || index < at)
    return pastStepDone(task, index, noForge) ? 'done' : undefined;
  return index === at ? cur : undefined;
}

/** Computes the six-step bar segments, trailing step text, and accessible summary. */
export function taskSteps(group: Group, task: TaskRow, status: string) {
  const noForge = group.info?.forge === 'none';
  const at = stepIndex(task, noForge);
  const cur = currentMark(group, task, at);
  const queued = task.state === 'queued';
  const items = stepKeys.map((key, i) => ({
    key,
    label: t(`task.step.${key}`),
    mark: queued ? undefined : segmentMark(task, i, at, cur, noForge),
    skipped: noForge && isRemote(i),
  }));
  if (queued)
    return { items, summary: '0 / 6', aria: t('task.steps.queued').replace('{status}', status) };
  if (task.state === 'merged') {
    return { items, summary: '6 / 6', aria: t('task.steps.merged').replace('{status}', status) };
  }
  const step = items[at].label;
  const aria = t('task.steps.at')
    .replace('{n}', `${at + 1}`)
    .replace('{step}', step)
    .replace('{status}', status);
  return { items, summary: `${step} · ${at + 1} / 6`, aria };
}

/** Formats the review line of `<dl class="kv">` from `task.review`, `task.reviewer` and `task.pass`. */
export function reviewLine(group: Group, task: TaskRow) {
  const pass = `${task.pass}`;
  if (task.review) {
    const verdict = t(task.review.ok ? 'task.review.ok' : 'task.review.fail')
      .replace('{round}', `${task.review.round}`)
      .replace('{score}', `${task.review.score}`);
    const who = group.member(task.review.by)?.name;
    const meta = who
      ? t('task.review.by').replace('{who}', who).replace('{pass}', pass)
      : t('task.review.pass').replace('{pass}', pass);
    return { verdict, tone: (task.review.ok ? 'ok' : 'danger') as 'ok' | 'danger', meta };
  }
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
  const who =
    last.by === 'system' ? t('chat.system') : (group.member(last.by)?.name ?? t('chat.gone'));
  const said = last.text || t(`chat.kind.${last.kind as Kind}`);
  return t('task.thread.last').replace('{who}', who).replace('{text}', said);
}
