import { describeFeature, loadFeature } from '@amiceli/vitest-cucumber';
import { flushSync } from 'svelte';
import { expect, vi } from 'vitest';
import { homes, type Group } from '../src/lib/groups.svelte';
import * as p from '../src/lib/proto';
import { browser, example, owner, TestHome } from './home';
import { all, choose, closePage, one, openPage, read, texts } from './page';

const feature = await loadFeature('../features/web_taskops.feature');
const g = 'rZNvy-1jH6Z-BcPqA5U5BQ';
const codex = '1PC8WineBrUQ-apCjx7tug';
const claude = 'LXCZmuGAXkvO-bSrOkuCfw';
const secretary = 'qBSFMsr2hHYKOMblEA_kdA';
const qwen = 'cXdlbi1tZW1iZXItMDAwMQ';
const mina = 'bWluYS1tZW1iZXItMDAwMQ';
const t12 = example('row-task').id as string;

type Sent = p.TaskAssign | p.TaskCommand;

describeFeature(feature, ({ Background, Scenario, AfterEachScenario }) => {
  let home: TestHome;
  let alex: ReturnType<typeof browser>;
  let group: Group;
  let answer: object;
  let sent: Sent[];

  const settled = (check: () => void) =>
    vi.waitFor(() => {
      flushSync();
      check();
    });
  const wait = (kind: string) => ({ kind, since: Date.now() });
  const task = (changes: object = {}) => {
    const row = { g, assignee: codex, reviewer: claude, review: undefined, ...changes };
    home.write(g, example('row-task', row));
    flushSync();
  };
  const button = (name: string, within = '.card .acts') =>
    all<HTMLButtonElement>(`${within} button`).find((each) => read(each) === name)!;
  const press = (name: string, within?: string) => {
    button(name, within).click();
    flushSync();
  };
  const menu = () => texts('.menu .item');
  const pick = async (name: string) => {
    press(name, '.menu');
    await new Promise((later) => setTimeout(later));
    flushSync();
  };
  const openThread = async () => {
    one<HTMLDetailsElement>('.card details').open = true;
    one('.card details').dispatchEvent(new Event('toggle'));
    await settled(() => expect(all('.card .in-thread')).toHaveLength(1));
  };
  const field = () => one<HTMLTextAreaElement>('.in-thread textarea');
  const write = (text: string) => {
    field().value = text;
    field().setSelectionRange(text.length, text.length);
    field().dispatchEvent(new Event('input', { bubbles: true }));
    flushSync();
  };
  const note = () => texts('.in-thread .note-line');
  const hold = () => [texts('.hold p')[0], ...texts('.hold button')];
  const received = (op: string, roles: object = {}) =>
    settled(() => expect(sent.at(-1)).toMatchObject({ op, id: t12, g, ...roles }));
  const onMenu = () => settled(() => expect(document.activeElement).toBe(one('.more')));
  const withMina = () => {
    const person = { id: mina, g, name: 'Mina', user: 'mina', role: 'member' };
    home.write(g, example('row-member-human', person));
    const ai = { id: qwen, g, name: 'Qwen Code', user: 'mina', host_name: 'mina-mac' };
    home.write(g, example('row-member-ai', { ...ai, state: 'offline', task: undefined }));
    flushSync();
  };
  const named = () => all<HTMLSelectElement>('.ask select').map((each) => each.value);
  const submit = () =>
    one('.ask').dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }));
  const chooseAI = (role: number, label: string) =>
    choose(all<HTMLSelectElement>('.ask select')[role], label);

  AfterEachScenario(() => closePage());

  Background(({ Given }) => {
    Given(
      'the chat of "Website" open in a browser of Alex, its owner, with Codex on T-12 and Claude Code as its reviewer',
      async () => {
        answer = { ok: true };
        sent = [];
        home = new TestHome();
        home.reply = (payload) => {
          const text = JSON.stringify(payload);
          const assign = payload.op === 'assign';
          sent.push(assign ? p.decode(p.TaskAssign, text) : p.decode(p.TaskCommand, text));
          return answer;
        };
        home.add(g, 'Website');
        const ai = (id: string, name: string, changes: object = {}) =>
          home.write(
            g,
            example('row-member-ai', {
              id,
              g,
              name,
              state: 'waiting',
              task: undefined,
              ...changes,
            }),
          );
        ai(secretary, 'Secretary', { secretary: true });
        ai(codex, 'Codex');
        ai(claude, 'Claude Code');
        task();
        home.write(g, example('row-msg-task', { g }));
        alex = browser(home);
        homes.push(alex.store);
        await alex.connect();
        group = alex.store.groups[0];
        await vi.waitFor(() => expect(group.task(t12)).toBeDefined());
        openPage(`#/groups/${g}`);
      },
    );
  });

  Scenario("A person answers an AI's question in the thread", ({ Given, When, Then, And }) => {
    Given('Codex asked "@Alex keep the field?" in the thread of T-12', () => {
      home.write(
        g,
        example('row-msg-question', {
          ...{ id: 'asked', thread: t12, by: codex },
          ...{ text: '@Alex keep the field?', mentions: [owner] },
        }),
      );
    });
    When('Alex opens the thread of T-12', openThread);
    Then(
      'its composer is named "Message in T-12\'s thread" and says "Write in T-12\'s thread, @ an AI to reach it"',
      async () => {
        expect(field().getAttribute('aria-label')).toBe("Message in T-12's thread");
        expect(field().placeholder).toBe("Write in T-12's thread, @ an AI to reach it");
        expect(one<HTMLTextAreaElement>('.dock textarea').getAttribute('aria-label')).toBe(
          'Message',
        );
        await settled(() => expect(texts('.tlist .text')).toEqual(['@Alex keep the field?']));
      },
    );
    When('Alex types "@" there', () => write('@'));
    Then('it offers "Codex", "Claude Code", "Alex" and "Secretary" in this order', () =>
      expect(texts('.in-thread .mentions .cut')).toEqual([
        'Codex',
        'Claude Code',
        'Alex',
        'Secretary',
      ]),
    );
    When('Alex types "Yes, keep it." there', () => {
      expect(note()).toEqual([
        '@ an AI if one should answer. Otherwise this just stays in the thread.',
      ]);
      write('');
      expect(note()).toEqual([]);
      write('Yes, keep it.');
    });
    Then('it says "@ an AI if one should answer. Otherwise this just stays in the thread."', () =>
      expect(note()).toEqual([
        '@ an AI if one should answer. Otherwise this just stays in the thread.',
      ]),
    );
    When('Alex folds the thread and opens it again', async () => {
      const fold = one<HTMLDetailsElement>('.card details');
      fold.open = false;
      fold.dispatchEvent(new Event('toggle'));
      flushSync();
      await openThread();
    });
    Then('its composer still holds "Yes, keep it."', () =>
      expect(field().value).toBe('Yes, keep it.'),
    );
    When('Alex types "@Codex yes, keep it." there and sends it', () => {
      write('@Codex yes, keep it.');
      expect(note()).toEqual([]);
      one('.in-thread form').dispatchEvent(
        new Event('submit', { bubbles: true, cancelable: true }),
      );
    });
    Then('the home receives that text in the thread of T-12 with a mention of Codex', async () => {
      await settled(() => expect(field().value).toBe(''));
      const env = home.asked.findLast((req) => req.t === 'cmd')!.env as p.Envelope;
      expect(p.decode(p.Post, env.p)).toMatchObject({
        ...{ t: 'post', g, kind: 'text', thread: t12, main: false },
        ...{ text: '@Codex yes, keep it.', mentions: [codex] },
      });
    });
    And(
      'the thread shows "@Codex yes, keep it.", its composer is empty and the main line has no such message',
      async () => {
        await settled(() =>
          expect(texts('.tlist .text')).toEqual(['@Alex keep the field?', '@Codex yes, keep it.']),
        );
        expect(texts('.msg-body > .text')).toEqual([]);
        expect(one<HTMLTextAreaElement>('.dock textarea').value).toBe('');
      },
    );
  });

  Scenario('A reply carries on a task that the round limit paused', ({ Given, When, Then }) => {
    Given('T-12 is paused after 10 rounds', () =>
      task({ state: 'paused', wait: wait('rounds'), thread: { count: 12, rounds: 10 } }),
    );
    Then(
      'its card says "The AIs went 10 rounds without a person. Reply in its thread to carry on." with "Reply" and "Resume"',
      () => {
        expect(hold()).toEqual([
          'The AIs went 10 rounds without a person. Reply in its thread to carry on.',
          'Reply',
          'Resume',
        ]);
        expect(one('.hold').dataset.tone).toBe('dawn');
      },
    );
    When('Alex presses "Reply"', () => press('Reply'));
    Then('the thread of T-12 is open and focus is in its composer', async () => {
      await settled(() => expect(document.activeElement).toBe(field()));
      expect(one<HTMLDetailsElement>('.card details').open).toBe(true);
    });
    When('Alex types "Carry on." there', () => write('Carry on.'));
    Then('it says "Sending this carries the task on."', () =>
      expect(note()).toEqual(['Sending this carries the task on.']),
    );
  });

  Scenario(
    'Away from the home the thread takes no text, and only the main composer says why',
    ({ When, Then, And }) => {
      When('Alex opens the thread of T-12', openThread);
      And('the home stops', () => {
        home.stop();
        alex.drop();
        flushSync();
      });
      Then('both composers take no text and one notice says why', () => {
        expect(all<HTMLTextAreaElement>('.composer textarea').map((each) => each.disabled)).toEqual(
          [true, true],
        );
        expect(texts('.composer [role=status]')).toEqual([
          'Cannot reach the group home. What is already here can still be read.',
        ]);
      });
      And('the menu of T-12 offers nothing that can be pressed', () =>
        expect(all<HTMLButtonElement>('.menu button').map((each) => each.disabled)).toEqual([
          true,
          true,
          true,
        ]),
      );
    },
  );

  Scenario('An admin pauses a task and resumes it', ({ Then, When, And }) => {
    Then(
      'the menu of T-12 offers "Pause", "Reassign" and "Cancel task", and goes when the page scrolls',
      () => {
        expect(menu()).toEqual(['Pause', 'Reassign', 'Cancel task']);
        const panel = one('.menu');
        expect(one('.more').getAttribute('popovertarget')).toBe(panel.id);
        expect(one('.more').getAttribute('aria-label')).toBe('Actions on T-12');
        const hidden = vi.fn();
        panel.hidePopover = hidden;
        const toggle = (newState: string) => {
          for (const type of ['beforetoggle', 'toggle'])
            panel.dispatchEvent(Object.assign(new Event(type), { newState }));
        };
        const at = (top: number) =>
          vi
            .spyOn(one('.more'), 'getBoundingClientRect')
            .mockReturnValue({ top, bottom: top + 36, right: innerWidth - 40 } as DOMRect);
        at(100);
        toggle('open');
        expect([panel.style.top, panel.style.bottom, panel.style.right]).toEqual([
          '140px',
          'auto',
          '40px',
        ]);
        dispatchEvent(new Event('scroll'));
        expect(hidden).toHaveBeenCalledTimes(1);
        toggle('closed');
        dispatchEvent(new Event('scroll'));
        expect(hidden).toHaveBeenCalledTimes(1);
        at(innerHeight - 60);
        toggle('open');
        expect([panel.style.top, panel.style.bottom]).toEqual(['auto', '64px']);
        toggle('closed');
      },
    );
    When('Alex picks "Pause"', () => pick('Pause'));
    Then('the home receives "pause" for T-12 and focus is on its menu button', async () => {
      await received('pause');
      await onMenu();
    });
    When('T-12 is paused by hand', () => task({ state: 'paused', wait: wait('manual') }));
    Then(
      'its card says "The secretary or an admin paused it. Its AIs stay stopped until one of them resumes it." with "Resume"',
      () =>
        expect(hold()).toEqual([
          'The secretary or an admin paused it. Its AIs stay stopped until one of them resumes it.',
          'Resume',
        ]),
    );
    And('the menu of T-12 offers "Resume", "Reassign" and "Cancel task"', () =>
      expect(menu()).toEqual(['Resume', 'Reassign', 'Cancel task']),
    );
    When('Alex presses "Resume"', () => press('Resume'));
    Then('the home receives "resume" for T-12', () => received('resume'));
  });

  Scenario('A cancel takes two steps', ({ When, Then, And }) => {
    const asked = () =>
      expect(texts('.ask b, .ask p')).toEqual([
        'Cancel T-12 for good?',
        'Its AIs stop and its open cards close.',
        'Changes not committed yet are lost; the branch stays.',
      ]);
    When('Alex picks "Cancel task"', () => pick('Cancel task'));
    Then(
      'the card asks "Cancel T-12 for good?", says what that does, and nothing is sent',
      async () => {
        await settled(() => expect(document.activeElement).toBe(one('.ask')));
        asked();
        expect(sent).toEqual([]);
      },
    );
    When('Alex presses "Keep it"', () => press('Keep it'));
    Then('the question is gone, nothing is sent and focus is on the menu button', async () => {
      await onMenu();
      expect(all('.ask')).toEqual([]);
      expect(sent).toEqual([]);
    });
    When('Alex picks "Cancel task" again', () => pick('Cancel task'));
    And('Alex presses "Cancel task"', () => press('Cancel task'));
    Then('the home receives "cancel" for T-12 and focus is on its thread', async () => {
      await received('cancel');
      await settled(() => expect(document.activeElement).toBe(one('.card summary')));
    });
    When('T-12 is cancelled', () => task({ state: 'cancelled', wait: null }));
    Then('its card has no menu', () =>
      expect(all('.card :is(.more, .menu, .ask, .hold)')).toEqual([]),
    );
  });

  Scenario('An admin gives a task to another AI', ({ When, Then }) => {
    When('Alex picks "Reassign"', async () => {
      withMina();
      await pick('Reassign');
    });
    Then(
      'the form names Codex to do it and Claude Code to review it, and cannot be sent yet',
      () => {
        const [assignee, reviewer] = all<HTMLSelectElement>('.ask select');
        expect([assignee.value, reviewer.value]).toEqual([codex, claude]);
        expect([...assignee.options].map((option) => option.text.trim())).toEqual([
          'Claude Code · workstation',
          'Codex · workstation',
          'Qwen Code · mina-mac · Offline',
        ]);
        expect(assignee.options).toHaveLength(reviewer.options.length);
        expect(button('Reassign').disabled).toBe(true);
        expect(texts('.notes p')).toEqual([]);
      },
    );
    When('Alex chooses "Claude Code · workstation" to do it', () =>
      chooseAI(0, 'Claude Code · workstation'),
    );
    Then('the form says "One AI cannot both do the work and review it."', () => {
      expect(texts('.notes p')).toEqual(['One AI cannot both do the work and review it.']);
      expect(button('Reassign').disabled).toBe(true);
    });
    When('Alex chooses "Qwen Code · mina-mac · Offline" to do it', () =>
      chooseAI(0, 'Qwen Code · mina-mac · Offline'),
    );
    Then('the form says that Codex stops and that Mina is asked first', () =>
      expect(texts('.notes p')).toEqual([
        'Codex stops here. Changes it has not committed are lost; its commits carry over.',
        'Mina is asked first: Qwen Code is theirs.',
      ]),
    );
    When('Alex presses "Reassign"', () => {
      expect(button('Reassign').disabled).toBe(false);
      submit();
    });
    Then(
      'the home receives "assign" for T-12 with Qwen Code as its assignee and no reviewer',
      async () => {
        await received('assign', { assignee: qwen });
        expect(sent[0]).not.toHaveProperty('reviewer');
        await settled(() => expect(all('.ask')).toEqual([]));
      },
    );
  });

  Scenario('What others change while the form is open stays theirs', ({ When, And, Then }) => {
    When('Alex picks "Reassign"', async () => {
      withMina();
      await pick('Reassign');
    });
    And('the secretary gives T-12 to Qwen Code, and its review to Codex', () =>
      task({ assignee: qwen, reviewer: codex }),
    );
    Then('the form names Qwen Code to do it and Codex to review it, and cannot be sent yet', () => {
      expect(named()).toEqual([qwen, codex]);
      expect(button('Reassign').disabled).toBe(true);
      expect(texts('.notes p')).toEqual([]);
    });
    When('Alex chooses "Claude Code · workstation" to review it', () =>
      chooseAI(1, 'Claude Code · workstation'),
    );
    And('the secretary gives T-12 back to Codex', () => {
      task({ assignee: codex, reviewer: qwen });
      expect(named()).toEqual([codex, claude]);
    });
    And('Alex presses "Reassign"', () => void submit());
    Then(
      'the home receives "assign" for T-12 with Claude Code as its reviewer and no assignee',
      async () => {
        await received('assign', { reviewer: claude });
        expect(sent[0]).not.toHaveProperty('assignee');
      },
    );
  });

  Scenario(
    'A task without a reviewer is given one, and one with a merge conflict goes back',
    ({ Given, Then, When, And }) => {
      Given('T-12 waits for a reviewer', () =>
        task({ reviewer: undefined, wait: wait('reviewer') }),
      );
      Then(
        'its card says "Ready for review, and nobody is named to do it. The secretary or an admin names a reviewer." with "Name a reviewer"',
        () =>
          expect(hold()).toEqual([
            'Ready for review, and nobody is named to do it. The secretary or an admin names a reviewer.',
            'Name a reviewer',
          ]),
      );
      When('Alex presses "Name a reviewer"', async () => {
        press('Name a reviewer');
        await settled(() => expect(all('.ask select')).toHaveLength(2));
        const reviewer = all<HTMLSelectElement>('.ask select')[1];
        expect(reviewer.selectedOptions[0].text).toBe('Nobody yet');
      });
      And('Alex chooses "Claude Code · workstation" to review it', () =>
        chooseAI(1, 'Claude Code · workstation'),
      );
      And('Alex presses "Reassign"', () => {
        submit();
      });
      Then(
        'the home receives "assign" for T-12 with Claude Code as its reviewer and no assignee',
        async () => {
          await received('assign', { reviewer: claude });
          expect(sent[0]).not.toHaveProperty('assignee');
          await settled(() => expect(all('.ask')).toEqual([]));
        },
      );
      When('T-12 has a merge conflict', () => {
        task({ wait: wait('conflict'), step: 'local_check' });
        expect(hold()).toEqual([
          'The merge did not go through. The secretary or an admin hands it back to be fixed.',
          'Hand back to Codex',
        ]);
        expect(one('.hold').dataset.tone).toBeUndefined();
      });
      And('Alex presses "Hand back to Codex"', () => press('Hand back to Codex'));
      Then('the home receives "assign" for T-12 with Codex as its assignee', async () => {
        await received('assign', { assignee: codex });
        expect(sent).toHaveLength(2);
      });
    },
  );

  Scenario('A stopped task is resumed, and a refusal is shown', ({ Given, Then, When }) => {
    const stopped = () => task({ state: 'failed', failures: 3, wait: null });
    const refused = async (code: string) => {
      answer = { err: { code, msg: 'refused by the test' } };
      press('Resume');
      await settled(() => expect(all('.acts .fail')).toHaveLength(1));
    };
    Given('T-12 stopped after 3 failures', stopped);
    Then(
      'its card says "Its AIs stay stopped until you resume it or give it to another AI. The count then starts again." with "Resume"',
      () =>
        expect(hold()).toEqual([
          'Its AIs stay stopped until you resume it or give it to another AI. The count then starts again.',
          'Resume',
        ]),
    );
    When('the home answers "conflict" and Alex presses "Resume"', () => refused('conflict'));
    Then(
      'the card says "Nothing changed: the task or its AIs moved on in the meantime." and still offers "Resume"',
      () => {
        expect(texts('.acts .fail')).toEqual([
          'Nothing changed: the task or its AIs moved on in the meantime.',
        ]);
        expect(button('Resume').disabled).toBe(false);
      },
    );
    When('the home answers "forbidden" and Alex presses "Resume"', async () => {
      await refused('forbidden');
      await settled(() => expect(sent).toHaveLength(2));
    });
    Then(
      'the card says "Not sent: this device may not do that." and still offers "Resume"',
      async () => {
        await settled(() =>
          expect(texts('.acts .fail')).toEqual(['Not sent: this device may not do that.']),
        );
        expect(button('Resume').disabled).toBe(false);
      },
    );
    When('the home answers "paused" and Alex confirms a cancel', async () => {
      answer = { err: { code: 'paused', msg: 'refused by the test' } };
      await pick('Cancel task');
      expect(all('.acts .fail')).toEqual([]);
      press('Cancel task');
      await settled(() => expect(all('.ask .fail')).toHaveLength(1));
    });
    Then('the question says "Nothing changed: the task or its AIs moved on in the meantime."', () =>
      expect(texts('.ask .fail')).toEqual([
        'Nothing changed: the task or its AIs moved on in the meantime.',
      ]),
    );
    When('T-12 is carried on', () => task());
    Then('the refusal is gone', () => expect(all('.acts .fail')).toEqual([]));
  });

  Scenario('A question goes away when another person was faster', ({ When, And, Then }) => {
    When('Alex picks "Reassign"', async () => {
      await pick('Reassign');
      expect(all('.ask')).toHaveLength(1);
    });
    And('T-12 enters "merge"', () => task({ step: 'merge' }));
    Then('the form is gone and the menu of T-12 offers "Cancel task"', () => {
      expect(all('.ask')).toEqual([]);
      expect(menu()).toEqual(['Cancel task']);
    });
    When('T-12 has a merge conflict', () => task({ wait: wait('conflict') }));
    Then('the form stays away', () => expect(all('.ask')).toEqual([]));
    When('T-12 enters "merge" again', () => task({ step: 'merge' }));
    And('Alex picks "Cancel task"', () => pick('Cancel task'));
    Then('the card also says "It has passed review and may be merging right now."', () =>
      expect(texts('.ask p')[0]).toBe('It has passed review and may be merging right now.'),
    );
  });

  Scenario('A pre-empted task carries on by itself', ({ Given, Then }) => {
    Given('T-12 is pre-empted', () => task({ state: 'paused', wait: wait('preempted') }));
    Then(
      'its card says "A message jumped the queue. It carries on by itself afterwards." without a button',
      () =>
        expect(hold()).toEqual(['A message jumped the queue. It carries on by itself afterwards.']),
    );
  });

  Scenario('The owner of the AI said no', ({ Given, Then, When }) => {
    const line =
      'Mina said no to Qwen Code here. The secretary or an admin gives the role to another AI.';
    Given('Mina denied that Qwen Code takes T-12', () => {
      withMina();
      task({ assignee: qwen });
      const body = { what: 'direct', ai: qwen, to: ['mina'] };
      const re = example('row-task').brief;
      home.write(g, example('row-msg-confirm-denied', { thread: null, re, body }));
      flushSync();
    });
    Then(`its card says "${line}" with "Reassign"`, () => {
      expect(hold()).toEqual([line, 'Reassign']);
      expect(one('.hold').dataset.tone).toBeUndefined();
    });
    When('Alex presses "Reassign"', () => press('Reassign'));
    Then('the form names Qwen Code to do it', () => settled(() => expect(named()[0]).toBe(qwen)));
  });

  Scenario(
    'A member who is no admin sees why, and writes in the thread',
    ({ Given, Then, And, When }) => {
      Given('Alex is a member and T-12 stopped after 3 failures', () => {
        home.write(g, example('row-member-human', { g, name: 'Alex', role: 'member' }));
        task({ state: 'failed', failures: 3, wait: null });
      });
      Then(
        'its card says "Its AIs stay stopped until an admin resumes it or gives it to another AI." without a button',
        () =>
          expect(hold()).toEqual([
            'Its AIs stay stopped until an admin resumes it or gives it to another AI.',
          ]),
      );
      And('the card of T-12 has no menu', () => expect(all('.card .more')).toEqual([]));
      When('Alex opens the thread of T-12', openThread);
      Then('its composer takes text', () => expect(field().disabled).toBe(false));
    },
  );
});
