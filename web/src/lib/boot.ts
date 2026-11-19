import { mount } from 'svelte';
import App from '../App.svelte';
import Upgrade from '../Upgrade.svelte';

/** C-9: the client needs native Ed25519 in WebCrypto. Without it (or without WebCrypto at all,
 *  as on an insecure origin) generating a key throws or rejects. */
export async function hasEd25519(crypto: Crypto): Promise<boolean> {
  try {
    await crypto.subtle.generateKey('Ed25519', false, ['sign', 'verify']);
    return true;
  } catch {
    return false;
  }
}

/** Mounts the application, or the upgrade notice in its place. */
export async function boot(target: HTMLElement, crypto: Crypto) {
  return mount((await hasEd25519(crypto)) ? App : Upgrade, { target });
}
