// The one loader of the interface texts. The shell keeps its texts in `<lang>.json`; each area
// of the interface keeps its own in `<area>.<lang>.json`, whose keys all begin with `<area>.`,
// and declares them for the type checker in `<area>.d.ts`. This module finds every file by its
// name, checks the rules of `merge` and merges them into one set of texts per language.
import en from './en.json';
import zh from './zh-CN.json';

type Texts = Record<string, string>;

/** The shell's texts, by language. The languages of the client are the languages it has. */
const shell = { 'zh-CN': zh, en };
export type Lang = keyof typeof shell;

/** The keys of each area, which its `<area>.d.ts` adds:
 *  `declare module './texts' { interface Areas { <area>: typeof <its English file> } }`. */
export interface Areas {}

type AreaKey = { [A in keyof Areas]: keyof Areas[A] & string }[keyof Areas];
/** A text that both languages have: one of the shell, or one of an area. */
export type Key = (keyof typeof en & keyof typeof zh) | AreaKey;

/** The first part of a key, up to its first dot: the area, or a namespace of the shell. */
const head = (key: string) => key.split('.')[0];

/** The sorted keys of a file, as one text. */
const keysOf = (texts: Texts) => Object.keys(texts).sort().join(' ');

/** Merges the shell's texts with the files of the areas, by file path (`./<area>.<lang>.json`).
 *  It throws, naming the file, when a file breaks a rule, so that no text can replace another:
 *  an area is named after no namespace of the shell, every key of an area begins with the
 *  area's name, both languages of an area have the same keys, and no text is empty. A key of
 *  one file can then equal no key of another. */
export function merge<L extends string>(shell: Record<L, Texts>, files: Record<string, Texts>) {
  const langs = Object.keys(shell);
  const reserved = new Set(
    Object.values<Texts>(shell).flatMap((texts) => Object.keys(texts).map(head)),
  );
  const merged = Object.fromEntries(langs.map((lang) => [lang, { ...shell[lang as L] }])) as Record<
    L,
    Texts
  >;
  const known = new Map<string, string>();
  for (const [path, texts] of Object.entries(files).sort()) {
    const [area, lang] = path.split('/').pop()!.split('.');
    const problem = rule(area, lang, texts, langs, reserved, known.get(area));
    if (problem) throw new Error(`${path}: ${problem}`);
    known.set(area, keysOf(texts));
    Object.assign(merged[lang as L], texts);
  }
  return merged;
}

/** What is wrong with one file of an area, if anything. `known` is the keys of the area's file
 *  in another language, when one was read before. */
function rule(
  area: string,
  lang: string,
  texts: Texts,
  langs: string[],
  reserved: Set<string>,
  known: string | undefined,
) {
  const keys = Object.keys(texts);
  const stray = keys.find((key) => head(key) !== area);
  const empty = keys.find((key) => !texts[key].trim());
  if (!langs.includes(lang)) return `no language ${lang}`;
  if (reserved.has(area)) return `the shell has the namespace ${area}`;
  if (stray !== undefined) return `${stray} is not a key of the area ${area}`;
  if (empty !== undefined) return `${empty} is empty`;
  if (known !== undefined && known !== keysOf(texts)) return 'the keys differ between languages';
  return undefined;
}

/** Every file of an area: two dots in its name, as `groups.en.json`. */
const files = import.meta.glob<Texts>('./*.*.json', { import: 'default', eager: true });

/** The texts of each language. */
export const messages = merge(shell, files) as Record<Lang, Record<Key, string>>;
export const langs = Object.keys(shell) as Lang[];
