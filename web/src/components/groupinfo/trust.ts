import type { Group } from '../../lib/groups.svelte';
import { aiMembers } from '../../lib/members';
import type { DeviceRow } from '../../lib/proto';

export function homeOwner(group: Group): string | undefined {
  if (group.home.owner) return group.home.me.user;
  return [...group.rows.values()].find(
    (row): row is DeviceRow => 'dev' in row && row.dev === group.home.device,
  )?.user;
}

/** The AIs that can be made secretary: never an external one (contracts/step-3/external.md
 *  section 2). */
export const secretaryCandidates = (group: Group) =>
  aiMembers(group).filter((ai) => ai.cli !== 'external');

export function secretaryWarning(
  account: string,
  owner: string,
  people: readonly string[],
  inviting = false,
): boolean {
  return account === 'personal' && people.some((user) => user !== owner) !== inviting;
}
