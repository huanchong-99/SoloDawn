import 'fake-indexeddb/auto';
import { describeFeature, loadFeature } from '@amiceli/vitest-cucumber';
import { flushSync, tick } from 'svelte';
import { expect, vi } from 'vitest';
import { find, Home, homes } from '../src/lib/groups.svelte';
import { decode as b64Decode } from '../src/lib/identity';
import { goldens } from '../src/lib/mock';
import type { Source } from '../src/lib/net.svelte';
import { prefs } from '../src/lib/prefs.svelte';
import * as p from '../src/lib/proto';
import { bindServiceWorker, type SwClient, type SwScope } from '../src/sw';
import { example, TestHome, user } from './home';
import { all, closePage, go, one, openPage, texts } from './page';

const feature = await loadFeature('../features/web_reminder.feature');

const website = 'rZNvy-1jH6Z-BcPqA5U5BQ';
const docsGroup = 'DrtCn6htSBwmMPrFPbHJHA';
const approvalId = 'netluHiPQf-UoheEIBfbLw';
const p256dh = Uint8Array.from({ length: 65 }, (_, i) => (i === 0 ? 4 : i + 2));
const auth = Uint8Array.from({ length: 16 }, (_, i) => i + 20);

vi.mock('../src/lib/net.svelte', async (actual) => {
  const real = await actual<typeof import('../src/lib/net.svelte')>();
  return {
    ...real,
    start: (source: Source) => ({
      net: { source, homes, path: () => ({ kind: 'direct' as const, rtt: 12, since: 1 }) },
      stop: () => {},
    }),
  };
});

interface PushHomeRig {
  home: TestHome;
  store: Home;
  pushes: (p.PushOn | p.PushOff)[];
  connect(): Promise<void>;
}

function makePushHome(name: string, g: string, groupName: string): PushHomeRig {
  const home = new TestHome(name);
  home.add(g, groupName);
  const sign = async (payload: string) => ({ p: payload, k: 'this-browser', s: 'signature' });
  const store = new Home({ user, sign });
  const pushes: (p.PushOn | p.PushOff)[] = [];

  return {
    home,
    store,
    pushes,
    async connect() {
      const link = store.connect();
      const base = home.connect(link.push);
      const ctl = {
        request: async (req: { t: string; env?: p.Envelope }) => {
          if (req.t === 'cmd' && req.env) {
            const raw = JSON.parse(req.env.p) as { t: string; off?: unknown };
            if (raw.t === 'push') {
              pushes.push(
                raw.off === true ? p.decode(p.PushOff, req.env.p) : p.decode(p.PushOn, req.env.p),
              );
              return { re: 99, ok: true };
            }
          }
          return base.ctl.request(req as never);
        },
      } as p.Ctl;
      const welcome = (await base.ctl.request({
        t: 'hello',
        v: 1,
        kind: 'browser',
        name: 'test',
        addrs: [],
      })) as p.Welcome;
      link.online(ctl, welcome);
      await vi.waitFor(() => expect(find(g)?.info).toBeDefined());
    },
  };
}

function installBrowserPush(options: {
  permission?: NotificationPermission;
  userAgent?: string;
  standalone?: boolean;
}) {
  let perm: NotificationPermission = options.permission ?? 'default';
  let unsubscribed = 0;
  let activeSub: {
    endpoint: string;
    options: { applicationServerKey: ArrayBuffer | null };
    getKey(name: 'p256dh' | 'auth'): ArrayBuffer;
    unsubscribe(): Promise<boolean>;
  } | null = null;
  const registered: string[] = [];
  const messageListeners = new Set<(ev: MessageEvent) => void>();

  const reg = {
    pushManager: {
      getSubscription: vi.fn(async () => activeSub),
      subscribe: vi.fn(async (init: { applicationServerKey: Uint8Array<ArrayBuffer> }) => {
        activeSub = {
          endpoint: 'https://push.example/send/browser-1',
          options: { applicationServerKey: init.applicationServerKey.slice().buffer },
          getKey: (name: 'p256dh' | 'auth') =>
            name === 'p256dh' ? p256dh.slice().buffer : auth.slice().buffer,
          unsubscribe: vi.fn(async () => {
            unsubscribed += 1;
            activeSub = null;
            return true;
          }),
        };
        return activeSub;
      }),
    },
  };

  vi.spyOn(navigator, 'userAgent', 'get').mockReturnValue(
    options.userAgent ?? 'Mozilla/5.0 (Macintosh; Intel Mac OS X 14_0)',
  );
  Object.defineProperty(navigator, 'standalone', {
    configurable: true,
    value: options.standalone,
  });
  Object.defineProperty(navigator, 'serviceWorker', {
    configurable: true,
    value: {
      register: vi.fn(async (url: string) => {
        registered.push(url);
        return reg;
      }),
      getRegistration: vi.fn(async () => reg),
      addEventListener: (type: string, fn: (ev: MessageEvent) => void) => {
        if (type === 'message') messageListeners.add(fn);
      },
      removeEventListener: (type: string, fn: (ev: MessageEvent) => void) => {
        if (type === 'message') messageListeners.delete(fn);
      },
    },
  });
  vi.stubGlobal('PushManager', class {});
  vi.stubGlobal('Notification', {
    get permission() {
      return perm;
    },
    requestPermission: vi.fn(async () => {
      perm = 'granted';
      return perm;
    }),
  });

  return {
    registered,
    get unsubscribed() {
      return unsubscribed;
    },
    get activeSub() {
      return activeSub;
    },
    deliverSwMessage(data: unknown) {
      for (const fn of messageListeners) fn(new MessageEvent('message', { data }));
    },
  };
}

function makeSwHarness(langSearch: string, windows: SwClient[]) {
  const listeners = new Map<string, (ev: unknown) => void>();
  const shown: { title: string; body: string; data: Record<string, string> }[] = [];
  const opened: string[] = [];

  const scope: SwScope = {
    location: { search: langSearch },
    navigator: { language: 'en-US' },
    registration: {
      scope: `${location.origin}/`,
      showNotification: vi.fn(async (title, opts) => {
        shown.push({
          title,
          body: opts.body,
          data: opts.data as unknown as Record<string, string>,
        });
      }),
    },
    clients: {
      claim: async () => {},
      matchAll: async () => windows,
      openWindow: async (url) => {
        opened.push(url);
        return null;
      },
    },
    skipWaiting: async () => {},
    addEventListener: ((type: string, fn: (ev: unknown) => void) => {
      listeners.set(type, fn);
    }) as SwScope['addEventListener'],
  };
  bindServiceWorker(scope);

  async function fire(type: string, payload: Record<string, unknown>) {
    let work = Promise.resolve();
    listeners.get(type)!({
      ...payload,
      waitUntil: (p: Promise<unknown>) => {
        work = p as Promise<void>;
      },
    });
    await work;
  }

  return { shown, opened, fire };
}

describeFeature(feature, ({ Background, Scenario, AfterEachScenario }) => {
  let workstation: PushHomeRig;
  let mini: PushHomeRig;
  let pushRig: ReturnType<typeof installBrowserPush>;
  let swHarness: ReturnType<typeof makeSwHarness>;

  AfterEachScenario(async () => {
    await closePage();
    localStorage.clear();
    history.replaceState(null, '', '/');
    delete (navigator as { standalone?: unknown }).standalone;
  });

  Background(({ Given }) => {
    Given('a browser set to English', () => {
      prefs.lang = 'en';
    });
  });

  Scenario(
    'Enabling notifications subscribes the browser, syncs homes, updates choices, and turns off',
    ({ Given, And, When, Then }) => {
      Given('a browser that allows Web Push notifications', () => {
        pushRig = installBrowserPush({ permission: 'default' });
      });
      And('the home "workstation" holds the group "Website"', async () => {
        workstation = makePushHome('workstation', website, 'Website');
        homes.push(workstation.store);
        await workstation.connect();
      });
      When('the person opens the client at "#/settings"', () => openPage('#/settings'));
      Then('the browser notifications status reads "Off in this browser"', () => {
        expect(texts('[aria-labelledby="s-noti"] .st')).toEqual(['Off in this browser']);
      });
      And('the Home Screen note is not shown on a desktop browser', () => {
        expect(all('[aria-labelledby="s-noti"] .note.ios')).toEqual([]);
      });
      When('the person turns browser notifications on', async () => {
        const toggle = one<HTMLInputElement>('[aria-labelledby="s-noti"] .switch');
        toggle.checked = true;
        toggle.dispatchEvent(new Event('change', { bubbles: true }));
        await vi.waitFor(() => expect(workstation.pushes).toHaveLength(1));
        flushSync();
      });
      Then('the browser notifications status reads "Allowed in this browser"', () => {
        expect(texts('[aria-labelledby="s-noti"] .st')).toEqual(['Allowed in this browser']);
        expect(one<HTMLInputElement>('[aria-labelledby="s-noti"] .switch').checked).toBe(true);
      });
      And(
        'the service worker "./sw.js?lang=en" is registered with a valid VAPID subscription',
        () => {
          expect(pushRig.registered).toContain('./sw.js?lang=en');
          const first = workstation.pushes[0] as p.PushOn;
          expect(b64Decode(first.vapid.pub)?.length).toBe(65);
          expect(b64Decode(first.vapid.priv)?.length).toBe(32);
        },
      );
      And(
        'the home "workstation" received a push subscription for "mention, approval, decision, brief, done"',
        () => {
          const first = workstation.pushes[0] as p.PushOn;
          expect(first.on.join(', ')).toBe('mention, approval, decision, brief, done');
        },
      );
      When(
        'the person navigates to "#/groups" and a second home "mini" connects with group "Docs"',
        async () => {
          go('#/groups');
          mini = makePushHome('mini', docsGroup, 'Docs');
          homes.push(mini.store);
          await mini.connect();
          flushSync();
          await vi.waitFor(() => expect(mini.pushes).toHaveLength(1));
        },
      );
      Then('the home "mini" also received the same push subscription', () => {
        const wsPush = workstation.pushes[0] as p.PushOn;
        const miniPush = mini.pushes[0] as p.PushOn;
        expect(miniPush.sub).toEqual(wsPush.sub);
        expect(miniPush.vapid).toEqual(wsPush.vapid);
        expect(miniPush.on).toEqual(wsPush.on);
      });
      When('the person returns to "#/settings" and unchecks "A task is done"', async () => {
        go('#/settings');
        const box = all<HTMLInputElement>('[aria-labelledby="s-noti"] .check input').find((el) =>
          el.closest('label')!.textContent!.includes('A task is done'),
        )!;
        box.checked = false;
        box.dispatchEvent(new Event('change', { bubbles: true }));
        await vi.waitFor(() => {
          expect(workstation.pushes).toHaveLength(2);
          expect(mini.pushes).toHaveLength(2);
        });
      });
      Then(
        'both homes received a push subscription for "mention, approval, decision, brief"',
        () => {
          expect((workstation.pushes[1] as p.PushOn).on.join(', ')).toBe(
            'mention, approval, decision, brief',
          );
          expect((mini.pushes[1] as p.PushOn).on.join(', ')).toBe(
            'mention, approval, decision, brief',
          );
        },
      );
      When('the person turns browser notifications off', async () => {
        const toggle = one<HTMLInputElement>('[aria-labelledby="s-noti"] .switch');
        toggle.checked = false;
        toggle.dispatchEvent(new Event('change', { bubbles: true }));
        await vi.waitFor(() => {
          expect(workstation.pushes).toHaveLength(3);
          expect(mini.pushes).toHaveLength(3);
        });
        flushSync();
      });
      Then('the browser notifications status reads "Off in this browser" again', () => {
        expect(texts('[aria-labelledby="s-noti"] .st')).toEqual(['Off in this browser']);
      });
      And('the browser subscription is unsubscribed and both homes received push off', () => {
        expect(pushRig.unsubscribed).toBe(1);
        expect(pushRig.activeSub).toBeNull();
        expect(workstation.pushes[2]).toMatchObject({ t: 'push', g: website, off: true });
        expect(mini.pushes[2]).toMatchObject({ t: 'push', g: docsGroup, off: true });
      });
    },
  );

  Scenario(
    'A closed-page push reminder holds no command or message text, shows the summary, and opens the card',
    ({ Given, And, When, Then }) => {
      let approvalMsg: p.MsgRow & { kind: 'approval' };
      Given(
        'the home "workstation" holds the group "Website" with a pending approval card',
        async () => {
          pushRig = installBrowserPush({ permission: 'granted' });
          workstation = makePushHome('workstation', website, 'Website');
          workstation.home.write(website, example('row-member-ai', { g: website }));
          approvalMsg = example('row-msg-approval-pending', {
            g: website,
            thread: null,
            text: 'Need a quick check before running Playwright.',
          }) as p.MsgRow & { kind: 'approval' };
          workstation.home.write(website, approvalMsg);
          homes.push(workstation.store);
          await workstation.connect();
        },
      );
      And(
        'the service worker receives the wire approval reminder while no window is open',
        async () => {
          swHarness = makeSwHarness('?lang=en', []);
          await swHarness.fire('push', { data: { text: () => goldens['push-reminder'] } });
        },
      );
      Then('the pushed payload holds neither message text nor approval command', () => {
        const raw = goldens['push-reminder'];
        expect(raw).not.toContain(approvalMsg.text);
        expect(raw).not.toContain(approvalMsg.body.command);
        expect(Object.keys(JSON.parse(raw)).sort()).toEqual([
          'g',
          'group',
          'kind',
          'msg',
          'title',
          'who',
        ]);
      });
      And(
        'the browser notification reads "Website · Approval" and "Codex: Time the path change in a browser."',
        () => {
          expect(swHarness.shown).toEqual([
            {
              title: 'Website · Approval',
              body: 'Codex: Time the path change in a browser.',
              data: { g: website, msg: approvalId, kind: 'approval' },
            },
          ]);
        },
      );
      When('the person clicks the notification to open the closed client', async () => {
        await swHarness.fire('notificationclick', {
          notification: { data: swHarness.shown[0].data, close: () => {} },
        });
        expect(swHarness.opened).toHaveLength(1);
        const target = new URL(swHarness.opened[0]);
        history.replaceState(null, '', `${target.pathname}${target.search}${target.hash}`);
        openPage(target.hash);
        await tick();
        flushSync();
      });
      Then(
        'the client opens the chat of "Website" with focus on the approval card\'s "Approve" button',
        async () => {
          await vi.waitFor(() => {
            expect(texts('.chat-head h2')).toEqual(['Website']);
            const cardEl = document.getElementById(`m-${approvalId}`)!;
            expect(cardEl).not.toBeNull();
            expect(document.activeElement?.textContent?.trim()).toBe('Approve');
            expect(cardEl.contains(document.activeElement)).toBe(true);
          });
          expect(location.search).toBe('');
        },
      );
    },
  );

  Scenario(
    'Clicking a reminder while SoloDawn is open in Chinese focuses the tab and jumps to the card',
    ({ Given, And, When, Then }) => {
      let focused = 0;
      Given(
        'the home "workstation" holds the group "Website" with a pending approval card',
        async () => {
          pushRig = installBrowserPush({ permission: 'granted' });
          workstation = makePushHome('workstation', website, 'Website');
          workstation.home.write(website, example('row-member-ai', { g: website }));
          workstation.home.write(
            website,
            example('row-msg-approval-pending', { g: website, thread: null }),
          );
          homes.push(workstation.store);
          await workstation.connect();
        },
      );
      And('the person opens the client at "#/settings" in Chinese', () => {
        openPage('#/settings');
        prefs.lang = 'zh-CN';
        flushSync();
      });
      When(
        'the service worker receives the wire approval reminder in Chinese and the person clicks it',
        async () => {
          const openWin: SwClient = {
            url: `${location.origin}/#/settings`,
            focus: async () => {
              focused += 1;
            },
            postMessage: (msg) => pushRig.deliverSwMessage(msg),
          };
          swHarness = makeSwHarness('?lang=zh-CN', [openWin]);
          await swHarness.fire('push', { data: { text: () => goldens['push-reminder'] } });
          await swHarness.fire('notificationclick', {
            notification: { data: swHarness.shown[0].data, close: () => {} },
          });
          await tick();
          flushSync();
        },
      );
      Then(
        'the browser notification reads "Website · 审批申请" and "Codex: Time the path change in a browser."',
        () => {
          expect(swHarness.shown[0]).toMatchObject({
            title: 'Website · 审批申请',
            body: 'Codex: Time the path change in a browser.',
          });
        },
      );
      And(
        'the open window moves to the chat of "Website" with focus on the approval card\'s "批准" button',
        async () => {
          expect(focused).toBe(1);
          expect(swHarness.opened).toEqual([]);
          await vi.waitFor(() => {
            expect(texts('.chat-head h2')).toEqual(['Website']);
            expect(document.activeElement?.textContent?.trim()).toBe('批准');
          });
        },
      );
    },
  );

  Scenario(
    'iPhone and iPad require adding SoloDawn to the Home Screen before enabling notifications',
    ({ Given, When, Then, And }) => {
      Given('an iPhone browser tab that has not been added to the Home Screen', () => {
        pushRig = installBrowserPush({
          permission: 'default',
          userAgent: 'Mozilla/5.0 (iPhone; CPU iPhone OS 17_5 like Mac OS X)',
          standalone: false,
        });
      });
      When('the person opens the client at "#/settings"', () => openPage('#/settings'));
      Then(
        'the browser notifications switch is disabled with status "Add to Home Screen first"',
        () => {
          const toggle = one<HTMLInputElement>('[aria-labelledby="s-noti"] .switch');
          expect(toggle.disabled).toBe(true);
          expect(texts('[aria-labelledby="s-noti"] .st')).toEqual(['Add to Home Screen first']);
        },
      );
      And(
        'the highlighted Home Screen note explains notifications and keeping the browser key after seven days without a visit',
        () => {
          const note = one('[aria-labelledby="s-noti"] .note.ios');
          expect(note.getAttribute('data-tone')).toBe('dawn');
          expect(note.textContent).toContain('add SoloDawn to your Home Screen first');
          expect(note.textContent).toContain('after seven days without a visit');
        },
      );
      When(
        'the same iPhone opens SoloDawn from the Home Screen with Web Push allowed',
        async () => {
          await closePage();
          pushRig = installBrowserPush({
            permission: 'default',
            userAgent: 'Mozilla/5.0 (iPhone; CPU iPhone OS 17_5 like Mac OS X)',
            standalone: true,
          });
          openPage('#/settings');
        },
      );
      Then('the browser notifications switch is enabled and the Home Screen note is hidden', () => {
        const toggle = one<HTMLInputElement>('[aria-labelledby="s-noti"] .switch');
        expect(toggle.disabled).toBe(false);
        expect(all('[aria-labelledby="s-noti"] .note.ios')).toEqual([]);
      });
    },
  );
});
