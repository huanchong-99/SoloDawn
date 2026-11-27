import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import { Home, homes } from '../../lib/groups.svelte';
import { goldens } from '../../lib/mock';
import { nodes } from '../../lib/nodes.svelte';
import { prefs } from '../../lib/prefs.svelte';
import {
  AIMemberRow,
  decode,
  HostInfoPush,
  HumanMemberRow,
  Welcome,
  type Profile,
} from '../../lib/proto';
import type { Listed } from '../model/model';
import {
  defaultsOf,
  draftOf,
  followers,
  sameProfile,
  summaryParts,
  unsetProfile,
} from './defaults';

const sign = () => new Promise<never>(() => {});
const user = 'akWV_5PtNLIGhakuMAvpD9LcG1D8TUfc1URh48VREcs';
const device = 'J3Uuh-y1z2AX536yAFlGHJL5sjxVpWVJavvVphvdm5Q';
const ai = decode(AIMemberRow, goldens['row-member-ai']);
const info = decode(HostInfoPush, goldens['host-info-push']);

const models: Listed[] = [
  { model: 'haiku', name: 'Haiku', efforts: [], tier: 'light' },
  { model: 'opus', name: 'Opus 4.5', efforts: ['low', 'high'], tier: 'strong' },
];

beforeEach(() => {
  prefs.lang = 'en';
});

afterEach(() => {
  nodes.infos.clear();
  homes.length = 0;
  prefs.lang = 'en';
});

describe('unsetProfile', () => {
  it('holds the unset values as a frozen record', () => {
    expect(unsetProfile).toStrictEqual({ model: '', effort: '', tier: 'standard', skills: '' });
    expect(Object.isFrozen(unsetProfile)).toBe(true);
  });
});

describe('summaryParts', () => {
  it('resolves listed model name, effort, tier label and skills while omitting empty skills', () => {
    const full: Profile = {
      model: 'opus',
      effort: 'high',
      tier: 'strong',
      skills: 'Go backend',
    };
    expect(summaryParts(full, models)).toStrictEqual({
      model: 'Opus 4.5',
      effort: 'high',
      tier: 'strong',
      tierLabel: 'More',
      skills: 'Go backend',
    });

    expect(summaryParts({ ...unsetProfile }, models)).toStrictEqual({
      model: "The CLI's default",
      effort: "The CLI's default",
      tier: 'standard',
      tierLabel: 'Normal',
    });
  });

  it('uses model id when unlisted and ignores an empty-id entry in models', () => {
    const withEmptyId: Listed[] = [
      { model: '', name: 'ShouldNotMatchEmpty', efforts: [], tier: 'light' },
      ...models,
    ];
    expect(summaryParts({ ...unsetProfile }, withEmptyId)).toStrictEqual({
      model: "The CLI's default",
      effort: "The CLI's default",
      tier: 'standard',
      tierLabel: 'Normal',
    });
    expect(
      summaryParts({ model: 'custom-model', effort: 'medium', tier: 'light', skills: '' }, models),
    ).toStrictEqual({
      model: 'custom-model',
      effort: 'medium',
      tier: 'light',
      tierLabel: 'Less',
    });
  });
});

describe('defaultsOf', () => {
  it('returns the matching agent defaults when present and undefined otherwise', () => {
    const home = new Home({ user, sign });
    expect(defaultsOf(home, 'claude')).toBeUndefined();

    const custom: Profile = {
      model: 'opus',
      effort: 'high',
      tier: 'strong',
      skills: 'Go backend',
    };
    nodes.infos.set(home, {
      ...info,
      agents: [
        { ...info.agents[0], agent: 'codex', defaults: undefined },
        { ...info.agents[0], agent: 'claude', defaults: custom },
      ],
    });
    expect(defaultsOf(home, 'claude')).toStrictEqual(custom);
    expect(defaultsOf(home, 'codex')).toBeUndefined();
    expect(defaultsOf(home, 'external')).toBeUndefined();
  });
});

describe('followers', () => {
  it('counts live AI members across readable homes that follow the device and CLI', () => {
    expect(followers(device, 'claude')).toBe(0);

    const first = new Home({ user, sign });
    const second = new Home({ user, sign });
    first.groups = [];
    const g1 = first.connect();
    const g2 = second.connect();
    g1.online({ request: () => new Promise(() => {}) } as never, {
      ...decode(Welcome, goldens.welcome),
      device,
      owner: true,
      groups: [
        { g: 'g-1', name: 'Website', seq: 0 },
        { g: 'g-2', name: 'Docs', seq: 0 },
      ],
    });
    g2.online({ request: () => new Promise(() => {}) } as never, {
      ...decode(Welcome, goldens.welcome),
      device: 'other-home',
      name: 'other',
      owner: false,
      groups: [{ g: 'g-3', name: 'App', seq: 0 }],
    });

    const human = decode(HumanMemberRow, goldens['row-member-human']);
    const base = { ...ai, host: device, cli: 'claude', follow: true as const };
    first.groups[0].apply([
      { ...base, id: 'm1', g: 'g-1', name: 'Claude 1' },
      { ...base, id: 'm2', g: 'g-1', name: 'Claude Own', follow: undefined },
      { ...base, id: 'm3', g: 'g-1', name: 'Codex', cli: 'codex' },
      { ...base, id: 'm4', g: 'g-1', name: 'Other Host', host: 'other-device' },
      { ...base, id: 'm5', g: 'g-1', name: 'Gone AI' },
      { r: 'member', id: 'm5', seq: 999999, gone: true },
      { ...human, g: 'g-1' },
    ]);
    first.groups[1].apply([{ ...base, id: 'm6', g: 'g-2', name: 'Claude 2', follow: undefined }]);
    second.groups[0].apply([{ ...base, id: 'm7', g: 'g-3', name: 'Claude 3' }]);
    homes.push(first, second);

    expect(followers(device, 'claude')).toBe(2);
    expect(followers(device, 'codex')).toBe(1);
    expect(followers('missing', 'claude')).toBe(0);
  });
});

describe('draftOf and sameProfile', () => {
  it('creates independent mutable drafts from defaults or unsetProfile', () => {
    const empty = draftOf(undefined);
    expect(empty).toStrictEqual(unsetProfile);
    expect(empty).not.toBe(unsetProfile);
    empty.skills = 'Edited';
    expect(unsetProfile.skills).toBe('');

    const source: Profile = { model: 'opus', effort: 'high', tier: 'strong', skills: 'Go' };
    const copy = draftOf(source);
    expect(copy).toStrictEqual(source);
    expect(copy).not.toBe(source);
    copy.model = 'haiku';
    expect(source.model).toBe('opus');
  });

  it('compares all four profile fields', () => {
    const a: Profile = { model: 'opus', effort: 'high', tier: 'strong', skills: 'Go' };
    expect(sameProfile(a, { ...a })).toBe(true);
    expect(sameProfile(a, { ...a, model: 'haiku' })).toBe(false);
    expect(sameProfile(a, { ...a, effort: 'low' })).toBe(false);
    expect(sameProfile(a, { ...a, tier: 'standard' })).toBe(false);
    expect(sameProfile(a, { ...a, skills: '' })).toBe(false);
  });
});
