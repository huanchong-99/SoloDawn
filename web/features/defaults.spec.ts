import 'fake-indexeddb/auto';
import { describeFeature, loadFeature } from '@amiceli/vitest-cucumber';
import { chromium } from 'playwright';
import { flushSync, tick } from 'svelte';
import { SvelteMap } from 'svelte/reactivity';
import { createServer } from 'vite';
import { afterEach, describe, expect, it, vi } from 'vitest';
import AgentDefaults from '../src/components/defaults/AgentDefaults.svelte';
import { Home, homes } from '../src/lib/groups.svelte';
import { goldens } from '../src/lib/mock';
import type { Path, Source } from '../src/lib/net.svelte';
import { nodes, type Computer } from '../src/lib/nodes.svelte';
import { prefs } from '../src/lib/prefs.svelte';
import * as p from '../src/lib/proto';
import { example, ownedNode, user, type OwnedNode } from './home';
import { all, choose, closePage, fill, one, openPage, read, show, texts } from './page';

const feature = await loadFeature('../features/web_defaults.feature');

const device = 'J3Uuh-y1z2AX536yAFlGHJL5sjxVpWVJavvVphvdm5Q';
const website = 'rZNvy-1jH6Z-BcPqA5U5BQ';
const appGroup = 'aZNvy-1jH6Z-BcPqA5U5BQ';
const docs = 'dZNvy-1jH6Z-BcPqA5U5BQ';
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
  let sentDefaults: Record<string, unknown>[];
  let refuseDefaults: string | undefined;

  const aiRow = (cli: string) =>
    all('.card.node .ai-row').find((row) =>
      row.querySelector('.ai-title')?.textContent?.includes(cli),
    )!;
  const defaultsBtn = (cli: string) =>
    aiRow(cli).querySelector<HTMLButtonElement>('[data-guide="defaults"]')!;
  const editor = (cli: string) => aiRow(cli).querySelector<HTMLFormElement>('form.editor')!;

  function interceptDefaults(node: OwnedNode) {
    const ctl = node.store.ctl!;
    const base = ctl.request.bind(ctl);
    ctl.request = (async (req: { t: string; env?: p.Envelope }) => {
      if (req.t === 'cmd' && req.env) {
        const payload = JSON.parse(req.env.p) as Record<string, unknown>;
        if (payload.t === 'agent.defaults') {
          sentDefaults.push(payload);
          if (refuseDefaults) {
            return { re: 1, err: { code: refuseDefaults, msg: 'refused' } };
          }
          const entry = node.info.agents.find((item) => item.agent === payload.agent);
          if (entry) {
            entry.defaults = {
              model: String(payload.model),
              effort: String(payload.effort),
              tier: String(payload.tier),
              skills: String(payload.skills),
            };
          }
          return { re: 1, ok: true };
        }
      }
      return base(req as never);
    }) as p.Ctl['request'];
  }

  AfterEachScenario(async () => {
    await closePage();
    paths.clear();
    nodes.infos.clear();
    prefs.lang = 'en';
  });

  Background(({ Given }) => {
    Given(
      'the owner\'s computer "workstation" with Claude Code and Codex, both with an account type',
      async () => {
        sentDefaults = [];
        refuseDefaults = undefined;
        const baseInfo = p.decode(p.HostInfo, goldens['host-info-answer']);
        const claude = {
          ...baseInfo.agents[0],
          account: 'personal',
          defaults: undefined,
          groups: [
            {
              g: website,
              member: 'claude-website',
              workspace: '/home/dev/website',
              level: 2,
              state: 'working',
              task: 'DrtCn6htSBwmMPrFPbHJHA',
              queued: 0,
              watchers: [],
            },
            {
              g: appGroup,
              member: 'claude-app',
              workspace: '/home/dev/app',
              level: 2,
              state: 'dormant',
              queued: 0,
              watchers: [],
            },
            {
              g: docs,
              member: 'claude-docs',
              workspace: '/home/dev/docs',
              level: 2,
              state: 'dormant',
              queued: 0,
              watchers: [],
            },
          ],
        };
        const codex = {
          ...baseInfo.agents[1],
          account: 'api_key',
          defaults: undefined,
          groups: [],
        };
        workstation = ownedNode('workstation', device, {
          homes: [
            { g: website, home: device, name: 'Website' },
            { g: appGroup, home: device, name: 'App' },
            { g: docs, home: device, name: 'Docs' },
          ],
          agents: [claude, codex],
        });

        workstation.home.add(website, 'Website');
        workstation.home.write(website, example('row-task', { g: website }));
        workstation.home.write(
          website,
          example('row-member-ai', {
            id: 'claude-website',
            g: website,
            user,
            name: 'Claude Code',
            cli: 'claude',
            host: device,
            follow: true,
          }),
        );

        workstation.home.add(appGroup, 'App');
        workstation.home.write(
          appGroup,
          example('row-member-ai', {
            id: 'claude-app',
            g: appGroup,
            user,
            name: 'Claude Code',
            cli: 'claude',
            host: device,
            follow: true,
          }),
        );

        workstation.home.add(docs, 'Docs');
        workstation.home.write(
          docs,
          example('row-member-ai', {
            id: 'claude-docs',
            g: docs,
            user,
            name: 'Opus',
            cli: 'claude',
            host: device,
            state: 'dormant',
            task: undefined,
            follow: undefined,
          }),
        );

        homes.push(workstation.store);
        await workstation.connect();
        interceptDefaults(workstation);
        paths.set(workstation.store, { kind: 'direct', rtt: 4, since: Date.now() });

        openPage('#/nodes');
        await vi.waitFor(() => {
          flushSync();
          expect(all('.card.node .ai-row')).toHaveLength(2);
          expect(nodes.infos.has(workstation.store)).toBe(true);
        });
        flushSync();
      },
    );
  });

  Scenario("The owner sets a CLI's defaults once", ({ Then, When, And }) => {
    Then('Claude Code shows "Not set" and "Set defaults"', () => {
      const row = aiRow('Claude Code');
      expect(read(row.querySelector('.def-head .badge')!)).toBe('Not set');
      expect(read(defaultsBtn('Claude Code'))).toBe('Set defaults');
      expect(read(row.querySelector('.sum')!)).toBe(
        "Not set yet: the CLI's own model, Normal work share",
      );
    });

    When('the owner opens the defaults of Claude Code', () => {
      defaultsBtn('Claude Code').click();
      flushSync();
      expect(editor('Claude Code')).toBeDefined();
    });

    And('chooses model "Opus", level "high", work share More and skills "Go backend"', () => {
      const form = editor('Claude Code');
      const selects = [...form.querySelectorAll<HTMLSelectElement>('select')];
      choose(selects[0], 'Opus');
      choose(selects[1], 'high');
      const strong = form.querySelector<HTMLInputElement>('input[type="radio"][value="strong"]')!;
      strong.checked = true;
      strong.dispatchEvent(new Event('change', { bubbles: true }));
      const skillsInput = form.querySelector<HTMLInputElement>('input.input')!;
      fill(skillsInput, 'Go backend');
      flushSync();
    });

    Then('the hint says saving also updates 2 group members that follow these', () => {
      expect(read(editor('Claude Code').querySelector('.foot .hint')!)).toBe(
        'Saving also updates 2 group members that follow these',
      );
    });

    When('the owner saves the defaults', async () => {
      const form = editor('Claude Code');
      form.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }));
      await vi.waitFor(() => {
        flushSync();
        expect(aiRow('Claude Code').querySelector('form.editor')).toBeNull();
      });
      await tick();
      flushSync();
    });

    Then(
      'exactly one agent.defaults is sent with "opus", "high", "strong" and "Go backend"',
      () => {
        expect(sentDefaults).toHaveLength(1);
        expect(sentDefaults[0]).toMatchObject({
          t: 'agent.defaults',
          to: device,
          agent: 'claude',
          model: 'opus',
          effort: 'high',
          tier: 'strong',
          skills: 'Go backend',
        });
      },
    );

    And('Claude Code shows "Opus · high · Work share More · Go backend"', () => {
      const row = aiRow('Claude Code');
      expect(row.querySelector('.def-head .badge')).toBeNull();
      expect(read(row.querySelector('.sum')!)).toBe('Opus · high · Work share More · Go backend');
    });

    And('focus is on Claude Code\'s "Edit" button', () => {
      const btn = defaultsBtn('Claude Code');
      expect(read(btn)).toBe('Edit');
      expect(document.activeElement).toBe(btn);
    });
  });

  Scenario('Each group line says whether it follows', ({ Then, And }) => {
    const permFor = (groupName: string) =>
      [...aiRow('Claude Code').querySelectorAll('.perm')].find((el) =>
        el.querySelector('a')?.textContent?.includes(groupName),
      )!;

    Then('the "Website" line of Claude Code says "Follows defaults"', () => {
      const line = permFor('Website');
      expect(read(line.querySelector('.def-follow')!)).toBe('Follows defaults');
      expect(line.querySelector('.perm-def .badge')).toBeNull();
    });

    And('the "Docs" line of Claude Code shows the badge "Own setting in this group"', () => {
      const line = permFor('Docs');
      expect(read(line.querySelector('.perm-def .badge')!)).toBe('Own setting in this group');
    });
  });

  Scenario('A refused save keeps the editor', ({ When, Then }) => {
    When('the node refuses agent.defaults with "bad_request"', async () => {
      refuseDefaults = 'bad_request';
      defaultsBtn('Claude Code').click();
      flushSync();
      const form = editor('Claude Code');
      const skillsInput = form.querySelector<HTMLInputElement>('input.input')!;
      fill(skillsInput, 'Go backend');
      flushSync();
      form.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }));
      await vi.waitFor(() => {
        flushSync();
        expect(aiRow('Claude Code').querySelector('.fail')).not.toBeNull();
      });
    });

    Then('the editor stays open with the failure text', () => {
      const row = aiRow('Claude Code');
      expect(row.querySelector('form.editor')).not.toBeNull();
      expect(texts('.ai-row .fail')).toEqual([
        'Not sent: the home refused it as too long or malformed.',
      ]);
      expect(workstation.info.agents[0].defaults).toBeUndefined();
    });
  });

  Scenario('Phone width', ({ When, Then }) => {
    let layoutCheck: {
      scrollWidth: number;
      clientWidth: number;
      overflows: { tag: string; cls: string; right: number }[];
      buttonHeights: number[];
    };

    When('the page is 375 px wide with the defaults editor open', async () => {
      const server = await createServer({
        mode: 'development',
        server: { host: '127.0.0.1', port: 0 },
        logLevel: 'silent',
      });
      await server.listen();
      const browser = await chromium.launch({ executablePath: process.env.CHROMIUM_PATH });
      try {
        const page = await browser.newPage({
          viewport: { width: 375, height: 812 },
          locale: 'en-US',
        });
        await page.goto(`${server.resolvedUrls!.local[0]}#/nodes`);
        await page.locator('[data-guide="defaults"]').first().waitFor();
        await page.locator('[data-guide="defaults"]').first().click();
        await page.locator('.defaults form.editor').waitFor();
        layoutCheck = await page.evaluate(() => {
          const vw = document.documentElement.clientWidth;
          const overflows = [...document.querySelectorAll('body *')]
            .map((el) => ({
              tag: el.tagName,
              cls: el.className?.toString() ?? '',
              rect: el.getBoundingClientRect(),
            }))
            .filter(({ rect }) => rect.width > 0 && rect.height > 0 && rect.right > vw + 0.5)
            .map(({ tag, cls, rect }) => ({ tag, cls, right: rect.right }));
          const buttonHeights = [
            ...document.querySelectorAll<HTMLButtonElement>('.defaults button'),
          ].map((btn) => btn.getBoundingClientRect().height);
          return {
            scrollWidth: document.documentElement.scrollWidth,
            clientWidth: vw,
            overflows,
            buttonHeights,
          };
        });
      } finally {
        await browser.close();
        await server.close();
      }
    });

    Then('no element is wider than the page, and the buttons are at least 44 px high', () => {
      expect(layoutCheck.scrollWidth).toBeLessThanOrEqual(375);
      expect(layoutCheck.overflows).toEqual([]);
      expect(layoutCheck.buttonHeights.length).toBeGreaterThanOrEqual(3);
      for (const h of layoutCheck.buttonHeights) expect(h).toBeGreaterThanOrEqual(44);
    });
  });
});

describe('AgentDefaults states', () => {
  const me = { user, sign: async (payload: string) => ({ p: payload, k: 'k', s: 's' }) };
  const baseInfo = p.decode(p.HostInfoPush, goldens['host-info-push']);

  function setup(opts: { online?: boolean; defaults?: p.Profile }) {
    prefs.lang = 'en';
    const home = new Home(me);
    const info: p.HostInfoPush = { ...baseInfo, device };
    const agent = { ...info.agents[0], account: 'personal', defaults: opts.defaults };
    const comp: Computer = {
      home,
      info,
      path: opts.online === false ? undefined : { kind: 'direct', rtt: 4, since: Date.now() },
      online: opts.online ?? true,
    };
    show(AgentDefaults, { comp, agent });
    one<HTMLButtonElement>('[data-guide="defaults"]').click();
    flushSync();
  }

  afterEach(async () => {
    await closePage();
    nodes.infos.clear();
    prefs.lang = 'en';
  });

  it('disables Save while the computer is offline', () => {
    setup({ online: false });
    expect(one<HTMLButtonElement>('form.editor button.btn-primary').disabled).toBe(true);
  });

  it('disables Save while nothing changed', () => {
    setup({
      defaults: { model: 'opus', effort: 'high', tier: 'strong', skills: 'Go backend' },
    });
    const save = one<HTMLButtonElement>('form.editor button.btn-primary');
    expect(save.disabled).toBe(true);
    fill(one<HTMLInputElement>('form.editor input.input'), 'Go backend, tests');
    flushSync();
    expect(save.disabled).toBe(false);
  });

  it('shows the no-followers hint when no member follows', () => {
    setup({});
    expect(read(one('form.editor .foot .hint'))).toBe('No group member follows these yet');
  });
});
