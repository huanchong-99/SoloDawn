// Evaluated in the actual browser by Playwright, on a localhost origin.
const sessions = new Map();
const wait = (ms) => new Promise((r) => setTimeout(r, ms));
const deadline = (p, event, ms = 15000) =>
  Promise.race([
    p,
    wait(ms).then(() => {
      throw Error(event + ' timeout');
    }),
  ]);
const event = (o, name) => new Promise((r) => o.addEventListener(name, r, { once: true }));
const bytes = (s) => new TextEncoder().encode(s).length;
const candidates = (s) => s.split(/\r?\n/).filter((l) => l.startsWith('a=candidate:'));
const stun = { urls: 'stun:10.71.0.2:3478' };
const turn = { urls: 'turn:10.71.0.2:3478?transport=udp', username: 's1', credential: 's1' };
let ws;
async function join(k) {
  ws = new WebSocket('ws://' + location.host + '/rv?k=' + k);
  await event(ws, 'message');
}

async function openCtl(mode) {
  const conn = await newID();
  const iceServers =
    mode === 'probe'
      ? [stun]
      : mode === 'silent'
        ? [stun, turn, { urls: 'stun:10.71.0.2:3479' }]
        : ['all', 'renom', 'relay'].includes(mode)
          ? [stun, turn]
          : [];
  const pc = new RTCPeerConnection({ iceServers, iceTransportPolicy: mode === 'relay' ? 'relay' : 'all' });
  const dc = pc.createDataChannel('ctl');
  sessions.set(conn, { pc, dc });
  const opened = event(dc, 'open');
  const offer = await pc.createOffer(),
    start = performance.now();
  let firstMs;
  pc.onicecandidate = (e) => {
    if (e.candidate) firstMs ??= Math.round(performance.now() - start);
  };
  const gathered = new Promise((r) =>
    pc.addEventListener('icegatheringstatechange', () => {
      if (pc.iceGatheringState === 'complete') r();
    }),
  );
  await pc.setLocalDescription(offer);
  await deadline(gathered, 'gather', 60000);
  const gatherMs = Math.round(performance.now() - start),
    sdp = pc.localDescription.sdp;
  const frame = await signOffer({
    t: 'offer',
    conn,
    probe: mode === 'probe',
    to: await (await fetch('/target')).text(),
    sdp,
  });
  const body = JSON.stringify(frame);
  const reply = event(ws, 'message');
  ws.send(body);
  const raw = (await deadline(reply, 'answer')).data,
    answer = await readAnswer(JSON.parse(raw));
  if (answer.conn !== conn) throw Error('conn mismatch');
  await evidence({
    mode,
    conn,
    firstMs,
    gatherMs,
    signedOffer: bytes(JSON.stringify(frame.env)),
    offerFrame: bytes(body),
    answerFrame: bytes(raw),
    signedAnswer: bytes(JSON.stringify(JSON.parse(raw).env)),
    offer: candidates(sdp),
    answer: candidates(answer.sdp),
  });
  await pc.setRemoteDescription({ type: 'answer', sdp: answer.sdp });
  await deadline(opened, 'ctl open');
  return conn;
}

async function echo(conn) {
  const { dc } = sessions.get(conn);
  const s = JSON.stringify({ nonce: await newID(), data: '' });
  const msg = s.slice(0, -2) + 'z'.repeat(60 * 1024 - bytes(s)) + s.slice(-2);
  const echoed = event(dc, 'message');
  dc.send(msg);
  const { data } = await deadline(echoed, 'echo', 5000);
  if (data !== msg || bytes(data) !== 61440) throw Error('message mismatch');
  return bytes(data);
}

async function pairs(conn) {
  const { pc } = sessions.get(conn);
  const stats = await pc.getStats(),
    all = [...stats.values()];
  const transport = all.find((s) => s.type === 'transport' && s.selectedCandidatePairId);
  const selected = all.filter(
    (s) => s.type === 'candidate-pair' && (s.selected || (s.nominated && s.state === 'succeeded')),
  );
  const pair = transport
    ? stats.get(transport.selectedCandidatePairId)
    : selected.length === 1
      ? selected[0]
      : null;
  const describe = (s) =>
    s &&
    [
      s.candidateType || s.type,
      { 1: 'udp', 2: 'tcp' }[s.protocol] || s.protocol,
      s.address || s.ip || '?',
      s.port,
    ].join('/');
  const node = await (await fetch('/pair?node&conn=' + conn)).json();
  return {
    browser: pair && [
      describe(stats.get(pair.localCandidateId)),
      describe(stats.get(pair.remoteCandidateId)),
    ],
    via: transport ? 'transport' : pair?.selected ? 'selected' : pair ? 'unique nominated' : null,
    node: node && [describe(node.Local), describe(node.Remote)],
    state: pc.iceConnectionState,
  };
}
