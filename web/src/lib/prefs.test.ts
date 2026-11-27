import { readFileSync } from 'node:fs';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

let systemDark: boolean;

/** A page load: fresh modules, with Svelte's runtime, so that flushSync runs this module's effect. */
async function load() {
  vi.resetModules();
  const { flushSync } = await import('svelte');
  const prefs = await import('./prefs.svelte');
  flushSync();
  return { ...prefs, flushSync };
}

beforeEach(() => {
  localStorage.clear();
  systemDark = false;
  vi.spyOn(navigator, 'language', 'get').mockReturnValue('en-US');
  vi.stubGlobal('matchMedia', (query: string) => ({
    get matches() {
      return query === '(prefers-color-scheme: dark)' && systemDark;
    },
    addEventListener() {},
    removeEventListener() {},
  }));
});

describe('first visit', () => {
  it('follows the system theme and the browser language', async () => {
    const { prefs } = await load();
    expect(prefs).toEqual({ theme: 'auto', accent: 'daybreak', lang: 'en' });
  });

  it('opens in Chinese for any Chinese locale', async () => {
    vi.spyOn(navigator, 'language', 'get').mockReturnValue('zh-TW');
    const { prefs } = await load();
    expect(prefs.lang).toBe('zh-CN');
  });
});

describe('remembered choices', () => {
  it('reads them from this browser', async () => {
    localStorage.setItem('sd-theme', 'dark');
    localStorage.setItem('sd-accent', 'nightfall');
    localStorage.setItem('sd-lang', 'zh-CN');
    const { prefs } = await load();
    expect(prefs).toEqual({ theme: 'dark', accent: 'nightfall', lang: 'zh-CN' });
  });

  it.each([['auto'], ['light'], ['dark']])('reads the theme %s', async (theme) => {
    localStorage.setItem('sd-theme', theme);
    const { prefs } = await load();
    expect(prefs.theme).toBe(theme);
  });

  it('reads English in a Chinese browser', async () => {
    vi.spyOn(navigator, 'language', 'get').mockReturnValue('zh-CN');
    localStorage.setItem('sd-lang', 'en');
    const { prefs } = await load();
    expect(prefs.lang).toBe('en');
  });

  it('ignores values it does not know, such as those of another version', async () => {
    localStorage.setItem('sd-theme', 'sepia');
    localStorage.setItem('sd-accent', 'cyan');
    localStorage.setItem('sd-lang', 'fr');
    const { prefs } = await load();
    expect(prefs).toEqual({ theme: 'auto', accent: 'daybreak', lang: 'en' });
  });

  it('writes nothing to this browser until the person changes a setting', async () => {
    const { flushSync } = await load();
    flushSync();
    expect(localStorage.length).toBe(0);
  });

  it('writes a change to <html> and to this browser', async () => {
    const { choose, flushSync, toggleLang, toggleTheme } = await load();
    expect(document.documentElement.dataset.theme).toBe('auto');
    expect(document.documentElement.dataset.accent).toBe('daybreak');
    expect(document.documentElement.lang).toBe('en');

    choose('theme', 'light');
    choose('accent', 'firstlight');
    choose('lang', 'zh-CN');
    flushSync();

    expect(document.documentElement.dataset.theme).toBe('light');
    expect(document.documentElement.dataset.accent).toBe('firstlight');
    expect(document.documentElement.lang).toBe('zh-CN');
    expect(localStorage.getItem('sd-theme')).toBe('light');
    expect(localStorage.getItem('sd-accent')).toBe('firstlight');
    expect(localStorage.getItem('sd-lang')).toBe('zh-CN');

    toggleTheme();
    toggleLang();
    expect(localStorage.getItem('sd-theme')).toBe('dark');
    expect(localStorage.getItem('sd-lang')).toBe('en');
  });
});

describe('the colour of the browser bar', () => {
  // index.html gives one colour for each system scheme.
  beforeEach(() => {
    document.head.innerHTML =
      '<meta name="theme-color" media="(prefers-color-scheme: light)" content="#ffffff">' +
      '<meta name="theme-color" media="(prefers-color-scheme: dark)" content="#141822">';
  });
  afterEach(() => {
    document.head.innerHTML = '';
  });
  const bars = () =>
    [...document.querySelectorAll('meta[name=theme-color]')].map((m) => m.getAttribute('content'));

  it.each([
    ['auto', false, '#ffffff'],
    ['auto', true, '#141822'],
    ['dark', false, '#141822'],
    ['light', true, '#ffffff'],
  ] as const)('is, for %s with system dark %s, %s', async (theme, system, color) => {
    systemDark = system;
    const { choose, flushSync } = await load();
    choose('theme', theme);
    flushSync();
    expect(bars()).toEqual([color, color]);
  });
});

describe('theme', () => {
  it.each([
    ['auto', true, true],
    ['auto', false, false],
    ['dark', false, true],
    ['light', true, false],
  ] as const)('%s with system dark %s is dark: %s', async (theme, system, dark) => {
    const { prefs, isDark } = await load();
    prefs.theme = theme;
    systemDark = system;
    expect(isDark()).toBe(dark);
  });

  it('toggles away from what is shown', async () => {
    systemDark = true;
    const { prefs, toggleTheme } = await load();
    toggleTheme();
    expect(prefs.theme).toBe('light');
    toggleTheme();
    expect(prefs.theme).toBe('dark');
  });
});

describe('theme colour', () => {
  it.each([
    ['daybreak'],
    ['rain'],
    ['pool'],
    ['shallows'],
    ['shade'],
    ['hills'],
    ['glow'],
    ['firstlight'],
    ['nightfall'],
  ])('reads %s back on the next visit', async (accent) => {
    localStorage.setItem('sd-accent', accent);
    const { prefs } = await load();
    expect(prefs.accent).toBe(accent);
  });

  it('keeps the default of earlier versions, so a person who never chose sees no change', () => {
    const css = readFileSync('src/styles/accents.css', 'utf8');
    expect(css).toContain(
      ":root,\n[data-accent='daybreak'] {\n" +
        '  --sky: light-dark(#3a57e8, #8fa3ff);\n' +
        '  --on-sky: light-dark(#ffffff, #0b1030);\n}',
    );
  });

  it('has a block in accents.css for each colour, and none for another', async () => {
    const { accents } = await load();
    const css = readFileSync('src/styles/accents.css', 'utf8');
    const blocks = [...css.matchAll(/\[data-accent='([\w-]+)'\]/g)].map((m) => m[1]);
    expect(blocks).toEqual(accents);
  });
});

describe('language', () => {
  it('toggles between the two and looks texts up in the current one', async () => {
    const { prefs, t, toggleLang } = await load();
    expect(t('nav.settings')).toBe('Settings');
    toggleLang();
    expect(prefs.lang).toBe('zh-CN');
    expect(t('nav.settings')).toBe('设置');
    toggleLang();
    expect(prefs.lang).toBe('en');
  });
});

it('applies successive toggles in the order their transition callbacks run', async () => {
  const changes: (() => void)[] = [];
  const { prefs, toggleTheme, toggleLang } = await load();
  Object.assign(document, {
    startViewTransition(change: () => void) {
      changes.push(change);
      return { finished: Promise.resolve() };
    },
  });
  try {
    toggleTheme();
    toggleTheme();
    toggleLang();
    toggleLang();
    expect(changes).toHaveLength(4);
    expect(prefs).toEqual({ theme: 'auto', accent: 'daybreak', lang: 'en' });
    for (const [i, [theme, lang]] of [
      ['dark', 'en'],
      ['light', 'en'],
      ['light', 'zh-CN'],
      ['light', 'en'],
    ].entries()) {
      changes[i]();
      expect(prefs).toEqual({ theme, accent: 'daybreak', lang });
    }
  } finally {
    Reflect.deleteProperty(document, 'startViewTransition');
  }
});

it('announces top bar actions in the chosen language and clears the status on a radio choice', async () => {
  const { announcement, choose, prefs, toggleLang, toggleTheme } = await load();
  expect(announcement.text).toBe('');
  toggleTheme();
  expect(announcement.text).toBe('Theme: Dark');
  toggleTheme();
  expect(announcement.text).toBe('Theme: Light');
  toggleLang();
  expect(announcement.text).toBe('语言: 简体中文');
  toggleLang();
  expect(announcement.text).toBe('Language: English');
  choose('theme', 'auto');
  expect(prefs).toEqual({ theme: 'auto', accent: 'daybreak', lang: 'en' });
  expect(announcement.text).toBe('');
});

describe('a browser that blocks site storage', () => {
  beforeEach(() => {
    vi.spyOn(globalThis, 'localStorage', 'get').mockImplementation(() => {
      throw new DOMException('The operation is insecure.', 'SecurityError');
    });
  });
  afterEach(() => vi.restoreAllMocks());

  it('opens with the defaults', async () => {
    const { prefs } = await load();
    expect(prefs).toEqual({ theme: 'auto', accent: 'daybreak', lang: 'en' });
    expect(document.documentElement.dataset.theme).toBe('auto');
  });

  it('applies a change without remembering it', async () => {
    const { choose, flushSync } = await load();
    choose('theme', 'dark');
    choose('accent', 'pool');
    choose('lang', 'zh-CN');
    flushSync();
    expect(document.documentElement.dataset.theme).toBe('dark');
    expect(document.documentElement.dataset.accent).toBe('pool');
    expect(document.documentElement.lang).toBe('zh-CN');
  });
});

describe('a preference that a part of the client keeps', () => {
  const choices = ['shown', 'hidden'] as const;

  it('is the default until the person chooses, and stores nothing before', async () => {
    const { preference } = await load();
    const hint = preference('hint', choices, 'shown');
    expect(hint.value).toBe('shown');
    expect(localStorage.length).toBe(0);
  });

  it('remembers what the person chose, and reads it on the next visit', async () => {
    const first = await load();
    first.preference('hint', choices, 'shown').value = 'hidden';
    expect(localStorage.getItem('sd-hint')).toBe('hidden');
    const next = await load();
    expect(next.preference('hint', choices, 'shown').value).toBe('hidden');
  });

  it('ignores a stored value it does not allow', async () => {
    localStorage.setItem('sd-hint', 'sideways');
    const { preference } = await load();
    expect(preference('hint', choices, 'shown').value).toBe('shown');
  });

  it('changes what a component draws from it', async () => {
    const { flushSync, preference } = await load();
    const { watch } = await import('../../features/watch.svelte');
    const hint = preference('hint', choices, 'shown');
    const seen: string[] = [];
    const stop = watch(() => {
      seen.push(hint.value);
    });
    flushSync();
    hint.value = 'hidden';
    flushSync();
    stop();
    expect(seen).toEqual(['shown', 'hidden']);
  });

  it('is the same preference when made again with the same values', async () => {
    const { preference } = await load();
    const hint = preference('hint', choices, 'shown');
    expect(preference('hint', ['shown', 'hidden'], 'shown')).toBe(hint);
  });

  it.each([
    ['hint', ['shown'], 'shown'],
    ['hint', ['shown', 'hidden'], 'hidden'],
    ['theme', ['auto', 'light', 'dark'], 'auto'],
    ['lang', ['zh-CN', 'en'], 'en'],
    ['accent', ['daybreak'], 'daybreak'],
  ] as const)('refuses %s made again with %j and %s', async (name, allowed, fallback) => {
    const { preference } = await load();
    preference('hint', choices, 'shown');
    expect(() => preference(name, allowed, fallback)).toThrow(`the preference ${name} exists`);
  });

  it('holds a choice for the visit when the browser blocks site storage', async () => {
    const { preference } = await load();
    vi.spyOn(globalThis, 'localStorage', 'get').mockImplementation(() => {
      throw new DOMException('The operation is insecure.', 'SecurityError');
    });
    const hint = preference('hint', choices, 'shown');
    hint.value = 'hidden';
    expect(hint.value).toBe('hidden');
    vi.restoreAllMocks();
  });
});
