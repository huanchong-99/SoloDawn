<script lang="ts">
  import { t } from '../../lib/prefs.svelte';
  import Bars from './Bars.svelte';
  import { tiers, tooLong, type Draft } from './profile';

  /** Tier and skills of a draft profile, for the add-AI dialog and the owner's editor. */
  let { draft = $bindable(), disabled = false }: { draft: Draft; disabled?: boolean } = $props();
  const uid = $props.id();
</script>

<fieldset class="field" {disabled}>
  <legend>{t('profile.tier')}</legend>
  <div class="seg tiers" style:--at={(tiers as readonly string[]).indexOf(draft.tier)}>
    {#each tiers as tier (tier)}
      <label>
        <input type="radio" name={`${uid}-tier`} value={tier} bind:group={draft.tier} />
        <span><Bars {tier} />{t(`profile.tier.${tier}`)}</span>
      </label>
    {/each}
  </div>
</fieldset>
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

<style>
  fieldset {
    margin: 0;
    padding: 0;
    border: 0;
  }
  legend {
    padding: 0;
    margin-bottom: 6px;
    font-weight: 500;
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
