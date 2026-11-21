// Reactive onboarding pairing session (planning/flows/pairing.md sections A, B and C;
// planning/contracts/envelope.md sections 2, 4 and 6; planning/contracts/datachannel.md sections 3
// and 7; planning/contracts/commands.md section 2).
import { DeviceLog } from '../../lib/devlog';
import {
  decode,
  encode,
  fingerprint,
  loadDevice,
  StorageBlocked,
  type Device,
} from '../../lib/identity';
import type { Source } from '../../lib/net.svelte';
import { mac, newSecret, parseSecret, room, shown, verifyPair } from '../../lib/pairing';
import { connect, type Heard, type Path, type Peer, type Session } from '../../lib/peer';
import { textIn, type Lang } from '../../lib/prefs.svelte';
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
} from '../../lib/proto';
import { takeLinkSecret } from './mounts';
import { devlogFrames, loadOwner, saveOwner } from './store';

const windowSeconds = 600;
const second = 1000;
const officialOrigin = 'https://solodawn.org';
const browserName = 'Browser';

const cliLabels: Record<string, string> = { claude: 'Claude Code', codex: 'Codex' };

/** Display label of an AI CLI identifier. */
export const cliName = (agent: string) => cliLabels[agent] ?? agent;

/** Signalling WebSocket URL of the page's own origin (`contracts/rendezvous.md` section 1). */
export const signalUrlOf = (loc: Pick<Location, 'protocol' | 'host'> = location) =>
  `${loc.protocol === 'https:' ? 'wss:' : 'ws:'}//${loc.host}/rv`;

/** Builds the onboarding prompt in `lang`, including `--rendezvous` outside the official origin. */
export function promptFor(
  lang: Lang,
  code: string,
  loc: Pick<Location, 'origin' | 'protocol' | 'host'> = location,
): string {
  const key = loc.origin === officialOrigin ? 'pairing.prompt.text' : 'pairing.prompt.text_rv';
  return textIn(lang, key)
    .replace('{install}', `${loc.origin}/install.md`)
    .replace('{code}', code)
    .replace('{rv}', signalUrlOf(loc));
}

export type AccountKind = 'personal' | 'team' | 'api_key';
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

const newId = () => encode(crypto.getRandomValues(new Uint8Array(16)));

function entryDevice(p: string): string | undefined {
  try {
    return (JSON.parse(p) as { dev?: string }).dev;
  } catch {
    return undefined;
  }
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
    this.#cleanup();
    const run = (this.#runId += 1);
    this.#resetState();
    if (import.meta.env.DEV && this.#source === 'mock') return this.#startMock(run);
    await this.#startLive(run);
  }

  /** Declares the account type of one CLI on the paired node (`contracts/commands.md` section 2). */
  async setAccount(agent: string, account: AccountKind) {
    if (!this.#ctl || !this.#device || !this.node) return;
    this.busy = true;
    this.error = undefined;
    const payload = JSON.stringify({
      t: 'agent.set',
      n: newId(),
      ts: Date.now(),
      to: this.node.id,
      agent,
      account,
    });
    const env = await this.#device.sign(payload);
    const answer = await this.#ctl.request({ t: 'cmd', env }).catch(() => undefined);
    this.busy = false;
    this.#applyAccountAnswer(agent, account, answer);
  }

  destroy() {
    this.#runId += 1;
    this.#discardSecret();
    this.#cleanup();
  }

  #active(run: number) {
    return run === this.#runId;
  }

  #applyAccountAnswer(
    agent: string,
    account: AccountKind,
    answer: CmdResult | ErrorResponse | undefined,
  ) {
    if (!answer) return this.#fail(4, 'lost');
    if (answer.err) {
      this.error = { step: 4, key: 'refused', code: answer.err.code };
      return;
    }
    this.#setAgentAccount(agent, account);
    this.#syncStage();
  }

  #setAgentAccount(agent: string, account: AccountKind) {
    this.agents = this.agents?.map((item) => (item.agent === agent ? { ...item, account } : item));
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
    this.#ctl = undefined;
  }

  #startCountdown(run: number) {
    this.#expiresAt = Date.now() + windowSeconds * second;
    this.#timer = setInterval(() => {
      if (!this.#active(run)) return;
      const left = Math.max(0, Math.ceil((this.#expiresAt - Date.now()) / second));
      this.remaining = left;
      if (left === 0) this.#expire();
    }, second);
  }

  #expire() {
    this.#discardSecret();
    this.#cleanup();
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

  #cleanup() {
    clearInterval(this.#timer);
    this.#timer = undefined;
    this.#session?.close();
    this.#session = undefined;
    this.#peer = undefined;
  }

  #fail(step: 1 | 2 | 3 | 4, key: ErrorKey, code?: string) {
    this.#discardSecret();
    this.#cleanup();
    this.error = { step, key, code };
    this.stage = 'failed';
  }

  async #startMock(run: number) {
    if (!import.meta.env.DEV) return;
    const secret = newSecret(crypto);
    this.#secret = secret;
    this.code = shown(secret);
    this.#startCountdown(run);
    const { mockCtl } = await import('../../lib/mock');
    const device = await loadDevice({ crypto, idb: () => indexedDB }).catch(() => undefined);
    if (!device || !this.#active(run)) return;
    this.#device = device;
    this.browserFingerprint = device.fingerprint;
    await this.#connectMock(
      run,
      mockCtl((push) => void this.#onPush(run, push)),
    );
  }

  async #connectMock(run: number, ctl: Ctl) {
    this.#ctl = ctl;
    const welcome = (await ctl.request({
      t: 'hello',
      v: 1,
      kind: 'browser',
      name: browserName,
      addrs: [],
    })) as Welcome;
    const nodeFp = await fingerprint(crypto, decode(welcome.device)!);
    if (!this.#active(run)) return;
    this.node = {
      id: welcome.device,
      name: welcome.name,
      fingerprint: nodeFp,
      version: welcome.version,
    };
    this.path = { kind: 'direct', rtt: 12, since: Date.now() };
    this.enrolled = true;
    await this.#fetchHostInfo(run, ctl);
  }

  async #startLive(run: number) {
    if (!this.#prepareSecret(takeLinkSecret())) return;
    this.#startCountdown(run);
    const loaded = await this.#loadIdentity();
    if (!loaded || !this.#active(run)) return;
    await this.#openSignalling(run, loaded);
  }

  async #openSignalling(run: number, loaded: Device) {
    this.#roomId = await room(crypto, this.#secret);
    const self = { role: this.role, k: loaded.id, kind: 'browser', name: browserName };
    this.#pairMsg = { t: 'pair', ...self, mac: await mac(crypto, this.#secret, self) };
    if (!this.#active(run) || !this.#secret) return;
    this.#session = connect({
      urls: [signalUrlOf()],
      device: loaded,
      known: (id) => this.#isKnown(id),
      name: browserName,
      heard: (url, frame) => void this.#onHeard(run, url, frame),
    });
  }

  #isKnown(id: string): boolean {
    if (id === this.node?.id) return true;
    return Boolean(this.#ownerLog?.valid(id));
  }

  #prepareSecret(linkSecret: string | undefined): boolean {
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
      this.#fail(1, 'bad_secret');
      return false;
    }
  }

  async #loadIdentity(): Promise<Device | undefined> {
    try {
      const device = await loadDevice({ crypto, idb: () => indexedDB });
      const owner = await loadOwner(crypto, () => indexedDB);
      this.#device = device;
      this.#ownerLog = owner.log;
      this.#addrs = owner.addrs;
      this.browserFingerprint = device.fingerprint;
      return device;
    } catch (err) {
      this.#fail(1, err instanceof StorageBlocked ? 'storage' : 'lost');
      return undefined;
    }
  }

  async #onHeard(run: number, url: string, frame: Heard) {
    if (!this.#active(run) || !this.#roomId) return;
    if (frame.t === 'ready') this.#joinRoom(url);
    else if (frame.t === 'joined') this.#sendPair(url, frame.room);
    else if (frame.t === 'room') await this.#receivePair(run, frame.room, frame.data);
  }

  #joinRoom(url: string) {
    if (!this.node) this.#session?.send(url, { t: 'join', room: this.#roomId });
  }

  #sendPair(url: string, roomId: string) {
    if (roomId === this.#roomId && this.#pairMsg) {
      this.#session?.send(url, { t: 'room', room: roomId, data: this.#pairMsg });
    }
  }

  async #receivePair(run: number, roomId: string, data: Pair) {
    if (roomId !== this.#roomId || this.node || data.kind !== 'node') return;
    const expected = this.role === 'a' ? 'b' : 'a';
    const valid = await verifyPair(crypto, this.#secret, data, expected);
    if (!valid || !this.#active(run) || this.node) return;
    await this.#dialNode(run, data);
  }

  async #dialNode(run: number, data: Pair) {
    const nodeFp = await fingerprint(crypto, decode(data.k)!);
    if (!this.#active(run) || !this.#session) return;
    this.node = { id: data.k, name: data.name, fingerprint: nodeFp };
    this.stage = 'linking';
    this.#peer = this.#session.peer(
      data.k,
      () => ({
        online: (ctl, welcome) => void this.#onOnline(run, ctl, welcome),
        push: (frame) => void this.#onPush(run, frame),
        offline: () => this.#onOffline(run),
        refused: (code) => this.#fail(3, 'refused', code),
      }),
      (path) => {
        if (this.#active(run)) this.path = path;
      },
    );
  }

  async #onOnline(run: number, ctl: Ctl, welcome: Welcome) {
    if (!this.#active(run) || !this.node) return;
    this.#ctl = ctl;
    this.#addrs[welcome.device] = welcome.addrs;
    this.node = { ...this.node, name: welcome.name, version: welcome.version };
    if (!this.enrolled) await this.#beginEnrolment(run, ctl, welcome);
  }

  async #beginEnrolment(run: number, ctl: Ctl, welcome: Welcome) {
    if (this.role === 'a') return this.#enrolNode(run, ctl);
    if (welcome.owner) return this.#finishEnrolment(run, ctl, welcome);
    if (this.#ownerLog) this.#fail(3, 'unadopted');
  }

  async #enrolNode(run: number, ctl: Ctl) {
    const log = await this.#ensureNodeInLog();
    if (!this.#active(run) || !this.#peer) return;
    for (const frame of devlogFrames(log.user, log.entries)) this.#peer.send(frame);
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
    if (!this.#active(run)) return;
    if (!answer) return this.#fail(3, 'lost');
    if (answer.err) return this.#fail(3, 'refused', answer.err.code);
    if (!answer.owner) return this.#fail(3, 'not_owner');
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
    const batch = await this.#applyEntries(log, frame.entries);
    if (!batch.ok) return this.#fail(3, 'unadopted');
    if (this.#active(run) && batch.mentionsMe) await this.#adoptLog(run, log);
  }

  async #applyEntries(
    log: DeviceLog,
    entries: Envelope[],
  ): Promise<{ ok: boolean; mentionsMe: boolean }> {
    let mentionsMe = false;
    for (const env of entries) {
      const ok = await log.apply(env).then(
        () => true,
        () => false,
      );
      if (!ok) return { ok: false, mentionsMe: false };
      if (entryDevice(env.p) === this.#device?.id) mentionsMe = true;
    }
    return { ok: true, mentionsMe };
  }

  async #adoptLog(run: number, log: DeviceLog) {
    if (!log.adopts(this.#device!.id, this.node!.id)) return this.#fail(3, 'unadopted');
    this.#ownerLog = log;
    await saveOwner(() => indexedDB, log, this.#addrs);
    if (this.#active(run)) await this.#confirmOwner(run, this.#ctl!);
  }

  async #finishEnrolment(run: number, ctl: Ctl, welcome: Welcome) {
    this.#addrs[welcome.device] = welcome.addrs;
    if (this.#ownerLog) await saveOwner(() => indexedDB, this.#ownerLog, this.#addrs);
    if (!this.#active(run)) return;
    this.enrolled = true;
    this.#discardSecret();
    void navigator.storage?.persist?.()?.catch(() => undefined);
    await this.#fetchHostInfo(run, ctl);
  }

  async #fetchHostInfo(run: number, ctl: Ctl) {
    const info = await ctl.request({ t: 'host.info' }).catch(() => undefined);
    if (!this.#active(run)) return;
    if (!info) return this.#fail(4, 'lost');
    if (info.err) return this.#fail(4, 'refused', info.err.code);
    this.#applyHostInfo(info);
  }

  #applyHostInfo(info: HostInfo | HostInfoPush) {
    if (this.node) {
      this.node = { ...this.node, name: info.name, os: info.os, version: info.version };
    }
    this.agents = info.agents;
    this.#syncStage();
  }

  #syncStage() {
    if (!this.agents) return;
    const undeclared = this.agents.some((item) => item.account === null);
    this.stage = undeclared ? 'accounts' : 'ready';
  }

  #onOffline(run: number) {
    if (this.#active(run) && !this.agents) this.#fail(3, 'lost');
  }
}
