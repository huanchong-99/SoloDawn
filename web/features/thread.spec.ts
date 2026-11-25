import { describeFeature, loadFeature } from '@amiceli/vitest-cucumber';
import { flushSync } from 'svelte';
import { expect, vi } from 'vitest';
import Thread from '../src/components/thread/Thread.svelte';
import { homes, type Group } from '../src/lib/groups.svelte';
import { examples } from '../src/lib/mock';
import { cards } from '../src/lib/mounts.svelte';
import { prefs } from '../src/lib/prefs.svelte';
import type * as p from '../src/lib/proto';
import { route } from '../src/lib/route.svelte';
import { browser, example, owner, TestHome, user } from './home';
import { all, closePage, one, openPage, show, texts } from './page';

const feature = await loadFeature('../features/web_thread.feature');

const website = 'rZNvy-1jH6Z-BcPqA5U5BQ';
const taskId = 'DrtCn6htSBwmMPrFPbHJHA';
const codexId = '1PC8WineBrUQ-apCjx7tug';
const claudeId = 'LXCZmuGAXkvO-bSrOkuCfw';
const secId = 'qBSFMsr2hHYKOMblEA_kdA';

describeFeature(feature, ({ Background, Scenario, AfterEachScenario }) => {
  let home: TestHome;
  let reader: ReturnType<typeof browser>;
  let group: Group;
  let task: p.TaskRow;

  const fetches = () => home.asked.filter((req) => req.t === 'fetch');

  async function read() {
    const answered = home.synced + home.groups.size;
    await reader.connect();
    await vi.waitFor(() => expect(home.synced).toBe(answered));
    await new Promise((applied) => setTimeout(applied));
    flushSync();
  }

  AfterEachScenario(async () => {
    prefs.lang = 'en';
    vi.unstubAllEnvs();
    await closePage();
  });

  Background(({ Given }) => {
    Given(
      'a group "Website" with a secretary, two AIs and task 12 "Show connection path"',
      async () => {
        prefs.lang = 'en';
        home = new TestHome();
        home.add(website, 'Website');
        home.write(
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
        home.write(
          website,
          example('row-member-ai', {
            id: codexId,
            g: website,
            name: 'Codex',
            cli: 'codex',
            state: 'waiting',
            task: undefined,
          }),
        );
        home.write(
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
        task = home.write(website, example('row-task', { g: website })) as p.TaskRow;
        reader = browser(home);
        homes.push(reader.store);
        await read();
        group = reader.store.groups[0];
      },
    );
  });

  Scenario(
    'Opening a thread fetches its newest page, and older pages load on demand',
    ({ Given, When, Then, And }) => {
      const items = Array.from({ length: 75 }, (_, i) => `step ${i + 1}`);
      Given('the thread of task 12 holds 75 progress messages on the home', async () => {
        reader.drop();
        for (const [i, text] of items.entries()) {
          home.write(
            website,
            example('row-msg-progress', {
              id: `prog-${i + 1}`,
              thread: taskId,
              by: codexId,
              text,
            }),
          );
        }
        reader = browser(home);
        homes.length = 0;
        homes.push(reader.store);
        await read();
        group = reader.store.groups[0];
        expect(group.thread(taskId)).toEqual([]);
      });
      When('the thread of task 12 is mounted', async () => {
        show(Thread, { group, task });
        await vi.waitFor(() => expect(fetches()).toHaveLength(1));
        await vi.waitFor(() => expect(group.paging(taskId).busy).toBe(false));
        flushSync();
      });
      Then('the browser fetches the newest 50 messages of task 12 and shows them in order', () => {
        expect(fetches()).toEqual([
          { t: 'fetch', g: website, thread: taskId, before: 0, limit: 50 },
        ]);
        expect(texts('.tlist .ti .text')).toEqual(items.slice(-50));
        expect(texts('.tpage button')).toEqual(['Load earlier messages']);
      });
      When('the reader presses "Load earlier messages"', async () => {
        one<HTMLButtonElement>('.tpage button').click();
        await vi.waitFor(() => expect(fetches()).toHaveLength(2));
        await vi.waitFor(() => expect(group.paging(taskId).more).toBe(false));
        flushSync();
      });
      Then(
        'the browser fetches the page before the oldest message held and shows all 75 in order',
        () => {
          const oldest = group.thread(taskId).find((m) => m.text === 'step 26')!.pos;
          expect(fetches()[1]).toEqual({
            t: 'fetch',
            g: website,
            thread: taskId,
            before: oldest,
            limit: 50,
          });
          expect(texts('.tlist .ti .text')).toEqual(items);
        },
      );
      And('the button "Load earlier messages" is gone', () => {
        expect(all('.tpage')).toEqual([]);
      });
    },
  );

  Scenario(
    'An unreachable home stops thread paging with a notice',
    ({ Given, When, Then, And }) => {
      Given('the thread of task 12 holds 75 progress messages on the home', () => {
        for (let i = 0; i < 75; i++) {
          home.write(
            website,
            example('row-msg-progress', {
              id: `off-${i}`,
              thread: taskId,
              by: codexId,
              text: `line ${i}`,
            }),
          );
        }
      });
      When('the home stops and the thread of task 12 is mounted', () => {
        home.stop();
        reader.drop();
        show(Thread, { group, task });
      });
      Then('no thread page is fetched', async () => {
        await new Promise((later) => setTimeout(later, 10));
        expect(fetches()).toEqual([]);
      });
      And('the thread says "Earlier messages cannot be loaded right now."', () => {
        expect(texts('.tpage')).toEqual(['Earlier messages cannot be loaded right now.']);
        group.paging(taskId).failed = true;
        flushSync();
        expect(texts('.tpage')).toEqual(['Earlier messages cannot be loaded right now.']);
      });
    },
  );

  Scenario(
    'A thread renders message kinds, report, checks, one-line links for main-line cards and the working indicator',
    ({ Given, When, Then, And }) => {
      const rowWith = (needle: string) =>
        all('.tlist > .ti').find((el) => el.textContent?.includes(needle))!;

      Given('the thread of task 12 holds every thread message kind', () => {
        home.write(website, example('row-msg-progress', { id: 'k-prog', thread: taskId }));
        home.write(
          website,
          example('row-msg-question', {
            id: 'k-q',
            thread: taskId,
            text: '@Secretary should `path` carry rtt?',
            mentions: [secId],
          }),
        );
        home.write(
          website,
          example('row-msg-text', {
            id: 'k-txt',
            thread: taskId,
            by: owner,
            text: 'Keep it optional.',
          }),
        );
        home.write(website, example('row-msg-report', { id: 'k-rep', thread: taskId }));
        home.write(
          website,
          example('row-msg-report', {
            id: 'k-rep-local',
            thread: taskId,
            text: 'Updated local report.',
            body: { commit: '1234567890abcdef', files: 1, added: 10, removed: 2 },
          }),
        );
        home.write(website, example('row-msg-checks', { id: 'k-chk-ok', thread: taskId }));
        home.write(
          website,
          example('row-msg-checks', {
            id: 'k-chk-cloud',
            thread: taskId,
            body: {
              stage: 'cloud',
              ok: false,
              commit: '9f2c1abf43f3',
              items: [{ name: 'ci/linux', ok: false, detail: 'exit 1' }],
            },
          }),
        );
        home.write(
          website,
          example('row-msg-checks', {
            id: 'k-chk-merge',
            thread: taskId,
            body: {
              stage: 'merge',
              ok: false,
              commit: '9f2c1abf43f3',
              items: [{ name: 'merge', ok: false, detail: 'CONFLICT in path.go' }],
            },
          }),
        );
        home.write(website, example('row-msg-approval-pending', { id: 'k-ap', thread: taskId }));
        home.write(
          website,
          example('row-msg-approval-approved', {
            id: 'k-ap-done',
            thread: taskId,
            body: {
              task: taskId,
              ai: codexId,
              tool: 'Bash',
              command: 'go test ./...',
              why: 'Approved earlier.',
              category: 'command',
              approvers: [user],
            },
          }),
        );
        home.write(website, example('row-msg-decision-pending', { id: 'k-dc', thread: taskId }));
        home.write(
          website,
          example('row-msg-decision-decided', {
            id: 'k-dc-done',
            thread: taskId,
            body: {
              task: taskId,
              question: 'Already answered?',
              options: [{ id: 'A', label: 'Yes', effect: 'Ship it.' }],
              to: [user],
            },
          }),
        );
        home.write(website, example('row-msg-summary', { id: 'k-sum', thread: taskId }));
      });
      And('the assignee "Codex" is working on task 12 for 6 minutes', () => {
        home.write(
          website,
          example('row-member-ai', {
            id: codexId,
            g: website,
            name: 'Codex',
            cli: 'codex',
            state: 'working',
            task: taskId,
            since: Date.now() - 6 * 60_000,
          }),
        );
      });
      let scrolled: unknown;
      When('the thread of task 12 is mounted', async () => {
        openPage(`#/groups/${website}`);
        show(Thread, { group, task });
        await vi.waitFor(() => expect(group.paging(taskId).more).toBe(false));
        flushSync();
      });
      Then(
        'the thread shows the badges for progress, question, report, approval, decision and summary',
        () => {
          expect(texts('.ti-head .badge')).toEqual([
            'Progress',
            'Question',
            'Report',
            'Report',
            'Approval',
            'Approval',
            'Decision',
            'Decision',
            'Summary',
          ]);
          expect(
            rowWith('PR #31').querySelector<HTMLElement>('.ti-head .badge')?.dataset.tone,
          ).toBe('ok');
          expect(rowWith('Keep it optional.').querySelector('.ti-head .badge')).toBeNull();
        },
      );
      And(
        'only cards waiting on the reader carry the dawn badge, and every main-line card links to its main-line message without duplicate ids',
        () => {
          expect(all('.thread [id]')).toEqual([]);
          const ids = all('[id]').map((el) => el.id);
          expect(new Set(ids).size).toBe(ids.length);
          const pendingAp = rowWith('Time the path change in a browser.');
          const decidedAp = rowWith('Approved earlier.');
          const pendingDc = rowWith('Where should the path appear?');
          const decidedDc = rowWith('Already answered?');
          const summaryRow = all('.tlist > .ti').find(
            (el) => el.querySelector('.ti-head .badge')?.textContent === 'Summary',
          )!;

          expect(pendingAp.querySelector<HTMLElement>('.ti-head .badge')?.dataset.tone).toBe(
            'dawn',
          );
          expect(
            decidedAp.querySelector<HTMLElement>('.ti-head .badge')?.dataset.tone,
          ).toBeUndefined();
          expect(pendingDc.querySelector<HTMLElement>('.ti-head .badge')?.dataset.tone).toBe(
            'dawn',
          );
          expect(
            decidedDc.querySelector<HTMLElement>('.ti-head .badge')?.dataset.tone,
          ).toBeUndefined();
          expect(summaryRow.querySelector<HTMLElement>('.ti-head .badge')?.dataset.tone).toBe('ok');

          expect(pendingAp.querySelector('a.tcard')?.getAttribute('href')).toBe('#m-k-ap');
          expect(decidedAp.querySelector('a.tcard')?.getAttribute('href')).toBe('#m-k-ap-done');
          expect(pendingDc.querySelector('a.tcard')?.getAttribute('href')).toBe('#m-k-dc');
          expect(decidedDc.querySelector('a.tcard')?.getAttribute('href')).toBe('#m-k-dc-done');
          expect(summaryRow.querySelector('a.tcard')?.getAttribute('href')).toBe('#m-k-sum');
          expect(all('.thread .card, .thread button:not(.tpage button)')).toEqual([]);
        },
      );
      And(
        'the summary line shows "T-12 · score 94" in English and "T-12 · 94 分" in Chinese',
        () => {
          const summaryLink = () => one<HTMLAnchorElement>('.thread a.tcard[href="#m-k-sum"]');
          expect(summaryLink().textContent).toBe('T-12 · score 94');
          prefs.lang = 'zh-CN';
          flushSync();
          expect(summaryLink().textContent).toBe('T-12 · 94 分');
          prefs.lang = 'en';
          flushSync();
        },
      );
      When('the reader clicks a main-line card link in the thread', () => {
        scrolled = undefined;
        const target = one('#m-k-ap');
        target.scrollIntoView = (arg) => {
          scrolled = arg;
        };
        one<HTMLAnchorElement>('.thread a.tcard[href="#m-k-ap"]').click();
        flushSync();
      });
      Then(
        'the hash route stays on the chat of "Website" and scrolls to the main-line message',
        () => {
          expect(location.hash).toBe(`#/groups/${website}`);
          expect(route.page).toBe('chat');
          expect(route.params).toEqual({ g: website });
          expect(texts('.chat-head h2')).toEqual(['Website']);
          expect(scrolled).toEqual({ behavior: 'smooth', block: 'center' });
          expect(one('#m-k-ap').contains(document.activeElement)).toBe(true);
        },
      );
      And('the question marks the mention "@Secretary" and the code span "path"', () => {
        const q = rowWith('should path carry rtt?');
        expect([...q.querySelectorAll('.text .at')].map((el) => el.textContent?.trim())).toEqual([
          '@Secretary',
        ]);
        expect([...q.querySelectorAll('.text code')].map((el) => el.textContent?.trim())).toEqual([
          'path',
        ]);
      });
      And(
        'the report metadata shows PR 31, short commit "9f2c1ab", "4 files" and "+212 −18"',
        () => {
          const meta = rowWith('PR #31')
            .querySelector('.meta')!
            .textContent!.replace(/\s+/g, ' ')
            .trim();
          expect(meta).toBe('PR #31 · 9f2c1ab · 4 files · +212 −18');
          const localMeta = rowWith('Updated local report.')
            .querySelector('.meta')!
            .textContent!.replace(/\s+/g, ' ')
            .trim();
          expect(localMeta).toBe('1234567 · 1 file · +10 −2');
        },
      );
      And(
        'the check rows show passing local checks, failing cloud checks and a merge conflict',
        () => {
          const localOk = rowWith('Local checks passed');
          expect(localOk.querySelector<HTMLElement>('.glyph')?.dataset.tone).toBe('ok');
          expect(localOk.textContent).toContain('go test ./... (42 tests)');
          const cloudFail = rowWith('Cloud checks failed');
          expect(cloudFail.querySelector<HTMLElement>('.glyph')?.dataset.tone).toBe('danger');
          expect(cloudFail.textContent).toContain('ci/linux (exit 1)');
          const mergeFail = rowWith('Merge conflict');
          expect(mergeFail.textContent).toContain('CONFLICT in path.go');
        },
      );
      And('the bottom of the thread shows "Codex is working · 6 min"', () => {
        expect(texts('.ti.working p')).toEqual(['Codex is working · 6 min']);
        home.write(
          website,
          example('row-member-ai', {
            id: codexId,
            g: website,
            name: 'Codex',
            cli: 'codex',
            state: 'working',
            task: taskId,
            since: Date.now() - 30_000,
          }),
        );
        flushSync();
        expect(texts('.ti.working p')).toEqual(['Codex is working']);
      });
      When('"Codex" works on another task or is waiting', () => {
        home.write(
          website,
          example('row-member-ai', {
            id: codexId,
            g: website,
            name: 'Codex',
            cli: 'codex',
            state: 'working',
            task: 'other-task',
            since: Date.now(),
          }),
        );
        flushSync();
        expect(all('.ti.working')).toEqual([]);
        home.write(
          website,
          example('row-member-ai', {
            id: codexId,
            g: website,
            name: 'Codex',
            cli: 'codex',
            state: 'waiting',
            task: taskId,
            since: Date.now(),
          }),
        );
        flushSync();
      });
      Then('the working indicator is gone', () => {
        expect(all('.ti.working')).toEqual([]);
      });
    },
  );

  Scenario(
    'Review blocks show pass, sent-back and veto states with round numbers and file locations',
    ({ Given, When, Then, And }) => {
      Given(
        'the thread of task 12 holds a sent-back review, a veto review and a passed review',
        () => {
          home.write(
            website,
            example('row-msg-review', {
              id: 'rev-fail',
              thread: taskId,
              body: {
                task: taskId,
                num: 12,
                title: 'Show connection path',
                commit: '9f2c1abf43',
                round: 1,
                score: 86,
                pass: 90,
                ok: false,
                items: [
                  { text: 'Missing `relay` event', veto: false, file: 'path.go', line: 88 },
                  { text: 'Unused field', veto: false, file: 'conn.go' },
                ],
              },
            }),
          );
          home.write(
            website,
            example('row-msg-review', {
              id: 'rev-veto',
              thread: taskId,
              body: {
                task: taskId,
                num: 12,
                title: 'Show connection path',
                commit: '9f2c1abf43',
                round: 2,
                score: 0,
                pass: 90,
                ok: false,
                items: [{ text: 'Leaked token in fixture', veto: true }],
              },
            }),
          );
          home.write(
            website,
            example('row-msg-review', {
              id: 'rev-ok',
              thread: taskId,
              body: {
                task: taskId,
                num: 12,
                title: 'Show connection path',
                commit: 'def4567890',
                round: 3,
                score: 94,
                pass: 90,
                ok: true,
                items: [],
              },
            }),
          );
        },
      );
      When('the thread of task 12 is mounted', async () => {
        show(Thread, { group, task });
        await vi.waitFor(() => expect(group.paging(taskId).more).toBe(false));
        flushSync();
      });
      Then(
        'the sent-back review has tone "danger", badge "Review · Round 1 · Sent back", score "86", "Pass 90" and locations "path.go:88" and "conn.go"',
        () => {
          const items = all('.tlist > .ti');
          const fail = items[0];
          expect(fail.querySelector('.ti-head .badge')?.textContent).toBe(
            'Review · Round 1 · Sent back',
          );
          expect(fail.querySelector<HTMLElement>('.ti-head .badge')?.dataset.tone).toBe('danger');
          expect(fail.querySelector<HTMLElement>('.review')?.dataset.tone).toBe('danger');
          expect(fail.querySelector('.score')?.textContent!.replace(/\s+/g, ' ').trim()).toBe(
            '86Pass 90',
          );
          expect(
            [...fail.querySelectorAll('.bul code')].map((el) => el.textContent?.trim()),
          ).toEqual(['relay', 'path.go:88', 'conn.go']);
        },
      );
      And('the veto review shows the "Veto" badge and score "0"', () => {
        const veto = all('.tlist > .ti')[1];
        expect([...veto.querySelectorAll('.veto')].map((el) => el.textContent?.trim())).toEqual([
          'Veto',
        ]);
        expect(veto.querySelector('.score b')?.textContent).toBe('0');
      });
      And(
        'the passed review has tone "ok", badge "Review · Round 3 · Passed" and score "94"',
        () => {
          const passed = all('.tlist > .ti')[2];
          expect(passed.querySelector('.ti-head .badge')?.textContent).toBe(
            'Review · Round 3 · Passed',
          );
          expect(passed.querySelector<HTMLElement>('.ti-head .badge')?.dataset.tone).toBe('ok');
          expect(passed.querySelector<HTMLElement>('.review')?.dataset.tone).toBe('ok');
          expect(passed.querySelector('.score b')?.textContent).toBe('94');
          expect(passed.querySelector('.summary')?.textContent).toContain('def4567');
        },
      );
    },
  );

  Scenario(
    "Task merges appear under the secretary's name, 1.0 system codes have client wording and postponed codes fall back cleanly",
    ({ Given, When, Then, And }) => {
      const activeCodes = [
        'task_merged',
        'task_failed',
        'turn_interrupted',
        'login_invalid',
        'session_replaced',
        'no_secretary',
        'rounds_limit',
        'preempted',
        'rate_limited',
        'group_paused',
        'group_resumed',
        'member_joined',
        'member_removed',
      ];

      Given(
        'the thread of task 12 holds a task_merged notice, every 1.0 system code and a postponed system code',
        () => {
          const bodies: p.MsgRow['body'][] = [
            {
              code: 'task_merged',
              task: taskId,
              commit: '9f2c1abf43f3',
              merge_commit: 'def4567890ab',
            },
            { code: 'task_failed', task: taskId, reason: 'failures' },
            { code: 'task_failed', task: taskId, reason: 'review_rounds' },
            {
              code: 'turn_interrupted',
              ai: codexId,
              reason: 'crash',
              task: taskId,
              detail: 'signal 9',
            },
            { code: 'turn_interrupted', ai: codexId, reason: 'timeout' },
            { code: 'turn_interrupted', ai: codexId, reason: 'stop' },
            { code: 'login_invalid', ai: codexId, detail: 'expired token' },
            { code: 'session_replaced', ai: codexId, task: taskId },
            { code: 'no_secretary' },
            { code: 'rounds_limit', task: taskId, n: 10 },
            { code: 'rounds_limit', task: 'paused-task', n: 3 },
            { code: 'preempted', task: taskId, by: 'mina' },
            { code: 'rate_limited', ai: codexId, resets: new Date(2026, 10, 22, 15).getTime() },
            { code: 'rate_limited', ai: codexId, resets: new Date(2026, 10, 23, 3).getTime() },
            { code: 'group_paused', by: 'mina' },
            { code: 'group_resumed', by: 'mina' },
            { code: 'member_joined', member: 'mina', by: owner },
            { code: 'member_removed', user: 'pat-user', name: "$& $'", by: 'mina' },
            { code: 'member_removed', user: 'sam-user', name: 'Sam' },
            { code: 'member_joined', member: 'gone', by: 'gone' },
            { code: 'cap_reached', ai: codexId, cap: 'day' },
          ];
          const ts = new Date(2026, 10, 22, 9, 58).getTime();
          home.write(
            website,
            example('row-member-human', { id: 'mina', name: 'Mina', user: 'mina-user' }),
          );
          home.write(
            website,
            example('row-task', {
              id: 'paused-task',
              num: 14,
              state: 'paused',
              wait: { kind: 'rounds', since: ts },
            }),
          );
          bodies.forEach((body, i) => {
            home.write(
              website,
              example('row-msg-system', { id: `sys-${i}`, thread: taskId, ts, body }),
            );
          });
        },
      );
      And('the main line also holds a task_merged notice', () => {
        home.write(
          website,
          example('row-msg-system', {
            id: 'sys-main-merged',
            thread: null,
            main: true,
            body: {
              code: 'task_merged',
              task: taskId,
              commit: '9f2c1abf43f3',
              merge_commit: 'def4567890ab',
            },
          }),
        );
      });
      When('the thread of task 12 is mounted', async () => {
        show(Thread, { group, task });
        await vi.waitFor(() => expect(group.paging(taskId).more).toBe(false));
        flushSync();
      });
      Then(
        'the task_merged row names "Secretary", "T-12", "Show connection path" and commits "9f2c1ab → def4567" with an ok check icon',
        () => {
          const merged = all('.tlist > .ti.sys')[0];
          expect(merged.querySelector('.glyph')?.getAttribute('data-tone')).toBe('ok');
          const text = merged.textContent!.replace(/\s+/g, ' ').trim();
          expect(text).toContain('Secretary merged T-12 · Show connection path');
          expect(text).toContain('9f2c1ab → def4567');
        },
      );
      And('every 1.0 system row is worded in English and the postponed row shows its code', () => {
        const rows = texts('.tlist .ti.sys');
        const rendered = rows.join('\n');
        for (const code of activeCodes) expect(rendered).not.toContain(code);
        expect(rendered).toContain('Task T-12 stopped after repeated failures.');
        expect(rendered).toContain('Task T-12 stopped after repeated failed reviews.');
        expect(rendered).toContain("Codex's turn crashed. · T-12 · signal 9");
        expect(rendered).toContain("Codex's turn timed out.");
        expect(rendered).toContain("Codex's turn was stopped.");
        expect(rendered).toContain("Codex's sign-in is no longer valid. · expired token");
        expect(rendered).toContain('Codex started a new session from the task notes. · T-12');
        expect(rendered).toContain('This group has no secretary AI.');
        expect(rows.slice(9)).toEqual(
          [
            'T-12 is paused: the AIs went 10 rounds without a person. Reply in its thread to carry on.',
            'T-14 is paused: the AIs went 3 rounds without a person. Reply in its thread to carry on.',
            'Mina jumped the queue. T-12 is paused and carries on afterwards.',
            'Codex is out of quota until 15:00, then carries on.',
            'Codex is out of quota until Nov 23, 03:00, then carries on.',
            'Mina paused the group. All its AIs stop for now.',
            'Mina resumed the group. Its AIs carry on.',
            'Mina joined, invited by Alex.',
            "Mina removed $& $' from the group.",
            'Sam left the group.',
            'Former member joined, invited by Former member.',
            'cap_reached',
          ].map((line) => `${line} · 09:58`),
        );
      });
      And('only the round limit of a task that waits for a person is warm', () => {
        const tones = all<HTMLElement>('.tlist .ti.sys .glyph').map((el) => el.dataset.tone);
        expect(tones.slice(9, 20)).toEqual([undefined, 'dawn', ...Array(9).fill(undefined)]);
      });
      And(
        'switching the language to Chinese words task_merged under "Secretary" as "已合并 T-12" and every other row in Chinese',
        () => {
          prefs.lang = 'zh-CN';
          flushSync();
          expect(all('.tlist > .ti.sys')[0].textContent).toContain('Secretary 已合并 T-12');
          expect(texts('.tlist .ti.sys').slice(9, 20)).toEqual(
            [
              'T-12 已暂停：AI 之间来回了 10 轮，没有人参与。在话题里说句话就会继续。',
              'T-14 已暂停：AI 之间来回了 3 轮，没有人参与。在话题里说句话就会继续。',
              'Mina 插队了，T-12 先暂停，之后自动继续。',
              'Codex 的额度用完了，15:00 重置后继续。',
              'Codex 的额度用完了，11月23日 03:00 重置后继续。',
              'Mina 暂停了这个群，AI 都先停下。',
              'Mina 恢复了这个群，AI 继续干活。',
              'Alex 邀请 Mina 加入了群。',
              "Mina 把 $& $' 移出了群。",
              'Sam 退出了群。',
              '已离开的成员 邀请 已离开的成员 加入了群。',
            ].map((line) => `${line} · 09:58`),
          );
          prefs.lang = 'en';
          flushSync();
        },
      );
      When('the reader opens the main line of "Website"', async () => {
        await closePage();
        homes.push(reader.store);
        openPage(`#/groups/${website}`);
      });
      Then('the main line also renders the task_merged notice under "Secretary"', () => {
        const lineMerged = one('#m-sys-main-merged');
        expect(lineMerged.textContent!.replace(/\s+/g, ' ')).toContain(
          'Secretary merged T-12 · Show connection path · 9f2c1ab → def4567',
        );
      });
    },
  );

  Scenario(
    "The states sheet renders every wire example with the thread area's mounted cards",
    ({ When, Then, And }) => {
      When('the reader opens "#/states" in a development server', async () => {
        await closePage();
        vi.stubEnv('MODE', 'development');
        openPage('#/states');
        await vi.waitFor(() => {
          flushSync();
          expect(all('.states .kind').length).toBeGreaterThan(0);
        });
      });
      Then(
        'the states sheet marks "review", "checks", "system" and "report" as mounted by "thread"',
        () => {
          for (const kind of ['review', 'checks', 'system', 'report']) {
            expect(cards.get(kind)?.area).toBe('thread');
            const section = one(`#states-${kind}`).closest('section')!;
            expect(section.querySelector('header .badge')?.textContent).toBe('Mounted by thread');
          }
        },
      );
      And(
        'none of "task", "approval", "brief", "decision", "summary", "call" or "handback" is mounted by "thread"',
        () => {
          for (const kind of [
            'task',
            'approval',
            'brief',
            'decision',
            'summary',
            'call',
            'handback',
          ]) {
            expect(cards.get(kind)?.area).not.toBe('thread');
          }
        },
      );
      And('every message wire example is rendered on the sheet', () => {
        const msgExamples = examples.filter((name) => name.startsWith('row-msg-')).sort();
        expect(texts('.states figure figcaption')).toEqual(msgExamples);
      });
    },
  );
});
