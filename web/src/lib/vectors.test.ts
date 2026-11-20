// The shared test vectors of contracts/envelope.md section 8, which internal/identity passes too.
import { describe, expect, it } from 'vitest';
import { DeviceLog } from './devlog';
import { Device, decode, encode, openCommand, verifyChallenge, verifyEnvelope } from './identity';
import { goldens } from './mock';
import { mac, parseSecret, room, shown, verifyPair } from './pairing';
import type { Envelope } from './proto';

interface Side {
  device: string;
  mac: string;
}

/** The shape of section 8. */
interface Vectors {
  keys: { name: string; seed: string; device: string; fingerprint: string }[];
  envelopes: (Envelope & { name: string; ok: boolean })[];
  rendezvous: { nonce: string; k: string; s: string }[];
  pairing: { shown: string; secret: string; room: string; a: Side; b: Side }[];
  devlogs: {
    name: string;
    user: string;
    entries: { env: Envelope; accept: boolean }[];
    valid: string[];
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

/** Whether this WebCrypto signs deterministically, as RFC 8032 does: Node's does, Safari's
 *  does not. The contract holds only Go to the bytes of s; a browser's signature must verify. */
async function deterministic(device: Device): Promise<boolean> {
  return (await device.sign('x')).s === (await device.sign('x')).s;
}

/** Signing p again with the seed of k: the same s where signing is deterministic, else a
 *  signature that verifies. */
async function signsAgain(device: Device, p: string, s: string): Promise<boolean> {
  const again = await device.sign(p);
  return (await deterministic(device)) ? again.s === s : verifyEnvelope(crypto, again);
}

describe('the shared crypto vectors', () => {
  it('have the cases section 8 asks for', () => {
    expect(vectors.keys.length).toBeGreaterThanOrEqual(2);
    expect(vectors.envelopes.length).toBeGreaterThanOrEqual(4);
    expect(vectors.rendezvous.length).toBeGreaterThanOrEqual(1);
    expect(vectors.pairing.length).toBeGreaterThanOrEqual(3);
    expect(vectors.devlogs.length).toBeGreaterThanOrEqual(8);
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
    const signer = devices.get(c.k);
    if (c.ok) expect(await signsAgain(signer!, c.p, c.s)).toBe(true);
  });

  it.each(vectors.rendezvous)('rendezvous nonce $nonce: the answer is for it alone', async (c) => {
    expect(await verifyChallenge(crypto, c.k, c.nonce, c.s)).toBe(true);
    expect(await verifyChallenge(crypto, c.k, c.nonce + 'A', c.s)).toBe(false);
    const signer = devices.get(c.k)!;
    const again = await signer.signChallenge(c.nonce);
    if (await deterministic(signer)) expect(again).toBe(c.s);
    expect(await verifyChallenge(crypto, c.k, c.nonce, again)).toBe(true);
  });

  it.each(vectors.pairing)('pairing $shown: secret, room and both macs', async (c) => {
    const secret = parseSecret(c.shown);
    expect(secret).toBe(c.secret);
    expect(shown(secret)).toBe(
      `${c.secret.slice(0, 4)}-${c.secret.slice(4, 8)}-${c.secret.slice(8, 12)}-${c.secret.slice(12)}`,
    );
    expect(await room(crypto, secret)).toBe(c.room);
    expect(await mac(crypto, secret, 'a', c.a.device)).toBe(c.a.mac);
    expect(await mac(crypto, secret, 'b', c.b.device)).toBe(c.b.mac);
    const a = {
      t: 'pair' as const,
      k: c.a.device,
      kind: 'browser',
      name: 'a',
      role: 'a',
      mac: c.a.mac,
    };
    const b = {
      t: 'pair' as const,
      k: c.b.device,
      kind: 'node',
      name: 'b',
      role: 'b',
      mac: c.b.mac,
    };
    expect(await verifyPair(crypto, secret, a, 'a')).toBe(true);
    expect(await verifyPair(crypto, secret, b, 'b')).toBe(true);
    expect(await verifyPair(crypto, secret, a, 'b')).toBe(false);
    expect(await verifyPair(crypto, secret, b, 'a')).toBe(false);
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
    expect(log.validDevices()).toStrictEqual(c.valid);
  });

  it('encodes a device id back to itself', () => {
    for (const k of vectors.keys) expect(encode(decode(k.device)!)).toBe(k.device);
  });
});
