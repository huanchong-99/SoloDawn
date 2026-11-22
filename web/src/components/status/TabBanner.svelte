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
    <button type="button" class="btn btn-sm btn-dawn" onclick={net.useHere}>
      {t('app.useHere')}
    </button>
  </div>
{/if}

<style>
  .tab-ban {
    display: flex;
    align-items: center;
    gap: var(--sp-3);
    padding: 8px var(--sp-4);
    background: var(--dawn-soft);
    border-bottom: 1px solid color-mix(in srgb, var(--dawn) 35%, var(--line));
    font-size: var(--fs-sm);
  }
</style>
