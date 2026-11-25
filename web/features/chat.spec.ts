import { describeFeature, loadFeature } from '@amiceli/vitest-cucumber';
import { flushSync } from 'svelte';
import { expect, vi } from 'vitest';
import { homes, type Group } from '../src/lib/groups.svelte';
import * as p from '../src/lib/proto';
import { browser, example, owner, TestHome } from './home';
import {
  all,
  closePage,
  field,
  one,
  openPage,
  otherBrowser,
  press,
  scrollToTop,
  send,
  texts,
  type,
} from './page';

const feature = await loadFeature('../features/web_chat.feature');

const website = 'rZNvy-1jH6Z-BcPqA5U5BQ';

describeFeature(feature, ({ Background, Scenario, AfterEachScenario }) => {
  let home: TestHome;
  let first: ReturnType<typeof browser>;
  let second: ReturnType<typeof browser>;
  let group: Group;

  const posts = () =>
    home.asked
      .filter((req) => req.t === 'cmd')
      .map((req) => ({ ...JSON.parse((req.env as p.Envelope).p), k: (req.env as p.Envelope).k }));
  const sendButton = () => one<HTMLButtonElement>('.composer .send');
  const said = () => texts('.composer [role=status]');
  /** What the home did with a command has reached the page. */
  const settled = async (count: number) => {
    await vi.waitFor(() => expect(posts()).toHaveLength(count));
    await new Promise((applied) => setTimeout(applied, 5));
    flushSync();
  };

  async function read(reader: ReturnType<typeof browser>) {
    const answered = home.synced + home.groups.size;
    await reader.connect();
    await vi.waitFor(() => expect(home.synced).toBe(answered));
    await new Promise((applied) => setTimeout(applied));
    flushSync();
  }

  AfterEachScenario(closePage);

  Background(({ Given }) => {
    Given(
      'the chat of the group "Website" open in a browser of its owner, set to English',
      async () => {
        home = new TestHome();
        home.add(website, 'Website');
        home.write(website, example('row-member-ai', { id: 'codex', name: 'Codex' }));
        home.write(
          website,
          example('row-member-ai', { id: 'claude', name: 'Claude Code', cli: 'claude' }),
        );
        first = browser(home);
        homes.push(first.store);
        await read(first);
        group = first.store.groups[0];
        openPage(`#/groups/${website}`);
        expect(texts('.chat-head h2')).toEqual(['Website']);
      },
    );
  });

  Scenario(
    'A message written in the composer appears in the main line of both browsers',
    ({ Given, When, Then, And }) => {
      Given('a second browser of the same person reads the group', async () => {
        second = await otherBrowser(home);
        await read(second);
      });
      When('the person writes "The path is direct." and sends it', async () => {
        type('  The path is direct.\n');
        expect(sendButton().disabled).toBe(false);
        send();
        await settled(1);
      });
      Then('the home is sent a post of kind "text" to the main line, signed by this device', () => {
        expect(posts()).toEqual([
          {
            t: 'post',
            n: expect.stringMatching(/^[A-Za-z0-9_-]{22}$/),
            ts: expect.any(Number),
            g: website,
            kind: 'text',
            thread: null,
            main: false,
            text: 'The path is direct.',
            mentions: [],
            re: null,
            body: {},
            k: 'this-browser',
          },
        ]);
      });
      And('the message "The path is direct." appears under the name "Alex", marked "You"', () => {
        expect(texts('.msg .text')).toEqual(['The path is direct.']);
        expect(texts('.msg-head b')).toEqual(['Alex']);
        expect(texts('.msg-head .badge')).toEqual(['You']);
        expect(one('.msg').id).toBe(`m-${posts()[0].n}`);
      });
      And('the second browser holds the message too', () => {
        expect(second.store.groups[0].main.map((msg) => msg.text)).toEqual(['The path is direct.']);
      });
      And('the composer is empty again', () => {
        expect(field().value).toBe('');
        expect(sendButton().disabled).toBe(true);
      });
    },
  );

  Scenario(
    'A mention is completed from the members and named in the post',
    ({ When, Then, And }) => {
      When('the person types "ask @c"', () => type('ask @c'));
      Then('the composer offers "Claude Code" and "Codex"', () => {
        expect(texts('[role=option] .cut')).toEqual(['Claude Code', 'Codex']);
        expect(all('[role=option]').map((o) => o.getAttribute('aria-selected'))).toEqual([
          'true',
          'false',
        ]);
        expect(field().getAttribute('aria-expanded')).toBe('true');
        expect(field().getAttribute('aria-activedescendant')).toBe(all('[role=option]')[0].id);
      });
      When('the person moves down once and presses Enter', async () => {
        press('ArrowDown');
        expect(field().getAttribute('aria-activedescendant')).toBe(all('[role=option]')[1].id);
        press('Enter');
        await vi.waitFor(() => expect(field().value).not.toBe('ask @c'));
        flushSync();
      });
      Then('the composer reads "ask @Codex " and offers nothing', () => {
        expect(field().value).toBe('ask @Codex ');
        expect(field().selectionStart).toBe(11);
        expect(all('[role=option]')).toEqual([]);
        expect(field().getAttribute('aria-expanded')).toBe('false');
        expect(posts()).toEqual([]);
      });
      When('the person adds "for the fields" and sends the message', async () => {
        type('ask @Codex for the fields');
        send();
        await settled(1);
      });
      Then('the post names the member "Codex" in its mentions', () => {
        expect(posts()[0]).toMatchObject({
          text: 'ask @Codex for the fields',
          mentions: ['codex'],
        });
      });
      And('the message shows "@Codex" as a mention', () => {
        expect(texts('.msg .text .at')).toEqual(['@Codex']);
        expect(texts('.msg .text')).toEqual(['ask @Codex for the fields']);
      });
    },
  );

  Scenario('The composer tells the writer before a message is too long', ({ When, Then, And }) => {
    When('the person writes a message of 5830 letters', () => type('a'.repeat(5830)));
    Then('the composer says nothing about its length', () => {
      expect(said()).toEqual([]);
    });
    When('the person writes "@Codex " and 5823 letters, which is as many characters', () => {
      const text = `@Codex ${'a'.repeat(5823)}`;
      expect(text).toHaveLength(5830);
      type(text);
    });
    Then('the composer says "Long message: 76% of what one message can hold."', () => {
      expect(said()).toEqual(['Long message: 76% of what one message can hold.']);
      expect(all('[role=option]')).toEqual([]);
    });
    And('the message can be sent', () => {
      expect(sendButton().disabled).toBe(false);
    });
    When('the person writes a message of 9000 letters', () => type('a'.repeat(9000)));
    Then(
      'the composer says "Too long to send: 114% of what one message can hold. Shorten it, or send it in parts."',
      () => {
        expect(said()).toEqual([
          'Too long to send: 114% of what one message can hold. Shorten it, or send it in parts.',
        ]);
      },
    );
    And('the message cannot be sent', async () => {
      expect(sendButton().disabled).toBe(true);
      send();
      press('Enter');
      await new Promise((later) => setTimeout(later, 5));
      expect(posts()).toEqual([]);
    });
  });

  Scenario('Messages are shown under the day they were written on', ({ Given, Then, And }) => {
    Given(
      'messages written yesterday at 23:50 and today at 00:10 and 00:12 by the same member',
      () => {
        const today = new Date().setHours(0, 0, 0, 0);
        home.say(website, 'codex', 'one', { ts: today - 10 * 60_000 });
        home.say(website, 'codex', 'two', { ts: today + 10 * 60_000 });
        home.say(website, 'codex', 'three', { ts: today + 12 * 60_000 });
        flushSync();
      },
    );
    Then(
      'the main line shows a day that begins with "yesterday" and a day that begins with "today"',
      () => {
        expect(texts('.sep').map((day) => day.split(' ')[0])).toEqual(['yesterday', 'today']);
        expect(all('.day').map((day) => day.querySelectorAll('.msg').length)).toEqual([1, 2]);
        expect(all('.sep time').map((time) => time.getAttribute('datetime'))).toEqual(
          [-1, 0].map((days) => {
            const day = new Date();
            day.setHours(0, 0, 0, 0);
            day.setDate(day.getDate() + days);
            return day.toISOString();
          }),
        );
      },
    );
    And('the last message is joined to the one before it, and the first of today is not', () => {
      expect(all('.msg').map((msg) => msg.classList.contains('joined'))).toEqual([
        false,
        false,
        true,
      ]);
      expect(texts('.msg-head b')).toEqual(['Codex', 'Codex']);
      expect(texts('.msg .text')).toEqual(['one', 'two', 'three']);
    });
  });

  Scenario('A message shows its mentions and its code as written', ({ Given, Then, And }) => {
    Given('"Codex" wrote "@Alex run `go test ./...` first" and mentioned the reader', () => {
      home.say(website, 'codex', '@Alex run `go test ./...` first', { mentions: [owner] });
      home.say(website, 'codex', '@Alex is named here but not mentioned');
      flushSync();
    });
    Then('the message shows the mention "@Alex" and the code "go test ./..."', () => {
      expect(texts('.msg .text .at')).toEqual(['@Alex']);
      expect(texts('.msg .text code')).toEqual(['go test ./...']);
    });
    And('the rest of its text is shown as written', () => {
      expect(texts('.msg .text')).toEqual([
        '@Alex run go test ./... first',
        '@Alex is named here but not mentioned',
      ]);
    });
  });

  const numbered = Array.from({ length: 120 }, (_, i) => `message ${i + 1}`);
  const fetches = () => home.asked.filter((req) => req.t === 'fetch');
  let shown: HTMLElement[];

  /** The group holds 120 messages, and a browser that held nothing has read it. */
  async function newest50() {
    first.drop();
    for (const text of numbered) home.say(website, owner, text);
    await closePage();
    first = browser(home);
    homes.push(first.store);
    await read(first);
    group = first.store.groups[0];
    openPage(`#/groups/${website}`);
    expect(texts('.msg .text')).toEqual(numbered.slice(-50));
    expect(fetches()).toEqual([]);
    shown = all('.msg');
  }

  Scenario(
    'Scrolling back fetches the pages before, one at a time',
    ({ Given, When, Then, And }) => {
      Given('the group holds 120 messages and the browser has read the newest 50', newest50);
      When('the top of the main line comes into view', () => scrollToTop());
      Then('the browser fetches pages until the home has no more', async () => {
        await vi.waitFor(() => expect(group.paging().more).toBe(false));
        flushSync();
        const lowest = (text: string) => group.main.find((msg) => msg.text === text)!.pos;
        // The first page is the one before the oldest message held, not the newest again.
        expect(fetches()).toEqual([
          { t: 'fetch', g: website, thread: null, before: lowest('message 71'), limit: 50 },
          { t: 'fetch', g: website, thread: null, before: lowest('message 21'), limit: 50 },
        ]);
      });
      And('the main line shows all 120 messages in the order they were written', () => {
        expect(texts('.msg .text')).toEqual(numbered);
      });
      And('the 50 messages shown before are the same parts of the page as before', () => {
        const now = all('.msg').slice(-50);
        expect(now.filter((part, i) => part !== shown[i])).toEqual([]);
      });
      And('it says "This is where the group starts."', () => {
        expect(texts('.edge')).toEqual(['This is where the group starts.']);
      });
    },
  );

  Scenario(
    'No page is asked for while the group is being read again',
    ({ Given, When, Then, And }) => {
      const after = Array.from({ length: 1500 }, (_, i) => `message ${i + 121}`);
      let asked: number;
      Given('the group holds 120 messages and the browser has read the newest 50', newest50);
      And(
        'the reader has scrolled back to the start, with the top of the main line still in view',
        async () => {
          scrollToTop();
          await vi.waitFor(() => expect(group.paging().more).toBe(false));
          flushSync();
          expect(texts('.msg .text')).toEqual(numbered);
        },
      );
      When('the home stops, 1500 messages are written, and the home returns', async () => {
        home.stop();
        first.drop();
        for (const text of after) home.say(website, owner, text);
        asked = fetches().length;
        await read(first);
        // One page is enough to see where the paging starts: the top leaves the view.
        await vi.waitFor(() => expect(fetches().length).toBeGreaterThan(asked));
        scrollToTop(false);
        await vi.waitFor(() => expect(group.paging().busy).toBe(false));
        flushSync();
      });
      Then(
        'the first page the browser asks for is the one before the oldest message that the new read brought',
        () => {
          const oldest = group.main.find((msg) => msg.text === 'message 1571')!.pos;
          expect(first.seen.filter((frame) => frame === 'reset')).toHaveLength(2);
          expect(fetches()[asked]).toEqual({
            t: 'fetch',
            g: website,
            thread: null,
            before: oldest,
            limit: 50,
          });
        },
      );
      And('the main line shows the newest messages without a gap', () => {
        const shownNow = texts('.msg .text');
        expect(shownNow.length).toBeGreaterThanOrEqual(100);
        expect(shownNow).toEqual([...numbered, ...after].slice(-shownNow.length));
        expect(texts('.edge')).toEqual(['Earlier messages']);
      });
    },
  );

  Scenario(
    'Without the home the top of the main line does not claim to be the start',
    ({ Given, When, Then, And }) => {
      Given('the group holds 120 messages and the browser has read the newest 50', newest50);
      When('the home stops and the top of the main line comes into view', () => {
        home.stop();
        first.drop();
        flushSync();
        scrollToTop();
      });
      Then('no page is fetched', async () => {
        await new Promise((later) => setTimeout(later, 5));
        expect(fetches()).toEqual([]);
      });
      And('the top of the main line says "Earlier messages cannot be loaded right now."', () => {
        expect(texts('.edge')).toEqual(['Earlier messages cannot be loaded right now.']);
      });
    },
  );

  Scenario(
    'What is typed while a message is on its way stays in the composer',
    ({ When, Then, And }) => {
      When(
        'the person sends "first" and types " and more" before the home has answered',
        async () => {
          type('first');
          send();
          expect(posts()).toEqual([]);
          type('first and more');
          await settled(1);
        },
      );
      Then('the home is sent the post "first"', () => {
        expect(posts().map((post) => post.text)).toEqual(['first']);
        expect(texts('.msg .text')).toEqual(['first']);
      });
      And('the composer holds " and more"', () => {
        expect(field().value).toBe(' and more');
        expect(field().selectionStart).toBe(9);
      });
    },
  );

  Scenario(
    'The main line is marked as focused by the keyboard only once a key was used',
    ({ When, Then }) => {
      const marked = () => one('.chat').classList.contains('keys');
      Then(
        'the chat is not marked as used with the keyboard, although its main line has focus',
        () => {
          expect(document.activeElement).toBe(one('.scroller'));
          expect(marked()).toBe(false);
        },
      );
      When('the person presses a key', () => {
        one('.scroller').dispatchEvent(new KeyboardEvent('keydown', { key: 'Tab', bubbles: true }));
        flushSync();
      });
      Then('the chat is marked as used with the keyboard', () => {
        expect(marked()).toBe(true);
      });
      When('the person uses the pointer', () => {
        one('.scroller').dispatchEvent(new Event('pointerdown', { bubbles: true }));
        flushSync();
      });
      Then('the chat is not marked as used with the keyboard any more', () => {
        expect(marked()).toBe(false);
      });
    },
  );

  Scenario('Enter sends a message, but not while an input method composes', ({ When, Then }) => {
    When('the person types "你好" and presses Enter while the input method composes', async () => {
      type('你好');
      press('Enter', { isComposing: true });
      press('Enter', { shiftKey: true });
      await new Promise((later) => setTimeout(later, 5));
    });
    Then('nothing is sent', () => {
      expect(posts()).toEqual([]);
      expect(field().value).toBe('你好');
    });
    When('the person presses Enter again', async () => {
      press('Enter');
      await settled(1);
    });
    Then('the home is sent the post "你好"', () => {
      expect(posts().map((post) => post.text)).toEqual(['你好']);
      expect(texts('.msg .text')).toEqual(['你好']);
    });
  });

  Scenario(
    'A command that the home refuses is told to the writer',
    ({ Given, When, Then, And }) => {
      Given('the home refuses posts', () => {
        home.refuse = 'bad_request';
      });
      When('the person writes "hello" and sends it', async () => {
        type('hello');
        send();
        await settled(1);
      });
      Then('the composer says "Not sent: the home refused it as too long or malformed."', () => {
        expect(texts('.composer [role=alert]')).toEqual([
          'Not sent: the home refused it as too long or malformed.',
        ]);
      });
      And('the text stays in the composer', () => {
        expect(field().value).toBe('hello');
        expect(texts('.msg .text')).toEqual([]);
      });
    },
  );
});
