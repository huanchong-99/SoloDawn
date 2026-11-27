<script lang="ts">
  import { homes, type Group } from '../../lib/groups.svelte';
  import { nodes } from '../../lib/nodes.svelte';
  import { t } from '../../lib/prefs.svelte';
  import type { AIMemberRow } from '../../lib/proto';
  import { hostHomeOf } from '../../lib/terminal.svelte';
  import Icon from '../Icon.svelte';
  import { listedModels } from './model';

  /** Model and level of an AI member card, and the refresh of its computer's list; its owner
   *  changes both in the profile editor (contracts/step-3/profiles.md section 2a). */
  let { group, ai }: { group: Group; ai: AIMemberRow } = $props();

  let refreshing = $state(false);

  const host = $derived(hostHomeOf(homes, ai.host));
  const models = $derived(listedModels(host, ai.cli));
  const listed = $derived(models?.find((model) => model.model === ai.model));
  const mine = $derived(ai.user === group.home.me.user);
  const emptyValue = $derived(t(ai.cli === 'external' ? 'model.unset' : 'model.default'));

  async function refresh() {
    refreshing = true;
    await nodes.refresh(host!);
    refreshing = false;
  }
</script>

<dt>{t('model.model')}</dt>
<dd>
  <span class="what">
    {#if ai.model}
      <span class="name">{listed?.name ?? ai.model}</span>
      {#if ai.cli !== 'external' && models && !listed}<span class="badge" data-tone="dawn">
          {t('model.notlisted')}
        </span>{/if}
    {:else}
      <span class="muted">{emptyValue}</span>
    {/if}
  </span>
  {#if mine && ai.cli !== 'external' && models}
    <button
      type="button"
      class="icon-btn spin"
      class:on={refreshing}
      aria-label={t('model.refresh')}
      title={t('model.refresh')}
      disabled={refreshing || !host?.ctl}
      onclick={refresh}
    >
      <Icon name="refresh" sm />
    </button>
  {/if}
</dd>
<dt>{t('model.effort')}</dt>
<dd class:muted={!ai.effort}>{ai.effort || emptyValue}</dd>

<style>
  .what {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 4px 6px;
  }
  /* The value, and beside it the refresh of its computer's list. */
  dd:has(> .what) {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 2px var(--sp-2);
  }

  .name {
    overflow-wrap: anywhere;
  }
  .spin {
    margin-left: -4px;
    width: 24px;
    height: 22px;
    min-height: 0;
    min-width: 0;
    color: var(--text-3);
  }
  .spin :global(svg) {
    transition: transform var(--dur-3) var(--ease-out);
  }
  .spin:hover:not(:disabled) :global(svg) {
    transform: rotate(90deg);
  }
  .spin.on :global(svg) {
    animation: spin 0.9s linear infinite;
  }
  .spin:disabled {
    cursor: default;
  }

  @keyframes spin {
    to {
      transform: rotate(360deg);
    }
  }
  @media (prefers-reduced-motion: reduce) {
    .spin.on :global(svg) {
      animation: none;
    }
  }
</style>
