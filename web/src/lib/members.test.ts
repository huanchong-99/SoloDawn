import { beforeEach, describe, expect, it, vi } from 'vitest';
import { Group, Home } from './groups.svelte';
import {
  addDisabledReason,
  aiMembers,
  candidatesOf,
  canMakeSecretary,
  humanMembers,
  isAdmin,
  joinError,
  joinGroup,
  otherWaiter,
  pathLabel,
  personTags,
  selectableCandidates,
  statusOf,
  suggestedName,
  taskWaitKind,
  waitingOnMe,
  waitingOnOther,
} from './members';
import { goldens } from './mock';
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
  });

  it('checks isAdmin across owner, admin, regular member and missing me', () => {
    expect(isAdmin(makeGroup({ role: 'owner' }).group)).toBe(true);
    expect(isAdmin(makeGroup({ role: 'admin' }).group)).toBe(true);
    expect(isAdmin(makeGroup({ role: 'member' }).group)).toBe(false);
    expect(isAdmin(makeGroup({ role: 'none' }).group)).toBe(false);
  });

  it('returns the exact addDisabledReason for offline home, missing info, non-admin and no owned host', () => {
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
    expect(addDisabledReason(regularMember.group, [ownedOnline])).toBe(
      'Only an admin can add an AI.',
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
    expect(statusOf(group, mk('offline'))).toEqual({ st: 'offline', text: 'Offline' });
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
              caps: {},
              groups: [
                {
                  g: 'other-group',
                  member: 'm0',
                  level: 3,
                  repo_config: false,
                  state: 'dormant',
                  queued: 0,
                },
                {
                  g: gid,
                  member: 'm1',
                  level: 3,
                  repo_config: false,
                  state: 'working',
                  queued: 0,
                },
              ],
            },
            {
              agent: 'codex',
              cli_version: '0.1.0',
              account: null,
              login_ok: false,
              caps: {},
              groups: [],
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
              caps: {},
              groups: [],
            },
            {
              agent: 'claude',
              cli_version: '2.0.0',
              account: 'api_key',
              login_ok: false,
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
    expect(selectableCandidates(list).map((c) => c.key)).toEqual(['dev-2:claude']);
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

  it('words home_unreachable, conflict and lost in joinError and leaves others to Failure', () => {
    expect(joinError('home_unreachable')).toBe(
      "The AI's computer cannot reach this group's home right now. Try again when both are online.",
    );
    expect(joinError('conflict')).toBe('That name is already used in this group.');
    expect(joinError('lost')).toBe("Not sent: the AI's computer cannot be reached.");
    expect(joinError('forbidden')).toBeUndefined();
  });

  it('signs host.join in joinGroup and handles missing target, offline target, refusal and rejection', async () => {
    const { group } = makeGroup({ secretary: undefined });
    const candidate = {
      key: 'dev-2:codex',
      home: makeHome(),
      device: 'dev-2',
      hostName: 'laptop',
      agent: 'codex',
      account: 'personal',
      loginOk: true,
      inGroup: false,
    };

    expect(await joinGroup(undefined, group, candidate, 'Codex', false)).toBe('lost');
    expect(await joinGroup(makeHome({ online: false }), group, candidate, 'Codex', false)).toBe(
      'lost',
    );

    const sent: unknown[] = [];
    const target = makeHome();
    vi.spyOn(Date, 'now').mockReturnValue(1763600000123);
    target.ctl = makeOnlineCtl(async (req) => {
      sent.push(req);
      return { re: 1, ok: true };
    });

    expect(await joinGroup(target, group, candidate, 'Lead', true)).toBe('');
    expect(sent).toHaveLength(1);
    const frame = sent[0] as { t: string; env: p.Envelope };
    expect(frame.t).toBe('cmd');
    expect(frame.env.k).toBe('dev-k');
    expect(JSON.parse(frame.env.p)).toEqual({
      t: 'host.join',
      n: expect.stringMatching(/^[A-Za-z0-9_-]{21}[AQgw]$/),
      ts: 1763600000123,
      to: 'dev-2',
      g: gid,
      home: homeDev,
      agent: 'codex',
      name: 'Lead',
      secretary: true,
    });

    target.ctl = makeOnlineCtl(async () => ({
      re: 2,
      err: { code: 'home_unreachable', msg: 'timeout' },
    }));
    expect(await joinGroup(target, group, candidate, 'Lead', false)).toBe('home_unreachable');

    target.ctl = makeOnlineCtl(async () => {
      throw new Error('closed');
    });
    expect(await joinGroup(target, group, candidate, 'Lead', false)).toBe('lost');
    vi.restoreAllMocks();
  });
});
