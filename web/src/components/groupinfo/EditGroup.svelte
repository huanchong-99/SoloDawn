<script lang="ts">
  import { untrack } from 'svelte';
  import type { Group } from '../../lib/groups.svelte';
  import { t } from '../../lib/prefs.svelte';
  import type { GroupRow } from '../../lib/proto';
  import Failure from '../Failure.svelte';
  import TemplateEditor from '../forge/TemplateEditor.svelte';
  import { emptyTemplates } from '../forge/templates';
  import SecretaryWarning from './SecretaryWarning.svelte';

  let { group, info, onclose }: { group: Group; info: GroupRow; onclose: () => void } = $props();
  const uid = $props.id();
  let dialog: HTMLDialogElement;
  let templates = $state(untrack(() => structuredClone(info.templates ?? emptyTemplates())));
  let secretary = $state(untrack(() => info.secretary ?? ''));
  let busy = $state(false);
  let failed = $state('');
  const candidates = $derived(group.members.filter((member) => member.kind === 'ai'));
  const selected = $derived(group.member(secretary));
  const people = $derived(
    group.members.filter((member) => member.kind === 'human').map((member) => member.user),
  );

  $effect(() => dialog.showModal());

  async function save(event: SubmitEvent) {
    event.preventDefault();
    busy = true;
    failed = '';
    const answer = await group.home.command({
      t: 'group',
      g: group.g,
      op: 'set',
      ...(info.forge === 'custom' ? { templates } : {}),
      ...(secretary ? { secretary } : {}),
    });
    busy = false;
    if (answer.err) failed = answer.err.code;
    else dialog.close();
  }
</script>

<dialog bind:this={dialog} class="dlg" aria-labelledby={`${uid}-title`} {onclose}>
  <form onsubmit={save}>
    <header><h2 id={`${uid}-title`}>{t('groupinfo.edit')}</h2></header>
    <p class="hint">{t('forge.fixed').replace('{forge}', info.forge)}</p>
    {#if info.forge === 'custom'}<TemplateEditor bind:templates />{/if}
    {#if candidates.length}
      <label class="field">
        <span id={`${uid}-secretary`}>{t('groupinfo.secretary')}</span>
        <select class="input" aria-labelledby={`${uid}-secretary`} bind:value={secretary}>
          <option value="" disabled>{t('groupinfo.secretary.choose')}</option>
          {#each candidates as ai (ai.id)}<option value={ai.id}>{ai.name}</option>{/each}
        </select>
      </label>
    {/if}
    {#if selected?.kind === 'ai'}
      <SecretaryWarning account={selected.account} owner={selected.user} {people} />
    {/if}
    {#if failed}<Failure code={failed} />{/if}
    <footer class="row">
      <span class="grow"></span>
      <button type="button" class="btn btn-ghost" onclick={() => dialog.close()}>
        {t('groupinfo.cancel')}
      </button>
      <button class="btn btn-primary" disabled={busy || !group.home.ctl}>
        {t('groupinfo.save')}
      </button>
    </footer>
  </form>
</dialog>
