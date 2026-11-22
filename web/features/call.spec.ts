import { describeFeature, loadFeature } from '@amiceli/vitest-cucumber';
import { flushSync } from 'svelte';
import { expect, vi } from 'vitest';
import Thread from '../src/components/thread/Thread.svelte';
import { homes, type Group } from '../src/lib/groups.svelte';
import { cards } from '../src/lib/mounts.svelte';
import { prefs } from '../src/lib/prefs.svelte';
import type * as p from '../src/lib/proto';
import { browser, example, TestHome } from './home';
import { all, closePage, one, openPage, show, texts } from './page';

const feature = await loadFeature('../features/web_call.feature');

const website = 'rZNvy-1jH6Z-BcPqA5U5BQ';
const taskId = 'DrtCn6htSBwmMPrFPbHJHA';
const codexId = '1PC8WineBrUQ-apCjx7tug';
const claudeId = 'LXCZmuGAXkvO-bSrOkuCfw';

describeFeature(feature, ({ Background, Scenario, AfterEachScenario }) => {
  let home: TestHome;
  let reader: ReturnType<typeof browser>;
  let group: Group;
  let task: p.TaskRow;
  let baseNow: number;

  async function syncGroup() {
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
    vi.useRealTimers();
  });

  Background(({ Given }) => {
    Given(
      'a group "Website" with "Claude Code", "Codex" and task 12 "Show connection path"',
      async () => {
        prefs.lang = 'en';
        baseNow = 1_795_326_000_000;
        vi.useFakeTimers({ toFake: ['Date', 'setInterval', 'clearInterval'] });
        vi.setSystemTime(baseNow);
        home = new TestHome();
        home.add(website, 'Website');
        home.write(
          website,
          example('row-member-ai', {
            id: claudeId,
            g: website,
            name: 'Claude Code',
            cli: 'claude',
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
        task = home.write(website, example('row-task', { g: website })) as p.TaskRow;
        reader = browser(home);
        homes.push(reader.store);
        await syncGroup();
        group = reader.store.groups[0];
      },
    );
  });

  Scenario(
    'A thread call transitions from waiting to answered with elapsed time and inline reply',
    ({ Given, When, Then, And }) => {
      Given(
        '"Claude Code" calls "Codex" in task 12 waiting on the spot at depth 1 made 12 seconds ago',
        () => {
          home.write(
            website,
            example('row-msg-call-waiting', {
              id: 'call-1',
              thread: taskId,
              by: claudeId,
              ts: baseNow - 12_000,
              text: '@Codex which fields go on `path`?',
              mentions: [codexId],
              body: { to: codexId, depth: 1, wait: true },
              state: { status: 'waiting' },
            }),
          );
        },
      );
      When('the thread of task 12 is mounted', async () => {
        show(Thread, { group, task });
        await vi.waitFor(() => expect(group.paging(taskId).more).toBe(false));
        flushSync();
      });
      Then('the call row has the sky "Call" badge and shows "Claude Code → Codex"', () => {
        const badge = one('.ti-head .badge');
        expect(badge.textContent).toBe('Call');
        expect(badge.dataset.tone).toBe('sky');
        expect(one('.call-route').textContent!.replace(/\s+/g, ' ').trim()).toContain(
          'Claude Code→Codex',
        );
      });
      And(
        'the call block shows "Waits for answer", waiting status "Waiting · 12 s", "Depth 1" and no reply',
        () => {
          const meta = one('.call-route .muted');
          expect(meta.textContent!.replace(/\s+/g, ' ').trim()).toBe(
            'Waits for answer · Waiting · 12 s · Depth 1',
          );
          expect(meta.querySelector('.st')?.getAttribute('data-st')).toBe('waiting');
          expect(all('.call .reply')).toEqual([]);
        },
      );
      And(
        'the call question appears once inside the call block with mention "@Codex" and code "path"',
        () => {
          expect(texts('.tlist .ti .text')).toEqual(['@Codex which fields go on path?']);
          expect(texts('.call > .text')).toEqual(['@Codex which fields go on path?']);
          expect(texts('.call > .text .at')).toEqual(['@Codex']);
          expect(texts('.call > .text code')).toEqual(['path']);
        },
      );
      When('"Codex" replies to the call after 38 seconds', () => {
        home.write(
          website,
          example('row-msg-reply', {
            id: 'reply-1',
            thread: taskId,
            by: codexId,
            ts: baseNow + 26_000,
            text: 'Add `limited` as a boolean.',
            re: 'call-1',
          }),
        );
        home.write(
          website,
          example('row-msg-call-answered', {
            id: 'call-1',
            thread: taskId,
            by: claudeId,
            ts: baseNow - 12_000,
            text: '@Codex which fields go on `path`?',
            mentions: [codexId],
            body: { to: codexId, depth: 1, wait: true },
            state: { status: 'answered', reply: 'reply-1', ms: 38_000 },
          }),
        );
        flushSync();
      });
      Then(
        'the call block shows "Took 38 s" and folds the reply of "Codex" with code "limited" inside it',
        () => {
          expect(one('.call-route .muted').textContent!.replace(/\s+/g, ' ').trim()).toBe(
            'Waits for answer · Took 38 s · Depth 1',
          );
          expect(all('.call .st')).toEqual([]);
          expect(all('.call .reply .av.ai')).toHaveLength(1);
          expect(texts('.call .reply b')).toEqual(['Codex']);
          expect(texts('.call .reply code')).toEqual(['limited']);
          expect(texts('.call .reply .text')).toEqual(['Codex Add limited as a boolean.']);
        },
      );
      And('the thread has one item and no separate reply row', () => {
        expect(all('.tlist > .ti')).toHaveLength(1);
        expect(texts('.ti-head .badge')).toEqual(['Call']);
      });
      And(
        'switching the language to Chinese words the call metadata as "当场等结果 · 用时 38 秒 · 调用第 1 层"',
        () => {
          prefs.lang = 'zh-CN';
          flushSync();
          expect(one('.call-route .muted').textContent!.replace(/\s+/g, ' ').trim()).toBe(
            '当场等结果 · 用时 38 秒 · 调用第 1 层',
          );
          prefs.lang = 'en';
          flushSync();
        },
      );
    },
  );

  Scenario(
    'An unanswered receipt-first call stays waiting with live elapsed time, and main-line calls fold replies',
    ({ Given, When, Then }) => {
      Given(
        'an unanswered receipt-first call at depth 2 in the main line made 6 minutes ago',
        () => {
          home.write(
            website,
            example('row-msg-call-waiting', {
              id: 'main-wait',
              thread: null,
              by: codexId,
              ts: baseNow - 6 * 60_000,
              text: 'Check the release notes.',
              body: { to: claudeId, depth: 2, wait: false },
              state: { status: 'waiting' },
            }),
          );
        },
      );
      When('the reader opens the main line of "Website"', () => {
        openPage(`#/groups/${website}`);
      });
      Then(
        'the main-line call shows "Gets a receipt", waiting status "Waiting · 6 min", "Depth 2" and no dawn highlight',
        () => {
          const card = one('#m-main-wait .call');
          expect(
            card.querySelector('.call-route .muted')!.textContent!.replace(/\s+/g, ' ').trim(),
          ).toBe('Gets a receipt · Waiting · 6 min · Depth 2');
          expect(all('#m-main-wait .need, #m-main-wait [data-tone="dawn"]')).toEqual([]);
        },
      );
      When('3 minutes pass and a second main-line call is answered in 500 milliseconds', () => {
        baseNow += 3 * 60_000;
        vi.advanceTimersByTime(3 * 60_000);
        home.write(
          website,
          example('row-msg-reply', {
            id: 'main-reply',
            thread: null,
            by: claudeId,
            ts: baseNow,
            text: 'Looks ready.',
            re: 'main-ans',
          }),
        );
        home.write(
          website,
          example('row-msg-call-answered', {
            id: 'main-ans',
            thread: null,
            by: codexId,
            ts: baseNow - 500,
            text: 'Is the summary ready?',
            body: { to: claudeId, depth: 1, wait: true },
            state: { status: 'answered', reply: 'main-reply', ms: 500 },
          }),
        );
        flushSync();
      });
      Then(
        'the unanswered call shows "Waiting · 9 min" and the answered main-line call folds its reply with "Took 500 ms"',
        () => {
          expect(
            one('#m-main-wait .call-route .muted').textContent!.replace(/\s+/g, ' ').trim(),
          ).toBe('Gets a receipt · Waiting · 9 min · Depth 2');
          expect(
            one('#m-main-ans .call-route .muted').textContent!.replace(/\s+/g, ' ').trim(),
          ).toBe('Waits for answer · Took 500 ms · Depth 1');
          expect(texts('#m-main-ans .reply .text')).toEqual(['Claude Code Looks ready.']);
          expect(all('#m-main-reply')).toEqual([]);
        },
      );
    },
  );

  Scenario(
    'The states sheet mounts call under the call area and renders both call wire examples',
    ({ When, Then, And }) => {
      When('the reader opens "#/states" in a development server', async () => {
        vi.stubEnv('MODE', 'development');
        openPage('#/states');
        await vi.waitFor(() => {
          flushSync();
          expect(all('#states-call')).toHaveLength(1);
        });
      });
      Then('the states sheet marks "call" as mounted by "call"', () => {
        expect(cards.get('call')?.area).toBe('call');
        const section = one('#states-call').closest('section')!;
        expect(section.querySelector('header .badge')?.textContent).toBe('Mounted by call');
      });
      And(
        'the sheet renders "row-msg-call-waiting" as waiting and "row-msg-call-answered" with the reply "Use the path change event."',
        () => {
          const section = one('#states-call').closest('section')!;
          const figures = [...section.querySelectorAll('figure')];
          expect(figures.map((f) => f.querySelector('figcaption')?.textContent)).toEqual([
            'row-msg-call-answered',
            'row-msg-call-waiting',
          ]);
          expect(figures[0].querySelector('.reply .text')?.textContent).toContain(
            'Use the path change event.',
          );
          expect(figures[1].querySelector('.st')?.getAttribute('data-st')).toBe('waiting');
        },
      );
    },
  );
});
