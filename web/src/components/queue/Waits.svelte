<script lang="ts">
  import { tick } from 'svelte';
  import type { Group } from '../../lib/groups.svelte';
  import { prefs, t } from '../../lib/prefs.svelte';
  import type { MsgRow, TaskRow } from '../../lib/proto';
  import { goFirst, waiting, type Wait } from '../../lib/queue';
  import { names } from '../../lib/stop';
  import type { Areas } from '../../messages/texts';
  import Failure from '../Failure.svelte';
  import Icon from '../Icon.svelte';

  /** Under a message of the reader's own that waits for an AI busy with a task: that it waits, and
   *  the choice to let it go first. Waiting needs nothing from them. */
  let { group, msg }: { group: Group; msg: MsgRow } = $props();

  let line = $state<HTMLElement>();
  let busy = $state(false);
  let failed = $state('');
  // What the message waited for when it was let go first; and that the home no longer held it, so
  // that nothing of it waits.
  let jumped = $state.raw<Wait[]>([]);
  let taken = $state(false);

  const waits = $derived(taken ? [] : waiting(group, msg));
  // The tasks that were interrupted for it and have not continued.
  const paused = $derived(
    jumped.filter(({ task }) => {
      const now = group.task(task.id);
      return now?.state === 'paused' && now.wait?.kind === 'preempted';
    }),
  );
  const still = $derived(waits.filter(({ ai }) => !paused.some((each) => each.ai.id === ai.id)));

  const tag = (task: TaskRow) => `T-${task.num}`;
  const word = (key: keyof Areas['queue'], { ai, task }: Wait) =>
    t(key)
      .replace('{task}', tag(task))
      .replace('{ai}', () => ai.name);

  /** What a `jump` does to the one AI: it interrupts a review, the session the message goes to, or
   *  a task, which pauses (flows/agent-turn.md C). */
  function effect({ ai, task }: Wait) {
    if (task.assignee !== ai.id) return 'queue.first.review';
    return msg.thread === task.id ? 'queue.first.turn' : 'queue.first';
  }
  const offer = $derived.by(() => {
    if (waits.length === 1) return word(effect(waits[0]), waits[0]);
    const tasks = [...new Set(waits.map(({ task }) => tag(task)))].map((name) => ({ name }));
    return t('queue.first.all').replace('{tasks}', names(tasks, prefs.lang));
  });

  async function first() {
    const all = waits;
    busy = true;
    failed = '';
    const code = (await goFirst(group, msg)).err?.code;
    busy = false;
    if (code === 'conflict') taken = true;
    else if (code) failed = code;
    else jumped = all;
    await tick();
    line?.focus();
  }
</script>

{#each paused as wait (wait.ai.id)}
  <p class="wait" tabindex="-1" bind:this={line}>
    <Icon name="pause" sm /><span>{word('queue.paused', wait)}</span>
  </p>
{/each}
{#each still as wait, i (wait.ai.id)}
  <p class="wait" tabindex="-1" bind:this={line}>
    <Icon name="clock" sm />
    <span role="status">{word(jumped.length ? 'queue.going' : 'queue.waits', wait)}</span>
    {#if !jumped.length && i === still.length - 1}
      <button
        type="button"
        class="go"
        disabled={busy || !group.home.ctl}
        aria-busy={busy}
        onclick={() => void first()}
      >
        {offer}
      </button>
    {/if}
  </p>
{/each}
{#if failed && waits.length}<Failure code={failed} />{/if}

<style>
  /* Under a message: a quiet line that says an AI is busy, with the offer as a link. */
  .wait {
    display: flex;
    align-items: baseline;
    flex-wrap: wrap;
    gap: 2px 6px;
    padding-inline: 2px;
    color: var(--text-3);
    font-size: var(--fs-xs);
    outline-offset: 2px;
    border-radius: var(--r-sm);
    animation: appear var(--dur-2) var(--ease-out);
  }
  .wait :global(.i) {
    align-self: center;
    width: 14px;
    height: 14px;
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
