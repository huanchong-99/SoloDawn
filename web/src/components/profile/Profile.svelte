<script lang="ts">
  import { tick, type Snippet } from 'svelte';
  import { homes, type Group } from '../../lib/groups.svelte';
  import { t } from '../../lib/prefs.svelte';
  import type { AIMemberRow } from '../../lib/proto';
  import { hostHomeOf } from '../../lib/terminal.svelte';
  import Failure from '../Failure.svelte';
  import { changed } from '../changed';
  import ModelFields from '../model/ModelFields.svelte';
  import { listedModels, modelChanges } from '../model/model';
  import ProfileFields from './ProfileFields.svelte';
  import TierMark from './TierMark.svelte';
  import {
    memberSetting,
    newDraft,
    profileChanges,
    profileSet,
    restoreDefaults,
    tooLong,
    type Draft,
    type MemberUpdate,
    type Tier,
  } from './profile';

  /** Tier and skills of an AI member card, with the suggestion, and its owner's editor of model,
   *  level, tier and skills (contracts/step-3/profiles.md section 2a). A secretary takes no tasks:
   *  its editor has no tier. */
  let {
    group,
    ai,
    suggested,
    children,
  }: {
    group: Group;
    ai: AIMemberRow;
    suggested: string | undefined;
    children: Snippet<[Snippet, Snippet, Snippet]>;
  } = $props();

  let editing = $state(false);
  let draft = $state<Draft>(newDraft());
  let editButton = $state<HTMLButtonElement>();
  let form = $state<HTMLFormElement>();
  let busy = $state(false);
  let failed = $state('');

  const mine = $derived(ai.user === group.home.me.user);
  const offline = $derived(!group.home.ctl);
  const models = $derived(listedModels(hostHomeOf(homes, ai.host), ai.cli) ?? []);
  const changes = $derived({ ...modelChanges(ai, draft), ...profileChanges(ai, draft) });
  const setting = $derived(memberSetting(ai, mine));
  const offer = $derived(mine && suggested !== undefined && suggested !== ai.tier);
  const label = $derived(
    t('profile.suggest').replace('{tier}', t(`profile.tier.${suggested as Tier}`)),
  );

  async function send(updates: MemberUpdate) {
    busy = true;
    failed = '';
    const answer = await group.home.command(profileSet(group.g, ai.id, updates));
    busy = false;
    if (answer.err) failed = answer.err.code;
    return !answer.err;
  }

  async function edit() {
    draft = { model: ai.model, effort: ai.effort, tier: ai.tier, skills: ai.skills };
    failed = '';
    editing = true;
    await tick();
    form?.focus();
  }

  async function close() {
    editing = false;
    failed = '';
    await tick();
    editButton?.focus();
  }
</script>

{#snippet share()}
  <dt>{t('profile.tier')}</dt>
  <dd class="tier-row">
    <TierMark tier={ai.tier} />
    {#if offer}
      <button
        type="button"
        class="suggest"
        disabled={busy || offline}
        title={label}
        onclick={() => send({ tier: suggested! })}
      >
        <span>{t(`profile.tier.${suggested as Tier}`)}</span>
        <b>{t('profile.suggest.use')}</b>
      </button>
    {/if}
  </dd>
{/snippet}

{#snippet skillsRow()}
  <dt>{t('profile.skills')}</dt>
  <dd class="skills" class:muted={!ai.skills}>{ai.skills || t('profile.skills.none')}</dd>
  {#if setting.shown}
    <dt>{t('defaults.setting')}</dt>
    <dd class="setting" class:muted={setting.follow} use:changed={ai.follow}>
      {#if setting.follow}
        <span class="follow-note">
          {t('defaults.follow.card').replace('{computer}', () => ai.host_name)}
        </span>
      {:else}
        <span class="badge">{t('defaults.own')}</span>
        {#if setting.restore}
          <button
            type="button"
            class="btn btn-sm btn-ghost"
            disabled={busy || offline}
            onclick={() => send(restoreDefaults())}
          >
            {t('defaults.restore')}
          </button>
        {/if}
      {/if}
    </dd>
  {/if}
{/snippet}

{#snippet editProfile()}
  {#if mine && !editing}
    <button bind:this={editButton} type="button" class="btn btn-sm" onclick={edit}>
      {t('profile.edit')}
    </button>
  {/if}
{/snippet}

{@render children(share, skillsRow, editProfile)}

{#if editing}
  <form
    bind:this={form}
    class="editor"
    aria-label={t('profile.edit')}
    tabindex="-1"
    onsubmit={async (event) => {
      event.preventDefault();
      if (await send(changes)) await close();
    }}
  >
    {#if setting.shown && setting.follow}
      <p class="hint">{t('defaults.edit.note')}</p>
    {/if}
    {#if ai.cli !== 'external'}
      <ModelFields bind:draft {models} disabled={busy || offline} />
    {/if}
    <ProfileFields bind:draft {ai} share={!ai.secretary} disabled={busy || offline} />
    <footer class="row">
      <span class="grow"></span>
      <button type="button" class="btn btn-sm btn-ghost" onclick={close}>
        {t('profile.cancel')}
      </button>
      <button
        class="btn btn-sm btn-primary"
        disabled={busy || offline || !Object.keys(changes).length || tooLong(draft)}
      >
        {t('profile.save')}
      </button>
    </footer>
  </form>
{/if}
{#if failed}<Failure code={failed} />{/if}

<style>
  .tier-row {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 6px;
  }
  .skills {
    overflow-wrap: anywhere;
  }
  .setting {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 4px 6px;
  }
  .setting:global([data-changed]) > * {
    animation: -global-land var(--dur-4) var(--spring);
  }
  .editor .hint {
    display: block;
    font-size: var(--fs-xs);
    color: var(--text-3);
  }
  .suggest {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    height: 22px;
    padding: 0 3px 0 8px;
    border: 1px solid var(--line);
    border-radius: var(--r-full);
    background: none;
    color: var(--text-3);
    font: inherit;
    font-size: var(--fs-xs);
    white-space: nowrap;
    cursor: pointer;
    animation: -global-appear var(--dur-2) var(--ease-out);
  }
  .suggest b {
    display: inline-grid;
    place-items: center;
    height: 16px;
    padding: 0 7px;
    border-radius: var(--r-full);
    background: var(--sky-soft);
    color: var(--sky);
    font-weight: 500;
  }
  .suggest:hover:not(:disabled) {
    border-color: var(--sky);
    color: var(--text-2);
  }
  .suggest:disabled {
    cursor: not-allowed;
    opacity: 0.6;
  }
  .editor {
    display: grid;
    gap: var(--sp-3);
    padding: var(--sp-3);
    border-radius: var(--r-md);
    background: var(--surface-2);
    animation: -global-appear var(--dur-2) var(--ease-out);
  }
  .editor:focus {
    outline: none;
  }
</style>
