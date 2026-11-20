// The shared test vectors of planning/contracts/vectors.md, which internal/identity passes too.
import { describe, expect, it } from 'vitest';
import { DeviceLog } from './devlog';
import {
  Device,
  decode,
  encode,
  openCommand,
  openSignal,
  verifyChallenge,
  verifyEnvelope,
} from './identity';
import * as v from 'valibot';
import { goldens } from './mock';
import { mac, parseSecret, room, shown, verifyPair } from './pairing';
import { Pair, type Envelope } from './proto';

interface Side {
  device: string;
  kind: string;
  name: string;
  mac: string;
}

/** The shape of section 1 of the vectors contract. */
interface Vectors {
  keys: { name: string; seed: string; device: string; fingerprint: string }[];
  envelopes: (Envelope & { name: string; ok: boolean })[];
  rendezvous: { nonce: string; k: string; s: string }[];
  received: {
    name: string;
    kind: 'command' | 'signal';
    me: string;
    now: number;
    known: boolean;
    env: Envelope;
    code: string;
  }[];
  pairing: { shown: string; secret: string; room: string; a: Side; b: Side }[];
  pair_messages: { name: string; me: 'a' | 'b'; secret: string; msg: unknown; ok: boolean }[];
  bad_secrets: { name: string; shown: string }[];
  devlogs: {
    name: string;
    user: string;
    entries: { env: Envelope; accept: boolean }[];
    valid: string[];
  }[];
  adopt: {
    name: string;
    user: string;
    me: string;
    peer: string;
    entries: Envelope[];
    ok: boolean;
  }[];
}

const vectors: Vectors = JSON.parse(goldens.crypto);

const { subtle } = crypto;
const everyone = () => true;
const now = 1763600000000;

/** The device of a seed: the seed as a PKCS #8 private key, the public key from its JWK. */
async function device(seed: string): Promise<Device> {
  const pkcs8 = new Uint8Array([
    ...[0x30, 0x2e, 0x02, 0x01, 0x00, 0x30, 0x05, 0x06, 0x03, 0x2b, 0x65, 0x70, 0x04, 0x22, 0x04],
    ...[0x20, ...decode(seed)!],
  ]);
  const privateKey = await subtle.importKey('pkcs8', pkcs8, 'Ed25519', true, ['sign']);
  const { x } = await subtle.exportKey('jwk', privateKey);
  const publicKey = await subtle.importKey('raw', decode(x!)!, 'Ed25519', true, ['verify']);
  return Device.of(crypto, { privateKey, publicKey });
}

const devices = new Map<string, Device>();

describe('the shared crypto vectors', () => {
  it('have every part, each with cases', () => {
    const parts = Object.entries(vectors).map(([part, cases]) => [part, cases.length > 0]);
    expect(parts).toStrictEqual(
      [
        'keys',
        'envelopes',
        'rendezvous',
        'received',
        'pairing',
        'pair_messages',
        'bad_secrets',
        'devlogs',
        'adopt',
      ].map((part) => [part, true]),
    );
  });

  it.each(vectors.keys)('key $name: device id and fingerprint follow from the seed', async (c) => {
    const d = await device(c.seed);
    expect(d.id).toBe(c.device);
    expect(d.fingerprint).toBe(c.fingerprint);
    devices.set(d.id, d);
  });

  it.each(vectors.envelopes)('envelope "$name": passes check 1 as ok says', async (c) => {
    const env = { p: c.p, k: c.k, s: c.s };
    expect(await verifyEnvelope(crypto, env)).toBe(c.ok);
    const opened = await openCommand(crypto, env, '', everyone, now);
    expect('code' in opened && opened.code === 'bad_sig').toBe(!c.ok);
    // Signature bytes are never compared (section 2 of the vectors contract): Safari's are
    // random. What the client signs itself must verify.
    if (c.ok) expect(await verifyEnvelope(crypto, await devices.get(c.k)!.sign(c.p))).toBe(true);
  });

  it.each(vectors.rendezvous)('rendezvous nonce $nonce: the answer is for it alone', async (c) => {
    expect(await verifyChallenge(crypto, c.k, c.nonce, c.s)).toBe(true);
    expect(await verifyChallenge(crypto, c.k, c.nonce + 'A', c.s)).toBe(false);
    const again = await devices.get(c.k)!.signChallenge(c.nonce);
    expect(await verifyChallenge(crypto, c.k, c.nonce, again)).toBe(true);
  });

  it.each(vectors.received)('received "$name": $code', async (c) => {
    const open = c.kind === 'signal' ? openSignal : openCommand;
    const opened = await open(crypto, c.env, c.me, () => c.known, c.now);
    expect('code' in opened ? opened.code : 'ok').toBe(c.code);
  });

  it.each(vectors.pairing)('pairing $shown: secret, room and both macs', async (c) => {
    const secret = parseSecret(c.shown);
    expect(secret).toBe(c.secret);
    expect(shown(secret)).toBe(
      `${c.secret.slice(0, 4)}-${c.secret.slice(4, 8)}-${c.secret.slice(8, 12)}-${c.secret.slice(12)}`,
    );
    expect(await room(crypto, secret)).toBe(c.room);
    const message = (role: string, side: Side) => ({
      t: 'pair' as const,
      k: side.device,
      kind: side.kind,
      name: side.name,
      role,
      mac: side.mac,
    });
    const [a, b] = [message('a', c.a), message('b', c.b)];
    expect(await mac(crypto, secret, a)).toBe(c.a.mac);
    expect(await mac(crypto, secret, b)).toBe(c.b.mac);
    expect(await verifyPair(crypto, secret, a, 'a')).toBe(true);
    expect(await verifyPair(crypto, secret, b, 'b')).toBe(true);
    expect(await verifyPair(crypto, secret, a, 'b')).toBe(false);
    expect(await verifyPair(crypto, secret, b, 'a')).toBe(false);
  });

  // A message is read as everything from the wire is; one that cannot be read is ignored.
  it.each(vectors.pair_messages)('pair message "$name": accepted is $ok', async (c) => {
    const read = v.safeParse(Pair, c.msg);
    const sender = c.me === 'a' ? 'b' : 'a';
    const accepted = read.success && (await verifyPair(crypto, c.secret, read.output, sender));
    expect(accepted).toBe(c.ok);
  });

  it.each(vectors.bad_secrets)('bad secret "$name": no secret', (c) => {
    expect(() => parseSecret(c.shown)).toThrow('not a pairing secret');
  });

  it.each(vectors.devlogs)('device log "$name"', async (c) => {
    const log = new DeviceLog(crypto, c.user);
    const accepted = [];
    for (const e of c.entries)
      accepted.push(
        await log.apply(e.env).then(
          () => true,
          () => false,
        ),
      );
    expect(accepted).toStrictEqual(c.entries.map((e) => e.accept));
    // valid lists the devices of keys only: a dev that is no key of the vectors can be valid too.
    const listed = log.validDevices().filter((d) => vectors.keys.some((k) => k.device === d));
    expect(listed).toStrictEqual(c.valid);
  });

  it.each(vectors.adopt)('adopt "$name"', async (c) => {
    const log = await DeviceLog.replay(crypto, c.user, c.entries);
    expect(log.adopts(c.me, c.peer)).toBe(c.ok);
  });

  it('encodes a device id back to itself', () => {
    for (const k of vectors.keys) expect(encode(decode(k.device)!)).toBe(k.device);
  });
});
