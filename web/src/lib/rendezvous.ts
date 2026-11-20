import * as v from 'valibot';
import { openSignal, utf8, type Device, type Known } from './identity';
import {
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

const maxBytes = 16_384;

function read(data: unknown): unknown {
  if (typeof data !== 'string' || utf8(data).length > maxBytes) return;
  try {
    return JSON.parse(data);
  } catch {
    // No frame to dispatch.
  }
}

function path(url: string, device: Device, known: Known, receive: Receive) {
  const socket = new WebSocket(url);
  // A slow handshake queues only its own copies. It never delays another path.
  let pending: string[] | undefined = [];

  socket.onmessage = async ({ data }) => {
    const challenge = v.safeParse(RVChallenge, read(data));
    if (!challenge.success) return socket.close();
    socket.onmessage = () => socket.close();
    const s = await device.signChallenge(challenge.output.nonce);
    if (socket.readyState !== WebSocket.OPEN) return;
    socket.onmessage = ready;
    socket.send(JSON.stringify({ t: 'auth', k: device.id, s } satisfies RVAuth));
  };

  function ready({ data }: MessageEvent) {
    const frame = v.safeParse(RVReady, read(data));
    if (!frame.success) return socket.close();
    socket.onmessage = message;
    for (const text of pending!) socket.send(text);
    pending = undefined;
    receive(url, frame.output);
  }

  async function message({ data }: MessageEvent) {
    const result = v.safeParse(incoming, read(data));
    if (!result.success) return;
    const frame = result.output;
    if (frame.t === 'from' && !(await accepts(frame))) return;
    if (socket.readyState === WebSocket.OPEN) receive(url, frame);
  }

  async function accepts({ k, env }: RVFrom) {
    return k === env.k && 'header' in (await openSignal(crypto, env, device.id, known, Date.now()));
  }

  socket.onclose = () => receive(url, { t: 'closed' });

  return {
    send(text: string) {
      if (socket.readyState >= WebSocket.CLOSING) return;
      if (pending) pending.push(text);
      else socket.send(text);
    },
    close: () => socket.close(),
  };
}

/** Open the supplied signalling URLs (configured rendezvous and/or local node).
 * The peer owns the signed payload; room data still needs the pairing MAC check. */
export function rendezvous(
  urls: readonly string[],
  device: Device,
  known: Known,
  receive: Receive,
) {
  const paths = Array.from(new Set(urls), (url) => path(url, device, known, receive));
  return {
    send(frame: Outgoing) {
      if (frame.t === 'watch' && frame.k.length > 64) throw new RangeError('watch exceeds 64 ids');
      const text = JSON.stringify(frame);
      if (utf8(text).length > maxBytes) throw new RangeError('frame exceeds 16 KiB');
      for (const path of paths) path.send(text);
    },
    close() {
      for (const path of paths) path.close();
    },
  };
}
