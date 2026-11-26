<script lang="ts">
  import { tick, untrack } from 'svelte';
  import type { Group, Home } from '../../lib/groups.svelte';
  import { nodes, type Folder } from '../../lib/nodes.svelte';
  import { prefs, t } from '../../lib/prefs.svelte';
  import { names } from '../../lib/stop';
  import { now, shortWhen } from '../../lib/when';
  import Failure from '../Failure.svelte';
  import Icon from '../Icon.svelte';

  /** The folder an AI works in (contracts/local.md section 7): a row that says which, and a dialog
   *  over the dialog that lists the folders its CLI was used in on its computer, newest first, and
   *  takes one typed by hand. A folder of another group cannot be chosen; one of an AI of `group`
   *  can, and is shared. Until the person chooses, the newest folder that no other group has is taken. */
  let {
    home,
    agent,
    ai,
    computer,
    group,
    workspace = $bindable(''),
  }: {
    home: Home;
    agent: string;
    /** The AI's name, as the texts say it. */
    ai: string;
    computer: string;
    group?: Group;
    workspace?: string;
  } = $props();
  const uid = $props.id();

  // The list is in the page only while it is open: it sits inside another dialog's form.
  let choosing = $state(false);
  let dialog = $state<HTMLDialogElement>();
  let folders = $state.raw<Folder[]>();
  let failed = $state('');
  let picked = $state('');
  let typed = $state('');
  let refused = $state('');

  /** The newest folder that no other group has: the list comes newest first. */
  const newest = (list: Folder[]) => list.find((folder) => !folder.other)?.path ?? '';

  // A list asked for before the AI changed is not shown.
  async function load() {
    failed = '';
    folders = undefined;
    const asked = `${home.device} ${agent}`;
    const answer = await nodes.folders(home, agent, group);
    if (asked !== `${home.device} ${agent}`) return;
    if (!Array.isArray(answer)) {
      failed = answer.err.code;
      folders = [];
      return;
    }
    folders = answer;
    workspace ||= newest(answer);
  }
  $effect(() => {
    void [home, agent];
    untrack(load);
  });

  const holder = $derived(workspace ? nodes.holder(home, workspace, group) : {});

  // A folder typed before is in the field again, where it can be read whole and changed.
  async function choose() {
    picked = workspace;
    typed = folders?.some((folder) => folder.path === workspace) ? '' : workspace;
    refused = '';
    choosing = true;
    await tick();
    dialog!.showModal();
  }

  /** Takes the folder typed, else the one ticked; a folder of another group is refused here. */
  function use() {
    const path = typed.trim() || picked;
    const other = nodes.holder(home, path, group).other;
    if (other) refused = t('member.folder.taken').replace('{group}', () => other);
    else if (path) {
      workspace = path;
      dialog!.close();
    }
  }

  // Enter in the field takes the folder: the dialog sits in another dialog's form.
  function enter(event: KeyboardEvent) {
    if (event.key !== 'Enter') return;
    event.preventDefault();
    use();
  }

  const named = (name: string) => ({ name });
  const used = (folder: Folder) =>
    t('member.folder.used').replace('{when}', shortWhen(folder.used, now(), prefs.lang));
</script>

<div class="folder">
  <div class="path-row">
    <Icon name="folder" sm />
    <code id={`${uid}-path`} class="path" title={workspace}>
      <bdi>{workspace || t(folders ? 'member.folder.none' : 'member.folder.loading')}</bdi>
    </code>
    <button
      type="button"
      class="btn btn-sm change"
      aria-label={t('member.folder.change.of').replace('{ai}', () => ai)}
      onclick={choose}
    >
      {t(workspace ? 'member.folder.change' : 'member.folder.choose')}
    </button>
  </div>
  {#if holder.ais}
    <p class="hint">
      {t('member.folder.shared').replace('{ai}', () => names(holder.ais!.map(named), prefs.lang))}
    </p>
  {/if}
</div>

{#if choosing}
  <dialog
    bind:this={dialog}
    class="dlg"
    aria-labelledby={`${uid}-title`}
    onclose={() => (choosing = false)}
  >
    <div class="body">
      <header>
        <h2 id={`${uid}-title`}>{t('member.folder.title')}</h2>
        <p>
          {t('member.folder.lead')
            .replace('{ai}', () => ai)
            .replace('{computer}', () => computer)}
        </p>
      </header>
      {#if !folders}
        <p class="hint" role="status">{t('member.folder.loading')}</p>
      {:else if failed}
        <Failure code={failed} />
      {:else if folders.length}
        <fieldset class="list">
          <legend class="sr">{t('member.folder.title')}</legend>
          {#each folders as folder (folder.path)}
            <label class="pick-row" class:dim={folder.other}>
              <input
                type="radio"
                name={`${uid}-folder`}
                value={folder.path}
                disabled={Boolean(folder.other)}
                bind:group={picked}
              />
              <Icon name="folder" sm />
              <span class="grow">
                <code class="path" title={folder.path}><bdi>{folder.path}</bdi></code>
                <small>
                  {#if folder.other}
                    {t('member.folder.other').replace('{group}', () => folder.other!)}
                  {:else}
                    {used(folder)}{#if folder.ais}{' '}·
                      {t('member.folder.mine').replace('{ai}', () =>
                        names(folder.ais!.map(named), prefs.lang),
                      )}{/if}
                  {/if}
                </small>
              </span>
            </label>
          {/each}
        </fieldset>
      {:else}
        <div class="blank">
          <span class="blank-mark"><Icon name="folder" /></span>
          <p>{t('member.folder.empty').replace('{ai}', () => ai)}</p>
        </div>
      {/if}
      <label class="field">
        <span>{t('member.folder.typed')}</span>
        <input
          class="input mono"
          bind:value={typed}
          placeholder={t('member.folder.typed.hint')}
          autocapitalize="off"
          spellcheck="false"
          onkeydown={enter}
          oninput={() => (refused = '')}
        />
      </label>
      {#if refused}<p class="fail" role="alert">{refused}</p>{/if}
      <footer class="row">
        <span class="grow"></span>
        <button type="button" class="btn btn-ghost" onclick={() => dialog!.close()}>
          {t('member.folder.cancel')}
        </button>
        <button
          type="button"
          class="btn btn-primary"
          disabled={!typed.trim() && !picked}
          onclick={use}
        >
          {t('member.folder.use')}
        </button>
      </footer>
    </div>
  </dialog>
{/if}

<style>
  /* One column as wide as the dialog, however long the path. */
  .folder {
    display: grid;
    grid-template-columns: minmax(0, 1fr);
    gap: 4px;
  }
  /* A link in the row: the accent text, no frame of its own. */
  .change {
    margin-right: -6px;
    border-color: transparent;
    background: transparent;
    box-shadow: none;
    color: var(--sky);
  }
  dialog {
    overflow: auto;
  }
  .body {
    display: flex;
    flex-direction: column;
    gap: var(--sp-4);
  }
  .list {
    display: grid;
    max-height: min(320px, 45dvh);
    overflow: auto;
  }
  .pick-row code {
    font-size: var(--fs-sm);
  }
  .pick-row :global(.i) {
    flex: none;
    color: var(--text-3);
  }
  .pick-row input {
    margin: 0;
  }
  /* A folder that cannot be chosen fades its path; the reason beside it stays in full. */
  .pick-row.dim {
    cursor: not-allowed;
  }
  .pick-row.dim code,
  .pick-row.dim :global(.i) {
    opacity: 0.55;
  }
  .grow {
    min-width: 0;
  }
  .blank {
    padding: var(--sp-4) var(--sp-2);
  }
  .fail {
    color: var(--danger);
    font-size: var(--fs-sm);
  }
</style>
