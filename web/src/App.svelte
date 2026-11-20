<script lang="ts">
  import Icon, { type IconName } from './components/Icon.svelte';
  import { isDark, t, toggleLang, toggleTheme } from './lib/prefs.svelte';
  import { pages, route, type Page } from './lib/route.svelte';
  import Groups from './routes/Groups.svelte';
  import Nodes from './routes/Nodes.svelte';
  import Settings from './routes/Settings.svelte';

  const views = { groups: Groups, nodes: Nodes, settings: Settings };
  const icons: Record<Page, IconName> = { groups: 'chat', nodes: 'pc', settings: 'sliders' };
  const View = $derived(views[route.page]);
  let main: HTMLElement;
</script>

<svelte:head><title>{t(`${route.page}.title`)} · SoloDawn</title></svelte:head>

<div class="app">
  <button class="skip" onclick={() => main.focus()}>{t('app.skip')}</button>
  <nav class="rail" aria-label={t('nav.label')}>
    <a class="logo" href="#/groups" aria-label="SoloDawn">
      <img class="logo-mark" src="icon.svg" alt="" />
    </a>
    {#each pages as page (page)}
      <a href="#/{page}" aria-current={page === route.page ? 'page' : undefined}>
        <Icon name={icons[page]} /><span>{t(`nav.${page}`)}</span>
      </a>
    {/each}
  </nav>
  <header class="bar">
    <div class="bar-title"><h1>{t(`${route.page}.title`)}</h1></div>
    <div class="global only-wide">
      <button class="icon-btn" onclick={toggleTheme} aria-label={t('bar.theme')}>
        <Icon name={isDark() ? 'sun' : 'moon'} />
      </button>
      <button class="icon-btn" onclick={toggleLang} aria-label={t('bar.lang')}>
        {t('bar.lang.other')}
      </button>
    </div>
  </header>
  <main bind:this={main} tabindex="-1"><View /></main>
</div>
