import { describeFeature, loadFeature } from '@amiceli/vitest-cucumber';
import { IDBFactory } from 'fake-indexeddb';
import { flushSync, tick } from 'svelte';
import { expect, vi } from 'vitest';
import { DeviceLog } from '../src/lib/devlog';
import { creating, takeCreate } from '../src/lib/groups.svelte';
import { Device, ed25519, encode, loadDevice, verifyEnvelope } from '../src/lib/identity';
import { loadOwner } from '../src/lib/owner';
import { mac, newSecret, parseSecret, room, shown, verifyPair } from '../src/lib/pairing';
import type { Devlog, Pair } from '../src/lib/proto';
import { all, closePage, go, one, openPage, texts } from './page';
import { FakePC } from './peer-fakes';

const feature = await loadFeature('../features/web_onboarding.feature');

class FakeSocket {
  static made: FakeSocket[] = [];
  sent: Record<string, unknown>[] = [];
  onmessage: ((event: { data: string }) => void | Promise<void>) | null = null;
  onclose: (() => void) | null = null;
  constructor(readonly url: string) {
    FakeSocket.made.push(this);
  }
  send(text: string) {
    this.sent.push(JSON.parse(text) as Record<string, unknown>);
  }
  close() {}
  async deliver(frame: unknown) {
    await this.onmessage?.({ data: JSON.stringify(frame) });
  }
  offers() {
    return this.sent
      .filter((frame) => frame.t === 'to')
      .map((frame) => JSON.parse((frame.env as { p: string }).p) as { t: string; conn: string })
      .filter((payload) => payload.t === 'offer');
  }
}

const newDevice = async () =>
  Device.of(crypto, await crypto.subtle.generateKey(ed25519, false, ['sign', 'verify']));
const makeId = () => encode(crypto.getRandomValues(new Uint8Array(16)));

async function signDeviceAdd(
  signer: Device,
  user: string,
  ver: number,
  dev: string,
  kind: 'node' | 'browser',
  name: string,
) {
  return signer.sign(
    JSON.stringify({
      t: 'device',
      n: makeId(),
      ts: Date.now(),
      user,
      ver,
      op: 'add',
      dev,
      kind,
      name,
    }),
  );
}

describeFeature(feature, ({ Background, Scenario, AfterEachScenario }) => {
  let idb: IDBFactory;
  let node: Device;
  let owner: Device;
  let copiedText: string;
  let persistCalls: number;
  let linkSecret: string;

  function setupBrowser() {
    FakeSocket.made = [];
    FakePC.reset();
    idb = new IDBFactory();
    copiedText = '';
    persistCalls = 0;
    vi.stubGlobal('indexedDB', idb);
    vi.stubGlobal('WebSocket', FakeSocket);
    vi.stubGlobal('RTCPeerConnection', FakePC);
    Object.defineProperty(navigator, 'clipboard', {
      configurable: true,
      value: {
        writeText: async (text: string) => {
          copiedText = text;
        },
      },
    });
    Object.defineProperty(navigator, 'storage', {
      configurable: true,
      value: {
        persist: async () => {
          persistCalls += 1;
          return true;
        },
      },
    });
  }

  async function signInSocket(ice: unknown[] = []) {
    await vi.waitFor(() => expect(FakeSocket.made.length).toBeGreaterThan(0));
    const ws = FakeSocket.made.at(-1)!;
    await ws.deliver({ t: 'challenge', nonce: 'LdAL134CIs7YgmZUganB2Q' });
    await vi.waitFor(() => expect(ws.sent.some((f) => f.t === 'auth')).toBe(true));
    await ws.deliver({ t: 'ready', ice, ttl: 3600 });
    return ws;
  }

  async function answerOffer(ws: FakeSocket, browserId: string) {
    await vi.waitFor(() => expect(ws.offers().length).toBeGreaterThan(0));
    const conn = ws.offers().at(-1)!.conn;
    const pc = FakePC.made.at(-1)!;
    const env = await node.sign(
      JSON.stringify({ t: 'answer', n: makeId(), ts: Date.now(), to: browserId, conn, sdp: 'ans' }),
    );
    await ws.deliver({ t: 'from', k: node.id, env });
    await vi.waitFor(() => expect(pc.remote).toBeDefined());
    pc.ctl.open();
    await vi.waitFor(() => expect(pc.ctl.frames().length).toBeGreaterThan(0));
    return pc;
  }

  function welcomeFrame(re: number, isOwner: boolean) {
    return {
      t: 'welcome',
      re,
      ok: true,
      v: 1,
      device: node.id,
      name: "Alex's MacBook",
      version: 'v0.3.1',
      time: Date.now(),
      addrs: ['wss://rv.example/rv'],
      groups: [],
      owner: isOwner,
    };
  }

  function hostInfoFrame(re: number, agents: unknown[]) {
    return {
      t: 'host.info',
      re,
      ok: true,
      device: node.id,
      name: "Alex's MacBook",
      os: 'macOS 15.2',
      version: 'v0.3.1',
      exe: '/usr/local/bin/solodawn',
      open: 0,
      max_open: 4,
      offline: false,
      update: null,
      homes: [],
      relay: null,
      agents,
      settings: {
        retention_days: 90,
        idle_minutes: 15,
        default_level: 2,
        all_level3: false,
        approvers: [],
      },
    };
  }

  AfterEachScenario(async () => {
    vi.useRealTimers();
    await closePage();
    history.replaceState(null, '', '/');
  });

  Background(({ Given }) => {
    Given('a browser set to English', () => {});
  });

  Scenario(
    'A fresh browser pairs a fresh node, enrols it, asks to keep storage and sets CLI accounts',
    ({ Given, When, Then, And }) => {
      let secret = '';
      let firstCode = '';
      let ws: FakeSocket;
      let pc: FakePC;
      let browserDev: Device;

      Given('a fresh browser and a fresh node on a rendezvous', async () => {
        setupBrowser();
        node = await newDevice();
      });

      When('the person opens the client at "#/onboarding"', async () => {
        openPage('#/onboarding');
        ws = await signInSocket();
        browserDev = await loadDevice({ crypto, idb: () => idb });
      });

      Then(
        'the prompt gives "install.md", "--rendezvous" and a 16-character code in four groups',
        () => {
          const text = one('#prompt').textContent!;
          expect(text).toContain(`${location.origin}/install.md`);
          expect(text).toContain(`--rendezvous ws://${location.host}/rv`);
          const match = /([0-9A-HJKMNP-TV-Z]{4}-){3}[0-9A-HJKMNP-TV-Z]{4}/.exec(text);
          expect(match).not.toBeNull();
          firstCode = match![0];
          secret = parseSecret(firstCode);
        },
      );

      And('the countdown starts at "10 min 0 s"', () => {
        expect(one('.prompt footer').textContent).toContain('10 min 0 s');
        expect(one('.link-steps').closest('section.card')!.textContent).not.toContain(
          'a relay takes over',
        );
      });

      When('the person copies the prompt and gives the code to the node', async () => {
        vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout'] });
        const copy = one<HTMLButtonElement>('.prompt header button');
        const write = vi
          .spyOn(navigator.clipboard, 'writeText')
          .mockRejectedValueOnce(new Error('denied'));
        copy.click();
        await expect(write.mock.results[0].value).rejects.toThrow('denied');
        await vi.advanceTimersByTimeAsync(1800);
        expect(copy.textContent?.trim()).toBe('Copy prompt');
        vi.useRealTimers();
        one<HTMLButtonElement>('.prompt header button').click();
        await tick();
        flushSync();
        const roomId = await room(crypto, secret);
        await vi.waitFor(() => expect(ws.sent).toContainEqual({ t: 'join', room: roomId }));
        await ws.deliver({ t: 'joined', room: roomId });
        await vi.waitFor(() =>
          expect(ws.sent.some((f) => f.t === 'room' && f.room === roomId)).toBe(true),
        );
        const browserRoom = ws.sent.find((f) => f.t === 'room') as { data: Pair };
        expect(await verifyPair(crypto, secret, browserRoom.data, 'a')).toBe(true);
        // Echoing the browser's own message back is ignored.
        await ws.deliver({ t: 'room', room: roomId, data: browserRoom.data });
        const nodeSelf = { role: 'b', k: node.id, kind: 'node', name: "Alex's MacBook" };
        const nodePair: Pair = {
          t: 'pair',
          ...nodeSelf,
          mac: await mac(crypto, secret, nodeSelf),
        };
        await ws.deliver({ t: 'room', room: roomId, data: nodePair });
        await vi.waitFor(() => {
          flushSync();
          expect(texts('.link-steps code')).toEqual([node.fingerprint]);
        });
      });

      Then('the clipboard holds the prompt', () => {
        expect(copiedText).toContain(firstCode);
        expect(texts('.prompt header button')).toEqual(['Copied']);
      });

      And('the page shows the node "Alex\'s MacBook" with its 12-digit fingerprint', () => {
        expect(one('.link-steps').textContent).toContain("Alex's MacBook");
        expect(texts('.link-steps code')).toEqual([node.fingerprint]);
        expect(node.fingerprint).toMatch(/^[0-9A-F]{4} [0-9A-F]{4} [0-9A-F]{4}$/);
      });

      When(
        'the connection opens and the node welcomes the browser before and after enrolment',
        async () => {
          pc = await answerOffer(ws, browserDev.id);
          // First hello: before devlog, owner is false.
          pc.ctl.deliver(welcomeFrame(1, false));
          await vi.waitFor(() => expect(pc.ctl.frames().some((f) => f.t === 'devlog')).toBe(true));
          await vi.waitFor(() =>
            expect(pc.ctl.frames().filter((f) => f.t === 'hello')).toHaveLength(2),
          );
          const secondHello = pc.ctl.frames().filter((f) => f.t === 'hello')[1];
          pc.ctl.deliver(welcomeFrame(secondHello.id, true));
          await vi.waitFor(() =>
            expect(pc.ctl.frames().some((f) => f.t === 'host.info')).toBe(true),
          );
          const hostReq = pc.ctl.frames().find((f) => f.t === 'host.info')!;
          pc.ctl.deliver(
            hostInfoFrame(hostReq.id, [
              {
                agent: 'claude',
                cli_version: '2.1.311',
                account: null,
                login_ok: true,
                driver: { accounts: ['personal', 'team', 'api_key'], takeover: 'new' },
                models: [],
                caps: {},
                groups: [],
              },
              {
                agent: 'codex',
                cli_version: '0.164.0',
                account: null,
                login_ok: true,
                driver: { accounts: ['personal', 'team', 'api_key'], takeover: 'resume' },
                models: [],
                caps: {},
                groups: [],
              },
            ]),
          );
          await vi.waitFor(() => {
            flushSync();
            expect(all('fieldset.acct')).toHaveLength(2);
          });
        },
      );

      Then(
        'the browser sent a two-entry device log without a request id and asked to keep storage',
        async () => {
          const sentDevlog = pc.ctl.frames().find((f) => f.t === 'devlog') as Devlog & {
            id?: unknown;
          };
          expect(sentDevlog.id).toBeUndefined();
          expect(sentDevlog.user).toBe(browserDev.id);
          expect(sentDevlog.entries).toHaveLength(2);
          const replayed = await DeviceLog.replay(crypto, sentDevlog.user, sentDevlog.entries);
          expect(replayed.adopts(node.id, browserDev.id)).toBe(true);
          expect(persistCalls).toBe(1);
          expect(one('#prompt').textContent).not.toContain(firstCode);
          const stored = await loadOwner(crypto, () => idb);
          expect(stored.log?.validDevices()).toStrictEqual([browserDev.id, node.id].sort());
        },
      );

      And('the page lists "Claude Code" and "Codex" for account declaration', () => {
        expect(one('.link-steps').textContent).toContain('Found: Claude Code, Codex');
        expect(texts('section.card h2.kind').slice(-2)).toEqual([
          'How does Claude Code sign in?',
          'How does Codex sign in?',
        ]);
        expect(all('.done-btn')).toHaveLength(0);
      });

      When(
        'the person declares "Personal subscription" for "Claude Code" and "API key" for "Codex"',
        async () => {
          const claudeRadio = one<HTMLInputElement>('input[name="acct-claude"][value="personal"]');
          claudeRadio.checked = true;
          claudeRadio.dispatchEvent(new Event('change', { bubbles: true }));
          await vi.waitFor(() =>
            expect(pc.ctl.frames().filter((f) => f.t === 'cmd')).toHaveLength(1),
          );
          const cmd1 = pc.ctl.frames().filter((f) => f.t === 'cmd')[0];
          pc.ctl.deliver({ re: cmd1.id, ok: true });
          await vi.waitFor(() => {
            flushSync();
            expect(one<HTMLFieldSetElement>('fieldset.acct').disabled).toBe(false);
          });

          const codexRadio = one<HTMLInputElement>('input[name="acct-codex"][value="api_key"]');
          codexRadio.checked = true;
          codexRadio.dispatchEvent(new Event('change', { bubbles: true }));
          await vi.waitFor(() =>
            expect(pc.ctl.frames().filter((f) => f.t === 'cmd')).toHaveLength(2),
          );
          const cmd2 = pc.ctl.frames().filter((f) => f.t === 'cmd')[1];
          pc.ctl.deliver({ re: cmd2.id, ok: true });
          await vi.waitFor(() => {
            flushSync();
            expect(all('.done-btn')).toHaveLength(1);
          });
        },
      );

      Then('the node received signed "agent.set" commands for "claude" and "codex"', async () => {
        const cmds = pc.ctl.frames().filter((f) => f.t === 'cmd');
        expect(cmds).toHaveLength(2);
        for (const cmd of cmds) {
          expect(await verifyEnvelope(crypto, cmd.env)).toBe(true);
        }
        const payloads = cmds.map((c) => JSON.parse(c.env.p));
        expect(payloads[0]).toMatchObject({
          t: 'agent.set',
          to: node.id,
          agent: 'claude',
          account: 'personal',
        });
        expect(payloads[1]).toMatchObject({
          t: 'agent.set',
          to: node.id,
          agent: 'codex',
          account: 'api_key',
        });
      });

      And('"Next: set your AIs\' defaults" leads to "#/nodes"', () => {
        const done = one<HTMLAnchorElement>('.done-btn');
        expect(done.textContent!.trim()).toBe("Next: set your AIs' defaults");
        expect(done.getAttribute('href')).toBe('#/nodes');
        done.click();
        expect(creating.open).toBe(false);
        expect(takeCreate()).toBe(false);
        // The browser tells the page that the address changed; the test DOM does not.
        go('#/nodes');
        expect(location.hash).toBe('#/nodes');
        expect(one('#page-title').textContent).toBe('My computers and AIs');
      });
    },
  );

  Scenario(
    'The pairing secret expires after 10 minutes and can be replaced',
    ({ Given, When, Then, And }) => {
      let initialCode = '';

      Given('a fresh browser and a fresh node on a rendezvous', async () => {
        vi.useFakeTimers();
        setupBrowser();
        node = await newDevice();
      });

      When('the person opens the client at "#/onboarding"', () => {
        openPage('#/onboarding');
        const match = /([0-9A-HJKMNP-TV-Z]{4}-){3}[0-9A-HJKMNP-TV-Z]{4}/.exec(
          one('#prompt').textContent!,
        );
        initialCode = match![0];
      });

      And('9 minutes and 40 seconds pass', async () => {
        await vi.advanceTimersByTimeAsync(580_000);
        flushSync();
      });

      Then('the countdown reads "20 s"', () => {
        expect(one('.prompt footer').textContent).toContain('20 s');
      });

      When('20 more seconds pass', async () => {
        await vi.advanceTimersByTimeAsync(20_000);
        flushSync();
      });

      Then('the pairing code is discarded and the page says it expired', () => {
        expect(one('#prompt').textContent).not.toContain(initialCode);
        expect(one('.prompt footer').textContent).toContain('expired and was discarded');
      });

      When('the person presses "New pairing code"', () => {
        one<HTMLButtonElement>('.prompt header button').click();
        flushSync();
      });

      Then('the prompt gives a new 16-character code and a fresh "10 min 0 s" countdown', () => {
        const match = /([0-9A-HJKMNP-TV-Z]{4}-){3}[0-9A-HJKMNP-TV-Z]{4}/.exec(
          one('#prompt').textContent!,
        );
        expect(match).not.toBeNull();
        expect(match![0]).not.toBe(initialCode);
        expect(one('.prompt footer').textContent).toContain('10 min 0 s');
      });
    },
  );

  Scenario(
    "A new browser opened from a pairing link adopts the node's device log",
    ({ Given, When, Then }) => {
      let ws: FakeSocket;
      let pc: FakePC;
      let browserDev: Device;

      Given(
        'a node that already belongs to the owner and created a browser pairing secret',
        async () => {
          setupBrowser();
          owner = await newDevice();
          node = await newDevice();
          linkSecret = newSecret(crypto);
        },
      );

      When('a fresh browser opens the pairing link', async () => {
        openPage(`#pair=${shown(linkSecret)}`);
        ws = await signInSocket([
          { urls: ['turn:rv.example:3478?transport=udp'], username: 'u', credential: 'p' },
        ]);
        browserDev = await loadDevice({ crypto, idb: () => idb });
        await vi.waitFor(() => {
          flushSync();
          expect(one('.link-steps').closest('section.card')!.textContent).toContain(
            'a relay takes over',
          );
        });
      });

      Then(
        'the address becomes "#/onboarding" and the browser joins the room in role "b"',
        async () => {
          expect(location.hash).toBe('#/onboarding');
          const roomId = await room(crypto, linkSecret);
          await vi.waitFor(() => expect(ws.sent).toContainEqual({ t: 'join', room: roomId }));
          await ws.deliver({ t: 'joined', room: roomId });
          await vi.waitFor(() =>
            expect(ws.sent.some((f) => f.t === 'room' && f.room === roomId)).toBe(true),
          );
          const sentPair = ws.sent.find((f) => f.t === 'room') as { data: Pair };
          expect(sentPair.data.role).toBe('b');
          expect(await verifyPair(crypto, linkSecret, sentPair.data, 'b')).toBe(true);
        },
      );

      When("the node pairs, welcomes the browser and pushes the owner's device log", async () => {
        const roomId = await room(crypto, linkSecret);
        const nodeSelf = { role: 'a', k: node.id, kind: 'node', name: 'workstation' };
        await ws.deliver({
          t: 'room',
          room: roomId,
          data: { t: 'pair', ...nodeSelf, mac: await mac(crypto, linkSecret, nodeSelf) },
        });
        pc = await answerOffer(ws, browserDev.id);
        pc.ctl.deliver(welcomeFrame(1, false));
        await tick();
        const e1 = await signDeviceAdd(owner, owner.id, 1, owner.id, 'browser', 'desktop');
        const e2 = await signDeviceAdd(owner, owner.id, 2, node.id, 'node', 'workstation');
        const e3 = await signDeviceAdd(node, owner.id, 3, browserDev.id, 'browser', 'Browser');
        pc.ctl.deliver({ t: 'devlog', user: owner.id, entries: [e1, e2, e3] });
        await vi.waitFor(() =>
          expect(pc.ctl.frames().filter((f) => f.t === 'hello')).toHaveLength(2),
        );
        const secondHello = pc.ctl.frames().filter((f) => f.t === 'hello')[1];
        pc.ctl.deliver(welcomeFrame(secondHello.id, true));
        await vi.waitFor(() => expect(pc.ctl.frames().some((f) => f.t === 'host.info')).toBe(true));
        const hostReq = pc.ctl.frames().find((f) => f.t === 'host.info')!;
        pc.ctl.deliver(hostInfoFrame(hostReq.id, []));
        await vi.waitFor(() => {
          flushSync();
          expect(all('.done-btn')).toHaveLength(1);
        });
      });

      Then(
        'the browser adopts the log, sends a second "hello" and asks to keep storage',
        async () => {
          expect(persistCalls).toBe(1);
          const stored = await loadOwner(crypto, () => idb);
          expect(stored.log?.user).toBe(owner.id);
          expect(stored.log?.valid(browserDev.id)).toBe(true);
        },
      );
    },
  );

  Scenario('A malformed pairing link shows its error', ({ Given, When, Then }) => {
    Given('a fresh browser and a fresh node on a rendezvous', () => {
      setupBrowser();
    });

    When('the person opens a pairing link with secret "not-a-secret"', () => {
      openPage('#pair=not-a-secret');
    });

    Then('the page shows that the pairing code in the link is not valid', async () => {
      await vi.waitFor(() => {
        flushSync();
        expect(texts('[role="alert"]')).toEqual(['The pairing code in this link is not valid.']);
      });
    });
  });
});
