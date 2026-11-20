import { describeFeature, loadFeature } from '@amiceli/vitest-cucumber';
import { IDBFactory } from 'fake-indexeddb';
import { expect, vi } from 'vitest';
import { DeviceLog, Refused } from '../src/lib/devlog';
import { Device, ed25519, loadDevice, openCommand, type KeyStore } from '../src/lib/identity';
import { mac, newSecret, parseSecret, room, shown, verifyPair } from '../src/lib/pairing';
import { examples, goldens, mockCtl, schemaOf } from '../src/lib/mock';
import * as motion from './motion';
import * as proto from '../src/lib/proto';

const feature = await loadFeature('../features/web.feature');

const noEd25519 = {
  subtle: { generateKey: () => Promise.reject(new DOMException('Ed25519', 'NotSupportedError')) },
} as unknown as Crypto;

let browserCrypto: Crypto;
let browserCss: Pick<typeof CSS, 'supports'>;
let browserIdb: () => IDBFactory;
let svelte: typeof import('svelte');
let app: Record<string, never> | undefined;

/** Loads the client the way a page load does: every module fresh (Svelte's runtime too), then boot. */
async function open(hash = '') {
  if (app) await svelte.unmount(app);
  vi.resetModules();
  location.hash = hash;
  svelte = await import('svelte');
  const { boot } = await import('../src/lib/boot');
  app = await boot(document.body, {
    crypto: browserCrypto,
    css: browserCss,
    idb: browserIdb,
  });
  svelte.flushSync();
}

/** The test runtime's WebCrypto has Ed25519, happy-dom's CSS.supports answers yes, and site
 *  storage is an empty IndexedDB that lasts the scenario. */
function supported() {
  const idb = new IDBFactory();
  browserCrypto = crypto;
  browserCss = CSS;
  browserIdb = () => idb;
}

/** This browser's device, as the client keeps it. */
const thisDevice = () => loadDevice({ crypto, idb: browserIdb });

const newDevice = async () =>
  Device.of(crypto, await crypto.subtle.generateKey(ed25519, false, ['sign', 'verify']));

const entry = (by: Device, user: Device, ver: number, op: string, dev: Device) =>
  by.sign(
    `{"t":"device","n":"n${ver}","ts":1,"user":"${user.id}","ver":${ver},"op":"${op}","dev":"${dev.id}"` +
      (op === 'add' ? ',"kind":"browser","name":"tablet"}' : '}'),
  );

const texts = (selector: string) =>
  [...document.querySelectorAll(selector)].map((e) => e.textContent!.trim());

function choose(label: string) {
  const option = [...document.querySelectorAll('label')].find(
    (l) => l.textContent!.trim() === label,
  )!;
  option.querySelector('input')!.click();
  svelte.flushSync();
}

function go(hash: string) {
  location.hash = hash;
  dispatchEvent(new HashChangeEvent('hashchange', { newURL: location.href }));
  svelte.flushSync();
}

function press(label: string) {
  const button = document.querySelector<HTMLElement>(`button[aria-label="${label}"]`)!;
  button.focus();
  button.click();
  svelte.flushSync();
}

function expectMainFocus(title: string) {
  const main = document.querySelector('main')!;
  expect(document.activeElement).toBe(main);
  expect(document.getElementById(main.getAttribute('aria-labelledby')!)!.textContent).toBe(title);
}

describeFeature(feature, ({ Background, Scenario, ScenarioOutline, AfterEachScenario }) => {
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
    Given('the browser supports Ed25519 in WebCrypto', supported);
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
    Given('the browser supports Ed25519 in WebCrypto', supported);
    And('the person opens the client', () => open());
    When('the address fragment becomes "#/nodes"', () => go('#/nodes'));
    Then('the current page is "我的电脑和 AI"', () => {
      expect(texts('h1')).toEqual(['我的电脑和 AI']);
      expect(document.title).toBe('我的电脑和 AI · SoloDawn');
      expect(texts('nav a[aria-current="page"]')).toEqual(['电脑和 AI']);
    });
    And('focus is on the main landmark named "我的电脑和 AI"', () =>
      expectMainFocus('我的电脑和 AI'),
    );
  });

  Scenario('The skip button moves focus into the current page', ({ Given, When, Then, And }) => {
    Given('the browser supports Ed25519 in WebCrypto', supported);
    And('the person opens the client', () => open());
    When('the person activates the skip button', () => {
      document.querySelector<HTMLButtonElement>('button.skip')!.click();
    });
    Then('focus is on the main landmark named "群聊"', () => expectMainFocus('群聊'));
  });

  Scenario('An unknown address opens the chats page', ({ Given, When, Then }) => {
    Given('the browser supports Ed25519 in WebCrypto', supported);
    When('the person opens the client at "#/nowhere"', () => open('#/nowhere'));
    Then('the current page is "群聊"', () => {
      expect(texts('h1')).toEqual(['群聊']);
    });
  });

  Scenario(
    'The language choice changes the interface texts and is remembered',
    ({ Given, When, Then, And }) => {
      Given('the browser supports Ed25519 in WebCrypto', supported);
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
    Given('the browser supports Ed25519 in WebCrypto', supported);
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
    Given('the browser supports Ed25519 in WebCrypto', supported);
    And('the person opens the client', () => open());
    When('the person presses "切换深色 / 浅色"', () => press('切换深色 / 浅色'));
    Then('the focused theme button announces "主题: 深色"', () => {
      expect(document.activeElement?.getAttribute('aria-label')).toBe('切换深色 / 浅色');
      expect(texts('[role=status]')).toEqual(['主题: 深色']);
    });
    When('the person presses "Switch language / 切换语言"', () =>
      press('Switch language / 切换语言'),
    );
    Then('the page uses the "dark" theme', () => {
      expect(document.documentElement.dataset.theme).toBe('dark');
    });
    And('the page is marked as "en"', () => {
      expect(document.documentElement.lang).toBe('en');
    });
    And('the focused language button announces "Language: English"', () => {
      expect(document.activeElement?.getAttribute('aria-label')).toBe('Switch language / 切换语言');
      expect(texts('[role=status]')).toEqual(['Language: English']);
      expect(document.title).toBe('Chats · SoloDawn');
    });
  });

  Scenario(
    'The mark of the current page moves to the entry that is opened',
    ({ Given, When, Then, And }) => {
      let direction: number[];
      Given('the browser supports Ed25519 in WebCrypto', motion.newPage);
      And('the person opens the client', () => motion.open());
      When('the address fragment becomes "#/settings"', async () => {
        direction = await motion.arrival('#/settings');
      });
      Then('the mark of the current page is on entry 3 of the navigation', () =>
        motion.expectMark('nav .tabs', 'a', 2),
      );
      And('the page arrives from the side of the later entries', () => {
        expect(direction).toEqual([0, 1]);
      });
    },
  );

  Scenario(
    'A page opened from a later one arrives from the other side',
    ({ Given, When, Then, And }) => {
      let direction: number[];
      Given('the browser supports Ed25519 in WebCrypto', motion.newPage);
      And('the person opens the client at "#/settings"', () => motion.open('#/settings'));
      When('the address fragment becomes "#/nodes"', async () => {
        direction = await motion.arrival('#/nodes');
      });
      Then('the mark of the current page is on entry 2 of the navigation', () =>
        motion.expectMark('nav .tabs', 'a', 1),
      );
      And('the page arrives from the side of the earlier entries', () => {
        expect(direction).toEqual([0, -1]);
      });
    },
  );

  Scenario('The mark of the chosen option moves with the choice', ({ Given, When, Then, And }) => {
    Given('the browser supports Ed25519 in WebCrypto', motion.newPage);
    And('the person opens the client at "#/settings"', () => motion.open('#/settings'));
    When('the person chooses the theme "深色"', () => motion.choose('深色'));
    And('the person chooses the language "English"', () => motion.choose('English'));
    Then('the mark of the theme is on option 3', () =>
      motion.expectMark('fieldset:has([name=theme])', 'label', 2),
    );
    And('the mark of the language is on option 2', () =>
      motion.expectMark('fieldset:has([name=lang])', 'label', 1),
    );
  });

  Scenario(
    'The page cross-fades to a new theme in a browser with view transitions',
    ({ Given, When, Then, And }) => {
      let themes: (string | undefined)[];
      Given('the browser supports Ed25519 in WebCrypto', motion.newPage);
      And('the browser has view transitions', motion.viewTransitions);
      And('the person opens the client', () => motion.open());
      When('the person presses "切换深色 / 浅色"', async () => {
        themes = await motion.crossFade();
      });
      Then('the page uses the "auto" theme until the cross-fade has its first picture', () => {
        expect(themes[0]).toBe('auto');
      });
      And('the page uses the "dark" theme in the cross-fade', () => {
        expect(themes[1]).toBe('dark');
      });
    },
  );

  Scenario('A press during the cross-fade reaches the page', ({ Given, When, Then, And }) => {
    let result: Awaited<ReturnType<typeof motion.pressDuringCrossFade>>;
    Given('the browser supports Ed25519 in WebCrypto', motion.newPage);
    And('the browser has view transitions', motion.viewTransitions);
    And('the person opens the client', () => motion.open());
    When('the person presses "切换深色 / 浅色" and again while the page cross-fades', async () => {
      result = await motion.pressDuringCrossFade();
    });
    Then('the second press came while the picture of the old page was up', () => {
      expect(result.pictures).toEqual(['::view-transition-old(root)']);
    });
    And('the page uses the "light" theme', () => {
      expect(result.theme).toBe('light');
    });
  });

  Scenario(
    'A cross-fade under way ends when the system starts to ask for reduced motion',
    ({ Given, When, Then, And }) => {
      let result: Awaited<ReturnType<typeof motion.reduceDuringCrossFade>>;
      Given('the browser supports Ed25519 in WebCrypto', motion.newPage);
      And('the browser has view transitions', motion.viewTransitions);
      And('the person opens the client', () => motion.open());
      When(
        'the person presses "切换深色 / 浅色" and the system asks for reduced motion while the page cross-fades',
        async () => {
          result = await motion.reduceDuringCrossFade();
        },
      );
      Then('the picture of the old page is gone', () => {
        expect(result.before).toEqual(['::view-transition-old(root)']);
        expect(result.after).toEqual([]);
      });
      And('the page uses the "dark" theme', () => {
        expect(result.theme).toBe('dark');
      });
    },
  );

  Scenario(
    'The theme changes at once when the system asks for reduced motion',
    ({ Given, When, Then, And }) => {
      let result: Awaited<ReturnType<typeof motion.reducedThemeChange>>;
      Given('the browser supports Ed25519 in WebCrypto', motion.newPage);
      And('the browser has view transitions', motion.viewTransitions);
      And('the system asks for reduced motion', motion.reduce);
      And('the person opens the client', () => motion.open());
      When('the person presses "切换深色 / 浅色"', async () => {
        result = await motion.reducedThemeChange();
      });
      Then('the page uses the "dark" theme', () => {
        expect(result.theme).toBe('dark');
      });
      And('the page has not cross-faded', () => {
        expect(result.transitions).toBe(0);
        expect(result.animations).toBe(0);
      });
    },
  );

  Scenario(
    'The client opens when the browser blocks site storage',
    ({ Given, When, Then, And }) => {
      Given('the browser supports Ed25519 in WebCrypto', supported);
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
      supported();
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
    'A browser that cannot keep its Ed25519 key gets the upgrade notice',
    ({ Given, When, Then, And }) => {
      // Firefox 129 to 135: the key is made and stored, but a structured clone of it (what
      // IndexedDB reads back) fails.
      Given('the browser makes Ed25519 keys but cannot clone them for storage', () => {
        supported();
        vi.spyOn(globalThis, 'structuredClone').mockImplementation(() => {
          throw new DOMException('CryptoKey', 'DataCloneError');
        });
      });
      When('the person opens the client', () => open());
      Then('the upgrade notice "这个浏览器需要升级" is shown', () => {
        expect(texts('h1')).toEqual(['这个浏览器需要升级']);
      });
      And('the application is not shown', () => {
        expect(document.querySelector('nav')).toBeNull();
      });
    },
  );

  Scenario(
    'A browser without light-dark() colours gets the upgrade notice',
    ({ Given, When, Then, And }) => {
      Given('the browser supports Ed25519 in WebCrypto but not light-dark() in CSS', () => {
        supported();
        browserCss = { supports: () => false };
      });
      When('the person opens the client', () => open());
      Then('the upgrade notice "这个浏览器需要升级" is shown', () => {
        expect(texts('h1')).toEqual(['这个浏览器需要升级']);
      });
      And('the application is not shown', () => {
        expect(document.querySelector('nav')).toBeNull();
      });
    },
  );

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
      And('each one is read without a field added that the client does not know', () => {
        for (const name of names) {
          const text = goldens[name].replace('{', '{"unknown":1,');
          const value = proto.decode(schemaOf[name], text);
          expect(value, name).toStrictEqual(JSON.parse(goldens[name]));
          expect(value, name).not.toStrictEqual(JSON.parse(text));
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

  Scenario(
    'A browser that blocks site storage gets the storage notice',
    ({ Given, When, Then, And }) => {
      Given('the browser supports Ed25519 in WebCrypto but blocks site storage', () => {
        supported();
        // What Firefox does when the person blocks site data: IndexedDB refuses to open.
        browserIdb = () => {
          throw new DOMException('The operation is insecure.', 'SecurityError');
        };
      });
      When('the person opens the client', () => open());
      Then('the notice "站点存储被阻止" is shown', () => {
        expect(texts('h1')).toEqual(['站点存储被阻止']);
      });
      And('the application is not shown', () => {
        expect(document.querySelector('nav')).toBeNull();
      });
    },
  );

  Scenario('The browser keeps its device key between visits', ({ Given, When, Then, And }) => {
    let firstId = '';
    Given('the browser supports Ed25519 in WebCrypto', supported);
    And('the person has opened the client once', async () => {
      await open();
      firstId = (await thisDevice()).id;
    });
    When('the person opens the client again', () => open());
    Then('the browser is the same device as the first time', async () => {
      expect(texts('nav a[aria-current="page"]')).toEqual(['群聊']);
      expect((await thisDevice()).id).toBe(firstId);
    });
  });

  ScenarioOutline(
    'A receiver answers a signed command with one code',
    ({ Given, When, Then }, variables) => {
      const now = 1763600000000;
      let browser: Device;
      let node: Device;
      let env: { p: string; k: string; s: string };
      let receiver = '';
      let at = now;
      let valid = true;
      Given("a command that this browser signed for the owner's node", async () => {
        [browser, node] = [await newDevice(), await newDevice()];
        receiver = node.id;
        env = await browser.sign(`{"t":"cmd","n":"c1","ts":${now},"to":"${node.id}"}`);
      });
      When('<event>', () => {
        const event: string = variables.event;
        if (event.includes('is changed')) env = { ...env, p: env.p.replace('cmd', 'cme') };
        if (event.includes('revoked')) valid = false;
        if (event.includes('another device') || event.includes('misdirected')) receiver = 'other';
        if (event.includes('25 hours') || event.includes('late')) at = now + 25 * 3_600_000;
      });
      Then('the answer is "<answer>"', async () => {
        const opened = await openCommand(crypto, env, receiver, () => valid, at);
        expect('code' in opened ? opened.code : 'accepted').toBe(variables.answer);
      });
    },
  );

  Scenario('A revoked device is out of the log for good', ({ Given, When, Then, And }) => {
    let me: Device;
    let tablet: Device;
    let node: Device;
    let log: DeviceLog;
    let revoke: { p: string; k: string; s: string };
    const refused = (env: Promise<{ p: string; k: string; s: string }>) =>
      env
        .then((e) => log.apply(e))
        .then(
          () => 'accepted',
          (e: unknown) => (e instanceof Refused ? 'refused' : e),
        );
    Given('the device log of a person with this browser, a tablet and a node', async () => {
      [me, tablet, node] = [await newDevice(), await newDevice(), await newDevice()];
      log = await DeviceLog.replay(crypto, me.id, [
        await entry(me, me, 1, 'add', me),
        await entry(me, me, 2, 'add', tablet),
        await entry(tablet, me, 3, 'add', node),
      ]);
    });
    When('the node revokes the tablet', async () => {
      revoke = await entry(node, me, 4, 'revoke', tablet);
      expect(await log.apply(revoke)).toBe(true);
    });
    Then('the valid devices are this browser and the node', () => {
      expect(log.validDevices()).toStrictEqual([me.id, node.id].sort());
    });
    And('an entry signed by the tablet is refused', async () => {
      expect(await refused(entry(tablet, me, 5, 'add', tablet))).toBe('refused');
    });
    And('an entry that adds the tablet again is refused', async () => {
      expect(await refused(entry(me, me, 5, 'add', tablet))).toBe('refused');
    });
    And('the same revoke arriving a second time changes nothing', async () => {
      expect(await log.apply(revoke)).toBe(false);
      expect(log.entries).toHaveLength(4);
    });
  });

  Scenario(
    'Two devices that share a pairing secret recognise each other',
    ({ Given, When, Then, And }) => {
      let secret = '';
      let typed = '';
      let browser: Device;
      let node: Device;
      const pair = async (side: Device, role: string, as: string) => ({
        t: 'pair' as const,
        k: side.id,
        kind: role === 'a' ? 'browser' : 'node',
        name: role,
        role,
        mac: await mac(crypto, as, role, side.id),
      });
      Given('this browser created a pairing secret', async () => {
        secret = newSecret(crypto);
        [browser, node] = [await newDevice(), await newDevice()];
      });
      When('a node is given the secret as a person typed it, in lower case', () => {
        typed = parseSecret(shown(secret).toLowerCase());
      });
      Then('both sides meet in the same room', async () => {
        expect(typed).toBe(secret);
        expect(await room(crypto, typed)).toBe(await room(crypto, secret));
      });
      And("each side accepts the other's pair message", async () => {
        expect(await verifyPair(crypto, typed, await pair(browser, 'a', secret), 'a')).toBe(true);
        expect(await verifyPair(crypto, secret, await pair(node, 'b', typed), 'b')).toBe(true);
      });
      And('neither side accepts its own pair message sent back', async () => {
        expect(await verifyPair(crypto, secret, await pair(browser, 'a', secret), 'b')).toBe(false);
        expect(await verifyPair(crypto, typed, await pair(node, 'b', typed), 'a')).toBe(false);
      });
    },
  );
});
