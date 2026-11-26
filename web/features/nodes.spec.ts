import 'fake-indexeddb/auto';
import { describeFeature, loadFeature } from '@amiceli/vitest-cucumber';
import { flushSync } from 'svelte';
import { SvelteMap } from 'svelte/reactivity';
import { expect, vi } from 'vitest';
import { find, Home, homes } from '../src/lib/groups.svelte';
import type { Path, Source } from '../src/lib/net.svelte';
import { nodes } from '../src/lib/nodes.svelte';
import { example, ownedNode, type OwnedNode } from './home';
import { all, closePage, go, one, openPage, texts } from './page';

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

describeFeature(feature, ({ Background, Scenario, AfterEachScenario }) => {
  let workstation: OwnedNode;
  let windowsBox: OwnedNode;

  AfterEachScenario(async () => {
    await closePage();
    paths.clear();
    nodes.infos.clear();
  });

  Background(({ Given }) => {
    Given('a browser set to English', () => {});
  });

  Scenario(
    'The indicator shows all four states, names them for screen readers, and shows in phone chats',
    ({ Given, When, Then, And }) => {
      Given('an owned computer "workstation" that hosts the group "Website"', async () => {
        workstation = ownedNode('workstation', 'J3Uuh-y1z2AX536yAFlGHJL5sjxVpWVJavvVphvdm5Q');
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
        expect(el.getAttribute('aria-label')).toBe('Connection status: reconnecting');
        expect(all('.chat-head .conn')).toEqual([]);
      });
      And('the indicator links to "#/nodes"', () => {
        expect(one<HTMLAnchorElement>('.bar .conn').getAttribute('href')).toBe('#/nodes');
      });
      When('the path to "workstation" becomes direct with latency 18 ms', () => {
        paths.set(workstation.store, { kind: 'direct', rtt: 18, since: Date.now() });
        flushSync();
      });
      Then(
        'the indicator in the top bar has state "ok", reads "Direct 18 ms" and is named "Connection status: direct, 18 ms"',
        () => {
          const el = one<HTMLAnchorElement>('.bar .conn');
          expect(el.dataset.st).toBe('ok');
          expect(texts('.bar .conn')).toEqual(['Direct 18 ms']);
          expect(el.getAttribute('aria-label')).toBe('Connection status: direct, 18 ms');
        },
      );
      When('the path to "workstation" becomes relayed', () => {
        paths.set(workstation.store, { kind: 'relay', rtt: 210, since: Date.now() });
        flushSync();
      });
      Then('the indicator in the top bar has state "limited" and reads "Relayed · limited"', () => {
        const el = one<HTMLAnchorElement>('.bar .conn');
        expect(el.dataset.st).toBe('limited');
        expect(texts('.bar .conn')).toEqual(['Relayed · limited']);
        expect(el.getAttribute('aria-label')).toBe('Connection status: relayed · limited');
      });
      When('"workstation" stops', () => {
        workstation.drop();
        flushSync();
      });
      Then(
        'the indicator in the top bar has state "down" and reads "Cannot reach group home"',
        () => {
          const el = one<HTMLAnchorElement>('.bar .conn');
          expect(el.dataset.st).toBe('down');
          expect(texts('.bar .conn')).toEqual(['Cannot reach group home']);
          expect(el.getAttribute('aria-label')).toBe('Connection status: cannot reach group home');
        },
      );
      When('the browser opens "#/nodes"', () => go('#/nodes'));
      Then(
        'the indicator in the top bar has state "down" and reads "Cannot reach computers"',
        () => {
          const el = one<HTMLAnchorElement>('.bar .conn');
          expect(el.dataset.st).toBe('down');
          expect(texts('.bar .conn')).toEqual(['Cannot reach computers']);
          expect(el.getAttribute('aria-label')).toBe('Connection status: cannot reach computers');
        },
      );
      When('the browser opens "#/groups/rZNvy-1jH6Z-BcPqA5U5BQ" on a 390 px phone', async () => {
        await closePage();
        homes.push(workstation.store);
        openPage(`#/groups/${website}`, 390);
      });
      Then(
        'the indicator in the chat head has state "down" and reads "Cannot reach group home"',
        () => {
          const el = one<HTMLAnchorElement>('.chat-head .conn');
          expect(el.dataset.st).toBe('down');
          expect(texts('.chat-head .conn')).toEqual(['Cannot reach group home']);
          expect(all('.bar .conn')).toEqual([]);
        },
      );
    },
  );

  Scenario(
    'Two computers are listed with direct, relayed, limited and offline states, and setting an account type updates the card',
    ({ Given, And, When, Then }) => {
      Given(
        'an owned computer "workstation" on "macOS 15.2" running "v0.3.1" with direct path 4 ms and group "Website"',
        async () => {
          workstation = ownedNode('workstation', 'J3Uuh-y1z2AX536yAFlGHJL5sjxVpWVJavvVphvdm5Q', {
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
        'an owned computer "windows-box" on "Windows 11" running "v0.3.1" on a relayed path for 26 minutes with a limited AI in "Website"',
        async () => {
          windowsBox = ownedNode('windows-box', 'W9Uuh-y1z2AX536yAFlGHJL5sjxVpWVJavvVphvdm5Q', {
            os: 'Windows 11',
            version: 'v0.3.1',
            open: 1,
            max_open: 3,
            homes: [],
            agents: [
              {
                agent: 'codex',
                cli_version: '0.164.0',
                account: 'api_key',
                login_ok: true,
                driver: { accounts: ['api_key'], takeover: 'none' },
                models: [],
                caps: {},
                groups: [
                  {
                    g: website,
                    member: '1PC8WineBrUQ-apCjx7tug',
                    level: 2,
                    state: 'limited',
                    queued: 0,
                    watchers: [],
                  },
                ],
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
          expect(ai.querySelector('.acct')).toBeNull();
          expect(ai.querySelector('.perm')!.textContent!.replace(/\s+/g, ' ').trim()).toBe(
            'Website Working · T-12',
          );
        },
      );
      And(
        '"workstation" lists "Codex" as "Account type not set" and "Not signed in" with the three account choices',
        () => {
          const ai = all('.card.node')[0].querySelectorAll('.ai-row')[1];
          expect(ai.textContent).toContain('Codex');
          expect(ai.textContent).toContain('Account type not set');
          expect(ai.textContent).toContain('Not signed in');
          const choices = [...ai.querySelectorAll('.acct label b')].map((b) => b.textContent);
          expect(choices).toEqual(['Personal subscription', 'Team plan', 'API key']);
        },
      );
      When('the person chooses "Team plan" for "Codex" on "workstation"', async () => {
        const ai = all('.card.node')[0].querySelectorAll('.ai-row')[1];
        const radio = ai.querySelector<HTMLInputElement>('input[value="team"]')!;
        radio.checked = true;
        radio.dispatchEvent(new Event('change', { bubbles: true }));
        await vi.waitFor(() => {
          flushSync();
          expect(ai.querySelector('.badge')!.textContent!.trim()).toBe('Team plan');
        });
      });
      Then(
        '"workstation" receives a signed "agent.set" command setting "codex" to "team" and lists "Codex" as "Team plan"',
        () => {
          expect(workstation.commands).toHaveLength(1);
          const payload = JSON.parse(workstation.commands[0].p) as Record<string, unknown>;
          expect(payload).toMatchObject({
            t: 'agent.set',
            to: workstation.device,
            agent: 'codex',
            account: 'team',
          });
          const ai = all('.card.node')[0].querySelectorAll('.ai-row')[1];
          expect(ai.querySelector('.badge')!.textContent!.trim()).toBe('Team plan');
        },
      );
      And(
        'the page lists "windows-box" as "Online" with link "Relay · 210 ms · 26 min", relay notice "Using a relay — speed and data are limited." and "Limited" AI state',
        () => {
          const card = all('.card.node')[1];
          expect(card.querySelector('h3')!.textContent).toBe('windows-box');
          expect(card.querySelector('header .st')!.textContent!.trim()).toBe('Online');
          expect(card.querySelector('.kv dd')!.textContent!.replace(/\s+/g, ' ').trim()).toBe(
            'Relay · 210 ms · 26 min',
          );
          expect(card.querySelector('.note b')!.textContent!.trim()).toBe(
            'Using a relay — speed and data are limited.',
          );
          const permSt = card.querySelector('.perm .st')!;
          expect(permSt.getAttribute('data-st')).toBe('limited');
          expect(permSt.textContent!.trim()).toBe('Limited');
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
      And(
        'the indicator in the top bar has state "some" and reads "Some computers unreachable"',
        () => {
          const el = one<HTMLAnchorElement>('.bar .conn');
          expect(el.dataset.st).toBe('some');
          expect(texts('.bar .conn')).toEqual(['Some computers unreachable']);
          expect(el.getAttribute('aria-label')).toBe(
            'Connection status: some computers unreachable',
          );
        },
      );
    },
  );

  Scenario(
    'An account type is corrected, with its members in the groups',
    ({ Given, When, Then, And }) => {
      let noteWhileOpen: Element | null | undefined;
      const row = () => all('.card.node .ai-row')[0];
      const radios = () => [...row().querySelectorAll<HTMLInputElement>('.acct input')];
      const chosen = () => radios().find((radio) => radio.checked)?.value;
      const commands = () =>
        workstation.home.asked
          .filter((req) => req.t === 'cmd')
          .map((req) => JSON.parse((req.env as { p: string }).p) as Record<string, unknown>);
      const member = () => workstation.home.groups.get(website)!.rows.get('member claude-row')!;
      const pick = (value: string) => {
        const radio = radios().find((each) => each.value === value)!;
        radio.checked = true;
        radio.focus();
        radio.dispatchEvent(new Event('change', { bubbles: true }));
      };

      Given(
        'an owned computer "workstation" on "macOS 15.2" running "v0.3.1" with direct path 4 ms and group "Website"',
        async () => {
          workstation = ownedNode('workstation', 'J3Uuh-y1z2AX536yAFlGHJL5sjxVpWVJavvVphvdm5Q');
          workstation.home.add(website, 'Website');
          workstation.home.write(
            website,
            example('row-member-ai', {
              ...{ id: 'claude-row', g: website, cli: 'claude', account: 'personal' },
              host: workstation.device,
            }),
          );
          homes.push(workstation.store);
          await workstation.connect();
          paths.set(workstation.store, { kind: 'direct', rtt: 4, since: Date.now() });
          await vi.waitFor(() => expect(find(website)?.member('claude-row')).toBeDefined());
        },
      );
      When(
        'the browser opens "#/nodes" and the person opens "Account type" of "Claude Code"',
        async () => {
          openPage('#/nodes');
          await vi.waitFor(() => {
            flushSync();
            expect(row()).toBeDefined();
          });
          expect(row().querySelector('.acct')).toBeNull();
          const button = row().querySelector<HTMLButtonElement>('button[aria-expanded]')!;
          expect(button.getAttribute('aria-label')).toBe('Account type · Claude Code');
          button.click();
          flushSync();
          expect(button.getAttribute('aria-expanded')).toBe('true');
        },
      );
      Then('"Personal subscription" is checked among the three choices', () => {
        expect(radios().map((radio) => radio.value)).toEqual(['personal', 'team', 'api_key']);
        expect(chosen()).toBe('personal');
        expect(row().querySelector('.note')).toBeNull();
      });
      When(
        'the group home refuses member commands and the person chooses "API key" for "Claude Code"',
        () => {
          workstation.home.reply = () => {
            flushSync();
            noteWhileOpen = row().querySelector('.note');
            return { err: { code: 'forbidden', msg: '' } };
          };
          pick('api_key');
        },
      );
      Then(
        '"workstation" receives "agent.set" with "api_key" and "member set" of "Claude Code" with "api_key", and the choice stays',
        async () => {
          await vi.waitFor(() => {
            flushSync();
            expect(row().querySelector('.fail')).not.toBeNull();
          });
          expect(workstation.commands.map((env) => JSON.parse(env.p))).toMatchObject([
            { t: 'agent.set', agent: 'claude', account: 'api_key' },
          ]);
          expect(commands()).toMatchObject([
            { t: 'member', g: website, op: 'set', id: 'claude-row', account: 'api_key' },
          ]);
          expect(noteWhileOpen).toBeNull();
          expect(chosen()).toBe('api_key');
          expect((document.activeElement as HTMLInputElement).value).toBe('api_key');
          expect(row().querySelector('.badge')!.textContent!.trim()).toBe('API key');
        },
      );
      And('the card says "Not changed yet in Website." and why', () => {
        expect(row().querySelector('.note .grow')!.textContent).toBe('Not changed yet in Website.');
        expect(row().querySelector('.fail')!.textContent!.trim()).toBe(
          'Not sent: this device may not do that.',
        );
      });
      When('the group home takes them again and the person presses "Try again"', () => {
        workstation.home.reply = (payload) => {
          workstation.home.write(website, { ...member(), account: payload.account } as never);
          return { ok: true };
        };
        const again = row().querySelector<HTMLButtonElement>('.note button')!;
        expect(again.textContent!.trim()).toBe('Try again');
        again.click();
      });
      Then('the note is gone and the member in "Website" has "api_key"', async () => {
        await vi.waitFor(() => {
          flushSync();
          expect(row().querySelector('.fail, .note')).toBeNull();
          expect(radios()[0].closest('fieldset')!.disabled).toBe(false);
        });
        expect(member().account).toBe('api_key');
        expect(commands()).toHaveLength(2);
      });
      When(
        '"workstation" refuses "agent.set" and the person chooses "Team plan" for "Claude Code"',
        async () => {
          const ctl = workstation.store.ctl!;
          vi.spyOn(ctl, 'request').mockResolvedValueOnce({
            re: 1,
            err: { code: 'conflict', msg: '' },
          } as never);
          pick('team');
          await vi.waitFor(() => {
            flushSync();
            expect(row().querySelector('.fail code')!.textContent).toBe('conflict');
          });
        },
      );
      Then('"API key" is checked again and no member command is sent', () => {
        expect(chosen()).toBe('api_key');
        expect(row().querySelector('.badge')!.textContent!.trim()).toBe('API key');
        expect(commands()).toHaveLength(2);
      });
    },
  );

  Scenario(
    'Adding a computer opens onboarding and adding a browser requests a one-time pairing link',
    ({ Given, When, Then, And }) => {
      Given(
        'an owned computer "workstation" on "macOS 15.2" running "v0.3.1" with direct path 4 ms and group "Website"',
        async () => {
          workstation = ownedNode('workstation', 'J3Uuh-y1z2AX536yAFlGHJL5sjxVpWVJavvVphvdm5Q');
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
    'An empty computers page shows a single Add a computer action and updates when a computer connects',
    ({ When, Then }) => {
      When('the browser opens "#/nodes"', () => openPage('#/nodes'));
      Then(
        'the page shows the empty notice "No computers connected yet" and one "Add a computer" link to "#/onboarding"',
        () => {
          expect(texts('.empty h3')).toEqual(['No computers connected yet']);
          expect(all('.empty a')).toEqual([]);
          const links = all<HTMLAnchorElement>('main a').filter((a) =>
            a.textContent!.includes('Add a computer'),
          );
          expect(links).toHaveLength(1);
          expect(links[0].getAttribute('href')).toBe('#/onboarding');
        },
      );
      When('an owned computer "workstation" connects with 3 open AIs', async () => {
        workstation = ownedNode('workstation', 'J3Uuh-y1z2AX536yAFlGHJL5sjxVpWVJavvVphvdm5Q', {
          open: 3,
          max_open: 4,
        });
        homes.push(workstation.store);
        await workstation.connect();
        paths.set(workstation.store, { kind: 'direct', rtt: 6, since: Date.now() });
        await vi.waitFor(() => {
          flushSync();
          expect(all('.card.node .sub')).toHaveLength(1);
        });
      });
      Then('the page lists "workstation" with "3 of 4 AIs open"', () => {
        expect(one('.card.node .sub').textContent!.replace(/\s+/g, ' ').trim()).toBe(
          'macOS 15.2 · Node v0.3.1 · 3 of 4 AIs open',
        );
      });
    },
  );
});
