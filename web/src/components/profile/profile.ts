import type { Home } from '../../lib/groups.svelte';
import { nodes } from '../../lib/nodes.svelte';
import type { AIMemberRow } from '../../lib/proto';

export const tiers = ['light', 'standard', 'strong'] as const;
export type Tier = (typeof tiers)[number];

export type Draft = Pick<AIMemberRow, 'model' | 'effort' | 'tier' | 'skills'>;

export function newDraft(): Draft {
  return { model: '', effort: '', tier: 'standard', skills: '' };
}

export function suggestedTier(host: Home | undefined, ai: AIMemberRow): string | undefined {
  if (!host) return undefined;
  return nodes.infos
    .get(host)
    ?.agents.find((agent) => agent.agent === ai.cli)
    ?.models.find((model) => model.model === ai.model)?.tier;
}

export function profileChanges(
  ai: AIMemberRow,
  draft: Draft,
): Partial<Pick<Draft, 'tier' | 'skills'>> {
  return {
    ...(ai.tier !== draft.tier ? { tier: draft.tier } : {}),
    ...(ai.skills !== draft.skills ? { skills: draft.skills } : {}),
  };
}
