<script lang="ts">
  import type { Group } from '../../lib/groups.svelte';
  import { isAdmin } from '../../lib/members';
  import { t } from '../../lib/prefs.svelte';
  import Commands from '../forge/Commands.svelte';
  import EditGroup from './EditGroup.svelte';
  import SecretaryWarning from './SecretaryWarning.svelte';
  import { homeOwner, secretaryCandidates } from './trust';

  let { group }: { group: Group } = $props();
  const uid = $props.id();
  let editing = $state(false);
  const owner = $derived(homeOwner(group));
  const ownerName = $derived(
    owner ? (group.personName(owner) ?? owner) : t('groupinfo.home.unknown'),
  );
  const info = $derived(group.info);
  const secretary = $derived(info?.secretary ? group.member(info.secretary) : undefined);
  const people = $derived(
    group.members.filter((member) => member.kind === 'human').map((member) => member.user),
  );
  const editable = $derived(info?.forge === 'custom' || secretaryCandidates(group).length > 0);
</script>

<section class="group-details field" aria-labelledby={`${uid}-title`}>
  <h2 class="sec-title" id={`${uid}-title`}>{t('groupinfo.details')}</h2>
  <dl class="kv">
    <dt>{t('groupinfo.home')}</dt>
    <dd>{group.home.name}</dd>
    <dt>{t('groupinfo.home.owner')}</dt>
    <dd>{ownerName}</dd>
  </dl>
  {#if !group.home.owner}<p class="hint">{t('groupinfo.home.foreign')}</p>{/if}
  {#if info?.kind === 'project'}
    <details class="fold">
      <summary>{t('forge.commands')}</summary>
      <div class="fold-body"><Commands {info} /></div>
    </details>
    {#if secretary?.kind === 'ai'}
      <p>{t('groupinfo.secretary')} · {secretary.name}</p>
      <SecretaryWarning account={secretary.account} owner={secretary.user} {people} />
    {/if}
    {#if editable && isAdmin(group)}
      <button
        type="button"
        class="btn btn-sm"
        disabled={!group.home.ctl}
        onclick={() => (editing = true)}
      >
        {t('groupinfo.edit')}
      </button>
    {/if}
    {#if editing}<EditGroup {group} {info} onclose={() => (editing = false)} />{/if}
  {/if}
</section>

<style>
  .group-details {
    display: flex;
    flex-direction: column;
    gap: var(--sp-2);
    padding: var(--sp-4) var(--sp-4) 14px;
    border-top: 1px solid var(--line);
  }
</style>
