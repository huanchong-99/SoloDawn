import { flushSync } from 'svelte';
import { SvelteMap } from 'svelte/reactivity';
import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import { all, choose, closePage, fill, one, show } from '../../../features/page';
import { Group, Home, homes } from '../../lib/groups.svelte';
import { nodes } from '../../lib/nodes.svelte';
import { operations } from '../forge/templates';
import { goldens } from '../../lib/mock';
import { prefs } from '../../lib/prefs.svelte';
import * as p from '../../lib/proto';
import EditGroup from './EditGroup.svelte';
import GroupDetails from './GroupDetails.svelte';
import SecretaryWarning from './SecretaryWarning.svelte';
import AddAI from '../member/AddAI.svelte';
import InvitationDialog from '../people/InvitationDialog.svelte';
import mounts from './mounts';
import { homeOwner, secretaryWarning } from './trust';

beforeEach(() => {
  prefs.lang = 'en';
});
afterEach(closePage);

const warning =
  "Every request to this personal-subscription secretary from someone other than its owner waits for the owner's confirmation, each time. With a team plan or an API key, the owner can list who may direct it.";

it.each([
  ['personal', [], true],
  ['personal', ['owner'], true],
  ['personal', ['owner', 'reader'], false],
  ['team', ['owner'], false],
  ['api_key', [], false],
])('warns before inviting another person for %s and %j', (account, people, expected) => {
  expect(secretaryWarning(account, 'owner', people, true)).toBe(expected);
  show(SecretaryWarning, { account, owner: 'owner', people, inviting: true });
  expect(document.querySelector('.note')?.textContent ?? '').toBe(expected ? warning : '');
});

it('warns in the invitation dialog before the first other person joins, not afterward', () => {
  const { group, info, ai, person } = setup();
  show(InvitationDialog, { group, open: true });
  expect(document.querySelector('dialog header .note')).toBeNull();
  group.apply([
    { ...info, seq: 100, secretary: ai.id },
    { ...ai, seq: 101, account: 'personal' },
  ]);
  flushSync();
  expect(one('dialog header .note').textContent).toBe(warning);
  group.apply([{ ...person, id: 'reader', user: 'reader', name: 'Reader', seq: 102 }]);
  flushSync();
  expect(document.querySelector('dialog header .note')).toBeNull();
});

it('discloses project commands and the selected personal secretary warning in Add AI', async () => {
  const { group, home, info, person, request } = setup();
  home.owner = true;
  homes.push(home);
  const hostInfo = p.decode(p.HostInfoPush, goldens['host-info-push']);
  hostInfo.agents = [
    { ...hostInfo.agents[0], agent: 'other-cli', account: 'personal', groups: [] },
  ];
  nodes.infos.set(home, hostInfo);
  request.mockResolvedValue({ ...hostInfo, re: 1, ok: true });
  group.apply([{ ...person, id: 'reader', user: 'reader', name: 'Reader', seq: 102 }]);
  show(AddAI, { group, open: true });
  await vi.waitFor(() =>
    expect(one<HTMLInputElement>('dialog input[type=radio]').checked).toBe(true),
  );
  expect(one('dialog details summary').textContent).toBe('Group commands');
  expect(all('dialog .arguments code').map((el) => el.textContent)).toEqual(
    operations.flatMap((op) => info.templates![op]),
  );
  expect(document.querySelector('dialog .sec-opt .note')).toBeNull();
  one<HTMLInputElement>('dialog input[type=checkbox]').click();
  flushSync();
  expect(one('dialog .sec-opt .note').textContent).toBe(warning);
  expect(one<HTMLButtonElement>('dialog footer .btn-primary').disabled).toBe(false);
  one<HTMLInputElement>('dialog input[type=checkbox]').click();
  flushSync();
  expect(document.querySelector('dialog .sec-opt .note')).toBeNull();
  expect(one('dialog .av').textContent?.trim()).toBe('O');
  fill(one<HTMLInputElement>('dialog input.input'), '   ');
  one('dialog form').dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }));
  expect(request.mock.calls.map(([req]) => req.t)).toEqual(['host.info']);
  fill(one<HTMLInputElement>('dialog input.input'), 'Project AI');
  request.mockResolvedValueOnce({ re: 1, err: { code: 'forbidden', msg: 'refused' } });
  one('dialog form').dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }));
  await vi.waitFor(() =>
    expect(one('[role=alert]')?.textContent?.trim()).toBe('Not sent: this device may not do that.'),
  );
  group.apply([{ ...info, kind: 'team', seq: 103 }]);
  flushSync();
  expect(document.querySelector('dialog details')).toBeNull();
});

it('closes Add AI when the parent withdraws the dialog', () => {
  const { group } = setup();
  const state = new SvelteMap([['open', true]]);
  show(AddAI, {
    group,
    get open() {
      return state.get('open')!;
    },
  });
  expect(one<HTMLDialogElement>('dialog').open).toBe(true);
  one('dialog form').dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }));
  expect(one<HTMLDialogElement>('dialog').open).toBe(true);
  state.set('open', false);
  flushSync();
  expect(one<HTMLDialogElement>('dialog').open).toBe(false);
});

function setup(forge = 'custom') {
  const person = p.decode(p.HumanMemberRow, goldens['row-member-human']);
  const home = new Home({
    user: person.user,
    sign: async (payload) => ({ p: payload, k: 'key', s: 'signature' }),
  });
  const group = new Group(home, person.g);
  const info = { ...p.decode(p.GroupRow, goldens['row-group-custom']), forge };
  delete info.secretary;
  const ai = p.decode(p.AIMemberRow, goldens['row-member-ai']);
  group.apply([info, person, ai]);
  home.groups = [group];
  home.name = 'Group computer';
  home.device = info.home;
  const request = vi.fn().mockResolvedValue({ re: 1, ok: true });
  home.ctl = { request: request as p.Ctl['request'] };
  return { group, home, info, ai, person, request };
}

it('mounts group details immediately before project memory', () => {
  expect(mounts['chat.side'][0]).toEqual({ order: 19, component: GroupDetails });
  expect(mounts['chat.side'][1].order).toBe(20);
});

it('attributes an owned home to the viewer and a foreign home only to a matching live device row', () => {
  const { home, group } = setup();
  home.owner = true;
  expect(homeOwner(group)).toBe(home.me.user);
  home.owner = false;
  expect(homeOwner(group)).toBeUndefined();
  const device = p.decode(p.DeviceRow, goldens['row-device']);
  group.apply([
    { r: 'device', id: 'gone', seq: 41, gone: true },
    { ...device, id: 'other', dev: 'different' },
  ]);
  expect(homeOwner(group)).toBeUndefined();
  group.apply([device]);
  expect(homeOwner(group)).toBe(device.user);
});

it.each([
  ['personal', [], false],
  ['personal', ['owner'], false],
  ['personal', ['owner', 'reader'], true],
  ['api', ['reader'], false],
])('warns for account %s and people %j only when needed', (account, people, expected) => {
  expect(secretaryWarning(account, 'owner', people)).toBe(expected);
  show(SecretaryWarning, { account, owner: 'owner', people });
  expect(document.querySelectorAll('.note').length).toBe(expected ? 1 : 0);
});

it('shows the foreign home and secretary warning without blocking settings', async () => {
  const { group, info, ai, person } = setup();
  group.apply([
    { ...info, seq: 100, secretary: ai.id },
    { ...ai, seq: 10001, user: 'secretary-owner', account: 'personal' },
    p.decode(p.DeviceRow, goldens['row-device']),
  ]);
  show(GroupDetails, { group });
  expect(all('section.field > dl dd').map((el) => el.textContent)).toEqual([
    'Group computer',
    person.name,
  ]);
  expect(one('section.field > p.hint').textContent).toBe(
    'That computer stores and orders the group and must stay on.',
  );
  expect(one('section > .note').textContent).toBe(
    "Every request to this personal-subscription secretary from someone other than its owner waits for the owner's confirmation, each time. With a team plan or an API key, the owner can list who may direct it.",
  );
  one<HTMLButtonElement>('section > button').click();
  flushSync();
  expect(one<HTMLDialogElement>('dialog').open).toBe(true);
  one<HTMLButtonElement>('dialog footer button[type=button]').click();
  flushSync();
  await vi.waitFor(() => expect(document.querySelector('dialog')).toBeNull());
});

it('shows an unknown home owner, a raw absent-owner id and an owned home without the foreign-home note', async () => {
  const { group, home } = setup();
  show(GroupDetails, { group });
  expect(one('section > dl dd:last-child').textContent).toBe('Owner not provided');
  group.apply([{ ...p.decode(p.DeviceRow, goldens['row-device']), user: 'keeper' }]);
  flushSync();
  expect(one('section > dl dd:last-child').textContent).toBe('keeper');
  home.owner = true;
  flushSync();
  expect(document.querySelector('section.field > p.hint')).toBeNull();
});

it('limits settings to project admins and hides unavailable secretary details', () => {
  const { group, person, info } = setup('github');
  group.rows.delete(`member ${p.decode(p.AIMemberRow, goldens['row-member-ai']).id}`);
  show(GroupDetails, { group });
  expect(document.querySelector('section > button')).toBeNull();
  group.apply([
    { ...info, seq: 100, forge: 'custom', secretary: person.id },
    { ...person, seq: 101, role: 'member' },
  ]);
  flushSync();
  expect(document.querySelector('section > button')).toBeNull();
  group.apply([{ ...info, seq: 102, kind: 'team' }]);
  flushSync();
  expect(document.querySelector('details')).toBeNull();
  group.rows.delete(`group ${group.g}`);
  flushSync();
  expect(document.querySelector('details')).toBeNull();
});

it('disables settings while offline', () => {
  const { group, home } = setup();
  home.ctl = undefined;
  show(GroupDetails, { group });
  expect(one<HTMLButtonElement>('section > button').disabled).toBe(true);
});

it('saves custom templates exactly, warns for a prospective secretary and reports refusals', async () => {
  const { group, info, ai, request } = setup();
  const onclose = vi.fn();
  request.mockResolvedValueOnce({ re: 1, err: { code: 'forbidden', msg: 'refused' } });
  group.apply([{ ...ai, seq: 10000, user: 'secretary-owner', account: 'personal' }]);
  show(EditGroup, { group, info, onclose });
  choose(one<HTMLSelectElement>('dialog select'), ai.name);
  flushSync();
  expect(document.querySelectorAll('dialog .note').length).toBe(1);
  fill(one<HTMLTextAreaElement>('fieldset textarea'), 'program');
  one('dialog form').dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }));
  await vi.waitFor(() =>
    expect(document.querySelector('[role=alert]')?.textContent?.trim()).toBe(
      'Not sent: this device may not do that.',
    ),
  );
  const { n, ts, ...command } = JSON.parse(request.mock.calls[0][0].env.p);
  expect(command).toEqual({
    t: 'group',
    g: group.g,
    op: 'set',
    secretary: ai.id,
    templates: { ...info.templates, push: ['program', ...info.templates!.push.slice(1)] },
  });
  expect(info.templates!.push[0]).toBe('git');
  one('dialog form').dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }));
  await vi.waitFor(() => expect(one<HTMLDialogElement>('dialog').open).toBe(false));
});

it('saves only a secretary for a built-in forge, and can cancel without a command', async () => {
  const { group, info, ai, request } = setup('github');
  delete info.templates;
  info.secretary = ai.id;
  show(EditGroup, { group, info, onclose: vi.fn() });
  expect(document.querySelector('fieldset')).toBeNull();
  group.apply([{ ...ai, seq: 10000, name: 'Renamed AI' }]);
  flushSync();
  expect(one<HTMLSelectElement>('dialog select').selectedOptions[0].textContent).toBe('Renamed AI');
  expect(document.querySelector('dialog select[aria-label="Forge"]')).toBeNull();
  one('dialog form').dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }));
  await vi.waitFor(() => expect(request).toHaveBeenCalledTimes(1));
  const { n, ts, ...command } = JSON.parse(request.mock.calls[0][0].env.p);
  expect(command).toEqual({ t: 'group', g: group.g, op: 'set', secretary: ai.id });
});

it('never offers an external AI as secretary', () => {
  const { group, info, ai } = setup('github');
  group.apply([{ ...ai, id: 'external', name: 'Scout', cli: 'external' }]);
  show(EditGroup, { group, info, onclose: vi.fn() });
  expect(all('dialog option').map((el) => el.textContent)).toEqual(['Choose an AI', ai.name]);
});

it('offers no settings when an external AI is the only one', () => {
  const { group, ai } = setup('github');
  show(GroupDetails, { group });
  expect(one('section > button').textContent!.trim()).toBe('Edit group settings');
  group.apply([{ ...ai, seq: 10000, cli: 'external' }]);
  flushSync();
  expect(document.querySelector('section > button')).toBeNull();
});

it('edits templates with no AI members and sends no secretary field', async () => {
  const { group, info, ai, request } = setup();
  group.rows.delete(`member ${ai.id}`);
  show(EditGroup, { group, info, onclose: vi.fn() });
  expect(document.querySelector('dialog select')).toBeNull();
  one('dialog form').dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }));
  await vi.waitFor(() => expect(request).toHaveBeenCalledTimes(1));
  const { n, ts, ...command } = JSON.parse(request.mock.calls[0][0].env.p);
  expect(command).toEqual({ t: 'group', g: group.g, op: 'set', templates: info.templates });
});
