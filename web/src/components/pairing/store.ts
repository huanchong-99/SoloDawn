// Persistence of the owner's device log and node signalling addresses in IndexedDB (`solodawn`
// version 1, store `device`, key `owner`, beside the device key of identity.ts), plus splitting
// device logs into 60 KiB `devlog` frames (contracts/conventions.md section 4).
import { DeviceLog } from '../../lib/devlog';
import { utf8 } from '../../lib/identity';
import type { Devlog, Envelope } from '../../lib/proto';

const ctlLimit = 61_440;
const ownerKey = 'owner';

/** What the browser stores once enrolled as a device of its owner. */
export interface StoredOwner {
  user: string;
  entries: Envelope[];
  addrs: Record<string, string[]>;
}

function openStore(idb: () => IDBFactory): Promise<IDBDatabase> {
  return new Promise((resolve, reject) => {
    const opening = idb().open('solodawn', 1);
    opening.onupgradeneeded = () => {
      if (!opening.result.objectStoreNames.contains('device')) {
        opening.result.createObjectStore('device');
      }
    };
    opening.onerror = () => reject(opening.error);
    opening.onsuccess = () => resolve(opening.result);
  });
}

function readRecord(idb: () => IDBFactory): Promise<StoredOwner | undefined> {
  return openStore(idb).then(
    (db) =>
      new Promise((resolve, reject) => {
        const tx = db.transaction('device', 'readonly');
        const req = tx.objectStore('device').get(ownerKey);
        let value: StoredOwner | undefined;
        req.onsuccess = () => (value = req.result as StoredOwner | undefined);
        tx.oncomplete = () => {
          db.close();
          resolve(value);
        };
        tx.onabort = () => {
          db.close();
          reject(tx.error);
        };
      }),
  );
}

/** Loads and verifies the stored owner log, if this browser already belongs to a person. */
export async function loadOwner(
  crypto: Crypto,
  idb: () => IDBFactory,
): Promise<{ log?: DeviceLog; addrs: Record<string, string[]> }> {
  const stored = await readRecord(idb);
  if (!stored) return { addrs: {} };
  const log = await DeviceLog.replay(crypto, stored.user, stored.entries);
  return { log, addrs: { ...stored.addrs } };
}

/** Stores the verified owner log and remembered node signalling URLs durably. */
export async function saveOwner(
  idb: () => IDBFactory,
  log: DeviceLog,
  addrs: Record<string, string[]>,
): Promise<void> {
  const db = await openStore(idb);
  return new Promise((resolve, reject) => {
    const tx = db.transaction('device', 'readwrite', { durability: 'strict' });
    const record: StoredOwner = { user: log.user, entries: [...log.entries], addrs };
    tx.objectStore('device').put(record, ownerKey);
    tx.oncomplete = () => {
      db.close();
      resolve();
    };
    tx.onabort = () => {
      db.close();
      reject(tx.error);
    };
  });
}

/** Packs device-log entries in order into `devlog` frames of at most 60 KiB each. */
export function devlogFrames(user: string, entries: readonly Envelope[]): Devlog[] {
  if (entries.length === 0) return [{ t: 'devlog', user, entries: [] }];
  const frames: Devlog[] = [];
  let batch: Envelope[] = [];
  for (const entry of entries) {
    const candidate: Devlog = { t: 'devlog', user, entries: [...batch, entry] };
    if (batch.length > 0 && utf8(JSON.stringify(candidate)).length > ctlLimit) {
      frames.push({ t: 'devlog', user, entries: batch });
      batch = [entry];
    } else {
      batch.push(entry);
    }
  }
  frames.push({ t: 'devlog', user, entries: batch });
  return frames;
}
