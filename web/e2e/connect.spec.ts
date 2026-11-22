// Browser end-to-end connection scenarios against real solodawn rendezvous and solodawn node
// processes across Chromium, Firefox and WebKit: onboarding pairing smoke, A1.3 (direct and
// relayed path indicators plus periodic host.info without probe migration), A1.11 (early and
// duplicate ICE candidates buffered until setRemoteDescription) and A1.12 (single active tab lock
// with a "-" device id, takeover and suppressed reconnect).
import { execFile } from 'node:child_process';
import { mkdir, mkdtemp, rm, writeFile } from 'node:fs/promises';
import { join, resolve } from 'node:path';
import { setTimeout as delay } from 'node:timers/promises';
import { promisify } from 'node:util';
import type { Browser, BrowserContext, Page } from 'playwright';
import { afterAll, afterEach, beforeAll, beforeEach, describe, expect, inject, test } from 'vitest';
import { launch } from './config';
import {
  parseSignallingEnvelope,
  pickLanIPv4,
  poll,
  readNodeDeviceId,
  startSignallingProxy,
  startStunResponder,
  type IceServer,
  type SignallingProxy,
} from './direct';
import { leader, startNet, stop, type Net, type Node } from './nodes';

const webDir = resolve(import.meta.dirname, '..');
const repoDir = resolve(webDir, '..');
const codePattern = /([0-9A-HJKMNP-TV-Z]{4}-){3}[0-9A-HJKMNP-TV-Z]{4}/;
const fpPattern = /[0-9A-F]{4} [0-9A-F]{4} [0-9A-F]{4}/;
const pastInitialReconnectMs = 1400;

const turnSource = `package main

import (
	"fmt"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/pion/turn/v5"
)

type relayPool struct {
	turn.RelayAddressGeneratorStatic
	mu    sync.Mutex
	ports map[int]bool
}

func (g *relayPool) AllocatePacketConn(conf turn.AllocateListenerConfig) (net.PacketConn, net.Addr, error) {
	conn, addr, err := g.RelayAddressGeneratorStatic.AllocatePacketConn(conf)
	if err != nil {
		return nil, nil, err
	}
	g.mu.Lock()
	g.ports[addr.(*net.UDPAddr).Port] = true
	g.mu.Unlock()
	return relaySocket{PacketConn: conn, pool: g}, addr, nil
}

func (g *relayPool) hasPort(addr net.Addr) bool {
	udp, ok := addr.(*net.UDPAddr)
	if !ok {
		return false
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.ports[udp.Port]
}

type relaySocket struct {
	net.PacketConn
	pool *relayPool
}

func (s relaySocket) ReadFrom(b []byte) (int, net.Addr, error) {
	for {
		n, addr, err := s.PacketConn.ReadFrom(b)
		if err != nil || s.pool.hasPort(addr) {
			return n, addr, err
		}
	}
}

func main() {
	host := os.Args[1]
	ip := net.ParseIP(host)
	pc, err := net.ListenPacket("udp4", net.JoinHostPort(host, "0"))
	if err != nil {
		os.Exit(1)
	}
	gen := &relayPool{
		RelayAddressGeneratorStatic: turn.RelayAddressGeneratorStatic{
			RelayAddress: ip,
			Address:      host,
		},
		ports: map[int]bool{},
	}
	srv, err := turn.NewServer(turn.ServerConfig{
		Realm: "solodawn.test",
		AuthHandler: func(r *turn.RequestAttributes) (string, []byte, bool) {
			key := turn.GenerateAuthKey("peer", "solodawn.test", "secret")
			return "peer", key, r.Username == "peer"
		},
		PacketConnConfigs: []turn.PacketConnConfig{{
			PacketConn:            pc,
			RelayAddressGenerator: gen,
		}},
	})
	if err != nil {
		os.Exit(1)
	}
	defer srv.Close()
	fmt.Printf("turn:%s?transport=udp\\n", pc.LocalAddr().String())
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	<-sig
}
`;

interface TurnProc {
  url: string;
  pid: number;
  stop(): Promise<void>;
}

/** Starts the Pion relay-only TURN server in its own process group via `leader` and `stop`. */
async function startTurnRelay(binPath: string, host: string, dir: string, pidRecord: string) {
  const proc = leader('turn-relay', [binPath, host], {}, dir, pidRecord);
  let url = '';
  let exited = false;
  void proc.closed.then(() => {
    exited = true;
  });
  for (const end = Date.now() + 15_000; !url; await delay(15)) {
    const match = /^(turn:\S+)\r?$/m.exec(proc.output());
    if (match) url = match[1];
    else if (exited || Date.now() > end) {
      await stop(proc);
      throw new Error(`turn-relay failed to print URL:\n${proc.output()}`);
    }
  }
  return { url, pid: proc.pid, stop: () => stop(proc) } satisfies TurnProc;
}

/** Reads the 16-character pairing code from #prompt once Onboarding has rendered it. */
async function waitForPairingCode(page: Page, expectedSignal: string): Promise<string> {
  await page.waitForFunction(
    ({ sig }) => {
      const notice = document.querySelector('main > .card.need h1, [role="alert"]');
      if (notice) throw new Error(`onboarding blocked: ${notice.textContent}`);
      const text = document.querySelector('#prompt')?.textContent ?? '';
      return (
        text.includes(`--rendezvous ${sig}`) &&
        /([0-9A-HJKMNP-TV-Z]{4}-){3}[0-9A-HJKMNP-TV-Z]{4}/.test(text)
      );
    },
    { sig: expectedSignal },
  );
  const promptText = (await page.locator('#prompt').textContent()) ?? '';
  const match = codePattern.exec(promptText);
  expect(match).not.toBeNull();
  return match ? match[0] : '';
}

/** Pairs a fresh browser on #/onboarding with node and waits until enrolment finishes. */
async function pairBrowserOnPage(
  page: Page,
  baseUrl: string,
  signalUrl: string,
  node: Node,
  label = 'workstation',
) {
  await page.goto(`${baseUrl}#/onboarding`);
  const code = await waitForPairingCode(page, signalUrl);
  const paired = await node.run(
    'solodawn',
    'pair',
    code,
    '--name',
    label,
    '--rendezvous',
    signalUrl,
  );
  expect(paired.code, paired.stderr).toBe(0);
  await page.waitForFunction(() =>
    (document.querySelector('.link-steps')?.textContent ?? '').includes('Found:'),
  );
}

async function installIndicatorProbe(page: Page, expectedDevice: string) {
  await page.addInitScript((wantDevice: string) => {
    let ctlOpenedAt = 0;
    let ctlChannel: RTCDataChannel | undefined;
    let nextReqId = 800_000;
    const origCreateChannel = RTCPeerConnection.prototype.createDataChannel;
    RTCPeerConnection.prototype.createDataChannel = function (
      label: string,
      opts?: RTCDataChannelInit,
    ) {
      const dc = origCreateChannel.call(this, label, opts);
      if (label === 'ctl') {
        dc.addEventListener('open', () => {
          ctlOpenedAt = performance.now();
          ctlChannel = dc;
        });
      }
      return dc;
    };
    const askHostInfo = () =>
      new Promise<boolean>((resolveInfo, rejectInfo) => {
        if (!ctlChannel || ctlChannel.readyState !== 'open') {
          rejectInfo(new Error('ctl channel not open'));
          return;
        }
        const reqId = (nextReqId += 1);
        const onMsg = (ev: MessageEvent) => {
          const msg = JSON.parse(String(ev.data)) as {
            re?: number;
            ok?: boolean;
            t?: string;
            device?: string;
          };
          if (msg.re === reqId) {
            ctlChannel?.removeEventListener('message', onMsg);
            resolveInfo(msg.ok === true && msg.t === 'host.info' && msg.device === wantDevice);
          }
        };
        ctlChannel.addEventListener('message', onMsg);
        ctlChannel.send(JSON.stringify({ id: reqId, t: 'host.info' }));
      });
    Object.assign(window, { __a13: { openedAt: () => ctlOpenedAt, askHostInfo } });
  }, expectedDevice);
}

async function measureIndicator(
  targetCtx: BrowserContext,
  baseUrl: string,
  signalUrl: string,
  expectedState: 'ok' | 'limited',
  expectedDevice: string,
) {
  const checkPage = await targetCtx.newPage();
  await checkPage.routeWebSocket(
    (url) => !url.href.startsWith(signalUrl),
    (ws) => ws.close(),
  );
  await installIndicatorProbe(checkPage, expectedDevice);
  await checkPage.goto(`${baseUrl}#/nodes`);
  try {
    return await checkPage.evaluate(async (wantSt) => {
      type Hook = { openedAt(): number; askHostInfo(): Promise<boolean> };
      const hook = (window as unknown as { __a13: Hook }).__a13;
      const sleep = (ms: number) => new Promise((r) => setTimeout(r, ms));
      const bar = () => document.querySelector('.bar a.conn') as HTMLElement | null;
      const waitCtl = async () => {
        for (const end = performance.now() + 15_000; hook.openedAt() === 0; await sleep(15)) {
          if (performance.now() >= end) throw new Error('ctl did not open');
        }
        return hook.openedAt();
      };
      const waitState = async (openedAt: number) => {
        for (const end = performance.now() + 2000; performance.now() < end; await sleep(15)) {
          const el = bar();
          if (el && el.dataset.st === wantSt) return Math.max(0, performance.now() - openedAt);
        }
        return -1;
      };
      const kindMap: Record<string, string> = { ok: 'direct', limited: 'relay' };
      const sampleThree = async () => {
        const out: { ok: boolean; kind: string | undefined }[] = [];
        for (let i = 0; i < 3; i++) {
          if (i > 0) await sleep(1000);
          const ok = await hook.askHostInfo();
          const el = bar();
          out.push({ ok, kind: el ? kindMap[el.dataset.st ?? ''] : undefined });
        }
        return out;
      };
      const indicatorMs = await waitState(await waitCtl());
      const el = bar();
      const text = (el ? (el.textContent ?? '') : '').replace(/\s+/g, ' ').trim();
      const label = el ? (el.getAttribute('aria-label') ?? '') : '';
      return { indicatorMs, text, label, samples: await sampleThree() };
    }, expectedState);
  } finally {
    await checkPage.close();
  }
}

async function installEarlyCandidateProbe(ctx: BrowserContext) {
  await ctx.addInitScript(() => {
    const iceCalls: {
      candidate: string;
      sdpMid: string | null | undefined;
      hadRemote: boolean;
    }[] = [];
    let fromFrames = 0;
    let ctlChannel: RTCDataChannel | undefined;

    const OrigWS = window.WebSocket;
    window.WebSocket = class extends OrigWS {
      constructor(url: string | URL, protocols?: string | string[]) {
        super(url, protocols);
        this.addEventListener('message', (ev) => {
          try {
            const parsed = JSON.parse(String(ev.data)) as { t?: string };
            if (parsed.t === 'from') fromFrames += 1;
          } catch {
            // ignore non-JSON
          }
        });
      }
    };

    const remoteReady = new WeakSet<RTCPeerConnection>();
    const origSetRemote: (
      this: RTCPeerConnection,
      desc: RTCSessionDescriptionInit,
    ) => Promise<void> = RTCPeerConnection.prototype.setRemoteDescription;
    RTCPeerConnection.prototype.setRemoteDescription = async function (
      desc: RTCSessionDescriptionInit,
    ) {
      await origSetRemote.call(this, desc);
      remoteReady.add(this);
    };
    const origAddIce: (this: RTCPeerConnection, cand?: RTCIceCandidateInit) => Promise<void> =
      RTCPeerConnection.prototype.addIceCandidate;
    RTCPeerConnection.prototype.addIceCandidate = function (
      cand?: RTCIceCandidateInit | null,
    ): Promise<void> {
      iceCalls.push({
        candidate: cand?.candidate ?? '',
        sdpMid: cand?.sdpMid,
        hadRemote: remoteReady.has(this),
      });
      return origAddIce.call(this, cand ?? undefined);
    };
    const origCreateChannel = RTCPeerConnection.prototype.createDataChannel;
    RTCPeerConnection.prototype.createDataChannel = function (
      label: string,
      opts?: RTCDataChannelInit,
    ) {
      const dc = origCreateChannel.call(this, label, opts);
      if (label === 'ctl') ctlChannel = dc;
      return dc;
    };

    const askHostInfo = () =>
      new Promise<boolean>((resolveInfo, rejectInfo) => {
        if (!ctlChannel || ctlChannel.readyState !== 'open') {
          rejectInfo(new Error('ctl channel is not open'));
          return;
        }
        const reqId = 900_001;
        const onMsg = (ev: MessageEvent) => {
          const msg = JSON.parse(String(ev.data)) as { re?: number; ok?: boolean; t?: string };
          if (msg.re === reqId) {
            ctlChannel?.removeEventListener('message', onMsg);
            resolveInfo(msg.ok === true && msg.t === 'host.info');
          }
        };
        ctlChannel.addEventListener('message', onMsg);
        ctlChannel.send(JSON.stringify({ id: reqId, t: 'host.info' }));
      });

    Object.assign(window, {
      __a111: { iceCalls: () => [...iceCalls], fromFrames: () => fromFrames, askHostInfo },
    });
  });
}

async function seedDashPrefixedDeviceKey(seedPage: Page): Promise<string> {
  return seedPage.evaluate(async () => {
    const toBase64Url = (bytes: Uint8Array) =>
      btoa(String.fromCharCode(...bytes))
        .replace(/\+/g, '-')
        .replace(/\//g, '_')
        .replace(/=+$/, '');
    const pickDashPair = async () => {
      for (;;) {
        const pair = await crypto.subtle.generateKey({ name: 'Ed25519' }, false, [
          'sign',
          'verify',
        ]);
        const raw = new Uint8Array(await crypto.subtle.exportKey('raw', pair.publicKey));
        const id = toBase64Url(raw);
        if (id.startsWith('-')) return { pair, id };
      }
    };
    const saveKey = (pair: CryptoKeyPair) =>
      new Promise<void>((resolveTx, rejectTx) => {
        const openReq = indexedDB.open('solodawn', 1);
        openReq.onupgradeneeded = () => openReq.result.createObjectStore('device');
        openReq.onerror = () => rejectTx(openReq.error);
        openReq.onsuccess = () => {
          const db = openReq.result;
          const tx = db.transaction('device', 'readwrite');
          tx.objectStore('device').put(pair, 'key');
          tx.oncomplete = () => {
            db.close();
            resolveTx();
          };
          tx.onabort = () => {
            db.close();
            rejectTx(tx.error);
          };
        };
      });
    const { pair, id } = await pickDashPair();
    await saveKey(pair);
    return id;
  });
}

async function openTrackedAppTab(
  tabPage: Page,
  baseUrl: string,
  signalUrl: string,
  expectedDevice: string,
) {
  await tabPage.routeWebSocket(
    (url) => !url.href.startsWith(signalUrl),
    (ws) => ws.close(),
  );
  await tabPage.addInitScript((wantDevice: string) => {
    const sockets: WebSocket[] = [];
    const pcs: RTCPeerConnection[] = [];
    let ctlChannel: RTCDataChannel | undefined;
    let ctlCloses = 0;
    let nextReqId = 950_000;
    const OrigWS = window.WebSocket;
    window.WebSocket = class extends OrigWS {
      constructor(url: string | URL, protocols?: string | string[]) {
        super(url, protocols);
        sockets.push(this);
      }
    };
    const OrigPC = window.RTCPeerConnection;
    window.RTCPeerConnection = class extends OrigPC {
      constructor(cfg?: RTCConfiguration) {
        super(cfg);
        pcs.push(this);
      }
    };
    const origCreateChannel = RTCPeerConnection.prototype.createDataChannel;
    RTCPeerConnection.prototype.createDataChannel = function (
      label: string,
      opts?: RTCDataChannelInit,
    ) {
      const dc = origCreateChannel.call(this, label, opts);
      if (label === 'ctl') {
        dc.addEventListener('open', () => {
          ctlChannel = dc;
        });
        dc.addEventListener('close', () => {
          if (ctlChannel === dc) ctlChannel = undefined;
          ctlCloses += 1;
        });
      }
      return dc;
    };
    const hostInfo = () =>
      new Promise<boolean>((resolveInfo) => {
        if (!ctlChannel || ctlChannel.readyState !== 'open') {
          resolveInfo(false);
          return;
        }
        const reqId = (nextReqId += 1);
        const onMsg = (ev: MessageEvent) => {
          const msg = JSON.parse(String(ev.data)) as {
            re?: number;
            ok?: boolean;
            t?: string;
            device?: string;
          };
          if (msg.re === reqId) {
            ctlChannel?.removeEventListener('message', onMsg);
            resolveInfo(msg.ok === true && msg.t === 'host.info' && msg.device === wantDevice);
          }
        };
        ctlChannel.addEventListener('message', onMsg);
        ctlChannel.send(JSON.stringify({ id: reqId, t: 'host.info' }));
      });
    Object.assign(window, {
      __netTrack: {
        wsCount: () => sockets.length,
        pcCount: () => pcs.length,
        offlineCount: () => ctlCloses,
        allClosed: () =>
          sockets.every((s) => s.readyState === WebSocket.CLOSED) &&
          pcs.every((p) => p.connectionState === 'closed'),
        hostInfo,
      },
    });
  }, expectedDevice);
  await tabPage.goto(`${baseUrl}#/nodes`);
}

describe('web client connections against real nodes (I-8)', () => {
  let browser: Browser;
  let workDir = '';
  let turnBin = '';
  let stunServer: { url: string; close(): Promise<void> } | undefined;
  let net: Net | undefined;
  let contexts: BrowserContext[] = [];
  let proxies: SignallingProxy[] = [];
  let turns: TurnProc[] = [];

  beforeAll(async () => {
    const { root, bin } = inject('e2e');
    const engine = inject('browser');
    workDir = await mkdtemp(join(root, 'i8-'));
    const srcDir = join(workDir, 'src');
    await mkdir(srcDir);
    const goFile = join(srcDir, 'main.go');
    turnBin = join(bin, `turn-relay-${engine}`);
    await writeFile(goFile, turnSource);
    const [, stun] = await Promise.all([
      promisify(execFile)('go', ['build', '-o', turnBin, goFile], { cwd: repoDir }),
      startStunResponder(engine === 'firefox' ? pickLanIPv4() : '127.0.0.1'),
    ]);
    stunServer = stun;
    browser = await launch(engine);
  });

  beforeEach(() => {
    net = undefined;
    contexts = [];
    proxies = [];
    turns = [];
  });

  afterEach(async () => {
    await Promise.allSettled(contexts.map((ctx) => ctx.close()));
    await Promise.allSettled(proxies.map((proxy) => proxy.close()));
    await Promise.allSettled(turns.map((turn) => turn.stop()));
    await net?.stop();
  });

  afterAll(async () => {
    await browser?.close();
    await stunServer?.close();
    if (workDir) await rm(workDir, { recursive: true, force: true });
  });

  async function newContext(): Promise<BrowserContext> {
    const ctx = await browser.newContext({ locale: 'en-US' });
    contexts.push(ctx);
    return ctx;
  }

  async function startDirectProxy(rv: Net['rendezvous'], reorderEarly = false) {
    const stunUrl = stunServer ? stunServer.url : '';
    const proxy = await startSignallingProxy({
      targetHttp: rv.url,
      targetWs: rv.signal,
      ice: [{ urls: [stunUrl] }],
      reorderEarly,
    });
    proxies.push(proxy);
    return proxy;
  }

  test('smoke: onboarding pairs a real node, verifies its fingerprint and declares CLI accounts', async () => {
    net = await startNet({ nodes: 1 });
    const [node] = net.nodes;
    const proxy = await startDirectProxy(net.rendezvous);
    const ctx = await newContext();
    const page = await ctx.newPage();

    await page.goto(`${proxy.url}#/onboarding`);
    const code = await waitForPairingCode(page, proxy.signal);
    const prompt = (await page.locator('#prompt').textContent()) ?? '';
    expect(prompt).toContain(`${proxy.url}install.md`);
    expect(prompt).toContain('git config --global user.email');

    const paired = await node.run(
      'solodawn',
      'pair',
      code,
      '--name',
      'workstation',
      '--rendezvous',
      proxy.signal,
    );
    expect(paired.code, paired.stderr).toBe(0);

    const status = await node.run('solodawn', 'status');
    expect(status.code, status.stderr).toBe(0);
    const fpMatch = fpPattern.exec(status.stdout);
    expect(fpMatch).not.toBeNull();
    const nodeFingerprint = fpMatch ? fpMatch[0] : '';

    await page.waitForFunction(() => {
      const steps = document.querySelector('.link-steps')?.textContent ?? '';
      return (
        steps.includes('Direct connection established') &&
        steps.includes('Found: Claude Code, Codex')
      );
    });

    expect((await page.locator('.link-steps code').textContent())?.trim()).toBe(nodeFingerprint);
    const stepsText = (await page.locator('.link-steps').textContent()) ?? '';
    expect(stepsText).toContain('workstation');
    expect(stepsText).toContain('dev');

    await page.locator('input[name="acct-claude"][value="personal"]').check();
    await page.waitForFunction(
      () => !(document.querySelector('fieldset.acct') as HTMLFieldSetElement | null)?.disabled,
    );
    await page.locator('input[name="acct-codex"][value="api_key"]').check();
    await page.locator('.done-btn').waitFor({ state: 'visible' });

    expect(((await page.locator('.done-btn').textContent()) ?? '').trim()).toBe(
      'Create your first group',
    );
    expect(await page.locator('.done-btn').getAttribute('href')).toBe('#/groups');
  });

  test('A1.3: indicator reports direct and relay within 1 s and host.info succeeds throughout', async () => {
    net = await startNet({ nodes: 2 });
    const [node, relayNode] = net.nodes;
    const [nodeId, relayNodeId] = await Promise.all([
      readNodeDeviceId(node),
      readNodeDeviceId(relayNode),
    ]);
    const engine = inject('browser');
    const relayHost = engine === 'firefox' ? pickLanIPv4() : '127.0.0.1';
    const turn = await startTurnRelay(
      turnBin,
      relayHost,
      net.dir,
      join(inject('e2e').root, 'pids'),
    );
    turns.push(turn);
    const turnEntry: IceServer = {
      urls: [turn.url],
      username: 'peer',
      credential: 'secret',
    };

    const directProxy = await startDirectProxy(net.rendezvous);

    const ctx = await newContext();
    const pairPage = await ctx.newPage();
    await pairBrowserOnPage(pairPage, directProxy.url, directProxy.signal, node);
    await pairPage.close();

    const relayProxy = await startSignallingProxy({
      targetHttp: net.rendezvous.url,
      targetWs: net.rendezvous.signal,
      ice: [turnEntry],
      dropDirect: true,
    });
    proxies.push(relayProxy);

    const directResult = await measureIndicator(
      ctx,
      directProxy.url,
      directProxy.signal,
      'ok',
      nodeId,
    );
    expect(directResult.indicatorMs).toBeGreaterThanOrEqual(0);
    expect(directResult.indicatorMs).toBeLessThanOrEqual(1000);
    expect(directResult.text).toMatch(/^Direct( \d+ ms)?$/);
    expect(directResult.label).toMatch(/^Connection status: direct(, \d+ ms)?$/);
    expect(directResult.samples).toStrictEqual([
      { ok: true, kind: 'direct' },
      { ok: true, kind: 'direct' },
      { ok: true, kind: 'direct' },
    ]);

    // Pair a second browser over relayProxy (where direct candidates are dropped in transit and
    // only TURN relay candidates cross) and verify Onboarding's relay status line.
    const relayOnboardCtx = await newContext();
    const relayOnboardPage = await relayOnboardCtx.newPage();
    await relayOnboardPage.goto(`${relayProxy.url}#/onboarding`);
    const relayCode = await waitForPairingCode(relayOnboardPage, relayProxy.signal);
    const relayPaired = await relayNode.run(
      'solodawn',
      'pair',
      relayCode,
      '--name',
      'relayed-box',
      '--rendezvous',
      relayProxy.signal,
    );
    expect(relayPaired.code, relayPaired.stderr).toBe(0);

    await relayOnboardPage.waitForFunction(() => {
      const steps = document.querySelector('.link-steps')?.textContent ?? '';
      return (
        steps.includes('Connected through encrypted relay') &&
        steps.includes('Found: Claude Code, Codex')
      );
    });
    await relayOnboardPage.close();

    const relayResult = await measureIndicator(
      relayOnboardCtx,
      relayProxy.url,
      relayProxy.signal,
      'limited',
      relayNodeId,
    );
    expect(relayResult.indicatorMs).toBeGreaterThanOrEqual(0);
    expect(relayResult.indicatorMs).toBeLessThanOrEqual(1000);
    expect(relayResult.text).toBe('Relayed · limited');
    expect(relayResult.label).toBe('Connection status: relayed · limited');
    expect(relayResult.samples).toStrictEqual([
      { ok: true, kind: 'relay' },
      { ok: true, kind: 'relay' },
      { ok: true, kind: 'relay' },
    ]);
  });

  test('A1.11: early and duplicate candidates are buffered until remote description and applied once', async () => {
    net = await startNet({ nodes: 1 });
    const [node] = net.nodes;
    const proxy = await startDirectProxy(net.rendezvous, true);

    const ctx = await newContext();
    await installEarlyCandidateProbe(ctx);

    const page = await ctx.newPage();
    await page.goto(`${proxy.url}#/onboarding`);
    const code = await waitForPairingCode(page, proxy.signal);

    const pairPromise = node.run(
      'solodawn',
      'pair',
      code,
      '--name',
      'workstation',
      '--rendezvous',
      proxy.signal,
    );

    const { nodeIce } = await proxy.earlyReady;
    const targetIce = parseSignallingEnvelope(nodeIce).inner;
    expect(targetIce).toBeDefined();

    await page.waitForFunction(() => (window as Record<string, any>).__a111.fromFrames() >= 2);
    proxy.releaseAnswer();

    const paired = await pairPromise;
    expect(paired.code, paired.stderr).toBe(0);

    await page.waitForFunction(() =>
      (document.querySelector('.link-steps')?.textContent ?? '').includes(
        'Found: Claude Code, Codex',
      ),
    );

    const afterAnswer = await page.evaluate(
      () =>
        (window as Record<string, any>).__a111.iceCalls() as {
          candidate: string;
          sdpMid: string | null | undefined;
          hadRemote: boolean;
        }[],
    );
    expect(afterAnswer.every((c) => c.hadRemote)).toBe(true);
    expect(
      afterAnswer.filter((c) => c.candidate === targetIce?.cand && c.sdpMid === targetIce?.mid),
    ).toHaveLength(1);

    const seenBeforeReplay = await page.evaluate(() =>
      (window as Record<string, any>).__a111.fromFrames(),
    );
    proxy.replayEarly();
    await page.waitForFunction(
      (prev: number) => (window as Record<string, any>).__a111.fromFrames() >= prev + 1,
      seenBeforeReplay,
    );

    const infoOk = await page.evaluate(() => (window as Record<string, any>).__a111.askHostInfo());
    expect(infoOk).toBe(true);

    const afterReplay = await page.evaluate(
      () =>
        (window as Record<string, any>).__a111.iceCalls() as {
          candidate: string;
          sdpMid: string | null | undefined;
          hadRemote: boolean;
        }[],
    );
    expect(
      afterReplay.filter((c) => c.candidate === targetIce?.cand && c.sdpMid === targetIce?.mid),
    ).toHaveLength(1);
  });

  test('A1.12: second tab with "-" device id stays idle until Use here, then closes first tab without reconnect', async () => {
    net = await startNet({ nodes: 1 });
    const [node] = net.nodes;
    const nodeId = await readNodeDeviceId(node);
    const proxy = await startDirectProxy(net.rendezvous);
    const ctx = await newContext();

    // Seed IndexedDB with a valid non-extractable Ed25519 device key whose canonical id starts with "-".
    const seedPage = await ctx.newPage();
    await seedPage.goto(`${proxy.url}install.md`);
    const seededDeviceId = await seedDashPrefixedDeviceKey(seedPage);
    expect(seededDeviceId.startsWith('-')).toBe(true);

    await pairBrowserOnPage(seedPage, proxy.url, proxy.signal, node);
    await seedPage.close();

    const tab1 = await ctx.newPage();
    await openTrackedAppTab(tab1, proxy.url, proxy.signal, nodeId);
    await poll(() => tab1.locator('.bar a.conn').getAttribute('data-st')).toBe('ok');
    await poll(() => tab1.locator('section.card.node header .st').textContent()).toContain(
      'Online',
    );
    expect(await tab1.evaluate(() => (window as Record<string, any>).__netTrack.hostInfo())).toBe(
      true,
    );

    const tab2 = await ctx.newPage();
    await openTrackedAppTab(tab2, proxy.url, proxy.signal, nodeId);
    await poll(() => tab2.locator('.tab-ban').textContent()).toContain(
      'SoloDawn is open in another tab.',
    );

    expect(await tab2.evaluate(() => (window as Record<string, any>).__netTrack.wsCount())).toBe(0);
    expect(await tab2.evaluate(() => (window as Record<string, any>).__netTrack.pcCount())).toBe(0);
    const heldLocks = await tab2.evaluate(async () => (await navigator.locks.query()).held ?? []);
    expect(heldLocks).toStrictEqual([
      expect.objectContaining({
        name: `solodawn-device-${seededDeviceId}`,
        mode: 'exclusive',
      }),
    ]);
    expect(await tab1.evaluate(() => (window as Record<string, any>).__netTrack.hostInfo())).toBe(
      true,
    );

    // Choose "Use here" in the second tab: it steals the lock and connects, while the first tab
    // closes its signalling and peer connections and renders the other-tab banner.
    await tab2.locator('.tab-ban button', { hasText: 'Use here' }).click();
    await poll(() => tab2.locator('.bar a.conn').getAttribute('data-st')).toBe('ok');
    await poll(() => tab2.locator('.tab-ban').count()).toBe(0);
    await poll(() => tab1.locator('.tab-ban').textContent()).toContain(
      'SoloDawn is open in another tab.',
    );
    await tab1.waitForFunction(
      () => (window as Record<string, any>).__netTrack.allClosed() === true,
    );
    await poll(() =>
      tab1.evaluate(() => (window as Record<string, any>).__netTrack.offlineCount()),
    ).toBe(1);
    expect(await tab2.evaluate(() => (window as Record<string, any>).__netTrack.hostInfo())).toBe(
      true,
    );

    const wsBeforeTrigger = await tab1.evaluate(() =>
      (window as Record<string, any>).__netTrack.wsCount(),
    );
    const pcBeforeTrigger = await tab1.evaluate(() =>
      (window as Record<string, any>).__netTrack.pcCount(),
    );

    // Trigger browser reconnect event in the first tab and wait past peer.ts's 1 s initial retry window.
    await tab1.evaluate(() => window.dispatchEvent(new Event('online')));
    await delay(pastInitialReconnectMs);

    expect(await tab1.evaluate(() => (window as Record<string, any>).__netTrack.wsCount())).toBe(
      wsBeforeTrigger,
    );
    expect(await tab1.evaluate(() => (window as Record<string, any>).__netTrack.pcCount())).toBe(
      pcBeforeTrigger,
    );
    await poll(() => tab1.locator('.tab-ban').textContent()).toContain(
      'SoloDawn is open in another tab.',
    );
    expect(await tab2.evaluate(() => (window as Record<string, any>).__netTrack.hostInfo())).toBe(
      true,
    );
  });
});
