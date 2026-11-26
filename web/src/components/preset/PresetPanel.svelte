<script lang="ts">
  import { authorOf } from '../../lib/groupinfo.svelte';
  import type { Group } from '../../lib/groups.svelte';
  import { isAdmin } from '../../lib/members';
  import { prefs, t } from '../../lib/prefs.svelte';
  import type { NoteRow } from '../../lib/proto';
  import { now, shortWhen } from '../../lib/when';
  import Failure from '../Failure.svelte';
  import Icon from '../Icon.svelte';
  import ForgetDialog, { type ForgetTarget } from '../groupinfo/ForgetDialog.svelte';
  import { canWrite, presets, PresetView, savePreset, validName } from './presets.svelte';

  let { group }: { group: Group } = $props();
  const uid = $props.id();
  let open = $state(false);
  let heading = $state<HTMLElement>();
  let pending = $state.raw<ForgetTarget>();
  let name = $state('');
  let text = $state('');
  let editing = $state(false);
  let busy = $state(false);
  let failed = $state('');
  const view = $derived(new PresetView(group));
  const notes = $derived(presets(group));
  const writable = $derived(canWrite(group));
  const deletable = $derived(Boolean(group.home.ctl) && isAdmin(group));

  $effect(() => {
    if (open) view.sync();
  });

  function edit(note: NoteRow) {
    name = note.name!;
    text = note.text;
    editing = true;
    failed = '';
  }

  function clear() {
    name = '';
    text = '';
    editing = false;
    failed = '';
  }

  async function submit(event: SubmitEvent) {
    event.preventDefault();
    busy = true;
    failed = await savePreset(group, name, text);
    busy = false;
    if (!failed) clear();
  }
</script>

<section class="preset" aria-labelledby={`${uid}-head`}>
  <details class="fold" bind:open>
    <summary bind:this={heading}>
      <Icon name="down" sm />
      <b id={`${uid}-head`}>{t('preset.title')}</b>
      <span class="cut grow muted">{t('preset.lead')}</span>
    </summary>
    {#if open}
      <div class="fold-body mem-body">
        {#if !group.home.ctl}<p class="hint">{t('preset.offline')}</p>{/if}
        {#if view.failed}<Failure code={view.failed} />{/if}
        {#if view.busy}<p class="hint" role="status">{t('preset.loading')}</p>{/if}
        <ul class="mem-list" aria-labelledby={`${uid}-head`}>
          {#each notes as note (note.id)}
            <li class="mem-item">
              <b>{note.name}</b>
              <div class="mem-meta">
                <b>{authorOf(group, note.by)}</b>
                <time datetime={new Date(note.ts).toISOString()}>
                  {shortWhen(note.ts, now(), prefs.lang)}
                </time>
                <button
                  type="button"
                  class="btn btn-sm btn-ghost"
                  disabled={!writable || busy}
                  onclick={() => edit(note)}
                >
                  {t('preset.edit')}
                </button>
                <button
                  type="button"
                  class="btn btn-sm btn-ghost del"
                  disabled={!deletable || busy}
                  aria-label={t('groupinfo.forget.note').replace('{text}', note.name!)}
                  onclick={() => (pending = { r: 'note', id: note.id, label: note.name!, heading })}
                >
                  {t('groupinfo.forget')}
                </button>
              </div>
              <p class="mem-text">{note.text}</p>
            </li>
          {/each}
        </ul>
        {#if view.loaded && notes.length === 0}<p class="hint">{t('preset.empty')}</p>{/if}
        <form class="mem-body" onsubmit={submit}>
          <label class="field">
            <span>{t('preset.name')}</span><input
              class="input"
              bind:value={name}
              required
              disabled={!writable || busy || editing}
            />
          </label>
          <label class="field">
            <span>{t('preset.text')}</span><textarea
              class="input"
              rows="4"
              bind:value={text}
              disabled={!writable || busy}></textarea>
          </label>
          {#if failed}<Failure code={failed} />{/if}
          <div class="row">
            <button class="btn btn-primary btn-sm" disabled={!writable || busy || !validName(name)}>
              {t('preset.save')}
            </button>
            {#if editing}<button
                type="button"
                class="btn btn-ghost btn-sm"
                disabled={busy}
                onclick={clear}
              >
                {t('preset.cancel')}
              </button>{/if}
          </div>
        </form>
      </div>
    {/if}
  </details>
</section>

{#if pending}<ForgetDialog {group} target={pending} onclose={() => (pending = undefined)} />{/if}

<style>
  .preset {
    display: flex;
    flex-direction: column;
    gap: var(--sp-2);
    padding: 0 var(--sp-4) 14px;
  }
  .preset > .fold {
    border-top: 0;
  }
  .preset > .fold > summary {
    border-radius: 0;
  }
  .mem-body {
    display: flex;
    flex-direction: column;
    gap: var(--sp-3);
    padding-top: var(--sp-3);
  }
  .mem-list {
    display: flex;
    flex-direction: column;
    gap: var(--sp-2);
  }
  .mem-item {
    display: flex;
    flex-direction: column;
    gap: 4px;
    padding: 10px var(--sp-3);
    border-radius: var(--r-md);
    background: var(--surface-2);
    font-size: var(--fs-sm);
    line-height: 1.5;
    min-width: 0;
  }
  .mem-meta {
    display: flex;
    align-items: center;
    gap: 6px;
    flex-wrap: wrap;
    font-size: var(--fs-xs);
    color: var(--text-3);
  }
  .mem-meta b {
    color: var(--text);
    font-size: var(--fs-sm);
  }
  .del {
    margin-left: auto;
  }
  .del:not([disabled]) {
    color: var(--danger);
    --hover: var(--danger-soft);
  }
  .mem-text {
    white-space: pre-wrap;
    overflow-wrap: anywhere;
  }
</style>
