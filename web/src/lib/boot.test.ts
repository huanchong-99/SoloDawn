import { IDBFactory } from 'fake-indexeddb';
import { mount, unmount } from 'svelte';
import App from '../App.svelte';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { boot, canKeepEd25519Key, hasLightDark, type Browser } from './boot';

type Call = (...args: unknown[]) => Promise<unknown>;

/** A WebCrypto whose three calls succeed unless a test replaces one. */
function fakeCrypto(replace: { generateKey?: Call; sign?: Call; verify?: Call } = {}) {
  const keys = { privateKey: { type: 'private' }, publicKey: { type: 'public' } };
  const signature = new Uint8Array([7, 7]).buffer;
  const subtle = {
    generateKey: vi.fn<Call>(() => Promise.resolve(keys)),
    sign: vi.fn<Call>(() => Promise.resolve(signature)),
    verify: vi.fn<Call>(() => Promise.resolve(true)),
  };
  Object.assign(subtle, replace);
  return { crypto: { subtle } as unknown as Crypto, subtle, keys, signature };
}

const notSupported = () => Promise.reject(new DOMException('Ed25519', 'NotSupportedError'));
const withLightDark = { supports: () => true };
const withoutLightDark = { supports: () => false };

afterEach(() => {
  vi.restoreAllMocks();
});

describe('canKeepEd25519Key', () => {
  it('makes a non-extractable Ed25519 key, signs with a clone of it and verifies', async () => {
    const { crypto, subtle, keys, signature } = fakeCrypto();
    expect(await canKeepEd25519Key(crypto)).toBe(true);
    expect(subtle.generateKey).toHaveBeenCalledExactlyOnceWith({ name: 'Ed25519' }, false, [
      'sign',
      'verify',
    ]);
    const [algorithm, key, message] = subtle.sign.mock.calls[0];
    expect(subtle.sign).toHaveBeenCalledOnce();
    expect(algorithm).toStrictEqual({ name: 'Ed25519' });
    expect(key).toStrictEqual(keys.privateKey);
    expect(key).not.toBe(keys.privateKey);
    expect(new TextDecoder().decode(message as Uint8Array)).toBe('solodawn boot check');
    expect(subtle.verify).toHaveBeenCalledExactlyOnceWith(
      { name: 'Ed25519' },
      keys.publicKey,
      signature,
      message,
    );
  });

  it('holds for the WebCrypto of the test runtime', async () => {
    expect(await canKeepEd25519Key(globalThis.crypto)).toBe(true);
  });

  it('is false when WebCrypto rejects the algorithm', async () => {
    const { crypto, subtle } = fakeCrypto({ generateKey: notSupported });
    expect(await canKeepEd25519Key(crypto)).toBe(false);
    expect(subtle.sign).not.toHaveBeenCalled();
  });

  it('is false when the key cannot be cloned, as in Firefox 129 to 135', async () => {
    vi.spyOn(globalThis, 'structuredClone').mockImplementation(() => {
      throw new DOMException('CryptoKey', 'DataCloneError');
    });
    const { crypto, subtle } = fakeCrypto();
    expect(await canKeepEd25519Key(crypto)).toBe(false);
    expect(subtle.sign).not.toHaveBeenCalled();
  });

  it('is false when the clone cannot sign', async () => {
    const { crypto, subtle } = fakeCrypto({
      sign: () => Promise.reject(new DOMException('key', 'InvalidAccessError')),
    });
    expect(await canKeepEd25519Key(crypto)).toBe(false);
    expect(subtle.verify).not.toHaveBeenCalled();
  });

  it('is false when the signature does not verify', async () => {
    const { crypto } = fakeCrypto({ verify: () => Promise.resolve(false) });
    expect(await canKeepEd25519Key(crypto)).toBe(false);
  });

  it('is false when verifying fails', async () => {
    const { crypto } = fakeCrypto({ verify: notSupported });
    expect(await canKeepEd25519Key(crypto)).toBe(false);
  });

  it('is false when the page has no WebCrypto', async () => {
    expect(await canKeepEd25519Key({} as Crypto)).toBe(false);
  });
});

describe('hasLightDark', () => {
  it('asks CSS whether a colour can be a light-dark() pair', () => {
    const supports = vi.fn(() => true);
    expect(hasLightDark({ supports })).toBe(true);
    expect(supports).toHaveBeenCalledExactlyOnceWith('color', 'light-dark(#000, #fff)');
  });

  it('is false when CSS says no', () => {
    expect(hasLightDark(withoutLightDark)).toBe(false);
  });
});

describe('boot', () => {
  let app: Record<string, never>;
  afterEach(() => unmount(app));

  /** A browser with the test runtime's WebCrypto and a fresh, empty IndexedDB. */
  const browser = (replace: Partial<Browser> = {}): Browser => ({
    crypto,
    css: withLightDark,
    idb: () => new IDBFactory(),
    ...replace,
  });
  const notice = () => document.querySelector('h1')!.textContent;

  it('mounts the application once the device key is kept', async () => {
    const idb = new IDBFactory();
    app = await boot(document.body, browser({ idb: () => idb }));
    expect(document.querySelector('nav')).not.toBeNull();
    expect(notice()).toBe('Chats');
    expect((await idb.databases()).map((d) => d.name)).toStrictEqual(['solodawn']);
  });

  it('mounts the upgrade notice when the key cannot be kept', async () => {
    const idb = new IDBFactory();
    app = await boot(
      document.body,
      browser({ crypto: fakeCrypto({ generateKey: notSupported }).crypto, idb: () => idb }),
    );
    expect(document.querySelector('nav')).toBeNull();
    expect(notice()).toBe('This browser needs an update');
    expect(await idb.databases()).toStrictEqual([]);
  });

  it('mounts the upgrade notice when CSS has no light-dark()', async () => {
    app = await boot(document.body, browser({ css: withoutLightDark }));
    expect(document.querySelector('nav')).toBeNull();
    expect(notice()).toBe('This browser needs an update');
  });

  it('mounts the storage notice when the browser blocks site storage', async () => {
    const idb = () => {
      throw new DOMException('The operation is insecure.', 'SecurityError');
    };
    app = await boot(document.body, browser({ idb }));
    expect(document.querySelector('nav')).toBeNull();
    expect(notice()).toBe('Site storage is blocked');
  });

  it('fails when loading the device fails for another reason', async () => {
    // The stand-in WebCrypto passes the browser check but cannot export the public key.
    await expect(boot(document.body, browser({ crypto: fakeCrypto().crypto }))).rejects.toThrow(
      TypeError,
    );
    app = mount(App, { target: document.body });
  });
});
