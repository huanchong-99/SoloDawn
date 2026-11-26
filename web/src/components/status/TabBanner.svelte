<script lang="ts">
  import type { Net } from '../../lib/net.svelte';
  import { t } from '../../lib/prefs.svelte';
  import Icon from '../Icon.svelte';

  /** Notice under the top bar while another tab of this browser holds the device's connection lock
   *  (planning/flows/connect.md section F; step-1-acceptance.md A1.12). */
  let { net }: { net: Net } = $props();
</script>

{#if net.otherTab}
  <div class="tab-ban" role="status">
    <Icon name="info" />
    <span class="grow">{t('app.otherTab')}</span>
    <button
      type="button"
      class="btn btn-sm btn-dawn"
      onclick={() => {
        net.useHere!();
        document.querySelector('main')!.focus();
      }}
    >
      {t('app.useHere')}
    </button>
  </div>
{/if}

<style>
  .tab-ban {
    display: flex;
    align-items: center;
    gap: var(--sp-3);
    min-height: 44px;
    padding: 6px var(--sp-4) 6px var(--sp-5);
    border-bottom: 1px solid var(--line);
    background: var(--surface-2);
    font-size: var(--fs-sm);
  }
  .tab-ban :global(.i) {
    width: 16px;
    height: 16px;
    color: var(--dawn-text);
  }
</style>
