import { flushSync } from 'svelte';
import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import { all, choose, closePage, fill, one, show } from '../../../features/page';
import { Home, homes } from '../../lib/groups.svelte';
import { goldens } from '../../lib/mock';
import { prefs } from '../../lib/prefs.svelte';
import * as p from '../../lib/proto';
import CreateGroup from '../groups/CreateGroup.svelte';
import Commands from './Commands.svelte';
import { emptyTemplates, operations } from './templates';

beforeEach(() => {
  prefs.lang = 'en';
});
afterEach(closePage);

function setup() {
  const home = new Home({
    user: 'owner',
    sign: async (payload) => ({ p: payload, k: 'key', s: 'signature' }),
  });
  home.owner = true;
  home.name = 'Computer';
  const request = vi.fn().mockResolvedValue({ re: 1, ok: true });
  home.ctl = { request: request as p.Ctl['request'] };
  homes.push(home);
  show(CreateGroup, { open: true });
  return { home, request };
}

function custom() {
  choose(one<HTMLSelectElement>('dialog select'), 'Custom command templates');
  flushSync();
}

it('starts with four independent required programs', () => {
  const first = emptyTemplates();
  first.push[0] = 'git';
  expect(emptyTemplates()).toEqual({ push: [''], open: [''], checks: [''], merge: [''] });
  const { home } = setup();
  home.name = 'Renamed computer';
  flushSync();
  expect(all<HTMLSelectElement>('dialog select').at(-1)!.selectedOptions[0].textContent).toBe(
    'Renamed computer',
  );
  custom();
  expect(all('fieldset legend').map((el) => el.textContent)).toEqual([
    'Push branch',
    'Open request',
    'Hosted checks',
    'Merge request',
  ]);
  expect(
    all<HTMLTextAreaElement>('fieldset textarea').map((el) => [el.value, el.required]),
  ).toEqual([
    ['', true],
    ['', true],
    ['', true],
    ['', true],
  ]);
  expect(all<HTMLButtonElement>('fieldset .row button').map((el) => el.disabled)).toEqual([
    true,
    true,
    true,
    true,
  ]);
  one<HTMLButtonElement>('dialog footer button[type=button]').click();
  flushSync();
  expect(one<HTMLDialogElement>('dialog').open).toBe(false);
});

it('creates a custom group with exact argument arrays, including empty and multiline arguments', async () => {
  const { request } = setup();
  const now = vi.spyOn(Date, 'now').mockReturnValue(42);
  const inputs = all<HTMLInputElement>('dialog input');
  [' Project ', ' https://git.example/team/project.git ', ' stable ', ' Owner '].forEach(
    (value, i) => fill(inputs[i], value),
  );
  custom();
  const templates = {
    push: ['git', '', 'two  spaces'],
    open: ['forge', '--title={title}', 'line\nbreak'],
    checks: ['forge'],
    merge: ['forge'],
  };
  const fields = all<HTMLFieldSetElement>('fieldset');
  operations.forEach((operation, at) => {
    const field = fields[at];
    templates[operation].forEach((argument, i) => {
      if (i > 0) {
        field.querySelector<HTMLButtonElement>(':scope > button')!.click();
        flushSync();
      }
      fill(field.querySelectorAll('textarea')[i], argument);
    });
  });
  const add = fields[0].querySelector<HTMLButtonElement>(':scope > button')!;
  add.click();
  flushSync();
  fields[0].querySelectorAll<HTMLButtonElement>('.row button')[3].click();
  flushSync();
  expect([...fields[0].querySelectorAll('textarea')].map((el) => el.value)).toEqual([
    'git',
    '',
    'two  spaces',
  ]);
  fill(one<HTMLTextAreaElement>('textarea[aria-describedby]'), ' pnpm check\n\n pnpm test ');
  one('dialog form').dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }));
  await vi.waitFor(() => expect(request).toHaveBeenCalledTimes(1));
  const env = request.mock.calls[0][0].env;
  const { n, ts, ...command } = JSON.parse(env.p);
  expect(command).toEqual({
    t: 'group',
    g: n,
    op: 'create',
    name: 'Project',
    kind: 'project',
    repo: 'https://git.example/team/project.git',
    base: 'stable',
    forge: 'custom',
    checks: ['pnpm check', 'pnpm test'],
    member_name: 'Owner',
    templates,
  });
  expect(ts).toBe(42);
  expect([env.k, env.s]).toEqual(['key', 'signature']);
  await vi.waitFor(() => expect(one<HTMLDialogElement>('dialog').open).toBe(false));
  now.mockRestore();
});

it('omits templates when changing back to a built-in forge and shows refusals without closing', async () => {
  const { request } = setup();
  request.mockResolvedValue({ re: 1, err: { code: 'bad_request', msg: 'refused' } });
  custom();
  choose(one<HTMLSelectElement>('dialog select'), 'None, this computer only');
  flushSync();
  expect(document.querySelector('fieldset')).toBeNull();
  one('dialog form').dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }));
  await vi.waitFor(() =>
    expect(document.querySelector('[role=alert]')?.textContent?.trim()).toBe(
      'Not sent: the home refused it as too long or malformed.',
    ),
  );
  const command = JSON.parse(request.mock.calls[0][0].env.p);
  expect([command.forge, Object.hasOwn(command, 'templates')]).toEqual(['none', false]);
  expect(one<HTMLDialogElement>('dialog').open).toBe(true);
});

it('discloses checks and each custom argument verbatim, marking empty arguments', () => {
  const info = p.decode(p.GroupRow, goldens['row-group-custom']);
  info.checks = ['printf "two  spaces"\nnext', 'pnpm test'];
  info.templates!.open.push('', 'line\nbreak');
  show(Commands, { info });
  expect(all('dd').map((el) => el.textContent)).toEqual([
    info.repo,
    info.base,
    'Custom command templates',
  ]);
  expect(all('section > p code').map((el) => el.textContent)).toEqual(info.checks);
  expect(
    all('.arguments').map((el) => [...el.querySelectorAll('code')].map((code) => code.textContent)),
  ).toEqual(
    operations.map((operation) => info.templates![operation].map((argument) => argument || '""')),
  );
  expect(one('.note').textContent).toBe(
    'These commands run on this computer as you, outside any sandbox, whenever an AI of yours works on a task of this group. Join only groups whose commands you would run yourself.',
  );
});

it.each([
  ['github', 'GitHub, through gh'],
  ['none', 'None, this computer only'],
  ['custom', 'Custom command templates'],
])('discloses an empty command list without inventing templates for %s', (forge, label) => {
  const info = p.decode(p.GroupRow, goldens['row-group']);
  delete info.repo;
  delete info.templates;
  info.forge = forge;
  info.checks = [];
  show(Commands, { info });
  expect(all('dd').map((el) => el.textContent)).toEqual(['None', info.base, label]);
  expect(one('section > p.hint').textContent).toBe('None');
  expect(document.querySelector('ol')).toBeNull();
});
