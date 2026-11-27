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

/** What a person was given to join a group. A link names the group and its home
 *  (contracts/step-2/people.md section 4); a code is the secret alone, and the home's `pair`
 *  message and the `welcome` name them (4.1). */
export interface Invitation {
  secret: string;
  g?: string;
  home?: string;
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

/** A `#join=` link: its secret, group and home, read without contacting the home. */
export function readInvitation(hash: string): Required<Invitation> | undefined {
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

/** A code as a person typed it: white space is removed and the rest read as a typed pairing
 *  secret (contracts/step-2/people.md 2.6); undefined for a text that is no code. */
export function readCode(text: string): Invitation | undefined {
  try {
    return { secret: parseSecret(text.replace(/\s/g, '')) };
  } catch {}
}

interface JoinOptions {
  device: Device;
  log: DeviceLog;
  browserName: string;
  /** The signalling path of a link: this page's rendezvous. */
  url: string;
  /** Every signalling path this browser has, on each of which a code's room is joined. */
  urls: string[];
  session: Pick<Session, 'watchRoom' | 'send'>;
  dial(home: string, listen: () => Listener, onpath: (path: Path | undefined) => void): JoinPeer;
  signal: AbortSignal;
  /** Keeps the home that admitted this browser, with its name and the path it answered on. */
  remember(home: string, url: string, name: string): Promise<void>;
  link(home: string): Link;
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

type GroupHome = Required<Pick<Invitation, 'g' | 'home'>>;
/** An invitation with what the attempt learns: for a code, the home and the path it answered on
 *  from its `pair` message, the group from `welcome.join`. */
type Found = Invitation & { url: string };

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

/** Joins the room on the link's one path, or a code's on every path, until the home's `pair`
 *  message comes on one of them. */
function joinRoom(found: Found, options: JoinOptions, id: string, self: Pair, dial: () => void) {
  const { session } = options;
  let offered = false;
  const receivePair = async (url: string, data: Pair) => {
    if (!(await invitationPair(found, data)) || offered) return;
    offered = true;
    Object.assign(found, { home: data.k, url });
    dial();
  };
  const watch = (url: string) =>
    session.watchRoom(url, id, (path, frame) => {
      if (offered || path !== url) return;
      if (!inRoom(frame, id)) return;
      if (frame.t === 'room') void receivePair(url, frame.data);
      else session.send(url, { t: 'room', room: id, data: self });
    });
  const stops = (found.home ? [options.url] : options.urls).map(watch);
  return () => {
    offered = true;
    for (const stop of stops) stop();
  };
}

function inRoom(frame: Heard, id: string): frame is Extract<Heard, { t: 'joined' | 'room' }> {
  return 'room' in frame && frame.room === id;
}

/** A link names its home; a code takes the device that proves it holds the code. */
function invitationPair({ home, secret }: Invitation, data: Pair) {
  return (
    (home ?? data.k) === data.k && data.kind === 'node' && verifyPair(crypto, secret, data, 'a')
  );
}

function open(found: Found, options: JoinOptions, id: string, pair: Pair) {
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
  const stop = joinRoom(found, options, id, pair, () => {
    step = 'hello';
    peer = options.dial(
      found.home!,
      () => {
        const link = start && options.link(found.home!);
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
            if (frame.t === 'groups' && frame.groups.some((group) => group.g === found.g)) {
              listed.resolve(frame);
            }
          },
        };
      },
      options.onpath,
    );
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
      start = options.link(found.home!);
      start.online(ctl, welcome);
      start.push(groups);
    },
  };
}

/** Joins the group of a link or a code; answers with the group and the outcome of telling the
 *  person's nodes its home. */
export async function joinGroup(invitation: Invitation, name: string, options: JoinOptions) {
  const { device, log, signal, browserName } = options;
  if (!log.valid(device.id)) throw new JoinError('onboarding', 'unpaired');
  const fields = { role: 'b' as const, k: device.id, kind: 'browser' as const, name: browserName };
  const id = await room(crypto, invitation.secret);
  const pair: Pair = { t: 'pair', ...fields, mac: await mac(crypto, invitation.secret, fields) };
  checkCancel(signal, 'room');
  const found: Found = { ...invitation, url: options.url };
  const connection = open(found, options, id, pair);
  const stop = abortPending(signal, (code) => {
    connection.failed(code);
    connection.stop();
    connection.close();
  });
  try {
    const { ctl, welcome } = await connection.ready;
    found.g ??= welcome.join;
    await admit(ctl, welcome, found, name, options, connection.next);
    connection.next('remember');
    await options.remember(found.home!, found.url, welcome.name);
    checkCancel(signal, 'remember');
    connection.next('groups');
    const groups = await (welcome.groups.some((group) => group.g === found.g)
      ? { t: 'groups' as const, groups: welcome.groups }
      : connection.listed);
    const group = groups.groups.find((group) => group.g === found.g)!.name;
    const nodes = options.ownNodes();
    connection.next('home.add');
    const envelopes = await Promise.all(
      nodes.map((node) => homeNotice(node, found as GroupHome, group, found.url)),
    );
    checkCancel(signal, 'home.add');
    const notices = notify(nodes, envelopes);
    connection.serve(ctl, welcome, groups);
    return { g: found.g!, peer: connection.peer, notices };
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
  if (welcome.v !== 3) throw new JoinError('hello', 'version');
  next('devlog');
  presentLog(ctl, log);
  next('join');
  const env = await device.sign(JSON.stringify({ t: 'join', n: newId(), ts: Date.now(), g, name }));
  const answer = await ctl.request({ t: 'cmd', env });
  if (answer.err) throw new JoinError('join', answer.err.code);
}
