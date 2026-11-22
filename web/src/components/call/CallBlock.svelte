<script lang="ts">
  import { callView } from '../../lib/call';
  import type { Part } from '../../lib/compose';
  import type { CardProps } from '../../lib/mounts.svelte';
  import { now } from '../../lib/when';
  import Avatar from '../Avatar.svelte';

  /** An AI-to-AI call block with route, question, waiting or elapsed time, and inline reply. */
  let { group, msg }: CardProps<'call'> = $props();

  const view = $derived(callView(group, msg, msg.state.status === 'waiting' ? now() : 0));
</script>

{#snippet richText(segs: Part[])}
  {#each segs as part, i (i)}
    {#if part.kind === 'code'}<code>{part.text}</code>{:else if part.kind === 'at'}<span class="at">
        {part.text}
      </span>{:else}{part.text}{/if}
  {/each}
{/snippet}

<div class="call">
  <p class="call-route">
    <span>{view.caller}</span><svg class="i sm" viewBox="0 0 24 24" aria-hidden="true">
      <path d="M5 12h13M13 6l6 6-6 6" />
    </svg><span class="sr">→</span><span>{view.callee}</span><span class="muted">
      {view.mode} ·
      {#if view.waiting}
        <span class="st" data-st="waiting"><i></i>{view.timing}</span>
      {:else}
        {view.timing}
      {/if}
      · {view.depth}
    </span>
  </p>

  {#if view.segs.length}
    <p class="text">{@render richText(view.segs)}</p>
  {/if}

  {#if view.reply}
    <div class="reply">
      <Avatar id={view.reply.by} name={view.reply.name} cli={view.reply.cli} size="sm" />
      <p class="text">
        <b>{view.reply.name}</b>{' '}{@render richText(view.reply.segs)}
      </p>
    </div>
  {/if}
</div>

<style>
  .call {
    display: flex;
    flex-direction: column;
    gap: 6px;
    padding: 8px 10px;
    border: 1px solid var(--line);
    border-left: 2px solid var(--sky);
    border-radius: var(--r-md);
    background: var(--surface);
    font-size: var(--fs-sm);
    min-width: 0;
  }
  .call-route {
    display: flex;
    align-items: center;
    gap: 6px;
    flex-wrap: wrap;
    font-weight: 600;
  }
  .call-route .muted {
    font-weight: 400;
    font-size: var(--fs-xs);
  }
  .call-route .st {
    color: inherit;
    font-size: inherit;
    font-weight: inherit;
    vertical-align: baseline;
  }
  .text {
    white-space: pre-wrap;
    overflow-wrap: anywhere;
  }
  code {
    padding: 1px 5px;
    border-radius: var(--r-sm);
    background: var(--surface-2);
  }
  .reply {
    display: grid;
    grid-template-columns: 22px minmax(0, 1fr);
    gap: var(--sp-2);
    padding-top: 6px;
    border-top: 1px dashed var(--line);
  }
  .reply > :global(.av) {
    margin-top: 1px;
  }
</style>
