<script lang="ts">
  import { t } from '../../lib/prefs.svelte';
  import type { Draft } from '../../lib/members';
  import type { Listed } from './model';

  /** Model and level of a draft: chosen from what the CLI lists, or typed when it lists nothing
   *  (contracts/step-3/profiles.md section 2a). The level is always shown, offering the chosen
   *  model's levels, and a stored level that is not among them. With `suggest`, choosing a model
   *  sets its tier. */
  let {
    draft = $bindable(),
    models,
    suggest = false,
    disabled = false,
  }: { draft: Draft; models: Listed[]; suggest?: boolean; disabled?: boolean } = $props();
  const uid = $props.id();

  const chosen = $derived(models.find((model) => model.model === draft.model));
  const efforts = $derived(chosen?.efforts ?? []);
  const stray = $derived(draft.effort && !efforts.includes(draft.effort) ? draft.effort : '');
  const why = $derived(chosen ? 'model.effort.none' : draft.model ? '' : 'model.effort.pick');

  function pick() {
    if (!chosen?.efforts.includes(draft.effort)) draft.effort = '';
    if (suggest) draft.tier = chosen?.tier ?? 'standard';
  }
</script>

{#if models.length}
  <div class="pair">
    <label class="field">
      <span class="row">
        <span id={`${uid}-model`} class="grow">{t('model.model')}</span>
        {#if chosen?.fast}<span class="badge fast">{t('model.fast')}</span>{/if}
      </span>
      <select
        class="input"
        aria-labelledby={`${uid}-model`}
        bind:value={draft.model}
        onchange={pick}
        {disabled}
      >
        <option value="">{t('model.default')}</option>
        {#each models as model (model.model)}
          <option value={model.model}>{model.name}</option>
        {/each}
        {#if draft.model && !chosen}
          <option value={draft.model}>{draft.model} · {t('model.notlisted')}</option>
        {/if}
      </select>
    </label>
    <label class="field">
      <span>{t('model.effort')}</span>
      <select
        class="input"
        bind:value={draft.effort}
        disabled={disabled || (!efforts.length && !stray)}
        aria-describedby={why && !efforts.length ? `${uid}-why` : undefined}
      >
        <option value="">{t('model.default')}</option>
        {#each efforts as effort (effort)}<option value={effort}>{effort}</option>{/each}
        {#if stray}<option value={stray}>{stray} · {t('model.notlisted')}</option>{/if}
      </select>
      {#if why && !efforts.length}
        <small id={`${uid}-why`} class="why">{t(why)}</small>
      {/if}
    </label>
  </div>
{:else}
  <div class="pair">
    <label class="field">
      <span>{t('model.model')}</span>
      <input
        class="input mono"
        bind:value={draft.model}
        pattern={'.{0,100}'}
        placeholder={t('model.model.hint')}
        {disabled}
      />
    </label>
    <label class="field">
      <span>{t('model.effort')}</span>
      <input
        class="input mono"
        bind:value={draft.effort}
        pattern={'.{0,32}'}
        placeholder={t('model.effort.hint')}
        {disabled}
      />
    </label>
  </div>
{/if}

<style>
  .pair {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(150px, 1fr));
    gap: var(--sp-3);
    align-items: start;
    animation: -global-appear var(--dur-2) var(--ease-out);
  }
  .why {
    color: var(--text-3);
    font-size: var(--fs-xs);
  }
  .fast {
    height: 18px;
    font-size: var(--fs-xs);
    animation: -global-appear var(--dur-2) var(--ease-out);
  }
</style>
