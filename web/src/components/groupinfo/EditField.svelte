<script lang="ts" module>
  /** One value of an edit in place: a line, a text, or a choice of `options`, which offers
   *  `placeholder` while nothing is chosen. */
  export interface Field {
    label: string;
    value: string;
    multiline?: boolean;
    maxlength?: number;
    options?: { value: string; label: string }[];
    placeholder?: string;
  }
</script>

<script lang="ts">
  import { onMount, untrack } from 'svelte';
  import { t } from '../../lib/prefs.svelte';
  import Failure from '../Failure.svelte';

  /** Changes values where they are shown. Enter in a line, Ctrl or ⌘ with Enter in a text, or
   *  Save sends them; Escape in a field, or Cancel, leaves them as they were. A refusal stays beside the
   *  fields, in the words of `reasons` where it has them. Nothing changed sends nothing. */
  let {
    fields,
    reasons = {},
    save,
    onclose,
  }: {
    fields: Field[];
    reasons?: Record<string, string>;
    save: (values: string[]) => Promise<string>;
    onclose: () => void;
  } = $props();
  const uid = $props.id();
  let form: HTMLFormElement;
  let draft = $state(untrack(() => fields.map((field) => field.value)));
  let busy = $state(false);
  let failed = $state('');
  const blank = $derived(draft.some((value) => !value.trim()));

  onMount(() => form.querySelector<HTMLElement>('.input')!.focus());

  async function submit(event: SubmitEvent) {
    event.preventDefault();
    if (draft.every((value, i) => value === fields[i].value)) return onclose();
    if (blank) return;
    busy = true;
    failed = await save(draft);
    busy = false;
    if (!failed) onclose();
  }

  /** Keys wait while a save is in flight: its answer closes or keeps this edit. */
  function keydown(event: KeyboardEvent) {
    if (busy) return;
    const send = event.key === 'Enter' && (event.ctrlKey || event.metaKey);
    if (event.key !== 'Escape' && !send) return;
    event.preventDefault();
    if (send) form.requestSubmit();
    else onclose();
  }
</script>

<form class="edit-field" bind:this={form} onsubmit={submit}>
  {#each fields as field, i (field.label)}
    {#if field.options}
      <select
        class="input"
        aria-label={field.label}
        disabled={busy}
        onkeydown={keydown}
        bind:value={draft[i]}
      >
        {#if !field.value}<option value="" disabled>{field.placeholder}</option>{/if}
        {#each field.options as option (option.value)}
          <option value={option.value}>{option.label}</option>
        {/each}
      </select>
    {:else if field.multiline}
      <textarea
        class="input"
        aria-label={field.label}
        aria-describedby={`${uid}-keys`}
        rows="4"
        maxlength={field.maxlength}
        readonly={busy}
        onkeydown={keydown}
        bind:value={draft[i]}></textarea>
    {:else}
      <input
        class="input"
        aria-label={field.label}
        maxlength={field.maxlength}
        readonly={busy}
        onkeydown={keydown}
        bind:value={draft[i]}
      />
    {/if}
  {/each}
  {#if failed && reasons[failed]}
    <p class="fail" role="alert">{reasons[failed]}</p>
  {:else if failed}
    <Failure code={failed} />
  {/if}
  <div class="row">
    {#if fields.some((field) => field.multiline)}
      <span class="hint cut grow" id={`${uid}-keys`}>{t('groupinfo.edit.keys')}</span>
    {:else}
      <span class="grow"></span>
    {/if}
    <button type="button" class="btn btn-sm btn-ghost" disabled={busy} onclick={onclose}>
      {t('groupinfo.cancel')}
    </button>
    <button class="btn btn-sm btn-primary" disabled={busy || blank}>{t('groupinfo.save')}</button>
  </div>
</form>

<style>
  .edit-field {
    display: flex;
    flex-direction: column;
    gap: var(--sp-2);
    min-width: 0;
    animation: appear var(--dur-2) var(--ease-out);
  }
  textarea {
    min-height: 96px;
    line-height: 1.5;
  }
  .fail {
    color: var(--danger);
    font-size: var(--fs-sm);
  }
</style>
