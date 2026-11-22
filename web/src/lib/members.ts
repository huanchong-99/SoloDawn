// Member-panel projections and add-AI candidate selection (planning/contracts/rows.md section 2,
// planning/contracts/commands.md section 1.2 and section 2).
import { lost, newId, type Group, type Home } from './groups.svelte';
import type { Net } from './net.svelte';
import { cliName } from './onboard.svelte';
import { t } from './prefs.svelte';
import type { AIMemberRow, HostInfoPush, HumanMemberRow, MsgRow, TaskRow } from './proto';
import { hostHomeOf } from './terminal.svelte';
import { clock } from './when';

/** An AI CLI registered on one of the owner's connected computers. */
/** The model, level and profile an AI member is added or edited with. */
export type Draft = Pick<AIMemberRow, 'model' | 'effort' | 'tier' | 'skills'>;

export interface Candidate {
  key: string;
  home: Home;
  device: string;
  hostName: string;
  agent: string;
  account: string | null;
  loginOk: boolean;
  inGroup: boolean;
}

/** AI members of the group, placing the secretary first and keeping name order otherwise. */
export function aiMembers(group: Group): AIMemberRow[] {
  const all = group.members.filter((m): m is AIMemberRow => m.kind === 'ai');
  return [...all.filter((m) => m.secretary), ...all.filter((m) => !m.secretary)];
}

/** Human members of the group, in name order. */
export const humanMembers = (group: Group): HumanMemberRow[] =>
  group.members.filter((m): m is HumanMemberRow => m.kind === 'human');

/** Whether the reader at this browser is the group's owner or an admin. */
export function isAdmin(group: Group): boolean {
  const me = group.me as HumanMemberRow | undefined;
  return Boolean(me && (me.role === 'owner' || me.role === 'admin'));
}

/** Reason why "Add AI" is unavailable right now, or undefined when enabled or still loading. */
export function addDisabledReason(group: Group, allHomes: readonly Home[]): string | undefined {
  if (!group.home.ctl) return t('member.add.offline');
  if (!group.info) return undefined;
  if (!group.me) return t('member.add.forbidden');
  if (!allHomes.some((home) => home.owner && Boolean(home.ctl))) return t('member.add.nohost');
  return undefined;
}

/** Role and "You" tags shown beside a human member's name. */
export function personTags(group: Group, person: HumanMemberRow): string {
  const tags = person.id === group.me?.id ? [t('chat.you')] : [];
  if (person.role === 'owner') tags.push(t('member.owner'));
  else if (person.role === 'admin') tags.push(t('member.admin'));
  return tags.join(' · ');
}

const cardOfAI = (msg: MsgRow, ai: AIMemberRow): boolean =>
  msg.by === ai.id || (msg.body as { ai?: string }).ai === ai.id;

/** Kind of wait on `task` that blocks on this browser's user, if any. */
export function taskWaitKind(group: Group, task: TaskRow): string | undefined {
  if (task.wait?.who?.includes(group.home.me.user)) return task.wait.kind;
  return group.waiting.find(
    (m) => m.thread === task.id || (m.body as { task?: string }).task === task.id,
  )?.kind;
}

function needYouTaskKey(kind: string) {
  if (kind === 'approval') return 'member.state.needyou.approval' as const;
  if (kind === 'decision') return 'member.state.needyou.decision' as const;
  return 'member.state.needyou.task' as const;
}

/** Status line when `ai` is waiting on this browser's user, or undefined when it is not. */
export function waitingOnMe(
  group: Group,
  ai: AIMemberRow,
  task: TaskRow | undefined,
): string | undefined {
  if (task) {
    const kind = taskWaitKind(group, task);
    if (kind) return t(needYouTaskKey(kind)).replace('{task}', `T-${task.num}`);
  }
  const count = ai.secretary
    ? group.waiting.length
    : group.waiting.filter((m) => cardOfAI(m, ai)).length;
  if (count === 0) return undefined;
  return count === 1
    ? t('member.state.needyou.one')
    : t('member.state.needyou.many').replace('{n}', String(count));
}

function cardWaiter(group: Group, msg: MsgRow): string | undefined {
  if (msg.kind === 'brief') return group.member(msg.body.requester)?.name;
  const user =
    msg.kind === 'approval' ? msg.body.approvers[0] : (msg.body as { to: string[] }).to[0];
  return group.personName(user);
}

const firstWaiter = (task: TaskRow | undefined): string | undefined => task?.wait?.who?.[0];

// Pending briefs, approvals and decisions sit on main (contracts/commands.md 1.3);
// group.waiting holds only cards that wait on this browser's user.
/** Display name of another member that `ai` is waiting on, if known. */
export function otherWaiter(
  group: Group,
  ai: AIMemberRow,
  task: TaskRow | undefined,
): string | undefined {
  const who = firstWaiter(task);
  if (who) return group.personName(who);
  const card = group.main.find((msg) => msg.state?.status === 'pending' && cardOfAI(msg, ai));
  return card ? cardWaiter(group, card) : undefined;
}

/** Status line when `ai` is waiting, but not on this browser's user. */
export function waitingOnOther(group: Group, ai: AIMemberRow, task: TaskRow | undefined): string {
  const who = otherWaiter(group, ai, task);
  if (task && who) {
    const key =
      task.wait?.kind === 'approval'
        ? 'member.state.waiting.approval'
        : 'member.state.waiting.task';
    return t(key).replace('{task}', `T-${task.num}`).replace('{who}', who);
  }
  if (who) return t('member.state.waiting.who').replace('{who}', who);
  return task
    ? `${t('member.state.waiting')} · T-${task.num} ${task.title}`
    : t('member.state.waiting');
}

const idleStates: Record<
  string,
  {
    st: string;
    key: 'member.state.sleeping' | 'member.state.limited' | 'member.state.takeover';
  }
> = {
  dormant: { st: 'sleeping', key: 'member.state.sleeping' },
  limited: { st: 'limited', key: 'member.state.limited' },
  takeover: { st: 'takeover', key: 'member.state.takeover' },
};

/** Status badge key and localized status line for an AI member card. */
export function statusOf(group: Group, ai: AIMemberRow): { st: string; text: string } {
  const task = group.task(ai.task);
  if (ai.state === 'waiting') {
    const mine = waitingOnMe(group, ai, task);
    return mine
      ? { st: 'needyou', text: mine }
      : { st: 'waiting', text: waitingOnOther(group, ai, task) };
  }
  if (ai.state === 'working') {
    const base = t('member.state.working');
    return { st: 'working', text: task ? `${base} · T-${task.num} ${task.title}` : base };
  }
  const idle = idleStates[ai.state];
  return idle
    ? { st: idle.st, text: t(idle.key) }
    : { st: 'offline', text: t('member.state.offline') };
}

/** Localized connection-path label for an AI member's host. */
export function pathLabel(ai: AIMemberRow): string {
  if (ai.state === 'offline') return t('member.path.offline');
  if (ai.path === 'local' || ai.path === 'direct' || ai.path === 'relay') {
    return t(`member.path.${ai.path}`);
  }
  return ai.path;
}

/** Usage bar model for an AI member card: the rate-limit window the CLI reported. */
export interface UsageMeter {
  label: string;
  value: string;
  fill: number;
  tone?: 'danger';
  note?: string;
}

/** Projects `ai.usage` into the card's `.meter` bar, or undefined when no window was reported. */
export function usageMeter(usage: AIMemberRow['usage'], lang: string): UsageMeter | undefined {
  const pct = usage.window_pct;
  if (pct === undefined) return undefined;
  const shown = Math.round(pct);
  const resets = usage.window_resets;
  return {
    label: t('member.usage.window'),
    value: `${shown}%`,
    fill: Math.min(100, Math.max(0, shown)),
    tone: pct >= 100 ? 'danger' : undefined,
    note:
      resets === undefined
        ? undefined
        : t('member.usage.resets').replace('{time}', clock(resets, lang)),
  };
}

/** Reason why "Open terminal" on an owned AI card is disabled, or `undefined` when direct. */
export function terminalDisabledReason(net: Net, ai: AIMemberRow): string | undefined {
  const host = hostHomeOf(net.homes, ai.host);
  if (!host?.ctl || ai.state === 'offline') return t('member.terminal.offline');
  return net.path(host)?.kind === 'direct' ? undefined : t('member.terminal.relay');
}

/** Registered AIs on the owner's connected computers, marking those already in `group`. */
export function candidatesOf(
  group: Group,
  owned: readonly Home[],
  infos: ReadonlyMap<Home, HostInfoPush>,
): Candidate[] {
  const existing = aiMembers(group);
  return owned.flatMap((home) => {
    const info = infos.get(home);
    if (!info) return [];
    return info.agents.map((entry) => ({
      key: `${info.device}:${entry.agent}`,
      home,
      device: info.device,
      hostName: info.name,
      agent: entry.agent,
      account: entry.account,
      loginOk: entry.login_ok,
      inGroup:
        entry.groups.some((g) => g.g === group.g) ||
        existing.some((m) => m.host === info.device && m.cli === entry.agent),
    }));
  });
}

/** Candidates that have an account type set and are not yet in the group. */
export const selectableCandidates = (candidates: readonly Candidate[]): Candidate[] =>
  candidates.filter((c) => c.account !== null && !c.inGroup);

/** Whether the add-AI dialog offers making the new AI the group's secretary. */
export const canMakeSecretary = (group: Group): boolean => !group.info?.secretary || isAdmin(group);

function freeName(group: Group, base: string): string {
  const taken = new Set(group.members.map((m) => m.name));
  if (!taken.has(base)) return base;
  let n = 2;
  while (taken.has(`${base} ${n}`)) n += 1;
  return `${base} ${n}`;
}

/** Suggests an unused member name in `group` for `agent` (or for a first secretary). */
export function suggestedName(
  group: Group,
  agent: string | undefined,
  asSecretary: boolean,
): string {
  if (asSecretary && !group.info?.secretary) {
    return freeName(group, t('member.join.secretary.name'));
  }
  return agent ? freeName(group, cliName(agent)) : '';
}

/** Worded error text for `host.join` failures, or undefined for generic `Failure` codes. */
export function joinError(code: string): string | undefined {
  if (code === 'home_unreachable' || code === 'conflict' || code === 'lost') {
    return t(`member.join.error.${code}`);
  }
  return undefined;
}

/** Signs and sends `host.join` to `target`, returning the refusal code or an empty string on ok. */
export async function joinGroup(
  target: Home | undefined,
  group: Group,
  candidate: Candidate,
  name: string,
  secretary: boolean,
  draft: Draft,
): Promise<string> {
  if (!target?.ctl) return 'lost';
  const env = await target.me.sign(
    JSON.stringify({
      t: 'host.join',
      n: newId(),
      ts: Date.now(),
      to: candidate.device,
      g: group.g,
      home: group.info!.home,
      agent: candidate.agent,
      name,
      secretary,
      ...draft,
    }),
  );
  const answer = await target.ctl.request({ t: 'cmd', env }).catch(lost);
  return answer.err ? answer.err.code : '';
}
