<script lang="ts">
  import { addNote, authorOf, editNote, Editing } from '../../lib/groupinfo.svelte';
  import type { Group } from '../../lib/groups.svelte';
  import { isAdmin } from '../../lib/members';
  import { prefs, t } from '../../lib/prefs.svelte';
  import { now, shortWhen } from '../../lib/when';
  import Failure from '../Failure.svelte';
  import Icon from '../Icon.svelte';
  import EditButton from '../groupinfo/EditButton.svelte';
  import EditField, { type Field } from '../groupinfo/EditField.svelte';
  import ForgetDialog, { type ForgetTarget } from '../groupinfo/ForgetDialog.svelte';
  import { presets, PresetView } from './presets.svelte';

  /** The style presets: a person writes and changes them in place, name and text (commands.md
   *  1.6; step-3/knowledge.md section 2); an admin deletes them. */
  let { group }: { group: Group } = $props();
  const uid = $props.id();
  const addId = `${uid}-add`;
  const editing = new Editing();
  const close = () => void editing.close();
  const reasons = $derived({ conflict: t('preset.taken'), not_found: t('groupinfo.edit.gone') });
  const fields = (name: string, text: string): Field[] => [
    { label: t('preset.name'), value: name, maxlength: 80 },
    { label: t('preset.text'), value: text, multiline: true },
  ];
  let open = $state(false);
  let heading = $state<HTMLElement>();
  let pending = $state.raw<ForgetTarget>();
  const view = $derived(new PresetView(group));
  const notes = $derived(presets(group));
  const offline = $derived(!group.home.ctl);
  const deletable = $derived(!offline && isAdmin(group));

  $effect(() => {
    if (open) view.sync();
  });

  /** A new preset never replaces one of the same name: that one is changed in place. */
  const add = ([name, text]: string[]) =>
    notes.some((note) => note.name === name.trim())
      ? Promise.resolve('conflict')
      : addNote(group, text, { type: 'preset', name: name.trim() });
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
        {#if offline}<p class="hint">{t('preset.offline')}</p>{/if}
        {#if view.failed}<Failure code={view.failed} />{/if}
        {#if view.busy}<p class="hint" role="status">{t('preset.loading')}</p>{/if}
        <ul class="mem-list" aria-labelledby={`${uid}-head`}>
          {#each notes as note (note.id)}
            {@const editId = `${uid}-edit-${note.id}`}
            <li class="mem-item">
              {#if editing.id !== editId}<b>{note.name}</b>{/if}
              <div class="mem-meta">
                <b>{authorOf(group, note.by)}</b>
                <time datetime={new Date(note.ts).toISOString()}>
                  {shortWhen(note.ts, now(), prefs.lang)}
                </time>
                {#if group.me}
                  <EditButton
                    id={editId}
                    label={t('preset.edit').replace('{name}', note.name!)}
                    {editing}
                    disabled={offline}
                  />
                {/if}
                <button
                  type="button"
                  class="btn btn-sm btn-ghost del"
                  disabled={!deletable}
                  aria-label={t('groupinfo.forget.note').replace('{text}', note.name!)}
                  onclick={() => (pending = { r: 'note', id: note.id, label: note.name!, heading })}
                >
                  {t('groupinfo.forget')}
                </button>
              </div>
              {#if editing.id === editId}
                <EditField
                  fields={fields(note.name!, note.text)}
                  {reasons}
                  save={([name, text]) => editNote(group, note, text, name.trim())}
                  onclose={close}
                />
              {:else}
                <p class="mem-text">{note.text}</p>
              {/if}
            </li>
          {/each}
        </ul>
        {#if view.loaded && notes.length === 0}<p class="hint">{t('preset.empty')}</p>{/if}
        {#if editing.id === addId}
          <div class="mem-item">
            <EditField fields={fields('', '')} {reasons} save={add} onclose={close} />
          </div>
        {:else if group.me}
          <button
            type="button"
            id={addId}
            class="btn btn-sm btn-ghost add"
            disabled={offline}
            onclick={() => (editing.id = addId)}
          >
            <Icon name="plus" sm />{t('preset.add')}
          </button>
        {/if}
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
  .del,
  .mem-meta :global(.edit) {
    margin-left: auto;
  }
  .mem-meta :global(.edit + .del) {
    margin-left: 0;
  }
  .del:not([disabled]) {
    color: var(--danger);
    --hover: var(--danger-soft);
  }
  .add {
    align-self: flex-start;
    gap: 4px;
    color: var(--sky);
  }
  .add[disabled] {
    color: var(--text-3);
  }
  .mem-text {
    white-space: pre-wrap;
    overflow-wrap: anywhere;
  }
</style>
