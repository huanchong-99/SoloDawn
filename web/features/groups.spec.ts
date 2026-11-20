import { describeFeature, loadFeature } from '@amiceli/vitest-cucumber';
import { flushSync } from 'svelte';
import { expect, vi } from 'vitest';
import { homes, type Group } from '../src/lib/groups.svelte';
import * as p from '../src/lib/proto';
import { browser, example, owner, TestHome, user } from './home';
import { all, closePage, field, fill, go, one, openPage, texts } from './page';

const feature = await loadFeature('../features/web_groups.feature');

const website = 'rZNvy-1jH6Z-BcPqA5U5BQ';
const docs = 'ZG9jcy1ncm91cC0wMDAwMQ';
const minute = 60_000;

describeFeature(feature, ({ Background, Scenario, AfterEachScenario }) => {
  let home: TestHome;
  let first: ReturnType<typeof browser>;
  let second: ReturnType<typeof browser>;
  let group: Group;
  let last: number;

  const syncs = () => home.asked.filter((req) => req.t === 'sync');
  const mainLine = (of: Group = group) => of.main.map((msg) => msg.text);
  const write = (count: number, from = 1) => {
    for (let i = from; i < from + count; i++) home.say(website, owner, `message ${i}`);
  };
  const numbered = (from: number, to: number) =>
    Array.from({ length: to - from + 1 }, (_, i) => `message ${from + i}`);

  /** The browser connects, and the home has answered the read of each of its groups. */
  async function read(reader = first) {
    const answered = home.synced + home.groups.size;
    await reader.connect();
    await vi.waitFor(() => expect(home.synced).toBe(answered));
    await new Promise((applied) => setTimeout(applied));
    for (const each of reader.store.groups) expect(each.cursor).toBe(home.groups.get(each.g)!.seq);
    flushSync();
  }

  /** What the home writes reaches the connected browser at once. */
  function readToTheEnd() {
    last = group.cursor;
    expect(last).toBe(home.groups.get(website)!.seq);
  }

  async function away(messages: number) {
    first.drop();
    write(messages);
    await read();
  }

  AfterEachScenario(closePage);

  Background(({ Given }) => {
    Given(
      'a home that holds the group "Website", and a browser of its owner that reads it',
      async () => {
        home = new TestHome();
        home.add(website, 'Website');
        first = browser(home);
        homes.push(first.store);
        await read();
        group = first.store.groups[0];
        expect(group.name).toBe('Website');
      },
    );
  });

  Scenario(
    'The list shows each group with its last message and what waits for the reader',
    ({ Given, When, Then, And }) => {
      Given(
        'the home also holds the group "Docs", where "Mina" wrote "Section 3 is in." a minute ago',
        () => {
          home.add(docs, 'Docs');
          home.write(docs, example('row-member-human', { id: 'mina', name: 'Mina', user: 'mina' }));
          home.say(docs, 'mina', 'Section 3 is in.', { ts: Date.now() - minute });
        },
      );
      And('a brief in "Website" waits for the reader to confirm it', () => {
        home.write(
          website,
          example('row-member-ai', { id: 'secretary', name: 'Secretary', secretary: true }),
        );
        home.write(
          website,
          example('row-msg-brief-pending', { by: 'secretary', ts: Date.now() - 10 * minute }),
        );
      });
      When('the browser has read both groups', async () => {
        first.drop();
        await read();
        openPage();
      });
      Then(
        'the list reads "Docs" with "Mina: Section 3 is in.", then "Website" with "Secretary: Show connection paths"',
        () => {
          expect(texts('.glist a b')).toEqual(['Docs', 'Website']);
          expect(texts('.glist .gprev')).toEqual([
            'Mina: Section 3 is in.',
            'Secretary: Show connection paths',
          ]);
        },
      );
      And('"Website" shows that 1 card waits for the reader, and "Docs" shows none', () => {
        const [docsRow, websiteRow] = all('.glist a');
        expect(docsRow.querySelector('.badge')).toBeNull();
        expect(websiteRow.querySelector('.badge')!.textContent!.trim()).toBe('1');
        expect(websiteRow.querySelector('.badge')!.getAttribute('aria-label')).toBe(
          'Waiting on you 1',
        );
        expect(texts('.needbar a')).toEqual(['Confirm start · Show connection paths']);
      });
    },
  );

  Scenario(
    'A group created in the dialog shows in another browser without a reload',
    ({ Given, When, Then, And }) => {
      let created: string;
      Given('a second browser of the same person is connected', async () => {
        second = browser(home);
        await read(second);
      });
      When(
        'the person creates the group "Handbook" in the dialog of the first browser',
        async () => {
          openPage();
          one('.head button').click();
          flushSync();
          expect(one<HTMLDialogElement>('dialog').open).toBe(true);
          const [name, repo, , member] = all<HTMLInputElement>('dialog input');
          fill(name, ' Handbook ');
          fill(repo, 'https://git.example/team/handbook.git');
          fill(member, 'Alex');
          fill(one<HTMLTextAreaElement>('dialog textarea'), 'pnpm test\n\n pnpm check ');
          one('dialog form').dispatchEvent(
            new Event('submit', { bubbles: true, cancelable: true }),
          );
          await vi.waitFor(() => expect(location.hash).toMatch(/^#\/groups\/.{22}$/));
          created = location.hash.split('/')[2];
        },
      );
      Then("the home has made the group that the command's nonce names", () => {
        const command = home.asked.find((req) => req.t === 'cmd')!;
        const payload = JSON.parse((command.env as p.Envelope).p);
        expect(payload).toMatchObject({
          t: 'group',
          op: 'create',
          n: created,
          g: created,
          name: 'Handbook',
          kind: 'project',
          repo: 'https://git.example/team/handbook.git',
          base: 'main',
          forge: 'github',
          checks: ['pnpm test', 'pnpm check'],
          member_name: 'Alex',
        });
        expect((command.env as p.Envelope).k).toBe('this-browser');
        expect(home.groups.get(created)!.name).toBe('Handbook');
      });
      And('both browsers list "Handbook" without a reload', async () => {
        await vi.waitFor(() => {
          flushSync();
          expect(texts('.glist a b')).toEqual(['Handbook', 'Website']);
        });
        expect(second.store.groups.map((each) => each.name)).toEqual(['Website', 'Handbook']);
        expect(one<HTMLDialogElement>('dialog').open).toBe(false);
      });
      And('the first browser has opened its chat', () => {
        // The browser tells the page that the address changed; the test DOM does not.
        go(location.hash);
        expect(texts('.chat-head h2')).toEqual(['Handbook']);
      });
    },
  );

  Scenario(
    'A browser that was away receives exactly the messages it missed, in order',
    ({ Given, When, Then, And }) => {
      Given('the browser has read the group up to its last row', readToTheEnd);
      When('the connection is lost, 20 messages are written, and the browser connects again', () =>
        away(20),
      );
      Then('the browser asks for the rows after the last one it applied', () => {
        expect(syncs().at(-1)).toEqual({ t: 'sync', g: website, since: last, state_only: false });
      });
      And('it is not told to reset', () => {
        // One reset, at the first read, which started from nothing.
        expect(first.seen.filter((frame) => frame === 'reset')).toHaveLength(1);
        expect(first.seen.at(-1)).toBe('rows');
      });
      And(
        'its main line ends with exactly those 20 messages, in the order they were written',
        () => {
          expect(mainLine()).toEqual(numbered(1, 20));
          expect(group.cursor).toBe(last + 20);
        },
      );
    },
  );

  Scenario(
    'After 1500 missed rows the browser starts again from the newest 50',
    ({ Given, When, Then, And }) => {
      Given('the browser has read the group up to its last row', () => {
        write(30);
        readToTheEnd();
        expect(mainLine()).toEqual(numbered(1, 30));
      });
      When(
        'the connection is lost, 1500 messages are written, and the browser connects again',
        async () => {
          first.drop();
          write(1500, 31);
          await read();
        },
      );
      Then('the browser is told to reset and holds the newest 50 messages of the main line', () => {
        expect(first.seen.filter((frame) => frame === 'reset')).toHaveLength(2);
        expect(mainLine()).toEqual(numbered(1481, 1530));
      });
      When('the reader scrolls back twice', async () => {
        await group.older();
        await group.older();
      });
      Then(
        'the browser has asked for the newest page and then for the page before its lowest message',
        () => {
          const lowest = group.main.find((msg) => msg.text === 'message 1481')!.pos;
          expect(home.asked.filter((req) => req.t === 'fetch')).toEqual([
            { t: 'fetch', g: website, thread: null, before: 0, limit: 50 },
            { t: 'fetch', g: website, thread: null, before: lowest, limit: 50 },
          ]);
        },
      );
      And('its main line holds the newest 100 messages in the order they were written', () => {
        expect(mainLine()).toEqual(numbered(1431, 1530));
        expect(group.cursor).toBe(home.groups.get(website)!.seq);
      });
    },
  );

  Scenario(
    'A card that is still pending is there after a cold start, however old',
    ({ Given, When, Then, And }) => {
      let cold: Group;
      Given(
        'a decision in "Website" waits for the reader, and 80 messages were written after it',
        () => {
          home.write(website, example('row-msg-decision-pending', { thread: null, main: false }));
          write(80);
        },
      );
      When('a browser that holds nothing reads the group', async () => {
        second = browser(home);
        await read(second);
        cold = second.store.groups[0];
      });
      Then('the decision waits for the reader in that browser', () => {
        expect(cold.waiting.map((msg) => msg.kind)).toEqual(['decision']);
        expect((cold.waiting[0].body as { to: string[] }).to).toEqual([user]);
      });
      And('its main line holds the decision and the newest 50 messages', () => {
        expect(mainLine(cold)).toEqual(['Choose a location.', ...numbered(31, 80)]);
      });
    },
  );

  Scenario(
    'A reset that breaks off after its first batch starts again from nothing',
    ({ Given, When, Then }) => {
      Given('the browser has read the group up to its last row', readToTheEnd);
      When(
        '1500 messages are written while it is away, and its next read breaks off after the first batch',
        async () => {
          first.drop();
          write(1500);
          home.batch = 20;
          home.breakAfter = 1;
          await first.connect();
          await vi.waitFor(() => expect(first.seen.at(-1)).toBe('rows'));
          await vi.waitFor(() => expect(syncs()).toHaveLength(2));
          first.drop();
        },
      );
      Then('the browser holds rows of that batch and its place is 0', () => {
        expect(first.seen.slice(-2)).toEqual(['reset', 'rows']);
        expect(group.rows.size).toBe(20);
        expect(group.cursor).toBe(0);
      });
      When('the browser connects again', () => read());
      Then('it asks for the group from 0', () => {
        expect(syncs().at(-1)).toEqual({ t: 'sync', g: website, since: 0, state_only: false });
        expect(group.cursor).toBe(home.groups.get(website)!.seq);
      });
    },
  );

  Scenario(
    'With the home stopped the group is read-only, and reading goes on when the home returns',
    ({ Given, When, Then, And }) => {
      Given('the browser has read the group up to its last row', () => {
        write(3);
        readToTheEnd();
      });
      And('the reader has opened the chat of "Website"', () => {
        openPage(`#/groups/${website}`);
        expect(texts('.msg .text')).toEqual(numbered(1, 3));
        expect(field().disabled).toBe(false);
        expect(all('.gdown')).toEqual([]);
      });
      When('the home stops', () => {
        home.stop();
        first.drop();
        flushSync();
      });
      Then(
        'the chat says "Cannot reach the group home. What is already here can still be read."',
        () => {
          expect(texts('.composer [role=status]')).toEqual([
            'Cannot reach the group home. What is already here can still be read.',
          ]);
        },
      );
      And(
        'the composer takes no text, and the list marks the group as one whose home cannot be reached',
        () => {
          expect(field().disabled).toBe(true);
          expect(one<HTMLButtonElement>('.composer .send').disabled).toBe(true);
          expect(texts('.gdown')).toEqual(['workstation · cannot reach the group home']);
        },
      );
      And('the messages read before are still shown', () => {
        expect(texts('.msg .text')).toEqual(numbered(1, 3));
      });
      When('the home returns with a message written meanwhile', async () => {
        write(1, 4);
        await read();
      });
      Then(
        'the browser asks for the rows after the last one it applied, and shows that message',
        () => {
          expect(syncs().at(-1)).toEqual({ t: 'sync', g: website, since: last, state_only: false });
          expect(texts('.msg .text')).toEqual(numbered(1, 4));
        },
      );
      And('the composer takes text again', () => {
        expect(field().disabled).toBe(false);
        expect(all('.composer [role=status]')).toEqual([]);
        expect(all('.gdown')).toEqual([]);
      });
    },
  );

  Scenario(
    'A row deleted on purpose does not come back with an older page',
    ({ Given, When, Then }) => {
      Given('the browser has read the group up to its last row', () => {
        write(2);
        readToTheEnd();
      });
      When(
        'a message is deleted on purpose, and a page that still holds it arrives afterwards',
        () => {
          const copy = group.main[0];
          home.remove(website, 'msg', copy.id);
          expect(mainLine()).toEqual(['message 2']);
          // What `older` does with the rows of a page.
          group.apply([copy], false);
        },
      );
      Then('the browser keeps the marker and does not show the message', () => {
        expect(mainLine()).toEqual(['message 2']);
        expect([...group.rows.values()].filter((row) => 'gone' in row)).toHaveLength(1);
        expect(group.cursor).toBe(home.groups.get(website)!.seq);
      });
    },
  );
});
