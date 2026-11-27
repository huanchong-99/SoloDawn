<script lang="ts">
  import type { Editing } from '../../lib/groupinfo.svelte';
  import { t } from '../../lib/prefs.svelte';
  import Icon from '../Icon.svelte';

  /** What opens an edit in place: an accent text button with a pencil, named for what it edits.
   *  Its `id` is the edit's, so that focus comes back to it when the edit ends. */
  let {
    id,
    label,
    editing,
    disabled = false,
  }: { id: string; label: string; editing: Editing; disabled?: boolean } = $props();
</script>

<button
  type="button"
  {id}
  class="btn btn-sm btn-ghost edit"
  aria-label={label}
  aria-expanded={editing.id === id}
  {disabled}
  onclick={() => (editing.id = editing.id === id ? undefined : id)}
>
  <Icon name="edit" sm />{t('groupinfo.change')}
</button>

<style>
  .edit {
    gap: 4px;
    padding-inline: 6px;
  }
  .edit:not([disabled]) {
    color: var(--sky);
  }
</style>
