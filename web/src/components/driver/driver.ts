// What the browser offers for an AI by its driver's declaration (planning/contracts/step-3/drivers.md
// section 6).
import type { Home } from '../../lib/groups.svelte';
import { nodes } from '../../lib/nodes.svelte';
import { accounts } from '../../lib/onboard.svelte';
import type { Driver } from '../../lib/proto';

/** The account types `driver` allows, in the client's order. */
export const offeredAccounts = (driver: Driver) =>
  accounts.filter((acct) => driver.accounts.includes(acct));

/** False when `host` is connected and its last `host.info` declares no takeover for `agent`. */
export const offersTerminal = (host: Home | undefined, agent: string) =>
  !host?.ctl ||
  (nodes.infos.get(host)?.agents.find((each) => each.agent === agent)?.driver.takeover ??
    'none') !== 'none';
