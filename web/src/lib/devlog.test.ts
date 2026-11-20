import { beforeAll, describe, expect, it, vi } from 'vitest';
import { DeviceLog, Refused } from './devlog';
import { Device, ed25519, encode, verifyEnvelope } from './identity';

const newDevice = async () =>
  Device.of(crypto, await crypto.subtle.generateKey(ed25519, false, ['sign', 'verify']));

let first: Device; // the person's first device: its id is the user id
let second: Device;
let third: Device;

beforeAll(async () => {
  [first, second, third] = await Promise.all([newDevice(), newDevice(), newDevice()]);
});

/** The payload of a log entry of first's person; an add names a node. */
const change = (ver: number | string, op: string, dev: string, user = first.id) =>
  `{"t":"device","n":"n${ver}","ts":1,"user":"${user}","ver":${ver},"op":"${op}","dev":"${dev}"` +
  (op === 'add' ? ',"kind":"node","name":"workstation"}' : '}');

const signed = (by: Device, ver: number | string, op: string, dev: Device | string) =>
  by.sign(change(ver, op, typeof dev === 'string' ? dev : dev.id));

const genesis = () => signed(first, 1, 'add', first);

/** first's log with the given entries applied. */
async function logWith(...entries: Promise<{ p: string; k: string; s: string }>[]) {
  const log = new DeviceLog(crypto, first.id);
  for (const env of entries) await log.apply(await env);
  return log;
}

const refusal = (promise: Promise<unknown>) =>
  promise.then(
    () => 'accepted',
    (e: unknown) => (e instanceof Refused ? e.reason : e),
  );

describe('DeviceLog.apply', () => {
  it('accepts the genesis and the next versions', async () => {
    const log = new DeviceLog(crypto, first.id);
    const one = await genesis();
    expect(await log.apply(one)).toBe(true);
    const two = await signed(first, 2, 'add', second);
    expect(await log.apply(two)).toBe(true);
    expect(log.entries).toStrictEqual([one, two]);
    expect(log.user).toBe(first.id);
    expect(log.valid(second.id)).toBe(true);
    expect(log.valid(third.id)).toBe(false);
  });

  it.each([
    ['an add of another device', () => signed(first, 1, 'add', second)],
    ['an add signed by another device', () => signed(second, 1, 'add', first)],
    ['a revoke of the first device', () => signed(first, 1, 'revoke', first)],
  ])('refuses as entry 1 %s', async (_, env) => {
    const log = new DeviceLog(crypto, first.id);
    expect(await refusal(log.apply(await env()))).toBe('genesis');
    expect(log.entries).toStrictEqual([]);
  });

  it('refuses an entry 1 of another person', async () => {
    const log = new DeviceLog(crypto, first.id);
    const theirs = await second.sign(change(1, 'add', second.id, second.id));
    expect(await refusal(log.apply(theirs))).toBe('person');
  });

  it('refuses a version that does not follow the log', async () => {
    const log = await logWith(genesis());
    expect(await refusal(log.apply(await signed(first, 3, 'add', second)))).toBe('gap');
    expect(await refusal(log.apply(await signed(first, 0, 'add', second)))).toBe('gap');
    expect(await refusal(log.apply(await signed(first, -1, 'add', second)))).toBe('gap');
    expect(log.entries).toHaveLength(1);
  });

  it('keeps the first entry accepted for a version, and takes the same entry again', async () => {
    const one = await genesis();
    const two = await signed(first, 2, 'add', second);
    const log = await logWith(Promise.resolve(one), Promise.resolve(two));
    expect(await refusal(log.apply(await signed(first, 2, 'add', third)))).toBe('fork');
    expect(await refusal(log.apply({ ...two, s: (await second.sign(two.p)).s }))).toBe('bad_sig');
    expect(await refusal(log.apply(await second.sign(two.p)))).toBe('fork');
    // The same payload signed again: Node's signatures are deterministic, so it differs only in k.
    expect(await log.apply(two)).toBe(false);
    expect(await log.apply(one)).toBe(false);
    expect(log.entries).toStrictEqual([one, two]);
  });

  it('refuses an entry signed by a device that is not valid before it', async () => {
    const log = await logWith(genesis(), signed(first, 2, 'add', second));
    expect(await refusal(log.apply(await signed(third, 3, 'add', third)))).toBe('unknown_device');
    await log.apply(await signed(first, 3, 'revoke', second));
    expect(await refusal(log.apply(await signed(second, 4, 'add', third)))).toBe('unknown_device');
  });

  it('never adds a revoked device again', async () => {
    const log = await logWith(
      genesis(),
      signed(first, 2, 'add', second),
      signed(first, 3, 'revoke', second),
    );
    expect(await refusal(log.apply(await signed(first, 4, 'add', second)))).toBe('revoked');
    expect(log.valid(second.id)).toBe(false);
    expect(await log.apply(await signed(first, 4, 'revoke', second))).toBe(true);
  });

  it('allows a second add, and an add after an add', async () => {
    const log = await logWith(genesis(), signed(first, 2, 'add', second));
    expect(await log.apply(await signed(first, 3, 'add', second))).toBe(true);
    expect(log.validDevices()).toHaveLength(2);
  });

  it.each([
    ['a signature by another key', async () => ({ ...(await genesis()), k: second.id })],
    ['a changed payload', async () => ({ ...(await genesis()), p: change(1, 'add', second.id) })],
  ])('refuses %s', async (_, env) => {
    expect(await refusal(new DeviceLog(crypto, first.id).apply(await env()))).toBe('bad_sig');
  });

  it.each([
    [
      'another payload type',
      () => first.sign(change(1, 'add', first.id).replace('device', 'post')),
    ],
    ['another op', () => signed(first, 1, 'rename', first)],
    ['a dev that is not canonical', () => signed(first, 1, 'add', first.id.slice(0, -1) + '=')],
    ['a dev of 31 bytes', () => signed(first, 1, 'add', 'A'.repeat(41) + 'A')],
    ['a dev that is a weak key', () => signed(first, 1, 'add', 'A'.repeat(43))],
    ['a version that is not an integer', () => signed(first, '1.5', 'add', first)],
    ['a version that is a string', () => signed(first, '"1"', 'add', first)],
    [
      'a user that is not a string',
      () => first.sign(change(1, 'add', first.id).replace(`"${first.id}"`, '5')),
    ],
    [
      'a dev that is missing',
      () => first.sign(change(1, 'add', first.id).replace(/,"dev":"[^"]*"/, '')),
    ],
    [
      'an add of another kind',
      () => first.sign(change(1, 'add', first.id).replace('node', 'phone')),
    ],
    [
      'an add of a browser without a name',
      () =>
        first.sign(
          change(1, 'add', first.id)
            .replace('node', 'browser')
            .replace(',"name":"workstation"', ''),
        ),
    ],
    [
      'an add whose name is not a string',
      () => first.sign(change(1, 'add', first.id).replace('"workstation"', '7')),
    ],
    ['an array', () => first.sign('[]')],
    ['null', () => first.sign('null')],
    ['text that is not JSON', () => first.sign('device')],
  ])('refuses %s as not a device-log entry', async (_, env) => {
    expect(await refusal(new DeviceLog(crypto, first.id).apply(await env()))).toBe('bad_request');
  });

  it('adds a dev that is no point of the curve, and never imports a dev as a key', async () => {
    // y = 2, for which the curve has no x: 32 bytes in their canonical spelling that are no key.
    const noPoint = 'Ag' + 'A'.repeat(41);
    const log = await logWith(genesis());
    const importKey = vi.spyOn(crypto.subtle, 'importKey');
    const add = await signed(first, 2, 'add', noPoint);
    expect(await log.apply(add)).toBe(true);
    expect(await log.apply(await signed(first, 3, 'revoke', noPoint))).toBe(true);
    const imported = importKey.mock.calls.map(([, key]) => encode(key as Uint8Array));
    expect(imported).toStrictEqual([first.id, first.id]);
    importKey.mockRestore();
    // Nothing verifies under it, so it can never sign an entry.
    expect(await verifyEnvelope(crypto, { ...add, k: noPoint })).toBe(false);
    expect(await refusal(log.apply({ ...add, k: noPoint }))).toBe('bad_sig');
  });

  it('refuses an entry whose p is more than 2,048 bytes as UTF-8', async () => {
    const two = change(2, 'add', second.id);
    const bytes = (p: string) => new TextEncoder().encode(p).length;
    // Filled with spaces, and with a name of two bytes to a character: fewer than 2,048
    // characters are still too many bytes.
    const spaces = (size: number) => two + ' '.repeat(size - bytes(two));
    const name = (size: number) => {
      const room = size - bytes(two);
      const fill = 'é'.repeat(Math.floor(room / 2)) + 'x'.repeat(room % 2);
      return two.replace('workstation', 'workstation' + fill);
    };
    for (const fill of [spaces, name]) {
      const log = await logWith(genesis());
      expect(bytes(fill(2049))).toBe(2049);
      expect(await refusal(log.apply(await first.sign(fill(2049))))).toBe('bad_request');
      expect(bytes(fill(2048))).toBe(2048);
      expect(await log.apply(await first.sign(fill(2048)))).toBe(true);
    }
    expect(name(2049).length).toBeLessThan(2048);
  });

  it('reads what section 4 reads and nothing else', async () => {
    const without = (p: string) => p.replace(/"n":"[^"]*","ts":1,/, '');
    const log = new DeviceLog(crypto, first.id);
    // No n and no ts; ver written 2.0 and a VER beside it; kind and name not read on a revoke.
    expect(await log.apply(await first.sign(without(change(1, 'add', first.id))))).toBe(true);
    const two = change('2.0', 'add', second.id).replace('"ver"', '"VER":9,"ver"');
    expect(await log.apply(await first.sign(two))).toBe(true);
    const three = change(3, 'revoke', second.id).replace('}', ',"kind":5}');
    expect(await log.apply(await first.sign(three))).toBe(true);
    expect(log.validDevices()).toStrictEqual([first.id]);
  });

  it('never revokes the last valid device', async () => {
    const log = await logWith(genesis(), signed(first, 2, 'add', second));
    expect(await log.apply(await signed(second, 3, 'revoke', first))).toBe(true);
    expect(await refusal(log.apply(await signed(second, 4, 'revoke', second)))).toBe('last');
    // A revoke of a device that is not valid removes nothing.
    expect(await log.apply(await signed(second, 4, 'revoke', third))).toBe(true);
    expect(log.validDevices()).toStrictEqual([second.id]);
  });

  it('refuses a revoke of the only device as entry 2', async () => {
    const log = await logWith(genesis());
    expect(await refusal(log.apply(await signed(first, 2, 'revoke', first)))).toBe('last');
  });

  it('checks entries that arrive at the same time one after the other', async () => {
    const log = await logWith(genesis());
    const [a, b] = [await signed(first, 2, 'add', second), await signed(first, 2, 'add', third)];
    // Which verification finishes first is up to WebCrypto: one entry wins, the other is a fork.
    const results = await Promise.all([refusal(log.apply(a)), refusal(log.apply(b))]);
    expect(results.sort()).toStrictEqual(['accepted', 'fork']);
    expect(log.entries).toHaveLength(2);
  });

  it('names the reason of a refusal', () => {
    const refused = new Refused('fork');
    expect(refused.name).toBe('Refused');
    expect(refused.message).toBe('device log: fork');
  });
});

describe('DeviceLog.replay', () => {
  it('verifies a whole log from entry 1', async () => {
    const entries = [await genesis(), await signed(first, 2, 'add', second)];
    const log = await DeviceLog.replay(crypto, first.id, entries);
    expect(log.entries).toStrictEqual(entries);
    expect(log.valid(second.id)).toBe(true);
  });

  it('fails with the first refusal', async () => {
    const entries = [await genesis(), await signed(third, 2, 'add', second)];
    expect(await refusal(DeviceLog.replay(crypto, first.id, entries))).toBe('unknown_device');
  });
});

describe('DeviceLog.validDevices', () => {
  it('lists each valid device once, in byte order', async () => {
    const log = await logWith(
      genesis(),
      signed(first, 2, 'add', second),
      signed(first, 3, 'add', second),
      signed(first, 4, 'add', third),
      signed(first, 5, 'revoke', third),
    );
    expect(log.validDevices()).toStrictEqual([first.id, second.id].sort());
  });
});

describe('DeviceLog.adopts', () => {
  it('adopts when the peer added this device and both are valid at the end', async () => {
    const log = await logWith(genesis(), signed(first, 2, 'add', second));
    expect(log.adopts(second.id, first.id)).toBe(true);
  });

  it('does not adopt when another device added this one', async () => {
    const log = await logWith(
      genesis(),
      signed(first, 2, 'add', third),
      signed(third, 3, 'add', second),
    );
    expect(log.adopts(second.id, first.id)).toBe(false);
    expect(log.adopts(second.id, third.id)).toBe(true);
  });

  it('does not adopt when this device or the peer is revoked by the end', async () => {
    const peerGone = await logWith(
      genesis(),
      signed(first, 2, 'add', third),
      signed(third, 3, 'add', second),
      signed(first, 4, 'revoke', third),
    );
    expect(peerGone.adopts(second.id, third.id)).toBe(false);
    const meGone = await logWith(
      genesis(),
      signed(first, 2, 'add', second),
      signed(first, 3, 'revoke', second),
    );
    expect(meGone.adopts(second.id, first.id)).toBe(false);
  });

  it('does not adopt a device the log never adds', async () => {
    const log = await logWith(genesis(), signed(first, 2, 'revoke', second));
    expect(log.adopts(second.id, first.id)).toBe(false);
    expect(log.adopts(third.id, first.id)).toBe(false);
  });
});
