// Shared end-to-end helpers for E-M1, E-M2, E-M3, and E-M5: local STUN + rendezvous proxy so
// direct WebRTC gathers loopback/LAN srflx candidates cleanly across Chromium, Firefox, and
// WebKit, isolated Git identity environment for clean CI runners, and fake CLI init steps.
import { execFile } from 'node:child_process';
import { createHash } from 'node:crypto';
import { createSocket, type Socket as UdpSocket } from 'node:dgram';
import { mkdir, writeFile } from 'node:fs/promises';
import { createServer, type IncomingMessage, type Server, type ServerResponse } from 'node:http';
import type { AddressInfo, Socket } from 'node:net';
import { networkInterfaces } from 'node:os';
import { join } from 'node:path';
import { promisify } from 'node:util';
import type { Browser, BrowserContext, Page } from 'playwright';
import { expect, vi } from 'vitest';
import type { BrowserName } from './config';
import type { Env, Rendezvous } from './nodes';

export const exec = promisify(execFile);

const wsGuid = '258EAFA5-E914-47DA-95CA-C5AB0DC85B11';
const codePattern = /\b([0-9A-HJKMNP-TV-Z]{4}(?:-[0-9A-HJKMNP-TV-Z]{4}){3})\b/;

/** Polls an asynchronous assertion with a 15 s default timeout suited for browser E2E flows. */
export function poll<T>(fn: () => Promise<T> | T, timeout = 15_000) {
  return expect.poll(fn, { timeout });
}

/** Git author/committer config environment for clean CI runners with no ~/.gitconfig. */
export const gitIdentityEnv: Env = {
  GIT_CONFIG_COUNT: '3',
  GIT_CONFIG_KEY_0: 'user.name',
  GIT_CONFIG_VALUE_0: 'Alex',
  GIT_CONFIG_KEY_1: 'user.email',
  GIT_CONFIG_VALUE_1: 'alex@example.test',
  GIT_CONFIG_KEY_2: 'commit.gpgsign',
  GIT_CONFIG_VALUE_2: 'false',
};

export interface DirectSurface {
  url: string;
  signal: string;
  close(): Promise<void>;
}

function pickLanIPv4(): string {
  const lists = Object.values(networkInterfaces()).flatMap((items) => items ?? []);
  const found = lists.find((entry) => entry.family === 'IPv4' && !entry.internal);
  return found ? found.address : '127.0.0.1';
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

async function startStunResponder(host: string): Promise<{ url: string; close(): Promise<void> }> {
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

function injectStun(rawFrame: string, stunUrl: string): string {
  const parsed = JSON.parse(rawFrame) as { t?: string; ice?: Array<{ urls: string[] }> };
  if (parsed.t !== 'ready' && parsed.t !== 'ice') return rawFrame;
  return JSON.stringify({ ...parsed, ice: [...(parsed.ice ?? []), { urls: [stunUrl] }] });
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

function bridgeUpgrade(
  req: IncomingMessage,
  socket: Socket,
  head: Buffer,
  targetWs: string,
  stunUrl: string,
  sockets: Set<Socket>,
  upstreams: Set<WebSocket>,
) {
  const key = req.headers['sec-websocket-key'];
  if (typeof key !== 'string') {
    socket.destroy();
    return;
  }
  const accept = createHash('sha1')
    .update(key + wsGuid)
    .digest('base64');
  socket.write(
    'HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n' +
      `Sec-WebSocket-Accept: ${accept}\r\n\r\n`,
  );
  sockets.add(socket);
  const upstream = new WebSocket(targetWs);
  upstreams.add(upstream);
  const pending: string[] = [];
  let closed = false;
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
    if (!closed) socket.write(encodeWsText(injectStun(String(ev.data), stunUrl)));
  });
  upstream.addEventListener('close', shut);
  upstream.addEventListener('error', shut);
  attachWsReader(
    socket,
    head,
    (raw) => {
      if (upstream.readyState === WebSocket.OPEN) upstream.send(raw);
      else pending.push(raw);
    },
    shut,
  );
}

/** Wraps a local rendezvous with a UDP STUN responder so direct WebRTC connects across browsers. */
export async function startDirectSurface(
  rv: Rendezvous,
  engine: BrowserName,
): Promise<DirectSurface> {
  const stun = await startStunResponder(engine === 'firefox' ? pickLanIPv4() : '127.0.0.1');
  const sockets = new Set<Socket>();
  const upstreams = new Set<WebSocket>();
  const server: Server = createServer((req, res) => void forwardHttp(rv.url, req, res));
  server.on('connection', (socket) => {
    sockets.add(socket);
    socket.on('close', () => sockets.delete(socket));
  });
  server.on('upgrade', (req, socket: Socket, head: Buffer) =>
    bridgeUpgrade(req, socket, head, rv.signal, stun.url, sockets, upstreams),
  );
  await new Promise<void>((resolveListen, rejectListen) => {
    server.once('error', rejectListen);
    server.listen(0, '127.0.0.1', () => resolveListen());
  });
  const { port } = server.address() as AddressInfo;
  return {
    url: `http://127.0.0.1:${port}/`,
    signal: `ws://127.0.0.1:${port}/rv`,
    close: async () => {
      for (const ws of upstreams) ws.close();
      for (const sock of sockets) sock.destroy();
      await new Promise<void>((done) => server.close(() => done()));
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

/** Creates a local Git repository initialized on branch `main` with one commit. */
export async function initGitRepo(
  dir: string,
  folderName: string,
  readme: string,
): Promise<string> {
  const repo = join(dir, folderName);
  await mkdir(repo, { recursive: true });
  await exec('git', ['init', '-b', 'main'], {
    cwd: repo,
    env: { ...process.env, ...gitIdentityEnv },
  });
  await writeFile(join(repo, 'README.md'), readme);
  await exec('git', ['add', 'README.md'], {
    cwd: repo,
    env: { ...process.env, ...gitIdentityEnv },
  });
  await exec('git', ['commit', '-m', 'Initial commit'], {
    cwd: repo,
    env: { ...process.env, ...gitIdentityEnv },
  });
  return repo;
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

/** Waits for modal dialogs to close and composer to enable, fills `text`, and submits it. */
export async function sendChatMessage(page: Page, text: string): Promise<void> {
  await poll(() => page.locator('dialog.dlg[open]').count()).toBe(0);
  const composer = page.locator('.composer textarea');
  const sendBtn = page.locator('.composer button.send');
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
