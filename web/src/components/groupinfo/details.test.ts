import { flushSync } from 'svelte';
import { SvelteMap } from 'svelte/reactivity';
import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import { all, choose, closePage, fill, one, show } from '../../../features/page';
import { Group, Home, homes } from '../../lib/groups.svelte';
import { nodes } from '../../lib/nodes.svelte';
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

it('warns in the invitation dialog before the first other person joins, not afterward', async () => {
  const { group, info, ai, person, request } = setup();
  show(InvitationDialog, { group, open: true });
  expect(document.querySelector('dialog header .note')).toBeNull();
  // The dialog reads the group's invitation code when it opens (people.md 2.6).
  await vi.waitFor(() => expect(request).toHaveBeenCalledOnce());
  expect(sent(request)).toEqual({ t: 'code', g: group.g, op: 'show' });
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

it('warns about a personal secretary in Add AI, and sends the folder it took', async () => {
  const { group, home, person, request } = setup();
  home.owner = true;
  homes.push(home);
  const hostInfo = p.decode(p.HostInfoPush, goldens['host-info-push']);
  hostInfo.agents = [
    { ...hostInfo.agents[0], agent: 'other-cli', account: 'personal', groups: [] },
  ];
  nodes.infos.set(home, hostInfo);
  const folders = p.decode(p.HostWorkspaces, goldens['host-workspaces-answer']);
  request.mockImplementation(async (req: { t: string }) =>
    req.t === 'host.workspaces' ? folders : { ...hostInfo, re: 1, ok: true },
  );
  group.apply([{ ...person, id: 'reader', user: 'reader', name: 'Reader', seq: 102 }]);
  show(AddAI, { group, open: true });
  await vi.waitFor(() =>
    expect(one<HTMLInputElement>('dialog input[type=radio]').checked).toBe(true),
  );
  await vi.waitFor(() =>
    expect(one('dialog .path-row code').textContent?.trim()).toBe('/home/dev/website'),
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
  expect(request.mock.calls.map(([req]) => req.t)).toEqual(['host.info', 'host.workspaces']);
  fill(one<HTMLInputElement>('dialog input.input'), 'Project AI');
  request.mockResolvedValueOnce({ re: 1, err: { code: 'forbidden', msg: 'refused' } });
  one('dialog form').dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }));
  await vi.waitFor(() =>
    expect(one('[role=alert]')?.textContent?.trim()).toBe('Not sent: this device may not do that.'),
  );
  const join = JSON.parse(request.mock.calls.at(-1)![0].env.p);
  expect(join).toMatchObject({
    t: 'host.join',
    name: 'Project AI',
    workspace: '/home/dev/website',
  });
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

function setup() {
  const person = p.decode(p.HumanMemberRow, goldens['row-member-human']);
  const home = new Home({
    user: person.user,
    sign: async (payload) => ({ p: payload, k: 'key', s: 'signature' }),
  });
  const group = new Group(home, person.g);
  const info = p.decode(p.GroupRow, goldens['row-group']);
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
  expect(all('section.field > dl dt').map((el) => el.textContent)).toEqual([
    'Group name',
    'Secretary',
    'Home computer',
    'Computer owner',
  ]);
  expect(all('section.field > dl dd').map((el) => el.textContent!.trim())).toEqual([
    `${info.name} Change`,
    `${ai.name} Change`,
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

it('limits settings to admins and hides unavailable secretary details', () => {
  const { group, person, info } = setup();
  group.rows.delete(`member ${p.decode(p.AIMemberRow, goldens['row-member-ai']).id}`);
  show(GroupDetails, { group });
  expect(one('section > button').textContent!.trim()).toBe('Edit group settings');
  group.apply([
    { ...info, seq: 100, secretary: person.id },
    { ...person, seq: 101, role: 'member' },
  ]);
  flushSync();
  expect(document.querySelector('section > button')).toBeNull();
  group.apply([{ ...person, seq: 103, role: 'admin' }]);
  flushSync();
  expect(one('section > button').textContent!.trim()).toBe('Edit group settings');
  group.rows.delete(`group ${group.g}`);
  flushSync();
  expect(document.querySelector('section > button')).toBeNull();
});

it('disables settings while offline', () => {
  const { group, home } = setup();
  home.ctl = undefined;
  show(GroupDetails, { group });
  expect(one<HTMLButtonElement>('section > button').disabled).toBe(true);
});

const save = () => one<HTMLButtonElement>('dialog footer .btn-primary');
const submit = () =>
  one('dialog form').dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }));
const input = (label: string) =>
  all('dialog label')
    .find((el) => el.textContent!.trim().startsWith(label))!
    .querySelector<HTMLInputElement>('input, textarea')!;
const limit = (label: string) =>
  one<HTMLInputElement>(
    `#${CSS.escape(all<HTMLLabelElement>('dialog .limit label').find((el) => el.textContent === label)!.htmlFor)}`,
  );
const sent = (request: ReturnType<typeof setup>['request'], i = 0) => {
  const { n: _n, ts: _ts, ...command } = JSON.parse(request.mock.calls[i][0].env.p);
  return command;
};

it('warns for a prospective secretary and reports refusals', async () => {
  const { group, info, ai, request } = setup();
  request.mockResolvedValueOnce({ re: 1, err: { code: 'forbidden', msg: 'refused' } });
  group.apply([{ ...ai, seq: 10000, user: 'secretary-owner', account: 'personal' }]);
  show(EditGroup, { group, info, onclose: vi.fn() });
  expect(save().disabled).toBe(true);
  choose(one<HTMLSelectElement>('dialog select'), ai.name);
  flushSync();
  expect(document.querySelectorAll('dialog .note').length).toBe(1);
  expect(save().disabled).toBe(false);
  submit();
  await vi.waitFor(() =>
    expect(document.querySelector('[role=alert]')?.textContent?.trim()).toBe(
      'Not sent: this device may not do that.',
    ),
  );
  expect(sent(request)).toEqual({ t: 'group', g: group.g, op: 'set', secretary: ai.id });
  request.mockResolvedValueOnce({ re: 1, err: { code: 'conflict', msg: 'busy' } });
  submit();
  await vi.waitFor(() =>
    expect(document.querySelector('[role=alert]')?.textContent).toBe(
      'Not saved: that AI is on an unfinished task, so it cannot be the secretary now.',
    ),
  );
  submit();
  await vi.waitFor(() => expect(one<HTMLDialogElement>('dialog').open).toBe(false));
});

it('corrects the name and the scores, and sends nothing else', async () => {
  const { group, ai, info, request } = setup();
  info.secretary = ai.id;
  show(EditGroup, { group, info, onclose: vi.fn() });
  // One kind of group: no repository, forge, check command or template is asked for.
  expect(all('dialog textarea')).toEqual([]);
  group.apply([{ ...ai, seq: 10000, name: 'Renamed AI' }]);
  flushSync();
  expect(one<HTMLSelectElement>('dialog select').selectedOptions[0].textContent).toBe('Renamed AI');
  fill(input('Group name'), ' Docs ');
  fill(input('Pass score'), '85');
  fill(input('Foundation tasks'), '60');
  expect([input('Pass score'), input('Foundation tasks')].map((el) => [el.min, el.max])).toEqual([
    ['1', '100'],
    ['1', '100'],
  ]);
  submit();
  await vi.waitFor(() => expect(one<HTMLDialogElement>('dialog').open).toBe(false));
  expect(sent(request)).toEqual({
    t: 'group',
    g: group.g,
    op: 'set',
    name: 'Docs',
    pass: 85,
    pass_foundation: 60,
  });
});

it('sends all limits when one changes, and opens the fold of a field the browser refuses', async () => {
  const { group, info, request } = setup();
  show(EditGroup, { group, info, onclose: vi.fn() });
  const fold = one<HTMLDetailsElement>('dialog details');
  expect(fold.open).toBe(false);
  expect(all('dialog .limit label').map((el) => el.textContent)).toEqual([
    'AI rounds in a thread',
    'Wake-ups per hour',
    'Longest turn',
    'Failures in a row',
    'Failed reviews',
    'Quiet task',
    'Wait for an approval',
    'Wait for another AI',
    'Chain of questions',
  ]);
  expect(all('dialog .limit .hint').every((el) => el.textContent)).toBe(true);
  const approval = limit('Wait for an approval');
  expect([approval.value, approval.max, approval.step]).toEqual(['10', '10', 'any']);
  const depth = limit('Chain of questions');
  expect([depth.value, depth.min, depth.max, depth.step]).toEqual(['4', '0', '4', '1']);
  expect(limit('Failed reviews').min).toBe('0');
  expect(limit('Failures in a row').min).toBe('1');
  fill(limit('Wake-ups per hour'), '40');
  fill(approval, '5');
  approval.dispatchEvent(new Event('invalid'));
  flushSync();
  expect(fold.open).toBe(true);
  submit();
  await vi.waitFor(() => expect(request).toHaveBeenCalledTimes(1));
  expect(sent(request)).toEqual({
    t: 'group',
    g: group.g,
    op: 'set',
    limits: { ...info.limits, wakes_per_hour: 40, approval_wait_s: 300 },
  });
});

it('never offers an external AI as secretary, and no choice when there is no other', () => {
  const { group, info, ai } = setup();
  group.apply([{ ...ai, id: 'external', name: 'Scout', cli: 'external' }]);
  show(EditGroup, { group, info, onclose: vi.fn() });
  expect(all('dialog option').map((el) => el.textContent)).toEqual(['Choose an AI', ai.name]);
  group.apply([{ ...ai, seq: 10000, cli: 'external' }]);
  flushSync();
  expect(document.querySelector('dialog select')).toBeNull();
});

const edit = (label: string) => one<HTMLButtonElement>(`button[aria-label="${label}"]`);
const press = (el: Element, key: string, init: KeyboardEventInit = {}) =>
  el.dispatchEvent(new KeyboardEvent('keydown', { key, bubbles: true, cancelable: true, ...init }));

it('renames the group in place: Enter sends it, Escape leaves it, focus goes back', async () => {
  const { group, info, request } = setup();
  show(GroupDetails, { group });
  const opener = edit('Change group name');
  expect(opener.getAttribute('aria-expanded')).toBe('false');
  opener.click();
  flushSync();
  const field = one<HTMLInputElement>('input[aria-label="Group name"]');
  expect([document.activeElement, field.value, field.maxLength]).toEqual([field, info.name, 80]);
  fill(field, 'Other');
  const escape = new KeyboardEvent('keydown', { key: 'Escape', bubbles: true, cancelable: true });
  field.dispatchEvent(escape);
  expect(escape.defaultPrevented).toBe(true);
  await vi.waitFor(() => expect(document.activeElement).toBe(edit('Change group name')));
  expect(document.querySelector('input[aria-label="Group name"]')).toBeNull();
  expect(request).not.toHaveBeenCalled();

  edit('Change group name').click();
  flushSync();
  fill(one<HTMLInputElement>('input[aria-label="Group name"]'), '  ');
  flushSync();
  expect(one<HTMLButtonElement>('.edit-field .btn-primary').disabled).toBe(true);
  fill(one<HTMLInputElement>('input[aria-label="Group name"]'), ' Docs ');
  one<HTMLFormElement>('.edit-field').requestSubmit();
  await vi.waitFor(() => expect(document.activeElement).toBe(edit('Change group name')));
  expect(sent(request)).toEqual({ t: 'group', g: group.g, op: 'set', name: 'Docs' });
});

it('keeps an unchanged value unsent, and a refused one open with its reason', async () => {
  const { group, request } = setup();
  show(GroupDetails, { group });
  edit('Change group name').click();
  flushSync();
  one<HTMLFormElement>('.edit-field').requestSubmit();
  await vi.waitFor(() => expect(document.querySelector('.edit-field')).toBeNull());
  expect(request).not.toHaveBeenCalled();
  request.mockResolvedValueOnce({ re: 1, err: { code: 'forbidden', msg: 'refused' } });
  edit('Change group name').click();
  flushSync();
  fill(one<HTMLInputElement>('input[aria-label="Group name"]'), 'Docs');
  one<HTMLFormElement>('.edit-field').requestSubmit();
  await vi.waitFor(() =>
    expect(one('.edit-field [role=alert]').textContent!.trim()).toBe(
      'Not sent: this device may not do that.',
    ),
  );
  one<HTMLButtonElement>('.edit-field .btn-ghost').click();
  await vi.waitFor(() => expect(document.querySelector('.edit-field')).toBeNull());
});

it('changes the secretary in place and says why an AI on a task cannot be one', async () => {
  const { group, info, ai, request } = setup();
  show(GroupDetails, { group });
  expect(one('section.field > dl dd:nth-of-type(2)').textContent!.trim()).toBe('None Change');
  edit('Change secretary').click();
  flushSync();
  const select = one<HTMLSelectElement>('select[aria-label="Secretary"]');
  expect(all('select option').map((el) => el.textContent)).toEqual(['Choose an AI', ai.name]);
  expect(one<HTMLButtonElement>('.edit-field .btn-primary').disabled).toBe(true);
  request.mockResolvedValueOnce({ re: 1, err: { code: 'conflict', msg: 'busy' } });
  choose(select, ai.name);
  flushSync();
  press(select, 'Enter', { metaKey: true });
  await vi.waitFor(() =>
    expect(one('.edit-field [role=alert]').textContent).toBe(
      'Not saved: that AI is on an unfinished task, so it cannot be the secretary now.',
    ),
  );
  expect(sent(request)).toEqual({ t: 'group', g: group.g, op: 'set', secretary: ai.id });
  press(select, 'a');
  expect(document.querySelector('.edit-field')).not.toBeNull();
  press(select, 'Escape');
  await vi.waitFor(() => expect(document.activeElement).toBe(edit('Change secretary')));
  group.apply([{ ...info, seq: 100, secretary: ai.id }]);
  flushSync();
  expect(one('section.field > dl dd:nth-of-type(2)').textContent!.trim()).toBe(`${ai.name} Change`);
});

it('shows the name but no way to change it to a member, and none offline', () => {
  const { group, home, person, ai } = setup();
  group.apply([{ ...person, seq: 101, role: 'member' }]);
  show(GroupDetails, { group });
  expect(document.querySelectorAll('button.edit').length).toBe(0);
  group.rows.delete(`member ${ai.id}`);
  group.apply([{ ...person, seq: 102, role: 'owner' }]);
  home.ctl = undefined;
  flushSync();
  expect(all('section.field > dl dt').map((el) => el.textContent)).toEqual([
    'Group name',
    'Home computer',
    'Computer owner',
  ]);
  expect(edit('Change group name').disabled).toBe(true);
});

it('ignores Escape while a save is in flight, so its answer settles this edit', async () => {
  const { group, request } = setup();
  let answer!: (value: object) => void;
  request.mockReturnValueOnce(new Promise((resolve) => (answer = resolve)));
  show(GroupDetails, { group });
  edit('Change group name').click();
  flushSync();
  const field = one<HTMLInputElement>('input[aria-label="Group name"]');
  fill(field, 'Docs');
  one<HTMLFormElement>('.edit-field').requestSubmit();
  await vi.waitFor(() => expect(request).toHaveBeenCalledTimes(1));
  flushSync();
  const escape = new KeyboardEvent('keydown', { key: 'Escape', bubbles: true, cancelable: true });
  field.dispatchEvent(escape);
  flushSync();
  expect([escape.defaultPrevented, field.isConnected, field.readOnly]).toEqual([false, true, true]);
  answer({ re: 1, err: { code: 'forbidden', msg: 'refused' } });
  await vi.waitFor(() =>
    expect(one('.edit-field [role=alert]').textContent!.trim()).toBe(
      'Not sent: this device may not do that.',
    ),
  );
  expect(field.value).toBe('Docs');
});
