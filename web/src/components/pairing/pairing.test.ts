import { IDBFactory } from 'fake-indexeddb';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { DeviceLog } from '../../lib/devlog';
import { Device, ed25519, encode, utf8 } from '../../lib/identity';
import { capturePairHash, takeLinkSecret } from './mounts';
import { cliName, PairingSession, promptFor, signalUrlOf } from './session.svelte';
import { devlogFrames, loadOwner, saveOwner } from './store';

const newDevice = async () =>
  Device.of(crypto, await crypto.subtle.generateKey(ed25519, false, ['sign', 'verify']));
const makeId = () => encode(crypto.getRandomValues(new Uint8Array(16)));

afterEach(() => {
  location.hash = '';
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe('pairing prompt and URLs', () => {
  it('omits --rendezvous on the official origin and includes it elsewhere', () => {
    const code = '7K2M-9QXD-4F8T-WB3N';
    const official = promptFor('en', code, {
      origin: 'https://solodawn.org',
      protocol: 'https:',
      host: 'solodawn.org',
    });
    expect(official).toContain('https://solodawn.org/install.md');
    expect(official).toContain(code);
    expect(official).not.toContain('--rendezvous');

    const selfHosted = promptFor('zh-CN', code, {
      origin: 'https://rv.example:8443',
      protocol: 'https:',
      host: 'rv.example:8443',
    });
    expect(selfHosted).toContain('https://rv.example:8443/install.md');
    expect(selfHosted).toContain('--rendezvous wss://rv.example:8443/rv');
    expect(signalUrlOf({ protocol: 'http:', host: 'localhost:47800' })).toBe(
      'ws://localhost:47800/rv',
    );
  });

  it('reads CLI display labels and query or malformed hash pairing secrets', () => {
    expect(cliName('claude')).toBe('Claude Code');
    expect(cliName('codex')).toBe('Codex');
    expect(cliName('custom')).toBe('custom');
    expect(takeLinkSecret('?pair=7K2M-9QXD-4F8T-WB3N')).toBe('7K2M-9QXD-4F8T-WB3N');
    expect(takeLinkSecret('')).toBeUndefined();
    location.hash = '#pair=%E0%A4%A';
    capturePairHash();
    expect(location.hash).toBe('#/onboarding');
    expect(takeLinkSecret('')).toBe('%E0%A4%A');
  });
});

describe('owner store and devlog framing', () => {
  it('saves, replays and closes IndexedDB without blocking upgrades', async () => {
    const idb = new IDBFactory();
    expect((await loadOwner(crypto, () => idb)).log).toBeUndefined();
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
    await saveOwner(() => idb, log, { node1: ['wss://rv.example/rv'] });
    const loaded = await loadOwner(crypto, () => idb);
    expect(loaded.log?.user).toBe(browser.id);
    expect(loaded.log?.valid(browser.id)).toBe(true);
    expect(loaded.addrs).toStrictEqual({ node1: ['wss://rv.example/rv'] });

    const upgraded = await new Promise<string>((resolve) => {
      const req = idb.open('solodawn', 9);
      req.onupgradeneeded = () => resolve('upgraded');
      req.onblocked = () => resolve('blocked');
    });
    expect(upgraded).toBe('upgraded');
  });

  it('splits large device logs into 60 KiB frames in order', () => {
    expect(devlogFrames('u', [])).toStrictEqual([{ t: 'devlog', user: 'u', entries: [] }]);
    const entry = { p: 'x'.repeat(1800), k: 'k'.repeat(43), s: 's'.repeat(86) };
    const many = Array.from({ length: 40 }, () => entry);
    const frames = devlogFrames('u', many);
    expect(frames.length).toBeGreaterThan(1);
    for (const frame of frames) {
      expect(utf8(JSON.stringify(frame)).length).toBeLessThanOrEqual(61_440);
    }
    expect(frames.flatMap((f) => f.entries)).toHaveLength(40);
  });
});

describe('PairingSession mock and storage failure paths', () => {
  it('populates sample state and declares CLI accounts in mock mode', async () => {
    vi.stubGlobal('indexedDB', new IDBFactory());
    const session = new PairingSession('mock');
    await vi.waitFor(() => expect(session.stage).toBe('accounts'));
    expect(session.node?.name).toBe('workstation');
    expect(session.agents?.map((a) => a.agent)).toEqual(['claude', 'codex']);
    await session.setAccount('codex', 'team');
    expect(session.stage).toBe('ready');
    session.destroy();
  });

  it('reports storage failure when IndexedDB is blocked', async () => {
    vi.stubGlobal('indexedDB', {
      open: () => {
        throw new DOMException('Blocked', 'SecurityError');
      },
    });
    const session = new PairingSession('live');
    await vi.waitFor(() => expect(session.stage).toBe('failed'));
    expect(session.error).toEqual({ step: 1, key: 'storage', code: undefined });
    session.destroy();
  });
});
