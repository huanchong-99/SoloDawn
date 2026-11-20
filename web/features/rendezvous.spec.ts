import { describeFeature, loadFeature } from '@amiceli/vitest-cucumber';
import { expect, vi } from 'vitest';
import { Device, ed25519, verifyChallenge } from '../src/lib/identity';
import { rendezvous } from '../src/lib/rendezvous';

const feature = await loadFeature('../features/web_rendezvous.feature');

const remote = 'wss://rv.example/rv';
const local = 'ws://localhost:47800/rv';
const nonces = { [remote]: 'LdAL134CIs7YgmZUganB2Q', [local]: 'AAAAAAAAAAAAAAAAAAAAAA' };
const ice = [{ urls: ['stun:rv.example:3478'] }];
const ready = {
  [remote]: { t: 'ready', ice, ttl: 3600 },
  [local]: { t: 'ready', ice: [], ttl: 3600 },
};

/** One end of a signalling address, as the test rendezvous holds it. */
class Address {
  sent: unknown[] = [];
  onmessage!: (event: { data: string }) => void | Promise<void>;
  constructor(readonly url: keyof typeof nonces) {
    addresses.set(url, this);
  }
  send(text: string) {
    this.sent.push(JSON.parse(text));
  }
  deliver(frame: unknown) {
    return this.onmessage({ data: JSON.stringify(frame) });
  }
  challenge() {
    return this.deliver({ t: 'challenge', nonce: nonces[this.url] });
  }
  async signIn() {
    await this.challenge();
    await this.deliver(ready[this.url]);
  }
}

const addresses = new Map<string, Address>();

/** A random id for the connection the signals are about. */
const conn = 'BBBBBBBBBBBBBBBBBBBBBA';
/** A frame the rendezvous sends after another, to know that the one before it was handled. */
const offline = { t: 'offline', k: 'the node of another person' };

const newDevice = async () =>
  Device.of(crypto, await crypto.subtle.generateKey(ed25519, false, ['sign', 'verify']));

describeFeature(feature, ({ Background, Scenario, AfterEachScenario }) => {
  let browser: Device;
  let node: Device;
  let client: ReturnType<typeof rendezvous>;
  let given: unknown[][];
  const rv = () => addresses.get(remote)!;
  const lan = () => addresses.get(local)!;

  AfterEachScenario(() => {
    vi.unstubAllGlobals();
  });

  Background(({ Given }) => {
    Given(
      'a browser that knows a node, with a rendezvous and a local node as signalling addresses',
      async () => {
        vi.stubGlobal('WebSocket', Address);
        [browser, node] = [await newDevice(), await newDevice()];
        given = [];
        client = rendezvous(
          [remote, local],
          browser,
          (device) => device === node.id,
          (...told) => given.push(told),
        );
      },
    );
  });

  Scenario(
    "The browser proves its key on each address with that address's challenge",
    ({ When, Then }) => {
      When('the rendezvous and the local node each send a challenge of their own', async () => {
        await rv().challenge();
        await lan().challenge();
      });
      Then("each receives the browser's signature of its own challenge", async () => {
        for (const address of [rv(), lan()]) {
          const [auth] = address.sent as { s: string }[];
          expect(address.sent).toStrictEqual([{ t: 'auth', k: browser.id, s: auth.s }]);
          expect(await verifyChallenge(crypto, browser.id, nonces[address.url], auth.s)).toBe(true);
        }
      });
      When('each answers that the browser is signed in', async () => {
        await rv().deliver(ready[remote]);
        await lan().deliver(ready[local]);
      });
      Then('the browser is told the ICE servers of each address', async () => {
        await vi.waitFor(() =>
          expect(given).toStrictEqual([
            [remote, ready[remote]],
            [local, ready[local]],
          ]),
        );
      });
    },
  );

  Scenario(
    'An offer goes out on every address, and a slow one delays no other',
    ({ Given, When, Then }) => {
      let offer: Parameters<typeof client.send>[0];
      Given('the rendezvous has signed the browser in and the local node has not yet', () =>
        rv().signIn(),
      );
      When('the browser sends the node an offer', async () => {
        const p = { t: 'offer', n: nonces[local], ts: Date.now(), to: node.id, conn };
        const env = await browser.sign(JSON.stringify({ ...p, sdp: 'v=0', probe: false }));
        offer = { t: 'to', k: node.id, env };
        client.send(offer);
      });
      Then('the rendezvous has the offer and the local node has nothing', () => {
        expect(rv().sent.slice(1)).toStrictEqual([offer]);
        expect(lan().sent).toStrictEqual([]);
      });
      When('the local node signs the browser in', () => lan().signIn());
      Then('the local node has the offer too', () => {
        expect(lan().sent.slice(1)).toStrictEqual([offer]);
      });
    },
  );

  Scenario(
    'An answer whose description a rendezvous changed is refused',
    ({ Given, When, Then }) => {
      let answer: { t: string; k: string; env: { p: string; k: string; s: string } };
      Given('both addresses have signed the browser in', async () => {
        await rv().signIn();
        await lan().signIn();
        await vi.waitFor(() => expect(given).toHaveLength(2));
        given = [];
      });
      When(
        "the node's signed answer arrives through the rendezvous with its description changed",
        async () => {
          const p = { t: 'answer', n: nonces[local], ts: Date.now(), to: browser.id, conn };
          const env = await node.sign(JSON.stringify({ ...p, sdp: 'v=0 as the node wrote it' }));
          answer = { t: 'from', k: node.id, env };
          const p2 = env.p.replace('as the node wrote it', 'as the rendezvous wants it');
          void rv().deliver({ ...answer, env: { ...env, p: p2 } });
          // One address delivers in order: once this has arrived, the answer has been handled.
          void rv().deliver(offline);
          await vi.waitFor(() => expect(given).toStrictEqual([[remote, offline]]));
        },
      );
      Then('the connection is given nothing', () => {
        expect(given).toStrictEqual([[remote, offline]]);
      });
      When('the same answer arrives through the local node as the node signed it', () =>
        lan().deliver(answer),
      );
      Then('the connection is given that answer from the local node', async () => {
        await vi.waitFor(() =>
          expect(given).toStrictEqual([
            [remote, offline],
            [local, answer],
          ]),
        );
      });
    },
  );
});
