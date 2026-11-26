import { describeFeature, loadFeature } from '@amiceli/vitest-cucumber';
import { flushSync } from 'svelte';
import { expect, vi } from 'vitest';
import { homes, type Group } from '../src/lib/groups.svelte';
import * as p from '../src/lib/proto';
import { browser, example, ownedNode, owner, TestHome, user, type OwnedNode } from './home';
import CreateGroup from '../src/components/groups/CreateGroup.svelte';
import {
  all,
  choose,
  closePage,
  field,
  fill,
  go,
  one,
  openPage,
  otherBrowser,
  show,
  text,
  texts,
} from './page';

const feature = await loadFeature('../features/web_groups.feature');

const website = 'rZNvy-1jH6Z-BcPqA5U5BQ';
const docs = 'ZG9jcy1ncm91cC0wMDAwMQ';
const mina = 'bWluYS1tZW1iZXItMDAwMQ';
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
      And('a confirmation in "Website" waits for the reader, the owner of "Codex"', () => {
        home.write(website, example('row-member-ai', { id: 'codex', name: 'Codex' }));
        home.write(
          website,
          example('row-member-human', { id: 'mina', name: 'Mina', user: 'mina' }),
        );
        const ask = home.say(website, 'mina', '@Codex tidy the header', {
          ts: Date.now() - 9 * minute,
        });
        home.write(
          website,
          example('row-msg-confirm-pending', {
            by: 'system',
            re: ask.id,
            ts: Date.now() - 9 * minute,
            body: { what: 'direct', ai: 'codex', to: [user] },
          }),
        );
      });
      When('the browser has read both groups', async () => {
        first.drop();
        await read();
        openPage();
      });
      Then(
        'the list reads "Docs" with "Mina: Section 3 is in.", then "Website" with "System: Let Codex act on the request from Mina."',
        () => {
          expect(texts('.glist a b')).toEqual(['Docs', 'Website']);
          expect(texts('.glist .gprev')).toEqual([
            'Mina: Section 3 is in.',
            'System: Let Codex act on the request from Mina.',
          ]);
        },
      );
      And('"Website" shows that 2 cards wait for the reader, and "Docs" shows none', () => {
        const [docsRow, websiteRow] = all('.glist a');
        expect(docsRow.querySelector('.badge')).toBeNull();
        // The count is part of the link's name, with what it counts in words.
        expect(texts('.glist a .badge')).toEqual(['Waiting on you 2']);
        expect(websiteRow.querySelector('.badge .sr')!.textContent).toBe('Waiting on you');
        expect(texts('.needbar a')).toEqual([
          'Confirm start · Show connection paths',
          'Confirm · Let Codex act on the request from Mina.',
        ]);
        expect(texts('.digest .todo b')).toEqual(texts('.needbar a'));
      });
    },
  );

  Scenario(
    'A group created in the dialog shows in another browser without a reload',
    ({ Given, When, Then, And }) => {
      let created: string;
      Given('a second browser of the same person is connected', async () => {
        second = await otherBrowser(home);
        await read(second);
      });
      When(
        'the person creates the group "Handbook" in the dialog of the first browser',
        async () => {
          openPage();
          one('.head button').click();
          flushSync();
          expect(one<HTMLDialogElement>('dialog').open).toBe(true);
          const [name, repo, , member] = all<HTMLInputElement>('dialog input.input');
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

  Scenario('The list shows a task and a notice as the chat words them', ({ Given, When, Then }) => {
    Given(
      'the last message of "Website" is the task T-12 "Show connection path", and the home also holds "Docs", whose last message says that "Mina" joined',
      () => {
        home.write(website, example('row-task'));
        home.write(website, example('row-msg-task', { ts: Date.now() }));
        home.add(docs, 'Docs');
        home.write(docs, example('row-member-human', { id: mina, user: mina, name: 'Mina' }));
        home.write(
          docs,
          example('row-msg-system', {
            thread: null,
            ts: Date.now(),
            body: { code: 'member_joined', member: mina, by: owner },
          }),
        );
      },
    );
    When('the browser has read both groups', async () => {
      first.drop();
      await read();
      openPage();
    });
    Then(
      'the list reads "System: T-12 Show connection path" under "Website" and "System: Mina joined, invited by Alex." under "Docs"',
      () => {
        const preview = (name: string) => texts('.glist .gprev')[texts('.glist a b').indexOf(name)];
        expect(preview('Website')).toBe('System: T-12 Show connection path');
        expect(preview('Docs')).toBe('System: Mina joined, invited by Alex.');
      },
    );
  });

  Scenario(
    'The last message of a team group names the AI of a bulletin',
    ({ Given, When, Then }) => {
      Given(
        'the home also holds the team group "Studio" with a bulletin of "Claude Code", sent from the computer of "Alex"',
        () => {
          home.add(docs, 'Studio', 'row-group-team');
          home.write(docs, example('row-msg-bulletin', { by: owner }));
        },
      );
      When('the browser has read both groups', async () => {
        first.drop();
        await read();
        openPage();
      });
      Then('the list reads "Claude Code: The website is ready." under "Studio"', () => {
        const at = texts('.glist a b').indexOf('Studio');
        expect(texts('.glist .gprev')[at]).toBe('Claude Code: The website is ready.');
      });
    },
  );

  let desk: OwnedNode;
  let studio: string;
  const signed = (req: object) => JSON.parse((req as { env: p.Envelope }).env.p);

  /** Connects another computer of the person, which knows no home yet. */
  async function ownNode(name: string) {
    const node = ownedNode(name, `${name}-device`, { homes: [] });
    node.home.reply = () => ({ ok: true });
    homes.push(node.store);
    await node.connect();
    return node;
  }

  const told = (node: OwnedNode) => node.home.asked.filter((req) => req.t === 'cmd').map(signed);

  const notice = (g: string) => (name: string) => [
    {
      t: 'home.add',
      n: expect.any(String),
      ts: expect.any(Number),
      to: `${name}-device`,
      g,
      home: first.store.device,
      name: 'Studio',
      addrs: [`ws://${location.host}/rv`],
    },
  ];

  function openDialog() {
    one('.head button').click();
    flushSync();
  }

  /** The person makes the team group "Studio" in the dialog. */
  function createTeam() {
    openPage();
    openDialog();
    const asks = () => texts('dialog .field > span:first-child');
    const project = asks();
    one('dialog input[value=team]').click();
    flushSync();
    expect(asks()).toEqual(['Group name', 'Your name in the group', 'Home computer']);
    one('dialog input[value=project]').click();
    flushSync();
    expect(asks()).toEqual(project);
    one('dialog input[value=team]').click();
    flushSync();
    const [name, member] = all<HTMLInputElement>('dialog input.input');
    fill(name, ' Studio ');
    fill(member, ' Alex ');
    one('dialog form').dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }));
  }

  function startsWithProject() {
    openDialog();
    expect(all<HTMLInputElement>('dialog input[type=radio]').map((el) => el.checked)).toEqual([
      true,
      false,
    ]);
  }

  const creates =
    'the person chooses "Team group" in the dialog, which then asks only for the names and the home, and creates "Studio"';

  Scenario('A team group is created without a repository', ({ Given, When, Then, And }) => {
    Given('the person owns a second computer, "desk", which is connected', async () => {
      desk = await ownNode('desk');
    });
    When(creates, async () => {
      createTeam();
      await vi.waitFor(() => expect(location.hash).toMatch(/^#\/groups\/.{22}$/));
      studio = location.hash.split('/')[2];
    });
    Then('the home has made the team group "Studio" without a repository, checks or forge', () => {
      const { n, ts, ...command } = signed(home.asked.findLast((req) => req.t === 'cmd')!);
      expect(command).toEqual({
        t: 'group',
        g: studio,
        op: 'create',
        name: 'Studio',
        kind: 'team',
        base: '',
        forge: 'none',
        checks: [],
        member_name: 'Alex',
      });
      expect(n).toBe(studio);
      expect(home.groups.get(studio)!.name).toBe('Studio');
    });
    And('"desk" has been told which computer is the home of "Studio"', () =>
      vi.waitFor(() => expect(told(desk)).toEqual(notice(studio)('desk'))),
    );
    And(
      'its chat opens as a team group, ready for the first message and for inviting people, and without AIs',
      async () => {
        await vi.waitFor(() => expect(first.store.groups.at(-1)!.info?.kind).toBe('team'));
        go(location.hash);
        expect(text('.chat-head h2')).toBe('Studio');
        expect(text('.chat-head p')).toBe('Team group · Members 1 · workstation');
        expect(field().disabled).toBe(false);
        expect(texts('.members button')).toEqual(['Invite']);
        expect(texts('.people .person-name')).toEqual(['Alex · You · Owner']);
        expect(all('.preset, .nosec')).toEqual([]);
      },
    );
    And('the dialog starts again with a project group', startsWithProject);
  });

  Scenario(
    'A computer that connects later is told the home of a team group',
    ({ Given, When, Then, And }) => {
      let late: OwnedNode[];
      Given('the home also holds the team group "Studio", which the browser shows', async () => {
        home.add(docs, 'Studio', 'row-group-team');
        first.drop();
        await read();
        openPage();
        expect(texts('.glist a b').sort()).toEqual(['Studio', 'Website']);
      });
      When('the person\'s computer "desk" connects, then "laptop"', async () => {
        late = [await ownNode('desk'), await ownNode('laptop')];
      });
      Then(
        '"desk" and "laptop" have each been told once which computer is the home of "Studio"',
        () =>
          vi.waitFor(() => expect(late.map(told)).toEqual(['desk', 'laptop'].map(notice(docs)))),
      );
      And(
        'neither has been told the home of "Website", a project group of a computer of the person',
        () => expect(late.flatMap(told).map(({ g }) => g)).toEqual([docs, docs]),
      );
    },
  );

  Scenario(
    'A group known from a computer of the person shows while its home is away',
    ({ Given, When, Then, And }) => {
      let desk: OwnedNode;
      const far = new TestHome('far');
      const away = browser(far);
      const row = () => all('.gitem').find((each) => each.textContent!.includes('Docs'))!;
      Given(
        '"desk", a computer of the person, knows that "Docs" is homed on a computer out of reach',
        () => {
          const known = [{ g: docs, home: 'far', name: 'Docs' }];
          desk = ownedNode('desk', 'desk-device', { homes: known });
          // As net.svelte.ts does with each home that it hears of.
          desk.store.onhomes = (entries) =>
            entries.forEach(({ g, name }) => away.store.expect(g, name));
          homes.push(desk.store, away.store);
          openPage();
        },
      );
      When('"desk" connects', () => desk.connect());
      Then(
        'the list shows "Docs" with "cannot reach the group home" and no word of its messages',
        async () => {
          await vi.waitFor(() => expect(texts('.gdown')).toEqual(['cannot reach the group home']));
          expect(row().querySelector('.gprev')!.textContent).toBe('');
        },
      );
      And('its chat says neither its kind nor how many members it has', () => {
        go(`#/groups/${docs}`);
        expect([text('.chat-head h2'), text('.chat-head p')]).toEqual(['Docs', '']);
        go('#/groups');
      });
      When('that computer connects, with "Mina: Section 3 is in." in "Docs"', async () => {
        far.add(docs, 'Docs');
        far.write(
          docs,
          example('row-member-human', { id: mina, g: docs, name: 'Mina', user: 'm' }),
        );
        far.say(docs, mina, 'Section 3 is in.');
        await away.connect();
      });
      Then('the list shows "Docs" with "Mina: Section 3 is in."', () =>
        vi.waitFor(() => {
          flushSync();
          expect(row().querySelector('.gprev')!.textContent).toBe('Mina: Section 3 is in.');
          expect(all('.gdown')).toEqual([]);
        }),
      );
    },
  );

  let laptop: TestHome;
  let onLaptop: ReturnType<typeof browser>;
  const commands = (of: TestHome) => of.asked.filter((req) => req.t === 'cmd');

  async function secondComputer() {
    laptop = new TestHome('laptop');
    onLaptop = browser(laptop);
    homes.push(onLaptop.store);
    await onLaptop.connect();
  }

  /** The dialog, filled in, with the computer of that name chosen as the home. */
  function dialogFor(computer: string) {
    openPage();
    one('.head button').click();
    flushSync();
    const [name, repo, , member] = all<HTMLInputElement>('dialog input.input');
    fill(name, 'Notes');
    fill(repo, 'https://git.example/team/notes.git');
    fill(member, 'Alex');
    const select = all<HTMLSelectElement>('dialog select')[1];
    expect([...select.options].map((option) => option.textContent!.trim())).toEqual([
      'workstation',
      'laptop',
    ]);
    choose(select, computer);
  }

  function create() {
    one('dialog form').dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }));
    flushSync();
  }

  Scenario(
    'The dialog sends to the computer that was chosen, whatever happens to the others',
    ({ Given, When, Then }) => {
      Given('the person owns a second computer, "laptop", which is connected', secondComputer);
      When(
        'the person chooses "laptop" in the dialog, the first computer goes away, and they create the group "Notes"',
        async () => {
          dialogFor('laptop');
          home.stop();
          first.drop();
          flushSync();
          create();
          await vi.waitFor(() => expect(commands(laptop)).toHaveLength(1));
        },
      );
      Then(
        '"laptop" has made the group "Notes", and the first computer was sent no command',
        () => {
          expect([...laptop.groups.values()].map((made) => made.name)).toEqual(['Notes']);
          expect(commands(home)).toEqual([]);
        },
      );
    },
  );

  Scenario('A group is not created on a computer that went away', ({ Given, When, Then }) => {
    Given('the person owns a second computer, "laptop", which is connected', secondComputer);
    When(
      'the person chooses "laptop" in the dialog, "laptop" goes away, and they press create',
      async () => {
        dialogFor('laptop');
        laptop.stop();
        onLaptop.drop();
        flushSync();
        create();
        await new Promise((later) => setTimeout(later, 5));
        flushSync();
      },
    );
    Then(
      'no command is sent, and the dialog says "Not sent: the group home cannot be reached."',
      () => {
        expect([...commands(home), ...commands(laptop)]).toEqual([]);
        expect(texts('dialog [role=alert]')).toEqual([
          'Not sent: the group home cannot be reached.',
        ]);
        expect(one<HTMLButtonElement>('dialog .btn-primary').disabled).toBe(false);
      },
    );
    When('the person presses create again', async () => {
      create();
      await vi.waitFor(() => expect(commands(home)).toHaveLength(1));
    });
    Then('the computer that is left has made the group "Notes"', () => {
      expect([...home.groups.values()].map((made) => made.name)).toEqual(['Website', 'Notes']);
      expect(commands(laptop)).toEqual([]);
    });
  });

  Scenario(
    'A computer that connects while the dialog is open is the one chosen',
    ({ Given, When, Then }) => {
      Given('the dialog is open while no computer is connected', () => {
        first.drop();
        show(CreateGroup, { open: true });
        expect(one<HTMLDialogElement>('dialog').open).toBe(true);
        expect([...all<HTMLSelectElement>('dialog select')[1].options]).toEqual([]);
      });
      When('the computer connects and the person creates the group "Notes"', async () => {
        await read();
        const [name, repo, , member] = all<HTMLInputElement>('dialog input.input');
        fill(name, 'Notes');
        fill(repo, 'https://git.example/team/notes.git');
        fill(member, 'Alex');
        create();
        await new Promise((later) => setTimeout(later, 5));
        flushSync();
      });
      Then(
        'the computer has made the group "Notes", and the dialog says nothing of a home out of reach',
        () => {
          expect(texts('dialog [role=alert]')).toEqual([]);
          expect([...home.groups.values()].map((made) => made.name)).toEqual(['Website', 'Notes']);
          expect(commands(home)).toHaveLength(1);
        },
      );
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
        'the browser has asked for the page before the oldest message it held, and then for the one before that',
        () => {
          const pos = (text: string) => group.main.find((msg) => msg.text === text)!.pos;
          expect(home.asked.filter((req) => req.t === 'fetch')).toEqual([
            { t: 'fetch', g: website, thread: null, before: pos('message 1481'), limit: 50 },
            { t: 'fetch', g: website, thread: null, before: pos('message 1431'), limit: 50 },
          ]);
        },
      );
      And('its main line holds the newest 150 messages in the order they were written', () => {
        expect(mainLine()).toEqual(numbered(1381, 1530));
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
        second = await otherBrowser(home);
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
      When('the reader of that browser scrolls back', () => cold.older());
      Then(
        'the browser asks for the page before the oldest of those 50, not for the one before the card',
        () => {
          const oldest = cold.main.find((msg) => msg.text === 'message 31')!.pos;
          expect(cold.main[0].pos).toBeLessThan(oldest);
          expect(home.asked.filter((req) => req.t === 'fetch')).toEqual([
            { t: 'fetch', g: website, thread: null, before: oldest, limit: 50 },
          ]);
        },
      );
      And('its main line holds the decision and all 80 messages', () => {
        expect(mainLine(cold)).toEqual(['Choose a location.', ...numbered(1, 80)]);
        expect(cold.paging().more).toBe(false);
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
