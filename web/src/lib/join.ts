import * as v from 'valibot';
import type { DeviceLog } from './devlog';
import { newId, type Home, type Link } from './groups.svelte';
import { isDevice, utf8, type Device } from './identity';
import { mac, parseSecret, room, verifyPair } from './pairing';
import type { Heard, Listener, Path, Peer, Session } from './peer';
import {
  Id,
  type Ctl,
  type Devlog,
  type Envelope,
  type Groups,
  type Pair,
  type Welcome,
} from './proto';

export interface Invitation {
  secret: string;
  g: string;
  home: string;
}

export class JoinError extends Error {
  constructor(
    readonly step:
      | 'link'
      | 'onboarding'
      | 'room'
      | 'hello'
      | 'devlog'
      | 'join'
      | 'groups'
      | 'remember'
      | 'home.add',
    readonly code: string,
  ) {
    super(`${step}: ${code}`);
  }
}

export function readInvitation(hash: string): Invitation | undefined {
  if (!hash.startsWith('#join=')) return undefined;
  const fields = new URLSearchParams(hash.slice(1));
  const g = fields.get('g');
  const home = fields.get('home');
  if (!v.is(Id, g) || !isDevice(String(home))) throw new JoinError('link', 'invalid');
  try {
    return { secret: parseSecret(fields.get('join')!), g, home: home! };
  } catch {
    throw new JoinError('link', 'invalid');
  }
}

interface JoinOptions {
  device: Device;
  log: DeviceLog;
  browserName: string;
  url: string;
  session: Pick<Session, 'watchRoom' | 'send'>;
  dial(listen: () => Listener, onpath: (path: Path | undefined) => void): JoinPeer;
  signal: AbortSignal;
  remember(name: string): Promise<void>;
  link(): Link;
  ownNodes(): Home[];
  onpath(path: Path | undefined): void;
}

type JoinPeer = Pick<Peer, 'close' | 'send'>;

export function presentLog(ctl: { send?(frame: Devlog): void }, log: DeviceLog) {
  const frame: Devlog = { t: 'devlog', user: log.user, entries: [] };
  for (const entry of log.entries) {
    frame.entries.push(entry);
    if (utf8(JSON.stringify(frame)).length <= 61_440) continue;
    frame.entries.pop();
    ctl.send!(frame);
    frame.entries = [entry];
  }
  ctl.send!(frame);
}

type GroupHome = Pick<Invitation, 'g' | 'home'>;

function homeNotice(node: Home, { g, home }: GroupHome, name: string, url: string) {
  return node.me.sign(
    JSON.stringify({
      t: 'home.add',
      n: newId(),
      ts: Date.now(),
      to: node.device,
      g,
      home,
      name,
      addrs: [url],
    }),
  );
}

async function sendNotice(node: Home, env: Envelope) {
  const answer = await node.ctl?.request({ t: 'cmd', env }).catch(() => undefined);
  if (!answer) return { node: node.device, error: new JoinError('home.add', 'lost') };
  if (answer.err) return { node: node.device, error: new JoinError('home.add', answer.err.code) };
}

async function notify(nodes: Home[], envelopes: Envelope[]) {
  const results = await Promise.all(nodes.map((node, i) => sendNotice(node, envelopes[i])));
  return results.filter((result) => result !== undefined);
}

/** Tells one of the person's nodes the home of a group (contracts/step-2/people.md section 7,
 *  point 2). */
export async function addHome(node: Home, group: GroupHome, name: string, url: string) {
  await sendNotice(node, await homeNotice(node, group, name, url));
}

function abortPending(signal: AbortSignal, failed: (code: string) => void) {
  const abort = () => failed('cancelled');
  const timer = setTimeout(() => failed('timeout'), 600_000);
  signal.addEventListener('abort', abort);
  return () => {
    clearTimeout(timer);
    signal.removeEventListener('abort', abort);
  };
}

function checkCancel(signal: AbortSignal, step: JoinError['step']) {
  if (signal.aborted) throw new JoinError(step, 'cancelled');
}

function joinRoom(
  invitation: Invitation,
  options: JoinOptions,
  id: string,
  self: Pair,
  dial: () => void,
) {
  const { session, url } = options;
  let offered = false;
  const receivePair = async (data: Pair) => {
    if (!(await invitationPair(invitation, data)) || offered) return;
    offered = true;
    dial();
  };
  const stop = session.watchRoom(url, id, (path, frame) => {
    if (offered || path !== url) return;
    if (!inRoom(frame, id)) return;
    if (frame.t === 'room') void receivePair(frame.data);
    else session.send(url, { t: 'room', room: id, data: self });
  });
  return () => {
    offered = true;
    stop();
  };
}

function inRoom(frame: Heard, id: string): frame is Extract<Heard, { t: 'joined' | 'room' }> {
  return 'room' in frame && frame.room === id;
}

function invitationPair(invitation: Invitation, data: Pair) {
  return (
    data.k === invitation.home &&
    data.kind === 'node' &&
    verifyPair(crypto, invitation.secret, data, 'a')
  );
}

function open(invitation: Invitation, options: JoinOptions, id: string, pair: Pair) {
  const ready = Promise.withResolvers<{ ctl: Ctl; welcome: Welcome }>();
  const listed = Promise.withResolvers<Groups>();
  void listed.promise.catch(() => {});
  let step: JoinError['step'] = 'room';
  let start: Link | undefined;
  let peer: JoinPeer | undefined;
  const failed = (code: string) => {
    const error = new JoinError(step, code);
    ready.reject(error);
    listed.reject(error);
  };
  const stop = joinRoom(invitation, options, id, pair, () => {
    step = 'hello';
    peer = options.dial(() => {
      const link = start && options.link();
      return {
        online: (ctl, welcome) => {
          if (!link) return ready.resolve({ ctl, welcome });
          presentLog(ctl, options.log);
          link.online(ctl, welcome);
        },
        offline: () => {
          const served = link ?? start;
          if (served) served.offline();
          else failed('lost');
        },
        refused: failed,
        push: (frame) => {
          if (start) return (link ?? start).push(frame);
          if (frame.t === 'groups' && frame.groups.some((group) => group.g === invitation.g)) {
            listed.resolve(frame);
          }
        },
      };
    }, options.onpath);
  });
  return {
    stop,
    get peer() {
      return peer!;
    },
    close() {
      peer?.close();
      peer = undefined;
    },
    ready: ready.promise,
    listed: listed.promise,
    failed,
    next: (value: JoinError['step']) => (step = value),
    error: (error: unknown) => (error instanceof JoinError ? error : new JoinError(step, 'lost')),
    serve(ctl: Ctl, welcome: Welcome, groups: Groups) {
      start = options.link();
      start.online(ctl, welcome);
      start.push(groups);
    },
  };
}

export async function joinGroup(invitation: Invitation, name: string, options: JoinOptions) {
  const { device, log, signal, browserName } = options;
  if (!log.valid(device.id)) throw new JoinError('onboarding', 'unpaired');
  const fields = { role: 'b' as const, k: device.id, kind: 'browser' as const, name: browserName };
  const id = await room(crypto, invitation.secret);
  const pair: Pair = { t: 'pair', ...fields, mac: await mac(crypto, invitation.secret, fields) };
  checkCancel(signal, 'room');
  const connection = open(invitation, options, id, pair);
  const stop = abortPending(signal, (code) => {
    connection.failed(code);
    connection.stop();
    connection.close();
  });
  try {
    const { ctl, welcome } = await connection.ready;
    await admit(ctl, welcome, invitation, name, options, connection.next);
    connection.next('remember');
    await options.remember(welcome.name);
    checkCancel(signal, 'remember');
    connection.next('groups');
    const groups = await (welcome.groups.some((group) => group.g === invitation.g)
      ? { t: 'groups' as const, groups: welcome.groups }
      : connection.listed);
    const group = groups.groups.find((group) => group.g === invitation.g)!.name;
    const nodes = options.ownNodes();
    connection.next('home.add');
    const envelopes = await Promise.all(
      nodes.map((node) => homeNotice(node, invitation, group, options.url)),
    );
    checkCancel(signal, 'home.add');
    const notices = notify(nodes, envelopes);
    connection.serve(ctl, welcome, groups);
    return { peer: connection.peer, notices };
  } catch (error) {
    connection.close();
    throw connection.error(error);
  } finally {
    connection.stop();
    stop();
  }
}

async function admit(
  ctl: Ctl,
  welcome: Welcome,
  { g }: Invitation,
  name: string,
  { device, log }: JoinOptions,
  next: (step: JoinError['step']) => void,
) {
  if (welcome.v !== 2) throw new JoinError('hello', 'version');
  next('devlog');
  presentLog(ctl, log);
  next('join');
  const env = await device.sign(JSON.stringify({ t: 'join', n: newId(), ts: Date.now(), g, name }));
  const answer = await ctl.request({ t: 'cmd', env });
  if (answer.err) throw new JoinError('join', answer.err.code);
}
