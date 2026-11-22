import 'fake-indexeddb/auto';
import { describeFeature, loadFeature } from '@amiceli/vitest-cucumber';
import { flushSync } from 'svelte';
import { expect, vi } from 'vitest';
import { homes } from '../src/lib/groups.svelte';
import { autoDismissMs } from '../src/lib/notify.svelte';
import { prefs } from '../src/lib/prefs.svelte';
import { browser, example, owner, TestHome } from './home';
import { all, closePage, one, openPage, texts } from './page';

const feature = await loadFeature('../features/web_notify.feature');

const website = 'rZNvy-1jH6Z-BcPqA5U5BQ';
const docs = 'ZG9jcy1ncm91cC0wMDAwMQ';
const codex = '1PC8WineBrUQ-apCjx7tug';
const secretary = 'qBSFMsr2hHYKOMblEA_kdA';
const claude = 'LXCZmuGAXkvO-bSrOkuCfw';
const taskId = 'DrtCn6htSBwmMPrFPbHJHA';
const approvalId = 'netluHiPQf-UoheEIBfbLw';

describeFeature(feature, ({ Background, Scenario, AfterEachScenario }) => {
  let home: TestHome;
  let reader: ReturnType<typeof browser>;
  let scrolled: Element[];

  AfterEachScenario(async () => {
    vi.useRealTimers();
    await closePage();
    vi.unstubAllEnvs();
  });

  Background(({ Given }) => {
    Given(
      'the groups "Website" and "Docs" read in a browser of their owner, set to English',
      async () => {
        scrolled = [];
        Element.prototype.scrollIntoView = function () {
          scrolled.push(this);
        };
        home = new TestHome('workstation');
        home.add(website, 'Website');
        home.write(website, example('row-member-ai', { id: codex, name: 'Codex', cli: 'codex' }));
        home.write(
          website,
          example('row-member-ai', { id: claude, name: 'Claude Code', cli: 'claude' }),
        );
        home.write(
          website,
          example('row-member-ai', {
            id: secretary,
            name: 'Secretary',
            cli: 'claude',
            secretary: true,
          }),
        );
        home.write(
          website,
          example('row-member-human', { id: 'mina-w', user: 'mina-u', name: 'Mina' }),
        );
        home.write(website, example('row-task', { id: taskId }));
        home.write(website, example('row-msg-task', { id: taskId, body: { task: taskId } }));
        home.write(website, example('row-msg-approval-pending', { id: 'existing-ap' }));

        home.add(docs, 'Docs');
        home.write(
          docs,
          example('row-member-human', { id: 'mina-d', g: docs, user: 'mina-u', name: 'Mina' }),
        );

        reader = browser(home);
        homes.push(reader.store);
        const target = home.synced + home.groups.size;
        await reader.connect();
        await vi.waitFor(() => expect(home.synced).toBe(target));
        await new Promise((applied) => setTimeout(applied));
      },
    );
  });

  Scenario(
    'Blocking cards appear in dawn on other pages, jump to the card on Open, and clear when resolved',
    ({ Given, Then, When }) => {
      Given('the person opens the client at "#/nodes"', () => openPage('#/nodes'));
      Then('no notification is shown for the rows already held', () => {
        expect(all('.notify')).toEqual([]);
      });
      When(
        '"Codex" posts a pending approval in "Website" and a pending approval for "Mina"',
        () => {
          home.write(website, example('row-msg-approval-pending', { id: approvalId, by: codex }));
          home.write(
            website,
            example('row-msg-approval-pending', {
              id: 'ap-mina',
              by: codex,
              body: {
                ...(example('row-msg-approval-pending').body as object),
                approvers: ['mina-u'],
              },
            }),
          );
          flushSync();
        },
      );
      Then(
        'the notification layer shows one warm notification "Approval · Time the path change in a browser." for "Website · Codex"',
        () => {
          expect(one('section.notify').getAttribute('aria-label')).toBe('Notifications');
          expect(texts('.toast b')).toEqual(['Approval · Time the path change in a browser.']);
          expect(texts('.toast p')).toEqual(['Website · Codex']);
          expect(one('.toast').classList.contains('need')).toBe(true);
          expect(one('.toast a.btn').classList.contains('btn-dawn')).toBe(true);
        },
      );
      When('"Secretary" posts a pending decision and a pending brief in "Website"', () => {
        home.write(website, example('row-msg-decision-pending', { id: 'dc-live', by: secretary }));
        home.write(website, example('row-msg-brief-pending', { id: 'br-live', by: secretary }));
        flushSync();
      });
      Then(
        'the notification layer shows the approval, "Your call · Where should the path appear?" and "Confirm start · Show connection paths"',
        () => {
          expect(texts('.toast b')).toEqual([
            'Approval · Time the path change in a browser.',
            'Your call · Where should the path appear?',
            'Confirm start · Show connection paths',
          ]);
        },
      );
      When('the approval card in "Website" is approved on another device', () => {
        home.write(website, example('row-msg-approval-approved', { id: approvalId, by: codex }));
        flushSync();
      });
      Then('the approval notification disappears, leaving the decision and the brief', () => {
        expect(texts('.toast b')).toEqual([
          'Your call · Where should the path appear?',
          'Confirm start · Show connection paths',
        ]);
      });
      When('the person clicks "Open" on the brief notification', async () => {
        all<HTMLAnchorElement>('.toast a.btn')[1].click();
        await vi.waitFor(() => expect(texts('.chat-head h2')).toEqual(['Website']));
        flushSync();
      });
      Then(
        'the chat of "Website" opens with focus on the "Confirm & start" button and no notifications remain',
        () => {
          expect(location.hash).toBe(`#/groups/${website}`);
          const briefEl = document.getElementById('m-br-live')!;
          expect(scrolled.at(-1)).toBe(briefEl);
          expect(document.activeElement?.textContent?.trim()).toBe('Confirm & start');
          expect(all('.notify')).toEqual([]);
        },
      );
    },
  );

  Scenario(
    'Mentions and completed tasks auto-dismiss, cap the stack, and can be dismissed by hand',
    ({ Given, When, Then }) => {
      Given('the person opens the client at "#/settings"', () => {
        vi.useFakeTimers();
        openPage('#/settings');
      });
      When(
        '"Codex" mentions the owner in "Website" with "Check `path.go` first" and "Secretary" posts a summary for task 12',
        () => {
          home.say(website, codex, 'Check `path.go` first', { mentions: [owner] });
          home.write(website, example('row-msg-summary', { id: 'sum-12', by: secretary }));
          flushSync();
        },
      );
      Then(
        'the notification layer shows two calm notifications: "Mentioned you · Check `path.go` first" and "Task done · T-12 Show connection path"',
        () => {
          expect(texts('.toast b')).toEqual([
            'Mentioned you · Check `path.go` first',
            'Task done · T-12 Show connection path',
          ]);
          expect(all('.toast.need')).toEqual([]);
          expect(all('.toast a.btn-dawn')).toEqual([]);
          expect(all('.toast').map((el) => el.getAttribute('data-tone'))).toEqual(['sky', 'ok']);
        },
      );
      When('the person clicks "Dismiss notification" on the mention notification', () => {
        all<HTMLButtonElement>('.toast button.icon-btn')[0].click();
        flushSync();
      });
      Then('only the task-done notification remains', () => {
        expect(texts('.toast b')).toEqual(['Task done · T-12 Show connection path']);
        expect(texts('.toast p')).toEqual(['Website · Secretary']);
      });
      When('8 seconds pass', () => {
        vi.advanceTimersByTime(autoDismissMs);
        flushSync();
      });
      Then('no notification is shown', () => {
        expect(all('.notify')).toEqual([]);
      });
    },
  );

  Scenario(
    'Main-line arrivals in the open chat stay quiet while thread mentions and other groups notify',
    ({ Given, When, Then }) => {
      Given('the person opens the client at "#/groups/rZNvy-1jH6Z-BcPqA5U5BQ"', () =>
        openPage(`#/groups/${website}`),
      );
      When(
        '"Codex" posts a pending approval in "Website" and mentions the owner in the main line of "Website"',
        () => {
          home.write(website, example('row-msg-approval-pending', { id: 'ap-open', by: codex }));
          home.say(website, codex, '@Alex main line ping', { mentions: [owner] });
          flushSync();
        },
      );
      Then('no notification is shown', () => {
        expect(all('.notify')).toEqual([]);
      });
      When(
        '"Codex" mentions the owner inside task 12\'s thread in "Website" and "Mina" mentions the owner in "Docs"',
        () => {
          home.write(
            website,
            example('row-msg-question', {
              id: 'thread-q',
              by: codex,
              thread: taskId,
              main: false,
              text: '@Alex keep fallback?',
              mentions: [owner],
            }),
          );
          const docsOwner = reader.store.groups.find((g) => g.g === docs)!.me!.id;
          home.say(docs, 'mina-d', '@Alex docs are ready', { mentions: [docsOwner] });
          flushSync();
        },
      );
      Then(
        'the notification layer shows the thread mention for "Website · Codex" and the mention for "Docs · Mina"',
        () => {
          expect(texts('.toast b')).toEqual([
            'Mentioned you · @Alex keep fallback?',
            'Mentioned you · @Alex docs are ready',
          ]);
          expect(texts('.toast p')).toEqual(['Website · Codex', 'Docs · Mina']);
        },
      );
      When('the person clicks "Open" on the thread mention notification', () => {
        all<HTMLAnchorElement>('.toast a.btn')[0].click();
        flushSync();
      });
      Then('the chat stays on "Website" and focus moves to task 12\'s card', () => {
        expect(location.hash).toBe(`#/groups/${website}`);
        const taskEl = document.getElementById(`m-${taskId}`)!;
        expect(scrolled.at(-1)).toBe(taskEl);
        expect(taskEl.contains(document.activeElement)).toBe(true);
        expect(texts('.toast p')).toEqual(['Docs · Mina']);
      });
    },
  );

  Scenario(
    "The states sheet lists notify at the shell's layer mounting point",
    ({ Given, When, Then }) => {
      Given('a development server', () => {
        vi.stubEnv('MODE', 'development');
      });
      When('the person opens the states sheet', async () => {
        openPage('#/states');
        await vi.waitFor(() => {
          flushSync();
          expect(all('.states')).toHaveLength(1);
        });
      });
      Then(
        'the mounting points show "notify · 10" at "layer" and notifications translate to Chinese',
        () => {
          const dts = texts('.points dt');
          const layerIndex = dts.indexOf('layer');
          expect(all('.points dd')[layerIndex].textContent).toContain('notify · 10');

          home.write(website, example('row-msg-approval-pending', { id: 'ap-zh', by: codex }));
          prefs.lang = 'zh-CN';
          flushSync();
          expect(one('section.notify').getAttribute('aria-label')).toBe('通知');
          expect(texts('.toast b')).toEqual(['审批 · Time the path change in a browser.']);
          expect(texts('.toast a.btn')).toEqual(['查看']);
          expect(one('.toast button.icon-btn').getAttribute('aria-label')).toBe('关闭通知');
        },
      );
    },
  );
});
