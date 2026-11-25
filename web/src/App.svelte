<script lang="ts">
  import { onDestroy, untrack, type Component } from 'svelte';
  import Icon, { type IconName } from './components/Icon.svelte';
  import Mount from './components/Mount.svelte';
  import { sourceOf, start, type Net } from './lib/net.svelte';
  import { nodes } from './lib/nodes.svelte';
  import {
    announcement,
    isDark,
    otherLang,
    prefs,
    t,
    textIn,
    toggleLang,
    toggleTheme,
  } from './lib/prefs.svelte';
  import { href, route, routes, tabs, type Tab } from './lib/route.svelte';

  type View = Component<{ params: Record<string, string | undefined>; net: Net }>;

  // Each page is drawn by the file its route names in routes/, found by name. A page of
  // routes/dev/ reads the node's wire examples: only a development server loads it, so that the
  // build leaves it out.
  const views = import.meta.glob<View>('./routes/*.svelte', { eager: true, import: 'default' });
  const devViews = import.meta.env.DEV
    ? import.meta.glob<View>('./routes/dev/*.svelte', { import: 'default' })
    : {};
  const loaded = $state<Record<string, View>>({});

  // The shell decides where the homes come from, once; every page is given the result.
  const { net, stop } = start(sourceOf(import.meta.env.MODE, location.search));
  onDestroy(stop);

  $effect(() => nodes.sync(net.homes));
  $effect(() => nodes.tell(net.homes));

  const icons: Record<Tab, IconName> = { groups: 'chat', nodes: 'pc', settings: 'sliders' };
  const at = $derived(routes[route.page]);
  const file = $derived(`./routes/${at.view}.svelte`);
  const View: View | undefined = $derived(views[file] ?? loaded[file]);
  const tab = $derived('tab' in at ? at.tab : undefined);
  const pageTitle = $derived(t(`page.${route.page}`));
  let main: HTMLElement;
  let previous = untrack(() => View);

  $effect(() => {
    if (file in devViews && !loaded[file]) void devViews[file]().then((v) => (loaded[file] = v));
  });
  // Another page component takes focus to the main landmark. Within one component, such as the
  // group list and an open chat, the component moves focus itself.
  $effect(() => {
    if (View && View !== previous) {
      previous = View;
      main.focus();
    }
  });
</script>

<svelte:head><title>{`${pageTitle} · SoloDawn`}</title></svelte:head>

<div class="app" style:--dir={route.dir} data-full={'full' in at ? at.full : undefined}>
  <button class="skip" onclick={() => main.focus()}>{t('app.skip')}</button>
  <span class="sr" role="status">{announcement.text}</span>
  <nav class="rail" aria-label={t('nav.label')}>
    <a class="logo" href={href('groups')} aria-label="SoloDawn">
      <img class="logo-mark" src="icon.svg" alt="" />
    </a>
    <div class="tabs" style:--at={tab ? tabs.indexOf(tab) : 0}>
      {#each tabs as entry (entry)}
        <a href={href(entry)} aria-current={entry === tab ? 'page' : undefined}>
          <Icon name={icons[entry]} /><span>{t(`nav.${entry}`)}</span>
        </a>
      {/each}
    </div>
  </nav>
  <header class="bar">
    <div class="bar-title">
      {#key View}<h1 id="page-title">{pageTitle}</h1>{/key}
    </div>
    <div class="global">
      <Mount at="bar" props={{ net }} />
      <button class="icon-btn only-wide" onclick={toggleTheme} aria-label={t('bar.theme')}>
        {#key isDark()}<span class="swap"><Icon name={isDark() ? 'sun' : 'moon'} /></span>{/key}
      </button>
      <!-- Named by its visible text first, then in both languages, each part in its own (WCAG 2.1
           SC 2.5.3 and 3.1.2). -->
      <button class="icon-btn only-wide" onclick={toggleLang}>
        {#key prefs.lang}<span class="swap" lang={otherLang()}>{t('bar.lang.other')}</span>{/key}
        <span class="sr">
          <span lang="en">{textIn('en', 'bar.lang')}</span> /
          <span lang="zh-CN">{textIn('zh-CN', 'bar.lang')}</span>
        </span>
      </button>
    </div>
  </header>
  <div class="ban"><Mount at="banner" props={{ net }} /></div>
  <main bind:this={main} tabindex="-1" aria-labelledby="page-title">
    {#if View && !(net.otherTab && route.page === 'onboarding')}
      <View params={route.params} {net} />
    {/if}
  </main>
  <Mount at="layer" props={{ net }} />
</div>
