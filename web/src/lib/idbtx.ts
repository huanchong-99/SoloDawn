// Opening, running and closing one IndexedDB transaction on database `solodawn` version 1,
// object store `device`, shared by identity.ts and owner.ts.

/** Opens IndexedDB (`solodawn` version 1, object store `device`), runs `work` in one transaction,
 *  and closes the database when the transaction completes or aborts. */
export function runTx<T>(
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
      // A browser without the durability option ignores it.
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
