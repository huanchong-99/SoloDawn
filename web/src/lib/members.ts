// Member-panel projections and add-AI candidate selection (planning/contracts/rows.md section 2,
// planning/contracts/commands.md section 1.2 and section 2).
import { lost, newId, type Group, type Home, type NewGroup } from './groups.svelte';
import type { Net } from './net.svelte';
import { cliName } from './onboard.svelte';
import { t, type Key } from './prefs.svelte';
import type {
  AIMemberRow,
  ErrorResponse,
  HostInfoPush,
  HostJoin,
  HumanMemberRow,
  MsgRow,
  TaskRow,
} from './proto';
import { hostHomeOf } from './terminal.svelte';
import { clockOn, dayOf, now } from './when';

/** The model, level and profile an AI member is added or edited with. */
export type Draft = Pick<AIMemberRow, 'model' | 'effort' | 'tier' | 'skills'> &
  Pick<HostJoin, 'account' | 'tool'>;

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

/** AI members of the group, placing the secretary first and keeping name order otherwise. */
export function aiMembers(group: Group): AIMemberRow[] {
  const all = group.members.filter((m): m is AIMemberRow => m.kind === 'ai');
  return [...all.filter((m) => m.secretary), ...all.filter((m) => !m.secretary)];
}

/** Whether the group has an AI member. */
export const hasAI = (group: Group): boolean => group.members.some((m) => m.kind === 'ai');

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
    key:
      | 'member.state.sleeping'
      | 'member.state.limited'
      | 'member.state.takeover'
      | 'member.state.paused';
  }
> = {
  dormant: { st: 'sleeping', key: 'member.state.sleeping' },
  limited: { st: 'limited', key: 'member.state.limited' },
  takeover: { st: 'takeover', key: 'member.state.takeover' },
  paused: { st: 'paused', key: 'member.state.paused' },
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

/** Usage line of an AI member card, with a bar when the CLI reported a share of its limit. */
export interface UsageMeter {
  label: string;
  value: string;
  fill?: number;
  tone?: 'danger';
  note?: string;
}

/** The turns of the day the AI's computer last reported in, while that day lasts. */
function turnsToday({ usage, last_seen }: AIMemberRow): UsageMeter | undefined {
  const turns = usage.turns_today;
  if (turns === undefined || dayOf(last_seen) !== dayOf(now())) return undefined;
  return { label: t('member.usage.turns'), value: String(turns) };
}

/** Projects the AI's usage into the card's `.meter`: the rate-limit window the CLI reported, or
 *  the turns of today for a CLI that reports none; undefined when there is neither. */
export function usageMeter(ai: AIMemberRow, lang: string): UsageMeter | undefined {
  const { window_pct: pct, window_resets: resets } = ai.usage;
  if (pct === undefined) return turnsToday(ai);
  const shown = Math.round(pct);
  return {
    label: t('member.usage.window'),
    value: `${shown}%`,
    fill: Math.min(100, Math.max(0, shown)),
    tone: pct >= 100 ? 'danger' : undefined,
    note:
      resets === undefined
        ? undefined
        : t('member.usage.resets').replace('{time}', clockOn(resets, now(), lang)),
  };
}

/** Reason why "Open terminal" on an owned AI card is disabled, or `undefined` when direct. */
export function terminalDisabledReason(net: Net, ai: AIMemberRow): string | undefined {
  const host = hostHomeOf(net.homes, ai.host);
  if (!host?.ctl || ai.state === 'offline') return t('member.terminal.offline');
  return net.path(host)?.kind === 'direct' ? undefined : t('member.terminal.relay');
}

/** Reason why "Watch" is disabled: the path to the AI's computer is known and relayed. Before a
 *  grant this browser may have none (contracts/step-2/watch-team.md section 1). */
export function watchDisabledReason(net: Net, ai: AIMemberRow): string | undefined {
  const host = hostHomeOf(net.homes, ai.host);
  return host && net.path(host)?.kind === 'relay' ? t('watch.relay') : undefined;
}

/** The tasks that keep an AI in the group: those that are not final and have it as assignee or
 *  reviewer (contracts/commands.md 1.2 `remove`), by number. No other row has either. */
export const openTasks = (group: Group, ai: AIMemberRow): TaskRow[] =>
  ([...group.rows.values()] as TaskRow[])
    .filter((task) => [task.assignee, task.reviewer].includes(ai.id))
    .filter((task) => !['done', 'cancelled'].includes(task.state))
    .sort((a, b) => a.num - b.num);

/** Registered AIs on the owner's computers, marking those already in `group`; none is in a group
 *  that is still being made. */
export function candidatesOf(
  group: Group | undefined,
  owned: readonly Home[],
  infos: ReadonlyMap<Home, HostInfoPush>,
): Candidate[] {
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
        entry.groups.some((g) => g.g === group?.g) ||
        (group !== undefined &&
          aiMembers(group).some((m) => m.host === info.device && m.cli === entry.agent)),
    }));
  });
}

/** Why an AI cannot be added now: its account type is not declared, its computer cannot be
 *  reached, or its sign-in is no longer valid (contracts/commands.md section 2). */
export function unavailable(candidate: Candidate): string | undefined {
  if (candidate.agent !== 'external' && candidate.account === null)
    return t('member.join.unconfigured');
  if (!candidate.home.ctl) return t('member.join.away');
  return candidate.loginOk ? undefined : t('member.join.nologin.why');
}

/** Candidates that have an account type set; one already in the group adds another member. */
export const selectableCandidates = (candidates: readonly Candidate[]): Candidate[] =>
  candidates.filter((c) => c.agent === 'external' || c.account !== null);

/** Whether the add-AI dialog offers making the new AI the group's secretary. */
export const canMakeSecretary = (group: Group): boolean => !group.info?.secretary || isAdmin(group);

/** `base`, or else `base 2`, `base 3` and on: the first of them that is not among `taken`. */
export function freeName(taken: Iterable<string>, base: string): string {
  const used = new Set(taken);
  if (!used.has(base)) return base;
  let n = 2;
  while (used.has(`${base} ${n}`)) n += 1;
  return `${base} ${n}`;
}

/** Whether two of `names` are one name once trimmed: a dialog says so before it sends anything. */
export const nameClash = (names: readonly string[]): boolean =>
  new Set(names.map((each) => each.trim())).size < names.length;

/** Suggests an unused member name in `group` for `agent` (or for a first secretary). */
export function suggestedName(
  group: Group,
  agent: string | undefined,
  asSecretary: boolean,
): string {
  const names = group.members.map((m) => m.name);
  if (asSecretary && !group.info?.secretary) {
    return freeName(names, t('member.join.secretary.name'));
  }
  return agent ? freeName(names, cliName(agent)) : '';
}

/** A refusal: its code, and the words of the node or home that refused. */
export type Refusal = ErrorResponse['err'];

/** Worded error text for `host.join` failures, or undefined for generic `Failure` codes. A
 *  `conflict` of the node is about the folder: its words name the group that has it (contracts/
 *  commands.md section 2). A taken name is the dialog's to see before it sends. */
export function joinError({ code, msg }: Refusal): string | undefined {
  if (code === 'conflict') return t('member.join.error.folder').replace('{msg}', () => msg);
  return t(`member.join.error.${code}` as Key);
}

/** An AI to add to a group: the CLI on one of the person's computers, its name and folder there,
 *  whether it is the secretary, and its profile. */
export interface NewAI {
  candidate: Candidate;
  name: string;
  secretary: boolean;
  /** The folder it works in, from `Nodes.folders` or typed by the person. */
  workspace: string;
  draft: Draft;
}

/** Signs and sends `host.join` to the computer of the AI, for group `g` homed on the device
 *  `home`; the refusal, or nothing on ok (contracts/commands.md section 2). */
export async function joinGroup(g: string, home: string, ai: NewAI) {
  const { candidate, name, secretary, workspace, draft } = ai;
  const target = candidate.home;
  if (!target.ctl) return lost().err;
  const external = candidate.agent === 'external';
  const { account, tool, ...profile } = draft;
  const env = await target.me.sign(
    JSON.stringify({
      t: 'host.join',
      n: newId(),
      ts: Date.now(),
      to: candidate.device,
      g,
      home,
      agent: candidate.agent,
      name,
      secretary: !external && secretary,
      workspace,
      ...profile,
      ...(external && { account, tool: tool || undefined }),
    }),
  );
  const answer = await target.ctl.request({ t: 'cmd', env }).catch(lost);
  return answer.err;
}

/** A person this one may add to a group without an invitation, with a name to give them. */
export interface Known {
  user: string;
  name: string;
}

/** The people in the groups homed on `home` that this person reads, less this person and those
 *  already in `group`: they can be added with `member add_person`. Each comes with the name they
 *  have in the first of those groups (contracts/step-2/people.md section 2.5). */
export function knownPeople(home: Home, group?: Group): Known[] {
  const known = new Map(
    home.groups
      .flatMap(humanMembers)
      .reverse()
      .map(({ user, name }) => [user, name]),
  );
  known.delete(home.me.user);
  if (group) for (const { user } of humanMembers(group)) known.delete(user);
  return [...known]
    .map(([user, name]) => ({ user, name }))
    .sort((a, b) => a.name.localeCompare(b.name));
}

/** Adds a known person to group `g` of `home`; the refusal, or nothing on ok. */
export async function addPerson(home: Home, g: string, { user, name }: Known) {
  const answer = await home.command({ t: 'member', g, op: 'add_person', user, name });
  return answer.err;
}

/** What the create dialog chose (contracts/commands.md 1.1). */
export interface Founding extends NewGroup {
  people: Known[];
  ais: NewAI[];
}

/** Where making a group stopped: `group create`, or the person or AI at `index` of its list. */
export interface Stop {
  step: 'group' | 'people' | 'ais';
  index: number;
  err: Refusal;
}

/** Creates a group on `home`, then adds each person and joins each AI, one at a time, in the
 *  order chosen (contracts/commands.md 1.1). Nothing in it is atomic: the first refusal stops it,
 *  and is answered with where it came; what came before it stays done. */
export async function found(
  home: Home,
  { people, ais, ...named }: Founding,
): Promise<{ g: string; stopped?: Stop }> {
  const { g, answer } = await home.create(named);
  if (answer.err) return { g, stopped: { step: 'group', index: 0, err: answer.err } };
  for (const [index, person] of people.entries()) {
    const err = await addPerson(home, g, person);
    if (err) return { g, stopped: { step: 'people', index, err } };
  }
  for (const [index, ai] of ais.entries()) {
    const err = await joinGroup(g, home.device, ai);
    if (err) return { g, stopped: { step: 'ais', index, err } };
  }
  return { g };
}
