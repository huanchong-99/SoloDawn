// Persistence of the owner's device log and node signalling addresses in IndexedDB (`solodawn`
// version 1, store `device`, key `owner`, beside the device key of identity.ts).
import { DeviceLog } from './devlog';
import { runTx } from './idbtx';
import type { Envelope } from './proto';

const ownerKey = 'owner';
const joinedKey = 'joined';
export type JoinedHomes = Record<string, { name: string; url: string }>;

/** What the browser stores once enrolled as a device of its owner. */
export interface StoredOwner {
  user: string;
  entries: Envelope[];
  addrs: Record<string, string[]>;
}

/** Loads and verifies the stored owner log, if this browser already belongs to a person. */
export async function loadOwner(
  crypto: Crypto,
  idb: () => IDBFactory,
): Promise<{ log?: DeviceLog; addrs: Record<string, string[]> }> {
  const stored = await runTx<StoredOwner | undefined>(idb, 'readonly', undefined, (store, set) => {
    const req = store.get(ownerKey);
    req.onsuccess = () => set(req.result as StoredOwner | undefined);
  });
  if (!stored) return { addrs: {} };
  const log = await DeviceLog.replay(crypto, stored.user, stored.entries);
  return { log, addrs: { ...stored.addrs } };
}

/** Stores the verified owner log and remembered node signalling URLs durably. */
export function saveOwner(
  idb: () => IDBFactory,
  log: DeviceLog,
  addrs: Record<string, string[]>,
): Promise<void> {
  const record: StoredOwner = { user: log.user, entries: [...log.entries], addrs };
  return runTx<void>(idb, 'readwrite', { durability: 'strict' }, (store) => {
    store.put(record, ownerKey);
  });
}

export function loadJoined(idb: () => IDBFactory) {
  return runTx<JoinedHomes>(idb, 'readonly', undefined, (store, set) => {
    const req = store.get(joinedKey);
    req.onsuccess = () => set((req.result as JoinedHomes | undefined) ?? {});
  });
}

export function saveJoined(idb: () => IDBFactory, homes: JoinedHomes) {
  return runTx<void>(idb, 'readwrite', { durability: 'strict' }, (store) => {
    store.put(homes, joinedKey);
  });
}
