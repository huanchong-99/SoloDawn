<script lang="ts">
  import { untrack } from 'svelte';
  import { homes, type Home } from '../../lib/groups.svelte';
  import {
    candidatesOf,
    found,
    freeName,
    joinError,
    knownPeople,
    lifetimes,
    nameClash,
    unavailable,
    type Candidate,
    type Draft,
    type OpenCode,
    type Refusal,
    type Stop,
  } from '../../lib/members';
  import { nodes } from '../../lib/nodes.svelte';
  import { cliName } from '../../lib/onboard.svelte';
  import { t, type Key } from '../../lib/prefs.svelte';
  import Avatar from '../Avatar.svelte';
  import Failure from '../Failure.svelte';
  import FolderPicker from '../member/FolderPicker.svelte';
  import CodeCard from '../people/CodeCard.svelte';
  import ModelFields from '../model/ModelFields.svelte';
  import { listedModels } from '../model/model';
  import ProfileFields from '../profile/ProfileFields.svelte';
  import { newDraft } from '../profile/profile';

  /** The create-group dialog (contracts/commands.md 1.1): a name, the person's own name, the
   *  people already in one of their groups on the chosen computer, and their connected AIs, each
   *  with its folder, model, level and tier, sent in its `host.join`. The group is created first, then each person and each AI is added; a step
   *  that fails is said beside its person or AI. `open` shows it. */
  let { open = $bindable(false) }: { open?: boolean } = $props();
  const uid = $props.id();

  // A group is created on a node of the person's own that can be reached.
  const owned = $derived(homes.filter((home) => home.owner && home.ctl));
  let dialog: HTMLDialogElement;
  // The computer chosen as the home: the Home itself, not its place in a list that changes as
  // computers come and go.
  let chosen = $state.raw<Home>();
  let name = $state('');
  let member = $state('');
  let people = $state<Record<string, boolean>>({});
  /** The AIs ticked, by candidate key, with their name, folder, model, level and tier. */
  let ais = $state<Record<string, { name: string; workspace: string; draft: Draft }>>({});
  let secretary = $state('');
  let busy = $state(false);
  let failed = $state('');
  /** Whether an invitation code is made with the group, and for how many hours. */
  let coded = $state(false);
  let hours = $state<number>(24);
  /** After the group exists, when one of its people, its AIs or its code was refused, or a code
   *  was made: what became of each person and AI, by user and by key, and of the code. */
  let outcome = $state.raw<{
    g: string;
    words: Record<string, { text: string; bad: boolean }>;
    code?: OpenCode;
    /** The refusal of `code new`. */
    codeFailed?: string;
  }>();
  let clashing = $state(false);
  /** Where the code that was made, or why not, is shown. The footer sticks to the dialog's lower
   *  edge, so in a long dialog that place is scrolled into sight as soon as it is there. */
  let made = $state<HTMLElement>();
  $effect(() => made?.scrollIntoView({ block: 'nearest', behavior: 'instant' }));

  const known = $derived(chosen ? knownPeople(chosen) : []);
  // Agents the node does not start are added from a group's member panel, with their tool.
  const candidates = $derived(
    candidatesOf(
      undefined,
      homes.filter((home) => home.owner),
      nodes.infos,
    ).filter((candidate) => candidate.agent !== 'external'),
  );

  const listed = () => owned.find((home) => home === chosen);
  /** The computer chosen before, when it is still to be had, or else the first. */
  const pick = () => (chosen = listed() ?? owned[0]);

  $effect(() => {
    if (!open) return;
    untrack(() => {
      pick();
      nodes.sync(homes, true);
    });
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

  /** Ticks or unticks an AI. While any AI is ticked, one of them is the secretary: the first one
   *  ticked, until the person chooses another. */
  function tick(candidate: Candidate, on: boolean) {
    // A newly ticked AI is named so that no other ticked AI has its name.
    const taken = Object.values(ais).map((ai) => ai.name);
    if (on) {
      const name = freeName(taken, cliName(candidate.agent));
      ais[candidate.key] = { name, workspace: '', draft: newDraft() };
    } else delete ais[candidate.key];
    if (!ais[secretary]) secretary = Object.keys(ais)[0] ?? '';
  }

  const ready = $derived(Object.values(ais).every((ai) => ai.workspace && ai.name.trim()));

  async function submit(event: SubmitEvent) {
    event.preventDefault();
    if (outcome) return close();
    failed = '';
    clashing = nameClash([member, ...Object.values(ais).map((ai) => ai.name)]);
    if (clashing) return;
    // The computer may have gone since it was chosen: nothing is sent to another in its place.
    // The first one left is chosen for the next try, and the person sees which.
    const home = listed();
    if (!home) {
      failed = 'lost';
      chosen = owned[0];
      return;
    }
    busy = true;
    const users = known.filter(({ user }) => people[user]);
    const keys = Object.keys(ais);
    const made = await found(home, {
      name: name.trim(),
      member_name: member.trim(),
      people: users,
      ais: keys.map((key) => ({
        candidate: candidates.find((each) => each.key === key)!,
        name: ais[key].name.trim(),
        secretary: secretary === key,
        workspace: ais[key].workspace,
        draft: ais[key].draft,
      })),
      hours: coded ? hours : undefined,
    });
    busy = false;
    // A code that was made is shown before the dialog closes, so that it can be copied.
    if (!made.stopped && !made.code) return close(made.g);
    if (made.stopped?.step === 'group') failed = made.stopped.err.code;
    else outcome = after(made, [...users.map((each) => each.user), ...keys]);
  }

  /** What became of the people, the AIs and the code of a group that exists. */
  function after(made: { g: string; stopped?: Stop; code?: OpenCode }, ids: string[]) {
    const { g, stopped, code } = made;
    if (!stopped) return { g, words: {}, code };
    if (stopped.step === 'code') return { g, words: {}, codeFailed: stopped.err.code };
    return { g, words: told(ids, made) };
  }

  /** What became of each person and AI, by user and by key, when one of them was refused: those
   *  before it were added, those after it were not sent. */
  function told(ids: string[], { stopped }: { stopped?: Stop }) {
    const { step, index, err } = stopped!;
    const at = step === 'people' ? index : ids.length - Object.keys(ais).length + index;
    const said = (i: number) =>
      i === at
        ? refusal(err, step === 'people')
        : t(i < at ? 'groups.create.added' : 'groups.create.unsent');
    return Object.fromEntries(ids.map((id, i) => [id, { text: said(i), bad: i >= at }]));
  }

  /** Closes the dialog and opens the new group's chat, which shows as soon as it is listed. */
  function close(g = outcome!.g) {
    name = member = secretary = '';
    people = {};
    ais = {};
    coded = false;
    outcome = undefined;
    dialog.close();
    location.hash = `#/groups/${g}`;
  }

  /** Why a person (`person`) or an AI was not added. */
  function refusal(err: Refusal, person: boolean) {
    const worded = person ? t(`groups.create.person.error.${err.code}` as Key) : joinError(err);
    return worded ?? `${t('chat.error.other')} ${err.code}`;
  }
</script>

<dialog
  bind:this={dialog}
  class="dlg wide"
  aria-labelledby={`${uid}-title`}
  onclose={() => (open = false)}
>
  <form onsubmit={submit}>
    <header>
      <h2 id={`${uid}-title`}>{t('groups.new')}</h2>
      <p class="lead">{t('groups.create.lead')}</p>
    </header>

    <div class="two">
      <label class="field">
        <span>{t('groups.create.name')}</span>
        <!-- The pattern asks for one character that is no space. -->
        <input
          class="input"
          bind:value={name}
          required
          pattern=".*\S.*"
          maxlength="80"
          disabled={Boolean(outcome)}
        />
      </label>
      <label class="field">
        <span>{t('groups.create.member')}</span>
        <input
          class="input"
          bind:value={member}
          required
          pattern=".*\S.*"
          maxlength="40"
          disabled={Boolean(outcome)}
        />
      </label>
    </div>
    {#if owned.length > 1}
      <label class="field">
        <span id={`${uid}-home`}>{t('groups.create.home')}</span>
        <select
          class="input"
          aria-labelledby={`${uid}-home`}
          bind:value={chosen}
          disabled={Boolean(outcome)}
        >
          {#each owned as home (home)}
            <option value={home}>{home.name}</option>
          {/each}
        </select>
      </label>
    {/if}

    <fieldset class="part">
      <legend class="sec-title">{t('groups.create.people')} · {known.length}</legend>
      {#if known.length}<p class="hint">{t('groups.create.people.lead')}</p>{/if}
      {#each known as person (person.user)}
        {@const said = outcome?.words[person.user]}
        <label class="pick-row">
          <Avatar id={person.user} name={person.name} />
          <span class="grow">
            <b>{person.name}</b>
            {#if said}<small class:bad={said.bad}>{said.text}</small>{/if}
          </span>
          <input type="checkbox" bind:checked={people[person.user]} disabled={Boolean(outcome)} />
        </label>
      {:else}
        <p class="hint">{t('groups.create.people.none')}</p>
      {/each}
      <p class="hint">{t('groups.create.people.hint')}</p>
      <div class="code-pick">
        <label class="check">
          <input type="checkbox" bind:checked={coded} disabled={Boolean(outcome)} />
          <span>{t('groups.create.code')}</span>
        </label>
        {#if coded && !outcome?.code}
          <fieldset
            class="seg"
            style:--at={(lifetimes as readonly number[]).indexOf(hours)}
            disabled={Boolean(outcome)}
          >
            <legend class="sr">{t('people.code.hours')}</legend>
            {#each lifetimes as each (each)}
              <label>
                <input type="radio" name={`${uid}-hours`} value={each} bind:group={hours} />
                <span>{t(`people.code.hours.${each}`)}</span>
              </label>
            {/each}
          </fieldset>
        {/if}
      </div>
    </fieldset>

    <fieldset class="part">
      <legend class="sec-title">{t('groups.create.ais')} · {candidates.length}</legend>
      {#each candidates as candidate (candidate.key)}
        {@const why = unavailable(candidate)}
        {@const ai = ais[candidate.key]}
        {@const said = outcome?.words[candidate.key]}
        <div class="ai" class:on={ai}>
          <label class="pick-row" class:dim={why}>
            <Avatar id={candidate.key} name={cliName(candidate.agent)} cli={candidate.agent} />
            <span class="grow">
              <b>{cliName(candidate.agent)}</b>
              <small>
                {candidate.hostName}{#if why ?? said}{' '}·
                  <span class:bad={said?.bad}>{why ?? said?.text}</span>{/if}
              </small>
            </span>
            <input
              type="checkbox"
              checked={Boolean(ai)}
              disabled={Boolean(why || outcome)}
              onchange={(event) => tick(candidate, event.currentTarget.checked)}
            />
          </label>
          {#if ai}
            <div class="ai-fields">
              <FolderPicker
                home={candidate.home}
                agent={candidate.agent}
                ai={ai.name || cliName(candidate.agent)}
                computer={candidate.hostName}
                bind:workspace={ai.workspace}
              />
              <label class="field">
                <span>
                  {t('groups.create.ai.name').replace('{ai}', () => cliName(candidate.agent))}
                </span>
                <input
                  class="input"
                  bind:value={ai.name}
                  required
                  pattern=".*\S.*"
                  maxlength="40"
                  disabled={Boolean(outcome)}
                />
              </label>
              <ModelFields
                bind:draft={ai.draft}
                models={listedModels(candidate.home, candidate.agent) ?? []}
                suggest
                disabled={Boolean(outcome)}
              />
              <ProfileFields
                bind:draft={ai.draft}
                skills={false}
                share={secretary !== candidate.key}
                disabled={Boolean(outcome)}
              />
              <label class="check">
                <input
                  type="radio"
                  name={`${uid}-secretary`}
                  checked={secretary === candidate.key}
                  disabled={Boolean(outcome)}
                  onchange={() => (secretary = candidate.key)}
                />
                <span>
                  {t('groups.create.ai.secretary').replace(
                    '{ai}',
                    () => ai.name.trim() || cliName(candidate.agent),
                  )}
                </span>
              </label>
            </div>
          {/if}
        </div>
      {:else}
        <p class="hint">{t('groups.create.ais.none')}</p>
      {/each}
    </fieldset>

    {#if failed}<Failure code={failed} />{/if}
    {#if clashing}<p class="hint bad" role="alert">{t('groups.create.names')}</p>{/if}
    <!-- Not when everyone was added and only the code was refused: that is said on its own. -->
    {#if outcome && !outcome.code && !outcome.codeFailed}
      <p class="hint warn" role="status">{t('groups.create.partly')}</p>
    {/if}
    <!-- The code that was made, or why not, beside the button the person just pressed. -->
    {#if outcome?.code || outcome?.codeFailed}
      <div class="made" bind:this={made}>
        {#if outcome.code}
          <CodeCard code={outcome.code} group={name.trim()} />
        {:else}
          <Failure code={outcome.codeFailed!} />
        {/if}
      </div>
    {/if}
    <footer class="row">
      <span class="grow"></span>
      {#if !outcome}
        <button type="button" class="btn btn-ghost" onclick={() => dialog.close()}>
          {t('groups.create.cancel')}
        </button>
      {/if}
      <button class="btn btn-primary" disabled={busy || !owned.length || !ready}>
        {t(outcome ? 'groups.create.open' : 'groups.create.submit')}
      </button>
    </footer>
  </form>
</dialog>

<style>
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
  .lead {
    margin-top: 4px;
    color: var(--text-2);
    font-size: var(--fs-ui);
  }
  .two {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(min(100%, 220px), 1fr));
    gap: var(--sp-4);
  }
  .part {
    display: grid;
    gap: 0;
    --av: 32px;
  }
  .part legend {
    margin-bottom: var(--sp-1);
  }
  .part > .hint {
    margin-top: var(--sp-1);
  }
  .pick-row input {
    margin: 0 4px 0 0;
  }
  /* A row that cannot be ticked fades its person or AI; the reason beside it stays in full. */
  .pick-row.dim {
    cursor: not-allowed;
  }
  .pick-row.dim b,
  .pick-row.dim :global(.av) {
    opacity: 0.55;
  }
  .ai + .ai {
    border-top: 1px solid var(--line);
  }
  .ai-fields {
    display: grid;
    gap: var(--sp-3);
    padding: 0 var(--sp-1) var(--sp-3) calc(var(--av) + var(--sp-3) + var(--sp-1));
    animation:
      rise var(--dur-2) var(--ease-out),
      appear var(--dur-2) var(--ease-out);
  }
  @media (max-width: 480px) {
    .ai-fields {
      padding-left: var(--sp-1);
    }
  }
  .bad {
    color: var(--danger);
  }
  .warn {
    color: var(--dawn-text);
  }
  .code-pick {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--sp-2) var(--sp-4);
    min-height: 44px;
    margin-top: var(--sp-2);
  }
  /* Clear of the sticky footer: 69 px, 77 px on touch. */
  .made {
    margin-top: var(--sp-2);
    scroll-margin-bottom: 96px;
  }
  .code-pick .seg {
    animation:
      rise var(--dur-2) var(--ease-out),
      appear var(--dur-2) var(--ease-out);
  }
  .check {
    display: inline-flex;
    align-items: center;
    gap: var(--sp-2);
    font-size: var(--fs-sm);
    cursor: pointer;
  }
</style>
