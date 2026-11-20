// A person's device log (contracts/envelope.md section 4). internal/identity/devlog.go is the Go
// implementation; both are held to the same test vectors.
import { isDevice, utf8, verifyEnvelope } from './identity';
import { readEntry, type Entry } from './payload';
import type { Envelope } from './proto';

/** Why an entry is refused: an envelope code, or one of the log's own rules. */
export type Reason =
  | 'bad_sig' // the signature does not cover the entry
  | 'bad_request' // not a device-log entry (section 4, rule 2)
  | 'unknown_device' // the signer is not valid before the entry
  | 'person' // the entry belongs to another person's log
  | 'genesis' // entry 1 must add its own signer, the person's first device
  | 'gap' // the version does not follow the log
  | 'fork' // a different entry already holds this version
  | 'revoked' // a revoked device is never added again
  | 'last'; // a revoke does not remove the last valid device

export class Refused extends Error {
  constructor(readonly reason: Reason) {
    super(`device log: ${reason}`);
    this.name = 'Refused';
  }
}

/** The device log of one person: the entries accepted so far and the valid devices that follow
 *  from them. */
export class DeviceLog {
  /** Exactly as signed; version v is entries[v - 1]. For storing and passing on. */
  readonly entries: Envelope[] = [];
  private readonly changes: Entry[] = [];

  /** user is the person: the device id of their first device. */
  constructor(
    private readonly crypto: Crypto,
    readonly user: string,
  ) {}

  /** A whole log of user, verified from entry 1. Rejects with the first entry's refusal. */
  static async replay(crypto: Crypto, user: string, entries: Envelope[]): Promise<DeviceLog> {
    const log = new DeviceLog(crypto, user);
    for (const env of entries) await log.apply(env);
    return log;
  }

  /** Checks one signed entry against the log and appends it. Resolves to false for an entry the
   *  log already holds: logs are passed on between devices, so the same entry arrives more than
   *  once. Rejects with Refused, and leaves the log as it was. */
  async apply(env: Envelope): Promise<boolean> {
    const e = await entry(this.crypto, env);
    // From here on nothing waits, so entries applied at the same time are checked one by one.
    if (e.user !== this.user) throw new Refused('person');
    if (e.ver !== this.entries.length + 1) return this.held(e.ver, env);
    this.follows(e, env.k);
    this.entries.push(env);
    this.changes.push(e);
    return true;
  }

  /** Whether a device is in the valid set. */
  valid(device: string): boolean {
    return this.last(device) === 'add';
  }

  /** The valid devices, in byte order of their ids. */
  validDevices(): string[] {
    return [...new Set(this.changes.map((c) => c.dev))].filter((d) => this.valid(d)).sort();
  }

  /** Whether a device that belongs to no person takes this log's person as its owner (section 6):
   *  the log adds the device in an entry signed by the peer it paired with, and both are valid at
   *  the end of the log. Whether the device is free to adopt, and on which connection the log
   *  arrived, is for the caller to know. */
  adopts(me: string, peer: string): boolean {
    // A device that is valid was never revoked, so every entry about it is an add.
    const addedByPeer = this.changes.some((c, i) => c.dev === me && this.entries[i].k === peer);
    return this.valid(me) && this.valid(peer) && addedByPeer;
  }

  /** An entry whose version is not the next one. The first entry accepted for a version wins;
   *  the same entry again is not an error. */
  private held(version: number, env: Envelope): false {
    if (version < 1 || version > this.entries.length) throw new Refused('gap');
    if (same(this.entries[version - 1]) !== same(env)) throw new Refused('fork');
    return false;
  }

  /** The entry for the next version, checked against the log before it. */
  private follows(e: Entry, signer: string): void {
    if (this.entries.length === 0) return genesis(e, signer, this.user);
    if (!this.valid(signer)) throw new Refused('unknown_device');
    const reason = this.forbids(e);
    if (reason !== undefined) throw new Refused(reason);
  }

  /** Rule 6: an add never names a device that was ever revoked, and a revoke never removes the
   *  last valid device, which would leave a log that could never change again. */
  private forbids(e: Entry): Reason | undefined {
    if (e.op === 'add') return this.last(e.dev) === 'revoke' ? 'revoked' : undefined;
    return this.valid(e.dev) && this.validDevices().length === 1 ? 'last' : undefined;
  }

  /** The op of the latest entry about a device, if any. A revoke stays the latest: no add is
   *  accepted after it. */
  private last(device: string): string | undefined {
    return this.changes.findLast((c) => c.dev === device)?.op;
  }
}

/** An envelope as one text, for comparing entries: the same p, k and s. p counts as the text that
 *  is signed, where TextEncoder writes a lone surrogate as U+FFFD, and as a Go node reads it: the
 *  escape "\ud800" and U+FFFD are one p. */
const same = (env: Envelope) => JSON.stringify([env.p.toWellFormed(), env.k, env.s]);

/** Entry 1: dev, k and user are the same key, and it is an add. */
function genesis(e: Entry, signer: string, user: string): void {
  if (e.op !== 'add' || e.dev !== user || signer !== user) throw new Refused('genesis');
}

/** The most bytes the payload of an entry may have, as UTF-8, so that its device row stays within
 *  the size of a row (section 4, rule 2). */
const maxEntry = 2048;

/** A signed device-log entry. Entries are replayed long after they were written and are
 *  addressed to nobody, so of the envelope checks only the signature applies. dev is a device id
 *  by its bytes alone: it is never imported as a key, and need not be a point of the curve. */
async function entry(crypto: Crypto, env: Envelope): Promise<Entry> {
  if (!(await verifyEnvelope(crypto, env))) throw new Refused('bad_sig');
  const e = utf8(env.p).length > maxEntry ? undefined : readEntry(env.p);
  if (e === undefined || !isDevice(e.dev)) throw new Refused('bad_request');
  return e;
}
