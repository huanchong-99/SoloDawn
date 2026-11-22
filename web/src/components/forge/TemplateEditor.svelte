<script lang="ts">
  import { t } from '../../lib/prefs.svelte';
  import { operations, type Templates } from './templates';

  let { templates = $bindable() }: { templates: Templates } = $props();
</script>

<p class="hint">{t('forge.arguments.hint')}</p>
<p class="hint">{t('forge.places')}</p>
{#each operations as operation (operation)}
  <fieldset class="field">
    <legend>{t(`forge.${operation}`)}</legend>
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
          class="btn btn-sm"
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
      class="btn btn-sm"
      onclick={() => (templates[operation] = [...templates[operation], ''])}
    >
      {t('forge.add').replace('{operation}', operation)}
    </button>
  </fieldset>
{/each}
