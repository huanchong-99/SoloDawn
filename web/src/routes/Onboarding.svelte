<script lang="ts">
  import { onDestroy, untrack } from 'svelte';
  import Icon from '../components/Icon.svelte';
  import StatusPanel from '../components/pairing/StatusPanel.svelte';
  import type { Net } from '../lib/net.svelte';
  import { PairingSession, promptFor } from '../lib/onboard.svelte';
  import {
    isDark,
    otherLang,
    prefs,
    t,
    textIn,
    toggleLang,
    toggleTheme,
  } from '../lib/prefs.svelte';
  import { href } from '../lib/route.svelte';

  let { net }: { params: Record<string, string | undefined>; net: Net } = $props();

  const maskedCode = '••••-••••-••••-••••';
  const focusable = (el: HTMLElement) => {
    el.tabIndex = 0;
  };
  const session = new PairingSession(untrack(() => net.source));
  let copied = $state(false);
  let copyTimer: ReturnType<typeof setTimeout> | undefined;

  onDestroy(() => {
    clearTimeout(copyTimer);
    session.destroy();
  });

  const promptText = $derived(promptFor(prefs.lang, session.code || maskedCode));

  const countdownText = $derived.by(() => {
    if (session.enrolled) return t('pairing.countdown.used');
    if (!session.code || session.stage === 'expired') return t('pairing.countdown.expired');
    const m = Math.floor(session.remaining / 60);
    const s = session.remaining % 60;
    if (m > 0) {
      return t('pairing.countdown.min_sec').replace('{m}', String(m)).replace('{s}', String(s));
    }
    return t('pairing.countdown.sec').replace('{s}', String(s));
  });

  async function copyPrompt() {
    if (!session.code) return;
    await navigator.clipboard?.writeText(promptText).catch(() => undefined);
    copied = true;
    clearTimeout(copyTimer);
    copyTimer = setTimeout(() => (copied = false), 1800);
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
    <a class="btn btn-sm only-wide" href={href('groups')}>{t('pairing.have_node')}</a>
  </header>

  <section class="hero">
    {#if session.role === 'b'}
      <span class="badge">{t('pairing.browser.step')}</span>
      <h1>{t('pairing.browser.title')}</h1>
      <p>{t('pairing.browser.body')}</p>
    {:else}
      <span class="badge">{t('pairing.hero.step')}</span>
      <h1>{t('pairing.hero.title')}</h1>
      <p>{t('pairing.hero.body')}</p>
    {/if}
  </section>

  <div class="wrap">
    <div>
      {#if session.role === 'b'}
        <section class="card browser-card" aria-labelledby="browser-h">
          <h2 class="kind" id="browser-h">{t('pairing.browser.card')}</h2>
          {#if session.browserFingerprint}
            <p class="muted">
              <span>{t('pairing.step.fingerprint')}</span>
              <code>{session.browserFingerprint}</code>
            </p>
          {/if}
          <p class="hint">{t('pairing.browser.hint')}</p>
        </section>
      {:else}
        <section class="card prompt" aria-labelledby="prompt-h">
          <header class="row">
            <h2 class="kind grow" id="prompt-h">{t('pairing.prompt.title')}</h2>
            {#if session.code}
              <button class="btn btn-dawn" onclick={copyPrompt}>
                <Icon name={copied ? 'check' : 'copy'} sm />
                <span>{t(copied ? 'pairing.prompt.copied' : 'pairing.prompt.copy')}</span>
              </button>
            {:else if !session.enrolled}
              <button class="btn btn-dawn" onclick={() => void session.start()}>
                <span>{t('pairing.prompt.renew')}</span>
              </button>
            {/if}
          </header>
          <pre id="prompt" use:focusable class:muted={!session.code}>{promptText}</pre>
          <footer class="hint">
            <Icon name="clock" sm />
            <span>{countdownText}</span>
          </footer>
        </section>

        <ol class="howto" aria-label={t('pairing.howto.label')}>
          <li>
            <div>
              <b>{t('pairing.howto.1.title')}</b>
              <span>{t('pairing.howto.1.body')}</span>
            </div>
          </li>
          <li>
            <div>
              <b>{t('pairing.howto.2.title')}</b>
              <span>{t('pairing.howto.2.body')}</span>
            </div>
          </li>
          <li>
            <div>
              <b>{t('pairing.howto.3.title')}</b>
              <span>{t('pairing.howto.3.body')}</span>
            </div>
          </li>
          <li>
            <div>
              <b>{t('pairing.howto.4.title')}</b>
              <span>{t('pairing.howto.4.body')}</span>
            </div>
          </li>
        </ol>
      {/if}
    </div>

    <div>
      <StatusPanel {session} />
    </div>

    <ul class="trust">
      <li>
        <b><Icon name="home" sm /><span>{t('pairing.trust.1.title')}</span></b>
        <span>{t('pairing.trust.1.body')}</span>
      </li>
      <li>
        <b><Icon name="direct" sm /><span>{t('pairing.trust.2.title')}</span></b>
        <span>{t('pairing.trust.2.body')}</span>
      </li>
      <li>
        <b><Icon name="key" sm /><span>{t('pairing.trust.3.title')}</span></b>
        <span>{t('pairing.trust.3.body')}</span>
      </li>
    </ul>
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
  .kind {
    font-weight: 600;
    font-size: var(--fs-sm);
    color: var(--text);
  }
  .prompt {
    padding: 0;
    gap: 0;
    overflow: hidden;
  }
  .prompt > header {
    padding: 10px 10px 10px var(--sp-4);
    border-bottom: 1px solid var(--line);
  }
  .prompt pre {
    padding: var(--sp-4);
    white-space: pre-wrap;
    overflow-wrap: anywhere;
    font-size: 13.5px;
    line-height: 1.7;
  }
  .prompt footer {
    display: flex;
    align-items: center;
    gap: 6px;
    padding: 10px var(--sp-4);
    border-top: 1px solid var(--line);
    background: var(--surface-2);
  }
  .browser-card p {
    font-size: var(--fs-sm);
  }
  .howto {
    counter-reset: n;
    display: flex;
    flex-direction: column;
    gap: var(--sp-4);
  }
  .howto li {
    counter-increment: n;
    display: grid;
    grid-template-columns: 28px minmax(0, 1fr);
    gap: var(--sp-3);
  }
  .howto li::before {
    content: counter(n);
    display: grid;
    place-items: center;
    width: 28px;
    height: 28px;
    border-radius: 50%;
    border: 1px solid var(--line-strong);
    font: 500 13px var(--font-mono);
    color: var(--text-2);
  }
  .howto b {
    display: block;
  }
  .howto span {
    color: var(--text-2);
    font-size: var(--fs-sm);
  }
  .trust {
    grid-column: 1 / -1;
    display: grid;
    grid-template-columns: repeat(3, minmax(0, 1fr));
    gap: var(--sp-5);
    padding-top: var(--sp-6);
    border-top: 1px solid var(--line);
    font-size: var(--fs-sm);
    color: var(--text-2);
  }
  .trust b {
    display: flex;
    align-items: center;
    gap: 6px;
    color: var(--text);
    margin-bottom: 2px;
  }
  @media (max-width: 860px) {
    .wrap {
      grid-template-columns: minmax(0, 1fr);
    }
    .trust {
      grid-template-columns: minmax(0, 1fr);
      gap: var(--sp-4);
    }
    .hero {
      padding-top: var(--sp-8);
    }
    .hero p {
      font-size: var(--fs-md);
    }
  }
</style>
