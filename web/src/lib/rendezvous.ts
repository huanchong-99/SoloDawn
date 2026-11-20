import * as v from 'valibot';
import { openSignal, utf8, type Device, type Known } from './identity';
import {
  Id,
  RVChallenge,
  RVFrom,
  RVICEAnswer,
  RVJoined,
  RVOffline,
  RVPresence,
  RVReady,
  RVRoom,
  type RVAuth,
  type RVICERequest,
  type RVJoin,
  type RVTo,
  type RVWatch,
} from './proto';

type Outgoing = RVTo | RVWatch | RVJoin | RVRoom | RVICERequest;
const incoming = v.variant('t', [RVFrom, RVOffline, RVPresence, RVJoined, RVRoom, RVICEAnswer]);
type Incoming = v.InferOutput<typeof incoming> | RVReady | { t: 'closed' };
type Receive = (url: string, frame: Incoming) => void;
type ICEServer = RVReady['ice'][number];

/** The largest frame, in UTF-8 bytes (contracts/rendezvous.md section 2). */
const maxFrame = 16_384;
/** A path sends at most `burst` frames in any `period` milliseconds, below the 100 of the
 *  contract and the 120 at which the server closes it, as a node does (internal/peer/path.go). */
const burst = 90;
const period = 10_000;

const closed = { t: 'closed' } as const;

// An ICE URL as pion's stun.ParseURI takes it (internal/peer/path.go, usable): a host, an
// optional port, and for TURN at most the transport udp or tcp. Schemes are lower case.
const hostPort = String.raw`(?:\[[0-9A-Fa-f:.]+\]|[^\s:/?#[\]@]+)(?::[0-9]+)?`;
const stunURL = new RegExp(`^stuns?:${hostPort}$`);
const turnURL = new RegExp(`^turns?:${hostPort}(?:\\?transport=(?:udp|tcp))?$`);
const relayed = (url: string) => turnURL.test(url);
const parses = (url: string) => stunURL.test(url) || relayed(url);

/** Whether the browser can use an ICE entry: Go drops an entry with a URL pion cannot read, or a
 *  TURN URL without a username. A browser also throws on an empty URL list and on TURN without a
 *  credential (WebRTC, "validate an ICE server"), so those go too. */
function usable({ urls, username, credential }: ICEServer): boolean {
  if (urls.length === 0 || !urls.every(parses)) return false;
  return !urls.some(relayed) || (Boolean(username) && credential !== undefined);
}

/** A `ready` or `ice` answer with only its usable entries, or undefined for a ttl below 1, which
 *  Go refuses too. */
function credentials<F extends RVReady | RVICEAnswer>(frame: F): F | undefined {
  return frame.ttl < 1 ? undefined : { ...frame, ice: frame.ice.filter(usable) };
}

function read(data: string): unknown {
  try {
    return JSON.parse(data);
  } catch {
    // A frame that is no JSON is read as nothing, which no schema accepts.
  }
}

const SignalConn = v.object({ conn: Id });

/** One signalling path. Its frames, and then its `closed`, reach receive in the order they
 *  arrived; after close() nothing does. */
function path(url: string, device: Device, known: Known, receive: Receive) {
  let silent = false;
  let delivered = Promise.resolve();
  // A from waits for its checks; the frames after it wait for it.
  const deliver = (frame: Incoming | undefined | Promise<Incoming | undefined>) => {
    delivered = delivered.then(async () => {
      const f = await frame;
      if (f !== undefined && !silent) receive(url, f);
    });
  };

  let socket: WebSocket;
  try {
    socket = new WebSocket(url);
  } catch {
    // The browser refuses the URL: ws:// from an HTTPS page, or no URL at all.
    deliver(closed);
    return {
      send() {},
      close() {
        silent = true;
      },
    };
  }

  let ready = false;
  let ended = false;
  const queue: string[] = []; // waits for ready, then for the pace
  // When each of the last `burst` frames went out, oldest at `next`, as Go's pacer keeps them.
  const sent = new Float64Array(burst).fill(-Infinity);
  let next = 0;
  let timer: ReturnType<typeof setTimeout> | undefined;

  function transmit(text: string) {
    sent[next] = Date.now();
    next = (next + 1) % burst;
    socket.send(text);
  }

  // How long the next frame waits until the frame `burst` places back is `period` old.
  const delay = () => sent[next] + period - Date.now();

  function drain() {
    while (ready && queue.length > 0) {
      const wait = delay();
      if (wait > 0) return schedule(wait);
      transmit(queue.shift()!);
    }
  }

  // One timer at a time drains the queue when the next place is free.
  function schedule(wait: number) {
    timer ??= setTimeout(() => {
      timer = undefined;
      drain();
    }, wait);
  }

  // A timer still due finds the queue empty.
  function end() {
    ended = true;
    queue.length = 0;
  }

  socket.onmessage = async ({ data }) => {
    const challenge = v.safeParse(RVChallenge, read(data));
    if (!challenge.success) return socket.close();
    const s = await device.signChallenge(challenge.output.nonce);
    socket.onmessage = onReady;
    transmit(JSON.stringify({ t: 'auth', k: device.id, s } satisfies RVAuth));
  };

  function onReady({ data }: MessageEvent) {
    const frame = v.safeParse(RVReady, read(data));
    const usable = frame.success ? credentials(frame.output) : undefined;
    if (usable === undefined) return socket.close();
    socket.onmessage = message;
    ready = true;
    drain();
    deliver(usable);
  }

  function message({ data }: MessageEvent) {
    const result = v.safeParse(incoming, read(data));
    if (result.success) deliver(accept(result.output));
  }

  function accept(frame: v.InferOutput<typeof incoming>) {
    if (frame.t === 'from') return accepts(frame).then((ok) => (ok ? frame : undefined));
    return frame.t === 'ice' ? credentials(frame) : frame;
  }

  // The relay's sender must be the signer, and conn a random id, as on a node
  // (internal/peer/signal.go).
  const accepts = async ({ k, env }: RVFrom) =>
    k === env.k &&
    'header' in (await openSignal(crypto, env, device.id, known, Date.now())) &&
    v.is(SignalConn, JSON.parse(env.p));

  socket.onclose = () => {
    end();
    deliver(closed);
  };

  return {
    send(text: string) {
      if (ended) return;
      queue.push(text);
      drain();
    },
    close() {
      silent = true;
      end();
      socket.close();
    },
  };
}

/** Signs in on each signalling URL (rendezvous and local node; contracts/rendezvous.md 2) and
 *  sends every frame on each of them, at most 90 frames in 10 seconds per path. receive gets
 *  each path's frames with its URL, in the order they arrived, and `closed` when the path ends,
 *  including a URL the browser refuses to open. A `from` reaches it only when its `k` is the
 *  envelope's signer, its envelope passes the receiver's checks for a signal and its `conn` is
 *  a random id. `ready` and `ice` carry only the ICE entries the browser can use; a `ready` with
 *  a ttl below 1 closes its path, and such an `ice` is dropped. Room data still needs the
 *  pairing mac check. A path does not reopen: the caller closes the client, after which it
 *  delivers nothing, and makes a new one. */
export function rendezvous(
  urls: readonly string[],
  device: Device,
  known: Known,
  receive: Receive,
) {
  const paths = Array.from(new Set(urls), (url) => path(url, device, known, receive));
  return {
    /** Throws a RangeError for a frame of more than 16 KiB, which the server would close the
     *  path for. */
    send(frame: Outgoing) {
      const text = JSON.stringify(frame);
      if (utf8(text).length > maxFrame) throw new RangeError('signalling frame exceeds 16 KiB');
      for (const path of paths) path.send(text);
    },
    close() {
      for (const path of paths) path.close();
    },
  };
}
