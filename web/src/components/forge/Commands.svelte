<script lang="ts">
  import { t } from '../../lib/prefs.svelte';
  import type { GroupRow } from '../../lib/proto';
  import { operations } from './templates';

  let { info }: { info: GroupRow } = $props();
</script>

<section aria-label={t('forge.commands')}>
  <dl class="kv">
    <dt>{t('groups.create.repo')}</dt>
    <dd><code>{info.repo ?? t('forge.none')}</code></dd>
    <dt>{t('groups.create.base')}</dt>
    <dd><code>{info.base}</code></dd>
    <dt>{t('forge.label')}</dt>
    <dd>
      {t(
        info.forge === 'github'
          ? 'groups.create.forge.github'
          : info.forge === 'none'
            ? 'groups.create.forge.none'
            : 'forge.custom',
      )}
    </dd>
  </dl>
  <h3 class="sec-title">{t('groups.create.checks')}</h3>
  {#each info.checks as check, i (i)}
    <p><code>{check}</code></p>
  {:else}<p class="hint">{t('forge.none')}</p>{/each}
  {#if info.forge === 'custom' && info.templates}
    {#each operations as operation (operation)}
      <div class="field">
        <h3 class="sec-title">{t(`forge.${operation}`)}</h3>
        <div class="arguments">
          {#each info.templates[operation] as argument, i (i)}
            <code>{argument || '""'}</code>
          {/each}
        </div>
      </div>
    {/each}
    <p class="hint">{t('forge.arguments.shown')}</p>
  {/if}
  <p class="note" data-tone="dawn">{t('forge.warning')}</p>
</section>

<style>
  section {
    display: flex;
    flex-direction: column;
    gap: var(--sp-3);
  }
  code {
    white-space: pre-wrap;
    overflow-wrap: anywhere;
  }
  .arguments {
    display: flex;
    flex-wrap: wrap;
    gap: 4px;
  }
  .arguments code {
    padding: 1px 6px;
    border: 1px solid var(--line);
    border-radius: var(--r-sm);
    background: var(--surface-2);
  }
</style>
