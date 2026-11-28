import { afterEach, describe, expect, it } from 'vitest';
import { Home } from '../../lib/groups.svelte';
import { goldens } from '../../lib/mock';
import { nodes } from '../../lib/nodes.svelte';
import { AIMemberRow, decode, HostInfoPush } from '../../lib/proto';
import { listedModels, modelChanges } from './model';

const ai = decode(AIMemberRow, goldens['row-member-ai']);
const home = new Home({ user: ai.user, sign: () => new Promise(() => {}) });

afterEach(() => nodes.infos.clear());

describe('model helpers', () => {
  it('distinguishes unavailable host information, missing CLI and an empty list', () => {
    expect(listedModels(undefined, 'claude')).toBeUndefined();
    expect(listedModels(home, 'claude')).toBeUndefined();
    const info = decode(HostInfoPush, goldens['host-info-push']);
    nodes.infos.set(home, {
      ...info,
      agents: [{ ...info.agents[0], agent: 'claude', models: [] }],
    });
    expect(listedModels(home, 'other')).toBeUndefined();
    expect(listedModels(home, 'claude')).toEqual([]);
  });

  it('returns the selected CLI models in their listed order with exact metadata', () => {
    const info = decode(HostInfoPush, goldens['host-info-push']);
    const models = [
      {
        model: 'opus',
        name: 'Opus',
        efforts: ['low', 'high'],
        fast: true as const,
        tier: 'strong',
      },
      { model: 'haiku', name: 'Haiku', efforts: [], tier: 'light' },
    ];
    nodes.infos.set(home, {
      ...info,
      agents: [
        { ...info.agents[0], agent: 'other', models: [] },
        { ...info.agents[0], agent: 'claude', models },
      ],
    });
    expect(listedModels(home, 'claude')).toEqual(models);
  });

  it('sends only changed model and effort including clearing either field', () => {
    const member = { ...ai, model: 'opus', effort: 'high' };
    expect(modelChanges(member, member)).toEqual({});
    expect(modelChanges(member, { ...member, model: '' })).toEqual({ model: '' });
    expect(modelChanges(member, { ...member, effort: '' })).toEqual({ effort: '' });
    expect(modelChanges(member, { ...member, model: 'haiku', effort: 'low' })).toEqual({
      model: 'haiku',
      effort: 'low',
    });
    expect(modelChanges(member, { ...member, tier: 'light', skills: 'Documentation' })).toEqual({});
  });

  it('keeps a listed default model entry with its metadata', () => {
    const info = decode(HostInfoPush, goldens['host-info-push']);
    const models = [
      {
        model: 'default',
        name: 'Default (recommended)',
        efforts: ['low', 'medium', 'high'],
        fast: true as const,
        tier: 'strong',
      },
      { model: 'opus', name: 'Opus', efforts: ['low', 'high'], tier: 'strong' },
    ];
    nodes.infos.set(home, {
      ...info,
      agents: [{ ...info.agents[0], agent: 'claude', models }],
    });
    expect(listedModels(home, 'claude')).toEqual(models);
  });

  it('keeps stored empty model unchanged and sends only effort when selecting an effort on default', () => {
    const member = { ...ai, model: '', effort: '' };
    expect(modelChanges(member, { ...member, model: '', effort: '' })).toEqual({});
    const withEffort = modelChanges(member, { ...member, model: '', effort: 'high' });
    expect(withEffort).toEqual({ effort: 'high' });
    expect('model' in withEffort).toBe(false);
    expect(
      modelChanges({ ...ai, model: '', effort: 'high' }, { ...ai, model: '', effort: '' }),
    ).toEqual({ effort: '' });
    expect(
      modelChanges({ ...ai, model: 'opus', effort: 'high' }, { ...ai, model: '', effort: 'high' }),
    ).toEqual({ model: '' });
  });
});
