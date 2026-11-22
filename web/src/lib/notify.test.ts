import { flushSync } from 'svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { watch } from '../../features/watch.svelte';
import { Group, Home, homes } from './groups.svelte';
import { goldens } from './mock';
import {
  autoDismissMs,
  classify,
  inOpenChat,
  maxNotices,
  Notifier,
  targetIdOf,
  titleOf,
  whoOf,
} from './notify.svelte';
import { prefs } from './prefs.svelte';
import * as p from './proto';
import { route } from './route.svelte';

const website = 'rZNvy-1jH6Z-BcPqA5U5BQ';
const otherGroup = 'ZG9jcy1ncm91cC0wMDAwMQ';
const ownerUser: string = JSON.parse(goldens['row-member-human']).user;
const ownerId: string = JSON.parse(goldens['row-member-human']).id;
const codexId = '1PC8WineBrUQ-apCjx7tug';
const secretaryId = 'qBSFMsr2hHYKOMblEA_kdA';
const taskId = 'DrtCn6htSBwmMPrFPbHJHA';

const sign = async (payload: string) => ({ p: payload, k: 'dev-k', s: 'sig' });

function row<T extends p.Row = p.Row>(name: string, changes: Record<string, unknown> = {}): T {
  return { ...p.decode(p.Row, goldens[name]), ...changes } as T;
}

function setRoute(hash: string): void {
  location.hash = hash;
  dispatchEvent(new HashChangeEvent('hashchange', { newURL: location.href }));
}

function makeReadyGroup(g = website, name = 'Website') {
  const home = new Home({ user: ownerUser, sign });
  const group = new Group(home, g);
  group.name = name;
  home.groups = [group];
  group.apply(
    [
      row('row-group', { id: g, name, seq: 1 }),
      row('row-member-human', { g, seq: 2, name: 'Alex' }),
      row('row-member-ai', { id: codexId, g, seq: 3, name: 'Codex', cli: 'codex' }),
      row('row-member-ai', {
        id: secretaryId,
        g,
        seq: 4,
        name: 'Secretary',
        cli: 'claude',
        secretary: true,
      }),
      row('row-task', { id: taskId, g, seq: 5 }),
    ],
    false,
  );
  group.cursor = 5;
  return { home, group };
}

function pushLive(group: Group, rows: p.Row[]): void {
  group.apply(rows, true);
}

describe('classify, whoOf, titleOf and targetIdOf', () => {
  beforeEach(() => {
    prefs.lang = 'en';
    setRoute('#/nodes');
  });

  it('classifies pending cards waiting on the reader and ignores others or decided states', () => {
    const { group } = makeReadyGroup();
    const approvalMine = row<p.MsgRow>('row-msg-approval-pending', { seq: 6, pos: 6 });
    const approvalOther = row<p.MsgRow>('row-msg-approval-pending', {
      id: 'ap-other',
      seq: 7,
      pos: 7,
      mentions: [ownerId],
      body: {
        ...(row<p.MsgRow>('row-msg-approval-pending').body as object),
        approvers: ['other-user'],
      },
    });
    const approvalDone = row<p.MsgRow>('row-msg-approval-approved', {
      id: 'ap-done',
      seq: 8,
      pos: 8,
    });
    const decisionMine = row<p.MsgRow>('row-msg-decision-pending', { seq: 9, pos: 9 });
    const decisionOther = row<p.MsgRow>('row-msg-decision-pending', {
      id: 'dc-other',
      seq: 10,
      pos: 10,
      mentions: [ownerId],
      body: { ...(row<p.MsgRow>('row-msg-decision-pending').body as object), to: ['other-user'] },
    });
    const briefMine = row<p.MsgRow>('row-msg-brief-pending', { seq: 11, pos: 11 });
    const briefOther = row<p.MsgRow>('row-msg-brief-pending', {
      id: 'br-other',
      seq: 12,
      pos: 12,
      mentions: [ownerId],
      body: {
        ...(row<p.MsgRow>('row-msg-brief-pending').body as object),
        requester: 'other-member',
      },
    });
    pushLive(group, [
      approvalMine,
      approvalOther,
      approvalDone,
      decisionMine,
      decisionOther,
      briefMine,
      briefOther,
    ]);

    expect(classify(group, approvalMine)).toBe('approval');
    expect(classify(group, approvalOther)).toBeUndefined();
    expect(classify(group, approvalDone)).toBeUndefined();
    expect(classify(group, decisionMine)).toBe('decision');
    expect(classify(group, decisionOther)).toBeUndefined();
    expect(classify(group, briefMine)).toBe('brief');
    expect(classify(group, briefOther)).toBeUndefined();
  });

  it('classifies summaries, mentions from others, and ignores system or own messages', () => {
    const { group } = makeReadyGroup();
    const summary = row<p.MsgRow>('row-msg-summary', { seq: 6, pos: 6 });
    const mention = row<p.MsgRow>('row-msg-text', {
      id: 'm-at',
      seq: 7,
      pos: 7,
      by: codexId,
      mentions: [ownerId],
    });
    const plain = row<p.MsgRow>('row-msg-text', {
      id: 'm-plain',
      seq: 8,
      pos: 8,
      by: codexId,
      mentions: [],
    });
    const ownMention = row<p.MsgRow>('row-msg-text', {
      id: 'm-self',
      seq: 9,
      pos: 9,
      by: ownerId,
      mentions: [ownerId],
    });
    const sysMsg = row<p.MsgRow>('row-msg-system', { seq: 10, pos: 10 });
    pushLive(group, [summary, mention, plain, ownMention, sysMsg]);

    expect(classify(group, summary)).toBe('done');
    expect(classify(group, mention)).toBe('mention');
    expect(classify(group, plain)).toBeUndefined();
    expect(classify(group, ownMention)).toBeUndefined();
    expect(classify(group, sysMsg)).toBeUndefined();

    const strangerHome = new Home({ user: 'stranger', sign });
    const noMeGroup = new Group(strangerHome, website);
    noMeGroup.apply([mention], true);
    expect(classify(noMeGroup, mention)).toBeUndefined();
  });

  it('derives author names, single-line titles and target message ids', () => {
    const { group } = makeReadyGroup();
    const approvalFromSecretary = row<p.MsgRow>('row-msg-approval-pending', {
      seq: 6,
      pos: 6,
      by: secretaryId,
      body: {
        ...(row<p.MsgRow>('row-msg-approval-pending').body as object),
        ai: codexId,
        why: undefined,
        command: 'pnpm install\npnpm test',
      },
    });
    const goneApproval = row<p.MsgRow>('row-msg-approval-pending', {
      id: 'ap-gone',
      seq: 7,
      pos: 7,
      body: { ...(row<p.MsgRow>('row-msg-approval-pending').body as object), ai: 'gone-ai' },
    });
    const summaryWithTask = row<p.MsgRow>('row-msg-summary', { seq: 8, pos: 8 });
    const summaryFallback = row<p.MsgRow>('row-msg-summary', {
      id: 'sum-fallback',
      seq: 9,
      pos: 9,
      text: 'Merged cleanly\nSecond line',
      body: { ...(row<p.MsgRow>('row-msg-summary').body as object), task: 'missing-task' },
    });
    expect(whoOf(group, approvalFromSecretary)).toBe('Codex');
    expect(whoOf(group, goneApproval)).toBe('Former member');
    expect(whoOf(group, summaryWithTask)).toBe('Secretary');
    expect(titleOf(group, approvalFromSecretary)).toBe('pnpm install');
    expect(titleOf(group, summaryWithTask)).toBe('T-12 Show connection path');
    expect(titleOf(group, summaryFallback)).toBe('Merged cleanly');

    const leadText = row<p.MsgRow>('row-msg-text', { id: 'lead-txt', seq: 8, pos: 8 });
    const otherTaskMsg = row<p.MsgRow>('row-msg-task', {
      id: 'other-task-card',
      seq: 9,
      pos: 9,
      body: { task: 'other-task' },
    });
    const taskCardMsg = row<p.MsgRow>('row-msg-task', {
      id: 'task-card-row-id',
      seq: 10,
      pos: 10,
      body: { task: taskId },
    });
    const threadMention = row<p.MsgRow>('row-msg-question', {
      id: 'q-thread',
      seq: 11,
      pos: 11,
      thread: taskId,
      main: false,
      mentions: [ownerId],
    });
    expect(targetIdOf(group, threadMention)).toBe(taskId);
    pushLive(group, [leadText, otherTaskMsg, taskCardMsg, threadMention]);
    expect(targetIdOf(group, threadMention)).toBe('task-card-row-id');
    expect(targetIdOf(group, approvalFromSecretary)).toBe(approvalFromSecretary.id);
    const mainNoThread = row<p.MsgRow>('row-msg-text', {
      id: 'main-msg',
      seq: 12,
      pos: 12,
      thread: null,
      main: false,
    });
    expect(targetIdOf(group, mainNoThread)).toBe('main-msg');
  });

  it('matches inOpenChat only for main-line messages of the open chat route', () => {
    const mainMsg = row<p.MsgRow>('row-msg-text', { thread: null, main: false });
    const raisedMsg = row<p.MsgRow>('row-msg-approval-pending', { thread: taskId, main: true });
    const threadOnly = row<p.MsgRow>('row-msg-question', { thread: taskId, main: false });

    setRoute(`#/groups/${website}`);
    expect(inOpenChat(website, mainMsg)).toBe(true);
    expect(inOpenChat(website, raisedMsg)).toBe(true);
    expect(inOpenChat(website, threadOnly)).toBe(false);
    expect(inOpenChat(otherGroup, mainMsg)).toBe(false);

    setRoute(`#/terminal/${website}/m`);
    expect(inOpenChat(website, mainMsg)).toBe(false);
  });
});

describe('Notifier', () => {
  beforeEach(() => {
    vi.useFakeTimers();
    prefs.lang = 'en';
    setRoute('#/nodes');
  });

  afterEach(() => {
    homes.splice(0);
    vi.useRealTimers();
    vi.restoreAllMocks();
    document.body.innerHTML = '';
  });

  it('ignores initial rows, cold sync, delta catch-up, older pages and card updates, and resets cleanly', () => {
    const notifier = new Notifier();
    const { home, group } = makeReadyGroup();
    group.cursor = 10;
    notifier.sync([home]);
    expect(notifier.items).toEqual([]);

    // Older fetched page below baseline cursor (seq 9 <= 10) does not notify or lower baseline.
    group.apply(
      [row<p.MsgRow>('row-msg-approval-pending', { id: 'old-fetch', seq: 9, pos: 9 })],
      false,
    );
    notifier.sync([home]);
    expect(notifier.items).toEqual([]);
    notifier.sync([home]);
    expect(notifier.items).toEqual([]);

    // Delta sync catch-up (high > cursor while #live is false) advances baseline without notifying.
    group.apply(
      [row<p.MsgRow>('row-msg-decision-pending', { id: 'catchup-dc', seq: 12, pos: 12 })],
      false,
    );
    notifier.sync([home]);
    expect(notifier.items).toEqual([]);
    group.cursor = 12;

    // Non-message rows, gone markers, card updates (where pos differs from seq), and plain messages do not notify.
    pushLive(group, [
      row('row-note', { id: 'note-live', seq: 13, pos: 13 }),
      row('row-task', { id: 't-new', seq: 14 }),
      { r: 'msg', id: 'gone-msg', seq: 15, gone: true },
      row<p.MsgRow>('row-msg-approval-pending', { id: 'updated-card', seq: 16, pos: 11 }),
      row<p.MsgRow>('row-msg-text', {
        id: 'plain-live',
        seq: 17,
        pos: 17,
        by: codexId,
        mentions: [],
      }),
    ]);
    notifier.sync([home]);
    expect(notifier.items).toEqual([]);

    // Reset clears the baseline so fresh post-reset live messages notify at lower sequence numbers.
    group.reset();
    notifier.sync([home]);
    group.apply(
      [
        row('row-group', { id: website, seq: 1 }),
        row('row-member-human', { g: website, seq: 2, name: 'Alex' }),
        row('row-member-ai', { id: codexId, g: website, seq: 3, name: 'Codex', cli: 'codex' }),
      ],
      false,
    );
    notifier.sync([home]);
    expect(notifier.items).toEqual([]);
    group.cursor = 3;
    pushLive(group, [
      row<p.MsgRow>('row-msg-approval-pending', { id: 'after-reset', seq: 4, pos: 4 }),
    ]);
    notifier.sync([home]);
    expect(notifier.items.map((i) => i.msg.id)).toEqual(['after-reset']);
  });

  it('handles an empty synced group and cold sync before live arrivals', () => {
    const notifier = new Notifier();
    const home = new Home({ user: ownerUser, sign });
    const emptySynced = new Group(home, website);
    emptySynced.cursor = 1;
    home.groups = [emptySynced];
    notifier.sync([home]);
    expect(notifier.items).toEqual([]);

    pushLive(emptySynced, [
      row('row-member-human', { g: website, seq: 2, name: 'Alex' }),
      row<p.MsgRow>('row-msg-approval-pending', { id: 'live-ap', seq: 3, pos: 3 }),
    ]);
    notifier.sync([home]);
    expect(notifier.items.map((i) => i.msg.id)).toEqual(['live-ap']);
  });

  it('orders simultaneous arrivals by pos, caps at maxNotices, and auto-dismisses only transient kinds', () => {
    const clearSpy = vi.spyOn(globalThis, 'clearTimeout');
    const notifier = new Notifier();
    const { home, group } = makeReadyGroup();
    notifier.sync([home]);

    const m1 = row<p.MsgRow>('row-msg-text', {
      id: 'm1',
      seq: 6,
      pos: 6,
      by: codexId,
      text: 'first',
      mentions: [ownerId],
    });
    const m2 = row<p.MsgRow>('row-msg-summary', { id: 'm2', seq: 7, pos: 7 });
    const m3 = row<p.MsgRow>('row-msg-approval-pending', { id: 'm3', seq: 8, pos: 8 });
    pushLive(group, [m2, m1, m3]);
    notifier.sync([home]);
    expect(notifier.items.map((i) => [i.msg.id, i.kind, i.needsYou])).toEqual([
      ['m1', 'mention', false],
      ['m2', 'done', false],
      ['m3', 'approval', true],
    ]);
    expect(notifier.items).toHaveLength(maxNotices);

    // 4th arrival evicts oldest ('m1') and clears its pending timer.
    const m4 = row<p.MsgRow>('row-msg-decision-pending', { id: 'm4', seq: 9, pos: 9 });
    pushLive(group, [m4]);
    notifier.sync([home]);
    expect(clearSpy).toHaveBeenCalledTimes(1);
    expect(notifier.items.map((i) => i.msg.id)).toEqual(['m2', 'm3', 'm4']);

    // Boundary before autoDismissMs keeps 'm2'; exact autoDismissMs dismisses 'm2' only.
    vi.advanceTimersByTime(autoDismissMs - 1);
    expect(notifier.items.map((i) => i.msg.id)).toEqual(['m2', 'm3', 'm4']);
    vi.advanceTimersByTime(1);
    expect(notifier.items.map((i) => i.msg.id)).toEqual(['m3', 'm4']);

    // Blocking cards ('m3', 'm4') stay across arbitrary elapsed time.
    vi.advanceTimersByTime(60_000);
    expect(notifier.items.map((i) => i.msg.id)).toEqual(['m3', 'm4']);
  });

  it('prunes resolved cards, deleted rows, opened chats and removed groups or homes without looping in $effect', () => {
    const clearSpy = vi.spyOn(globalThis, 'clearTimeout');
    const notifier = new Notifier();
    const { home, group } = makeReadyGroup();
    homes.splice(0, homes.length, home);
    const stop = watch(() => notifier.sync(homes));
    flushSync();

    const ap1 = row<p.MsgRow>('row-msg-approval-pending', { id: 'ap-1', seq: 6, pos: 6 });
    const ap2 = row<p.MsgRow>('row-msg-approval-pending', { id: 'ap-2', seq: 7, pos: 7 });
    const mention = row<p.MsgRow>('row-msg-text', {
      id: 'men-1',
      seq: 8,
      pos: 8,
      by: codexId,
      mentions: [ownerId],
    });
    pushLive(group, [ap1, ap2, mention]);
    flushSync();
    expect(notifier.items.map((i) => i.msg.id)).toEqual(['ap-1', 'ap-2', 'men-1']);

    // Approving ap-1 prunes only ap-1 while ap-2 remains waiting.
    pushLive(group, [row<p.MsgRow>('row-msg-approval-approved', { id: 'ap-1', seq: 9, pos: 6 })]);
    flushSync();
    expect(notifier.items.map((i) => i.msg.id)).toEqual(['ap-2', 'men-1']);

    // Gone marker on men-1 prunes it and cancels its auto-dismiss timer.
    clearSpy.mockClear();
    pushLive(group, [{ r: 'msg', id: 'men-1', seq: 10, gone: true }]);
    flushSync();
    expect(clearSpy).toHaveBeenCalledTimes(1);
    expect(notifier.items.map((i) => i.msg.id)).toEqual(['ap-2']);

    // Thread-only mention stays when opening the chat, while main-line ap-2 is pruned.
    pushLive(group, [
      row<p.MsgRow>('row-msg-question', {
        id: 'thread-men',
        seq: 11,
        pos: 11,
        thread: taskId,
        main: false,
        mentions: [ownerId],
      }),
    ]);
    flushSync();
    expect(notifier.items.map((i) => i.msg.id)).toEqual(['ap-2', 'thread-men']);
    setRoute(`#/groups/${website}`);
    flushSync();
    expect(notifier.items.map((i) => i.msg.id)).toEqual(['thread-men']);

    // Removing the group from home.groups prunes remaining notifications of that group.
    home.groups = [];
    flushSync();
    expect(notifier.items).toEqual([]);

    // Removing the home from homes prunes notifications of that home.
    setRoute('#/nodes');
    home.groups = [group];
    flushSync();
    pushLive(group, [row<p.MsgRow>('row-msg-brief-pending', { id: 'br-1', seq: 12, pos: 12 })]);
    flushSync();
    expect(notifier.items.map((i) => i.msg.id)).toEqual(['br-1']);
    homes.splice(0);
    flushSync();
    expect(notifier.items).toEqual([]);

    stop();
  });

  it('prunes when a row disappears from group.rows and handles repeated dismiss and clear', () => {
    const clearSpy = vi.spyOn(globalThis, 'clearTimeout');
    const notifier = new Notifier();
    const { home, group } = makeReadyGroup();
    notifier.sync([home]);

    pushLive(group, [
      row<p.MsgRow>('row-msg-text', {
        id: 'men-a',
        seq: 6,
        pos: 6,
        by: codexId,
        mentions: [ownerId],
      }),
      row<p.MsgRow>('row-msg-text', {
        id: 'men-b',
        seq: 7,
        pos: 7,
        by: codexId,
        mentions: [ownerId],
      }),
    ]);
    notifier.sync([home]);
    const firstKey = notifier.items[0].key;
    clearSpy.mockClear();
    notifier.dismiss(firstKey);
    expect(clearSpy).toHaveBeenCalledTimes(1);
    notifier.dismiss(firstKey);
    expect(clearSpy).toHaveBeenCalledTimes(1);

    group.rows.delete('msg men-b');
    notifier.sync([home]);
    expect(notifier.items).toEqual([]);

    pushLive(group, [row<p.MsgRow>('row-msg-summary', { id: 'sum-clear', seq: 8, pos: 8 })]);
    notifier.sync([home]);
    clearSpy.mockClear();
    notifier.clear();
    expect(clearSpy).toHaveBeenCalledTimes(1);
    expect(notifier.items).toEqual([]);
    notifier.clear();
    expect(clearSpy).toHaveBeenCalledTimes(1);
  });

  it('suppresses arrival in an open main line and opens target messages across routes', async () => {
    const notifier = new Notifier();
    const { home, group } = makeReadyGroup();
    setRoute(`#/groups/${website}`);
    notifier.sync([home]);

    // Main-line arrival while open is suppressed; thread-only mention notifies.
    pushLive(group, [
      row<p.MsgRow>('row-msg-approval-pending', { id: 'suppressed-ap', seq: 6, pos: 6 }),
      row<p.MsgRow>('row-msg-question', {
        id: 'open-thread-men',
        seq: 7,
        pos: 7,
        thread: taskId,
        main: false,
        mentions: [ownerId],
      }),
    ]);
    notifier.sync([home]);
    expect(notifier.items.map((i) => i.msg.id)).toEqual(['open-thread-men']);

    // Opening while already on the same chat does not dispatch hashchange, and focuses target.
    const taskEl = document.createElement('div');
    taskEl.id = `m-${taskId}`;
    taskEl.scrollIntoView = vi.fn();
    document.body.appendChild(taskEl);
    const hashSpy = vi.fn();
    addEventListener('hashchange', hashSpy);
    const sameChatEvent = new MouseEvent('click', { cancelable: true });
    await notifier.open(sameChatEvent, notifier.items[0]);
    expect(sameChatEvent.defaultPrevented).toBe(true);
    expect(hashSpy).not.toHaveBeenCalled();
    expect(document.activeElement).toBe(taskEl);
    expect(notifier.items).toEqual([]);

    // Opening from terminal route with same g navigates to chat and focuses decision input.
    setRoute(`#/terminal/${website}/${codexId}`);
    hashSpy.mockClear();
    pushLive(group, [row<p.MsgRow>('row-msg-decision-pending', { id: 'dc-jump', seq: 8, pos: 8 })]);
    notifier.sync([home]);
    const dcEl = document.createElement('div');
    dcEl.id = 'm-dc-jump';
    dcEl.scrollIntoView = vi.fn();
    const input = document.createElement('input');
    dcEl.appendChild(input);
    document.body.appendChild(dcEl);

    const crossRouteEvent = new MouseEvent('click', { cancelable: true });
    const opening = notifier.open(crossRouteEvent, notifier.items[0]);
    expect(crossRouteEvent.defaultPrevented).toBe(true);
    await opening;
    expect(route.page).toBe('chat');
    expect(route.params.g).toBe(website);
    expect(hashSpy).toHaveBeenCalledTimes(1);
    expect(document.activeElement).toBe(input);

    // Opening from another group's chat switches group.
    setRoute(`#/groups/${otherGroup}`);
    hashSpy.mockClear();
    pushLive(group, [row<p.MsgRow>('row-msg-brief-pending', { id: 'br-jump', seq: 9, pos: 9 })]);
    notifier.sync([home]);
    const brEl = document.createElement('div');
    brEl.id = 'm-br-jump';
    brEl.scrollIntoView = vi.fn();
    const btn = document.createElement('button');
    brEl.appendChild(btn);
    document.body.appendChild(brEl);

    await notifier.open(new MouseEvent('click', { cancelable: true }), notifier.items[0]);
    expect(route.page).toBe('chat');
    expect(route.params.g).toBe(website);
    expect(hashSpy).toHaveBeenCalledTimes(1);
    expect(document.activeElement).toBe(btn);
    removeEventListener('hashchange', hashSpy);
  });
});
