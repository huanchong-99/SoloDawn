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
  import { Knowledge } from './knowledge.svelte';

  /** The knowledge base: a person writes and edits documents in place, name and text
   *  (commands.md 1.6; step-3/knowledge.md section 2); an admin deletes them. */
  let { group }: { group: Group } = $props();
  const uid = $props.id();
  const addId = `${uid}-add`;
  const editing = new Editing();
  const close = () => void editing.close();
  const reasons = $derived({ conflict: t('kb.taken'), not_found: t('groupinfo.edit.gone') });
  const fields = (name: string, text: string): Field[] => [
    { label: t('kb.name'), value: name, maxlength: 80, placeholder: t('kb.name') },
    { label: t('kb.text'), value: text, multiline: true, placeholder: t('kb.text') },
  ];
  let open = $state(false);
  let heading = $state<HTMLElement>();
  let pending = $state.raw<ForgetTarget>();
  let query = $state('');
  const view = $derived(new Knowledge(group));
  const documents = $derived(view.documents);
  const offline = $derived(!group.home.ctl);
  const deletable = $derived(!offline && isAdmin(group));

  $effect(() => {
    if (open) view.sync();
  });

  /** A new document never replaces one of the same name that this browser holds. */
  const add = ([name, text]: string[]) =>
    view.has(name.trim())
      ? Promise.resolve('conflict')
      : addNote(group, text, { type: 'kb', name: name.trim() });

  function clear() {
    query = '';
    view.clear();
  }

  function search(event: SubmitEvent) {
    event.preventDefault();
    void view.search(query);
  }
</script>

<section class="kb" aria-labelledby={`${uid}-title`}>
  <details class="fold" bind:open>
    <summary bind:this={heading}>
      <Icon name="down" sm />
      <b id={`${uid}-title`}>{t('kb.title')}</b>
      <span class="cut grow muted">{t('kb.lead')}</span>
    </summary>
    {#if open}
      <div class="fold-body mem-body">
        {#if offline}<p class="hint">{t('kb.offline')}</p>{/if}
        <form class="row" onsubmit={search}>
          <label class="field grow">
            <span class="sr">{t('kb.query')}</span>
            <input class="input" bind:value={query} disabled={offline || view.busy} />
          </label>
          <button class="btn btn-primary btn-sm" disabled={offline || view.busy || !query.trim()}>
            {t('kb.search')}
          </button>
          {#if view.ids !== undefined}
            <button type="button" class="btn btn-ghost btn-sm" onclick={clear}>
              {t('kb.clear')}
            </button>
          {/if}
        </form>
        {#if view.failed}<Failure code={view.failed} />{/if}
        {#if view.busy}<p class="hint" role="status">{t('kb.loading')}</p>{/if}
        {#if view.ids === undefined && view.more}
          <button
            type="button"
            class="btn btn-sm btn-ghost older"
            disabled={offline || view.busy}
            onclick={() => void view.older()}
          >
            {t('kb.earlier')}
          </button>
        {/if}
        {#if view.narrow}<p class="hint">{t('kb.narrow')}</p>{/if}
        <ul class="mem-list" aria-labelledby={`${uid}-title`}>
          {#each documents as document (document.id)}
            {@const editId = `${uid}-edit-${document.id}`}
            <li class="mem-item">
              {#if editing.id !== editId}<b>{document.name}</b>{/if}
              <div class="mem-meta">
                <b>{authorOf(group, document.by)}</b>
                <time datetime={new Date(document.ts).toISOString()}>
                  {shortWhen(document.ts, now(), prefs.lang)}
                </time>
                <span class="mem-acts">
                  {#if group.me}
                    <EditButton
                      id={editId}
                      label={t('kb.edit').replace('{name}', document.name!)}
                      {editing}
                      disabled={offline}
                    />
                  {/if}
                  <button
                    type="button"
                    class="btn btn-sm btn-ghost del"
                    disabled={!deletable}
                    aria-label={t('kb.forget').replace('{text}', document.name!)}
                    onclick={() =>
                      (pending = {
                        r: 'note',
                        kind: 'doc',
                        id: document.id,
                        label: document.name!,
                        heading,
                      })}
                  >
                    {t('groupinfo.forget')}
                  </button>
                </span>
              </div>
              {#if editing.id === editId}
                <EditField
                  fields={fields(document.name!, document.text)}
                  {reasons}
                  save={([name, text]) => editNote(group, document, text, name.trim())}
                  onclose={close}
                />
              {:else}
                <p class="mem-text">{document.text}</p>
              {/if}
            </li>
          {/each}
        </ul>
        {#if !view.busy && !view.failed && documents.length === 0}
          {#if view.ids !== undefined}<p class="hint">{t('kb.noMatch')}</p>
          {:else if view.loaded}<p class="hint">{t('kb.empty')}</p>{/if}
        {/if}
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
            <Icon name="plus" sm />{t('kb.add')}
          </button>
        {/if}
      </div>
    {/if}
  </details>
</section>

{#if pending}<ForgetDialog {group} target={pending} onclose={() => (pending = undefined)} />{/if}

<style>
  .kb {
    display: flex;
    flex-direction: column;
    gap: var(--sp-2);
    padding: 0 var(--sp-4) 14px;
  }
  .kb > .fold {
    border-top: 0;
  }
  .kb > .fold > summary {
    border-radius: 0;
  }
  .mem-body {
    display: flex;
    flex-direction: column;
    gap: var(--sp-3);
    padding-top: var(--sp-3);
  }
  .older {
    align-self: flex-start;
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
  .mem-acts {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    flex: none;
    white-space: nowrap;
    margin-left: auto;
  }
  .add {
    align-self: flex-start;
    gap: 4px;
    color: var(--sky);
  }
  .add[disabled] {
    color: var(--text-3);
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
