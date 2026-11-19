import { unmount } from 'svelte';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { boot, hasEd25519 } from './boot';

const cryptoWith = (generateKey: () => Promise<unknown>) =>
  ({ subtle: { generateKey } }) as unknown as Crypto;
const supported = cryptoWith(() => Promise.resolve({}));
const unsupported = cryptoWith(() => Promise.reject(new DOMException('', 'NotSupportedError')));

describe('hasEd25519', () => {
  it('asks WebCrypto for a non-extractable Ed25519 signing key', async () => {
    const generateKey = vi.fn(() => Promise.resolve({}));
    expect(await hasEd25519(cryptoWith(generateKey))).toBe(true);
    expect(generateKey).toHaveBeenCalledWith('Ed25519', false, ['sign', 'verify']);
  });

  it('is false when WebCrypto rejects the algorithm', async () => {
    expect(await hasEd25519(unsupported)).toBe(false);
  });

  it('is false when the page has no WebCrypto', async () => {
    expect(await hasEd25519({} as Crypto)).toBe(false);
  });
});

describe('boot', () => {
  let app: Record<string, never>;
  afterEach(() => unmount(app));

  it('mounts the application', async () => {
    app = await boot(document.body, supported);
    expect(document.querySelector('nav')).not.toBeNull();
    expect(document.querySelector('h1')!.textContent).toBe('Chats');
  });

  it('mounts the upgrade notice in its place', async () => {
    app = await boot(document.body, unsupported);
    expect(document.querySelector('nav')).toBeNull();
    expect(document.querySelector('h1')!.textContent).toBe('This browser needs an update');
  });
});
