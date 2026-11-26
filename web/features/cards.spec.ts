import { describeFeature, loadFeature } from '@amiceli/vitest-cucumber';
import { flushSync } from 'svelte';
import { expect, vi } from 'vitest';
import { homes } from '../src/lib/groups.svelte';
import * as p from '../src/lib/proto';
import { ActHome, browser, example, user } from './home';
import { all, closePage, fill, one, openPage, texts } from './page';

const feature = await loadFeature('../features/web_cards.feature');

const website = 'rZNvy-1jH6Z-BcPqA5U5BQ';
const secretary = 'qBSFMsr2hHYKOMblEA_kdA';
const codex = '1PC8WineBrUQ-apCjx7tug';
const claude = 'LXCZmuGAXkvO-bSrOkuCfw';
const taskId = 'DrtCn6htSBwmMPrFPbHJHA';

describeFeature(feature, ({ Background, Scenario, AfterEachScenario }) => {
  let home: ActHome;
  let reader: ReturnType<typeof browser>;

  const acts = () =>
    home.asked
      .filter((req) => req.t === 'cmd')
      .map((req) => JSON.parse((req.env as p.Envelope).p) as Record<string, unknown>)
      .filter((payload) => payload.t === 'act');

  const settled = async (count: number) => {
    await vi.waitFor(() => expect(acts()).toHaveLength(count));
    await new Promise((applied) => setTimeout(applied, 5));
    flushSync();
  };

  const buttonNamed = (root: ParentNode, label: string) =>
    [...root.querySelectorAll<HTMLButtonElement>('button')].find((b) =>
      b.textContent!.replace(/\s+/g, ' ').trim().includes(label),
    )!;

  AfterEachScenario(closePage);

  Background(({ Given }) => {
    Given(
      'the chat of "Website" open with its secretary, working AIs and task "T-12"',
      async () => {
        home = new ActHome();
        home.add(website, 'Website');
        home.write(
          website,
          example('row-member-ai', {
            id: secretary,
            name: 'Secretary',
            cli: 'claude',
            secretary: true,
          }),
        );
        home.write(website, example('row-member-ai', { id: codex, name: 'Codex', cli: 'codex' }));
        home.write(
          website,
          example('row-member-ai', { id: claude, name: 'Claude Code', cli: 'claude' }),
        );
        home.write(website, example('row-task'));
        reader = browser(home);
        homes.push(reader.store);
        const target = home.synced + home.groups.size;
        await reader.connect();
        await vi.waitFor(() => expect(home.synced).toBe(target));
        await new Promise((applied) => setTimeout(applied));
        openPage(`#/groups/${website}`);
      },
    );
  });

  Scenario(
    'Confirming a pending brief sends act confirm and folds the confirmed brief',
    ({ Given, Then, When, And }) => {
      Given('the secretary has posted a pending brief', () => {
        home.write(website, example('row-msg-brief-pending'));
        flushSync();
      });
      Then(
        'the brief card stands expanded, marked "Needs you", with its goal, criteria, scope and split',
        () => {
          const card = one('.msg-body .card.need');
          expect(texts('.msg-body .card .card-head .badge')).toEqual(['Needs you']);
          expect(card.querySelector('h3')!.textContent).toBe('Show connection paths');
          expect(card.querySelector('details.fold')).toBeNull();
          expect(texts('.msg-body .card .num li')).toEqual(['Show a path change within 1 s.']);
          expect(texts('.msg-body .card .pill-do')).toEqual(['In']);
          expect(texts('.msg-body .card .pill-dont')).toEqual(['Out']);
          const split = texts('.msg-body .card .split li');
          expect(split[0]).toContain('Report path changes');
          expect(split[0]).toContain('Foundation');
          expect(split[0]).toContain('Codex · reviewed by Claude Code');
          expect(split[1]).toContain('Show the path');
          expect(split[1]).toContain('(after #1)');
          expect(split[1]).toContain('Claude Code · reviewed by Codex');
          expect(split[2]).toContain('Pass score 90 (foundation 70)');
        },
      );
      When('the owner presses "Confirm & start" on the brief card', async () => {
        buttonNamed(one('.msg-body .card'), 'Confirm & start').click();
        flushSync();
        await settled(1);
      });
      Then('the home receives a signed "confirm" act on the brief with no extra fields', () => {
        expect(acts()).toEqual([
          {
            t: 'act',
            n: expect.stringMatching(/^[A-Za-z0-9_-]{22}$/),
            ts: expect.any(Number),
            g: website,
            msg: 'KaiCW9JC8UOG7lKNduDo8Q',
            action: 'confirm',
          },
        ]);
      });
      And('the brief card shows "Alex confirmed" and folds its details', () => {
        expect(all('.msg-body .card.need')).toEqual([]);
        expect(one('.msg-body .card .card-head .badge').textContent).toContain('Alex confirmed ·');
        const fold = one<HTMLDetailsElement>('.msg-body .card details.fold');
        expect(fold.open).toBe(false);
        expect(fold.querySelector('summary b')!.textContent).toBe('Goal, acceptance, scope, split');
      });
    },
  );

  Scenario(
    'Sending a brief back with changes checks the 1 KB note limit and shows superseded state',
    ({ Given, When, Then, And }) => {
      Given('the secretary has posted a pending brief', () => {
        home.write(website, example('row-msg-brief-pending'));
        flushSync();
      });
      When('the owner presses "Change it" on the brief card', () => {
        buttonNamed(one('.msg-body .card'), 'Change it').click();
        flushSync();
      });
      And('the owner types a note of 1023 letters on the brief card', () => {
        const area = one<HTMLTextAreaElement>('.msg-body .card textarea');
        fill(area, 'a'.repeat(1023));
        flushSync();
      });
      Then(
        'the brief card warns that the note is too long and keeps "Send changes" disabled',
        () => {
          const card = one('.msg-body .card');
          expect(texts('.msg-body .card [role=status]')).toEqual([
            'Too long: shorten the note to fit 1 KB.',
          ]);
          expect(buttonNamed(card, 'Send changes').disabled).toBe(true);
          // Exact 1024-byte JSON boundary (1022 ASCII letters + 2 quotes = 1024 bytes) is accepted.
          fill(one<HTMLTextAreaElement>('.msg-body .card textarea'), 'a'.repeat(1022));
          flushSync();
          expect(buttonNamed(card, 'Send changes').disabled).toBe(false);
          // Cancel closes the revise form cleanly before reopening it.
          buttonNamed(card, 'Cancel').click();
          flushSync();
          expect(card.querySelector('textarea')).toBeNull();
          buttonNamed(card, 'Change it').click();
          flushSync();
        },
      );
      When('the owner types "Include the relay path." and sends the changes', async () => {
        fill(one<HTMLTextAreaElement>('.msg-body .card textarea'), '  Include the relay path.  ');
        flushSync();
        one('.msg-body .card form').dispatchEvent(
          new Event('submit', { bubbles: true, cancelable: true }),
        );
        flushSync();
        await settled(1);
      });
      Then(
        'the home receives a signed "revise" act on the brief with text "Include the relay path."',
        () => {
          expect(acts()[0]).toEqual({
            t: 'act',
            n: expect.stringMatching(/^[A-Za-z0-9_-]{22}$/),
            ts: expect.any(Number),
            g: website,
            msg: 'KaiCW9JC8UOG7lKNduDo8Q',
            action: 'revise',
            text: 'Include the relay path.',
          });
        },
      );
      And(
        'the brief card shows "Alex asked for changes" with the note "Include the relay path."',
        () => {
          const badge = one('.msg-body .card .card-head .badge');
          expect(badge.textContent).toContain('Alex asked for changes ·');
          expect(badge.getAttribute('data-tone')).toBeNull();
          expect(texts('.msg-body .card .revise-note')).toEqual(['Include the relay path.']);
          expect(one<HTMLDetailsElement>('.msg-body .card details.fold').open).toBe(false);
        },
      );
      When('the home marks the brief superseded', () => {
        home.write(website, example('row-msg-brief-superseded'));
        flushSync();
      });
      Then('the brief card shows "Superseded" and keeps the note "Include the relay path."', () => {
        expect(texts('.msg-body .card .card-head .badge')).toEqual(['Superseded']);
        expect(texts('.msg-body .card .revise-note')).toEqual(['Include the relay path.']);
      });
    },
  );

  const fourOptions = [
    { id: 'A', label: 'Top banner', effect: 'Always visible.' },
    { id: 'B', label: 'Connection indicator', effect: 'Quiet; click for details.' },
    { id: 'C', label: 'Toast once', effect: 'Fades after 5 seconds.' },
    { id: 'D', label: 'Settings page only', effect: 'Never interrupts.' },
  ];

  Scenario(
    'Picking an option on a decision card sends act choose, alone or with a note',
    ({ Given, Then, When, And }) => {
      Given('an AI has posted a pending decision with four options recommending "B"', () => {
        home.write(
          website,
          example('row-msg-decision-pending', {
            body: {
              task: taskId,
              question: 'Where should the path appear?',
              context: 'Four places fit the layout.',
              options: fourOptions,
              recommended: 'B',
              to: [user],
            },
          }),
        );
        flushSync();
      });
      Then(
        'the decision card shows all four options, marks "B" as "Recommended", and offers a free-text field',
        () => {
          const card = one('.msg-body .card.need');
          expect(card.getAttribute('aria-label')).toBe('Your call');
          expect(texts('.msg-body .card .card-head .mono')).toEqual(['T-12']);
          expect(texts('.msg-body .card .opts .opt b')).toEqual([
            'A · Top banner',
            'B · Connection indicator',
            'C · Toast once',
            'D · Settings page only',
          ]);
          expect(texts('.msg-body .card .opts .opt .badge')).toEqual(['Recommended']);
          const input = card.querySelector<HTMLInputElement>('input[type=text]')!;
          expect(input.placeholder).toBe('Your own answer, or a note');
          expect(buttonNamed(card, 'Send answer').disabled).toBe(true);
        },
      );
      When('the owner picks option "A" without typing a note', async () => {
        buttonNamed(one('.msg-body .card'), 'A · Top banner').click();
        flushSync();
        await settled(1);
      });
      Then('the home receives a signed "choose" act with arg "A" and no text', () => {
        expect(acts()[0]).toEqual({
          t: 'act',
          n: expect.stringMatching(/^[A-Za-z0-9_-]{22}$/),
          ts: expect.any(Number),
          g: website,
          msg: 'hq411Ypqo7V0LflO-dcWIg',
          action: 'choose',
          arg: 'A',
        });
        expect('text' in acts()[0]).toBe(false);
      });
      And('the decision card shows "Alex decided" and marks option "A" as "Chosen"', () => {
        expect(one('.msg-body .card .card-head .badge').textContent).toContain('Alex decided ·');
        expect(texts('.msg-body .card .opt.chosen b')).toEqual(['A · Top banner']);
        expect(texts('.msg-body .card .opt.chosen .badge')).toEqual(['Chosen']);
      });
      Given('an AI has posted a second pending decision with options "A" and "B"', () => {
        home.write(
          website,
          example('row-msg-decision-pending', {
            id: 'secondDecisionCard000A',
            body: {
              task: taskId,
              question: 'Keep old logs?',
              options: fourOptions.slice(0, 2),
              recommended: 'B',
              to: [user],
            },
          }),
        );
        flushSync();
      });
      When(
        'the owner types "Quiet indicator." in the decision field and picks option "B"',
        async () => {
          const second = all('.msg-body .card')[1];
          fill(second.querySelector<HTMLInputElement>('input')!, '  Quiet indicator.  ');
          flushSync();
          buttonNamed(second, 'B · Connection indicator').click();
          flushSync();
          await settled(2);
        },
      );
      Then(
        'the home receives a signed "choose" act with arg "B" and text "Quiet indicator."',
        () => {
          expect(acts()[1]).toEqual({
            t: 'act',
            n: expect.stringMatching(/^[A-Za-z0-9_-]{22}$/),
            ts: expect.any(Number),
            g: website,
            msg: 'secondDecisionCard000A',
            action: 'choose',
            arg: 'B',
            text: 'Quiet indicator.',
          });
        },
      );
      And(
        'the second decision card marks option "B" as "Chosen" and shows "Quiet indicator."',
        () => {
          const second = all('.msg-body .card')[1];
          expect(second.querySelector('.opt.chosen b')!.textContent).toBe(
            'B · Connection indicator',
          );
          expect(second.querySelector('.answer')!.textContent).toBe('Quiet indicator.');
        },
      );
    },
  );

  Scenario(
    'Answering a decision card in free text works both with options and without options',
    ({ Given, When, Then, And }) => {
      Given('an AI has posted a pending decision with four options recommending "B"', () => {
        home.write(
          website,
          example('row-msg-decision-pending', {
            body: {
              task: taskId,
              question: 'Where should the path appear?',
              options: fourOptions,
              recommended: 'B',
              to: [user],
            },
          }),
        );
        flushSync();
      });
      When('the owner types a free-text answer of 1023 letters on the decision card', () => {
        fill(one<HTMLInputElement>('.msg-body .card input'), 'x'.repeat(1023));
        flushSync();
      });
      Then(
        'the decision card warns that the answer is too long and disables its options and send button',
        () => {
          const card = one('.msg-body .card');
          expect(texts('.msg-body .card [role=status]')).toEqual([
            'Too long: shorten your answer to fit 1 KB.',
          ]);
          expect(buttonNamed(card, 'A · Top banner').disabled).toBe(true);
          expect(buttonNamed(card, 'Send answer').disabled).toBe(true);
          // Exact 1024-byte JSON boundary (1022 ASCII letters + 2 quotes = 1024 bytes) is enabled.
          fill(one<HTMLInputElement>('.msg-body .card input'), 'x'.repeat(1022));
          flushSync();
          expect(buttonNamed(card, 'Send answer').disabled).toBe(false);
        },
      );
      When(
        'the owner types "Put a subtle dot in the footer instead." and submits the free-text answer',
        async () => {
          const input = one<HTMLInputElement>('.msg-body .card input');
          fill(input, '  Put a subtle dot in the footer instead.  ');
          flushSync();
          // IME composition Enter does not submit.
          const composing = new KeyboardEvent('keydown', {
            key: 'Enter',
            bubbles: true,
            cancelable: true,
            isComposing: true,
          });
          input.dispatchEvent(composing);
          expect(composing.defaultPrevented).toBe(true);
          one('.msg-body .card form').dispatchEvent(
            new Event('submit', { bubbles: true, cancelable: true }),
          );
          flushSync();
          await settled(1);
        },
      );
      Then(
        'the home receives a signed "choose" act with text "Put a subtle dot in the footer instead." and no arg',
        () => {
          expect(acts()[0]).toEqual({
            t: 'act',
            n: expect.stringMatching(/^[A-Za-z0-9_-]{22}$/),
            ts: expect.any(Number),
            g: website,
            msg: 'hq411Ypqo7V0LflO-dcWIg',
            action: 'choose',
            text: 'Put a subtle dot in the footer instead.',
          });
          expect('arg' in acts()[0]).toBe(false);
        },
      );
      And(
        'the decision card shows "Alex decided" and "Put a subtle dot in the footer instead." with no option marked "Chosen"',
        () => {
          expect(one('.msg-body .card .card-head .badge').textContent).toContain('Alex decided ·');
          expect(all('.msg-body .card .opt.chosen')).toEqual([]);
          expect(texts('.msg-body .card .answer')).toEqual([
            'Put a subtle dot in the footer instead.',
          ]);
        },
      );
      Given('an AI has posted a pending free-text decision with no options', () => {
        home.write(
          website,
          example('row-msg-decision-pending', {
            id: 'freeTextOnlyDecision0A',
            thread: null,
            main: false,
            body: {
              question: 'How many days should local logs be retained?',
              options: [],
              to: [user],
            },
          }),
        );
        flushSync();
      });
      When(
        'the owner types "Keep logs for 30 days." and submits the free-text answer',
        async () => {
          const second = all('.msg-body .card')[1];
          expect(second.querySelector('.btn-dawn')!.textContent!.trim()).toBe('Send answer');
          fill(second.querySelector<HTMLInputElement>('input')!, 'Keep logs for 30 days.');
          flushSync();
          second
            .querySelector('form')!
            .dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }));
          flushSync();
          await settled(2);
        },
      );
      Then(
        'the home receives a signed "choose" act with text "Keep logs for 30 days." and no arg',
        () => {
          expect(acts()[1]).toEqual({
            t: 'act',
            n: expect.stringMatching(/^[A-Za-z0-9_-]{22}$/),
            ts: expect.any(Number),
            g: website,
            msg: 'freeTextOnlyDecision0A',
            action: 'choose',
            text: 'Keep logs for 30 days.',
          });
          expect('arg' in acts()[1]).toBe(false);
        },
      );
    },
  );

  Scenario(
    'Cards waiting on another person, expired decisions, offline state and refused actions stay clear',
    ({ Given, And, Then, When }) => {
      Given('"Mina" is a non-admin member of "Website"', () => {
        home.write(
          website,
          example('row-member-human', {
            id: 'mina-member-id',
            user: 'mina-user-id',
            name: 'Mina',
            role: 'member',
          }),
        );
      });
      And(
        'an AI has posted a pending decision addressed only to "Mina", and an expired decision',
        () => {
          home.write(
            website,
            example('row-msg-decision-pending', {
              id: 'minaDecisionCard00000A',
              body: {
                question: 'Which branch name?',
                options: [{ id: 'A', label: 'feat/x', effect: 'Short.' }],
                to: ['mina-user-id'],
              },
            }),
          );
          home.write(
            website,
            example('row-msg-decision-expired', { id: 'expiredDecisionCard00A' }),
          );
          flushSync();
        },
      );
      And(
        'the home also holds the golden confirmed brief, revise brief and decided decision',
        () => {
          home.write(website, example('row-msg-brief-confirmed', { id: 'goldenBriefConfirmed0A' }));
          home.write(website, example('row-msg-brief-revise', { id: 'goldenBriefRevise0000A' }));
          home.write(
            website,
            example('row-msg-decision-decided', { id: 'goldenDecisionDecidedA' }),
          );
          home.write(
            website,
            example('row-msg-brief-pending', {
              id: 'minaBriefPending00000A',
              body: {
                ...(example('row-msg-brief-pending').body as Record<string, unknown>),
                requester: 'mina-member-id',
              },
            }),
          );
          flushSync();
        },
      );
      Then(
        'the decision for "Mina" says "Waiting on Mina", disables its options and hides the answer form',
        () => {
          const [minaCard] = all('.msg-body .card');
          expect(minaCard.classList.contains('need')).toBe(false);
          expect(minaCard.querySelector('.card-head .badge')!.textContent!.trim()).toBe(
            'Waiting on Mina',
          );
          expect(minaCard.querySelector<HTMLButtonElement>('button.opt')!.disabled).toBe(true);
          expect(minaCard.querySelector('form')).toBeNull();
        },
      );
      And('the expired decision shows "Expired"', () => {
        const expired = all('.msg-body .card')[1];
        expect(expired.querySelector('.card-head .badge')!.textContent).toContain('Expired ·');
        expect(expired.querySelectorAll('.opt.static')).toHaveLength(1);
      });
      And(
        'the golden confirmed brief, revise brief and decided decision show their acted states',
        () => {
          const [, , confirmedBrief, reviseBrief, decidedDecision, minaBrief] =
            all('.msg-body .card');
          expect(confirmedBrief.querySelector('.card-head .badge')!.textContent).toContain(
            'Alex confirmed ·',
          );
          const reviseBadge = reviseBrief.querySelector('.card-head .badge')!;
          expect(reviseBadge.textContent).toContain('Alex asked for changes ·');
          expect(reviseBadge.getAttribute('data-tone')).toBeNull();
          expect(reviseBrief.querySelector('.revise-note')!.textContent).toBe(
            'Include the relay path.',
          );
          expect(decidedDecision.querySelector('.opt.chosen b')!.textContent).toBe('A · Header');
          expect(decidedDecision.querySelector('.answer')!.textContent).toBe('Header works.');
          expect(minaBrief.classList.contains('need')).toBe(true);
          const minaBriefBadge = minaBrief.querySelector('.card-head .badge')!;
          expect(minaBriefBadge.textContent!.trim()).toBe('Waiting on Mina');
          expect(minaBriefBadge.getAttribute('data-tone')).toBeNull();
        },
      );
      Given(
        'the secretary has posted a pending brief for the owner, and the home refuses acts with "conflict"',
        () => {
          home.refuse = 'conflict';
          home.write(website, example('row-msg-brief-pending'));
          flushSync();
        },
      );
      When('the owner presses "Confirm & start" on the brief card', async () => {
        const brief = all('.msg-body .card')[6];
        buttonNamed(brief, 'Confirm & start').click();
        flushSync();
        await settled(1);
      });
      Then('the brief card reports the refusal "conflict"', () => {
        const brief = all('.msg-body .card')[6];
        expect(brief.querySelector('[role=alert]')!.textContent).toContain('conflict');
      });
      When('the connection to the home is lost', () => {
        home.stop();
        reader.drop();
        flushSync();
      });
      Then('the pending brief disables its action buttons while offline', () => {
        const brief = all('.msg-body .card')[6];
        expect(buttonNamed(brief, 'Confirm & start').disabled).toBe(true);
        expect(buttonNamed(brief, 'Change it').disabled).toBe(true);
      });
    },
  );

  Scenario(
    'Summary cards show the done task, review score, rounds and formatted duration',
    ({ Given, And, Then }) => {
      Given(
        'the secretary has posted the golden summary for "T-12" with text "All criteria passed."',
        () => {
          home.write(website, example('row-msg-summary', { text: 'All criteria passed.' }));
        },
      );
      And(
        'the secretary has posted summaries of 1 round in 45 minutes and of 3 rounds in 2 hours for a task the browser does not hold',
        () => {
          home.write(
            website,
            example('row-msg-summary', {
              id: 'localSummaryCard00000A',
              body: { task: 'missing-task', score: 91, rounds: 1, minutes: 45 },
            }),
          );
          home.write(
            website,
            example('row-msg-summary', {
              id: 'exactHourSummaryCard0A',
              body: { task: 'missing-task', score: 100, rounds: 3, minutes: 120 },
            }),
          );
          flushSync();
        },
      );
      Then(
        'the first summary card shows "Done", "T-12", "Show connection path", "94", "2 rounds" and "1h 12m"',
        () => {
          expect(texts('.msg-body .text')).toContain('All criteria passed.');
          const first = all('.msg-body .card')[0];
          expect(first.querySelector('.kind')!.textContent).toBe('Done');
          expect(first.querySelector('.card-head .mono')!.textContent).toBe('T-12');
          expect(first.querySelector('.card-head .badge')!.getAttribute('data-tone')).toBe('ok');
          expect(first.querySelector('h3')!.textContent).toBe('Show connection path');
          expect(
            [...first.querySelectorAll('.stats li')].map((li) =>
              li.textContent!.replace(/\s+/g, ' ').trim(),
            ),
          ).toEqual(['94Review score', '2 roundsReview rounds', '1h 12mTime spent']);
        },
      );
      And(
        'the second summary card shows "1 round" and "45m" without a title, and the third "2h"',
        () => {
          const second = all('.msg-body .card')[1];
          expect(second.querySelector('h3')).toBeNull();
          expect(
            [...second.querySelectorAll('.stats li')].map((li) =>
              li.textContent!.replace(/\s+/g, ' ').trim(),
            ),
          ).toEqual(['91Review score', '1 roundReview rounds', '45mTime spent']);
          const third = all('.msg-body .card')[2];
          expect(third.querySelectorAll('.stats li b')[2].textContent).toBe('2h');
        },
      );
    },
  );
});
