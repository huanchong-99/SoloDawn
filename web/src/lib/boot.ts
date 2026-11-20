import { mount } from 'svelte';
import App from '../App.svelte';
import Notice from '../Notice.svelte';
import { ed25519, loadDevice, StorageBlocked, type KeyStore } from './identity';

type Css = Pick<typeof CSS, 'supports'>;

// Not empty: Safari's verify() returns false for every signature over an empty message.
const probe = new TextEncoder().encode('solodawn boot check');

/** C-9: the device key is a non-extractable Ed25519 key that IndexedDB keeps. IndexedDB stores a
 *  key as a structured clone, so a clone of the key must still sign: Firefox 129 to 135 makes the
 *  key but cannot read a clone of it back. False as well when the page has no WebCrypto. */
export async function canKeepEd25519Key(crypto: Crypto): Promise<boolean> {
  try {
    const { privateKey, publicKey } = await crypto.subtle.generateKey(ed25519, false, [
      'sign',
      'verify',
    ]);
    const signature = await crypto.subtle.sign(ed25519, structuredClone(privateKey), probe);
    return await crypto.subtle.verify(ed25519, publicKey, signature, probe);
  } catch {
    return false;
  }
}

/** Every colour in tokens.css is a light-dark() pair, and the build ships it as written. */
export const hasLightDark = (css: Css) => css.supports('color', 'light-dark(#000, #fff)');

/** What boot needs from the browser: its WebCrypto, CSS and storage. */
export interface Browser extends KeyStore {
  css: Css;
}

/** Which notice replaces the application, if any: the browser is below the baseline
 *  (docs/decisions/implemented/architecture/2026-11-20-browser-baseline.md), or it blocks the
 *  storage that keeps the device key. Loading the device creates the key on first run. */
async function notice(browser: Browser): Promise<'upgrade' | 'storage' | undefined> {
  if (!((await canKeepEd25519Key(browser.crypto)) && hasLightDark(browser.css))) return 'upgrade';
  return loadDevice(browser).then(() => undefined, blocked);
}

const blocked = (error: unknown) => {
  if (error instanceof StorageBlocked) return 'storage' as const;
  throw error;
};

/** Mounts the application, or a notice in its place. */
export async function boot(target: HTMLElement, browser: Browser) {
  const kind = await notice(browser);
  return kind ? mount(Notice, { target, props: { kind } }) : mount(App, { target });
}
