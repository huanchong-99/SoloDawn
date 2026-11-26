<script lang="ts">
  import type { Group } from '../../lib/groups.svelte';
  import { t } from '../../lib/prefs.svelte';
  import type { HumanMemberRow } from '../../lib/proto';
  import Failure from '../Failure.svelte';

  let { group, person }: { group: Group; person: HumanMemberRow } = $props();
  const authority = $derived(group.me?.role);
  const self = $derived(group.me?.id === person.id);
  const canSet = $derived(authority === 'owner' && person.role !== 'owner');
  const canRemove = $derived(
    person.role !== 'owner' &&
      (self || canSet || (authority === 'admin' && person.role === 'member')),
  );
  const next = $derived(person.role === 'admin' ? 'member' : 'admin');
  let busy = $state(false);
  let failure = $state('');
  // Removal, or leaving, takes two presses, as for an AI (RemoveAI.svelte).
  let asked = $state(false);
  const removal = $derived(self ? 'people.leave' : 'people.remove');

  function press(event: MouseEvent & { currentTarget: HTMLButtonElement }) {
    event.currentTarget.focus();
    asked = !asked;
    if (!asked) void send('remove');
  }

  function escape(event: KeyboardEvent) {
    if (!asked || event.key !== 'Escape') return;
    event.preventDefault();
    asked = false;
  }

  async function send(op: 'set' | 'remove') {
    busy = true;
    failure = '';
    const answer = await group.home.command({
      t: 'member',
      g: group.g,
      op,
      id: person.id,
      ...(op === 'set' ? { role: next } : {}),
    });
    busy = false;
    if (answer.err) failure = answer.err.code;
  }
</script>

{#if canRemove}
  <div class="person-actions row">
    {#if canSet}
      <button
        type="button"
        class="btn btn-sm"
        disabled={busy || !group.home.ctl}
        onclick={() => send('set')}
      >
        {t(`people.role.${next}`)}
      </button>
    {/if}
    <button
      type="button"
      class="btn btn-sm {asked ? 'btn-danger' : 'btn-ghost'}"
      disabled={busy || !group.home.ctl}
      onclick={press}
      onkeydown={escape}
    >
      {t(asked ? (`${removal}.confirm` as const) : removal)}
    </button>
    {#if failure === 'conflict'}
      <p class="hint" role="alert">{t('people.conflict')}</p>
    {:else if failure}
      <Failure code={failure} />
    {/if}
  </div>
{/if}

<style>
  .person-actions {
    grid-column: 2 / -1;
  }
</style>
