// Member-panel projections and add-AI candidate selection (planning/contracts/rows.md section 2,
// planning/contracts/commands.md section 1.2 and section 2).
import { lost, newId, type Group, type Home } from './groups.svelte';
import { t } from './prefs.svelte';
import type { AIMemberRow, HostInfo, HumanMemberRow, MsgRow, TaskRow } from './proto';

/** An AI CLI registered on one of the owner's connected computers. */
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

/** Display name of a supported CLI, or the raw identifier for an unknown one. */
export function cliTitle(agent: string): string {
  if (agent === 'claude') return 'Claude Code';
  if (agent === 'codex') return 'Codex';
  return agent;
}

/** Localized label of an AI account type, or the raw value for an unknown one. */
export function accountTitle(account: string): string {
  if (account === 'personal' || account === 'team' || account === 'api_key') {
    return t(`member.account.${account}`);
  }
  return account;
}

/** AI members of the group, placing the secretary first and keeping name order otherwise. */
export function aiMembers(group: Group): AIMemberRow[] {
  const all = group.members.filter((m): m is AIMemberRow => m.kind === 'ai');
  return [...all.filter((m) => m.secretary), ...all.filter((m) => !m.secretary)];
}

/** Human members of the group, in name order. */
export const humanMembers = (group: Group): HumanMemberRow[] =>
  group.members.filter((m): m is HumanMemberRow => m.kind === 'human');

/** Display name of the human member with `userId` in `group`. */
export const humanName = (group: Group, userId: string | undefined): string | undefined =>
  humanMembers(group).find((person) => person.user === userId)?.name;

/** Whether the reader at this browser is the group's owner or an admin. */
export function isAdmin(group: Group): boolean {
  const me = group.me as HumanMemberRow | undefined;
  return Boolean(me && (me.role === 'owner' || me.role === 'admin'));
}

/** Reason why "Add AI" is unavailable right now, or undefined when enabled. */
export function addDisabledReason(group: Group, allHomes: readonly Home[]): string | undefined {
  if (!group.home.ctl || !group.info) return t('member.add.offline');
  if (!isAdmin(group)) return t('member.add.forbidden');
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

function taskOf(group: Group, id: string | undefined): TaskRow | undefined {
  const row = group.rows.get(`task ${id}`);
  return row && !('gone' in row) ? (row as TaskRow) : undefined;
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

// Pending briefs, approvals and decisions sit on main (contracts/commands.md 1.3);
// group.waiting holds only cards that wait on this browser's user.
/** Display name of another member that `ai` is waiting on, if known. */
export function otherWaiter(
  group: Group,
  ai: AIMemberRow,
  task: TaskRow | undefined,
): string | undefined {
  const taskWho = task?.wait?.who?.[0];
  if (taskWho) return humanName(group, taskWho);
  for (const msg of group.main) {
    if (msg.state?.status !== 'pending' || !cardOfAI(msg, ai)) continue;
    if (msg.kind === 'brief') return group.member(msg.body.requester)?.name;
    if (msg.kind === 'approval') return humanName(group, msg.body.approvers[0]);
    return humanName(group, (msg.body as { to: string[] }).to[0]);
  }
  return undefined;
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

/** Status badge key and localized status line for an AI member card. */
export function statusOf(group: Group, ai: AIMemberRow): { st: string; text: string } {
  const task = taskOf(group, ai.task);
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
  if (ai.state === 'dormant') return { st: 'sleeping', text: t('member.state.sleeping') };
  if (ai.state === 'limited') return { st: 'limited', text: t('member.state.limited') };
  return { st: 'offline', text: t('member.state.offline') };
}

/** Localized connection-path label for an AI member's host. */
export function pathLabel(ai: AIMemberRow): string {
  if (ai.state === 'offline') return t('member.path.offline');
  if (ai.path === 'local' || ai.path === 'direct' || ai.path === 'relay') {
    return t(`member.path.${ai.path}`);
  }
  return ai.path;
}

/** Registered AIs on the owner's connected computers, marking those already in `group`. */
export function candidatesOf(
  group: Group,
  owned: readonly Home[],
  infos: ReadonlyMap<Home, HostInfo>,
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
  return agent ? freeName(group, cliTitle(agent)) : '';
}

/** Worded error text for `host.join` failures, or undefined for generic `Failure` codes. */
export function joinError(code: string): string | undefined {
  if (code === 'home_unreachable' || code === 'conflict' || code === 'lost') {
    return t(`member.join.error.${code}`);
  }
  return undefined;
}

/** Reads `host.info` once for each connected home in `targets` not yet in `asked`. */
export async function readHosts(
  targets: readonly Home[],
  asked: Set<Home>,
  held: ReadonlyMap<Home, HostInfo>,
): Promise<{ infos: Map<Home, HostInfo>; error: string } | undefined> {
  const fresh = targets.filter((home) => Boolean(home.ctl) && !asked.has(home));
  if (fresh.length === 0) return undefined;
  for (const home of fresh) asked.add(home);
  const results = await Promise.all(
    fresh.map(async (home) => ({
      home,
      answer: await home.ctl!.request({ t: 'host.info' }).catch(lost),
    })),
  );
  const infos = new Map(held);
  let error = '';
  for (const { home, answer } of results) {
    if (answer.err) error ||= answer.err.code;
    else infos.set(home, answer);
  }
  return { infos, error: infos.size === 0 ? error : '' };
}

/** Signs and sends `host.join` to `target`, returning the refusal code or an empty string on ok. */
export async function joinGroup(
  target: Home | undefined,
  group: Group,
  candidate: Candidate,
  name: string,
  secretary: boolean,
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
    }),
  );
  const answer = await target.ctl.request({ t: 'cmd', env }).catch(lost);
  return answer.err ? answer.err.code : '';
}
