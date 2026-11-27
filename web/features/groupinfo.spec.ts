import { describeFeature, loadFeature } from '@amiceli/vitest-cucumber';
import { flushSync, tick } from 'svelte';
import { expect, vi } from 'vitest';
import { Home, homes } from '../src/lib/groups.svelte';
import * as p from '../src/lib/proto';
import { example, owner, TestHome, user } from './home';
import { all, closePage, one, openPage, texts } from './page';

const feature = await loadFeature('../features/web_groupinfo.feature');

const website = 'rZNvy-1jH6Z-BcPqA5U5BQ';
const taskId = 'DrtCn6htSBwmMPrFPbHJHA';
const secId = 'qBSFMsr2hHYKOMblEA_kdA';
const claudeId = 'LXCZmuGAXkvO-bSrOkuCfw';
const note1Id = '7bRlYkKR5AU8bF6kt-syDQ';
const note2Id = 'AcVx2WScEL_cTdsBkR_uUA';
const reviewId = 'yXrOTI_vLO6PoPPJ9SqrGA';

const sign = async (payload: string) => ({ p: payload, k: 'this-browser', s: 'signature' });

type StoredRow = p.Row & Record<string, unknown>;

function matchesKind(row: StoredRow, kind: string): boolean {
  if ('gone' in row) return false;
  if (kind === 'note') return row.r === 'note';
  return row.r === 'msg' && row.kind === 'review';
}

function belowPos(row: StoredRow, before: number): boolean {
  return before === 0 || (row.pos as number) < before;
}

function pageByKind(home: TestHome, req: Record<string, unknown>, re: number) {
  const stored = home.groups.get(req.g as string);
  const before = req.before as number;
  const scope = [...(stored?.rows.values() ?? [])]
    .filter((row) => matchesKind(row, req.kind as string) && belowPos(row, before))
    .sort((a, b) => (b.pos as number) - (a.pos as number));
  const rows = scope.slice(0, req.limit as number);
  return { t: 'fetched' as const, re, ok: true as const, rows, more: scope.length > rows.length };
}

interface MemoryRig {
  home: TestHome;
  store: Home;
  forgets: p.Forget[];
  writes: Record<string, unknown>[];
  refuseWrite?: string;
  fetches: Record<string, unknown>[];
  refuseForget?: string;
  freshBrowser(): void;
  connect(): Promise<void>;
  drop(): void;
}

function makeRig(): MemoryRig {
  const home = new TestHome('workstation');
  const forgets: p.Forget[] = [];
  const writes: Record<string, unknown>[] = [];
  const fetches: Record<string, unknown>[] = [];
  let drop = () => {};
  let seq = 200;

  async function handleForget(env: p.Envelope) {
    await new Promise((later) => setTimeout(later));
    expect(env.k).toBe('this-browser');
    const cmd = p.decode(p.Forget, env.p);
    forgets.push(cmd);
    if (rig.refuseForget) {
      return { re: (seq += 1), err: { code: rig.refuseForget, msg: 'refused' } };
    }
    home.remove(cmd.g, cmd.r, cmd.id);
    return { re: (seq += 1), ok: true as const };
  }

  // `note` and `note_edit` reach the home's own handler: it writes the note as a home does.
  home.reply = (payload) => {
    writes.push(payload);
    if (rig.refuseWrite) return { err: { code: rig.refuseWrite, msg: 'refused' } };
    const { id, task, text } = payload as { id: string; task?: string | null; text: string };
    const old = home.groups.get(website)!.rows.get(`note ${id}`) as p.NoteRow | undefined;
    const note = { ...(example('row-note') as p.NoteRow), ...old };
    home.write(website, { ...note, id, task: task ?? null, text, by: owner });
    return { ok: true };
  };

  const rig: MemoryRig = {
    home,
    store: new Home({ user, sign }),
    forgets,
    writes,
    fetches,
    freshBrowser() {
      rig.drop();
      rig.store = new Home({ user, sign });
    },
    async connect() {
      const link = rig.store.connect();
      const connection = home.connect((frame) => link.push(frame));
      drop = () => {
        connection.close();
        link.offline();
      };
      const ctl: p.Ctl = {
        request: (async (req: Record<string, unknown>) => {
          if (req.t === 'fetch' && 'kind' in req) {
            await new Promise((later) => setTimeout(later));
            fetches.push(req);
            return pageByKind(home, req, (seq += 1));
          }
          if (
            req.t === 'cmd' &&
            (JSON.parse((req.env as p.Envelope).p) as { t?: string }).t === 'forget'
          ) {
            return handleForget(req.env as p.Envelope);
          }
          return connection.ctl.request(req as Parameters<p.Ctl['request']>[0]);
        }) as p.Ctl['request'],
      };
      const welcome = await ctl.request({
        t: 'hello',
        v: 1,
        kind: 'browser',
        name: 'test',
        addrs: [],
      });
      link.online(ctl, welcome as p.Welcome);
    },
    drop: () => drop(),
  };
  return rig;
}

function buttonByLabel(label: string): HTMLButtonElement {
  return all<HTMLButtonElement>('#chat-side button').find(
    (btn) => (btn.getAttribute('aria-label') ?? btn.textContent!.trim()) === label,
  )!;
}

describeFeature(feature, ({ Background, Scenario, AfterEachScenario }) => {
  let rig: MemoryRig;

  async function syncRig() {
    const expected = rig.home.synced + rig.home.groups.size;
    await rig.connect();
    await vi.waitFor(() => expect(rig.home.synced).toBe(expected));
    await new Promise((applied) => setTimeout(applied));
    flushSync();
  }

  async function openMemory() {
    const beforeCount = rig.fetches.length;
    const fold = one<HTMLDetailsElement>('.groupinfo details.fold');
    fold.open = true;
    fold.dispatchEvent(new Event('toggle'));
    flushSync();
    await vi.waitFor(() => expect(rig.fetches.length).toBeGreaterThanOrEqual(beforeCount + 2));
    await new Promise((applied) => setTimeout(applied, 5));
    flushSync();
  }

  AfterEachScenario(closePage);

  Background(({ Given }) => {
    Given(
      'a group "Website" homed on "workstation", open in a browser of its owner in English',
      async () => {
        rig = makeRig();
        rig.home.add(website, 'Website');
        rig.home.write(
          website,
          example('row-member-ai', {
            id: secId,
            g: website,
            name: 'Secretary',
            cli: 'claude',
            secretary: true,
            state: 'waiting',
            task: undefined,
          }),
        );
        rig.home.write(
          website,
          example('row-member-ai', {
            id: claudeId,
            g: website,
            name: 'Claude Code',
            cli: 'claude',
            state: 'dormant',
            task: undefined,
          }),
        );
        homes.push(rig.store);
        await syncRig();
        openPage(`#/groups/${website}`, 1440);
        expect(texts('.chat-head h2')).toEqual(['Website']);
      },
    );
  });

  Scenario(
    'Project memory lists notes and the review ledger, and forgetting removes an entry within 1 second',
    ({ Given, When, Then, And }) => {
      Given(
        'the home holds two notes and a scored review whose task was removed by retention',
        async () => {
          await closePage();
          rig.freshBrowser();
          rig.home.write(website, example('row-task', { id: taskId, g: website, num: 12 }));
          rig.home.write(
            website,
            example('row-note', {
              id: note1Id,
              g: website,
              task: taskId,
              by: owner,
              text: 'Keep `rtt_ms` empty on relay.',
            }),
          );
          rig.home.write(
            website,
            example('row-note', {
              id: note2Id,
              g: website,
              task: null,
              by: secId,
              text: 'Path events include the connection id.',
            }),
          );
          rig.home.write(website, example('row-msg-review', { id: reviewId, g: website }));
          homes.push(rig.store);
          await syncRig();
          openPage(`#/groups/${website}`, 1440);
        },
      );
      Then(
        'no memory fetch has been sent and the group info panel folds "Project memory" with "Kept past retention"',
        () => {
          expect(rig.fetches).toEqual([]);
          expect(texts('.groupinfo > h2')).toEqual(['Group info']);
          expect(texts('.groupinfo details.fold > summary')).toEqual([
            'Project memory Kept past retention',
          ]);
        },
      );
      When('the owner opens "Project memory"', openMemory);
      Then('the browser fetches kind "note" and kind "review" from 0 with limit 50', () => {
        expect(rig.fetches).toEqual([
          { t: 'fetch', g: website, kind: 'note', before: 0, limit: 50 },
          { t: 'fetch', g: website, kind: 'review', before: 0, limit: 50 },
        ]);
      });
      And(
        'the notes list shows "Keep `rtt_ms` empty on relay." on "T-12" by "Alex" and "Path events include the connection id." by "Secretary"',
        () => {
          const noteSecs = all('.groupinfo .mem-sec')[0];
          expect(noteSecs.querySelector('h3')!.textContent!.trim()).toBe('Notes');
          const items = [...noteSecs.querySelectorAll('.mem-item')];
          expect(items).toHaveLength(2);
          expect(items[0].querySelector('.mem-meta')!.textContent).toContain('Alex');
          expect(items[0].querySelector('.mem-meta')!.textContent).toContain('T-12');
          expect(items[0].querySelector('.mem-text code')!.textContent).toBe('rtt_ms');
          expect(items[1].querySelector('.mem-meta')!.textContent).toContain('Secretary');
          expect(items[1].querySelector('.mem-text')!.textContent!.trim()).toBe(
            'Path events include the connection id.',
          );
        },
      );
      And(
        'the review ledger shows "T-12", "86 / 90", "Show connection paths", "Claude Code · Round 1" and remark "Missing relay path" at "path.go:88"',
        () => {
          rig.home.remove(website, 'task', taskId);
          flushSync();
          const ledgerSec = all('.groupinfo .mem-sec')[1];
          expect(ledgerSec.querySelector('h3')!.textContent!.trim()).toBe('Review ledger');
          const entry = ledgerSec.querySelector('.mem-item')!;
          expect(entry.querySelector('.mem-meta')!.textContent).toContain('T-12');
          expect(entry.querySelector('.badge')!.textContent!.trim()).toBe('86 / 90');
          expect(entry.querySelector('.mem-sub')!.textContent!.replace(/\s+/g, ' ').trim()).toBe(
            'Show connection paths · Claude Code · Round 1',
          );
          expect(entry.querySelector('.bul li')!.textContent!.replace(/\s+/g, ' ').trim()).toBe(
            'Missing relay path path.go:88',
          );
        },
      );
      When(
        'the owner presses "Delete note: Keep rtt_ms empty on relay." and cancels the confirmation',
        async () => {
          buttonByLabel('Delete note: Keep rtt_ms empty on relay.').click();
          flushSync();
          expect(texts('#chat-side dialog h2')).toEqual(['Delete this note?']);
          expect(texts('#chat-side dialog .preview')).toEqual(['Keep rtt_ms empty on relay.']);
          one<HTMLButtonElement>('#chat-side dialog .btn-ghost').click();
          await tick();
          flushSync();
        },
      );
      Then('no forget command was sent and the confirmation is gone', () => {
        expect(rig.forgets).toEqual([]);
        expect(all('#chat-side dialog')).toEqual([]);
      });
      When(
        'the owner presses "Delete note: Keep rtt_ms empty on relay." and confirms "Delete"',
        async () => {
          const started = Date.now();
          buttonByLabel('Delete note: Keep rtt_ms empty on relay.').click();
          flushSync();
          one('#chat-side dialog form').dispatchEvent(
            new Event('submit', { bubbles: true, cancelable: true }),
          );
          await vi.waitFor(() => expect(rig.forgets).toHaveLength(1));
          await tick();
          flushSync();
          expect(Date.now() - started).toBeLessThan(1000);
        },
      );
      Then(
        'the home receives a signed "forget" for "note" and that note disappears within 1 second with focus on the notes heading',
        () => {
          expect(rig.forgets[0]).toEqual({
            t: 'forget',
            n: expect.stringMatching(/^[A-Za-z0-9_-]{21}[AQgw]$/),
            ts: expect.any(Number),
            g: website,
            r: 'note',
            id: note1Id,
          });
          expect(texts('.groupinfo .mem-text')).toEqual(['Path events include the connection id.']);
          expect(document.activeElement?.textContent?.trim()).toBe('Notes');
        },
      );
      When(
        'the owner presses "Delete review: T-12 Show connection paths" and confirms "Delete"',
        async () => {
          buttonByLabel('Delete review: T-12 Show connection paths').click();
          flushSync();
          expect(texts('#chat-side dialog h2')).toEqual(['Delete this review from the ledger?']);
          one('#chat-side dialog form').dispatchEvent(
            new Event('submit', { bubbles: true, cancelable: true }),
          );
          await vi.waitFor(() => expect(rig.forgets).toHaveLength(2));
          await tick();
          flushSync();
        },
      );
      Then(
        'the home receives a signed "forget" for "msg" and the review ledger reads "No scored reviews yet."',
        () => {
          expect(rig.forgets[1]).toMatchObject({ t: 'forget', g: website, r: 'msg', id: reviewId });
          expect(all('.groupinfo .mem-sec')[1].textContent).toContain('No scored reviews yet.');
        },
      );
      When('another device forgets the remaining note', () => {
        rig.home.remove(website, 'note', note2Id);
        flushSync();
      });
      Then('the notes list drops it within 1 second and reads "No notes yet."', () => {
        expect(texts('.groupinfo .mem-text')).toEqual([]);
        expect(all('.groupinfo .mem-sec')[0].textContent).toContain('No notes yet.');
      });
    },
  );

  Scenario('Older pages of project memory load on demand', ({ Given, When, Then }) => {
    const allTexts = Array.from({ length: 65 }, (_, i) => `lesson ${i + 1}`);
    Given('the home holds 65 notes', async () => {
      await closePage();
      rig.freshBrowser();
      for (const [i, text] of allTexts.entries()) {
        rig.home.write(
          website,
          example('row-note', { id: `n-${i + 1}`, g: website, by: owner, text }),
        );
      }
      homes.push(rig.store);
      await syncRig();
      openPage(`#/groups/${website}`, 1440);
    });
    When('the owner opens "Project memory"', openMemory);
    Then(
      'the notes list shows the newest 50 notes in creation order and offers "Load earlier"',
      () => {
        expect(texts('.groupinfo .mem-text')).toEqual(allTexts.slice(-50));
        expect(texts('.groupinfo .older')).toEqual(['Load earlier']);
      },
    );
    When('the owner presses "Load earlier"', async () => {
      one<HTMLButtonElement>('.groupinfo .older').click();
      await vi.waitFor(() => {
        flushSync();
        expect(texts('.groupinfo .mem-text')).toHaveLength(65);
        expect(all('.groupinfo .older')).toEqual([]);
      });
    });
    Then('all 65 notes are shown in creation order and "Load earlier" is gone', () => {
      expect(texts('.groupinfo .mem-text')).toEqual(allTexts);
      expect(all('.groupinfo .older')).toEqual([]);
    });
  });

  Scenario(
    'Offline read-only, non-admin member, refusal and narrow-screen Escape states',
    ({ Given, When, Then, And }) => {
      Given(
        'the home holds one note and one scored review and the owner has opened "Project memory"',
        async () => {
          rig.home.write(
            website,
            example('row-note', {
              id: note1Id,
              g: website,
              by: owner,
              text: 'Keep sockets short.',
            }),
          );
          rig.home.write(website, example('row-msg-review', { id: reviewId, g: website }));
          flushSync();
          await openMemory();
        },
      );
      When('the home disconnects', () => {
        rig.drop();
        flushSync();
      });
      Then(
        'the delete buttons are disabled with reason "Cannot reach the group home." while the note and review stay readable',
        () => {
          const dels = all<HTMLButtonElement>('.groupinfo .del');
          expect(dels).toHaveLength(2);
          expect(dels.map((b) => b.disabled)).toEqual([true, true]);
          const whyId = dels[0].getAttribute('aria-describedby')!;
          expect(document.getElementById(whyId)?.textContent?.trim()).toBe(
            'Cannot reach the group home.',
          );
          expect(texts('.groupinfo .mem-text')).toEqual(['Keep sockets short.']);
        },
      );
      When('the home reconnects and the reader becomes a non-admin member', async () => {
        await syncRig();
        rig.home.write(
          website,
          example('row-member-human', {
            id: owner,
            g: website,
            user,
            name: 'Alex',
            role: 'member',
            online: true,
          }),
        );
        flushSync();
      });
      Then(
        'the delete buttons are disabled with reason "Only an admin can delete project memory."',
        () => {
          const dels = all<HTMLButtonElement>('.groupinfo .del');
          expect(dels.map((b) => b.disabled)).toEqual([true, true]);
          const whyId = dels[0].getAttribute('aria-describedby')!;
          expect(document.getElementById(whyId)?.textContent?.trim()).toBe(
            'Only an admin can delete project memory.',
          );
        },
      );
      When(
        'the reader becomes an admin again and the home refuses "forget" with "forbidden"',
        () => {
          rig.refuseForget = 'forbidden';
          rig.home.write(
            website,
            example('row-member-human', {
              id: owner,
              g: website,
              user,
              name: 'Alex',
              role: 'admin',
              online: true,
            }),
          );
          flushSync();
        },
      );
      And('the owner confirms deleting the note', async () => {
        buttonByLabel('Delete note: Keep sockets short.').click();
        flushSync();
        one('#chat-side dialog form').dispatchEvent(
          new Event('submit', { bubbles: true, cancelable: true }),
        );
        await vi.waitFor(() => expect(rig.forgets).toHaveLength(1));
        flushSync();
      });
      Then('the dialog stays open and says "Not sent: this device may not do that."', () => {
        expect(one<HTMLDialogElement>('#chat-side dialog').open).toBe(true);
        expect(texts('#chat-side dialog [role=alert]')).toEqual([
          'Not sent: this device may not do that.',
        ]);
      });
      When(
        'the chat is reopened at 820 px wide, the owner opens "People and details" and "Project memory", opens delete on the note and presses Escape',
        async () => {
          await closePage();
          rig.refuseForget = undefined;
          homes.push(rig.store);
          await syncRig();
          openPage(`#/groups/${website}`, 820);
          one<HTMLButtonElement>('.side-open').click();
          await tick();
          flushSync();
          await openMemory();
          buttonByLabel('Delete note: Keep sockets short.').click();
          flushSync();
          const dlg = one<HTMLDialogElement>('#chat-side dialog');
          expect(dlg.open).toBe(true);
          dlg
            .querySelector('button')!
            .dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }));
          dlg.close();
          await tick();
          flushSync();
        },
      );
      Then('the confirmation dialog closes and the side panel stays open', () => {
        expect(all('#chat-side dialog')).toEqual([]);
        expect(one('#chat-side').classList.contains('open')).toBe(true);
      });
    },
  );
  Scenario(
    'A member writes and corrects notes in place, on a narrow screen',
    ({ Given, When, Then }) => {
      const field = () => one<HTMLTextAreaElement>('.groupinfo .edit-field textarea');
      const key = (init: KeyboardEventInit) =>
        field().dispatchEvent(
          new KeyboardEvent('keydown', { bubbles: true, cancelable: true, ...init }),
        );
      const type = (text: string) => {
        field().value = text;
        field().dispatchEvent(new Event('input', { bubbles: true }));
        flushSync();
      };
      async function sent(count: number) {
        await vi.waitFor(() => expect(rig.writes).toHaveLength(count));
        await tick();
        flushSync();
      }
      Given(
        'the home holds one note and a member reads "Project memory" at 820 px wide',
        async () => {
          await closePage();
          rig.freshBrowser();
          rig.home.write(
            website,
            example('row-note', {
              id: note1Id,
              g: website,
              by: secId,
              text: 'Keep sockets short.',
            }),
          );
          rig.home.write(
            website,
            example('row-member-human', {
              id: owner,
              g: website,
              user,
              name: 'Alex',
              role: 'member',
              online: true,
            }),
          );
          homes.push(rig.store);
          await syncRig();
          openPage(`#/groups/${website}`, 820);
          one<HTMLButtonElement>('.side-open').click();
          await tick();
          flushSync();
          await openMemory();
          expect(all('.groupinfo .add')).toHaveLength(1);
        },
      );
      When(
        'the member presses "Change note: Keep sockets short." and saves "Keep sockets under 30 s." with Ctrl and Enter',
        async () => {
          buttonByLabel('Change note: Keep sockets short.').click();
          flushSync();
          expect([document.activeElement, field().value]).toEqual([field(), 'Keep sockets short.']);
          type('Keep sockets under 30 s.');
          key({ key: 'Enter', ctrlKey: true });
          await sent(1);
        },
      );
      Then(
        'the home receives a signed "note_edit" for that note, which reads "Keep sockets under 30 s." with focus back on its button',
        async () => {
          expect(rig.writes[0]).toEqual({
            t: 'note_edit',
            n: expect.stringMatching(/^[A-Za-z0-9_-]{21}[AQgw]$/),
            ts: expect.any(Number),
            g: website,
            id: note1Id,
            text: 'Keep sockets under 30 s.',
          });
          expect(texts('.groupinfo .mem-text')).toEqual(['Keep sockets under 30 s.']);
          await vi.waitFor(() =>
            expect(document.activeElement).toBe(
              buttonByLabel('Change note: Keep sockets under 30 s.'),
            ),
          );
        },
      );
      When('the home refuses the next edit with "not_found"', async () => {
        rig.refuseWrite = 'not_found';
        buttonByLabel('Change note: Keep sockets under 30 s.').click();
        flushSync();
        type('Keep sockets under 10 s.');
        one<HTMLButtonElement>('.groupinfo .edit-field .btn-primary').click();
        await sent(2);
      });
      Then('the field stays open and says "Not saved: it was deleted in the meantime."', () => {
        expect(texts('.groupinfo .edit-field [role=alert]')).toEqual([
          'Not saved: it was deleted in the meantime.',
        ]);
      });
      When('the member presses Escape in the field', async () => {
        key({ key: 'Escape' });
        await tick();
        flushSync();
      });
      Then(
        'the field is gone, the note reads "Keep sockets under 30 s." and the side panel stays open',
        () => {
          expect(all('.groupinfo .edit-field')).toEqual([]);
          expect(texts('.groupinfo .mem-text')).toEqual(['Keep sockets under 30 s.']);
          expect(one('#chat-side').classList.contains('open')).toBe(true);
        },
      );
      When('the member adds the note "Ask before merging."', async () => {
        rig.refuseWrite = undefined;
        one<HTMLButtonElement>('.groupinfo .add').click();
        flushSync();
        expect(field().value).toBe('');
        type('Ask before merging.');
        key({ key: 'Enter', metaKey: true });
        await sent(3);
      });
      Then(
        'the home receives a signed "note" with a new id and no task, and the notes end with "Ask before merging."',
        () => {
          expect(rig.writes[2]).toEqual({
            t: 'note',
            n: expect.any(String),
            ts: expect.any(Number),
            g: website,
            id: expect.stringMatching(/^[A-Za-z0-9_-]{21}[AQgw]$/),
            task: null,
            text: 'Ask before merging.',
          });
          expect(texts('.groupinfo .mem-text')).toEqual([
            'Keep sockets under 30 s.',
            'Ask before merging.',
          ]);
        },
      );
      When('the home disconnects', () => {
        rig.drop();
        flushSync();
      });
      Then('the change and add buttons are disabled', () => {
        const buttons = all<HTMLButtonElement>('.groupinfo .edit, .groupinfo .add');
        expect(buttons.map((each) => each.disabled)).toEqual([true, true, true]);
      });
    },
  );
});
