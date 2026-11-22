<script lang="ts">
  import { untrack } from 'svelte';
  import { homes, type Home } from '../../lib/groups.svelte';
  import { t } from '../../lib/prefs.svelte';
  import Failure from '../Failure.svelte';

  /** The create-group dialog (contracts/commands.md 1.1). `open` shows it. */
  let { open = $bindable(false) }: { open?: boolean } = $props();
  const uid = $props.id();

  // A group is created on a node of the person's own that can be reached.
  const owned = $derived(homes.filter((home) => home.owner && home.ctl));
  let dialog: HTMLDialogElement;
  let opener: HTMLElement | null = null;
  // The computer chosen as the home: the Home itself, not its place in a list that changes as
  // computers come and go.
  let chosen = $state.raw<Home>();
  let name = $state('');
  let repo = $state('');
  let base = $state('main');
  let forge = $state('github');
  let checks = $state('');
  let member = $state('');
  let busy = $state(false);
  let failed = $state('');

  const listed = () => owned.find((home) => home === chosen);
  /** The computer chosen before, when it is still to be had, or else the first. */
  const pick = () => (chosen = listed() ?? owned[0]);

  function focusFirstControl() {
    if (dialog.contains(document.activeElement)) return;
    dialog
      .querySelector<HTMLElement>(
        'input:not([disabled]), select:not([disabled]), button:not([disabled])',
      )
      ?.focus();
  }

  function handleKeydown(event: KeyboardEvent) {
    if (event.key === 'Escape' && dialog.open) {
      event.preventDefault();
      dialog.close();
    }
  }

  function restoreOpener() {
    if (!opener?.isConnected) return;
    const lost =
      document.activeElement === document.body || dialog.contains(document.activeElement);
    if (lost) opener.focus();
  }

  function handleClose() {
    open = false;
    restoreOpener();
    opener = null;
  }

  $effect(() => {
    if (!open) {
      if (dialog.open) dialog.close();
      return;
    }
    untrack(pick);
    if (!dialog.open) {
      opener = document.activeElement instanceof HTMLElement ? document.activeElement : null;
      dialog.showModal();
      focusFirstControl();
    }
  });
  // A computer that appears while there was none to choose from is chosen: the select shows it,
  // and what the select shows is what is sent to.
  let none = true;
  $effect(() => {
    const some = owned.length > 0;
    if (some && none) untrack(pick);
    none = !some;
  });

  async function submit(event: SubmitEvent) {
    event.preventDefault();
    failed = '';
    // The computer may have gone since it was chosen: nothing is sent to another in its place.
    // The first one left is chosen for the next try, and the person sees which.
    const home = listed();
    if (!home) {
      failed = 'lost';
      chosen = owned[0];
      return;
    }
    busy = true;
    const { g, answer } = await home.create({
      name: name.trim(),
      repo: repo.trim(),
      base: base.trim(),
      forge,
      checks: checks
        .split('\n')
        .map((line) => line.trim())
        .filter(Boolean),
      member_name: member.trim(),
    });
    busy = false;
    if (answer.err) {
      failed = answer.err.code;
      return;
    }
    name = repo = checks = '';
    dialog.close();
    // The group itself arrives in a push; its chat opens as soon as it is listed.
    location.hash = `#/groups/${g}`;
  }
</script>

<dialog
  bind:this={dialog}
  class="dlg"
  aria-labelledby={`${uid}-title`}
  onkeydown={handleKeydown}
  onclose={handleClose}
>
  <form onsubmit={submit}>
    <header>
      <h2 id={`${uid}-title`}>{t('groups.new')}</h2>
      <p class="lead">{t('groups.create.lead')}</p>
    </header>

    <label class="field">
      <span>{t('groups.create.name')}</span>
      <!-- The pattern asks for one character that is no space. -->
      <input class="input" bind:value={name} required pattern=".*\S.*" maxlength="80" />
    </label>
    <label class="field">
      <span id={`${uid}-repo`}>{t('groups.create.repo')}</span>
      <input
        class="input mono"
        aria-labelledby={`${uid}-repo`}
        aria-describedby={`${uid}-repo-hint`}
        bind:value={repo}
        required
        pattern=".*\S.*"
        placeholder="https://github.com/you/project.git"
        autocapitalize="off"
        spellcheck="false"
      />
      <span class="hint" id={`${uid}-repo-hint`}>{t('groups.create.repo.hint')}</span>
    </label>
    <div class="two">
      <label class="field">
        <span>{t('groups.create.base')}</span>
        <input
          class="input mono"
          bind:value={base}
          required
          pattern="\S+"
          autocapitalize="off"
          spellcheck="false"
        />
      </label>
      <label class="field">
        <span>{t('groups.create.member')}</span>
        <input class="input" bind:value={member} required pattern=".*\S.*" maxlength="40" />
      </label>
    </div>
    <label class="field">
      <span id={`${uid}-forge`}>{t('groups.create.forge')}</span>
      <select class="input" aria-labelledby={`${uid}-forge`} bind:value={forge}>
        <option value="github">{t('groups.create.forge.github')}</option>
        <option value="none">{t('groups.create.forge.none')}</option>
      </select>
    </label>
    <label class="field">
      <span id={`${uid}-checks`}>{t('groups.create.checks')}</span>
      <textarea
        class="input mono"
        aria-labelledby={`${uid}-checks`}
        aria-describedby={`${uid}-checks-hint`}
        bind:value={checks}
        rows="2"
        placeholder={'go vet ./...\ngo test ./...'}
        autocapitalize="off"
        spellcheck="false"></textarea>
      <span class="hint" id={`${uid}-checks-hint`}>{t('groups.create.checks.hint')}</span>
    </label>
    <label class="field">
      <span id={`${uid}-home`}>{t('groups.create.home')}</span>
      <select class="input" aria-labelledby={`${uid}-home`} bind:value={chosen}>
        {#each owned as home (home)}
          <option value={home}>{home.name}</option>
        {/each}
      </select>
    </label>

    {#if failed}<Failure code={failed} />{/if}
    <footer class="row">
      <span class="grow"></span>
      <button type="button" class="btn btn-ghost" onclick={() => dialog.close()}>
        {t('groups.create.cancel')}
      </button>
      <button class="btn btn-primary" disabled={busy || !owned.length}>
        {t('groups.create.submit')}
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
  .two {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(min(100%, 180px), 1fr));
    gap: var(--sp-4);
  }
</style>
