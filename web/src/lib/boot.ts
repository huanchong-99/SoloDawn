import { mount } from 'svelte';
import App from '../App.svelte';
import Upgrade from '../Upgrade.svelte';

type Css = Pick<typeof CSS, 'supports'>;

const ed25519 = { name: 'Ed25519' } as const;
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

/** Mounts the application, or the upgrade notice in its place when the browser is below the
 *  baseline (docs/decisions/implemented/architecture/2026-11-20-browser-baseline.md). */
export async function boot(target: HTMLElement, crypto: Crypto, css: Css) {
  const supported = (await canKeepEd25519Key(crypto)) && hasLightDark(css);
  return mount(supported ? App : Upgrade, { target });
}
