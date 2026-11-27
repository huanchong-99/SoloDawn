import type { Home } from '../../lib/groups.svelte';
import { nodes } from '../../lib/nodes.svelte';
import type { Draft } from '../../lib/members';
import type { AIMemberRow } from '../../lib/proto';

export const tiers = ['light', 'standard', 'strong'] as const;
export type Tier = (typeof tiers)[number];

export type { Draft };

/** Fields accepted by `member set` for profile edits or restoring defaults. */
export type MemberUpdate = Partial<Pick<Draft, 'model' | 'effort' | 'tier' | 'skills'>> & {
  follow?: true;
};

/** Display state of the defaults setting row on an AI member card. */
export type SettingState =
  | { shown: false }
  | { shown: true; follow: true }
  | { shown: true; follow: false; restore: boolean };

export function newDraft(): Draft {
  return { model: '', effort: '', tier: 'standard', skills: '' };
}

/** Whether the draft's skills exceed the 200 code points of the contract. */
export const tooLong = (draft: Draft) => [...draft.skills].length > 200;

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

/** Whether a member card shows the defaults setting row, and whether its owner can restore it. */
export function memberSetting(ai: AIMemberRow, mine: boolean): SettingState {
  if (ai.cli === 'external') return { shown: false };
  if (ai.follow) return { shown: true, follow: true };
  return { shown: true, follow: false, restore: mine };
}

/** `member set` update fields to restore computer defaults (the host fills the four fields). */
export const restoreDefaults = (): MemberUpdate => ({ follow: true });

/** Builds a `member set` command payload for `ai` in group `g`. */
export function profileSet(g: string, id: string, updates: MemberUpdate) {
  return { t: 'member' as const, g, op: 'set' as const, id, ...updates };
}
