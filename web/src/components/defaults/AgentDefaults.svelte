<script lang="ts">
  import { tick } from 'svelte';
  import { nodes, type Computer } from '../../lib/nodes.svelte';
  import { cliName } from '../../lib/onboard.svelte';
  import { t } from '../../lib/prefs.svelte';
  import type * as p from '../../lib/proto';
  import Failure from '../Failure.svelte';
  import Icon from '../Icon.svelte';
  import ModelFields from '../model/ModelFields.svelte';
  import Bars from '../profile/Bars.svelte';
  import ProfileFields from '../profile/ProfileFields.svelte';
  import { tooLong } from '../profile/profile';
  import { draftOf, followers, sameProfile, summaryParts } from './defaults';

  /** Computer-level CLI defaults row and inline editor inside an AI row on the Computers page
   *  (planning/contracts/step-3/profiles.md section 2a). */
  let {
    comp,
    agent,
  }: {
    comp: Computer;
    agent: p.HostInfoPush['agents'][number];
  } = $props();

  const uid = $props.id();

  let editing = $state(false);
  let draft = $state<p.Profile>(draftOf(undefined));
  let saving = $state(false);
  let failed = $state('');
  let landed = $state(0);
  let toggleBtn = $state<HTMLButtonElement>();
  let form = $state<HTMLFormElement>();

  const label = $derived(cliName(agent.agent));
  const summary = $derived(agent.defaults ? summaryParts(agent.defaults, agent.models) : undefined);
  const count = $derived(followers(comp.info!.device, agent.agent));
  const unchanged = $derived(agent.defaults !== undefined && sameProfile(draft, agent.defaults));
  const cannotSave = $derived(!comp.online || saving || tooLong(draft) || unchanged);

  function toggle() {
    if (editing) {
      editing = false;
      failed = '';
      return;
    }
    draft = draftOf(agent.defaults);
    failed = '';
    editing = true;
  }

  async function cancel() {
    editing = false;
    failed = '';
    await tick();
    toggleBtn?.focus();
  }

  async function save(event: SubmitEvent) {
    event.preventDefault();
    if (cannotSave) return;
    saving = true;
    failed = '';
    const answer = await nodes.setDefaults(comp.home, comp.info!.device, agent.agent, draft);
    saving = false;
    if (answer.err) {
      failed = answer.err.code;
      await tick();
      if (document.activeElement === document.body) form?.focus();
      return;
    }
    landed += 1;
    editing = false;
    await tick();
    toggleBtn?.focus();
  }
</script>

<div class="defaults">
  <div class="def-head">
    <span class="lbl">{t('defaults.title')}</span>
    {#if !agent.defaults}
      <span class="badge" data-tone="dawn">{t('defaults.unset')}</span>
    {/if}
    <span class="grow"></span>
    <button
      bind:this={toggleBtn}
      type="button"
      data-guide="defaults"
      data-unset={!agent.defaults ? '' : undefined}
      class={['btn', 'btn-sm', agent.defaults && 'btn-ghost turn']}
      aria-expanded={editing}
      aria-controls={editing ? `${uid}-editor` : undefined}
      aria-label={`${t(agent.defaults ? 'defaults.edit' : 'defaults.set')} · ${label}`}
      onclick={toggle}
    >
      {t(agent.defaults ? 'defaults.edit' : 'defaults.set')}{#if agent.defaults}<Icon
          name="down"
          sm
        />{/if}
    </button>
  </div>

  {#key landed}
    {#if summary}
      <p class="sum" class:land={landed > 0}>
        <span>{summary.model}</span>{' '}<span class="sep" aria-hidden="true">·</span>{' '}<span>
          {summary.effort}
        </span>{' '}<span class="sep" aria-hidden="true">·</span>{' '}<span class="sum-tier">
          {t('profile.tier')}
          <Bars tier={summary.tier} />
          {summary.tierLabel}
        </span>{#if summary.skills}{' '}<span class="sep" aria-hidden="true">·</span>{' '}<span>
            {summary.skills}
          </span>{/if}
      </p>
    {:else}
      <p class="sum" class:land={landed > 0}>{t('defaults.unset.line')}</p>
    {/if}
  {/key}

  {#if editing}
    <form
      id={`${uid}-editor`}
      bind:this={form}
      class="editor"
      tabindex="-1"
      aria-label={`${t('defaults.title')} · ${label}`}
      onsubmit={save}
    >
      <p class="hint">{t('defaults.lead').replace('{ai}', () => label)}</p>
      <ModelFields bind:draft models={agent.models} suggest disabled={saving} />
      <ProfileFields bind:draft share skills disabled={saving} />
      <footer class="foot">
        <p class="hint">
          {count > 0
            ? t('defaults.followers').replace('{n}', String(count))
            : t('defaults.followers.none')}
        </p>
        <button type="button" class="btn btn-sm btn-ghost" disabled={saving} onclick={cancel}>
          {t('defaults.cancel')}
        </button>
        <button class="btn btn-sm btn-primary" disabled={cannotSave}>
          {t('defaults.save')}
        </button>
      </footer>
    </form>
    {#if failed}<Failure code={failed} />{/if}
  {/if}
</div>

<style>
  .defaults {
    display: grid;
    gap: var(--sp-2);
    min-width: 0;
  }
  .def-head {
    display: flex;
    align-items: center;
    gap: var(--sp-2);
    min-width: 0;
  }
  .def-head .lbl {
    font-size: var(--fs-sm);
    font-weight: 500;
    color: var(--text-2);
    white-space: nowrap;
  }
  .turn {
    margin-right: -6px;
  }
  .turn :global(.i) {
    transition: transform var(--dur-2) var(--ease);
  }
  .turn[aria-expanded='true'] :global(.i) {
    transform: rotate(180deg);
  }
  .sum {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 4px 6px;
    font-size: var(--fs-sm);
    color: var(--text-2);
    overflow-wrap: anywhere;
  }
  .sum.land {
    animation:
      rise var(--dur-2) var(--ease-out),
      appear var(--dur-2) var(--ease-out);
  }
  .sum .sep {
    color: var(--text-3);
  }
  .sum-tier {
    display: inline-flex;
    align-items: center;
    gap: 6px;
  }
  .editor {
    display: grid;
    gap: var(--sp-3);
    min-width: 0;
    margin-top: var(--sp-1);
    padding: var(--sp-3);
    border-radius: var(--r-md);
    background: var(--surface-2);
    animation: appear var(--dur-2) var(--ease-out);
  }
  .editor:focus {
    outline: none;
  }
  .editor .hint {
    display: block;
    font-size: var(--fs-xs);
    color: var(--text-3);
  }
  .foot {
    display: flex;
    align-items: center;
    justify-content: flex-end;
    gap: var(--sp-2);
    flex-wrap: wrap;
  }
  .foot .hint {
    flex: 1;
    min-width: 180px;
  }
  @media (max-width: 640px) {
    .def-head .btn,
    .foot .btn {
      min-height: 44px;
    }
    .foot .hint {
      flex-basis: 100%;
      min-width: 0;
    }
  }
  @media (pointer: coarse) {
    .def-head .btn,
    .foot .btn {
      min-height: 44px;
    }
  }
</style>
