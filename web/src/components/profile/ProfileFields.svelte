<script lang="ts">
  import { t } from '../../lib/prefs.svelte';
  import type { AIMemberRow } from '../../lib/proto';
  import Bars from './Bars.svelte';
  import Quota from './Quota.svelte';
  import { tiers, tooLong, type Draft } from './profile';

  /** Tier and skills of a draft profile, for the add-AI and create-group dialogs and the owner's
   *  editor; with `ai`, its usage line under the tier; without `skills`, the tier alone; without
   *  `share` (a secretary, which takes no tasks), no tier: the draft keeps its value. */
  let {
    draft = $bindable(),
    ai,
    skills = true,
    share = true,
    disabled = false,
  }: {
    draft: Draft;
    ai?: AIMemberRow;
    skills?: boolean;
    share?: boolean;
    disabled?: boolean;
  } = $props();
  const uid = $props.id();
</script>

{#if share}
  <fieldset class="field work" {disabled} aria-describedby={`${uid}-tier-hint`}>
    <legend>{t('profile.tier')}</legend>
    <p id={`${uid}-tier-hint`} class="tier-hint">{t('profile.tier.hint')}</p>
    <div class="seg tiers" style:--at={(tiers as readonly string[]).indexOf(draft.tier)}>
      {#each tiers as tier (tier)}
        <label>
          <input type="radio" name={`${uid}-tier`} value={tier} bind:group={draft.tier} />
          <span><Bars {tier} />{t(`profile.tier.${tier}`)}</span>
        </label>
      {/each}
    </div>
    {#if ai}<Quota {ai} />{/if}
  </fieldset>
{/if}
{#if skills}
  <label class="field">
    <span class="row">
      <span class="grow">{t('profile.skills')}</span>
      <small class="count" class:over={tooLong(draft)} aria-hidden="true">
        {[...draft.skills].length}/200
      </small>
    </span>
    <input
      class="input"
      bind:value={draft.skills}
      aria-invalid={tooLong(draft)}
      placeholder={t('profile.skills.hint')}
      {disabled}
    />
  </label>
{/if}

<style>
  fieldset {
    margin: 0;
    padding: 0;
    border: 0;
  }
  legend {
    padding: 0;
    margin-bottom: 2px;
    font-weight: 500;
  }
  .tier-hint {
    margin-bottom: 6px;
    color: var(--text-3);
    font-size: var(--fs-xs);
    line-height: 1.45;
  }
  .work :global(.meter),
  .work :global(.quota-none) {
    margin-top: var(--sp-2);
  }
  .tiers {
    display: grid;
    width: 100%;
  }
  .tiers label {
    padding: 0 8px;
  }
  .tiers span {
    display: inline-flex;
    align-items: center;
    gap: 6px;
  }
  .tiers label:has(:checked) {
    color: var(--sky);
  }
  .count {
    color: var(--text-3);
    font-size: var(--fs-xs);
    font-variant-numeric: tabular-nums;
    font-weight: 400;
  }
  .count.over {
    color: var(--dawn-text);
  }
</style>
