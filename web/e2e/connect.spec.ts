// Browser end-to-end connection scenarios for unit I-8 against real solodawn rendezvous and
// solodawn node processes across Chromium, Firefox and WebKit: onboarding pairing smoke, A1.3
// (direct and relayed path indicators plus periodic host.info without probe migration), A1.11
// (early and duplicate ICE candidates buffered until setRemoteDescription) and A1.12 (single
// active tab lock with a "-" device id, takeover and suppressed reconnect).
import { execFile, spawn } from 'node:child_process';
import { createHash } from 'node:crypto';
import { createSocket, type Socket as UdpSocket } from 'node:dgram';
import { appendFileSync } from 'node:fs';
import { mkdir, mkdtemp, rm, writeFile } from 'node:fs/promises';
import { createServer, type IncomingMessage, type Server } from 'node:http';
import type { AddressInfo, Socket } from 'node:net';
import { networkInterfaces } from 'node:os';
import { join, resolve } from 'node:path';
import { setTimeout as delay } from 'node:timers/promises';
import { promisify } from 'node:util';
import type { Browser, BrowserContext, Page } from 'playwright';
import { afterAll, afterEach, beforeAll, beforeEach, describe, expect, inject, test } from 'vitest';
import { launch } from './config';
import { alive, startNet, type Net, type Node } from './nodes';

const webDir = resolve(import.meta.dirname, '..');
const repoDir = resolve(webDir, '..');
const wsGuid = '258EAFA5-E914-47DA-95CA-C5AB0DC85B11';
const codePattern = /([0-9A-HJKMNP-TV-Z]{4}-){3}[0-9A-HJKMNP-TV-Z]{4}/;
const fpPattern = /[0-9A-F]{4} [0-9A-F]{4} [0-9A-F]{4}/;

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

interface IceServer {
  urls: string[];
  username?: string;
  credential?: string;
}

interface ProxyOptions {
  targetHttp: string;
  targetWs: string;
  ice?: IceServer[];
  dropDirect?: boolean;
  reorderEarly?: boolean;
}

interface ProxyHandle {
  url: string;
  signal: string;
  earlyReady: Promise<{ browserIce: string; nodeIce: string }>;
  releaseAnswer(): void;
  replayEarly(): void;
  close(): Promise<void>;
}

interface TurnProc {
  url: string;
  pid: number;
  stop(): Promise<void>;
}

/** Picks a non-loopback IPv4 interface address for Firefox TURN tests, or 127.0.0.1. */
function pickLanIPv4(): string {
  for (const list of Object.values(networkInterfaces())) {
    for (const entry of list ?? []) {
      if (entry.family === 'IPv4' && !entry.internal) return entry.address;
    }
  }
  return '127.0.0.1';
}

/** Encodes an unmasked RFC 6455 text frame from the local proxy server to its client. */
function encodeWsText(text: string): Buffer {
  const payload = Buffer.from(text, 'utf8');
  if (payload.length < 126) {
    return Buffer.concat([Buffer.from([0x81, payload.length]), payload]);
  }
  const header = Buffer.alloc(4);
  header[0] = 0x81;
  header[1] = 126;
  header.writeUInt16BE(payload.length, 2);
  return Buffer.concat([header, payload]);
}

/** Attaches an RFC 6455 reader for masked client frames on a raw upgraded TCP socket. */
function readWsClient(
  socket: Socket,
  initial: Buffer,
  onText: (text: string) => void,
  onClose: () => void,
) {
  let buf = Buffer.from(initial);
  let parts: Buffer[] = [];
  const parse = () => {
    while (buf.length >= 2) {
      const fin = (buf[0] & 0x80) !== 0;
      const op = buf[0] & 0x0f;
      const masked = (buf[1] & 0x80) !== 0;
      let len = buf[1] & 0x7f;
      let offset = 2;
      if (len === 126) {
        if (buf.length < 4) return;
        len = buf.readUInt16BE(2);
        offset = 4;
      } else if (len === 127) {
        if (buf.length < 10) return;
        len = Number(buf.readBigUInt64BE(2));
        offset = 10;
      }
      const maskBytes = masked ? 4 : 0;
      if (buf.length < offset + maskBytes + len) return;
      const body = Buffer.from(buf.subarray(offset + maskBytes, offset + maskBytes + len));
      if (masked) {
        for (let i = 0; i < len; i++) body[i] ^= buf[offset + (i & 3)];
      }
      buf = buf.subarray(offset + maskBytes + len);
      if (op === 0x8) {
        onClose();
        return;
      }
      if (op === 0x9) {
        socket.write(Buffer.concat([Buffer.from([0x8a, body.length]), body]));
        continue;
      }
      if (op === 0x1 || op === 0x0) {
        parts.push(body);
        if (fin) {
          const full = Buffer.concat(parts).toString('utf8');
          parts = [];
          onText(full);
        }
      }
    }
  };
  socket.on('data', (chunk: Buffer) => {
    buf = Buffer.concat([buf, chunk]);
    parse();
  });
  socket.on('close', onClose);
  socket.on('error', onClose);
  parse();
}

function parseEnvelopePayload(rawFrame: string): {
  outer: { t?: string; k?: string; env?: { p?: string; k?: string } };
  inner?: { t?: string; cand?: string; mid?: string; conn?: string };
} {
  const outer = JSON.parse(rawFrame) as {
    t?: string;
    k?: string;
    env?: { p?: string; k?: string };
  };
  if ((outer.t === 'to' || outer.t === 'from') && typeof outer.env?.p === 'string') {
    return {
      outer,
      inner: JSON.parse(outer.env.p) as { t?: string; cand?: string; mid?: string; conn?: string },
    };
  }
  return { outer };
}

function isNonRelayIce(rawFrame: string): boolean {
  const { inner } = parseEnvelopePayload(rawFrame);
  return inner?.t === 'ice' && !(inner.cand ?? '').includes(' typ relay');
}

/** Starts an RFC 5389 STUN Binding responder so srflx candidates gather over loopback or LAN. */
async function startStunServer(host: string): Promise<{ url: string; close(): Promise<void> }> {
  const sock: UdpSocket = createSocket('udp4');
  sock.on('message', (msg, rinfo) => {
    if (msg.length < 20 || msg.readUInt16BE(0) !== 0x0001) return;
    const cookie = 0x2112a442;
    const attr = Buffer.alloc(12);
    attr.writeUInt16BE(0x0020, 0);
    attr.writeUInt16BE(8, 2);
    attr[4] = 0;
    attr[5] = 0x01;
    attr.writeUInt16BE(rinfo.port ^ (cookie >>> 16), 6);
    const octets = rinfo.address.split('.').map(Number);
    const ipv4 = ((octets[0] << 24) | (octets[1] << 16) | (octets[2] << 8) | octets[3]) >>> 0;
    attr.writeUInt32BE((ipv4 ^ cookie) >>> 0, 8);
    const hdr = Buffer.alloc(20);
    hdr.writeUInt16BE(0x0101, 0);
    hdr.writeUInt16BE(attr.length, 2);
    msg.copy(hdr, 4, 4, 20);
    sock.send(Buffer.concat([hdr, attr]), rinfo.port, rinfo.address);
  });
  await new Promise<void>((resolveBind, rejectBind) => {
    sock.once('error', rejectBind);
    sock.bind(0, host, () => resolveBind());
  });
  const { port } = sock.address();
  return {
    url: `stun:${host}:${port}`,
    close: () => new Promise<void>((done) => sock.close(() => done())),
  };
}

function injectIceServers(rawFrame: string, extra: IceServer[] | undefined): string {
  if (!extra?.length) return rawFrame;
  const parsed = JSON.parse(rawFrame) as { t?: string; ice?: IceServer[]; ttl?: number };
  if (parsed.t !== 'ready' && parsed.t !== 'ice') return rawFrame;
  return JSON.stringify({ ...parsed, ice: [...(parsed.ice ?? []), ...extra] });
}

/** Starts a local HTTP and WebSocket proxy in front of a real solodawn rendezvous server. */
async function startProxy(opts: ProxyOptions): Promise<ProxyHandle> {
  const sockets = new Set<Socket>();
  const upstreams = new Set<WebSocket>();
  const readyGate = Promise.withResolvers<{ browserIce: string; nodeIce: string }>();
  const answerGate = Promise.withResolvers<void>();

  let browserUpstream: WebSocket | undefined;
  let browserSendDown: ((text: string) => void) | undefined;
  let firstBrowserIce = '';
  let firstNodeIce = '';
  let browserFlushed = false;
  let nodeEarlySent = false;
  let nodeHeld: string[] = [];
  let browserOffer = '';
  let browserHeldIce: string[] = [];
  let answerReleased = false;

  const forwardHttp = async (req: IncomingMessage) => {
    const target = new URL(req.url ?? '/', opts.targetHttp);
    return fetch(target, { method: req.method });
  };

  const server: Server = createServer(async (req, res) => {
    try {
      const upstream = await forwardHttp(req);
      const headers: Record<string, string> = {};
      upstream.headers.forEach((value, key) => {
        if (key !== 'transfer-encoding' && key !== 'content-length') headers[key] = value;
      });
      const body = Buffer.from(await upstream.arrayBuffer());
      res.writeHead(upstream.status, headers);
      res.end(body);
    } catch {
      res.writeHead(502);
      res.end();
    }
  });

  server.on('connection', (socket) => {
    sockets.add(socket);
    socket.on('close', () => sockets.delete(socket));
  });

  server.on('upgrade', (req, socket: Socket, head: Buffer) => {
    const key = req.headers['sec-websocket-key'];
    if (typeof key !== 'string') {
      socket.destroy();
      return;
    }
    const isBrowserConn = Boolean(req.headers.origin);
    const accept = createHash('sha1')
      .update(key + wsGuid)
      .digest('base64');
    socket.write(
      'HTTP/1.1 101 Switching Protocols\r\n' +
        'Upgrade: websocket\r\n' +
        'Connection: Upgrade\r\n' +
        `Sec-WebSocket-Accept: ${accept}\r\n\r\n`,
    );
    sockets.add(socket);

    const upstream = new WebSocket(opts.targetWs);
    upstreams.add(upstream);
    const pendingUp: string[] = [];
    let closed = false;

    const sendDown = (text: string) => {
      if (!closed && !socket.destroyed) socket.write(encodeWsText(text));
    };
    const sendUp = (text: string) => {
      if (closed) return;
      if (upstream.readyState === WebSocket.OPEN) upstream.send(text);
      else if (upstream.readyState === WebSocket.CONNECTING) pendingUp.push(text);
    };
    const shut = () => {
      if (closed) return;
      closed = true;
      upstreams.delete(upstream);
      sockets.delete(socket);
      try {
        upstream.close();
      } catch {
        // already closing
      }
      if (!socket.destroyed) socket.destroy();
    };

    if (isBrowserConn) {
      browserUpstream = upstream;
      browserSendDown = sendDown;
    }

    upstream.addEventListener('open', () => {
      while (pendingUp.length > 0 && upstream.readyState === WebSocket.OPEN) {
        upstream.send(pendingUp.shift()!);
      }
    });

    upstream.addEventListener('message', (ev) => {
      const raw = injectIceServers(String(ev.data), opts.ice);
      if (opts.dropDirect && isNonRelayIce(raw)) return;
      if (opts.reorderEarly && isBrowserConn && !answerReleased) {
        const { outer, inner } = parseEnvelopePayload(raw);
        if (outer.t === 'from' && (inner?.t === 'answer' || inner?.t === 'ice')) {
          if (inner.t === 'ice' && !firstNodeIce) firstNodeIce = raw;
          nodeHeld.push(raw);
          const hasAnswer = nodeHeld.some((f) => parseEnvelopePayload(f).inner?.t === 'answer');
          if (hasAnswer && firstNodeIce && !nodeEarlySent) {
            nodeEarlySent = true;
            sendDown(firstNodeIce);
            sendDown(firstNodeIce);
            readyGate.resolve({ browserIce: firstBrowserIce, nodeIce: firstNodeIce });
            void answerGate.promise.then(() => {
              answerReleased = true;
              for (const held of nodeHeld) sendDown(held);
              nodeHeld = [];
            });
          }
          return;
        }
      }
      sendDown(raw);
    });

    upstream.addEventListener('close', shut);
    upstream.addEventListener('error', shut);

    const maybeFlushBrowserEarly = () => {
      if (browserFlushed || !browserOffer || !firstBrowserIce) return;
      browserFlushed = true;
      sendUp(firstBrowserIce);
      sendUp(firstBrowserIce);
      void delay(60).then(() => {
        sendUp(browserOffer);
        for (const item of browserHeldIce) sendUp(item);
        browserHeldIce = [];
      });
    };

    readWsClient(
      socket,
      head,
      (raw) => {
        if (opts.dropDirect && isNonRelayIce(raw)) return;
        if (opts.reorderEarly && isBrowserConn && !browserFlushed) {
          const { outer, inner } = parseEnvelopePayload(raw);
          if (outer.t === 'to' && inner?.t === 'offer') {
            browserOffer = raw;
            maybeFlushBrowserEarly();
            return;
          }
          if (outer.t === 'to' && inner?.t === 'ice') {
            if (!firstBrowserIce) firstBrowserIce = raw;
            browserHeldIce.push(raw);
            maybeFlushBrowserEarly();
            return;
          }
        }
        sendUp(raw);
      },
      shut,
    );
  });

  await new Promise<void>((resolveListen, rejectListen) => {
    server.once('error', rejectListen);
    server.listen(0, '127.0.0.1', () => resolveListen());
  });
  const { port } = server.address() as AddressInfo;

  return {
    url: `http://127.0.0.1:${port}/`,
    signal: `ws://127.0.0.1:${port}/rv`,
    earlyReady: readyGate.promise,
    releaseAnswer: () => answerGate.resolve(),
    replayEarly: () => {
      if (firstBrowserIce && browserUpstream?.readyState === WebSocket.OPEN) {
        browserUpstream.send(firstBrowserIce);
      }
      if (firstNodeIce && browserSendDown) {
        browserSendDown(firstNodeIce);
      }
    },
    close: async () => {
      for (const ws of upstreams) {
        try {
          ws.close();
        } catch {
          // ignore
        }
      }
      for (const sock of sockets) sock.destroy();
      await new Promise<void>((done) => server.close(() => done()));
    },
  };
}

/** Starts the Pion relay-only TURN server in its own process group and records its pgid. */
async function startTurnRelay(binPath: string, host: string, dir: string, pidRecord: string) {
  const child = spawn(binPath, [host], { cwd: dir, detached: true });
  child.stdin.end();
  const pid = child.pid;
  if (pid === undefined) throw new Error('cannot start turn-relay');
  appendFileSync(pidRecord, `${pid}\n`);
  const closed = new Promise<void>((done) => {
    child.on('close', () => done());
    child.on('error', () => done());
  });
  const url = await new Promise<string>((resolveUrl, rejectUrl) => {
    let text = '';
    child.stdout.on('data', (chunk: Buffer) => {
      text += chunk.toString('utf8');
      const nl = text.indexOf('\n');
      if (nl !== -1) resolveUrl(text.slice(0, nl).trim());
    });
    child.on('close', () => rejectUrl(new Error('turn-relay exited before printing URL')));
  });
  const stop = async () => {
    try {
      process.kill(-pid, 'SIGTERM');
    } catch {
      // already gone
    }
    await Promise.race([closed, delay(3000)]);
    try {
      process.kill(-pid, 'SIGKILL');
    } catch {
      // already gone
    }
    await closed;
    for (const end = Date.now() + 5000; alive(pid); await delay(25)) {
      if (Date.now() > end) throw new Error('turn-relay process group did not stop');
    }
  };
  return { url, pid, stop } satisfies TurnProc;
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
  return match![0];
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

describe('web client connections against real nodes (I-8)', () => {
  let browser: Browser;
  let workDir = '';
  let turnBin = '';
  let stunServer: { url: string; close(): Promise<void> } | undefined;
  let net: Net | undefined;
  let contexts: BrowserContext[] = [];
  let proxies: ProxyHandle[] = [];
  let turns: TurnProc[] = [];

  beforeAll(async () => {
    const { root } = inject('e2e');
    const engine = inject('browser');
    workDir = await mkdtemp(join(root, 'i8-'));
    const srcDir = join(workDir, 'src');
    await mkdir(srcDir);
    const goFile = join(srcDir, 'main.go');
    turnBin = join(workDir, 'turn-relay');
    await writeFile(goFile, turnSource);
    const [, stun] = await Promise.all([
      promisify(execFile)('go', ['build', '-o', turnBin, goFile], { cwd: repoDir }),
      startStunServer(engine === 'firefox' ? pickLanIPv4() : '127.0.0.1'),
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
    for (const ctx of contexts) await ctx.close().catch(() => {});
    for (const proxy of proxies) await proxy.close().catch(() => {});
    for (const turn of turns) await turn.stop().catch(() => {});
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
    const proxy = await startProxy({
      targetHttp: rv.url,
      targetWs: rv.signal,
      ice: [{ urls: [stunServer!.url] }],
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
    const nodeFingerprint = fpMatch![0];

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
    net = await startNet({ nodes: 1 });
    const [node] = net.nodes;
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

    const relayProxy = await startProxy({
      targetHttp: net.rendezvous.url,
      targetWs: net.rendezvous.signal,
      ice: [turnEntry],
      dropDirect: true,
    });
    proxies.push(relayProxy);

    const measureIndicator = async (
      targetCtx: BrowserContext,
      baseUrl: string,
      signalUrl: string,
      expectedState: 'ok' | 'limited',
    ) => {
      const checkPage = await targetCtx.newPage();
      await checkPage.routeWebSocket(
        (url) => !url.href.startsWith(signalUrl),
        (ws) => ws.close(),
      );
      await checkPage.addInitScript(() => {
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
        Object.assign(window, {
          __a13: {
            openedAt: () => ctlOpenedAt,
            askHostInfo: () =>
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
                  };
                  if (msg.re === reqId) {
                    ctlChannel!.removeEventListener('message', onMsg);
                    resolveInfo(msg.ok === true && msg.t === 'host.info');
                  }
                };
                ctlChannel.addEventListener('message', onMsg);
                ctlChannel.send(JSON.stringify({ id: reqId, t: 'host.info' }));
              }),
          },
        });
      });
      await checkPage.goto(`${baseUrl}#/nodes`);
      try {
        return await checkPage.evaluate(async (wantSt) => {
          const hook = (
            window as unknown as { __a13: { openedAt(): number; askHostInfo(): Promise<boolean> } }
          ).__a13;
          for (const deadline = performance.now() + 15_000; hook.openedAt() === 0;) {
            if (performance.now() >= deadline) throw new Error('ctl did not open');
            await new Promise((r) => setTimeout(r, 15));
          }
          const openedAt = hook.openedAt();
          let indicatorMs = -1;
          for (const deadline = performance.now() + 2000; performance.now() < deadline;) {
            const el = document.querySelector<HTMLElement>('.bar a.conn');
            if (el?.dataset.st === wantSt) {
              indicatorMs = Math.max(0, performance.now() - openedAt);
              break;
            }
            await new Promise((r) => setTimeout(r, 15));
          }
          const el = document.querySelector<HTMLElement>('.bar a.conn');
          const text = (el?.textContent ?? '').replace(/\s+/g, ' ').trim();
          const label = el?.getAttribute('aria-label') ?? '';

          const samples: { ok: boolean; kind: string | undefined }[] = [];
          for (let i = 0; i < 3; i++) {
            if (i > 0) await new Promise((r) => setTimeout(r, 1000));
            const ok = await hook.askHostInfo();
            const curSt = document.querySelector<HTMLElement>('.bar a.conn')?.dataset.st;
            const kind = curSt === 'ok' ? 'direct' : curSt === 'limited' ? 'relay' : undefined;
            samples.push({ ok, kind });
          }
          return { indicatorMs, text, label, samples };
        }, expectedState);
      } finally {
        await checkPage.close();
      }
    };

    const directResult = await measureIndicator(ctx, directProxy.url, directProxy.signal, 'ok');
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
    const relayNet = await startNet({ nodes: 1 });
    try {
      const [relayNode] = relayNet.nodes;
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
    } finally {
      await relayNet.stop();
    }
  });

  test('A1.11: early and duplicate candidates are buffered until remote description and applied once', async () => {
    net = await startNet({ nodes: 1 });
    const [node] = net.nodes;
    const proxy = await startDirectProxy(net.rendezvous, true);

    const ctx = await newContext();
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

      Object.assign(window, {
        __a111: {
          iceCalls: () => [...iceCalls],
          fromFrames: () => fromFrames,
          askHostInfo: () =>
            new Promise<boolean>((resolveInfo, rejectInfo) => {
              if (!ctlChannel || ctlChannel.readyState !== 'open') {
                rejectInfo(new Error('ctl channel is not open'));
                return;
              }
              const reqId = 900_001;
              const onMsg = (ev: MessageEvent) => {
                const msg = JSON.parse(String(ev.data)) as {
                  re?: number;
                  ok?: boolean;
                  t?: string;
                };
                if (msg.re === reqId) {
                  ctlChannel!.removeEventListener('message', onMsg);
                  resolveInfo(msg.ok === true && msg.t === 'host.info');
                }
              };
              ctlChannel.addEventListener('message', onMsg);
              ctlChannel.send(JSON.stringify({ id: reqId, t: 'host.info' }));
            }),
        },
      });
    });

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
    const targetIce = parseEnvelopePayload(nodeIce).inner!;

    await page.waitForFunction(() => (window as Record<string, any>).__a111.fromFrames() >= 2);
    await delay(80);

    const beforeAnswer = await page.evaluate(() =>
      (window as Record<string, any>).__a111.iceCalls(),
    );
    expect(beforeAnswer).toStrictEqual([]);

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
      afterAnswer.filter((c) => c.candidate === targetIce.cand && c.sdpMid === targetIce.mid),
    ).toHaveLength(1);

    const seenBeforeReplay = await page.evaluate(() =>
      (window as Record<string, any>).__a111.fromFrames(),
    );
    proxy.replayEarly();
    await page.waitForFunction(
      (prev: number) => (window as Record<string, any>).__a111.fromFrames() >= prev + 1,
      seenBeforeReplay,
    );
    await delay(80);

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
      afterReplay.filter((c) => c.candidate === targetIce.cand && c.sdpMid === targetIce.mid),
    ).toHaveLength(1);
  });

  test('A1.12: second tab with "-" device id stays idle until Use here, then closes first tab without reconnect', async () => {
    net = await startNet({ nodes: 1 });
    const [node] = net.nodes;
    const proxy = await startDirectProxy(net.rendezvous);
    const ctx = await newContext();

    // Seed IndexedDB with a valid non-extractable Ed25519 device key whose canonical id starts with "-".
    const seedPage = await ctx.newPage();
    await seedPage.goto(`${proxy.url}install.md`);
    const seededDeviceId = await seedPage.evaluate(async () => {
      const toBase64Url = (bytes: Uint8Array) =>
        btoa(String.fromCharCode(...bytes))
          .replace(/\+/g, '-')
          .replace(/\//g, '_')
          .replace(/=+$/, '');
      let chosen: CryptoKeyPair | undefined;
      let chosenId = '';
      while (!chosen) {
        const pair = await crypto.subtle.generateKey({ name: 'Ed25519' }, false, [
          'sign',
          'verify',
        ]);
        const raw = new Uint8Array(await crypto.subtle.exportKey('raw', pair.publicKey));
        const id = toBase64Url(raw);
        if (id.startsWith('-')) {
          chosen = pair;
          chosenId = id;
        }
      }
      await new Promise<void>((resolveTx, rejectTx) => {
        const openReq = indexedDB.open('solodawn', 1);
        openReq.onupgradeneeded = () => openReq.result.createObjectStore('device');
        openReq.onerror = () => rejectTx(openReq.error);
        openReq.onsuccess = () => {
          const db = openReq.result;
          const tx = db.transaction('device', 'readwrite');
          tx.objectStore('device').put(chosen, 'key');
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
      return chosenId;
    });
    expect(seededDeviceId.startsWith('-')).toBe(true);

    await pairBrowserOnPage(seedPage, proxy.url, proxy.signal, node);
    await seedPage.close();

    const openAppTab = async (tabPage: Page) => {
      await tabPage.routeWebSocket(
        (url) => !url.href.startsWith(proxy.signal),
        (ws) => ws.close(),
      );
      await tabPage.addInitScript(() => {
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
        Object.assign(window, {
          __netTrack: {
            wsCount: () => sockets.length,
            pcCount: () => pcs.length,
            offlineCount: () => ctlCloses,
            allClosed: () =>
              sockets.every((s) => s.readyState === WebSocket.CLOSED) &&
              pcs.every((p) => p.connectionState === 'closed'),
            hostInfo: () =>
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
                  };
                  if (msg.re === reqId) {
                    ctlChannel!.removeEventListener('message', onMsg);
                    resolveInfo(msg.ok === true && msg.t === 'host.info');
                  }
                };
                ctlChannel.addEventListener('message', onMsg);
                ctlChannel.send(JSON.stringify({ id: reqId, t: 'host.info' }));
              }),
          },
        });
      });
      await tabPage.goto(`${proxy.url}#/nodes`);
    };

    const tab1 = await ctx.newPage();
    await openAppTab(tab1);
    await expect
      .poll(() => tab1.locator('.bar a.conn').getAttribute('data-st'), { timeout: 15_000 })
      .toBe('ok');
    await expect
      .poll(() => tab1.locator('section.card.node header .st').textContent(), { timeout: 15_000 })
      .toContain('Online');
    expect(await tab1.evaluate(() => (window as Record<string, any>).__netTrack.hostInfo())).toBe(
      true,
    );

    const tab2 = await ctx.newPage();
    await openAppTab(tab2);
    await expect
      .poll(() => tab2.locator('.tab-ban').textContent())
      .toContain('SoloDawn is open in another tab.');

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
    await expect.poll(() => tab2.locator('.bar a.conn').getAttribute('data-st')).toBe('ok');
    await expect.poll(() => tab2.locator('.tab-ban').count()).toBe(0);
    await expect
      .poll(() => tab1.locator('.tab-ban').textContent())
      .toContain('SoloDawn is open in another tab.');
    await tab1.waitForFunction(
      () => (window as Record<string, any>).__netTrack.allClosed() === true,
    );
    expect(
      await tab1.evaluate(() => (window as Record<string, any>).__netTrack.offlineCount()),
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

    // Trigger browser reconnect event in the first tab and wait past the 1 s initial retry window.
    await tab1.evaluate(() => window.dispatchEvent(new Event('online')));
    await delay(1400);

    expect(await tab1.evaluate(() => (window as Record<string, any>).__netTrack.wsCount())).toBe(
      wsBeforeTrigger,
    );
    expect(await tab1.evaluate(() => (window as Record<string, any>).__netTrack.pcCount())).toBe(
      pcBeforeTrigger,
    );
    await expect
      .poll(() => tab1.locator('.tab-ban').textContent())
      .toContain('SoloDawn is open in another tab.');
    expect(await tab2.evaluate(() => (window as Record<string, any>).__netTrack.hostInfo())).toBe(
      true,
    );
  });
});
