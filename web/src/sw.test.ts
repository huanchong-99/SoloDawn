import { describe, expect, it, vi } from 'vitest';
import { goldens } from './lib/mock';
import { decode, Reminder } from './lib/proto';
import {
  bindServiceWorker,
  formatReminder,
  resolveReminderLang,
  type SwClient,
  type SwScope,
} from './sw';

const defaultScopeOpts = {
  search: '',
  language: 'en-US',
  scope: 'https://app.example/sub/',
  windows: [] as SwClient[],
};

function makeScope(opts: Partial<typeof defaultScopeOpts> = {}) {
  const cfg = { ...defaultScopeOpts, ...opts };
  const listeners = new Map<string, (ev: unknown) => void>();
  const shown: { title: string; options: Record<string, unknown> }[] = [];
  const opened: string[] = [];
  const claimed = vi.fn(() => Promise.resolve());
  const skipped = vi.fn(() => Promise.resolve());
  const matchAll = vi.fn(() => Promise.resolve(cfg.windows));

  const scope: SwScope = {
    location: { search: cfg.search },
    navigator: { language: cfg.language },
    registration: {
      scope: cfg.scope,
      showNotification: vi.fn((title, options) => {
        shown.push({ title, options });
        return Promise.resolve();
      }),
    },
    clients: {
      claim: claimed,
      matchAll,
      openWindow: vi.fn((url: string) => {
        opened.push(url);
        return Promise.resolve(null);
      }),
    },
    skipWaiting: skipped,
    addEventListener: ((type: string, listener: (ev: unknown) => void) => {
      listeners.set(type, listener);
    }) as SwScope['addEventListener'],
  };

  bindServiceWorker(scope);

  async function fire(type: string, event: Record<string, unknown> = {}) {
    let pending: Promise<unknown> = Promise.resolve();
    const waitUntil = vi.fn((work: Promise<unknown>) => {
      pending = work;
    });
    listeners.get(type)!({ ...event, waitUntil });
    await pending;
    return waitUntil;
  }

  return { shown, opened, claimed, skipped, matchAll, fire };
}

describe('resolveReminderLang', () => {
  it('prefers a valid lang query parameter and falls back to browser locale prefix', () => {
    expect(resolveReminderLang('?lang=en', 'zh-CN')).toBe('en');
    expect(resolveReminderLang('?lang=zh-CN', 'en-US')).toBe('zh-CN');
    expect(resolveReminderLang('', 'zh')).toBe('zh-CN');
    expect(resolveReminderLang('?lang=fr', 'zh-TW')).toBe('zh-CN');
    expect(resolveReminderLang('', 'en-zh')).toBe('en');
    expect(resolveReminderLang('?lang=other', 'fr-FR')).toBe('en');
  });
});

describe('formatReminder', () => {
  const sample = decode(Reminder, goldens['push-reminder']);

  it('formats each reminder kind and a stopped task in English and Chinese and passes unknown kinds through', () => {
    const cases = [
      ['approval', 'Approval', '审批申请'],
      ['decision', 'Decision', '请拍板'],
      ['brief', 'Task brief', '任务说明'],
      ['confirm', 'Request for your AI', '有人想用你的 AI'],
      ['mention', 'Task stopped', '任务已停下'],
      ['done', 'Task done', '任务完成'],
      ['custom', 'custom', 'custom'],
    ] as const;
    for (const [kind, enLabel, zhLabel] of cases) {
      const item = { ...sample, kind };
      expect(formatReminder(item, 'en')).toEqual({
        title: `Website · ${enLabel}`,
        body: 'Codex: Time the path change in a browser.',
      });
      expect(formatReminder(item, 'zh-CN')).toEqual({
        title: `Website · ${zhLabel}`,
        body: 'Codex: Time the path change in a browser.',
      });
    }
  });

  it('shows only who in the body when title is absent or empty', () => {
    const { title: _dropped, ...withoutTitle } = sample;
    expect(formatReminder(withoutTitle, 'en')).toEqual({
      title: 'Website · Approval',
      body: 'Codex',
    });
    for (const title of [undefined, ''])
      expect(formatReminder({ ...sample, kind: 'mention', title }, 'en')).toEqual({
        title: 'Website · Mention',
        body: 'Codex',
      });
    expect(formatReminder({ ...withoutTitle, kind: 'mention' }, 'zh-CN').title).toBe(
      'Website · 提及',
    );
    expect(formatReminder({ ...sample, title: '' }, 'zh-CN')).toEqual({
      title: 'Website · 审批申请',
      body: 'Codex',
    });
  });
});

describe('the wire example of a confirm reminder', () => {
  it('names the group, the kind and the AI', () => {
    const reminder = decode(Reminder, goldens['push-reminder-confirm']);
    expect(formatReminder(reminder, 'en')).toEqual({
      title: 'Website · Request for your AI',
      body: 'Codex',
    });
    expect(formatReminder(reminder, 'zh-CN')).toEqual({
      title: 'Website · 有人想用你的 AI',
      body: 'Codex',
    });
  });
});

describe('service worker events', () => {
  it('skips waiting on install and claims clients on activate', async () => {
    const rig = makeScope();
    expect(await rig.fire('install')).toHaveBeenCalledTimes(1);
    expect(rig.skipped).toHaveBeenCalledTimes(1);
    expect(await rig.fire('activate')).toHaveBeenCalledTimes(1);
    expect(rig.claimed).toHaveBeenCalledTimes(1);
  });

  it('shows a localized notification for a valid push payload and ignores invalid ones', async () => {
    const rig = makeScope({ search: '?lang=zh-CN', scope: 'https://app.example/client/' });
    await rig.fire('push', {});
    await rig.fire('push', { data: null });
    await rig.fire('push', { data: { text: () => '' } });
    await rig.fire('push', { data: { text: () => '{not json' } });
    await rig.fire('push', { data: { text: () => '{"g":"bad"}' } });
    expect(rig.shown).toEqual([]);

    const waitUntil = await rig.fire('push', { data: { text: () => goldens['push-reminder'] } });
    expect(waitUntil).toHaveBeenCalledTimes(1);
    expect(rig.shown).toEqual([
      {
        title: 'Website · 审批申请',
        options: {
          body: 'Codex: Time the path change in a browser.',
          icon: 'https://app.example/client/icon.svg',
          tag: 'rZNvy-1jH6Z-BcPqA5U5BQ/netluHiPQf-UoheEIBfbLw',
          data: {
            g: 'rZNvy-1jH6Z-BcPqA5U5BQ',
            msg: 'netluHiPQf-UoheEIBfbLw',
            kind: 'approval',
          },
        },
      },
    ]);
  });

  it('focuses an open window in scope and posts the reminder target', async () => {
    const outside: SwClient = {
      url: 'https://other.example/?ref=https://app.example/sub/',
      focus: vi.fn(() => Promise.resolve()),
      postMessage: vi.fn(),
    };
    const inside: SwClient = {
      url: 'https://app.example/sub/#/settings',
      focus: vi.fn(() => Promise.resolve()),
      postMessage: vi.fn(),
    };
    const rig = makeScope({ windows: [outside, inside] });
    const close = vi.fn();

    await rig.fire('notificationclick', {
      notification: {
        data: { g: 'rZNvy-1jH6Z-BcPqA5U5BQ', msg: 'netluHiPQf-UoheEIBfbLw', kind: 'approval' },
        close,
      },
    });

    expect(close).toHaveBeenCalledTimes(1);
    expect(rig.matchAll).toHaveBeenCalledWith({ type: 'window', includeUncontrolled: true });
    expect(outside.focus).not.toHaveBeenCalled();
    expect(inside.focus).toHaveBeenCalledTimes(1);
    expect(inside.postMessage).toHaveBeenCalledWith({
      t: 'reminder',
      g: 'rZNvy-1jH6Z-BcPqA5U5BQ',
      msg: 'netluHiPQf-UoheEIBfbLw',
      kind: 'approval',
    });
    expect(rig.opened).toEqual([]);
  });

  it('opens a new window at the group and message query when no window in scope is open', async () => {
    const rig = makeScope({ scope: 'https://app.example/sub/' });
    const close = vi.fn();
    await rig.fire('notificationclick', {
      notification: { data: { g: 'g/1', msg: 'm/2', kind: 'k/3' }, close },
    });
    expect(close).toHaveBeenCalledTimes(1);
    expect(rig.opened).toEqual(['https://app.example/sub/?msg=m%2F2&kind=k%2F3#/groups/g%2F1']);
  });

  it('binds globalThis automatically only when ServiceWorkerGlobalScope exists', async () => {
    const spy = vi.spyOn(globalThis, 'addEventListener');
    vi.resetModules();
    await import('./sw');
    expect(spy).not.toHaveBeenCalledWith('push', expect.any(Function));

    vi.stubGlobal('ServiceWorkerGlobalScope', class {});
    try {
      vi.resetModules();
      await import('./sw');
      expect(spy).toHaveBeenCalledWith('install', expect.any(Function));
      expect(spy).toHaveBeenCalledWith('activate', expect.any(Function));
      expect(spy).toHaveBeenCalledWith('push', expect.any(Function));
      expect(spy).toHaveBeenCalledWith('notificationclick', expect.any(Function));
    } finally {
      vi.unstubAllGlobals();
      spy.mockRestore();
    }
  });
});
