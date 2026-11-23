<script lang="ts">
  import { tick } from 'svelte';
  import { homes, type Group } from '../../lib/groups.svelte';
  import { nodes } from '../../lib/nodes.svelte';
  import { t } from '../../lib/prefs.svelte';
  import type { AIMemberRow } from '../../lib/proto';
  import { hostHomeOf } from '../../lib/terminal.svelte';
  import Failure from '../Failure.svelte';
  import Icon from '../Icon.svelte';
  import { newDraft } from '../profile/profile';
  import ModelFields from './ModelFields.svelte';
  import { listedModels, modelChanges } from './model';

  /** Model and level of an AI member card, the refresh of its computer's list, and the owner's
   *  editor for both (contracts/step-3/profiles.md section 2a). */
  let { group, ai }: { group: Group; ai: AIMemberRow } = $props();

  let editing = $state(false);
  let draft = $state(newDraft());
  let editButton = $state<HTMLButtonElement>();
  let form = $state<HTMLFormElement>();
  let busy = $state(false);
  let refreshing = $state(false);
  let failed = $state('');

  const host = $derived(hostHomeOf(homes, ai.host));
  const models = $derived(listedModels(host, ai.cli));
  const listed = $derived(models?.find((model) => model.model === ai.model));
  const mine = $derived(ai.user === group.home.me.user);
  const offline = $derived(!group.home.ctl);
  const changes = $derived(modelChanges(ai, draft));

  async function refresh() {
    refreshing = true;
    await nodes.refresh(host!);
    refreshing = false;
  }

  async function edit() {
    draft = { ...newDraft(), model: ai.model, effort: ai.effort };
    failed = '';
    editing = true;
    await tick();
    form?.focus();
  }

  async function close() {
    editing = false;
    failed = '';
    await tick();
    editButton?.focus();
  }

  async function save(event: SubmitEvent) {
    event.preventDefault();
    busy = true;
    failed = '';
    const answer = await group.home.command({
      t: 'member',
      g: group.g,
      op: 'set',
      id: ai.id,
      ...changes,
    });
    busy = false;
    if (answer.err) failed = answer.err.code;
    else await close();
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
      <span class="muted">{t('model.default')}</span>
    {/if}
  </span>
  {#if mine && ai.cli !== 'external' && (models || !editing)}
    <span class="acts">
      {#if !editing}
        <button
          bind:this={editButton}
          type="button"
          class="change"
          aria-label={t('model.edit')}
          onclick={edit}
        >
          {t('model.change')}
        </button>
      {/if}
      {#if models}
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
    </span>
  {/if}
</dd>
<dt>{t('model.effort')}</dt>
<dd class:muted={!ai.effort}>{ai.effort || t('model.default')}</dd>
{#if editing || failed}
  <dd class="wide">
    {#if editing}
      <form
        bind:this={form}
        class="editor"
        aria-label={t('model.edit')}
        tabindex="-1"
        onsubmit={save}
      >
        <ModelFields bind:draft models={models ?? []} disabled={busy || offline} />
        <footer class="row">
          <span class="grow"></span>
          <button type="button" class="btn btn-sm btn-ghost" onclick={close}>
            {t('model.cancel')}
          </button>
          <button
            class="btn btn-sm btn-primary"
            disabled={busy || offline || !Object.keys(changes).length}
          >
            {t('model.save')}
          </button>
        </footer>
      </form>
    {/if}
    {#if failed}<Failure code={failed} />{/if}
  </dd>
{/if}

<style>
  .wide {
    display: grid;
    gap: var(--sp-2);
    grid-column: 1 / -1;
    margin: 4px 0;
  }
  .what {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 4px 6px;
  }
  .acts {
    display: flex;
    align-items: center;
    gap: var(--sp-2);
    margin-top: 2px;
    animation: -global-appear var(--dur-2) var(--ease-out);
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
  .change {
    padding: 0;
    border: 0;
    background: none;
    color: var(--sky);
    font: inherit;
    font-size: var(--fs-xs);
    font-weight: 500;
    cursor: pointer;
  }
  .change:hover {
    text-decoration: underline;
    text-underline-offset: 3px;
  }
  .editor {
    display: grid;
    gap: var(--sp-3);
    padding: var(--sp-3);
    border: 1px solid var(--line);
    border-radius: var(--r-md);
    background: var(--surface-2);
    animation: -global-appear var(--dur-2) var(--ease-out);
  }
  .editor:focus {
    outline: none;
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
