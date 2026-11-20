import { describeFeature, loadFeature } from '@amiceli/vitest-cucumber';
import { expect, vi } from 'vitest';
import { Device, ed25519, verifyEnvelope } from '../src/lib/identity';
import { connect, type Session } from '../src/lib/peer';
import type { Ctl, Welcome } from '../src/lib/proto';
import { FakePC } from './peer-fakes';

const feature = await loadFeature('../features/web_peer.feature');

const url = 'wss://rv.example/rv';
const cand = 'candidate:1 1 udp 2130706431 192.0.2.10 47800 typ host';
const fetch = { t: 'fetch', g: 'group', thread: null, before: 0, limit: 50 } as const;
const fetched = { t: 'fetched', ok: true, rows: [], more: false };

/** The rendezvous's end of the browser's signalling address. */
class Address {
  static last: Address;
  sent: { t: string; k: string; env: { p: string; k: string; s: string } }[] = [];
  onmessage!: (event: { data: string }) => void | Promise<void>;
  constructor() {
    Address.last = this;
  }
  send(text: string) {
    this.sent.push(JSON.parse(text));
  }
  close() {}
  deliver(frame: unknown) {
    return this.onmessage({ data: JSON.stringify(frame) });
  }
  /** The payloads of the offers the browser sent the node, oldest first. */
  offers() {
    return this.sent
      .filter((frame) => frame.t === 'to')
      .map((frame) => JSON.parse(frame.env.p))
      .filter((payload) => payload.t === 'offer');
  }
}

const newDevice = async () =>
  Device.of(crypto, await crypto.subtle.generateKey(ed25519, false, ['sign', 'verify']));
const newId = () => btoa('0123456789abcdef').slice(0, 21) + 'A';

describeFeature(feature, ({ Background, Scenario, AfterEachScenario }) => {
  let browser: Device;
  let node: Device;
  let session: Session;
  let heard: { t: string }[];
  let welcomed: { ctl: Ctl; welcome: Welcome }[];
  let offline: number;

  AfterEachScenario(() => {
    session.close();
    vi.unstubAllGlobals();
  });

  /** A signal of the node about a connection of the browser, through the rendezvous. Resolves
   *  once the browser has checked it and handed it on. */
  async function signal(conn: string, fields: object) {
    const header = { n: newId(), ts: Date.now(), to: browser.id, conn };
    const env = await node.sign(JSON.stringify({ ...fields, ...header }));
    const before = heard.length;
    void Address.last.deliver({ t: 'from', k: node.id, env });
    await vi.waitFor(() => expect(heard.length).toBe(before + 1));
  }

  const welcome = () => ({
    t: 'welcome',
    re: 1,
    ok: true,
    v: 1,
    device: node.id,
    name: 'workstation',
    version: 'v0.3.1',
    time: Date.now(),
    addrs: [],
    groups: [],
    owner: true,
  });

  /** Answers the newest offer, opens its ctl and welcomes the browser's hello. */
  async function connectNewest() {
    const pc = FakePC.made.at(-1)!;
    const before = welcomed.length;
    await signal(Address.last.offers().at(-1).conn, { t: 'answer', sdp: 'answer' });
    pc.ctl.open();
    await vi.waitFor(() => expect(pc.ctl.frames()).toHaveLength(1));
    pc.ctl.deliver(welcome());
    await vi.waitFor(() => expect(welcomed).toHaveLength(before + 1));
    return pc;
  }

  async function offer() {
    session.peer(node.id, () => ({
      online: (ctl, welcome) => welcomed.push({ ctl, welcome }),
      push: () => {},
      offline: () => (offline += 1),
    }));
    await vi.waitFor(() => expect(Address.last.offers()).toHaveLength(1));
  }

  Background(({ Given }) => {
    Given('a browser signed in on a rendezvous, and a node it knows', async () => {
      FakePC.reset();
      vi.stubGlobal('WebSocket', Address);
      vi.stubGlobal('RTCPeerConnection', FakePC);
      [browser, node] = [await newDevice(), await newDevice()];
      [heard, welcomed, offline] = [[], [], 0];
      session = connect({
        urls: [url],
        device: browser,
        known: (device) => device === node.id,
        name: 'phone browser',
        heard: (_, frame) => heard.push(frame),
      });
      await Address.last.deliver({ t: 'challenge', nonce: 'LdAL134CIs7YgmZUganB2Q' });
      await Address.last.deliver({ t: 'ready', ice: [], ttl: 3600 });
      await vi.waitFor(() => expect(heard).toHaveLength(1));
    });
  });

  Scenario('Early and repeated candidates of the node are applied once', ({ When, Then, And }) => {
    let pc: FakePC;
    let conn: string;
    When('the browser offers the node a connection', offer);
    Then('the node receives an offer the browser signed', async () => {
      const [sent] = Address.last.sent.filter((frame) => frame.t === 'to');
      expect(sent.k).toBe(node.id);
      expect(sent.env.k).toBe(browser.id);
      expect(await verifyEnvelope(crypto, sent.env)).toBe(true);
      [pc] = FakePC.made;
      const [offer] = Address.last.offers();
      expect(offer).toMatchObject({ sdp: pc.local!.sdp, probe: false, to: node.id });
      conn = offer.conn;
    });
    When('two copies of a candidate of the node arrive before its answer', async () => {
      await signal(conn, { t: 'ice', cand, mid: '0' });
      await signal(conn, { t: 'ice', cand, mid: '0' });
    });
    Then('the browser has applied no candidate', () => {
      expect(pc.added).toStrictEqual([]);
    });
    When("the node's answer arrives", async () => {
      await signal(conn, { t: 'answer', sdp: 'answer' });
      await vi.waitFor(() => expect(pc.remote).toStrictEqual({ type: 'answer', sdp: 'answer' }));
    });
    Then('the browser has applied the candidate exactly once', async () => {
      await vi.waitFor(() => expect(pc.added).toStrictEqual([{ candidate: cand, sdpMid: '0' }]));
      expect(pc.early).toBe(0);
    });
    When('the connection opens and another copy of the candidate arrives', async () => {
      pc.ctl.open();
      await vi.waitFor(() => expect(pc.ctl.frames()).toHaveLength(1));
      pc.ctl.deliver(welcome());
      await vi.waitFor(() => expect(welcomed).toHaveLength(1));
      await signal(conn, { t: 'ice', cand, mid: '0' });
    });
    Then('the candidate is still applied once', () => {
      expect(pc.added).toHaveLength(1);
    });
    And('a request on the connection is answered', async () => {
      const asked = welcomed[0].ctl.request(fetch);
      expect(pc.ctl.frames().at(-1)).toStrictEqual({ ...fetch, id: 2 });
      pc.ctl.deliver({ ...fetched, re: 2 });
      expect(await asked).toStrictEqual({ ...fetched, re: 2 });
    });
  });

  Scenario(
    'An unanswered command is sent again on the next connection',
    ({ Given, When, Then }) => {
      let first: FakePC;
      let second: FakePC;
      let answered: ReturnType<Ctl['request']>;
      const cmd = { t: 'cmd', env: { p: '{"t":"post"}', k: 'signer', s: 'signature' } } as const;
      Given('the browser is connected to the node', async () => {
        await offer();
        first = await connectNewest();
      });
      When(
        'the browser sends a command and the connection is lost before the node answers',
        async () => {
          answered = welcomed[0].ctl.request(cmd);
          expect(first.ctl.frames().at(-1)).toStrictEqual({ ...cmd, id: 2 });
          first.ctl.onclose();
          await vi.waitFor(() => expect(offline).toBe(1));
        },
      );
      Then('the browser offers the node a new connection', async () => {
        await vi.waitFor(() => expect(Address.last.offers()).toHaveLength(2), { timeout: 5000 });
        const [one, two] = Address.last.offers();
        expect(two.conn).not.toBe(one.conn);
        expect(first.closes).toBe(1);
      });
      When('the node welcomes the browser on it', async () => {
        second = await connectNewest();
      });
      Then('the command is sent again and its answer reaches the sender', async () => {
        expect(second.ctl.frames().at(-1)).toStrictEqual({ ...cmd, id: 2 });
        second.ctl.deliver({ re: 2, ok: true, num: 14 });
        expect(await answered).toStrictEqual({ re: 2, ok: true, num: 14 });
      });
    },
  );
});
