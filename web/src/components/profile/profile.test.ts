import { afterEach, describe, expect, it, vi } from 'vitest';
import { Home } from '../../lib/groups.svelte';
import { goldens } from '../../lib/mock';
import { nodes } from '../../lib/nodes.svelte';
import { AIMemberRow, decode, HostInfoPush } from '../../lib/proto';
import {
  memberSetting,
  newDraft,
  profileChanges,
  profileSet,
  restoreDefaults,
  suggestedTier,
  tiers,
  tooLong,
} from './profile';

const ai = decode(AIMemberRow, goldens['row-member-ai']);
const home = new Home({ user: ai.user, sign: () => new Promise(() => {}) });

afterEach(() => {
  nodes.infos.clear();
  vi.restoreAllMocks();
});

describe('profile helpers', () => {
  it('lists the tiers and creates independent default drafts', () => {
    expect(tiers).toEqual(['light', 'standard', 'strong']);
    const draft = newDraft();
    expect(draft).toEqual({ model: '', effort: '', tier: 'standard', skills: '' });
    draft.skills = 'Protocol design';
    expect(newDraft()).toEqual({ model: '', effort: '', tier: 'standard', skills: '' });
  });

  it('has no suggestion without a host, host info, matching CLI or listed model', () => {
    const get = vi.spyOn(nodes.infos, 'get');
    expect(suggestedTier(undefined, ai)).toBeUndefined();
    expect(get).not.toHaveBeenCalled();
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

  it('derives the defaults setting state for external, following and own members', () => {
    expect(memberSetting({ ...ai, cli: 'external', follow: true }, true)).toEqual({
      shown: false,
    });
    expect(memberSetting({ ...ai, cli: 'external', follow: undefined }, false)).toEqual({
      shown: false,
    });
    expect(memberSetting({ ...ai, cli: 'claude', follow: true }, true)).toEqual({
      shown: true,
      follow: true,
    });
    expect(memberSetting({ ...ai, cli: 'claude', follow: true }, false)).toEqual({
      shown: true,
      follow: true,
    });
    expect(memberSetting({ ...ai, cli: 'claude', follow: undefined }, true)).toEqual({
      shown: true,
      follow: false,
      restore: true,
    });
    expect(memberSetting({ ...ai, cli: 'claude', follow: undefined }, false)).toEqual({
      shown: true,
      follow: false,
      restore: false,
    });
  });

  it('builds member.set payloads for restoring defaults and saving own profile changes', () => {
    expect(restoreDefaults()).toEqual({ follow: true });
    expect(profileSet('group-1', 'ai-1', restoreDefaults())).toEqual({
      t: 'member',
      g: 'group-1',
      op: 'set',
      id: 'ai-1',
      follow: true,
    });
    expect(profileSet('group-1', 'ai-1', { tier: 'light', skills: 'Docs' })).toEqual({
      t: 'member',
      g: 'group-1',
      op: 'set',
      id: 'ai-1',
      tier: 'light',
      skills: 'Docs',
    });
  });
});
