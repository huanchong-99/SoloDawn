import { expect, it } from 'vitest';
import en from './en.json';
import zh from './zh-CN.json';

it('has every text in both languages', () => {
  expect(Object.keys(en)).toEqual(Object.keys(zh));
  expect([...Object.values(en), ...Object.values(zh)].filter((text) => !text.trim())).toEqual([]);
});
