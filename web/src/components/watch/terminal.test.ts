import { flushSync } from 'svelte';
import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import { closePage, go, show } from '../../../features/page';
import { Group, Home, homes } from '../../lib/groups.svelte';
import { goldens } from '../../lib/mock';
import { prefs } from '../../lib/prefs.svelte';
import * as p from '../../lib/proto';
import * as terminal from '../../lib/terminal.svelte';
import Terminal from '../../routes/Terminal.svelte';
import ReminderSync from '../reminder/ReminderSync.svelte';

const state = vi.hoisted(() => ({ instances: [] as any[], fit: vi.fn() }));
vi.mock('@xterm/xterm', () => ({
  Terminal: class {
    cols = 80;
    rows = 24;
    textarea = document.createElement('textarea');
    written: unknown[] = [];
    resize = vi.fn((cols: number, rows: number) => {
      this.cols = cols;
      this.rows = rows;
    });
    dispose = vi.fn();
    onData = vi.fn();
    onBinary = vi.fn();
    onResize = vi.fn();
    clear = vi.fn();
    focus = vi.fn();
    constructor(readonly options: object) {
      state.instances.push(this);
    }
    loadAddon() {}
    open() {}
    attachCustomKeyEventHandler() {}
    write(chunk: unknown) {
      this.written.push(chunk);
    }
  },
}));
vi.mock('@xterm/addon-fit', () => ({
  FitAddon: class {
    fit = state.fit;
  },
}));
vi.mock('@xterm/xterm/css/xterm.css', () => ({}));

beforeEach(() => {
  prefs.lang = 'en';
  state.instances.length = 0;
  state.fit.mockClear();
});
afterEach(closePage);

/** The examples' group with an AI in a takeover, read by its owner or by someone else. */
function setup(reader: 'owner' | 'watcher') {
  const person = p.decode(p.HumanMemberRow, goldens['row-member-human']);
  const ai = p.decode(p.AIMemberRow, goldens['row-member-ai']);
  ai.state = 'takeover';
  const user = reader === 'owner' ? ai.user : 'viewer';
  const home = new Home({ user, sign: async (payload) => ({ p: payload, k: 'key', s: 'sig' }) });
  const group = new Group(home, ai.g);
  group.apply([
    { ...person, id: 'q', name: 'Q', user: ai.user },
    { ...person, id: 'p', name: 'P', user: 'viewer' },
    ai,
  ]);
  home.groups = [group];
  let seq = 900;
  /** The home writes P's request to watch, or settles it. */
  const card = (state: object, changes: object = {}) => {
    const row = p.decode(p.Row, goldens['row-msg-confirm-pending']);
    const body = { what: 'watch', ai: ai.id, to: [ai.user] };
    group.apply([
      { ...row, id: 'ask', by: 'p', seq: (seq += 1), body, state, ...changes } as p.Row,
    ]);
    flushSync();
  };
  const channel: p.PtyChannel = {
    binaryType: 'blob',
    readyState: 'open',
    onmessage: null,
    onclose: null,
    send: vi.fn(),
    close: vi.fn(),
  };
  const settled = (status: string) => ({ status, by: 'q', at: ai.since });
  homes.push(home);
  return { home, group, ai, card, settled, channel };
}

const params = ({ ai }: { ai: p.AIMemberRow }) => ({ g: ai.g, member: ai.id });
const direct = () => ({ kind: 'direct' as const, rtt: 1, since: 0 });
const words = (selector: string) =>
  document.querySelector(selector)?.textContent?.replace(/\s+/g, ' ').trim();

it('waits on the page for the consent, then reaches the computer and starts by itself', async () => {
  vi.useFakeTimers();
  const s = setup('watcher');
  const asked: string[] = [];
  s.home.ctl = {
    request: (async ({ env }: { env: p.Envelope }) => {
      asked.push(JSON.parse(env.p).t);
      s.card({ status: 'pending' });
      return { re: 1, ok: true };
    }) as unknown as p.Ctl['request'],
  };
  // As net does: the first time nothing answers, with the consent the computer does.
  const start = vi
    .fn()
    .mockResolvedValueOnce({ re: 1, err: { code: 'forbidden', msg: 'the grant is on its way' } })
    .mockResolvedValue({ re: 1, ok: true });
  const held: string[] = [];
  const host = new Home(s.home.me);
  host.device = s.ai.host;
  const watchHost = vi.fn((id: string) => {
    held.push(id);
    if (!homes.includes(host)) homes.push(host);
    else if (!host.ctl) host.ctl = { request: start as p.Ctl['request'], channel: () => s.channel };
    return () => {
      held.pop();
      if (!held.length) homes.splice(homes.indexOf(host), 1);
    };
  });
  const net = { watchHost, source: 'mock' as const, homes, path: direct };
  show(Terminal, { params: params(s), net });
  expect(document.querySelector('[aria-label="Watching"]')).toBeNull();
  expect(words('.takeover')).toBe(
    "The owner's consent is needed With it you can watch, but not type or resize the owner's terminal. Ask to watch Back to chat",
  );
  document.querySelector<HTMLButtonElement>('.takeover button')!.click();
  await vi.advanceTimersByTimeAsync(0);
  expect(words('.takeover')).toBe(
    'Waiting for Q The terminal opens here by itself once your request is confirmed. Back to chat',
  );
  expect([asked, watchHost.mock.calls, start.mock.calls]).toEqual([['post'], [[s.ai.host]], []]);

  s.card(s.settled('confirmed'));
  await vi.advanceTimersByTimeAsync(0);
  expect([watchHost.mock.calls.length, held, homes.includes(host)]).toEqual([2, [s.ai.host], true]);
  expect(start.mock.calls).toEqual([[{ t: 'watch.start', g: s.ai.g, member: s.ai.id }]]);
  expect(words('.takeover b')).toMatch(/^Confirmed by Q · /);
  await vi.advanceTimersByTimeAsync(1000);
  expect(words('.takeover b')).toBe('Watching · receive only');
  expect([start.mock.calls.length, watchHost.mock.calls.length]).toEqual([2, 2]);
  await closePage();
  await vi.advanceTimersByTimeAsync(0);
  expect([held, homes.includes(host)]).toEqual([[], false]);
  vi.useRealTimers();
});

/** A watcher's page with the consent of the owner and the AI's computer. */
async function watcherPage(start: ReturnType<typeof vi.fn>) {
  vi.useFakeTimers();
  const s = setup('watcher');
  s.home.ctl = { request: vi.fn() as p.Ctl['request'] };
  s.card(s.settled('confirmed'));
  const host = new Home(s.home.me);
  host.device = s.ai.host;
  host.ctl = { request: start as p.Ctl['request'], channel: () => s.channel };
  homes.push(host);
  const release = vi.fn();
  const watchHost = vi.fn(() => release);
  show(Terminal, {
    params: params(s),
    net: { watchHost, source: 'mock' as const, homes, path: direct },
  });
  await vi.advanceTimersByTimeAsync(0);
  let seq = 950;
  const ai = async (state: p.AIMemberRow['state']) => {
    s.group.apply([{ ...s.ai, seq: (seq += 1), state }]);
    await vi.advanceTimersByTimeAsync(0);
  };
  return { ...s, watchHost, release, setAI: ai };
}

it('offers to ask again when a start stays refused, with one button that keeps the focus', async () => {
  const refused = { re: 1, err: { code: 'forbidden', msg: '' } };
  // A while after the question, so that the page is seen between the two.
  const start = vi.fn(() => new Promise((answer) => setTimeout(answer, 100, refused)));
  await watcherPage(start);
  expect(words('.takeover')).toMatch(
    /^Confirmed by Q · \S+ The terminal opens here by itself once the computer has the consent\. If it does not, ask again\. Back to chat$/,
  );
  await vi.advanceTimersByTimeAsync(1200);
  expect(start).toHaveBeenCalledTimes(2);
  const button = document.querySelector<HTMLButtonElement>('.takeover button')!;
  expect([words('.takeover button'), button.disabled]).toEqual(['Ask to watch', false]);
  button.focus();
  const changes: MutationRecord[] = [];
  const observer = new MutationObserver((records) => changes.push(...records));
  observer.observe(document.querySelector('.takeover')!, { childList: true, subtree: true });
  await vi.advanceTimersByTimeAsync(600_000);
  expect(start.mock.calls.length).toBeGreaterThan(5);
  expect(document.querySelector('.takeover button')).toBe(button);
  expect([document.activeElement, changes]).toEqual([button, []]);
  observer.disconnect();
  vi.useRealTimers();
});

it('lets go of the computer and asks it nothing more once the takeover has ended', async () => {
  const start = vi.fn().mockResolvedValue({ re: 1, err: { code: 'forbidden', msg: '' } });
  const s = await watcherPage(start);
  expect([s.watchHost.mock.calls.length, s.release.mock.calls.length]).toEqual([1, 0]);
  await s.setAI('waiting');
  expect([s.watchHost.mock.calls.length, s.release.mock.calls.length]).toEqual([1, 1]);
  await vi.advanceTimersByTimeAsync(600_000);
  expect(start).toHaveBeenCalledTimes(1);
  await s.setAI('takeover');
  expect([s.watchHost.mock.calls.length, start.mock.calls.length]).toEqual([2, 2]);
  vi.useRealTimers();
});

it('keeps an open watch while the computer of the AI has lost the home', async () => {
  const s = await watcherPage(vi.fn().mockResolvedValue({ re: 1, ok: true }));
  expect(words('.takeover b')).toBe('Watching · receive only');
  await s.setAI('offline');
  expect([words('.takeover b'), s.release.mock.calls]).toEqual(['Watching · receive only', []]);
  s.channel.onclose!({});
  await vi.advanceTimersByTimeAsync(0);
  expect(s.release).toHaveBeenCalledTimes(1);
  vi.useRealTimers();
});

it('says that a request was denied and that the takeover has ended', () => {
  const s = setup('watcher');
  s.card({ status: 'pending' });
  show(Terminal, {
    params: params(s),
    net: { source: 'mock' as const, homes, path: () => undefined },
  });
  expect(words('.takeover b')).toBe('Waiting for Q');
  expect(document.querySelector('.takeover button')).toBeNull();
  s.card(s.settled('denied'));
  expect(words('.takeover b')).toMatch(/^Denied by Q · /);
  expect(words('.takeover button')).toBe('Ask to watch');
  s.group.apply([{ ...s.ai, seq: 950, state: 'waiting', since: s.ai.since + 1 }]);
  flushSync();
  expect(words('.takeover b')).toBe('The takeover has ended');
  expect(document.querySelector('.takeover button')).toBeNull();
});

/** The owner's page of the terminal, taken over. */
async function ownerPage() {
  const s = setup('owner');
  s.home.device = s.ai.host;
  const sent: { t: string; action?: string; msg?: string }[] = [];
  let refuse = false;
  s.home.ctl = {
    request: (async (req: { t: string; env: p.Envelope }) => {
      if (req.t !== 'cmd') return { re: 1, err: { code: 'forbidden', msg: '' } };
      const { t, action, msg } = JSON.parse(req.env.p);
      sent.push({ t, action, msg });
      return refuse ? { re: 1, err: { code: 'conflict', msg: '' } } : { re: 1, ok: true };
    }) as unknown as p.Ctl['request'],
    channel: () => s.channel,
    send: vi.fn(),
  };
  const net = { source: 'mock' as const, homes, path: direct };
  vi.stubGlobal(
    'ResizeObserver',
    class {
      observe() {}
      disconnect() {}
    },
  );
  const address = `#/terminal/${s.ai.g}/${s.ai.id}`;
  go(address);
  show(Terminal, { params: params(s), net });
  await vi.waitFor(() => expect(state.instances.length).toBe(1));
  flushSync();
  const buttons = () => [
    ...document.querySelectorAll<HTMLButtonElement>('.asked div:not([inert]) button'),
  ];
  return { ...s, address, net, sent, buttons, refuse: () => (refuse = true) };
}

it('lets the owner answer on the terminal page, without taking the focus or ending the takeover', async () => {
  const s = await ownerPage();
  const region = document.querySelector('.asked')!;
  expect([region.getAttribute('aria-live'), s.buttons()]).toEqual(['polite', []]);
  const typing = document.createElement('textarea');
  document.body.append(typing);
  typing.focus();
  s.card({ status: 'pending' });
  expect(words('.asked p')).toBe('Let P watch Codex’s terminal.');
  expect(s.buttons().map((button) => button.textContent!.trim())).toEqual(['Confirm', 'Deny']);
  expect(document.activeElement).toBe(typing);
  s.buttons()[0].click();
  await vi.waitFor(() =>
    expect(s.sent).toEqual([{ t: 'takeover.start' }, { t: 'act', action: 'confirm', msg: 'ask' }]),
  );
  expect(region.contains(document.activeElement)).toBe(true);
  s.card(s.settled('confirmed'));
  expect(s.buttons()).toEqual([]);
  expect(state.instances[0].focus).toHaveBeenCalledExactlyOnceWith();
  expect(s.channel.close).not.toHaveBeenCalled();
  typing.remove();
});

it('drops a request that is answered elsewhere, and shows a refused answer', async () => {
  const s = await ownerPage();
  s.card({ status: 'pending' });
  s.card(s.settled('denied'));
  expect(s.buttons()).toEqual([]);
  expect(state.instances[0].focus).not.toHaveBeenCalled();
  s.refuse();
  s.card({ status: 'pending' }, { id: 'again' });
  s.buttons()[1].click();
  await vi.waitFor(() => expect(words('.asked .fail code')).toBe('conflict'));
  expect(s.sent.at(-1)).toEqual({ t: 'act', action: 'deny', msg: 'again' });
});

it('shows no other card, and none outside the takeover', async () => {
  const s = await ownerPage();
  s.card(
    { status: 'pending' },
    { id: 'a', body: { what: 'direct', ai: s.ai.id, to: [s.ai.user] } },
  );
  s.card({ status: 'pending' }, { id: 'b', body: { what: 'watch', ai: 'other', to: [s.ai.user] } });
  expect(s.buttons()).toEqual([]);
  s.card({ status: 'pending' });
  expect(s.buttons().length).toBe(2);
  s.group.apply([{ ...s.ai, seq: 950, state: 'waiting' }]);
  flushSync();
  expect(s.buttons()).toEqual([]);
});

/** The service worker of the page: what tells it that a system notification was clicked. */
function notifications() {
  let open = (_: MessageEvent) => {};
  Object.defineProperty(navigator, 'serviceWorker', {
    configurable: true,
    value: {
      addEventListener: (_: string, fn: typeof open) => (open = fn),
      removeEventListener() {},
    },
  });
  return {
    click: (data: object) => open(new MessageEvent('message', { data })),
    end: () => Reflect.deleteProperty(navigator, 'serviceWorker'),
  };
}

it('brings a system notification of the request to its card, and not to the chat', async () => {
  const s = await ownerPage();
  const { click, end } = notifications();
  show(ReminderSync, { net: s.net });
  s.card({ status: 'pending' });
  s.card(
    { status: 'pending' },
    { id: 'b', body: { what: 'direct', ai: s.ai.id, to: [s.ai.user] } },
  );
  const data = { g: s.ai.g, msg: 'ask', kind: 'confirm' };
  click(data);
  expect(location.hash).toBe(s.address);
  expect(document.activeElement).toBe(s.buttons()[0]);
  click({ ...data, msg: 'b' });
  expect(location.hash).toBe(`#/groups/${s.ai.g}`);
  end();
});

it('fetches the card of a system notification that it does not hold before it leaves', async () => {
  const s = await ownerPage();
  const { click, end } = notifications();
  show(ReminderSync, { net: s.net });
  const row = (id: string, changes: object = {}) => ({
    ...p.decode(p.Row, goldens['row-msg-confirm-pending']),
    id,
    by: 'p',
    body: { what: 'watch', ai: s.ai.id, to: [s.ai.user] },
    ...changes,
  });
  const rows: Record<string, object> = {
    ask: row('ask'),
    b: row('b', { body: { what: 'direct', ai: s.ai.id, to: [s.ai.user] } }),
  };
  const fetched: string[] = [];
  const online: p.Ctl = {
    request: (async (req: { t: string; row: string }) => {
      if (req.t !== 'fetch') return { re: 1, err: { code: 'forbidden', msg: '' } };
      fetched.push(req.row);
      return { re: 1, ok: true, rows: [rows[req.row]], more: false };
    }) as unknown as p.Ctl['request'],
  };
  const data = { g: s.ai.g, kind: 'confirm' };
  // The home is being reconnected: the card is not here yet.
  s.home.ctl = undefined;
  s.group.live = true;
  click({ ...data, msg: 'ask' });
  flushSync();
  expect([fetched, location.hash]).toEqual([[], s.address]);
  s.group.live = false;
  flushSync();
  s.home.ctl = online;
  s.group.live = true;
  await vi.waitFor(() => expect(document.activeElement).toBe(s.buttons()[0] ?? null));
  expect([fetched, location.hash]).toEqual([['ask'], s.address]);
  // Another card is in the chat.
  click({ ...data, msg: 'b' });
  expect(location.hash).toBe(s.address);
  await vi.waitFor(() => expect(location.hash).toBe(`#/groups/${s.ai.g}`));
  expect(fetched).toEqual(['ask', 'b']);
  // What is no confirm card is in the chat for certain.
  go(s.address);
  click({ ...data, msg: 'c', kind: 'mention' });
  expect(location.hash).toBe(`#/groups/${s.ai.g}`);
  end();
});

it('opens the receive-only terminal, applies initial and pushed sizes, and closes on pagehide', async () => {
  const ai = p.decode(p.AIMemberRow, goldens['row-member-ai']);
  const home = new Home({ user: 'viewer', sign: async () => ({ p: '', k: '', s: '' }) });
  const group = new Group(home, ai.g);
  group.apply([p.decode(p.Row, goldens['row-group']), { ...ai, state: 'takeover' }]);
  home.groups = [group];
  home.device = ai.host;
  const channel: p.PtyChannel = {
    binaryType: 'blob',
    readyState: 'open',
    onmessage: null,
    onclose: null,
    send: vi.fn(),
    close: vi.fn(),
  };
  const request = vi.fn().mockResolvedValue({ re: 1, ok: true });
  const send = vi.fn();
  const open = vi.fn(() => channel);
  const release = vi.fn();
  const watchHost = vi.fn(() => release);
  home.ctl = { request: request as p.Ctl['request'], send, channel: open };
  homes.push(home);
  const size = { t: 'pty.resize' as const, g: ai.g, ai: ai.id, cols: 100, rows: 30 };
  home.terminalSizes.set(`${ai.g}:${ai.id}`, size);
  show(Terminal, {
    params: { g: ai.g, member: ai.id },
    net: {
      watchHost,
      source: 'mock' as const,
      homes,
      path: () => ({ kind: 'direct' as const, rtt: 1, since: 0 }),
    },
  });
  await vi.waitFor(() => expect(state.instances.length).toBe(1));
  flushSync();
  const term = state.instances[0];
  expect(term.options.disableStdin).toBe(true);
  expect([term.cols, term.rows]).toEqual([100, 30]);
  expect(document.querySelector('.keys')).toBeNull();
  expect(document.body.textContent).not.toContain('Queued messages');
  expect(document.body.textContent).not.toContain('Tab types into the terminal');
  expect(document.querySelector('.takeover')!.textContent).toContain('Watching · receive only');
  channel.onmessage!({ data: new Uint8Array([27, 91, 50, 74]) });
  expect(term.written).toEqual([new Uint8Array([27, 91, 50, 74])]);
  home.terminalSizes.set(`${ai.g}:${ai.id}`, { ...size, cols: 120, rows: 40 });
  flushSync();
  expect([term.cols, term.rows]).toEqual([120, 40]);
  expect(term.onData.mock.calls).toEqual([]);
  expect(term.onBinary.mock.calls).toEqual([]);
  expect(term.onResize.mock.calls).toEqual([]);
  expect([send.mock.calls, (channel.send as ReturnType<typeof vi.fn>).mock.calls]).toEqual([
    [],
    [],
  ]);
  expect(watchHost).toHaveBeenCalledExactlyOnceWith(ai.host);
  expect(release).toHaveBeenCalledTimes(0);
  channel.onclose!({});
  await vi.waitFor(() => expect(release).toHaveBeenCalledTimes(1));
  dispatchEvent(new Event('pagehide'));
  expect(channel.close).toHaveBeenCalledExactlyOnceWith();
  expect(request.mock.calls).toEqual([[{ t: 'watch.start', g: ai.g, member: ai.id }]]);
});

it('buffers early owner output, forwards binary input and refits after a viewport resize', async () => {
  const ai = p.decode(p.AIMemberRow, goldens['row-member-ai']);
  const home = new Home({ user: ai.user, sign: async () => ({ p: '', k: '', s: '' }) });
  const group = new Group(home, ai.g);
  group.apply([ai]);
  home.groups = [group];
  home.device = ai.host;
  const send = vi.fn();
  const channel: p.PtyChannel = {
    binaryType: 'blob',
    readyState: 'open',
    onmessage: null,
    onclose: null,
    send,
    close: vi.fn(),
  };
  let message: p.PtyChannel['onmessage'];
  Object.defineProperty(channel, 'onmessage', {
    get: () => message,
    set: (next: p.PtyChannel['onmessage']) => {
      message = next;
      next?.({ data: new Uint8Array([65]) });
    },
  });
  let resized = () => {};
  vi.stubGlobal(
    'ResizeObserver',
    class {
      constructor(callback: () => void) {
        resized = callback;
      }
      observe() {}
      disconnect() {}
    },
  );
  home.ctl = {
    request: vi
      .fn()
      .mockImplementation(async (req: { t: string }) =>
        req.t === 'host.info'
          ? { ...p.decode(p.HostInfoPush, goldens['host-info-push']), re: 1, ok: true }
          : { re: 1, ok: true },
      ) as p.Ctl['request'],
    channel: () => channel,
    send: vi.fn(),
  };
  homes.push(home);
  show(Terminal, {
    params: { g: ai.g, member: ai.id },
    net: {
      source: 'mock' as const,
      homes,
      path: () => ({ kind: 'direct' as const, rtt: 1, since: 0 }),
    },
  });
  await vi.waitFor(() => expect(state.instances.length).toBe(1));
  const term = state.instances[0];
  expect(term.options.disableStdin).toBe(false);
  expect(term.written).toEqual([new Uint8Array([65])]);
  term.onBinary.mock.calls[0][0]('\xff');
  expect(send.mock.calls).toEqual([[new Uint8Array([255])]]);
  expect(state.fit.mock.calls).toEqual([[]]);
  resized();
  expect(state.fit.mock.calls).toEqual([[], []]);
});

it('renders a status that has no optional detail or error', () => {
  vi.spyOn(terminal, 'takeoverBarState').mockReturnValue({
    title: 'Terminal status',
    handback: false,
  });
  show(Terminal, {
    params: { g: 'missing', member: 'missing' },
    net: {
      source: 'mock' as const,
      homes,
      path: () => undefined,
    },
  });
  expect(document.querySelector('.takeover')!.textContent!.replace(/\s+/g, ' ').trim()).toBe(
    'Terminal status Back to chat',
  );
});
