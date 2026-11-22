// Service worker for closed-page Web Push reminders (planning/contracts/commands.md section 1.7;
// planning/steps/step-1-acceptance.md A8.3). Decodes proto.Reminder payloads without message text
// or commands, shows "group · kind" and "who: title" in the reader's language, and opens or focuses
// the group's chat at the card when clicked.
import { decode, Reminder } from './lib/proto';

export type ReminderLang = 'en' | 'zh-CN';

export interface ReminderTarget {
  g: string;
  msg: string;
  kind: string;
}

export interface SwClient {
  readonly url: string;
  focus(): Promise<unknown>;
  postMessage(message: { t: 'reminder' } & ReminderTarget): void;
}

export interface SwNotification {
  readonly data: ReminderTarget;
  close(): void;
}

export interface SwPushEvent {
  readonly data?: { text(): string } | null;
  waitUntil(work: Promise<unknown>): void;
}

export interface SwClickEvent {
  readonly notification: SwNotification;
  waitUntil(work: Promise<unknown>): void;
}

export interface SwLifecycleEvent {
  waitUntil(work: Promise<unknown>): void;
}

export interface SwScope {
  readonly location: { readonly search: string };
  readonly navigator: { readonly language: string };
  readonly registration: {
    readonly scope: string;
    showNotification(
      title: string,
      options: { body: string; icon: string; tag: string; data: ReminderTarget },
    ): Promise<void>;
  };
  readonly clients: {
    claim(): Promise<void>;
    matchAll(options: { type: 'window'; includeUncontrolled: boolean }): Promise<SwClient[]>;
    openWindow(url: string): Promise<unknown>;
  };
  skipWaiting(): Promise<void>;
  addEventListener(type: 'install' | 'activate', listener: (ev: SwLifecycleEvent) => void): void;
  addEventListener(type: 'push', listener: (ev: SwPushEvent) => void): void;
  addEventListener(type: 'notificationclick', listener: (ev: SwClickEvent) => void): void;
}

const kindLabels: Record<ReminderLang, Record<string, string>> = {
  en: {
    approval: 'Approval',
    decision: 'Decision',
    brief: 'Task brief',
    mention: 'Mention',
    done: 'Task done',
  },
  'zh-CN': {
    approval: '审批申请',
    decision: '请拍板',
    brief: '任务说明',
    mention: '提及',
    done: '任务完成',
  },
};

/** Resolves the notification language from the worker's `?lang=` parameter or browser locale. */
export function resolveReminderLang(search: string, browserLang: string): ReminderLang {
  const chosen = new URLSearchParams(search).get('lang');
  if (chosen === 'en' || chosen === 'zh-CN') return chosen;
  return browserLang.startsWith('zh') ? 'zh-CN' : 'en';
}

/** Formats a decoded `Reminder` summary into the notification title and body. */
export function formatReminder(reminder: Reminder, lang: ReminderLang) {
  const kind = kindLabels[lang][reminder.kind] ?? reminder.kind;
  const body = reminder.title ? `${reminder.who}: ${reminder.title}` : reminder.who;
  return {
    title: `${reminder.group} · ${kind}`,
    body,
  };
}

async function onPush(scope: SwScope, raw: string | undefined): Promise<void> {
  if (!raw) return;
  let reminder: Reminder;
  try {
    reminder = decode(Reminder, raw);
  } catch {
    return;
  }
  const lang = resolveReminderLang(scope.location.search, scope.navigator.language);
  const { title, body } = formatReminder(reminder, lang);
  await scope.registration.showNotification(title, {
    body,
    icon: new URL('icon.svg', scope.registration.scope).href,
    tag: `${reminder.g}/${reminder.msg}`,
    data: { g: reminder.g, msg: reminder.msg, kind: reminder.kind },
  });
}

async function onClick(scope: SwScope, notification: SwNotification): Promise<void> {
  notification.close();
  const target = notification.data;
  const base = scope.registration.scope;
  const windows = await scope.clients.matchAll({ type: 'window', includeUncontrolled: true });
  const existing = windows.find((win) => win.url.startsWith(base));
  if (existing) {
    await existing.focus();
    existing.postMessage({ t: 'reminder', ...target });
    return;
  }
  const query = `?msg=${encodeURIComponent(target.msg)}&kind=${encodeURIComponent(target.kind)}`;
  const hash = `#/groups/${encodeURIComponent(target.g)}`;
  await scope.clients.openWindow(new URL(`${query}${hash}`, base).href);
}

/** Attaches lifecycle, `push`, and `notificationclick` listeners to a service worker scope. */
export function bindServiceWorker(scope: SwScope): void {
  scope.addEventListener('install', (ev) => ev.waitUntil(scope.skipWaiting()));
  scope.addEventListener('activate', (ev) => ev.waitUntil(scope.clients.claim()));
  scope.addEventListener('push', (ev) => ev.waitUntil(onPush(scope, ev.data?.text())));
  scope.addEventListener('notificationclick', (ev) =>
    ev.waitUntil(onClick(scope, ev.notification)),
  );
}

if ('ServiceWorkerGlobalScope' in globalThis) {
  bindServiceWorker(globalThis as unknown as SwScope);
}
