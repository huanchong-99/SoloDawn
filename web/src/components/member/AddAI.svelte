<script lang="ts">
  import { untrack } from 'svelte';
  import { homes, type Group } from '../../lib/groups.svelte';
  import {
    candidatesOf,
    canMakeSecretary,
    joinError,
    joinGroup,
    selectableCandidates,
    suggestedName,
    type Candidate,
  } from '../../lib/members';
  import type { Points } from '../../lib/mounts.svelte';
  import { nodes } from '../../lib/nodes.svelte';
  import { accountLabel, cliName } from '../../lib/onboard.svelte';
  import { t } from '../../lib/prefs.svelte';
  import Avatar from '../Avatar.svelte';
  import Failure from '../Failure.svelte';
  import Commands from '../forge/Commands.svelte';
  import SecretaryWarning from '../groupinfo/SecretaryWarning.svelte';
  import Mount from '../Mount.svelte';
  import ProfileFields from '../profile/ProfileFields.svelte';
  import { newDraft, tooLong } from '../profile/profile';

  /** The dialog that adds a registered AI from one of the owner's computers to the group
   *  (contracts/commands.md section 2, `host.join`). */
  let {
    group,
    open = $bindable(false),
    onjoined,
  }: {
    group: Group;
    open?: boolean;
    onjoined?: (event: Points['member.joined']) => void;
  } = $props();
  const uid = $props.id();

  const owned = $derived(homes.filter((home) => home.owner && home.ctl));
  const loading = $derived(owned.some((home) => !nodes.infos.has(home)));
  const people = $derived(
    group.members.filter((member) => member.kind === 'human').map((member) => member.user),
  );

  let dialog: HTMLDialogElement;
  let name = $state('');
  let customName = $state(false);
  let secretary = $state(false);
  let busy = $state(false);
  let failed = $state('');
  let draft = $state(newDraft());

  const candidates = $derived(candidatesOf(group, owned, nodes.infos));
  const selectable = $derived(selectableCandidates(candidates));
  let chosen = $state.raw<Candidate>();
  const canSecretary = $derived(chosen?.agent !== 'external' && canMakeSecretary(group));

  $effect(() => {
    if (!open) {
      if (dialog.open) dialog.close();
      return;
    }
    untrack(() => {
      chosen = undefined;
      name = '';
      customName = false;
      secretary = false;
      failed = '';
      draft = newDraft();
      nodes.sync(homes, true);
    });
    if (!dialog.open) dialog.showModal();
  });

  $effect(() => {
    if (!open) return;
    nodes.sync(homes);
    if (!chosen && selectable.length) {
      const first = selectable[0];
      chosen = first;
      if (!untrack(() => customName)) {
        name = suggestedName(
          group,
          first.agent,
          untrack(() => secretary),
        );
      }
    }
  });

  function pickCandidate(candidate: Candidate) {
    chosen = candidate;
    if (candidate.agent === 'external') secretary = false;
    if (!customName) name = suggestedName(group, candidate.agent, secretary);
  }

  function toggleSecretary(checked: boolean) {
    secretary = checked;
    if (!customName) name = suggestedName(group, chosen?.agent, checked);
  }

  async function submit(event: SubmitEvent) {
    event.preventDefault();
    failed = '';
    const candidate = chosen;
    const trimmed = name.trim();
    if (!candidate || !group.info || !trimmed) return;
    const target = owned.find((home) => home === candidate.home);
    busy = true;
    const code = await joinGroup(target, group, candidate, trimmed, secretary, draft);
    busy = false;
    if (code === 'lost' && !target?.ctl) chosen = selectable[0];
    if (code) failed = code;
    else {
      dialog.close();
      onjoined?.({ group, candidate, name: trimmed });
    }
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
      <h2 id={`${uid}-title`}>{t('member.join.title')}</h2>
      <p class="lead">{t('member.join.lead')}</p>
    </header>

    <fieldset class="field">
      <legend>{t('member.join.cli')}</legend>
      {#if candidates.length}
        <div class="choices">
          {#each candidates as candidate (candidate.key)}
            {@const external = candidate.agent === 'external'}
            {@const disabled = !selectable.includes(candidate)}
            <label class="choice" class:disabled>
              <input
                type="radio"
                name="candidate"
                checked={chosen?.key === candidate.key}
                {disabled}
                onchange={() => pickCandidate(candidate)}
              />
              <Avatar id={candidate.key} name={cliName(candidate.agent)} cli={candidate.agent} />
              <span class="grow">
                <b>{cliName(candidate.agent)} · {candidate.hostName}</b>
                <span class="sub">
                  {#if candidate.inGroup}
                    {t('member.join.joined')}
                  {:else if external}
                    {t('external.join.hint')}
                  {:else if candidate.account === null}
                    {t('member.join.unconfigured')}
                  {:else}
                    {accountLabel(candidate.account)}{#if !candidate.loginOk}{' '}<span
                        class="warn"
                      >
                        · {t('member.join.nologin')}
                      </span>{/if}
                  {/if}
                </span>
              </span>
            </label>
          {/each}
        </div>
      {:else}
        <p class="hint" role={loading ? 'status' : undefined}>
          {t(loading ? 'member.join.loading' : 'member.join.none')}
        </p>
      {/if}
    </fieldset>

    <Mount
      at="member.add"
      props={{
        group,
        candidate: chosen,
        get draft() {
          return draft;
        },
        set draft(value) {
          draft = value;
        },
      }}
    />

    <label class="field">
      <span>{t('member.join.name')}</span>
      <input
        class="input"
        bind:value={name}
        oninput={() => (customName = true)}
        required
        pattern=".*\S.*"
        maxlength="40"
      />
    </label>

    <ProfileFields bind:draft />

    {#if canSecretary}
      <div class="sec-opt">
        <label class="check">
          <input
            type="checkbox"
            checked={secretary}
            onchange={(e) => toggleSecretary(e.currentTarget.checked)}
          />
          <span>{t('member.join.secretary')}</span>
        </label>
        {#if secretary && chosen?.account}
          <SecretaryWarning account={chosen.account} owner={chosen.home.me.user} {people} />
        {/if}
        {#if group.info?.secretary}
          <p class="hint">{t('member.join.secretary.replace')}</p>
        {/if}
      </div>
    {/if}

    {#if failed}
      {@const message = joinError(failed)}
      {#if message}
        <p class="fail" role="alert">{message}</p>
      {:else}
        <Failure code={failed} />
      {/if}
    {/if}
    {#if group.info?.kind === 'project'}
      <details class="fold">
        <summary>{t('forge.commands')}</summary>
        <div class="fold-body"><Commands info={group.info} /></div>
      </details>
    {/if}
    <footer class="row">
      <span class="grow"></span>
      <button type="button" class="btn btn-ghost" onclick={() => dialog.close()}>
        {t('member.join.cancel')}
      </button>
      <button
        class="btn btn-primary"
        disabled={busy || (!chosen && !selectable.length) || tooLong(draft)}
      >
        {t('member.join.submit')}
      </button>
    </footer>
  </form>
</dialog>

<style>
  dialog {
    overflow: auto;
  }
  .lead {
    margin-top: 4px;
    color: var(--text-2);
    font-size: var(--fs-ui);
  }
  legend {
    padding: 0;
    margin-bottom: 6px;
    font-weight: 500;
  }
  /* Each AI that can join is a radio card. */
  .choices {
    display: grid;
    gap: var(--sp-2);
    --av: 32px;
  }
  .choice {
    display: flex;
    align-items: center;
    gap: var(--sp-3);
    min-height: 56px;
    padding: 10px 14px;
    border: 1px solid var(--line);
    border-radius: 12px;
    background: var(--surface);
    cursor: pointer;
    font-size: var(--fs-sm);
    line-height: 1.45;
    color: var(--text-2);
  }
  .choice:has(:checked) {
    border-color: var(--sky);
    background: var(--sky-soft);
    box-shadow: inset 0 0 0 0.5px var(--sky);
  }
  .choice.disabled {
    opacity: 0.55;
    cursor: not-allowed;
  }
  .choice b {
    display: block;
    color: var(--text);
    font-size: var(--fs-ui);
    font-weight: 500;
  }
  .sub {
    display: block;
    font-size: var(--fs-xs);
  }
  .warn {
    color: var(--dawn-text);
  }
  .sec-opt {
    display: grid;
    gap: 4px;
  }
  .check {
    display: inline-flex;
    align-items: center;
    gap: var(--sp-2);
    font-size: var(--fs-sm);
    cursor: pointer;
  }
  .fail {
    color: var(--danger);
    font-size: var(--fs-sm);
  }
</style>
