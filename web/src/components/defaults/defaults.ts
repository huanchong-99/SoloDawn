// Computer-level CLI defaults, one-line profile summaries, and follower counts
// (planning/contracts/step-3/profiles.md sections 1a and 2a).
import { homes, type Home } from '../../lib/groups.svelte';
import { aiMembers } from '../../lib/members';
import { nodes } from '../../lib/nodes.svelte';
import { t } from '../../lib/prefs.svelte';
import type * as p from '../../lib/proto';
import type { Listed } from '../model/model';
import type { Tier } from '../profile/profile';

/** A profile's four fields when a CLI's defaults have not been set yet. */
export const unsetProfile: Readonly<p.Profile> = Object.freeze({
  model: '',
  effort: '',
  tier: 'standard',
  skills: '',
});

/** Parts of the one-line summary for a profile: resolved model and effort labels, work-share
 *  tier with its label, and optional skills. */
export interface SummaryParts {
  model: string;
  effort: string;
  tier: Tier;
  tierLabel: string;
  skills?: string;
}

const modelLabel = (id: string, models: readonly Listed[]) =>
  id ? (models.find((entry) => entry.model === id)?.name ?? id) : t('model.default');

/** Formats the parts of the one-line summary for `profile` against `models`. */
export function summaryParts(profile: p.Profile, models: readonly Listed[]): SummaryParts {
  const tier = profile.tier as Tier;
  return {
    model: modelLabel(profile.model, models),
    effort: profile.effort || t('model.default'),
    tier,
    tierLabel: t(`profile.tier.${tier}`),
    ...(profile.skills ? { skills: profile.skills } : {}),
  };
}

/** Configured defaults for `agent` on `home`, or `undefined` when not yet set. */
export function defaultsOf(home: Home, agent: string): p.Profile | undefined {
  return nodes.infos.get(home)?.agents.find((entry) => entry.agent === agent)?.defaults;
}

/** Counts live AI members across groups this browser reads that follow `agent` on `device`. */
export function followers(device: string, agent: string): number {
  return homes
    .flatMap((home) => home.groups)
    .flatMap(aiMembers)
    .filter((ai) => ai.host === device && ai.cli === agent && ai.follow).length;
}

/** Returns a mutable draft initialized from `defaults` or `unsetProfile`. */
export function draftOf(defaults: p.Profile | undefined): p.Profile {
  return { ...(defaults ?? unsetProfile) };
}

/** Whether two profiles hold identical model, effort, tier and skills values. */
export function sameProfile(a: p.Profile, b: p.Profile): boolean {
  return a.model === b.model && a.effort === b.effort && a.tier === b.tier && a.skills === b.skills;
}
