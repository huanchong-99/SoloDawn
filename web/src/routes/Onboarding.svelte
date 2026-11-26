<script lang="ts">
  import { onDestroy, untrack } from 'svelte';
  import Icon from '../components/Icon.svelte';
  import StatusPanel from '../components/pairing/StatusPanel.svelte';
  import type { Net } from '../lib/net.svelte';
  import { PairingSession, promptFor } from '../lib/onboard.svelte';
  import {
    announcement,
    isDark,
    otherLang,
    prefs,
    t,
    textIn,
    toggleLang,
    toggleTheme,
  } from '../lib/prefs.svelte';
  import { href } from '../lib/route.svelte';
  import '../styles/onboarding.css';

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
    await navigator.clipboard?.writeText(promptText).catch(() => undefined);
    copied = true;
    announcement.text = t('pairing.prompt.copied');
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
            {#if session.code || !session.enrolled}
              <button
                type="button"
                class="btn btn-dawn"
                onclick={() => void (session.code ? copyPrompt() : session.start())}
              >
                {#if session.code}<Icon name={copied ? 'check' : 'copy'} sm />{/if}
                <span>
                  {t(
                    session.code
                      ? copied
                        ? 'pairing.prompt.copied'
                        : 'pairing.prompt.copy'
                      : 'pairing.prompt.renew',
                  )}
                </span>
              </button>
            {/if}
          </header>
          <pre
            id="prompt"
            use:focusable
            role="region"
            aria-labelledby="prompt-h"
            class:muted={!session.code}>{promptText}</pre>
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
  .kind {
    font-weight: 600;
    font-size: var(--fs-ui);
    color: var(--text);
  }
  /* The prompt: a card with its title and the copy button, the text in mono on the second
     surface, and a line that says how long it holds. */
  .prompt {
    padding: 0;
    gap: 0;
    overflow: hidden;
  }
  .prompt > header {
    padding: var(--sp-3) var(--sp-3) var(--sp-3) var(--sp-4);
  }
  .prompt pre {
    margin: 0 var(--sp-3);
    padding: var(--sp-3) var(--sp-4);
    border-radius: var(--r-md);
    background: var(--surface-2);
    white-space: pre-wrap;
    overflow-wrap: anywhere;
    font-size: var(--fs-sm);
    line-height: 1.7;
  }
  .prompt pre:focus-visible {
    outline-offset: 2px;
  }
  .prompt footer {
    display: flex;
    align-items: center;
    gap: 6px;
    padding: 10px var(--sp-4) var(--sp-3);
  }
  .browser-card p {
    font-size: var(--fs-ui);
  }
  .howto {
    counter-reset: n;
    display: flex;
    flex-direction: column;
    gap: var(--sp-4);
    padding-inline: var(--sp-1);
  }
  .howto li {
    counter-increment: n;
    display: grid;
    grid-template-columns: 24px minmax(0, 1fr);
    gap: var(--sp-3);
  }
  .howto li::before {
    content: counter(n);
    display: grid;
    place-items: center;
    width: 24px;
    height: 24px;
    border-radius: 50%;
    background: var(--surface-3);
    font-size: var(--fs-xs);
    font-weight: 500;
    color: var(--text-2);
  }
  .howto b {
    display: block;
    font-size: var(--fs-ui);
    font-weight: 500;
  }
  .howto span {
    color: var(--text-2);
    font-size: var(--fs-sm);
  }
  /* What the person can count on: three columns, each under a small glyph. */
  .trust {
    grid-column: 1 / -1;
    display: grid;
    grid-template-columns: repeat(3, minmax(0, 1fr));
    gap: var(--sp-6);
    padding-top: var(--sp-6);
    border-top: 1px solid var(--line);
    font-size: var(--fs-sm);
    line-height: 1.5;
    color: var(--text-2);
  }
  .trust b {
    display: flex;
    align-items: center;
    gap: var(--sp-2);
    color: var(--text);
    font-size: var(--fs-ui);
    font-weight: 500;
    margin-bottom: 4px;
  }
  .trust b :global(.i) {
    color: var(--text-3);
  }
  @media (max-width: 860px) {
    .trust {
      grid-template-columns: minmax(0, 1fr);
      gap: var(--sp-4);
    }
  }
</style>
