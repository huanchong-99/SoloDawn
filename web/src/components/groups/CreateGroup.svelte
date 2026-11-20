<script lang="ts">
  import { homes } from '../../lib/groups.svelte';
  import { t } from '../../lib/prefs.svelte';
  import Failure from '../Failure.svelte';

  /** The create-group dialog (contracts/commands.md 1.1). `open` shows it. */
  let { open = $bindable(false) }: { open?: boolean } = $props();
  const uid = $props.id();

  // A group is created on a node of the person's own that can be reached.
  const owned = $derived(homes.filter((home) => home.owner && home.ctl));
  let dialog: HTMLDialogElement;
  let at = $state(0);
  let name = $state('');
  let repo = $state('');
  let base = $state('main');
  let forge = $state('github');
  let checks = $state('');
  let member = $state('');
  let busy = $state(false);
  let failed = $state('');

  $effect(() => {
    if (open) dialog.showModal();
  });

  async function submit(event: SubmitEvent) {
    event.preventDefault();
    busy = true;
    failed = '';
    const { g, answer } = await owned[at].create({
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
  onclose={() => (open = false)}
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
      <span>{t('groups.create.repo')}</span>
      <input
        class="input mono"
        bind:value={repo}
        required
        pattern=".*\S.*"
        placeholder="https://github.com/you/project.git"
        autocapitalize="off"
        spellcheck="false"
      />
      <span class="hint">{t('groups.create.repo.hint')}</span>
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
      <span>{t('groups.create.forge')}</span>
      <select class="input" bind:value={forge}>
        <option value="github">{t('groups.create.forge.github')}</option>
        <option value="none">{t('groups.create.forge.none')}</option>
      </select>
    </label>
    <label class="field">
      <span>{t('groups.create.checks')}</span>
      <textarea
        class="input mono"
        bind:value={checks}
        rows="2"
        placeholder={'go vet ./...\ngo test ./...'}
        autocapitalize="off"
        spellcheck="false"></textarea>
      <span class="hint">{t('groups.create.checks.hint')}</span>
    </label>
    <label class="field">
      <span>{t('groups.create.home')}</span>
      <select class="input" bind:value={at}>
        {#each owned as home, i (home)}
          <option value={i}>{home.name}</option>
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
    grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
    gap: var(--sp-4);
  }
</style>
