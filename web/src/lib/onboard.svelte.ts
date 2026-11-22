// Reactive onboarding pairing session, `#pair=<secret>` link capture, `agent.set` host command,
// and shared CLI and account labels (planning/flows/pairing.md sections A, B and C;
// planning/contracts/envelope.md sections 2, 4 and 6; planning/contracts/datachannel.md sections 3
// and 7; planning/contracts/commands.md section 2).
import { DeviceLog } from './devlog';
import { lost, newId } from './groups.svelte';
import { decode, fingerprint, loadDevice, StorageBlocked, type Device } from './identity';
import type { Source } from './net.svelte';
import { loadOwner, saveOwner } from './owner';
import { mac, newSecret, parseSecret, room, shown, verifyPair } from './pairing';
import { connect, type Heard, type Path, type Peer, type Session } from './peer';
import { t, textIn, type Lang } from './prefs.svelte';
import type {
  CmdResult,
  Ctl,
  Devlog,
  Envelope,
  ErrorResponse,
  HostInfo,
  HostInfoPush,
  Pair,
  Push,
  Welcome,
} from './proto';

const windowSeconds = 600;
const second = 1000;

/** Default device display name used by browser pairing and live sessions. */
export const browserName = 'Browser';

/** Account types declared with `agent.set` (`contracts/commands.md` section 2). */
export const accounts = ['personal', 'team', 'api_key'] as const;
export type AccountKind = (typeof accounts)[number];

const cliLabels: Record<string, string> = { claude: 'Claude Code', codex: 'Codex' };

/** Display label of an AI CLI identifier. */
export const cliName = (agent: string) => cliLabels[agent] ?? agent;

const accountKeys: Record<string, `pairing.acct.${AccountKind}`> = {
  personal: 'pairing.acct.personal',
  team: 'pairing.acct.team',
  api_key: 'pairing.acct.api_key',
};

/** Localized label of a declared AI account type, or `account` as given when unknown. */
export const accountLabel = (account: string) =>
  account in accountKeys ? t(accountKeys[account]) : account;

/** Signalling WebSocket URL of the page's own origin (`contracts/rendezvous.md` section 1). */
export const signalUrlOf = (loc: Pick<Location, 'protocol' | 'host'> = location) =>
  `${loc.protocol === 'https:' ? 'wss:' : 'ws:'}//${loc.host}/rv`;

/** Builds the onboarding prompt in `lang` with the page's install and signalling URLs. */
export function promptFor(
  lang: Lang,
  code: string,
  loc: Pick<Location, 'origin' | 'protocol' | 'host'> = location,
): string {
  return textIn(lang, 'pairing.prompt.text_rv')
    .replace('{install}', `${loc.origin}/install.md`)
    .replace('{code}', code)
    .replace('{rv}', signalUrlOf(loc));
}

let pending: string | undefined;

function decodePart(raw: string): string {
  try {
    return decodeURIComponent(raw);
  } catch {
    return raw;
  }
}

/** Captures a `#pair=<secret>` address fragment and rewrites it to `#/onboarding`. */
export function capturePairHash() {
  if (!location.hash.startsWith('#pair=')) return;
  pending = decodePart(location.hash.slice('#pair='.length));
  location.hash = '#/onboarding';
}

/** Claims the secret captured from a `#pair=<secret>` link, once. */
export function takeLinkSecret(): string | undefined {
  capturePairHash();
  const taken = pending;
  pending = undefined;
  return taken;
}

/** Sends a signed `agent.set` host command (`contracts/commands.md` section 2). */
export async function setAccount(
  ctl: Ctl | undefined,
  signer: { sign(p: string): Promise<Envelope> },
  to: string,
  agent: string,
  account: AccountKind,
): Promise<CmdResult | ErrorResponse> {
  const env = await signer.sign(
    JSON.stringify({ t: 'agent.set', n: newId(), ts: Date.now(), to, agent, account }),
  );
  return ctl ? ctl.request({ t: 'cmd', env }).catch(lost) : lost();
}

/** Opens the development mock node; kept at module scope so production builds tree-shake `mock.ts`. */
async function openMock(onpush: (push: Push) => void) {
  const { mockCtl } = await import('./mock');
  const device = await loadDevice({ crypto, idb: () => indexedDB }).catch(() => undefined);
  if (!device) return undefined;
  const ctl = mockCtl(onpush);
  const welcome = (await ctl.request({
    t: 'hello',
    v: 1,
    kind: 'browser',
    name: browserName,
    addrs: [],
  })) as Welcome;
  const nodeFp = await fingerprint(crypto, decode(welcome.device)!);
  return { device, ctl, welcome, nodeFp };
}

export type Stage = 'waiting' | 'linking' | 'accounts' | 'ready' | 'expired' | 'failed';
export type ErrorKey = 'bad_secret' | 'unadopted' | 'not_owner' | 'lost' | 'refused' | 'storage';

export interface NodeSummary {
  id: string;
  name: string;
  fingerprint: string;
  version?: string;
  os?: string;
}

export interface FailureInfo {
  step: 1 | 2 | 3 | 4;
  key: ErrorKey;
  code?: string;
}

/** Drives one onboarding pairing session from secret creation (or `#pair=` link) to `agent.set`. */
export class PairingSession {
  role = $state<'a' | 'b'>('a');
  code = $state('');
  remaining = $state(windowSeconds);
  stage = $state<Stage>('waiting');
  browserFingerprint = $state('');
  node = $state<NodeSummary | undefined>();
  path = $state<Path | undefined>();
  enrolled = $state(false);
  agents = $state<HostInfo['agents'] | undefined>();
  busy = $state(false);
  error = $state<FailureInfo | undefined>();

  #source: Source;
  #secret = '';
  #roomId = '';
  #pairMsg: Pair | undefined;
  #device: Device | undefined;
  #ownerLog: DeviceLog | undefined;
  #candidateLog: DeviceLog | undefined;
  #addrs: Record<string, string[]> = {};
  #session: Session | undefined;
  #peer: Peer | undefined;
  #ctl: Ctl | undefined;
  #timer: ReturnType<typeof setInterval> | undefined;
  #expiresAt = 0;
  #runId = 0;

  constructor(source: Source) {
    this.#source = source;
    void this.start();
  }

  /** Starts or restarts pairing with a fresh secret and a 10-minute window. */
  async start() {
    this.#stop();
    const run = this.#runId;
    this.#resetState();
    if (import.meta.env.DEV && this.#source === 'mock') {
      const secret = newSecret(crypto);
      this.#secret = secret;
      this.code = shown(secret);
      this.#startCountdown();
      const ready = await openMock((push) => void this.#onPush(run, push));
      if (!ready || !this.#active(run)) return;
      this.#applyMock(run, ready);
      return;
    }
    await this.#startLive(run);
  }

  /** Declares the account type of one CLI on the paired node (`contracts/commands.md` section 2). */
  async setAccount(agent: string, account: AccountKind) {
    if (!this.#device || !this.node || !this.agents) return;
    this.busy = true;
    this.error = undefined;
    const answer = await setAccount(this.#ctl, this.#device, this.node.id, agent, account);
    this.busy = false;
    this.#applyAccountAnswer(this.#runId, agent, account, answer);
  }

  /** Stops timers and closes any open signalling session. */
  destroy() {
    this.#stop();
  }

  #active(run: number) {
    return run === this.#runId;
  }

  #applyAccountAnswer(
    run: number,
    agent: string,
    account: AccountKind,
    answer: CmdResult | ErrorResponse,
  ) {
    if (answer.err) {
      if (answer.err.code === 'lost') this.#fail(run, 4, 'lost');
      else this.error = { step: 4, key: 'refused', code: answer.err.code };
      return;
    }
    this.agents = this.agents!.map((item) => (item.agent === agent ? { ...item, account } : item));
    this.#syncStage();
  }

  #resetState() {
    this.role = 'a';
    this.code = '';
    this.remaining = windowSeconds;
    this.stage = 'waiting';
    this.node = undefined;
    this.path = undefined;
    this.enrolled = false;
    this.agents = undefined;
    this.busy = false;
    this.error = undefined;
    this.#candidateLog = undefined;
  }

  #startCountdown() {
    this.#expiresAt = Date.now() + windowSeconds * second;
    this.#timer = setInterval(() => {
      const left = Math.max(0, Math.ceil((this.#expiresAt - Date.now()) / second));
      this.remaining = left;
      if (left === 0) this.#expire();
    }, second);
  }

  #expire() {
    this.#stop();
    this.stage = 'expired';
  }

  #discardSecret() {
    this.#secret = '';
    this.#roomId = '';
    this.#pairMsg = undefined;
    this.code = '';
    clearInterval(this.#timer);
    this.#timer = undefined;
  }

  #stop() {
    this.#runId += 1;
    this.#discardSecret();
    this.#session?.close();
    this.#session = undefined;
    this.#peer = undefined;
    this.#ctl = undefined;
  }

  #fail(run: number, step: 1 | 2 | 3 | 4, key: ErrorKey, code?: string) {
    if (!this.#active(run)) return;
    this.#stop();
    this.error = { step, key, code };
    this.stage = 'failed';
  }

  #applyMock(run: number, ready: NonNullable<Awaited<ReturnType<typeof openMock>>>) {
    this.#device = ready.device;
    this.browserFingerprint = ready.device.fingerprint;
    this.#ctl = ready.ctl;
    this.node = {
      id: ready.welcome.device,
      name: ready.welcome.name,
      fingerprint: ready.nodeFp,
      version: ready.welcome.version,
    };
    this.path = { kind: 'direct', rtt: 12, since: Date.now() };
    this.enrolled = true;
    void this.#fetchHostInfo(run, ready.ctl);
  }

  async #startLive(run: number) {
    if (!this.#prepareSecret(run, takeLinkSecret())) return;
    this.#startCountdown();
    const loaded = await this.#loadAndSign(run);
    if (!loaded || !this.#active(run)) return;
    this.#session = connect({
      urls: [signalUrlOf()],
      device: loaded,
      known: (id) => id === this.node?.id || Boolean(this.#ownerLog?.valid(id)),
      name: browserName,
      heard: (url, frame) => void this.#onHeard(run, url, frame),
    });
  }

  #prepareSecret(run: number, linkSecret: string | undefined): boolean {
    if (linkSecret === undefined) {
      this.#secret = newSecret(crypto);
      this.code = shown(this.#secret);
      return true;
    }
    this.role = 'b';
    try {
      this.#secret = parseSecret(linkSecret);
      return true;
    } catch {
      this.#fail(run, 1, 'bad_secret');
      return false;
    }
  }

  async #loadAndSign(run: number): Promise<Device | undefined> {
    try {
      const device = await loadDevice({ crypto, idb: () => indexedDB });
      const owner = await loadOwner(crypto, () => indexedDB);
      this.#device = device;
      this.#ownerLog = owner.log;
      this.#addrs = owner.addrs;
      this.browserFingerprint = device.fingerprint;
      this.#roomId = await room(crypto, this.#secret);
      const self = { role: this.role, k: device.id, kind: 'browser', name: browserName };
      this.#pairMsg = { t: 'pair', ...self, mac: await mac(crypto, this.#secret, self) };
      return device;
    } catch (err) {
      this.#fail(run, 1, err instanceof StorageBlocked ? 'storage' : 'lost');
      return undefined;
    }
  }

  async #onHeard(run: number, url: string, frame: Heard) {
    if (!this.#active(run) || !this.#roomId) return;
    await this.#dispatchHeard(run, url, frame);
  }

  async #dispatchHeard(run: number, url: string, frame: Heard) {
    if (frame.t === 'ready') this.#joinRoom(url);
    else if (frame.t === 'joined') this.#sendPair(url, frame.room);
    else if (frame.t === 'room') await this.#receivePair(run, frame.room, frame.data);
  }

  #joinRoom(url: string) {
    if (!this.node) this.#session!.send(url, { t: 'join', room: this.#roomId });
  }

  #sendPair(url: string, roomId: string) {
    if (roomId === this.#roomId) {
      this.#session!.send(url, { t: 'room', room: roomId, data: this.#pairMsg! });
    }
  }

  #isCandidatePair(roomId: string, data: Pair): boolean {
    return roomId === this.#roomId && !this.node && data.kind === 'node';
  }

  async #receivePair(run: number, roomId: string, data: Pair) {
    if (!this.#isCandidatePair(roomId, data)) return;
    const expected = this.role === 'a' ? 'b' : 'a';
    const valid = await verifyPair(crypto, this.#secret, data, expected);
    if (!valid || !this.#active(run)) return;
    await this.#dialNode(run, data);
  }

  async #dialNode(run: number, data: Pair) {
    const nodeFp = await fingerprint(crypto, decode(data.k)!);
    if (!this.#active(run) || this.node) return;
    this.node = { id: data.k, name: data.name, fingerprint: nodeFp };
    this.stage = 'linking';
    this.#peer = this.#session!.peer(
      data.k,
      () => ({
        online: (ctl, welcome) => void this.#onOnline(run, ctl, welcome),
        push: (frame) => void this.#onPush(run, frame),
        offline: () => this.#onOffline(run),
        refused: (code) => this.#fail(run, 3, 'refused', code),
      }),
      (path) => {
        if (this.#active(run)) this.path = path;
      },
    );
  }

  async #onOnline(run: number, ctl: Ctl, welcome: Welcome) {
    if (!this.#active(run)) return;
    this.#ctl = ctl;
    this.#addrs[welcome.device] = welcome.addrs;
    this.node = { ...this.node!, name: welcome.name, version: welcome.version };
    if (!this.enrolled) await this.#beginEnrolment(run, ctl, welcome);
  }

  async #beginEnrolment(run: number, ctl: Ctl, welcome: Welcome) {
    if (this.role === 'a') return this.#enrolNode(run, ctl);
    if (welcome.owner) return this.#finishEnrolment(run, ctl, welcome);
    if (this.#ownerLog) this.#fail(run, 3, 'unadopted');
  }

  async #enrolNode(run: number, ctl: Ctl) {
    const peer = this.#peer!;
    const log = await this.#ensureNodeInLog();
    peer.send({ t: 'devlog', user: log.user, entries: [...log.entries] });
    await this.#confirmOwner(run, ctl);
  }

  async #ensureNodeInLog(): Promise<DeviceLog> {
    const device = this.#device!;
    const target = this.node!;
    const log = this.#ownerLog ?? new DeviceLog(crypto, device.id);
    if (log.entries.length === 0) {
      await log.apply(await this.#signEntry(log.user, 1, device.id, 'browser', browserName));
    }
    if (!log.valid(target.id)) {
      const ver = log.entries.length + 1;
      await log.apply(await this.#signEntry(log.user, ver, target.id, 'node', target.name));
    }
    this.#ownerLog = log;
    await saveOwner(() => indexedDB, log, this.#addrs);
    return log;
  }

  #signEntry(user: string, ver: number, dev: string, kind: 'node' | 'browser', name: string) {
    return this.#device!.sign(
      JSON.stringify({
        t: 'device',
        n: newId(),
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

  async #confirmOwner(run: number, ctl: Ctl) {
    const answer = await ctl
      .request({ t: 'hello', v: 1, kind: 'browser', name: browserName, addrs: [] })
      .catch(() => undefined);
    if (!answer) return this.#fail(run, 3, 'lost');
    if (answer.err) return this.#fail(run, 3, 'refused', answer.err.code);
    if (!answer.owner) return this.#fail(run, 3, 'not_owner');
    await this.#finishEnrolment(run, ctl, answer);
  }

  async #onPush(run: number, frame: Push) {
    if (!this.#active(run)) return;
    if (frame.t === 'host.info') this.#applyHostInfo(frame);
    else if (frame.t === 'devlog') await this.#onDevlog(run, frame);
  }

  #canAdopt(): boolean {
    return !this.enrolled && this.role === 'b' && !this.#ownerLog && Boolean(this.#ctl);
  }

  async #onDevlog(run: number, frame: Devlog) {
    if (!this.#canAdopt()) return;
    const log = (this.#candidateLog ??= new DeviceLog(crypto, frame.user));
    for (const env of frame.entries) {
      const ok = await log.apply(env).then(
        () => true,
        () => false,
      );
      if (!ok) return this.#fail(run, 3, 'unadopted');
    }
    if (log.valid(this.#device!.id)) await this.#adoptLog(run, log);
  }

  async #adoptLog(run: number, log: DeviceLog) {
    this.#ownerLog = log;
    await saveOwner(() => indexedDB, log, this.#addrs);
    await this.#confirmOwner(run, this.#ctl!);
  }

  async #finishEnrolment(run: number, ctl: Ctl, welcome: Welcome) {
    this.#addrs[welcome.device] = welcome.addrs;
    if (this.#ownerLog) await saveOwner(() => indexedDB, this.#ownerLog, this.#addrs);
    if (!this.#active(run)) return;
    this.enrolled = true;
    this.#discardSecret();
    void navigator.storage?.persist?.().catch(() => undefined);
    await this.#fetchHostInfo(run, ctl);
  }

  async #fetchHostInfo(run: number, ctl: Ctl) {
    const info = await ctl.request({ t: 'host.info' }).catch(() => undefined);
    if (!info) return this.#fail(run, 4, 'lost');
    if (info.err) return this.#fail(run, 4, 'refused', info.err.code);
    if (this.#active(run)) this.#applyHostInfo(info);
  }

  #applyHostInfo(info: HostInfo | HostInfoPush) {
    this.node = { ...this.node!, name: info.name, os: info.os, version: info.version };
    this.agents = info.agents;
    this.#syncStage();
  }

  #syncStage() {
    const undeclared = this.agents!.some((item) => item.account === null);
    this.stage = undeclared ? 'accounts' : 'ready';
  }

  #onOffline(run: number) {
    if (this.#active(run) && !this.agents) this.#fail(run, 3, 'lost');
  }
}
