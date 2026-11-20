// Browser preferences (theme, language) and the lookup of interface texts.
import { MediaQuery } from 'svelte/reactivity';
import en from '../messages/en.json';
import zh from '../messages/zh-CN.json';

export type Key = keyof typeof zh;
export type Theme = 'auto' | 'light' | 'dark';

const messages = { 'zh-CN': zh, en } satisfies Record<string, Record<Key, string>>;
export type Lang = keyof typeof messages;

export const prefs = $state({
  theme: (localStorage.getItem('sd-theme') ?? 'auto') as Theme,
  lang: (localStorage.getItem('sd-lang') ??
    (navigator.language.startsWith('zh') ? 'zh-CN' : 'en')) as Lang,
});

// <html> and this browser's storage follow the preferences. tokens.css reads data-theme.
$effect.root(() => {
  $effect(() => {
    document.documentElement.dataset.theme = prefs.theme;
    document.documentElement.lang = prefs.lang;
    localStorage.setItem('sd-theme', prefs.theme);
    localStorage.setItem('sd-lang', prefs.lang);
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
