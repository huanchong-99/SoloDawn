// What a notice of the home says (planning/contracts/messages.md 2.11), worded by its code: as a
// row of the chat, and in a notification.
import type { Group } from './groups.svelte';
import { prefs, t } from './prefs.svelte';
import type { MsgRow } from './proto';
import { clockOn } from './when';

type SystemMsg = Extract<MsgRow, { kind: 'system' }>;
type Body = SystemMsg['body'];
type Wording<B> = (group: Group, body: B, ts: number) => string;

const who = (group: Group, id: string) => group.member(id)?.name ?? t('chat.gone');
const join = (...items: (string | undefined)[]) => items.filter(Boolean).join(' · ');

/** A task's number; of one that is not held, the start of its id. */
export function taskTag(group: Group, id: string) {
  const row = group.task(id);
  return row ? `T-${row.num}` : id.slice(0, 7);
}

const turnKey = (reason: string) => (reason === 'crash' || reason === 'timeout' ? reason : 'stop');

function removed(group: Group, name: string, by: string | undefined) {
  if (by === undefined) return t('thread.sys.member_left').replace('{who}', () => name);
  return t('thread.sys.member_removed')
    .replace('{by}', () => who(group, by))
    .replace('{who}', () => name);
}

// `task_merged` is drawn by its row, and `cap_reached` is postponed: both are named by their code.
const words: { [B in Body as B['code']]?: Wording<B> } = {
  task_failed: (group, { reason, task }) =>
    t(`thread.sys.task_failed.${reason === 'review_rounds' ? reason : 'failures'}`).replace(
      '{task}',
      taskTag(group, task),
    ),
  turn_interrupted: (group, { reason, ai, task, detail }) =>
    join(
      t(`thread.sys.turn_interrupted.${turnKey(reason)}`).replace('{ai}', () => who(group, ai)),
      task && taskTag(group, task),
      detail,
    ),
  login_invalid: (group, { ai, detail }) =>
    join(
      t('thread.sys.login_invalid').replace('{ai}', () => who(group, ai)),
      detail,
    ),
  session_replaced: (group, { ai, task }) =>
    join(
      t('thread.sys.session_replaced').replace('{ai}', () => who(group, ai)),
      task && taskTag(group, task),
    ),
  rounds_limit: (group, { task, n }) =>
    t('thread.sys.rounds_limit').replace('{task}', taskTag(group, task)).replace('{n}', String(n)),
  preempted: (group, { task, by }) =>
    t('thread.sys.preempted')
      .replace('{task}', taskTag(group, task))
      .replace('{by}', () => who(group, by)),
  rate_limited: (group, { ai, resets }, ts) =>
    t('thread.sys.rate_limited')
      .replace('{time}', clockOn(resets, ts, prefs.lang))
      .replace('{ai}', () => who(group, ai)),
  group_paused: (group, { by }) =>
    t('thread.sys.group_paused').replace('{by}', () => who(group, by)),
  group_resumed: (group, { by }) =>
    t('thread.sys.group_resumed').replace('{by}', () => who(group, by)),
  member_joined: (group, { by, member }) =>
    t('thread.sys.member_joined')
      .replace('{by}', () => who(group, by))
      .replace('{who}', () => who(group, member)),
  member_removed: (group, { name, by }) => removed(group, name, by),
  no_secretary: () => t('thread.sys.no_secretary'),
};

/** A notice in the reader's language. */
export function systemText(group: Group, { body, ts }: SystemMsg) {
  const wording = words[body.code] as Wording<Body> | undefined;
  return wording ? wording(group, body, ts) : body.code;
}
