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
    'A task sent back by review shows its six-step bar, checks, review verdict and thread preview',
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
      And(
        'the six-step bar marks "done, done, done, done, fail, -" and reads "Review · 5 / 6"',
        () => {
          expect(stepMarks()).toBe('done, done, done, done, fail, -');
          expect(texts('.card .steps span')).toEqual(['Review · 5 / 6']);
          expect(one('.card .steps').getAttribute('aria-label')).toBe(
            'Progress: step 5 of 6, Review (Sent back · round 1)',
          );
        },
      );
      And(
        'the metadata shows owner "Codex · workstation" and branch "task/12-show-connection-path" with link "PR #31"',
        () => {
          expect(kv('Owner')?.textContent?.replace(/\s+/g, ' ').trim()).toBe('Codex · workstation');
          expect(kv('Branch')?.querySelector('code')?.textContent).toBe(
            'task/12-show-connection-path',
          );
          const prLink = kv('Branch')?.querySelector('a');
          expect(prLink?.textContent).toBe('PR #31');
          expect(prLink?.getAttribute('href')).toBe('https://git.example/team/website/pull/31');
        },
      );
      And('the checks line shows "Local checks passed" and "Hosted checks passed"', () => {
        const res = [...(kv('Checks')?.querySelectorAll('.res') ?? [])];
        expect(res.map((el) => [el.getAttribute('data-tone'), el.textContent?.trim()])).toEqual([
          ['ok', 'Local checks passed'],
          ['ok', 'Hosted checks passed'],
        ]);
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
        'the task card shows "打回 · 第 1 轮", "评审 · 5 / 6", "第 1 轮 86 分，打回" and an open thread fold',
        () => {
          expect(texts('.card-head .badge')).toEqual(['打回 · 第 1 轮']);
          expect(texts('.card .steps span')).toEqual(['评审 · 5 / 6']);
          expect(kv('评审')?.querySelector('.res')?.textContent?.trim()).toBe(
            '第 1 轮 86 分，打回',
          );
          expect(texts('.card .fold summary b')).toEqual(['话题 · 7 条']);
          expect(one<HTMLDetailsElement>('.card details.fold').open).toBe(true);
        },
      );
    },
  );

  Scenario(
    'A task moves from queued through each step to merged, with and without a remote forge',
    ({ When, Then }) => {
      const baseTask = (changes: Record<string, unknown> = {}) => {
        const raw = { ...example('row-task'), id: task15, num: 15, title: 'Add path badge' };
        delete (raw as Record<string, unknown>).branch;
        delete (raw as Record<string, unknown>).pr;
        delete (raw as Record<string, unknown>).pr_url;
        delete (raw as Record<string, unknown>).review;
        delete (raw as Record<string, unknown>).commit;
        return {
          ...raw,
          slug: 'add-path-badge',
          state: 'queued',
          step: 'branch',
          checks: {},
          wait: { kind: 'depends', since: 1763600000000 },
          depends: [task12],
          thread: { count: 0, rounds: 0 },
          ...changes,
        } as p.Row & Record<string, unknown>;
      };

      let stepAfterFail = '';
      let checkFailText = '';

      When('a queued task "T-15" "Add path badge" is created without a branch yet', () => {
        home.write(website, baseTask());
        home.write(website, taskMsg('msg-t15', task15));
        flushSync();
      });
      Then(
        'the task card shows badge "Queued", steps "-, -, -, -, -, -" with "0 / 6", no branch row, "Not run yet" and "Not started · Claude Code to review · pass 90"',
        () => {
          expect(texts('.card-head .badge')).toEqual(['Queued']);
          expect(stepMarks()).toBe('-, -, -, -, -, -');
          expect(texts('.card .steps span')).toEqual(['0 / 6']);
          expect(one('.card .steps').getAttribute('aria-label')).toBe(
            'Progress: 0 of 6 steps, Queued',
          );
          expect(kv('Branch')).toBeUndefined();
          expect(kv('Checks')?.textContent?.trim()).toBe('Not run yet');
          expect(kv('Review')?.textContent?.trim()).toBe(
            'Not started · Claude Code to review · pass 90',
          );
        },
      );
      When('the task becomes active at step "branch"', () => {
        home.write(website, baseTask({ state: 'active', step: 'branch', wait: null }));
        flushSync();
      });
      Then(
        'the task card shows working badge "In progress" and steps "now, -, -, -, -, -" with "Work · 1 / 6"',
        () => {
          const badge = one('.card-head .badge');
          expect(badge.textContent?.trim()).toBe('In progress');
          expect(badge.getAttribute('data-tone')).toBe('sky');
          expect(badge.querySelector('.st')?.getAttribute('data-st')).toBe('working');
          expect(stepMarks()).toBe('now, -, -, -, -, -');
          expect(texts('.card .steps span')).toEqual(['Work · 1 / 6']);
          expect(kv('Branch')).toBeUndefined();
        },
      );
      When(
        'the task advances to "local_check" on branch "task/15-add-path-badge" with a failing local check, then a passing one',
        () => {
          home.write(
            website,
            baseTask({
              state: 'active',
              step: 'local_check',
              wait: null,
              branch: 'task/15-add-path-badge',
              checks: { local: { ok: false, at: 1763600001000, commit: 'abc1234' } },
            }),
          );
          flushSync();
          stepAfterFail = stepMarks();
          checkFailText = kv('Checks')?.textContent?.trim() ?? '';

          home.write(
            website,
            baseTask({
              state: 'active',
              step: 'local_check',
              wait: null,
              branch: 'task/15-add-path-badge',
              checks: { local: { ok: true, at: 1763600002000, commit: 'abc1234' } },
            }),
          );
          flushSync();
        },
      );
      Then(
        'the six-step bar first marks "done, fail, -, -, -, -" with "Local checks failed", then "done, done, now, -, -, -" with "Open PR · 3 / 6" and "Local checks passed"',
        () => {
          expect(kv('Branch')?.textContent?.trim()).toBe('task/15-add-path-badge');
          expect(stepAfterFail).toBe('done, fail, -, -, -, -');
          expect(checkFailText).toBe('Local checks failed');
          expect(stepMarks()).toBe('done, done, now, -, -, -');
          expect(texts('.card .steps span')).toEqual(['Open PR · 3 / 6']);
          expect(kv('Checks')?.textContent?.trim()).toBe('Local checks passed');
        },
      );
      When(
        'the task advances to "cloud_check" on PR 44 without a link yet and a failing hosted check',
        () => {
          home.write(
            website,
            baseTask({
              state: 'active',
              step: 'cloud_check',
              wait: null,
              branch: 'task/15-add-path-badge',
              pr: 44,
              checks: {
                local: { ok: true, at: 1763600002000, commit: 'abc1234' },
                cloud: { ok: false, at: 1763600003000, commit: 'abc1234' },
              },
            }),
          );
          flushSync();
        },
      );
      Then(
        'the branch line shows plain text "PR #44" with no link, and the six-step bar marks "done, done, done, fail, -, -" with "Hosted checks · 4 / 6" and "Hosted checks failed"',
        () => {
          expect(kv('Branch')?.querySelector('a')).toBeNull();
          expect(kv('Branch')?.textContent?.replace(/\s+/g, ' ').trim()).toBe(
            'task/15-add-path-badge · PR #44',
          );
          expect(stepMarks()).toBe('done, done, done, fail, -, -');
          expect(texts('.card .steps span')).toEqual(['Hosted checks · 4 / 6']);
          expect(kv('Checks')?.textContent?.replace(/\s+/g, ' ').trim()).toBe(
            'Local checks passed Hosted checks failed',
          );
        },
      );
      When(
        'hosted checks pass, the pull request URL arrives, the review passes with score 94 in round 2, and the task enters "merge"',
        () => {
          home.write(
            website,
            baseTask({
              state: 'active',
              step: 'merge',
              wait: null,
              branch: 'task/15-add-path-badge',
              pr: 44,
              pr_url: 'https://git.example/pull/44',
              checks: {
                local: { ok: true, at: 1763600002000, commit: 'abc1234' },
                cloud: { ok: true, at: 1763600004000, commit: 'abc1234' },
              },
              review: {
                round: 2,
                failed: 1,
                score: 94,
                ok: true,
                by: claudeId,
                commit: 'abc1234',
                at: 1763600005000,
              },
            }),
          );
          flushSync();
        },
      );
      Then(
        'the six-step bar marks "done, done, done, done, done, now" with "Merge · 6 / 6" and the review line shows "Round 2 · 94, passed"',
        () => {
          expect(kv('Branch')?.querySelector('a')?.getAttribute('href')).toBe(
            'https://git.example/pull/44',
          );
          expect(stepMarks()).toBe('done, done, done, done, done, now');
          expect(texts('.card .steps span')).toEqual(['Merge · 6 / 6']);
          expect(kv('Review')?.querySelector('.res')?.getAttribute('data-tone')).toBe('ok');
          expect(kv('Review')?.querySelector('.res')?.textContent?.trim()).toBe(
            'Round 2 · 94, passed',
          );
        },
      );
      When('the task becomes "merged"', () => {
        home.write(
          website,
          baseTask({
            state: 'merged',
            step: 'merge',
            wait: null,
            branch: 'task/15-add-path-badge',
            pr: 44,
            checks: {
              local: { ok: true, at: 1763600002000, commit: 'abc1234' },
              cloud: { ok: true, at: 1763600004000, commit: 'abc1234' },
            },
            review: {
              round: 2,
              failed: 1,
              score: 94,
              ok: true,
              by: claudeId,
              commit: 'abc1234',
              at: 1763600005000,
            },
          }),
        );
        flushSync();
      });
      Then(
        'the task card shows ok badge "Merged · 94" and steps "done, done, done, done, done, done" with "6 / 6"',
        () => {
          const badge = one('.card-head .badge');
          expect(badge.textContent?.trim()).toBe('Merged · 94');
          expect(badge.getAttribute('data-tone')).toBe('ok');
          expect(stepMarks()).toBe('done, done, done, done, done, done');
          expect(texts('.card .steps span')).toEqual(['6 / 6']);
        },
      );
      When('the group has forge "none"', () => {
        home.write(website, example('row-group', { id: website, name: 'Website', forge: 'none' }));
        flushSync();
      });
      Then(
        'the merged six-step bar skips pull request and hosted checks as "done, done, -, -, done, done"',
        () => {
          expect(stepMarks()).toBe('done, done, -, -, done, done');
          expect(all('.card .steps i').map((el) => el.classList.contains('skip'))).toEqual([
            false,
            false,
            true,
            true,
            false,
            false,
          ]);
        },
      );
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
        'the task card shows solid dawn badge "Waiting for your confirmation", step "wait" and a checks link to the confirmation',
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
      Then('the task card shows the takeover badge "Owner is driving" with step "now"', () => {
        expect(badge().textContent?.trim()).toBe('Owner is driving');
        expect(badge().querySelector<HTMLElement>('.st')?.dataset.st).toBe('takeover');
        expect(badge().dataset.tone).toBeUndefined();
        expect(mark()).toBe('now');
      });
    },
  );

  Scenario(
    'Waiting tasks distinguish waits on the reader from waits on another person, a reviewer or a conflict',
    ({ When, Then }) => {
      const seen: Array<[string, string | null, boolean, string]> = [];
      let reviewerBadge = '';
      let scrolled: unknown;
      let windowClicks = 0;

      When('the home writes "row-task-waiting" and a task message for it', () => {
        home.write(website, example('row-task-waiting'));
        home.write(website, taskMsg('msg-t13', task13));
        flushSync();
      });
      Then(
        'the task card shows solid dawn badge "Waiting for your decision", steps "wait, -, -, -, -, -" with "Work · 1 / 6", and a checks link to "#m-TDRDfXwvOO-iOegSUoyidg"',
        () => {
          const badge = one('.card-head .badge');
          expect(badge.textContent?.trim()).toBe('Waiting for your decision');
          expect(badge.getAttribute('data-tone')).toBe('dawn');
          expect(badge.classList.contains('solid')).toBe(true);
          expect(stepMarks()).toBe('wait, -, -, -, -, -');
          expect(texts('.card .steps span')).toEqual(['Work · 1 / 6']);
          expect(kv('Branch')).toBeUndefined();
          expect(kv('Checks')?.textContent?.replace(/\s+/g, ' ').trim()).toBe(
            'Not run yet · Waiting for your decision',
          );
          expect(kv('Checks')?.querySelector('a')?.getAttribute('href')).toBe(
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
          one('#m-ap-card-1').scrollIntoView = (arg) => {
            scrolled = arg;
          };
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
          expect(scrolled).toEqual({ behavior: 'smooth', block: 'center' });
        },
      );
      When('the task waits for a reviewer, and then hits a merge conflict', () => {
        home.write(
          website,
          example('row-task-waiting', {
            step: 'review',
            pr: 19,
            wait: { kind: 'reviewer', since: 1763600000000 },
          }),
        );
        flushSync();
        reviewerBadge = one('.card-head .badge').textContent?.trim() ?? '';

        home.write(
          website,
          example('row-task-waiting', {
            step: 'local_check',
            pr: 19,
            checks: { local: { ok: true, at: 1763600000000, commit: 'abc1234' } },
            wait: { kind: 'conflict', since: 1763600000000 },
          }),
        );
        flushSync();
      });
      Then(
        'the task card shows "Waiting for a reviewer", and then danger "Merge conflict" with step "fail"',
        () => {
          expect(reviewerBadge).toBe('Waiting for a reviewer');
          const badge = one('.card-head .badge');
          expect(badge.textContent?.trim()).toBe('Merge conflict');
          expect(badge.getAttribute('data-tone')).toBe('danger');
          expect(stepMarks()).toBe('done, fail, -, -, -, -');
          expect(kv('Checks')?.textContent?.replace(/\s+/g, ' ').trim()).toBe(
            'Local checks passed Merge conflict',
          );
        },
      );
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
      'the task card shows paused badge "Paused", steps "done, done, -, -, -, -" with "Review · 5 / 6", and "Local checks passed"',
      () => {
        const badge = one('.card-head .badge');
        expect(badge.textContent?.trim()).toBe('Paused');
        expect(badge.querySelector('.st')?.getAttribute('data-st')).toBe('paused');
        expect(stepMarks()).toBe('done, done, -, -, -, -');
        expect(texts('.card .steps span')).toEqual(['Review · 5 / 6']);
        expect(kv('Branch')?.textContent?.trim()).toBe('task/14-fix-the-relay-notice');
        expect(kv('Checks')?.textContent?.trim()).toBe('Local checks passed');
      },
    );
    When(
      'the task stops in state "failed" after 3 consecutive failures, and then after 6 failed reviews',
      () => {
        home.write(
          website,
          example('row-task-paused', {
            state: 'failed',
            step: 'local_check',
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
            pr: 22,
            wait: null,
            failures: 0,
            review: {
              round: 6,
              failed: 6,
              score: 82,
              ok: false,
              by: claudeId,
              commit: '9f2c1abf43f3224eb6826ef971a943dbab4795ce',
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
        expect(crashSteps).toBe('done, done, fail, -, -, -');
        const badge = one('.card-head .badge');
        expect(badge.textContent?.trim()).toBe('Stopped after 6 failed reviews');
        expect(badge.getAttribute('data-tone')).toBe('danger');
        expect(badge.classList.contains('solid')).toBe(true);
        expect(stepMarks()).toBe('done, done, done, done, fail, -');
      },
    );
    When('the task becomes "cancelled"', () => {
      home.write(
        website,
        example('row-task-paused', { state: 'cancelled', step: 'review', pr: 22, wait: null }),
      );
      flushSync();
    });
    Then('the task card shows "Cancelled" with no active step marker', () => {
      expect(texts('.card-head .badge')).toEqual(['Cancelled']);
      expect(stepMarks()).toBe('done, done, done, done, -, -');
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
        'the "task" section is marked "Mounted by task" and renders "T-12" "Show connection path" with six steps',
        () => {
          const section = one('#states-task').closest('section')!;
          expect(section.querySelector('header .badge')?.textContent?.trim()).toBe(
            'Mounted by task',
          );
          expect(section.querySelector('.card-head .mono')?.textContent).toBe('T-12');
          expect(section.querySelector('.card-head h3')?.textContent).toBe('Show connection path');
          expect(section.querySelectorAll('.steps i')).toHaveLength(6);
        },
      );
    },
  );
});
