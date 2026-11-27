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
