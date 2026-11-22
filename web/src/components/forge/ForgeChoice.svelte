<script lang="ts">
  import { t } from '../../lib/prefs.svelte';
  import TemplateEditor from './TemplateEditor.svelte';
  import type { Templates } from './templates';

  let {
    forge = $bindable(),
    templates = $bindable(),
  }: {
    forge: string;
    templates: Templates;
  } = $props();
  const uid = $props.id();
</script>

<label class="field">
  <span id={`${uid}-forge`}>{t('groups.create.forge')}</span>
  <select class="input" aria-labelledby={`${uid}-forge`} bind:value={forge}>
    <option value="github">{t('groups.create.forge.github')}</option>
    <option value="none">{t('groups.create.forge.none')}</option>
    <option value="custom">{t('forge.custom')}</option>
  </select>
</label>
{#if forge === 'custom'}<TemplateEditor bind:templates />{/if}
