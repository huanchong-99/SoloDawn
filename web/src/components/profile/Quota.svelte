<script lang="ts">
  import { usageMeter } from '../../lib/members';
  import { prefs, t } from '../../lib/prefs.svelte';
  import type { AIMemberRow } from '../../lib/proto';

  /** The AI's usage line: its share of the window and when it resets, the turns of today, or that
   *  nothing was reported. */
  let { ai }: { ai: AIMemberRow } = $props();
  const meter = $derived(usageMeter(ai, prefs.lang));
</script>

{#if meter?.fill !== undefined}
  <div
    class="meter"
    role="meter"
    aria-label={meter.label}
    aria-valuenow={meter.fill}
    aria-valuetext={meter.note ? `${meter.value} · ${meter.note}` : meter.value}
    data-tone={meter.tone}
    style={`--v: ${meter.fill}%`}
  >
    <span>{meter.label}</span><b>{meter.value}</b><i></i>{#if meter.note}<small>
        {meter.note}
      </small>{/if}
  </div>
{:else if meter}
  <p class="meter"><span>{meter.label}</span><b>{meter.value}</b></p>
{:else}
  <p class="quota-none">{t('member.usage.none')}</p>
{/if}

<style>
  .quota-none {
    color: var(--text-3);
    font-size: var(--fs-xs);
  }
</style>
