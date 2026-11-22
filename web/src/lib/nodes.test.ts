import { afterEach, describe, expect, it, vi } from 'vitest';
import { watch } from '../../features/watch.svelte';
import { Home, lost } from './groups.svelte';
import { goldens } from './mock';
import type { Net, Path } from './net.svelte';
import { indicator, Nodes, nodes } from './nodes.svelte';
import * as p from './proto';

const user = 'akWV_5PtNLIGhakuMAvpD9LcG1D8TUfc1URh48VREcs';
const deviceId = 'J3Uuh-y1z2AX536yAFlGHJL5sjxVpWVJavvVphvdm5Q';
const website = 'rZNvy-1jH6Z-BcPqA5U5BQ';

const signed: string[] = [];
const me = {
  user,
  sign: async (payload: string) => {
    signed.push(payload);
    return { p: payload, k: 'this-browser', s: 'signature' };
  },
};

interface Pending {
  req: Record<string, unknown>;
  answer(frame: object): void;
  fail(): void;
}

function connection() {
  const asked: Pending[] = [];
  const ctl = {
    request: (req: Record<string, unknown>) =>
      new Promise((answer, fail) => asked.push({ req, answer, fail: () => fail(new Error()) })),
  } as p.Ctl;
  return { ctl, asked };
}

const settle = () => new Promise((done) => setTimeout(done));

function welcome(changes: object = {}): p.Welcome {
  return {
    ...p.decode(p.Welcome, goldens.welcome),
    groups: [],
    ...changes,
  };
}

function connectHome(home: Home, changes: object = {}) {
  const { ctl, asked } = connection();
  const link = home.connect();
  link.online(ctl, welcome(changes));
  return { ctl, asked, link };
}

function hostAnswer(changes: object = {}): p.HostInfo {
  return {
    ...p.decode(p.HostInfo, goldens['host-info-answer']),
    ...changes,
  };
}

function netWith(homes: Home[], paths = new Map<Home, Path | undefined>()): Net {
  return {
    source: 'live',
    homes,
    path: (home) => paths.get(home),
  };
}

afterEach(() => {
  signed.length = 0;
  vi.restoreAllMocks();
});

describe('indicator', () => {
  it('is undefined when no home is known', () => {
    expect(indicator(netWith([]))).toBeUndefined();
  });

  it('reports down when every known home is unreachable', () => {
    const first = new Home(me);
    const second = new Home(me);
    expect(indicator(netWith([first, second]))).toStrictEqual({ st: 'down' });
  });

  it('reports linking when any connected home still has no selected candidate pair', () => {
    const ready = new Home(me);
    const selecting = new Home(me);
    connectHome(ready);
    connectHome(selecting);
    const paths = new Map<Home, Path | undefined>([
      [ready, { kind: 'direct', rtt: 14, since: 100 }],
      [selecting, undefined],
    ]);
    expect(indicator(netWith([ready, selecting], paths))).toStrictEqual({ st: 'linking' });
    expect(indicator(netWith([selecting], paths))).toStrictEqual({ st: 'linking' });
  });

  it('reports limited when all connected homes have paths and at least one is relayed', () => {
    const direct = new Home(me);
    const relayed = new Home(me);
    connectHome(direct);
    connectHome(relayed);
    const paths = new Map<Home, Path | undefined>([
      [direct, { kind: 'direct', rtt: 12, since: 100 }],
      [relayed, { kind: 'relay', rtt: 210, since: 200 }],
    ]);
    expect(indicator(netWith([direct, relayed], paths))).toStrictEqual({ st: 'limited' });
    expect(indicator(netWith([relayed], paths))).toStrictEqual({ st: 'limited' });
  });

  it('reports ok with the first measured rtt, or undefined rtt, ignoring offline homes', () => {
    const unmeasured = new Home(me);
    const measured = new Home(me);
    const offline = new Home(me);
    connectHome(unmeasured);
    connectHome(measured);
    const paths = new Map<Home, Path | undefined>([
      [unmeasured, { kind: 'direct', rtt: undefined, since: 100 }],
      [measured, { kind: 'direct', rtt: 18, since: 100 }],
    ]);
    expect(indicator(netWith([unmeasured, measured, offline], paths))).toStrictEqual({
      st: 'ok',
      rtt: 18,
    });
    expect(indicator(netWith([unmeasured], paths))).toStrictEqual({
      st: 'ok',
      rtt: undefined,
    });
  });

  it('reflects only the open group home when a group is given', () => {
    const up = new Home(me);
    const down = new Home(me);
    connectHome(up, { groups: [{ g: 'g-up', name: 'Up', seq: 0 }] });
    const downConn = connectHome(down, { groups: [{ g: website, name: 'Website', seq: 0 }] });
    downConn.asked[0].answer({ t: 'synced', re: 1, ok: true, g: website, seq: 0 });
    downConn.link.offline();

    const paths = new Map<Home, Path | undefined>([[up, { kind: 'direct', rtt: 9, since: 50 }]]);
    const net = netWith([up, down], paths);
    expect(indicator(net)).toStrictEqual({ st: 'ok', rtt: 9 });
    expect(indicator(net, down.groups[0])).toStrictEqual({ st: 'down' });
    expect(indicator(net, up.groups[0])).toStrictEqual({ st: 'ok', rtt: 9 });
  });
});

describe('Nodes', () => {
  it('exports a shared Nodes instance', () => {
    expect(nodes).toBeInstanceOf(Nodes);
  });

  it('queries host.info once per online owner connection and stores the state without re or ok', async () => {
    const store = new Nodes();
    const offline = new Home(me);
    const foreign = new Home(me);
    const owned = new Home(me);
    const foreignConn = connectHome(foreign, { owner: false });
    const ownedConn = connectHome(owned, { owner: true });

    const seen: number[] = [];
    const stop = watch(() => {
      seen.push(store.infos.size);
    });

    store.sync([offline, foreign, owned]);
    // Calling sync again with the same active ctl sends no duplicate request.
    store.sync([offline, foreign, owned]);
    expect(foreignConn.asked).toEqual([]);
    expect(ownedConn.asked.map((ask) => ask.req)).toStrictEqual([{ t: 'host.info' }]);

    const answer = hostAnswer();
    ownedConn.asked[0].answer(answer);
    await settle();

    const { re: _re, ok: _ok, ...expected } = answer;
    expect(store.infos.get(owned)).toStrictEqual(expected);
    expect(seen.at(-1)).toBe(1);

    // Passing again = true re-reads host.info on the same ctl while still skipping non-owners.
    store.sync([offline, foreign, owned], true);
    expect(foreignConn.asked).toEqual([]);
    expect(ownedConn.asked).toHaveLength(2);
    const refreshed = hostAnswer({ open: 4 });
    ownedConn.asked[1].answer(refreshed);
    await settle();
    const { re: _re2, ok: _ok2, ...expectedRefreshed } = refreshed;
    expect(store.infos.get(owned)).toStrictEqual(expectedRefreshed);

    // Default again = false still skips an unchanged ctl afterwards.
    store.sync([offline, foreign, owned]);
    expect(ownedConn.asked).toHaveLength(2);
    stop();
  });

  it('queries host.info again after an owner home goes offline and reconnects, even with the same ctl', async () => {
    const store = new Nodes();
    const home = new Home(me);
    const { ctl, asked, link } = connectHome(home, { owner: true });

    store.sync([home]);
    expect(asked).toHaveLength(1);

    link.offline();
    store.sync([home]);
    expect(asked).toHaveLength(1);

    // Reconnect using the exact same ctl instance.
    home.ctl = ctl;
    store.sync([home]);
    expect(asked).toHaveLength(2);
  });

  it('ignores a host.info answer that arrives after the connection changed, failed, or errored', async () => {
    const store = new Nodes();
    const home = new Home(me);
    const first = connectHome(home, { owner: true });
    store.sync([home]);

    const second = connectHome(home, { owner: true });
    store.sync([home]);
    // Late answer from the replaced connection is dropped.
    first.asked[0].answer(hostAnswer());
    await settle();
    expect(store.infos.size).toBe(0);

    // Error answer on the active connection is ignored.
    second.asked[0].answer({ re: 1, err: { code: 'forbidden', msg: 'denied' } });
    await settle();
    expect(store.infos.size).toBe(0);

    // Rejected request on a third connection is caught and ignored.
    const third = connectHome(home, { owner: true });
    store.sync([home]);
    third.asked[0].fail();
    await settle();
    expect(store.infos.size).toBe(0);
  });

  it('lists owned homes with their info and path and excludes foreign homes', async () => {
    const store = new Nodes();
    const online = new Home(me);
    const pending = new Home(me);
    const foreign = new Home(me);
    const onlineConn = connectHome(online, { owner: true });
    const pendingConn = connectHome(pending, { owner: true });
    connectHome(foreign, { owner: false });

    store.sync([online, pending, foreign]);
    const answer = hostAnswer();
    onlineConn.asked[0].answer(answer);
    await settle();
    pendingConn.link.offline();

    const directPath: Path = { kind: 'direct', rtt: 8, since: 500 };
    const net = netWith([online, pending, foreign], new Map([[online, directPath]]));
    const { re: _re, ok: _ok, ...onlineInfo } = answer;

    expect(store.list(net)).toStrictEqual([
      { home: online, info: onlineInfo, path: directPath, online: true },
      { home: pending, info: undefined, path: undefined, online: false },
    ]);
  });

  it('sends agent.set and refreshes host.info on success, without refreshing on failure', async () => {
    vi.spyOn(Date, 'now').mockReturnValue(1763600000000);
    const store = new Nodes();
    const home = new Home(me);
    const { asked, link } = connectHome(home, { owner: true });

    const call = store.setAccount(home, deviceId, 'codex', 'api_key');
    await vi.waitFor(() => expect(asked).toHaveLength(1));
    expect(JSON.parse(signed[0])).toStrictEqual({
      t: 'agent.set',
      n: expect.stringMatching(/^[A-Za-z0-9_-]{21}[AQgw]$/),
      ts: 1763600000000,
      to: deviceId,
      agent: 'codex',
      account: 'api_key',
    });
    asked[0].answer({ re: 1, ok: true });

    await vi.waitFor(() => expect(asked).toHaveLength(2));
    expect(asked[1].req).toStrictEqual({ t: 'host.info' });
    const updated = hostAnswer({ open: 3 });
    asked[1].answer(updated);

    expect(await call).toStrictEqual({ re: 1, ok: true });
    const { re: _re, ok: _ok, ...expected } = updated;
    expect(store.infos.get(home)).toStrictEqual(expected);

    // Refused agent.set returns the error and does not re-read host.info.
    const errFrame = { re: 2, err: { code: 'conflict', msg: 'denied' } };
    const refused = store.setAccount(home, deviceId, 'codex', 'team');
    await vi.waitFor(() => expect(asked).toHaveLength(3));
    asked[2].answer(errFrame);
    expect(await refused).toStrictEqual(errFrame);
    expect(asked).toHaveLength(3);

    // Offline home returns lost() without sending cmd or host.info.
    link.offline();
    expect(await store.setAccount(home, deviceId, 'codex', 'personal')).toStrictEqual(lost());
    expect(asked).toHaveLength(3);
  });

  it('signs and sends pair.browser and returns link and expires, or the error response', async () => {
    vi.spyOn(Date, 'now').mockReturnValue(1763600000000);
    const store = new Nodes();
    const home = new Home(me);
    const { asked, link } = connectHome(home, { owner: true });

    const call = store.pairBrowser(home, deviceId);
    await vi.waitFor(() => expect(asked).toHaveLength(1));
    const payload = JSON.parse(signed[0]) as Record<string, unknown>;
    expect(payload).toStrictEqual({
      t: 'pair.browser',
      n: expect.stringMatching(/^[A-Za-z0-9_-]{21}[AQgw]$/),
      ts: 1763600000000,
      to: deviceId,
    });
    expect(asked[0].req).toStrictEqual({
      t: 'cmd',
      env: { p: signed[0], k: 'this-browser', s: 'signature' },
    });

    asked[0].answer(p.decode(p.CmdResult, goldens['cmd-pair-link']));
    expect(await call).toStrictEqual({
      link: 'https://rv.example/#pair=7K2M-9QXD-4F8T-WB3N',
      expires: 1763600600000,
    });

    // An error response from the node is returned as-is.
    const refused = store.pairBrowser(home, deviceId);
    await vi.waitFor(() => expect(asked).toHaveLength(2));
    const errFrame = { re: 8, err: { code: 'conflict', msg: 'expired' } };
    asked[1].answer(errFrame);
    expect(await refused).toStrictEqual(errFrame);

    // Rejected request resolves to lost().
    const failing = store.pairBrowser(home, deviceId);
    await vi.waitFor(() => expect(asked).toHaveLength(3));
    asked[2].fail();
    expect(await failing).toStrictEqual(lost());

    // Offline home resolves to lost() without sending.
    link.offline();
    expect(await store.pairBrowser(home, deviceId)).toStrictEqual(lost());
    expect(asked).toHaveLength(3);
  });
});
