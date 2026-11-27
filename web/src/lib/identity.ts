// The browser half of contracts/envelope.md: this device's key, signed envelopes and the
// rendezvous answer. internal/identity is the reference; both are held to
// internal/proto/testdata/crypto.json. The device log is in devlog.ts, pairing in pairing.ts.
import { runTx } from './idbtx';
import { readHeader, type Header } from './payload';
import type { Envelope } from './proto';

export const ed25519 = { name: 'Ed25519' } as const;

// Each use of a device key signs under its own prefix, so a signature made for one purpose never
// verifies for the other.
const envelopeContext = 'solodawn/1\n';
const challengeContext = 'solodawn/rv/1\n';

export const utf8 = (text: string) => new TextEncoder().encode(text);

/** base64url without padding, as the wire carries binary values. The bytes are not spread into
 *  one call: decode encodes what it is sent, and arguments by the hundred thousand overflow. */
export const encode = (bytes: Uint8Array) =>
  btoa(Array.from(bytes, (b) => String.fromCharCode(b)).join(''))
    .replaceAll('+', '-')
    .replaceAll('/', '_')
    .replaceAll('=', '');

/** A base64url value in its one canonical spelling, or null: other values of the unused trailing
 *  bits, padding, white space and the standard alphabet are refused, so a key has one device id. */
export function decode(text: string): Uint8Array<ArrayBuffer> | null {
  let bytes = new Uint8Array();
  try {
    bytes = Uint8Array.from(atob(text.replaceAll('-', '+').replaceAll('_', '/')), (c) =>
      c.charCodeAt(0),
    );
  } catch {
    // not base64: bytes stays empty, which encodes to "" and so matches only ""
  }
  return encode(bytes) === text ? bytes : null;
}

/** Bytes as lower-case hex. */
export const hex = (bytes: Uint8Array) =>
  [...bytes].map((b) => b.toString(16).padStart(2, '0')).join('');

/** The form of a device id shown to people: the first 6 bytes of SHA-256 of the public key, as
 *  12 upper-case hex digits in groups of four, such as "3F9A 0C21 77BE". */
export async function fingerprint(
  crypto: Crypto,
  publicKey: Uint8Array<ArrayBuffer>,
): Promise<string> {
  const hash = new Uint8Array(await crypto.subtle.digest('SHA-256', publicKey));
  const digits = hex(hash.slice(0, 6)).toUpperCase();
  return `${digits.slice(0, 4)} ${digits.slice(4, 8)} ${digits.slice(8)}`;
}

/** A device's key pair with the names others know it by. */
export class Device {
  private constructor(
    /** The 32-byte public key, base64url (43 characters). */
    readonly id: string,
    readonly fingerprint: string,
    private readonly key: CryptoKey,
    private readonly crypto: Crypto,
  ) {}

  /** The device of a key pair whose public key can be exported. */
  static async of(crypto: Crypto, pair: CryptoKeyPair): Promise<Device> {
    const publicKey = new Uint8Array(await crypto.subtle.exportKey('raw', pair.publicKey));
    return new Device(
      encode(publicKey),
      await fingerprint(crypto, publicKey),
      pair.privateKey,
      crypto,
    );
  }

  /** The envelope of payload p. The signature covers p as given: p is the exact JSON text the
   *  receiver gets, never parsed and encoded again. */
  async sign(p: string): Promise<Envelope> {
    return { p, k: this.id, s: await this.signText(envelopeContext + p) };
  }

  /** The answer to the nonce of a rendezvous handshake, signed as the text in the frame. */
  signChallenge(nonce: string): Promise<string> {
    return this.signText(challengeContext + nonce);
  }

  private async signText(text: string): Promise<string> {
    return encode(new Uint8Array(await this.crypto.subtle.sign(ed25519, this.key, utf8(text))));
  }
}

// The fourteen encodings of the points of small order, under which a signature can be made with
// no private key: these values with the highest bit of the last byte cleared (section 2). They
// are compared here, before WebCrypto, because Ed25519 libraries do not agree on them.
const weakKeys = new Set([
  '0000000000000000000000000000000000000000000000000000000000000000',
  '0100000000000000000000000000000000000000000000000000000000000000',
  '26e8958fc2b227b045c3f489f2ef98f0d5dfac05d3c63339b13802886d53fc05',
  'c7176a703d4dd84fba3c0b760d10670f2a2053fa2c39ccc64ec7fd7792ac037a',
  'ecffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff7f',
  'edffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff7f',
  'eeffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff7f',
]);

/** Whether 32 bytes are one of the fourteen weak values. */
export function isWeak(point: Uint8Array): boolean {
  const cleared = point.slice();
  cleared[31] &= 0x7f;
  return weakKeys.has(hex(cleared));
}

/** Whether a text is a device id: a 32-byte public key in its canonical spelling, not weak. */
export function isDevice(id: string): boolean {
  const key = decode(id);
  return key?.length === 32 && !isWeak(key);
}

// L, the order of the Ed25519 group: the second half of a signature, S, is a number below it.
const order = 2n ** 252n + 27742317777372353535851937790883648493n;

/** Whether 32 bytes, little-endian, are a number below L. */
const isScalar = (bytes: Uint8Array) =>
  bytes.reduceRight((n, b) => n * 256n + BigInt(b), 0n) < order;

/** The checks of section 3, check 1, that come before WebCrypto: the device is a device id, and
 *  the signature is 64 bytes in their canonical spelling whose first half, R, is not weak and
 *  whose second half, S, is below L. Made here so that every implementation answers alike. */
function plausible(
  device: string,
  signature: Uint8Array | null,
): signature is Uint8Array<ArrayBuffer> {
  return (
    isDevice(device) &&
    signature?.length === 64 &&
    !isWeak(signature.slice(0, 32)) &&
    isScalar(signature.slice(32))
  );
}

/** Whether s is a signature of the device over text (section 3, check 1). */
export async function verify(
  crypto: Crypto,
  device: string,
  text: string,
  s: string,
): Promise<boolean> {
  const signature = decode(s);
  if (!plausible(device, signature)) return false;
  try {
    const key = await crypto.subtle.importKey('raw', decode(device)!, ed25519, false, ['verify']);
    return await crypto.subtle.verify(ed25519, key, signature, utf8(text));
  } catch {
    // A browser may refuse to import 32 bytes that are not a point: no signature verifies.
    return false;
  }
}

/** Whether the signature of an envelope covers its payload for its signer. */
export const verifyEnvelope = (crypto: Crypto, env: Envelope) =>
  verify(crypto, env.k, envelopeContext + env.p, env.s);

/** Whether s is the device's answer to a rendezvous nonce. */
export const verifyChallenge = (crypto: Crypto, device: string, nonce: string, s: string) =>
  verify(crypto, device, challengeContext + nonce, s);

/** Why a receiver refuses an envelope, as the code goes on the wire. `bad_request`: the signed
 *  payload is not what the contract describes. */
export type Code = 'bad_sig' | 'unknown_device' | 'bad_request' | 'not_for_me' | 'stale';

/** An opened envelope: its payload's common fields, for the caller to parse p by its type, or the
 *  code of the first check that failed. */
export type Opened = { header: Header } | { code: Code };

/** Whether a device is a currently valid device of someone the receiver listens to: only the
 *  caller knows its owner, its groups and its pairing peers. */
export type Known = (device: string) => boolean;

const minute = 60_000;

/** How a receiver reads one kind of payload: how far ts may be from its clock, either way, and
 *  whether the payload must name it in to. */
interface Kind {
  window: number;
  addressed: boolean;
}

const signal: Kind = { window: 2 * minute, addressed: true };
const command: Kind = { window: 24 * 60 * minute, addressed: false };

/** The receiver's checks of contracts/envelope.md section 3 for a signalling payload, which must
 *  name this device (me) in to and whose ts must be within two minutes of now (in ms). */
export const openSignal = (crypto: Crypto, env: Envelope, me: string, known: Known, now: number) =>
  open(crypto, env, me, known, now, signal);

/** openSignal for a command, whose ts must be within 24 hours and which may have no to. */
export const openCommand = (crypto: Crypto, env: Envelope, me: string, known: Known, now: number) =>
  open(crypto, env, me, known, now, command);

/** The checks in the contract's order. */
async function open(
  crypto: Crypto,
  env: Envelope,
  me: string,
  known: Known,
  now: number,
  kind: Kind,
): Promise<Opened> {
  if (!(await verifyEnvelope(crypto, env))) return { code: 'bad_sig' };
  if (!known(env.k)) return { code: 'unknown_device' };
  const header = readHeader(env.p);
  if (header === undefined) return { code: 'bad_request' };
  return addressed(header, me, now, kind);
}

function addressed(header: Header, me: string, now: number, kind: Kind): Opened {
  // An empty to is a value, and not this device's id.
  if (header.to === undefined ? kind.addressed : header.to !== me) return { code: 'not_for_me' };
  const age = now - header.ts;
  if (age > kind.window || age < -kind.window) return { code: 'stale' };
  return { header };
}

/** Raised when IndexedDB refuses to open, read or write: the person or the browser blocks site
 *  storage. Not an old browser: the client shows its own notice. */
export class StorageBlocked extends Error {
  constructor(cause: unknown) {
    super('site storage is blocked', { cause });
    this.name = 'StorageBlocked';
  }
}

/** What loadDevice needs from the browser. idb is read only when used: reading it can throw. */
export interface KeyStore {
  crypto: Crypto;
  idb: () => IDBFactory;
}

/** This browser's device, its key kept in IndexedDB (database `solodawn`, store `device`, key
 *  `key`) and created on first run. Each call makes a candidate key and stores it only when none
 *  is stored, in one transaction, so two pages opened at once keep one key. The transaction is
 *  durable: a new device key must not be lost to a power cut. It does not ask for persistent
 *  storage: Firefox would prompt the person on a first visit; enrolment (F-04) asks. Rejects with
 *  StorageBlocked when IndexedDB fails. */
export async function loadDevice({ crypto, idb }: KeyStore): Promise<Device> {
  const fresh = await crypto.subtle.generateKey(ed25519, false, ['sign', 'verify']);
  const kept = await keep(idb, 'key', fresh).catch((cause: unknown) => {
    throw new StorageBlocked(cause);
  });
  return Device.of(crypto, kept);
}

/** Stores `fresh` in IndexedDB (`solodawn`, store `device`) under `name` when absent and returns
 *  whichever value is kept, in one durable transaction. */
export function keep<T>(idb: () => IDBFactory, name: string, fresh: T): Promise<T> {
  return runTx<T>(idb, 'readwrite', { durability: 'strict' }, (store, set) => {
    set(fresh);
    const stored = store.get(name);
    stored.onsuccess = () => {
      if (stored.result) set(stored.result as T);
      else store.add(fresh, name);
    };
  });
}
