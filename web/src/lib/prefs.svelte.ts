// Browser preferences (theme, language) and the lookup of interface texts.
import { MediaQuery } from 'svelte/reactivity';
import en from '../messages/en.json';
import zh from '../messages/zh-CN.json';

export type Key = keyof typeof zh;

const messages = { 'zh-CN': zh, en } satisfies Record<string, Record<Key, string>>;
export type Lang = keyof typeof messages;
export const langs = Object.keys(messages) as Lang[];

export const themes = ['auto', 'light', 'dark'] as const;
export type Theme = (typeof themes)[number];

// This browser's storage is a boundary: a browser that blocks site data throws on any access to
// it, and a value may come from another version of the client. Either way the default applies.
function stored<T extends string>(key: string, allowed: readonly T[], fallback: T): T {
  try {
    const value = localStorage.getItem(key);
    return allowed.find((a) => a === value) ?? fallback;
  } catch {
    return fallback;
  }
}

function store(key: string, value: string) {
  try {
    localStorage.setItem(key, value);
  } catch {
    // Not remembered: the choice holds for this visit only.
  }
}

export const prefs = $state({
  theme: stored('sd-theme', themes, themes[0]),
  lang: stored('sd-lang', langs, navigator.language.startsWith('zh') ? 'zh-CN' : 'en'),
});

// <html> and this browser's storage follow the preferences. tokens.css reads data-theme.
$effect.root(() => {
  $effect(() => {
    document.documentElement.dataset.theme = prefs.theme;
    document.documentElement.lang = prefs.lang;
    store('sd-theme', prefs.theme);
    store('sd-lang', prefs.lang);
  });
});

const systemDark = new MediaQuery('prefers-color-scheme: dark');

export const isDark = () => (prefs.theme === 'auto' ? systemDark.current : prefs.theme === 'dark');

export function toggleTheme() {
  prefs.theme = isDark() ? 'light' : 'dark';
}

export function toggleLang() {
  prefs.lang = prefs.lang === 'en' ? 'zh-CN' : 'en';
}

export const t = (key: Key) => messages[prefs.lang][key];
