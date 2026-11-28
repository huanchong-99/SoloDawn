import { describeFeature, loadFeature } from '@amiceli/vitest-cucumber';
import { flushSync } from 'svelte';
import { expect, vi } from 'vitest';
import { homes, type Group } from '../src/lib/groups.svelte';
import { goldens } from '../src/lib/mock';
import { nodes } from '../src/lib/nodes.svelte';
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
  read as readText,
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

  AfterEachScenario(async () => {
    await closePage();
    nodes.infos.clear();
  });

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
          const [name, member] = all<HTMLInputElement>('dialog input.input');
          fill(name, ' Handbook ');
          fill(member, 'Alex');
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
          member_name: 'Alex',
        });
        expect(Object.keys(payload).toSorted()).toEqual(
          ['t', 'n', 'ts', 'g', 'op', 'name', 'member_name'].toSorted(),
        );
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

  Scenario('The group list offers to join a group by code', ({ When, Then }) => {
    When('the reader opens the group list', () => openPage());
    Then(
      '"Join a group" beside "New group" opens the join page, which asks for an invitation code and a name',
      () => {
        expect(texts('.head :is(a, button)')).toEqual(['New group', 'Join a group']);
        const link = one<HTMLAnchorElement>('.head a');
        expect(link.getAttribute('href')).toBe('#/join');
        go(link.getAttribute('href')!);
        expect(text('main h1')).toBe('Join a group');
        expect(texts('main form label span')).toEqual([
          'Invitation code',
          'Your name in this group',
        ]);
      },
    );
  });

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

  const notice = (g: string, name: string) => (to: string) => [
    {
      t: 'home.add',
      n: expect.any(String),
      ts: expect.any(Number),
      to: `${to}-device`,
      g,
      home: first.store.device,
      name,
      addrs: [`ws://${location.host}/rv`],
    },
  ];

  Scenario(
    'A group created with nobody else is ready for its people',
    ({ Given, When, Then, And }) => {
      Given('the person owns a second computer, "desk", which is connected', async () => {
        desk = await ownNode('desk');
      });
      When('the person creates "Studio" in the dialog with nobody ticked', async () => {
        openPage();
        one('.head button').click();
        flushSync();
        const [name, member] = all<HTMLInputElement>('dialog input.input');
        fill(name, ' Studio ');
        fill(member, ' Alex ');
        one('dialog form').dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }));
        await vi.waitFor(() => expect(location.hash).toMatch(/^#\/groups\/.{22}$/));
        studio = location.hash.split('/')[2];
      });
      Then('the home has made "Studio" from its name and the person\'s name alone', () => {
        const { n, ts, ...command } = signed(home.asked.findLast((req) => req.t === 'cmd')!);
        expect(command).toEqual({
          t: 'group',
          g: studio,
          op: 'create',
          name: 'Studio',
          member_name: 'Alex',
        });
        expect([n, typeof ts]).toEqual([studio, 'number']);
        expect(home.groups.get(studio)!.name).toBe('Studio');
      });
      And(
        'its chat opens ready for the first message and for inviting people, without AIs and without a secretary warning',
        async () => {
          await vi.waitFor(() => expect(first.store.groups.at(-1)!.info?.name).toBe('Studio'));
          go(location.hash);
          expect(text('.chat-head h2')).toBe('Studio');
          expect(text('.chat-head p')).toBe('Members 1 · workstation');
          expect(field().disabled).toBe(false);
          expect(field().placeholder).toBe('Message the people of this group');
          expect(texts('.members button')).toEqual(['Add AI', 'Invite']);
          expect(texts('.members > .hint')).toEqual(['No AI in this group yet.']);
          expect(texts('.people .person-name')).toEqual(['Alex · You · Owner']);
          expect(all('.member, .nosec')).toEqual([]);
        },
      );
      And(
        '"desk" has been told nothing, since "Studio" is on a computer of the person',
        async () => {
          await new Promise((later) => setTimeout(later, 5));
          expect(told(desk)).toEqual([]);
        },
      );
    },
  );

  Scenario(
    "A computer that connects later is told the home of a group on another person's computer",
    ({ Given, When, Then, And }) => {
      let late: OwnedNode[];
      Given('the browser also reads "Docs", homed on a computer of "Mina"', async () => {
        const far = new TestHome('mina-pc');
        far.add(docs, 'Docs');
        const reader = browser(far);
        homes.push(reader.store);
        await reader.connect();
        // Mina's computer says it is not the person's own.
        reader.store.owner = false;
        openPage();
        await vi.waitFor(() =>
          expect(reader.store.groups.map((each) => each.name)).toEqual(['Docs']),
        );
      });
      When('the person\'s computer "desk" connects, then "laptop"', async () => {
        late = [await ownNode('desk'), await ownNode('laptop')];
      });
      Then(
        '"desk" and "laptop" have each been told once which computer is the home of "Docs"',
        () =>
          vi.waitFor(() =>
            expect(late.map(told)).toEqual(['desk', 'laptop'].map(notice(docs, 'Docs'))),
          ),
      );
      And('neither has been told the home of "Website", which is on a computer of the person', () =>
        expect(late.flatMap(told).map(({ g }) => g)).toEqual([docs, docs]),
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
      And('its chat says neither its home nor how many members it has', () => {
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
    const [name, member] = all<HTMLInputElement>('dialog input.input');
    fill(name, 'Notes');
    fill(member, 'Alex');
    const select = one<HTMLSelectElement>('dialog select');
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

  const coded = JSON.parse(goldens['cmd-code']) as { code: string; expires: number };
  /** Opens the dialog, names the group "Notes" and its maker "Alex", ticks the code and creates
   *  it, the code chosen for `hours` when given. */
  function createWithCode(hours?: string) {
    openPage();
    one('.head button').click();
    flushSync();
    const [name, member] = all<HTMLInputElement>('dialog input.input');
    fill(name, 'Notes');
    fill(member, 'Alex');
    expect(all('dialog .seg')).toEqual([]);
    one<HTMLInputElement>('dialog .code-pick input[type=checkbox]').click();
    flushSync();
    expect(texts('dialog .code-pick label')).toEqual([
      'Also make an invitation code',
      '1 hour',
      '24 hours',
      '7 days',
    ]);
    if (hours)
      all<HTMLInputElement>('dialog .seg input')
        .find((input) => input.closest('label')!.textContent!.trim() === hours)!
        .click();
    flushSync();
    one('dialog form').dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }));
  }
  const made = () =>
    home.asked
      .filter((req) => req.t === 'cmd')
      .map((req) => {
        const { n: _n, ts: _ts, ...fields } = signed(req);
        return fields;
      });
  const footer = () => texts('dialog footer button');
  /** Where the page scrolls an element into view, from now on. */
  const scrolled = () =>
    vi.spyOn(HTMLElement.prototype, 'scrollIntoView').mockImplementation(() => {});

  Scenario(
    'A group made with an invitation code shows the code before the dialog closes',
    ({ When, Then, And }) => {
      When(
        'the person ticks "Also make an invitation code", chooses 7 days and creates the group "Notes"',
        async () => {
          home.reply = (payload) =>
            payload.t === 'code' ? { ok: true, code: coded.code, expires: coded.expires } : {};
          const scroll = scrolled();
          createWithCode('7 days');
          await vi.waitFor(() => expect(made()).toHaveLength(2));
          // Below a long dialog's sticky footer the code would not be seen: it is brought up.
          await vi.waitFor(() => expect(scroll.mock.contexts).toEqual([one('dialog .made')]));
          expect(one('dialog .made .code-card')).not.toBeNull();
          expect(scroll.mock.calls).toEqual([[{ block: 'nearest', behavior: 'instant' }]]);
        },
      );
      Then('the home has made "Notes" and then a code for 168 hours', () => {
        const [create, code] = made();
        expect(create).toMatchObject({ t: 'group', op: 'create', name: 'Notes' });
        expect(code).toEqual({ t: 'code', g: create.g, op: 'new', hours: 168 });
      });
      And(
        'the dialog shows the code with "Copy code", and its one button reads "Open the group"',
        async () => {
          await vi.waitFor(() => expect(text('dialog .code-card output')).toBe(coded.code));
          expect(texts('dialog .code-card button')).toEqual(['Copy code', 'Copy invitation']);
          expect(footer()).toEqual(['Open the group']);
          expect(one<HTMLDialogElement>('dialog').open).toBe(true);
          expect(all('dialog .warn')).toEqual([]);
          expect(one<HTMLInputElement>('dialog .code-pick input').disabled).toBe(true);
        },
      );
      When('the person presses "Open the group"', () => {
        one<HTMLButtonElement>('dialog footer button').click();
        flushSync();
      });
      Then('the chat of "Notes" opens', async () => {
        await vi.waitFor(() => expect(location.hash).toBe(`#/groups/${made()[0].g}`));
        expect(one<HTMLDialogElement>('dialog').open).toBe(false);
      });
    },
  );

  Scenario(
    'A code the home refuses after making the group is said in the dialog',
    ({ When, Then, And }) => {
      When(
        'the person ticks "Also make an invitation code", creates the group "Notes", and the home refuses the code',
        async () => {
          home.reply = () => ({ err: { code: 'forbidden', msg: 'refused by the test' } });
          const scroll = scrolled();
          createWithCode();
          await vi.waitFor(() => expect(scroll.mock.contexts).toEqual([one('dialog .made')]));
          expect(one('dialog .made .fail')).not.toBeNull();
          await vi.waitFor(() => expect(made()).toHaveLength(2));
          expect(made()[1]).toMatchObject({ t: 'code', op: 'new', hours: 24 });
        },
      );
      Then(
        'the dialog says why the code was not made, and not that someone could not be added',
        async () => {
          await vi.waitFor(() =>
            expect(texts('dialog [role=alert]')).toEqual([
              'Not sent: this device may not do that.',
            ]),
          );
          expect(texts('dialog [role=status]')).toEqual([]);
          expect(all('dialog .code-card')).toEqual([]);
        },
      );
      And('its one button reads "Open the group"', () =>
        expect(footer()).toEqual(['Open the group']),
      );
    },
  );

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
        // With no computer there is nothing to choose a home from.
        expect(all('dialog select')).toEqual([]);
      });
      When('the computer connects and the person creates the group "Notes"', async () => {
        await read();
        const [name, member] = all<HTMLInputElement>('dialog input.input');
        fill(name, 'Notes');
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

  Scenario('The create-group dialog follows by default', ({ Given, When, Then }) => {
    let rig: OwnedNode;
    const aiSection = (cli: string) =>
      all('dialog div.ai').find((row) => readText(row.querySelector('b')!) === cli)!;

    Given('the owner\'s computer "desk" has Claude Code and Codex with defaults', async () => {
      const base = p.decode(p.HostInfo, goldens['host-info-answer']);
      rig = ownedNode('desk', 'desk-device', {
        homes: [],
        agents: [
          { ...base.agents[0], groups: [] },
          {
            ...base.agents[1],
            account: 'api_key',
            login_ok: true,
            models: [
              { model: 'gpt-5.4', name: 'GPT-5.4', efforts: ['low', 'high'], tier: 'strong' },
            ],
            defaults: {
              model: 'gpt-5.4',
              effort: 'high',
              tier: 'strong',
              skills: 'Review',
            },
            groups: [],
          },
        ],
      });
      rig.home.reply = (payload) => (payload.t === 'host.join' ? { ok: true } : {});
      homes.push(rig.store);
      await rig.connect();
    });

    When('the owner ticks Claude Code and Codex in the create-group dialog', async () => {
      openPage();
      one('.head button').click();
      flushSync();
      await vi.waitFor(() => expect(all('dialog div.ai')).toHaveLength(2));
      for (const cli of ['Claude Code', 'Codex']) {
        aiSection(cli).querySelector<HTMLInputElement>('input[type=checkbox]')!.click();
        flushSync();
      }
      await vi.waitFor(() =>
        expect(texts('dialog .ai-fields .path-row code')).toEqual([
          'No folder chosen',
          'No folder chosen',
        ]),
      );
      for (const cli of ['Claude Code', 'Codex']) {
        const row = aiSection(cli);
        row.querySelector<HTMLButtonElement>('.path-row .change')!.click();
        flushSync();
        await vi.waitFor(() =>
          expect(
            row.querySelector<HTMLButtonElement>('dialog[open] footer .btn-primary')?.disabled,
          ).toBe(false),
        );
        row.querySelector<HTMLButtonElement>('dialog[open] footer .btn-primary')!.click();
        flushSync();
      }
      expect(texts('dialog .ai-fields .path-row code')).toEqual([
        '/home/dev/website',
        '/home/dev/website',
      ]);
    });

    Then('each has the switch on', () => {
      const switches = all<HTMLButtonElement>('dialog .ai-fields [role="switch"]');
      expect(switches).toHaveLength(2);
      expect(switches.map((sw) => sw.getAttribute('aria-checked'))).toEqual(['true', 'true']);
      expect(all('dialog .ai-fields select, dialog .ai-fields .tiers')).toEqual([]);
    });

    When("the owner turns Codex's switch off and creates the group", async () => {
      const codexRow = aiSection('Codex');
      codexRow.querySelector<HTMLButtonElement>('[role="switch"]')!.click();
      flushSync();
      expect(codexRow.querySelector('[role="switch"]')!.getAttribute('aria-checked')).toBe('false');
      const [model, effort] = codexRow.querySelectorAll<HTMLSelectElement>('select');
      expect(model.value).toBe('gpt-5.4');
      expect(effort.value).toBe('high');
      expect(codexRow.querySelector<HTMLInputElement>('.tiers input:checked')!.value).toBe(
        'strong',
      );
      const [name, member] = all<HTMLInputElement>('dialog .two input');
      fill(name, 'Studio');
      fill(member, 'Alex');
      create();
      await vi.waitFor(() => expect(location.hash).toMatch(/^#\/groups\/.{22}$/));
    });

    Then(
      "Claude Code's host.join carries follow, and Codex's carries its four fields and no follow",
      () => {
        const joins = rig.home.asked
          .filter((req) => req.t === 'cmd')
          .map(signed)
          .filter((cmd) => cmd.t === 'host.join') as Record<string, unknown>[];
        expect(joins).toHaveLength(2);
        const claudeJoin = joins.find((j) => j.agent === 'claude')!;
        const codexJoin = joins.find((j) => j.agent === 'codex')!;
        expect(claudeJoin.follow).toBe(true);
        expect(
          'model' in claudeJoin ||
            'effort' in claudeJoin ||
            'tier' in claudeJoin ||
            'skills' in claudeJoin,
        ).toBe(false);
        expect(codexJoin).toMatchObject({
          agent: 'codex',
          model: 'gpt-5.4',
          effort: 'high',
          tier: 'strong',
          skills: 'Review',
        });
        expect('follow' in codexJoin).toBe(false);
      },
    );
  });

  Scenario(
    'Choosing a folder for one AI fills the others on that computer',
    ({ Given, When, Then }) => {
      let deskRig: OwnedNode;
      let laptopRig: OwnedNode;
      const aiRows = () => all('dialog > form div.ai');
      const submit = () => one<HTMLButtonElement>('dialog > form > footer .btn-primary');
      const why = () => {
        const id = submit().getAttribute('aria-describedby');
        return id ? document.getElementById(id)?.textContent?.trim() : undefined;
      };
      const chooseFolder = async (row: Element) => {
        row.querySelector<HTMLButtonElement>('.path-row .change')!.click();
        flushSync();
        await vi.waitFor(() =>
          expect(
            row.querySelector<HTMLButtonElement>('dialog[open] footer .btn-primary')?.disabled,
          ).toBe(false),
        );
        row.querySelector<HTMLButtonElement>('dialog[open] footer .btn-primary')!.click();
        flushSync();
      };

      Given(
        'the owner\'s computer "desk" has Claude Code and Codex, and "laptop" has Claude Code',
        async () => {
          const base = p.decode(p.HostInfo, goldens['host-info-answer']);
          deskRig = ownedNode('desk', 'desk-device', {
            homes: [],
            agents: [
              {
                ...base.agents[0],
                agent: 'claude',
                account: 'personal',
                login_ok: true,
                groups: [],
              },
              { ...base.agents[1], agent: 'codex', account: 'api_key', login_ok: true, groups: [] },
            ],
          });
          laptopRig = ownedNode('laptop', 'laptop-device', {
            homes: [],
            agents: [
              {
                ...base.agents[0],
                agent: 'claude',
                account: 'personal',
                login_ok: true,
                groups: [],
              },
            ],
          });
          homes.push(deskRig.store, laptopRig.store);
          await deskRig.connect();
          await laptopRig.connect();
        },
      );

      When('the owner ticks all three in the create-group dialog', async () => {
        openPage();
        one('.head button').click();
        flushSync();
        await vi.waitFor(() => expect(aiRows()).toHaveLength(3));
        for (const row of aiRows()) {
          row.querySelector<HTMLInputElement>('input[type=checkbox]')!.click();
        }
        flushSync();
      });

      Then('"Create group" is disabled and explains that Claude Code needs a folder', () => {
        expect(submit().disabled).toBe(true);
        expect(why()).toBe('Choose a folder for Claude Code first');
      });

      When('the owner chooses a folder for Claude Code on "desk"', async () => {
        await chooseFolder(aiRows()[0]);
      });

      Then(
        'Codex on "desk" uses that folder too, "laptop" still has no folder, and the reason names the AI on "laptop"',
        () => {
          expect(texts('dialog .ai-fields .path-row code')).toEqual([
            '/home/dev/website',
            '/home/dev/website',
            'No folder chosen',
          ]);
          expect(submit().disabled).toBe(true);
          expect(why()).toBe('Choose a folder for Claude Code 2 first');
        },
      );

      When('the owner chooses a folder for the AI on "laptop"', async () => {
        await chooseFolder(aiRows()[2]);
      });

      Then('"Create group" is enabled without a hint', () => {
        expect(submit().disabled).toBe(false);
        expect(submit().hasAttribute('aria-describedby')).toBe(false);
        expect(why()).toBeUndefined();
      });
    },
  );
});
