// Reading the fields of a signed payload that contracts/envelope.md sections 3 and 4 check. Fields
// are read by their exact names and the last of a repeated name counts, which is what JSON.parse
// does (contracts/conventions.md section 2). An integer is a number whose value is a safe integer:
// 2.0 is 2, and so is 2.0000000000000001 once parsed; 2^53 and 1e20 are not. No field read here
// may be null: null has the wrong type.
import type { DeviceEntry } from './proto';

/** The fields common to every payload. */
export type Header = Pick<DeviceEntry, 'n' | 'ts' | 'g' | 'to'> & { t: string };

/** The fields of a device-log entry that the log reads. */
export type Entry = Pick<DeviceEntry, 'user' | 'ver' | 'op' | 'dev'>;

type Fields = Record<string, unknown>;

/** p parsed, or undefined when it is not JSON or is null. A JSON value that is not an object has
 *  none of the fields, so the checks after this refuse it. */
function parse(p: string): Fields | undefined {
  let value: Fields | null | undefined;
  try {
    value = JSON.parse(p);
  } catch {
    // not JSON: value stays undefined
  }
  return value ?? undefined;
}

const optionalString = (value: unknown) => value === undefined || typeof value === 'string';

/** Section 3, check 3: t and n are strings, ts an integer, g and to strings when there. */
const isHeader = (o: Fields) =>
  typeof o.t === 'string' &&
  typeof o.n === 'string' &&
  Number.isSafeInteger(o.ts) &&
  optionalString(o.g) &&
  optionalString(o.to);

/** The common fields of a payload, or undefined when they are not as section 3 says
 *  (`bad_request`). */
export function readHeader(p: string): Header | undefined {
  const o = parse(p);
  return o !== undefined && isHeader(o) ? (o as Header) : undefined;
}

/** Section 4, rule 2, but for `dev` being a device id, which the log checks. n and ts are not
 *  read, nor kind and name on a revoke. */
const isEntry = (o: Fields) =>
  o.t === 'device' &&
  typeof o.user === 'string' &&
  typeof o.dev === 'string' &&
  Number.isSafeInteger(o.ver) &&
  isChange(o);

const isChange = (o: Fields) => o.op === 'revoke' || (o.op === 'add' && isAddition(o));

const isAddition = (o: Fields) =>
  (o.kind === 'node' || o.kind === 'browser') && typeof o.name === 'string';

/** A device-log entry, or undefined when its payload is not one. */
export function readEntry(p: string): Entry | undefined {
  const o = parse(p);
  return o !== undefined && isEntry(o) ? (o as Entry) : undefined;
}
