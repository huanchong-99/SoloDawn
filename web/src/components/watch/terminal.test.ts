import { flushSync } from 'svelte';
import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import { closePage, show } from '../../../features/page';
import { Group, Home, homes } from '../../lib/groups.svelte';
import { goldens } from '../../lib/mock';
import { prefs } from '../../lib/prefs.svelte';
import * as p from '../../lib/proto';
import * as terminal from '../../lib/terminal.svelte';
import Terminal from '../../routes/Terminal.svelte';

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
  home.ctl = { request: request as p.Ctl['request'], send, channel: open };
  homes.push(home);
  const size = { t: 'pty.resize' as const, g: ai.g, ai: ai.id, cols: 100, rows: 30 };
  home.terminalSizes.set(`${ai.g}:${ai.id}`, size);
  show(Terminal, {
    params: { g: ai.g, member: ai.id },
    net: {
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
    request: vi.fn().mockResolvedValue({ re: 1, ok: true }) as p.Ctl['request'],
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
