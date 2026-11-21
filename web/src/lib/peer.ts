// The browser's connections to nodes: the offering side of planning/flows/connect.md sections A
// and C, over the signalling of rendezvous.ts, with the ctl channel of
// planning/contracts/datachannel.md sections 1 to 3. internal/peer is the node's side of the same
// protocol. The relay-free probe and the move of section B are built on a Peer's dial, hold and
// adopt.
import * as v from 'valibot';
import { encode, utf8, type Device, type Known } from './identity';
import {
  Answer,
  decodeAnswer,
  Frame,
  ICECandidate,
  type AnswerTo,
  type Ctl,
  type Devlog,
  type Envelope,
  type ErrorResponse,
  type Push,
  type RequestType,
  type RVJoin,
  type RVReady,
  type RVRoom,
  type Welcome,
} from './proto';
import { rendezvous } from './rendezvous';

const second = 1000;
const minute = 60 * second;
/** How long an offer waits for its answer, a hello for its welcome and a request for fresh ICE
 *  servers for its answer (flows/connect.md step 15, internal/peer/path.go). */
const patience = 10 * second;
/** The largest ctl message, in UTF-8 bytes (contracts/conventions.md section 4). */
const ctlLimit = 61_440;
/** The candidates one connection takes, held or applied, and the keys one watch names
 *  (contracts/rendezvous.md sections 2 and 2.3). */
const most = 64;
/** The longest wait a browser's timer takes; a longer one would end at once. */
const longest = 2 ** 31 - 1;

type Client = ReturnType<typeof rendezvous>;
/** A frame of a signalling path, as rendezvous.ts hands it on. */
export type Heard = Parameters<Parameters<typeof rendezvous>[3]>[1];
type ICEServer = RVReady['ice'][number];
type Timer = ReturnType<typeof setTimeout>;
type Request = { t: RequestType; [field: string]: unknown };
type Response = AnswerTo<RequestType> | ErrorResponse;
/** One entry of the browser's statistics, whose fields depend on its type. */
type Stat = Record<string, any>;

/** How a connection reaches its node, as this browser's own statistics show it
 *  (flows/connect.md step 8). */
export interface Path {
  /** `relay` when either candidate of the selected pair is a relay candidate. */
  kind: 'direct' | 'relay';
  /** The latest round trip of that pair in milliseconds, where the browser reports one. */
  rtt: number | undefined;
  /** When a path of this kind was first read, in Unix milliseconds. */
  since: number;
}

/** What hears one connection of a node, from its welcome until it is gone. A node's next
 *  connection gets a listener of its own, so nothing of an earlier one is applied to it. */
export interface Listener {
  /** The node answered hello on this connection, which serves from now on. Requests go to ctl,
   *  which stays the same for every connection of that node. */
  online(ctl: Ctl, welcome: Welcome): void;
  push(frame: Push): void;
  /** The connection is gone, or another took its place. */
  offline(): void;
  /** The node refused this browser's hello with `version` or `forbidden`, the code given here.
   *  No connection serves this listener, and the peer offers that node nothing more: only a new
   *  peer tries again. */
  refused?(code: string): void;
}

export interface Options {
  /** The signalling addresses: rendezvous URLs and, from a page a node serves, that node. */
  urls: readonly string[];
  device: Device;
  /** Whose signals are listened to (contracts/envelope.md section 3, check 2). */
  known: Known;
  /** This browser's name, as its hello gives it. */
  name: string;
  /** Every frame of every signalling path, after this module has used it: pairing reads `ready`,
   *  `joined` and `room` here. */
  heard?: (url: string, frame: Heard) => void;
}

const Signal = v.variant('t', [Answer, ICECandidate]);

/** The refusals of a hello that trying again cannot change (contracts/datachannel.md section 9):
 *  the node does not speak this protocol version, or does not accept this device. */
const final = (code: string | undefined) => code === 'version' || code === 'forbidden';

const newId = () => encode(crypto.getRandomValues(new Uint8Array(16)));

/** What a request is rejected with when its connection ended before the response. */
class Lost extends Error {}
const lost = () => new Lost('connection lost');

/** How long something that opened at `since` has been open, in ms: 0 when it never opened. */
const elapsed = (since: number | undefined) =>
  since === undefined ? 0 : performance.now() - since;

/** A message as an object. What is no JSON, as a binary message is none, and what is no object
 *  have none of a frame's fields. */
function read(data: string): { id?: unknown; re?: unknown } {
  try {
    return Object(JSON.parse(data));
  } catch {
    return {};
  }
}

/** When to try again (flows/connect.md steps 15 and 16): after 1, 2, 4 … 60 seconds. */
class Retry {
  #delay = second;
  #timer: Timer | undefined;

  constructor(private readonly run: () => void) {}

  /** Runs after the back-off. `open` is how long what just ended stayed open, in ms: after 60
   *  seconds of it the series starts again. */
  after(open: number) {
    if (open >= minute) this.#delay = second;
    this.#timer = setTimeout(() => {
      this.#timer = undefined;
      this.#delay = Math.min(2 * this.#delay, minute);
      this.run();
    }, this.#delay);
  }

  /** Ends a wait at once and starts the series again: the other side was reported present, or
   *  this side's network changed. Does nothing when nothing waits. */
  now() {
    if (this.#timer === undefined) return;
    this.stop();
    this.#delay = second;
    this.run();
  }

  stop() {
    clearTimeout(this.#timer);
    this.#timer = undefined;
  }
}

/** The candidate pair in use: the one the transport names (Chromium, WebKit), or the one marked
 *  `selected` where the statistics have no transport (Firefox). */
function selected(stats: RTCStatsReport): Stat | undefined {
  const all: Stat[] = [...stats.values()];
  const named = all.find((s) => s.type === 'transport')?.selectedCandidatePairId;
  return stats.get(named) ?? all.find((s) => s.type === 'candidate-pair' && s.selected);
}

const milliseconds = (seconds: unknown) =>
  typeof seconds === 'number' ? Math.round(seconds * second) : undefined;

/** Whether either candidate of a pair is a relay candidate. One the browser hides is none. */
const relayed = (stats: RTCStatsReport, pair: Stat) =>
  [pair.localCandidateId, pair.remoteCandidateId].some(
    (id) => stats.get(id)?.candidateType === 'relay',
  );

/** The path the statistics show, or undefined while no pair is selected. `before` is the path
 *  read last: a kind that stays keeps its time. */
function pathOf(stats: RTCStatsReport, before: Path | undefined): Path | undefined {
  const pair = selected(stats);
  if (!pair) return undefined;
  const kind = relayed(stats, pair) ? 'relay' : 'direct';
  const since = before?.kind === kind ? before.since : Date.now();
  return { kind, rtt: milliseconds(pair.currentRoundTripTime), since };
}

/** What a connection and a peer need of the session that made them. */
interface Hub {
  readonly device: Device;
  readonly name: string;
  /** Whether a signalling path is signed in. */
  ready(): boolean;
  /** The ICE servers of every path, without the relays for a probe. */
  servers(probe: boolean): ICEServer[];
  /** Sends a signal on every path that is signed in, and names those paths. */
  signal(target: string, env: Envelope): string[];
  add(conn: Connection): void;
  drop(conn: Connection): void;
  /** The connections of `peer` that are not gone. */
  dialled(peer: Peer): Connection[];
  unlink(peer: Peer): void;
}

/** One offered connection to a node, with its ctl channel: an attempt until ctl opens, and gone
 *  for good once it closes. */
export class Connection {
  /** The random id that tells this connection's signals from every other's. */
  readonly conn = newId();
  /** The node it is offered to. */
  readonly target: string;
  /** Resolves when ctl opens. */
  readonly opened: Promise<void>;
  /** Resolves when the connection is gone, with how long its ctl stayed open in ms. */
  readonly closed: Promise<number>;
  /** The path read last, or undefined: ctl is not open, or no pair can be read. */
  path: Path | undefined;
  /** The code with which the node refused this connection's hello, when it did. */
  refused: string | undefined;
  /** Hears the node's pushes. */
  onpush: (frame: Push) => void = () => {};
  /** Hears the path each time it is read: when ctl opens, and every second after. */
  onpath: (path: Path | undefined) => void = () => {};

  readonly #hub: Hub;
  readonly #pc: RTCPeerConnection;
  readonly #ctl: RTCDataChannel;
  readonly #opening = Promise.withResolvers<void>();
  readonly #closing = Promise.withResolvers<number>();
  readonly #deadline: Timer;
  /** The paths the offer went out on that have not reported the node offline. */
  #paths = new Set<string>();
  #answered = false;
  /** Every distinct candidate of the node, and those that wait for its answer. */
  readonly #seen = new Set<string>();
  #held: RTCIceCandidateInit[] | undefined = [];
  #next = 0;
  readonly #waiting = new Map<
    number,
    { t: RequestType; resolve: (answer: Response) => void; reject: (e: Error) => void }
  >();
  #since: number | undefined;
  #poll: Timer | undefined;
  #gone = false;

  constructor(
    hub: Hub,
    /** The peer that dialled it. */
    readonly owner: Peer,
    probe: boolean,
  ) {
    this.#hub = hub;
    this.target = owner.target;
    this.opened = this.#opening.promise;
    this.closed = this.#closing.promise;
    this.#pc = new RTCPeerConnection({ iceServers: hub.servers(probe) });
    this.#ctl = this.#pc.createDataChannel('ctl');
    this.#ctl.onopen = () => {
      this.#since = performance.now();
      this.#poll = setInterval(this.#look, second);
      this.#look();
      this.#opening.resolve();
    };
    this.#ctl.onmessage = ({ data }) => this.#deliver(data);
    this.#ctl.onclose = this.close;
    this.#pc.onconnectionstatechange = () => {
      if (this.#pc.connectionState === 'failed') this.close();
    };
    this.#pc.onicecandidate = this.#trickle;
    this.#deadline = setTimeout(this.close, patience);
    hub.add(this);
    this.#offer(probe).catch(this.close);
  }

  /** The description goes out as soon as it is set, and nobody waits for the candidates. */
  async #offer(probe: boolean) {
    const offer = await this.#pc.createOffer();
    await this.#pc.setLocalDescription(offer);
    this.#paths = new Set(await this.#signal({ t: 'offer', sdp: offer.sdp, probe }));
  }

  /** Each candidate goes out when it is gathered. No end of candidates is sent: neither the
   *  null event nor an empty candidate. */
  readonly #trickle = ({ candidate }: { candidate: RTCIceCandidate | null }) => {
    if (!candidate?.candidate) return;
    const { candidate: cand, sdpMid } = candidate;
    this.#signal({ t: 'ice', cand, mid: sdpMid ?? '' }).catch(this.close);
  };

  async #signal(fields: object) {
    const header = { n: newId(), ts: Date.now(), to: this.target, conn: this.conn };
    const env = await this.#hub.device.sign(JSON.stringify({ ...fields, ...header }));
    return this.#hub.signal(this.target, env);
  }

  /** A signal of this connection that passed the receiver's checks of rendezvous.ts: the node's
   *  answer, of which the first copy is used, or one of its candidates. */
  signal(payload: unknown) {
    const signal = v.safeParse(Signal, payload);
    if (!signal.success) return;
    if (signal.output.t === 'ice') return this.#candidate(signal.output);
    this.#answer(signal.output.sdp).catch(this.close);
  }

  async #answer(sdp: string) {
    if (this.#answered) return;
    this.#answered = true;
    clearTimeout(this.#deadline);
    await this.#pc.setRemoteDescription({ type: 'answer', sdp });
    const held = this.#held!;
    this.#held = undefined;
    held.forEach(this.#add);
  }

  /** Each distinct candidate is applied once, and only after the answer: until then it is held
   *  (contracts/rendezvous.md section 2.3, "Trickle"). */
  #candidate({ cand, mid }: v.InferOutput<typeof ICECandidate>) {
    const id = JSON.stringify([mid, cand]);
    if (this.#seen.has(id) || this.#seen.size === most) return;
    this.#seen.add(id);
    const init = { candidate: cand, sdpMid: mid };
    if (this.#held) this.#held.push(init);
    else this.#add(init);
  }

  /** A candidate the browser refuses is dropped. */
  readonly #add = (init: RTCIceCandidateInit) => {
    this.#pc.addIceCandidate(init).catch(() => {});
  };

  /** A signalling path reported the node offline, or closed. An offer still unanswered when
   *  every path it went out on has done so is retired at once (flows/connect.md step 15). */
  offline(url: string) {
    if (!this.#answered && this.#paths.delete(url) && this.#paths.size === 0) this.close();
  }

  readonly #look = () => {
    this.#read().catch(() => {});
  };

  async #read() {
    const stats = await this.#pc.getStats();
    if (this.#gone) return;
    this.path = pathOf(stats, this.path);
    this.onpath(this.path);
  }

  /** Whether the connection has ended. */
  get gone() {
    return this.#gone;
  }

  /** Sends a request and answers its response. Rejects when the connection ends first, and when
   *  the response is not an answer of that request's kind. Throws a RangeError for a request that
   *  no id could fit into a ctl message. */
  request(req: Request): Promise<Response> {
    const largest = JSON.stringify({ ...req, id: Number.MAX_SAFE_INTEGER });
    if (utf8(largest).length > ctlLimit) throw new RangeError('ctl frame exceeds 60 KiB');
    return new Promise((resolve, reject) => {
      if (this.#gone) return reject(lost());
      const id = (this.#next += 1);
      this.#waiting.set(id, { t: req.t, resolve, reject });
      this.#send({ ...req, id });
    });
  }

  /** Says hello on an open ctl and answers the node's welcome. When the node refuses, or has
   *  not answered within 10 seconds, the connection closes and the promise rejects. */
  async hello(): Promise<Welcome> {
    const timer = setTimeout(this.close, patience);
    const hello = { t: 'hello' as const, v: 1, kind: 'browser', name: this.#hub.name, addrs: [] };
    try {
      const answer = (await this.request(hello)) as Welcome | ErrorResponse;
      if (!answer.err) return answer;
      this.refused = answer.err.code;
      throw new Error(`hello refused: ${this.refused}`);
    } catch (error) {
      this.close();
      throw error;
    } finally {
      clearTimeout(timer);
    }
  }

  /** Another data channel of this connection, such as `pty:<g>:<member>`
   *  (contracts/datachannel.md section 1). It ends with the connection. */
  channel(label: string) {
    return this.#pc.createDataChannel(label);
  }

  /** Sends a push on an open ctl. Throws a RangeError when the frame exceeds 60 KiB. */
  send(frame: Devlog) {
    if (utf8(JSON.stringify(frame)).length > ctlLimit) {
      throw new RangeError('ctl frame exceeds 60 KiB');
    }
    if (!this.#gone) this.#send(frame);
  }

  /** A channel that cannot send is closing: the connection is gone. */
  #send(frame: object) {
    try {
      this.#ctl.send(JSON.stringify(frame));
    } catch {
      this.close();
    }
  }

  /** contracts/datachannel.md section 2 and contracts/conventions.md section 3: a message with
   *  an `id` is a request, one with `re` a response, any other a push. What is none of them is
   *  dropped, and ctl stays open. */
  #deliver(data: string) {
    const frame = read(data);
    if (frame.id !== undefined) return this.#refuse(frame.id);
    if (frame.re !== undefined) return this.#settle(frame.re as number, data);
    const push = v.safeParse(Frame, frame);
    if (push.success) this.onpush(push.output as Push);
  }

  /** The browser serves no request: one whose id can be read is answered `bad_request`. */
  #refuse(id: unknown) {
    if (!Number.isSafeInteger(id)) return;
    this.#send({ re: id, err: { code: 'bad_request', msg: 'the web client serves no request' } });
  }

  /** A response goes to the request it names, and to none when it names none. It is read as
   *  the answer of that request's type (`decodeAnswer` of proto.ts). One that is no such answer
   *  fails the request: nothing else will answer it. */
  #settle(re: number, data: string) {
    const waiting = this.#waiting.get(re);
    // Taking the entry out is what claims it: a second response with the same `re` finds none.
    if (!this.#waiting.delete(re)) return;
    const { t, resolve, reject } = waiting!;
    try {
      resolve(decodeAnswer(t, data));
    } catch {
      reject(new Error('unreadable answer'));
    }
  }

  /** Ends the connection: nothing of it is applied from here on, and its requests reject. */
  readonly close = () => {
    if (this.#gone) return;
    this.#gone = true;
    clearTimeout(this.#deadline);
    clearInterval(this.#poll);
    this.#hub.drop(this);
    this.#ctl.onmessage = null;
    this.#pc.close();
    for (const { reject } of this.#waiting.values()) reject(lost());
    this.#closing.resolve(elapsed(this.#since));
  };
}

interface Ask {
  req: Request;
  resolve: (answer: Response) => void;
  reject: (error: Error) => void;
}

/** One node this browser keeps a connection to (flows/connect.md sections A and C). It offers,
 *  says hello, and after each loss offers again with the back-off of step 15, until the node
 *  refuses its hello with `version` or `forbidden`. It is the `Ctl` of
 *  that node for as long as it lives: a request whose connection is lost before its response is
 *  sent again after the next welcome under a new id (steps 12 and 18), except a `sync`, which
 *  rejects, because the listener reads again from its cursor after every welcome. A request made
 *  while no connection serves rejects at once, and every request rejects when the peer closes.
 *  A rejection is never an answer of the node: `ErrorResponse` is only what a node sent. */
export class Peer {
  readonly #hub: Hub;
  readonly #listen: () => Listener;
  readonly #onpath: (path: Path | undefined) => void;
  /** The connection that serves, the listener that hears it, and the attempt to get one. */
  #conn: Connection | undefined;
  #listener: Listener | undefined;
  #attempt: Connection | undefined;
  #closed = false;
  /** The requests without a response, in the order they were first made. */
  readonly #asked = new Set<Ask>();
  /** How long the connection that was lost stayed open, until its reconnect is scheduled. */
  #lost: number | undefined;
  #held = 0;

  constructor(
    hub: Hub,
    readonly target: string,
    listen: () => Listener,
    onpath: (path: Path | undefined) => void,
  ) {
    this.#hub = hub;
    this.#listen = listen;
    this.#onpath = onpath;
    this.#try();
  }

  /** An attempt needs a signalling path; without one it counts as failed. */
  readonly #try = () => {
    if (!this.#hub.ready()) return this.#retry.after(0);
    const conn = (this.#attempt = this.dial(false));
    conn.opened
      .then(() => conn.hello())
      .then(
        (welcome) => this.adopt(conn, welcome),
        () => {},
      );
  };
  readonly #retry = new Retry(this.#try);

  readonly request = ((req: Request) =>
    new Promise<Response>((resolve, reject) => {
      if (!this.#conn) return reject(lost());
      const ask = { req, resolve, reject };
      this.#put(ask);
      this.#asked.add(ask);
    })) as Ctl['request'];

  /** A request whose connection is lost stays asked; any other end is the request's own. */
  #put(ask: Ask) {
    this.#conn!.request(ask.req).then(
      (answer) => {
        this.#asked.delete(ask);
        ask.resolve(answer);
      },
      (error: Error) => {
        if (!(error instanceof Lost)) this.#fail(ask, error);
      },
    );
  }

  #fail(ask: Ask, error: Error = lost()) {
    this.#asked.delete(ask);
    ask.reject(error);
  }

  /** Offers this node a connection that is not the peer's until `adopt`: its own next attempt,
   *  or a probe, which is offered without the relays (contracts/rendezvous.md section 5). The
   *  peer closes it when it closes itself. A closed peer offers nothing: it throws. */
  dial(probe: boolean) {
    if (this.#closed) throw new Error('peer closed');
    const conn = new Connection(this.#hub, this, probe);
    conn.closed.then((open) => this.#ended(conn, open));
    return conn;
  }

  /** Makes a connection on which the node answered hello the one that serves. Every other
   *  connection the peer dialled is closed: the one before it, an attempt or a probe still on
   *  its way. Its listener hears `online`, and then the requests without a response are sent
   *  again. A closed peer adopts nothing: it closes the connection, so a welcome that comes
   *  late cannot bring it back. Nor is a connection adopted that is already gone: the one that
   *  serves stays. */
  adopt(conn: Connection, welcome: Welcome) {
    if (this.#closed || conn.gone) return conn.close();
    this.#retry.stop();
    this.#lost = undefined;
    this.#end(conn);
    this.#conn = conn;
    const unanswered = [...this.#asked];
    const listener = (this.#listener = this.#listen());
    conn.onpush = (frame) => listener.push(frame);
    conn.onpath = this.#onpath;
    this.#onpath(conn.path);
    listener.online(this, welcome);
    for (const ask of unanswered) this.#put(ask);
  }

  /** Until the function it returns is called, the loss of the connection starts no reconnect:
   *  a move waits for its welcome (flows/connect.md step 11.1). Then one starts, if the
   *  connection was lost meanwhile and none was adopted. Calling it again changes nothing. */
  hold() {
    this.#held += 1;
    let released = false;
    return () => {
      if (released) return;
      released = true;
      this.#held -= 1;
      this.#again();
    };
  }

  /** The node was reported present, or this browser's network changed: a reconnect that waits
   *  for its back-off starts at once (flows/connect.md step 16). */
  present() {
    this.#retry.now();
  }

  /** A data channel beside ctl on the connection that serves, or undefined while none does. */
  channel(label: string) {
    return this.#conn?.channel(label);
  }

  /** Sends a push on the connection that serves, if any. */
  send(frame: Devlog) {
    this.#conn?.send(frame);
  }

  /** Closes every connection it dialled, for good: nothing reconnects, nothing can be dialled
   *  or adopted, and every request rejects. */
  close() {
    this.#closed = true;
    this.#retry.stop();
    this.#lost = undefined;
    this.#end();
    for (const ask of this.#asked) this.#fail(ask);
    this.#hub.unlink(this);
  }

  /** Closes every connection the peer dialled but `keep`. Their end is then no loss. */
  #end(keep?: Connection) {
    if (this.#conn) this.#offline();
    this.#attempt = undefined;
    for (const conn of this.#hub.dialled(this)) if (conn !== keep) conn.close();
  }

  #offline() {
    this.#conn = undefined;
    this.#listener!.offline();
    this.#onpath(undefined);
    for (const ask of this.#asked) if (ask.req.t === 'sync') this.#fail(ask);
  }

  /** The end of a connection this peer dialled. Only that of the one that serves, or of its own
   *  attempt, leads to a reconnect: a probe and a connection that was replaced do not. Nor does
   *  an attempt whose hello the node refused for good: its listener hears why, and the peer
   *  stops. */
  #ended(conn: Connection, open: number) {
    if (conn === this.#conn) this.#offline();
    else if (conn !== this.#attempt) return;
    this.#attempt = undefined;
    if (final(conn.refused)) return this.#stop(conn.refused!);
    this.#lost = open;
    this.#again();
  }

  /** The node refused the peer for good. No welcome will come, so the requests that waited
   *  for the next one reject, and a listener hears the code. */
  #stop(code: string) {
    for (const ask of this.#asked) this.#fail(ask);
    this.#listen().refused?.(code);
  }

  #again() {
    if (this.#held > 0 || this.#lost === undefined) return;
    this.#retry.after(this.#lost);
    this.#lost = undefined;
  }
}

/** One signalling path: its client, the ICE servers it gave, and when it signed in. */
interface Way {
  url: string;
  client: Client;
  ice: ICEServer[];
  ready?: number;
  refresh?: Timer;
  retry: Retry;
}

/** The signalling paths and connections of one tab. */
export interface Session {
  /** Keeps a connection to a node. `listen` is called for each connection that comes to serve,
   *  and `onpath` with its path each time it is read, and with undefined when it is lost. A
   *  second peer for a node closes the first. */
  peer(target: string, listen: () => Listener, onpath?: (path: Path | undefined) => void): Peer;
  /** Sends a pairing frame on one signalling path. */
  send(url: string, frame: RVJoin | RVRoom): void;
  /** Closes every signalling path and connection; nothing opens again. */
  close(): void;
}

const signedIn = (way: Way) => way.ready !== undefined;
const relays = ({ urls }: ICEServer) => urls.some((url) => url.startsWith('turn'));

class Signalling implements Hub, Session {
  readonly device: Device;
  readonly name: string;
  readonly #known: Known;
  readonly #heard: (url: string, frame: Heard) => void;
  readonly #ways = new Map<string, Way>();
  readonly #peers = new Map<string, Peer>();
  readonly #conns = new Map<string, Connection>();

  constructor({ urls, device, known, name, heard = () => {} }: Options) {
    this.device = device;
    this.name = name;
    this.#known = known;
    this.#heard = heard;
    for (const url of new Set(urls)) {
      const reopen = () => (way.client = this.#open(url));
      const way: Way = { url, client: this.#open(url), ice: [], retry: new Retry(reopen) };
      this.#ways.set(url, way);
    }
    addEventListener('online', this.#online);
  }

  /** One client for each path, so that a path can open again by itself; the watch list waits in
   *  it until the path is signed in. */
  #open(url: string) {
    const client = rendezvous([url], this.device, this.#known, this.#hear);
    client.send(this.#watch());
    return client;
  }

  #watch() {
    return { t: 'watch', k: [...this.#peers.keys()].slice(0, most) } as const;
  }

  #watchAgain() {
    for (const way of this.#ways.values()) way.client.send(this.#watch());
  }

  readonly #on: { [T in Heard['t']]?: (way: Way, frame: Extract<Heard, { t: T }>) => void } = {
    ready: (way, frame) => {
      way.ready = performance.now();
      this.#fresh(way, frame);
    },
    ice: (way, frame) => this.#fresh(way, frame),
    // rendezvous.ts has checked the envelope, and that its payload has a conn.
    from: (_, { k, env }) => {
      const payload = JSON.parse(env.p);
      this.#conns.get(`${k} ${payload.conn}`)?.signal(payload);
    },
    offline: (way, { k }) => {
      for (const conn of this.#conns.values()) if (conn.target === k) conn.offline(way.url);
    },
    presence: (_, { k, on }) => {
      if (on) this.#peers.get(k)?.present();
    },
    closed: (way) => this.#shut(way),
  };

  readonly #hear = (url: string, frame: Heard) => {
    const on = this.#on[frame.t] as ((way: Way, frame: Heard) => void) | undefined;
    on?.(this.#ways.get(url)!, frame);
    this.#heard(url, frame);
  };

  /** The ICE servers of a path are asked for again at half their ttl, and every 10 seconds
   *  until they come (contracts/rendezvous.md section 2.1, internal/peer/path.go). */
  #fresh(way: Way, { ice, ttl }: { ice: ICEServer[]; ttl: number }) {
    way.ice = ice;
    clearTimeout(way.refresh);
    way.refresh = setTimeout(() => this.#ask(way), Math.min(ttl * 500, longest));
  }

  #ask(way: Way) {
    way.client.send({ t: 'ice' });
    way.refresh = setTimeout(() => this.#ask(way), patience);
  }

  /** A path that closed can bring no answer, so it counts as one that reported every node
   *  offline. It opens again with the back-off of a connection. */
  #shut(way: Way) {
    way.client.close();
    clearTimeout(way.refresh);
    way.ice = [];
    const open = elapsed(way.ready);
    way.ready = undefined;
    for (const conn of this.#conns.values()) conn.offline(way.url);
    way.retry.after(open);
  }

  /** This browser's network changed: nothing waits for its back-off (flows/connect.md step 16). */
  readonly #online = () => {
    for (const way of this.#ways.values()) way.retry.now();
    for (const peer of this.#peers.values()) peer.present();
  };

  ready() {
    return [...this.#ways.values()].some(signedIn);
  }

  servers(probe: boolean) {
    const all = [...this.#ways.values()].flatMap((way) => way.ice);
    return probe ? all.filter((server) => !relays(server)) : all;
  }

  signal(target: string, env: Envelope) {
    const ways = [...this.#ways.values()].filter(signedIn);
    for (const way of ways) way.client.send({ t: 'to', k: target, env });
    return ways.map((way) => way.url);
  }

  add(conn: Connection) {
    this.#conns.set(`${conn.target} ${conn.conn}`, conn);
  }

  drop(conn: Connection) {
    this.#conns.delete(`${conn.target} ${conn.conn}`);
  }

  dialled(peer: Peer) {
    return [...this.#conns.values()].filter((conn) => conn.owner === peer);
  }

  unlink(peer: Peer) {
    if (this.#peers.get(peer.target) !== peer) return;
    this.#peers.delete(peer.target);
    this.#watchAgain();
  }

  peer(
    target: string,
    listen: () => Listener,
    onpath: (path: Path | undefined) => void = () => {},
  ) {
    this.#peers.get(target)?.close();
    const peer = new Peer(this, target, listen, onpath);
    this.#peers.set(target, peer);
    this.#watchAgain();
    return peer;
  }

  send(url: string, frame: RVJoin | RVRoom) {
    this.#ways.get(url)?.client.send(frame);
  }

  close() {
    removeEventListener('online', this.#online);
    for (const way of this.#ways.values()) {
      way.retry.stop();
      clearTimeout(way.refresh);
      way.client.close();
    }
    for (const peer of [...this.#peers.values()]) peer.close();
  }
}

/** Opens this tab's signalling paths. Only the tab that holds the device's lock may call it
 *  (tablock.ts), and it closes the session when it loses the lock. */
export const connect = (options: Options): Session => new Signalling(options);
