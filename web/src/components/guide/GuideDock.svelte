<script lang="ts">
  import { onDestroy, tick, untrack } from 'svelte';
  import { prefersReducedMotion } from 'svelte/motion';
  import { MediaQuery } from 'svelte/reactivity';
  import { fade } from 'svelte/transition';
  import { creating, recent } from '../../lib/groups.svelte';
  import {
    canCollapseOnEscape,
    closeGuide,
    collapseGuide,
    createAnnouncer,
    createOverlapTracker,
    expandGuide,
    finishGuide,
    guideState,
    guideSteps,
    guideUi,
    modal,
    stepActionLabel,
    stepDoneText,
    stepIntent,
    type GuideStep,
  } from '../../lib/guide.svelte';
  import type { Net } from '../../lib/net.svelte';
  import { nodes } from '../../lib/nodes.svelte';
  import { announcement, prefs, t } from '../../lib/prefs.svelte';
  import { route } from '../../lib/route.svelte';
  import Icon from '../Icon.svelte';
  import { beacon, clearBeacon, setBeaconHost } from './beacon';

  /** Persistent checklist dock, collapsed pill, phone bottom sheet, and 4 s closed note. */
  let { net }: { net: Net } = $props();
  const uid = $props.id();

  const narrow = new MediaQuery('max-width: 767px');
  const computers = $derived(nodes.list(net));
  const groups = $derived(recent());
  const steps = $derived(guideSteps(computers, groups));
  const doneCount = $derived(steps.filter((s) => s.done).length);
  const allDone = $derived(doneCount === steps.length);
  const current = $derived(steps.find((s) => !s.done));
  const visible = $derived(guideState.value === 'open' && route.page !== 'join');
  const hidePill = $derived(narrow.current && (route.page === 'chat' || route.page === 'terminal'));

  let beaconLayer = $state<HTMLElement>();
  let dock = $state<HTMLElement>();
  let dockHeading = $state<HTMLElement>();
  let pillEl = $state<HTMLButtonElement>();
  let finishTimer: ReturnType<typeof setTimeout> | undefined;
  let toggleAnimated = $state(false);

  // Initial mount at phone width when guide is already open should show as pill.
  if (untrack(() => narrow.current && guideState.value === 'open')) collapseGuide();
  const announcer = untrack(() => createAnnouncer(steps, computers));

  let wasOpen = untrack(() => visible);
  let wasAllDone = untrack(() => allDone);
  let wasExpanded = untrack(() => visible && guideUi.expanded);
  let returnFocus = false;
  const overlap = untrack(() =>
    createOverlapTracker(
      { page: route.page, group: route.params.g, stepId: current?.id, doneCount },
      () => {
        returnFocus = true;
      },
    ),
  );

  $effect(() => {
    setBeaconHost(beaconLayer);
    return () => {
      setBeaconHost(undefined);
      clearBeacon();
    };
  });

  onDestroy(() => clearTimeout(finishTimer));

  $effect(() => {
    if (visible && !wasOpen) {
      announcer.reset(steps, computers);
      wasAllDone = allDone;
      toggleAnimated = false;
    }
    if (!visible) {
      returnFocus = false;
      clearTimeout(finishTimer);
      finishTimer = undefined;
    }
    wasOpen = visible;
  });

  $effect(() => {
    const done = announcer.next(visible, steps, computers);
    if (done) {
      announcement.text = t('guide.announce').replace('{step}', t(`guide.step.${done.id}`));
    }
    if (visible && allDone && !wasAllDone) {
      clearTimeout(finishTimer);
      if (narrow.current) {
        finishTimer = setTimeout(() => {
          if (visible && allDone) expandGuide();
        }, 1200);
      } else {
        expandGuide();
      }
    }
    wasAllDone = allDone;
  });

  $effect(() => {
    const openExpanded = visible && guideUi.expanded;
    if (openExpanded && !wasExpanded && guideUi.focusHeading) {
      returnFocus = false;
      guideUi.focusHeading = false;
      void tick().then(() => dockHeading?.focus());
    } else if (!openExpanded && wasExpanded && returnFocus) {
      returnFocus = false;
      guideUi.focusHeading = false;
      void tick().then(() => pillEl?.focus());
    } else if (openExpanded !== wasExpanded) {
      guideUi.focusHeading = false;
    }
    wasExpanded = openExpanded;
  });

  $effect(() =>
    overlap.sync({
      visible,
      narrow: narrow.current,
      dock,
      stepId: current?.id,
      page: route.page,
      group: route.params.g,
      doneCount,
    }),
  );

  const ringCirc = 2 * Math.PI * 11;
  const ringGap = 4;
  const ringDash = ringCirc / 5 - ringGap;
  const ringStep = ringCirc / 5;

  function refreshOnline() {
    for (const comp of computers) if (comp.online) void nodes.refresh(comp.home);
  }

  function act(step: GuideStep[number]) {
    const intent = stepIntent(step, steps[3].g);
    if (!intent) return;
    if (intent.kind === 'refresh') return refreshOnline();
    toggleAnimated = true;
    collapseGuide();
    if (intent.create) creating.open = true;
    location.hash = intent.hash;
    if (intent.selector) void beacon(intent.selector);
  }

  function collapse() {
    returnFocus = true;
    toggleAnimated = true;
    collapseGuide();
  }

  function expand() {
    toggleAnimated = true;
    expandGuide(true);
  }

  function onWindowKey(event: KeyboardEvent) {
    const canCollapse = visible && !narrow.current && guideUi.expanded;
    if (!canCollapseOnEscape(event, dock, document.activeElement, canCollapse)) return;
    event.preventDefault();
    collapse();
  }

  function dockOut(_node: Element) {
    if (prefersReducedMotion.current || guideState.value !== 'done') {
      return { duration: 0 };
    }
    return {
      duration: 140,
      css: (progress: number) =>
        `opacity: ${progress}; transform: scale(${0.96 + 0.04 * progress}); transform-origin: bottom right;`,
    };
  }

  const pillStepTitle = $derived(current ? t(`guide.step.${current.id}`) : t('guide.done.title'));
  const pillLabel = $derived(`${t('guide.expand')}: ${doneCount}/5 · ${pillStepTitle}`);
</script>

<svelte:window onkeydown={onWindowKey} />

<div class="guide-layer" aria-hidden="true" bind:this={beaconLayer}></div>

{#snippet body(headingId: string, inSheet: boolean)}
  {#if !inSheet}
    {#key allDone}
      <div class="horizon" aria-hidden="true"></div>
    {/key}
  {/if}
  <div class="dock-head">
    <h2 id={headingId} tabindex="-1" bind:this={dockHeading}>
      {t(allDone ? 'guide.done.title' : 'guide.title')}
    </h2>
    <span class="count">{t('guide.count').replace('{done}', String(doneCount))}</span>
    {#if !allDone && !inSheet}
      <button type="button" class="icon-btn" aria-label={t('guide.collapse')} onclick={collapse}>
        <Icon name="down" sm />
      </button>
    {/if}
    <button type="button" class="icon-btn" aria-label={t('guide.close')} onclick={closeGuide}>
      <Icon name="x" sm />
    </button>
  </div>
  <div
    class="segs"
    class:all={allDone}
    role="progressbar"
    aria-label={t('guide.title')}
    aria-valuemin={0}
    aria-valuemax={5}
    aria-valuenow={doneCount}
  >
    {#each steps as step (step.id)}
      <i class:done={step.done} class:cur={!allDone && current?.id === step.id}></i>
    {/each}
  </div>

  {#if allDone}
    <div class="finish">
      <p>{t('guide.done.body')}</p>
      <button type="button" class="btn btn-sm btn-primary" onclick={finishGuide}>
        {t('guide.done.close')}
      </button>
    </div>
  {:else}
    <ol class="steps">
      {#each steps as step, idx (step.id)}
        {@const isCurrent = current?.id === step.id}
        <li
          class="step"
          class:done={step.done}
          class:cur={isCurrent}
          aria-current={isCurrent ? 'step' : undefined}
        >
          <span class="mark" class:ok={step.done} class:cur={isCurrent} aria-hidden="true">
            {#if step.done}
              <Icon name="check" />
            {:else}
              {idx + 1}
            {/if}
          </span>
          <b>
            {#if step.done}<span class="sr">{t('guide.done')} </span>{/if}{t(
              `guide.step.${step.id}`,
            )}
          </b>
          <span class="right">{step.done ? stepDoneText(step) : ''}</span>
          {#if isCurrent}
            <div class="body">
              {#if step.id === 'ais' && step.offline}
                <p>{t('guide.step.ais.offline').replace('{name}', step.offline)}</p>
              {:else if step.id === 'ais' && step.none}
                <p>{t('guide.step.ais.none')}</p>
                <button type="button" class="btn btn-sm btn-primary" onclick={() => act(step)}>
                  {stepActionLabel(step)}
                </button>
              {:else}
                <p>{t(`guide.step.${step.id}.body`)}</p>
                {#if step.id === 'defaults' && step.left.length > 0}
                  <span class="left">
                    {t('guide.step.defaults.left').replace(
                      '{list}',
                      step.left.join(prefs.lang === 'zh-CN' ? '、' : ', '),
                    )}
                  </span>
                {/if}
                {#if step.id === 'group'}
                  <p class="tip">{t('guide.step.group.tip')}</p>
                {/if}
                <button type="button" class="btn btn-sm btn-primary" onclick={() => act(step)}>
                  {stepActionLabel(step)}
                </button>
              {/if}
            </div>
          {/if}
        </li>
      {/each}
    </ol>
  {/if}
{/snippet}

{#snippet pill(phone: boolean)}
  <button
    bind:this={pillEl}
    type="button"
    class="guide-pill"
    class:phone
    class:over-chat={!phone && route.page === 'chat'}
    class:over-full={phone && route.page === 'onboarding'}
    aria-label={pillLabel}
    onclick={expand}
  >
    <span class="ring" aria-hidden="true">
      <svg viewBox="0 0 26 26">
        <circle cx="13" cy="13" r="11" fill="none" stroke="var(--surface-3)" stroke-width="2.5" />
        {#each steps as step, i (step.id)}
          {#if step.done}
            <circle
              cx="13"
              cy="13"
              r="11"
              fill="none"
              stroke="var(--ok)"
              stroke-width="2.5"
              stroke-linecap="round"
              stroke-dasharray={`${ringDash.toFixed(2)} ${(ringCirc - ringDash).toFixed(2)}`}
              stroke-dashoffset={(-i * ringStep).toFixed(2)}
            />
          {/if}
        {/each}
      </svg>
      {#key doneCount}<span class="ring-num">{doneCount}</span>{/key}
    </span>
    <span class="t-wrap">
      {#key current?.id}
        <span class="t" out:fade={{ duration: prefersReducedMotion.current ? 0 : 140 }}>
          {pillStepTitle}
        </span>
      {/key}
    </span>
    <span class="chev" aria-hidden="true"><Icon name="down" sm /></span>
  </button>
{/snippet}

{#if visible}
  {#if !narrow.current}
    {#if guideUi.expanded}
      <aside
        bind:this={dock}
        class="guide-dock"
        class:toggle={toggleAnimated}
        aria-labelledby={`${uid}-dh`}
        out:dockOut
      >
        {@render body(`${uid}-dh`, false)}
      </aside>
    {:else}
      {@render pill(false)}
    {/if}
  {:else if !hidePill && !guideUi.expanded}
    {@render pill(true)}
  {/if}
{/if}

{#if visible && narrow.current && guideUi.expanded}
  <dialog
    use:modal
    class="dlg sheet guide-sheet"
    aria-labelledby={`${uid}-sh`}
    oncancel={(event) => {
      event.preventDefault();
      collapse();
    }}
    onclick={(event) => {
      if (event.target === event.currentTarget) collapse();
    }}
  >
    {@render body(`${uid}-sh`, true)}
  </dialog>
{/if}

{#if guideUi.closedNote && route.page !== 'join'}
  <p
    class="guide-toast"
    class:phone={narrow.current}
    class:over-full={narrow.current && route.page === 'onboarding'}
    role="status"
    out:fade={{ duration: prefersReducedMotion.current ? 0 : 140 }}
  >
    <Icon name="info" sm /><span>{t('guide.closed')}</span>
  </p>
{/if}

<style>
  .guide-layer {
    position: fixed;
    inset: 0;
    pointer-events: none;
    z-index: 4;
  }
  .guide-dock {
    position: fixed;
    right: 24px;
    bottom: 24px;
    z-index: 4;
    width: 344px;
    max-width: calc(100vw - 32px);
    max-height: calc(100dvh - 48px);
    overflow-y: auto;
    background: var(--surface);
    border-radius: var(--r-xl);
    box-shadow: var(--shadow-pop);
    animation: g-dock-in var(--dur-3) var(--ease-out) both;
  }
  .guide-dock.toggle {
    transform-origin: bottom right;
    animation: g-dock-toggle 160ms var(--ease-out) both;
  }
  .horizon {
    height: 2px;
    background: var(--horizon);
    transform-origin: left;
    animation: g-sweep var(--dur-4) var(--ease-out) both 80ms;
  }
  .dock-head {
    display: flex;
    align-items: center;
    gap: 6px;
    padding: 14px 10px 0 16px;
  }
  .dock-head h2 {
    font-size: var(--fs-lg);
    font-weight: 600;
    flex: 1;
  }
  .dock-head h2:focus:not(:focus-visible) {
    outline: none;
  }
  .count {
    font-size: var(--fs-xs);
    color: var(--text-3);
    font-variant-numeric: tabular-nums;
    margin-right: 4px;
    display: inline-block;
  }
  .dock-head .icon-btn {
    width: 30px;
    height: 30px;
    border-radius: 8px;
    color: var(--text-3);
  }
  .segs {
    display: grid;
    grid-template-columns: repeat(5, 1fr);
    gap: 4px;
    padding: 10px 16px 6px;
  }
  .segs i {
    height: 4px;
    border-radius: 2px;
    background: var(--surface-3);
    position: relative;
    overflow: hidden;
  }
  .segs i::after {
    content: '';
    position: absolute;
    inset: 0;
    border-radius: inherit;
    transform-origin: left;
    transform: scaleX(0);
  }
  .segs i.done::after {
    background: var(--ok);
    transform: scaleX(1);
    animation: g-fill var(--dur-3) var(--ease-out) both;
  }
  .segs i.cur::after {
    background: var(--sky);
    transform: scaleX(1);
    animation: g-fill var(--dur-3) var(--ease-out) both;
  }
  .segs.all i::after {
    background: var(--ok);
    transform: scaleX(1);
    animation: g-fill var(--dur-3) var(--ease-out) both;
  }
  .segs.all i:nth-child(2)::after {
    animation-delay: 30ms;
  }
  .segs.all i:nth-child(3)::after {
    animation-delay: 60ms;
  }
  .segs.all i:nth-child(4)::after {
    animation-delay: 90ms;
  }
  .segs.all i:nth-child(5)::after {
    animation-delay: 120ms;
  }
  .steps {
    padding: 4px 8px 12px;
  }
  .step {
    display: grid;
    grid-template-columns: 20px minmax(0, 1fr) auto;
    gap: 4px 10px;
    padding: 8px;
    border-radius: var(--r-md);
    align-items: start;
  }
  .step b {
    font-weight: 500;
    font-size: var(--fs-ui);
    color: var(--text-3);
    line-height: 20px;
  }
  .step.done b {
    color: var(--text-2);
  }
  .step.cur {
    background: var(--surface-2);
  }
  .step.cur b {
    color: var(--text);
    font-weight: 600;
  }
  .step .right {
    font-size: var(--fs-xs);
    color: var(--text-3);
    line-height: 20px;
    max-width: 150px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .mark {
    width: 20px;
    height: 20px;
    border-radius: 50%;
    display: grid;
    place-items: center;
    font-size: var(--fs-xs);
    font-weight: 500;
    background: var(--surface-3);
    color: var(--text-3);
    font-variant-numeric: tabular-nums;
  }
  .mark.cur {
    background: none;
    box-shadow: inset 0 0 0 1.5px var(--sky);
    color: var(--sky);
  }
  .mark.ok {
    background: none;
    color: var(--ok);
    animation: g-land var(--dur-4) var(--spring) both;
  }
  .body {
    grid-column: 2 / 4;
    display: grid;
    gap: 8px;
    animation: g-body var(--dur-2) var(--ease-out) both 60ms;
  }
  .body p {
    font-size: var(--fs-sm);
    color: var(--text-2);
    line-height: 1.55;
  }
  .body .left {
    font-size: var(--fs-xs);
    color: var(--text-2);
    font-weight: 500;
  }
  .body .tip {
    font-size: var(--fs-xs);
    color: var(--text-3);
  }
  .body .btn {
    justify-self: start;
  }
  .finish {
    padding: 4px 16px 16px;
    display: grid;
    gap: 12px;
  }
  .finish p {
    font-size: var(--fs-sm);
    color: var(--text-2);
  }
  .finish .btn {
    justify-self: end;
  }
  .guide-pill {
    position: fixed;
    right: 24px;
    bottom: 24px;
    z-index: 4;
    display: inline-flex;
    align-items: center;
    gap: 10px;
    height: 40px;
    padding: 0 12px 0 8px;
    border: 0;
    border-radius: var(--r-full);
    background: var(--raised);
    box-shadow: var(--shadow-pop);
    color: var(--text);
    cursor: pointer;
    max-width: min(340px, calc(100vw - 32px));
    transform-origin: bottom right;
    animation: g-dock-toggle 160ms var(--ease-out) both;
  }
  .guide-pill.phone {
    right: 16px;
    bottom: calc(var(--tab-h) + 12px + env(safe-area-inset-bottom));
    max-width: calc(100vw - 32px);
  }
  .guide-pill.phone.over-full {
    bottom: calc(16px + env(safe-area-inset-bottom));
  }
  @media (min-width: 768px) and (max-width: 1279px) {
    .guide-pill.over-chat {
      bottom: 124px;
    }
  }
  @media (pointer: coarse) {
    .guide-pill {
      height: 44px;
    }
  }
  .ring {
    position: relative;
    width: 26px;
    height: 26px;
    flex: none;
  }
  .ring svg {
    width: 26px;
    height: 26px;
    transform: rotate(-90deg);
  }
  .ring-num {
    position: absolute;
    inset: 0;
    display: grid;
    place-items: center;
    font-size: 11px;
    font-weight: 600;
    font-variant-numeric: tabular-nums;
    animation: g-land var(--dur-4) var(--spring) both;
  }
  .t-wrap {
    display: grid;
    min-width: 0;
  }
  .guide-pill .t {
    grid-area: 1 / 1;
    font-size: var(--fs-sm);
    font-weight: 500;
    max-width: 260px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    animation: g-body var(--dur-2) var(--ease-out) both;
  }
  .guide-pill.phone .t {
    max-width: 220px;
  }
  .guide-pill .chev {
    display: grid;
    place-items: center;
    color: var(--text-3);
    transform: rotate(-90deg);
  }
  .guide-sheet {
    padding: 8px 0 calc(var(--sp-5) + env(safe-area-inset-bottom));
  }
  .guide-sheet .dock-head {
    padding-top: 16px;
  }
  @media (max-width: 767px) {
    .guide-sheet .body .btn,
    .guide-sheet .finish .btn {
      min-height: 44px;
    }
  }
  .guide-toast {
    position: fixed;
    right: 24px;
    bottom: 24px;
    z-index: 4;
    display: inline-flex;
    align-items: center;
    gap: 8px;
    padding: 10px 14px;
    border-radius: 12px;
    background: var(--raised);
    box-shadow: var(--shadow-pop);
    font-size: var(--fs-sm);
    color: var(--text-2);
    max-width: min(340px, calc(100vw - 32px));
    pointer-events: none;
    animation: g-dock-in var(--dur-3) var(--ease-out) both;
  }
  .guide-toast.phone {
    right: 16px;
    bottom: calc(var(--tab-h) + 12px + env(safe-area-inset-bottom));
  }
  .guide-toast.phone.over-full {
    bottom: calc(16px + env(safe-area-inset-bottom));
  }

  :global(.guide-beacon) {
    position: fixed;
    z-index: 4;
    pointer-events: none;
    transition: opacity var(--dur-out) var(--ease-in);
  }
  :global(.guide-beacon.leaving) {
    opacity: 0;
  }
  :global(.guide-beacon-ring),
  :global(.guide-beacon-pulse) {
    position: absolute;
    inset: 0;
    border-radius: inherit;
    border: 2px solid var(--sky);
  }
  :global(.guide-beacon:not([data-reduced]) .guide-beacon-ring) {
    animation: -global-guide-beacon-in var(--dur-3) var(--ease-out) both;
  }
  :global(.guide-beacon:not([data-reduced]) .guide-beacon-pulse) {
    opacity: 0;
    animation: -global-guide-beacon-pulse 900ms var(--ease-out) 2 320ms;
  }
  @keyframes -global-guide-beacon-in {
    from {
      opacity: 0;
      transform: scale(1.04);
    }
  }
  @keyframes -global-guide-beacon-pulse {
    0% {
      opacity: 0.5;
      transform: scale(1);
    }
    100% {
      opacity: 0;
      transform: scale(1.12);
    }
  }
  @keyframes g-dock-in {
    from {
      opacity: 0;
      transform: translateY(var(--travel));
    }
  }
  @keyframes g-dock-toggle {
    from {
      opacity: 0;
      transform: scale(0.96);
    }
  }
  @keyframes g-sweep {
    from {
      transform: scaleX(0);
    }
  }
  @keyframes g-fill {
    from {
      transform: scaleX(0);
    }
  }
  @keyframes g-land {
    from {
      opacity: 0;
      transform: scale(0.6);
    }
  }
  @keyframes g-body {
    from {
      opacity: 0;
      transform: translateY(var(--rise));
    }
  }
</style>
