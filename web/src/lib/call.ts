// Pure display derivations for AI-to-AI call blocks (contracts/messages.md 2.6, 3.4; flows/ask.md).
import { parts, type Part } from './compose';
import type { Group, Member } from './groups.svelte';
import { t } from './prefs.svelte';
import type { MsgRow } from './proto';

export type CallMsg = Extract<MsgRow, { kind: 'call' }>;

export interface ReplyView {
  by: string;
  name: string;
  cli: string | undefined;
  segs: Part[];
}

export interface CallView {
  caller: string;
  callee: string;
  mode: string;
  waiting: boolean;
  timing: string;
  depth: string;
  segs: Part[];
  reply: ReplyView | undefined;
}

function formatSpan(ms: number): string {
  if (ms < 60_000) return t('call.time.s').replace('{n}', String(Math.floor(ms / 1_000)));
  if (ms < 3_600_000) return t('call.time.m').replace('{n}', String(Math.floor(ms / 60_000)));
  if (ms < 86_400_000) return t('call.time.h').replace('{n}', String(Math.floor(ms / 3_600_000)));
  return t('call.time.d').replace('{n}', String(Math.floor(ms / 86_400_000)));
}

/** Words an elapsed millisecond span; sub-second values stay in `ms` when `subSecond` is true. */
export function formatElapsed(ms: number, subSecond = false): string {
  const safe = Math.max(0, ms);
  if (subSecond && safe < 1_000) return t('call.time.ms').replace('{n}', String(safe));
  return formatSpan(safe);
}

function textSegments(group: Group, text: string, mentions: string[]): Part[] {
  return parts(
    text,
    group.members.filter((member) => mentions.includes(member.id)),
  );
}

function memberView(group: Group, id: string): { member: Member | undefined; name: string } {
  const member = group.member(id);
  return { member, name: member?.name ?? t('chat.gone') };
}

function findReply(group: Group, id: string): MsgRow | undefined {
  const row = group.rows.get(`msg ${id}`);
  return row && !('gone' in row) ? (row as MsgRow) : undefined;
}

function buildReply(group: Group, replyId: string): ReplyView | undefined {
  const row = findReply(group, replyId);
  if (!row) return undefined;
  const { member, name } = memberView(group, row.by);
  return {
    by: row.by,
    name,
    cli: member?.kind === 'ai' ? member.cli : undefined,
    segs: textSegments(group, row.text, row.mentions),
  };
}

/** Derives all display fields of a `call` message at `nowMs` from cached group rows. */
export function callView(group: Group, msg: CallMsg, nowMs: number): CallView {
  const base = {
    caller: memberView(group, msg.by).name,
    callee: memberView(group, msg.body.to).name,
    mode: t(msg.body.wait ? 'call.mode.wait' : 'call.mode.async'),
    depth: t('call.depth').replace('{n}', String(msg.body.depth)),
    segs: textSegments(group, msg.text, msg.mentions),
  };
  if (msg.state.status === 'waiting') {
    return {
      ...base,
      waiting: true,
      timing: t('call.waiting').replace('{time}', formatElapsed(nowMs - msg.ts)),
      reply: undefined,
    };
  }
  return {
    ...base,
    waiting: false,
    timing: t('call.took').replace('{time}', formatElapsed(msg.state.ms, true)),
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
