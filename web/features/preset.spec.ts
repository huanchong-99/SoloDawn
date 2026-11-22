import { describeFeature, loadFeature } from '@amiceli/vitest-cucumber';
import { flushSync, tick } from 'svelte';
import { expect, vi } from 'vitest';
import { Group, Home, homes } from '../src/lib/groups.svelte';
import * as p from '../src/lib/proto';
import { example, user, owner } from './home';
import { all, closePage, one, openPage, texts } from './page';

const feature = await loadFeature('../features/web_preset.feature');
const gid = 'rZNvy-1jH6Z-BcPqA5U5BQ';
const style = 'One accent colour.';
let group: Group;
let seq = 1000;
let commands: Record<string, unknown>[];
let fetches: Record<string, unknown>[];
let held: p.NoteRow[];

async function request(req: Record<string, unknown>) {
  if (req.t === 'fetch' && req.kind) {
    fetches.push(req);
    return { re: 1, ok: true, rows: req.kind === 'preset' ? held : [], more: false };
  }
  if (req.t === 'cmd') {
    const cmd = JSON.parse((req.env as p.Envelope).p);
    commands.push(cmd);
    applyNote(cmd);
    return { re: 1, ok: true };
  }
  return { re: 1, ok: true, rows: [], more: false };
}

function applyNote(cmd: Record<string, unknown>) {
  const previous = held.find((row) => row.name === cmd.name);
  const note = example('row-note', {
    ...cmd,
    r: 'note',
    id: previous?.id ?? cmd.id,
    by: owner,
    ts: 1,
    seq: ++seq,
  }) as p.NoteRow;
  held = held.filter((row) => row.id !== note.id).concat(note);
  group.apply([note]);
}

function setup() {
  commands = [];
  fetches = [];
  held = [
    example('row-note', {
      id: 'preset',
      type: 'preset',
      name: 'Calm dashboard',
      text: style,
    }) as p.NoteRow,
  ];
  const home = new Home({
    user,
    sign: async (payload) => ({ p: payload, k: 'key', s: 'signature' }),
  });
  home.ctl = { request: request as unknown as p.Ctl['request'] };
  group = new Group(home, gid);
  group.apply([
    example('row-group'),
    example('row-member-human', { id: owner, user, online: true }),
    example('row-msg-brief-preset', {
      main: true,
      body: { ...(example('row-msg-brief-preset').body as object), preset: 'Calm dashboard' },
    }),
    example('row-note', { id: 'context', text: 'Context only.' }),
    example('row-note', { id: 'kb', type: 'kb', name: 'Decision', text: 'Knowledge only.' }),
  ]);
  home.groups = [group];
  homes.push(home);
  openPage(`#/groups/${gid}`, 1440);
}

async function openFold(selector: string) {
  one<HTMLDetailsElement>(selector).open = true;
  one(selector).dispatchEvent(new Event('toggle'));
  await tick();
  await vi.waitFor(() => {
    flushSync();
    expect(all(`${selector} [role=status]`)).toHaveLength(0);
  });
}

function fill(selector: string, value: string) {
  const input = one<HTMLInputElement>(selector);
  input.value = value;
  input.dispatchEvent(new Event('input', { bubbles: true }));
  flushSync();
}

async function save() {
  one('.preset form').dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }));
  await tick();
  await vi.waitFor(() => {
    flushSync();
    expect(one<HTMLButtonElement>('.preset form button').disabled).toBe(true);
  });
}

describeFeature(feature, ({ Scenario, AfterEachScenario }) => {
  AfterEachScenario(closePage);
  Scenario('A brief names a preset and opens its text', ({ Given, When, Then, And }) => {
    Given('a group with a brief naming Calm dashboard', setup);
    When('the reader opens the brief preset', () =>
      openFold('[aria-label^="Task brief"] dd details'),
    );
    Then('the card names Calm dashboard and shows its text', () => {
      expect(texts('[aria-label^="Task brief"] dt')).toEqual([
        'Goal',
        'Acceptance',
        'Scope',
        'Split',
        'Style preset',
      ]);
      expect(all('[aria-label^="Task brief"] dd details.fold')).toEqual([]);
      expect(texts('[aria-label^="Task brief"] dd details summary')).toEqual(['Calm dashboard']);
      expect(all('[aria-label^="Task brief"] dd details summary svg.i.sm')).toHaveLength(1);
      expect(texts('[aria-label^="Task brief"] dd details p')).toEqual([style]);
    });
    And('typed notes do not appear in Project memory', async () => {
      await openFold('.groupinfo:not(.preset) details');
      expect(texts('.groupinfo:not(.preset) .mem-text')).toEqual(['Context only.']);
    });
  });
  Scenario('A person lists creates and changes presets', ({ Given, When, Then }) => {
    Given('an online human member opens Style presets', async () => {
      setup();
      await openFold('.preset details');
    });
    Then('the editor fetches presets and lists their name and text', () => {
      expect(fetches).toEqual([{ t: 'fetch', g: gid, kind: 'preset', before: 0, limit: 50 }]);
      expect(texts('.preset .mem-item > b')).toEqual(['Calm dashboard']);
      expect(texts('.preset .mem-text')).toEqual([style]);
      const list = one('.preset .mem-list');
      expect(document.getElementById(list.getAttribute('aria-labelledby')!)?.textContent).toBe(
        'Style presets',
      );
    });
    When('the member creates Quiet forms', async () => {
      fill('.preset input', 'Quiet forms');
      fill('.preset textarea', 'No motion.');
      await save();
    });
    Then('a preset note command creates Quiet forms', () => {
      expect(commands[0]).toEqual({
        t: 'note',
        n: expect.any(String),
        ts: expect.any(Number),
        g: gid,
        id: expect.any(String),
        task: null,
        type: 'preset',
        name: 'Quiet forms',
        text: 'No motion.',
      });
      expect(texts('.preset .mem-text')).toEqual([style, 'No motion.']);
    });
    When('the member edits Quiet forms', async () => {
      all<HTMLButtonElement>('.preset .mem-item button')[2].click();
      flushSync();
      expect(one<HTMLInputElement>('.preset input').disabled).toBe(true);
      fill('.preset textarea', 'Short motion.');
      await save();
    });
    Then('a preset note command replaces its text', () => {
      expect(commands[1]).toMatchObject({
        t: 'note',
        name: 'Quiet forms',
        text: 'Short motion.',
        type: 'preset',
        task: null,
      });
      expect(commands[1].id).not.toBe(commands[0].id);
      expect(texts('.preset .mem-text')).toEqual([style, 'Short motion.']);
    });
  });
  Scenario('Missing presets and read-only states', ({ Given, When, Then }) => {
    Given('a group with a brief naming Calm dashboard', setup);
    When('the named preset is deleted and the reader opens it', async () => {
      held = [];
      await openFold('[aria-label^="Task brief"] dd details');
    });
    Then('the brief says the preset was deleted', () => {
      expect(texts('[aria-label^="Task brief"] dd details p')).toEqual([
        'This preset was deleted.',
      ]);
    });
    When('the home disconnects and the reader opens Style presets', async () => {
      group.apply([
        example('row-note', { id: 'cached', type: 'preset', name: 'Cached', text: style }),
      ]);
      group.home.ctl = undefined;
      await openFold('.preset details');
    });
    Then('presets remain readable but writing is disabled', () => {
      expect(texts('.preset .mem-text')).toEqual([style]);
      expect(one<HTMLInputElement>('.preset input').disabled).toBe(true);
      expect(one<HTMLTextAreaElement>('.preset textarea').disabled).toBe(true);
      expect(texts('.preset .hint')).toEqual([
        'Cannot reach the group home. Presets are read-only.',
      ]);
    });
  });
});
