import 'fake-indexeddb/auto';
import { IDBFactory } from 'fake-indexeddb';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { DeviceLog } from './devlog';
import type { Group } from './groups.svelte';
import {
  autoCollapseIfOverlapping,
  boxesOverlap,
  canCollapseOnEscape,
  canWatchDock,
  clearUserOpened,
  closeGuide,
  collapseGuide,
  createAnnouncer,
  createOverlapTracker,
  dockOverlapsTargets,
  expandGuide,
  finishGuide,
  guideState,
  guideStates,
  guideSteps,
  guideUi,
  type GuideStep,
  isBrandNew,
  isUserOpened,
  modal,
  observeDockLayout,
  openGuide,
  type OverlapTargetContext,
  resetGuideUi,
  sameTargetContext,
  stepActionLabel,
  stepDoneText,
  stepIntent,
  stepTargetSelector,
} from './guide.svelte';
import { Device, ed25519 } from './identity';
import type { Computer } from './nodes.svelte';
import { cliName } from './onboard.svelte';
import { saveJoined, saveOwner } from './owner';
import { prefs } from './prefs.svelte';
import type { HostInfoPush } from './proto';

type Agent = HostInfoPush['agents'][number];

const validDefaults = {
  model: 'claude-sonnet',
  effort: 'medium',
  tier: 'standard',
  skills: 'coding',
};

function agent(overrides: Partial<Agent> = {}): Agent {
  return {
    agent: 'claude',
    account: 'personal',
    defaults: { ...validDefaults },
    ...overrides,
  } as Agent;
}

function computer(
  name: string,
  opts: {
    online?: boolean;
    infoName?: string | null;
    agents?: Agent[];
    noInfo?: boolean;
    home?: object;
  } = {},
): Computer {
  const { online = true, infoName = name, agents = [], noInfo = false } = opts;
  return {
    home: opts.home ?? { id: `${name}-id`, name, key: 'k', addrs: [], seq: 1 },
    online,
    info: noInfo || infoName === null ? null : { name: infoName, agents },
  } as unknown as Computer;
}

function group(
  g: string,
  opts: {
    name?: string;
    secretary?: string | null;
    noInfo?: boolean;
    me?: { id: string; user: string; kind?: 'human' | 'ai' } | null;
    members?: Array<{ id: string; user: string; kind?: 'human' | 'ai' }>;
    main?: Array<{ kind: string; thread: string | null; by: string }>;
  } = {},
): Group {
  const me = opts.me === undefined ? { id: 'm-me', user: 'u-me', kind: 'human' as const } : opts.me;
  const members = opts.members ?? [
    ...(me ? [{ id: me.id, user: me.user, kind: me.kind ?? ('human' as const) }] : []),
    { id: 'm-sec', user: 'u-me', kind: 'ai' as const },
  ];
  return {
    g,
    name: opts.name ?? `Group ${g}`,
    info: opts.noInfo
      ? null
      : { secretary: opts.secretary === undefined ? 'm-sec' : opts.secretary },
    me,
    members,
    main: opts.main ?? [],
  } as unknown as Group;
}

async function createOwnerLog(): Promise<DeviceLog> {
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
      name: 'Guide Test',
    }),
  );
  const log = new DeviceLog(crypto, dev.id);
  await log.apply(env);
  return log;
}

describe('guide state & timers', () => {
  beforeEach(() => {
    vi.useFakeTimers();
    localStorage.clear();
    prefs.lang = 'en';
    resetGuideUi();
    guideState.value = 'new';
  });

  afterEach(() => {
    resetGuideUi();
    vi.useRealTimers();
    localStorage.clear();
  });

  it('initializes defaults on fresh module load and persists sd-guide-state', async () => {
    vi.resetModules();
    const fresh = await import('./guide.svelte');
    expect(fresh.guideUi).toEqual({ expanded: true, closedNote: false, focusHeading: false });
    expect(fresh.isUserOpened()).toBe(false);
    expect(fresh.guideState.value).toBe('new');

    fresh.guideState.value = 'open';
    expect(localStorage.getItem('sd-guide-state')).toBe('open');

    localStorage.setItem('sd-guide-state', 'bad');
    vi.resetModules();
    const fallback = await import('./guide.svelte');
    expect(fallback.guideState.value).toBe('new');
  });

  it('persists sd-guide-state in localStorage across valid values', () => {
    expect(guideStates).toEqual(['new', 'open', 'closed', 'done']);
    expect(guideState.value).toBe('new');

    for (const next of guideStates) {
      guideState.value = next;
      expect(guideState.value).toBe(next);
      expect(localStorage.getItem('sd-guide-state')).toBe(next);
    }

    localStorage.setItem('sd-guide-state', 'unknown');
    expect(guideState.value).toBe('done');
  });

  it('manages open, collapse, expand, finish, reset, and cancels the 4 s closed note timer', async () => {
    closeGuide();
    expect(guideState.value).toBe('closed');
    expect(guideUi.expanded).toBe(false);
    expect(guideUi.closedNote).toBe(true);

    await vi.advanceTimersByTimeAsync(3999);
    expect(guideUi.closedNote).toBe(true);
    await vi.advanceTimersByTimeAsync(1);
    expect(guideUi.closedNote).toBe(false);

    closeGuide();
    await vi.advanceTimersByTimeAsync(3500);
    closeGuide();
    await vi.advanceTimersByTimeAsync(3999);
    expect(guideUi.closedNote).toBe(true);
    await vi.advanceTimersByTimeAsync(1);
    expect(guideUi.closedNote).toBe(false);

    closeGuide();
    await vi.advanceTimersByTimeAsync(2000);
    openGuide();
    expect(guideState.value).toBe('open');
    expect(guideUi.expanded).toBe(true);
    expect(guideUi.focusHeading).toBe(true);
    expect(guideUi.closedNote).toBe(false);
    guideUi.closedNote = true;
    await vi.advanceTimersByTimeAsync(2000);
    expect(guideUi.closedNote).toBe(true);
    guideUi.closedNote = false;

    collapseGuide();
    expect(guideUi.expanded).toBe(false);
    expect(guideUi.focusHeading).toBe(false);
    expandGuide();
    expect(guideUi.expanded).toBe(true);
    expect(guideUi.focusHeading).toBe(false);
    expandGuide(true);
    expect(guideUi.expanded).toBe(true);
    expect(guideUi.focusHeading).toBe(true);

    openGuide();
    expect(guideUi.focusHeading).toBe(true);
    closeGuide();
    expect(guideUi.focusHeading).toBe(false);
    await vi.advanceTimersByTimeAsync(2000);
    finishGuide();
    expect(guideState.value).toBe('done');
    expect(guideUi.closedNote).toBe(false);
    expect(guideUi.focusHeading).toBe(false);
    guideUi.closedNote = true;
    await vi.advanceTimersByTimeAsync(2000);
    expect(guideUi.closedNote).toBe(true);
    guideUi.closedNote = false;
    openGuide();
    expect(guideUi.focusHeading).toBe(true);
    finishGuide();
    expect(guideUi.focusHeading).toBe(false);

    closeGuide();
    await vi.advanceTimersByTimeAsync(2000);
    resetGuideUi();
    expect(guideUi.expanded).toBe(true);
    expect(guideUi.closedNote).toBe(false);
    expect(guideUi.focusHeading).toBe(false);
    guideUi.closedNote = true;
    await vi.advanceTimersByTimeAsync(2000);
    expect(guideUi.closedNote).toBe(true);
    openGuide();
    expect(guideUi.focusHeading).toBe(true);
    resetGuideUi();
    expect(guideUi.focusHeading).toBe(false);
    guideUi.closedNote = true;
    await vi.advanceTimersByTimeAsync(2000);
    expect(guideUi.closedNote).toBe(true);
  });
});

describe('modal action', () => {
  it('opens closed dialogs on mount and closes open dialogs on destroy', () => {
    let isOpen = false;
    const dlg = {
      get open() {
        return isOpen;
      },
      showModal: vi.fn(() => {
        isOpen = true;
      }),
      close: vi.fn(() => {
        isOpen = false;
      }),
    } as unknown as HTMLDialogElement;

    const first = modal(dlg);
    expect(dlg.showModal).toHaveBeenCalledTimes(1);
    first.destroy();
    expect(dlg.close).toHaveBeenCalledTimes(1);

    isOpen = true;
    const second = modal(dlg);
    expect(dlg.showModal).toHaveBeenCalledTimes(1);
    isOpen = false;
    second.destroy();
    expect(dlg.close).toHaveBeenCalledTimes(1);
  });
});

describe('step intents, Escape guard, labels, and announcements', () => {
  it('maps each step to its navigation or refresh intent', () => {
    expect(stepIntent({ id: 'connect', done: false, name: '' }, undefined)).toEqual({
      kind: 'go',
      hash: '#/onboarding',
    });
    expect(
      stepIntent({ id: 'ais', done: false, count: 0, none: true, offline: undefined }, undefined),
    ).toEqual({ kind: 'refresh' });
    expect(
      stepIntent({ id: 'ais', done: false, count: 1, none: false, offline: undefined }, undefined),
    ).toEqual({
      kind: 'go',
      hash: '#/nodes',
      selector: '.ai-row fieldset.acct',
    });
    expect(stepIntent({ id: 'defaults', done: false, left: ['Codex'] }, undefined)).toEqual({
      kind: 'go',
      hash: '#/nodes',
      selector: '[data-guide="defaults"][data-unset]',
    });
    expect(
      stepIntent({ id: 'group', done: false, name: undefined, g: undefined }, undefined),
    ).toEqual({
      kind: 'go',
      hash: '#/groups',
      create: true,
    });
    expect(stepIntent({ id: 'ask', done: false }, 'g-1')).toEqual({
      kind: 'go',
      hash: '#/groups/g-1',
      selector: '.composer:not(.in-thread)',
    });
    expect(stepIntent({ id: 'ask', done: false }, undefined)).toBeUndefined();
  });

  it('allows Escape collapse only when open, unprevented, and focused inside the dock', () => {
    const inside = document.createElement('button');
    const outside = document.createElement('button');
    const dock = document.createElement('aside');
    dock.appendChild(inside);

    expect(
      canCollapseOnEscape({ key: 'Escape', defaultPrevented: false }, dock, inside, true),
    ).toBe(true);
    expect(
      canCollapseOnEscape({ key: 'Escape', defaultPrevented: false }, dock, inside, false),
    ).toBe(false);
    expect(canCollapseOnEscape({ key: 'Enter', defaultPrevented: false }, dock, inside, true)).toBe(
      false,
    );
    expect(canCollapseOnEscape({ key: 'Escape', defaultPrevented: true }, dock, inside, true)).toBe(
      false,
    );
    expect(
      canCollapseOnEscape({ key: 'Escape', defaultPrevented: false }, undefined, inside, true),
    ).toBe(false);
    expect(
      canCollapseOnEscape({ key: 'Escape', defaultPrevented: false }, dock, outside, true),
    ).toBe(false);
  });

  it('formats localized completion summaries and action labels', () => {
    prefs.lang = 'en';
    expect(stepDoneText({ id: 'connect', done: true, name: 'Mini' })).toBe('Connected "Mini"');
    expect(stepDoneText({ id: 'ais', done: true, count: 1, none: false, offline: undefined })).toBe(
      '1 AI ready',
    );
    expect(stepDoneText({ id: 'ais', done: true, count: 2, none: false, offline: undefined })).toBe(
      '2 AIs ready',
    );
    expect(stepDoneText({ id: 'defaults', done: true, left: [] })).toBe('Defaults set');
    expect(stepDoneText({ id: 'group', done: true, name: 'Website', g: 'g1' })).toBe(
      'Created "Website"',
    );
    expect(stepDoneText({ id: 'group', done: true, name: undefined, g: 'g1' })).toBe('Created ""');
    expect(stepDoneText({ id: 'ask', done: true })).toBe('Sent');

    expect(stepActionLabel({ id: 'connect', done: false, name: '' })).toBe('Connect');
    expect(
      stepActionLabel({ id: 'ais', done: false, count: 0, none: true, offline: undefined }),
    ).toBe('Check again');
    expect(
      stepActionLabel({ id: 'ais', done: false, count: 1, none: false, offline: undefined }),
    ).toBe('Check them');
    expect(stepActionLabel({ id: 'defaults', done: false, left: [] })).toBe('Set them');
    expect(stepActionLabel({ id: 'group', done: false, name: undefined, g: undefined })).toBe(
      'Create a group',
    );
    expect(stepActionLabel({ id: 'ask', done: false })).toBe('Open the group');
  });

  it('suppresses Done announcements on initial load and offline-online flips until host.info is seen', () => {
    const home1 = { id: 'h1' };
    const home2 = { id: 'h2' };
    const announcer = createAnnouncer(guideSteps([], []), []);

    const onlineUnset = computer('Mini', {
      home: home1,
      online: true,
      agents: [agent({ account: 'personal', defaults: undefined })],
    });
    expect(announcer.next(true, guideSteps([onlineUnset], []), [onlineUnset])).toBeUndefined();

    const onlineReady = computer('Mini', {
      home: home1,
      online: true,
      agents: [agent()],
    });
    expect(announcer.next(true, guideSteps([onlineReady], []), [onlineReady])?.id).toBe('defaults');

    // Offline while keeping stale host.info must remove home1 from seen, so reconnect stays silent.
    const offlineWithInfo = computer('Mini', {
      home: home1,
      online: false,
      agents: [agent({ account: 'personal', defaults: undefined })],
    });
    expect(
      announcer.next(true, guideSteps([offlineWithInfo], []), [offlineWithInfo]),
    ).toBeUndefined();
    expect(announcer.next(true, guideSteps([onlineReady], []), [onlineReady])).toBeUndefined();

    // Online without host.info (missing info while online) must also disarm and stay silent.
    const onlineNoInfo = computer('Mini', { home: home1, online: true, noInfo: true });
    expect(announcer.next(true, guideSteps([onlineNoInfo], []), [onlineNoInfo])).toBeUndefined();
    expect(announcer.next(true, guideSteps([onlineReady], []), [onlineReady])).toBeUndefined();

    // Dropping unseen second computer while first finishes must stay silent (armed was false).
    const unseenC2 = computer('Desk', { home: home2, online: true, noInfo: true });
    const twoHosts = createAnnouncer(guideSteps([onlineUnset, unseenC2], []), [
      onlineUnset,
      unseenC2,
    ]);
    expect(twoHosts.next(true, guideSteps([onlineReady], []), [onlineReady])).toBeUndefined();

    // Unseen second computer alongside seen ready first computer blocks announcement until seen.
    const unseenExtra = createAnnouncer(guideSteps([onlineUnset], []), [onlineUnset]);
    expect(
      unseenExtra.next(true, guideSteps([onlineReady, unseenC2], []), [onlineReady, unseenC2]),
    ).toBeUndefined();

    const starter = group('g1');
    expect(
      announcer.next(false, guideSteps([onlineReady], [starter]), [onlineReady]),
    ).toBeUndefined();
    expect(
      announcer.next(true, guideSteps([onlineReady], [starter]), [onlineReady]),
    ).toBeUndefined();

    // Reset clears previously seen homes before checking newly connected host.info.
    announcer.reset(guideSteps([], []), []);
    expect(announcer.next(true, guideSteps([onlineReady], []), [onlineReady])).toBeUndefined();

    announcer.reset(guideSteps([onlineUnset], []), [onlineUnset]);
    expect(announcer.next(true, guideSteps([onlineReady], []), [onlineReady])?.id).toBe('defaults');
  });
});

describe('guideSteps (B.3 matrix, operand flips, and boundaries)', () => {
  it('handles 0 computers: step 1 incomplete and step 2 neither none nor offline', () => {
    expect(guideSteps([], [])).toEqual([
      { id: 'connect', done: false, name: '' },
      { id: 'ais', done: false, count: 0, none: false, offline: undefined },
      { id: 'defaults', done: false, left: [] },
      { id: 'group', done: false, name: undefined, g: undefined },
      { id: 'ask', done: false },
    ]);
  });

  it('handles 1 computer offline vs online without info vs online fallback name', () => {
    const offlineWithInfo = computer('Home Mac', {
      online: false,
      infoName: 'Studio Mac',
      agents: [agent()],
    });
    expect(guideSteps([offlineWithInfo], [])).toEqual([
      { id: 'connect', done: true, name: 'Studio Mac' },
      { id: 'ais', done: false, count: 0, none: false, offline: 'Studio Mac' },
      { id: 'defaults', done: false, left: [] },
      { id: 'group', done: false, name: undefined, g: undefined },
      { id: 'ask', done: false },
    ]);

    const offlineNoInfo = computer('Mini', { online: false, noInfo: true });
    expect(guideSteps([offlineNoInfo], []).slice(0, 2)).toEqual([
      { id: 'connect', done: true, name: 'Mini' },
      { id: 'ais', done: false, count: 0, none: false, offline: 'Mini' },
    ]);

    const onlineNoInfo = computer('OnlineNoInfo', { online: true, noInfo: true });
    expect(guideSteps([onlineNoInfo], []).slice(0, 2)).toEqual([
      { id: 'connect', done: true, name: 'OnlineNoInfo' },
      { id: 'ais', done: false, count: 0, none: false, offline: 'OnlineNoInfo' },
    ]);

    const secondOffline = computer('Backup', { online: false, noInfo: true });
    const firstOnline = computer('Primary', { online: true, agents: [agent()] });
    expect(guideSteps([firstOnline, secondOffline], []).slice(0, 2)).toEqual([
      { id: 'connect', done: true, name: 'Primary' },
      { id: 'ais', done: true, count: 1, none: false, offline: undefined },
    ]);
  });

  it('ignores external AIs and marks step 2 none when an online computer reports zero real AIs', () => {
    const onlyZero = computer('HostA', { agents: [] });
    expect(guideSteps([onlyZero], []).slice(0, 3)).toEqual([
      { id: 'connect', done: true, name: 'HostA' },
      { id: 'ais', done: false, count: 0, none: true, offline: undefined },
      { id: 'defaults', done: false, left: [] },
    ]);

    const onlyExternal = computer('HostB', {
      agents: [agent({ agent: 'external', account: 'personal', defaults: undefined })],
    });
    expect(guideSteps([onlyExternal], []).slice(1, 3)).toEqual([
      { id: 'ais', done: false, count: 0, none: true, offline: undefined },
      { id: 'defaults', done: false, left: [] },
    ]);
  });

  it('requires every non-external AI across computers to have an account before step 2 or 3 finishes', () => {
    const unconfigured = computer('Host', {
      agents: [agent({ agent: 'claude', account: null, defaults: undefined })],
    });
    expect(guideSteps([unconfigured], []).slice(1, 3)).toEqual([
      { id: 'ais', done: false, count: 1, none: false, offline: undefined },
      { id: 'defaults', done: false, left: [] },
    ]);

    const mixedAccounts = computer('Host', {
      agents: [
        agent({ agent: 'claude', account: 'personal', defaults: { ...validDefaults } }),
        agent({ agent: 'codex', account: null, defaults: undefined }),
      ],
    });
    expect(guideSteps([mixedAccounts], []).slice(1, 3)).toEqual([
      { id: 'ais', done: false, count: 2, none: false, offline: undefined },
      { id: 'defaults', done: false, left: [] },
    ]);

    const ready = computer('Host', {
      agents: [agent({ agent: 'claude', account: 'personal', defaults: undefined })],
    });
    expect(guideSteps([ready], []).slice(1, 3)).toEqual([
      { id: 'ais', done: true, count: 1, none: false, offline: undefined },
      { id: 'defaults', done: false, left: [cliName('claude')] },
    ]);
  });

  it('lists missing defaults cleanly across multiple agents and deduplicates CLIs across hosts', () => {
    const comp1 = computer('Mini', {
      agents: [
        agent({ agent: 'claude', account: 'personal', defaults: { ...validDefaults } }),
        agent({ agent: 'codex', account: 'team', defaults: undefined }),
      ],
    });
    const comp2 = computer('Studio', {
      agents: [
        agent({ agent: 'codex', account: 'api', defaults: undefined }),
        agent({ agent: 'gemini', account: 'personal', defaults: undefined }),
      ],
    });

    const partial = guideSteps([comp1, comp2], []);
    expect(partial[1]).toEqual({
      id: 'ais',
      done: true,
      count: 4,
      none: false,
      offline: undefined,
    });
    expect(partial[2]).toEqual({
      id: 'defaults',
      done: false,
      left: [cliName('codex'), cliName('gemini')],
    });

    const comp1Ready = computer('Mini', {
      agents: [
        agent({ agent: 'claude', account: 'personal', defaults: { ...validDefaults } }),
        agent({ agent: 'codex', account: 'team', defaults: { ...validDefaults, model: 'gpt-5' } }),
      ],
    });
    const comp2Ready = computer('Studio', {
      agents: [
        agent({
          agent: 'gemini',
          account: 'personal',
          defaults: { ...validDefaults, model: 'gemini' },
        }),
      ],
    });
    expect(guideSteps([comp1Ready, comp2Ready], [])[2]).toEqual({
      id: 'defaults',
      done: true,
      left: [],
    });
  });

  it('evaluates step 4 starter group rules and latest group selection', () => {
    const readyComp = computer('Mini', { agents: [agent()] });

    const noInfo = group('g0', { noInfo: true });
    const noSecretary = group('g1', { secretary: '' });
    const noMe = group('g2', { me: null });
    const humanOnlyMe = group('g3', {
      members: [
        { id: 'm-me', user: 'u-me', kind: 'human' },
        { id: 'm-other-human', user: 'u-me', kind: 'human' },
      ],
    });
    const onlyOthersAi = group('g4', {
      secretary: 'm-other-ai',
      members: [
        { id: 'm-me', user: 'u-me', kind: 'human' },
        { id: 'm-other-ai', user: 'u-other', kind: 'ai' },
      ],
    });

    expect(
      guideSteps([readyComp], [noInfo, noSecretary, noMe, humanOnlyMe, onlyOthersAi])[3],
    ).toEqual({
      id: 'group',
      done: false,
      name: undefined,
      g: undefined,
    });

    const starterOne = group('g5', {
      name: 'First Team',
      members: [
        { id: 'm-me', user: 'u-me', kind: 'human' },
        { id: 'm-other-ai', user: 'u-other', kind: 'ai' },
        { id: 'm-sec', user: 'u-me', kind: 'ai' },
      ],
    });
    const starterTwo = group('g6', { name: 'Older Team' });

    expect(guideSteps([readyComp], [noSecretary, starterOne, starterTwo])[3]).toEqual({
      id: 'group',
      done: true,
      name: 'First Team',
      g: 'g5',
    });
  });

  it('requires a main-line text message from me inside a starter group for step 5', () => {
    const readyComp = computer('Mini', { agents: [agent()] });

    const nonStarterWithMsg = group('non-starter', {
      secretary: '',
      main: [{ kind: 'text', thread: null, by: 'm-me' }],
    });
    const starterStatusOnly = group('g-status', {
      main: [{ kind: 'system', thread: null, by: 'm-me' }],
    });
    const starterThreadOnly = group('g-thread', {
      main: [{ kind: 'text', thread: 'root-1', by: 'm-me' }],
    });
    const starterFromAi = group('g-other', {
      main: [{ kind: 'text', thread: null, by: 'm-sec' }],
    });

    expect(
      guideSteps(
        [readyComp],
        [nonStarterWithMsg, starterStatusOnly, starterThreadOnly, starterFromAi],
      )[4],
    ).toEqual({ id: 'ask', done: false });

    const starterWithMine = group('g-done', {
      main: [
        { kind: 'system', thread: null, by: 'm-me' },
        { kind: 'text', thread: 'root-1', by: 'm-me' },
        { kind: 'text', thread: null, by: 'm-sec' },
        { kind: 'text', thread: null, by: 'm-me' },
      ],
    });

    expect(guideSteps([readyComp], [starterStatusOnly, starterWithMine])[4]).toEqual({
      id: 'ask',
      done: true,
    });
  });
});

describe('isBrandNew', () => {
  it('returns true only when owner and joined records are both empty, and false on errors', async () => {
    expect(await isBrandNew()).toBe(true);

    const freshIdb = new IDBFactory();
    expect(await isBrandNew(crypto, () => freshIdb)).toBe(true);

    const ownerOnlyIdb = new IDBFactory();
    const log = await createOwnerLog();
    await saveOwner(() => ownerOnlyIdb, log, { 'node-1': ['ws://127.0.0.1:9000'] });
    expect(await isBrandNew(crypto, () => ownerOnlyIdb)).toBe(false);

    const joinedOnlyIdb = new IDBFactory();
    await saveJoined(() => joinedOnlyIdb, {
      'home-1': { name: 'Friend', url: 'https://friend.example' },
    });
    expect(await isBrandNew(crypto, () => joinedOnlyIdb)).toBe(false);

    const brokenIdb = () =>
      ({
        open() {
          throw new Error('idb blocked');
        },
      }) as unknown as IDBFactory;
    expect(await isBrandNew(crypto, brokenIdb)).toBe(false);
  });
});

describe('user-opened tracking, step targets, box overlap, and layout observer', () => {
  afterEach(() => {
    resetGuideUi();
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
    document.body.innerHTML = '';
  });

  it('tracks isUserOpened across open, expand, collapse, close, finish, and reset', () => {
    resetGuideUi();
    expect(isUserOpened()).toBe(false);

    openGuide();
    expect(isUserOpened()).toBe(true);
    clearUserOpened();
    expect(isUserOpened()).toBe(false);

    expandGuide(true);
    expect(isUserOpened()).toBe(true);
    collapseGuide();
    expect(isUserOpened()).toBe(false);

    expandGuide();
    expect(isUserOpened()).toBe(false);
    expect(guideUi.focusHeading).toBe(false);

    expandGuide(false, true);
    expect(isUserOpened()).toBe(true);
    expect(guideUi.focusHeading).toBe(false);

    expandGuide(true, false);
    expect(isUserOpened()).toBe(false);
    expect(guideUi.focusHeading).toBe(true);

    openGuide();
    closeGuide();
    expect(isUserOpened()).toBe(false);

    openGuide();
    finishGuide();
    expect(isUserOpened()).toBe(false);

    openGuide();
    resetGuideUi();
    expect(isUserOpened()).toBe(false);
  });

  it('maps each page and step to its interactive target selector', () => {
    expect(stepTargetSelector(undefined, 'onboarding')).toBeUndefined();
    expect(stepTargetSelector('connect', 'onboarding')).toBe(
      '.onboarding .card, .onboarding fieldset.acct, .onboarding .done-btn',
    );
    expect(stepTargetSelector('ais', 'onboarding')).toBe(
      '.onboarding .card, .onboarding fieldset.acct, .onboarding .done-btn',
    );
    expect(stepTargetSelector('defaults', 'onboarding')).toBe(
      '.onboarding .card, .onboarding fieldset.acct, .onboarding .done-btn',
    );

    expect(stepTargetSelector('ais', 'nodes')).toBe('.ai-row fieldset.acct');
    expect(stepTargetSelector('defaults', 'nodes')).toBe(
      '[data-guide="defaults"][data-unset], .defaults .editor',
    );
    expect(stepTargetSelector('connect', 'nodes')).toBeUndefined();
    expect(stepTargetSelector('group', 'nodes')).toBeUndefined();

    expect(stepTargetSelector('group', 'groups')).toBe('.list .head button, .list .empty button');
    expect(stepTargetSelector('connect', 'groups')).toBeUndefined();

    expect(stepTargetSelector('ask', 'chat')).toBe('.composer:not(.in-thread)');
    expect(stepTargetSelector('group', 'chat')).toBeUndefined();

    expect(stepTargetSelector('connect', 'settings')).toBeUndefined();
  });

  it('detects strict positive overlap between laid-out boxes', () => {
    const box = (left: number, top: number, right: number, bottom: number) => ({
      left,
      top,
      right,
      bottom,
      width: right - left,
      height: bottom - top,
    });
    const dock = box(100, 100, 200, 200);

    expect(boxesOverlap(box(150, 120, 150, 180), dock)).toBe(false);
    expect(boxesOverlap(box(120, 150, 180, 150), dock)).toBe(false);
    expect(boxesOverlap(dock, box(150, 120, 150, 180))).toBe(false);
    expect(boxesOverlap(dock, box(120, 150, 180, 150))).toBe(false);

    // Touching edges do not overlap.
    expect(boxesOverlap(dock, box(0, 100, 100, 200))).toBe(false);
    expect(boxesOverlap(dock, box(200, 100, 300, 200))).toBe(false);
    expect(boxesOverlap(dock, box(100, 0, 200, 100))).toBe(false);
    expect(boxesOverlap(dock, box(100, 200, 200, 300))).toBe(false);

    // 1 px intersection overlaps.
    expect(boxesOverlap(dock, box(199, 199, 250, 250))).toBe(true);
    expect(boxesOverlap(dock, box(50, 50, 101, 101))).toBe(true);
    expect(boxesOverlap(dock, box(120, 120, 140, 140))).toBe(true);
  });

  it('checks whether dock rect intersects any matching step target in root', () => {
    const dock = document.createElement('aside');
    vi.spyOn(dock, 'getBoundingClientRect').mockReturnValue({
      left: 500,
      top: 400,
      right: 844,
      bottom: 800,
      width: 344,
      height: 400,
    } as DOMRect);

    const querySpy = vi.spyOn(document, 'querySelectorAll');
    expect(dockOverlapsTargets(dock, undefined, 'nodes', document)).toBe(false);
    expect(dockOverlapsTargets(dock, 'defaults', 'settings', document)).toBe(false);
    expect(querySpy).not.toHaveBeenCalled();
    expect(dockOverlapsTargets(dock, 'defaults', 'nodes', document)).toBe(false);
    expect(querySpy).toHaveBeenCalledTimes(1);
    querySpy.mockRestore();

    const first = document.createElement('button');
    first.dataset.guide = 'defaults';
    first.dataset.unset = '';
    vi.spyOn(first, 'getBoundingClientRect').mockReturnValue({
      left: 500,
      top: 300,
      right: 600,
      bottom: 332,
      width: 100,
      height: 32,
    } as DOMRect);
    const second = document.createElement('button');
    second.dataset.guide = 'defaults';
    second.dataset.unset = '';
    vi.spyOn(second, 'getBoundingClientRect').mockReturnValue({
      left: 0,
      top: 0,
      right: 0,
      bottom: 0,
      width: 0,
      height: 0,
    } as DOMRect);
    document.body.append(first, second);

    expect(dockOverlapsTargets(dock, 'defaults', 'nodes', document)).toBe(false);

    vi.spyOn(second, 'getBoundingClientRect').mockReturnValue({
      left: 510,
      top: 520,
      right: 610,
      bottom: 552,
      width: 100,
      height: 32,
    } as DOMRect);
    expect(dockOverlapsTargets(dock, 'defaults', 'nodes')).toBe(true);
    document.body.innerHTML = '';
  });

  it('observes main DOM/resize/interaction and window resize/scroll/animationend and cleans up', async () => {
    const roObserve = vi.fn();
    const roDisconnect = vi.fn();
    let roCallback!: (entries: ResizeObserverEntry[]) => void;
    vi.stubGlobal(
      'ResizeObserver',
      class {
        constructor(cb: (entries: ResizeObserverEntry[]) => void) {
          roCallback = cb;
        }
        observe = roObserve;
        disconnect = roDisconnect;
      },
    );

    const main = document.createElement('main');
    const mainChild = document.createElement('div');
    main.appendChild(mainChild);
    const dock = document.createElement('aside');
    const dockChild = document.createElement('div');
    dock.appendChild(dockChild);
    document.body.append(main, dock);

    const addWinSpy = vi.spyOn(window, 'addEventListener');
    const removeWinSpy = vi.spyOn(window, 'removeEventListener');
    const addMainSpy = vi.spyOn(main, 'addEventListener');
    const removeMainSpy = vi.spyOn(main, 'removeEventListener');

    const onLayoutChange = vi.fn();
    const onSettled = vi.fn();
    const stop = observeDockLayout(main, dock, { onLayoutChange, onSettled });

    expect(roObserve).toHaveBeenCalledWith(main);
    expect(addMainSpy).toHaveBeenCalledWith('pointerdown', onLayoutChange, { passive: true });
    expect(addMainSpy).toHaveBeenCalledWith('focusin', onLayoutChange, { passive: true });
    expect(addWinSpy).toHaveBeenCalledWith('resize', onLayoutChange, { passive: true });
    expect(addWinSpy).toHaveBeenCalledWith('scroll', expect.any(Function), {
      capture: true,
      passive: true,
    });
    expect(addWinSpy).toHaveBeenCalledWith('animationend', onSettled, {
      capture: true,
      passive: true,
    });

    // ResizeObserver initial measurement does not trigger layout change.
    roCallback([
      { contentRect: { width: 800, height: 600 } as DOMRectReadOnly },
    ] as ResizeObserverEntry[]);
    roCallback([
      { contentRect: { width: 800, height: 600 } as DOMRectReadOnly },
    ] as ResizeObserverEntry[]);
    expect(onLayoutChange).not.toHaveBeenCalled();

    // Width or height delta triggers onLayoutChange.
    roCallback([
      { contentRect: { width: 700, height: 600 } as DOMRectReadOnly },
    ] as ResizeObserverEntry[]);
    expect(onLayoutChange).toHaveBeenCalledTimes(1);
    roCallback([
      { contentRect: { width: 700, height: 500 } as DOMRectReadOnly },
    ] as ResizeObserverEntry[]);
    expect(onLayoutChange).toHaveBeenCalledTimes(2);

    // Child/subtree DOM mutations inside main trigger onLayoutChange.
    const added = document.createElement('span');
    mainChild.appendChild(added);
    await vi.waitFor(() => expect(onLayoutChange).toHaveBeenCalledTimes(3));

    // Scroll inside dock is ignored; scroll outside dock and non-Node window scroll trigger.
    dockChild.dispatchEvent(new Event('scroll', { bubbles: false }));
    expect(onLayoutChange).toHaveBeenCalledTimes(3);
    mainChild.dispatchEvent(new Event('scroll', { bubbles: false }));
    expect(onLayoutChange).toHaveBeenCalledTimes(4);
    window.dispatchEvent(new Event('scroll'));
    expect(onLayoutChange).toHaveBeenCalledTimes(5);

    // Pointer/focus inside main and window resize trigger onLayoutChange.
    main.dispatchEvent(new Event('pointerdown'));
    main.dispatchEvent(new Event('focusin'));
    window.dispatchEvent(new Event('resize'));
    expect(onLayoutChange).toHaveBeenCalledTimes(8);

    // animationend invokes onSettled without invoking onLayoutChange.
    window.dispatchEvent(new Event('animationend'));
    expect(onSettled).toHaveBeenCalledTimes(1);
    expect(onLayoutChange).toHaveBeenCalledTimes(8);

    // Cleanup removes all listeners and disconnects observers.
    stop();
    expect(roDisconnect).toHaveBeenCalledTimes(1);
    expect(removeMainSpy).toHaveBeenCalledWith('pointerdown', onLayoutChange);
    expect(removeMainSpy).toHaveBeenCalledWith('focusin', onLayoutChange);
    expect(removeWinSpy).toHaveBeenCalledWith('resize', onLayoutChange);
    expect(removeWinSpy).toHaveBeenCalledWith('scroll', expect.any(Function), true);
    expect(removeWinSpy).toHaveBeenCalledWith('animationend', onSettled, true);

    main.dispatchEvent(new Event('pointerdown'));
    window.dispatchEvent(new Event('resize'));
    window.dispatchEvent(new Event('scroll'));
    window.dispatchEvent(new Event('animationend'));
    mainChild.appendChild(document.createElement('span'));
    await Promise.resolve();
    expect(onLayoutChange).toHaveBeenCalledTimes(8);
    expect(onSettled).toHaveBeenCalledTimes(1);

    document.body.innerHTML = '';
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });

  it('compares overlap target context across page, group, stepId, and doneCount', () => {
    const base: OverlapTargetContext = {
      page: 'groups',
      group: 'g1',
      stepId: 'group',
      doneCount: 2,
    };
    expect(sameTargetContext(base, { ...base })).toBe(true);
    expect(sameTargetContext(base, { ...base, page: 'nodes' })).toBe(false);
    expect(sameTargetContext(base, { ...base, group: 'g2' })).toBe(false);
    expect(sameTargetContext(base, { ...base, stepId: 'ask' })).toBe(false);
    expect(sameTargetContext(base, { ...base, doneCount: 3 })).toBe(false);
  });

  it('checks all four dock-watching eligibility conditions in canWatchDock', () => {
    resetGuideUi();
    const dock = document.createElement('aside');
    expect(canWatchDock({ visible: true, narrow: false, dock })).toBe(true);
    expect(canWatchDock({ visible: false, narrow: false, dock })).toBe(false);
    expect(canWatchDock({ visible: true, narrow: true, dock })).toBe(false);
    expect(canWatchDock({ visible: true, narrow: false, dock: undefined })).toBe(false);
    collapseGuide();
    expect(canWatchDock({ visible: true, narrow: false, dock })).toBe(false);
  });

  it('auto-collapses on overlap only when watchable and notifies focus only if dock holds focus', () => {
    resetGuideUi();
    document.body.innerHTML = '';
    const main = document.createElement('main');
    const mainBtn = document.createElement('button');
    mainBtn.dataset.guide = 'defaults';
    mainBtn.dataset.unset = '';
    main.appendChild(mainBtn);

    const dock = document.createElement('aside');
    const dockBtn = document.createElement('button');
    dock.appendChild(dockBtn);
    document.body.append(main, dock);

    vi.spyOn(dock, 'getBoundingClientRect').mockReturnValue({
      left: 500,
      top: 400,
      right: 844,
      bottom: 800,
      width: 344,
      height: 400,
    } as DOMRect);
    let targetTop = 500;
    vi.spyOn(mainBtn, 'getBoundingClientRect').mockImplementation(
      () =>
        ({
          left: 510,
          top: targetTop,
          right: 610,
          bottom: targetTop + 32,
          width: 100,
          height: 32,
        }) as DOMRect,
    );

    const onFocusInsideDock = vi.fn();
    dockBtn.focus();
    expect(document.activeElement).toBe(dockBtn);

    // Not watchable -> false without collapsing or reporting dock focus.
    expect(
      autoCollapseIfOverlapping(
        { visible: false, narrow: false, dock, stepId: 'defaults', page: 'nodes' },
        onFocusInsideDock,
      ),
    ).toBe(false);
    expect(guideUi.expanded).toBe(true);
    expect(onFocusInsideDock).not.toHaveBeenCalled();

    // User-opened override suppresses auto-collapse even on overlap.
    expandGuide(true);
    expect(
      autoCollapseIfOverlapping(
        { visible: true, narrow: false, dock, stepId: 'defaults', page: 'nodes' },
        onFocusInsideDock,
      ),
    ).toBe(false);
    expect(guideUi.expanded).toBe(true);
    expect(onFocusInsideDock).not.toHaveBeenCalled();

    // Clear override and test non-overlapping layout -> false.
    clearUserOpened();
    targetTop = 100;
    expect(
      autoCollapseIfOverlapping(
        { visible: true, narrow: false, dock, stepId: 'defaults', page: 'nodes' },
        onFocusInsideDock,
      ),
    ).toBe(false);
    expect(guideUi.expanded).toBe(true);
    expect(onFocusInsideDock).not.toHaveBeenCalled();

    // Overlap while focus is on a main control -> collapses WITHOUT calling onFocusInsideDock.
    targetTop = 500;
    mainBtn.focus();
    expect(document.activeElement).toBe(mainBtn);
    expect(
      autoCollapseIfOverlapping(
        { visible: true, narrow: false, dock, stepId: 'defaults', page: 'nodes' },
        onFocusInsideDock,
      ),
    ).toBe(true);
    expect(guideUi.expanded).toBe(false);
    expect(onFocusInsideDock).not.toHaveBeenCalled();
    expect(document.activeElement).toBe(mainBtn);

    // Overlap while focus is inside the dock -> collapses AND calls onFocusInsideDock.
    expandGuide(false);
    dockBtn.focus();
    expect(document.activeElement).toBe(dockBtn);
    expect(
      autoCollapseIfOverlapping(
        { visible: true, narrow: false, dock, stepId: 'defaults', page: 'nodes' },
        onFocusInsideDock,
      ),
    ).toBe(true);
    expect(guideUi.expanded).toBe(false);
    expect(onFocusInsideDock).toHaveBeenCalledTimes(1);

    document.body.innerHTML = '';
    vi.restoreAllMocks();
  });

  it('tracks context changes, preserves manual open on animationend, and resets it on layout change', () => {
    resetGuideUi();
    document.body.innerHTML = '';
    vi.stubGlobal(
      'ResizeObserver',
      class {
        observe() {}
        disconnect() {}
      },
    );

    const main = document.createElement('main');
    const target = document.createElement('button');
    target.dataset.guide = 'defaults';
    target.dataset.unset = '';
    main.appendChild(target);
    const dock = document.createElement('aside');
    const dockBtn = document.createElement('button');
    dock.appendChild(dockBtn);
    document.body.append(main, dock);

    vi.spyOn(dock, 'getBoundingClientRect').mockReturnValue({
      left: 500,
      top: 400,
      right: 844,
      bottom: 800,
      width: 344,
      height: 400,
    } as DOMRect);
    let targetTop = 100;
    vi.spyOn(target, 'getBoundingClientRect').mockImplementation(
      () =>
        ({
          left: 510,
          top: targetTop,
          right: 610,
          bottom: targetTop + 32,
          width: 100,
          height: 32,
        }) as DOMRect,
    );

    const onFocusInsideDock = vi.fn();
    const initial: OverlapTargetContext = {
      page: 'nodes',
      group: 'g1',
      stepId: 'ais',
      doneCount: 1,
    };
    const tracker = createOverlapTracker(initial, onFocusInsideDock);
    // Mutating initial after creation must not alter the tracker's stored context.
    initial.page = 'chat';

    // Same context preserves userOpened and attaches layout observers.
    expandGuide(true);
    const stopA = tracker.sync({
      visible: true,
      narrow: false,
      dock,
      page: 'nodes',
      group: 'g1',
      stepId: 'ais',
      doneCount: 1,
    });
    expect(typeof stopA).toBe('function');
    expect(isUserOpened()).toBe(true);
    stopA?.();

    // Context change clears userOpened even when dock cannot currently be watched.
    const stopHidden = tracker.sync({
      visible: false,
      narrow: false,
      dock,
      page: 'nodes',
      group: 'g2',
      stepId: 'ais',
      doneCount: 1,
    });
    expect(stopHidden).toBeUndefined();
    expect(isUserOpened()).toBe(false);

    // Repeating the updated context leaves userOpened intact.
    expandGuide(true);
    expect(
      tracker.sync({
        visible: false,
        narrow: false,
        dock,
        page: 'nodes',
        group: 'g2',
        stepId: 'ais',
        doneCount: 1,
      }),
    ).toBeUndefined();
    expect(isUserOpened()).toBe(true);

    // Transitioning to an overlapping step clears userOpened and collapses at once.
    targetTop = 500;
    dockBtn.focus();
    expect(
      tracker.sync({
        visible: true,
        narrow: false,
        dock,
        page: 'nodes',
        group: 'g2',
        stepId: 'defaults',
        doneCount: 2,
      }),
    ).toBeUndefined();
    expect(isUserOpened()).toBe(false);
    expect(guideUi.expanded).toBe(false);
    expect(onFocusInsideDock).toHaveBeenCalledTimes(1);

    // While user-opened, animationend keeps dock open; layout change clears override and collapses.
    expandGuide(true);
    targetTop = 100;
    const stopOpen = tracker.sync({
      visible: true,
      narrow: false,
      dock,
      page: 'nodes',
      group: 'g2',
      stepId: 'defaults',
      doneCount: 2,
    });
    expect(typeof stopOpen).toBe('function');

    targetTop = 500;
    window.dispatchEvent(new Event('animationend'));
    expect(guideUi.expanded).toBe(true);
    expect(isUserOpened()).toBe(true);

    target.focus();
    main.dispatchEvent(new Event('pointerdown'));
    expect(guideUi.expanded).toBe(false);
    expect(isUserOpened()).toBe(false);
    expect(onFocusInsideDock).toHaveBeenCalledTimes(1);
    stopOpen?.();

    // When not user-opened, animationend re-evaluates and collapses upon overlap.
    resetGuideUi();
    targetTop = 100;
    const stopSettled = tracker.sync({
      visible: true,
      narrow: false,
      dock,
      page: 'nodes',
      group: 'g2',
      stepId: 'defaults',
      doneCount: 2,
    });
    expect(typeof stopSettled).toBe('function');
    targetTop = 500;
    dockBtn.focus();
    window.dispatchEvent(new Event('animationend'));
    expect(guideUi.expanded).toBe(false);
    expect(onFocusInsideDock).toHaveBeenCalledTimes(2);
    stopSettled?.();

    document.body.innerHTML = '';
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });
});
