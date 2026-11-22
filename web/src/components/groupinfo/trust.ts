import type { Group } from '../../lib/groups.svelte';
import type { DeviceRow } from '../../lib/proto';

export function homeOwner(group: Group): string | undefined {
  if (group.home.owner) return group.home.me.user;
  return [...group.rows.values()].find(
    (row): row is DeviceRow => 'dev' in row && row.dev === group.home.device,
  )?.user;
}

export function secretaryWarning(
  account: string,
  owner: string,
  people: readonly string[],
): boolean {
  return account === 'personal' && people.some((user) => user !== owner);
}
