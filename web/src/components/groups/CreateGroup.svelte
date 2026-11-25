<script lang="ts">
  import { untrack } from 'svelte';
  import { homes, type Home, type NewGroup } from '../../lib/groups.svelte';
  import { addHome } from '../../lib/join';
  import { signalUrlOf } from '../../lib/onboard.svelte';
  import { t } from '../../lib/prefs.svelte';
  import Failure from '../Failure.svelte';
  import ForgeChoice from '../forge/ForgeChoice.svelte';
  import { emptyTemplates } from '../forge/templates';
  import Icon from '../Icon.svelte';

  /** The create-group dialog (contracts/commands.md 1.1; contracts/step-2/watch-team.md
   *  section 7). `open` shows it. */
  let { open = $bindable(false) }: { open?: boolean } = $props();
  const uid = $props.id();

  // A group is created on a node of the person's own that can be reached.
  const owned = $derived(homes.filter((home) => home.owner && home.ctl));
  let dialog: HTMLDialogElement;
  // The computer chosen as the home: the Home itself, not its place in a list that changes as
  // computers come and go.
  let chosen = $state.raw<Home>();
  let kind = $state<NewGroup['kind']>('project');
  let name = $state('');
  let repo = $state('');
  let base = $state('main');
  let forge = $state('github');
  let templates = $state(emptyTemplates());
  let checks = $state('');
  let member = $state('');
  let busy = $state(false);
  let failed = $state('');
  // A group that was created, when some of the person's computers were not told its home.
  let untold = $state('');

  const kinds = [
    { kind: 'project', icon: 'fork' },
    { kind: 'team', icon: 'users' },
  ] as const;

  const listed = () => owned.find((home) => home === chosen);
  /** The computer chosen before, when it is still to be had, or else the first. */
  const pick = () => (chosen = listed() ?? owned[0]);

  $effect(() => {
    if (!open) return;
    untrack(pick);
    dialog.showModal();
  });
  // A computer that appears while there was none to choose from is chosen: the select shows it,
  // and what the select shows is what is sent to.
  let none = true;
  $effect(() => {
    const some = owned.length > 0;
    if (some && none) untrack(pick);
    none = !some;
  });

  /** What a group of the chosen kind is created with beside the names. */
  function fields() {
    if (kind === 'team') return { kind, base: '', forge: 'none', checks: [] };
    return {
      kind,
      repo: repo.trim(),
      base: base.trim(),
      forge,
      ...(forge === 'custom' ? { templates } : {}),
      checks: checks
        .split('\n')
        .map((line) => line.trim())
        .filter(Boolean),
    };
  }

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
    const title = name.trim();
    const { g, answer } = await home.create({
      name: title,
      ...fields(),
      member_name: member.trim(),
    });
    if (answer.err) {
      busy = false;
      failed = answer.err.code;
      return;
    }
    const missed = kind === 'team' ? await announce(home, g, title) : [];
    busy = false;
    name = repo = checks = '';
    templates = emptyTemplates();
    kind = 'project';
    if (missed.length) untold = g;
    else show(g);
  }

  /** The person's other computers learn a team group from this browser alone: no AI joins it. */
  const announce = (home: Home, g: string, title: string) =>
    addHome(
      owned.filter((node) => node !== home),
      { g, home: home.device },
      title,
      signalUrlOf(),
    );

  function show(g: string) {
    dialog.close();
    // The group itself arrives in a push; its chat opens as soon as it is listed.
    location.hash = `#/groups/${g}`;
  }
</script>

<dialog
  bind:this={dialog}
  class="dlg"
  aria-labelledby={`${uid}-title`}
  onclose={() => {
    open = false;
    untold = '';
  }}
>
  {#if untold}
    <div class="told">
      <h2 id={`${uid}-title`}>{t('groups.new')}</h2>
      <p role="status">{t('groups.create.notifications')}</p>
      <footer class="row">
        <span class="grow"></span>
        <button type="button" class="btn btn-primary" onclick={() => show(untold)}>
          {t('groups.create.open')}
        </button>
      </footer>
    </div>
  {:else}
    <form onsubmit={submit}>
      <header>
        <h2 id={`${uid}-title`}>{t('groups.new')}</h2>
        <p class="lead">{t('groups.create.lead')}</p>
      </header>

      <fieldset class="acct kinds">
        <legend class="sr">{t('groups.create.kind')}</legend>
        {#each kinds as { kind: each, icon } (each)}
          <label>
            <input
              type="radio"
              name={`${uid}-kind`}
              value={each}
              aria-labelledby={`${uid}-${each}`}
              aria-describedby={`${uid}-${each}-hint`}
              bind:group={kind}
            />
            <b id={`${uid}-${each}`}>{t(`groups.create.kind.${each}`)}</b>
            <Icon name={icon} />
            <span id={`${uid}-${each}-hint`}>{t(`groups.create.kind.${each}.hint`)}</span>
          </label>
        {/each}
      </fieldset>
      <label class="field">
        <span>{t('groups.create.name')}</span>
        <!-- The pattern asks for one character that is no space. -->
        <input class="input" bind:value={name} required pattern=".*\S.*" maxlength="80" />
      </label>
      {#if kind === 'project'}
        <label class="field arrives">
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
      {/if}
      <div class="two">
        {#if kind === 'project'}
          <label class="field arrives">
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
        {/if}
        <label class="field">
          <span>{t('groups.create.member')}</span>
          <input class="input" bind:value={member} required pattern=".*\S.*" maxlength="40" />
        </label>
      </div>
      {#if kind === 'project'}
        <div class="part arrives">
          <ForgeChoice bind:forge bind:templates />
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
        </div>
      {/if}
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
  {/if}
</dialog>

<style>
  /* Held by its top: the choice of kind stays under the pointer while the fields below change. */
  dialog {
    --top: clamp(16px, 8dvh - 40px, 56px);
    margin-top: var(--top);
    max-height: calc(100dvh - 2 * var(--top));
    overflow: auto;
  }
  .told,
  .part {
    display: grid;
    gap: var(--sp-4);
  }
  .lead {
    margin-top: 6px;
    color: var(--text-2);
    font-size: var(--fs-sm);
  }
  .two,
  .kinds {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(min(100%, 180px), 1fr));
    gap: var(--sp-4);
  }
  .kinds {
    gap: var(--sp-2);
  }
  .kinds label {
    display: grid;
    grid-template-columns: auto minmax(0, 1fr) auto;
    align-items: center;
    gap: 2px var(--sp-2);
  }
  .kinds input {
    margin: 0;
  }
  .kinds span {
    grid-column: 1 / -1;
  }
  .kinds :global(.i) {
    color: var(--text-3);
    transition: transform var(--dur-4) var(--spring);
  }
  .kinds label:has(:checked) :global(.i) {
    color: var(--sky);
    transform: scale(1.15);
  }
  .arrives {
    animation:
      rise var(--dur-3) var(--ease-out),
      appear var(--dur-2) var(--ease-out);
  }
</style>
