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
  import { nodes } from '../../lib/nodes.svelte';
  import { accountLabel, cliName } from '../../lib/onboard.svelte';
  import { t } from '../../lib/prefs.svelte';
  import Avatar from '../Avatar.svelte';
  import Failure from '../Failure.svelte';
  import Commands from '../forge/Commands.svelte';
  import SecretaryWarning from '../groupinfo/SecretaryWarning.svelte';

  /** The dialog that adds a registered AI from one of the owner's computers to the group
   *  (contracts/commands.md section 2, `host.join`). */
  let { group, open = $bindable(false) }: { group: Group; open?: boolean } = $props();
  const uid = $props.id();

  const owned = $derived(homes.filter((home) => home.owner && home.ctl));
  const loading = $derived(owned.some((home) => !nodes.infos.has(home)));
  const canSecretary = $derived(canMakeSecretary(group));
  const people = $derived(
    group.members.filter((member) => member.kind === 'human').map((member) => member.user),
  );

  let dialog: HTMLDialogElement;
  let name = $state('');
  let customName = $state(false);
  let secretary = $state(false);
  let busy = $state(false);
  let failed = $state('');

  const candidates = $derived(candidatesOf(group, owned, nodes.infos));
  const selectable = $derived(selectableCandidates(candidates));
  let chosen = $state.raw<Candidate>();

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
    if (!customName) name = suggestedName(group, candidate.agent, secretary);
  }

  function toggleSecretary(checked: boolean) {
    secretary = checked;
    if (!customName) name = suggestedName(group, chosen?.agent, checked);
  }

  async function submit(event: SubmitEvent) {
    event.preventDefault();
    failed = '';
    if (!chosen || !group.info) return;
    const trimmed = name.trim();
    if (!trimmed) return;
    const target = owned.find((home) => home === chosen!.home);
    busy = true;
    const code = await joinGroup(target, group, chosen, trimmed, secretary);
    busy = false;
    if (code === 'lost' && !target?.ctl) chosen = selectable[0];
    if (code) failed = code;
    else dialog.close();
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
            {@const disabled = candidate.account === null || candidate.inGroup}
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
      <button class="btn btn-primary" disabled={busy || (!chosen && !selectable.length)}>
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
    margin-top: 6px;
    color: var(--text-2);
    font-size: var(--fs-sm);
  }
  legend {
    padding: 0;
    margin-bottom: 6px;
    font-weight: 500;
  }
  .choices {
    display: grid;
    gap: var(--sp-2);
  }
  .choice {
    display: flex;
    align-items: center;
    gap: var(--sp-3);
    padding: 10px 12px;
    border: 1px solid var(--line);
    border-radius: var(--r-md);
    cursor: pointer;
    font-size: var(--fs-sm);
    color: var(--text-2);
  }
  .choice:has(:checked) {
    border-color: var(--sky);
    background: var(--sky-soft);
  }
  .choice.disabled {
    border-style: dashed;
    background: var(--surface-2);
    color: var(--text-3);
    cursor: not-allowed;
  }
  .choice input {
    margin: 0;
    accent-color: var(--sky);
  }
  .choice b {
    display: block;
    color: var(--text);
    font-size: 14px;
  }
  .choice.disabled b {
    color: var(--text-3);
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
  .check input {
    width: 16px;
    height: 16px;
    margin: 0;
    accent-color: var(--sky);
  }
  .fail {
    color: var(--danger);
    font-size: var(--fs-sm);
  }
</style>
