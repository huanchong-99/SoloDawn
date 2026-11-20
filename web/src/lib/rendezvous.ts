import * as v from 'valibot';
import { openSignal, type Device, type Known } from './identity';
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

function read(data: string): unknown {
  try {
    return JSON.parse(data);
  } catch {
    // A frame that is no JSON is read as nothing, which no schema accepts.
  }
}

function path(url: string, device: Device, known: Known, receive: Receive) {
  const socket = new WebSocket(url);
  // Frames sent before this path's ready wait here, so a slow path delays no other.
  let pending: string[] | undefined = [];

  socket.onmessage = async ({ data }) => {
    const challenge = v.safeParse(RVChallenge, read(data));
    if (!challenge.success) return socket.close();
    const s = await device.signChallenge(challenge.output.nonce);
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
    if (frame.t === 'from' && 'code' in (await opened(frame))) return;
    receive(url, frame);
  }

  const opened = ({ env }: RVFrom) => openSignal(crypto, env, device.id, known, Date.now());

  socket.onclose = () => receive(url, { t: 'closed' });

  return {
    send(text: string) {
      if (pending) pending.push(text);
      else socket.send(text);
    },
    close: () => socket.close(),
  };
}

/** Signs in on each signalling URL (rendezvous and local node; contracts/rendezvous.md 2) and
 *  sends every frame on each of them. receive gets each path's frames with its URL, and
 *  `closed` when the path closes. A `from` reaches it only when its envelope passes the
 *  receiver's checks for a signal; its sender is the envelope's `k`. Room data still needs the
 *  pairing mac check. */
export function rendezvous(
  urls: readonly string[],
  device: Device,
  known: Known,
  receive: Receive,
) {
  const paths = Array.from(new Set(urls), (url) => path(url, device, known, receive));
  return {
    send(frame: Outgoing) {
      const text = JSON.stringify(frame);
      for (const path of paths) path.send(text);
    },
    close() {
      for (const path of paths) path.close();
    },
  };
}
