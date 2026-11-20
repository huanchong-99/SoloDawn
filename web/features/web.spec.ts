import { describeFeature, loadFeature } from '@amiceli/vitest-cucumber';
import * as v from 'valibot';
import { expect, vi } from 'vitest';
import { examples, goldens, mockCtl, schemaOf } from '../src/lib/mock';
import * as proto from '../src/lib/proto';

const feature = await loadFeature('../features/web.feature');

const noEd25519 = {
  subtle: { generateKey: () => Promise.reject(new DOMException('Ed25519', 'NotSupportedError')) },
} as unknown as Crypto;

let browserCrypto: Crypto;
let svelte: typeof import('svelte');
let app: Record<string, never> | undefined;

/** Loads the client the way a page load does: every module fresh (Svelte's runtime too), then boot. */
async function open(hash = '') {
  if (app) await svelte.unmount(app);
  vi.resetModules();
  location.hash = hash;
  svelte = await import('svelte');
  const { boot } = await import('../src/lib/boot');
  app = await boot(document.body, browserCrypto);
  svelte.flushSync();
}

const texts = (selector: string) =>
  [...document.querySelectorAll(selector)].map((e) => e.textContent!.trim());

function choose(label: string) {
  const option = [...document.querySelectorAll('label')].find(
    (l) => l.textContent!.trim() === label,
  )!;
  option.querySelector('input')!.click();
  svelte.flushSync();
}

function press(label: string) {
  document.querySelector<HTMLElement>(`button[aria-label="${label}"]`)!.click();
  svelte.flushSync();
}

describeFeature(feature, ({ Background, Scenario, AfterEachScenario }) => {
  AfterEachScenario(() => {
    vi.restoreAllMocks();
    localStorage.clear();
  });

  Background(({ Given }) => {
    Given('a browser set to Chinese and a light system theme', () => {
      vi.spyOn(navigator, 'language', 'get').mockReturnValue('zh-CN');
      vi.spyOn(window, 'matchMedia').mockReturnValue({
        matches: false,
        addEventListener() {},
        removeEventListener() {},
      } as unknown as MediaQueryList);
    });
  });

  Scenario('The shell opens on the chats page', ({ Given, When, Then, And }) => {
    Given('the browser supports Ed25519 in WebCrypto', () => {
      browserCrypto = crypto;
    });
    When('the person opens the client', () => open());
    Then('the navigation reads "群聊", "电脑和 AI", "设置"', () => {
      expect(texts('nav a[href^="#/"] span')).toEqual(['群聊', '电脑和 AI', '设置']);
    });
    And('the current page is "群聊"', () => {
      expect(texts('h1')).toEqual(['群聊']);
      expect(texts('nav a[aria-current="page"]')).toEqual(['群聊']);
    });
  });

  Scenario('The address fragment selects the page', ({ Given, When, Then, And }) => {
    Given('the browser supports Ed25519 in WebCrypto', () => {
      browserCrypto = crypto;
    });
    And('the person opens the client', () => open());
    When('the address fragment becomes "#/nodes"', () => {
      location.hash = '#/nodes';
      dispatchEvent(new HashChangeEvent('hashchange'));
      svelte.flushSync();
    });
    Then('the current page is "我的电脑和 AI"', () => {
      expect(texts('h1')).toEqual(['我的电脑和 AI']);
      expect(texts('nav a[aria-current="page"]')).toEqual(['电脑和 AI']);
    });
  });

  Scenario('An unknown address opens the chats page', ({ Given, When, Then }) => {
    Given('the browser supports Ed25519 in WebCrypto', () => {
      browserCrypto = crypto;
    });
    When('the person opens the client at "#/nowhere"', () => open('#/nowhere'));
    Then('the current page is "群聊"', () => {
      expect(texts('h1')).toEqual(['群聊']);
    });
  });

  Scenario(
    'The language choice changes the interface texts and is remembered',
    ({ Given, When, Then, And }) => {
      Given('the browser supports Ed25519 in WebCrypto', () => {
        browserCrypto = crypto;
      });
      And('the person opens the client at "#/settings"', () => open('#/settings'));
      When('the person chooses the language "English"', () => choose('English'));
      Then('the navigation reads "Chats", "Computers", "Settings"', () => {
        expect(texts('nav a[href^="#/"] span')).toEqual(['Chats', 'Computers', 'Settings']);
      });
      And('the page is marked as "en"', () => {
        expect(document.documentElement.lang).toBe('en');
      });
      And(
        'the navigation reads "Chats", "Computers", "Settings" after the client is opened again',
        async () => {
          await open();
          expect(texts('nav a[href^="#/"] span')).toEqual(['Chats', 'Computers', 'Settings']);
        },
      );
    },
  );

  Scenario('The theme choice is applied and remembered', ({ Given, When, Then, And }) => {
    Given('the browser supports Ed25519 in WebCrypto', () => {
      browserCrypto = crypto;
    });
    And('the person opens the client at "#/settings"', () => open('#/settings'));
    When('the person chooses the theme "深色"', () => choose('深色'));
    Then('the page uses the "dark" theme', () => {
      expect(document.documentElement.dataset.theme).toBe('dark');
    });
    And('the page uses the "dark" theme after the client is opened again', async () => {
      delete document.documentElement.dataset.theme;
      await open();
      expect(document.documentElement.dataset.theme).toBe('dark');
    });
  });

  Scenario('The top bar switches theme and language', ({ Given, When, Then, And }) => {
    Given('the browser supports Ed25519 in WebCrypto', () => {
      browserCrypto = crypto;
    });
    And('the person opens the client', () => open());
    When('the person presses "切换深色 / 浅色"', () => press('切换深色 / 浅色'));
    And('the person presses "Switch language / 切换语言"', () =>
      press('Switch language / 切换语言'),
    );
    Then('the page uses the "dark" theme', () => {
      expect(document.documentElement.dataset.theme).toBe('dark');
    });
    And('the page is marked as "en"', () => {
      expect(document.documentElement.lang).toBe('en');
    });
  });

  Scenario(
    'The client opens when the browser blocks site storage',
    ({ Given, When, Then, And }) => {
      Given('the browser supports Ed25519 in WebCrypto', () => {
        browserCrypto = crypto;
      });
      And('the browser blocks site storage', () => {
        vi.spyOn(globalThis, 'localStorage', 'get').mockImplementation(() => {
          throw new DOMException('The operation is insecure.', 'SecurityError');
        });
      });
      When('the person opens the client', () => open());
      Then('the current page is "群聊"', () => {
        expect(texts('h1')).toEqual(['群聊']);
      });
      And('the page uses the "auto" theme', () => {
        expect(document.documentElement.dataset.theme).toBe('auto');
      });
    },
  );

  Scenario('A browser without Ed25519 gets the upgrade notice', ({ Given, When, Then, And }) => {
    Given('the browser lacks Ed25519 in WebCrypto', () => {
      browserCrypto = noEd25519;
    });
    When('the person opens the client', () => open());
    Then('the upgrade notice "这个浏览器需要升级" is shown', () => {
      expect(texts('h1')).toEqual(['这个浏览器需要升级']);
    });
    And('the application is not shown', () => {
      expect(document.querySelector('nav')).toBeNull();
    });
  });

  Scenario(
    'Every wire example of the node is read as a typed frame',
    ({ Given, When, Then, And }) => {
      let names: string[];
      let read: unknown[];
      Given('the wire examples in "internal/proto/testdata"', () => {
        names = examples;
        expect(names).toContain('welcome');
      });
      When('the client reads each one with the schema of its file name', () => {
        read = names.map((name) => proto.decode(schemaOf[name], goldens[name]));
      });
      Then('each one is read with every field as written', () => {
        expect(read).toStrictEqual(names.map((name) => JSON.parse(goldens[name])));
      });
      And('each one is refused with one unknown field added', () => {
        for (const name of names) {
          const text = goldens[name].replace('{', '{"unknown":1,');
          expect(() => proto.decode(schemaOf[name], text), name).toThrow(v.ValiError);
        }
      });
    },
  );

  Scenario('Screens can sync a group with no node', ({ Given, When, Then }) => {
    const seen: string[] = [];
    let ctl: proto.Ctl;
    let welcome: proto.Welcome;
    Given('a mock connection, with no node', () => {
      ctl = mockCtl((push) => seen.push(push.t));
    });
    When('the client sends "hello"', async () => {
      const answer = await ctl.request({
        t: 'hello',
        v: 1,
        kind: 'browser',
        name: 'test',
        addrs: [],
      });
      welcome = answer as proto.Welcome;
    });
    Then('the answer lists the group "Website"', () => {
      expect(welcome.re).toBe(1);
      expect(welcome.groups.map((g) => g.name)).toEqual(['Website']);
    });
    When('the client syncs that group', async () => {
      const g = welcome.groups[0].g;
      const synced = await ctl.request({ t: 'sync', g, since: 0, state_only: false });
      seen.push(`${(synced as proto.Synced).t} ${(synced as proto.Synced).seq}`);
    });
    Then('the pushes "reset" and "rows" arrive before the answer "synced" at 815', () => {
      expect(seen).toEqual(['reset', 'rows', 'synced 815']);
    });
  });

  Scenario('Screens can page a thread and send a command with no node', ({ Given, When, Then }) => {
    let ctl: proto.Ctl;
    let page: proto.Fetched;
    let result: proto.CmdResult | proto.ErrorResponse;
    Given('a mock connection, with no node', () => {
      ctl = mockCtl(() => {});
    });
    When('the client fetches the newest page of the main line', async () => {
      const g = 'rZNvy-1jH6Z-BcPqA5U5BQ';
      page = (await ctl.request({
        t: 'fetch',
        g,
        before: 0,
        limit: 50,
        thread: null,
      })) as proto.Fetched;
    });
    Then('the page holds the message "Choose a location."', () => {
      expect(page.rows.map((row) => (row as proto.MsgRow).text)).toEqual(['Choose a location.']);
    });
    When('the client sends a command', async () => {
      const env = proto.decode(proto.Cmd, goldens.cmd).env;
      result = await ctl.request({ t: 'cmd', env });
    });
    Then('the command is accepted', () => {
      expect(result).toStrictEqual({ re: 2, ok: true });
    });
  });
});
