import { afterEach, describe, expect, it } from 'vitest';
import { Home } from '../../lib/groups.svelte';
import { goldens } from '../../lib/mock';
import { nodes } from '../../lib/nodes.svelte';
import { AIMemberRow, decode, HostInfoPush } from '../../lib/proto';
import { newDraft, profileChanges, suggestedTier, tiers, tooLong } from './profile';

const ai = decode(AIMemberRow, goldens['row-member-ai']);
const home = new Home({ user: ai.user, sign: () => new Promise(() => {}) });

afterEach(() => nodes.infos.clear());

describe('profile helpers', () => {
  it('lists the tiers and creates independent default drafts', () => {
    expect(tiers).toEqual(['light', 'standard', 'strong']);
    const draft = newDraft();
    expect(draft).toEqual({ model: '', effort: '', tier: 'standard', skills: '' });
    draft.skills = 'Protocol design';
    expect(newDraft()).toEqual({ model: '', effort: '', tier: 'standard', skills: '' });
  });

  it('has no suggestion without a host, host info, matching CLI or listed model', () => {
    expect(suggestedTier(undefined, ai)).toBeUndefined();
    expect(suggestedTier(home, ai)).toBeUndefined();
    const info = decode(HostInfoPush, goldens['host-info-push']);
    nodes.infos.set(home, { ...info, agents: [] });
    expect(suggestedTier(home, ai)).toBeUndefined();
    nodes.infos.set(home, {
      ...info,
      agents: [{ ...info.agents[0], agent: ai.cli, models: [] }],
    });
    expect(suggestedTier(home, ai)).toBeUndefined();
  });

  it('limits skills to 200 Unicode code points rather than UTF-16 units', () => {
    const draft = newDraft();
    expect(tooLong(draft)).toBe(false);
    expect(tooLong({ ...draft, skills: 'a'.repeat(200) })).toBe(false);
    expect(tooLong({ ...draft, skills: 'a'.repeat(201) })).toBe(true);
    expect(tooLong({ ...draft, skills: '😀'.repeat(200) })).toBe(false);
    expect(tooLong({ ...draft, skills: '😀'.repeat(201) })).toBe(true);
    expect(tooLong({ ...draft, skills: '😀'.repeat(100) + 'a'.repeat(100) })).toBe(false);
    expect(tooLong({ ...draft, skills: '😀'.repeat(100) + 'a'.repeat(101) })).toBe(true);
  });

  it('matches the CLI and model rather than taking the first listed suggestion', () => {
    const info = decode(HostInfoPush, goldens['host-info-push']);
    const model = { model: ai.model, name: 'Selected model', efforts: [], tier: 'strong' };
    nodes.infos.set(home, {
      ...info,
      agents: [
        { ...info.agents[0], agent: 'other-cli', models: [{ ...model, tier: 'light' }] },
        {
          ...info.agents[0],
          agent: ai.cli,
          models: [{ ...model, model: 'other-model', tier: 'standard' }, model],
        },
      ],
    });
    expect(suggestedTier(home, ai)).toBe('strong');
    expect(suggestedTier(home, { ...ai, model: 'not-listed' })).toBeUndefined();
  });

  it('returns only changed tier and skills, including clearing skills', () => {
    const member = { ...ai, tier: 'standard', skills: 'Protocol design' };
    expect(profileChanges(member, { ...member })).toEqual({});
    expect(profileChanges(member, { ...member, tier: 'strong' })).toEqual({ tier: 'strong' });
    expect(profileChanges(member, { ...member, skills: '' })).toEqual({ skills: '' });
    expect(profileChanges(member, { ...member, tier: 'light', skills: 'Documentation' })).toEqual({
      tier: 'light',
      skills: 'Documentation',
    });
    expect(profileChanges(member, { ...member, model: 'different', effort: 'high' })).toEqual({});
  });
});
