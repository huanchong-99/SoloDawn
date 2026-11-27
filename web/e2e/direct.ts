// Shared end-to-end helpers for connect.spec.ts, E-M1, E-M2, E-M3, and E-M5: local STUN
// responder, rendezvous HTTP + `/rv` WebSocket proxy (with optional TURN-only filtering and
// downstream early-ICE reordering), node device-id reader, the create-group dialog and the folder
// list, and fake CLI handshake steps.
import { execFile } from 'node:child_process';
import { createHash, createPrivateKey, createPublicKey } from 'node:crypto';
import { createSocket, type Socket as UdpSocket } from 'node:dgram';
import { mkdir, readFile, realpath } from 'node:fs/promises';
import { createServer, type IncomingMessage, type Server, type ServerResponse } from 'node:http';
import type { AddressInfo, Socket } from 'node:net';
import { networkInterfaces } from 'node:os';
import { join } from 'node:path';
import { promisify } from 'node:util';
import type { Browser, BrowserContext, Locator, Page } from 'playwright';
import { expect, vi } from 'vitest';
import type { BrowserName } from './config';
import type { Node, Rendezvous } from './nodes';

export const exec = promisify(execFile);

const wsGuid = '258EAFA5-E914-47DA-95CA-C5AB0DC85B11';
const codePattern = /\b([0-9A-HJKMNP-TV-Z]{4}(?:-[0-9A-HJKMNP-TV-Z]{4}){3})\b/;
const ed25519Pkcs8Prefix = Buffer.from('302e020100300506032b657004220420', 'hex');

/** Polls an asynchronous assertion with a 15 s default timeout suited for browser E2E flows. */
export function poll<T>(fn: () => Promise<T> | T, timeout = 15_000) {
  return expect.poll(fn, { timeout });
}

export interface IceServer {
  urls: string[];
  username?: string;
  credential?: string;
}

export interface SignallingProxyOptions {
  targetHttp: string;
  targetWs: string;
  ice?: IceServer[];
  dropDirect?: boolean;
  dropLan?: boolean;
  reorderEarly?: boolean;
}

export interface SignallingProxy {
  url: string;
  signal: string;
  earlyReady: Promise<{ nodeIce: string }>;
  releaseAnswer(): void;
  replayEarly(): void;
  close(): Promise<void>;
}

export interface SignallingEnvelope {
  outer: { t?: string; k?: string; env?: { p?: string; k?: string } };
  inner?: { t?: string; cand?: string; mid?: string; conn?: string };
}

export interface DirectSurface {
  url: string;
  signal: string;
  close(): Promise<void>;
}

/** Picks a non-loopback IPv4 interface address for Firefox ICE tests, or 127.0.0.1. */
export function pickLanIPv4(): string {
  const lists = Object.values(networkInterfaces()).flatMap((items) => items ?? []);
  const found = lists.find((entry) => entry.family === 'IPv4' && !entry.internal);
  return found ? found.address : '127.0.0.1';
}

/** Reads a node's canonical base64url Ed25519 device id from `<dataDir>/device.key`. */
export async function readNodeDeviceId(node: Node): Promise<string> {
  const seed = await readFile(join(node.dataDir, 'device.key'));
  const priv = createPrivateKey({
    key: Buffer.concat([ed25519Pkcs8Prefix, seed]),
    format: 'der',
    type: 'pkcs8',
  });
  return String(createPublicKey(priv).export({ format: 'jwk' }).x);
}

function encodeStunBindingSuccess(req: Buffer, port: number, address: string): Buffer {
  const cookie = 0x2112a442;
  const attr = Buffer.alloc(12);
  attr.writeUInt16BE(0x0020, 0);
  attr.writeUInt16BE(8, 2);
  attr[5] = 0x01;
  attr.writeUInt16BE(port ^ (cookie >>> 16), 6);
  const octets = address.split('.').map(Number);
  const ipv4 = ((octets[0] << 24) | (octets[1] << 16) | (octets[2] << 8) | octets[3]) >>> 0;
  attr.writeUInt32BE((ipv4 ^ cookie) >>> 0, 8);
  const hdr = Buffer.alloc(20);
  hdr.writeUInt16BE(0x0101, 0);
  hdr.writeUInt16BE(attr.length, 2);
  req.copy(hdr, 4, 4, 20);
  return Buffer.concat([hdr, attr]);
}

/** Starts an RFC 5389 STUN Binding responder so srflx candidates gather over loopback or LAN. */
export async function startStunResponder(
  host: string,
): Promise<{ url: string; close(): Promise<void> }> {
  const sock: UdpSocket = createSocket('udp4');
  sock.on('message', (msg, rinfo) => {
    if (msg.length < 20 || msg.readUInt16BE(0) !== 0x0001) return;
    sock.send(encodeStunBindingSuccess(msg, rinfo.port, rinfo.address), rinfo.port, rinfo.address);
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

function encodeWsText(text: string): Buffer {
  const payload = Buffer.from(text, 'utf8');
  if (payload.length < 126) {
    return Buffer.concat([Buffer.from([0x81, payload.length]), payload]);
  }
  if (payload.length < 65_536) {
    const header = Buffer.alloc(4);
    header[0] = 0x81;
    header[1] = 126;
    header.writeUInt16BE(payload.length, 2);
    return Buffer.concat([header, payload]);
  }
  const header = Buffer.alloc(10);
  header[0] = 0x81;
  header[1] = 127;
  header.writeBigUInt64BE(BigInt(payload.length), 2);
  return Buffer.concat([header, payload]);
}

function frameHeader(buf: Buffer): { len: number; offset: number } | null {
  const code = buf[1] & 0x7f;
  if (code < 126) return { len: code, offset: 2 };
  if (code === 126) return buf.length < 4 ? null : { len: buf.readUInt16BE(2), offset: 4 };
  return buf.length < 10 ? null : { len: Number(buf.readBigUInt64BE(2)), offset: 10 };
}

function unmaskFrameBody(buf: Buffer, offset: number, len: number, masked: boolean): Buffer {
  const start = offset + (masked ? 4 : 0);
  const body = Buffer.from(buf.subarray(start, start + len));
  if (!masked) return body;
  for (let i = 0; i < len; i++) body[i] ^= buf[offset + (i & 3)];
  return body;
}

interface ParsedWsFrame {
  op: number;
  fin: boolean;
  body: Buffer;
  rest: Buffer;
}

function takeWsFrame(buf: Buffer): ParsedWsFrame | null {
  if (buf.length < 2) return null;
  const hdr = frameHeader(buf);
  if (!hdr) return null;
  const masked = (buf[1] & 0x80) !== 0;
  const total = hdr.offset + (masked ? 4 : 0) + hdr.len;
  if (buf.length < total) return null;
  return {
    op: buf[0] & 0x0f,
    fin: (buf[0] & 0x80) !== 0,
    body: unmaskFrameBody(buf, hdr.offset, hdr.len, masked),
    rest: buf.subarray(total),
  };
}

function dispatchWsFrame(
  socket: Socket,
  frame: ParsedWsFrame,
  parts: Buffer[],
  onText: (text: string) => void,
  onClose: () => void,
): boolean {
  if (frame.op === 0x8) {
    onClose();
    return false;
  }
  if (frame.op === 0x9) {
    socket.write(Buffer.concat([Buffer.from([0x8a, frame.body.length]), frame.body]));
    return true;
  }
  if (frame.op === 0x1 || frame.op === 0x0) {
    parts.push(frame.body);
    if (frame.fin) {
      onText(Buffer.concat(parts).toString('utf8'));
      parts.length = 0;
    }
  }
  return true;
}

function attachWsReader(
  socket: Socket,
  initial: Buffer,
  onText: (text: string) => void,
  onClose: () => void,
) {
  let buf: Buffer = Buffer.from(initial);
  const parts: Buffer[] = [];
  const drain = () => {
    for (let next = takeWsFrame(buf); next; next = takeWsFrame(buf)) {
      buf = next.rest;
      if (!dispatchWsFrame(socket, next, parts, onText, onClose)) return;
    }
  };
  socket.on('data', (chunk: Buffer) => {
    buf = Buffer.concat([buf, chunk]);
    drain();
  });
  socket.on('close', onClose);
  socket.on('error', onClose);
  drain();
}

/** Parses an outer rendezvous frame and, for `to`/`from`, its signed payload JSON. */
export function parseSignallingEnvelope(rawFrame: string): SignallingEnvelope {
  const outer = JSON.parse(rawFrame) as SignallingEnvelope['outer'];
  const relayed = outer.t === 'to' || outer.t === 'from';
  if (!relayed || typeof outer.env?.p !== 'string') return { outer };
  return { outer, inner: JSON.parse(outer.env.p) as SignallingEnvelope['inner'] };
}

function isNonRelayIce(rawFrame: string): boolean {
  const { inner } = parseSignallingEnvelope(rawFrame);
  return inner?.t === 'ice' && !(inner.cand ?? '').includes(' typ relay');
}

function isLanHostIce(rawFrame: string): boolean {
  const { inner } = parseSignallingEnvelope(rawFrame);
  if (inner?.t !== 'ice') return false;
  const cand = inner.cand ?? '';
  if (!cand.includes(' typ host')) return false;
  return !/\s(?:127\.0\.0\.1|\S+\.local)\s/.test(cand);
}

function dropIceFrame(rawFrame: string, opts: SignallingProxyOptions): boolean {
  return (
    (Boolean(opts.dropDirect) && isNonRelayIce(rawFrame)) ||
    (Boolean(opts.dropLan) && isLanHostIce(rawFrame))
  );
}

function injectIceServers(rawFrame: string, extra: IceServer[] | undefined): string {
  if (!extra?.length) return rawFrame;
  const parsed = JSON.parse(rawFrame) as { t?: string; ice?: IceServer[] };
  if (parsed.t !== 'ready' && parsed.t !== 'ice') return rawFrame;
  return JSON.stringify({ ...parsed, ice: [...(parsed.ice ?? []), ...extra] });
}

interface EarlyReorderer {
  earlyReady: Promise<{ nodeIce: string }>;
  releaseAnswer(): void;
  replayEarly(): void;
  setBrowserSink(sendDown: (text: string) => void): void;
  interceptDown(isBrowserConn: boolean, raw: string, sendDown: (text: string) => void): boolean;
}

function isAnswerOrIceFromPeer(raw: string): {
  match: boolean;
  isIce: boolean;
  isAnswer: boolean;
} {
  const { outer, inner } = parseSignallingEnvelope(raw);
  const isIce = outer.t === 'from' && inner?.t === 'ice';
  const isAnswer = outer.t === 'from' && inner?.t === 'answer';
  return { match: isIce || isAnswer, isIce, isAnswer };
}

function createEarlyReorderer(enabled: boolean): EarlyReorderer {
  const readyGate = Promise.withResolvers<{ nodeIce: string }>();
  const answerGate = Promise.withResolvers<void>();
  let sink: ((text: string) => void) | undefined;
  let firstNodeIce = '';
  let hasAnswer = false;
  let earlySent = false;
  let released = false;
  const held: string[] = [];

  const maybeSendEarly = (sendDown: (text: string) => void) => {
    if (earlySent || !hasAnswer || !firstNodeIce) return;
    earlySent = true;
    sendDown(firstNodeIce);
    sendDown(firstNodeIce);
    readyGate.resolve({ nodeIce: firstNodeIce });
    void answerGate.promise.then(() => {
      released = true;
      for (const frame of held.splice(0)) sendDown(frame);
    });
  };

  const holdFrame = (
    raw: string,
    isIce: boolean,
    isAnswer: boolean,
    sendDown: (text: string) => void,
  ) => {
    if (isIce && !firstNodeIce) firstNodeIce = raw;
    if (isAnswer) hasAnswer = true;
    held.push(raw);
    maybeSendEarly(sendDown);
  };

  return {
    earlyReady: readyGate.promise,
    releaseAnswer: () => answerGate.resolve(),
    replayEarly: () => {
      if (firstNodeIce && sink) sink(firstNodeIce);
    },
    setBrowserSink: (sendDown) => {
      sink = sendDown;
    },
    interceptDown: (isBrowserConn, raw, sendDown) => {
      if (!enabled || !isBrowserConn || released) return false;
      const info = isAnswerOrIceFromPeer(raw);
      if (!info.match) return false;
      holdFrame(raw, info.isIce, info.isAnswer, sendDown);
      return true;
    },
  };
}

async function forwardHttp(targetHttp: string, req: IncomingMessage, res: ServerResponse) {
  try {
    const upstream = await fetch(new URL(req.url ?? '/', targetHttp), { method: req.method });
    const headers: Record<string, string> = {};
    upstream.headers.forEach((value, key) => {
      if (key !== 'transfer-encoding' && key !== 'content-length') headers[key] = value;
    });
    res.writeHead(upstream.status, headers);
    res.end(Buffer.from(await upstream.arrayBuffer()));
  } catch {
    res.writeHead(502);
    res.end();
  }
}

function acceptWsUpgrade(req: IncomingMessage, socket: Socket): boolean {
  const key = req.headers['sec-websocket-key'];
  if (typeof key !== 'string') {
    socket.destroy();
    return false;
  }
  const accept = createHash('sha1')
    .update(key + wsGuid)
    .digest('base64');
  socket.write(
    'HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n' +
      `Sec-WebSocket-Accept: ${accept}\r\n\r\n`,
  );
  return true;
}

function bridgeUpgrade(
  req: IncomingMessage,
  socket: Socket,
  head: Buffer,
  opts: SignallingProxyOptions,
  reorder: EarlyReorderer,
  sockets: Set<Socket>,
  upstreams: Set<WebSocket>,
) {
  if (!acceptWsUpgrade(req, socket)) return;
  const isBrowserConn = Boolean(req.headers.origin);
  sockets.add(socket);
  const upstream = new WebSocket(opts.targetWs);
  upstreams.add(upstream);
  const pending: string[] = [];
  let closed = false;
  const sendDown = (text: string) => {
    if (!closed && !socket.destroyed) socket.write(encodeWsText(text));
  };
  if (isBrowserConn) reorder.setBrowserSink(sendDown);
  const shut = () => {
    if (closed) return;
    closed = true;
    upstreams.delete(upstream);
    sockets.delete(socket);
    upstream.close();
    socket.destroy();
  };
  upstream.addEventListener('open', () => {
    for (const item of pending.splice(0)) upstream.send(item);
  });
  upstream.addEventListener('message', (ev) => {
    const raw = injectIceServers(String(ev.data), opts.ice);
    if (dropIceFrame(raw, opts)) return;
    if (!reorder.interceptDown(isBrowserConn, raw, sendDown)) sendDown(raw);
  });
  upstream.addEventListener('close', shut);
  upstream.addEventListener('error', shut);
  attachWsReader(
    socket,
    head,
    (raw) => {
      if (dropIceFrame(raw, opts)) return;
      if (upstream.readyState === WebSocket.OPEN) upstream.send(raw);
      else pending.push(raw);
    },
    shut,
  );
}

/** Starts a local HTTP and `/rv` WebSocket proxy in front of a real rendezvous server. */
export async function startSignallingProxy(opts: SignallingProxyOptions): Promise<SignallingProxy> {
  const sockets = new Set<Socket>();
  const upstreams = new Set<WebSocket>();
  const reorder = createEarlyReorderer(Boolean(opts.reorderEarly));
  const server: Server = createServer((req, res) => void forwardHttp(opts.targetHttp, req, res));
  server.on('connection', (socket) => {
    sockets.add(socket);
    socket.on('close', () => sockets.delete(socket));
  });
  server.on('upgrade', (req, socket: Socket, head: Buffer) =>
    bridgeUpgrade(req, socket, head, opts, reorder, sockets, upstreams),
  );
  await new Promise<void>((resolveListen, rejectListen) => {
    server.once('error', rejectListen);
    server.listen(0, '127.0.0.1', () => resolveListen());
  });
  const { port } = server.address() as AddressInfo;
  return {
    url: `http://127.0.0.1:${port}/`,
    signal: `ws://127.0.0.1:${port}/rv`,
    earlyReady: reorder.earlyReady,
    releaseAnswer: reorder.releaseAnswer,
    replayEarly: reorder.replayEarly,
    close: async () => {
      for (const ws of upstreams) ws.close();
      for (const sock of sockets) sock.destroy();
      await new Promise<void>((done) => server.close(() => done()));
    },
  };
}

/** Wraps a local rendezvous with a UDP STUN responder so direct WebRTC connects across browsers. */
export async function startDirectSurface(
  rv: Rendezvous,
  engine: BrowserName,
): Promise<DirectSurface> {
  const stun = await startStunResponder(engine === 'firefox' ? pickLanIPv4() : '127.0.0.1');
  const proxy = await startSignallingProxy({
    targetHttp: rv.url,
    targetWs: rv.signal,
    ice: [{ urls: [stun.url] }],
  });
  return {
    url: proxy.url,
    signal: proxy.signal,
    close: async () => {
      await proxy.close();
      await stun.close();
    },
  };
}

/** Opens an English browser context whose WebSockets route only through `surface.signal`. */
export async function openDirectContext(
  browser: Browser,
  contexts: BrowserContext[],
  surface: DirectSurface,
): Promise<BrowserContext> {
  const context = await browser.newContext({
    locale: 'en-US',
    viewport: { width: 1440, height: 900 },
  });
  contexts.push(context);
  await context.routeWebSocket(
    (url) => !url.href.startsWith(surface.signal),
    (ws) => ws.close(),
  );
  return context;
}

/** Makes the folder `name` in `dir` for an AI to work in; returns its path with its links resolved,
 *  as the node keeps it. */
export async function makeFolder(dir: string, name: string): Promise<string> {
  const path = join(dir, name);
  await mkdir(path);
  return realpath(path);
}

/** Types `folder` in the folder list of the one folder picker in `scope`, and takes it. */
export async function chooseFolder(scope: Locator, folder: string): Promise<void> {
  await scope.getByRole('button', { name: /^Change the folder of / }).click();
  const list = scope.page().getByRole('dialog', { name: 'Choose a folder' });
  await list.getByLabel('Another folder').fill(folder);
  await list.getByRole('button', { name: 'Use this folder' }).click();
  await poll(() => scope.locator('.path-row code').textContent()).toBe(folder);
}

/** An AI ticked in the create-group dialog: its row, by CLI and computer, and what it gets. */
export interface GroupAI {
  cli: string;
  computer: string;
  name: string;
  folder: string;
  secretary?: boolean;
}

/** Fills the open create-group dialog, creates the group and returns its id once its chat opens. */
export async function createGroup(
  page: Page,
  {
    name,
    member,
    home,
    ais = [],
  }: { name: string; member: string; home?: string; ais?: GroupAI[] },
): Promise<string> {
  const dialog = page.getByRole('dialog', { name: 'New group' });
  await dialog.getByLabel('Group name').fill(name);
  await dialog.getByLabel('Your name in the group').fill(member);
  if (home) await dialog.getByLabel('Home computer').selectOption({ label: home });
  for (const ai of ais) await tickAI(dialog, ai);
  await dialog.getByRole('button', { name: 'Create group' }).click();
  await poll(() => page.url()).toMatch(/#\/groups\/[^/]+$/);
  return new URL(page.url()).hash.slice('#/groups/'.length);
}

async function tickAI(dialog: Locator, { cli, computer, name, folder, secretary }: GroupAI) {
  const row = dialog.locator('.ai').filter({ hasText: cli }).filter({ hasText: computer });
  await row.getByRole('checkbox').check();
  await row.getByLabel(`Name of ${cli} in the group`).fill(name);
  await chooseFolder(row, folder);
  if (secretary) await row.getByRole('radio', { name: `Make ${name} the secretary` }).check();
}

/** Waits for Onboarding to render a live 16-character pairing code in `#prompt`. */
export async function readPairingCode(page: Page, expectedSignal: string): Promise<string> {
  const prompt = page.locator('#prompt');
  let found = '';
  await vi.waitFor(async () => {
    const muted = await prompt.evaluate((el) => el.classList.contains('muted'));
    expect(muted).toBe(false);
    const text = (await prompt.textContent()) ?? '';
    expect(text).toContain(`--rendezvous ${expectedSignal}`);
    const match = codePattern.exec(text);
    expect(match, text).not.toBeNull();
    found = match ? match[1] : '';
  });
  return found;
}

/** Pairs a new browser with a node through the onboarding page, up to the direct connection. */
export async function pairPrimaryBrowser(
  page: Page,
  surface: DirectSurface,
  node: Node,
  label: string,
) {
  await page.goto(`${surface.url}#/onboarding`);
  const code = await readPairingCode(page, surface.signal);
  const paired = await node.run(
    'solodawn',
    'pair',
    code,
    '--name',
    label,
    '--rendezvous',
    surface.signal,
  );
  expect(paired.code, paired.stderr).toBe(0);
  await poll(() => page.locator('input[name="acct-claude"][value="personal"]').count()).toBe(1);
  await page.locator('input[name="acct-claude"][value="personal"]').check();
  await poll(() => page.locator('fieldset.acct').first().isDisabled()).toBe(false);
  await page.locator('input[name="acct-codex"][value="api_key"]').check();
  await finishOnboarding(page);
  await poll(() => page.locator('.bar a.conn').getAttribute('data-st')).toBe('ok');
}

/** Presses the onboarding's last button ("Next: set your AIs' defaults"), waits for `#/nodes`,
 *  then navigates to `#/groups` and opens "New group" when `keep` is true (or leaves the plain
 *  group list when `keep` is false). */
export async function finishOnboarding(page: Page, keep = false): Promise<void> {
  await poll(() => page.locator('a.done-btn').count()).toBe(1);
  await page.locator('a.done-btn').click();
  await poll(() => page.url()).toContain('#/nodes');
  await page
    .locator('.rail .tabs a[href="#/groups"], nav a[href="#/groups"]:visible')
    .first()
    .click();
  await poll(() => page.url()).toContain('#/groups');
  if (keep) {
    await page.locator('button', { hasText: 'New group' }).first().click();
    await poll(() => page.locator('dialog.dlg[open]').count()).toBe(1);
    return;
  }
  await poll(() => page.locator('dialog.dlg[open]').count()).toBe(0);
}

/** Waits for modal dialogs to close and the main line's composer to enable, fills `text`, and
 *  submits it. */
export async function sendChatMessage(page: Page, text: string): Promise<void> {
  await poll(() => page.locator('dialog.dlg[open]').count()).toBe(0);
  const composer = page.locator('.dock .composer textarea');
  const sendBtn = page.locator('.dock .composer button.send');
  await poll(() => composer.isDisabled()).toBe(false);
  await composer.fill(text);
  expect(await sendBtn.isDisabled()).toBe(false);
  await sendBtn.click();
  await poll(() => composer.inputValue()).toBe('');
}

/** Initial stdin/MCP/init handshake steps for a scripted fake Claude Code process. */
export function claudeInitSteps() {
  return [
    { dir: 'stdin', msg: { type: 'user', message: { role: 'user' } } },
    {
      dir: 'mcp<-cli',
      msg: {
        jsonrpc: '2.0',
        id: 0,
        method: 'initialize',
        params: {
          protocolVersion: '2025-11-25',
          capabilities: {},
          clientInfo: { name: 'claude-code', version: '2.1.311' },
        },
      },
    },
    { dir: 'mcp->cli', msg: { jsonrpc: '2.0', id: 0, result: {} } },
    { dir: 'mcp<-cli', msg: { jsonrpc: '2.0', method: 'notifications/initialized' } },
    { dir: 'stdout', msg: { type: 'system', subtype: 'init', session_id: '$session' } },
  ];
}

/** Initial app-server/thread/turn/MCP handshake steps for a scripted fake Codex process. */
export function codexInitSteps(turnId = 'turn-1') {
  return [
    { dir: 'stdin', msg: { method: 'initialize' } },
    { dir: 'stdout', msg: { id: '$id', result: {} } },
    { dir: 'stdin', msg: { method: 'initialized' } },
    { dir: 'stdin', msg: { method: 'thread/start' } },
    { dir: 'stdout', msg: { id: '$id', result: { thread: { id: '$session' } } } },
    { dir: 'stdin', msg: { method: 'turn/start', params: { threadId: '$session' } } },
    {
      dir: 'stdout',
      msg: { id: '$id', result: { turn: { id: turnId, status: 'inProgress', error: null } } },
    },
    {
      dir: 'mcp<-cli',
      msg: {
        jsonrpc: '2.0',
        id: 0,
        method: 'initialize',
        params: {
          protocolVersion: '2025-06-18',
          capabilities: {},
          clientInfo: { name: 'codex-mcp-client', version: '0.164.0' },
        },
      },
    },
    { dir: 'mcp->cli', msg: { jsonrpc: '2.0', id: 0, result: {} } },
    { dir: 'mcp<-cli', msg: { jsonrpc: '2.0', method: 'notifications/initialized' } },
  ];
}
