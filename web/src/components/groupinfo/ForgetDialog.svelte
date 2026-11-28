<script lang="ts">
  import { forgetMemory, hasMemoryRow, type ForgetRowType } from '../../lib/groupinfo.svelte';
  import type { Group } from '../../lib/groups.svelte';
  import { t } from '../../lib/prefs.svelte';
  import Failure from '../Failure.svelte';

  /** Row and focus handles passed to the forget confirmation dialog. */
  export interface ForgetTarget {
    r: ForgetRowType;
    kind?: 'note' | 'preset' | 'doc';
    id: string;
    label: string;
    heading: HTMLElement | undefined;
  }

  const confirmText = {
    note: { title: 'groupinfo.confirm.note', body: 'groupinfo.confirm.body' },
    preset: { title: 'preset.confirm', body: 'preset.confirm.body' },
    doc: { title: 'kb.confirm', body: 'kb.confirm.body' },
    review: { title: 'groupinfo.confirm.review', body: 'groupinfo.confirm.body' },
  } as const;

  /** Confirmation dialog that sends `forget` for one note, preset, document, or review ledger entry. */
  let {
    group,
    target,
    onclose,
  }: {
    group: Group;
    target: ForgetTarget;
    onclose: () => void;
  } = $props();
  const uid = $props.id();
  const copy = $derived(confirmText[target.r === 'note' ? (target.kind ?? 'note') : 'review']);

  let dialog: HTMLDialogElement;
  let busy = $state(false);
  let failed = $state('');
  let removed = false;

  $effect(() => {
    if (!dialog.open) dialog.showModal();
  });

  $effect(() => {
    if (!busy && !hasMemoryRow(group, target.r, target.id) && dialog.open) {
      removed = true;
      dialog.close();
    }
  });

  async function submit(event: SubmitEvent) {
    event.preventDefault();
    failed = '';
    busy = true;
    const code = await forgetMemory(group, target.r, target.id);
    busy = false;
    if (code) {
      failed = code;
      return;
    }
    removed = true;
    dialog.close();
  }

  // The dialog itself returns focus to the entry's button; a deleted entry has none left.
  function handleClose() {
    if (removed) target.heading?.focus();
    onclose();
  }
</script>

<dialog bind:this={dialog} class="dlg" aria-labelledby={`${uid}-title`} onclose={handleClose}>
  <form onsubmit={submit}>
    <header>
      <h2 id={`${uid}-title`}>{t(copy.title)}</h2>
      <p class="lead">{t(copy.body)}</p>
    </header>

    <p class="preview">{target.label}</p>

    {#if failed}<Failure code={failed} />{/if}

    <footer class="row">
      <span class="grow"></span>
      <button type="button" class="btn btn-ghost" onclick={() => dialog.close()}>
        {t('groupinfo.confirm.cancel')}
      </button>
      <button class="btn btn-danger" disabled={busy || !group.home.ctl}>
        {t('groupinfo.confirm.submit')}
      </button>
    </footer>
  </form>
</dialog>

<style>
  dialog {
    overflow: auto;
  }
  .lead {
    margin-top: 6px;
    color: var(--text-2);
    font-size: var(--fs-sm);
  }
  .preview {
    padding: 10px 12px;
    border: 1px solid var(--line);
    border-radius: var(--r-md);
    background: var(--surface-2);
    font-size: var(--fs-sm);
    overflow-wrap: anywhere;
  }
</style>
