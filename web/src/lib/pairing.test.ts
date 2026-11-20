import { describe, expect, it, vi } from 'vitest';
import { mac, newSecret, parseSecret, room, shown, verifyPair } from './pairing';
import type { Pair } from './proto';

/** A WebCrypto whose random bytes are the given ones. */
const randomBytes = (bytes: number[]) =>
  ({
    getRandomValues: (into: Uint8Array) => {
      into.set(bytes);
      return into;
    },
  }) as unknown as Crypto;

describe('newSecret', () => {
  // The expected values are Go's: base32.NewEncoding("0123456789ABCDEFGHJKMNPQRSTVWXYZ").
  it('encodes 10 random bytes as 16 Crockford base32 characters', () => {
    expect(newSecret(randomBytes([0, 1, 2, 3, 4, 5, 6, 7, 8, 9]))).toBe('000G40R40M30E209');
    expect(newSecret(randomBytes([255, 254, 253, 252, 251, 250, 249, 248, 247, 246]))).toBe(
      'ZZZFVZ7VZBWZHXZP',
    );
  });

  it('asks for exactly 10 random bytes', () => {
    const getRandomValues = vi.fn((into: Uint8Array) => into);
    expect(newSecret({ getRandomValues } as unknown as Crypto)).toBe('0000000000000000');
    expect(getRandomValues.mock.calls[0][0]).toHaveLength(10);
  });

  it('is a secret that parses as itself', () => {
    const secret = newSecret(crypto);
    expect(parseSecret(secret)).toBe(secret);
    expect(newSecret(crypto)).not.toBe(secret);
  });
});

describe('parseSecret', () => {
  it('takes the canonical form, lower case and the hyphens a person copied', () => {
    expect(parseSecret('7K2M9QXD4F8TWB3N')).toBe('7K2M9QXD4F8TWB3N');
    expect(parseSecret('7k2m-9qxd-4f8t-wb3n')).toBe('7K2M9QXD4F8TWB3N');
    expect(parseSecret('-7K2M--9QXD-4F8TWB3N-')).toBe('7K2M9QXD4F8TWB3N');
  });

  it('upper-cases a to z and changes no other character', () => {
    // toUpperCase would make "S" of the long s, "SS" of the sharp s and "FF" of the ligature.
    expect(() => parseSecret('7K2M-9QXD-4F8T-WB3ſ')).toThrow('not a pairing secret');
    expect(() => parseSecret('7K2M9QXD4F8TWBß')).toThrow('not a pairing secret');
    expect(() => parseSecret('7K2M9QXD4F8TWBﬀ')).toThrow('not a pairing secret');
    expect(() => parseSecret('7K2M9QXD4F8TWB3ｎ')).toThrow('not a pairing secret');
  });

  it.each([
    ['15 characters', '7K2M9QXD4F8TWB3'],
    ['17 characters', '7K2M9QXD4F8TWB3NN'],
    ['a letter Crockford leaves out', '7K2M9QXD4F8TWB3I'],
    ['a letter Crockford leaves out', '7K2M9QXD4F8TWB3L'],
    ['a letter Crockford leaves out', '7K2M9QXD4F8TWB3O'],
    ['a letter Crockford leaves out', '7K2M9QXD4F8TWB3U'],
    ['a space', '7K2M 9QXD4F8TWB3'],
    ['a leading space', ' 7K2M9QXD4F8TWB3N'],
    ['a trailing line break', '7K2M9QXD4F8TWB3N\n'],
    ['an en dash', '7K2M–9QXD4F8TWB3N'],
    ['nothing', ''],
  ])('refuses %s', (_, text) => {
    expect(() => parseSecret(text)).toThrow(
      'not a pairing secret: want 16 characters such as 7K2M-9QXD-4F8T-WB3N',
    );
  });
});

describe('shown', () => {
  it('shows four groups of four', () => {
    expect(shown('0123456789ABCDEF')).toBe('0123-4567-89AB-CDEF');
  });
});

describe('room', () => {
  it('is 32 lower-case hex digits that differ by secret', async () => {
    const a = await room(crypto, '7K2M9QXD4F8TWB3N');
    expect(a).toMatch(/^[0-9a-f]{32}$/);
    expect(await room(crypto, '7K2M9QXD4F8TWB3P')).not.toBe(a);
  });
});

describe('mac', () => {
  const secret = '7K2M9QXD4F8TWB3N';
  const k = '11qYAYKxCrfVS_7TyWQHOg7hcvPapiMlrwIaaPcHURo';
  const sent = { role: 'a', k, kind: 'node', name: 'workstation' };

  // The expected values are Node's: an HMAC-SHA256 over the text as its TextEncoder encodes it.
  it('covers the role, the device id, the kind and the name, one to a line', async () => {
    expect(await mac(crypto, secret, sent)).toBe('iWG_BRkXCtR8OKQJz6EGeQxUUffZlDp8IOm-XRifvqw');
    expect(await mac(crypto, secret, { ...sent, role: 'b' })).toBe(
      'dhGdvYswXbCsrMb2YuWEEhmMkUfFsrC3UA5SFI2akqI',
    );
  });

  it('takes the name as it stands, and a lone surrogate in it as U+FFFD', async () => {
    const odd = '3VEPY8_4LjatXYx7y3qgVWyipZ2TPYHIr4mhBMe1QZc';
    const name = (text: string) => ({ role: 'a', k, kind: 'browser', name: text });
    expect(await mac(crypto, secret, name('café\n\ud800 🙂'))).toBe(odd);
    expect(await mac(crypto, secret, name('café\n\ufffd 🙂'))).toBe(odd);
    expect(await mac(crypto, secret, name('cafe\u0301\n\ufffd 🙂'))).not.toBe(odd);
    expect(await mac(crypto, secret, name(' café\n\ufffd 🙂'))).not.toBe(odd);
  });

  it('differs when a part moves into the next', async () => {
    const macs = await Promise.all(
      [
        sent,
        { ...sent, kind: '', name: 'node\nworkstation' },
        { ...sent, kind: 'nodeworkstation', name: '' },
        { ...sent, name: '' },
      ].map((m) => mac(crypto, secret, m)),
    );
    expect(new Set(macs).size).toBe(4);
  });
});

describe('verifyPair', () => {
  const secret = '7K2M9QXD4F8TWB3N';
  const device = 'akWV_5PtNLIGhakuMAvpD9LcG1D8TUfc1URh48VREcs';
  /** A pair message with the mac that follows from it, unless one is given. */
  const message = async (role: string, override: Partial<Pair> = {}) => {
    const m = { k: device, kind: 'browser', name: 'phone browser', role, ...override };
    return { t: 'pair' as const, mac: await mac(crypto, secret, m), ...m };
  };

  it('accepts the other side and refuses a message in another role', async () => {
    expect(await verifyPair(crypto, secret, await message('b'), 'b')).toBe(true);
    expect(await verifyPair(crypto, secret, await message('b'), 'a')).toBe(false);
  });

  it('refuses another secret, another device and a mac for another role', async () => {
    expect(await verifyPair(crypto, '7K2M9QXD4F8TWB3P', await message('b'), 'b')).toBe(false);
    const other = 'J3Uuh-y1z2AX536yAFlGHJL5sjxVpWVJavvVphvdm5Q';
    const m = await message('b');
    expect(await verifyPair(crypto, secret, { ...m, k: other }, 'b')).toBe(false);
    const forA = (await message('a')).mac;
    expect(await verifyPair(crypto, secret, { ...m, mac: forA }, 'b')).toBe(false);
  });

  it('refuses a message whose kind or name was changed on the way', async () => {
    const m = await message('b');
    expect(await verifyPair(crypto, secret, { ...m, name: 'laptop' }, 'b')).toBe(false);
    expect(await verifyPair(crypto, secret, { ...m, name: '' }, 'b')).toBe(false);
    expect(await verifyPair(crypto, secret, { ...m, kind: 'node' }, 'b')).toBe(false);
  });

  it('accepts a node and a browser, and ignores any other kind even with the right mac', async () => {
    expect(await verifyPair(crypto, secret, await message('b', { kind: 'node' }), 'b')).toBe(true);
    expect(await verifyPair(crypto, secret, await message('b', { name: '' }), 'b')).toBe(true);
    for (const kind of ['phone', '', 'Node', 'nodebrowser']) {
      expect(await verifyPair(crypto, secret, await message('b', { kind }), 'b')).toBe(false);
    }
  });

  it('refuses a mac that is not in its canonical spelling', async () => {
    const m = await message('b');
    expect(await verifyPair(crypto, secret, { ...m, mac: m.mac + '=' }, 'b')).toBe(false);
    expect(await verifyPair(crypto, secret, { ...m, mac: 'not base64!' }, 'b')).toBe(false);
  });

  it('refuses a message whose role is its own, whatever role its mac is for', async () => {
    const m = await message('b');
    expect(await verifyPair(crypto, secret, { ...m, role: 'a' }, 'b')).toBe(false);
  });

  it('keys the HMAC with the secret bytes, not extractable', async () => {
    const importKey = vi.spyOn(crypto.subtle, 'importKey');
    await message('a');
    expect(importKey).toHaveBeenCalledExactlyOnceWith(
      'raw',
      new TextEncoder().encode(secret),
      { name: 'HMAC', hash: 'SHA-256' },
      false,
      ['sign', 'verify'],
    );
    importKey.mockRestore();
  });

  it('ignores a message whose k is not a device id, even with the right mac', async () => {
    for (const k of ['A'.repeat(43), device + '=', 'abc']) {
      expect(await verifyPair(crypto, secret, await message('b', { k }), 'b')).toBe(false);
    }
  });

  it('refuses a mac of another length', async () => {
    const m = await message('b');
    expect(await verifyPair(crypto, secret, { ...m, mac: m.mac.slice(0, 40) }, 'b')).toBe(false);
  });
});
