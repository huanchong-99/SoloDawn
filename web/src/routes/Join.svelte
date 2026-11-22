<script lang="ts">
  import Icon from '../components/Icon.svelte';
  import JoinForm from '../components/people/JoinForm.svelte';
  import { readInvitation } from '../lib/join';
  import {
    isDark,
    otherLang,
    prefs,
    t,
    textIn,
    toggleLang,
    toggleTheme,
  } from '../lib/prefs.svelte';
  import { href, type PageProps } from '../lib/route.svelte';
  import '../styles/onboarding.css';

  let { params, net }: PageProps<'join'> = $props();
  const invitation = $derived(read(params.link));

  function read(link: string | undefined) {
    try {
      return readInvitation(link ?? '');
    } catch {}
  }
</script>

<div class="onboarding">
  <header class="top">
    <a class="brand grow" href={href('groups')}>
      <img class="logo-mark" src="icon.svg" alt="" />SoloDawn
    </a>
    <button class="icon-btn" onclick={toggleLang}>
      {#key prefs.lang}<span class="swap" lang={otherLang()}>{t('bar.lang.other')}</span>{/key}
      <span class="sr">
        <span lang="en">{textIn('en', 'bar.lang')}</span> /
        <span lang="zh-CN">{textIn('zh-CN', 'bar.lang')}</span>
      </span>
    </button>
    <button class="icon-btn" onclick={toggleTheme} aria-label={t('bar.theme')}>
      {#key isDark()}<span class="swap"><Icon name={isDark() ? 'sun' : 'moon'} /></span>{/key}
    </button>
  </header>

  <section class="hero">
    <span class="badge">{t('people.join.badge')}</span>
    <h1>{t('page.join')}</h1>
    {#if invitation}<p>{t('people.join.body')}</p>{/if}
  </section>

  <div class="wrap single">
    {#key params.link}
      <JoinForm {invitation} {net} />
    {/key}
  </div>
</div>
