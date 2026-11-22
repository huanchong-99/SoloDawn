import { describeFeature, loadFeature } from '@amiceli/vitest-cucumber';
import { flushSync, tick } from 'svelte';
import { expect, vi } from 'vitest';
import { Group, Home, homes } from '../src/lib/groups.svelte';
import type { Ctl, Envelope, NoteRow, Row } from '../src/lib/proto';
import { example, owner, user } from './home';
import { all, closePage, one, openPage, texts } from './page';

const feature = await loadFeature('../features/web_kb.feature');
const gid = 'rZNvy-1jH6Z-BcPqA5U5BQ';
let group: Group;
let held: NoteRow[];
let fetches: Record<string, unknown>[];
let commands: Record<string, unknown>[];

async function request(req: Record<string, unknown>) {
  if (req.t === 'cmd') {
    const cmd = JSON.parse((req.env as Envelope).p);
    commands.push(cmd);
    held = held.filter((row) => row.id !== cmd.id);
    group.apply([{ r: 'note', id: cmd.id, seq: 10000, gone: true }]);
    return { re: 1, ok: true };
  }
  if (req.t === 'fetch' && req.kind === 'kb') {
    fetches.push(req);
    if (req.q !== undefined) {
      const matches = req.q === 'writer' ? [...held].reverse() : [];
      return { re: 1, ok: true, rows: matches, more: matches.length > 1 };
    }
    const rows = held.filter((row) => !req.before || row.pos < (req.before as number)).toReversed();
    return {
      re: 1,
      ok: true,
      rows: rows.slice(0, req.limit as number),
      more: rows.length > (req.limit as number),
    };
  }
  return { re: 1, ok: true, rows: [], more: false };
}

function setup(count: number) {
  fetches = [];
  commands = [];
  held = Array.from(
    { length: count },
    (_, i) =>
      example('row-note-kb', {
        id: `document-${i + 1}`,
        seq: i + 1000,
        pos: i + 1,
        by: owner,
        name: `Decision ${i + 1}`,
        text: `Writer decision ${i + 1}.`,
      }) as NoteRow,
  );
  const home = new Home({ user, sign: async (p) => ({ p, k: 'key', s: 'signature' }) });
  home.ctl = { request: request as unknown as Ctl['request'] };
  group = new Group(home, gid);
  group.apply([
    example('row-group'),
    example('row-member-human', { id: owner, user, name: 'Owner' }),
    example('row-note', { id: 'context', text: 'Context only.' }),
  ]);
  home.groups = [group];
  homes.push(home);
  openPage(`#/groups/${gid}`, 1440);
}

async function settle() {
  await tick();
  await vi.waitFor(() => {
    flushSync();
    expect(all('.kb [role=status]')).toEqual([]);
  });
}

async function open(selector = '.kb details') {
  one<HTMLDetailsElement>(selector).open = true;
  one(selector).dispatchEvent(new Event('toggle'));
  await settle();
}

async function search(q: string) {
  const input = one<HTMLInputElement>('.kb input');
  input.value = q;
  input.dispatchEvent(new Event('input', { bubbles: true }));
  flushSync();
  one('.kb form').dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }));
  await settle();
}

function clear() {
  one<HTMLButtonElement>('.kb .btn-ghost').click();
  flushSync();
}

describeFeature(feature, ({ Scenario, AfterEachScenario }) => {
  AfterEachScenario(closePage);
  Scenario(
    'Documents load on opening and older pages load on demand',
    ({ Given, When, Then, And }) => {
      Given('an online group with 51 knowledge-base documents', () => {
        setup(51);
        expect(fetches).toEqual([]);
      });
      When('the reader opens Knowledge base', () => open());
      Then('the browser lists the newest 50 documents with names authors times and text', () => {
        expect(fetches).toEqual([{ t: 'fetch', g: gid, kind: 'kb', before: 0, limit: 50 }]);
        expect(texts('.kb .mem-item > b')).toEqual(held.slice(1).map((row) => row.name));
        expect(texts('.kb .mem-text')).toEqual(held.slice(1).map((row) => row.text));
        expect(texts('.kb .mem-meta > b')).toEqual(Array(50).fill('Owner'));
        expect(all<HTMLTimeElement>('.kb time').map((el) => el.dateTime)).toEqual(
          held.slice(1).map((row) => new Date(row.ts).toISOString()),
        );
      });
      When('the reader loads earlier documents', async () => {
        one<HTMLButtonElement>('.kb .older').click();
        await settle();
      });
      Then('all 51 documents appear in creation order', () => {
        expect(fetches[1]).toEqual({ t: 'fetch', g: gid, kind: 'kb', before: 2, limit: 50 });
        expect(texts('.kb .mem-item > b')).toEqual(held.map((row) => row.name));
        expect(all('.kb .older')).toEqual([]);
      });
      And('typed documents stay out of Project memory', async () => {
        await open('.groupinfo details');
        expect(texts('.groupinfo .mem-text')).toEqual(['Context only.']);
      });
    },
  );
  Scenario(
    'Search preserves the home ranking and clearing restores the list',
    ({ Given, When, Then }) => {
      Given('an online group with two knowledge-base documents', async () => {
        setup(2);
        await open();
      });
      When('the reader searches for writer', () => search('writer'));
      Then('the browser shows the home matches best first and asks for a narrower query', () => {
        expect(fetches.at(-1)).toEqual({
          t: 'fetch',
          g: gid,
          kind: 'kb',
          before: 0,
          limit: 20,
          q: 'writer',
        });
        expect(texts('.kb .mem-item > b')).toEqual(['Decision 2', 'Decision 1']);
        expect(texts('.kb .hint')).toEqual(['More documents match. Narrow your query.']);
      });
      When('the reader searches for absent', () => search('absent'));
      Then('the browser shows No matching documents', () => {
        expect(texts('.kb .hint')).toEqual(['No matching documents.']);
        expect(texts('.kb .mem-item > b')).toEqual([]);
      });
      When('the reader clears the search', clear);
      Then('both documents are listed in creation order', () => {
        expect(texts('.kb .mem-item > b')).toEqual(['Decision 1', 'Decision 2']);
        expect(one<HTMLInputElement>('.kb input').value).toBe('');
      });
    },
  );
  Scenario(
    'Forgetting removes a document from results and the list',
    ({ Given, When, Then, And }) => {
      Given('an online group with two knowledge-base documents', async () => {
        setup(2);
        await open();
      });
      When('the reader searches for writer', () => search('writer'));
      And('the admin deletes the first result', async () => {
        one<HTMLButtonElement>('.kb .del').click();
        flushSync();
        expect(texts('#chat-side dialog .preview')).toEqual(['Decision 2']);
        one('#chat-side dialog form').dispatchEvent(
          new Event('submit', { bubbles: true, cancelable: true }),
        );
        await vi.waitFor(() => {
          flushSync();
          expect(all('#chat-side dialog')).toEqual([]);
        });
      });
      Then('that document is absent from the results and focus returns to the fold', () => {
        expect(commands[0]).toEqual({
          t: 'forget',
          g: gid,
          r: 'note',
          id: 'document-2',
          n: expect.any(String),
          ts: expect.any(Number),
        });
        expect(texts('.kb .mem-item > b')).toEqual(['Decision 1']);
        expect(document.activeElement).toBe(one('.kb summary'));
      });
      When('the reader clears the search', clear);
      Then('that document is absent from the list', () => {
        expect(texts('.kb .mem-item > b')).toEqual(['Decision 1']);
      });
      When('the reader searches again for writer', () => search('writer'));
      Then('the home no longer returns the forgotten document', () => {
        expect(texts('.kb .mem-item > b')).toEqual(['Decision 1']);
      });
    },
  );
  Scenario('Offline documents are read-only', ({ Given, When, Then }) => {
    Given('an online group with two knowledge-base documents', async () => {
      setup(2);
      await open();
    });
    When('the home disconnects', () => {
      group.home.ctl = undefined;
      flushSync();
    });
    Then('cached documents remain readable and search paging and deletion are disabled', () => {
      expect(texts('.kb .mem-text')).toEqual(['Writer decision 1.', 'Writer decision 2.']);
      expect(texts('.kb .hint')).toEqual(['Cannot reach the group home. Documents are read-only.']);
      expect(one<HTMLInputElement>('.kb input').disabled).toBe(true);
      expect(one<HTMLButtonElement>('.kb form button').disabled).toBe(true);
      expect(all<HTMLButtonElement>('.kb .del').map((button) => button.disabled)).toEqual([
        true,
        true,
      ]);
    });
  });
});
