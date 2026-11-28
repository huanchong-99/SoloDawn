import { flushSync, tick } from 'svelte';
import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import { all, closePage, fill, one, show, texts } from '../../../features/page';
import { Group, Home, homes } from '../../lib/groups.svelte';
import { goldens } from '../../lib/mock';
import { nodes } from '../../lib/nodes.svelte';
import { prefs } from '../../lib/prefs.svelte';
import * as p from '../../lib/proto';
import AddAI from './AddAI.svelte';
import MemberPanel from './MemberPanel.svelte';

const human = p.decode(p.HumanMemberRow, goldens['row-member-human']);
const golden = p.decode(p.AIMemberRow, goldens['row-member-ai']);

/** A group's panel, read by its owner or, with `role`, by another person of that role, who owns
 *  none of the AIs. */
function setup(changes: Partial<p.AIMemberRow> = {}, role?: string, hostHome?: (h: Home) => void) {
  const home = new Home({
    user: role ? 'reader' : human.user,
    sign: async (payload) => ({ p: payload, k: 'key', s: 'signature' }),
  });
  hostHome?.(home);
  const group = new Group(home, human.g);
  const ai: p.AIMemberRow = { ...golden, user: human.user, secretary: false, ...changes };
  const reader = { ...human, id: 'reader', user: 'reader', name: 'Reader', role: role ?? 'member' };
  group.apply([p.decode(p.GroupRow, goldens['row-group']), human, reader, ai]);
  home.groups = [group];
  const base = p.decode(p.HostInfoPush, goldens['host-info-push']);
  const expiredInfo: p.HostInfoPush = {
    ...base,
    offline: false,
    agents: base.agents.map((a) => (a.agent === 'claude' ? { ...a, login_ok: false } : a)),
  };
  const request = vi.fn(async (req: { t: string; env: p.Envelope }): Promise<any> =>
    req.t === 'host.info' ? { re: 1, ok: true, ...expiredInfo } : { re: 1, ok: true },
  );
  home.ctl = { request: request as unknown as p.Ctl['request'] };
  show(MemberPanel, {
    group,
    net: { source: 'mock', homes: hostHome ? [home] : [], path: () => undefined },
  });
  return { group, ai, home, request, expiredInfo };
}

const removal = () =>
  all<HTMLButtonElement>('.member .card-foot button').find((each) =>
    ['Remove AI', 'Confirm removal'].includes(each.textContent!.trim()),
  )!;

async function press() {
  removal().click();
  await tick();
}

beforeEach(() => {
  prefs.lang = 'en';
});
afterEach(async () => {
  nodes.infos.clear();
  await closePage();
});

it.each(['claude', 'codex', 'opencode', 'qwen', 'external'])(
  'removes a %s AI after a second press by its owner',
  async (cli) => {
    const { group, ai, request } = setup({ cli });
    await press();
    expect(removal().textContent!.trim()).toBe('Confirm removal');
    expect(removal().classList.contains('btn-danger')).toBe(true);
    expect(texts('.card-foot .why')).toEqual([]);
    await press();
    await vi.waitFor(() => expect(request).toHaveBeenCalledTimes(1));
    const { n: _n, ts: _ts, ...command } = JSON.parse(request.mock.calls[0][0].env.p);
    expect(command).toEqual({ t: 'member', g: group.g, op: 'remove', id: ai.id });
    expect(removal().textContent!.trim()).toBe('Remove AI');
  },
);

it('lets an admin remove the AI of another person, and no other reader', async () => {
  const { group, request } = setup({ state: 'working' }, 'admin');
  expect(texts('.card-foot button')).toEqual(['Remove AI']);
  await press();
  await press();
  await vi.waitFor(() => expect(request).toHaveBeenCalledTimes(1));
  group.apply([{ ...human, id: 'reader', user: 'reader', role: 'member', seq: 10000 }]);
  flushSync();
  expect(all('.card-foot')).toEqual([]);
});

it('takes the first press back on Escape, and is off while the home cannot be reached', async () => {
  const { home, request } = setup();
  const escape = () =>
    new KeyboardEvent('keydown', { key: 'Escape', bubbles: true, cancelable: true });
  const idle = escape();
  removal().dispatchEvent(idle);
  expect(idle.defaultPrevented).toBe(false);
  await press();
  removal().dispatchEvent(new KeyboardEvent('keydown', { key: 'a', bubbles: true }));
  await tick();
  expect(removal().textContent!.trim()).toBe('Confirm removal');
  const back = escape();
  removal().dispatchEvent(back);
  await tick();
  expect(back.defaultPrevented).toBe(true);
  expect(removal().textContent!.trim()).toBe('Remove AI');
  expect(request).not.toHaveBeenCalled();
  home.ctl = undefined;
  flushSync();
  expect(removal().disabled).toBe(true);
});

it('says on the first press that an AI on unfinished tasks cannot be removed, and which', async () => {
  const { group, ai, request } = setup({ name: '$& Codex' });
  const task = p.decode(p.TaskRow, goldens['row-task']);
  const on = (id: string, num: number) => ({ ...task, id, num, assignee: ai.id, seq: 20000 + num });
  group.apply([on('t-a', 12), on('t-b', 14)]);
  await press();
  await press();
  expect(removal().textContent!.trim()).toBe('Remove AI');
  expect(texts('.card-foot [role=alert]')).toEqual([
    '$& Codex is on T-12 and T-14. Reassign or cancel that work first.',
  ]);
  expect(one('.card-foot .why').id).toBe(removal().getAttribute('aria-describedby'));
  expect(request).not.toHaveBeenCalled();
  // Once the tasks are over the same place asks for the second press.
  group.apply([
    { ...on('t-a', 12), state: 'done', seq: 30000 },
    { ...on('t-b', 14), state: 'cancelled', seq: 30001 },
  ]);
  flushSync();
  expect(removal().textContent!.trim()).toBe('Confirm removal');
});

it('says what removing the secretary means, and shows a refusal of the home until the next try', async () => {
  const { request } = setup({ secretary: true });
  await press();
  expect(texts('.card-foot .why')).toEqual([
    'The group will have no secretary until you choose another one.',
  ]);
  request.mockResolvedValueOnce({ re: 1, err: { code: 'forbidden', msg: '' } });
  await press();
  await vi.waitFor(() =>
    expect(texts('.card-foot [role=alert]')).toEqual(['Not sent: this device may not do that.']),
  );
  await press();
  expect(texts('.card-foot [role=alert]')).toEqual([]);
});

it('shows the turns of today as plain text and a share of the limit as a meter', () => {
  const { group, ai } = setup({ usage: { turns_today: 14 }, last_seen: Date.now() });
  expect(texts('.member .meter')).toEqual(['Turns today14']);
  expect(all('.member [role=meter]')).toEqual([]);
  group.apply([{ ...ai, usage: { turns_today: 15, window_pct: 40 }, seq: ai.seq + 1 }]);
  flushSync();
  expect(texts('.member [role=meter]')).toEqual(['Quota used this window40%']);
});

it('prompts the owner to re-sign in when a limited AI has login_ok false on its online computer', async () => {
  const { home, ai, expiredInfo } = setup(
    {
      cli: 'claude',
      state: 'limited',
      host: golden.host,
      host_name: 'Studio',
    },
    undefined,
    (h) => {
      h.device = golden.host;
      h.owner = true;
    },
  );
  nodes.infos.set(home, expiredInfo);
  await vi.waitFor(() => {
    flushSync();
    expect(one('.member p.st').textContent!.trim()).toBe('Limited · sign-in expired');
  });
  const note = one('.member .note[data-tone="danger"]');
  expect(note.textContent).toContain('Claude Code needs to sign in again.');
  expect(note.textContent).toContain('Run it directly on Studio');
  expect(note.querySelector('a')?.getAttribute('href')).toBe('#/nodes');
  expect(home.device).toBe(golden.host);
  expect(ai.cli).toBe('claude');
});

async function pickFolderInAddAI(path: string) {
  one<HTMLButtonElement>('dialog .path-row .change').click();
  flushSync();
  await vi.waitFor(() => expect(one<HTMLDialogElement>('dialog dialog[open]')).not.toBeNull());
  const picker = one<HTMLDialogElement>('dialog dialog[open]');
  fill(picker.querySelector<HTMLInputElement>('input.mono')!, path);
  flushSync();
  picker.querySelector<HTMLButtonElement>('footer .btn-primary')!.click();
  flushSync();
}

function setupAddAIGroup() {
  const home = new Home({
    user: human.user,
    sign: async (payload) => ({ p: payload, k: 'key', s: 'sig' }),
  });
  home.owner = true;
  home.device = 'dev-ws';
  home.name = 'Workstation';
  const group = new Group(home, human.g);
  const infoRow = p.decode(p.GroupRow, goldens['row-group']);
  delete infoRow.secretary;
  group.apply([infoRow, human]);
  home.groups = [group];
  return { group, home, infoRow };
}

function setupAddAINode(opts: {
  user: string;
  device: string;
  name: string;
  agents: p.HostInfoPush['agents'];
  request?: ReturnType<typeof vi.fn>;
}) {
  const home = new Home({
    user: opts.user,
    sign: async (payload) => ({ p: payload, k: 'key', s: 'sig' }),
  });
  home.owner = true;
  home.device = opts.device;
  home.name = opts.name;
  const base = p.decode(p.HostInfoPush, goldens['host-info-push']);
  const info: p.HostInfoPush = {
    ...base,
    device: opts.device,
    name: opts.name,
    agents: opts.agents,
  };
  const folders = p.decode(p.HostWorkspaces, goldens['host-workspaces-answer']);
  const request =
    opts.request ??
    vi.fn(async (req: { t: string }) =>
      req.t === 'host.workspaces' ? folders : { ...info, re: 1, ok: true },
    );
  home.ctl = { request: request as unknown as p.Ctl['request'] };
  return { home, info, request };
}

it('defaults AddAI folder from nodes.infos, resets across computers, and keeps manual choice on same computer', async () => {
  const { group, home: home1, infoRow } = setupAddAIGroup();
  const base = p.decode(p.HostInfoPush, goldens['host-info-push']);
  const folders = p.decode(p.HostWorkspaces, goldens['host-workspaces-answer']);
  const hostEntry = (member: string, workspace: string) => ({
    g: group.g,
    member,
    workspace,
    level: 3,
    state: 'dormant',
    queued: 0,
    watchers: [],
  });

  // Home 1 has a secretary in group (codex) and a worker (claude); qwen is not in group yet.
  group.apply([
    { ...infoRow, seq: 100, secretary: 'sec-ws' },
    { ...golden, id: 'wrk-ws', seq: 101, cli: 'claude', host: 'dev-ws', secretary: false },
    { ...golden, id: 'sec-ws', seq: 102, cli: 'codex', host: 'dev-ws', secretary: true },
  ]);
  const info1: p.HostInfoPush = {
    ...base,
    device: 'dev-ws',
    name: 'Workstation',
    agents: [
      { ...base.agents[0], agent: 'qwen', account: 'personal', groups: [] },
      {
        ...base.agents[0],
        agent: 'claude',
        account: 'personal',
        groups: [hostEntry('wrk-ws', '/srv/ws-worker')],
      },
      {
        ...base.agents[1],
        agent: 'codex',
        account: 'api_key',
        groups: [hostEntry('sec-ws', '/srv/ws-sec')],
      },
    ],
  };
  home1.ctl = {
    request: vi.fn(async (req: { t: string }) =>
      req.t === 'host.workspaces' ? folders : { ...info1, re: 1, ok: true },
    ) as unknown as p.Ctl['request'],
  };

  // Home 2 has two CLIs and an existing group workspace '/srv/laptop'.
  const { home: home2, info: info2 } = setupAddAINode({
    user: human.user,
    device: 'dev-lap',
    name: 'Laptop',
    agents: [
      {
        ...base.agents[0],
        agent: 'claude',
        account: 'personal',
        groups: [hostEntry('wrk-lap', '/srv/laptop')],
      },
      { ...base.agents[1], agent: 'codex', account: 'api_key', groups: [] },
    ],
  });
  homes.push(home1, home2);
  nodes.infos.set(home1, info1);
  nodes.infos.set(home2, info2);

  show(AddAI, { group, open: true });
  const radios = () => all<HTMLInputElement>('dialog input[name="candidate"]');
  const pathCode = () => one('dialog .path-row code');
  const folderBtn = () => one<HTMLButtonElement>('dialog .path-row .change');
  const submitBtn = () => one<HTMLButtonElement>('dialog > form > footer .btn-primary');

  await vi.waitFor(() => expect(radios()[0].checked).toBe(true));
  // Prefers the secretary's workspace on Workstation and offers "Change".
  expect(pathCode().textContent?.trim()).toBe('/srv/ws-sec');
  expect(folderBtn().textContent?.trim()).toBe('Change');
  expect(submitBtn().disabled).toBe(false);
  expect(submitBtn().hasAttribute('aria-describedby')).toBe(false);

  // Switching to another computer picks that computer's default folder.
  radios()[3].click();
  flushSync();
  expect(pathCode().textContent?.trim()).toBe('/srv/laptop');

  // Manually choosing a folder is preserved when switching CLI on the same computer and on nodes.infos updates.
  await pickFolderInAddAI('/srv/manual-lap');
  expect(pathCode().textContent?.trim()).toBe('/srv/manual-lap');
  radios()[4].click();
  flushSync();
  expect(pathCode().textContent?.trim()).toBe('/srv/manual-lap');
  nodes.infos.set(home2, {
    ...info2,
    agents: info2.agents.map((a) =>
      a.agent === 'codex' ? { ...a, groups: [hostEntry('other', '/srv/lap-new')] } : a,
    ),
  });
  flushSync();
  expect(pathCode().textContent?.trim()).toBe('/srv/manual-lap');

  // Switching back to Workstation resets to Workstation's default folder.
  radios()[0].click();
  flushSync();
  expect(pathCode().textContent?.trim()).toBe('/srv/ws-sec');
});

it('fills AddAI folder when nodes.infos arrives after opening unless user chose one, and explains disabled submit', async () => {
  const { group, home } = setupAddAIGroup();
  const base = p.decode(p.HostInfoPush, goldens['host-info-push']);
  const folders = p.decode(p.HostWorkspaces, goldens['host-workspaces-answer']);
  const initialInfo: p.HostInfoPush = {
    ...base,
    device: 'dev-ws',
    name: 'Workstation',
    agents: [{ ...base.agents[0], agent: 'claude', account: 'personal', groups: [] }],
  };
  let resolveJoin!: (value: unknown) => void;
  const request = vi.fn((req: { t: string }) => {
    if (req.t === 'host.workspaces') return Promise.resolve(folders);
    if (req.t === 'host.info') return Promise.resolve({ ...initialInfo, re: 1, ok: true });
    return new Promise((resolve) => {
      resolveJoin = resolve;
    });
  });
  home.ctl = { request: request as unknown as p.Ctl['request'] };
  homes.push(home);
  nodes.infos.set(home, initialInfo);

  show(AddAI, { group, open: true });
  const submitBtn = () => one<HTMLButtonElement>('dialog > form > footer .btn-primary');
  const whyEl = () => document.querySelector('dialog > form > p.hint');
  const whyText = () => {
    const id = submitBtn().getAttribute('aria-describedby');
    return id ? document.getElementById(id)?.textContent?.trim() : undefined;
  };
  await vi.waitFor(() =>
    expect(one<HTMLInputElement>('dialog input[name="candidate"]').checked).toBe(true),
  );

  // Missing folder reason.
  expect(submitBtn().disabled).toBe(true);
  expect(whyText()).toBe('Choose a folder for Claude Code first');
  expect(whyEl()?.textContent?.trim()).toBe('Choose a folder for Claude Code first');

  // Delayed nodes.infos arrival fills the group's folder automatically.
  nodes.infos.set(home, {
    ...initialInfo,
    agents: [
      {
        ...initialInfo.agents[0],
        groups: [
          {
            g: group.g,
            member: 'm1',
            workspace: '/srv/delayed',
            level: 3,
            state: 'dormant',
            queued: 0,
            watchers: [],
          },
        ],
      },
    ],
  });
  flushSync();
  expect(one('dialog .path-row code').textContent?.trim()).toBe('/srv/delayed');
  expect(submitBtn().disabled).toBe(false);
  expect(submitBtn().hasAttribute('aria-describedby')).toBe(false);
  expect(whyEl()).toBeNull();

  // Empty name reason.
  const nameInput = one<HTMLInputElement>('dialog label.field input.input');
  fill(nameInput, '   ');
  flushSync();
  expect(submitBtn().disabled).toBe(true);
  expect(whyText()).toBe('Enter a name for Claude Code first');

  // Skills too long reason (when follow switch is off).
  fill(nameInput, 'Reviewer');
  flushSync();
  one<HTMLButtonElement>('dialog [role="switch"]').click();
  flushSync();
  const skillsInput = one<HTMLInputElement>('dialog .custom-fields > label.field input.input');
  fill(skillsInput, 'x'.repeat(201));
  flushSync();
  expect(submitBtn().disabled).toBe(true);
  expect(whyText()).toBe('Shorten what Reviewer is good at first');

  // Fixing skills enables submit and clears the hint.
  fill(skillsInput, 'x'.repeat(200));
  flushSync();
  expect(submitBtn().disabled).toBe(false);
  expect(submitBtn().hasAttribute('aria-describedby')).toBe(false);
  expect(whyEl()).toBeNull();

  // While busy submitting, button is disabled without showing any hint.
  one('dialog form').dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }));
  flushSync();
  expect(submitBtn().disabled).toBe(true);
  expect(submitBtn().hasAttribute('aria-describedby')).toBe(false);
  expect(whyEl()).toBeNull();
  await vi.waitFor(() => expect(resolveJoin).toBeTypeOf('function'));
  resolveJoin({ re: 1, ok: true });
  await vi.waitFor(() => expect(one<HTMLDialogElement>('dialog').open).toBe(false));
});
