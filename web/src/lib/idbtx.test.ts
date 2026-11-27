import { IDBDatabase as FakeDatabase, IDBFactory } from 'fake-indexeddb';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { runTx } from './idbtx';

afterEach(() => {
  vi.restoreAllMocks();
});

const upgradable = (idb: IDBFactory) =>
  new Promise<string>((resolve) => {
    const opening = idb.open('solodawn', 9);
    opening.onupgradeneeded = () => {
      opening.result.close();
      resolve('upgraded');
    };
    opening.onblocked = () => resolve('blocked');
  });

describe('runTx', () => {
  it('opens solodawn v1 device store, passes mode and options, resolves set value, and closes db', async () => {
    const idb = new IDBFactory();
    const transaction = vi.spyOn(FakeDatabase.prototype, 'transaction');

    await runTx<void>(
      () => idb,
      'readwrite',
      { durability: 'strict' },
      (store) => {
        store.put('saved', 'k');
      },
    );
    expect(transaction).toHaveBeenCalledWith('device', 'readwrite', { durability: 'strict' });

    const value = await runTx<string | undefined>(
      () => idb,
      'readonly',
      undefined,
      (store, set) => {
        const req = store.get('k');
        req.onsuccess = () => set(req.result as string | undefined);
      },
    );
    expect(value).toBe('saved');
    expect(transaction).toHaveBeenCalledWith('device', 'readonly', undefined);

    const meta = await new Promise<{ version: number; stores: string[] }>((resolve) => {
      const opening = idb.open('solodawn');
      opening.onsuccess = () => {
        const db = opening.result;
        const result = { version: db.version, stores: [...db.objectStoreNames] };
        db.close();
        resolve(result);
      };
    });
    expect(meta).toStrictEqual({ version: 1, stores: ['device'] });
    expect(await upgradable(idb)).toBe('upgraded');
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
    const work = vi.fn();
    await expect(runTx(() => idb, 'readonly', undefined, work)).rejects.toMatchObject({
      name: 'VersionError',
    });
    expect(work).not.toHaveBeenCalled();
  });

  it('closes the database and rejects when the transaction aborts', async () => {
    const idb = new IDBFactory();
    await expect(
      runTx<void>(
        () => idb,
        'readwrite',
        { durability: 'strict' },
        (store) => {
          store.add('first', 'k');
          store.add('second', 'k');
        },
      ),
    ).rejects.toMatchObject({ name: 'ConstraintError' });
    expect(await upgradable(idb)).toBe('upgraded');
  });

  it('rejects when idb() throws', async () => {
    const refusal = new DOMException('The operation is insecure.', 'SecurityError');
    await expect(
      runTx(
        () => {
          throw refusal;
        },
        'readonly',
        undefined,
        () => undefined,
      ),
    ).rejects.toBe(refusal);
  });
});
