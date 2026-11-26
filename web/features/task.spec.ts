import { describeFeature, loadFeature } from '@amiceli/vitest-cucumber';
import { flushSync } from 'svelte';
import { expect, vi } from 'vitest';
import { homes } from '../src/lib/groups.svelte';
import { prefs } from '../src/lib/prefs.svelte';
import type * as p from '../src/lib/proto';
import { route } from '../src/lib/route.svelte';
import { browser, example, TestHome } from './home';
import { all, closePage, one, openPage, texts } from './page';

const feature = await loadFeature('../features/web_task.feature');

const website = 'rZNvy-1jH6Z-BcPqA5U5BQ';
const claudeId = 'LXCZmuGAXkvO-bSrOkuCfw';
const codexId = '1PC8WineBrUQ-apCjx7tug';
const task12 = 'DrtCn6htSBwmMPrFPbHJHA';
const task13 = 'R0gyAncJnBJT8iCk5-s78A';
const task14 = '6CnN7e5G1xM63en6iUJ6ew';
const task15 = 'task-15-add-path-badge';

const stepMarks = () =>
  all('.card .steps i')
    .map((el) => el.getAttribute('data-s') ?? '-')
    .join(', ');

const kv = (label: string) => {
  const dt = all('.card .kv dt').find((el) => el.textContent?.trim() === label);
  return dt?.nextElementSibling as HTMLElement | undefined;
};

describeFeature(feature, ({ Background, Scenario, AfterEachScenario }) => {
  let home: TestHome;
  let reader: ReturnType<typeof browser>;
  const group = () => reader.store.groups[0];

  async function read() {
    const answered = home.synced + home.groups.size;
    await reader.connect();
    await vi.waitFor(() => expect(home.synced).toBe(answered));
    await new Promise((applied) => setTimeout(applied));
    flushSync();
  }

  function taskMsg(id: string, task: string) {
    return example('row-msg-task', { id, body: { task } });
  }

  AfterEachScenario(async () => {
    prefs.lang = 'en';
    vi.unstubAllEnvs();
    await closePage();
  });

  Background(({ Given }) => {
    Given(
      'the chat of the group "Website" open in a browser of its owner, set to English',
      async () => {
        home = new TestHome();
        home.add(website, 'Website');
        home.write(website, example('row-member-ai'));
        home.write(
          website,
          example('row-member-ai', { id: claudeId, name: 'Claude Code', cli: 'claude' }),
        );
        home.write(
          website,
          example('row-member-human', { id: 'mina', name: 'Mina', user: 'mina-user' }),
        );
        reader = browser(home);
        homes.push(reader.store);
        await read();
        openPage(`#/groups/${website}`);
        expect(texts('.chat-head h2')).toEqual(['Website']);
      },
    );
  });

  Scenario(
    'A task message shows a minimal card until its task row arrives, and when it is removed',
    ({ When, Then }) => {
      When(
        'the home writes a task message for "DrtCn6htSBwmMPrFPbHJHA" before the task row arrives',
        () => {
          home.write(website, example('row-msg-task'));
          flushSync();
        },
      );
      Then('the main line shows a minimal task card naming "DrtCn6htSBwmMPrFPbHJHA"', () => {
        expect(texts('.msg.sys .card-head')).toEqual(['Task DrtCn6htSBwmMPrFPbHJHA']);
        expect(all('.card .steps')).toEqual([]);
      });
      When('the home writes the wire example "row-task"', () => {
        home.write(website, example('row-task'));
        flushSync();
      });
      Then('the task card shows "T-12" and "Show connection path"', () => {
        expect(texts('.card-head .mono')).toEqual(['T-12']);
        expect(texts('.card-head h3')).toEqual(['Show connection path']);
      });
      When('the home removes the task row "DrtCn6htSBwmMPrFPbHJHA"', () => {
        home.remove(website, 'task', task12);
        flushSync();
      });
      Then(
        'the main line shows the minimal task card naming "DrtCn6htSBwmMPrFPbHJHA" again',
        () => {
          expect(texts('.msg.sys .card-head')).toEqual(['Task DrtCn6htSBwmMPrFPbHJHA']);
        },
      );
    },
  );

  Scenario(
    'A task sent back by review shows its three-step bar, folder, review verdict and thread preview',
    ({ When, Then, And }) => {
      When('the home writes the wire examples "row-task" and "row-msg-task"', () => {
        home.write(website, example('row-task'));
        home.write(website, example('row-msg-task'));
        flushSync();
      });
      Then(
        'the task card shows "T-12", "Show connection path" and the danger badge "Sent back · round 1"',
        () => {
          expect(texts('.card-head .mono')).toEqual(['T-12']);
          expect(texts('.card-head h3')).toEqual(['Show connection path']);
          const badge = one('.card-head .badge');
          expect(badge.textContent?.trim()).toBe('Sent back · round 1');
          expect(badge.getAttribute('data-tone')).toBe('danger');
          expect(badge.classList.contains('solid')).toBe(false);
        },
      );
      And('the three-step bar marks "done, now, -" and reads "Review · 2 / 3"', () => {
        expect(stepMarks()).toBe('done, now, -');
        expect(texts('.card .steps span')).toEqual(['Review · 2 / 3']);
        expect(one('.card .steps').getAttribute('aria-label')).toBe(
          'Progress: step 2 of 3, Review (Sent back · round 1)',
        );
      });
      And('the metadata shows owner "Codex · workstation" and folder "/home/dev/website"', () => {
        expect(kv('Owner')?.textContent?.replace(/\s+/g, ' ').trim()).toBe('Codex · workstation');
        expect(kv('Folder')?.textContent).toBe('/home/dev/website');
        const dts = one('.card .steps').closest('.card')!.querySelectorAll('.kv dt');
        expect([...dts].map((dt) => dt.textContent)).toEqual(['Owner', 'Folder', 'Review']);
      });
      And('the review line shows "Round 1 · 86, sent back" and "Claude Code · pass 90"', () => {
        expect(kv('Review')?.querySelector('.res')?.getAttribute('data-tone')).toBe('danger');
        expect(kv('Review')?.querySelector('.res')?.textContent?.trim()).toBe(
          'Round 1 · 86, sent back',
        );
        expect(kv('Review')?.querySelector('.muted')?.textContent?.trim()).toBe(
          'Claude Code · pass 90',
        );
      });
      And(
        'the folded thread summary shows "Thread · 7" and "Latest: Codex · Added the path indicator."',
        () => {
          expect(one<HTMLDetailsElement>('.card details.fold').open).toBe(false);
          expect(texts('.card .fold summary b')).toEqual(['Thread · 7']);
          expect(texts('.card .fold summary .grow')).toEqual([
            'Latest: Codex · Added the path indicator.',
          ]);
        },
      );
      When('the reader switches the language to Chinese and expands the thread fold', () => {
        prefs.lang = 'zh-CN';
        one<HTMLElement>('.card .fold summary').click();
        one<HTMLDetailsElement>('.card details.fold').open = true;
        flushSync();
      });
      Then(
        'the task card shows "打回 · 第 1 轮", "评审 · 2 / 3", "第 1 轮 86 分，打回" and an open thread fold',
        () => {
          expect(texts('.card-head .badge')).toEqual(['打回 · 第 1 轮']);
          expect(texts('.card .steps span')).toEqual(['评审 · 2 / 3']);
          expect(kv('评审')?.querySelector('.res')?.textContent?.trim()).toBe(
            '第 1 轮 86 分，打回',
          );
          expect(texts('.card .fold summary b')).toEqual(['话题 · 7 条']);
          expect(one<HTMLDetailsElement>('.card details.fold').open).toBe(true);
        },
      );
    },
  );

  Scenario('A task moves from queued through work and review to done', ({ When, Then }) => {
    const baseTask = (changes: Record<string, unknown> = {}) => {
      const raw = { ...example('row-task'), id: task15, num: 15, title: 'Add path badge' };
      delete (raw as Record<string, unknown>).review;
      delete (raw as Record<string, unknown>).report;
      return {
        ...raw,
        state: 'queued',
        step: 'work',
        wait: { kind: 'depends', since: 1763600000000 },
        depends: [task12],
        thread: { count: 0, rounds: 0 },
        ...changes,
      } as p.Row & Record<string, unknown>;
    };
    const holds = () => texts('#m-msg-t15 .hold p');

    When('a queued task "T-15" "Add path badge" waits for T-12', () => {
      home.write(website, example('row-task'));
      home.write(website, baseTask());
      home.write(website, taskMsg('msg-t15', task15));
      flushSync();
    });
    Then(
      'the task card shows badge "Queued", steps "-, -, -" with "Queued · 0 / 3", "Waits for T-12 to be done." and "Not started · Claude Code to review · pass 90"',
      () => {
        expect(texts('.card-head .badge')).toEqual(['Queued']);
        expect(stepMarks()).toBe('-, -, -');
        expect(texts('.card .steps span')).toEqual(['Queued · 0 / 3']);
        expect(one('.card .steps').getAttribute('aria-label')).toBe(
          'Progress: 0 of 3 steps, Queued',
        );
        expect(holds()).toEqual(['Waits for T-12 to be done.']);
        expect(kv('Review')?.textContent?.trim()).toBe(
          'Not started · Claude Code to review · pass 90',
        );
      },
    );
    When('the task becomes active at step "work"', () => {
      home.write(website, baseTask({ state: 'active', step: 'work', wait: null }));
      flushSync();
    });
    Then(
      'the task card shows working badge "In progress" and steps "now, -, -" with "Work · 1 / 3"',
      () => {
        const badge = one('.card-head .badge');
        expect(badge.textContent?.trim()).toBe('In progress');
        expect(badge.getAttribute('data-tone')).toBe('sky');
        expect(badge.querySelector('.st')?.getAttribute('data-st')).toBe('working');
        expect(stepMarks()).toBe('now, -, -');
        expect(texts('.card .steps span')).toEqual(['Work · 1 / 3']);
        expect(holds()).toEqual([]);
      },
    );
    When('the assignee submits its report and the task is at step "review"', () => {
      home.write(
        website,
        baseTask({ state: 'active', step: 'review', wait: null, report: 'report-t15' }),
      );
      flushSync();
    });
    Then('the three-step bar marks "done, now, -" with "Review · 2 / 3"', () => {
      expect(stepMarks()).toBe('done, now, -');
      expect(texts('.card .steps span')).toEqual(['Review · 2 / 3']);
    });
    When('the review passes with score 94 in round 2, and the task is done', () => {
      home.write(
        website,
        baseTask({
          state: 'done',
          step: 'review',
          wait: null,
          report: 'report-t15',
          review: {
            round: 2,
            failed: 1,
            score: 94,
            ok: true,
            by: claudeId,
            report: 'report-t15',
            at: 1763600005000,
          },
        }),
      );
      flushSync();
    });
    Then(
      'the task card shows ok badge "Done · 94", steps "done, done, done" with "Done · 3 / 3" and the review line "Round 2 · 94, passed"',
      () => {
        const badge = one('.card-head .badge');
        expect(badge.textContent?.trim()).toBe('Done · 94');
        expect(badge.getAttribute('data-tone')).toBe('ok');
        expect(stepMarks()).toBe('done, done, done');
        expect(texts('.card .steps span')).toEqual(['Done · 3 / 3']);
        expect(kv('Review')?.querySelector('.res')?.getAttribute('data-tone')).toBe('ok');
        expect(kv('Review')?.querySelector('.res')?.textContent?.trim()).toBe(
          'Round 2 · 94, passed',
        );
      },
    );
  });

  Scenario(
    'A task waits for its folder and starts when the folder is free',
    ({ Given, When, Then }) => {
      const card = () => one('#m-msg-t15');
      const t15 = (changes: Record<string, unknown> = {}) =>
        ({
          ...example('row-task'),
          id: task15,
          num: 15,
          title: 'Write the usage note',
          state: 'queued',
          step: 'work',
          wait: { kind: 'busy', since: 1763600000000 },
          review: undefined,
          report: undefined,
          thread: { count: 0, rounds: 0 },
          ...changes,
        }) as p.Row & Record<string, unknown>;
      Given('Codex works on T-12 in "/home/dev/website"', () => {
        home.write(website, example('row-task', { state: 'active', step: 'work' }));
        flushSync();
        expect(group().member(codexId)).toMatchObject({
          name: 'Codex',
          workspace: '/home/dev/website',
        });
      });
      When(
        'the home writes T-15 for Codex, queued until its folder is free, and a task message for it',
        () => {
          home.write(website, t15());
          home.write(website, taskMsg('msg-t15', task15));
          flushSync();
        },
      );
      Then(
        'the card of T-15 shows "Queued", the folder "/home/dev/website" and "Waits for its folder, which T-12 is using."',
        () => {
          expect(card().querySelector('.card-head .badge')?.textContent?.trim()).toBe('Queued');
          expect(card().querySelector('.kv code')?.textContent).toBe('/home/dev/website');
          expect(
            [...card().querySelectorAll('.hold p')].map((el) => el.textContent!.trim()),
          ).toEqual(['Waits for its folder, which T-12 is using.']);
        },
      );
      When('T-12 is done and the home starts T-15', () => {
        home.write(website, example('row-task', { state: 'done', step: 'review' }));
        home.write(website, t15({ state: 'active', wait: null }));
        flushSync();
      });
      Then('the card of T-15 shows "In progress" with "Work · 1 / 3" and no longer waits', () => {
        expect(card().querySelector('.card-head .badge')?.textContent?.trim()).toBe('In progress');
        expect(card().querySelector('.steps span')?.textContent).toBe('Work · 1 / 3');
        expect(card().querySelectorAll('.hold')).toHaveLength(0);
      });
    },
  );

  Scenario(
    "A task held for its AI's owner asks that owner, and shows when the owner has taken over its terminal",
    ({ When, Then }) => {
      const held = example('row-task-confirm') as p.TaskRow & Record<string, unknown>;
      const card = held.wait!.msg!;
      const badge = () => one('.card-head .badge');
      const mark = () => all('.card .steps i')[0].getAttribute('data-s');
      When(
        'the home writes "row-task-confirm", its pending confirmation and a task message for it',
        () => {
          home.write(website, held);
          home.write(website, example('row-msg-confirm-pending', { id: card }));
          home.write(website, taskMsg('msg-t15', held.id));
          flushSync();
        },
      );
      Then(
        'the task card shows solid dawn badge "Waiting for your confirmation", step "wait" and a link to the confirmation',
        () => {
          const task = one('#m-msg-t15');
          const pill = task.querySelector<HTMLElement>('.card-head .badge')!;
          expect(pill.textContent?.trim()).toBe('Waiting for your confirmation');
          expect(pill.dataset.tone).toBe('dawn');
          expect(pill.classList.contains('solid')).toBe(true);
          expect(task.querySelector('.steps i')?.getAttribute('data-s')).toBe('wait');
          const link = task.querySelector<HTMLAnchorElement>('.kv a')!;
          expect(link.getAttribute('href')).toBe(`#m-${card}`);
          expect(link.textContent?.trim()).toBe('Waiting for your confirmation');
          home.remove(website, 'msg', card);
          flushSync();
        },
      );
      When('the confirmation waits on "Mina" instead', () => {
        home.write(website, { ...held, wait: { ...held.wait!, who: ['mina-user'] } });
        flushSync();
      });
      Then('the task card shows neutral "Waiting for Mina\'s confirmation" with step "now"', () => {
        expect(badge().textContent?.trim()).toBe("Waiting for Mina's confirmation");
        expect(badge().dataset.tone).toBeUndefined();
        expect(badge().classList.contains('solid')).toBe(false);
        expect(mark()).toBe('now');
      });
      When(
        "the card is confirmed and the assignee's owner takes over its terminal for this task",
        () => {
          home.write(website, { ...held, wait: null });
          home.write(
            website,
            example('row-member-ai', { id: held.assignee, state: 'takeover', task: held.id }),
          );
          flushSync();
        },
      );
      Then(`the task card shows the takeover badge "AI's owner is driving" with step "now"`, () => {
        expect(badge().textContent?.trim()).toBe("AI's owner is driving");
        expect(badge().querySelector<HTMLElement>('.st')?.dataset.st).toBe('takeover');
        expect(badge().dataset.tone).toBeUndefined();
        expect(mark()).toBe('now');
      });
    },
  );

  Scenario(
    'Waiting tasks distinguish waits on the reader from waits on another person or a reviewer',
    ({ When, Then }) => {
      const seen: Array<[string, string | null, boolean, string]> = [];
      let scrolled: unknown;
      let windowClicks = 0;

      When('the home writes "row-task-waiting" and a task message for it', () => {
        home.write(website, example('row-task-waiting'));
        home.write(website, taskMsg('msg-t13', task13));
        flushSync();
      });
      Then(
        'the task card shows solid dawn badge "Waiting for your decision", steps "wait, -, -" with "Work · 1 / 3", and a link to "#m-TDRDfXwvOO-iOegSUoyidg"',
        () => {
          const badge = one('.card-head .badge');
          expect(badge.textContent?.trim()).toBe('Waiting for your decision');
          expect(badge.getAttribute('data-tone')).toBe('dawn');
          expect(badge.classList.contains('solid')).toBe(true);
          expect(stepMarks()).toBe('wait, -, -');
          expect(texts('.card .steps span')).toEqual(['Work · 1 / 3']);
          expect(kv('Waiting')?.textContent?.trim()).toBe('Waiting for your decision');
          expect(kv('Waiting')?.querySelector('a')?.getAttribute('href')).toBe(
            '#m-TDRDfXwvOO-iOegSUoyidg',
          );
          expect(kv('Review')?.textContent?.trim()).toBe('Not started · pass 70');
          expect(texts('.card .fold summary .grow')).toEqual(['No messages yet']);
        },
      );
      When(
        'the task instead waits on an approval from the reader, then from "Mina", then from an unknown user',
        () => {
          const viewer = JSON.parse(JSON.stringify(example('row-task-waiting').wait)).who[0];
          for (const who of [[viewer], ['mina-user'], ['stranger']]) {
            home.write(
              website,
              example('row-task-waiting', {
                wait: { kind: 'approval', msg: 'ap-card-1', who, since: 1763600000000 },
              }),
            );
            flushSync();
            const badge = one('.card-head .badge');
            seen.push([
              badge.textContent?.trim() ?? '',
              badge.getAttribute('data-tone'),
              badge.classList.contains('solid'),
              all('.card .steps i')[0].getAttribute('data-s') ?? '-',
            ]);
          }
        },
      );
      Then(
        'the task card shows solid dawn "Waiting for your approval" with step "wait", then neutral "Waiting for Mina\'s approval" with step "now", then "Waiting for approval"',
        () => {
          expect(seen).toEqual([
            ['Waiting for your approval', 'dawn', true, 'wait'],
            ["Waiting for Mina's approval", null, false, 'now'],
            ['Waiting for approval', null, false, 'now'],
          ]);
        },
      );
      When(
        'the reader clicks "Waiting for your approval" on the task card for main-line card "ap-card-1"',
        () => {
          const viewer = JSON.parse(JSON.stringify(example('row-task-waiting').wait)).who[0];
          home.write(website, example('row-msg-approval-pending', { id: 'ap-card-1', main: true }));
          home.write(
            website,
            example('row-task-waiting', {
              wait: { kind: 'approval', msg: 'ap-card-1', who: [viewer], since: 1763600000000 },
            }),
          );
          flushSync();
          scrolled = undefined;
          windowClicks = 0;
          const onWindowClick = () => {
            windowClicks += 1;
          };
          window.addEventListener('click', onWindowClick);
          one('.scroller').scrollTo = ((arg: ScrollToOptions) => {
            scrolled = arg;
          }) as Element['scrollTo'];
          const link = one<HTMLAnchorElement>('.card .kv a[href="#m-ap-card-1"]');
          expect(link.textContent?.trim()).toBe('Waiting for your approval');
          link.click();
          window.removeEventListener('click', onWindowClick);
          flushSync();
        },
      );
      Then(
        'the chat stays open at the group\'s address, the thread stays closed, and "#m-ap-card-1" scrolls into view',
        () => {
          expect(location.hash).toBe(`#/groups/${website}`);
          expect(route.page).toBe('chat');
          expect(route.params).toEqual({ g: website });
          expect(texts('.chat-head h2')).toEqual(['Website']);
          expect(one<HTMLDetailsElement>('.card details.fold').open).toBe(false);
          expect(windowClicks).toBe(0);
          expect(scrolled).toEqual({ top: expect.any(Number), behavior: 'smooth' });
        },
      );
      When('the task waits for a reviewer', () => {
        home.write(
          website,
          example('row-task-waiting', {
            step: 'review',
            report: 'report-t13',
            wait: { kind: 'reviewer', since: 1763600000000 },
          }),
        );
        flushSync();
      });
      Then('the task card shows "Waiting for a reviewer" at step "review"', () => {
        expect(one('.card-head .badge').textContent?.trim()).toBe('Waiting for a reviewer');
        expect(texts('.card .steps span')).toEqual(['Review · 2 / 3']);
      });
    },
  );

  Scenario('Paused, stopped and cancelled tasks show their state', ({ When, Then }) => {
    let crashBadge = '';
    let crashSteps = '';

    When('the home writes "row-task-paused" and a task message for it', () => {
      home.write(website, example('row-task-paused'));
      home.write(website, taskMsg('msg-t14', task14));
      flushSync();
    });
    Then(
      'the task card shows paused badge "Paused" and steps "done, -, -" with "Review · 2 / 3"',
      () => {
        const badge = one('.card-head .badge');
        expect(badge.textContent?.trim()).toBe('Paused');
        expect(badge.querySelector('.st')?.getAttribute('data-st')).toBe('paused');
        expect(stepMarks()).toBe('done, -, -');
        expect(texts('.card .steps span')).toEqual(['Review · 2 / 3']);
      },
    );
    When(
      'the task stops in state "failed" after 3 consecutive failures, and then after 6 failed reviews',
      () => {
        home.write(
          website,
          example('row-task-paused', {
            state: 'failed',
            step: 'work',
            wait: null,
            failures: 3,
          }),
        );
        flushSync();
        crashBadge = one('.card-head .badge').textContent?.trim() ?? '';
        crashSteps = stepMarks();

        home.write(
          website,
          example('row-task-paused', {
            state: 'failed',
            step: 'review',
            wait: null,
            failures: 0,
            review: {
              round: 6,
              failed: 6,
              score: 82,
              ok: false,
              by: claudeId,
              report: 'report-t14',
              at: 1763600001000,
            },
          }),
        );
        flushSync();
      },
    );
    Then(
      'the task card shows solid danger badge "Stopped after 3 failures" with step "fail", and then "Stopped after 6 failed reviews"',
      () => {
        expect(crashBadge).toBe('Stopped after 3 failures');
        expect(crashSteps).toBe('fail, -, -');
        const badge = one('.card-head .badge');
        expect(badge.textContent?.trim()).toBe('Stopped after 6 failed reviews');
        expect(badge.getAttribute('data-tone')).toBe('danger');
        expect(badge.classList.contains('solid')).toBe(true);
        expect(stepMarks()).toBe('done, fail, -');
      },
    );
    When('the task becomes "cancelled"', () => {
      home.write(
        website,
        example('row-task-paused', { state: 'cancelled', step: 'review', wait: null }),
      );
      flushSync();
    });
    Then('the task card shows "Cancelled" with no active step marker', () => {
      expect(texts('.card-head .badge')).toEqual(['Cancelled']);
      expect(stepMarks()).toBe('done, -, -');
    });
  });

  Scenario(
    'The development states sheet mounts the task card for the wire example',
    ({ Given, When, Then }) => {
      Given('a development server', () => {
        vi.stubEnv('MODE', 'development');
      });
      When('the person opens the client at "#/states"', async () => {
        await closePage();
        openPage('#/states');
        await vi.waitFor(() => {
          flushSync();
          expect(all('.states')).toHaveLength(1);
        });
      });
      Then(
        'the "task" section is marked "Mounted by task" and renders "T-12" "Show connection path" with three steps',
        () => {
          const section = one('#states-task').closest('section')!;
          expect(section.querySelector('header .badge')?.textContent?.trim()).toBe(
            'Mounted by task',
          );
          expect(section.querySelector('.card-head .mono')?.textContent).toBe('T-12');
          expect(section.querySelector('.card-head h3')?.textContent).toBe('Show connection path');
          expect(section.querySelectorAll('.steps i')).toHaveLength(3);
        },
      );
    },
  );
});
