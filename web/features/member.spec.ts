import { describeFeature, loadFeature } from '@amiceli/vitest-cucumber';
import { flushSync, tick } from 'svelte';
import { SvelteMap } from 'svelte/reactivity';
import { expect, vi } from 'vitest';
import { Home, homes } from '../src/lib/groups.svelte';
import { goldens } from '../src/lib/mock';
import type { Path, Source } from '../src/lib/net.svelte';
import { nodes } from '../src/lib/nodes.svelte';
import * as p from '../src/lib/proto';
import { example, owner, TestHome, user } from './home';
import { all, closePage, fill, one, openPage, texts } from './page';

const feature = await loadFeature('../features/web_member.feature');

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

const website = 'rZNvy-1jH6Z-BcPqA5U5BQ';
const workstationId = 'J3Uuh-y1z2AX536yAFlGHJL5sjxVpWVJavvVphvdm5Q';
const laptopId = 'bGFwdG9wLWRldmljZS1pZA';
const sign = async (payload: string) => ({ p: payload, k: 'this-browser', s: 'signature' });

interface HostNode {
  home: TestHome;
  store: Home;
  device: string;
  agents: p.HostInfo['agents'];
  joins: p.HostJoin[];
  refuseJoin?: string;
  connect(): Promise<void>;
  drop(): void;
}

function makeNode(
  name: string,
  device: string,
  agents: p.HostInfo['agents'],
  groupHome?: TestHome,
): HostNode {
  const home = new TestHome(name);
  const store = new Home({ user, sign });
  const joins: p.HostJoin[] = [];
  let drop = () => {};
  const node: HostNode = {
    home,
    store,
    device,
    agents,
    joins,
    async connect() {
      const link = store.connect();
      const connection = home.connect((frame) => link.push(frame));
      drop = () => {
        connection.close();
        link.offline();
      };
      let seq = 100;
      function writeJoinedAI(join: p.HostJoin) {
        const target = groupHome ?? home;
        const memberId = `ai-${joins.length}`;
        target.write(
          join.g,
          example('row-member-ai', {
            id: memberId,
            g: join.g,
            name: join.name,
            cli: join.agent,
            host: node.device,
            host_name: name,
            secretary: join.secretary,
            state: 'waiting',
            task: undefined,
            path: target === home ? 'local' : 'direct',
          }),
        );
        if (join.secretary) {
          target.write(
            join.g,
            example('row-group', { id: join.g, name: 'Website', secretary: memberId }),
          );
        }
      }
      async function handleJoinCmd(env: p.Envelope) {
        await new Promise((later) => setTimeout(later));
        expect(env.k).toBe('this-browser');
        const join = p.decode(p.HostJoin, env.p);
        joins.push(join);
        if (node.refuseJoin) {
          return { re: (seq += 1), err: { code: node.refuseJoin, msg: 'refused' } };
        }
        writeJoinedAI(join);
        return { re: (seq += 1), ok: true as const };
      }
      const ctl: p.Ctl = {
        request: (async (req: { t: string; env?: p.Envelope }) => {
          if (req.t === 'host.info') {
            await new Promise((later) => setTimeout(later));
            const base = p.decode(p.HostInfo, goldens['host-info-answer']);
            return { ...base, re: (seq += 1), device: node.device, name, agents: node.agents };
          }
          if (
            req.t === 'cmd' &&
            req.env &&
            (JSON.parse(req.env.p) as { t?: string }).t === 'host.join'
          ) {
            return handleJoinCmd(req.env);
          }
          return connection.ctl.request(req as Parameters<p.Ctl['request']>[0]);
        }) as p.Ctl['request'],
      };
      const welcome = (await ctl.request({
        t: 'hello',
        v: 1,
        kind: 'browser',
        name: 'test',
        addrs: [],
      })) as p.Welcome;
      link.online(ctl, { ...welcome, device: node.device });
    },
    drop: () => drop(),
  };
  return node;
}

describeFeature(feature, ({ Background, Scenario, AfterEachScenario }) => {
  let workstation: HostNode;
  let laptop: HostNode;

  async function syncNode(node: HostNode) {
    const expected = node.home.synced + node.home.groups.size;
    await node.connect();
    await vi.waitFor(() => expect(node.home.synced).toBe(expected));
    await new Promise((applied) => setTimeout(applied));
    flushSync();
  }

  async function addLaptop() {
    laptop = makeNode(
      'laptop',
      laptopId,
      [
        {
          agent: 'codex',
          cli_version: '0.164.0',
          account: 'personal',
          login_ok: true,
          driver: { accounts: ['api_key'], takeover: 'none' },
          caps: {},
          groups: [],
        },
        {
          agent: 'claude',
          cli_version: '2.1.311',
          account: 'api_key',
          login_ok: false,
          driver: { accounts: ['api_key'], takeover: 'none' },
          caps: {},
          groups: [],
        },
      ],
      workstation.home,
    );
    homes.push(laptop.store);
    await syncNode(laptop);
  }

  async function submitLaptopRefusal(code: string) {
    laptop.refuseJoin = code;
    one<HTMLButtonElement>('.members .head button').click();
    flushSync();
    await vi.waitFor(() => expect(all('dialog .choice')).toHaveLength(4));
    flushSync();
    one('dialog form').dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }));
    await vi.waitFor(() => expect(laptop.joins).toHaveLength(1));
    flushSync();
  }

  AfterEachScenario(async () => {
    await closePage();
    paths.clear();
    nodes.infos.clear();
  });

  Background(({ Given }) => {
    Given(
      'a group "Website" homed on "workstation", open in a browser of its owner in English',
      async () => {
        workstation = makeNode('workstation', workstationId, [
          {
            agent: 'claude',
            cli_version: '2.1.311',
            account: 'personal',
            login_ok: true,
            driver: { accounts: ['api_key'], takeover: 'none' },
            caps: {},
            groups: [
              {
                g: website,
                member: '1PC8WineBrUQ-apCjx7tug',
                level: 2,
                state: 'working',
                queued: 0,
              },
            ],
          },
          {
            agent: 'codex',
            cli_version: '0.164.0',
            account: null,
            login_ok: false,
            driver: { accounts: ['api_key'], takeover: 'none' },
            caps: {},
            groups: [],
          },
        ]);
        workstation.home.add(website, 'Website');
        workstation.home.write(
          website,
          example('row-member-ai', {
            id: 'claude-ws',
            g: website,
            name: 'Claude Code',
            cli: 'claude',
            host: workstationId,
            host_name: 'workstation',
            state: 'working',
            task: undefined,
            path: 'local',
          }),
        );
        homes.push(workstation.store);
        await syncNode(workstation);
        paths.set(workstation.store, { kind: 'direct', rtt: 8, since: Date.now() });
        openPage(`#/groups/${website}`, 1440);
        expect(texts('.chat-head h2')).toEqual(['Website']);
      },
    );
  });

  Scenario(
    'AI member cards show secretary, working, waiting, sleeping, limited and offline states',
    ({ Given, When, Then, And }) => {
      Given(
        "the group has a secretary, a working AI on T-12, a waiting AI on T-13, Mina's AI on T-11, and a dormant AI",
        () => {
          const h = workstation.home;
          h.write(
            website,
            example('row-member-human', {
              id: 'mina',
              g: website,
              user: 'mina-user',
              name: 'Mina',
              role: 'member',
              online: true,
            }),
          );
          h.write(
            website,
            example('row-task', { id: 't12', g: website, num: 12, title: 'Fix review remarks' }),
          );
          h.write(
            website,
            example('row-task-waiting', {
              id: 't13',
              g: website,
              num: 13,
              title: 'Install deps',
              wait: { kind: 'approval', who: [user], since: Date.now() },
            }),
          );
          h.write(
            website,
            example('row-task-waiting', {
              id: 't11',
              g: website,
              num: 11,
              title: 'Update docs',
              wait: { kind: 'approval', who: ['mina-user'], since: Date.now() },
            }),
          );
          h.write(
            website,
            example('row-msg-brief-pending', {
              id: 'brief-wait',
              by: 'sec-ai',
              body: {
                ...(example('row-msg-brief-pending').body as object),
                requester: owner,
              },
            }),
          );
          h.write(
            website,
            example('row-msg-decision-pending', {
              id: 'dec-wait',
              by: 'sec-ai',
              thread: null,
              body: { question: 'Which port?', options: [], to: [user] },
            }),
          );
          const resetAt = new Date(2026, 10, 20, 15, 0).getTime();
          h.write(
            website,
            example('row-member-ai', {
              id: 'sec-ai',
              g: website,
              name: 'Secretary',
              cli: 'claude',
              secretary: true,
              state: 'waiting',
              task: undefined,
              host: workstationId,
              host_name: 'workstation',
              path: 'local',
              account: 'personal',
              usage: { window_pct: 18 },
            }),
          );
          h.write(
            website,
            example('row-member-ai', {
              id: 'claude-ws',
              g: website,
              name: 'Claude Code',
              cli: 'claude',
              secretary: false,
              state: 'working',
              task: 't12',
              host: laptopId,
              host_name: 'laptop',
              path: 'direct',
              account: 'team',
              usage: { window_pct: 62, window_resets: resetAt },
            }),
          );
          h.write(
            website,
            example('row-member-ai', {
              id: 'codex-win',
              g: website,
              name: 'Codex',
              cli: 'codex',
              secretary: false,
              state: 'waiting',
              task: 't13',
              host: 'win-id',
              host_name: 'windows',
              path: 'relay',
              account: 'personal',
              usage: {},
            }),
          );
          h.write(
            website,
            example('row-member-ai', {
              id: 'mina-ai',
              g: website,
              user: 'mina-user',
              name: 'Mina Claude',
              cli: 'claude',
              secretary: false,
              state: 'waiting',
              task: 't11',
              host: 'mina-id',
              host_name: 'laptop',
              path: 'direct',
              account: 'team',
              usage: { window_pct: 100 },
            }),
          );
          h.write(
            website,
            example('row-member-ai', {
              id: 'dormant-ai',
              g: website,
              name: 'Night Codex',
              cli: 'codex',
              secretary: false,
              state: 'dormant',
              task: undefined,
              host: 'linux-id',
              host_name: 'old-box',
              path: 'direct',
              account: 'api_key',
              usage: { turns_today: 3 },
            }),
          );
          flushSync();
        },
      );
      Then(
        'the AI section header reads "AI · 5" and places the secretary first with the secretary mark',
        () => {
          expect(texts('.members h2')[0]).toBe('AI · 5');
          expect(texts('.member h3')).toEqual([
            'Secretary',
            'Claude Code',
            'Codex',
            'Mina Claude · Mina',
            'Night Codex',
          ]);
          expect(all('.member .av-wrap').map((el) => el.classList.contains('sec'))).toEqual([
            true,
            false,
            false,
            false,
            false,
          ]);
        },
      );
      And(
        'the cards show "Waiting · 2 items waiting on you", "Working · T-12 Fix review remarks", "Waiting · T-13 waiting for your approval", "Waiting · T-11 waiting for Mina\'s approval" and "Sleeping · wakes when @mentioned"',
        () => {
          expect(texts('.member p.st')).toEqual([
            'Waiting · 2 items waiting on you',
            'Working · T-12 Fix review remarks',
            'Waiting · T-13 waiting for your approval',
            "Waiting · T-11 waiting for Mina's approval",
            'Sleeping · wakes when @mentioned',
          ]);
          expect(all('.member p.st').map((el) => el.dataset.st)).toEqual([
            'needyou',
            'working',
            'needyou',
            'waiting',
            'sleeping',
          ]);
        },
      );
      And(
        'the computers read "workstation · group home", "laptop · direct" and "windows · relayed, limited"',
        () => {
          const computers = all('.member .kv').map((dl) =>
            dl.querySelectorAll('dd')[0].textContent!.replace(/\s+/g, ' ').trim(),
          );
          expect(computers).toEqual([
            'workstation · group home',
            'laptop · direct',
            'windows · relayed, limited',
            'laptop · direct',
            'old-box · direct',
          ]);
        },
      );
      And(
        'the accounts read "Personal subscription · takes orders from owner", "Team plan" and "API key"',
        () => {
          const accounts = all('.member .kv').map((dl) =>
            dl.querySelectorAll('dd')[1].textContent!.replace(/\s+/g, ' ').trim(),
          );
          expect(accounts).toEqual([
            'Personal subscription · takes orders from owner',
            'Team plan',
            'Personal subscription · takes orders from owner',
            'Team plan',
            'API key',
          ]);
        },
      );
      And(
        'owned AI cards link to "Open terminal" when direct and disable "Open terminal" when relayed or offline, while Mina\'s AI shows none',
        async () => {
          const winBox = makeNode('windows', 'win-id', [], workstation.home);
          homes.push(winBox.store);
          await syncNode(winBox);
          paths.set(winBox.store, { kind: 'relay', rtt: 120, since: Date.now() });
          flushSync();

          const cards = all('.member');
          expect(cards[0].querySelector('a')!.getAttribute('href')).toBe(
            `#/terminal/${website}/sec-ai`,
          );
          expect(cards[1].querySelector<HTMLButtonElement>('button')!.disabled).toBe(true);
          expect(cards[1].querySelector('.card-foot .hint')!.textContent).toBe('Computer offline');
          expect(cards[2].querySelector<HTMLButtonElement>('button')!.disabled).toBe(true);
          expect(cards[2].querySelector('.card-foot .hint')!.textContent).toBe('Direct link only');
          expect(cards[3].querySelector('.card-foot')).toBeNull();
          expect(cards[4].querySelector<HTMLButtonElement>('button')!.disabled).toBe(true);
        },
      );
      And('the cards show a window usage bar where reported', () => {
        expect(all('.member').map((card) => Boolean(card.querySelector('.meter')))).toEqual([
          true,
          true,
          false,
          true,
          false,
        ]);
        expect(texts('.member .meter span')).toEqual(['Window used', 'Window used', 'Window used']);
        expect(texts('.member .meter b')).toEqual(['18%', '62%', '100%']);
        expect(all('.member .meter').map((m) => m.getAttribute('style'))).toEqual([
          '--v: 18%;',
          '--v: 62%;',
          '--v: 100%;',
        ]);
        expect(all('.member .meter').map((m) => m.dataset.tone)).toEqual([
          undefined,
          undefined,
          'danger',
        ]);
        expect(texts('.member .meter small')).toEqual(['Resets 15:00']);
      });
      When('the dormant AI wakes to work and two other AIs go offline and hit a limit', () => {
        const h = workstation.home;
        h.write(
          website,
          example('row-member-ai', {
            id: 'claude-ws',
            g: website,
            name: 'Claude Code',
            cli: 'claude',
            state: 'limited',
            task: undefined,
          }),
        );
        h.write(
          website,
          example('row-member-ai', {
            id: 'codex-win',
            g: website,
            name: 'Codex',
            cli: 'codex',
            state: 'offline',
            task: undefined,
            host_name: 'windows',
            path: 'relay',
          }),
        );
        h.write(
          website,
          example('row-member-ai', {
            id: 'dormant-ai',
            g: website,
            name: 'Night Codex',
            cli: 'codex',
            state: 'working',
            task: undefined,
          }),
        );
        flushSync();
      });
      Then('those cards update to "Working", "Offline" with "offline" and "Limited"', () => {
        expect(texts('.member p.st')).toEqual([
          'Waiting · 2 items waiting on you',
          'Limited',
          'Offline',
          "Waiting · T-11 waiting for Mina's approval",
          'Working',
        ]);
        expect(all('.member p.st').map((el) => el.dataset.st)).toEqual([
          'needyou',
          'limited',
          'offline',
          'waiting',
          'working',
        ]);
        const codexComputer = all('.member .kv')[2]
          .querySelectorAll('dd')[0]
          .textContent!.replace(/\s+/g, ' ')
          .trim();
        expect(codexComputer).toBe('windows · offline');
      });
    },
  );

  Scenario('The people list shows names, roles and online status', ({ Given, Then, And }) => {
    Given('the group also has "Mina" as an online admin and "Sam" as an offline member', () => {
      workstation.home.write(
        website,
        example('row-member-human', {
          id: 'mina',
          g: website,
          user: 'mina-user',
          name: 'Mina',
          role: 'admin',
          online: true,
        }),
      );
      workstation.home.write(
        website,
        example('row-member-human', {
          id: 'sam',
          g: website,
          user: 'sam-user',
          name: 'Sam',
          role: 'member',
          online: false,
        }),
      );
      flushSync();
    });
    Then('the people section reads "People · 3"', () => {
      expect(texts('.members h2')[1]).toBe('People · 3');
    });
    And(
      'the people list shows "Alex · You · Owner" as "Online", "Mina · Admin" as "Online", and "Sam" as "Offline"',
      () => {
        expect(texts('.people .person > .grow')).toEqual([
          'Alex · You · Owner',
          'Mina · Admin',
          'Sam',
        ]);
        expect(texts('.people .person .st')).toEqual(['Online', 'Online', 'Offline']);
        expect(all('.people .person .st').map((el) => el.dataset.st)).toEqual([
          'ok',
          'ok',
          'offline',
        ]);
      },
    );
  });

  Scenario(
    'Below 1024 px the member panel opens and closes from the chat header',
    ({ Given, When, Then }) => {
      Given('the chat is reopened on a screen 820 px wide', async () => {
        await closePage();
        workstation = makeNode('workstation', workstationId, [
          {
            agent: 'codex',
            cli_version: '0.164.0',
            account: 'personal',
            login_ok: true,
            driver: { accounts: ['api_key'], takeover: 'none' },
            caps: {},
            groups: [],
          },
        ]);
        workstation.home.add(website, 'Website');
        homes.push(workstation.store);
        await syncNode(workstation);
        openPage(`#/groups/${website}`, 820);
      });
      When('the owner opens "People and details"', async () => {
        one<HTMLButtonElement>('.side-open').click();
        await tick();
        flushSync();
      });
      Then('the side panel is open with the member list and the chat behind it is inert', () => {
        expect(one('#chat-side').classList.contains('open')).toBe(true);
        expect(one('.scroller').hasAttribute('inert')).toBe(true);
        expect(texts('#chat-side .people .person > .grow')).toEqual(['Alex · You · Owner']);
      });
      When('the owner opens "Add AI" and presses Escape in the dialog', async () => {
        one<HTMLButtonElement>('.members .head button').click();
        flushSync();
        const dlg = one<HTMLDialogElement>('.members ~ dialog, #chat-side dialog');
        expect(dlg.open).toBe(true);
        const input = dlg.querySelector('input[type=text], input:not([type])')!;
        input.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }));
        dlg.close();
        await tick();
        flushSync();
      });
      Then('the dialog closes and the side panel stays open', () => {
        expect(all('#chat-side dialog')).toEqual([]);
        expect(one('#chat-side').classList.contains('open')).toBe(true);
      });
      When('the owner presses "Close the panel"', async () => {
        one<HTMLButtonElement>('.side-close').click();
        await tick();
        flushSync();
      });
      Then('the side panel closes and focus returns to "People and details"', () => {
        expect(one('#chat-side').classList.contains('open')).toBe(false);
        expect(document.activeElement).toBe(one('.side-open'));
      });
    },
  );

  Scenario(
    'Adding an AI reads connected computers, suggests a free name and sends host.join',
    ({ Given, When, Then, And }) => {
      Given(
        'the group has no secretary yet and a second computer "laptop" is connected',
        async () => {
          const groupWithoutSec = { ...example('row-group', { id: website, name: 'Website' }) };
          delete (groupWithoutSec as { secretary?: string }).secretary;
          workstation.home.write(website, groupWithoutSec);
          await addLaptop();
        },
      );
      When('the owner opens "Add AI"', async () => {
        one<HTMLButtonElement>('.members .head button').click();
        flushSync();
        expect(one('#chat-side dialog .hint').getAttribute('role')).toBe('status');
        expect(texts('#chat-side dialog .hint')).toEqual(['Reading your computers…']);
        await vi.waitFor(() => expect(all('dialog .choice')).toHaveLength(4));
        flushSync();
      });
      Then(
        '"Claude Code · workstation" is disabled as "Already in this group" and "Codex · workstation" is disabled as "Choose an account type on the Computers page first"',
        () => {
          const labels = texts('dialog .choice b');
          const subs = texts('dialog .choice .sub');
          const radios = all<HTMLInputElement>('dialog .choice input');
          expect(labels).toEqual([
            'Claude Code · workstation',
            'Codex · workstation',
            'Codex · laptop',
            'Claude Code · laptop',
          ]);
          expect(subs[0]).toBe('Already in this group');
          expect(subs[1]).toBe('Choose an account type on the Computers page first');
          expect(radios.map((r) => r.disabled)).toEqual([true, true, false, false]);
        },
      );
      And(
        '"Codex · laptop" is chosen with name "Codex" and "Claude Code · laptop" warns "Not signed in" and suggests "Claude Code 2"',
        async () => {
          const radios = all<HTMLInputElement>('dialog .choice input');
          const nameInput = one<HTMLInputElement>('dialog input.input');
          expect(radios.map((r) => r.checked)).toEqual([false, false, true, false]);
          expect(texts('dialog .choice .sub')[3]).toBe('API key · Not signed in');
          expect(nameInput.value).toBe('Codex');
          radios[3].click();
          flushSync();
          expect(nameInput.value).toBe('Claude Code 2');
          radios[2].click();
          flushSync();
          expect(nameInput.value).toBe('Codex');

          // Reopening shows cached candidates immediately while refreshing host.info in the background.
          one<HTMLButtonElement>('dialog .btn-ghost').click();
          await tick();
          flushSync();
          laptop.agents = [laptop.agents[0], { ...laptop.agents[1], login_ok: true }];
          one<HTMLButtonElement>('.members .head button').click();
          flushSync();
          expect(all('dialog .choice')).toHaveLength(4);
          expect(texts('dialog .choice .sub')[3]).toBe('API key · Not signed in');
          await vi.waitFor(() => {
            flushSync();
            expect(texts('dialog .choice .sub')[3]).toBe('API key');
          });
          expect(one<HTMLInputElement>('dialog input.input').value).toBe('Codex');
        },
      );
      When(
        'the owner toggles "Make this AI the group\'s secretary" on and off, then checks it and names the AI "Lead Secretary"',
        () => {
          const secCheck = one<HTMLInputElement>('dialog .check input');
          const nameInput = one<HTMLInputElement>('dialog input.input');
          secCheck.click();
          flushSync();
          expect(nameInput.value).toBe('Secretary');
          secCheck.click();
          flushSync();
          expect(nameInput.value).toBe('Codex');
          secCheck.click();
          flushSync();
          expect(nameInput.value).toBe('Secretary');
          fill(nameInput, '  Lead Secretary  ');
          flushSync();
        },
      );
      And('the owner submits the add-AI dialog', async () => {
        one('dialog form').dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }));
        await vi.waitFor(() => expect(laptop.joins).toHaveLength(1));
        await new Promise((later) => setTimeout(later, 5));
        flushSync();
      });
      Then(
        '"laptop" receives a signed "host.join" for "codex" named "Lead Secretary" with secretary true',
        () => {
          expect(laptop.joins[0]).toEqual({
            t: 'host.join',
            n: expect.stringMatching(/^[A-Za-z0-9_-]{21}[AQgw]$/),
            ts: expect.any(Number),
            to: laptopId,
            g: website,
            home: workstationId,
            agent: 'codex',
            name: 'Lead Secretary',
            secretary: true,
          });
          expect(workstation.joins).toEqual([]);
        },
      );
      And('the dialog closes and the new AI card "Lead Secretary" appears in the panel', () => {
        expect(all('#chat-side dialog')).toEqual([]);
        expect(texts('.member h3')).toEqual(['Lead Secretary', 'Claude Code']);
        expect(texts('.members h2')[0]).toBe('AI · 2');
      });
    },
  );

  Scenario('A name conflict when adding an AI is reported clearly', ({ Given, When, Then }) => {
    Given('a second computer "laptop" is connected', addLaptop);
    When(
      '"laptop" refuses "host.join" with "conflict" and the owner submits "Codex · laptop"',
      () => submitLaptopRefusal('conflict'),
    );
    Then('the dialog stays open and says "That name is already used in this group."', () => {
      expect(one<HTMLDialogElement>('#chat-side dialog').open).toBe(true);
      expect(texts('dialog [role=alert]')).toEqual(['That name is already used in this group.']);
    });
  });

  Scenario(
    "An unreachable group home from the AI's computer is reported clearly",
    ({ Given, When, Then }) => {
      Given('a second computer "laptop" is connected', addLaptop);
      When(
        '"laptop" refuses "host.join" with "home_unreachable" and the owner submits "Codex · laptop"',
        () => submitLaptopRefusal('home_unreachable'),
      );
      Then(
        'the dialog stays open and says "The AI\'s computer cannot reach this group\'s home right now. Try again when both are online."',
        () => {
          expect(one<HTMLDialogElement>('#chat-side dialog').open).toBe(true);
          expect(texts('dialog [role=alert]')).toEqual([
            "The AI's computer cannot reach this group's home right now. Try again when both are online.",
          ]);
        },
      );
    },
  );

  Scenario('A lost AI computer and disabled Add AI states explain why', ({ Given, When, Then }) => {
    Given('a second computer "laptop" is connected', addLaptop);
    When('the owner opens "Add AI" and "laptop" disconnects before the owner submits', async () => {
      one<HTMLButtonElement>('.members .head button').click();
      flushSync();
      await vi.waitFor(() => expect(all('dialog .choice')).toHaveLength(4));
      laptop.drop();
      flushSync();
      one('dialog form').dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }));
      await new Promise((later) => setTimeout(later, 5));
      flushSync();
    });
    Then(
      'no command is sent to "workstation" and the dialog says "Not sent: the AI\'s computer cannot be reached."',
      () => {
        expect(workstation.joins).toEqual([]);
        expect(laptop.joins).toEqual([]);
        expect(texts('dialog [role=alert]')).toEqual([
          "Not sent: the AI's computer cannot be reached.",
        ]);
      },
    );
    When('the owner closes the dialog and becomes a non-admin member', () => {
      one<HTMLButtonElement>('dialog .btn-ghost').click();
      workstation.home.write(
        website,
        example('row-member-human', {
          id: owner,
          g: website,
          user,
          name: 'Alex',
          role: 'member',
          online: true,
        }),
      );
      flushSync();
    });
    Then('the "Add AI" button is disabled with reason "Only an admin can add an AI."', () => {
      expect(one<HTMLButtonElement>('.members .head button').disabled).toBe(true);
      expect(texts('.members .add-why')).toEqual(['Only an admin can add an AI.']);
    });
    When('the group home disconnects', () => {
      workstation.drop();
      flushSync();
    });
    Then('the "Add AI" button is disabled with reason "Cannot reach the group home."', () => {
      expect(one<HTMLButtonElement>('.members .head button').disabled).toBe(true);
      expect(texts('.members .add-why')).toEqual(['Cannot reach the group home.']);
    });
  });
});
