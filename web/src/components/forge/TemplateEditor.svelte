<script lang="ts">
  import { t } from '../../lib/prefs.svelte';
  import { operations, type Templates } from './templates';

  let { templates = $bindable() }: { templates: Templates } = $props();
</script>

<p class="hint">{t('forge.arguments.hint')}</p>
<p class="hint">{t('forge.places')}</p>
{#each operations as operation (operation)}
  <fieldset class="field">
    <legend class="sec-title">{t(`forge.${operation}`)}</legend>
    {#each templates[operation] as _, i (i)}
      <div class="row">
        <label class="field grow">
          <span>{t(i === 0 ? 'forge.program' : 'forge.argument').replace('{n}', String(i))}</span>
          <textarea
            class="input mono"
            rows="1"
            required={i === 0}
            bind:value={templates[operation][i]}
            autocapitalize="off"
            spellcheck="false"></textarea>
        </label>
        <button
          type="button"
          class="btn btn-sm btn-ghost"
          disabled={templates[operation].length === 1}
          aria-label={t('forge.remove').replace('{n}', String(i)).replace('{operation}', operation)}
          onclick={() => (templates[operation] = templates[operation].filter((_, at) => at !== i))}
        >
          {t('forge.remove.short')}
        </button>
      </div>
    {/each}
    <button
      type="button"
      class="btn btn-sm btn-ghost"
      onclick={() => (templates[operation] = [...templates[operation], ''])}
    >
      {t('forge.add')}
    </button>
  </fieldset>
{/each}

<style>
  .row {
    align-items: flex-end;
  }
  fieldset > button {
    justify-self: start;
  }
</style>
