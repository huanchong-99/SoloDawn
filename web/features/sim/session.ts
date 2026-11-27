// The signalling and the connections of the demo, in place of peer.ts's `connect`: the client's
// own code above them runs unchanged. A peer reaches a demo computer after a moment, on a direct
// path, unless that computer is down; every answer and push passes through JSON and the client's
// schemas, as on a real connection. Pairing rooms are answered for the invitations and the open
// invitation codes of the world, and, for onboarding, once the computer was given the code (the
// demo panel stands in for the AI on that computer to which the person gives it). A room nobody
// answers is given up after 20 seconds of the world's clock, not 10 minutes.
import { mac, room } from '../../src/lib/pairing';
import type { Heard, Listener, Options, Path, Peer, Session } from '../../src/lib/peer';
import * as p from '../../src/lib/proto';
import type { Line, SimNode } from './node';
import type { World } from './world';

type Hear = (url: string, frame: Heard) => void;
type Ask = { t: p.RequestType; [field: string]: unknown };

const later = (ms: number, fn: () => void) => setTimeout(fn, ms);
const roundTrip = <S extends p.RequestType>(t: S, answer: unknown) =>
  p.decodeAnswer(t, JSON.stringify(answer));

/** Every peer of the page, so that a computer that goes down or comes back is heard by them. */
const peers = new Set<FakePeer>();

/** Brings the peers in line with their computers: those of a computer that went down lose their
 *  connection, those of one that is back connect. */
export function refresh() {
  for (const peer of peers) peer.refresh();
}

class FakePeer {
  /** Hears that an attempt failed to reach the computer; the peer still tries again. */
  unreached = () => {};
  #line: Line | undefined;
  #listener: Listener | undefined;
  #closed = false;
  #trying = false;

  constructor(
    readonly world: World,
    readonly target: string,
    readonly listen: () => Listener,
    readonly onpath: (path: Path | undefined) => void,
    /** The group of the invitation or code this peer came by, which its `welcome` names. */
    readonly join?: string,
  ) {
    peers.add(this);
    this.#try(250);
  }

  #node() {
    return this.world.node(this.target);
  }

  /** A computer the page cannot reach: unknown, down, or, in the failing pairing, any not yet
   *  paired, as behind a firewall that drops every connection. */
  #unreachable(node: SimNode | undefined) {
    return !node || node.down || (this.world.mode === 'pairfail' && !node.paired);
  }

  #try(ms: number) {
    if (this.#closed || this.#trying || this.#line) return;
    this.#trying = true;
    later(ms, () => {
      this.#trying = false;
      void this.#open();
    });
  }

  async #open() {
    const node = this.#node();
    if (this.#closed || this.#line) return;
    if (this.#unreachable(node)) {
      if (node) later(1500, () => this.unreached());
      return this.#try(4000);
    }
    const line = node!.open((frame) => this.#push(frame));
    const hello = {
      t: 'hello',
      v: 3,
      kind: 'browser',
      name: 'Browser',
      addrs: [],
      join: this.join,
    };
    const welcome = roundTrip('hello', await line.request(hello));
    if (welcome.err) return this.listen().refused?.(welcome.err.code);
    this.#line = line;
    this.#listener = this.listen();
    const path = { kind: 'direct' as const, rtt: 14, since: Date.now() };
    this.onpath(path);
    this.#listener.online(this as unknown as p.Ctl, welcome);
    // The path is read again a moment later, as peer.ts reads the browser's statistics again: who
    // started following it after `online`, as a join does, hears it then.
    later(1000, () => this.#line === line && this.onpath(path));
  }

  #push(frame: p.Push) {
    try {
      this.#listener?.push(p.decode(p.Frame, JSON.stringify(frame)) as p.Push);
    } catch (error) {
      console.error('demo: a push the client cannot read', frame, error);
    }
  }

  refresh() {
    if (this.#unreachable(this.#node())) this.#drop();
    else this.#try(400);
  }

  #drop() {
    if (!this.#line) return;
    this.#line.close();
    this.#line = undefined;
    this.#listener?.offline();
    this.onpath(undefined);
    this.#try(4000);
  }

  async request(req: Ask) {
    const line = this.#line;
    if (!line) throw new Error('connection lost');
    const answer = await line.request(req as never);
    if (this.#line !== line) throw new Error('connection lost');
    return roundTrip(req.t, answer);
  }

  send(frame: p.Devlog | p.PTYResize) {
    this.#line?.send(frame);
  }

  channel(label: string) {
    return this.#line?.channel(label);
  }

  present() {}

  hold() {
    return () => {};
  }

  /** Closed for good: as peer.ts does, a connection that served is reported lost, so that its
   *  home is not asked anything more on it. */
  close() {
    this.#closed = true;
    peers.delete(this);
    this.#drop();
  }
}

/** A browser's pairing message waiting in a room for the computer's (contracts/envelope.md 6). */
interface Offer {
  url: string;
  hear: Hear;
}

/** The rooms of onboarding that wait for a computer to be given their code. */
const offers = new Map<string, Offer>();

/** The computer `node` was given the pairing code `secret`: it answers in that code's room with
 *  role `role`. Whether a browser was waiting there. */
export async function answerRoom(node: SimNode, secret: string, role: 'a' | 'b') {
  const id = await room(crypto, secret);
  const offer = offers.get(id);
  if (!offer) return false;
  offers.delete(id);
  const fields = { role, k: node.device, kind: 'node', name: node.name };
  const data = {
    t: 'pair' as const,
    ...fields,
    mac: await mac(crypto, secret, fields),
    exp: Date.now() + 600_000,
  };
  offer.hear(offer.url, { t: 'room', room: id, data });
  return true;
}

class FakeSession {
  #peers = new Map<string, FakePeer>();
  #rooms = new Set<{ url: string; id: string; heard: Hear }>();
  /** The homes that answered a room of an invitation or a code, with its group. */
  #joining = new Map<string, string>();
  #closed = false;

  get roomWait() {
    return 20_000 * this.world.pace;
  }

  constructor(
    readonly world: World,
    readonly options: Options,
  ) {
    for (const url of new Set(options.urls))
      later(50, () => this.#hear(url, { t: 'ready', ice: [], ttl: 600 } as Heard));
  }

  #hear = (url: string, frame: Heard) => {
    if (this.#closed) return;
    this.options.heard?.(url, frame);
    for (const each of this.#rooms) if (each.url === url) each.heard(url, frame);
  };

  peer(
    target: string,
    listen: () => Listener,
    onpath: (path: Path | undefined) => void = () => {},
  ) {
    this.#peers.get(target)?.close();
    const peer = new FakePeer(this.world, target, listen, onpath, this.#joining.get(target));
    this.#joining.delete(target);
    this.#peers.set(target, peer);
    return peer as unknown as Peer;
  }

  send(url: string, frame: p.RVJoin | p.RVRoom) {
    if (frame.t === 'join')
      return later(80, () => this.#hear(url, { t: 'joined', room: frame.room }));
    void this.#offered(url, frame);
  }

  /** A browser's pairing message: an invitation or an open code of the world is answered by its
   *  home at once; an onboarding room waits for the computer to be given the code. */
  async #offered(url: string, frame: p.RVRoom) {
    const { invites, codes } = this.world.memory;
    const open = Object.keys(codes).filter((g) => this.world.code(g));
    const secrets = [
      ...Object.entries(invites).map(([secret, { g }]) => [secret, g]),
      ...open.map((g) => [codes[g].code, g]),
    ];
    offers.set(frame.room, { url, hear: this.#hear });
    for (const [secret, g] of secrets) {
      if ((await room(crypto, secret)) !== frame.room) continue;
      const home = this.world.home(g)!;
      this.#joining.set(home.device, g);
      return later(600, () => void answerRoom(home, secret, 'a'));
    }
  }

  watchRoom(url: string, id: string, heard: Hear) {
    const watching = { url, id, heard };
    this.#rooms.add(watching);
    this.send(url, { t: 'join', room: id });
    return () => void this.#rooms.delete(watching);
  }

  close() {
    this.#closed = true;
    for (const peer of this.#peers.values()) peer.close();
  }
}

/** The `connect` of the demo: a session of the world in place of the signalling paths. */
export const fakeConnect =
  (world: World) =>
  (options: Options): Session =>
    new FakeSession(world, options) as unknown as Session;
