// The demo of a development server: `?demo` opens the client on a simulated world, `?demo=fresh`
// on the same world before the browser paired with a computer (onboarding), `?demo=pairfail` on
// one whose computer the browser cannot reach, and `&reset` starts the world again. The client's
// own code runs unchanged; this replaces only what is behind its connection (peer.ts `simulate`)
// and the push service, and stores this browser's pairing with the world where the client keeps
// its own (owner.ts).
import { DeviceLog } from '../../src/lib/devlog';
import { newId } from '../../src/lib/groups.svelte';
import { loadDevice, type Device } from '../../src/lib/identity';
import { saveOwner } from '../../src/lib/owner';
import { simulate } from '../../src/lib/peer';
import { Director } from './director';
import { Events } from './events';
import { Panel } from './panel';
import { installReminders } from './reminders';
import { computers, seed } from './seed';
import { fakeConnect } from './session';
import { address, storageKey, World, type Mode, type Saved } from './world';

const modes: Mode[] = ['full', 'fresh', 'pairfail'];

/** Removes what this browser holds of a person and their homes; its device key stays. */
function forget(): Promise<void> {
  return new Promise((resolve, reject) => {
    const opening = indexedDB.open('solodawn', 1);
    opening.onupgradeneeded = () => opening.result.createObjectStore('device');
    opening.onerror = () => reject(opening.error);
    opening.onsuccess = () => {
      const db = opening.result;
      const tx = db.transaction('device', 'readwrite');
      for (const key of ['owner', 'joined']) tx.objectStore('device').delete(key);
      tx.oncomplete = () => (db.close(), resolve());
    };
  });
}

/** The browser is paired with both of the person's computers, as after onboarding. */
async function pair(device: Device, world: World) {
  const log = new DeviceLog(crypto, device.id);
  const entry = (ver: number, dev: string, kind: string, name: string) =>
    device.sign(
      JSON.stringify({
        t: 'device',
        n: newId(),
        ts: Date.now(),
        user: log.user,
        ver,
        op: 'add',
        dev,
        kind,
        name,
      }),
    );
  await log.apply(await entry(1, device.id, 'browser', 'Browser'));
  const own = [...world.nodes.values()].filter((node) => node.paired);
  for (const [i, node] of own.entries())
    await log.apply(await entry(i + 2, node.device, 'node', node.name));
  await saveOwner(() => indexedDB, log, {});
}

function read(): Saved | undefined {
  try {
    return JSON.parse(localStorage.getItem(storageKey) ?? 'null') ?? undefined;
  } catch {
    return undefined;
  }
}

/** A new world of the mode, stored, and the browser paired as that mode starts. */
async function create(world: World, device: Device) {
  localStorage.removeItem(storageKey);
  await forget();
  if (world.mode === 'full') {
    seed(world);
    await pair(device, world);
  } else {
    computers(world);
  }
  world.save();
}

/** The world of the address: the one stored, or a new one when the address asks for a reset or
 *  another mode, or the browser is not the one the stored world knows. */
async function open(mode: Mode, reset: boolean) {
  const device = await loadDevice({ crypto, idb: () => indexedDB });
  const saved = read();
  const world = new World(device.id, mode, localStorage);
  const fresh = reset || saved?.v !== 2 || saved.mode !== mode || saved.me !== device.id;
  if (fresh) await create(world, device);
  else {
    computers(world);
    world.restore(saved!);
  }
  return { world, fresh };
}

export async function install() {
  const params = new URLSearchParams(location.search);
  const mode = modes.find((each) => each === params.get('demo')) ?? 'full';
  const { world, fresh } = await open(mode, params.has('reset'));
  history.replaceState(null, '', address(mode) + location.hash);
  const director = new Director(world);
  director.resume();
  simulate(fakeConnect(world));
  installReminders();
  new Panel(world, new Events(world, director));
  if (fresh && mode !== 'full') location.hash = '#/onboarding';
  Object.assign(globalThis, { demo: { world, director } });
}
