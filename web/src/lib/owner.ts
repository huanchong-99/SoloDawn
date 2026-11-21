// Persistence of the owner's device log and node signalling addresses in IndexedDB (`solodawn`
// version 1, store `device`, key `owner`, beside the device key of identity.ts).
import { DeviceLog } from './devlog';
import type { Envelope } from './proto';

const ownerKey = 'owner';

/** What the browser stores once enrolled as a device of its owner. */
export interface StoredOwner {
  user: string;
  entries: Envelope[];
  addrs: Record<string, string[]>;
}

function runTx<T>(
  idb: () => IDBFactory,
  mode: IDBTransactionMode,
  options: IDBTransactionOptions | undefined,
  work: (store: IDBObjectStore, set: (value: T) => void) => void,
): Promise<T> {
  return new Promise((resolve, reject) => {
    const opening = idb().open('solodawn', 1);
    opening.onupgradeneeded = () => opening.result.createObjectStore('device');
    opening.onerror = () => reject(opening.error);
    opening.onsuccess = () => {
      const db = opening.result;
      const tx = db.transaction('device', mode, options);
      let value!: T;
      work(tx.objectStore('device'), (v) => (value = v));
      tx.oncomplete = () => {
        db.close();
        resolve(value);
      };
      tx.onabort = () => {
        db.close();
        reject(tx.error);
      };
    };
  });
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
