// A browser's RTCPeerConnection and its data channels as far as peer.ts uses them, for the unit
// tests of src/lib/peer.ts and the steps of ../features/web_peer.feature. A real one cannot run
// under the unit-test runner; the three browsers run it in unit I-8.

/** One data channel: what was sent on it, and the events a browser would fire. */
export class FakeChannel {
  readyState = 'connecting';
  /** Every message handed to send, in order. */
  sent: string[] = [];
  /** A channel that is closing throws on send, as a browser's does. */
  closing = false;
  onopen!: () => void;
  onclose!: () => void;
  onmessage: ((event: { data: unknown }) => void) | null = null;
  constructor(readonly label: string) {}
  send(text: string) {
    if (this.closing) throw new DOMException('the channel is closing', 'InvalidStateError');
    this.sent.push(text);
  }
  open() {
    this.readyState = 'open';
    this.onopen();
  }
  /** A message from the node: an object as its JSON, anything else as it is. */
  deliver(message: unknown) {
    const data =
      message !== null && typeof message === 'object' ? JSON.stringify(message) : message;
    this.onmessage?.({ data });
  }
  frames() {
    return this.sent.map((text) => JSON.parse(text));
  }
}

type Stats = Map<string, Record<string, unknown>>;

/** One peer connection. Its promises resolve at once unless a test holds them. */
export class FakePC {
  /** Every connection made since the last reset, oldest first. */
  static made: FakePC[] = [];
  /** The next createOffer fails, as it does in a browser without WebRTC. */
  static refuseOffer = false;
  static reset() {
    FakePC.made = [];
    FakePC.refuseOffer = false;
  }

  channels: FakeChannel[] = [];
  local: RTCSessionDescriptionInit | undefined;
  remote: RTCSessionDescriptionInit | undefined;
  /** Every candidate handed to addIceCandidate; none may come before the remote description. */
  added: RTCIceCandidateInit[] = [];
  early = 0;
  closes = 0;
  connectionState = 'new';
  /** What getStats answers, and how often it was asked. */
  stats: Stats = new Map();
  statsAsked = 0;
  /** When set, getStats and setRemoteDescription wait for it. */
  hold: Promise<unknown> | undefined;
  onicecandidate!: (event: { candidate: unknown }) => void;
  onconnectionstatechange!: () => void;

  constructor(readonly config: RTCConfiguration) {
    FakePC.made.push(this);
  }
  get ctl() {
    return this.channels[0];
  }
  createDataChannel(label: string) {
    const channel = new FakeChannel(label);
    this.channels.push(channel);
    return channel;
  }
  async createOffer() {
    if (FakePC.refuseOffer) throw new Error('no offer');
    return { type: 'offer', sdp: `offer ${FakePC.made.indexOf(this) + 1}` };
  }
  async setLocalDescription(description: RTCSessionDescriptionInit) {
    this.local = description;
  }
  async setRemoteDescription(description: RTCSessionDescriptionInit) {
    await this.hold;
    if (description.sdp === 'unreadable') throw new Error('unreadable description');
    this.remote = description;
  }
  async addIceCandidate(candidate: RTCIceCandidateInit) {
    if (!this.remote) this.early += 1;
    this.added.push(candidate);
    if (candidate.candidate === 'refused') throw new Error('refused candidate');
  }
  async getStats() {
    this.statsAsked += 1;
    await this.hold;
    if (this.stats.has('broken')) throw new Error('no statistics');
    return this.stats;
  }
  close() {
    this.closes += 1;
  }
  /** The browser gathered a candidate, or reports the end of gathering with null. */
  gather(candidate: { candidate: string; sdpMid: string | null } | null) {
    this.onicecandidate({ candidate });
  }
  becomes(state: string) {
    this.connectionState = state;
    this.onconnectionstatechange();
  }
}

/** Statistics as Chromium and WebKit give them: a transport, wherever it stands, names the
 *  selected pair. */
export function transportStats(local: string, remote: string, rtt?: number): Stats {
  return new Map<string, Record<string, unknown>>([
    ['P1', { type: 'candidate-pair', id: 'P1', localCandidateId: 'R', remoteCandidateId: 'R' }],
    [
      'P2',
      {
        type: 'candidate-pair',
        id: 'P2',
        localCandidateId: 'L',
        remoteCandidateId: 'M',
        currentRoundTripTime: rtt,
      },
    ],
    ['L', { type: 'local-candidate', id: 'L', candidateType: local }],
    ['M', { type: 'remote-candidate', id: 'M', candidateType: remote }],
    ['R', { type: 'local-candidate', id: 'R', candidateType: 'relay' }],
    ['T', { type: 'transport', id: 'T', selectedCandidatePairId: 'P2' }],
  ]);
}
