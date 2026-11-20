import { expect, it } from 'vitest';

// Each area of the interface keeps its texts in <area>.en.json and <area>.zh-CN.json.
const files = import.meta.glob<Record<string, string>>('./*.*.json', {
  import: 'default',
  eager: true,
});
const areas = [...new Set(Object.keys(files).map((path) => path.split('.')[1].slice(1)))];

it('finds the areas', () => {
  expect(areas).toEqual(expect.arrayContaining(['chat', 'groups']));
});

it.each(areas)('has every text of %s in both languages, under keys of that area', (area) => {
  const en = files[`./${area}.en.json`];
  const zh = files[`./${area}.zh-CN.json`];
  expect(Object.keys(en)).toEqual(Object.keys(zh));
  expect(Object.keys(en).filter((key) => !key.startsWith(`${area}.`))).toEqual([]);
  expect([...Object.values(en), ...Object.values(zh)].filter((text) => !text.trim())).toEqual([]);
});
