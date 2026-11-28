import { beforeEach, describe, expect, it, vi } from 'vitest';
import { newDraft } from '../components/profile/profile';
import { Group, Home } from './groups.svelte';
import {
  addDisabledReason,
  aiMembers,
  candidatesOf,
  canMakeSecretary,
  codeUntil,
  humanMembers,
  addPerson,
  found,
  freeName,
  groupWorkspace,
  hasAI,
  hasHostAgent,
  invitationText,
  isAdmin,
  joinError,
  joinGroup,
  knownPeople,
  lifetimes,
  nameClash,
  openTasks,
  otherWaiter,
  pathLabel,
  personTags,
  selectableCandidates,
  sendCode,
  statusOf,
  suggestedName,
  taskWaitKind,
  terminalDisabledReason,
  usageMeter,
  waitingOnMe,
  workingAis,
  unavailable,
  waitingOnOther,
  watchDisabledReason,
  type Candidate,
  type NewAI,
} from './members';
import { goldens } from './mock';
import type { Net } from './net.svelte';
import { prefs } from './prefs.svelte';
import * as p from './proto';

const gid = 'rZNvy-1jH6Z-BcPqA5U5BQ';
const meUser = 'alex-user';
const meMemberId = 'alex-member';
const homeDev = 'home-dev';

const sign = async (payload: string) => ({ p: payload, k: 'dev-k', s: 'dev-s' });

let seq = 1000;
const nextSeq = () => (seq += 1);

function row<T extends p.Row = p.Row>(name: string, overrides: Record<string, unknown> = {}): T {
  const base = p.decode(p.Row, goldens[name]);
  return { ...base, seq: nextSeq(), ...overrides } as T;
}

function makeOnlineCtl(handler?: (req: unknown) => Promise<unknown>): p.Ctl {
  return {
    request: (async (req: unknown) => {
      if (handler) return handler(req);
      return { re: 1, ok: true };
    }) as p.Ctl['request'],
  };
}

function makeHome(opts: { user?: string; owner?: boolean; online?: boolean } = {}): Home {
  const home = new Home({ user: opts.user ?? meUser, sign });
  if (opts.online !== false) {
    const link = home.connect();
    const welcome = p.decode(p.Welcome, goldens['welcome']);
    link.online(makeOnlineCtl(), { ...welcome, owner: opts.owner ?? true, groups: [] });
  }
  return home;
}

function makeInfoRow(secretary: string | undefined): p.GroupRow {
  const info = row<p.GroupRow>('row-group', { id: gid, home: homeDev });
  if (secretary === undefined) delete (info as { secretary?: string }).secretary;
  else info.secretary = secretary;
  return info;
}

function makeGroup(
  opts: {
    role?: 'owner' | 'admin' | 'member' | 'none';
    online?: boolean;
    withInfo?: boolean;
    secretary?: string;
  } = {},
) {
  const { role = 'owner', online, withInfo = true, secretary } = opts;
  const home = makeHome({ online, owner: true });
  const group = new Group(home, gid);
  const rows: p.Row[] = withInfo ? [makeInfoRow(secretary)] : [];
  if (role !== 'none') {
    rows.push(
      row<p.HumanMemberRow>('row-member-human', {
        id: meMemberId,
        g: gid,
        user: meUser,
        name: 'Alex',
        role,
        online: true,
      }),
    );
  }
  group.apply(rows);
  return { home, group };
}

beforeEach(() => {
  prefs.lang = 'en';
  seq = 1000;
});

describe('member lists, roles and addDisabledReason', () => {
  it('separates AI and human members and places the secretary first regardless of name order', () => {
    const { group } = makeGroup();
    const alpha = row<p.AIMemberRow>('row-member-ai', {
      id: 'ai-a',
      g: gid,
      name: 'Alpha AI',
      secretary: false,
    });
    const zedSec = row<p.AIMemberRow>('row-member-ai', {
      id: 'ai-z',
      g: gid,
      name: 'Zed Secretary',
      secretary: true,
    });
    const beta = row<p.AIMemberRow>('row-member-ai', {
      id: 'ai-b',
      g: gid,
      name: 'Beta AI',
      secretary: false,
    });
    const mina = row<p.HumanMemberRow>('row-member-human', {
      id: 'mina-id',
      g: gid,
      user: 'mina-u',
      name: 'Mina',
      role: 'admin',
    });
    group.apply([alpha, zedSec, beta, mina]);

    expect(aiMembers(group).map((m) => m.name)).toEqual(['Zed Secretary', 'Alpha AI', 'Beta AI']);
    expect(humanMembers(group).map((m) => m.name)).toEqual(['Alex', 'Mina']);

    // Also check when secretary is already alphabetically first.
    const { group: g2 } = makeGroup();
    const earlySec = row<p.AIMemberRow>('row-member-ai', {
      id: 'ai-1',
      g: gid,
      name: 'AAA Sec',
      secretary: true,
    });
    const lateWork = row<p.AIMemberRow>('row-member-ai', {
      id: 'ai-2',
      g: gid,
      name: 'ZZZ Worker',
      secretary: false,
    });
    g2.apply([earlySec, lateWork]);
    expect(aiMembers(g2).map((m) => m.name)).toEqual(['AAA Sec', 'ZZZ Worker']);
    expect(workingAis(g2).map((m) => m.name)).toEqual(['ZZZ Worker']);

    // workingAis excludes secretaries matched by group.info.secretary even if member.secretary is false.
    const { group: g3 } = makeGroup({ withInfo: true, secretary: 'ai-sec-by-info' });
    g3.apply([
      row<p.AIMemberRow>('row-member-ai', {
        id: 'ai-sec-by-info',
        g: gid,
        name: 'Info Secretary',
        secretary: false,
      }),
      row<p.AIMemberRow>('row-member-ai', {
        id: 'ai-w1',
        g: gid,
        name: 'Worker One',
        secretary: false,
      }),
    ]);
    expect(workingAis(g3).map((m) => m.name)).toEqual(['Worker One']);

    // workingAis still works before a row-group info row arrives (group.info === undefined).
    const { group: g4 } = makeGroup({ withInfo: false });
    g4.apply([
      row<p.AIMemberRow>('row-member-ai', {
        id: 'ai-no-info',
        g: gid,
        name: 'Early Worker',
        secretary: false,
      }),
    ]);
    expect(workingAis(g4).map((m) => m.name)).toEqual(['Early Worker']);
  });

  it('says whether a group has an AI member and whether a device/CLI pair is already in it', () => {
    const { group } = makeGroup();
    expect(hasAI(group)).toBe(false);
    expect(hasHostAgent(group, 'dev-1', 'claude')).toBe(false);
    group.apply([
      row<p.AIMemberRow>('row-member-ai', {
        id: 'ai-a',
        g: gid,
        name: 'Alpha AI',
        host: 'dev-1',
        cli: 'claude',
      }),
    ]);
    expect(hasAI(group)).toBe(true);
    expect(hasHostAgent(group, 'dev-1', 'claude')).toBe(true);
    expect(hasHostAgent(group, 'dev-1', 'codex')).toBe(false);
    expect(hasHostAgent(group, 'dev-2', 'claude')).toBe(false);
  });

  it('checks isAdmin across owner, admin, regular member and missing me', () => {
    expect(isAdmin(makeGroup({ role: 'owner' }).group)).toBe(true);
    expect(isAdmin(makeGroup({ role: 'admin' }).group)).toBe(true);
    expect(isAdmin(makeGroup({ role: 'member' }).group)).toBe(false);
    expect(isAdmin(makeGroup({ role: 'none' }).group)).toBe(false);
  });

  it('returns the exact addDisabledReason for offline home, missing info, non-member and no owned host', () => {
    const ownedOnline = makeHome({ owner: true, online: true });
    const ownedOffline = makeHome({ owner: true, online: false });
    const guestOnline = makeHome({ owner: false, online: true });

    const offlineHome = makeGroup({ online: false, withInfo: true });
    expect(addDisabledReason(offlineHome.group, [ownedOnline])).toBe(
      'Cannot reach the group home.',
    );

    const offlineNoInfo = makeGroup({ role: 'none', online: false, withInfo: false });
    expect(addDisabledReason(offlineNoInfo.group, [ownedOnline])).toBe(
      'Cannot reach the group home.',
    );

    const noInfo = makeGroup({ role: 'none', online: true, withInfo: false });
    expect(addDisabledReason(noInfo.group, [])).toBeUndefined();

    const regularMember = makeGroup({ role: 'member', online: true, withInfo: true });
    expect(addDisabledReason(regularMember.group, [guestOnline, ownedOnline])).toBeUndefined();
    const nonMember = makeGroup({ role: 'none', online: true, withInfo: true });
    expect(addDisabledReason(nonMember.group, [ownedOnline])).toBe(
      'Only group members can add an AI.',
    );

    const adminNoHost = makeGroup({ role: 'admin', online: true, withInfo: true });
    expect(addDisabledReason(adminNoHost.group, [guestOnline, ownedOffline])).toBe(
      'Connect one of your computers first.',
    );

    expect(addDisabledReason(adminNoHost.group, [ownedOnline])).toBeUndefined();
  });

  it('formats personTags for self, owner, admin, plain member and absent me', () => {
    const { group } = makeGroup({ role: 'owner' });
    const alex = humanMembers(group)[0];
    const mina = row<p.HumanMemberRow>('row-member-human', {
      id: 'mina',
      g: gid,
      user: 'mina-u',
      name: 'Mina',
      role: 'admin',
    });
    const sam = row<p.HumanMemberRow>('row-member-human', {
      id: 'sam',
      g: gid,
      user: 'sam-u',
      name: 'Sam',
      role: 'member',
    });
    expect(personTags(group, alex)).toBe('You · Owner');
    expect(personTags(group, mina)).toBe('Admin');
    expect(personTags(group, sam)).toBe('');

    const { group: noMe } = makeGroup({ role: 'none' });
    expect(personTags(noMe, mina)).toBe('Admin');
  });
});

describe('AI status and path projections', () => {
  it('resolves taskWaitKind from task.wait.who and from pending thread or task cards', () => {
    const { group } = makeGroup();
    const directWait = row<p.TaskRow>('row-task-waiting', {
      id: 't1',
      g: gid,
      num: 1,
      wait: { kind: 'approval', who: [meUser], since: 1 },
    });
    expect(taskWaitKind(group, directWait)).toBe('approval');

    // wait is null -> falls back to group.waiting matching m.thread === task.id
    const threadTask = row<p.TaskRow>('row-task', { id: 't2', g: gid, num: 2, wait: null });
    group.apply([
      threadTask,
      row<p.MsgRow>('row-msg-decision-pending', {
        id: 'dec-thread',
        g: gid,
        thread: 't2',
        main: true,
        body: { question: 'Which?', options: [], to: [meUser] },
      }),
    ]);
    expect(taskWaitKind(group, threadTask)).toBe('decision');

    // wait has no who -> falls back to group.waiting matching m.body.task === task.id
    const bodyTask = row<p.TaskRow>('row-task-waiting', {
      id: 't3',
      g: gid,
      num: 3,
      wait: { kind: 'depends', since: 1 },
    });
    group.apply([
      bodyTask,
      row<p.MsgRow>('row-msg-approval-pending', {
        id: 'app-body',
        g: gid,
        thread: null,
        main: true,
        body: {
          task: 't3',
          ai: 'codex-1',
          tool: 'bash',
          command: 'npm i',
          category: 'install',
          approvers: [meUser],
        },
      }),
    ]);
    expect(taskWaitKind(group, bodyTask)).toBe('approval');

    // wait.who is someone else and waiting cards belong to other tasks -> undefined
    const otherTask = row<p.TaskRow>('row-task-waiting', {
      id: 't4',
      g: gid,
      num: 4,
      wait: { kind: 'approval', who: ['other-user'], since: 1 },
    });
    expect(taskWaitKind(group, otherTask)).toBeUndefined();
  });

  it('formats waitingOnMe for approval, decision, generic task wait, secretary count and AI cards', () => {
    const { group } = makeGroup();
    const ai = row<p.AIMemberRow>('row-member-ai', {
      id: 'ai-1',
      g: gid,
      secretary: false,
      state: 'waiting',
    });
    const sec = row<p.AIMemberRow>('row-member-ai', {
      id: 'sec-1',
      g: gid,
      secretary: true,
      state: 'waiting',
    });

    const tApproval = row<p.TaskRow>('row-task-waiting', {
      id: 'ta',
      g: gid,
      num: 7,
      wait: { kind: 'approval', who: [meUser], since: 1 },
    });
    const tDecision = row<p.TaskRow>('row-task-waiting', {
      id: 'td',
      g: gid,
      num: 8,
      wait: { kind: 'decision', who: [meUser], since: 1 },
    });
    const tGeneric = row<p.TaskRow>('row-task-waiting', {
      id: 'tg',
      g: gid,
      num: 9,
      wait: { kind: 'conflict', who: [meUser], since: 1 },
    });
    const tOther = row<p.TaskRow>('row-task-waiting', {
      id: 'to',
      g: gid,
      num: 10,
      wait: { kind: 'approval', who: ['mina-u'], since: 1 },
    });

    expect(waitingOnMe(group, ai, tApproval)).toBe('Waiting · T-7 waiting for your approval');
    expect(waitingOnMe(group, ai, tDecision)).toBe('Waiting · T-8 waiting for your decision');
    expect(waitingOnMe(group, ai, tGeneric)).toBe('Waiting · T-9 waiting on you');
    expect(waitingOnMe(group, ai, tOther)).toBeUndefined();
    expect(waitingOnMe(group, ai, undefined)).toBeUndefined();

    // 1 card posted directly by ai
    group.apply([
      row<p.MsgRow>('row-msg-decision-pending', {
        id: 'd1',
        g: gid,
        by: 'ai-1',
        thread: null,
        main: true,
        body: { question: 'Port?', options: [], to: [meUser] },
      }),
    ]);
    expect(waitingOnMe(group, ai, undefined)).toBe('Waiting · 1 item waiting on you');

    // 2nd card posted by sec on behalf of ai-1 (body.ai === 'ai-1'), plus 3rd card for another AI
    group.apply([
      row<p.MsgRow>('row-msg-approval-pending', {
        id: 'a2',
        g: gid,
        by: 'sec-1',
        thread: null,
        main: true,
        body: {
          task: 'tx',
          ai: 'ai-1',
          tool: 'bash',
          command: 'ls',
          category: 'exec',
          approvers: [meUser],
        },
      }),
      row<p.MsgRow>('row-msg-decision-pending', {
        id: 'd3',
        g: gid,
        by: 'other-ai',
        thread: null,
        main: true,
        body: { question: 'Other?', options: [], to: [meUser] },
      }),
    ]);
    expect(waitingOnMe(group, ai, undefined)).toBe('Waiting · 2 items waiting on you');
    expect(waitingOnMe(group, sec, undefined)).toBe('Waiting · 3 items waiting on you');
  });

  it('finds otherWaiter and formats waitingOnOther from task.wait and from pending main cards', () => {
    const { group } = makeGroup();
    group.apply([
      row<p.HumanMemberRow>('row-member-human', {
        id: 'mina-m',
        g: gid,
        user: 'mina-u',
        name: 'Mina',
        role: 'member',
      }),
    ]);
    const ai = row<p.AIMemberRow>('row-member-ai', { id: 'ai-1', g: gid, state: 'waiting' });

    // From task.wait.who
    const tApp = row<p.TaskRow>('row-task-waiting', {
      id: 't11',
      g: gid,
      num: 11,
      title: 'Docs',
      wait: { kind: 'approval', who: ['mina-u'], since: 1 },
    });
    expect(otherWaiter(group, ai, tApp)).toBe('Mina');
    expect(waitingOnOther(group, ai, tApp)).toBe("Waiting · T-11 waiting for Mina's approval");

    const tDec = row<p.TaskRow>('row-task-waiting', {
      id: 't12',
      g: gid,
      num: 12,
      title: 'API',
      wait: { kind: 'decision', who: ['mina-u'], since: 1 },
    });
    expect(waitingOnOther(group, ai, tDec)).toBe('Waiting · T-12 waiting on Mina');

    // Task with wait: null and task with wait without who, no cards on main
    const tNullWait = row<p.TaskRow>('row-task', {
      id: 't13',
      g: gid,
      num: 13,
      title: 'Clean up',
      wait: null,
    });
    const tNoWho = row<p.TaskRow>('row-task-waiting', {
      id: 't14',
      g: gid,
      num: 14,
      title: 'Review step',
      wait: { kind: 'reviewer', since: 1 },
    });
    expect(waitingOnOther(group, ai, tNullWait)).toBe('Waiting · T-13 Clean up');
    expect(waitingOnOther(group, ai, tNoWho)).toBe('Waiting · T-14 Review step');
    expect(waitingOnOther(group, ai, undefined)).toBe('Waiting');

    // Main-line non-pending, plain text, and another AI's pending card are skipped
    group.apply([
      row<p.MsgRow>('row-msg-text', { id: 'm-txt', g: gid, by: 'ai-1', thread: null }),
      row<p.MsgRow>('row-msg-brief-confirmed', { id: 'm-conf', g: gid, by: 'ai-1', thread: null }),
      row<p.MsgRow>('row-msg-decision-pending', {
        id: 'm-other-ai',
        g: gid,
        by: 'someone-else',
        thread: null,
        main: true,
        body: { question: 'Skip?', options: [], to: ['mina-u'] },
      }),
    ]);
    expect(otherWaiter(group, ai, undefined)).toBeUndefined();

    // Pending brief on main for Mina
    group.apply([
      row<p.MsgRow>('row-msg-brief-pending', {
        id: 'm-brief',
        g: gid,
        by: 'ai-1',
        thread: null,
        body: {
          ...(p.decode(p.MsgRow, goldens['row-msg-brief-pending']).body as object),
          requester: 'mina-m',
        },
      }),
    ]);
    expect(waitingOnOther(group, ai, undefined)).toBe('Waiting · waiting on Mina');
    // Task with wait: null or empty who: [] falls through to the pending main card.
    expect(waitingOnOther(group, ai, tNullWait)).toBe('Waiting · T-13 waiting on Mina');
    const tEmptyWho = row<p.TaskRow>('row-task-waiting', {
      id: 't15',
      g: gid,
      num: 15,
      title: 'Empty who',
      wait: { kind: 'approval', who: [], since: 1 },
    });
    expect(otherWaiter(group, ai, tEmptyWho)).toBe('Mina');

    // Pending brief whose requester left the group -> undefined
    const { group: gGone } = makeGroup();
    gGone.apply([
      row<p.MsgRow>('row-msg-brief-pending', {
        id: 'm-brief-gone',
        g: gid,
        by: 'ai-1',
        thread: null,
        body: {
          ...(p.decode(p.MsgRow, goldens['row-msg-brief-pending']).body as object),
          requester: 'left-member',
        },
      }),
    ]);
    expect(otherWaiter(gGone, ai, undefined)).toBeUndefined();

    // Pending approval on main (by secretary for body.ai === 'ai-1') for Mina
    const { group: gApp } = makeGroup();
    gApp.apply([
      row<p.HumanMemberRow>('row-member-human', {
        id: 'mina-m',
        g: gid,
        user: 'mina-u',
        name: 'Mina',
        role: 'member',
      }),
      row<p.MsgRow>('row-msg-approval-pending', {
        id: 'm-app',
        g: gid,
        by: 'sec-1',
        thread: 't99',
        main: true,
        body: {
          task: 't99',
          ai: 'ai-1',
          tool: 'bash',
          command: 'git push',
          category: 'push',
          approvers: ['mina-u'],
        },
      }),
    ]);
    expect(otherWaiter(gApp, ai, undefined)).toBe('Mina');

    // Pending decision on main by ai-1 for Mina
    const { group: gDec } = makeGroup();
    gDec.apply([
      row<p.HumanMemberRow>('row-member-human', {
        id: 'mina-m',
        g: gid,
        user: 'mina-u',
        name: 'Mina',
        role: 'member',
      }),
      row<p.MsgRow>('row-msg-decision-pending', {
        id: 'm-dec',
        g: gid,
        by: 'ai-1',
        thread: null,
        main: true,
        body: { question: 'Where?', options: [], to: ['mina-u'] },
      }),
    ]);
    expect(otherWaiter(gDec, ai, undefined)).toBe('Mina');
  });

  it('computes statusOf for waiting, working (with, without and deleted task), dormant, limited and offline', () => {
    const { group } = makeGroup();
    group.apply([
      row<p.TaskRow>('row-task', { id: 't12', g: gid, num: 12, title: 'Ship panel' }),
      row<p.TaskRow>('row-task', { id: 't-del', g: gid, num: 15, title: 'Removed' }),
      { r: 'task', id: 't-del', seq: nextSeq(), gone: true },
      row<p.TaskRow>('row-task-waiting', {
        id: 't-me',
        g: gid,
        num: 16,
        title: 'Approve',
        wait: { kind: 'approval', who: [meUser], since: 1 },
      }),
    ]);

    const mk = (state: string, task?: string) =>
      row<p.AIMemberRow>('row-member-ai', { id: `ai-${state}`, g: gid, state, task });

    expect(statusOf(group, mk('working', 't12'))).toEqual({
      st: 'working',
      text: 'Working · T-12 Ship panel',
    });
    expect(statusOf(group, mk('working'))).toEqual({ st: 'working', text: 'Working' });
    expect(statusOf(group, mk('working', 't-del'))).toEqual({ st: 'working', text: 'Working' });
    expect(statusOf(group, mk('waiting', 't-me'))).toEqual({
      st: 'needyou',
      text: 'Waiting · T-16 waiting for your approval',
    });
    expect(statusOf(group, mk('waiting'))).toEqual({ st: 'waiting', text: 'Waiting' });
    expect(statusOf(group, mk('dormant'))).toEqual({
      st: 'sleeping',
      text: 'Sleeping · wakes when @mentioned',
    });
    expect(statusOf(group, mk('limited'))).toEqual({ st: 'limited', text: 'Limited' });
    expect(statusOf(group, mk('takeover'))).toEqual({ st: 'takeover', text: 'Owner is driving' });
    expect(statusOf(group, mk('offline'))).toEqual({ st: 'offline', text: 'Offline' });
    expect(statusOf(group, mk('paused'))).toEqual({ st: 'paused', text: 'Paused' });
    expect(statusOf(group, mk('unexpected'))).toEqual({ st: 'offline', text: 'Offline' });
  });

  it('computes pathLabel for offline, local, direct, relay and unknown paths', () => {
    const mk = (state: string, path: string) =>
      row<p.AIMemberRow>('row-member-ai', { id: 'ai-p', g: gid, state, path });
    expect(pathLabel(mk('offline', 'direct'))).toBe('offline');
    expect(pathLabel(mk('working', 'local'))).toBe('group home');
    expect(pathLabel(mk('working', 'direct'))).toBe('direct');
    expect(pathLabel(mk('working', 'relay'))).toBe('relayed, limited');
    expect(pathLabel(mk('working', 'mesh'))).toBe('mesh');
    prefs.lang = 'zh-CN';
    expect(pathLabel(mk('working', 'direct'))).toBe('直连');
  });

  it('projects usageMeter from the rate-limit window, or else from the turns of today', () => {
    const resetAt = new Date(2026, 10, 20, 15, 0).getTime();
    const at = new Date(2026, 10, 20, 9, 0).getTime();
    vi.useFakeTimers({ now: new Date(2026, 10, 20, 23, 59) });
    const meter = (usage: p.AIMemberRow['usage'], lang = 'en', last_seen = at) =>
      usageMeter(row<p.AIMemberRow>('row-member-ai', { usage, last_seen }), lang);
    const bar = (value: string, fill: number, tone?: 'danger', note?: string) => ({
      label: 'Quota used this window',
      value,
      fill,
      tone,
      note,
    });

    expect(meter({})).toBeUndefined();
    expect(meter({ window_resets: resetAt })).toBeUndefined();
    expect(meter({ window_pct: 18, turns_today: 14 })).toEqual(bar('18%', 18));
    expect(meter({ window_pct: -4 })).toEqual(bar('-4%', 0));
    expect(meter({ window_pct: 62.5, window_resets: resetAt })).toEqual(
      bar('63%', 63, undefined, 'Resets 15:00'),
    );
    // A window that resets on another day says the date.
    const later = new Date(2026, 10, 23, 3, 0).getTime();
    expect(meter({ window_pct: 40, window_resets: later })).toEqual(
      bar('40%', 40, undefined, 'Resets Nov 23, 03:00'),
    );
    expect(meter({ window_pct: 99.9 })).toEqual(bar('100%', 100));
    expect(meter({ window_pct: 100 })).toEqual(bar('100%', 100, 'danger'));
    expect(meter({ window_pct: 125 })).toEqual(bar('125%', 100, 'danger'));
    const turns = { label: 'Turns today', value: '14' };
    expect(meter({ turns_today: 14, window_resets: resetAt })).toStrictEqual(turns);
    // Reported before midnight: the count is yesterday's.
    vi.setSystemTime(new Date(2026, 10, 21, 0, 0));
    expect(meter({ turns_today: 14 })).toBeUndefined();

    vi.setSystemTime(new Date(2026, 10, 20, 23, 59));
    prefs.lang = 'zh-CN';
    expect(meter({ window_pct: 62, window_resets: resetAt }, 'zh-CN')).toEqual({
      label: '这一轮额度已用',
      value: '62%',
      fill: 62,
      tone: undefined,
      note: '15:00 重置',
    });
    expect(meter({ window_pct: 62, window_resets: later }, 'zh-CN')?.note).toBe(
      '11月23日 03:00 重置',
    );
    vi.useRealTimers();
  });

  it('disables watching on a relayed path only', () => {
    const { home } = makeGroup();
    const ai = row<p.AIMemberRow>('row-member-ai', { g: gid, host: home.device });
    const why = (entries: Home[], path: ReturnType<Net['path']>) =>
      watchDisabledReason({ source: 'live', homes: entries, path: () => path }, ai);
    const relay = { kind: 'relay', rtt: 80, since: 1 } as const;
    expect(why([home], relay)).toBe('Watching requires a direct connection');
    expect(why([home], { kind: 'direct', rtt: 4, since: 1 })).toBeUndefined();
    expect(why([home], undefined)).toBeUndefined();
    expect(why([], relay)).toBeUndefined();
  });

  it('lists the tasks that are not final and have the AI as assignee or reviewer, by number', () => {
    const { group } = makeGroup();
    const ai = row<p.AIMemberRow>('row-member-ai', { id: 'ai-o', g: gid });
    const task = (num: number, state: string, assignee: string, reviewer: string) =>
      row('row-task', { id: `t${num}`, g: gid, num, state, assignee, reviewer });
    const gone = { r: 'task', id: 't9', seq: nextSeq(), gone: true } as p.Row;
    group.apply([
      ai,
      task(8, 'failed', 'other', 'ai-o'),
      task(3, 'queued', 'ai-o', 'other'),
      task(5, 'paused', 'ai-o', 'other'),
      task(4, 'active', 'other', 'ai-o'),
      task(1, 'done', 'ai-o', 'other'),
      task(2, 'cancelled', 'ai-o', 'other'),
      task(6, 'active', 'other', 'another'),
      gone,
    ]);
    expect(openTasks(group, ai).map((each) => each.num)).toEqual([3, 4, 5, 8]);
  });

  it('computes terminalDisabledReason across offline host/AI, relay, linking and direct', () => {
    const { home } = makeGroup();
    const ai = row<p.AIMemberRow>('row-member-ai', {
      id: 'ai-t',
      g: gid,
      host: home.device,
      state: 'working',
    });
    const offlineHost = makeHome({ online: false });
    offlineHost.device = 'off-dev';
    const offlineAiHost = row<p.AIMemberRow>('row-member-ai', {
      id: 'ai-off-h',
      g: gid,
      host: 'off-dev',
      state: 'working',
    });
    const netOf = (entries: Home[], pathFn: Net['path']): Net => ({
      source: 'live',
      homes: entries,
      path: pathFn,
    });

    expect(
      terminalDisabledReason(
        netOf([], () => undefined),
        ai,
      ),
    ).toBe('Computer offline');
    expect(
      terminalDisabledReason(
        netOf([offlineHost], () => undefined),
        offlineAiHost,
      ),
    ).toBe('Computer offline');
    expect(
      terminalDisabledReason(
        netOf([home], () => ({ kind: 'direct', rtt: 4, since: 1 })),
        { ...ai, state: 'offline' },
      ),
    ).toBe('Computer offline');
    expect(
      terminalDisabledReason(
        netOf([home], () => undefined),
        ai,
      ),
    ).toBe('Direct link only');
    expect(
      terminalDisabledReason(
        netOf([home], () => ({ kind: 'relay', rtt: 80, since: 1 })),
        ai,
      ),
    ).toBe('Direct link only');
    expect(
      terminalDisabledReason(
        netOf([home], () => ({ kind: 'direct', rtt: 4, since: 1 })),
        ai,
      ),
    ).toBeUndefined();
  });
});

describe('add-AI candidates, suggested names, errors and host commands', () => {
  it('builds candidates, marks inGroup from host.info or group rows, and filters selectable', () => {
    const { home: h1, group } = makeGroup();
    const h2 = makeHome();
    const hMissing = makeHome();

    group.apply([
      row<p.AIMemberRow>('row-member-ai', {
        id: 'in-rows',
        g: gid,
        host: 'dev-2',
        cli: 'codex',
      }),
      row<p.AIMemberRow>('row-member-ai', {
        id: 'elsewhere',
        g: gid,
        host: 'dev-3',
        cli: 'codex',
      }),
    ]);

    const baseInfo = p.decode(p.HostInfo, goldens['host-info-answer']);
    const infos = new Map<Home, p.HostInfo>([
      [
        h1,
        {
          ...baseInfo,
          device: 'dev-1',
          name: 'workstation',
          agents: [
            {
              agent: 'claude',
              cli_version: '2.0.0',
              account: 'personal',
              login_ok: true,
              driver: { accounts: ['api_key'], takeover: 'none' },
              models: [],
              caps: {},
              groups: [
                {
                  g: 'other-group',
                  member: 'm0',
                  workspace: '/srv/docs',
                  level: 3,
                  state: 'dormant',
                  queued: 0,
                  watchers: [],
                },
                {
                  g: gid,
                  member: 'm1',
                  workspace: '/srv/site',
                  level: 3,
                  state: 'working',
                  queued: 0,
                  watchers: [],
                },
              ],
            },
            {
              agent: 'codex',
              cli_version: '0.1.0',
              account: null,
              login_ok: false,
              driver: { accounts: ['api_key'], takeover: 'none' },
              models: [],
              caps: {},
              groups: [
                {
                  g: 'other-group',
                  member: 'm2',
                  workspace: '/srv/docs',
                  level: 3,
                  state: 'working',
                  queued: 0,
                  watchers: [],
                },
              ],
            },
          ],
        },
      ],
      [
        h2,
        {
          ...baseInfo,
          device: 'dev-2',
          name: 'laptop',
          agents: [
            {
              agent: 'codex',
              cli_version: '0.1.0',
              account: 'team',
              login_ok: true,
              driver: { accounts: ['api_key'], takeover: 'none' },
              models: [],
              caps: {},
              groups: [],
            },
            {
              agent: 'claude',
              cli_version: '2.0.0',
              account: 'api_key',
              login_ok: false,
              driver: { accounts: ['api_key'], takeover: 'none' },
              models: [],
              caps: {},
              groups: [],
            },
          ],
        },
      ],
    ]);

    const list = candidatesOf(group, [h1, h2, hMissing], infos);
    expect(list).toEqual([
      {
        key: 'dev-1:claude',
        home: h1,
        device: 'dev-1',
        hostName: 'workstation',
        agent: 'claude',
        account: 'personal',
        loginOk: true,
        inGroup: true,
      },
      {
        key: 'dev-1:codex',
        home: h1,
        device: 'dev-1',
        hostName: 'workstation',
        agent: 'codex',
        account: null,
        loginOk: false,
        inGroup: false,
      },
      {
        key: 'dev-2:codex',
        home: h2,
        device: 'dev-2',
        hostName: 'laptop',
        agent: 'codex',
        account: 'team',
        loginOk: true,
        inGroup: true,
      },
      {
        key: 'dev-2:claude',
        home: h2,
        device: 'dev-2',
        hostName: 'laptop',
        agent: 'claude',
        account: 'api_key',
        loginOk: false,
        inGroup: false,
      },
    ]);
    expect(selectableCandidates(list).map((c) => c.key)).toEqual([
      'dev-1:claude',
      'dev-2:codex',
      'dev-2:claude',
    ]);
  });

  it('checks canMakeSecretary across missing info, no secretary, admin and regular member', () => {
    expect(canMakeSecretary(makeGroup({ role: 'member', withInfo: false }).group)).toBe(true);
    expect(canMakeSecretary(makeGroup({ role: 'member', secretary: undefined }).group)).toBe(true);
    expect(canMakeSecretary(makeGroup({ role: 'admin', secretary: 'sec-1' }).group)).toBe(true);
    expect(canMakeSecretary(makeGroup({ role: 'member', secretary: 'sec-1' }).group)).toBe(false);
  });

  it('suggests unused names for CLIs and secretaries, numbering when taken', () => {
    const { group } = makeGroup({ secretary: undefined });
    expect(suggestedName(group, undefined, false)).toBe('');
    expect(suggestedName(group, 'claude', false)).toBe('Claude Code');
    expect(suggestedName(group, 'qwen', false)).toBe('Qwen Code');
    expect(suggestedName(group, 'claude', true)).toBe('Secretary');

    const { group: noInfo } = makeGroup({ withInfo: false });
    expect(suggestedName(noInfo, 'codex', true)).toBe('Secretary');

    group.apply([
      row<p.AIMemberRow>('row-member-ai', { id: 'a1', g: gid, name: 'Claude Code' }),
      row<p.AIMemberRow>('row-member-ai', { id: 'a2', g: gid, name: 'Claude Code 2' }),
      row<p.AIMemberRow>('row-member-ai', { id: 'a3', g: gid, name: 'Secretary' }),
    ]);
    expect(suggestedName(group, 'claude', false)).toBe('Claude Code 3');
    expect(suggestedName(group, 'claude', true)).toBe('Secretary 2');

    // When the group already has a secretary, toggling secretary keeps the CLI's free name.
    const { group: withSec } = makeGroup({ secretary: 'sec-id' });
    expect(suggestedName(withSec, 'codex', true)).toBe('Codex');
  });

  it('finds the default groupWorkspace on a home, preferring the secretary', () => {
    const baseInfo = p.decode(p.HostInfoPush, goldens['host-info-push']);
    const entry = (g: string, member: string, workspace: string) => ({
      g,
      member,
      workspace,
      level: 3,
      state: 'dormant',
      queued: 0,
      watchers: [],
    });

    const { home, group } = makeGroup({ secretary: 'sec-by-info' });
    // No host info -> empty string.
    expect(groupWorkspace(group, home, new Map())).toBe('');

    // Host info with only another group or empty workspace -> empty string.
    const noMatchInfo: p.HostInfoPush = {
      ...baseInfo,
      agents: [
        { ...baseInfo.agents[0], agent: 'claude', groups: [entry('other-g', 'm0', '/srv/other')] },
        { ...baseInfo.agents[1], agent: 'codex', groups: [entry(gid, 'm1', '')] },
      ],
    };
    expect(groupWorkspace(group, home, new Map([[home, noMatchInfo]]))).toBe('');

    // Single matching AI in this group -> returns its workspace.
    const singleInfo: p.HostInfoPush = {
      ...baseInfo,
      agents: [
        {
          ...baseInfo.agents[0],
          agent: 'claude',
          groups: [entry('other-g', 'm0', '/srv/other'), entry(gid, 'm1', '/srv/single')],
        },
      ],
    };
    expect(groupWorkspace(group, home, new Map([[home, singleInfo]]))).toBe('/srv/single');

    // Differing folders on the same home: prefers the secretary (from group.info.secretary).
    const multiInfo: p.HostInfoPush = {
      ...baseInfo,
      agents: [
        { ...baseInfo.agents[0], agent: 'claude', groups: [entry(gid, 'wrk-1', '/srv/worker')] },
        {
          ...baseInfo.agents[1],
          agent: 'codex',
          groups: [entry(gid, 'sec-by-info', '/srv/secretary')],
        },
      ],
    };
    expect(groupWorkspace(group, home, new Map([[home, multiInfo]]))).toBe('/srv/secretary');

    // Differing folders without group.info: still prefers AI member with secretary === true.
    const { home: homeNoInfo, group: groupNoInfo } = makeGroup({ withInfo: false });
    groupNoInfo.apply([
      row<p.AIMemberRow>('row-member-ai', { id: 'wrk-1', g: gid, secretary: false }),
      row<p.AIMemberRow>('row-member-ai', { id: 'sec-by-info', g: gid, secretary: true }),
    ]);
    expect(groupWorkspace(groupNoInfo, homeNoInfo, new Map([[homeNoInfo, multiInfo]]))).toBe(
      '/srv/secretary',
    );

    // Differing folders without any secretary -> uses the first listed matching entry.
    const { home: homeNoSec, group: groupNoSec } = makeGroup({ secretary: undefined });
    expect(groupWorkspace(groupNoSec, homeNoSec, new Map([[homeNoSec, multiInfo]]))).toBe(
      '/srv/worker',
    );
  });

  it('words home_unreachable, conflict and lost in joinError and leaves others to Failure', () => {
    const err = (code: string, msg = '') => joinError({ code, msg });
    expect(err('home_unreachable')).toBe(
      "The AI's computer cannot reach this group's home right now. Try again when both are online.",
    );
    // A conflict of the node is about the folder, and its words name the group that has it.
    expect(err('conflict', 'the folder is the workspace of an AI of the group "Docs"')).toBe(
      'This folder is already the workspace of another group. The computer said: the folder is the workspace of an AI of the group "Docs"',
    );
    expect(err('lost')).toBe("Not sent: the AI's computer cannot be reached.");
    expect(err('forbidden')).toBeUndefined();
  });

  it('words the refusals of a folder that is not there or not whole in joinError', () => {
    const err = (code: string) => joinError({ code, msg: 'the node said' });
    expect(err('not_found')).toBe('There is no such folder on that computer.');
    expect(err('bad_request')).toBe(
      'Give the whole path of the folder, such as /Users/you/project.',
    );
  });

  it('numbers a free name from 2 and says when two names are one once trimmed', () => {
    expect(freeName([], 'Codex')).toBe('Codex');
    expect(freeName(['Codex', 'Codex 3'], 'Codex')).toBe('Codex 2');
    expect(freeName(new Set(['Codex', 'Codex 2']), 'Codex')).toBe('Codex 3');
    expect(nameClash(['Alex', 'Codex'])).toBe(false);
    expect(nameClash(['Alex', ' Alex '])).toBe(true);
    expect(nameClash([])).toBe(false);
  });

  it('signs host.join in joinGroup and handles an offline computer, refusal and rejection', async () => {
    const offline = makeHome({ online: false });
    const candidate = {
      key: 'dev-2:codex',
      home: offline,
      device: 'dev-2',
      hostName: 'laptop',
      agent: 'codex',
      account: 'personal',
      loginOk: true,
      inGroup: false,
    };
    const workspace = '/home/dev/website';
    const draft = { model: 'o3', effort: 'high', tier: 'strong', skills: 'Protocol design' };
    const ai = (changes: Partial<NewAI> = {}): NewAI => ({
      candidate,
      name: 'Lead',
      secretary: true,
      workspace,
      follow: false,
      draft,
      ...changes,
    });
    expect(await joinGroup(gid, homeDev, ai())).toMatchObject({ code: 'lost' });

    const sent: unknown[] = [];
    const target = makeHome();
    candidate.home = target;
    vi.spyOn(Date, 'now').mockReturnValue(1763600000123);
    target.ctl = makeOnlineCtl(async (req) => {
      sent.push(req);
      return { re: 1, ok: true };
    });

    expect(await joinGroup(gid, homeDev, ai())).toBeUndefined();
    expect(sent).toHaveLength(1);
    const frame = sent[0] as { t: string; env: p.Envelope };
    expect(frame.t).toBe('cmd');
    expect(frame.env.k).toBe('dev-k');
    expect(p.decode(p.HostJoin, frame.env.p)).toEqual({
      t: 'host.join',
      n: expect.stringMatching(/^[A-Za-z0-9_-]{21}[AQgw]$/),
      ts: 1763600000123,
      to: 'dev-2',
      g: gid,
      home: homeDev,
      agent: 'codex',
      name: 'Lead',
      secretary: true,
      workspace,
      model: 'o3',
      effort: 'high',
      tier: 'strong',
      skills: 'Protocol design',
    });

    const plain = ai({ name: 'Default', secretary: false, draft: newDraft() });
    expect(await joinGroup(gid, homeDev, plain)).toBeUndefined();
    expect(JSON.parse((sent[1] as { env: p.Envelope }).env.p)).toEqual({
      t: 'host.join',
      n: expect.stringMatching(/^[A-Za-z0-9_-]{21}[AQgw]$/),
      ts: 1763600000123,
      to: 'dev-2',
      g: gid,
      home: homeDev,
      agent: 'codex',
      name: 'Default',
      secretary: false,
      workspace,
      model: '',
      effort: '',
      tier: 'standard',
      skills: '',
    });

    const external = { ...candidate, agent: 'external', account: null, key: 'dev-2:external' };
    expect(selectableCandidates([external, { ...candidate, account: null }])).toEqual([external]);
    const scout = (account: string, tool: string) =>
      ai({ candidate: external, name: 'Scout', draft: { ...draft, account, tool } });
    expect(await joinGroup(gid, homeDev, scout('api_key', 'ZCode'))).toBeUndefined();
    const fields = (i: number) => {
      const { n, ts, ...payload } = JSON.parse((sent[i] as { env: p.Envelope }).env.p);
      return payload;
    };
    expect(fields(2)).toEqual({
      t: 'host.join',
      to: 'dev-2',
      g: gid,
      home: homeDev,
      agent: 'external',
      name: 'Scout',
      secretary: false,
      workspace,
      ...draft,
      account: 'api_key',
      tool: 'ZCode',
    });
    expect(await joinGroup(gid, homeDev, scout('team', ''))).toBeUndefined();
    expect(fields(3)).toEqual({ ...fields(2), account: 'team', tool: undefined });
    const other = ai({ follow: false, draft: { ...draft, account: 'team', tool: 'other' } });
    expect(await joinGroup(gid, homeDev, other)).toBeUndefined();
    expect(fields(4)).toEqual({
      ...fields(0),
    });

    const following = ai({ name: 'Follower', secretary: false, follow: true });
    expect(await joinGroup(gid, homeDev, following)).toBeUndefined();
    const followPayload = p.decode(p.HostJoin, (sent[5] as { env: p.Envelope }).env.p);
    expect(followPayload).toEqual({
      t: 'host.join',
      n: expect.stringMatching(/^[A-Za-z0-9_-]{21}[AQgw]$/),
      ts: 1763600000123,
      to: 'dev-2',
      g: gid,
      home: homeDev,
      agent: 'codex',
      name: 'Follower',
      secretary: false,
      workspace,
      follow: true,
    });

    target.ctl = makeOnlineCtl(async () => ({
      re: 2,
      err: { code: 'conflict', msg: 'the folder of Docs' },
    }));
    expect(await joinGroup(gid, homeDev, ai())).toEqual({
      code: 'conflict',
      msg: 'the folder of Docs',
    });

    target.ctl = makeOnlineCtl(async () => {
      throw new Error('closed');
    });
    expect(await joinGroup(gid, homeDev, ai())).toMatchObject({ code: 'lost' });
    vi.restoreAllMocks();
  });
});

describe('people and groups made with them', () => {
  const person = (g: string, id: string, user: string, name: string) =>
    row<p.HumanMemberRow>('row-member-human', { id, g, user, name });

  /** A home with two of this person's groups: Website with Mina and Leo, Docs with Mina as Mi. */
  function twoGroups() {
    const home = makeHome();
    const website = new Group(home, gid);
    const docs = new Group(home, 'docs');
    website.apply([
      person(gid, meMemberId, meUser, 'Alex'),
      person(gid, 'mina-1', 'mina-u', 'Mina'),
      person(gid, 'leo-1', 'leo-u', 'Leo'),
      row('row-member-ai', { id: 'ai-1', g: gid }),
    ]);
    docs.apply([
      person('docs', 'alex-2', meUser, 'Alex'),
      person('docs', 'mina-2', 'mina-u', 'Mi'),
    ]);
    home.groups = [website, docs];
    return { home, website, docs };
  }

  it('knows the people of the groups on a home, by the name of their first group there', () => {
    const { home, docs } = twoGroups();
    expect(knownPeople(home)).toEqual([
      { user: 'leo-u', name: 'Leo' },
      { user: 'mina-u', name: 'Mina' },
    ]);
    expect(knownPeople(home, docs)).toEqual([{ user: 'leo-u', name: 'Leo' }]);
    docs.apply([{ r: 'member', id: 'mina-2', seq: nextSeq(), gone: true }]);
    expect(knownPeople(home, docs)).toEqual([
      { user: 'leo-u', name: 'Leo' },
      { user: 'mina-u', name: 'Mina' },
    ]);
    expect(knownPeople(makeHome())).toEqual([]);
  });

  it('adds a known person with member add_person', async () => {
    const { home } = twoGroups();
    const sent: p.Envelope[] = [];
    home.ctl = makeOnlineCtl(async (req) => {
      sent.push((req as { env: p.Envelope }).env);
      return sent.length > 1
        ? { re: 2, err: { code: 'not_found', msg: 'no log' } }
        : { re: 1, ok: true };
    });
    const mina = { user: 'mina-u', name: 'Mina' };
    expect(await addPerson(home, gid, mina)).toBeUndefined();
    expect(p.decode(p.MemberAddPerson, sent[0].p)).toEqual({
      t: 'member',
      n: expect.any(String),
      ts: expect.any(Number),
      g: gid,
      op: 'add_person',
      ...mina,
    });
    expect(await addPerson(home, gid, mina)).toEqual({ code: 'not_found', msg: 'no log' });
  });

  it('creates a group, then adds its people and joins its AIs one at a time, up to a refusal', async () => {
    const home = makeHome();
    const sent: Record<string, unknown>[] = [];
    let refuse = (_fields: Record<string, unknown>) => false;
    home.ctl = makeOnlineCtl(async (req) => {
      const fields = JSON.parse((req as { env: p.Envelope }).env.p);
      sent.push(fields);
      // Each answer waits a moment: what is sent one at a time is never sent together.
      await new Promise((later) => setTimeout(later));
      return refuse(fields)
        ? { re: 2, err: { code: 'conflict', msg: 'taken' } }
        : { re: 1, ok: true };
    });
    const candidate: Candidate = {
      key: 'dev:claude',
      home,
      device: home.device,
      hostName: 'workstation',
      agent: 'claude',
      account: 'personal',
      loginOk: true,
      inGroup: false,
    };
    const ai = (name: string) => ({
      candidate,
      name,
      secretary: false,
      workspace: '/x',
      follow: false,
      draft: newDraft(),
    });
    const founding = {
      name: 'Docs',
      member_name: 'Alex',
      people: [
        { user: 'mina-u', name: 'Mina' },
        { user: 'leo-u', name: 'Leo' },
      ],
      ais: [ai('Claude'), ai('Claude 2')],
    };
    const steps = () => sent.map((fields) => [fields.t, fields.name]);
    const made = await found(home, founding);
    expect(made).toEqual({ g: sent[0].g });
    expect(steps()).toEqual([
      ['group', 'Docs'],
      ['member', 'Mina'],
      ['member', 'Leo'],
      ['host.join', 'Claude'],
      ['host.join', 'Claude 2'],
    ]);
    expect(sent.slice(1).map((fields) => fields.g)).toEqual(Array(4).fill(made.g));
    expect(sent[3].home).toBe(home.device);

    const err = { code: 'conflict', msg: 'taken' };
    for (const [stop, at, step, index] of [
      ['Leo', 3, 'people', 1],
      ['Claude', 4, 'ais', 0],
      ['Docs', 1, 'group', 0],
    ] as const) {
      sent.length = 0;
      refuse = (fields) => fields.name === stop;
      expect(await found(home, founding)).toEqual({
        g: sent[0].g,
        stopped: { step, index, err },
      });
      expect(sent).toHaveLength(at);
    }
  });
});

describe('the invitation code of a group', () => {
  const golden = JSON.parse(goldens['cmd-code']) as { code: string; expires: number };
  const open = { code: golden.code, expires: golden.expires };

  /** A home whose answers are given in turn; what it was sent, without `n` and `ts`. */
  function answering(...answers: object[]) {
    const home = makeHome();
    const sent: Record<string, unknown>[] = [];
    home.ctl = makeOnlineCtl(async (req) => {
      const { n: _n, ts: _ts, ...fields } = JSON.parse((req as { env: p.Envelope }).env.p);
      sent.push(fields);
      return { re: sent.length, ...answers[sent.length - 1] };
    });
    return { home, sent };
  }

  it('offers 1 hour, 24 hours and 7 days', () => expect(lifetimes).toEqual([1, 24, 168]));

  it('makes a code for its hours and answers it', async () => {
    const { home, sent } = answering(JSON.parse(goldens['cmd-code']));
    expect(await sendCode(home, gid, 'new', 168)).toEqual({ open });
    expect(sent).toEqual([{ t: 'code', g: gid, op: 'new', hours: 168 }]);
    expect(p.decode(p.Code, JSON.stringify({ ...sent[0], n: gid, ts: 1 }))).toBeTruthy();
  });

  it('shows the code a repeated new was answered without', async () => {
    const { home, sent } = answering({ ok: true }, { ok: true, ...open });
    expect(await sendCode(home, gid, 'new', 24)).toEqual({ open });
    expect(sent).toEqual([
      { t: 'code', g: gid, op: 'new', hours: 24 },
      { t: 'code', g: gid, op: 'show' },
    ]);
  });

  it.each(['off', 'show'] as const)('answers no code after %s with no field', async (op) => {
    const { home, sent } = answering({ ok: true });
    expect(await sendCode(home, gid, op)).toEqual({});
    expect(sent).toEqual([{ t: 'code', g: gid, op }]);
  });

  it('answers the refusal', async () => {
    const err = { code: 'forbidden', msg: 'not an admin' };
    const { home } = answering({ err }, { ok: true, ...open });
    expect(await sendCode(home, gid, 'new', 24)).toEqual({ err });
  });

  it('says until when a code is valid, and the invitation with its date and the code alone on its last line', () => {
    vi.useFakeTimers({ toFake: ['Date'] });
    try {
      const expires = new Date(2026, 10, 28, 18, 5).getTime();
      vi.setSystemTime(new Date(2026, 10, 28, 9, 0));
      prefs.lang = 'en';
      expect(codeUntil({ ...open, expires }, 'en')).toBe('Valid until 18:05');
      expect(invitationText('Web $& team', { ...open, expires }, 'en')).toBe(
        `Join the group "Web $& team" in SoloDawn: on the group list choose "Join a group" and enter this code (valid until Nov 28, 18:05):\n${open.code}`,
      );
      vi.setSystemTime(new Date(2026, 10, 27, 9, 0));
      expect(codeUntil({ ...open, expires }, 'en')).toBe('Valid until Nov 28, 18:05');
      prefs.lang = 'zh-CN';
      expect(codeUntil({ ...open, expires }, 'zh-CN')).toBe('11月28日 18:05 失效');
      expect(invitationText('网站', { ...open, expires }, 'zh-CN')).toBe(
        `用 SoloDawn 加入群「网站」：在群列表点「加入群」，输入下面的邀请码（11月28日 18:05 前有效）：\n${open.code}`,
      );
    } finally {
      prefs.lang = 'en';
      vi.useRealTimers();
    }
  });

  it('makes the code after the people and AIs of a new group, and says where it stopped', async () => {
    const created = { ok: true };
    const founding = { name: 'Docs', member_name: 'Alex', people: [], ais: [], hours: 1 };
    const made = answering(created, { ok: true, ...open });
    const result = await found(made.home, founding);
    expect(result).toEqual({ g: made.sent[0].g, code: open });
    expect(made.sent[1]).toEqual({ t: 'code', g: result.g, op: 'new', hours: 1 });
    const err = { code: 'internal', msg: 'disk' };
    const refused = answering(created, { err });
    const stopped = await found(refused.home, founding);
    expect(stopped).toEqual({ g: refused.sent[0].g, stopped: { step: 'code', index: 0, err } });
    const none = answering(created);
    expect(await found(none.home, { ...founding, hours: undefined })).toEqual({
      g: none.sent[0].g,
    });
    expect(none.sent).toHaveLength(1);
  });
});

describe('AIs that cannot be added now', () => {
  const candidate = (changes: Partial<Candidate> = {}): Candidate => ({
    key: 'dev:claude',
    home: makeHome(),
    device: 'dev',
    hostName: 'workstation',
    agent: 'claude',
    account: 'personal',
    loginOk: true,
    inGroup: false,
    ...changes,
  });

  it('say why: no account type, a computer away, a sign-in no longer valid', () => {
    expect(unavailable(candidate())).toBeUndefined();
    expect(unavailable(candidate({ agent: 'external', account: null }))).toBeUndefined();
    expect(unavailable(candidate({ account: null }))).toBe(
      'Choose an account type on the Computers page first',
    );
    expect(unavailable(candidate({ home: makeHome({ online: false }) }))).toBe(
      'Its computer cannot be reached now',
    );
    expect(unavailable(candidate({ loginOk: false }))).toBe(
      'Its sign-in is no longer valid: sign in again on its computer',
    );
  });

  it('are listed for a group still being made, none of them in it', () => {
    const home = makeHome();
    const info = p.decode(p.HostInfo, goldens['host-info-answer']);
    const listed = candidatesOf(undefined, [home], new Map([[home, info]]));
    expect(listed.map((each) => [each.agent, each.inGroup])).toEqual(
      info.agents.map((agent) => [agent.agent, false]),
    );
  });
});
