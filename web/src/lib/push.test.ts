import 'fake-indexeddb/auto';
import { createECDH } from 'node:crypto';
import { IDBFactory } from 'fake-indexeddb';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { Home } from './groups.svelte';
import { decode, encode } from './identity';
import { goldens } from './mock';
import { preference, prefs } from './prefs.svelte';
import * as p from './proto';
import {
  defaultPushEnv,
  dropPushSubscription,
  ensurePushSubscription,
  inspectPushState,
  loadVapid,
  reminderKinds,
  Reminders,
  type BrowserPushReg,
  type BrowserPushSub,
  type PushEnv,
} from './push';

const sampleGroup = 'rZNvy-1jH6Z-BcPqA5U5BQ';
const secondGroup = 'DrtCn6htSBwmMPrFPbHJHA';
const p256dhBytes = Uint8Array.from({ length: 65 }, (_, i) => (i === 0 ? 4 : i + 1));
const authBytes = Uint8Array.from({ length: 16 }, (_, i) => i + 10);

interface SubOverrides {
  endpoint?: string;
  p256dh?: ArrayBuffer | null;
  auth?: ArrayBuffer | null;
}

function makeSub(serverKey: ArrayBuffer | null, opts: SubOverrides = {}) {
  const keys = {
    p256dh: opts.p256dh === undefined ? p256dhBytes.slice().buffer : opts.p256dh,
    auth: opts.auth === undefined ? authBytes.slice().buffer : opts.auth,
  };
  let unsubscribed = 0;
  const sub: BrowserPushSub & { readonly unsubscribed: number } = {
    endpoint: opts.endpoint ?? 'https://push.example/send/sub-1',
    options: { applicationServerKey: serverKey },
    get unsubscribed() {
      return unsubscribed;
    },
    getKey: (name) => keys[name],
    unsubscribe: vi.fn(async () => {
      unsubscribed += 1;
      return true;
    }),
  };
  return sub;
}

interface EnvOptions {
  userAgent: string;
  maxTouchPoints?: number;
  standalone?: boolean;
  standaloneMedia: boolean;
  permission: NotificationPermission;
  requestResult: NotificationPermission;
  withSw: boolean;
  withPushManager: boolean;
  withNotification: boolean;
  existingSub: BrowserPushSub | null;
  createdSub?: (key: Uint8Array<ArrayBuffer>) => BrowserPushSub;
  idb?: () => IDBFactory;
}

const defaultEnvOptions: EnvOptions = {
  userAgent: 'Mozilla/5.0 (Macintosh; Intel Mac OS X 14_0)',
  standaloneMedia: false,
  permission: 'granted',
  requestResult: 'granted',
  withSw: true,
  withPushManager: true,
  withNotification: true,
  existingSub: null,
};

function makePushReg(
  initial: BrowserPushSub | null,
  createdSub: EnvOptions['createdSub'],
  subscribeCalls: { userVisibleOnly: true; applicationServerKey: Uint8Array<ArrayBuffer> }[],
): BrowserPushReg {
  let activeSub = initial;
  const buildSub = (key: Uint8Array<ArrayBuffer>) =>
    createdSub ? createdSub(key) : makeSub(key.slice().buffer);
  return {
    pushManager: {
      getSubscription: vi.fn(async () => activeSub),
      subscribe: vi.fn(async (call) => {
        subscribeCalls.push(call);
        activeSub = buildSub(call.applicationServerKey);
        return activeSub;
      }),
    },
  };
}

function makeEnv(opts: Partial<EnvOptions> = {}) {
  const cfg = { ...defaultEnvOptions, ...opts };
  const fallbackIdb = new IDBFactory();
  let currentPerm = cfg.permission;
  const registered: string[] = [];
  const subscribeCalls: { userVisibleOnly: true; applicationServerKey: Uint8Array<ArrayBuffer> }[] =
    [];
  const reg = makePushReg(cfg.existingSub, cfg.createdSub, subscribeCalls);

  const requestPermission = vi.fn(async () => {
    currentPerm = cfg.requestResult;
    return currentPerm;
  });

  const sw: PushEnv['serviceWorker'] = {
    register: vi.fn(async (url: string) => {
      registered.push(url);
      return reg;
    }),
    getRegistration: vi.fn(async () => reg),
  };

  const notif: PushEnv['Notification'] = {
    get permission() {
      return currentPerm;
    },
    requestPermission,
  };

  const env: PushEnv = {
    crypto,
    idb: cfg.idb ?? (() => fallbackIdb),
    navigator: {
      userAgent: cfg.userAgent,
      maxTouchPoints: cfg.maxTouchPoints,
      standalone: cfg.standalone,
    },
    matchMedia: vi.fn((query: string) => ({
      matches: query === '(display-mode: standalone)' && cfg.standaloneMedia,
    })),
    serviceWorker: cfg.withSw ? sw : undefined,
    PushManager: cfg.withPushManager ? class {} : undefined,
    Notification: cfg.withNotification ? notif : undefined,
  };

  return { env, reg, registered, subscribeCalls, requestPermission };
}

function makeHomeRig(groups: string[] = [sampleGroup], online = true) {
  const sign = async (payload: string) => ({ p: payload, k: 'browser-dev', s: 'sig' });
  const home = new Home({ user: 'user-1', sign });
  const sent: Record<string, unknown>[] = [];
  let refuseCode: string | undefined;
  let link = home.connect();

  const makeCtl = (): p.Ctl =>
    ({
      request: async (req: { t: string; env?: p.Envelope }) => {
        if (req.t === 'cmd' && req.env) {
          sent.push(JSON.parse(req.env.p) as Record<string, unknown>);
          return refuseCode
            ? { re: 1, err: { code: refuseCode, msg: 'refused' } }
            : { re: 1, ok: true };
        }
        return { t: 'synced', re: 1, ok: true, g: groups[0] ?? sampleGroup, seq: 0 };
      },
    }) as p.Ctl;

  const welcome = (): p.Welcome => ({
    ...p.decode(p.Welcome, goldens.welcome),
    groups: groups.map((g, idx) => ({ g, name: `Group ${idx + 1}`, seq: 0 })),
  });

  let ctl = makeCtl();
  if (online) link.online(ctl, welcome());

  return {
    home,
    sent,
    setRefuse(code: string | undefined) {
      refuseCode = code;
    },
    drop: () => link.offline(),
    reconnect() {
      link = home.connect();
      ctl = makeCtl();
      link.online(ctl, welcome());
    },
    resume() {
      link = home.connect();
      link.online(ctl, welcome());
    },
  };
}

describe('reminderKinds and defaultPushEnv', () => {
  it('lists the six reminder event kinds and reads browser globals lazily', () => {
    expect(reminderKinds).toEqual(['mention', 'approval', 'decision', 'brief', 'confirm', 'done']);
    const env = defaultPushEnv();
    expect(env.crypto).toBe(crypto);
    expect(env.idb()).toBe(indexedDB);
    expect(env.navigator).toBe(navigator);
    expect(typeof env.matchMedia('(display-mode: standalone)').matches).toBe('boolean');
  });
});

describe('inspectPushState', () => {
  it('treats iPhone, iPad, iPod, and touch-capable Macintosh outside standalone mode as ios', () => {
    const state = (userAgent: string, maxTouchPoints?: number) =>
      inspectPushState(makeEnv({ userAgent, maxTouchPoints, standalone: false }).env, true);
    expect(state('Mozilla/5.0 (iPhone; CPU iPhone OS 17_5)')).toBe('ios');
    expect(state('Mozilla/5.0 (iPad; CPU OS 17_5)')).toBe('ios');
    expect(state('Mozilla/5.0 (iPod touch; CPU iPhone OS 17_5)')).toBe('ios');
    expect(state('Mozilla/5.0 (Macintosh)', 2)).toBe('ios');
    expect(state('Mozilla/5.0 (Macintosh)', 1)).toBe('allowed');
    expect(state('Mozilla/5.0 (Macintosh)')).toBe('allowed');
    expect(state('Mozilla/5.0 (Linux; Android 15)', 5)).toBe('allowed');
  });

  it('reads standalone mode from navigator.standalone or the display-mode media query', () => {
    const iphone = 'Mozilla/5.0 (iPhone; CPU iPhone OS 17_5)';
    const mediaOnly = makeEnv({ userAgent: iphone, standalone: false, standaloneMedia: true });
    expect(inspectPushState(mediaOnly.env, true)).toBe('allowed');
    expect(mediaOnly.env.matchMedia).toHaveBeenCalledWith('(display-mode: standalone)');
    expect(inspectPushState(makeEnv({ userAgent: iphone, standalone: true }).env, true)).toBe(
      'allowed',
    );
  });

  it('classifies ios, unsupported, denied, off, and allowed across all boundaries', () => {
    const iosTab = makeEnv({
      userAgent: 'Mozilla/5.0 (iPhone; CPU iPhone OS 17_5)',
      standalone: false,
      withSw: false,
      withPushManager: false,
      withNotification: false,
    });
    expect(inspectPushState(iosTab.env, true)).toBe('ios');

    const iosStandalone = makeEnv({
      userAgent: 'Mozilla/5.0 (iPhone; CPU iPhone OS 17_5)',
      standalone: true,
      permission: 'granted',
    });
    expect(inspectPushState(iosStandalone.env, true)).toBe('allowed');

    expect(inspectPushState(makeEnv({ withSw: false }).env, true)).toBe('unsupported');
    expect(inspectPushState(makeEnv({ withPushManager: false }).env, true)).toBe('unsupported');
    expect(inspectPushState(makeEnv({ withNotification: false }).env, true)).toBe('unsupported');
    expect(inspectPushState(makeEnv({ permission: 'denied' }).env, true)).toBe('denied');
    expect(inspectPushState(makeEnv({ permission: 'denied' }).env, false)).toBe('denied');
    expect(inspectPushState(makeEnv({ permission: 'default' }).env, true)).toBe('off');
    expect(inspectPushState(makeEnv({ permission: 'granted' }).env, false)).toBe('off');
    expect(inspectPushState(makeEnv({ permission: 'granted' }).env, true)).toBe('allowed');
  });
});

describe('loadVapid', () => {
  it('generates a P-256 VAPID pair compatible with Go ecdh and keeps the stored pair', async () => {
    const idb = new IDBFactory();
    const first = await loadVapid({ crypto, idb: () => idb });
    const pub = decode(first.pub)!;
    const priv = decode(first.priv)!;
    expect(pub.length).toBe(65);
    expect(pub[0]).toBe(4);
    expect(priv.length).toBe(32);

    const ecdh = createECDH('prime256v1');
    ecdh.setPrivateKey(Buffer.from(priv));
    expect(new Uint8Array(ecdh.getPublicKey())).toEqual(pub);

    const rawStored = await new Promise<unknown>((resolve, reject) => {
      const openReq = idb.open('solodawn', 1);
      openReq.onerror = () => reject(openReq.error);
      openReq.onsuccess = () => {
        const db = openReq.result;
        const getReq = db.transaction('device', 'readonly').objectStore('device').get('vapid');
        getReq.onsuccess = () => {
          db.close();
          resolve(getReq.result);
        };
      };
    });
    expect(rawStored).toEqual(first);
    expect(await loadVapid({ crypto, idb: () => idb })).toEqual(first);
  });

  it('surfaces IndexedDB open and transaction abort failures', async () => {
    const openErr = new Error('open failed');
    const failingOpen = {
      open() {
        const req = { error: openErr, onerror: null as null | (() => void), onsuccess: null };
        queueMicrotask(() => req.onerror?.());
        return req;
      },
    } as unknown as IDBFactory;
    await expect(loadVapid({ crypto, idb: () => failingOpen })).rejects.toBe(openErr);

    const abortErr = new Error('tx aborted');
    let closed = false;
    const failingTx = {
      open() {
        const tx = {
          error: abortErr,
          oncomplete: null,
          onabort: null as null | (() => void),
          objectStore: () => ({
            get: () => ({ result: undefined, onsuccess: null }),
            add: () => {},
          }),
        };
        const db = {
          close: () => {
            closed = true;
          },
          transaction: (_store: string, _mode: string, opts: { durability?: string }) => {
            expect(opts).toEqual({ durability: 'strict' });
            queueMicrotask(() => tx.onabort?.());
            return tx;
          },
        };
        const req = {
          result: db,
          onerror: null,
          onupgradeneeded: null,
          onsuccess: null as null | (() => void),
        };
        queueMicrotask(() => req.onsuccess?.());
        return req;
      },
    } as unknown as IDBFactory;
    await expect(loadVapid({ crypto, idb: () => failingTx })).rejects.toBe(abortErr);
    expect(closed).toBe(true);
  });
});

describe('ensurePushSubscription and dropPushSubscription', () => {
  it('registers sw.js with lang, subscribes with VAPID key, and reuses matching subscription', async () => {
    const rig = makeEnv();
    const first = await ensurePushSubscription(rig.env, 'zh-CN');
    expect(rig.registered).toEqual(['./sw.js?lang=zh-CN']);
    expect(rig.subscribeCalls).toHaveLength(1);
    expect(rig.subscribeCalls[0].userVisibleOnly).toBe(true);
    expect(encode(rig.subscribeCalls[0].applicationServerKey)).toBe(first.vapid.pub);
    expect(first.sub).toEqual({
      endpoint: 'https://push.example/send/sub-1',
      p256dh: encode(p256dhBytes),
      auth: encode(authBytes),
    });

    const second = await ensurePushSubscription(rig.env, 'en');
    expect(rig.registered).toEqual(['./sw.js?lang=zh-CN', './sw.js?lang=en']);
    expect(rig.subscribeCalls).toHaveLength(1);
    expect(second).toEqual(first);
  });

  it('replaces an existing subscription when applicationServerKey is null, shorter, or differs', async () => {
    const idb = new IDBFactory();
    const vapid = await loadVapid({ crypto, idb: () => idb });
    const validKey = decode(vapid.pub)!;
    const wrongKey = validKey.slice();
    wrongKey[64] ^= 0xff;

    for (const badKey of [null, validKey.slice(0, 10).buffer, wrongKey.buffer]) {
      const stale = makeSub(badKey, { endpoint: 'https://push.example/stale' });
      const rig = makeEnv({ idb: () => idb, existingSub: stale });
      const fresh = await ensurePushSubscription(rig.env, 'en');
      expect(stale.unsubscribed).toBe(1);
      expect(rig.subscribeCalls).toHaveLength(1);
      expect(fresh.sub.endpoint).toBe('https://push.example/send/sub-1');
    }
  });

  it('drops an active subscription and tolerates missing serviceWorker, registration, or sub', async () => {
    await expect(dropPushSubscription(makeEnv({ withSw: false }).env)).resolves.toBeUndefined();

    const emptyReg = makeEnv();
    emptyReg.env.serviceWorker!.getRegistration = vi.fn(async () => undefined);
    await expect(dropPushSubscription(emptyReg.env)).resolves.toBeUndefined();

    await expect(dropPushSubscription(makeEnv({ existingSub: null }).env)).resolves.toBeUndefined();

    const live = makeSub(new ArrayBuffer(65));
    await dropPushSubscription(makeEnv({ existingSub: live }).env);
    expect(live.unsubscribed).toBe(1);
  });
});

describe('Reminders', () => {
  beforeEach(() => {
    localStorage.clear();
    prefs.lang = 'en';
  });

  it('subscribes, syncs connected homes with groups, deduplicates, reconnects, and disables', async () => {
    const store = new Reminders();
    const rig = makeEnv({ permission: 'default', requestResult: 'granted' });
    const homeA = makeHomeRig([sampleGroup, secondGroup]);
    const offlineHome = makeHomeRig([sampleGroup], false);
    const emptyHome = makeHomeRig([], true);

    expect(store.enabled).toBe(false);
    expect(store.activeKinds).toEqual(p.decode(p.PushOn, goldens['push-on']).on);
    expect(store.state(rig.env)).toBe('off');
    expect(await store.sync([homeA.home], rig.env)).toEqual({ ok: true });
    expect(homeA.sent).toEqual([]);

    expect(await store.enable([homeA.home, offlineHome.home, emptyHome.home], rig.env)).toEqual({
      ok: true,
    });
    expect(localStorage.getItem('sd-reminder')).toBe('on');
    expect(rig.requestPermission).toHaveBeenCalledTimes(1);
    expect(store.enabled).toBe(true);
    expect(store.state(rig.env)).toBe('allowed');
    expect(homeA.sent).toHaveLength(1);
    expect(offlineHome.sent).toEqual([]);
    expect(emptyHome.sent).toEqual([]);

    const pushed = p.decode(p.PushOn, JSON.stringify(homeA.sent[0]));
    expect(pushed.g).toBe(sampleGroup);
    expect(pushed.on).toEqual(['mention', 'approval', 'decision', 'brief', 'confirm', 'done']);

    await store.sync([homeA.home], rig.env, 'en');
    expect(homeA.sent).toHaveLength(1);

    homeA.reconnect();
    await store.sync([homeA.home], rig.env, 'en');
    expect(homeA.sent).toHaveLength(2);

    expect(await store.disable([homeA.home, offlineHome.home, emptyHome.home], rig.env)).toEqual({
      ok: true,
    });
    expect(localStorage.getItem('sd-reminder')).toBe('off');
    expect(store.enabled).toBe(false);
    expect(homeA.sent).toHaveLength(3);
    expect(p.decode(p.PushOff, JSON.stringify(homeA.sent[2]))).toMatchObject({
      t: 'push',
      g: sampleGroup,
      off: true,
    });
  });

  it('handles unsupported, dismissed, denied, and failed subscription attempts cleanly', async () => {
    const store = new Reminders();
    const blocked = makeEnv({ permission: 'denied' });
    expect(await store.enable([], blocked.env)).toEqual({ code: 'unsupported' });
    expect(blocked.requestPermission).not.toHaveBeenCalled();

    expect(
      await store.enable([], makeEnv({ permission: 'default', requestResult: 'default' }).env),
    ).toEqual({ code: 'dismissed' });
    expect(store.enabled).toBe(false);

    expect(
      await store.enable([], makeEnv({ permission: 'default', requestResult: 'denied' }).env),
    ).toEqual({ code: 'denied' });
    expect(store.enabled).toBe(false);

    const broken = makeEnv({
      permission: 'granted',
      createdSub: () => {
        throw new DOMException('push service unavailable', 'AbortError');
      },
    });
    expect(await store.enable([], broken.env)).toEqual({ code: 'subscribe_failed' });
    expect(store.enabled).toBe(false);
  });

  it('updates individual event kinds and re-syncs only while allowed', async () => {
    const store = new Reminders();
    const rig = makeEnv({ permission: 'granted' });
    const home = makeHomeRig([sampleGroup]);

    expect(await store.setKind('done', false, [home.home], rig.env)).toEqual({ ok: true });
    expect(localStorage.getItem('sd-reminder-done')).toBe('off');
    expect(store.hasKind('done')).toBe(false);
    expect(home.sent).toEqual([]);

    await store.enable([home.home], rig.env);
    expect(rig.requestPermission).not.toHaveBeenCalled();
    expect((home.sent[0] as { on: string[] }).on).toEqual([
      'mention',
      'approval',
      'decision',
      'brief',
      'confirm',
    ]);

    expect(await store.setKind('confirm', false, [home.home], rig.env)).toEqual({ ok: true });
    expect(localStorage.getItem('sd-reminder-confirm')).toBe('off');
    expect(store.hasKind('confirm')).toBe(false);
    expect(store.activeKinds).toEqual(['mention', 'approval', 'decision', 'brief']);
    expect(home.sent).toHaveLength(2);

    expect(await store.setKind('done', true, [home.home], rig.env)).toEqual({ ok: true });
    expect(home.sent).toHaveLength(3);
    expect((home.sent[2] as { on: string[] }).on).toEqual([
      'mention',
      'approval',
      'decision',
      'brief',
      'done',
    ]);
  });

  it('reports the first home error on enable, sync, and disable, and retries failed homes', async () => {
    const store = new Reminders();
    const rig = makeEnv({ permission: 'granted' });
    const bad1 = makeHomeRig([sampleGroup]);
    const bad2 = makeHomeRig([sampleGroup]);
    const good = makeHomeRig([sampleGroup]);
    bad1.setRefuse('forbidden');
    bad2.setRefuse('stale');

    expect(await store.enable([bad1.home, bad2.home, good.home], rig.env)).toEqual({
      code: 'forbidden',
    });
    bad1.setRefuse(undefined);
    bad2.setRefuse(undefined);
    expect(await store.sync([bad1.home, bad2.home, good.home], rig.env)).toEqual({ ok: true });
    expect(bad1.sent).toHaveLength(2);
    expect(bad2.sent).toHaveLength(2);
    expect(good.sent).toHaveLength(1);

    rig.env.serviceWorker!.register = vi.fn(async () => {
      throw new Error('sw broken');
    });
    expect(await store.sync([good.home], rig.env)).toEqual({ code: 'subscribe_failed' });

    rig.env.serviceWorker!.getRegistration = vi.fn(async () => {
      throw new Error('unsub broken');
    });
    bad1.setRefuse('conflict');
    bad2.setRefuse('stale');
    expect(await store.disable([bad1.home, bad2.home], rig.env)).toEqual({ code: 'conflict' });
  });

  it('skips homes that are connected to no node, and keeps each choice under its own name', async () => {
    const store = new Reminders();
    const rig = makeEnv({ permission: 'granted' });
    const home = makeHomeRig([sampleGroup]);
    await store.enable([home.home], rig.env);
    home.drop();
    expect(await store.sync([home.home], rig.env)).toEqual({ ok: true });
    expect(await store.disable([home.home], rig.env)).toEqual({ ok: true });
    expect(home.sent).toHaveLength(1);

    for (const kind of reminderKinds) {
      await store.setKind(kind, false, [], rig.env);
      expect(localStorage.getItem(`sd-reminder-${kind}`)).toBe('off');
      await store.setKind(kind, true, [], rig.env);
    }
    expect(() => preference('reminder', ['off', 'on'] as const, 'off')).not.toThrow();
  });

  it('does not cache a sent subscription if disabled or disconnected while command is in flight', async () => {
    const store = new Reminders();
    const rig = makeEnv({ permission: 'granted' });
    const home = makeHomeRig([sampleGroup]);
    const origCommand = home.home.command.bind(home.home);
    let mode: 'drop' | 'disable' | 'none' = 'drop';
    home.home.command = async (fields, n) => {
      const answer = await origCommand(fields, n);
      if (mode === 'drop') home.drop();
      if (mode === 'disable' && !('off' in fields)) await store.disable([], rig.env);
      return answer;
    };

    await store.enable([home.home], rig.env);
    mode = 'none';
    home.resume();
    await store.sync([home.home], rig.env);
    expect(home.sent).toHaveLength(2);

    const home2 = makeHomeRig([sampleGroup]);
    const orig2 = home2.home.command.bind(home2.home);
    mode = 'disable';
    home2.home.command = async (fields, n) => {
      const answer = await orig2(fields, n);
      if (mode === 'disable' && !('off' in fields)) await store.disable([], rig.env);
      return answer;
    };
    await store.enable([home2.home], rig.env);
    mode = 'none';
    await store.enable([home2.home], rig.env);
    expect(home2.sent).toHaveLength(2);
  });
});
