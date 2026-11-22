// Browser Web Push capability checks, VAPID key pair persistence in IndexedDB, PushManager
// subscription lifecycle, and sending `push` commands to connected homes
// (planning/contracts/commands.md section 1.7; planning/contracts/storage.md section 5).
import type { Home } from './groups.svelte';
import { decode, encode, keep, type KeyStore } from './identity';
import { preference, prefs, type Lang, type Preference } from './prefs.svelte';
import type * as p from './proto';

/** The five reminder event kinds of `commands.md` section 1.7, in wire-example order. */
export const reminderKinds = ['mention', 'approval', 'decision', 'brief', 'done'] as const;
export type ReminderKind = (typeof reminderKinds)[number];

/** How this browser currently stands with Web Push notifications. */
export type PushState = 'allowed' | 'off' | 'denied' | 'ios' | 'unsupported';

export interface VapidKeys {
  pub: string;
  priv: string;
}

export interface BrowserPushSub {
  readonly endpoint: string;
  readonly options: { readonly applicationServerKey: ArrayBuffer | null };
  getKey(name: 'p256dh' | 'auth'): ArrayBuffer | null;
  unsubscribe(): Promise<boolean>;
}

export interface BrowserPushReg {
  readonly pushManager: {
    getSubscription(): Promise<BrowserPushSub | null>;
    subscribe(options: {
      userVisibleOnly: true;
      applicationServerKey: Uint8Array<ArrayBuffer>;
    }): Promise<BrowserPushSub>;
  };
}

export interface PushEnv extends KeyStore {
  navigator: {
    readonly userAgent: string;
    readonly maxTouchPoints?: number;
    readonly standalone?: boolean;
  };
  matchMedia(query: string): { readonly matches: boolean };
  serviceWorker?: {
    register(scriptURL: string): Promise<BrowserPushReg>;
    getRegistration(clientURL?: string): Promise<BrowserPushReg | undefined>;
  };
  PushManager?: unknown;
  Notification?: {
    readonly permission: NotificationPermission;
    requestPermission(): Promise<NotificationPermission>;
  };
}

const ecdsaP256 = { name: 'ECDSA', namedCurve: 'P-256' } as const;

/** Default browser environment used in production and integration tests. */
export function defaultPushEnv(): PushEnv {
  return {
    crypto,
    idb: () => indexedDB,
    navigator: navigator as PushEnv['navigator'],
    matchMedia: (query) => matchMedia(query),
    serviceWorker: navigator.serviceWorker as PushEnv['serviceWorker'],
    PushManager: (globalThis as { PushManager?: unknown }).PushManager,
    Notification: (globalThis as { Notification?: PushEnv['Notification'] }).Notification,
  };
}

/** True on iPhone, iPod, iPad, and iPadOS reporting a desktop Macintosh user agent with touch. */
export function isIosDevice(nav: { userAgent: string; maxTouchPoints?: number }): boolean {
  if (/iPad|iPhone|iPod/.test(nav.userAgent)) return true;
  return /Macintosh/.test(nav.userAgent) && (nav.maxTouchPoints ?? 0) > 1;
}

/** True when opened as an installed Home Screen / standalone web application. */
export function isStandaloneMode(env: Pick<PushEnv, 'navigator' | 'matchMedia'>): boolean {
  return env.navigator.standalone === true || env.matchMedia('(display-mode: standalone)').matches;
}

function hasPushApi(env: PushEnv): boolean {
  return Boolean(env.serviceWorker && env.PushManager && env.Notification);
}

function permissionState(permission: NotificationPermission, enabled: boolean): PushState {
  if (permission === 'denied') return 'denied';
  return enabled && permission === 'granted' ? 'allowed' : 'off';
}

/** Classifies whether Web Push can be used in `env`, or why it cannot yet. */
export function inspectPushState(env: PushEnv, enabled: boolean): PushState {
  if (isIosDevice(env.navigator) && !isStandaloneMode(env)) return 'ios';
  if (!hasPushApi(env)) return 'unsupported';
  return permissionState(env.Notification!.permission, enabled);
}

/** Loads this browser's P-256 VAPID key pair from IndexedDB, creating it once if absent. */
export async function loadVapid({ crypto, idb }: KeyStore): Promise<VapidKeys> {
  const pair = await crypto.subtle.generateKey(ecdsaP256, true, ['sign', 'verify']);
  const pub = encode(new Uint8Array(await crypto.subtle.exportKey('raw', pair.publicKey)));
  const jwk = await crypto.subtle.exportKey('jwk', pair.privateKey);
  return keep(idb, 'vapid', { pub, priv: jwk.d! });
}

function sameBytes(actual: ArrayBuffer | null, expected: Uint8Array): boolean {
  if (!actual || actual.byteLength !== expected.length) return false;
  const view = new Uint8Array(actual);
  return view.every((byte, i) => byte === expected[i]);
}

function readSubscription(sub: BrowserPushSub): p.PushOn['sub'] {
  return {
    endpoint: sub.endpoint,
    p256dh: encode(new Uint8Array(sub.getKey('p256dh')!)),
    auth: encode(new Uint8Array(sub.getKey('auth')!)),
  };
}

/** Registers `./sw.js` with `lang` and returns the browser's `PushSubscription` and VAPID keys. */
export async function ensurePushSubscription(
  env: PushEnv,
  lang: Lang,
): Promise<{ sub: p.PushOn['sub']; vapid: VapidKeys }> {
  const reg = await env.serviceWorker!.register(`./sw.js?lang=${encodeURIComponent(lang)}`);
  const vapid = await loadVapid(env);
  const serverKey = decode(vapid.pub)!;
  let sub = await reg.pushManager.getSubscription();
  if (sub && !sameBytes(sub.options.applicationServerKey, serverKey)) {
    await sub.unsubscribe();
    sub = null;
  }
  if (!sub) {
    sub = await reg.pushManager.subscribe({
      userVisibleOnly: true,
      applicationServerKey: serverKey,
    });
  }
  return { sub: readSubscription(sub), vapid };
}

/** Unsubscribes the browser's active `PushSubscription`, if one exists. */
export async function dropPushSubscription(env: PushEnv): Promise<void> {
  if (!env.serviceWorker) return;
  const reg = await env.serviceWorker.getRegistration();
  const sub = await reg?.pushManager.getSubscription();
  await sub?.unsubscribe();
}

function connectedGroupHomes(homes: readonly Home[]) {
  return homes.filter((home) => home.ctl !== undefined && home.groups.length > 0);
}

/** Manages browser reminder preferences and synchronises `push` commands across connected homes. */
export class Reminders {
  readonly #enabled: Preference<'off' | 'on'>;
  readonly #kinds: Record<ReminderKind, Preference<'on' | 'off'>>;
  #sent = new WeakMap<Home, { ctl: p.Ctl; spec: string }>();

  constructor() {
    this.#enabled = preference('reminder', ['off', 'on'] as const, 'off');
    this.#kinds = {
      mention: preference('reminder-mention', ['on', 'off'] as const, 'on'),
      approval: preference('reminder-approval', ['on', 'off'] as const, 'on'),
      decision: preference('reminder-decision', ['on', 'off'] as const, 'on'),
      brief: preference('reminder-brief', ['on', 'off'] as const, 'on'),
      done: preference('reminder-done', ['on', 'off'] as const, 'on'),
    };
  }

  /** Whether the person chose to turn browser reminders on. */
  get enabled(): boolean {
    return this.#enabled.value === 'on';
  }

  /** Whether `kind` is selected under "Notify me when". */
  hasKind(kind: ReminderKind): boolean {
    return this.#kinds[kind].value === 'on';
  }

  /** The enabled reminder event kinds in wire order. */
  get activeKinds(): ReminderKind[] {
    return reminderKinds.filter((kind) => this.hasKind(kind));
  }

  /** Current capability and permission state for `env`. */
  state(env: PushEnv = defaultPushEnv()): PushState {
    return inspectPushState(env, this.enabled);
  }

  /** Requests permission if needed, subscribes, enables reminders, and notifies connected homes. */
  async enable(
    homes: readonly Home[],
    env: PushEnv = defaultPushEnv(),
  ): Promise<{ ok: true } | { code: string }> {
    if (inspectPushState(env, false) !== 'off') return { code: 'unsupported' };
    const perm =
      env.Notification!.permission === 'granted'
        ? 'granted'
        : await env.Notification!.requestPermission();
    if (perm !== 'granted') return { code: perm === 'denied' ? 'denied' : 'dismissed' };
    try {
      const { sub, vapid } = await ensurePushSubscription(env, prefs.lang);
      this.#enabled.value = 'on';
      return await this.#pushOnAll(homes, sub, vapid, this.activeKinds);
    } catch {
      return { code: 'subscribe_failed' };
    }
  }

  /** Disables reminders, unsubscribes in this browser, and sends `{ off: true }` to homes. */
  async disable(
    homes: readonly Home[],
    env: PushEnv = defaultPushEnv(),
  ): Promise<{ ok: true } | { code: string }> {
    this.#enabled.value = 'off';
    this.#sent = new WeakMap();
    await dropPushSubscription(env).catch(() => {});
    let failed: string | undefined;
    for (const home of connectedGroupHomes(homes)) {
      const answer = await home.command({ t: 'push', g: home.groups[0].g, off: true });
      if (answer.err) failed ??= answer.err.code;
    }
    return failed ? { code: failed } : { ok: true };
  }

  /** Updates one reminder event kind and re-sends `push` to connected homes while enabled. */
  async setKind(
    kind: ReminderKind,
    checked: boolean,
    homes: readonly Home[],
    env: PushEnv = defaultPushEnv(),
  ): Promise<{ ok: true } | { code: string }> {
    this.#kinds[kind].value = checked ? 'on' : 'off';
    return this.sync(homes, env);
  }

  /** Ensures every connected home with a group holds the current subscription, VAPID, and `on`. */
  async sync(
    homes: readonly Home[],
    env: PushEnv = defaultPushEnv(),
    lang: Lang = prefs.lang,
  ): Promise<{ ok: true } | { code: string }> {
    if (inspectPushState(env, this.enabled) !== 'allowed') return { ok: true };
    try {
      const { sub, vapid } = await ensurePushSubscription(env, lang);
      return await this.#pushOnAll(homes, sub, vapid, this.activeKinds);
    } catch {
      return { code: 'subscribe_failed' };
    }
  }

  async #pushOnAll(
    homes: readonly Home[],
    sub: p.PushOn['sub'],
    vapid: VapidKeys,
    on: ReminderKind[],
  ): Promise<{ ok: true } | { code: string }> {
    const spec = JSON.stringify({ sub, vapid, on });
    let failed: string | undefined;
    for (const home of connectedGroupHomes(homes)) {
      const ctl = home.ctl!;
      const last = this.#sent.get(home);
      if (last?.ctl === ctl && last.spec === spec) continue;
      const answer = await home.command({ t: 'push', g: home.groups[0].g, sub, vapid, on });
      if (answer.err) failed ??= answer.err.code;
      else if (this.enabled && home.ctl === ctl) this.#sent.set(home, { ctl, spec });
    }
    return failed ? { code: failed } : { ok: true };
  }
}

export const reminders = new Reminders();
