<script lang="ts">
  import type { Group } from '../../lib/groups.svelte';
  import { t } from '../../lib/prefs.svelte';
  import type { HumanMemberRow } from '../../lib/proto';
  import Failure from '../Failure.svelte';

  let { group, person }: { group: Group; person: HumanMemberRow } = $props();
  const authority = $derived((group.me as HumanMemberRow | undefined)?.role);
  const self = $derived(group.me?.id === person.id);
  const canSet = $derived(authority === 'owner' && person.role !== 'owner');
  const canRemove = $derived(
    person.role !== 'owner' &&
      (self || canSet || (authority === 'admin' && person.role === 'member')),
  );
  const next = $derived(person.role === 'admin' ? 'member' : 'admin');
  let busy = $state(false);
  let failure = $state('');

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
{#if canRemove}
  <button
    type="button"
    class="btn btn-sm btn-ghost"
    disabled={busy || !group.home.ctl}
    onclick={() => send('remove')}
  >
    {t(self ? 'people.leave' : 'people.remove')}
  </button>
{/if}
{#if failure === 'conflict'}
  <p class="hint" role="alert">{t('people.conflict')}</p>
{:else if failure}
  <Failure code={failure} />
{/if}
