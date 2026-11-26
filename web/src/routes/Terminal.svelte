<script lang="ts">
  import { onMount, untrack } from 'svelte';
  import { prefersReducedMotion } from 'svelte/motion';
  import { fade } from 'svelte/transition';
  import ConfirmCard from '../components/confirm/ConfirmCard.svelte';
  import Failure from '../components/Failure.svelte';
  import Icon from '../components/Icon.svelte';
  import TerminalInfo from '../components/terminal/TerminalInfo.svelte';
  import AskToWatch from '../components/watch/AskToWatch.svelte';
  import Watchers from '../components/watch/Watchers.svelte';
  import { Watch, watchBar, watchRequest } from '../components/watch/watch.svelte';
  import { find } from '../lib/groups.svelte';
  import { onOpenTerminal } from '../lib/notify.svelte';
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
    watches,
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
  const watching = $derived(watches(params.g, params.member));
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
  // Reached for while the takeover lasts. A watch that is open stays when the AI's computer loses
  // the home, and so the AI is offline.
  const reached = $derived(
    watch.phase === 'active' || (watch.phase !== 'ended' && ai?.state === 'takeover'),
  );
  const watchHost = $derived(watching && reached ? ai?.host : undefined);
  const session = $derived(watching ? watch : tk);
  // The requests to watch that wait for this reader, the owner.
  const asked = $derived(group ? group.waiting.filter((msg) => onOpenTerminal(group, msg)) : []);
  const request = $derived(watching ? watchRequest(group!, ai!) : undefined);
  const consent = $derived(request?.state.status === 'confirmed' ? request.id : undefined);
  const size = $derived(host?.terminalSizes.get(`${g}:${memberId}`));
  const backHref = $derived(href('chat', { g: params.g }));
  const subtitle = $derived(headerSubtitle(ai, task, group?.name));
  const bar = $derived(
    watching
      ? watchBar(watch, group!, ai!)
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
  $effect(() => {
    if (!watching || reached) session.follow(targetOf(host, ai), path);
  });
  // A card that goes while it has the focus gives it to the terminal.
  $effect.pre(() => {
    const ids = asked.map(({ id }) => `m-${id}`);
    const card = document.activeElement?.closest('.asked > div');
    if (card && !ids.includes(card.id)) untrack(() => term?.focus());
  });
  $effect(() => {
    const current = watch;
    if (consent) untrack(() => current.consented());
  });
  // With the consent the computer answers this browser, so it is reached for again.
  $effect(() => {
    void consent;
    const reach = net.watchHost;
    if (!reach || !watchHost) return;
    const release = untrack(() => reach(watchHost));
    // After the effect has run again, so that a connection that is open stays.
    return () => queueMicrotask(release);
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
      <div class="takeover" class:driving={bar.handback} role="status">
        <b>
          {#if watching}<Icon name="eye" sm />{:else}<i class="sq"></i>{/if}<span>{bar.title}</span>
        </b>
        <span class="grow">
          {#if bar.errorCode}
            <Failure code={bar.errorCode} />
          {:else if bar.detail}
            {bar.detail}
          {/if}
        </span>
        {#if bar.ask && group && ai}
          <AskToWatch {group} {ai} again />
        {/if}
        {#if bar.handback}
          <a class="btn btn-dawn" href={backHref} bind:this={exit} onclick={() => tk.handBack()}>
            {t('terminal.handback')}
          </a>
        {:else}
          <a class="btn" href={backHref} bind:this={exit}>{t('terminal.back')}</a>
        {/if}
      </div>

      <div class="screen">
        <div class="asked" aria-live="polite">
          {#each asked as msg (msg.id)}
            <div id={`m-${msg.id}`} out:fade={{ duration: prefersReducedMotion.current ? 0 : 140 }}>
              <ConfirmCard group={group!} {msg} />
            </div>
          {/each}
        </div>
        <div
          class="term"
          id="term"
          bind:this={termHost}
          role="region"
          aria-label={t('terminal.aria.term').replace('{ai}', ai?.name ?? t('terminal.title'))}
        ></div>
      </div>

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
    padding: 0 var(--sp-4) 0 var(--sp-2);
    border-bottom: 1px solid var(--line);
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
  /* The bar that says who drives, and under it the terminal as a framed dark panel. */
  .stage {
    display: grid;
    grid-template-rows: auto minmax(0, 1fr) auto;
    gap: var(--sp-3);
    padding: var(--sp-4);
  }
  .takeover {
    display: flex;
    align-items: center;
    gap: var(--sp-3);
    flex-wrap: wrap;
    min-height: 52px;
    padding: var(--sp-2) var(--sp-2) var(--sp-2) var(--sp-4);
    border-radius: 12px;
    background: var(--surface);
    box-shadow:
      var(--shadow-1),
      0 0 0 1px var(--line);
    color: var(--text-2);
    font-size: var(--fs-sm);
    overflow: hidden;
  }
  /* While the owner drives, an edge of dawn along its top: a person is at the keys. */
  .takeover.driving {
    box-shadow:
      inset 0 2px 0 var(--dawn),
      var(--shadow-1),
      0 0 0 1px var(--line);
  }
  .takeover b {
    display: inline-flex;
    align-items: center;
    gap: var(--sp-2);
    color: var(--text);
    font-size: var(--fs-ui);
  }
  .takeover .sq {
    width: 9px;
    height: 9px;
    border-radius: 2px;
    background: var(--text);
  }
  .takeover.driving .sq {
    background: var(--dawn);
  }
  .screen {
    position: relative;
    display: grid;
    min-height: 0;
    border: 1px solid var(--term-line);
    border-radius: 12px;
    background: var(--term-bg);
    overflow: hidden;
  }
  /* Over the terminal, which keeps its size and its focus, where the notifications are. */
  .asked {
    position: absolute;
    inset: 0 0 auto auto;
    z-index: 2;
    display: grid;
    gap: var(--sp-2);
    width: min(380px, 100%);
    max-height: 100%;
    padding: var(--sp-3);
    overflow: auto;
    pointer-events: none;
  }
  .asked > div {
    border-radius: var(--r-lg);
    box-shadow: var(--shadow-pop);
    pointer-events: auto;
    animation:
      drop var(--dur-2) var(--ease-out),
      appear var(--dur-2) var(--ease-out);
  }
  .asked :global(.card) {
    border-color: transparent;
    background: var(--raised);
    box-shadow: none;
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
    padding: var(--sp-4);
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
    border-radius: 8px;
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
    .takeover > :global(:is(.btn-sm, .fail)) {
      order: 4;
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
  /* On a phone the terminal has the screen to its edges, with its keys under it. */
  @media (max-width: 767px) {
    .term-head {
      padding: 0 var(--sp-3) 0 var(--sp-1);
    }
    .stage {
      gap: 0;
      height: calc(100dvh - var(--bar-h));
      padding: 0;
      background: var(--term-bg);
    }
    .takeover {
      border-radius: 0;
      padding-inline: var(--sp-3);
    }
    .screen {
      border: 0;
      border-radius: 0;
    }
    .term :global(.xterm) {
      padding: var(--sp-3);
    }
  }
</style>
