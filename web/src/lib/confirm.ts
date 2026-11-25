// What a confirm card asks of the owner of an AI, in one sentence
// (planning/contracts/step-2/confirm.md sections 1 and 3).
import type { Group } from './groups.svelte';
import { t } from './prefs.svelte';
import type { MsgRow } from './proto';

type Confirm = Extract<MsgRow, { kind: 'confirm' }>;

const sentences = {
  direct: 'confirm.request.direct',
  ask: 'confirm.request.ask',
  watch: 'confirm.request.watch',
} as const;

const nameOf = (group: Group, id: string) => group.member(id)?.name ?? t('chat.gone');

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
      .replace('{title}', () => task.title);
  return t(sentences[msg.body.what as keyof typeof sentences])
    .replace('{ai}', ai)
    .replace('{who}', () => requester(group, msg));
}
