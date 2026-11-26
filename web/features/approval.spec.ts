import { describeFeature, loadFeature } from '@amiceli/vitest-cucumber';
import { flushSync } from 'svelte';
import { expect, vi } from 'vitest';
import { Home, homes, type Group } from '../src/lib/groups.svelte';
import { goldens } from '../src/lib/mock';
import { prefs } from '../src/lib/prefs.svelte';
import * as p from '../src/lib/proto';
import { clock } from '../src/lib/when';
import { example, owner, TestHome, user } from './home';
import { all, closePage, one, openPage, texts } from './page';

const feature = await loadFeature('../features/web_approval.feature');

const website = 'rZNvy-1jH6Z-BcPqA5U5BQ';
const codex = '1PC8WineBrUQ-apCjx7tug';
const taskId = 'DrtCn6htSBwmMPrFPbHJHA';
const approvalId = 'netluHiPQf-UoheEIBfbLw';

interface HeldAct {
  payload: p.Act;
  resolve: (answer: object) => void;
}

/** Connects a browser whose `act` commands can be held in flight, refused, or applied to the
 *  test home's rows. */
function approvalBrowser(home: TestHome) {
  const sign = async (payload: string) => ({ p: payload, k: 'this-browser', s: 'signature' });
  const store = new Home({ user, sign });
  const acts: p.Act[] = [];
  let holdActs = false;
  let refuseActs: string | undefined;
  let held: HeldAct | undefined;
  let drop = () => {};

  return {
    store,
    acts,
    set hold(value: boolean) {
      holdActs = value;
    },
    set refuse(code: string | undefined) {
      refuseActs = code;
    },
    release(answer: object) {
      held?.resolve(answer);
      held = undefined;
    },
    async connect() {
      const link = store.connect();
      const connection = home.connect((frame) => link.push(frame));
      drop = () => {
        connection.close();
        link.offline();
      };
      const ctl = {
        request: async (req: Record<string, unknown>) => {
          if (req.t === 'cmd') {
            const env = req.env as p.Envelope;
            const parsed = JSON.parse(env.p) as { t: string };
            if (parsed.t === 'act') {
              const act = p.decode(p.Act, env.p);
              acts.push(act);
              if (holdActs) {
                const out = await new Promise<object>((resolve) => {
                  held = { payload: act, resolve };
                });
                return { re: 99, ...out };
              }
              await new Promise((later) => setTimeout(later));
              if (refuseActs) {
                return { re: 99, err: { code: refuseActs, msg: 'refused' } };
              }
              return { re: 99, ok: true };
            }
          }
          return (connection.ctl.request as (r: Record<string, unknown>) => Promise<unknown>)(req);
        },
      } as p.Ctl;
      const welcome = await connection.ctl.request({
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
}

describeFeature(feature, ({ Background, Scenario, AfterEachScenario }) => {
  let home: TestHome;
  let client: ReturnType<typeof approvalBrowser>;
  let group: Group;

  async function read() {
    const answered = home.synced + home.groups.size;
    await client.connect();
    await vi.waitFor(() => expect(home.synced).toBe(answered));
    await new Promise((applied) => setTimeout(applied));
    flushSync();
  }

  function approvalBody(changes: Record<string, unknown> = {}) {
    const base = p.decode(p.MsgRow, goldens['row-msg-approval-pending']).body as Record<
      string,
      unknown
    >;
    const copy = { ...base, ...changes };
    for (const [k, val] of Object.entries(copy)) {
      if (val === undefined) delete copy[k];
    }
    return copy;
  }

  AfterEachScenario(async () => {
    await closePage();
    vi.unstubAllEnvs();
  });

  Background(({ Given }) => {
    Given(
      'the chat of the group "Website" open in a browser of its owner, set to English',
      async () => {
        home = new TestHome('workstation');
        home.add(website, 'Website');
        home.write(website, example('row-member-ai'));
        home.write(website, example('row-task'));
        client = approvalBrowser(home);
        homes.push(client.store);
        await read();
        group = client.store.groups[0];
        openPage(`#/groups/${website}`);
        expect(texts('.chat-head h2')).toEqual(['Website']);
      },
    );
  });

  Scenario(
    'The waiting bar lists the cards waiting on the reader and jumps to a card',
    ({ Given, When, Then }) => {
      let scrolled: Element[] = [];
      Given(
        'the group holds a pending brief, approval, decision and confirmation for the reader, and an approval for "Mina"',
        () => {
          scrolled = [];
          Element.prototype.scrollTo = function () {
            scrolled.push(this);
          };
          home.write(
            website,
            example('row-member-human', { id: 'mina-m', user: 'mina-u', name: 'Mina' }),
          );
          home.write(website, example('row-msg-brief-pending', { id: 'b-1', pos: 10 }));
          home.write(website, example('row-msg-approval-pending', { id: approvalId, pos: 20 }));
          home.write(website, example('row-msg-decision-pending', { id: 'd-1', pos: 30 }));
          home.write(
            website,
            example('row-msg-confirm-pending', {
              id: 'c-1',
              by: 'mina-m',
              body: { what: 'watch', ai: example('row-member-ai').id, to: [user] },
            }),
          );
          home.write(
            website,
            example('row-msg-approval-pending', {
              id: 'ap-mina',
              pos: 40,
              body: approvalBody({ approvers: ['mina-u'] }),
            }),
          );
          flushSync();
        },
      );
      Then(
        'the chat waiting bar reads "Waiting on you · 4" with chips for the brief, the approval, the decision and the confirmation',
        () => {
          expect(one('nav.needbar').getAttribute('aria-label')).toBe('Waiting on you');
          expect(texts('nav.needbar b')).toEqual(['Waiting on you · 4']);
          expect(texts('nav.needbar a')).toEqual([
            'Confirm start · Show connection paths',
            'Approval · Time the path change in a browser.',
            'Your call · Where should the path appear?',
            'Confirm · Let Mina watch Codex’s terminal.',
          ]);
        },
      );
      When('the person clicks the approval chip in the waiting bar', () => {
        all<HTMLAnchorElement>('nav.needbar a')[1].click();
        flushSync();
      });
      Then(
        'the chat stays open on "Website" and focus moves to the Approve button of that card',
        () => {
          expect(location.hash).toBe(`#/groups/${website}`);
          const msgEl = document.getElementById(`m-${approvalId}`)!;
          expect(scrolled.at(-1)).toBe(one('.scroller'));
          expect(document.activeElement).toBe(msgEl.querySelector('button.btn-dawn'));
        },
      );
      When('the person clicks the brief chip in the waiting bar', () => {
        all<HTMLAnchorElement>('nav.needbar a')[0].click();
        flushSync();
      });
      Then('focus moves to the brief message in the main line', () => {
        const briefEl = document.getElementById('m-b-1')!;
        expect(scrolled.at(-1)).toBe(one('.scroller'));
        const target = briefEl.querySelector('button:not([disabled])') ?? briefEl;
        expect(document.activeElement).toBe(target);
      });
      When('the person clicks the decision chip in the waiting bar', () => {
        all<HTMLAnchorElement>('nav.needbar a')[2].click();
        flushSync();
      });
      Then('focus moves to the decision message in the main line', () => {
        const decisionEl = document.getElementById('m-d-1')!;
        expect(scrolled.at(-1)).toBe(one('.scroller'));
        const target =
          decisionEl.querySelector('input:not([disabled]), textarea:not([disabled])') ?? decisionEl;
        expect(document.activeElement).toBe(target);
      });
      When('the person clicks the confirmation chip in the waiting bar', () => {
        all<HTMLAnchorElement>('nav.needbar a')[3].click();
        flushSync();
      });
      Then('focus moves to the Confirm button of that card', () => {
        const msgEl = document.getElementById('m-c-1')!;
        expect(scrolled.at(-1)).toBe(one('.scroller'));
        expect(document.activeElement).toBe(msgEl.querySelector('button.btn-dawn'));
      });
      When('the four cards waiting on the reader are no longer pending', () => {
        home.write(website, example('row-msg-brief-confirmed', { id: 'b-1' }));
        home.write(website, example('row-msg-approval-approved', { id: approvalId }));
        home.write(website, example('row-msg-decision-decided', { id: 'd-1' }));
        home.write(website, example('row-msg-confirm-denied', { id: 'c-1' }));
        flushSync();
      });
      Then('the chat has no waiting bar', () => {
        expect(all('nav.needbar')).toEqual([]);
        expect(one('.chat > .top').children).toHaveLength(0);
      });
    },
  );

  Scenario(
    'Approving a pending card sends act approve and shows the approved state',
    ({ Given, When, Then, And }) => {
      const decidedAt = 1763600000000;
      Given('a pending command approval for "Codex" on task 12 without truncation', () => {
        home.write(
          website,
          example('row-msg-approval-pending', {
            id: approvalId,
            body: approvalBody({ full: undefined }),
          }),
        );
        flushSync();
      });
      Then(
        'the card has the dawn edge, "Needs you", category "Command", "workstation (your computer) · Task T-12", the command and the reason',
        () => {
          const card = one('#m-netluHiPQf-UoheEIBfbLw section.card');
          expect(card.classList.contains('need')).toBe(true);
          expect(texts('#m-netluHiPQf-UoheEIBfbLw .card-head .kind')).toEqual(['Approval']);
          expect(texts('#m-netluHiPQf-UoheEIBfbLw .card-head .badge')).toEqual([
            'Command',
            'Needs you',
          ]);
          expect(one('#m-netluHiPQf-UoheEIBfbLw .badge.solid')!.getAttribute('data-tone')).toBe(
            'dawn',
          );
          expect(texts('#m-netluHiPQf-UoheEIBfbLw .who')).toEqual([
            'Codex · workstation (your computer) · Task T-12',
          ]);
          expect(texts('#m-netluHiPQf-UoheEIBfbLw .cmd')).toEqual([
            'pnpm exec playwright install chromium',
          ]);
          expect(texts('#m-netluHiPQf-UoheEIBfbLw dd').at(-1)).toBe(
            'Time the path change in a browser.',
          );
          expect(texts('#m-netluHiPQf-UoheEIBfbLw .card-foot .hint')).toEqual([
            'Applies to this one request',
          ]);
        },
      );
      And('the card shows no truncation warning', () => {
        expect(all('#m-netluHiPQf-UoheEIBfbLw .note')).toEqual([]);
      });
      When('the person clicks "Approve"', async () => {
        client.hold = true;
        one<HTMLButtonElement>('#m-netluHiPQf-UoheEIBfbLw .btn-dawn').click();
        await vi.waitFor(() => expect(client.acts).toHaveLength(1));
        flushSync();
      });
      Then('the Approve and Deny buttons are disabled while the home has not answered', () => {
        expect(
          all<HTMLButtonElement>('#m-netluHiPQf-UoheEIBfbLw .card-foot button').map(
            (b) => b.disabled,
          ),
        ).toEqual([true, true]);
      });
      When('the home answers and writes the approved row', async () => {
        client.release({ ok: true });
        home.write(
          website,
          example('row-msg-approval-approved', {
            id: approvalId,
            body: approvalBody({ full: undefined }),
          }),
        );
        await new Promise((later) => setTimeout(later, 5));
        flushSync();
      });
      Then('the home received an act command of action "approve" for that card', () => {
        expect(client.acts).toEqual([
          {
            t: 'act',
            n: expect.stringMatching(/^[A-Za-z0-9_-]{22}$/),
            ts: expect.any(Number),
            g: website,
            msg: approvalId,
            action: 'approve',
          },
        ]);
      });
      And(
        'the card has no dawn edge, reads "Approved by Alex" and says "Approved for this one request. Codex can continue."',
        () => {
          const card = one('#m-netluHiPQf-UoheEIBfbLw section.card');
          expect(card.classList.contains('need')).toBe(false);
          expect(all('#m-netluHiPQf-UoheEIBfbLw .card-foot button')).toEqual([]);
          const status = all('#m-netluHiPQf-UoheEIBfbLw .card-head .badge')[1];
          expect(status.getAttribute('data-tone')).toBe('ok');
          expect(status.textContent!.trim()).toBe(`Approved by Alex · ${clock(decidedAt, 'en')}`);
          expect(texts('#m-netluHiPQf-UoheEIBfbLw > .msg-body section.card > .hint')).toEqual([
            'Approved for this one request. Codex can continue.',
          ]);
          expect(all('nav.needbar')).toEqual([]);
        },
      );
    },
  );

  Scenario(
    'Denying a truncated request shows the K-21 warning first and then the denied state',
    ({ Given, When, Then, And }) => {
      const decidedAt = 1763600000000;
      Given('a pending file approval of 4096 bytes for "Codex" with no reason', () => {
        home.write(
          website,
          example('row-msg-approval-pending', {
            id: approvalId,
            body: approvalBody({
              category: 'file',
              command: 'patch README.md',
              full: 4096,
              why: undefined,
            }),
          }),
        );
        flushSync();
      });
      Then(
        'the card shows category "File", omits the "Why" row, and warns that only the beginning of 4096 bytes is shown',
        () => {
          expect(texts('#m-netluHiPQf-UoheEIBfbLw .card-head .badge')).toEqual([
            'File',
            'Needs you',
          ]);
          expect(texts('#m-netluHiPQf-UoheEIBfbLw dt')).toEqual(['Who', 'Wants to']);
          expect(texts('#m-netluHiPQf-UoheEIBfbLw .note')).toEqual([
            'Only the beginning is shown (4096 bytes in all), and approving covers the whole request. Recommended: deny it and ask the AI to split it into smaller steps.',
          ]);
        },
      );
      And('the chat waiting bar names the command as the chip subject', () => {
        expect(texts('nav.needbar a')).toEqual(['Approval · patch README.md']);
      });
      When('the person clicks "Deny" and the home writes the denied row', async () => {
        all<HTMLButtonElement>('#m-netluHiPQf-UoheEIBfbLw .card-foot button')[1].click();
        await vi.waitFor(() => expect(client.acts).toHaveLength(1));
        home.write(
          website,
          example('row-msg-approval-denied', {
            id: approvalId,
            body: approvalBody({
              category: 'file',
              command: 'patch README.md',
              full: 4096,
              why: undefined,
            }),
          }),
        );
        await new Promise((later) => setTimeout(later, 5));
        flushSync();
      });
      Then('the home received an act command of action "deny" for that card', () => {
        expect(client.acts[0]).toMatchObject({
          t: 'act',
          g: website,
          msg: approvalId,
          action: 'deny',
        });
      });
      And(
        'the card reads "Denied by Alex" and says "Codex will try another way without this permission, or report back."',
        () => {
          const status = all('#m-netluHiPQf-UoheEIBfbLw .card-head .badge')[1];
          expect(status.getAttribute('data-tone')).toBe('danger');
          expect(status.textContent!.trim()).toBe(`Denied by Alex · ${clock(decidedAt, 'en')}`);
          expect(texts('#m-netluHiPQf-UoheEIBfbLw section.card > .hint')).toEqual([
            'Codex will try another way without this permission, or report back.',
          ]);
        },
      );
    },
  );

  Scenario(
    'A pending approval for another member is shown without warm emphasis and cannot be clicked',
    ({ Given, When, Then, And }) => {
      Given(
        '"Mina" owns "Claude Code" on "Mina\'s MacBook" and a pending network approval waits on "Mina"',
        () => {
          home.write(
            website,
            example('row-member-human', { id: 'mina-m', user: 'mina-u', name: 'Mina' }),
          );
          home.write(
            website,
            example('row-member-ai', {
              id: 'mina-ai',
              user: 'mina-u',
              name: 'Claude Code',
              cli: 'claude',
              host_name: "Mina's MacBook",
            }),
          );
          home.write(
            website,
            example('row-msg-approval-pending', {
              id: 'ap-mina',
              by: 'mina-ai',
              body: approvalBody({
                ai: 'mina-ai',
                task: 'missing-task',
                category: 'network',
                approvers: ['mina-u'],
                full: undefined,
              }),
            }),
          );
          flushSync();
        },
      );
      Then(
        'the card has no dawn edge, reads "Waiting on Mina" and category "Network", and shows "Mina\'s MacBook" without "your computer"',
        () => {
          const card = one('#m-ap-mina section.card');
          expect(card.classList.contains('need')).toBe(false);
          expect(texts('#m-ap-mina .card-head .badge')).toEqual(['Network', 'Waiting on Mina']);
          expect(all('#m-ap-mina .btn-dawn')).toEqual([]);
          expect(texts('#m-ap-mina .who')).toEqual(["Claude Code · Mina's MacBook"]);
        },
      );
      And(
        'the Approve and Deny buttons are disabled with the lock hint "Only the computer\'s owner Mina can approve"',
        () => {
          const buttons = all<HTMLButtonElement>('#m-ap-mina .card-foot button');
          expect(buttons.map((b) => [b.textContent!.trim(), b.disabled])).toEqual([
            ['Approve', true],
            ['Deny', true],
          ]);
          expect(texts('#m-ap-mina .card-foot .hint')).toEqual([
            "Only the computer's owner Mina can approve",
          ]);
        },
      );
      When('a pending approval waits on an unknown user with category "other"', () => {
        home.write(
          website,
          example('row-msg-approval-pending', {
            id: 'ap-unknown',
            body: approvalBody({
              ai: 'gone-ai',
              task: 'missing-task',
              category: 'other',
              approvers: ['stranger-user'],
              full: undefined,
            }),
          }),
        );
        flushSync();
      });
      Then(
        'that card reads "Waiting for owner", category "Other", and "Only the computer\'s owner can approve"',
        () => {
          expect(one('#m-ap-unknown .av').classList.contains('ai')).toBe(true);
          expect(texts('#m-ap-unknown .card-head .badge')).toEqual(['Other', 'Waiting for owner']);
          expect(texts('#m-ap-unknown .who > span:not(.av)')).toEqual(['Former member']);
          expect(texts('#m-ap-unknown .card-foot .hint')).toEqual([
            "Only the computer's owner can approve",
          ]);
          home.write(
            website,
            example('row-msg-approval-approved', {
              id: 'ap-gone-ok',
              body: approvalBody({ ai: 'gone-ai', full: undefined }),
              state: {
                ...(p.decode(p.MsgRow, goldens['row-msg-approval-approved']).state as object),
                by: 'gone-human',
              },
            }),
          );
          home.write(
            website,
            example('row-msg-approval-denied', {
              id: 'ap-gone-deny',
              body: approvalBody({ ai: 'gone-ai', full: undefined }),
            }),
          );
          flushSync();
          expect(one('#m-ap-gone-ok .card-head .badge:last-child').textContent).toContain(
            'Approved by Former member',
          );
          expect(texts('#m-ap-gone-ok section.card > .hint')).toEqual([
            'Approved for this one request.',
          ]);
          expect(texts('#m-ap-gone-deny section.card > .hint')).toEqual([
            'Denied. The AI will try another way or report back.',
          ]);
        },
      );
    },
  );

  Scenario(
    'A card closed by a reassignment names who gave the task to which AI, also after that person left',
    ({ Given, When, Then }) => {
      const reassigned = '#m-netluHiPQf-UoheEIBfbLw';
      Given('a pending command approval for "Codex" on task 12 without truncation', () => {
        home.write(
          website,
          example('row-msg-approval-pending', {
            id: approvalId,
            body: approvalBody({ full: undefined }),
          }),
        );
        flushSync();
      });
      When(
        'the admin "Mina" gives T-12 to "Claude Code" and the home closes the card as reassigned by her',
        async () => {
          home.write(
            website,
            example('row-member-human', {
              id: 'mina-m',
              user: 'mina-u',
              name: 'Mina',
              role: 'admin',
            }),
          );
          home.write(website, example('row-member-ai', { id: 'claude-m', name: 'Claude Code' }));
          home.write(website, example('row-task', { assignee: 'claude-m' }));
          home.write(
            website,
            example('row-msg-approval-reassigned', {
              id: approvalId,
              body: approvalBody({ full: undefined }),
              state: { status: 'reassigned', by: 'mina-m', at: 1763600000000 },
            }),
          );
          await new Promise((later) => setTimeout(later, 5));
          flushSync();
        },
      );
      Then(
        'the card reads "Reassigned" and says "Mina gave the task to Claude Code; this request is closed."',
        () => {
          expect(texts(`${reassigned} .card-head .badge`).at(-1)).toBe(
            `Reassigned · ${clock(1763600000000, 'en')}`,
          );
          expect(texts(`${reassigned} p.hint`)).toEqual([
            'Mina gave the task to Claude Code; this request is closed.',
          ]);
        },
      );
      When('Mina leaves the group', async () => {
        home.remove(website, 'member', 'mina-m');
        await new Promise((later) => setTimeout(later, 5));
        flushSync();
      });
      Then(
        'the card still names Mina, in English and in Chinese "Mina 把任务改派给了 Claude Code，这个申请随之关闭。"',
        () => {
          expect(group.member('mina-m')).toBeUndefined();
          expect(texts(`${reassigned} p.hint`)).toEqual([
            'Mina gave the task to Claude Code; this request is closed.',
          ]);
          prefs.lang = 'zh-CN';
          flushSync();
          expect(texts(`${reassigned} p.hint`)).toEqual([
            'Mina 把任务改派给了 Claude Code，这个申请随之关闭。',
          ]);
        },
      );
    },
  );

  Scenario(
    'Offline and refused actions disable or report errors cleanly',
    ({ Given, When, Then }) => {
      Given('a pending command approval for "Codex" on task 12 without truncation', () => {
        home.write(
          website,
          example('row-msg-approval-pending', {
            id: approvalId,
            body: approvalBody({ full: undefined }),
          }),
        );
        flushSync();
      });
      When('the home stops', () => {
        home.stop();
        client.drop();
        flushSync();
      });
      Then(
        'the Approve and Deny buttons are disabled and the card says "Cannot reach the group home"',
        () => {
          expect(
            all<HTMLButtonElement>('#m-netluHiPQf-UoheEIBfbLw .card-foot button').map(
              (b) => b.disabled,
            ),
          ).toEqual([true, true]);
          expect(texts('#m-netluHiPQf-UoheEIBfbLw .card-foot .hint')).toEqual([
            'Cannot reach the group home',
          ]);
        },
      );
      When(
        'the home returns, refuses act commands with "conflict", and the person clicks "Approve"',
        async () => {
          client.refuse = 'conflict';
          await read();
          one<HTMLButtonElement>('#m-netluHiPQf-UoheEIBfbLw .btn-dawn').click();
          await vi.waitFor(() =>
            expect(texts('#m-netluHiPQf-UoheEIBfbLw [role=alert]')).toHaveLength(1),
          );
          flushSync();
        },
      );
      Then(
        'the card alerts "Not sent. The home answered: conflict" and enables the buttons again',
        () => {
          expect(texts('#m-netluHiPQf-UoheEIBfbLw [role=alert]')).toEqual([
            'Not sent. The home answered: conflict',
          ]);
          expect(
            all<HTMLButtonElement>('#m-netluHiPQf-UoheEIBfbLw .card-foot button').map(
              (b) => b.disabled,
            ),
          ).toEqual([false, false]);
        },
      );
    },
  );

  Scenario(
    'The states sheet renders all five wire examples of approval cards',
    ({ Given, When, Then, And }) => {
      Given('a development server', () => {
        vi.stubEnv('MODE', 'development');
      });
      When('the person opens the states sheet', async () => {
        await closePage();
        openPage('#/states');
        await vi.waitFor(() => {
          flushSync();
          expect(all('.states')).toHaveLength(1);
        });
      });
      Then(
        'the mounting points show "approval · 20" at "chat.top" and the approval section is mounted by "approval"',
        () => {
          const dts = texts('.points dt');
          const chatTopIndex = dts.indexOf('chat.top');
          expect(all('.points dd')[chatTopIndex].textContent).toContain('approval · 20');
          const section = one('section[aria-labelledby="states-approval"]');
          expect(section.querySelector('header .badge[data-tone="ok"]')!.textContent!.trim()).toBe(
            'Mounted by approval',
          );
        },
      );
      And(
        'the five approval examples show "Approved by owner", "Denied by owner", "Expired", "Needs you" and "Reassigned"',
        () => {
          const at = clock(1763600000000, 'en');
          const section = one('section[aria-labelledby="states-approval"]');
          expect(
            [...section.querySelectorAll('figcaption')].map((el) => el.textContent!.trim()),
          ).toEqual([
            'row-msg-approval-approved',
            'row-msg-approval-denied',
            'row-msg-approval-expired',
            'row-msg-approval-pending',
            'row-msg-approval-reassigned',
          ]);
          expect(
            [...section.querySelectorAll('.card-head .badge:last-child')].map((el) =>
              el.textContent!.trim(),
            ),
          ).toEqual([
            `Approved by owner · ${at}`,
            `Denied by owner · ${at}`,
            `Expired · ${at}`,
            'Needs you',
            `Reassigned · ${at}`,
          ]);
          prefs.lang = 'zh-CN';
          flushSync();
          expect(
            [...section.querySelectorAll('.card-head .badge:last-child')].map((el) =>
              el.textContent!.trim(),
            ),
          ).toEqual([
            `owner 已批准 · ${clock(1763600000000, 'zh-CN')}`,
            `owner 已拒绝 · ${clock(1763600000000, 'zh-CN')}`,
            `已过期 · ${clock(1763600000000, 'zh-CN')}`,
            '等你审批',
            `已改派 · ${clock(1763600000000, 'zh-CN')}`,
          ]);
          expect(texts('section[aria-labelledby="states-approval"] p.hint')).toContain(
            'Secretary 把任务改派给了 Codex，这个申请随之关闭。',
          );
        },
      );
    },
  );
});
