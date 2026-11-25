<script lang="ts">
  import { tick } from 'svelte';
  import type { Group } from '../../lib/groups.svelte';
  import { t } from '../../lib/prefs.svelte';
  import type { MsgRow } from '../../lib/proto';
  import type { Areas } from '../../messages/texts';
  import { goFirst, waiting, type Wait } from '../../lib/queue';
  import Failure from '../Failure.svelte';
  import Icon from '../Icon.svelte';

  /** Under a message of the reader's own that waits for an AI busy with a task: that it waits, and
   *  the choice to let it go first. Waiting needs nothing from them. */
  let { group, msg }: { group: Group; msg: MsgRow } = $props();

  let line = $state<HTMLElement>();
  let busy = $state(false);
  let failed = $state('');
  // The wait that was let go first; and that the home no longer held it, so that none is left.
  let jumped = $state.raw<Wait>();
  let taken = $state(false);

  const waits = $derived(taken ? [] : waiting(group, msg));
  const task = $derived(group.task(jumped?.task?.id));
  const pausedFor = $derived(
    task?.state === 'paused' && task.wait?.kind === 'preempted' ? task : undefined,
  );

  const word = (key: keyof Areas['queue'], wait: Wait) =>
    t(key)
      .replace('{task}', `T-${wait.task!.num}`)
      .replace('{ai}', () => wait.ai.name);

  async function first(wait: Wait) {
    busy = true;
    failed = '';
    const code = (await goFirst(group, msg)).err?.code;
    busy = false;
    if (code === 'conflict') taken = true;
    else if (code) failed = code;
    else jumped = wait;
    await tick();
    line?.focus();
  }
</script>

{#if jumped && pausedFor}
  <p class="wait" tabindex="-1" bind:this={line}>
    <Icon name="pause" sm /><span>{word('queue.paused', jumped)}</span>
  </p>
{:else}
  {#each waits as { wait, first: can } (wait.ai.id)}
    {@const going = jumped?.ai.id === wait.ai.id}
    <p class="wait" tabindex="-1" bind:this={line}>
      <Icon name="clock" sm />
      <span role="status">{word(going ? 'queue.going' : 'queue.waits', wait)}</span>
      {#if can && !going}
        <button
          type="button"
          class="go"
          disabled={busy || !group.home.ctl}
          aria-busy={busy}
          onclick={() => void first(wait)}
        >
          {word(wait.task!.assignee === wait.ai.id ? 'queue.first' : 'queue.first.review', wait)}
        </button>
      {/if}
    </p>
  {/each}
{/if}
{#if failed}<Failure code={failed} />{/if}

<style>
  .wait {
    display: flex;
    align-items: baseline;
    flex-wrap: wrap;
    gap: 2px 6px;
    color: var(--text-3);
    font-size: var(--fs-xs);
    outline-offset: 2px;
    border-radius: var(--r-sm);
    animation: appear var(--dur-3) var(--ease-out);
  }
  .wait :global(.i) {
    align-self: center;
  }
  .go {
    padding: 0;
    border: 0;
    border-radius: var(--r-sm);
    background: none;
    color: var(--sky);
    font-weight: 500;
  }
  .go:hover:not([disabled]) {
    text-decoration: underline;
  }
  .go[disabled] {
    color: var(--text-3);
    cursor: default;
  }
  @media (pointer: coarse) {
    .go {
      min-height: 36px;
    }
  }
</style>
