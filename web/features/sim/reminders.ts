// Browser reminders in the demo (contracts/commands.md 1.7). A development server has no service
// worker and the demo no push service, so a registration of this page stands in for both: the
// client subscribes as it would, its `push` command reaches the demo computers, and a reminder is
// shown with the browser's own notification and the service worker's wording. A click opens its
// card as a click on a real one does (ReminderSync).
import { encode } from '../../src/lib/identity';
import type * as p from '../../src/lib/proto';
import { formatReminder } from '../../src/sw';

const events = new EventTarget();
let subscribed: { key: ArrayBuffer } | undefined;

const bytes = (n: number) => crypto.getRandomValues(new Uint8Array(n)).buffer;
const p256dh = bytes(65);
const auth = bytes(16);

function subscription(key: ArrayBuffer) {
  return {
    endpoint: `https://push.demo.invalid/${encode(new Uint8Array(bytes(8)))}`,
    options: { applicationServerKey: key },
    getKey: (name: 'p256dh' | 'auth') => (name === 'p256dh' ? p256dh : auth),
    unsubscribe: async () => {
      subscribed = undefined;
      return true;
    },
  };
}

const registration = {
  pushManager: {
    getSubscription: async () => (subscribed ? subscription(subscribed.key) : null),
    subscribe: async ({ applicationServerKey }: { applicationServerKey: Uint8Array }) => {
      subscribed = { key: applicationServerKey.slice().buffer };
      return subscription(subscribed.key);
    },
  },
};

const permissionKey = 'sd-demo-notify';

function permission(): NotificationPermission {
  try {
    return localStorage.getItem(permissionKey) === 'granted' ? 'granted' : 'default';
  } catch {
    return 'default';
  }
}

/** A system notification as the demo shows it: a card in the corner of the page, so that it works
 *  in every browser and asks for no real permission. */
class DemoNotification {
  static get permission() {
    return permission();
  }

  static async requestPermission(): Promise<NotificationPermission> {
    localStorage.setItem(permissionKey, 'granted');
    return 'granted';
  }

  onclick: (() => void) | null = null;

  constructor(title: string, { body }: { body: string }) {
    const card = document.createElement('button');
    card.type = 'button';
    card.dataset.demo = 'notification';
    card.setAttribute(
      'style',
      'position:fixed;top:12px;right:12px;z-index:10000;width:320px;text-align:left;padding:12px 14px;border-radius:14px;border:1px solid var(--line);background:var(--surface);color:var(--text);font:inherit;font-size:13px;box-shadow:0 10px 32px rgb(0 0 0 / .25);cursor:pointer',
    );
    const head = document.createElement('div');
    head.setAttribute('style', 'font-size:11px;color:var(--text-2);margin-bottom:4px');
    head.textContent = 'SoloDawn · 系统通知（演示）';
    const strong = document.createElement('b');
    strong.textContent = title;
    const text = document.createElement('div');
    text.textContent = body;
    card.append(head, strong, text);
    card.onclick = () => {
      card.remove();
      this.onclick?.();
    };
    document.body.append(card);
    setTimeout(() => card.remove(), 8000);
  }
}

/** Puts the stand-in registration where the client looks for a service worker, and the demo's
 *  notifications where it looks for the browser's. */
export function installReminders() {
  const container = {
    register: async () => registration,
    getRegistration: async () => registration,
    addEventListener: events.addEventListener.bind(events),
    removeEventListener: events.removeEventListener.bind(events),
  };
  Object.defineProperty(navigator, 'serviceWorker', { configurable: true, value: container });
  Object.defineProperty(globalThis, 'Notification', {
    configurable: true,
    value: DemoNotification,
  });
  Object.defineProperty(globalThis, 'PushManager', { configurable: true, value: class {} });
}

/** Shows a reminder as the service worker would, once the person turned reminders on. */
export function remind(reminder: p.Reminder, lang: 'en' | 'zh-CN') {
  if (!subscribed || permission() !== 'granted') return false;
  const { title, body } = formatReminder(reminder, lang);
  const shown = new DemoNotification(title, { body });
  shown.onclick = () => {
    const data = { t: 'reminder', g: reminder.g, msg: reminder.msg, kind: reminder.kind };
    events.dispatchEvent(new MessageEvent('message', { data }));
  };
  return true;
}
