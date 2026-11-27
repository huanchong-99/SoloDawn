<script lang="ts">
  import { Editing, setGroup } from '../../lib/groupinfo.svelte';
  import type { Group } from '../../lib/groups.svelte';
  import { isAdmin } from '../../lib/members';
  import { t } from '../../lib/prefs.svelte';
  import EditButton from './EditButton.svelte';
  import EditField from './EditField.svelte';
  import EditGroup from './EditGroup.svelte';
  import SecretaryWarning from './SecretaryWarning.svelte';
  import { homeOwner, secretaryCandidates } from './trust';

  /** The group's name and secretary, which an admin changes where they are shown (commands.md
   *  1.1 `group set`), its home, and the dialog of the rest of its settings. */
  let { group }: { group: Group } = $props();
  const uid = $props.id();
  const nameId = `${uid}-name`;
  const secretaryId = `${uid}-secretary`;
  let dialog = $state(false);
  const editing = new Editing();
  const owner = $derived(homeOwner(group));
  const ownerName = $derived(
    owner ? (group.personName(owner) ?? owner) : t('groupinfo.home.unknown'),
  );
  const info = $derived(group.info);
  const admin = $derived(isAdmin(group));
  const secretary = $derived(info?.secretary ? group.member(info.secretary) : undefined);
  const candidates = $derived(
    secretaryCandidates(group).map((ai) => ({ value: ai.id, label: ai.name })),
  );
  const people = $derived(
    group.members.filter((member) => member.kind === 'human').map((member) => member.user),
  );
  const close = () => void editing.close();
</script>

<section class="group-details field" aria-labelledby={`${uid}-title`}>
  <h2 class="sec-title" id={`${uid}-title`}>{t('groupinfo.details')}</h2>
  <dl class="kv">
    {#if info}
      <dt>{t('groupinfo.name')}</dt>
      <dd>
        {#if editing.id === nameId}
          <EditField
            fields={[{ label: t('groupinfo.name'), value: info.name, maxlength: 80 }]}
            save={([name]) => setGroup(group, { name: name.trim() })}
            onclose={close}
          />
        {:else}
          <span class="value">
            <span class="grow">{info.name}</span>
            {#if admin}
              <EditButton
                id={nameId}
                label={t('groupinfo.name.edit')}
                {editing}
                disabled={!group.home.ctl}
              />
            {/if}
          </span>
        {/if}
      </dd>
      {#if secretary?.kind === 'ai' || (admin && candidates.length)}
        <dt>{t('groupinfo.secretary')}</dt>
        <dd>
          {#if editing.id === secretaryId}
            <EditField
              fields={[
                {
                  label: t('groupinfo.secretary'),
                  value: secretary?.id ?? '',
                  options: candidates,
                  placeholder: t('groupinfo.secretary.choose'),
                },
              ]}
              reasons={{ conflict: t('groupinfo.secretary.conflict') }}
              save={([id]) => setGroup(group, { secretary: id })}
              onclose={close}
            />
          {:else}
            <span class="value">
              <span class="grow">{secretary?.name ?? t('groupinfo.secretary.none')}</span>
              {#if admin && candidates.length}
                <EditButton
                  id={secretaryId}
                  label={t('groupinfo.secretary.edit')}
                  {editing}
                  disabled={!group.home.ctl}
                />
              {/if}
            </span>
          {/if}
        </dd>
      {/if}
    {/if}
    {#if group.home.name}
      <dt>{t('groupinfo.home')}</dt>
      <dd>{group.home.name}</dd>
    {/if}
    <dt>{t('groupinfo.home.owner')}</dt>
    <dd>{ownerName}</dd>
  </dl>
  {#if !group.home.owner}<p class="hint">{t('groupinfo.home.foreign')}</p>{/if}
  {#if secretary?.kind === 'ai'}
    <SecretaryWarning account={secretary.account} owner={secretary.user} {people} />
  {/if}
  {#if info}
    {#if admin}
      <button
        type="button"
        class="btn btn-sm"
        disabled={!group.home.ctl}
        onclick={() => (dialog = true)}
      >
        {t('groupinfo.edit')}
      </button>
    {/if}
    {#if dialog}<EditGroup {group} {info} onclose={() => (dialog = false)} />{/if}
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
  /* A value and the button that changes it share the line. */
  .value {
    display: flex;
    align-items: center;
    gap: var(--sp-2);
  }
  .grow {
    min-width: 0;
    overflow-wrap: anywhere;
  }
</style>
