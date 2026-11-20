import { IDBFactory } from 'fake-indexeddb';
import { afterEach, describe, expect, it, vi } from 'vitest';
import {
  decode,
  Device,
  ed25519,
  encode,
  fingerprint,
  hex,
  isDevice,
  isWeak,
  loadDevice,
  openCommand,
  openSignal,
  StorageBlocked,
  verify,
  verifyChallenge,
  verifyEnvelope,
  type KeyStore,
} from './identity';

afterEach(() => {
  vi.restoreAllMocks();
});

const newDevice = async () =>
  Device.of(crypto, await crypto.subtle.generateKey(ed25519, false, ['sign', 'verify']));

// A device id of the shared vectors, and the same 32 bytes spelled otherwise.
const id = 'akWV_5PtNLIGhakuMAvpD9LcG1D8TUfc1URh48VREcs';

describe('encode and decode', () => {
  it('spell bytes in base64url without padding', () => {
    expect(encode(new Uint8Array([251, 255, 191]))).toBe('-_-_');
    expect(encode(new Uint8Array([0]))).toBe('AA');
    expect(encode(new Uint8Array([0, 0]))).toBe('AAA');
    expect(decode('-_-_')).toStrictEqual(new Uint8Array([251, 255, 191]));
    expect(decode('')).toStrictEqual(new Uint8Array());
    expect(decode(id)).toHaveLength(32);
  });

  it.each([
    ['other trailing bits', id.slice(0, -1) + 't'],
    ['padding', 'AA=='],
    ['the standard alphabet', '+/+/'],
    ['white space', 'AA AA'],
    ['a line break', 'AAAA\nAAAA'],
    ['a character outside the alphabet', 'AA*A'],
    ['an impossible length', 'AAAAA'],
  ])('decode refuses %s', (_, text) => {
    expect(decode(text)).toBeNull();
  });

  it('read a text of any length: a k as long as a frame is refused, not an error', async () => {
    const long = 'A'.repeat(400_000);
    expect(decode(long)).toHaveLength(300_000);
    expect(encode(new Uint8Array(300_000))).toBe(long);
    expect(await verify(crypto, long, 'x', long)).toBe(false);
  });
});

describe('Device', () => {
  it('is known by its public key and its fingerprint', async () => {
    const pair = await crypto.subtle.generateKey(ed25519, false, ['sign', 'verify']);
    const device = await Device.of(crypto, pair);
    const raw = new Uint8Array(await crypto.subtle.exportKey('raw', pair.publicKey));
    expect(device.id).toBe(encode(raw));
    expect(device.id).toHaveLength(43);
    expect(device.fingerprint).toBe(await fingerprint(crypto, raw));
  });

  it('signs "solodawn/1\\n" followed by p as given', async () => {
    const device = await newDevice();
    const p = '{"t":"post", "text":"é"}';
    const env = await device.sign(p);
    expect(env.p).toBe(p);
    expect(env.k).toBe(device.id);
    expect(await verify(crypto, device.id, 'solodawn/1\n' + p, env.s)).toBe(true);
    expect(await verifyEnvelope(crypto, env)).toBe(true);
  });

  it('answers a rendezvous nonce under its own prefix', async () => {
    const device = await newDevice();
    const s = await device.signChallenge('q0J');
    expect(await verify(crypto, device.id, 'solodawn/rv/1\nq0J', s)).toBe(true);
    expect(await verifyChallenge(crypto, device.id, 'q0J', s)).toBe(true);
    expect(await verifyEnvelope(crypto, { p: 'q0J', k: device.id, s })).toBe(false);
  });
});

describe('fingerprint', () => {
  it('is the first 6 bytes of SHA-256, upper-case hex in groups of four', async () => {
    // SHA-256 of nothing is e3b0c442 98fc1c14…
    expect(await fingerprint(crypto, new Uint8Array())).toBe('E3B0 C442 98FC');
    // SHA-256 of "abc" is ba7816bf 8f01cfea…: a zero digit kept in 8F01.
    expect(await fingerprint(crypto, new TextEncoder().encode('abc'))).toBe('BA78 16BF 8F01');
  });
});

describe('verify', () => {
  it('refuses a device id or signature that is not canonical, or not a key', async () => {
    const device = await newDevice();
    const s = await device.signChallenge('n');
    const text = 'solodawn/rv/1\nn';
    expect(await verify(crypto, device.id, text, s)).toBe(true);
    expect(await verify(crypto, device.id + '=', text, s)).toBe(false);
    expect(await verify(crypto, device.id, text, s + '=')).toBe(false);
    expect(await verify(crypto, device.id.slice(0, 40), text, s)).toBe(false);
    expect(await verify(crypto, device.id, text, s.slice(0, 84))).toBe(false);
    expect(await verify(crypto, device.id, text + ' ', s)).toBe(false);
  });

  const notAPoint = () => Promise.reject(new DOMException('point', 'DataError'));

  it.each([
    ['refuses the key', { importKey: notAPoint }],
    ['fails to verify', { importKey: () => Promise.resolve({}), verify: notAPoint }],
  ])('is false when WebCrypto %s', async (_, subtle) => {
    const refusing = { subtle } as unknown as Crypto;
    expect(await verify(refusing, id, 'x', encode(new Uint8Array(64).fill(1)))).toBe(false);
  });
});

/** The fourteen weak encodings: seven values, each also with the highest bit of its last byte set. */
const weak = [
  '0000000000000000000000000000000000000000000000000000000000000000',
  '0100000000000000000000000000000000000000000000000000000000000000',
  '26e8958fc2b227b045c3f489f2ef98f0d5dfac05d3c63339b13802886d53fc05',
  'c7176a703d4dd84fba3c0b760d10670f2a2053fa2c39ccc64ec7fd7792ac037a',
  'ecffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff7f',
  'edffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff7f',
  'eeffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff7f',
].flatMap((h) => {
  const bytes = Uint8Array.from(h.match(/../g)!, (b) => parseInt(b, 16));
  const high = bytes.slice();
  high[31] |= 0x80;
  return [bytes, high];
});

describe('weak keys', () => {
  it('are the fourteen encodings of the points of small order', () => {
    expect(new Set(weak.map(hex)).size).toBe(14);
    for (const point of weak) expect(isWeak(point), hex(point)).toBe(true);
  });

  it('leave the bytes given as they are', () => {
    const point = weak[11]; // edff…7f with the highest bit set
    expect(isWeak(point)).toBe(true);
    expect(hex(point)).toBe('edffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff');
  });

  it('are not device ids, and a real key is', async () => {
    for (const point of weak) expect(isDevice(encode(point))).toBe(false);
    expect(isDevice(id)).toBe(true);
    expect(isDevice((await newDevice()).id)).toBe(true);
    const near = weak[4].slice(); // 26e8…05 with its first byte changed
    near[0] = 0x27;
    expect(isDevice(encode(near))).toBe(true);
  });

  it('fail check 1 as k, before WebCrypto is asked', async () => {
    const spy = vi.spyOn(crypto.subtle, 'verify');
    const importing = vi.spyOn(crypto.subtle, 'importKey');
    // A forgery under the identity point that only the check of k stops: R is the base point,
    // which is not weak, and S is one. Go and the three browser engines verify it for any text;
    // OpenSSL, which runs these tests, does not, so only the spies can tell.
    const base = [0x58, ...new Array<number>(31).fill(0x66)];
    const forged = encode(new Uint8Array([...base, 1, ...new Array<number>(31).fill(0)]));
    for (const point of weak) {
      expect(await verify(crypto, encode(point), 'any text', forged)).toBe(false);
    }
    expect(spy).not.toHaveBeenCalled();
    expect(importing).not.toHaveBeenCalled();
  });

  it('fail check 1 as the first half of a signature, before WebCrypto is asked', async () => {
    const device = await newDevice();
    const real = decode(await device.signChallenge('n'))!;
    const spy = vi.spyOn(crypto.subtle, 'verify');
    for (const point of weak) {
      const s = encode(new Uint8Array([...point, ...real.slice(32)]));
      expect(await verifyChallenge(crypto, device.id, 'n', s)).toBe(false);
    }
    expect(spy).not.toHaveBeenCalled();
  });
});

describe('the length of a signature', () => {
  it('fails check 1 when it is not 64 bytes, before WebCrypto is asked', async () => {
    const device = await newDevice();
    const real = decode(await device.signChallenge('n'))!;
    const importing = vi.spyOn(crypto.subtle, 'importKey');
    // Nothing but the length is wrong: R is a real one, and S stays below L either way.
    for (const s of [real.slice(0, 63), new Uint8Array([...real, 0])]) {
      expect(await verifyChallenge(crypto, device.id, 'n', encode(s))).toBe(false);
    }
    expect(importing).not.toHaveBeenCalled();
  });
});

describe('the second half of a signature', () => {
  const order = 2n ** 252n + 27742317777372353535851937790883648493n;
  /** A signature whose R is a real one's and whose S is the number given. */
  const withS = (real: Uint8Array, value: bigint) => {
    const out = real.slice();
    for (let i = 32; i < 64; i++) out[i] = Number((value >> BigInt(8 * (i - 32))) & 255n);
    return encode(out);
  };

  it('fails check 1 at L and above, before WebCrypto is asked', async () => {
    const device = await newDevice();
    const real = decode(await device.signChallenge('n'))!;
    const spy = vi.spyOn(crypto.subtle, 'verify');
    for (const value of [order, order + 1n, 2n ** 256n - 1n]) {
      expect(await verifyChallenge(crypto, device.id, 'n', withS(real, value))).toBe(false);
    }
    expect(spy).not.toHaveBeenCalled();
  });

  it('is left to WebCrypto below L', async () => {
    const device = await newDevice();
    const real = decode(await device.signChallenge('n'))!;
    const spy = vi.spyOn(crypto.subtle, 'verify');
    expect(await verifyChallenge(crypto, device.id, 'n', withS(real, order - 1n))).toBe(false);
    expect(await verifyChallenge(crypto, device.id, 'n', withS(real, 1n))).toBe(false);
    expect(spy).toHaveBeenCalledTimes(2);
  });
});

describe('openSignal and openCommand', () => {
  const now = 1763600000000;
  const everyone = () => true;
  const nobody = () => false;
  const payload = (fields: string) => `{"t":"offer","n":"x","ts":${now}${fields}}`;

  it('give the payload of an envelope that passes every check', async () => {
    const device = await newDevice();
    const env = await device.sign(payload(',"g":"G","to":"me","sdp":"v=0"'));
    expect(await openSignal(crypto, env, 'me', everyone, now)).toStrictEqual({
      header: { t: 'offer', n: 'x', ts: now, g: 'G', to: 'me', sdp: 'v=0' },
    });
  });

  it('ask known about the signer', async () => {
    const device = await newDevice();
    const known = vi.fn(() => true);
    await openCommand(crypto, await device.sign(payload('')), 'me', known, now);
    expect(known).toHaveBeenCalledExactlyOnceWith(device.id);
  });

  it('answer the first check that fails, in the contract order', async () => {
    const device = await newDevice();
    const late = await device.sign(`{"t":"cmd","n":"c","ts":0,"to":"you"}`);
    const tampered = { ...late, p: late.p.replace('cmd', 'cme') };
    const code = async (env: typeof late, me: string, known: () => boolean) =>
      openCommand(crypto, env, me, known, now);
    expect(await code(tampered, 'me', nobody)).toStrictEqual({ code: 'bad_sig' });
    expect(await code(late, 'me', nobody)).toStrictEqual({ code: 'unknown_device' });
    const odd = await device.sign('{"t":"cmd","n":"c","ts":"0","to":"you"}');
    expect(await code(odd, 'me', nobody)).toStrictEqual({ code: 'unknown_device' });
    expect(await code(odd, 'me', everyone)).toStrictEqual({ code: 'bad_request' });
    expect(await code(late, 'me', everyone)).toStrictEqual({ code: 'not_for_me' });
    expect(await code(late, 'you', everyone)).toStrictEqual({ code: 'stale' });
  });

  it('refuse a signature by a key that is not a device id with bad_sig', async () => {
    const device = await newDevice();
    const env = await device.sign(payload(',"to":"me"'));
    const opened = await openSignal(crypto, { ...env, k: 'A'.repeat(43) }, 'me', everyone, now);
    expect(opened).toStrictEqual({ code: 'bad_sig' });
  });

  it.each([
    ['no t', '{"n":"x","ts":1}'],
    ['no n', '{"t":"cmd","ts":1}'],
    ['no ts', '{"t":"cmd","n":"x"}'],
    ['a t that is not a string', '{"t":1,"n":"x","ts":1}'],
    ['an n that is not a string', '{"t":"cmd","n":1,"ts":1}'],
    ['a ts that is a string', '{"t":"cmd","n":"x","ts":"1"}'],
    ['a ts that is a fraction', '{"t":"cmd","n":"x","ts":1.5}'],
    ['a ts of 2^53', '{"t":"cmd","n":"x","ts":9007199254740992}'],
    ['a ts of 1e20', '{"t":"cmd","n":"x","ts":1e20}'],
    ['a ts that is null', '{"t":"cmd","n":"x","ts":null}'],
    ['a t that is null', '{"t":null,"n":"x","ts":1}'],
    ['a g that is null', '{"t":"cmd","n":"x","ts":1,"g":null}'],
    ['a g that is not a string', '{"t":"cmd","n":"x","ts":1,"g":1}'],
    ['a to that is null', '{"t":"cmd","n":"x","ts":1,"to":null}'],
    ['an array', '[]'],
    ['null', 'null'],
    ['a string', '"cmd"'],
    ['text that is not JSON', 'cmd'],
  ])('answer bad_request for a payload with %s', async (_, p) => {
    const env = await (await newDevice()).sign(p);
    expect(await openCommand(crypto, env, 'me', everyone, 1)).toStrictEqual({
      code: 'bad_request',
    });
  });

  it('read fields by their exact names, the last of a repeat, and integers by value', async () => {
    const device = await newDevice();
    const p = '{"t":"cmd","n":"x","ts":1.0000000000000001,"TO":"o","to":"o","to":"me","Ts":"no"}';
    const opened = await openCommand(crypto, await device.sign(p), 'me', everyone, 1);
    expect(opened).toMatchObject({ header: { ts: 1, to: 'me' } });
  });

  it('let a command have no to, and read an empty to as a value', async () => {
    const device = await newDevice();
    const none = await device.sign(payload(''));
    expect(await openCommand(crypto, none, 'me', everyone, now)).toHaveProperty('header');
    const empty = await device.sign(payload(',"to":""'));
    expect(await openCommand(crypto, empty, 'me', everyone, now)).toStrictEqual({
      code: 'not_for_me',
    });
  });

  it('refuse signalling without this device in to', async () => {
    const device = await newDevice();
    for (const p of [payload(''), payload(',"to":""'), payload(',"to":"you"')]) {
      expect(await openSignal(crypto, await device.sign(p), 'me', everyone, now)).toStrictEqual({
        code: 'not_for_me',
      });
    }
  });

  it.each([
    ['signalling', openSignal, 2 * 60_000],
    ['a command', openCommand, 24 * 3_600_000],
  ])('accept %s within its window, ends included', async (_, open, window) => {
    const device = await newDevice();
    const env = await device.sign(payload(',"to":"me"'));
    const at = async (t: number) => open(crypto, env, 'me', everyone, t);
    expect(await at(now + window)).toHaveProperty('header');
    expect(await at(now - window)).toHaveProperty('header');
    expect(await at(now + window + 1)).toStrictEqual({ code: 'stale' });
    expect(await at(now - window - 1)).toStrictEqual({ code: 'stale' });
  });
});

describe('loadDevice', () => {
  const store = (replace: Partial<KeyStore> = {}): KeyStore => {
    const idb = new IDBFactory();
    return {
      crypto,
      idb: () => idb,
      storage: { persist: vi.fn(() => Promise.resolve(true)) },
      ...replace,
    };
  };
  const tick = () => new Promise((resolve) => setTimeout(resolve));

  /** Resolves when the database can be upgraded: no connection to it is left open. */
  const upgradable = (idb: IDBFactory) =>
    new Promise<string>((resolve) => {
      const opening = idb.open('solodawn', 9);
      opening.onupgradeneeded = () => resolve('upgraded');
      opening.onblocked = () => resolve('blocked');
    });

  it('closes the database after keeping the key', async () => {
    const browser = store();
    await loadDevice(browser);
    expect(await upgradable(browser.idb())).toBe('upgraded');
  });

  it('closes the database when the key cannot be written', async () => {
    const pair = { privateKey: { sign: () => 0 }, publicKey: {} };
    const unstorable = {
      subtle: { generateKey: () => Promise.resolve(pair) },
    } as unknown as Crypto;
    const browser = store({ crypto: unstorable });
    await loadDevice(browser).catch(() => undefined);
    expect(await upgradable(browser.idb())).toBe('upgraded');
  });

  it('creates the key on first run and reads it back afterwards', async () => {
    const browser = store();
    const first = await loadDevice(browser);
    const again = await loadDevice(browser);
    expect(again.id).toBe(first.id);
    expect(again.fingerprint).toBe(first.fingerprint);
    const env = await again.sign('{}');
    expect(await verifyEnvelope(crypto, { ...env, k: first.id })).toBe(true);
  });

  it('keeps the key pair in database solodawn, store device, under key', async () => {
    const browser = store();
    const device = await loadDevice(browser);
    const pair = await new Promise<CryptoKeyPair>((resolve) => {
      const opening = browser.idb().open('solodawn');
      opening.onsuccess = () => {
        const get = opening.result.transaction('device').objectStore('device').get('key');
        get.onsuccess = () => {
          opening.result.close();
          resolve(get.result);
        };
      };
    });
    expect(pair.privateKey.extractable).toBe(false);
    expect(pair.privateKey.usages).toStrictEqual(['sign']);
    const raw = new Uint8Array(await crypto.subtle.exportKey('raw', pair.publicKey));
    expect(encode(raw)).toBe(device.id);
  });

  it('asks once for persistent storage, after creating the key', async () => {
    const browser = store();
    await loadDevice(browser);
    await tick();
    expect(browser.storage.persist).toHaveBeenCalledOnce();
    await loadDevice(browser);
    await tick();
    expect(browser.storage.persist).toHaveBeenCalledOnce();
  });

  it('ignores how the request for persistent storage ends', async () => {
    const persist = vi.fn(() => Promise.reject(new DOMException('no', 'NotAllowedError')));
    await loadDevice(store({ storage: { persist } }));
    await tick();
    expect(persist).toHaveBeenCalledOnce();
  });

  it('keeps one key when two pages open at once', async () => {
    const browser = store();
    const [a, b] = await Promise.all([loadDevice(browser), loadDevice(browser)]);
    expect(b.id).toBe(a.id);
    await tick();
    expect(browser.storage.persist).toHaveBeenCalledOnce();
  });

  it('fails with StorageBlocked when IndexedDB cannot be reached', async () => {
    const refusal = new DOMException('The operation is insecure.', 'SecurityError');
    const idb = () => {
      throw refusal;
    };
    const failure = await loadDevice(store({ idb })).catch((e: unknown) => e);
    expect(failure).toBeInstanceOf(StorageBlocked);
    expect(failure).toMatchObject({
      name: 'StorageBlocked',
      message: 'site storage is blocked',
      cause: refusal,
    });
  });

  it('fails with StorageBlocked when the database does not open', async () => {
    const idb = new IDBFactory();
    await new Promise((resolve) => {
      const newer = idb.open('solodawn', 2);
      newer.onsuccess = () => resolve(newer.result.close());
    });
    const failure = await loadDevice(store({ idb: () => idb })).catch((e: unknown) => e);
    expect(failure).toBeInstanceOf(StorageBlocked);
    expect((failure as Error).cause).toMatchObject({ name: 'VersionError' });
  });

  it('fails with StorageBlocked when the key cannot be written', async () => {
    const pair = { privateKey: { sign: () => 0 }, publicKey: {} };
    const unstorable = {
      subtle: { generateKey: () => Promise.resolve(pair) },
    } as unknown as Crypto;
    const failure = await loadDevice(store({ crypto: unstorable })).catch((e: unknown) => e);
    expect(failure).toBeInstanceOf(StorageBlocked);
  });
});
