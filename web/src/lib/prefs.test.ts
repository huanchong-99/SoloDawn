import { beforeEach, describe, expect, it, vi } from 'vitest';

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
  }));
});

describe('first visit', () => {
  it('follows the system theme and the browser language', async () => {
    const { prefs } = await load();
    expect(prefs).toEqual({ theme: 'auto', lang: 'en' });
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
    localStorage.setItem('sd-lang', 'zh-CN');
    const { prefs } = await load();
    expect(prefs).toEqual({ theme: 'dark', lang: 'zh-CN' });
  });

  it('writes a change to <html> and to this browser', async () => {
    const { prefs, flushSync } = await load();
    expect(document.documentElement.dataset.theme).toBe('auto');
    expect(document.documentElement.lang).toBe('en');

    prefs.theme = 'light';
    prefs.lang = 'zh-CN';
    flushSync();

    expect(document.documentElement.dataset.theme).toBe('light');
    expect(document.documentElement.lang).toBe('zh-CN');
    expect(localStorage.getItem('sd-theme')).toBe('light');
    expect(localStorage.getItem('sd-lang')).toBe('zh-CN');
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
