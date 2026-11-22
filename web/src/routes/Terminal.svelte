<script lang="ts">
  import { onMount } from 'svelte';
  import Failure from '../components/Failure.svelte';
  import Icon from '../components/Icon.svelte';
  import TerminalInfo from '../components/terminal/TerminalInfo.svelte';
  import { find } from '../lib/groups.svelte';
  import { t } from '../lib/prefs.svelte';
  import { href, type PageProps } from '../lib/route.svelte';
  import {
    headerSubtitle,
    hostHomeOf,
    linkBadgeOf,
    phoneKeys,
    sessionTaskOf,
    Takeover,
    takeoverBarState,
    targetOf,
  } from '../lib/terminal.svelte';
  import { now } from '../lib/when';

  /** The terminal of an AI (`#/terminal/<g>/<member>[/<task>]`, `web/design/terminal.html`). */
  let { params, net }: PageProps<'terminal'> = $props();

  const termTheme = {
    background: '#0a0c11',
    foreground: '#d5d9e3',
    cursor: '#f6b152',
    black: '#0a0c11',
    green: '#7bd88f',
    yellow: '#f6b152',
    blue: '#8cabff',
    red: '#ff857d',
  };

  let termHost = $state<HTMLElement>();
  let term: import('@xterm/xterm').Terminal | undefined;
  // What the computer sends before xterm has loaded.
  const early: (Uint8Array | string)[] = [];
  const write = (chunk: Uint8Array | string) => (term ? term.write(chunk) : early.push(chunk));

  const group = $derived(find(params.g));
  const member = $derived(group?.member(params.member));
  const ai = $derived(member?.kind === 'ai' ? member : undefined);
  const host = $derived(ai ? hostHomeOf(net.homes, ai.host) : undefined);
  const path = $derived(host ? net.path(host) : undefined);
  const task = $derived(sessionTaskOf(group, ai, params.task));
  // Another address is another takeover: the effect below hands the previous one back. The shell
  // hands a new `params` on every `hashchange`, so the takeover is keyed on the values.
  const g = $derived(params.g);
  const memberId = $derived(params.member);
  const taskId = $derived(params.task);
  const tk = $derived(new Takeover(g, memberId, taskId, write));
  const backHref = $derived(href('chat', { g: params.g }));
  const subtitle = $derived(headerSubtitle(ai, task, group?.name));
  const bar = $derived(
    takeoverBarState({
      phase: tk.phase,
      ai,
      online: Boolean(host?.ctl),
      path,
      errorCode: tk.errorCode,
      startedAt: tk.startedAt,
      nowMs: now(),
    }),
  );

  $effect(() => {
    const current = tk;
    term?.clear();
    if (term) current.resize(term.cols, term.rows);
    return () => current.handBack();
  });
  $effect(() => tk.follow(targetOf(host, ai), path));

  onMount(() => {
    let cancelled = false;
    let observer: ResizeObserver | undefined;
    void Promise.all([
      import('@xterm/xterm'),
      import('@xterm/addon-fit'),
      import('@xterm/xterm/css/xterm.css'),
    ]).then(([{ Terminal: XTerm }, { FitAddon }]) => {
      if (cancelled || !termHost) return;
      const instance = new XTerm({
        cursorBlink: true,
        fontFamily:
          "'Geist Mono', ui-monospace, 'SF Mono', 'Cascadia Mono', Menlo, Consolas, monospace",
        fontSize: 13,
        lineHeight: 1.45,
        theme: termTheme,
      });
      const fit = new FitAddon();
      instance.loadAddon(fit);
      instance.open(termHost);
      term = instance;
      for (const chunk of early.splice(0)) instance.write(chunk);
      instance.onData((text) => tk.type(text));
      instance.onBinary((raw) => tk.typeBinary(raw));
      instance.onResize(({ cols, rows }) => tk.resize(cols, rows));
      fit.fit();
      tk.resize(instance.cols, instance.rows);
      observer = new ResizeObserver(() => fit.fit());
      observer.observe(termHost);
    });
    return () => {
      cancelled = true;
      observer?.disconnect();
      term?.dispose();
      term = undefined;
    };
  });
</script>

<svelte:window onpagehide={() => tk.handBack()} />

<div class="terminal-page">
  <header class="term-head">
    <a class="icon-btn" href={backHref} aria-label={t('terminal.back')}>
      <Icon name="back" />
    </a>
    <div class="bar-title">
      <h2>{ai ? `${ai.name} · ${t('terminal.title')}` : t('terminal.title')}</h2>
      {#if subtitle}<p>{subtitle}</p>{/if}
    </div>
  </header>

  <div class="takeover-wrap">
    <section class="stage" aria-label={t('terminal.aria.stage')}>
      <div class="takeover" role="status">
        <b><i class="sq"></i><span>{bar.title}</span></b>
        <span class="grow">
          {#if bar.detail}
            {bar.detail}
          {:else if bar.errorCode}
            <Failure code={bar.errorCode} />
          {/if}
        </span>
        {#if bar.handback}
          <a class="btn btn-dawn" href={backHref} onclick={() => tk.handBack()}>
            {t('terminal.handback')}
          </a>
        {:else}
          <a class="btn" href={backHref}>{t('terminal.back')}</a>
        {/if}
      </div>

      <div
        class="term"
        id="term"
        bind:this={termHost}
        role="region"
        aria-label={t('terminal.aria.term').replace('{ai}', ai?.name ?? t('terminal.title'))}
      ></div>

      <div class="keys only-narrow" role="toolbar" aria-label={t('terminal.aria.keys')}>
        {#each phoneKeys as key (key.label)}
          <button
            type="button"
            class:active={Boolean(key.ctrl && tk.ctrl)}
            aria-label={key.ariaKey ? t(key.ariaKey) : undefined}
            aria-pressed={key.ctrl ? tk.ctrl : undefined}
            disabled={tk.phase !== 'active'}
            onclick={() => (tk.press(key), term?.focus())}
          >
            {key.label}
          </button>
        {/each}
      </div>
    </section>

    <TerminalInfo {ai} {task} link={linkBadgeOf(Boolean(host?.ctl), path)} />
  </div>
</div>

<style>
  .terminal-page {
    display: grid;
    grid-template-rows: var(--bar-h) minmax(0, 1fr);
    height: 100%;
    min-height: 0;
  }
  .term-head {
    display: flex;
    align-items: center;
    gap: var(--sp-2);
    min-width: 0;
    height: var(--bar-h);
    padding: 0 var(--sp-4);
    border-bottom: 1px solid var(--line);
    background: var(--surface);
  }
  .term-head h2 {
    font-size: var(--fs-md);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .takeover-wrap {
    display: grid;
    height: 100%;
    min-height: 0;
    grid-template-columns: minmax(0, 1fr) var(--side-w);
  }
  .takeover-wrap > * {
    min-height: 0;
    min-width: 0;
  }
  .stage {
    display: grid;
    grid-template-rows: auto minmax(0, 1fr) auto;
    background: var(--term-bg);
  }
  .takeover {
    display: flex;
    align-items: center;
    gap: var(--sp-3);
    flex-wrap: wrap;
    padding: 10px var(--sp-4);
    background: var(--text);
    color: var(--bg);
    font-size: var(--fs-sm);
  }
  .takeover b {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    font-size: 14px;
  }
  .takeover .sq {
    width: 9px;
    height: 9px;
    border-radius: 2px;
    background: var(--dawn);
  }
  .takeover > span {
    opacity: 0.82;
  }
  .takeover :global(.fail) {
    color: inherit;
  }
  .term {
    min-height: 0;
    min-width: 0;
    overflow: hidden;
    background: var(--term-bg);
    color: var(--term-fg);
  }
  /* FitAddon counts the padding of .xterm, not of its parent. xterm.css paints the viewport
     black, and it spans that padding. */
  .term :global(.xterm) {
    height: 100%;
    padding: var(--sp-3) var(--sp-4);
  }
  .term :global(.xterm-viewport) {
    background-color: var(--term-bg);
  }
  .keys {
    display: flex;
    gap: 6px;
    padding: 8px var(--sp-3) calc(8px + env(safe-area-inset-bottom));
    background: var(--term-bg);
    border-top: 1px solid var(--term-line);
    overflow-x: auto;
    scrollbar-width: none;
  }
  .keys button {
    flex: none;
    min-width: 44px;
    height: 36px;
    padding: 0 10px;
    border: 1px solid var(--term-line);
    border-radius: var(--r-sm);
    background: var(--term-key);
    color: var(--term-fg);
    font: 12px var(--font-mono);
  }
  .keys button.active {
    border-color: var(--dawn);
    color: var(--dawn);
  }
  .keys button:disabled {
    opacity: 0.45;
    cursor: not-allowed;
  }
  @media (hover: hover) {
    .keys button:not(:disabled):hover {
      background: var(--term-line);
    }
  }
  @media (max-width: 640px) {
    .takeover b {
      flex: 1;
    }
    .takeover > .grow {
      order: 3;
      flex-basis: 100%;
    }
  }
  @media (max-width: 1023px) {
    .terminal-page {
      display: block;
      height: auto;
    }
    .takeover-wrap {
      display: block;
      height: auto;
    }
    .stage {
      height: calc(100dvh - 2 * var(--bar-h));
    }
  }
  @media (max-width: 767px) {
    .term-head {
      padding: 0 var(--sp-3) 0 var(--sp-2);
    }
    .stage {
      height: calc(100dvh - var(--bar-h));
    }
  }
</style>
