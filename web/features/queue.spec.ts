import { describeFeature, loadFeature } from '@amiceli/vitest-cucumber';
import { flushSync } from 'svelte';
import { expect, vi } from 'vitest';
import { find, homes, newId } from '../src/lib/groups.svelte';
import * as p from '../src/lib/proto';
import { browser, example, owner, TestHome } from './home';
import { all, closePage, one, openPage, read, text, texts } from './page';

const feature = await loadFeature('../features/web_queue.feature');
const g = 'rZNvy-1jH6Z-BcPqA5U5BQ';
const codex = '1PC8WineBrUQ-apCjx7tug';
const claude = 'Y2xhdWRlLW1lbWJlci0wMQ';
const mina = 'bWluYS1tZW1iZXItMDAwMQ';
const task = example('row-task').id as string;

describeFeature(feature, ({ Background, Scenario, AfterEachScenario }) => {
  let home: TestHome;
  let alex: ReturnType<typeof browser>;
  let answer: object;
  let sent: string[];

  const working = { g, name: 'Codex', state: 'working', task };
  const reviewer = 'cmV2aWV3ZXItMDAwMDAwMDE';
  const settled = (check: () => void) =>
    vi.waitFor(() => {
      flushSync();
      check();
    });
  const say = (by = owner) => {
    const row = home.say(g, by, '@Codex where are you?', { id: newId(), mentions: [codex] });
    flushSync();
    return row.id as string;
  };
  const button = (name: string) =>
    all<HTMLButtonElement>('.wait button').find((each) => read(each) === name)!;
  const press = (name: string) => {
    button(name).click();
    flushSync();
  };
  /** Codex takes the message: its turn is of its group session, and T-12 waits for it. */
  const take = () => {
    home.write(g, example('row-member-ai', { ...working, task: undefined }));
    const wait = { kind: 'preempted', since: Date.now() };
    home.write(g, example('row-task', { g, assignee: codex, num: 12, state: 'paused', wait }));
    flushSync();
  };
  const silent = () => settled(() => expect(all('.wait')).toEqual([]));

  AfterEachScenario(() => closePage());

  Background(({ Given }) => {
    Given(
      'Alex in "Website", where Codex works on T-12 since before anything Alex writes',
      async () => {
        answer = { ok: true };
        sent = [];
        home = new TestHome();
        home.reply = (payload) => {
          sent.push(p.decode(p.Jump, JSON.stringify(payload)).msg);
          return answer;
        };
        home.add(g, 'Website');
        home.write(g, example('row-member-human', { id: mina, g, name: 'Mina', user: 'mina' }));
        home.write(g, example('row-task', { g, assignee: codex, num: 12 }));
        home.write(g, example('row-member-ai', working));
        alex = browser(home);
        homes.push(alex.store);
        await alex.connect();
        await vi.waitFor(() => expect(find(g)?.task(task)).toBeDefined());
        openPage(`#/groups/${g}`);
      },
    );
  });

  Scenario('The author lets a waiting message go first', ({ When, Then }) => {
    let id: string;
    When('Alex writes to Codex', () => {
      id = say();
    });
    Then(
      'the message says that Codex is busy with T-12 and offers "Go first, pausing T-12"',
      () => {
        expect(text(`#m-${id} .wait span`)).toBe('Codex is busy with T-12. This waits its turn.');
        expect(texts('.wait button')).toEqual(['Go first, pausing T-12']);
      },
    );
    When('Alex presses "Go first, pausing T-12"', () => {
      press('Go first, pausing T-12');
      expect(one<HTMLButtonElement>('.wait button').disabled).toBe(true);
    });
    Then(
      'the home receives "jump" for that message, and it reads "Codex takes this next." without a button',
      async () => {
        await settled(() => expect(text('.wait')).toBe('Codex takes this next.'));
        expect(sent).toEqual([id]);
        expect(all('.wait button')).toEqual([]);
        await settled(() => expect(document.activeElement).toBe(one('.wait')));
      },
    );
    When('Codex takes the message and T-12 is pre-empted', take);
    Then('the message says that T-12 is paused while Codex takes it', () =>
      expect(texts('.wait')).toEqual([
        'T-12 is paused while Codex takes this. It continues by itself.',
      ]),
    );
    When('T-12 continues', () => {
      home.write(g, example('row-task', { g, assignee: codex, num: 12 }));
      flushSync();
    });
    Then('the message says nothing of it any more', silent);
  });

  Scenario("A reviewer's review is interrupted, not a task of its own", ({ Given, When, Then }) => {
    Given('Codex reviews T-12', () => {
      home.write(g, example('row-task', { g, assignee: reviewer, num: 12 }));
    });
    When('Alex writes to Codex', () => void say());
    Then('the message offers "Go first, interrupting the review"', () =>
      expect(texts('.wait button')).toEqual(['Go first, interrupting the review']),
    );
  });

  Scenario('Every waiting message can go first', ({ When, Then }) => {
    let ids: string[];
    When('Alex writes to Codex twice', () => {
      ids = [say(), say()];
    });
    Then('both messages offer "Go first, pausing T-12"', () => {
      expect(all('.wait button').map((each) => [each.closest('.msg')!.id, read(each)])).toEqual(
        ids.map((id) => [`m-${id}`, 'Go first, pausing T-12']),
      );
    });
  });

  Scenario('A message that waits for two AIs goes first with both', ({ Given, When, Then }) => {
    let id: string;
    Given('Claude works on T-13', () => {
      const t13 = 'dGFzay0xMy0wMDAwMDAwMQ';
      home.write(g, example('row-task', { g, id: t13, assignee: claude, num: 13 }));
      home.write(
        g,
        example('row-member-ai', { g, id: claude, name: 'Claude', state: 'working', task: t13 }),
      );
    });
    When('Alex writes to Codex and Claude', () => {
      id = newId();
      home.say(g, owner, '@Codex @Claude where are you?', { id, mentions: [codex, claude] });
      flushSync();
    });
    Then(
      'both lines say that the message waits, and one choice offers "Go first, interrupting T-13 and T-12"',
      () => {
        expect(texts('.wait span')).toEqual([
          'Claude is busy with T-13. This waits its turn.',
          'Codex is busy with T-12. This waits its turn.',
        ]);
        expect(texts('.wait button')).toEqual(['Go first, interrupting T-13 and T-12']);
      },
    );
    When('Alex presses "Go first, interrupting T-13 and T-12"', () =>
      press('Go first, interrupting T-13 and T-12'),
    );
    Then('the home receives one "jump", and both AIs take the message next', async () => {
      await settled(() =>
        expect(texts('.wait')).toEqual(['Claude takes this next.', 'Codex takes this next.']),
      );
      expect(sent).toEqual([id]);
    });
    When('Codex takes the message and T-12 is pre-empted', take);
    Then('the message says that T-12 is paused and that Claude takes it next', () =>
      expect(texts('.wait')).toEqual([
        'T-12 is paused while Codex takes this. It continues by itself.',
        'Claude takes this next.',
      ]),
    );
  });

  Scenario('A message in the thread of the task that the AI works on', ({ Given, When, Then }) => {
    let id: string;
    Given("T-12's card in the main line, its thread open", async () => {
      home.write(g, example('row-msg-task', { g, id: task, body: { task } }));
      await settled(() => expect(all('.card details')).toHaveLength(1));
      one<HTMLDetailsElement>('.card details').open = true;
      one('.card details').dispatchEvent(new Event('toggle'));
      await settled(() => expect(all('.card .in-thread')).toHaveLength(1));
    });
    When('Alex writes to Codex in the thread', () => {
      id = newId();
      home.say(g, owner, '@Codex and the docs', { id, mentions: [codex], thread: task });
      flushSync();
    });
    Then('the message offers "Go first, interrupting what Codex is doing"', () =>
      expect(texts('.thread .wait button')).toEqual(['Go first, interrupting what Codex is doing']),
    );
    When('Alex presses "Go first, interrupting what Codex is doing"', () =>
      press('Go first, interrupting what Codex is doing'),
    );
    Then('the home receives "jump" for the message in the thread', async () => {
      await settled(() => expect(texts('.thread .wait')).toEqual(['Codex takes this next.']));
      expect(sent).toEqual([id]);
    });
  });

  Scenario('The message was taken in the meantime', ({ Given, When, And, Then }) => {
    Given('the home answers "conflict" to a jump', () => {
      answer = { err: { code: 'conflict', msg: 'nothing queued' } };
    });
    When('Alex writes to Codex', () => void say());
    And('Alex presses "Go first, pausing T-12"', () => press('Go first, pausing T-12'));
    Then('the message says nothing of it any more', async () => {
      await silent();
      expect(all('.msg .fail')).toEqual([]);
    });
  });

  Scenario('The home refuses for another reason', ({ Given, When, And, Then }) => {
    Given('the home answers "stale" to a jump', () => {
      answer = { err: { code: 'stale', msg: 'the clock is off' } };
    });
    When('Alex writes to Codex', () => void say());
    And('Alex presses "Go first, pausing T-12"', () => press('Go first, pausing T-12'));
    Then('the message still offers it, and says why it was refused', async () => {
      await settled(() => expect(all('.msg .fail')).toHaveLength(1));
      expect(button('Go first, pausing T-12').disabled).toBe(false);
    });
    When('Codex takes the message', () => {
      home.write(g, example('row-member-ai', working));
      flushSync();
    });
    Then('neither is left', () => expect(all('.wait, .msg .fail')).toEqual([]));
  });

  Scenario("Somebody else's message says nothing", ({ When, Then }) => {
    When('Mina writes to Codex', () => void say(mina));
    Then('no message says that it waits', () => expect(all('.wait')).toEqual([]));
  });

  Scenario('The home is away', ({ When, And, Then }) => {
    When('Alex writes to Codex', () => void say());
    And('the home stops', () => {
      alex.drop();
      flushSync();
    });
    Then('the choice is disabled', () =>
      expect(button('Go first, pausing T-12').disabled).toBe(true),
    );
  });
});
