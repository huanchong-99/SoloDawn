<script lang="ts">
  import { guideState, isBrandNew, closeGuide, modal, openGuide } from '../../lib/guide.svelte';
  import { recent } from '../../lib/groups.svelte';
  import type { Net } from '../../lib/net.svelte';
  import { nodes } from '../../lib/nodes.svelte';
  import { otherLang, prefs, t, textIn, toggleLang } from '../../lib/prefs.svelte';
  import { route } from '../../lib/route.svelte';

  const stepIds = ['connect', 'ais', 'defaults', 'group', 'ask'] as const;

  /** First-run welcome modal shown once on `#/groups` for a brand-new browser. */
  let { net }: { net: Net } = $props();
  const uid = $props.id();

  let brandNew = $state(false);

  $effect(() => {
    if (route.page !== 'groups' || guideState.value !== 'new') {
      brandNew = false;
      return;
    }
    let active = true;
    void isBrandNew().then((fresh) => {
      if (active) brandNew = fresh;
    });
    return () => {
      active = false;
    };
  });

  const shown = $derived(
    guideState.value === 'new' &&
      route.page === 'groups' &&
      brandNew &&
      nodes.list(net).length === 0 &&
      recent().length === 0,
  );
</script>

{#if shown}
  <dialog
    use:modal
    class="dlg welcome"
    aria-labelledby={`${uid}-h`}
    oncancel={(event) => {
      event.preventDefault();
      closeGuide();
    }}
  >
    <div class="horizon" aria-hidden="true"></div>
    <div class="in">
      <span class="w-logo" aria-hidden="true"><i></i></span>
      <div>
        <h2 id={`${uid}-h`}>{t('guide.welcome.title')}</h2>
        <p>{t('guide.welcome.body')}</p>
      </div>
      <ol class="wl">
        {#each stepIds as id (id)}
          <li>{t(`guide.step.${id}`)}</li>
        {/each}
      </ol>
      <footer class="acts">
        <button type="button" class="btn btn-primary" onclick={openGuide}>
          {t('guide.welcome.start')}
        </button>
        <button type="button" class="btn btn-ghost" onclick={closeGuide}>
          {t('guide.welcome.later')}
        </button>
        <button type="button" class="btn btn-ghost" onclick={toggleLang}>
          {#key prefs.lang}<span class="swap" lang={otherLang()}>{t('bar.lang.other')}</span>{/key}
          <span class="sr">
            <span lang="en">{textIn('en', 'bar.lang')}</span> /
            <span lang="zh-CN">{textIn('zh-CN', 'bar.lang')}</span>
          </span>
        </button>
      </footer>
    </div>
  </dialog>
{/if}

<style>
  .welcome {
    padding: 0;
    overflow-x: hidden;
    overflow-y: auto;
  }
  .horizon {
    height: 2px;
    background: var(--horizon);
    transform-origin: left;
    animation: w-sweep var(--dur-4) var(--ease-out) both 80ms;
  }
  .in {
    padding: var(--sp-6);
    display: grid;
    gap: var(--sp-4);
  }
  .w-logo {
    width: 40px;
    height: 40px;
    border-radius: 12px;
    display: grid;
    place-items: center;
    background: var(--surface-2);
  }
  .w-logo i {
    width: 22px;
    height: 3px;
    border-radius: 2px;
    background: var(--horizon);
    display: block;
  }
  h2 {
    font-size: var(--fs-xl);
    font-weight: 600;
    line-height: 1.3;
    margin-bottom: 4px;
  }
  p {
    color: var(--text-2);
    font-size: var(--fs-ui);
  }
  .wl {
    display: grid;
    gap: 10px;
    counter-reset: n;
  }
  .wl li {
    counter-increment: n;
    display: grid;
    grid-template-columns: 24px minmax(0, 1fr);
    gap: 12px;
    align-items: center;
    color: var(--text-2);
    font-size: var(--fs-ui);
    font-weight: 400;
    animation: w-appear var(--dur-2) var(--ease-out) both;
  }
  .wl li::before {
    content: counter(n);
    width: 24px;
    height: 24px;
    border-radius: 50%;
    background: var(--surface-3);
    display: grid;
    place-items: center;
    font-size: var(--fs-xs);
    font-weight: 500;
  }
  .wl li:nth-child(1) {
    animation-delay: 120ms;
  }
  .wl li:nth-child(2) {
    animation-delay: calc(120ms + var(--stagger));
  }
  .wl li:nth-child(3) {
    animation-delay: calc(120ms + 2 * var(--stagger));
  }
  .wl li:nth-child(4) {
    animation-delay: calc(120ms + 3 * var(--stagger));
  }
  .wl li:nth-child(5) {
    animation-delay: calc(120ms + 4 * var(--stagger));
  }
  .acts {
    display: flex;
    gap: var(--sp-2);
    align-items: center;
    flex-wrap: wrap;
  }
  .acts .btn-primary {
    min-width: 160px;
  }
  @media (max-width: 767px) {
    .in {
      padding: var(--sp-6) var(--sp-4) calc(var(--sp-5) + env(safe-area-inset-bottom));
    }
    .acts {
      flex-direction: column;
      align-items: stretch;
    }
    .acts .btn-primary {
      width: 100%;
    }
  }
  @keyframes w-sweep {
    from {
      transform: scaleX(0);
    }
  }
  @keyframes w-appear {
    from {
      opacity: 0;
      transform: translateY(var(--rise));
    }
  }
</style>
