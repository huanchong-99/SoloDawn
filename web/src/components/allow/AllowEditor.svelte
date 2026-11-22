<script lang="ts">
  import { tick, type Snippet } from 'svelte';
  import type { Group } from '../../lib/groups.svelte';
  import { t } from '../../lib/prefs.svelte';
  import type { AIMemberRow, HumanMemberRow } from '../../lib/proto';
  import Failure from '../Failure.svelte';

  let {
    group,
    ai,
    humans,
    children,
  }: {
    group: Group;
    ai: AIMemberRow;
    humans: HumanMemberRow[];
    children: Snippet<[Snippet, Snippet]>;
  } = $props();
  const fields = ['allow', 'approvers'] as const;
  type Field = (typeof fields)[number];

  let before: AIMemberRow;
  let draft = $state<Pick<AIMemberRow, Field>>();
  let editButton = $state<HTMLButtonElement>();
  let form = $state<HTMLFormElement>();
  let busy = $state(false);
  let failed = $state('');

  const offline = $derived(!group.home.ctl);
  const people = $derived(humans.filter((person) => person.user !== ai.user));

  function differs(field: Field, list: string[]) {
    return (
      list.length !== before[field].length || list.some((user) => !before[field].includes(user))
    );
  }

  async function edit() {
    before = ai;
    draft = { allow: [...ai.allow], approvers: [...ai.approvers] };
    await tick();
    form?.focus();
  }

  async function close() {
    draft = undefined;
    failed = '';
    await tick();
    editButton?.focus();
  }

  async function save(list: Pick<AIMemberRow, Field>, changed: Field[]) {
    busy = true;
    failed = '';
    const updates = Object.fromEntries(changed.map((field) => [field, list[field]]));
    const answer = await group.home.command({
      t: 'member',
      g: group.g,
      op: 'set',
      id: ai.id,
      ...updates,
    });
    busy = false;
    if (answer.err) failed = answer.err.code;
    else await close();
  }

  function toggle(list: string[], user: string, checked: boolean) {
    return checked ? [...list, user] : list.filter((entry) => entry !== user);
  }
</script>

{#snippet accessRows()}
  {#each fields as field (field)}
    {@const names = people
      .filter((person) => ai[field].includes(person.user))
      .map((person) => person.name)}
    <dt>{t(`allow.${field}`)}</dt>
    <dd>{names.join(t('allow.sep')) || t('allow.none')}</dd>
  {/each}
{/snippet}

{#snippet editAccess()}
  {#if !draft}
    <button bind:this={editButton} type="button" class="btn btn-sm" onclick={edit}>
      {t('allow.edit')}
    </button>
  {/if}
{/snippet}

{@render children(accessRows, editAccess)}

{#if draft}
  {@const lists = draft}
  {@const changed = fields.filter((field) => differs(field, lists[field]))}
  <form
    bind:this={form}
    class="field"
    aria-label={t('allow.edit')}
    tabindex="-1"
    onsubmit={(event) => {
      event.preventDefault();
      void save(lists, changed);
    }}
  >
    {#each fields as field (field)}
      <fieldset class="field" disabled={busy || offline}>
        <legend>{t(`allow.${field}`)}</legend>
        {#each people as person (person.user)}
          <label class="check">
            <input
              type="checkbox"
              checked={lists[field].includes(person.user)}
              onchange={(event) =>
                (lists[field] = toggle(lists[field], person.user, event.currentTarget.checked))}
            />
            <span>{person.name}</span>
          </label>
        {:else}
          <p class="hint">{t('allow.none')}</p>
        {/each}
      </fieldset>
    {/each}
    {#if ai.account === 'personal'}<p class="hint">{t('allow.personal')}</p>{/if}
    {#if offline}<p class="hint">{t('allow.offline')}</p>{/if}
    {#if failed}<Failure code={failed} />{/if}
    <div class="card-foot">
      <button type="submit" class="btn btn-sm" disabled={busy || offline || !changed.length}>
        {t('allow.save')}
      </button>
      <button type="button" class="btn btn-sm" disabled={busy} onclick={close}>
        {t('allow.cancel')}
      </button>
    </div>
  </form>
{/if}

<style>
  .check {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    font-size: var(--fs-sm);
    input {
      width: 16px;
      height: 16px;
      margin: 0;
      accent-color: var(--sky);
    }
  }
</style>
