<script lang="ts">
  import { onDestroy, onMount, untrack } from 'svelte';
  import Failure from '../components/Failure.svelte';
  import Icon from '../components/Icon.svelte';
  import TerminalInfo from '../components/terminal/TerminalInfo.svelte';
  import { find, type Home } from '../lib/groups.svelte';
  import { nodes } from '../lib/nodes.svelte';
  import { t } from '../lib/prefs.svelte';
  import { href, type PageProps } from '../lib/route.svelte';
  import {
    applyCtrl,
    binaryBytes,
    bridgePty,
    endTakeover,
    headerSubtitle,
    hostHomeOf,
    linkBadgeOf,
    phoneKeys,
    ptyLabel,
    queuedCountOf,
    sendResize,
    sessionTaskOf,
    startTakeover,
    takeoverBarState,
    type PhoneKey,
    type PtyBridge,
    type TakeoverPhase,
  } from '../lib/terminal';
  import { now } from '../lib/when';

  /** Interactive terminal takeover page (`#/terminal/:g/:member/:task?`, F-12). */
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
  let phase = $state<TakeoverPhase>('idle');
  let errorCode = $state('');
  let startedAt = $state(0);
  let ctrl = $state(false);
  let openedKey = '';
  let runSeq = 0;

  let pty: PtyBridge | undefined;
  let activeTarget:
    { host: Home; to: string; g: string; member: string; ended: boolean } | undefined;
  let term: import('@xterm/xterm').Terminal | undefined;
  let fitAddon: import('@xterm/addon-fit').FitAddon | undefined;
  const pendingChunks: (Uint8Array | string)[] = [];

  const group = $derived(find(params.g));
  const memberRow = $derived(params.member ? group?.member(params.member) : undefined);
  const ai = $derived(memberRow?.kind === 'ai' ? memberRow : undefined);
  const owned = $derived(Boolean(group && ai && ai.user === group.home.me.user));
  const host = $derived(ai ? hostHomeOf(net.homes, group, ai.host) : undefined);
  const path = $derived(host ? net.path(host) : undefined);
  const task = $derived(sessionTaskOf(group, ai, params.task));
  const hostInfo = $derived(host?.hostInfo ?? (host ? nodes.infos.get(host) : undefined));
  const queued = $derived(ai ? queuedCountOf(hostInfo, params.g, ai.id) : 0);
  const backHref = $derived(href('chat', { g: params.g }));
  const subtitle = $derived(headerSubtitle(ai, task, group?.name));
  const link = $derived(linkBadgeOf(Boolean(host?.ctl), path));
  const bar = $derived(
    takeoverBarState({
      phase,
      ai,
      owned,
      online: Boolean(host?.ctl),
      path,
      errorCode,
      startedAt,
      nowMs: phase === 'active' ? now() : startedAt,
      queued,
    }),
  );

  function fitAndResize() {
    if (!term || !fitAddon) return;
    try {
      fitAddon.fit();
    } catch {
      // Unsized test containers keep xterm's default dimensions.
    }
    if (activeTarget && !activeTarget.ended) {
      sendResize(activeTarget.host.ctl, activeTarget.g, activeTarget.member, term.cols, term.rows);
    }
  }

  function writeChunk(chunk: Uint8Array | string) {
    if (term) term.write(chunk);
    else pendingChunks.push(chunk);
  }

  function handleInput(raw: string) {
    const next = applyCtrl(raw, ctrl);
    ctrl = next.ctrl;
    pty?.sendText(next.out);
  }

  function pressPhoneKey(key: PhoneKey) {
    if (key.ctrl) {
      ctrl = !ctrl;
    } else if (key.seq) {
      handleInput(key.seq);
    }
    term?.focus();
  }

  function handBack() {
    runSeq += 1;
    pty?.close();
    pty = undefined;
    if (activeTarget && !activeTarget.ended) {
      activeTarget.ended = true;
      void endTakeover(activeTarget.host, activeTarget.to, activeTarget.g, activeTarget.member);
    }
    if (phase === 'active' || phase === 'starting') phase = 'ended';
  }

  async function openTakeover(
    targetHost: Home,
    to: string,
    g: string,
    member: string,
    taskId?: string,
  ) {
    const myRun = (runSeq += 1);
    const current = { host: targetHost, to, g, member, ended: false };
    activeTarget = current;
    phase = 'starting';
    errorCode = '';
    const code = await startTakeover(targetHost, to, g, member, taskId);
    if (myRun !== runSeq) {
      if (code === '' && !current.ended) {
        current.ended = true;
        void endTakeover(targetHost, to, g, member);
      }
      return;
    }
    if (code === 'relay') {
      current.ended = true;
      phase = 'relay';
      return;
    }
    if (code !== '') {
      current.ended = true;
      errorCode = code;
      phase = 'error';
      return;
    }
    if (net.path(targetHost)?.kind === 'relay') {
      current.ended = true;
      void endTakeover(targetHost, to, g, member);
      phase = 'relayed';
      return;
    }
    const ch = targetHost.ctl?.channel?.(ptyLabel(g, member));
    if (!ch) {
      current.ended = true;
      void endTakeover(targetHost, to, g, member);
      errorCode = 'lost';
      phase = 'error';
      return;
    }
    startedAt = Date.now();
    phase = 'active';
    pty = bridgePty(ch, {
      onOpen: () => fitAndResize(),
      onChunk: writeChunk,
      onClose: () => {
        current.ended = true;
        pty = undefined;
        if (phase === 'active') phase = 'ended';
      },
    });
    fitAndResize();
  }

  $effect(() => {
    if (host?.ctl) nodes.sync([host]);
  });

  // Reset when navigating between two terminal addresses while mounted.
  $effect(() => {
    const key = `${params.g}/${params.member}/${params.task ?? ''}`;
    if (openedKey && openedKey !== key) {
      untrack(() => {
        handBack();
        phase = 'idle';
        errorCode = '';
        ctrl = false;
        term?.clear();
      });
    }
    openedKey = key;
  });

  // Start takeover once the owned AI and a direct path to its host are ready, or refuse on relay.
  $effect(() => {
    if (phase !== 'idle' || !ai || !owned || !host?.ctl || ai.state === 'offline' || !path) return;
    if (path.kind === 'relay') {
      phase = 'relay';
      return;
    }
    const targetHost = host;
    const targetAi = ai;
    const g = params.g;
    const taskId = params.task;
    untrack(() => void openTakeover(targetHost, targetAi.host, g, targetAi.id, taskId));
  });

  // A7.9: when an active pty connection switches to relay, close pty cleanly and keep ctl open.
  $effect(() => {
    if (phase === 'active' && path?.kind === 'relay') {
      untrack(() => {
        if (activeTarget) activeTarget.ended = true;
        pty?.close();
        pty = undefined;
        phase = 'relayed';
      });
    }
  });

  onMount(() => {
    let cancelled = false;
    let observer: ResizeObserver | undefined;
    let dataSub: { dispose(): void } | undefined;
    let binSub: { dispose(): void } | undefined;
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
      fitAddon = fit;
      for (const chunk of pendingChunks.splice(0)) instance.write(chunk);
      dataSub = instance.onData(handleInput);
      binSub = instance.onBinary((raw) => pty?.send(binaryBytes(raw)));
      fitAndResize();
      if (typeof ResizeObserver !== 'undefined') {
        observer = new ResizeObserver(() => fitAndResize());
        observer.observe(termHost);
      }
    });
    return () => {
      cancelled = true;
      observer?.disconnect();
      dataSub?.dispose();
      binSub?.dispose();
      term?.dispose();
      term = undefined;
      fitAddon = undefined;
    };
  });

  onDestroy(handBack);
</script>

<svelte:window onpagehide={handBack} onbeforeunload={handBack} />

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
          <a class="btn btn-dawn" href={backHref} onclick={handBack}>{t('terminal.handback')}</a>
        {:else}
          <a class="btn btn-sm" href={backHref}>{t('terminal.back')}</a>
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
            class:active={Boolean(key.ctrl && ctrl)}
            aria-label={key.ariaKey ? t(key.ariaKey) : undefined}
            aria-pressed={key.ctrl ? ctrl : undefined}
            disabled={phase !== 'active'}
            onclick={() => pressPhoneKey(key)}
          >
            {key.label}
          </button>
        {/each}
      </div>
    </section>

    <TerminalInfo {ai} {task} {link} {queued} />
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
  .term :global(.xterm) {
    height: 100%;
    padding: var(--sp-3) var(--sp-4);
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
