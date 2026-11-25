import { flushSync, tick } from 'svelte';
import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import { all, closePage, one, show, texts } from '../../../features/page';
import { Group, Home, homes } from '../../lib/groups.svelte';
import { goldens } from '../../lib/mock';
import { mounts } from '../../lib/mounts.svelte';
import { nodes } from '../../lib/nodes.svelte';
import { prefs } from '../../lib/prefs.svelte';
import * as p from '../../lib/proto';
import MemberModel from '../model/MemberModel.svelte';
import { newDraft } from '../profile/profile';
import AddExternal from './AddExternal.svelte';
import AfterJoin from './AfterJoin.svelte';
import MemberExternal from './MemberExternal.svelte';
import SetupDialog from './SetupDialog.svelte';
import { checkReplies, checkText, latestCheck, serverEntry, setupPrompt } from './setup';

const member = p.decode(p.AIMemberRow, goldens['row-member-ai-external']);
const person = p.decode(p.HumanMemberRow, goldens['row-member-human']);
const baseMsg = p.decode(p.MsgRow, goldens['row-msg-text']);
const hostInfo = p.decode(p.HostInfoPush, goldens['host-info-push']);
const exe = 'C:\\Program Files\\SoloDawn\\solodawn.exe';
const owner = { ...person, user: member.user };
const message = (pos: number, changes: Partial<p.MsgRow> = {}) =>
  ({
    ...baseMsg,
    id: `msg-${pos}`,
    pos,
    seq: pos,
    text: checkText,
    by: owner.id,
    mentions: [member.id],
    ...changes,
  }) as p.MsgRow;

let group: Group;
let request: ReturnType<typeof vi.fn>;
let candidate: {
  key: string;
  home: Home;
  device: string;
  hostName: string;
  agent: string;
  account: null;
  loginOk: boolean;
  inGroup: boolean;
};

beforeEach(() => {
  prefs.lang = 'en';
  const home = new Home({ user: owner.user, sign: async (p) => ({ p, k: 'key', s: 'signature' }) });
  group = new Group(home, member.g);
  group.name = 'Website';
  group.apply([{ ...owner, g: member.g }]);
  request = vi.fn(async () => ({ re: 1, ok: true }));
  home.ctl = { request: request as p.Ctl['request'] };
  home.device = member.host;
  candidate = {
    key: 'node:external',
    home,
    device: member.host,
    hostName: member.host_name,
    agent: 'external',
    account: null,
    loginOk: true,
    inGroup: false,
  };
});

afterEach(async () => {
  nodes.infos.clear();
  await closePage();
});

it('mounts the external fields, card, and setup at the member points', () => {
  expect(mounts['member.add']).toContainEqual({
    area: 'external',
    order: 5,
    component: AddExternal,
  });
  expect(mounts['member.ai']).toContainEqual({
    area: 'external',
    order: 5,
    component: MemberExternal,
  });
  expect(mounts['member.joined']).toEqual([{ area: 'external', order: 10, component: AfterJoin }]);
});

it('fills the prompt literally and writes a standalone MCP server entry', () => {
  const result = setupPrompt({ group: 'Website $& {name}', name: 'Scout', member: member.id, exe });
  expect(
    result.startsWith(
      'You are joining the SoloDawn group "Website $& {name}" as the AI member "Scout".\n',
    ),
  ).toBe(true);
  expect(result).toContain(
    `SoloDawn runs on this computer. Its program is: ${exe}\nYour member id is: ${member.id}\n`,
  );
  expect(result).toContain(`/tmp/solodawn-${member.id}.lock /bin/sh /path/to/runner.sh`);
  expect(result).toContain(`/TN "SoloDawn ${member.id}"`);
  expect(JSON.parse(serverEntry(exe, member.id))).toEqual({
    solodawn: { command: exe, args: ['mcp', '--member', member.id] },
  });
});

it('finds the latest check from its sender and counts only later main-line text by this member', () => {
  const first = message(1);
  const last = message(2);
  const reply = message(3, { by: member.id, thread: null, text: 'Ready' });
  const rows = [
    message(0, { by: member.id }),
    first,
    last,
    message(last.pos, { id: 'same-position', by: member.id }),
    reply,
    message(4, { by: 'another-owner' }),
    message(5, { text: 'Ordinary post' }),
    message(6, { mentions: ['another-ai'] }),
    message(7, { kind: 'question' }),
    message(8, { by: member.id, thread: 'task' }),
    message(9, { by: member.id, kind: 'progress' }),
  ];
  expect(latestCheck(rows, undefined, member.id)).toBeUndefined();
  expect(latestCheck(rows, owner.id, member.id)).toBe(last);
  expect(checkReplies(rows, last, member.id)).toEqual([reply]);
});

it('shows the account and optional tool fields only for an external candidate', () => {
  const draft = newDraft();
  show(AddExternal, { group, candidate: undefined, draft });
  expect(all('select, input')).toEqual([]);
  show(AddExternal, { group, candidate, draft });
  const select = one<HTMLSelectElement>('select');
  expect(select.required).toBe(true);
  expect([...select.options].map((option) => option.value)).toEqual([
    '',
    'personal',
    'team',
    'api_key',
  ]);
  const input = one<HTMLInputElement>('input');
  expect(input.required).toBe(false);
  expect(input.pattern).toBe('[^\\p{Cc}]{0,32}');
  select.value = 'personal';
  select.dispatchEvent(new Event('change'));
  input.value = 'ZCode';
  input.dispatchEvent(new Event('input'));
  expect(draft).toMatchObject({ account: 'personal', tool: 'ZCode' });
});

it('waits for the added member row, opens once and posts the check for its exact id', async () => {
  const close = vi.fn();
  show(AfterJoin, { group, candidate, name: member.name });
  expect(all('dialog')).toEqual([]);
  group.apply([{ ...member, name: 'Someone else' }]);
  flushSync();
  expect(all('dialog')).toEqual([]);
  group.apply([{ ...member, seq: member.seq + 1 }]);
  await vi.waitFor(() => expect(request).toHaveBeenCalledTimes(1));
  const [frame] = request.mock.calls[0] as unknown as [{ t: string; env: p.Envelope }];
  expect(frame.t).toBe('cmd');
  expect(JSON.parse(frame.env.p)).toEqual({
    t: 'post',
    n: expect.any(String),
    ts: expect.any(Number),
    g: member.g,
    kind: 'text',
    thread: null,
    main: false,
    text: 'Setup check: answer here in one line, then finish.',
    mentions: [member.id],
    re: null,
    body: {},
  });
  expect(one<HTMLDialogElement>('dialog').open).toBe(true);
  expect(texts('dialog .hint')).toEqual([
    'Connect to the AI’s computer to load the setup prompt and MCP entry.',
  ]);
  one('dialog header button').click();
  await vi.waitFor(() => expect(all('dialog')).toHaveLength(0));
  show(SetupDialog, { group, ai: member, onclose: close });
  await tick();
  expect(request).toHaveBeenCalledTimes(1);
});

it('renders live progress, reports duplicate responses and copies the complete prompt', async () => {
  homes.push(group.home);
  nodes.infos.set(group.home, { ...hostInfo, exe });
  const writeText = vi
    .fn()
    .mockResolvedValueOnce(undefined)
    .mockRejectedValueOnce(new Error('blocked'));
  vi.spyOn(navigator, 'clipboard', 'get').mockReturnValue({ writeText } as unknown as Clipboard);
  const check = message(member.seq + 1);
  group.apply([{ ...member, state: 'offline' }, check]);
  show(SetupDialog, {
    group,
    get ai() {
      return group.member(member.id) as p.AIMemberRow;
    },
    onclose: vi.fn(),
  });
  const done = () => texts('dialog li.done > span:nth-child(2)');
  const update = (seq: number, state: string) => {
    group.apply([{ ...member, seq, state }]);
    flushSync();
  };
  expect(texts('dialog .st')).toEqual(['Offline']);
  expect(done()).toEqual([]);
  update(check.seq + 1, 'waiting');
  expect(done()).toEqual(['Runner seen']);
  update(check.seq + 2, 'working');
  expect(done()).toEqual(['Runner seen', 'Item taken']);
  group.apply([message(check.seq + 3, { by: member.id, thread: null })]);
  flushSync();
  expect(done()).toEqual(['Runner seen', 'Item taken', 'Member answered']);
  update(check.seq + 4, 'waiting');
  expect(done()).toEqual(['Runner seen', 'Item taken', 'Member answered', 'Run ended']);
  update(check.seq + 5, 'paused');
  expect(texts('dialog .st')).toEqual(['Paused']);
  group.apply([message(check.seq + 6, { by: member.id, thread: null })]);
  flushSync();
  expect(done()).toEqual(['Runner seen', 'Item taken']);
  expect(texts('dialog p[role=status]')).toEqual([
    'The member posted more than one response. The check expects one.',
  ]);

  one('dialog .btn-primary').click();
  const prompt = one<HTMLTextAreaElement>('dialog textarea').value;
  await vi.waitFor(() => expect(writeText).toHaveBeenCalledExactlyOnceWith(prompt));
  await vi.waitFor(() => expect(texts('dialog p[role=status]')).toContain('Prompt copied.'));
  one('dialog .btn-primary').click();
  await vi.waitFor(() =>
    expect(texts('dialog p[role=status]')).toContain(
      'Could not copy. Select the prompt and copy it manually.',
    ),
  );
  request.mockResolvedValueOnce({ re: 1, err: { code: 'forbidden' } });
  one('dialog footer button').click();
  await vi.waitFor(() =>
    expect(texts('dialog [role=alert]')).toEqual(['Not sent: this device may not do that.']),
  );
});

it('uses the unset profile label for external AI and keeps the default for managed AI', () => {
  show(MemberModel, { group, ai: { ...member, model: '', effort: '' } });
  show(MemberModel, { group, ai: { ...member, cli: 'codex', model: '', effort: '' } });
  expect(texts('dd')).toEqual([
    'Not set',
    'Not set',
    "The CLI's default Change",
    "The CLI's default",
  ]);
});

it('keeps setup with the owner', async () => {
  show(MemberExternal, { group, ai: { ...member, cli: 'codex' } });
  expect(all('dt, dd')).toEqual([]);
  show(MemberExternal, { group, ai: { ...member, user: 'other-user', tool: undefined } });
  expect(texts('dd')).toEqual(['External AI']);
  expect(all('button')).toEqual([]);
  show(MemberExternal, { group, ai: member });
  expect(texts('dd.tool')).toEqual(['External AI', 'External AI · ZCode']);
  expect(texts('.actions button')).toEqual(['Setup']);
  one('.actions button').click();
  await tick();
  expect(one<HTMLDialogElement>('dialog').open).toBe(true);
  one('dialog header button').click();
  await vi.waitFor(() => expect(all('dialog')).toHaveLength(0));
});
