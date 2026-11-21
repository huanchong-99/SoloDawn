// Pairing area mount entry and #pair=<secret> link capture (planning/flows/pairing.md section C).
import type { Mounts } from '../../lib/mounts.svelte';

let pending: string | undefined;

function decodePart(raw: string): string {
  try {
    return decodeURIComponent(raw);
  } catch {
    return raw;
  }
}

/** Captures a `#pair=<secret>` address fragment and opens `#/onboarding` so the one-time secret
 *  leaves the address bar while onboarding joins the room in role `b`. */
export function capturePairHash() {
  if (!location.hash.startsWith('#pair=')) return;
  pending = decodePart(location.hash.slice('#pair='.length));
  location.hash = '#/onboarding';
}

/** Claims the secret from a `#pair=<secret>` link or `?pair=<secret>` query, once. */
export function takeLinkSecret(search = location.search): string | undefined {
  capturePairHash();
  if (pending !== undefined) {
    const taken = pending;
    pending = undefined;
    return taken;
  }
  return new URLSearchParams(search).get('pair') ?? undefined;
}

capturePairHash();
addEventListener('hashchange', capturePairHash);

export default {} satisfies Mounts;
