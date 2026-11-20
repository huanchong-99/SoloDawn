// Pairing two devices that have never met (contracts/envelope.md section 6), as
// internal/identity/pairing.go does it. A secret here is its canonical form: 16 Crockford base32
// characters in upper case without hyphens, whose ASCII bytes are the secret bytes.
import { decode, encode, hex, isDevice, utf8 } from './identity';
import type { Pair } from './proto';

const crockford = '0123456789ABCDEFGHJKMNPQRSTVWXYZ';
const secretForm = /^[0-9A-HJKMNP-TV-Z]{16}$/;

/** 80 random bits as a pairing secret. How long it is valid, and that it is used once, is kept
 *  by whoever hands it out. */
export function newSecret(crypto: Crypto): string {
  const bits = [...crypto.getRandomValues(new Uint8Array(10))]
    .map((b) => b.toString(2).padStart(8, '0'))
    .join('');
  return bits.replace(/[01]{5}/g, (five) => crockford[parseInt(five, 2)]);
}

/** A secret as a person typed it: the hyphens are removed and a to z made upper case, and what
 *  remains must be 16 characters of the alphabet. Throws on any other text. No other character is
 *  changed: toUpperCase would make "SS" of the sharp s and "S" of the long s. */
export function parseSecret(text: string): string {
  const secret = text.replaceAll('-', '').replace(/[a-z]/g, (c) => c.toUpperCase());
  if (!secretForm.test(secret)) {
    throw new Error('not a pairing secret: want 16 characters such as 7K2M-9QXD-4F8T-WB3N');
  }
  return secret;
}

/** The secret as shown to people: four groups of four. */
export const shown = (secret: string) =>
  `${secret.slice(0, 4)}-${secret.slice(4, 8)}-${secret.slice(8, 12)}-${secret.slice(12)}`;

/** The id of the rendezvous room in which the two sides meet: 32 lower-case hex digits. */
export async function room(crypto: Crypto, secret: string): Promise<string> {
  const hash = await crypto.subtle.digest('SHA-256', utf8('solodawn/room/1\n' + secret));
  return hex(new Uint8Array(hash).slice(0, 16));
}

const hmacKey = (crypto: Crypto, secret: string) =>
  crypto.subtle.importKey('raw', utf8(secret), { name: 'HMAC', hash: 'SHA-256' }, false, [
    'sign',
    'verify',
  ]);

/** What the mac of a pair message covers: the role, "a" for the side that created the secret and
 *  "b" for the other, and the device id, kind and name the message carries. */
export type Paired = Pick<Pair, 'role' | 'k' | 'kind' | 'name'>;

/** Each part as it stands in the message, encoded as TextEncoder does: a lone surrogate becomes
 *  U+FFFD, which is what a Go node reads for its escape. name comes last and may hold line breaks. */
const pairText = (m: Paired) => utf8(`solodawn/pair/1\n${m.role}\n${m.k}\n${m.kind}\n${m.name}`);

/** The mac of a pair message. */
export async function mac(crypto: Crypto, secret: string, m: Paired) {
  const key = await hmacKey(crypto, secret);
  return encode(new Uint8Array(await crypto.subtle.sign('HMAC', key, pairText(m))));
}

const isKind = (kind: string) => kind === 'node' || kind === 'browser';

/** Whether a received pair message proves that its sender holds the secret and wrote its role,
 *  device id, kind and name (section 6). role is the role the other side must have: a message in
 *  the receiver's own role is its own message sent back, and proves nothing. k must be a device
 *  id and kind node or browser. The mac is compared in constant time, in its canonical spelling
 *  only. Any other message is ignored. */
export async function verifyPair(crypto: Crypto, secret: string, m: Pair, role: string) {
  const sent = decode(m.mac);
  if (m.role !== role || !isDevice(m.k) || !isKind(m.kind) || sent === null) return false;
  const key = await hmacKey(crypto, secret);
  return crypto.subtle.verify('HMAC', key, sent, pairText(m));
}
