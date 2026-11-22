// Pure display derivations for AI-to-AI call blocks (contracts/messages.md 2.6, 3.4; flows/ask.md).
import { createSubscriber } from 'svelte/reactivity';
import type { Group } from './groups.svelte';
import { t } from './prefs.svelte';
import type { MsgRow } from './proto';
import { now } from './when';

export type CallMsg = Extract<MsgRow, { kind: 'call' }>;

export interface ReplyView {
  by: string;
  name: string;
  cli: string | undefined;
  text: string;
  mentions: string[];
}

export interface CallView {
  caller: string;
  callee: string;
  mode: string;
  waiting: boolean;
  timing: string;
  depth: string;
  reply: ReplyView | undefined;
}

/** Once a second, while anything that is drawn waits on a call made less than a minute ago. */
const everySecond = createSubscriber((update) => {
  const timer = setInterval(update, 1_000);
  return () => clearInterval(timer);
});

/** Words an elapsed millisecond span in whole seconds, minutes, hours or days. */
export function formatElapsed(ms: number): string {
  const safe = Math.max(0, ms);
  if (safe < 60_000) return t('call.time.s').replace('{n}', String(Math.floor(safe / 1_000)));
  if (safe < 3_600_000) return t('call.time.m').replace('{n}', String(Math.floor(safe / 60_000)));
  if (safe < 86_400_000)
    return t('call.time.h').replace('{n}', String(Math.floor(safe / 3_600_000)));
  return t('call.time.d').replace('{n}', String(Math.floor(safe / 86_400_000)));
}

/** How long a call has waited. Under a minute it is counted in seconds, so it is drawn again
 *  every second; after that `now` draws it again every minute. */
function waited(msg: CallMsg): number {
  const age = now() - msg.ts;
  if (age < 60_000) everySecond();
  return age;
}

function nameOf(group: Group, id: string): string {
  return group.member(id)?.name ?? t('chat.gone');
}

function buildReply(group: Group, replyId: string): ReplyView | undefined {
  const row = group.rows.get(`msg ${replyId}`);
  if (!row || 'gone' in row) return undefined;
  const reply = row as MsgRow;
  const member = group.member(reply.by);
  return {
    by: reply.by,
    name: nameOf(group, reply.by),
    cli: member?.kind === 'ai' ? member.cli : undefined,
    text: reply.text,
    mentions: reply.mentions,
  };
}

/** Derives all display fields of a `call` message from cached group rows. */
export function callView(group: Group, msg: CallMsg): CallView {
  const base = {
    caller: nameOf(group, msg.by),
    callee: nameOf(group, msg.body.to),
    mode: t(msg.body.wait ? 'call.mode.wait' : 'call.mode.async'),
    depth: t('call.depth').replace('{n}', String(msg.body.depth)),
  };
  if (msg.state.status !== 'answered') {
    return {
      ...base,
      waiting: true,
      timing: t('call.waiting').replace('{time}', formatElapsed(waited(msg))),
      reply: undefined,
    };
  }
  return {
    ...base,
    waiting: false,
    timing: t('call.took').replace('{time}', formatElapsed(msg.state.ms)),
    reply: buildReply(group, msg.state.reply),
  };
}

/** Drops standalone `reply` rows whose answered `call` row is in the same list when `callMounted` holds. */
export function withoutInlineReplies(msgs: MsgRow[], callMounted: boolean): MsgRow[] {
  if (!callMounted) return msgs;
  const inline = new Set<string>();
  for (const msg of msgs) {
    if (msg.kind === 'call' && msg.state.status === 'answered') inline.add(msg.state.reply);
  }
  return msgs.filter((msg) => !inline.has(msg.id));
}
