import { describeFeature, loadFeature } from '@amiceli/vitest-cucumber';
import { flushSync, tick } from 'svelte';
import { SvelteMap } from 'svelte/reactivity';
import { expect, vi } from 'vitest';
import { Home, homes } from '../src/lib/groups.svelte';
import { goldens } from '../src/lib/mock';
import type { Path, Source } from '../src/lib/net.svelte';
import type { Refusal } from '../src/lib/members';
import { nodes } from '../src/lib/nodes.svelte';
import * as p from '../src/lib/proto';
import { example, owner, TestHome, user } from './home';
import { all, closePage, fill, go, one, read, openPage, texts } from './page';

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
  /** The groups whose home the node knows, when not those of `host-info-answer`. */
  homes?: p.HostInfo['homes'];
  joins: p.HostJoin[];
  refuseJoin?: Refusal;
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
        const memberId = `ai-${name}-${joins.length}`;
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
            example('row-group', {
              id: join.g,
              name: target.groups.get(join.g)!.name,
              secretary: memberId,
            }),
          );
        }
      }
      async function handleJoinCmd(env: p.Envelope) {
        await new Promise((later) => setTimeout(later));
        expect(env.k).toBe('this-browser');
        const join = p.decode(p.HostJoin, env.p);
        joins.push(join);
        if (node.refuseJoin) {
          return { re: (seq += 1), err: node.refuseJoin };
        }
        writeJoinedAI(join);
        return { re: (seq += 1), ok: true as const };
      }
      async function hostInfo() {
        await new Promise((later) => setTimeout(later));
        const base = p.decode(p.HostInfo, goldens['host-info-answer']);
        const { device, agents, homes = base.homes } = node;
        return { ...base, re: (seq += 1), device, name, agents, homes };
      }
      const ctl: p.Ctl = {
        request: (async (req: { t: string; env?: p.Envelope }) => {
          if (req.t === 'host.info') return hostInfo();
          if (req.t === 'host.workspaces')
            return p.decodeAnswer('host.workspaces', goldens['host-workspaces-answer']);
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
          driver: { accounts: ['personal', 'team', 'api_key'], takeover: 'new' },
          models: [],
          caps: {},
          groups: [],
        },
        {
          agent: 'claude',
          cli_version: '2.1.311',
          account: 'api_key',
          login_ok: false,
          driver: { accounts: ['personal', 'team', 'api_key'], takeover: 'new' },
          models: [],
          caps: {},
          groups: [],
        },
      ],
      workstation.home,
    );
    homes.push(laptop.store);
    await syncNode(laptop);
  }

  /** The folder list of the chosen AI has been read, and its newest folder taken. */
  const folderTaken = () =>
    vi.waitFor(() => expect(one('dialog .path-row code').textContent).toContain('/home/dev/'));

  /** Opens "Add AI" and chooses Codex on the laptop, with its folder taken. */
  async function chooseLaptopCodex() {
    one<HTMLButtonElement>('.members .head button').click();
    flushSync();
    await vi.waitFor(() => expect(all('dialog .choice')).toHaveLength(4));
    flushSync();
    all<HTMLInputElement>('dialog .choice input')[2].click();
    flushSync();
    await folderTaken();
  }
  const submitDialog = () =>
    one('dialog form').dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }));

  async function submitLaptopRefusal(code: string, msg = 'refused') {
    laptop.refuseJoin = { code, msg };
    await chooseLaptopCodex();
    submitDialog();
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
            driver: { accounts: ['personal', 'team', 'api_key'], takeover: 'new' },
            models: [],
            caps: {},
            groups: [
              {
                g: website,
                member: '1PC8WineBrUQ-apCjx7tug',
                workspace: '/home/dev/website',
                level: 2,
                state: 'working',
                queued: 0,
                watchers: [],
              },
            ],
          },
          {
            agent: 'codex',
            cli_version: '0.164.0',
            account: null,
            login_ok: false,
            driver: { accounts: ['personal', 'team', 'api_key'], takeover: 'new' },
            models: [],
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
            dl.querySelectorAll('dd')[2].textContent!.replace(/\s+/g, ' ').trim(),
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
          const winBox = makeNode(
            'windows',
            'win-id',
            [
              {
                agent: 'codex',
                cli_version: '0.164.0',
                account: 'team',
                login_ok: true,
                driver: { accounts: ['personal', 'team', 'api_key'], takeover: 'resume' },
                models: [],
                caps: {},
                groups: [],
              },
            ],
            workstation.home,
          );
          homes.push(winBox.store);
          await syncNode(winBox);
          paths.set(winBox.store, { kind: 'relay', rtt: 120, since: Date.now() });
          flushSync();

          const cards = all('.member');
          expect(cards[0].querySelector('a')!.getAttribute('href')).toBe(
            `#/terminal/${website}/sec-ai`,
          );
          expect(cards[1].querySelector<HTMLButtonElement>('.card-foot button')!.disabled).toBe(
            true,
          );
          expect(cards[1].querySelector('.card-foot .hint')!.textContent).toBe('Computer offline');
          expect(cards[2].querySelector<HTMLButtonElement>('.card-foot button')!.disabled).toBe(
            true,
          );
          expect(cards[2].querySelector('.card-foot .hint')!.textContent).toBe('Direct link only');
          expect(read(cards[3].querySelector('.card-foot')!)).toBe('Remove AI');
          expect(cards[4].querySelector<HTMLButtonElement>('.card-foot button')!.disabled).toBe(
            true,
          );
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

  Scenario(
    'The people list shows names and roles, and nothing about being online',
    ({ Given, Then, And }) => {
      Given('the group also has "Mina" as an admin and "Sam" as a member', () => {
        workstation.home.write(
          website,
          example('row-member-human', {
            id: 'mina',
            g: website,
            user: 'mina-user',
            name: 'Mina',
            role: 'admin',
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
          }),
        );
        flushSync();
      });
      Then('the people section reads "People · 3"', () => {
        expect(texts('.members h2')[1]).toBe('People · 3');
      });
      And(
        'the people list shows "Alex · You · Owner", "Mina · Admin" and "Sam" with no state',
        () => {
          expect(texts('.people .person > .grow')).toEqual([
            'Alex · You · Owner',
            'Mina · Admin',
            'Sam',
          ]);
          expect(all('.people .st')).toEqual([]);
        },
      );
    },
  );

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
            driver: { accounts: ['personal', 'team', 'api_key'], takeover: 'new' },
            models: [],
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
        nodes.infos.clear();
        one<HTMLButtonElement>('.members .head button').click();
        flushSync();
        expect(one('#chat-side dialog .hint').getAttribute('role')).toBe('status');
        expect(texts('#chat-side dialog .hint')).toEqual(['Reading your computers…']);
        await vi.waitFor(() => expect(all('dialog .choice')).toHaveLength(4));
        flushSync();
      });
      Then(
        '"Claude Code · workstation" says "In this group · adds another member" and is selectable again and "Codex · workstation" is disabled as "Choose an account type on the Computers page first"',
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
          expect(subs[0]).toBe('In this group · adds another member');
          expect(subs[1]).toBe('Choose an account type on the Computers page first');
          expect(radios.map((r) => r.disabled)).toEqual([false, true, false, false]);
        },
      );
      And(
        '"Claude Code · workstation" is initially chosen with name "Claude Code 2" and the owner chooses "Codex · laptop"',
        async () => {
          const radios = all<HTMLInputElement>('dialog .choice input');
          const nameInput = one<HTMLInputElement>('dialog input[required]');
          expect(radios.map((r) => r.checked)).toEqual([true, false, false, false]);
          expect(nameInput.value).toBe('Claude Code 2');
          radios[2].click();
          flushSync();
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
          expect(one<HTMLInputElement>('dialog input[required]').value).toBe('Claude Code 2');
          all<HTMLInputElement>('dialog .choice input')[2].click();
          flushSync();
          expect(one<HTMLInputElement>('dialog input[required]').value).toBe('Codex');
        },
      );
      When(
        'the owner toggles "Make this AI the group\'s secretary" on and off, then checks it and names the AI "Lead Secretary"',
        () => {
          const secCheck = one<HTMLInputElement>('dialog .check input');
          const nameInput = one<HTMLInputElement>('dialog input[required]');
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
            workspace: '/home/dev/website',
            model: '',
            effort: '',
            tier: 'standard',
            skills: '',
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

  const docs = 'ZG9jcy1ncm91cC0wMDAwMQ';
  /** An entry of `host.info` for an AI of group `g` in `workspace`. */
  const working = (g: string, member: string, workspace: string) => ({
    g,
    member,
    workspace,
    level: 2,
    state: 'waiting' as const,
    queued: 0,
    watchers: [],
  });
  const folderRows = () =>
    all('dialog dialog .pick-row').map((row) => ({
      path: read(row.querySelector('code')!),
      said: read(row.querySelector('small')!),
      disabled: row.querySelector('input')!.disabled,
      chosen: row.querySelector('input')!.checked,
    }));

  const aiRows = () => all('dialog div.ai');
  const aiRow = (name: string, computer: string) =>
    aiRows().find(
      (row) =>
        read(row.querySelector('b')!) === name &&
        read(row.querySelector('small')!).startsWith(computer),
    )!;
  const peopleRows = () => all('dialog .part:first-of-type .pick-row');
  /** Another person of the group "Website", whom the create dialog offers. */
  async function addPerson(id: string, name: string) {
    workstation.home.write(
      website,
      example('row-member-human', { id, g: website, user: `${id}-user`, name }),
    );
    await new Promise((applied) => setTimeout(applied));
    flushSync();
  }
  async function openNewGroup(rows = 2) {
    go('#/groups');
    one<HTMLButtonElement>('.head .btn').click();
    flushSync();
    await vi.waitFor(() => expect(aiRows()).toHaveLength(rows));
  }
  /** Ticks "Claude Code" on the workstation and waits for its folder. */
  async function tickWorkstationClaude() {
    aiRow('Claude Code', 'workstation').querySelector('input')!.click();
    flushSync();
    await vi.waitFor(() =>
      expect(one('dialog .ai-fields .path-row code').textContent).toContain('/home/dev/'),
    );
  }
  /** Names the group and the owner, and submits the dialog. */
  function createAs(group: string, member: string) {
    const [name, who] = all<HTMLInputElement>('dialog .two input');
    fill(name, group);
    fill(who, member);
    submitDialog();
  }
  const sentCommands = () =>
    workstation.home.asked
      .filter((req) => req.t === 'cmd')
      .map((req) => JSON.parse((req.env as p.Envelope).p));

  Scenario(
    'The folder list says which AI of the group has a folder, and a folder of another group cannot be chosen',
    ({ Given, When, Then }) => {
      Given(
        'on "workstation" Claude Code of this group works in "/home/dev/website" and an AI of the group "Docs" in "/home/dev/notes"',
        () => {
          workstation.agents[0].groups = [
            working(website, 'claude-ws', '/home/dev/website'),
            working(docs, 'docs-ai', '/home/dev/notes'),
          ];
          workstation.homes = [
            { g: website, home: workstationId, name: 'Website' },
            { g: docs, home: workstationId, name: 'Docs' },
          ];
        },
      );
      When(
        'the owner opens "Add AI" with "Claude Code · workstation" chosen and opens its folder list',
        async () => {
          one<HTMLButtonElement>('.members .head button').click();
          flushSync();
          await vi.waitFor(() => expect(all('dialog .choice')).toHaveLength(2));
          await folderTaken();
          one<HTMLButtonElement>('dialog .path-row .change').click();
          await vi.waitFor(() => expect(all('dialog dialog .pick-row')).toHaveLength(2));
          flushSync();
        },
      );
      Then(
        '"/home/dev/website" is chosen and says "Claude Code works here too", and "/home/dev/notes" cannot be chosen and says "Taken by the group Docs"',
        () => {
          expect(folderRows()).toEqual([
            {
              path: '/home/dev/website',
              said: expect.stringMatching(/^Last used .+ · Claude Code works here too$/),
              disabled: false,
              chosen: true,
            },
            {
              path: '/home/dev/notes',
              said: 'Taken by the group Docs',
              disabled: true,
              chosen: false,
            },
          ]);
        },
      );
      When('the owner types "/home/dev/notes" as another folder and takes it', () => {
        fill(one<HTMLInputElement>('dialog dialog input.mono'), '/home/dev/notes');
        one<HTMLButtonElement>('dialog dialog footer .btn-primary').click();
        flushSync();
      });
      Then(
        'the list says "This folder belongs to the group Docs. Choose another one." and the folder stays "/home/dev/website"',
        () => {
          expect(texts('dialog dialog [role=alert]')).toEqual([
            'This folder belongs to the group Docs. Choose another one.',
          ]);
          expect(one<HTMLDialogElement>('dialog dialog').open).toBe(true);
          expect(read(one('dialog .path-row code'))).toBe('/home/dev/website');
        },
      );
      When('the owner closes the list', () => {
        one<HTMLButtonElement>('dialog dialog footer .btn-ghost').click();
        flushSync();
      });
      Then(
        'the dialog says the folder is shared with Claude Code and takes one task at a time',
        () => {
          expect(all('dialog dialog')).toEqual([]);
          expect(texts('dialog .folder .hint')).toEqual([
            'Shared with Claude Code: the folder takes one task at a time.',
          ]);
        },
      );
    },
  );

  Scenario(
    'A group is created with a person and two AIs, each in its folder, the first AI the secretary',
    ({ Given, When, Then, And }) => {
      let studio: string;
      Given(
        '"Mina" is in the group "Website" on "workstation", and a second computer "laptop" is connected',
        async () => {
          workstation.home.write(
            website,
            example('row-member-human', {
              id: 'mina',
              g: website,
              user: 'mina-user',
              name: 'Mina',
            }),
          );
          await addLaptop();
        },
      );
      When('the owner opens "New group" from the group list', async () => {
        go('#/groups');
        one<HTMLButtonElement>('.head .btn').click();
        flushSync();
        await vi.waitFor(() => expect(aiRows()).toHaveLength(4));
      });
      Then(
        'the dialog offers "Mina" and the four AIs, with "Codex · workstation" and "Claude Code · laptop" disabled and their computers before their reasons',
        () => {
          expect(texts('dialog .part:first-of-type .pick-row b')).toEqual(['Mina']);
          expect(
            aiRows().map((row) => [
              read(row.querySelector('b')!),
              read(row.querySelector('small')!),
              row.querySelector('input')!.disabled,
            ]),
          ).toEqual([
            ['Claude Code', 'workstation', false],
            ['Codex', 'workstation · Choose an account type on the Computers page first', true],
            ['Codex', 'laptop', false],
            [
              'Claude Code',
              'laptop · Its sign-in is no longer valid: sign in again on its computer',
              true,
            ],
          ]);
          expect(all('dialog div.ai small .bad')).toEqual([]);
        },
      );
      When('the owner ticks "Mina", "Claude Code · workstation" and "Codex · laptop"', async () => {
        one<HTMLInputElement>('dialog .part:first-of-type input[type=checkbox]').click();
        aiRow('Claude Code', 'workstation').querySelector('input')!.click();
        flushSync();
        aiRow('Codex', 'laptop').querySelector('input')!.click();
        flushSync();
        await vi.waitFor(() =>
          expect(texts('dialog .ai-fields .path-row code')).toEqual([
            '/home/dev/notes',
            '/home/dev/website',
          ]),
        );
      });
      Then(
        '"Claude Code" is the secretary, "Codex" is not, each secretary choice is named for its AI, and each AI has the newest folder that no other group has',
        () => {
          const radios = all<HTMLInputElement>('dialog .ai-fields input[type=radio]');
          expect(radios.map((radio) => radio.checked)).toEqual([true, false]);
          expect(new Set(radios.map((radio) => radio.name)).size).toBe(1);
          expect(texts('dialog .ai-fields .check')).toEqual([
            'Make Claude Code the secretary',
            'Make Codex the secretary',
          ]);
          expect(
            all<HTMLInputElement>('dialog .ai-fields input.input').map((input) => input.value),
          ).toEqual(['Claude Code', 'Codex']);
        },
      );
      When(
        'the owner makes "Codex" the secretary, names the group "Studio" and themself "Alex", and creates it',
        async () => {
          all<HTMLInputElement>('dialog .ai-fields input[type=radio]')[1].click();
          const [name, member] = all<HTMLInputElement>('dialog .two input');
          fill(name, 'Studio');
          fill(member, 'Alex');
          one('dialog form').dispatchEvent(
            new Event('submit', { bubbles: true, cancelable: true }),
          );
          await vi.waitFor(() => expect(location.hash).toMatch(/^#\/groups\/.{22}$/));
          studio = location.hash.split('/')[2];
        },
      );
      Then(
        'the home makes "Studio", adds Mina, and each computer receives a signed "host.join" with the AI\'s folder, "Codex" as the secretary',
        () => {
          const sent = workstation.home.asked
            .filter((req) => req.t === 'cmd')
            .map((req) => JSON.parse((req.env as p.Envelope).p));
          expect(sent).toMatchObject([
            { t: 'group', op: 'create', g: studio, name: 'Studio', member_name: 'Alex' },
            { t: 'member', op: 'add_person', g: studio, user: 'mina-user', name: 'Mina' },
          ]);
          const joined = (join: p.HostJoin) => [
            join.to,
            join.g,
            join.agent,
            join.name,
            join.workspace,
            join.secretary,
          ];
          expect(workstation.joins.map(joined)).toEqual([
            [workstationId, studio, 'claude', 'Claude Code', '/home/dev/notes', false],
          ]);
          expect(laptop.joins.map(joined)).toEqual([
            [laptopId, studio, 'codex', 'Codex', '/home/dev/website', true],
          ]);
        },
      );
      And('the chat of "Studio" opens with Mina and the two AIs', async () => {
        await vi.waitFor(() => {
          go(location.hash);
          expect(texts('.chat-head h2')).toEqual(['Studio']);
          expect(texts('.member h3')).toEqual(['Codex', 'Claude Code']);
        });
        expect(all('dialog[open]')).toEqual([]);
        expect(texts('.people .person-name')).toEqual(['Alex · You · Owner', 'Mina']);
      });
    },
  );

  Scenario(
    "A folder that is another group's, refused by the AI's computer, is reported with that group",
    ({ Given, When, Then }) => {
      Given('a second computer "laptop" is connected', addLaptop);
      When(
        '"laptop" refuses "host.join" with "conflict" naming the group "Docs" and the owner submits "Codex · laptop"',
        () => submitLaptopRefusal('conflict', 'the folder is the workspace of the group "Docs"'),
      );
      Then(
        'the dialog stays open and says the folder is the workspace of another group, in the words of the computer that name "Docs"',
        () => {
          expect(one<HTMLDialogElement>('#chat-side dialog').open).toBe(true);
          expect(texts('dialog [role=alert]')).toEqual([
            'This folder is already the workspace of another group. The computer said: the folder is the workspace of the group "Docs"',
          ]);
        },
      );
    },
  );

  Scenario(
    'A name that a member of the group has is refused by the dialog, and nothing is sent',
    ({ Given, When, Then }) => {
      const nameField = () => one<HTMLInputElement>('dialog input[required]');
      Given('a second computer "laptop" is connected', addLaptop);
      When(
        'the owner opens "Add AI", chooses "Codex · laptop", names the AI "Claude Code" and submits',
        async () => {
          await chooseLaptopCodex();
          fill(nameField(), 'Claude Code');
          flushSync();
          submitDialog();
          await new Promise((later) => setTimeout(later, 5));
          flushSync();
        },
      );
      Then(
        'the dialog stays open and says "That name is already used in this group." and "laptop" receives no "host.join"',
        () => {
          expect(one<HTMLDialogElement>('#chat-side dialog').open).toBe(true);
          expect(texts('dialog [role=alert]')).toEqual([
            'That name is already used in this group.',
          ]);
          expect(laptop.joins).toEqual([]);
          expect(workstation.joins).toEqual([]);
        },
      );
      When('the owner names the AI "Helper" and submits', () => {
        fill(nameField(), 'Helper');
        flushSync();
        submitDialog();
      });
      Then(
        '"laptop" receives a signed "host.join" for "codex" named "Helper" and the dialog closes',
        async () => {
          await vi.waitFor(() => expect(laptop.joins).toHaveLength(1));
          expect(laptop.joins[0]).toMatchObject({ agent: 'codex', name: 'Helper', g: website });
          await vi.waitFor(() => expect(one('#chat-side dialog[open]')).toBeNull());
        },
      );
    },
  );

  Scenario(
    'Two members of a new group cannot have one name, and nothing is sent',
    ({ Given, When, Then }) => {
      Given('the owner is on the group list', () => go('#/groups'));
      When(
        'the owner opens "New group", ticks "Claude Code · workstation", names the group "Studio" and themself "Claude Code", and creates it',
        async () => {
          await openNewGroup();
          await tickWorkstationClaude();
          createAs('Studio', 'Claude Code');
          flushSync();
        },
      );
      Then(
        'the dialog stays open and says "Two members cannot have one name: change one of the names." and the home receives no command',
        () => {
          expect(one<HTMLDialogElement>('dialog').open).toBe(true);
          expect(texts('dialog [role=alert]')).toEqual([
            'Two members cannot have one name: change one of the names.',
          ]);
          expect(sentCommands()).toEqual([]);
          expect(workstation.joins).toEqual([]);
        },
      );
      When('the owner names themself "Alex" and creates it', async () => {
        createAs('Studio', 'Alex');
        await vi.waitFor(() => expect(location.hash).toMatch(/^#\/groups\/.{22}$/));
      });
      Then('the home makes "Studio" and the chat of "Studio" opens', async () => {
        const studio = location.hash.split('/')[2];
        expect(sentCommands()).toMatchObject([
          { t: 'group', op: 'create', g: studio, name: 'Studio', member_name: 'Alex' },
        ]);
        await vi.waitFor(() => {
          go(location.hash);
          expect(texts('.chat-head h2')).toEqual(['Studio']);
        });
        expect(all('dialog[open]')).toEqual([]);
      });
    },
  );

  Scenario(
    'A group whose second person is refused says where it stopped, and opens all the same',
    ({ Given, When, Then, And }) => {
      let studio: string;
      const said = (row: Element) => {
        const small = row.querySelector('small')!;
        return [read(small), small.classList.contains('bad')];
      };
      Given(
        '"Mina" and "Sam" are in the group "Website" on "workstation", and the home no longer has a log of "Sam"',
        async () => {
          await addPerson('mina', 'Mina');
          await addPerson('sam', 'Sam');
          workstation.home.forgotten.add('sam-user');
        },
      );
      When(
        'the owner opens "New group", ticks "Mina", "Sam" and "Claude Code · workstation", names the group "Studio" and themself "Alex", and creates it',
        async () => {
          await openNewGroup();
          await vi.waitFor(() => expect(peopleRows()).toHaveLength(2));
          for (const row of peopleRows()) row.querySelector('input')!.click();
          flushSync();
          await tickWorkstationClaude();
          createAs('Studio', 'Alex');
          await vi.waitFor(() => expect(all('dialog [role=status]')).toHaveLength(1));
          studio = sentCommands()[0].g;
        },
      );
      Then(
        'the dialog stays open and says "Mina" was "Added", "Sam" was refused with "Not in any of your groups on this computer any more: invite them by link." and "Claude Code" is "Not sent: an earlier step was refused"',
        () => {
          expect(one<HTMLDialogElement>('dialog').open).toBe(true);
          const [mina, sam] = peopleRows();
          expect(read(mina.querySelector('b')!)).toBe('Mina');
          expect(said(mina)).toEqual(['Added', false]);
          expect(said(sam)).toEqual([
            'Not in any of your groups on this computer any more: invite them by link.',
            true,
          ]);
          const ai = aiRow('Claude Code', 'workstation');
          expect(read(ai.querySelector('small')!)).toBe(
            'workstation · Not sent: an earlier step was refused',
          );
          expect(ai.querySelector('small .bad')).not.toBeNull();
        },
      );
      And(
        'the dialog says "The group exists, but not everyone could be added. Add them again from its member panel." and its button reads "Open the group" with no "Cancel"',
        () => {
          expect(texts('dialog [role=status]')).toEqual([
            'The group exists, but not everyone could be added. Add them again from its member panel.',
          ]);
          expect(texts('dialog footer .btn')).toEqual(['Open the group']);
          expect(one<HTMLButtonElement>('dialog footer .btn').disabled).toBe(false);
        },
      );
      And(
        'the home made "Studio" and added Mina and tried Sam, and "workstation" received no "host.join"',
        () => {
          expect(sentCommands()).toMatchObject([
            { t: 'group', op: 'create', g: studio, name: 'Studio' },
            { t: 'member', op: 'add_person', g: studio, user: 'mina-user' },
            { t: 'member', op: 'add_person', g: studio, user: 'sam-user' },
          ]);
          expect(workstation.joins).toEqual([]);
        },
      );
      When('the owner presses "Open the group"', () => {
        submitDialog();
      });
      Then('the chat of "Studio" opens with Mina and without Sam', async () => {
        await vi.waitFor(() => {
          go(location.hash);
          expect(location.hash).toBe(`#/groups/${studio}`);
          expect(texts('.chat-head h2')).toEqual(['Studio']);
        });
        expect(all('dialog[open]')).toEqual([]);
        expect(texts('.people .person-name')).toEqual(['Alex · You · Owner', 'Mina']);
      });
    },
  );

  Scenario(
    'A group whose second AI is refused for its folder says so beside it, and the earlier ones stay added',
    ({ Given, When, Then, And }) => {
      Given(
        '"Mina" is in the group "Website" on "workstation", and "laptop" is connected and refuses "host.join" with "not_found"',
        async () => {
          await addPerson('mina', 'Mina');
          await addLaptop();
          laptop.refuseJoin = { code: 'not_found', msg: 'no such folder' };
        },
      );
      When(
        'the owner opens "New group", ticks "Mina", "Claude Code · workstation" and "Codex · laptop", names the group "Studio" and themself "Alex", and creates it',
        async () => {
          await openNewGroup(4);
          one<HTMLInputElement>('dialog .part:first-of-type input[type=checkbox]').click();
          await tickWorkstationClaude();
          aiRow('Codex', 'laptop').querySelector('input')!.click();
          flushSync();
          await vi.waitFor(() =>
            expect(texts('dialog .ai-fields .path-row code')).toEqual([
              '/home/dev/notes',
              '/home/dev/website',
            ]),
          );
          createAs('Studio', 'Alex');
          await vi.waitFor(() => expect(all('dialog [role=status]')).toHaveLength(1));
        },
      );
      Then(
        'Mina and "Claude Code" are "Added" and "Codex" says "There is no such folder on that computer."',
        () => {
          const small = (row: Element) => row.querySelector('small')!;
          expect([
            read(small(peopleRows()[0])),
            small(peopleRows()[0]).classList.contains('bad'),
          ]).toEqual(['Added', false]);
          const claude = aiRow('Claude Code', 'workstation');
          expect(read(small(claude))).toBe('workstation · Added');
          expect(small(claude).querySelector('.bad')).toBeNull();
          const codex = aiRow('Codex', 'laptop');
          expect(read(small(codex))).toBe('laptop · There is no such folder on that computer.');
          expect(small(codex).querySelector('.bad')).not.toBeNull();
        },
      );
      And('both computers received "host.join" and the button reads "Open the group"', () => {
        expect(workstation.joins).toHaveLength(1);
        expect(laptop.joins).toHaveLength(1);
        expect(texts('dialog footer .btn')).toEqual(['Open the group']);
      });
    },
  );

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
      all<HTMLInputElement>('dialog .choice input')[2].click();
      flushSync();
      await folderTaken();
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
    When('the owner closes the dialog and becomes a non-admin member', async () => {
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
      await syncNode(laptop);
    });
    Then('the member adds an AI from their connected computer', async () => {
      const add = one<HTMLButtonElement>('.members .head button');
      expect(add.disabled).toBe(false);
      expect(texts('.members .add-why')).toEqual([]);
      add.click();
      await vi.waitFor(() =>
        expect(one<HTMLInputElement>('dialog[open] input[name=candidate]:checked').disabled).toBe(
          false,
        ),
      );
      all<HTMLInputElement>('dialog .choice input')[2].click();
      flushSync();
      await folderTaken();
      one('dialog[open] form').dispatchEvent(
        new Event('submit', { bubbles: true, cancelable: true }),
      );
      await vi.waitFor(() =>
        expect(laptop.joins).toMatchObject([
          {
            t: 'host.join',
            g: website,
            home: workstationId,
            agent: 'codex',
            name: 'Codex',
            secretary: false,
          },
        ]),
      );
      await vi.waitFor(() => expect(one('#chat-side dialog[open]')).toBeNull());
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
