// The emergency stop (planning/product-rules.md R-64): an admin pauses every AI of a group
// (contracts/commands.md 1.1), and the owner of a computer takes all AIs there offline (section 2).
import type { Group, Home } from './groups.svelte';
import { aiMembers, hasAI, isAdmin, statusOf } from './members';
import { nodes } from './nodes.svelte';
import { t } from './prefs.svelte';
import type { AIMemberRow } from './proto';

/** Pauses the group, or resumes it. The group's row and a `system` row come back from the home. */
export const setPaused = (group: Group, paused: boolean) =>
  group.home.command({ t: 'group', g: group.g, op: paused ? 'pause' : 'resume' });

/** Whether this person may stop the AIs of the group: an admin, where a group has AIs. */
export const mayPause = (group: Group) => isAdmin(group) && hasAI(group);

/** How many AIs of the group run a turn now. */
export const working = (group: Group) =>
  aiMembers(group).filter((ai) => ai.state === 'working').length;

/** Whether the owner took the AIs of this computer offline, as its last `host.info` says. */
export const isOffline = (home: Home | undefined) => nodes.infos.get(home!)?.offline === true;

/** The computers of this person: those whose AIs are offline, and the others. */
export function computers(homes: readonly Home[]) {
  const mine = homes.filter((home) => home.owner);
  return {
    stopped: mine.filter((home) => isOffline(home)),
    running: mine.filter((home) => !isOffline(home)),
  };
}

/** Names as one phrase in the reader's language, with the space that Chinese
 *  text has beside a Latin name. */
export const names = (named: readonly { name: string }[], lang: string) =>
  new Intl.ListFormat(lang)
    .formatToParts(named.map(({ name }) => name))
    .map(({ type, value }) => (type === 'literal' ? value.replace('和', ' 和 ') : value))
    .join('');

/** Which AIs of one computer its groups show as offline. The node pushes no `host.info`; a change
 *  here is the sign that another browser of the owner took its AIs offline, or back. */
export const offlineAIs = (home: Home, homes: readonly Home[]) =>
  homes
    .flatMap((each) => each.groups)
    .flatMap(aiMembers)
    .filter((ai) => ai.host === home.device && ai.state === 'offline')
    .map((ai) => ai.id)
    .join(' ');

/** An AI's status, where `host` is its computer when this person owns that: it says so when they
 *  took it offline. A running turn or an open terminal tells more than the last `host.info`. */
export function statusIn(group: Group, ai: AIMemberRow, host: Home | undefined) {
  return isOffline(host) && ai.state !== 'working' && ai.state !== 'takeover'
    ? { st: 'offline', text: t('stop.ai.offline') }
    : statusOf(group, ai);
}
