import 'fake-indexeddb/auto';
import { describeFeature, loadFeature } from '@amiceli/vitest-cucumber';
import { flushSync } from 'svelte';
import { SvelteMap } from 'svelte/reactivity';
import { expect, vi } from 'vitest';
import { find, Home, homes } from '../src/lib/groups.svelte';
import { goldens } from '../src/lib/mock';
import type { Path, Source } from '../src/lib/net.svelte';
import { nodes } from '../src/lib/nodes.svelte';
import * as p from '../src/lib/proto';
import { example, TestHome } from './home';
import { all, closePage, one, openPage, texts } from './page';

const feature = await loadFeature('../features/web_nodes.feature');

const website = 'rZNvy-1jH6Z-BcPqA5U5BQ';
const paths = new SvelteMap<Home, Path | undefined>();

vi.mock('../src/lib/net.svelte', async (actual) => {
  const real = await actual<typeof import('../src/lib/net.svelte')>();
  return {
    ...real,
    start: (source: Source) => ({
      net: {
        source,
        homes,
        path: (home: Home) => (home.ctl ? paths.get(home) : undefined),
      },
      stop: () => {},
    }),
  };
});

interface Rig {
  home: TestHome;
  store: Home;
  device: string;
  commands: p.Envelope[];
  connect(): Promise<void>;
  drop(): void;
}

const user: string = JSON.parse(goldens['row-member-human']).user;
const sign = async (payload: string) => ({ p: payload, k: 'this-browser', s: 'signature' });

function makeNode(name: string, device: string, overrides: Partial<p.HostInfo> = {}): Rig {
  const home = new TestHome(name);
  const store = new Home({ user, sign });
  const commands: p.Envelope[] = [];
  let drop = () => {};

  const info: p.HostInfo = {
    ...p.decode(p.HostInfo, goldens['host-info-answer']),
    device,
    name,
    ...overrides,
  };

  return {
    home,
    store,
    device,
    commands,
    async connect() {
      const link = store.connect();
      const base = home.connect(link.push);
      const ctl = {
        request: async (req: { t: string; env?: p.Envelope }) => {
          if (req.t === 'host.info') return info;
          if (req.t === 'cmd' && req.env) {
            const parsed = JSON.parse(req.env.p) as { t: string };
            if (parsed.t === 'pair.browser') {
              commands.push(req.env);
              return p.decode(p.CmdResult, goldens['cmd-pair-link']);
            }
          }
          return base.ctl.request(req as never);
        },
      } as p.Ctl;
      drop = () => {
        base.close();
        link.offline();
      };
      const welcome = (await base.ctl.request({
        t: 'hello',
        v: 1,
        kind: 'browser',
        name: 'test',
        addrs: [],
      })) as p.Welcome;
      link.online(ctl, { ...welcome, device });
    },
    drop: () => drop(),
  };
}

describeFeature(feature, ({ Background, Scenario, AfterEachScenario }) => {
  let workstation: Rig;
  let windowsBox: Rig;

  AfterEachScenario(async () => {
    await closePage();
    paths.clear();
    nodes.infos.clear();
    nodes.devices.clear();
  });

  Background(({ Given }) => {
    Given('a browser set to English', () => {});
  });

  Scenario(
    'The top-bar indicator shows all four connection states and links to the computers page',
    ({ Given, When, Then, And }) => {
      Given('an owned computer "workstation" that hosts the group "Website"', async () => {
        workstation = makeNode('workstation', 'J3Uuh-y1z2AX536yAFlGHJL5sjxVpWVJavvVphvdm5Q');
        workstation.home.add(website, 'Website');
        homes.push(workstation.store);
        await workstation.connect();
        await vi.waitFor(() => expect(find(website)?.info).toBeDefined());
      });
      When(
        'the browser opens "#/groups/rZNvy-1jH6Z-BcPqA5U5BQ" while selecting a candidate pair',
        () => openPage(`#/groups/${website}`),
      );
      Then('the indicator in the top bar has state "linking" and reads "Reconnecting"', () => {
        const el = one<HTMLAnchorElement>('.bar .conn');
        expect(el.dataset.st).toBe('linking');
        expect(texts('.bar .conn')).toEqual(['Reconnecting']);
      });
      And('the indicator links to "#/nodes"', () => {
        expect(one<HTMLAnchorElement>('.bar .conn').getAttribute('href')).toBe('#/nodes');
      });
      When('the path to "workstation" becomes direct with latency 18 ms', () => {
        paths.set(workstation.store, { kind: 'direct', rtt: 18, since: Date.now() });
        flushSync();
      });
      Then('the indicator in the top bar has state "ok" and reads "Direct 18 ms"', () => {
        expect(one('.bar .conn').dataset.st).toBe('ok');
        expect(texts('.bar .conn')).toEqual(['Direct 18 ms']);
      });
      When('the path to "workstation" becomes relayed', () => {
        paths.set(workstation.store, { kind: 'relay', rtt: 210, since: Date.now() });
        flushSync();
      });
      Then('the indicator in the top bar has state "limited" and reads "Relayed · limited"', () => {
        expect(one('.bar .conn').dataset.st).toBe('limited');
        expect(texts('.bar .conn')).toEqual(['Relayed · limited']);
      });
      When('"workstation" stops', () => {
        workstation.drop();
        flushSync();
      });
      Then(
        'the indicator in the top bar has state "down" and reads "Cannot reach group home"',
        () => {
          expect(one('.bar .conn').dataset.st).toBe('down');
          expect(texts('.bar .conn')).toEqual(['Cannot reach group home']);
        },
      );
    },
  );

  Scenario(
    'Two computers are listed with direct, relayed and offline states, relay quota and AIs',
    ({ Given, And, When, Then }) => {
      Given(
        'an owned computer "workstation" on "macOS 15.2" running "v0.3.1" with direct path 4 ms and group "Website"',
        async () => {
          workstation = makeNode('workstation', 'J3Uuh-y1z2AX536yAFlGHJL5sjxVpWVJavvVphvdm5Q', {
            relay: null,
          });
          workstation.home.add(website, 'Website');
          workstation.home.write(website, example('row-task', { g: website }));
          homes.push(workstation.store);
          await workstation.connect();
          paths.set(workstation.store, { kind: 'direct', rtt: 4, since: Date.now() });
          await vi.waitFor(() => expect(find(website)?.info).toBeDefined());
        },
      );
      And(
        'an owned computer "windows-box" on "Windows 11" running "v0.3.1" on a relayed path for 26 minutes with quota 3.1 MB of 50 MB',
        async () => {
          windowsBox = makeNode('windows-box', 'W9Uuh-y1z2AX536yAFlGHJL5sjxVpWVJavvVphvdm5Q', {
            os: 'Windows 11',
            version: 'v0.3.1',
            open: 1,
            max_open: 3,
            homes: [],
            relay: { used: 3_250_000, limit: 52_428_800 },
            agents: [
              {
                agent: 'codex',
                cli_version: '0.164.0',
                account: 'api_key',
                login_ok: true,
                caps: {},
                groups: [],
              },
            ],
          });
          homes.push(windowsBox.store);
          await windowsBox.connect();
          paths.set(windowsBox.store, {
            kind: 'relay',
            rtt: 210,
            since: Date.now() - 26 * 60_000,
          });
        },
      );
      When('the browser opens "#/nodes"', async () => {
        openPage('#/nodes');
        await vi.waitFor(() => {
          flushSync();
          expect(all('.card.node')).toHaveLength(2);
          expect(texts('.card.node .sub')).toHaveLength(2);
        });
      });
      Then(
        'the page lists "workstation" as "Online" on "macOS 15.2 · Node v0.3.1 · 2 of 4 AIs open" with link "Direct · 4 ms"',
        () => {
          const card = all('.card.node')[0];
          expect(card.querySelector('h3')!.textContent).toBe('workstation');
          expect(card.querySelector('header .st')!.textContent!.trim()).toBe('Online');
          expect(card.querySelector('.sub')!.textContent!.replace(/\s+/g, ' ').trim()).toBe(
            'macOS 15.2 · Node v0.3.1 · 2 of 4 AIs open',
          );
          expect(card.querySelector('.kv dd')!.textContent!.replace(/\s+/g, ' ').trim()).toBe(
            'Direct · 4 ms',
          );
        },
      );
      And('"workstation" warns "This computer is the home of 1 group — keep it on."', () => {
        const card = all('.card.node')[0];
        expect(card.querySelector('.note b')!.textContent!.trim()).toBe(
          'This computer is the home of 1 group — keep it on.',
        );
      });
      And(
        '"workstation" lists "Claude Code" as "Personal subscription" and "Signed in", working in "Website"',
        () => {
          const ai = all('.card.node')[0].querySelectorAll('.ai-row')[0];
          expect(ai.textContent).toContain('Claude Code');
          expect(ai.textContent).toContain('Personal subscription');
          expect(ai.textContent).toContain('Signed in');
          expect(ai.querySelector('.perm')!.textContent!.replace(/\s+/g, ' ').trim()).toBe(
            'Website Working · T-12',
          );
        },
      );
      And('"workstation" lists "Codex" as "Account type not set" and "Not signed in"', () => {
        const ai = all('.card.node')[0].querySelectorAll('.ai-row')[1];
        expect(ai.textContent).toContain('Codex');
        expect(ai.textContent).toContain('Account type not set');
        expect(ai.textContent).toContain('Not signed in');
      });
      And(
        'the page lists "windows-box" as "Online" with link "Relay · 210 ms · 26 min" and relay notice "Relay data today: 3.1 MB / 50 MB"',
        () => {
          const card = all('.card.node')[1];
          expect(card.querySelector('h3')!.textContent).toBe('windows-box');
          expect(card.querySelector('header .st')!.textContent!.trim()).toBe('Online');
          expect(card.querySelector('.kv dd')!.textContent!.replace(/\s+/g, ' ').trim()).toBe(
            'Relay · 210 ms · 26 min',
          );
          expect(card.querySelector('.note .hint')!.textContent!.trim()).toBe(
            'Relay data today: 3.1 MB / 50 MB',
          );
        },
      );
      When('"windows-box" stops', () => {
        windowsBox.drop();
        flushSync();
      });
      Then(
        '"windows-box" is marked "Offline" and its AIs say "Offline · wakes when the computer returns"',
        () => {
          const card = all('.card.node')[1];
          expect(card.querySelector('header .st')!.getAttribute('data-st')).toBe('offline');
          expect(card.querySelector('header .st')!.textContent!.trim()).toBe('Offline');
          expect(card.querySelector('.ai-row .st')!.textContent!.trim()).toBe(
            'Offline · wakes when the computer returns',
          );
        },
      );
    },
  );

  Scenario(
    'Adding a computer opens onboarding and adding a browser requests a one-time pairing link',
    ({ Given, When, Then, And }) => {
      Given(
        'an owned computer "workstation" on "macOS 15.2" running "v0.3.1" with direct path 4 ms and group "Website"',
        async () => {
          workstation = makeNode('workstation', 'J3Uuh-y1z2AX536yAFlGHJL5sjxVpWVJavvVphvdm5Q');
          workstation.home.add(website, 'Website');
          homes.push(workstation.store);
          await workstation.connect();
          paths.set(workstation.store, { kind: 'direct', rtt: 4, since: Date.now() });
        },
      );
      When('the browser opens "#/nodes"', async () => {
        openPage('#/nodes');
        await vi.waitFor(() => {
          flushSync();
          expect(all('.card.node .sub')).toHaveLength(1);
        });
      });
      Then('the "Add a computer" link leads to "#/onboarding"', () => {
        const link = all<HTMLAnchorElement>('.page-head a').find((a) =>
          a.textContent!.includes('Add a computer'),
        );
        expect(link?.getAttribute('href')).toBe('#/onboarding');
      });
      When(
        'the person opens "Add a browser" and creates a pairing link on "workstation"',
        async () => {
          const btn = all<HTMLButtonElement>('.page-head button').find((b) =>
            b.textContent!.includes('Add a browser'),
          )!;
          expect(btn.disabled).toBe(false);
          btn.click();
          flushSync();
          expect(one<HTMLDialogElement>('dialog').open).toBe(true);
          one('dialog form').dispatchEvent(
            new Event('submit', { bubbles: true, cancelable: true }),
          );
          await vi.waitFor(() => {
            flushSync();
            expect(all('dialog .cmd')).toHaveLength(1);
          });
        },
      );
      Then('"workstation" receives a signed "pair.browser" command for its device id', () => {
        expect(workstation.commands).toHaveLength(1);
        const payload = JSON.parse(workstation.commands[0].p) as Record<string, unknown>;
        expect(payload).toMatchObject({
          t: 'pair.browser',
          to: workstation.device,
        });
      });
      And(
        'the dialog shows the pairing link "https://rv.example/#pair=7K2M-9QXD-4F8T-WB3N" with a copy button',
        async () => {
          const writeText = vi.fn().mockResolvedValue(undefined);
          vi.stubGlobal('navigator', { ...navigator, clipboard: { writeText } });
          expect(texts('dialog .cmd')).toEqual(['https://rv.example/#pair=7K2M-9QXD-4F8T-WB3N']);
          const copyBtn = one<HTMLButtonElement>('dialog .pair-box button');
          expect(copyBtn.textContent!.trim()).toBe('Copy link');
          copyBtn.click();
          await vi.waitFor(() => {
            flushSync();
            expect(copyBtn.textContent!.trim()).toBe('Copied');
          });
          expect(writeText).toHaveBeenCalledWith('https://rv.example/#pair=7K2M-9QXD-4F8T-WB3N');
        },
      );
    },
  );

  Scenario(
    'An empty computers page prompts to add a computer, and a host.info push updates a card live',
    ({ When, Then }) => {
      When('the browser opens "#/nodes"', () => openPage('#/nodes'));
      Then(
        'the page shows the empty notice "No computers connected yet" with a link to "#/onboarding"',
        () => {
          expect(texts('.empty h3')).toEqual(['No computers connected yet']);
          expect(one<HTMLAnchorElement>('.empty a').getAttribute('href')).toBe('#/onboarding');
        },
      );
      When(
        'an owned computer "workstation" connects and pushes a host.info update with 3 open AIs',
        async () => {
          workstation = makeNode('workstation', 'J3Uuh-y1z2AX536yAFlGHJL5sjxVpWVJavvVphvdm5Q');
          homes.push(workstation.store);
          await workstation.connect();
          paths.set(workstation.store, { kind: 'direct', rtt: 6, since: Date.now() });
          await vi.waitFor(() => {
            flushSync();
            expect(all('.card.node .sub')).toHaveLength(1);
          });
          nodes.push({
            ...p.decode(p.HostInfoPush, goldens['host-info-push']),
            device: workstation.device,
            name: 'workstation',
            open: 3,
            max_open: 4,
            offline: false,
          });
          flushSync();
        },
      );
      Then('the page lists "workstation" with "3 of 4 AIs open"', () => {
        expect(one('.card.node .sub').textContent!.replace(/\s+/g, ' ').trim()).toBe(
          'macOS 15.2 · Node v0.3.1 · 3 of 4 AIs open',
        );
      });
    },
  );
});
