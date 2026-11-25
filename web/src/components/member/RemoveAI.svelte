<script lang="ts">
  import type { Group } from '../../lib/groups.svelte';
  import { openTasks } from '../../lib/members';
  import { prefs, t } from '../../lib/prefs.svelte';
  import type { AIMemberRow } from '../../lib/proto';
  import Failure from '../Failure.svelte';

  /** Takes an AI out of the group, for its owner and an admin (contracts/commands.md 1.2), in two
   *  presses; an AI on an unfinished task stays, and the first press says so. */
  let { group, ai }: { group: Group; ai: AIMemberRow } = $props();
  const uid = $props.id();
  let asked = $state(false);
  let busy = $state(false);
  let failed = $state('');

  const open = $derived(openTasks(group, ai));
  const sure = $derived(asked && !open.length);
  const tasks = $derived(
    new Intl.ListFormat(prefs.lang).format(open.map((task) => `T-${task.num}`)),
  );

  async function press(event: MouseEvent & { currentTarget: HTMLButtonElement }) {
    event.currentTarget.focus();
    failed = '';
    if (!sure) {
      asked = true;
      return;
    }
    asked = false;
    busy = true;
    const answer = await group.home.command({ t: 'member', g: group.g, op: 'remove', id: ai.id });
    busy = false;
    if (answer.err) failed = answer.err.code;
  }

  function escape(event: KeyboardEvent) {
    if (!asked || event.key !== 'Escape') return;
    event.preventDefault();
    asked = false;
  }
</script>

<button
  type="button"
  class="btn btn-sm {sure ? 'btn-danger' : 'btn-ghost'}"
  disabled={busy || !group.home.ctl}
  aria-describedby={asked ? `${uid}-why` : undefined}
  onclick={press}
  onkeydown={escape}
>
  {t(sure ? 'member.remove.confirm' : 'member.remove')}
</button>
{#if asked && open.length}
  <p class="why" id={`${uid}-why`} role="alert">
    {t('member.remove.busy')
      .replace('{name}', () => ai.name)
      .replace('{tasks}', tasks)}
  </p>
{:else if asked && ai.secretary}
  <p class="why" id={`${uid}-why`} role="status">{t('member.remove.secretary')}</p>
{:else if failed}
  <Failure code={failed} />
{/if}

<style>
  .why {
    flex-basis: 100%;
    color: var(--text-2);
    font-size: var(--fs-sm);
    animation: -global-appear var(--dur-2) var(--ease-out);
  }
</style>
