import { describe, expect, it } from 'vitest';
import en from './en.json';
import { langs, merge, messages } from './texts';
import zh from './zh-CN.json';

const shell = {
  en: { 'app.skip': 'Skip', 'nav.x': 'X' },
  xx: { 'app.skip': 'Sauter', 'nav.x': 'X' },
};

describe('merge', () => {
  it('adds the files of each area to the texts of their language', () => {
    const merged = merge(shell, {
      './chat.en.json': { 'chat.send': 'Send', 'chat.back': 'Back' },
      './chat.xx.json': { 'chat.back': 'Retour', 'chat.send': 'Envoyer' },
      'groups.xx.json': { 'groups.new': 'Nouveau' },
      'groups.en.json': { 'groups.new': 'New' },
    });
    expect(merged).toStrictEqual({
      en: {
        'app.skip': 'Skip',
        'nav.x': 'X',
        'chat.send': 'Send',
        'chat.back': 'Back',
        'groups.new': 'New',
      },
      xx: {
        'app.skip': 'Sauter',
        'nav.x': 'X',
        'chat.back': 'Retour',
        'chat.send': 'Envoyer',
        'groups.new': 'Nouveau',
      },
    });
  });

  it('leaves the shell it was given as it was', () => {
    merge(shell, {
      './chat.en.json': { 'chat.send': 'Send' },
      './chat.xx.json': { 'chat.send': 'Envoyer' },
    });
    expect(shell.en).toStrictEqual({ 'app.skip': 'Skip', 'nav.x': 'X' });
  });

  it.each([
    [{ './chat.fr.json': { 'chat.send': 'Envoyer' } }, './chat.fr.json: no language fr'],
    [{ './app.en.json': { 'app.more': 'More' } }, './app.en.json: the shell has the namespace app'],
    [{ './nav.xx.json': { 'nav.y': 'Y' } }, './nav.xx.json: the shell has the namespace nav'],
    [
      { './chat.en.json': { 'chat.send': 'Send', 'chats.back': 'Back' } },
      './chat.en.json: chats.back is not a key of the area chat',
    ],
    [
      { './chat.en.json': { 'chat.send': 'Send', 'app.skip': 'Skip it' } },
      './chat.en.json: app.skip is not a key of the area chat',
    ],
    [{ './chat.en.json': { 'chat.send': ' \n' } }, './chat.en.json: chat.send is empty'],
    [
      { './chat.en.json': { 'chat.send': 'Send' }, './chat.xx.json': { 'chat.sent': 'Envoyé' } },
      './chat.xx.json: the keys differ between languages',
    ],
    [
      {
        './chat.xx.json': { 'chat.send': 'Envoyer', 'chat.back': 'Retour' },
        './chat.en.json': { 'chat.send': 'Send' },
      },
      './chat.xx.json: the keys differ between languages',
    ],
  ])('refuses %j: %s', (files, problem) => {
    expect(() => merge(shell, files)).toThrow(new Error(problem));
  });

  it('tells the keys of an area apart by their borders, not by their letters', () => {
    expect(() =>
      merge(shell, {
        './chat.en.json': { 'chat.a': 'A', 'chat.b': 'B' },
        './chat.xx.json': { 'chat.achat.b': 'AB' },
      }),
    ).toThrow(new Error('./chat.xx.json: the keys differ between languages'));
  });

  it.each([
    [{ './chat.en.json': { 'chat.a': 5 } }, './chat.en.json: chat.a is not text'],
    [{ './chat.en.json': { 'chat.a': null, 'chat.b': '' } }, './chat.en.json: chat.a is not text'],
    [{ './chat.en.json': { 'chat.a': '', 'chat.b': ['b'] } }, './chat.en.json: chat.b is not text'],
  ])('refuses a value that is no text, before an empty one: %j', (files, problem) => {
    expect(() => merge(shell, files)).toThrow(new Error(problem));
  });

  it.each([
    [{ './chat.en.json': { 'chat.send': 'Send' } }, 'chat: no xx'],
    [{ './chat.xx.json': { 'chat.send': 'Envoyer' } }, 'chat: no en'],
    [
      {
        './chat.en.json': { 'chat.send': 'Send' },
        './chat.xx.json': { 'chat.send': 'Envoyer' },
        './groups.en.json': { 'groups.new': 'New' },
      },
      'groups: no xx',
    ],
  ])('refuses an area without a file in every language: %j', (files, problem) => {
    expect(() => merge(shell, files)).toThrow(new Error(problem));
  });

  it('reads an empty text before a missing language as the first problem', () => {
    expect(() => merge(shell, { './chat.fr.json': { 'chat.send': '' } })).toThrow(
      './chat.fr.json: no language fr',
    );
  });
});

describe('the files of the client', () => {
  it('have the shell texts in both languages, none empty', () => {
    expect(Object.keys(en).sort()).toEqual(Object.keys(zh).sort());
    expect([...Object.values(en), ...Object.values(zh)].filter((text) => !text.trim())).toEqual([]);
  });

  it('are merged into both languages with the same keys', () => {
    expect(langs).toEqual(['zh-CN', 'en']);
    expect(Object.keys(messages.en).sort()).toEqual(Object.keys(messages['zh-CN']).sort());
    expect(messages.en['nav.settings']).toBe('Settings');
    expect(messages['zh-CN']['chat.send']).toBe('发送');
  });

  // Each area declares its keys for the type checker in <area>.d.ts.
  const declarations = import.meta.glob<string>('./*.d.ts', {
    query: '?raw',
    import: 'default',
    eager: true,
  });
  const areas = [
    ...new Set(
      Object.keys(import.meta.glob('./*.*.json')).map((path) => path.split('.')[1].slice(1)),
    ),
  ].sort();

  it('find the areas', () => {
    expect(areas).toEqual(
      expect.arrayContaining(['chat', 'groups', 'nodes', 'settings', 'states']),
    );
  });

  it.each(areas)('declare the keys of %s for the type checker', (area) => {
    expect(declarations[`./${area}.d.ts`]).toContain(`${area}: typeof texts;`);
    expect(declarations[`./${area}.d.ts`]).toContain(`from './${area}.en.json';`);
  });
});
