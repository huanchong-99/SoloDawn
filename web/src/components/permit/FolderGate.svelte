<script lang="ts">
  import { t } from '../../lib/prefs.svelte';
  import Icon from '../Icon.svelte';
  import AlertSketch from './AlertSketch.svelte';
  import type { GateView } from './permit.svelte.ts';

  let {
    state: view,
    computer,
    onsettings,
    onall,
    onretry,
  }: {
    state: GateView;
    computer: string;
    onsettings: () => void;
    onall: () => void;
    onretry: () => void;
  } = $props();
</script>

{#if view.state === 'asking'}
  <div class="gate" role="status">
    <AlertSketch />
    <p>{t('permit.folder.wait').replace('{name}', () => computer)}</p>
  </div>
{:else if view.state === 'refused'}
  <div class="gate no" role="alert">
    <span class="ico"><Icon name="alert" sm /></span>
    <div class="body">
      <p>
        {view.settings
          ? t('permit.folder.settings').replace('{name}', () => computer)
          : t('permit.folder.no')}
      </p>
      <div class="acts">
        <button type="button" class="btn btn-sm" onclick={onsettings}>
          {t('permit.folder.open')}
        </button>
        <button type="button" class="btn btn-sm" onclick={onall}>
          {t('permit.files.all')}
        </button>
        <button type="button" class="btn btn-sm btn-ghost" onclick={onretry}>
          {t('permit.folder.retry')}
        </button>
      </div>
    </div>
  </div>
{/if}

<style>
  .gate {
    background: light-dark(var(--surface-2), var(--surface-3));
    border-radius: var(--r-md);
    padding: 12px;
    display: grid;
    grid-template-columns: 120px minmax(0, 1fr);
    gap: 12px;
    align-items: center;
  }
  @media (prefers-reduced-motion: no-preference) {
    .gate {
      animation: gate-appear var(--dur-2) var(--ease-out) both;
    }
  }
  .gate.no {
    grid-template-columns: 16px minmax(0, 1fr);
    align-items: start;
  }
  .ico {
    display: inline-flex;
    color: var(--danger);
    margin-top: 2px;
  }
  .body {
    min-width: 0;
  }
  p {
    font-size: var(--fs-sm);
    line-height: 1.55;
  }
  .acts {
    display: flex;
    flex-wrap: wrap;
    gap: 6px;
    margin-top: 8px;
  }
  @media (max-width: 520px) {
    .gate:not(.no) {
      grid-template-columns: minmax(0, 1fr);
      justify-items: start;
    }
    .acts button {
      min-height: 44px;
      height: auto;
      white-space: normal;
      padding-block: 6px;
    }
  }
  @keyframes gate-appear {
    from {
      opacity: 0;
      transform: translateY(var(--rise));
    }
  }
</style>
