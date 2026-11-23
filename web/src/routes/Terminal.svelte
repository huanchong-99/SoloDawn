<script lang="ts">
  import { onMount, untrack } from 'svelte';
  import Failure from '../components/Failure.svelte';
  import Icon from '../components/Icon.svelte';
  import TerminalInfo from '../components/terminal/TerminalInfo.svelte';
  import AskToWatch from '../components/watch/AskToWatch.svelte';
  import Watchers from '../components/watch/Watchers.svelte';
  import { Watch } from '../components/watch/watch.svelte';
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
  let exit = $state<HTMLElement>();
  let term = $state.raw<import('@xterm/xterm').Terminal>();
  // What the computer sends before xterm has loaded.
  const early: (Uint8Array | string)[] = [];
  const write = (chunk: Uint8Array | string) => (term ? term.write(chunk) : early.push(chunk));

  const group = $derived(find(params.g));
  const member = $derived(group?.member(params.member));
  const ai = $derived(member?.kind === 'ai' ? member : undefined);
  const watching = $derived(Boolean(ai && group && ai.user !== group.home.me.user));
  const host = $derived(ai ? hostHomeOf(net.homes, ai.host) : undefined);
  const path = $derived(host ? net.path(host) : undefined);
  const task = $derived(sessionTaskOf(group, ai, params.task));
  // Another address is another takeover: the effect below hands the previous one back. The shell
  // hands a new `params` on every `hashchange`, so the takeover is keyed on the values.
  const g = $derived(params.g);
  const memberId = $derived(params.member);
  const taskId = $derived(params.task);
  const tk = $derived(new Takeover(g, memberId, taskId, write));
  const watch = $derived(new Watch(g, memberId, write));
  const watchHost = $derived(watching && watch.phase !== 'ended' ? ai?.host : undefined);
  const session = $derived(watching ? watch : tk);
  const size = $derived(host?.terminalSizes.get(`${g}:${memberId}`));
  const backHref = $derived(href('chat', { g: params.g }));
  const subtitle = $derived(headerSubtitle(ai, task, group?.name));
  const bar = $derived(
    watching
      ? {
          title: t(`watch.${watch.phase}`),
          detail: t('watch.readonly'),
          handback: false,
          errorCode: watch.errorCode,
        }
      : takeoverBarState({
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
    const current = session;
    untrack(() => {
      term?.clear();
      if (term && !watching) tk.resize(term.cols, term.rows);
    });
    return () => current.handBack();
  });
  $effect(() => session.follow(targetOf(host, ai), path));
  $effect(() => {
    if (watchHost) return net.watchHost?.(watchHost);
  });
  $effect(() => {
    if (watching && term && size) term.resize(size.cols, size.rows);
  });

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
        cursorBlink: !watching,
        disableStdin: watching,
        fontFamily:
          "'Geist Mono', ui-monospace, 'SF Mono', 'Cascadia Mono', Menlo, Consolas, monospace",
        fontSize: 13,
        lineHeight: 1.45,
        theme: termTheme,
      });
      const fit = new FitAddon();
      instance.loadAddon(fit);
      instance.open(termHost);
      instance.textarea!.setAttribute('aria-describedby', 'term-leave');
      // Tab and Shift+Tab belong to the terminal, so Ctrl+M, which a terminal reads as Enter,
      // is the keyboard's way out of it: to the bar's button.
      instance.attachCustomKeyEventHandler((event) => {
        const leave = event.ctrlKey && event.code === 'KeyM';
        if (leave) {
          event.preventDefault();
          exit!.focus();
        }
        return !leave;
      });
      term = instance;
      for (const chunk of early.splice(0)) instance.write(chunk);
      if (!watching) {
        instance.onData((text) => tk.type(text));
        instance.onBinary((raw) => tk.typeBinary(raw));
        instance.onResize(({ cols, rows }) => tk.resize(cols, rows));
        fit.fit();
        tk.resize(instance.cols, instance.rows);
        observer = new ResizeObserver(() => fit.fit());
        observer.observe(termHost);
      }
    });
    return () => {
      cancelled = true;
      observer?.disconnect();
      term?.dispose();
      term = undefined;
    };
  });
</script>

<svelte:window onpagehide={() => session.handBack()} />

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
          {#if bar.errorCode}
            <Failure code={bar.errorCode} />
          {:else if bar.detail}
            {bar.detail}
          {/if}
        </span>
        {#if watching && watch.errorCode === 'forbidden' && group && ai}
          <AskToWatch {group} {ai} />
        {/if}
        {#if bar.handback}
          <a class="btn btn-dawn" href={backHref} bind:this={exit} onclick={() => tk.handBack()}>
            {t('terminal.handback')}
          </a>
        {:else}
          <a class="btn" href={backHref} bind:this={exit}>{t('terminal.back')}</a>
        {/if}
      </div>

      <div
        class="term"
        id="term"
        bind:this={termHost}
        role="region"
        aria-label={t('terminal.aria.term').replace('{ai}', ai?.name ?? t('terminal.title'))}
      ></div>

      {#if !watching}<div
          class="keys only-narrow"
          role="toolbar"
          aria-label={t('terminal.aria.keys')}
        >
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
        </div>{/if}
    </section>

    <TerminalInfo {ai} {task} {watching} link={linkBadgeOf(Boolean(host?.ctl), path)}>
      {#if !watching && group && host}<Watchers {group} {host} member={memberId} />{/if}
    </TerminalInfo>
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
  /* xterm draws faint text at half the foreground's opacity, 4.1 to 1 on --term-bg. */
  .term :global(.xterm-rows .xterm-dim:not([class*='xterm-fg-'])) {
    color: var(--term-dim);
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
