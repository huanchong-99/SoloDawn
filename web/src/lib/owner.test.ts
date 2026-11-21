import { IDBDatabase as FakeDatabase, IDBFactory } from 'fake-indexeddb';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { DeviceLog } from './devlog';
import { Device, ed25519, encode } from './identity';
import { loadOwner, saveOwner, type StoredOwner } from './owner';

const newDevice = async () =>
  Device.of(crypto, await crypto.subtle.generateKey(ed25519, false, ['sign', 'verify']));
const makeId = () => encode(crypto.getRandomValues(new Uint8Array(16)));

afterEach(() => {
  vi.restoreAllMocks();
});

async function makeLog() {
  const browser = await newDevice();
  const log = new DeviceLog(crypto, browser.id);
  await log.apply(
    await browser.sign(
      JSON.stringify({
        t: 'device',
        n: makeId(),
        ts: 1,
        user: browser.id,
        ver: 1,
        op: 'add',
        dev: browser.id,
        kind: 'browser',
        name: 'Browser',
      }),
    ),
  );
  return { browser, log };
}

describe('loadOwner and saveOwner', () => {
  it('answers empty addrs before enrolment, then stores and replays under solodawn v1 device/owner', async () => {
    const idb = new IDBFactory();
    const transaction = vi.spyOn(FakeDatabase.prototype, 'transaction');
    expect(await loadOwner(crypto, () => idb)).toStrictEqual({ addrs: {} });
    expect(transaction).toHaveBeenCalledWith('device', 'readonly', undefined);

    const { browser, log } = await makeLog();
    const addrs = { node1: ['wss://rv.example/rv'] };
    await saveOwner(() => idb, log, addrs);
    expect(transaction).toHaveBeenCalledWith('device', 'readwrite', { durability: 'strict' });

    const raw = await new Promise<{ version: number; record: StoredOwner }>((resolve) => {
      const opening = idb.open('solodawn');
      opening.onsuccess = () => {
        const db = opening.result;
        const get = db.transaction('device').objectStore('device').get('owner');
        get.onsuccess = () => {
          const version = db.version;
          db.close();
          resolve({ version, record: get.result as StoredOwner });
        };
      };
    });
    expect(raw.version).toBe(1);
    expect(raw.record.user).toBe(browser.id);
    expect(raw.record.entries).toHaveLength(1);
    expect(raw.record.addrs).toStrictEqual(addrs);

    const loaded = await loadOwner(crypto, () => idb);
    expect(loaded.log?.user).toBe(browser.id);
    expect(loaded.log?.valid(browser.id)).toBe(true);
    expect(loaded.addrs).toStrictEqual(addrs);
    loaded.addrs.node1 = [];
    expect((await loadOwner(crypto, () => idb)).addrs).toStrictEqual({
      node1: ['wss://rv.example/rv'],
    });

    const upgraded = await new Promise<string>((resolve) => {
      const req = idb.open('solodawn', 9);
      req.onupgradeneeded = () => {
        req.result.close();
        resolve('upgraded');
      };
      req.onblocked = () => resolve('blocked');
    });
    expect(upgraded).toBe('upgraded');
  });

  it('rejects when opening the database fails', async () => {
    const idb = new IDBFactory();
    await new Promise<void>((resolve) => {
      const newer = idb.open('solodawn', 2);
      newer.onsuccess = () => {
        newer.result.close();
        resolve();
      };
    });
    await expect(loadOwner(crypto, () => idb)).rejects.toMatchObject({ name: 'VersionError' });
  });

  it('closes the database and rejects when the transaction aborts', async () => {
    const idb = new IDBFactory();
    const { log } = await makeLog();
    await saveOwner(() => idb, log, {});

    vi.spyOn(FakeDatabase.prototype, 'transaction').mockImplementationOnce(function (
      this: IDBDatabase,
      names,
      mode,
      options,
    ) {
      const tx = FakeDatabase.prototype.transaction.call(this, names, mode, options);
      const realStore = tx.objectStore.bind(tx);
      vi.spyOn(tx, 'objectStore').mockImplementation((name: string) => {
        const store = realStore(name);
        const realPut = store.put.bind(store);
        vi.spyOn(store, 'put').mockImplementation((value: unknown, key?: IDBValidKey) => {
          const req = realPut(value, key);
          tx.abort();
          return req;
        });
        return store;
      });
      return tx;
    });
    await expect(saveOwner(() => idb, log, {})).rejects.toBeDefined();

    const upgraded = await new Promise<string>((resolve) => {
      const req = idb.open('solodawn', 9);
      req.onupgradeneeded = () => {
        req.result.close();
        resolve('upgraded');
      };
      req.onblocked = () => resolve('blocked');
    });
    expect(upgraded).toBe('upgraded');
  });
});
