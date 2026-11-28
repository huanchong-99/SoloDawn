// @vitest-environment happy-dom
import { flushSync, mount, unmount } from 'svelte';
import { SvelteMap } from 'svelte/reactivity';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { announcement, prefs } from '../../lib/prefs.svelte';
import { messages } from '../../messages/texts';
import FolderGate from './FolderGate.svelte';
import Permit from './Permit.svelte';
import SystemAsks from './SystemAsks.svelte';
import { asks, browserPlatform, gate, needs, rows, type Permissions } from './permit.svelte.ts';

const perms = (overrides: Partial<Permissions> = {}): Permissions => ({
  local_network: true,
  incoming: true,
  full_disk: false,
  ...overrides,
});

describe('rows (every row and each condition operand flipped alone)', () => {
  it('Local network: allowed ignores opened states vs disallowed states', () => {
    const [okClosed] = rows(perms({ local_network: true }), {});
    expect(okClosed).toEqual({
      id: 'local_network',
      title: 'permit.lan',
      ok: true,
      mark: 'ok',
      line: 'permit.lan.ok',
    });

    const [okWithAsk] = rows(perms({ local_network: true }), { local_network: 'ask' });
    expect(okWithAsk.coach).toBeUndefined();
    expect(okWithAsk.action).toBeUndefined();

    const [okWithSettings] = rows(perms({ local_network: true }), { local_network: 'settings' });
    expect(okWithSettings.coach).toBeUndefined();

    const [todoClosed] = rows(perms({ local_network: false }));
    expect(todoClosed).toEqual({
      id: 'local_network',
      title: 'permit.lan',
      ok: false,
      mark: 'need',
      line: 'permit.lan.todo',
      action: { label: 'permit.lan.ask', primary: true },
      coach: undefined,
    });

    const [todoAsk] = rows(perms({ local_network: false }), { local_network: 'ask' });
    expect(todoAsk.action).toBeUndefined();
    expect(todoAsk.coach).toEqual({
      sketch: 'alert',
      line: 'permit.lan.coach',
      buttons: [
        { label: 'permit.lan.nowindow', variant: 'link', action: 'nowindow' },
        { label: 'permit.again', variant: 'ghost', action: 'again' },
      ],
    });

    const [todoSettings] = rows(perms({ local_network: false }), { local_network: 'settings' });
    expect(todoSettings.action).toBeUndefined();
    expect(todoSettings.coach).toEqual({
      line: 'permit.lan.settings',
      buttons: [{ label: 'permit.again', variant: 'ghost', action: 'again' }],
    });
  });

  it('Incoming connections: allowed ignores opened vs blocked closed and open', () => {
    const [, okClosed] = rows(perms({ incoming: true }), { incoming: false });
    expect(okClosed).toEqual({
      id: 'incoming',
      title: 'permit.in',
      ok: true,
      mark: 'ok',
      line: 'permit.in.ok',
    });

    const [, okOpened] = rows(perms({ incoming: true }), { incoming: true });
    expect(okOpened.coach).toBeUndefined();

    const [, blockedClosed] = rows(perms({ incoming: false }), { incoming: false });
    expect(blockedClosed).toEqual({
      id: 'incoming',
      title: 'permit.in',
      ok: false,
      mark: 'need',
      line: 'permit.in.todo',
      hint: undefined,
      action: { label: 'permit.in.open', primary: false },
      coach: undefined,
    });

    const [, blockedOpen] = rows(perms({ incoming: false }), { incoming: true });
    expect(blockedOpen.action).toBeUndefined();
    expect(blockedOpen.coach).toEqual({
      line: 'permit.in.settings',
      buttons: [{ label: 'permit.again', variant: 'ghost', action: 'again' }],
    });
  });

  it('Files: Full Disk Access on ignores opened vs per-folder closed and open', () => {
    const [, , okClosed] = rows(perms({ full_disk: true }), { full_disk: false });
    expect(okClosed).toEqual({
      id: 'full_disk',
      title: 'permit.files',
      ok: true,
      mark: 'ok',
      line: 'permit.files.full',
    });

    const [, , okOpened] = rows(perms({ full_disk: true }), { full_disk: true });
    expect(okOpened.coach).toBeUndefined();

    const [, , perFolderClosed] = rows(perms({ full_disk: false }), { full_disk: false });
    expect(perFolderClosed).toEqual({
      id: 'full_disk',
      title: 'permit.files',
      ok: false,
      mark: 'free',
      line: 'permit.files.ask',
      hint: 'permit.files.hint',
      action: { label: 'permit.files.all', primary: false },
      coach: undefined,
    });

    const [, , perFolderOpen] = rows(perms({ full_disk: false }), { full_disk: true });
    expect(perFolderOpen.action).toBeUndefined();
    expect(perFolderOpen.coach).toEqual({
      sketch: 'drag',
      line: 'permit.files.coach',
      more: 'permit.files.plus',
      buttons: [
        { label: 'permit.again', variant: 'ghost', action: 'again' },
        { label: 'permit.hide', variant: 'ghost', action: 'hide' },
      ],
    });
  });
});

describe('needs', () => {
  it('requires user when local_network or incoming is false, ignoring full_disk', () => {
    expect(needs({ local_network: true, incoming: true, full_disk: false })).toBe(false);
    expect(needs({ local_network: true, incoming: true, full_disk: true })).toBe(false);
    expect(needs({ local_network: false, incoming: true, full_disk: true })).toBe(true);
    expect(needs({ local_network: true, incoming: false, full_disk: true })).toBe(true);
    expect(needs({ local_network: false, incoming: false, full_disk: false })).toBe(true);
  });
});

describe('asks and browserPlatform', () => {
  it('classifies Mac, Windows, Linux, and mobile/unknown platform strings', () => {
    expect(asks('MacIntel')).toBe('mac');
    expect(asks('macOS')).toBe('mac');
    expect(asks('Darwin')).toBe('mac');
    expect(asks('Mozilla/5.0 (Macintosh; Intel Mac OS X 14_5)')).toBe('mac');
    expect(asks('Win32')).toBe('win');
    expect(asks('Windows')).toBe('win');
    expect(asks('Mozilla/5.0 (Windows NT 10.0; Win64; x64)')).toBe('win');
    expect(asks('Linux x86_64')).toBe('none');
    expect(asks('Mozilla/5.0 (X11; Linux x86_64)')).toBe('none');
    expect(asks('iPhone')).toBe('both');
    expect(asks('Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X)')).toBe('both');
    expect(asks('Mozilla/5.0 (iPad; CPU OS 17_0 like Mac OS X)')).toBe('both');
    expect(asks('Mozilla/5.0 (iPod touch; CPU iPhone OS 15_0 like Mac OS X)')).toBe('both');
    expect(asks('Mozilla/5.0 (Linux; Android 14; Pixel 8)')).toBe('both');
    expect(asks('Mobile')).toBe('both');
    expect(asks('CrOS x86_64')).toBe('both');
    expect(asks('')).toBe('both');
  });

  it('picks platform in order userAgentData -> mobile UA -> platform -> userAgent', () => {
    expect(
      browserPlatform({
        userAgentData: { platform: 'Windows' },
        platform: 'MacIntel',
        userAgent: 'Linux',
      }),
    ).toBe('Windows');
    expect(
      browserPlatform({
        userAgentData: { platform: '' },
        platform: 'Linux armv8l',
        userAgent: 'Mozilla/5.0 (Linux; Android 14)',
      }),
    ).toBe('Mozilla/5.0 (Linux; Android 14)');
    expect(
      browserPlatform({
        userAgentData: {},
        platform: 'MacIntel',
        userAgent: 'Mozilla/5.0 (X11; Linux)',
      }),
    ).toBe('MacIntel');
    expect(
      browserPlatform({
        platform: '',
        userAgent: 'Mozilla/5.0 (X11; Linux x86_64)',
      }),
    ).toBe('Mozilla/5.0 (X11; Linux x86_64)');
    expect(browserPlatform({})).toBe('');
  });
});

describe('gate (folder dialog state machine)', () => {
  beforeEach(() => vi.useFakeTimers());
  afterEach(() => vi.useRealTimers());

  it('waits silently for 500 ms, then shows asking until ok resolves', async () => {
    const g = gate();
    expect(g.value).toEqual({ state: 'idle' });
    expect(g.busy).toBe(false);

    let finish!: (value: { re: number; ok: true }) => void;
    const ask = vi.fn(
      () =>
        new Promise<{ re: number; ok: true }>((resolve) => {
          finish = resolve;
        }),
    );
    const promise = g.check(ask, '/Users/me/Documents/site');
    expect(ask).toHaveBeenCalledWith({ path: '/Users/me/Documents/site' });
    expect(g.value).toEqual({ state: 'waiting' });
    expect(g.busy).toBe(true);

    vi.advanceTimersByTime(499);
    expect(g.value).toEqual({ state: 'waiting' });
    vi.advanceTimersByTime(1);
    expect(g.value).toEqual({ state: 'asking' });
    expect(g.busy).toBe(true);

    finish({ re: 1, ok: true });
    expect(await promise).toEqual({ kind: 'ok' });
    expect(g.value).toEqual({ state: 'idle' });
    expect(g.busy).toBe(false);
  });

  it('clears the 500 ms timer when ok arrives early', async () => {
    const g = gate();
    const res = await g.check(async () => ({ re: 1, ok: true }), '/Users/me/app');
    expect(res).toEqual({ kind: 'ok' });
    vi.advanceTimersByTime(1000);
    expect(g.value).toEqual({ state: 'idle' });
  });

  it('enters refused on permission, switches sentence on openedSettings, and passes non-permission errors', async () => {
    const g = gate();
    g.openedSettings();
    expect(g.value).toEqual({ state: 'idle' });

    const refused = await g.check(
      async () => ({ re: 1, err: { code: 'permission', msg: 'denied' } }),
      '/Users/me/Downloads/x',
    );
    expect(refused).toEqual({ kind: 'permission' });
    expect(g.value).toEqual({ state: 'refused', settings: false });
    expect(g.busy).toBe(false);

    g.openedSettings();
    expect(g.value).toEqual({ state: 'refused', settings: true });

    const notFound = await g.check(
      async () => ({ re: 2, err: { code: 'not_found', msg: 'missing' } }),
      '/missing',
    );
    expect(notFound).toEqual({ kind: 'error', err: { code: 'not_found', msg: 'missing' } });
    expect(g.value).toEqual({ state: 'idle' });
  });

  it('handles reset during in-flight request and rejected ask calls cleanly', async () => {
    const g = gate();
    let finish!: (value: { re: number; ok: true }) => void;
    const pending = g.check(
      () =>
        new Promise((resolve) => {
          finish = resolve;
        }),
      '/Users/me/site',
    );
    g.reset();
    expect(g.value).toEqual({ state: 'idle' });
    vi.advanceTimersByTime(600);
    expect(g.value).toEqual({ state: 'idle' });
    finish({ re: 1, ok: true });
    expect(await pending).toEqual({ kind: 'cancelled' });

    const errored = await g.check(
      async () => ({ re: 3, err: { code: 'lost', msg: '' } }),
      '/Users/me/site',
    );
    expect(errored).toEqual({ kind: 'error', err: { code: 'lost', msg: '' } });
    vi.advanceTimersByTime(600);
    expect(g.value).toEqual({ state: 'idle' });

    let finishFirst!: (value: { re: number; ok: true }) => void;
    let finishSecond!: (value: { re: number; ok: true }) => void;
    const first = g.check(
      () =>
        new Promise((resolve) => {
          finishFirst = resolve;
        }),
      '/first',
    );
    vi.advanceTimersByTime(300);
    const second = g.check(
      () =>
        new Promise((resolve) => {
          finishSecond = resolve;
        }),
      '/second',
    );
    vi.advanceTimersByTime(250);
    expect(g.value).toEqual({ state: 'waiting' });
    vi.advanceTimersByTime(250);
    expect(g.value).toEqual({ state: 'asking' });
    finishFirst({ re: 1, ok: true });
    expect(await first).toEqual({ kind: 'cancelled' });
    expect(g.value).toEqual({ state: 'asking' });
    finishSecond({ re: 2, ok: true });
    expect(await second).toEqual({ kind: 'ok' });
    expect(g.value).toEqual({ state: 'idle' });
  });
});

describe('permit components', () => {
  const prevLang = prefs.lang;
  let host: HTMLDivElement;

  beforeEach(() => {
    prefs.lang = 'en';
    announcement.text = '';
    host = document.createElement('div');
    document.body.appendChild(host);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    host.remove();
    prefs.lang = prevLang;
  });

  it('Permit renders collapsible plain block on Computers, coaches, announcements, and Windows relay', async () => {
    const calls: unknown[] = [];
    const props = new SvelteMap<string, unknown>([
      ['name', 'workstation'],
      ['os', 'darwin/arm64'],
      ['relayed', false],
      ['permissions', perms({ local_network: true, incoming: true, full_disk: false })],
      ['plain', true],
      ['open', false],
    ]);
    const app = mount(Permit, {
      target: host,
      props: {
        get name() {
          return props.get('name') as string;
        },
        get os() {
          return props.get('os') as string;
        },
        get relayed() {
          return props.get('relayed') as boolean;
        },
        get permissions() {
          return props.get('permissions') as Permissions | undefined;
        },
        get plain() {
          return props.get('plain') as boolean;
        },
        get open() {
          return props.get('open') as boolean;
        },
        ask: async (body = {}) => {
          calls.push(body);
          return { re: 1, ok: true as const };
        },
      },
    });
    flushSync();
    expect(calls).toEqual([{}]);
    expect(host.textContent).toContain('macOS permissions · Ready');
    expect(host.querySelectorAll('.perm-rows li')).toHaveLength(0);
    expect(announcement.text).toBe('');

    host.querySelector<HTMLButtonElement>('button.turn')!.click();
    flushSync();
    expect(host.querySelector('.perm-head h3')?.textContent?.trim()).toBe('macOS permissions');
    expect(host.querySelectorAll('.perm-rows li')).toHaveLength(3);

    const allowAll = [...host.querySelectorAll<HTMLButtonElement>('button')].find((b) =>
      b.textContent?.includes('Allow all folders'),
    )!;
    allowAll.click();
    flushSync();
    expect(calls.at(-1)).toEqual({ open: 'full_disk', lang: 'en' });
    const drag1 = host.querySelector('.sk-drag');
    expect(drag1).not.toBeNull();

    const coach = host.querySelector<HTMLElement>('.coach')!;
    coach.dispatchEvent(new PointerEvent('pointerenter', { bubbles: true }));
    flushSync();
    const drag2 = host.querySelector('.sk-drag');
    expect(drag2).not.toBeNull();
    expect(drag2).not.toBe(drag1);

    coach.dispatchEvent(new FocusEvent('focusin', { bubbles: true }));
    flushSync();
    const drag3 = host.querySelector('.sk-drag');
    expect(drag3).not.toBeNull();
    expect(drag3).not.toBe(drag2);

    const hideBtn = [...host.querySelectorAll<HTMLButtonElement>('button')].find((b) =>
      b.textContent?.includes('Hide'),
    )!;
    hideBtn.click();
    flushSync();
    expect(host.querySelector('.sk-drag')).toBeNull();

    props.set('permissions', perms({ local_network: false, incoming: false, full_disk: false }));
    flushSync();
    const btns = [...host.querySelectorAll<HTMLButtonElement>('.perm-rows .act')];
    btns[0].click();
    flushSync();
    expect(calls.at(-1)).toEqual({});
    expect(host.querySelector('.sk-alert')).not.toBeNull();

    const noWin = [...host.querySelectorAll<HTMLButtonElement>('button')].find((b) =>
      b.textContent?.includes('No window? Open System Settings'),
    )!;
    noWin.click();
    flushSync();
    expect(calls.at(-1)).toEqual({ open: 'local_network' });

    const fwBtn = [...host.querySelectorAll<HTMLButtonElement>('.perm-rows .act')].find((b) =>
      b.textContent?.includes('Open System Settings'),
    )!;
    fwBtn.click();
    flushSync();
    expect(calls.at(-1)).toEqual({ open: 'incoming' });

    props.set('permissions', perms({ local_network: true, incoming: true, full_disk: true }));
    flushSync();
    expect(announcement.text).toBe('Allowed: Files');
    expect(host.querySelectorAll('.coach')).toHaveLength(0);

    props.set('permissions', undefined);
    props.set('os', 'windows/amd64');
    props.set('relayed', true);
    flushSync();
    expect(host.textContent).toContain('Windows Security');

    props.set('relayed', false);
    flushSync();
    expect(host.textContent).toBe('');

    unmount(app);
  });

  it('Permit onboarding uses h2, starts local_network open when false, and includes foot login line', () => {
    const app = mount(Permit, {
      target: host,
      props: {
        name: 'studio',
        os: 'darwin/arm64',
        relayed: false,
        permissions: perms({ local_network: false }),
        ask: async () => ({ re: 1, ok: true as const }),
        open: true,
      },
    });
    flushSync();
    expect(host.querySelector('.perm-head h2')?.textContent?.trim()).toBe(
      'Permissions on this Mac',
    );
    expect(host.querySelector('.sk-alert')).not.toBeNull();
    expect(host.textContent).toContain('Background Items Added');
    expect(host.textContent).toContain('After you update SoloDawn');
    unmount(app);
  });

  it('SystemAsks renders per navigator platform and FolderGate renders asking & refused views', () => {
    vi.stubGlobal('navigator', {
      userAgentData: { platform: 'macOS' },
      platform: '',
      userAgent: '',
    });
    const mac = mount(SystemAsks, { target: host });
    flushSync();
    expect(host.textContent).toContain('accept incoming network connections');
    expect(host.textContent).not.toContain('Windows Security');
    unmount(mac);

    vi.stubGlobal('navigator', {
      userAgentData: { platform: 'Linux' },
      platform: 'Linux x86_64',
      userAgent: 'Mozilla/5.0 (X11; Linux x86_64)',
    });
    const linux = mount(SystemAsks, { target: host });
    flushSync();
    expect(host.textContent).toBe('');
    unmount(linux);

    vi.stubGlobal('navigator', {
      userAgentData: undefined,
      platform: 'iPhone',
      userAgent: 'iPhone',
    });
    const both = mount(SystemAsks, { target: host });
    flushSync();
    expect(host.textContent).toContain('On a Mac');
    expect(host.textContent).toContain('On Windows');
    unmount(both);

    const events: string[] = [];
    const gateProps = new SvelteMap<string, ReturnType<typeof gate>['value']>([
      ['state', { state: 'asking' }],
    ]);
    const fg = mount(FolderGate, {
      target: host,
      props: {
        get state() {
          return gateProps.get('state')!;
        },
        computer: 'workstation',
        onsettings: () => events.push('settings'),
        onall: () => events.push('all'),
        onretry: () => events.push('retry'),
      },
    });
    flushSync();
    expect(host.querySelector('.sk-alert')).not.toBeNull();
    expect(host.textContent).toContain('workstation');

    gateProps.set('state', { state: 'refused', settings: false });
    flushSync();
    const actionBtns = [...host.querySelectorAll<HTMLButtonElement>('button')];
    expect(actionBtns).toHaveLength(3);
    actionBtns.forEach((b) => b.click());
    expect(events).toEqual(['settings', 'all', 'retry']);

    gateProps.set('state', { state: 'refused', settings: true });
    flushSync();
    expect(host.textContent).toContain('Files and Folders is open on “workstation”');
    unmount(fg);
  });

  const stalePermitTexts = [
    '查找本地网络上的设备',
    'would like to find devices',
    '应用程序',
    'Applications folder',
  ] as const;

  it.each([
    {
      lang: 'en' as const,
      coach:
        'Look at the screen of “studio”: macOS is asking “Allow “SoloDawn” to find devices on local networks?”. Click Allow.',
      coachMsg:
        'Look at the screen of “{name}”: macOS is asking “Allow “SoloDawn” to find devices on local networks?”. Click Allow.',
      filesPlus:
        "Can't drop it? Click + under the list, press ⇧⌘G, type ~/Applications, and choose SoloDawn.",
      sysMac1: '“Allow “SoloDawn” to find devices on local networks?”: click Allow.',
    },
    {
      lang: 'zh-CN' as const,
      coach: '看「studio」的屏幕：macOS 在问「允许“SoloDawn”查找本地网络中的设备？」，点「允许」。',
      coachMsg:
        '看「{name}」的屏幕：macOS 在问「允许“SoloDawn”查找本地网络中的设备？」，点「允许」。',
      filesPlus: '拖不进去？点列表下方的「+」，按 ⇧⌘G，输入 ~/Applications，再选 SoloDawn。',
      sysMac1: '「允许“SoloDawn”查找本地网络中的设备？」：点「允许」。',
    },
  ])(
    'Permit and SystemAsks match updated macOS Local Network and ~/Applications wording in $lang',
    ({ lang, coach, coachMsg, filesPlus, sysMac1 }) => {
      prefs.lang = lang;

      expect(messages[lang]['permit.lan.coach']).toBe(coachMsg);
      expect(messages[lang]['permit.files.plus']).toBe(filesPlus);
      expect(messages[lang]['permit.sys.mac.1']).toBe(sysMac1);

      for (const key of ['permit.lan.coach', 'permit.files.plus', 'permit.sys.mac.1'] as const) {
        for (const bad of stalePermitTexts) {
          expect(messages[lang][key]).not.toContain(bad);
        }
      }
      for (const [key, text] of Object.entries(messages[lang])) {
        for (const bad of stalePermitTexts) {
          // permit.sys.mac.2 keeps macOS's literal Chinese firewall prompt ("你要应用程序“solodawn”...").
          if (key === 'permit.sys.mac.2' && bad === '应用程序') continue;
          expect(text).not.toContain(bad);
        }
      }

      const app = mount(Permit, {
        target: host,
        props: {
          name: 'studio',
          os: 'darwin/arm64',
          relayed: false,
          permissions: perms({ local_network: false, full_disk: false }),
          ask: async () => ({ re: 1, ok: true as const }),
          open: true,
        },
      });
      flushSync();

      const allowAll = [...host.querySelectorAll<HTMLButtonElement>('.perm-rows .act')].find((b) =>
        b.textContent?.includes(messages[lang]['permit.files.all']),
      )!;
      allowAll.click();
      flushSync();

      expect(host.querySelectorAll('.coach p[role="status"]')[0]?.textContent).toBe(coach);
      expect(host.querySelector('.coach .more')?.textContent).toBe(filesPlus);
      for (const bad of stalePermitTexts) {
        expect(host.textContent).not.toContain(bad);
      }
      unmount(app);

      vi.stubGlobal('navigator', {
        userAgentData: { platform: 'macOS' },
        platform: '',
        userAgent: '',
      });
      const sys = mount(SystemAsks, { target: host });
      flushSync();

      const items = host.querySelectorAll('.sys-asks li');
      expect(items[0]?.textContent).toBe(sysMac1);
      for (const bad of stalePermitTexts) {
        expect(items[0]?.textContent).not.toContain(bad);
      }
      for (const bad of stalePermitTexts) {
        if (lang === 'zh-CN' && bad === '应用程序') continue;
        expect(host.textContent).not.toContain(bad);
      }
      unmount(sys);
    },
  );
});
