<script lang="ts">
  import Icon from '../components/Icon.svelte';
  import JoinForm from '../components/people/JoinForm.svelte';
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

  let { params, net }: PageProps<'join'> = $props();
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
    <a class="btn btn-sm only-wide" href={href('groups')}>{t('pairing.have_node')}</a>
  </header>

  <section class="hero">
    <span class="badge">{t('people.join.badge')}</span>
    <h1>{t('page.join')}</h1>
    <p>{t('people.join.body')}</p>
  </section>

  <div class="wrap">
    <div>
      {#key params.link}
        <JoinForm link={params.link} {net} />
      {/key}
    </div>
  </div>
</div>

<style>
  .top {
    display: flex;
    align-items: center;
    gap: var(--sp-2);
    max-width: 1080px;
    margin: 0 auto;
    padding: var(--sp-3) var(--sp-5);
  }
  .brand {
    display: inline-flex;
    align-items: center;
    gap: 10px;
    color: var(--text);
    font-weight: 600;
    font-size: var(--fs-lg);
    letter-spacing: -0.02em;
  }
  .brand:hover {
    text-decoration: none;
  }
  .hero {
    position: relative;
    overflow: clip;
    padding: var(--sp-12) var(--sp-5) 76px;
    text-align: center;
  }
  .hero::before {
    content: '';
    position: absolute;
    left: 50%;
    top: calc(100% - 52px);
    width: 2400px;
    height: 2400px;
    translate: -50% 0;
    border-radius: 50%;
    border-top: 1px solid color-mix(in srgb, var(--dawn) 70%, transparent);
    background: radial-gradient(
      closest-side at 50% 0,
      color-mix(in srgb, var(--dawn) 16%, transparent),
      transparent 22%
    );
    mask-image: linear-gradient(90deg, transparent 36%, #000 46%, #000 54%, transparent 64%);
    pointer-events: none;
  }
  .hero > * {
    position: relative;
  }
  .hero h1 {
    font-size: var(--fs-3xl);
    letter-spacing: -0.03em;
    max-width: 16em;
    margin: var(--sp-4) auto var(--sp-3);
    text-wrap: balance;
  }
  .hero p {
    color: var(--text-2);
    font-size: var(--fs-lg);
    max-width: 34em;
    margin: 0 auto;
    text-wrap: pretty;
  }
  .wrap {
    max-width: 1080px;
    margin: 0 auto;
    padding: 0 var(--sp-5) var(--sp-16);
    display: grid;
    gap: var(--sp-5);
    grid-template-columns: minmax(0, 1.25fr) minmax(0, 1fr);
    align-items: start;
  }
  .wrap > div {
    display: flex;
    flex-direction: column;
    gap: var(--sp-5);
    min-width: 0;
  }
  @media (max-width: 860px) {
    .wrap {
      grid-template-columns: minmax(0, 1fr);
    }
    .hero {
      padding-top: var(--sp-8);
    }
    .hero p {
      font-size: var(--fs-md);
    }
  }
</style>
