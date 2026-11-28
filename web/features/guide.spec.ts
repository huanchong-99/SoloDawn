import 'fake-indexeddb/auto';
import { describeFeature, loadFeature } from '@amiceli/vitest-cucumber';
import { chromium, type Browser, type Page } from 'playwright';
import { flushSync, tick } from 'svelte';
import { createServer, type ViteDevServer } from 'vite';
import { afterAll, afterEach, beforeAll, describe, expect, it, vi } from 'vitest';
import { clearBeacon } from '../src/components/guide/beacon';
import { DeviceLog } from '../src/lib/devlog';
import { creating, find, homes, newId, type Home } from '../src/lib/groups.svelte';
import { guideState, guideUi, isBrandNew, resetGuideUi } from '../src/lib/guide.svelte';
import { runTx } from '../src/lib/idbtx';
import { Device, ed25519 } from '../src/lib/identity';
import type { Net, Path, Source } from '../src/lib/net.svelte';
import { nodes } from '../src/lib/nodes.svelte';
import { saveJoined, saveOwner } from '../src/lib/owner';
import { announcement } from '../src/lib/prefs.svelte';
import type { HostInfoPush } from '../src/lib/proto';
import { example, ownedNode, owner, user, type OwnedNode } from './home';
import { all, closePage, go, one, openPage } from './page';

const feature = await loadFeature('../features/web_guide.feature');

const websiteId = 'rZNvy-1jH6Z-BcPqA5U5BQ';
const nonStarterId = 'DrtCn6htSBwmMPrFPbHJHA';
const secAiId = '1PC8WineBrUQ-apCjx7tug';
const otherUserAiId = 'Y2xhdWRlLW1lbWJlci0wMQ';
const paths = new Map<Home, Path | undefined>();
let currentNet!: Net;

const readyDefaults = {
  model: 'sonnet',
  effort: 'medium',
  tier: 'standard',
  skills: 'coding',
};

type Agent = HostInfoPush['agents'][number];

function makeAgent(overrides: Partial<Agent> = {}): Agent {
  return {
    agent: 'claude',
    cli_version: '1.0.0',
    account: null,
    login_ok: true,
    driver: { accounts: ['personal', 'team', 'api'], takeover: 'none' },
    models: [],
    caps: {},
    defaults: undefined,
    groups: [],
    ...overrides,
  };
}

vi.mock('@xterm/xterm', () => ({
  Terminal: class {
    cols = 80;
    rows = 24;
    textarea = document.createElement('textarea');
    attachCustomKeyEventHandler() {}
    loadAddon() {}
    open() {}
    write() {}
    onData() {}
    onBinary() {}
    onResize() {}
    focus() {}
    clear() {}
    dispose() {}
  },
}));

vi.mock('@xterm/addon-fit', () => ({
  FitAddon: class {
    fit() {}
  },
}));

vi.mock('@xterm/xterm/css/xterm.css', () => ({}));

vi.mock('../src/lib/net.svelte', async (actual) => {
  const real = await actual<typeof import('../src/lib/net.svelte')>();
  return {
    ...real,
    start: (source: Source) => {
      const net: Net = {
        source,
        homes,
        path: (home: Home) => (home.ctl ? paths.get(home) : undefined),
      };
      currentNet = net;
      return { net, stop: () => {} };
    },
  };
});

async function clearSolodawnIdb() {
  await runTx(
    () => indexedDB,
    'readwrite',
    undefined,
    (store) => {
      store.clear();
    },
  );
}

async function seedOwnerRecord() {
  const pair = (await crypto.subtle.generateKey(ed25519, false, [
    'sign',
    'verify',
  ])) as CryptoKeyPair;
  const dev = await Device.of(crypto, pair);
  const env = await dev.sign(
    JSON.stringify({
      t: 'device',
      n: 'AAAAAAAAAAAAAAAAAAAAAA',
      ts: 1,
      user: dev.id,
      ver: 1,
      op: 'add',
      dev: dev.id,
      kind: 'browser',
      name: 'Guide Browser',
    }),
  );
  const log = new DeviceLog(crypto, dev.id);
  await log.apply(env);
  await saveOwner(() => indexedDB, log, {});
  return dev.id;
}

function setHostAgents(agents: Agent[], name = 'workstation') {
  const key = [...nodes.infos.keys()][0]!;
  const prev = nodes.infos.get(key)!;
  nodes.infos.set(key, { ...prev, name, agents });
  flushSync();
}

function addStarterGroup(rig: OwnedNode, g = websiteId, groupName = 'Website') {
  rig.home.add(g, groupName);
  rig.home.write(g, example('row-member-human', { id: owner, g, user, name: 'Me' }));
  rig.home.write(g, example('row-member-ai', { id: secAiId, g, user, name: 'Claude Code' }));
  rig.home.write(
    g,
    example('row-member-ai', { id: 'worker-ai-1', g, user, name: 'Codex', cli: 'codex' }),
  );
  rig.home.write(
    g,
    example('row-member-ai', { id: 'worker-ai-2', g, user, name: 'Worker 2', cli: 'claude' }),
  );
  rig.home.write(g, example('row-group', { g, name: groupName, secretary: secAiId }));
}

function clickButton(selector: string, match: string) {
  const btn = all<HTMLButtonElement>(selector).find((b) =>
    (b.textContent ?? '').replace(/\s+/g, ' ').trim().includes(match),
  );
  expect(btn, `button containing "${match}" in ${selector}`).toBeDefined();
  btn!.click();
  flushSync();
}

async function waitDone(check: () => void) {
  await vi.waitFor(() => {
    flushSync();
    check();
  });
}

describeFeature(feature, ({ Scenario, AfterEachScenario }) => {
  let workstation: OwnedNode | undefined;

  AfterEachScenario(async () => {
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
    vi.useRealTimers();
    await closePage();
    clearBeacon();
    resetGuideUi();
    homes.length = 0;
    paths.clear();
    nodes.infos.clear();
    localStorage.clear();
    announcement.text = '';
    guideState.value = 'new';
    workstation = undefined;
    await clearSolodawnIdb();
  });

  Scenario(
    'A brand-new browser is welcomed and taken to pairing (G.1)',
    ({ Given, When, Then }) => {
      Given('a browser with no computer and no group', async () => {
        await clearSolodawnIdb();
      });

      When('it opens the client', async () => {
        openPage('#/groups', 1440, 'fine');
        await waitDone(() => expect(one<HTMLDialogElement>('dialog.welcome').open).toBe(true));
      });

      Then('the welcome dialog says "Welcome to SoloDawn" and lists the five steps', () => {
        const dlg = one<HTMLDialogElement>('dialog.welcome');
        expect(dlg.open).toBe(true);
        expect(dlg.querySelector('h2')?.textContent).toBe('Welcome to SoloDawn');
        expect([...dlg.querySelectorAll('.wl li')].map((el) => el.textContent?.trim())).toEqual([
          'Connect your computer',
          'Check the AIs found',
          "Set each AI's defaults",
          'Create your first group',
          'Send the secretary a first request',
        ]);
      });

      When('the person chooses "Start"', async () => {
        clickButton('dialog.welcome button', 'Start');
        await tick();
        expect(document.activeElement).toBe(one('.guide-dock h2'));
      });

      Then('the guide dock is open at 0 of 5 with "Connect your computer" current', async () => {
        expect(one('.guide-dock .count').textContent).toBe('0 of 5');
        expect(one('.guide-dock .step.cur b').textContent).toContain('Connect your computer');

        // M6: collapse with [⌄] and Escape preserves focus on pill; expand restores focus on heading.
        one<HTMLButtonElement>('.guide-dock button[aria-label="Collapse"]').click();
        flushSync();
        await tick();
        expect(document.activeElement).toBe(one('.guide-pill'));

        one<HTMLButtonElement>('.guide-pill').click();
        flushSync();
        await tick();
        expect(document.activeElement).toBe(one('.guide-dock h2'));

        one<HTMLButtonElement>('.guide-dock button[aria-label="Collapse"]').focus();
        window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', cancelable: true }));
        flushSync();
        await tick();
        expect(document.activeElement).toBe(one('.guide-pill'));

        one<HTMLButtonElement>('.guide-pill').click();
        flushSync();
        await tick();
      });

      When('the person chooses "Connect"', () => {
        clickButton('.guide-dock .step.cur button', 'Connect');
        go(location.hash);
      });

      Then('the address is "#/onboarding" and neither the guide dock nor its pill is shown', () => {
        expect(location.hash).toBe('#/onboarding');
        expect(all('.guide-dock')).toHaveLength(0);
        expect(all('.guide-pill')).toHaveLength(0);
      });
    },
  );

  Scenario('Each step checks off from the real state (G.2)', ({ Given, When, Then }) => {
    let composer: HTMLTextAreaElement;

    Given('the guide is open', async () => {
      await clearSolodawnIdb();
      await seedOwnerRecord();
      guideState.value = 'open';
      openPage('#/groups', 1440, 'fine');
      flushSync();
      expect(one('.guide-dock')).toBeTruthy();
    });

    When(
      'the browser has the computer "workstation" whose Claude Code has no account type',
      async () => {
        workstation = ownedNode('workstation', 'J3Uuh-y1z2AX536yAFlGHJL5sjxVpWVJavvVphvdm5Q', {
          homes: [],
          agents: [makeAgent()],
        });
        homes.push(workstation.store);
        await workstation.connect();
        await waitDone(() => expect(nodes.list(currentNet)[0]?.info?.agents).toHaveLength(1));
        expect(announcement.text).toBe('');
      },
    );

    Then('step 1 is done with "Connected "workstation"" and step 2 is current', () => {
      expect(one('.guide-dock .step.done .right').textContent).toContain('Connected "workstation"');
      expect(one('.guide-dock .step.cur b').textContent).toContain('Check the AIs found');
    });

    When('Claude Code gets the account type personal', () => {
      setHostAgents([makeAgent({ account: 'personal' })]);
    });

    Then('step 2 is done and step 3 is current, saying "Still to set: Claude Code"', () => {
      expect(announcement.text).toBe('Done: Check the AIs found');
      expect(one('.guide-dock .step.cur b').textContent).toContain("Set each AI's defaults");
      expect(one('.guide-dock .step.cur .left').textContent).toBe('Still to set: Claude Code');
    });

    When('host.info gives Claude Code defaults', () => {
      setHostAgents([makeAgent({ account: 'personal', defaults: { ...readyDefaults } })]);
    });

    Then('step 3 is done and step 4 is current', async () => {
      expect(announcement.text).toBe("Done: Set each AI's defaults");
      expect(one('.guide-dock .step.cur b').textContent).toContain('Create your first group');

      // M2: choosing "Create a group" while already mounted on #/groups opens the dialog via $effect.
      expect(location.hash).toBe('#/groups');
      clickButton('.guide-dock .step.cur button', 'Create a group');
      await waitDone(() => {
        expect(one<HTMLDialogElement>('dialog[open]')).toBeTruthy();
        expect(creating.open).toBe(false);
      });
      one<HTMLDialogElement>('dialog[open]').close();

      // Re-expand pill and verify a non-starter group (other user's AI only) does not finish step 4.
      one<HTMLButtonElement>('.guide-pill').click();
      flushSync();
      workstation!.home.add(nonStarterId, 'Other Only');
      workstation!.home.write(
        nonStarterId,
        example('row-member-human', { id: owner, g: nonStarterId, user, name: 'Me' }),
      );
      workstation!.home.write(
        nonStarterId,
        example('row-member-ai', {
          id: otherUserAiId,
          g: nonStarterId,
          user: 'other-user',
          name: 'Peer AI',
        }),
      );
      workstation!.home.write(
        nonStarterId,
        example('row-group', { g: nonStarterId, name: 'Other Only', secretary: otherUserAiId }),
      );
      workstation!.drop();
      await workstation!.connect();
      await waitDone(() => expect(find(nonStarterId)?.info).toBeDefined());
      setHostAgents([makeAgent({ account: 'personal', defaults: { ...readyDefaults } })]);
      expect(one('.guide-dock .step.cur b').textContent).toContain('Create your first group');
    });

    When('the group "Website" has a secretary and 1 working AI', async () => {
      workstation!.home.add(websiteId, 'Website');
      workstation!.home.write(
        websiteId,
        example('row-member-human', { id: owner, g: websiteId, user, name: 'Me' }),
      );
      workstation!.home.write(
        websiteId,
        example('row-member-ai', { id: secAiId, g: websiteId, user, name: 'Claude Code' }),
      );
      workstation!.home.write(
        websiteId,
        example('row-member-ai', {
          id: 'worker-ai-1',
          g: websiteId,
          user,
          name: 'Codex',
          cli: 'codex',
        }),
      );
      workstation!.home.write(
        websiteId,
        example('row-group', { g: websiteId, name: 'Website', secretary: secAiId }),
      );
      workstation!.drop();
      await workstation!.connect();
      await waitDone(() => expect(find(websiteId)?.info?.secretary).toBe(secAiId));
      setHostAgents([makeAgent({ account: 'personal', defaults: { ...readyDefaults } })]);
    });

    Then('step 4 stays current and its action is "Add AI"', async () => {
      expect(one('.guide-dock .step.cur b').textContent).toContain('Create your first group');
      expect(one('.guide-dock .step.cur .body p').textContent?.trim()).toBe(
        'This group needs 1 more working AI. Click Add AI; you can add the same CLI again with another name.',
      );
      expect(all('.guide-dock .step.cur .tip')).toHaveLength(0);
      expect(one('.guide-dock .step.cur button').textContent?.trim()).toBe('Add AI');

      clickButton('.guide-dock .step.cur button', 'Add AI');
      go(location.hash);
      expect(location.hash).toBe(`#/groups/${websiteId}`);
      await waitDone(() => expect(one('[data-guide="add-ai"]')).toBeTruthy());
      one<HTMLButtonElement>('.guide-pill').click();
      flushSync();
    });

    When('the group "Website" has a secretary and 2 working AIs besides it', () => {
      workstation!.home.write(
        websiteId,
        example('row-member-ai', {
          id: 'worker-ai-2',
          g: websiteId,
          user,
          name: 'Worker 2',
          cli: 'claude',
        }),
      );
      flushSync();
    });

    Then('step 4 is done with "Created "Website"" and step 5 is current', () => {
      expect(one('.guide-dock .step:nth-child(4) .right').textContent).toContain(
        'Created "Website"',
      );
      expect(one('.guide-dock .step.cur b').textContent).toContain(
        'Send the secretary a first request',
      );
    });

    When('the person\'s text message appears in the main line of "Website"', () => {
      clickButton('.guide-dock .step.cur button', 'Open the group');
      go(location.hash);
      composer = one<HTMLTextAreaElement>('.composer textarea');
      composer.focus();
      expect(document.activeElement).toBe(composer);

      workstation!.home.say(websiteId, secAiId, 'AI main hello', { id: newId() });
      workstation!.home.say(websiteId, owner, 'Thread reply', {
        id: newId(),
        thread: 'thread-root-id',
      });
      flushSync();
      expect(one('.guide-pill .t').textContent).toBe('Send the secretary a first request');

      workstation!.home.say(websiteId, owner, 'Build a to-do list page', { id: newId() });
      flushSync();
    });

    Then('the dock shows "You\'re all set" with 5 of 5', async () => {
      await waitDone(() => {
        expect(one('.guide-dock h2').textContent).toBe("You're all set");
        expect(one('.guide-dock .count').textContent).toBe('5 of 5');
      });
      await tick();
      expect(document.activeElement).toBe(composer);
    });

    When('the person chooses "Done"', () => {
      clickButton('.guide-dock .finish button', 'Done');
    });

    Then('the dock is gone and the stored guide state is "done"', () => {
      expect(all('.guide-dock')).toHaveLength(0);
      expect(localStorage.getItem('sd-guide-state')).toBe('done');
    });
  });

  Scenario('Clicks alone do not tick a step (G.3)', ({ Given, When, Then, And }) => {
    Given('the guide is open at step 3', async () => {
      await clearSolodawnIdb();
      await seedOwnerRecord();
      guideState.value = 'open';
      workstation = ownedNode('workstation', 'J3Uuh-y1z2AX536yAFlGHJL5sjxVpWVJavvVphvdm5Q', {
        homes: [],
        agents: [makeAgent({ agent: 'claude', account: 'personal', defaults: undefined })],
      });
      homes.push(workstation.store);
      await workstation.connect();
      openPage('#/groups', 1440, 'fine');
      await waitDone(() =>
        expect(one('.guide-dock .step.cur b').textContent).toContain("Set each AI's defaults"),
      );
    });

    When('the person chooses "Set them"', () => {
      clickButton('.guide-dock .step.cur button', 'Set them');
      go(location.hash);
    });

    Then(
      'the address is "#/nodes" and the beacon surrounds Claude Code\'s "Set defaults" button',
      async () => {
        expect(location.hash).toBe('#/nodes');
        await waitDone(() => expect(all('.ai-row [data-guide="defaults"]')).toHaveLength(1));
        const claudeBtn = one<HTMLButtonElement>('.ai-row [data-guide="defaults"]');
        expect(claudeBtn.hasAttribute('data-unset')).toBe(true);
        vi.spyOn(claudeBtn, 'getBoundingClientRect').mockReturnValue({
          left: 200,
          top: 120,
          width: 96,
          height: 32,
          right: 296,
          bottom: 152,
        } as DOMRect);

        await waitDone(() => expect(all('.guide-beacon')).toHaveLength(1));
        const beaconEl = one<HTMLElement>('.guide-beacon');
        expect([
          beaconEl.style.left,
          beaconEl.style.top,
          beaconEl.style.width,
          beaconEl.style.height,
        ]).toEqual(['196px', '116px', '104px', '40px']);
        clearBeacon();

        // M3: two CLIs (Claude Code set, Codex unset) — beacon targets Codex's [data-unset] button.
        setHostAgents([
          makeAgent({ agent: 'claude', account: 'personal', defaults: { ...readyDefaults } }),
          makeAgent({ agent: 'codex', account: 'team', defaults: undefined }),
        ]);
        await waitDone(() => expect(all('.ai-row [data-guide="defaults"]')).toHaveLength(2));
        const [setClaudeBtn, unsetCodexBtn] = all<HTMLButtonElement>(
          '.ai-row [data-guide="defaults"]',
        );
        expect(setClaudeBtn.hasAttribute('data-unset')).toBe(false);
        expect(unsetCodexBtn.hasAttribute('data-unset')).toBe(true);
        vi.spyOn(setClaudeBtn, 'getBoundingClientRect').mockReturnValue({
          left: 100,
          top: 80,
          width: 90,
          height: 30,
        } as DOMRect);
        vi.spyOn(unsetCodexBtn, 'getBoundingClientRect').mockReturnValue({
          left: 240,
          top: 310,
          width: 104,
          height: 32,
        } as DOMRect);

        one<HTMLButtonElement>('.guide-pill').click();
        flushSync();
        clickButton('.guide-dock .step.cur button', 'Set them');
        await waitDone(() => expect(all('.guide-beacon')).toHaveLength(1));
        const codexBeacon = one<HTMLElement>('.guide-beacon');
        expect([
          codexBeacon.style.left,
          codexBeacon.style.top,
          codexBeacon.style.width,
          codexBeacon.style.height,
        ]).toEqual(['236px', '306px', '112px', '40px']);
      },
    );

    And('step 3 is still current', () => {
      expect(one('.guide-pill .t').textContent).toBe("Set each AI's defaults");
      one<HTMLButtonElement>('.guide-pill').click();
      flushSync();
      expect(one('.guide-dock .step.cur b').textContent).toContain("Set each AI's defaults");
    });
  });

  Scenario('No AI found, and a computer that is offline (G.4)', ({ Given, Then, When }) => {
    let refreshed: Home[] = [];

    Given('the guide is open and the computer "workstation" lists no CLI', async () => {
      refreshed = [];
      await clearSolodawnIdb();
      await seedOwnerRecord();
      guideState.value = 'open';
      workstation = ownedNode('workstation', 'J3Uuh-y1z2AX536yAFlGHJL5sjxVpWVJavvVphvdm5Q', {
        homes: [],
        agents: [],
      });
      homes.push(workstation.store);
      await workstation.connect();
      openPage('#/groups', 1440, 'fine');
      await waitDone(() =>
        expect(one('.guide-dock .step.cur').textContent).toContain(
          'No AI coding assistant found yet.',
        ),
      );
    });

    Then('step 2 says no AI coding assistant was found and offers "Check again"', () => {
      expect(one('.guide-dock .step.cur').textContent).toContain(
        'No AI coding assistant found yet.',
      );
      expect(one('.guide-dock .step.cur button').textContent?.trim()).toBe('Check again');
    });

    When('the person chooses "Check again"', () => {
      vi.spyOn(nodes, 'refresh').mockImplementation(async (home) => {
        refreshed.push(home);
      });
      clickButton('.guide-dock .step.cur button', 'Check again');
    });

    Then('host.info is requested with refresh', () => {
      expect(refreshed).toEqual([workstation!.store]);
    });

    When('the computer goes offline', async () => {
      workstation!.drop();
      flushSync();
      expect(announcement.text).toBe('');
    });

    Then('step 2 says ""workstation" is offline"', async () => {
      expect(one('.guide-dock .step.cur').textContent).toContain('"workstation" is offline');

      // Minor 1: offline -> online reconnect with host.info must not announce "Done: ..."
      await workstation!.connect();
      await waitDone(() => expect(nodes.list(currentNet)[0]?.online).toBe(true));
      expect(announcement.text).toBe('');
    });
  });

  Scenario(
    'Skip, then reopen from the top bar and from Settings (G.5)',
    ({ Given, When, Then, And }) => {
      Given('the welcome dialog is shown', async () => {
        await clearSolodawnIdb();
        openPage('#/groups', 1440, 'fine');
        await waitDone(() => expect(one<HTMLDialogElement>('dialog.welcome').open).toBe(true));
      });

      When('the person chooses "Look around first"', () => {
        vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout'] });
        clickButton('dialog.welcome button', 'Look around first');
      });

      Then(
        'no dock is shown and the note says the guide can be reopened in Settings › Getting started',
        async () => {
          expect(all('.guide-dock')).toHaveLength(0);
          expect(one('.guide-toast').textContent).toContain(
            'Guide closed. Reopen it any time in Settings › Getting started.',
          );
          await vi.advanceTimersByTimeAsync(3999);
          flushSync();
          expect(guideUi.closedNote).toBe(true);
          await vi.advanceTimersByTimeAsync(1);
          expect(guideUi.closedNote).toBe(false);
          vi.useRealTimers();
          await vi.waitFor(() => {
            flushSync();
            expect(all('.guide-toast')).toHaveLength(0);
          });
        },
      );

      And('the top bar has "Guide 0/5"', () => {
        const chip = one<HTMLButtonElement>('.guide-chip');
        expect(chip.getAttribute('aria-label')).toBe('Guide 0/5');
        expect(chip.textContent?.replace(/\s+/g, ' ').trim()).toBe('Guide 0/5');
      });

      When('the person chooses "Guide 0/5"', async () => {
        one<HTMLButtonElement>('.guide-chip').click();
        flushSync();
        await tick();
        expect(document.activeElement).toBe(one('.guide-dock h2'));
      });

      Then('the dock is open', () => {
        expect(one('.guide-dock')).toBeTruthy();
        expect(localStorage.getItem('sd-guide-state')).toBe('open');
      });

      When('the person closes the guide', () => {
        one<HTMLButtonElement>('.guide-dock button[aria-label="Close the guide"]').click();
        flushSync();
        expect(guideState.value).toBe('closed');
        expect(all('.guide-dock')).toHaveLength(0);
        expect(one('.guide-chip').getAttribute('aria-label')).toBe('Guide 0/5');
      });

      And('opens Settings and chooses "Open the guide"', async () => {
        go('#/settings');
        expect(one('.guide-card').textContent).toContain('0 of 5 done');
        clickButton('.guide-card button', 'Open the guide');
        await tick();
      });

      Then('the dock is open again', () => {
        expect(one('.guide-dock')).toBeTruthy();
        expect(localStorage.getItem('sd-guide-state')).toBe('open');
        expect(document.activeElement).toBe(one('.guide-dock h2'));
      });
    },
  );

  Scenario('Nobody else is welcomed (G.6)', ({ Given, When, Then }) => {
    Given('a browser with a paired computer and a group', async () => {
      await clearSolodawnIdb();
      await seedOwnerRecord();
      workstation = ownedNode('workstation', 'J3Uuh-y1z2AX536yAFlGHJL5sjxVpWVJavvVphvdm5Q', {
        homes: [],
        agents: [makeAgent({ account: 'personal', defaults: { ...readyDefaults } })],
      });
      addStarterGroup(workstation, websiteId, 'Website');
      homes.push(workstation.store);
      await workstation.connect();
    });

    When('it opens the client', async () => {
      openPage('#/groups', 1440, 'fine');
      await waitDone(() => expect(find(websiteId)?.info).toBeDefined());
      expect(await isBrandNew()).toBe(false);
      await tick();
      flushSync();
    });

    Then('no welcome dialog, no dock and no chip are shown', () => {
      expect(all('dialog.welcome[open]')).toHaveLength(0);
      expect(all('.guide-dock')).toHaveLength(0);
      expect(all('.guide-chip')).toHaveLength(0);
    });

    Given('a browser that joined a group by invitation', async () => {
      await closePage();
      homes.length = 0;
      nodes.infos.clear();
      await clearSolodawnIdb();
      await saveJoined(() => indexedDB, {
        'friend-home': { name: 'Friend Home', url: 'https://friend.example' },
      });
    });

    When('the joined browser opens the client', async () => {
      openPage('#join=mock-invite-token', 1440, 'fine');
      await tick();
      flushSync();
      expect(all('dialog.welcome[open]')).toHaveLength(0);
      expect(all('.guide-dock')).toHaveLength(0);
      expect(all('.guide-chip')).toHaveLength(0);

      await closePage();
      openPage('#/groups', 1440, 'fine');
      expect(await isBrandNew()).toBe(false);
      await tick();
      flushSync();
    });

    Then('no welcome dialog is shown', () => {
      expect(all('dialog.welcome[open]')).toHaveLength(0);
      expect(all('.guide-dock')).toHaveLength(0);
      expect(all('.guide-chip')).toHaveLength(0);
    });
  });

  Scenario('Phone width (G.7)', ({ Given, Then, When }) => {
    Given('the guide is open at step 3 on a page 375 px wide', async () => {
      await clearSolodawnIdb();
      await seedOwnerRecord();
      guideState.value = 'open';
      workstation = ownedNode('workstation', 'J3Uuh-y1z2AX536yAFlGHJL5sjxVpWVJavvVphvdm5Q', {
        homes: [],
        agents: [makeAgent({ account: 'personal', defaults: undefined })],
      });
      addStarterGroup(workstation, websiteId, 'Website');
      homes.push(workstation.store);
      await workstation.connect();
      openPage('#/groups', 375, 'coarse');
      await waitDone(() => {
        expect(one('.guide-pill.phone')).toBeTruthy();
        expect(find(websiteId)?.info?.secretary).toBe(secAiId);
        expect(nodes.list(currentNet)[0]?.info).toBeDefined();
      });
    });

    Then(
      'the dock is a pill above the bottom navigation and no element is wider than the page',
      () => {
        const pill = one<HTMLButtonElement>('.guide-pill.phone');
        expect(pill).toBeTruthy();
        expect(document.documentElement.scrollWidth).toBeLessThanOrEqual(375);
        for (const el of all<HTMLElement>('.app, .app *')) {
          expect(el.getBoundingClientRect().right).toBeLessThanOrEqual(375);
        }
      },
    );

    When('the person taps the pill', async () => {
      one<HTMLButtonElement>('.guide-pill.phone').click();
      flushSync();
      await tick();
      expect(document.activeElement).toBe(one('dialog.guide-sheet h2'));
    });

    Then('the steps open as a bottom sheet with its close button', async () => {
      const sheet = one<HTMLDialogElement>('dialog.guide-sheet[open]');
      expect(sheet).toBeTruthy();
      expect(
        one<HTMLButtonElement>('dialog.guide-sheet button[aria-label="Close the guide"]'),
      ).toBeTruthy();

      // Backdrop click collapses back to the phone pill and restores focus on it.
      sheet.click();
      flushSync();
      await tick();
      expect(all('dialog.guide-sheet[open]')).toHaveLength(0);
      expect(document.activeElement).toBe(one('.guide-pill.phone'));

      // R1: clicking the sheet's [×] button closes the guide and shows the top-bar chip.
      one<HTMLButtonElement>('.guide-pill.phone').click();
      flushSync();
      await tick();
      one<HTMLButtonElement>('dialog.guide-sheet button[aria-label="Close the guide"]').click();
      flushSync();
      await tick();
      expect(all('dialog.guide-sheet[open]')).toHaveLength(0);
      expect(guideState.value).toBe('closed');
      expect(all('.guide-pill')).toHaveLength(0);
      expect(one('.guide-chip').getAttribute('aria-label')).toBe('Guide 3/5');

      // Reopen and collapse back to the phone pill, then verify the 1.2 s finish sheet delay.
      one<HTMLButtonElement>('.guide-chip').click();
      flushSync();
      await tick();
      one<HTMLDialogElement>('dialog.guide-sheet[open]').dispatchEvent(
        new Event('cancel', { cancelable: true }),
      );
      flushSync();
      await tick();
      setHostAgents([makeAgent({ account: 'personal', defaults: { ...readyDefaults } })]);
      vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout'] });
      workstation!.home.say(websiteId, owner, 'First request', { id: newId() });
      flushSync();
      expect(all('dialog.guide-sheet[open]')).toHaveLength(0);
      await vi.advanceTimersByTimeAsync(1199);
      flushSync();
      expect(all('dialog.guide-sheet[open]')).toHaveLength(0);
      await vi.advanceTimersByTimeAsync(1);
      flushSync();
      expect(one<HTMLDialogElement>('dialog.guide-sheet[open]')).toBeTruthy();
      vi.useRealTimers();
      one<HTMLDialogElement>('dialog.guide-sheet[open]').dispatchEvent(
        new Event('cancel', { cancelable: true }),
      );
      flushSync();
      await tick();
    });

    When('the person opens the chat of "Website"', () => {
      go(`#/groups/${websiteId}`);
    });

    Then('the pill is hidden', () => {
      expect(all('.guide-pill')).toHaveLength(0);
      go(`#/terminal/${websiteId}/${secAiId}`);
      expect(all('.guide-pill')).toHaveLength(0);
    });
  });

  Scenario('Reduced motion (G.8)', ({ Given, When, Then }) => {
    let unmountFresh: (() => Promise<void>) | undefined;
    let animateSpy: ReturnType<typeof vi.spyOn>;

    Given('the person prefers reduced motion', async () => {
      await clearSolodawnIdb();
      await seedOwnerRecord();
      vi.resetModules();
      vi.stubGlobal('matchMedia', (query: string) => ({
        matches: query.includes('prefers-reduced-motion: reduce') || query.includes('min-width'),
        media: query,
        addEventListener() {},
        removeEventListener() {},
        addListener() {},
        removeListener() {},
        dispatchEvent() {
          return false;
        },
      }));
      animateSpy = vi.spyOn(Element.prototype, 'animate');
    });

    When('a step is done and the beacon is shown', async () => {
      vi.stubGlobal(
        'IntersectionObserver',
        class {
          observe() {}
          disconnect() {}
        },
      );
      location.hash = '#/nodes';
      const svelte = await import('svelte');
      const groupsLib = await import('../src/lib/groups.svelte');
      vi.doMock('../src/lib/net.svelte', async (actual) => {
        const real = await actual<typeof import('../src/lib/net.svelte')>();
        return {
          ...real,
          start: (source: Source) => ({
            net: {
              source,
              homes: groupsLib.homes,
              path: (home: Home) => (home.ctl ? paths.get(home) : undefined),
            },
            stop: () => {},
          }),
        };
      });
      const { prefs } = await import('../src/lib/prefs.svelte');
      prefs.lang = 'en';
      const { default: App } = await import('../src/App.svelte');
      const guide = await import('../src/lib/guide.svelte');
      const nodesLib = await import('../src/lib/nodes.svelte');
      const beaconMod = await import('../src/components/guide/beacon');
      const homeMod = await import('./home');

      guide.guideState.value = 'open';
      workstation = homeMod.ownedNode(
        'workstation',
        'J3Uuh-y1z2AX536yAFlGHJL5sjxVpWVJavvVphvdm5Q',
        {
          homes: [],
          agents: [makeAgent({ account: 'personal', defaults: undefined })],
        },
      );
      groupsLib.homes.push(workstation.store);
      await workstation.connect();
      dispatchEvent(new HashChangeEvent('hashchange', { newURL: location.href }));

      const target = document.createElement('div');
      document.body.replaceChildren(target);
      const mounted = svelte.mount(App, { target });
      unmountFresh = async () => {
        await svelte.unmount(mounted);
        beaconMod.clearBeacon();
        groupsLib.homes.length = 0;
        nodesLib.nodes.infos.clear();
        guide.resetGuideUi();
        guide.guideState.value = 'new';
      };

      await vi.waitFor(() => {
        svelte.flushSync();
        expect(one('.ai-row [data-guide="defaults"][data-unset]')).toBeTruthy();
      });

      clickButton('.guide-dock .step.cur button', 'Set them');
      await vi.waitFor(() => {
        svelte.flushSync();
        expect(one('.guide-beacon')).toBeTruthy();
      });

      // Mark step 3 done while pill is shown so pill title swaps under reduced motion (0 ms out-fade).
      const key = [...nodesLib.nodes.infos.keys()][0]!;
      const prev = nodesLib.nodes.infos.get(key)!;
      nodesLib.nodes.infos.set(key, {
        ...prev,
        agents: [makeAgent({ account: 'personal', defaults: { ...readyDefaults } })],
      });
      svelte.flushSync();
    });

    Then('no element of the guide has a running animation', async () => {
      const beaconEl = one<HTMLElement>('.guide-beacon');
      expect(beaconEl.dataset.reduced).toBe('true');
      expect(all('.guide-beacon-pulse')).toHaveLength(0);
      const guideEls = all<HTMLElement>(
        '.guide-pill, .guide-pill *, .guide-beacon, .guide-beacon *',
      );
      expect(guideEls.length).toBeGreaterThan(0);
      for (const el of guideEls) {
        expect(el.getAnimations?.() ?? []).toEqual([]);
      }
      expect(animateSpy).not.toHaveBeenCalled();
      await unmountFresh?.();
    });
  });
});

describe('guide layout, focus, and reduced motion in Chromium', () => {
  let server: ViteDevServer;
  let browser: Browser;

  beforeAll(async () => {
    server = await createServer({ logLevel: 'warn', server: { host: '127.0.0.1', port: 0 } });
    await server.listen();
    browser = await chromium.launch({ executablePath: process.env.CHROMIUM_PATH });
  }, 30_000);

  afterAll(async () => {
    await browser?.close();
    await server?.close();
  });

  async function seedBrowserGuide(
    page: Page,
    opts: { defaultsSet: boolean; hasGroup: boolean; collapsed: boolean; workingAis?: number },
  ) {
    await page.evaluate(
      `(async () => {
        const opts = ${JSON.stringify(opts)};
        const { homes } = await import('/src/lib/groups.svelte.ts');
        const { guideState, guideUi } = await import('/src/lib/guide.svelte.ts');
        const { ownedNode, example, owner, user } = await import('/features/home.ts');
        const { DeviceLog } = await import('/src/lib/devlog.ts');
        const { Device, ed25519 } = await import('/src/lib/identity.ts');
        const { saveOwner } = await import('/src/lib/owner.ts');

        const pair = await crypto.subtle.generateKey(ed25519, false, ['sign', 'verify']);
        const dev = await Device.of(crypto, pair);
        const env = await dev.sign(
          JSON.stringify({
            t: 'device',
            n: 'AAAAAAAAAAAAAAAAAAAAAA',
            ts: 1,
            user: dev.id,
            ver: 1,
            op: 'add',
            dev: dev.id,
            kind: 'browser',
            name: 'Chromium Test',
          }),
        );
        const log = new DeviceLog(crypto, dev.id);
        await log.apply(env);
        await saveOwner(() => indexedDB, log, {});

        const ready = { model: 'sonnet', effort: 'medium', tier: 'standard', skills: 'coding' };
        const rig = ownedNode('workstation', 'J3Uuh-y1z2AX536yAFlGHJL5sjxVpWVJavvVphvdm5Q', {
          homes: [],
          agents: [
            {
              agent: 'claude',
              cli_version: '1.0.0',
              account: 'personal',
              login_ok: true,
              driver: { accounts: ['personal', 'team', 'api'], takeover: 'none' },
              models: [],
              caps: {},
              defaults: opts.defaultsSet ? ready : undefined,
              groups: [],
            },
          ],
        });
        const g = 'rZNvy-1jH6Z-BcPqA5U5BQ';
        const secId = '1PC8WineBrUQ-apCjx7tug';
        if (opts.hasGroup) {
          const workers = opts.workingAis ?? 2;
          rig.home.add(g, 'Website');
          rig.home.write(g, example('row-member-human', { id: owner, g, user, name: 'Alex' }));
          rig.home.write(g, example('row-member-ai', { id: secId, g, user, name: 'Claude Code' }));
          if (workers >= 1) {
            rig.home.write(
              g,
              example('row-member-ai', { id: 'worker-ai-1', g, user, name: 'Codex', cli: 'codex' }),
            );
          }
          if (workers >= 2) {
            rig.home.write(
              g,
              example('row-member-ai', { id: 'worker-ai-2', g, user, name: 'Worker 2', cli: 'claude' }),
            );
          }
          rig.home.write(g, example('row-group', { g, name: 'Website', secretary: secId }));
        }
        homes.push(rig.store);
        await rig.connect();
        guideState.value = 'open';
        guideUi.expanded = !opts.collapsed;
      })()`,
    );
  }

  it.each([768, 1024])(
    'collapsed pill does not cover composer send button on chat at %d px',
    async (width) => {
      const ctx = await browser.newContext({ viewport: { width, height: 900 }, locale: 'en-US' });
      const page = await ctx.newPage();
      await page.goto(`${server.resolvedUrls!.local[0]}?nomock#/groups/rZNvy-1jH6Z-BcPqA5U5BQ`);
      await seedBrowserGuide(page, { defaultsSet: true, hasGroup: true, collapsed: true });
      await page.locator('.guide-pill').waitFor();
      await page.locator('.composer .send').waitFor();

      const rects = await page.evaluate(() => {
        const pill = document.querySelector('.guide-pill')!.getBoundingClientRect();
        const send = document.querySelector('.composer .send')!.getBoundingClientRect();
        return {
          scrollWidth: document.documentElement.scrollWidth,
          pill: { left: pill.left, top: pill.top, right: pill.right, bottom: pill.bottom },
          send: { left: send.left, top: send.top, right: send.right, bottom: send.bottom },
        };
      });
      expect(rects.scrollWidth).toBeLessThanOrEqual(width);
      const disjoint =
        rects.pill.bottom <= rects.send.top ||
        rects.pill.top >= rects.send.bottom ||
        rects.pill.right <= rects.send.left ||
        rects.pill.left >= rects.send.right;
      expect(disjoint).toBe(true);
      await ctx.close();
    },
  );

  it('375 px phone stays within viewport width and preserves sheet/pill focus', async () => {
    const ctx = await browser.newContext({
      viewport: { width: 375, height: 812 },
      hasTouch: true,
      locale: 'en-US',
    });
    const page = await ctx.newPage();
    await page.goto(`${server.resolvedUrls!.local[0]}?nomock#/groups`);
    await seedBrowserGuide(page, { defaultsSet: false, hasGroup: true, collapsed: true });
    await page.locator('.guide-pill.phone').waitFor();
    await page.evaluate(async () => {
      await document.fonts.ready;
      await new Promise((frame) => requestAnimationFrame(frame));
      const ending = document
        .getAnimations()
        .filter((a) => a.effect?.getComputedTiming().endTime !== Infinity);
      await Promise.all(ending.map((a) => a.finished.catch(() => undefined)));
    });

    const overflow = await page.evaluate(() => ({
      scrollWidth: document.documentElement.scrollWidth,
      maxRight: Math.max(
        ...[...document.querySelectorAll('.app, .app *')].map(
          (el) => el.getBoundingClientRect().right,
        ),
      ),
    }));
    expect(overflow.scrollWidth).toBeLessThanOrEqual(375);
    expect(overflow.maxRight).toBeLessThanOrEqual(375);

    await page.locator('.guide-pill.phone').click();
    await page.locator('dialog.guide-sheet[open]').waitFor();
    await page.keyboard.press('Escape');
    await page.locator('dialog.guide-sheet[open]').waitFor({ state: 'hidden' });
    expect(
      await page.evaluate(() => document.activeElement?.classList.contains('guide-pill')),
    ).toBe(true);

    await page.locator('.guide-pill.phone').click();
    await page.locator('dialog.guide-sheet[open]').waitFor();
    await page.locator('dialog.guide-sheet button[aria-label="Close the guide"]').click();
    await page.locator('dialog.guide-sheet[open]').waitFor({ state: 'hidden' });
    expect(await page.evaluate(() => localStorage.getItem('sd-guide-state'))).toBe('closed');
    expect(await page.locator('.guide-pill').count()).toBe(0);
    await page.locator('.guide-chip').waitFor();
    await ctx.close();
  });

  it('emulated reduced-motion produces zero running CSS/WAAPI animations on guide and beacon', async () => {
    const ctx = await browser.newContext({
      viewport: { width: 1440, height: 900 },
      locale: 'en-US',
    });
    const page = await ctx.newPage();
    await page.emulateMedia({ reducedMotion: 'reduce' });
    await page.goto(`${server.resolvedUrls!.local[0]}?nomock#/nodes`);
    await seedBrowserGuide(page, { defaultsSet: false, hasGroup: false, collapsed: false });
    await page.locator('.guide-dock .step.cur button').click();
    await page.locator('.guide-beacon[data-reduced="true"]').waitFor();

    const check = await page.evaluate(() => {
      const els = [
        ...document.querySelectorAll('.guide-pill, .guide-pill *, .guide-beacon, .guide-beacon *'),
      ];
      return {
        pulseCount: document.querySelectorAll('.guide-beacon-pulse').length,
        runningAnimations: els.flatMap((el) => el.getAnimations()).length,
      };
    });
    expect(check.pulseCount).toBe(0);
    expect(check.runningAnimations).toBe(0);
    await ctx.close();
  });

  async function seedOnboardingGuide(page: Page, withAgents: boolean) {
    await page.evaluate(
      `(async () => {
        const withAgents = ${JSON.stringify(withAgents)};
        const { guideState, guideUi } = await import('/src/lib/guide.svelte.ts');
        guideState.value = 'open';
        guideUi.expanded = true;
        if (!withAgents) return;
        const wrap = document.querySelector('.onboarding .wrap > div:nth-child(2)');
        if (!wrap) return;
        const card = document.createElement('section');
        card.className = 'card';
        const fs = document.createElement('fieldset');
        fs.className = 'acct';
        fs.textContent = 'Account type';
        card.appendChild(fs);
        const nextBtn = document.createElement('a');
        nextBtn.className = 'btn btn-dawn done-btn';
        nextBtn.textContent = "Next: set your AIs' defaults";
        wrap.append(card, nextBtn);
      })()`,
    );
  }

  async function seedTwoNodesGuide(page: Page) {
    await page.evaluate(
      `(async () => {
        const { homes } = await import('/src/lib/groups.svelte.ts');
        const { guideState, guideUi } = await import('/src/lib/guide.svelte.ts');
        const { ownedNode } = await import('/features/home.ts');
        const rig = ownedNode('workstation', 'J3Uuh-y1z2AX536yAFlGHJL5sjxVpWVJavvVphvdm5Q', {
          homes: [],
          agents: [
            {
              agent: 'claude',
              cli_version: '1.0.0',
              account: 'personal',
              login_ok: true,
              driver: { accounts: ['personal', 'team', 'api'], takeover: 'none' },
              models: [],
              caps: {},
              defaults: undefined,
              groups: [],
            },
            {
              agent: 'codex',
              cli_version: '0.164.0',
              account: 'personal',
              login_ok: true,
              driver: { accounts: ['personal', 'team', 'api'], takeover: 'none' },
              models: [],
              caps: {},
              defaults: undefined,
              groups: [],
            },
          ],
        });
        homes.push(rig.store);
        await rig.connect();
        guideState.value = 'open';
        guideUi.expanded = true;
      })()`,
    );
  }

  it.each([
    { width: 1440, height: 900 },
    { width: 1280, height: 800 },
  ])(
    'hides both guide dock and pill on #/onboarding at $width x $height before and after pairing cards appear',
    async ({ width, height }) => {
      const ctx = await browser.newContext({ viewport: { width, height }, locale: 'en-US' });
      const page = await ctx.newPage();
      await page.goto(`${server.resolvedUrls!.local[0]}?nomock#/onboarding`);
      await seedOnboardingGuide(page, false);
      await page.locator('#prompt').waitFor();
      expect(await page.locator('.guide-dock').count()).toBe(0);
      expect(await page.locator('.guide-pill').count()).toBe(0);

      // Account type cards and Next button arriving keep both dock and pill hidden on #/onboarding.
      await seedOnboardingGuide(page, true);
      await page.locator('.onboarding .done-btn').waitFor();
      expect(await page.locator('.guide-dock').count()).toBe(0);
      expect(await page.locator('.guide-pill').count()).toBe(0);
      await ctx.close();
    },
  );

  it.each([
    { width: 1440, height: 900 },
    { width: 1280, height: 800 },
  ])(
    'auto-collapses on #/nodes at $width x $height over Set defaults and Save editor, and expands only when opened',
    async ({ width, height }) => {
      const ctx = await browser.newContext({ viewport: { width, height }, locale: 'en-US' });
      const page = await ctx.newPage();
      await page.goto(`${server.resolvedUrls!.local[0]}?nomock#/nodes`);
      await seedTwoNodesGuide(page);
      await page.locator('[data-guide="defaults"][data-unset]').first().waitFor();
      await page.locator('.guide-pill').waitFor();
      expect(await page.locator('.guide-dock').count()).toBe(0);

      // Click first Set defaults -> Save editor opens cleanly uncovered; guide stays collapsed.
      await page.locator('[data-guide="defaults"][data-unset]').first().click();
      await page.locator('.defaults .editor .btn-primary').waitFor();
      expect(await page.locator('.guide-dock').count()).toBe(0);

      // Person can still reopen pill and trigger beacon cleanly.
      await page.locator('.guide-pill').click();
      await page.locator('.guide-dock').waitFor();
      await page.locator('.guide-dock .step.cur button').click();
      await page.locator('.guide-pill').waitFor();
      await page.locator('.guide-beacon').waitFor();
      await ctx.close();
    },
  );

  it('auto-collapses when opening Save editor on 1-AI #/nodes at 1440x900 and collapses on resize to 1280x800', async () => {
    const ctx = await browser.newContext({
      viewport: { width: 1440, height: 900 },
      locale: 'en-US',
    });
    const page = await ctx.newPage();
    await page.goto(`${server.resolvedUrls!.local[0]}?nomock#/nodes`);
    await seedBrowserGuide(page, { defaultsSet: false, hasGroup: false, collapsed: false });
    await page.locator('.guide-dock').waitFor();
    await page.locator('[data-guide="defaults"][data-unset]').click();
    await page.locator('.defaults .editor .btn-primary').waitFor();
    await page.locator('.guide-pill').waitFor();
    expect(await page.locator('.guide-dock').count()).toBe(0);

    // Closing editor keeps it collapsed (expands only when person opens it).
    await page.locator('.defaults .editor button', { hasText: 'Cancel' }).click();
    await page.locator('.defaults .editor').waitFor({ state: 'hidden' });
    expect(await page.locator('.guide-pill').count()).toBe(1);
    expect(await page.locator('.guide-dock').count()).toBe(0);

    // Reopen at 1440x900 (no overlap on 1 AI), then resize to 1280x800 -> overlaps and auto-collapses.
    await page.locator('.guide-pill').click();
    await page.locator('.guide-dock').waitFor();
    await page.setViewportSize({ width: 1280, height: 800 });
    await page.locator('.guide-pill').waitFor();
    expect(await page.locator('.guide-dock').count()).toBe(0);
    await ctx.close();
  });

  it('1279 px chat pill rule keeps collapsed pill clear of composer send button', async () => {
    const ctx = await browser.newContext({
      viewport: { width: 1279, height: 900 },
      locale: 'en-US',
    });
    const page = await ctx.newPage();
    await page.goto(`${server.resolvedUrls!.local[0]}?nomock#/groups/rZNvy-1jH6Z-BcPqA5U5BQ`);
    await seedBrowserGuide(page, { defaultsSet: true, hasGroup: true, collapsed: false });
    await page.locator('.guide-pill.over-chat').waitFor();
    await page.locator('.composer .send').waitFor();

    const rects = await page.evaluate(() => {
      const pill = document.querySelector('.guide-pill')!.getBoundingClientRect();
      const send = document.querySelector('.composer .send')!.getBoundingClientRect();
      return {
        pill: { left: pill.left, top: pill.top, right: pill.right, bottom: pill.bottom },
        send: { left: send.left, top: send.top, right: send.right, bottom: send.bottom },
      };
    });
    const disjoint =
      rects.pill.bottom <= rects.send.top ||
      rects.pill.top >= rects.send.bottom ||
      rects.pill.right <= rects.send.left ||
      rects.pill.left >= rects.send.right;
    expect(disjoint).toBe(true);
    await ctx.close();
  });

  it.each([
    { width: 768, targetSelector: '.side-open', hiddenSelector: '[data-guide="add-ai"]' },
    { width: 1440, targetSelector: '[data-guide="add-ai"]', hiddenSelector: '.side-open' },
  ])(
    'step 4 Add AI rings visible $targetSelector at $width px while $hiddenSelector is hidden',
    async ({ width, targetSelector, hiddenSelector }) => {
      const ctx = await browser.newContext({ viewport: { width, height: 900 }, locale: 'en-US' });
      const page = await ctx.newPage();
      await page.goto(`${server.resolvedUrls!.local[0]}?nomock#/groups`);
      await seedBrowserGuide(page, {
        defaultsSet: true,
        hasGroup: true,
        workingAis: 1,
        collapsed: false,
      });
      await page.locator('.guide-dock .step.cur button', { hasText: 'Add AI' }).click();
      await page.locator('.guide-beacon').waitFor();

      const info = await page.evaluate(
        ({ visSel, hidSel }) => {
          const vis = document.querySelector<HTMLElement>(visSel)!;
          const hid = document.querySelector<HTMLElement>(hidSel)!;
          const b = document.querySelector<HTMLElement>('.guide-beacon')!.getBoundingClientRect();
          const t = vis.getBoundingClientRect();
          return {
            visOk: vis.checkVisibility({ visibilityProperty: true }),
            hidOk: hid.checkVisibility({ visibilityProperty: true }),
            beacon: { left: b.left, top: b.top, right: b.right, bottom: b.bottom },
            target: { left: t.left, top: t.top, right: t.right, bottom: t.bottom },
          };
        },
        { visSel: targetSelector, hidSel: hiddenSelector },
      );

      expect(info.visOk).toBe(true);
      expect(info.hidOk).toBe(false);
      expect(info.beacon.left).toBeCloseTo(info.target.left - 4, 1);
      expect(info.beacon.top).toBeCloseTo(info.target.top - 4, 1);
      expect(info.beacon.right).toBeLessThanOrEqual(width);
      await ctx.close();
    },
  );
});

describe('guide overlap auto-collapse in happy-dom', () => {
  afterEach(async () => {
    vi.restoreAllMocks();
    await closePage();
    clearBeacon();
    resetGuideUi();
    homes.length = 0;
    paths.clear();
    nodes.infos.clear();
    localStorage.clear();
    guideState.value = 'new';
  });

  it('auto-collapses when dock overlaps target, preserves focus, and restores pill focus if dock held focus', async () => {
    await clearSolodawnIdb();
    await seedOwnerRecord();
    guideState.value = 'open';
    const rig = ownedNode('workstation', 'J3Uuh-y1z2AX536yAFlGHJL5sjxVpWVJavvVphvdm5Q', {
      homes: [],
      agents: [makeAgent({ account: 'personal', defaults: undefined })],
    });
    homes.push(rig.store);
    await rig.connect();

    const rectSpy = vi
      .spyOn(HTMLElement.prototype, 'getBoundingClientRect')
      .mockImplementation(function (this: HTMLElement) {
        if (this.classList.contains('guide-dock')) {
          return {
            left: 1000,
            top: 450,
            right: 1344,
            bottom: 850,
            width: 344,
            height: 400,
          } as DOMRect;
        }
        if (this.matches('[data-guide="defaults"][data-unset], .defaults .editor')) {
          return {
            left: 1010,
            top: 500,
            right: 1120,
            bottom: 540,
            width: 110,
            height: 40,
          } as DOMRect;
        }
        return { left: 0, top: 0, right: 0, bottom: 0, width: 0, height: 0 } as DOMRect;
      });

    openPage('#/nodes', 1440, 'fine');
    await waitDone(() => expect(all('.guide-pill')).toHaveLength(1));
    expect(all('.guide-dock')).toHaveLength(0);
    expect(guideUi.expanded).toBe(false);

    // Reopening via pill focuses dock heading; mutating main while dock holds focus returns focus to pill.
    one<HTMLButtonElement>('.guide-pill').click();
    flushSync();
    await tick();
    expect(all('.guide-dock')).toHaveLength(1);
    expect(document.activeElement).toBe(one('.guide-dock h2'));

    one('main').appendChild(document.createElement('div'));
    await waitDone(() => expect(all('.guide-pill')).toHaveLength(1));
    await tick();
    expect(document.activeElement).toBe(one('.guide-pill'));
    rectSpy.mockRestore();
  });

  it('keeps active focus on a main control when DOM mutation causes overlap without pointer or focusin', async () => {
    await clearSolodawnIdb();
    await seedOwnerRecord();
    guideState.value = 'open';
    const rig = ownedNode('workstation', 'J3Uuh-y1z2AX536yAFlGHJL5sjxVpWVJavvVphvdm5Q', {
      homes: [],
      agents: [makeAgent({ account: 'personal', defaults: undefined })],
    });
    homes.push(rig.store);
    await rig.connect();

    const rectSpy = vi
      .spyOn(HTMLElement.prototype, 'getBoundingClientRect')
      .mockImplementation(function (this: HTMLElement) {
        if (this.classList.contains('guide-dock')) {
          return {
            left: 1000,
            top: 450,
            right: 1344,
            bottom: 850,
            width: 344,
            height: 400,
          } as DOMRect;
        }
        if (this.matches('[data-guide="defaults"][data-unset]')) {
          return {
            left: 100,
            top: 100,
            right: 220,
            bottom: 132,
            width: 120,
            height: 32,
          } as DOMRect;
        }
        if (this.matches('.defaults .editor')) {
          return {
            left: 1010,
            top: 500,
            right: 1200,
            bottom: 600,
            width: 190,
            height: 100,
          } as DOMRect;
        }
        return { left: 0, top: 0, right: 0, bottom: 0, width: 0, height: 0 } as DOMRect;
      });

    openPage('#/nodes', 1440, 'fine');
    await waitDone(() => expect(all('main [data-guide="defaults"][data-unset]')).toHaveLength(1));
    expect(all('.guide-dock')).toHaveLength(1);

    const mainBtn = one<HTMLButtonElement>('main [data-guide="defaults"][data-unset]');
    mainBtn.focus();
    await tick();
    expect(document.activeElement).toBe(mainBtn);
    expect(all('.guide-dock')).toHaveLength(1);

    // Trigger overlap strictly via a DOM mutation (MutationObserver), with no pointerdown/focusin.
    const defaultsWrap = document.createElement('div');
    defaultsWrap.className = 'defaults';
    const editor = document.createElement('div');
    editor.className = 'editor';
    defaultsWrap.appendChild(editor);
    one('main').appendChild(defaultsWrap);

    await waitDone(() => expect(all('.guide-pill')).toHaveLength(1));
    await tick();
    expect(all('.guide-dock')).toHaveLength(0);
    expect(document.activeElement).toBe(mainBtn);
    rectSpy.mockRestore();
  });
});
