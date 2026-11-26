<script lang="ts">
  import { untrack } from 'svelte';
  import type { Group } from '../../lib/groups.svelte';
  import { changesOf, draftOf } from '../../lib/groupset';
  import { t } from '../../lib/prefs.svelte';
  import type { GroupRow } from '../../lib/proto';
  import Failure from '../Failure.svelte';
  import TemplateEditor from '../forge/TemplateEditor.svelte';
  import Icon from '../Icon.svelte';
  import LimitFields from './LimitFields.svelte';
  import SecretaryWarning from './SecretaryWarning.svelte';
  import { secretaryCandidates } from './trust';

  /** What an admin changes of a group (contracts/commands.md 1.1 `group set`). A team group has
   *  its name only (contracts/step-2/watch-team.md section 7). */
  let { group, info, onclose }: { group: Group; info: GroupRow; onclose: () => void } = $props();
  const uid = $props.id();
  let dialog: HTMLDialogElement;
  const before = untrack(() => draftOf(info));
  let draft = $state(untrack(() => draftOf(info)));
  let busy = $state(false);
  let failed = $state('');
  const project = $derived(info.kind === 'project');
  const changes = $derived(changesOf(before, draft, info.limits));
  const candidates = $derived(secretaryCandidates(group));
  const selected = $derived(group.member(draft.secretary));
  const people = $derived(
    group.members.filter((member) => member.kind === 'human').map((member) => member.user),
  );
  const scores = ['pass', 'pass_foundation'] as const;

  $effect(() => dialog.showModal());

  async function save(event: SubmitEvent) {
    event.preventDefault();
    busy = true;
    failed = '';
    const answer = await group.home.command({ t: 'group', g: group.g, op: 'set', ...changes });
    busy = false;
    if (answer.err) failed = answer.err.code;
    else dialog.close();
  }

  /** A field the browser refuses can be seen and focused in an open fold only. */
  const unfold = (event: Event & { currentTarget: HTMLDetailsElement }) =>
    (event.currentTarget.open = true);
</script>

<dialog bind:this={dialog} class="dlg" aria-labelledby={`${uid}-title`} {onclose}>
  <form onsubmit={save}>
    <header><h2 id={`${uid}-title`}>{t('groupinfo.edit')}</h2></header>
    <label class="field">
      <span>{t('groups.create.name')}</span>
      <input class="input" bind:value={draft.name} required pattern=".*\S.*" maxlength="80" />
    </label>
    {#if project}
      {#if candidates.length}
        <label class="field">
          <span id={`${uid}-secretary`}>{t('groupinfo.secretary')}</span>
          <select class="input" aria-labelledby={`${uid}-secretary`} bind:value={draft.secretary}>
            <option value="" disabled>{t('groupinfo.secretary.choose')}</option>
            {#each candidates as ai (ai.id)}<option value={ai.id}>{ai.name}</option>{/each}
          </select>
        </label>
      {/if}
      {#if selected?.kind === 'ai'}
        <SecretaryWarning account={selected.account} owner={selected.user} {people} />
      {/if}
      <label class="field">
        <span id={`${uid}-checks`}>{t('groups.create.checks')}</span>
        <textarea
          class="input mono"
          aria-labelledby={`${uid}-checks`}
          aria-describedby={`${uid}-checks-hint`}
          bind:value={draft.checks}
          rows="3"
          placeholder={'go vet ./...\ngo test ./...'}
          autocapitalize="off"
          spellcheck="false"></textarea>
        <span class="hint" id={`${uid}-checks-hint`}>{t('groups.create.checks.hint')}</span>
      </label>
      <fieldset class="field" aria-describedby={`${uid}-scores-hint`}>
        <legend class="sr">{t('groupinfo.scores')}</legend>
        <div class="scores">
          {#each scores as score (score)}
            <label class="field">
              <span>{t(`groupinfo.${score}`)}</span>
              <input
                class="input"
                type="number"
                inputmode="numeric"
                min="1"
                max="100"
                required
                bind:value={draft[score]}
              />
            </label>
          {/each}
        </div>
        <span class="hint" id={`${uid}-scores-hint`}>{t('groupinfo.scores.hint')}</span>
      </fieldset>
      <div class="folds">
        {#if draft.templates}
          <details class="fold" oninvalidcapture={unfold}>
            <summary><Icon name="down" sm /><b>{t('forge.custom')}</b></summary>
            <div class="fold-body part"><TemplateEditor bind:templates={draft.templates} /></div>
          </details>
        {/if}
        <details class="fold" oninvalidcapture={unfold}>
          <summary>
            <Icon name="down" sm /><b>{t('groupinfo.limits')}</b>
            <span class="cut grow muted">{t('groupinfo.limits.lead')}</span>
          </summary>
          <div class="fold-body"><LimitFields bind:limits={draft.limits} /></div>
        </details>
      </div>
    {/if}
    {#if failed === 'conflict'}
      <p class="fail" role="alert">{t('groupinfo.secretary.conflict')}</p>
    {:else if failed}
      <Failure code={failed} />
    {/if}
    <footer class="row">
      <span class="grow"></span>
      <button type="button" class="btn btn-ghost" onclick={() => dialog.close()}>
        {t('groupinfo.cancel')}
      </button>
      <button
        class="btn btn-primary"
        disabled={busy || !group.home.ctl || !Object.keys(changes).length}
      >
        {t('groupinfo.save')}
      </button>
    </footer>
  </form>
</dialog>

<style>
  /* Held by its top, so that what a fold opens grows below the pointer. */
  dialog {
    overflow: auto;
  }
  @media (min-width: 768px) {
    dialog {
      --top: clamp(16px, 8dvh - 40px, 56px);
      margin-top: var(--top);
      max-height: calc(100dvh - 2 * var(--top));
    }
  }
  .scores {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: var(--sp-4);
  }
  .folds .fold {
    margin: 0 calc(-1 * var(--sp-5));
  }
  .folds .fold:last-child {
    border-bottom: 1px solid var(--line);
  }
  .folds summary {
    padding-inline: var(--sp-5);
    border-radius: 0;
  }
  .part {
    display: grid;
    gap: var(--sp-4);
  }
  .fold-body {
    padding-inline: var(--sp-5);
    animation:
      rise var(--dur-3) var(--ease-out),
      appear var(--dur-2) var(--ease-out);
  }
  .fail {
    color: var(--danger);
    font-size: var(--fs-sm);
  }
</style>
