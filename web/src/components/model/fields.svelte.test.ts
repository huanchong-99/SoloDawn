import { flushSync } from 'svelte';
import { afterEach, beforeEach, expect, it } from 'vitest';
import { all, choose, closePage, one, show, texts } from '../../../features/page';
import { goldens } from '../../lib/mock';
import { prefs } from '../../lib/prefs.svelte';
import { AIMemberRow, decode } from '../../lib/proto';
import ProfileFields from '../profile/ProfileFields.svelte';
import { newDraft } from '../profile/profile';
import ModelFields from './ModelFields.svelte';

const models = [
  { model: 'opus', name: 'Opus', efforts: ['low', 'high'], tier: 'strong' },
  { model: 'haiku', name: 'Haiku', efforts: [], tier: 'light' },
];
const ai = decode(AIMemberRow, goldens['row-member-ai']);
const modelSelect = () => all<HTMLSelectElement>('select')[0];
const modelOptions = () =>
  [...modelSelect().options].map((option) => [option.value, option.text.trim()]);
const level = () => all<HTMLSelectElement>('select')[1];
const levels = () => [...level().options].map((option) => [option.value, option.text.trim()]);

beforeEach(() => {
  prefs.lang = 'en';
});
afterEach(closePage);

it('offers the level of the chosen model, refreshed when the model changes', () => {
  const draft = $state(newDraft());
  show(ModelFields, { draft, models, suggest: true });
  expect(levels()).toEqual([['', "The CLI's default"]]);
  expect(level().disabled).toBe(true);
  expect(texts('.why')).toEqual(['Choose a model to see its levels']);
  expect(level().getAttribute('aria-describedby')).toBe(one('.why').id);
  choose(all<HTMLSelectElement>('select')[0], 'Opus');
  expect(levels()).toEqual([
    ['', "The CLI's default"],
    ['low', 'low'],
    ['high', 'high'],
  ]);
  expect(level().disabled).toBe(false);
  expect(all('.why')).toEqual([]);
  expect(level().hasAttribute('aria-describedby')).toBe(false);
  choose(level(), 'high');
  expect(draft).toMatchObject({ model: 'opus', effort: 'high', tier: 'strong' });
  choose(all<HTMLSelectElement>('select')[0], 'Haiku');
  expect(draft).toMatchObject({ model: 'haiku', effort: '', tier: 'light' });
  expect(levels()).toEqual([['', "The CLI's default"]]);
  expect(level().disabled).toBe(true);
  expect(texts('.why')).toEqual(['This model has no levels']);
});

it('keeps a stored level that the list does not hold as a choice', () => {
  const draft = $state({ ...newDraft(), model: 'gone', effort: 'deep' });
  show(ModelFields, { draft, models });
  expect(levels()).toEqual([
    ['', "The CLI's default"],
    ['deep', 'deep · Not listed'],
  ]);
  expect(level().value).toBe('deep');
  expect(level().disabled).toBe(false);
  expect(all('.why')).toEqual([]);
  draft.model = 'opus';
  flushSync();
  expect(levels()).toEqual([
    ['', "The CLI's default"],
    ['low', 'low'],
    ['high', 'high'],
    ['deep', 'deep · Not listed'],
  ]);
});

it('merges a listed default model into the CLI default option and restores its levels, badge, and tier', () => {
  const listed = [
    {
      model: 'default',
      name: 'Default (recommended)',
      efforts: ['low', 'medium', 'high'],
      fast: true as const,
      tier: 'strong',
    },
    ...models,
  ];
  const draft = $state(newDraft());
  show(ModelFields, { draft, models: listed, suggest: true });
  expect(modelOptions()).toEqual([
    ['', "The CLI's default"],
    ['opus', 'Opus'],
    ['haiku', 'Haiku'],
  ]);
  expect(modelSelect().querySelector('option[value="default"]')).toBeNull();
  expect(draft.model).toBe('');
  expect(modelSelect().value).toBe('');
  expect(level().disabled).toBe(false);
  expect(levels()).toEqual([
    ['', "The CLI's default"],
    ['low', 'low'],
    ['medium', 'medium'],
    ['high', 'high'],
  ]);
  expect(texts('.fast')).toEqual(['Fast mode']);
  expect(all('.why')).toEqual([]);

  choose(level(), 'medium');
  expect(draft).toMatchObject({ model: '', effort: 'medium' });

  choose(modelSelect(), 'Opus');
  expect(draft).toMatchObject({ model: 'opus', effort: '', tier: 'strong' });
  expect(all('.fast')).toEqual([]);
  expect(levels()).toEqual([
    ['', "The CLI's default"],
    ['low', 'low'],
    ['high', 'high'],
  ]);

  choose(level(), 'high');
  choose(modelSelect(), "The CLI's default");
  expect(draft).toMatchObject({ model: '', effort: 'high', tier: 'strong' });
  expect(modelSelect().value).toBe('');
  expect(level().disabled).toBe(false);
  expect(levels()).toEqual([
    ['', "The CLI's default"],
    ['low', 'low'],
    ['medium', 'medium'],
    ['high', 'high'],
  ]);
  expect(texts('.fast')).toEqual(['Fast mode']);

  choose(modelSelect(), 'Haiku');
  expect(draft).toMatchObject({ model: 'haiku', effort: '', tier: 'light' });
  expect(level().disabled).toBe(true);

  choose(modelSelect(), "The CLI's default");
  expect(draft).toMatchObject({ model: '', effort: '', tier: 'strong' });
  expect(level().disabled).toBe(false);
  expect(levels()).toEqual([
    ['', "The CLI's default"],
    ['low', 'low'],
    ['medium', 'medium'],
    ['high', 'high'],
  ]);
  expect(texts('.fast')).toEqual(['Fast mode']);

  prefs.lang = 'zh-CN';
  flushSync();
  expect(modelOptions()).toEqual([
    ['', 'CLI 默认'],
    ['opus', 'Opus'],
    ['haiku', 'Haiku'],
  ]);
});

it('shows no-levels reason when a listed default model has empty efforts', () => {
  const listed = [
    { model: 'default', name: 'Default (recommended)', efforts: [], tier: 'standard' },
    ...models,
  ];
  const draft = $state(newDraft());
  show(ModelFields, { draft, models: listed });
  expect(modelOptions()).toEqual([
    ['', "The CLI's default"],
    ['opus', 'Opus'],
    ['haiku', 'Haiku'],
  ]);
  expect(draft.model).toBe('');
  expect(level().disabled).toBe(true);
  expect(levels()).toEqual([['', "The CLI's default"]]);
  expect(texts('.why')).toEqual(['This model has no levels']);
  expect(all('.fast')).toEqual([]);
});

it('keeps a listed default model visible and selected for a member already saved with "default"', () => {
  const listed = [
    {
      model: 'default',
      name: 'Default (recommended)',
      efforts: ['low', 'medium', 'high'],
      fast: true as const,
      tier: 'strong',
    },
    ...models,
  ];
  const draft = $state({ ...newDraft(), model: 'default', effort: 'high' });
  show(ModelFields, { draft, models: listed, suggest: true });
  expect(modelOptions()).toEqual([
    ['', "The CLI's default"],
    ['default', 'Default (recommended)'],
    ['opus', 'Opus'],
    ['haiku', 'Haiku'],
  ]);
  expect(modelSelect().value).toBe('default');
  expect(modelSelect().selectedIndex).toBe(1);
  expect(level().value).toBe('high');
  expect(texts('.fast')).toEqual(['Fast mode']);

  choose(modelSelect(), "The CLI's default");
  expect(draft.model).toBe('');
  expect(modelSelect().value).toBe('');
  expect(level().value).toBe('high');
  expect(modelOptions()).toEqual([
    ['', "The CLI's default"],
    ['opus', 'Opus'],
    ['haiku', 'Haiku'],
  ]);
  expect(modelSelect().querySelector('option[value="default"]')).toBeNull();
});

it('words the work share with its hint, shows the quota with an AI and can leave out skills', () => {
  const draft = $state(newDraft());
  show(ProfileFields, { draft, ai: { ...ai, usage: {} }, skills: false });
  expect(texts('legend')).toEqual(['Work share']);
  expect(texts('.seg label')).toEqual(['Less', 'Normal', 'More']);
  expect(one('fieldset').getAttribute('aria-describedby')).toBe(one('.tier-hint').id);
  expect(texts('.quota-none')).toEqual(['No quota reported yet']);
  expect(all('input:not([type=radio])')).toEqual([]);
  prefs.lang = 'zh-CN';
  flushSync();
  expect(texts('legend')).toEqual(['派活']);
  expect(texts('.seg label')).toEqual(['少', '适中', '多']);
  expect(texts('.quota-none')).toEqual(['还没有额度数据']);
});

it('shows the window used in the work block, and no quota without an AI', async () => {
  const draft = $state(newDraft());
  show(ProfileFields, { draft, ai: { ...ai, usage: { window_pct: 40 } } });
  expect(texts('fieldset [role=meter]')).toEqual(['Quota used this window40%']);
  expect(all('input.input')).toHaveLength(1);
  await closePage();
  show(ProfileFields, { draft });
  expect(all('[role=meter], .quota-none')).toEqual([]);
});

it('offers no work share for a secretary, keeps the draft value, and offers it again after', async () => {
  const draft = $state({ ...newDraft(), tier: 'light', skills: 'Coordination' });
  const props = $state({ draft, ai: { ...ai, usage: {} }, share: false });
  show(ProfileFields, props);
  expect(all('fieldset, .tiers, input[type=radio], .quota-none')).toEqual([]);
  expect(one<HTMLInputElement>('input.input').value).toBe('Coordination');
  props.share = true;
  flushSync();
  expect(texts('legend')).toEqual(['Work share']);
  expect(one<HTMLInputElement>('.tiers input:checked').value).toBe('light');
  expect(draft.tier).toBe('light');
});
