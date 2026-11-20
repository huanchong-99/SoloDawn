import 'fake-indexeddb/auto';
import { readdirSync, readFileSync } from 'node:fs';
import { join } from 'node:path';
import { describeFeature, loadFeature } from '@amiceli/vitest-cucumber';
import { flushSync, tick } from 'svelte';
import { expect, vi } from 'vitest';
import { find, homes } from '../src/lib/groups.svelte';
import { cards, gather, mounts, points, type Mounts } from '../src/lib/mounts.svelte';
import { merge } from '../src/messages/texts';
import en from '../src/messages/en.json';
import zh from '../src/messages/zh-CN.json';
import * as areas from './fixtures/mounts';
import { browser, example, TestHome } from './home';
import { all, closePage, go, one, openPage, texts } from './page';

const feature = await loadFeature('../features/web_shell.feature');

// The client's mounting points hold, for each scenario, what its stand-in areas mount.
vi.mock('../src/lib/mounts.svelte', async (actual) => {
  const real = await actual<typeof import('../src/lib/mounts.svelte')>();
  const held = new Map<string, { component: unknown }>();
  return {
    ...real,
    mounts: Object.fromEntries(real.points.map((point) => [point, []])),
    cards: held,
    card: (kind: string) => held.get(kind)?.component,
  };
});

/** The areas mount these, as if their files were there. */
function mount(files: Record<string, Mounts>) {
  const found = gather(files);
  for (const point of points) (mounts[point] as unknown[]).splice(0, Infinity, ...found.at[point]);
  for (const [kind, held] of found.cards) (cards as Map<string, unknown>).set(kind, held);
}

const website = 'rZNvy-1jH6Z-BcPqA5U5BQ';
const app = () => one('.app');

describeFeature(feature, ({ Background, Scenario, ScenarioOutline, AfterEachScenario }) => {
  let home: TestHome;

  /** A browser of the person reads the home's groups. */
  async function reach(g: string, name: string) {
    home = new TestHome('workstation');
    home.add(g, name);
    home.write(g, example('row-member-ai', { g }));
    const reader = browser(home);
    homes.push(reader.store);
    await reader.connect();
    await vi.waitFor(() => expect(find(g)?.info).toBeDefined());
  }

  AfterEachScenario(async () => {
    await closePage();
    mount({});
    vi.unstubAllEnvs();
    history.replaceState(null, '', '/');
  });

  Background(({ Given }) => {
    Given('a browser set to English', () => {});
  });

  ScenarioOutline(
    'Each page of the design opens at its address',
    ({ When, Then, And }, variables) => {
      When('the person opens the client at "<address>"', () => openPage(variables.address));
      Then('the page "<title>" is open, drawn by "<drawn by>"', async () => {
        const drawn = {
          'a coming page': '.pending',
          'the group list': '.groups',
          'its own page': '.page:not(.pending)',
          'the states sheet': '.states',
        }[variables['drawn by'] as string]!;
        await vi.waitFor(() => {
          flushSync();
          expect(all(drawn)).toHaveLength(1);
        });
        expect(texts('h1')).toEqual([variables.title]);
        expect(document.title).toBe(`${variables.title} · SoloDawn`);
      });
      And('the navigation marks "<entry>"', () => {
        const entry = variables.entry === 'nothing' ? [] : [variables.entry];
        expect(texts('nav a[aria-current="page"]')).toEqual(entry);
      });
    },
  );

  Scenario(
    'A page that is not built yet says so and leads back to the chats',
    ({ When, Then, And }) => {
      When('the person opens the client at "#/terminal/g/m"', () => openPage('#/terminal/g/m'));
      Then('the page says "This page is not built yet"', () => {
        expect(texts('main .page-head p')).toEqual(['This page is not built yet']);
      });
      And('its link "Back to the chats" leads to "#/groups"', () => {
        const link = all<HTMLAnchorElement>('main a').find(
          (a) => a.textContent!.trim() === 'Back to the chats',
        );
        expect(link?.getAttribute('href')).toBe('#/groups');
      });
    },
  );

  Scenario('The address gives the page its parameters', ({ Given, When, Then }) => {
    Given('the home of the group "Website" is reached', () => reach(website, 'Website'));
    When('the person opens the client at "#/groups/rZNvy-1jH6Z-BcPqA5U5BQ"', () =>
      openPage(`#/groups/${website}`),
    );
    Then('the chat of "Website" is open', () => {
      expect(texts('.chat-head h2')).toEqual(['Website']);
    });
    When('the address becomes "#/groups"', () => go('#/groups'));
    Then('no chat is open', () => {
      expect(all('.chat')).toEqual([]);
      expect(all('.groups.open')).toEqual([]);
    });
  });

  Scenario(
    "The address of a group that has not been read keeps the chat's place, with a way back",
    ({ When, Then, And }) => {
      When('the person opens the client at "#/groups/not-read-yet"', () =>
        openPage('#/groups/not-read-yet'),
      );
      Then(
        'the page says "This group has not been read yet. It is read once the computer that is its home can be reached."',
        () => {
          expect(all('.groups.open')).toHaveLength(1);
          expect(texts('.missing p')).toEqual([
            'This group has not been read yet. It is read once the computer that is its home can be reached.',
          ]);
        },
      );
      And('its link "Back to the group list" leads to "#/groups"', () => {
        expect(one('.missing a').getAttribute('aria-label')).toBe('Back to the group list');
        expect(one('.missing a').getAttribute('href')).toBe('#/groups');
      });
      When('the home of the group "not-read-yet" is reached', async () => {
        await reach('not-read-yet', 'not-read-yet');
        flushSync();
      });
      Then('the chat of "not-read-yet" is open', () => {
        expect(texts('.chat-head h2')).toEqual(['not-read-yet']);
        expect(all('.missing')).toEqual([]);
      });
    },
  );

  ScenarioOutline(
    'A page has the screen to itself where its route asks for it',
    ({ When, Then }, variables) => {
      When('the person opens the client at "<address>"', () => openPage(variables.address));
      Then('the shell gives the page the whole screen "<where>"', () => {
        const full = { 'on a phone': 'phone', 'at every width': 'always', nowhere: undefined };
        expect(app().dataset.full).toBe(full[variables.where as keyof typeof full]);
      });
    },
  );

  Scenario(
    'In a development server every page is given the group of the wire examples',
    ({ Given, When, Then }) => {
      Given('a development server', () => {
        vi.stubEnv('MODE', 'development');
      });
      When('the person opens the client at "#/groups"', () => openPage('#/groups'));
      Then('the group list shows "Website" from the computer "workstation"', async () => {
        await vi.waitFor(() => {
          flushSync();
          expect(texts('.glist a b')).toEqual(['Website']);
        });
        expect(find(website)!.home.name).toBe('workstation');
      });
    },
  );

  ScenarioOutline(
    'Elsewhere no page is given a home until the connections to the nodes are wired',
    ({ Given, When, Then }, variables) => {
      Given('<where>', () => {
        const nomock = (variables.where as string).includes('nomock');
        vi.stubEnv('MODE', nomock ? 'development' : 'production');
        if (nomock) history.replaceState(null, '', '/?nomock');
      });
      When('the person opens the client at "#/groups"', () => openPage('#/groups'));
      Then('the group list is empty', async () => {
        await new Promise((later) => setTimeout(later, 50));
        flushSync();
        expect(homes).toEqual([]);
        expect(texts('.empty h3')).toEqual(['No groups yet']);
      });
    },
  );

  Scenario(
    'What areas mount in the top bar comes first, in order, at every page',
    ({ Given, When, Then }) => {
      Given(
        'an area that mounts "Pause all" at 20 and another that mounts the indicator at 10',
        () => mount(areas.bar),
      );
      When('the person opens the client at "#/settings"', () => openPage('#/settings'));
      Then(
        'the top bar reads the indicator "live", "Pause all", then the theme and language buttons',
        () => {
          const parts = [...one('.bar .global').children].map(
            (part) => part.getAttribute('aria-label') ?? part.textContent!.trim(),
          );
          expect(parts.slice(0, 4)).toEqual([
            'live',
            'Pause all',
            'Toggle theme',
            '中 Switch language / 切换语言',
          ]);
        },
      );
    },
  );

  Scenario(
    "An area's banner shows under the top bar and its layer after the page",
    ({ Given, When, Then }) => {
      Given('an area that mounts a notice as a banner and as a layer', () => mount(areas.notices));
      When('the person opens the client at "#/nodes"', () => openPage('#/nodes'));
      Then('a notice stands between the top bar and the page, and another after the page', () => {
        const order = [...app().children].map((part) => part.className || part.tagName);
        expect(order.slice(order.indexOf('bar'))).toEqual(['bar', 'ban', 'MAIN', 'probe notice']);
        expect(texts('.ban .notice')).toEqual(['notice · live']);
      });
    },
  );

  Scenario(
    'A card mounted for its kind replaces the one-line stub',
    ({ Given, When, Then, And }) => {
      Given("an area that mounts the card of approvals and of the home's notices", () =>
        mount(areas.cards),
      );
      And(
        'the home of the group "Website" holds a pending approval, a pending brief and a notice of the home',
        async () => {
          await reach(website, 'Website');
          home.write(
            website,
            example('row-msg-approval-pending', { thread: null, text: 'May I?' }),
          );
          home.write(website, example('row-msg-brief-pending', { thread: null }));
          home.write(website, example('row-msg-system', { thread: null }));
          await new Promise((applied) => setTimeout(applied, 5));
        },
      );
      When('the person opens the client at "#/groups/rZNvy-1jH6Z-BcPqA5U5BQ"', () =>
        openPage(`#/groups/${website}`),
      );
      Then('the approval is drawn by its card under the head and text of its message', () => {
        const approval = one('.card-of').closest('article.msg')!;
        expect(approval.querySelector('.msg-head b')!.textContent).toBe('Codex');
        expect(approval.querySelector('.msg-body .text')).not.toBeNull();
        expect(approval.querySelector('.msg-body > .card-of')!.textContent).toBe(
          'approval netluHiPQf-UoheEIBfbLw in Website',
        );
        expect(approval.querySelector('.stub')).toBeNull();
      });
      And('the brief is still the one-line stub', () => {
        expect(texts('.stub .kind')).toEqual(['Task brief']);
      });
      And('the notice of the home is drawn whole by its card, with no line of its own', () => {
        expect(texts('.msg.sys > .card-of')).toEqual(['system u8XmYeEGxtzY3G3RhkVMLw in Website']);
        expect(all('.msg.note')).toEqual([]);
      });
    },
  );

  Scenario(
    'A chat holds what areas mount in its head, above its main line and beside it',
    ({ Given, When, Then, And }) => {
      Given('an area that mounts a part in the head, above the main line and beside it', () =>
        mount(areas.chat),
      );
      And('the home of the group "Website" is reached', () => reach(website, 'Website'));
      When('the person opens the client at "#/groups/rZNvy-1jH6Z-BcPqA5U5BQ"', () =>
        openPage(`#/groups/${website}`),
      );
      Then(
        'the head, the place above the main line and the side panel each hold the part for "Website"',
        () => {
          expect(texts('.chat-head .part')).toEqual(['Website']);
          expect(texts('.chat > .top .part')).toEqual(['Website']);
          expect(texts('.chat > .side .part')).toEqual(['Website']);
        },
      );
      And('the head has a button "People and details" that opens the side panel', () => {
        const toggle = one('.chat-head .side-open');
        expect(toggle.getAttribute('aria-label')).toBe('People and details');
        expect(toggle.getAttribute('aria-controls')).toBe(one('.chat > .side').id);
        expect(toggle.getAttribute('aria-expanded')).toBe('false');
      });
    },
  );

  /** The parts of the chat that the panel covers below 1024 px. */
  const behind = () => all('.chat > :not(.side)');
  const press = async (selector: string) => {
    one<HTMLButtonElement>(selector).click();
    await tick();
    flushSync();
  };
  const escape = async (init: KeyboardEventInit, target: EventTarget = window) => {
    target.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true, ...init }));
    await tick();
    flushSync();
  };
  const panelOpen = () => one('.chat > .side').classList.contains('open');
  type Step = (text: string, step: () => void | Promise<void>) => void;
  /** The steps that open a chat whose area mounts a part at each point of the chat. */
  const mountedChat = (Given: Step, width: number) => {
    Given('an area that mounts a part in the head, above the main line and beside it', () =>
      mount(areas.chat),
    );
    Given('the home of the group "Website" is reached', () => reach(website, 'Website'));
    Given(
      `the person opens the client at "#/groups/rZNvy-1jH6Z-BcPqA5U5BQ" on a screen ${width} px wide`,
      () => openPage(`#/groups/${website}`, width),
    );
  };

  Scenario(
    'Below 1024 px the side panel opens over the chat, holds the focus, and closes',
    ({ Given, When, Then, And }) => {
      mountedChat((text, step) => (text.startsWith('an area') ? Given : And)(text, step), 820);
      When('the person presses "People and details"', () => press('.side-open'));
      Then('the side panel is open, with focus on its button "Close the panel"', () => {
        expect(panelOpen()).toBe(true);
        expect(one('.side-open').getAttribute('aria-expanded')).toBe('true');
        expect(document.activeElement?.getAttribute('aria-label')).toBe('Close the panel');
      });
      And('the head, the main line and the composer behind it cannot be reached', () => {
        expect(behind().map((part) => part.hasAttribute('inert'))).toEqual([
          true,
          true,
          true,
          true,
        ]);
        expect(one('.chat-head').contains(one('.side-open'))).toBe(true);
        expect(one('.scroller[inert]').getAttribute('aria-label')).toBe('Main line');
        expect(all('.dock[inert] .composer')).toHaveLength(1);
      });
      When('the person presses Escape', () => escape({}));
      Then('the side panel is closed and focus is on "People and details"', () => {
        expect(panelOpen()).toBe(false);
        expect(document.activeElement).toBe(one('.side-open'));
      });
      And('the chat can be reached again', () => {
        expect(behind().map((part) => part.hasAttribute('inert'))).toEqual([
          false,
          false,
          false,
          false,
        ]);
      });
      When('the person presses "People and details" and then "Close the panel"', async () => {
        await press('.side-open');
        expect(document.activeElement).toBe(one('.side-close'));
        await press('.side-close');
      });
      Then('the side panel is closed again, with focus back on "People and details"', () => {
        expect(panelOpen()).toBe(false);
        expect(document.activeElement).toBe(one('.side-open'));
      });
    },
  );

  Scenario(
    'An Escape that a dialog takes, or that a part already used, leaves the side panel open',
    ({ Given, When, Then, And }) => {
      mountedChat((text, step) => (text.startsWith('an area') ? Given : And)(text, step), 820);
      And('the person has opened the side panel', () => press('.side-open'));
      When('the person presses Escape in a dialog', async () => {
        const dialog = document.body.appendChild(document.createElement('dialog'));
        const field = dialog.appendChild(document.createElement('input'));
        await escape({}, field);
        dialog.remove();
      });
      Then('the side panel is open', () => expect(panelOpen()).toBe(true));
      When('the person presses Escape where a part has already used it', async () => {
        const event = new KeyboardEvent('keydown', { key: 'Escape', cancelable: true });
        event.preventDefault();
        dispatchEvent(event);
        await tick();
        flushSync();
      });
      Then('the side panel is still open', () => expect(panelOpen()).toBe(true));
    },
  );

  Scenario(
    'From 1024 px the side panel stands beside the chat and takes no focus',
    ({ Given, When, Then, And }) => {
      mountedChat((text, step) => (text.startsWith('an area') ? Given : And)(text, step), 1440);
      When('the person presses "People and details"', async () => {
        one<HTMLButtonElement>('.side-open').focus();
        await press('.side-open');
      });
      Then('focus is still on "People and details" and the chat can be reached', () => {
        expect(document.activeElement).toBe(one('.side-open'));
        expect(behind().some((part) => part.hasAttribute('inert'))).toBe(false);
      });
      When('the person presses Escape', () => escape({}));
      Then('the side panel is still marked open', () => expect(panelOpen()).toBe(true));
    },
  );

  Scenario('Two areas cannot mount the card of one kind', ({ When, Then }) => {
    let gathering: () => unknown;
    When('two areas each mount a card for approvals', () => {
      const card = areas.cards['../components/approval/mounts.ts'].cards!.approval!;
      gathering = () =>
        gather({
          '../components/approval/mounts.ts': { cards: { approval: card } },
          '../components/thread/mounts.ts': { cards: { approval: card } },
        });
    });
    Then('gathering the mounts fails, naming both areas', () => {
      expect(gathering).toThrow('the card approval is mounted by approval and by thread');
    });
  });

  ScenarioOutline(
    'The texts of an area cannot replace another text',
    ({ When, Then }, variables) => {
      let loading: () => unknown;
      When('an area adds the file "<file>" with the key "<key>"', () => {
        loading = () =>
          merge({ 'zh-CN': zh, en }, { [`./${variables.file}`]: { [variables.key]: 'x' } });
      });
      Then('loading the texts fails: "<problem>"', () => {
        expect(loading).toThrow(new Error(`./${variables.file}: ${variables.problem}`));
      });
    },
  );

  Scenario('Each web slice runs its own feature file', ({ Then, And }) => {
    // Vitest runs in web/.
    const root = join(process.cwd(), '../features');
    const here = join(process.cwd(), 'features');
    const features = readdirSync(root).filter((name) => /^web(_\w+)?\.feature$/.test(name));
    Then('every web feature file is tagged "@web"', () => {
      expect(features).toEqual(expect.arrayContaining(['web.feature', 'web_shell.feature']));
      for (const name of features)
        expect([name, readFileSync(join(root, name), 'utf8').split('\n')[0]]).toEqual([
          name,
          '@web',
        ]);
    });
    And(
      'every web feature file but the end-to-end one is run by the spec file of its slice',
      () => {
        for (const name of features) {
          const slice = name === 'web.feature' ? 'web' : name.slice(4, -8);
          const steps = slice === 'e2e' ? 'e2e.ts' : `${slice}.spec.ts`;
          const text = readFileSync(join(here, steps), 'utf8');
          expect([name, text.includes(`loadFeature('../features/${name}')`)]).toEqual([name, true]);
        }
      },
    );
  });
});
