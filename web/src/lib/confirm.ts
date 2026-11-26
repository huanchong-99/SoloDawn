// What a confirm card asks of the owner of an AI, in one sentence
// (planning/contracts/step-2/confirm.md sections 1 and 3).
import type { Group } from './groups.svelte';
import { prefs, t } from './prefs.svelte';
import { taskTitle } from './task';
import type { MsgRow } from './proto';
import { clock } from './when';

export type Confirm = Extract<MsgRow, { kind: 'confirm' }>;

/** A request to watch the terminal of an AI. */
export const isWatch = (msg: MsgRow): msg is Confirm =>
  msg.kind === 'confirm' && msg.body.what === 'watch';

const sentences = {
  direct: 'confirm.request.direct',
  ask: 'confirm.request.ask',
  watch: 'confirm.request.watch',
} as const;

const nameOf = (group: Group, id: string | undefined) => group.nameOf(id);

/** Who asked: the author of a `watch` card, or else of the request. A brief is written by the
 *  secretary; the word behind its roles is the person who confirmed it. */
function requester(group: Group, msg: Confirm): string {
  if (msg.body.what === 'watch') return nameOf(group, msg.by);
  const request = group.rows.get(`msg ${msg.re}`) as MsgRow | undefined;
  if (!request || 'gone' in request) return t('confirm.requester_unknown');
  if (request.kind === 'brief' && request.state.status === 'confirmed')
    return nameOf(group, request.state.by);
  return nameOf(group, request.by);
}

/** "Let Codex act on the request from Pat." The request of a role in a task without a brief is the
 *  `task` message, which has the task's id. No row says who started such a task, so the sentence
 *  names the task. */
export function confirmSentence(group: Group, msg: Confirm): string {
  const ai = () => nameOf(group, msg.body.ai);
  const task = group.task(msg.re ?? undefined);
  if (task)
    return t('confirm.request.task')
      .replace('{ai}', ai)
      .replace('{task}', () => `T-${task.num}`)
      .replace('{title}', () => taskTitle(task));
  return t(sentences[msg.body.what as keyof typeof sentences])
    .replace('{ai}', ai)
    .replace('{who}', () => requester(group, msg));
}

/** Whom a pending card waits for, or how it ended: "Confirmed by Pat · 09:58". */
export function confirmStatus(group: Group, { body, state }: Confirm): string {
  if (state.status === 'pending') {
    const people = body.to.map((user) => group.personName(user) ?? t('chat.gone'));
    return t('confirm.waiting').replace('{who}', () => people.join(t('confirm.sep')));
  }
  const label = t(`confirm.${state.status}`).replace('{time}', clock(state.at, prefs.lang));
  // Only the label of a card that was decided has a {who}; an expired card has no `by`.
  const { by } = state as { by?: string };
  return label.replace('{who}', () => nameOf(group, by));
}
