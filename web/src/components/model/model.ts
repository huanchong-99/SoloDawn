import type { Home } from '../../lib/groups.svelte';
import type { Draft } from '../../lib/members';
import { nodes } from '../../lib/nodes.svelte';
import type { AIMemberRow, HostInfoPush } from '../../lib/proto';

export type Listed = HostInfoPush['agents'][number]['models'][number];

/** The models `host` lists for `cli`, or `undefined` while its `host.info` is not known. */
export function listedModels(host: Home | undefined, cli: string): Listed[] | undefined {
  return host && nodes.infos.get(host)?.agents.find((agent) => agent.agent === cli)?.models;
}

/** The model and level of `draft` that differ from `ai`. */
export function modelChanges(
  ai: AIMemberRow,
  draft: Draft,
): Partial<Pick<Draft, 'model' | 'effort'>> {
  return {
    ...(ai.model !== draft.model ? { model: draft.model } : {}),
    ...(ai.effort !== draft.effort ? { effort: draft.effort } : {}),
  };
}
