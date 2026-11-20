// Browser preferences (theme, language) and the lookup of interface texts.
import { MediaQuery } from 'svelte/reactivity';
import chatEn from '../messages/chat.en.json';
import chatZh from '../messages/chat.zh-CN.json';
import en from '../messages/en.json';
import groupsEn from '../messages/groups.en.json';
import groupsZh from '../messages/groups.zh-CN.json';
import zh from '../messages/zh-CN.json';
import { shift } from './motion';

// The shell's texts, then those of each area of the interface, which has its own two files.
const messages = {
  'zh-CN': { ...zh, ...groupsZh, ...chatZh },
  en: { ...en, ...groupsEn, ...chatEn },
};
export type Key = keyof (typeof messages)['zh-CN'] & keyof (typeof messages)['en'];
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

// Until the person changes a setting nothing is stored, so the client follows the browser's
// language and scheme: a default is not a choice.
export const prefs = $state({
  theme: stored('sd-theme', themes, themes[0]),
  lang: stored('sd-lang', langs, navigator.language.startsWith('zh') ? 'zh-CN' : 'en'),
});

/** Sets a preference as the person chose it, and remembers it in this browser. */
function set<K extends keyof typeof prefs>(key: K, value: (typeof prefs)[K]) {
  prefs[key] = value;
  store(`sd-${key}`, value);
}

const systemDark = new MediaQuery('prefers-color-scheme: dark');

export const isDark = () => (prefs.theme === 'auto' ? systemDark.current : prefs.theme === 'dark');

// The browser's own bar: index.html gives a colour for each system scheme, which applies before
// the script runs. From then on both tags take the colour of the theme shown, chosen or not.
const bars = () => document.querySelectorAll<HTMLMetaElement>('meta[name=theme-color]');
const barColour = new Map(
  [...bars()].map((meta) => [meta.matches('[media*=dark]'), meta.getAttribute('content')]),
);

// <html> follows the preferences; tokens.css reads data-theme.
$effect.root(() => {
  $effect(() => {
    document.documentElement.dataset.theme = prefs.theme;
    document.documentElement.lang = prefs.lang;
  });
  $effect(() => {
    const colour = barColour.get(isDark())!;
    for (const meta of bars()) meta.setAttribute('content', colour);
  });
});

/** Changes a preference; the page cross-fades to its new look. */
export function choose<K extends keyof typeof prefs>(key: K, value: (typeof prefs)[K]) {
  shift(() => {
    set(key, value);
    announcement.text = '';
  });
}

// Radios expose their checked state natively. The two action buttons announce their result.
export const announcement = $state({ text: '' });

/** The language the interface does not show, which the language button offers. */
export const otherLang = () => langs.find((lang) => lang !== prefs.lang)!;

export const toggleTheme = () =>
  shift(() => {
    set('theme', isDark() ? 'light' : 'dark');
    announcement.text = `${t('settings.theme')}: ${t(`settings.theme.${prefs.theme}`)}`;
  });

export const toggleLang = () =>
  shift(() => {
    set('lang', otherLang());
    announcement.text = `${t('settings.lang')}: ${t(`settings.lang.${prefs.lang}`)}`;
  });

/** An interface text in a given language, for a text that names both languages. */
export const textIn = (lang: Lang, key: Key) => messages[lang][key];

export const t = (key: Key) => textIn(prefs.lang, key);
